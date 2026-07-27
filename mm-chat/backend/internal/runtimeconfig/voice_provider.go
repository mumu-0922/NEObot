package runtimeconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/voicejobs"
)

const (
	voiceConnectionFingerprintVersion = "voice-provider-connection/v1"
	voiceConnectionTestTimeout        = 20 * time.Second
	voiceConnectionTestText           = "你好，这是语音连接测试。"
)

var (
	ErrVoiceProviderConfigUnsupported = errors.New("voice provider configuration is unsupported")
	ErrVoiceProviderNotFound          = errors.New("voice provider configuration was not found")
	ErrVoiceProviderSecretRequired    = errors.New("voice provider API key is required")
	ErrVoiceProviderConnectionFailed  = errors.New("voice provider connection test failed")
	ErrVoiceProviderConfigChanged     = errors.New("voice provider configuration changed during connection testing")
	ErrVoiceProviderNotConfigured     = errors.New("voice provider is not configured")
	ErrVoiceProviderResolutionFailed  = errors.New("voice provider resolution failed")
)

type AdminVoiceProviderConfigResponse struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Provider            string     `json:"provider"`
	BaseURL             string     `json:"baseUrl"`
	Model               string     `json:"model"`
	Voice               string     `json:"voice"`
	Enabled             bool       `json:"enabled"`
	HasAPIKey           bool       `json:"hasApiKey"`
	ConnectionTestValid bool       `json:"connectionTestValid"`
	ConnectionTestedAt  *time.Time `json:"connectionTestedAt,omitempty"`
}

type AdminVoiceProviderConfigsResponse struct {
	Providers        []AdminVoiceProviderConfigResponse `json:"providers"`
	ActiveProviderID string                             `json:"activeProviderId,omitempty"`
}

type UpdateAdminVoiceProviderConfigRequest struct {
	Enabled      bool           `json:"enabled"`
	APIKeySecret map[string]any `json:"apiKeySecret"`
	ClearAPIKey  bool           `json:"clearApiKey"`
}

type AdminVoiceProviderConnectionResponse struct {
	Provider    AdminVoiceProviderConfigResponse `json:"provider"`
	ContentType string                           `json:"contentType"`
	Size        int64                            `json:"size"`
}

type CommitVoiceProviderConnectionInput struct {
	ID                         string
	UserID                     string
	ProviderID                 string
	ExpectedEncryptedSecretRef string
	ExpectedVoiceProvider      string
	ExpectedBaseURL            string
	ExpectedVoiceModel         string
	ExpectedVoiceID            string
	ExpectedEnabled            bool
	ConnectionTestSHA256       string
	ConnectionTestedAt         time.Time
	Enabled                    bool
}

type ResolvedVoiceProvider struct {
	ProviderID string
	BaseURL    string
	APIKey     string
	ModelID    string
	VoiceID    string
}

func WithVoiceProviderHTTPClient(client *http.Client) ServiceOption {
	return func(s *Service) {
		s.voiceHTTPClient = client
	}
}

func (s *Service) AdminVoiceProviderConfigs(
	ctx context.Context,
) (AdminVoiceProviderConfigsResponse, error) {
	if s == nil || s.repo == nil {
		return AdminVoiceProviderConfigsResponse{}, ErrDatabaseRequired
	}
	stored, err := s.repo.ListProviderConfigs(ctx, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return AdminVoiceProviderConfigsResponse{}, err
	}
	response := AdminVoiceProviderConfigsResponse{
		Providers: make([]AdminVoiceProviderConfigResponse, 0, 1),
	}
	for _, item := range stored {
		if strings.TrimSpace(item.Config.Kind) != providerConfigKindVoice {
			continue
		}
		providerID, err := validateStoredVoiceProvider(item)
		if err != nil || providerID != voiceProviderSiliconFlow || len(response.Providers) != 0 {
			return AdminVoiceProviderConfigsResponse{}, ErrVoiceProviderConfigUnsupported
		}
		provider := adminVoiceProviderResponse(item, providerID)
		response.Providers = append(response.Providers, provider)
		if provider.Enabled {
			response.ActiveProviderID = string(providerID)
		}
	}
	return response, nil
}

func (s *Service) UpsertAdminVoiceProviderConfig(
	ctx context.Context,
	providerValue string,
	request UpdateAdminVoiceProviderConfigRequest,
) (AdminVoiceProviderConfigResponse, error) {
	if s == nil || s.repo == nil {
		return AdminVoiceProviderConfigResponse{}, ErrDatabaseRequired
	}
	providerID, err := normalizeProductionVoiceProviderID(providerValue)
	if err != nil || (request.ClearAPIKey && len(request.APIKeySecret) > 0) {
		return AdminVoiceProviderConfigResponse{}, ErrVoiceProviderConfigUnsupported
	}
	recordID := voiceProviderRecordID(providerID)
	user := auth.UserOrDevelopment(ctx)
	current, hasCurrent, err := s.repo.GetProviderConfig(ctx, user.ID, recordID)
	if err != nil {
		return AdminVoiceProviderConfigResponse{}, err
	}
	if hasCurrent {
		if _, err := validateStoredVoiceProvider(current); err != nil {
			return AdminVoiceProviderConfigResponse{}, err
		}
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
			return AdminVoiceProviderConfigResponse{}, err
		}
		plaintext, err := s.DecryptOptionalSecret(envelope, voiceProviderIngressContext(providerID))
		if err != nil {
			return AdminVoiceProviderConfigResponse{}, err
		}
		secretRef, err = s.encryptVoiceProviderSecretAtRest(user.ID, recordID, plaintext)
		if err != nil {
			return AdminVoiceProviderConfigResponse{}, err
		}
	}

	fingerprint := ""
	testedAt := ""
	connectionValid := false
	if hasCurrent {
		fingerprint = strings.TrimSpace(current.Config.ConnectionTestSHA256)
		testedAt = strings.TrimSpace(current.Config.ConnectionTestedAt)
		connectionValid = voiceProviderConnectionTestValidForValues(
			recordID,
			providerID,
			SiliconFlowVoiceBaseURL,
			SiliconFlowVoiceModelID,
			SiliconFlowVoiceID,
			secretRef,
			fingerprint,
			testedAt,
		)
	}
	if !connectionValid {
		fingerprint = ""
		testedAt = ""
	}
	enabled := hasCurrent && current.Config.Enabled && request.Enabled && connectionValid
	stored, err := s.repo.UpsertProviderConfig(ctx, UpsertProviderConfigInput{
		UserID:             user.ID,
		ProviderID:         recordID,
		Label:              "SiliconFlow TTS",
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Kind:                 providerConfigKindVoice,
			VoiceProvider:        string(providerID),
			BaseURL:              SiliconFlowVoiceBaseURL,
			VoiceModel:           SiliconFlowVoiceModelID,
			VoiceID:              SiliconFlowVoiceID,
			Enabled:              enabled,
			ConnectionTestSHA256: fingerprint,
			ConnectionTestedAt:   testedAt,
		},
	})
	if err != nil {
		return AdminVoiceProviderConfigResponse{}, err
	}
	return adminVoiceProviderResponse(stored, providerID), nil
}

func (s *Service) DeleteAdminVoiceProviderConfig(ctx context.Context, providerValue string) error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}
	providerID, err := normalizeProductionVoiceProviderID(providerValue)
	if err != nil {
		return err
	}
	recordID := voiceProviderRecordID(providerID)
	userID := auth.UserOrDevelopment(ctx).ID
	stored, ok, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrVoiceProviderNotFound
	}
	if _, err := validateStoredVoiceProvider(stored); err != nil {
		return err
	}
	if err := s.repo.DeleteProviderConfig(ctx, userID, recordID); err != nil {
		if errors.Is(err, ErrProviderConfigNotFound) {
			return ErrVoiceProviderNotFound
		}
		return err
	}
	return nil
}

func (s *Service) TestAdminVoiceProviderConnection(
	ctx context.Context,
	providerValue string,
) (AdminVoiceProviderConnectionResponse, error) {
	return s.commitAdminVoiceProviderConnection(ctx, providerValue, false)
}

func (s *Service) ActivateAdminVoiceProvider(
	ctx context.Context,
	providerValue string,
) (AdminVoiceProviderConnectionResponse, error) {
	return s.commitAdminVoiceProviderConnection(ctx, providerValue, true)
}

func (s *Service) commitAdminVoiceProviderConnection(
	ctx context.Context,
	providerValue string,
	activate bool,
) (AdminVoiceProviderConnectionResponse, error) {
	if s == nil || s.repo == nil {
		return AdminVoiceProviderConnectionResponse{}, ErrDatabaseRequired
	}
	providerID, err := normalizeProductionVoiceProviderID(providerValue)
	if err != nil {
		return AdminVoiceProviderConnectionResponse{}, err
	}
	recordID := voiceProviderRecordID(providerID)
	userID := auth.UserOrDevelopment(ctx).ID
	stored, ok, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		return AdminVoiceProviderConnectionResponse{}, err
	}
	if !ok {
		return AdminVoiceProviderConnectionResponse{}, ErrVoiceProviderNotFound
	}
	if _, err := validateStoredVoiceProvider(stored); err != nil {
		return AdminVoiceProviderConnectionResponse{}, err
	}
	apiKey, err := s.decryptStoredVoiceProviderSecret(stored)
	if err != nil {
		return AdminVoiceProviderConnectionResponse{}, err
	}
	if apiKey == "" {
		return AdminVoiceProviderConnectionResponse{}, ErrVoiceProviderSecretRequired
	}
	executor, err := voicejobs.NewOpenAICompatibleExecutor(voicejobs.OpenAICompatibleExecutorConfig{
		BaseURL:            stored.Config.BaseURL,
		APIKey:             apiKey,
		Timeout:            voiceConnectionTestTimeout,
		HTTPClient:         s.voiceHTTPClient,
		DefaultSpeechModel: stored.Config.VoiceModel,
		DefaultSpeechVoice: stored.Config.VoiceID,
	})
	apiKey = ""
	if err != nil {
		return AdminVoiceProviderConnectionResponse{}, ErrVoiceProviderConfigUnsupported
	}
	testCtx, cancel := context.WithTimeout(ctx, voiceConnectionTestTimeout)
	result, err := executor.Synthesize(testCtx, voicejobs.SynthesizeRequest{
		Provider: voicejobs.ProviderDefault,
		Text:     voiceConnectionTestText,
	})
	cancel()
	if err != nil || result.Body == nil || result.Size <= 0 {
		return AdminVoiceProviderConnectionResponse{}, ErrVoiceProviderConnectionFailed
	}
	if _, err := io.Copy(io.Discard, result.Body); err != nil {
		return AdminVoiceProviderConnectionResponse{}, ErrVoiceProviderConnectionFailed
	}
	fingerprint := voiceProviderConnectionFingerprint(
		recordID,
		providerID,
		stored.Config.BaseURL,
		stored.Config.VoiceModel,
		stored.Config.VoiceID,
		stored.EncryptedSecretRef,
	)
	committed, err := s.repo.CommitVoiceProviderConnection(ctx, CommitVoiceProviderConnectionInput{
		ID:                         stored.ID,
		UserID:                     stored.UserID,
		ProviderID:                 stored.ProviderID,
		ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
		ExpectedVoiceProvider:      string(providerID),
		ExpectedBaseURL:            stored.Config.BaseURL,
		ExpectedVoiceModel:         stored.Config.VoiceModel,
		ExpectedVoiceID:            stored.Config.VoiceID,
		ExpectedEnabled:            stored.Config.Enabled,
		ConnectionTestSHA256:       fingerprint,
		ConnectionTestedAt:         time.Now().UTC(),
		Enabled:                    activate || stored.Config.Enabled,
	})
	if err != nil {
		if errors.Is(err, ErrProviderConfigChanged) {
			return AdminVoiceProviderConnectionResponse{}, ErrVoiceProviderConfigChanged
		}
		return AdminVoiceProviderConnectionResponse{}, err
	}
	return AdminVoiceProviderConnectionResponse{
		Provider:    adminVoiceProviderResponse(committed, providerID),
		ContentType: result.ContentType,
		Size:        result.Size,
	}, nil
}

func (s *Service) ResolveVoiceProvider(ctx context.Context) (ResolvedVoiceProvider, error) {
	if s == nil || s.repo == nil {
		return ResolvedVoiceProvider{}, ErrVoiceProviderNotConfigured
	}
	stored, err := s.repo.ListProviderConfigs(ctx, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return ResolvedVoiceProvider{}, ErrVoiceProviderResolutionFailed
	}
	var active *StoredProviderConfig
	for index := range stored {
		if strings.TrimSpace(stored[index].Config.Kind) != providerConfigKindVoice ||
			!stored[index].Config.Enabled {
			continue
		}
		if active != nil {
			return ResolvedVoiceProvider{}, ErrVoiceProviderResolutionFailed
		}
		active = &stored[index]
	}
	if active == nil {
		return ResolvedVoiceProvider{}, ErrVoiceProviderNotConfigured
	}
	providerID, err := validateStoredVoiceProvider(*active)
	if err != nil || providerID != voiceProviderSiliconFlow || !VoiceProviderConnectionTestValid(*active) {
		return ResolvedVoiceProvider{}, ErrVoiceProviderResolutionFailed
	}
	apiKey, err := s.decryptStoredVoiceProviderSecret(*active)
	if err != nil || apiKey == "" {
		return ResolvedVoiceProvider{}, ErrVoiceProviderResolutionFailed
	}
	return ResolvedVoiceProvider{
		ProviderID: string(providerID),
		BaseURL:    active.Config.BaseURL,
		APIKey:     apiKey,
		ModelID:    active.Config.VoiceModel,
		VoiceID:    active.Config.VoiceID,
	}, nil
}

func (s *Service) publicVoiceConfig(ctx context.Context) VoiceConfig {
	config := VoiceConfig{
		ElevenLabsAvailable: false,
		MimoAvailable:       false,
		DefaultSTTAvailable: false,
		DefaultTTSAvailable: false,
	}
	resolved, err := s.ResolveVoiceProvider(ctx)
	if err != nil {
		return config
	}
	config.DefaultTTSAvailable = true
	config.DefaultProvider = resolved.ProviderID
	config.TTSModel = resolved.ModelID
	config.TTSVoiceID = resolved.VoiceID
	return config
}

func VoiceProviderConnectionTestValid(stored StoredProviderConfig) bool {
	providerID, err := validateStoredVoiceProvider(stored)
	if err != nil || providerID != voiceProviderSiliconFlow {
		return false
	}
	return voiceProviderConnectionTestValidForValues(
		stored.ProviderID,
		providerID,
		stored.Config.BaseURL,
		stored.Config.VoiceModel,
		stored.Config.VoiceID,
		stored.EncryptedSecretRef,
		stored.Config.ConnectionTestSHA256,
		stored.Config.ConnectionTestedAt,
	)
}

func voiceProviderConnectionTestValidForValues(
	recordID string,
	providerID voiceProviderID,
	baseURL string,
	modelID string,
	voiceID string,
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
	expected := voiceProviderConnectionFingerprint(
		recordID, providerID, baseURL, modelID, voiceID, secretRef,
	)
	return subtleStringEqual(storedFingerprint, expected)
}

func voiceProviderConnectionFingerprint(
	recordID string,
	providerID voiceProviderID,
	baseURL string,
	modelID string,
	voiceID string,
	secretRef string,
) string {
	parts := []string{
		voiceConnectionFingerprintVersion,
		strings.TrimSpace(recordID),
		string(providerID),
		strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		strings.TrimSpace(modelID),
		strings.TrimSpace(voiceID),
		strings.TrimSpace(secretRef),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func normalizeProductionVoiceProviderID(value string) (voiceProviderID, error) {
	providerID, ok := normalizeVoiceProviderID(value)
	if !ok || providerID != voiceProviderSiliconFlow {
		return "", ErrVoiceProviderConfigUnsupported
	}
	return providerID, nil
}

func validateStoredVoiceProvider(stored StoredProviderConfig) (voiceProviderID, error) {
	if strings.TrimSpace(stored.Config.Kind) != providerConfigKindVoice {
		return "", ErrVoiceProviderConfigUnsupported
	}
	providerID, ok := normalizeVoiceProviderID(stored.Config.VoiceProvider)
	if !ok || stored.ProviderID != voiceProviderRecordID(providerID) {
		return "", ErrVoiceProviderConfigUnsupported
	}
	if providerID == voiceProviderSiliconFlow &&
		(strings.TrimRight(strings.TrimSpace(stored.Config.BaseURL), "/") != SiliconFlowVoiceBaseURL ||
			strings.TrimSpace(stored.Config.VoiceModel) != SiliconFlowVoiceModelID ||
			strings.TrimSpace(stored.Config.VoiceID) != SiliconFlowVoiceID) {
		return "", ErrVoiceProviderConfigUnsupported
	}
	return providerID, nil
}

func adminVoiceProviderResponse(
	stored StoredProviderConfig,
	providerID voiceProviderID,
) AdminVoiceProviderConfigResponse {
	connectionValid := VoiceProviderConnectionTestValid(stored)
	var testedAt *time.Time
	if connectionValid {
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(stored.Config.ConnectionTestedAt)); err == nil {
			utc := parsed.UTC()
			testedAt = &utc
		}
	}
	return AdminVoiceProviderConfigResponse{
		ID:                  voiceProviderRecordID(providerID),
		Name:                nonEmpty(stored.Label, "SiliconFlow TTS"),
		Provider:            string(providerID),
		BaseURL:             stored.Config.BaseURL,
		Model:               stored.Config.VoiceModel,
		Voice:               stored.Config.VoiceID,
		Enabled:             stored.Config.Enabled && connectionValid,
		HasAPIKey:           strings.TrimSpace(stored.EncryptedSecretRef) != "",
		ConnectionTestValid: connectionValid,
		ConnectionTestedAt:  testedAt,
	}
}

func (s *Service) encryptVoiceProviderSecretAtRest(
	userID string,
	recordID string,
	plaintext string,
) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", ErrVoiceProviderSecretRequired
	}
	if s.providerSecrets == nil {
		return "", ErrProviderSecretVaultUnavailable
	}
	secretBytes := []byte(plaintext)
	plaintext = ""
	envelope, err := s.providerSecrets.Encrypt(secretBytes, voiceProviderSecretContext(userID, recordID))
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

func (s *Service) decryptStoredVoiceProviderSecret(stored StoredProviderConfig) (string, error) {
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
		voiceProviderSecretContext(stored.UserID, stored.ProviderID),
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
