package runtimeconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	providerConfigKindModel            = "model"
	providerConfigKindSearch           = "search"
	searchProviderRecordPrefix         = "SEARCH:"
	searchConnectionFingerprintVersion = "search-provider-connection/v1"
	searchConnectionTestTimeout        = 15 * time.Second
	searchConnectionTestQuery          = "OpenAI official documentation"
)

var (
	ErrSearchProviderConfigUnsupported = errors.New("search provider configuration is unsupported")
	ErrSearchProviderNotFound          = errors.New("search provider configuration was not found")
	ErrSearchProviderSecretRequired    = errors.New("search provider API key is required")
	ErrSearchProviderConnectionFailed  = errors.New("search provider connection test failed")
	ErrSearchProviderConfigChanged     = errors.New("search provider configuration changed during connection testing")
)

var supportedSearchProviderIDs = []websearch.ProviderID{
	websearch.ProviderTavily,
	websearch.ProviderFirecrawl,
	websearch.ProviderExa,
	websearch.ProviderBocha,
}

func WithSearchProviderHTTPClient(client websearch.HTTPDoer) ServiceOption {
	return func(s *Service) {
		s.searchHTTPClient = client
	}
}

type AdminSearchProviderConfigResponse struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Provider            websearch.ProviderID `json:"provider"`
	BaseURL             string               `json:"baseUrl"`
	Enabled             bool                 `json:"enabled"`
	HasAPIKey           bool                 `json:"hasApiKey"`
	ConnectionTestValid bool                 `json:"connectionTestValid"`
	ConnectionTestedAt  *time.Time           `json:"connectionTestedAt,omitempty"`
}

type AdminSearchProviderConfigsResponse struct {
	Providers        []AdminSearchProviderConfigResponse `json:"providers"`
	ActiveProviderID string                              `json:"activeProviderId,omitempty"`
}

type UpdateAdminSearchProviderConfigRequest struct {
	Name         string         `json:"name"`
	BaseURL      string         `json:"baseUrl"`
	Enabled      bool           `json:"enabled"`
	APIKeySecret map[string]any `json:"apiKeySecret"`
	ClearAPIKey  bool           `json:"clearApiKey"`
}

type AdminSearchProviderConnectionResponse struct {
	Provider    AdminSearchProviderConfigResponse `json:"provider"`
	SourceCount int                               `json:"sourceCount"`
	ImageCount  int                               `json:"imageCount"`
}

type CommitSearchProviderConnectionInput struct {
	ID                         string
	UserID                     string
	ProviderID                 string
	ExpectedEncryptedSecretRef string
	ExpectedSearchProvider     string
	ExpectedBaseURL            string
	ExpectedEnabled            bool
	ConnectionTestSHA256       string
	ConnectionTestedAt         time.Time
	Enabled                    bool
}

func IsModelProviderConfig(stored StoredProviderConfig) bool {
	kind := strings.TrimSpace(stored.Config.Kind)
	return kind == "" || kind == providerConfigKindModel
}

func isSearchProviderConfig(stored StoredProviderConfig) bool {
	return strings.TrimSpace(stored.Config.Kind) == providerConfigKindSearch
}

func isReservedSearchProviderRecordID(providerID string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(providerID)), searchProviderRecordPrefix)
}

func searchProviderRecordID(providerID websearch.ProviderID) string {
	return searchProviderRecordPrefix + strings.ToUpper(string(providerID))
}

func normalizeSearchProviderID(value string) (websearch.ProviderID, error) {
	providerID := websearch.ProviderID(strings.ToLower(strings.TrimSpace(value)))
	for _, supported := range supportedSearchProviderIDs {
		if providerID == supported {
			return providerID, nil
		}
	}
	return "", ErrSearchProviderConfigUnsupported
}

func searchProviderDefaultName(providerID websearch.ProviderID) string {
	switch providerID {
	case websearch.ProviderTavily:
		return "Tavily"
	case websearch.ProviderFirecrawl:
		return "Firecrawl"
	case websearch.ProviderExa:
		return "Exa"
	case websearch.ProviderBocha:
		return "Bocha"
	default:
		return "Search Provider"
	}
}

func searchProviderIngressContext(providerID websearch.ProviderID) string {
	return "provider:search:" + string(providerID)
}

func searchProviderSecretContext(userID string, recordID string) string {
	return "provider:search:" + strings.TrimSpace(userID) + ":" + strings.TrimSpace(recordID)
}

func storedProviderSecretContext(
	userID string,
	recordID string,
	payload StoredProviderConfigPayload,
) (string, bool) {
	switch strings.TrimSpace(payload.Kind) {
	case "", providerConfigKindModel:
		return modelProviderSecretContext(userID, recordID), true
	case providerConfigKindSearch:
		providerID, err := normalizeSearchProviderID(payload.SearchProvider)
		if err != nil || recordID != searchProviderRecordID(providerID) {
			return "", false
		}
		return searchProviderSecretContext(userID, recordID), true
	default:
		return "", false
	}
}

func (s *Service) AdminSearchProviderConfigs(
	ctx context.Context,
) (AdminSearchProviderConfigsResponse, error) {
	if s.repo == nil {
		return AdminSearchProviderConfigsResponse{}, ErrDatabaseRequired
	}
	stored, err := s.repo.ListProviderConfigs(ctx, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return AdminSearchProviderConfigsResponse{}, err
	}
	byProvider := make(map[websearch.ProviderID]AdminSearchProviderConfigResponse)
	activeProviderID := ""
	for _, item := range stored {
		if !isSearchProviderConfig(item) {
			continue
		}
		providerID, err := validateStoredSearchProvider(item)
		if err != nil {
			return AdminSearchProviderConfigsResponse{}, err
		}
		response := adminSearchProviderResponse(item, providerID)
		if _, duplicate := byProvider[providerID]; duplicate {
			return AdminSearchProviderConfigsResponse{}, ErrSearchProviderConfigUnsupported
		}
		byProvider[providerID] = response
		if response.Enabled {
			if activeProviderID != "" {
				return AdminSearchProviderConfigsResponse{}, ErrSearchProviderConfigUnsupported
			}
			activeProviderID = string(providerID)
		}
	}
	providers := make([]AdminSearchProviderConfigResponse, 0, len(byProvider))
	for _, providerID := range supportedSearchProviderIDs {
		if response, ok := byProvider[providerID]; ok {
			providers = append(providers, response)
		}
	}
	return AdminSearchProviderConfigsResponse{
		Providers: providers, ActiveProviderID: activeProviderID,
	}, nil
}

func (s *Service) UpsertAdminSearchProviderConfig(
	ctx context.Context,
	providerValue string,
	request UpdateAdminSearchProviderConfigRequest,
) (AdminSearchProviderConfigResponse, error) {
	if s.repo == nil {
		return AdminSearchProviderConfigResponse{}, ErrDatabaseRequired
	}
	providerID, err := normalizeSearchProviderID(providerValue)
	if err != nil || (request.ClearAPIKey && len(request.APIKeySecret) > 0) {
		return AdminSearchProviderConfigResponse{}, ErrSearchProviderConfigUnsupported
	}
	recordID := searchProviderRecordID(providerID)
	user := auth.UserOrDevelopment(ctx)
	current, hasCurrent, err := s.repo.GetProviderConfig(ctx, user.ID, recordID)
	if err != nil {
		return AdminSearchProviderConfigResponse{}, err
	}
	if hasCurrent {
		if _, err := validateStoredSearchProvider(current); err != nil {
			return AdminSearchProviderConfigResponse{}, err
		}
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = searchProviderDefaultName(providerID)
	}
	if len(name) > 128 {
		return AdminSearchProviderConfigResponse{}, ErrSearchProviderConfigUnsupported
	}
	baseURL := strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	if _, err := websearch.NewProvider(providerID, websearch.Config{
		APIKey: "configuration-validation", BaseURL: baseURL,
	}); err != nil {
		return AdminSearchProviderConfigResponse{}, ErrSearchProviderConfigUnsupported
	}
	secretRef := ""
	if hasCurrent {
		secretRef = strings.TrimSpace(current.EncryptedSecretRef)
	}
	if request.ClearAPIKey {
		secretRef = ""
	}
	if len(request.APIKeySecret) > 0 {
		envelope, err := parseEncryptedSecretEnvelope(request.APIKeySecret)
		if err != nil {
			return AdminSearchProviderConfigResponse{}, err
		}
		plaintext, err := s.DecryptOptionalSecret(
			envelope,
			searchProviderIngressContext(providerID),
		)
		if err != nil {
			return AdminSearchProviderConfigResponse{}, err
		}
		secretRef, err = s.encryptSearchProviderSecretAtRest(
			user.ID,
			recordID,
			plaintext,
		)
		if err != nil {
			return AdminSearchProviderConfigResponse{}, err
		}
	}

	connectionTestSHA256 := ""
	connectionTestedAt := ""
	connectionTestValid := false
	if hasCurrent {
		connectionTestSHA256 = strings.TrimSpace(current.Config.ConnectionTestSHA256)
		connectionTestedAt = strings.TrimSpace(current.Config.ConnectionTestedAt)
		connectionTestValid = searchProviderConnectionTestValidForValues(
			recordID,
			providerID,
			baseURL,
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
			Kind: providerConfigKindSearch, SearchProvider: string(providerID),
			BaseURL: baseURL, Enabled: enabled,
			ConnectionTestSHA256: connectionTestSHA256,
			ConnectionTestedAt:   connectionTestedAt,
		},
	})
	if err != nil {
		return AdminSearchProviderConfigResponse{}, err
	}
	return adminSearchProviderResponse(stored, providerID), nil
}

func (s *Service) DeleteAdminSearchProviderConfig(
	ctx context.Context,
	providerValue string,
) error {
	if s.repo == nil {
		return ErrDatabaseRequired
	}
	providerID, err := normalizeSearchProviderID(providerValue)
	if err != nil {
		return err
	}
	recordID := searchProviderRecordID(providerID)
	userID := auth.UserOrDevelopment(ctx).ID
	stored, ok, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSearchProviderNotFound
	}
	if _, err := validateStoredSearchProvider(stored); err != nil {
		return err
	}
	if err := s.repo.DeleteProviderConfig(ctx, userID, recordID); err != nil {
		if errors.Is(err, ErrProviderConfigNotFound) {
			return ErrSearchProviderNotFound
		}
		return err
	}
	return nil
}

func (s *Service) TestAdminSearchProviderConnection(
	ctx context.Context,
	providerValue string,
) (AdminSearchProviderConnectionResponse, error) {
	return s.commitAdminSearchProviderConnection(ctx, providerValue, false)
}

func (s *Service) ActivateAdminSearchProvider(
	ctx context.Context,
	providerValue string,
) (AdminSearchProviderConnectionResponse, error) {
	return s.commitAdminSearchProviderConnection(ctx, providerValue, true)
}

func (s *Service) commitAdminSearchProviderConnection(
	ctx context.Context,
	providerValue string,
	activate bool,
) (AdminSearchProviderConnectionResponse, error) {
	if s.repo == nil {
		return AdminSearchProviderConnectionResponse{}, ErrDatabaseRequired
	}
	providerID, err := normalizeSearchProviderID(providerValue)
	if err != nil {
		return AdminSearchProviderConnectionResponse{}, err
	}
	recordID := searchProviderRecordID(providerID)
	userID := auth.UserOrDevelopment(ctx).ID
	stored, ok, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		return AdminSearchProviderConnectionResponse{}, err
	}
	if !ok {
		return AdminSearchProviderConnectionResponse{}, ErrSearchProviderNotFound
	}
	if _, err := validateStoredSearchProvider(stored); err != nil {
		return AdminSearchProviderConnectionResponse{}, err
	}
	apiKey, err := s.decryptStoredSearchProviderSecret(stored)
	if err != nil {
		return AdminSearchProviderConnectionResponse{}, err
	}
	if apiKey == "" {
		return AdminSearchProviderConnectionResponse{}, ErrSearchProviderSecretRequired
	}
	provider, err := websearch.NewProvider(providerID, websearch.Config{
		APIKey: apiKey, BaseURL: stored.Config.BaseURL, Client: s.searchHTTPClient,
	})
	apiKey = ""
	if err != nil {
		return AdminSearchProviderConnectionResponse{}, ErrSearchProviderConfigUnsupported
	}
	testCtx, cancel := context.WithTimeout(ctx, searchConnectionTestTimeout)
	result, err := provider.Search(testCtx, websearch.Request{
		Query: searchConnectionTestQuery, Scope: websearch.ScopeGeneral, MaxResults: 1,
	})
	cancel()
	if err != nil {
		return AdminSearchProviderConnectionResponse{}, ErrSearchProviderConnectionFailed
	}
	fingerprint := searchProviderConnectionFingerprint(
		recordID,
		providerID,
		stored.Config.BaseURL,
		stored.EncryptedSecretRef,
	)
	committed, err := s.repo.CommitSearchProviderConnection(
		ctx,
		CommitSearchProviderConnectionInput{
			ID: stored.ID, UserID: stored.UserID, ProviderID: stored.ProviderID,
			ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
			ExpectedSearchProvider:     string(providerID),
			ExpectedBaseURL:            stored.Config.BaseURL,
			ExpectedEnabled:            stored.Config.Enabled,
			ConnectionTestSHA256:       fingerprint,
			ConnectionTestedAt:         time.Now().UTC(),
			Enabled:                    activate || stored.Config.Enabled,
		},
	)
	if err != nil {
		if errors.Is(err, ErrProviderConfigChanged) {
			return AdminSearchProviderConnectionResponse{}, ErrSearchProviderConfigChanged
		}
		return AdminSearchProviderConnectionResponse{}, err
	}
	return AdminSearchProviderConnectionResponse{
		Provider:    adminSearchProviderResponse(committed, providerID),
		SourceCount: len(result.Sources),
		ImageCount:  len(result.Images),
	}, nil
}

// ResolveActive implements websearch.Resolver from server-owned Postgres/vault
// state. An active external provider never falls back after resolution.
func (s *Service) ResolveActive(ctx context.Context) (websearch.ActiveExecution, error) {
	if s == nil || s.repo == nil {
		return websearch.ActiveExecution{}, websearch.ErrNotConfigured
	}
	stored, err := s.repo.ListProviderConfigs(ctx, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return websearch.ActiveExecution{}, websearch.ErrResolutionFailed
	}
	var active *StoredProviderConfig
	for index := range stored {
		if !isSearchProviderConfig(stored[index]) || !stored[index].Config.Enabled {
			continue
		}
		if active != nil {
			return websearch.ActiveExecution{}, websearch.ErrResolutionFailed
		}
		active = &stored[index]
	}
	if active != nil {
		providerID, err := validateStoredSearchProvider(*active)
		if err != nil || !SearchProviderConnectionTestValid(*active) {
			return websearch.ActiveExecution{}, websearch.ErrResolutionFailed
		}
		apiKey, err := s.decryptStoredSearchProviderSecret(*active)
		if err != nil || apiKey == "" {
			return websearch.ActiveExecution{}, websearch.ErrResolutionFailed
		}
		provider, err := websearch.NewProvider(providerID, websearch.Config{
			APIKey: apiKey, BaseURL: active.Config.BaseURL, Client: s.searchHTTPClient,
		})
		apiKey = ""
		if err != nil {
			return websearch.ActiveExecution{}, websearch.ErrResolutionFailed
		}
		return websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: provider,
		}, nil
	}
	for _, item := range stored {
		if !IsModelProviderConfig(item) || !item.Config.Enabled ||
			item.Config.Type != ProviderTypeOpenAI || !ProviderConnectionTestValid(item) {
			continue
		}
		apiKey, err := s.decryptStoredProviderSecret(item, item.Config.Type)
		if err == nil && strings.TrimSpace(apiKey) != "" {
			apiKey = ""
			return websearch.ActiveExecution{
				Mode:         websearch.ExecutionModelBuiltIn,
				ModelBuiltIn: websearch.ModelBuiltInOpenAI,
			}, nil
		}
	}
	return websearch.ActiveExecution{}, websearch.ErrNotConfigured
}

func SearchProviderConnectionTestValid(stored StoredProviderConfig) bool {
	if !isSearchProviderConfig(stored) {
		return false
	}
	providerID, err := validateStoredSearchProvider(stored)
	if err != nil {
		return false
	}
	return searchProviderConnectionTestValidForValues(
		stored.ProviderID,
		providerID,
		stored.Config.BaseURL,
		stored.EncryptedSecretRef,
		stored.Config.ConnectionTestSHA256,
		stored.Config.ConnectionTestedAt,
	)
}

func searchProviderConnectionTestValidForValues(
	recordID string,
	providerID websearch.ProviderID,
	baseURL string,
	secretRef string,
	storedFingerprint string,
	testedAt string,
) bool {
	storedFingerprint = strings.TrimSpace(storedFingerprint)
	if storedFingerprint == "" || strings.TrimSpace(testedAt) == "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(testedAt)); err != nil {
		return false
	}
	expected := searchProviderConnectionFingerprint(recordID, providerID, baseURL, secretRef)
	return subtleStringEqual(storedFingerprint, expected)
}

func searchProviderConnectionFingerprint(
	recordID string,
	providerID websearch.ProviderID,
	baseURL string,
	secretRef string,
) string {
	parts := []string{
		searchConnectionFingerprintVersion,
		strings.TrimSpace(recordID),
		string(providerID),
		strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		strings.TrimSpace(secretRef),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func validateStoredSearchProvider(
	stored StoredProviderConfig,
) (websearch.ProviderID, error) {
	if !isSearchProviderConfig(stored) {
		return "", ErrSearchProviderConfigUnsupported
	}
	providerID, err := normalizeSearchProviderID(stored.Config.SearchProvider)
	if err != nil || stored.ProviderID != searchProviderRecordID(providerID) {
		return "", ErrSearchProviderConfigUnsupported
	}
	return providerID, nil
}

func adminSearchProviderResponse(
	stored StoredProviderConfig,
	providerID websearch.ProviderID,
) AdminSearchProviderConfigResponse {
	connectionValid := SearchProviderConnectionTestValid(stored)
	var testedAt *time.Time
	if connectionValid {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(stored.Config.ConnectionTestedAt))
		if err == nil {
			utc := parsed.UTC()
			testedAt = &utc
		}
	}
	return AdminSearchProviderConfigResponse{
		ID:       searchProviderRecordID(providerID),
		Name:     nonEmpty(stored.Label, searchProviderDefaultName(providerID)),
		Provider: providerID, BaseURL: strings.TrimSpace(stored.Config.BaseURL),
		Enabled:             stored.Config.Enabled && connectionValid,
		HasAPIKey:           strings.TrimSpace(stored.EncryptedSecretRef) != "",
		ConnectionTestValid: connectionValid, ConnectionTestedAt: testedAt,
	}
}

func (s *Service) encryptSearchProviderSecretAtRest(
	userID string,
	recordID string,
	plaintext string,
) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", ErrSearchProviderSecretRequired
	}
	if s.providerSecrets == nil {
		return "", ErrProviderSecretVaultUnavailable
	}
	secretBytes := []byte(plaintext)
	plaintext = ""
	envelope, err := s.providerSecrets.Encrypt(
		secretBytes,
		searchProviderSecretContext(userID, recordID),
	)
	clear(secretBytes)
	if err != nil {
		return "", ErrProviderSecretInvalid
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > maxStoredProviderSecretRefBytes {
		return "", ErrProviderSecretInvalid
	}
	return string(encoded), nil
}

func (s *Service) decryptStoredSearchProviderSecret(
	stored StoredProviderConfig,
) (string, error) {
	encoded := strings.TrimSpace(stored.EncryptedSecretRef)
	if encoded == "" {
		return "", nil
	}
	if storedSecretAlgorithm(encoded) != providersecrets.Algorithm || s.providerSecrets == nil {
		if s.providerSecrets == nil {
			return "", ErrProviderSecretVaultUnavailable
		}
		return "", ErrProviderSecretInvalid
	}
	envelope, err := providersecrets.ParseEnvelope(encoded)
	if err != nil {
		return "", ErrProviderSecretInvalid
	}
	plaintext, err := s.providerSecrets.Decrypt(
		envelope,
		searchProviderSecretContext(stored.UserID, stored.ProviderID),
	)
	if err != nil {
		return "", ErrProviderSecretInvalid
	}
	decrypted := strings.TrimSpace(string(plaintext))
	clear(plaintext)
	if decrypted == "" {
		return "", ErrProviderSecretInvalid
	}
	return decrypted, nil
}
