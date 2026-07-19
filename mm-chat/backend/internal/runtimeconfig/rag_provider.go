package runtimeconfig

import (
	"context"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	providerConfigKindRAG           = "rag"
	ragProviderRecordPrefix         = "RAG:"
	ragConnectionFingerprintVersion = "rag-provider-connection/v1"
	ragConnectionTestTimeout        = 40 * time.Second
	ragMaxAPIKeyBytes               = 4096
	minerUMaxResponseBytes          = 1 << 20
	jinaMaxResponseBytes            = 16 << 20

	minerUAllocateURL      = ragproviders.MinerUAllocateEndpoint
	minerUUploadHost       = ragproviders.MinerUUploadHost
	minerUUploadPathPrefix = ragproviders.MinerUUploadPathPrefix
	minerUModelVersion     = ragproviders.MinerUModelVersion

	jinaEmbeddingsURL  = ragproviders.JinaEmbeddingsEndpoint
	jinaRerankURL      = ragproviders.JinaRerankEndpoint
	jinaEmbeddingModel = ragproviders.JinaEmbeddingModel
	jinaRerankModel    = ragproviders.JinaRerankModel
	jinaDimensions     = ragproviders.JinaEmbeddingDimensions

	ragConnectionSentinel = "mm-chat provider connection test"
)

type RAGProviderID string

const (
	RAGProviderMinerU RAGProviderID = "mineru"
	RAGProviderJina   RAGProviderID = "jina"
)

var supportedRAGProviderIDs = []RAGProviderID{
	RAGProviderMinerU,
	RAGProviderJina,
}

var (
	ErrRAGProviderConfigUnsupported      = errors.New("rag provider configuration is unsupported")
	ErrRAGProviderNotFound               = errors.New("rag provider configuration was not found")
	ErrRAGProviderSecretRequired         = errors.New("rag provider api key is required")
	ErrRAGProviderSecretVaultUnavailable = errors.New("rag provider secret vault is unavailable")
	ErrRAGProviderSecretInvalid          = errors.New("rag provider secret is invalid")
	ErrRAGProviderConnectionFailed       = errors.New("rag provider connection test failed")
	ErrRAGProviderConfigChanged          = errors.New("rag provider configuration changed during connection testing")
)

type AdminRAGProviderConfigResponse struct {
	ID                  string        `json:"id"`
	Name                string        `json:"name"`
	Provider            RAGProviderID `json:"provider"`
	Enabled             bool          `json:"enabled"`
	HasAPIKey           bool          `json:"hasApiKey"`
	ConnectionTestValid bool          `json:"connectionTestValid"`
	ConnectionTestedAt  *time.Time    `json:"connectionTestedAt,omitempty"`
	EmbeddingModel      string        `json:"embeddingModel,omitempty"`
	EmbeddingDimensions int           `json:"embeddingDimensions,omitempty"`
	RerankModel         string        `json:"rerankModel,omitempty"`
	ParserModel         string        `json:"parserModel,omitempty"`
}

type AdminRAGProviderConfigsResponse struct {
	Providers []AdminRAGProviderConfigResponse `json:"providers"`
}

type UpdateAdminRAGProviderConfigRequest struct {
	Name         string         `json:"name"`
	Enabled      bool           `json:"enabled"`
	APIKeySecret map[string]any `json:"apiKeySecret"`
	ClearAPIKey  bool           `json:"clearApiKey"`
}

type AdminRAGProviderConnectionResponse struct {
	Provider AdminRAGProviderConfigResponse `json:"provider"`
	Checks   []string                       `json:"checks"`
}

type CommitRAGProviderConnectionInput struct {
	ID                         string
	UserID                     string
	ProviderID                 string
	ExpectedEncryptedSecretRef string
	ExpectedRAGProvider        string
	ExpectedEnabled            bool
	ConnectionTestSHA256       string
	ConnectionTestedAt         time.Time
	Enabled                    bool
}

type ragProviderConnectionRepository interface {
	CommitRAGProviderConnection(
		context.Context,
		CommitRAGProviderConnectionInput,
	) (StoredProviderConfig, error)
}

func WithRAGProviderHTTPClient(client websearch.HTTPDoer) ServiceOption {
	return func(s *Service) {
		s.ragHTTPClient = client
	}
}

func isReservedRAGProviderRecordID(providerID string) bool {
	return strings.HasPrefix(
		strings.ToUpper(strings.TrimSpace(providerID)),
		ragProviderRecordPrefix,
	)
}

func isRAGProviderConfig(stored StoredProviderConfig) bool {
	return strings.TrimSpace(stored.Config.Kind) == providerConfigKindRAG
}

func normalizeRAGProviderID(value string) (RAGProviderID, error) {
	providerID := RAGProviderID(strings.ToLower(strings.TrimSpace(value)))
	for _, supported := range supportedRAGProviderIDs {
		if providerID == supported {
			return providerID, nil
		}
	}
	return "", ErrRAGProviderConfigUnsupported
}

func ragProviderRecordID(providerID RAGProviderID) string {
	return ragProviderRecordPrefix + strings.ToUpper(string(providerID))
}

func ragProviderDefaultName(providerID RAGProviderID) string {
	if providerID == RAGProviderMinerU {
		return "MinerU"
	}
	if providerID == RAGProviderJina {
		return "Jina AI"
	}
	return "RAG Provider"
}

func ragProviderIngressContext(providerID RAGProviderID) string {
	return "provider:rag:" + string(providerID)
}

func ragProviderSecretContext(userID string, recordID string) string {
	return "provider:rag:" + strings.TrimSpace(userID) + ":" + strings.TrimSpace(recordID)
}

func (s *Service) AdminRAGProviderConfigs(
	ctx context.Context,
) (AdminRAGProviderConfigsResponse, error) {
	if s.repo == nil {
		return AdminRAGProviderConfigsResponse{}, ErrDatabaseRequired
	}
	stored, err := s.repo.ListProviderConfigs(ctx, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return AdminRAGProviderConfigsResponse{}, err
	}
	byProvider := make(map[RAGProviderID]AdminRAGProviderConfigResponse)
	for _, item := range stored {
		if !isRAGProviderConfig(item) {
			continue
		}
		providerID, err := validateStoredRAGProvider(item)
		if err != nil {
			return AdminRAGProviderConfigsResponse{}, err
		}
		if _, duplicate := byProvider[providerID]; duplicate {
			return AdminRAGProviderConfigsResponse{}, ErrRAGProviderConfigUnsupported
		}
		byProvider[providerID] = adminRAGProviderResponse(item, providerID)
	}
	providers := make([]AdminRAGProviderConfigResponse, 0, len(byProvider))
	for _, providerID := range supportedRAGProviderIDs {
		if response, ok := byProvider[providerID]; ok {
			providers = append(providers, response)
		}
	}
	return AdminRAGProviderConfigsResponse{Providers: providers}, nil
}

func (s *Service) UpsertAdminRAGProviderConfig(
	ctx context.Context,
	providerValue string,
	request UpdateAdminRAGProviderConfigRequest,
) (AdminRAGProviderConfigResponse, error) {
	if s.repo == nil {
		return AdminRAGProviderConfigResponse{}, ErrDatabaseRequired
	}
	providerID, err := normalizeRAGProviderID(providerValue)
	if err != nil || (request.ClearAPIKey && len(request.APIKeySecret) > 0) {
		return AdminRAGProviderConfigResponse{}, ErrRAGProviderConfigUnsupported
	}
	recordID := ragProviderRecordID(providerID)
	user := auth.UserOrDevelopment(ctx)
	current, hasCurrent, err := s.repo.GetProviderConfig(ctx, user.ID, recordID)
	if err != nil {
		return AdminRAGProviderConfigResponse{}, err
	}
	if hasCurrent {
		if _, err := validateStoredRAGProvider(current); err != nil {
			return AdminRAGProviderConfigResponse{}, err
		}
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = ragProviderDefaultName(providerID)
	}
	if len([]byte(name)) > 128 {
		return AdminRAGProviderConfigResponse{}, ErrRAGProviderConfigUnsupported
	}
	secretRef := ""
	if hasCurrent {
		secretRef = strings.TrimSpace(current.EncryptedSecretRef)
	}
	if request.ClearAPIKey {
		secretRef = ""
	} else if len(request.APIKeySecret) > 0 {
		envelope, err := parseEncryptedSecretEnvelope(request.APIKeySecret)
		if err != nil {
			return AdminRAGProviderConfigResponse{}, err
		}
		plaintext, err := s.DecryptOptionalSecret(
			envelope,
			ragProviderIngressContext(providerID),
		)
		if err != nil {
			return AdminRAGProviderConfigResponse{}, err
		}
		secretRef, err = s.encryptRAGProviderSecretAtRest(
			user.ID,
			recordID,
			plaintext,
		)
		if err != nil {
			return AdminRAGProviderConfigResponse{}, err
		}
	}

	connectionTestSHA256 := ""
	connectionTestedAt := ""
	connectionTestValid := false
	if hasCurrent {
		connectionTestSHA256 = strings.TrimSpace(current.Config.ConnectionTestSHA256)
		connectionTestedAt = strings.TrimSpace(current.Config.ConnectionTestedAt)
		connectionTestValid = ragProviderConnectionTestValidForValues(
			recordID,
			providerID,
			secretRef,
			connectionTestSHA256,
			connectionTestedAt,
		)
	}
	if !connectionTestValid {
		connectionTestSHA256 = ""
		connectionTestedAt = ""
	}
	enabled := hasCurrent && current.Config.Enabled && request.Enabled && connectionTestValid
	stored, err := s.repo.UpsertProviderConfig(ctx, UpsertProviderConfigInput{
		UserID: user.ID, ProviderID: recordID, Label: name,
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(providerID),
			Enabled: enabled, ConnectionTestSHA256: connectionTestSHA256,
			ConnectionTestedAt: connectionTestedAt,
		},
	})
	if err != nil {
		return AdminRAGProviderConfigResponse{}, err
	}
	return adminRAGProviderResponse(stored, providerID), nil
}

func (s *Service) DeleteAdminRAGProviderConfig(
	ctx context.Context,
	providerValue string,
) error {
	if s.repo == nil {
		return ErrDatabaseRequired
	}
	providerID, err := normalizeRAGProviderID(providerValue)
	if err != nil {
		return err
	}
	recordID := ragProviderRecordID(providerID)
	userID := auth.UserOrDevelopment(ctx).ID
	stored, ok, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRAGProviderNotFound
	}
	if _, err := validateStoredRAGProvider(stored); err != nil {
		return err
	}
	if err := s.repo.DeleteProviderConfig(ctx, userID, recordID); err != nil {
		if errors.Is(err, ErrProviderConfigNotFound) {
			return ErrRAGProviderNotFound
		}
		return err
	}
	return nil
}

func (s *Service) TestAdminRAGProviderConnection(
	ctx context.Context,
	providerValue string,
) (AdminRAGProviderConnectionResponse, error) {
	return s.commitAdminRAGProviderConnection(ctx, providerValue, false)
}

func (s *Service) ActivateAdminRAGProvider(
	ctx context.Context,
	providerValue string,
) (AdminRAGProviderConnectionResponse, error) {
	return s.commitAdminRAGProviderConnection(ctx, providerValue, true)
}

func (s *Service) commitAdminRAGProviderConnection(
	ctx context.Context,
	providerValue string,
	activate bool,
) (AdminRAGProviderConnectionResponse, error) {
	if s.repo == nil {
		return AdminRAGProviderConnectionResponse{}, ErrDatabaseRequired
	}
	committer, ok := s.repo.(ragProviderConnectionRepository)
	if !ok {
		return AdminRAGProviderConnectionResponse{}, ErrDatabaseRequired
	}
	providerID, err := normalizeRAGProviderID(providerValue)
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	recordID := ragProviderRecordID(providerID)
	userID := auth.UserOrDevelopment(ctx).ID
	stored, found, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	if !found {
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderNotFound
	}
	if _, err := validateStoredRAGProvider(stored); err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	apiKey, err := s.decryptStoredRAGProviderSecret(stored)
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	if apiKey == "" {
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderSecretRequired
	}
	if !validRAGAPIKey(apiKey) {
		apiKey = ""
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderConnectionFailed
	}
	testCtx, cancel := context.WithTimeout(ctx, ragConnectionTestTimeout)
	checks, err := s.testRAGProviderConnection(testCtx, providerID, apiKey)
	cancel()
	apiKey = ""
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderConnectionFailed
	}
	fingerprint := ragProviderConnectionFingerprint(
		recordID,
		providerID,
		stored.EncryptedSecretRef,
	)
	committed, err := committer.CommitRAGProviderConnection(
		ctx,
		CommitRAGProviderConnectionInput{
			ID: stored.ID, UserID: stored.UserID, ProviderID: stored.ProviderID,
			ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
			ExpectedRAGProvider:        string(providerID),
			ExpectedEnabled:            stored.Config.Enabled,
			ConnectionTestSHA256:       fingerprint,
			ConnectionTestedAt:         time.Now().UTC(),
			Enabled:                    activate || stored.Config.Enabled,
		},
	)
	if err != nil {
		if errors.Is(err, ErrProviderConfigChanged) {
			return AdminRAGProviderConnectionResponse{}, ErrRAGProviderConfigChanged
		}
		return AdminRAGProviderConnectionResponse{}, err
	}
	return AdminRAGProviderConnectionResponse{
		Provider: adminRAGProviderResponse(committed, providerID),
		Checks:   checks,
	}, nil
}

func validateStoredRAGProvider(
	stored StoredProviderConfig,
) (RAGProviderID, error) {
	if !isRAGProviderConfig(stored) {
		return "", ErrRAGProviderConfigUnsupported
	}
	providerID, err := normalizeRAGProviderID(stored.Config.RAGProvider)
	if err != nil || stored.ProviderID != ragProviderRecordID(providerID) {
		return "", ErrRAGProviderConfigUnsupported
	}
	return providerID, nil
}

func adminRAGProviderResponse(
	stored StoredProviderConfig,
	providerID RAGProviderID,
) AdminRAGProviderConfigResponse {
	connectionValid := RAGProviderConnectionTestValid(stored)
	var testedAt *time.Time
	if connectionValid {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(stored.Config.ConnectionTestedAt))
		if err == nil {
			utc := parsed.UTC()
			testedAt = &utc
		}
	}
	response := AdminRAGProviderConfigResponse{
		ID:       ragProviderRecordID(providerID),
		Name:     nonEmpty(stored.Label, ragProviderDefaultName(providerID)),
		Provider: providerID, Enabled: stored.Config.Enabled && connectionValid,
		HasAPIKey:           strings.TrimSpace(stored.EncryptedSecretRef) != "",
		ConnectionTestValid: connectionValid, ConnectionTestedAt: testedAt,
	}
	if providerID == RAGProviderMinerU {
		response.ParserModel = minerUModelVersion
	} else if providerID == RAGProviderJina {
		response.EmbeddingModel = jinaEmbeddingModel
		response.EmbeddingDimensions = config.DefaultRAGJinaDimensions
		response.RerankModel = jinaRerankModel
	}
	return response
}
