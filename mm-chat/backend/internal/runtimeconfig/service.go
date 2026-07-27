package runtimeconfig

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	serverDefaultProviderID = "SERVER_DEFAULT"
	byokAlgorithm           = "RSA-OAEP-256+A256GCM"
)

var (
	ErrBYOKNotConfigured               = errors.New("byok key is not configured")
	ErrProviderModelsUnsupported       = errors.New("provider model listing is only available for server-default providers")
	ErrPlaintextProviderSecret         = errors.New("plaintext provider secrets are not accepted")
	ErrProviderSecretRequired          = errors.New("provider api key is required")
	ErrProviderConfigUnsupported       = errors.New("provider configuration is unsupported")
	ErrDatabaseRequired                = errors.New("database is required")
	ErrProviderConfigNotFound          = errors.New("provider configuration was not found")
	ErrProviderSecretVaultUnavailable  = errors.New("provider secret vault is unavailable")
	ErrProviderSecretInvalid           = errors.New("provider secret is invalid")
	ErrProviderDisabled                = errors.New("provider is disabled")
	ErrProviderActivationRequired      = errors.New("provider activation is required")
	ErrProviderConnectionTestFailed    = errors.New("provider connection test failed")
	ErrProviderConfigChanged           = errors.New("provider configuration changed during connection test")
	ErrModelBuiltInSearchUnsupported   = errors.New("model built-in search configuration is unsupported")
	ErrModelBuiltInSearchTestFailed    = errors.New("model built-in search connection test failed")
	ErrModelBuiltInSearchConfigChanged = errors.New("model built-in search configuration changed during connection testing")
	ErrTaskModelSettingsInvalid        = errors.New("task model settings are invalid")
	ErrTaskModelUnavailable            = errors.New("task model is unavailable")
)

const maxProviderModelsResponseBytes = 2 << 20
const maxStoredProviderSecretRefBytes = 96 << 10

type Service struct {
	cfg                      config.Config
	repo                     ProviderConfigRepository
	toolCapabilityRepo       ToolCapabilityCacheRepository
	toolCapabilityWarmup     func(context.Context, ToolCapabilityWarmupRequest)
	taskModelRepo            TaskModelSettingsRepository
	providerSecrets          *providersecrets.Vault
	searchHTTPClient         websearch.HTTPDoer
	ragHTTPClient            websearch.HTTPDoer
	voiceHTTPClient          *http.Client
	searchAvailable          func(context.Context) bool
	modelBuiltInSearchTester ModelBuiltInSearchTester

	byokMu        sync.Mutex
	ephemeralBYOK *rsa.PrivateKey
}

type ServiceOption func(*Service)

func WithProviderConfigRepository(repo ProviderConfigRepository) ServiceOption {
	return func(s *Service) {
		s.repo = repo
		if capabilityRepo, ok := repo.(ToolCapabilityCacheRepository); ok {
			s.toolCapabilityRepo = capabilityRepo
		}
	}
}

func WithTaskModelSettingsRepository(repo TaskModelSettingsRepository) ServiceOption {
	return func(s *Service) {
		s.taskModelRepo = repo
	}
}

func WithToolCapabilityWarmupScheduler(
	schedule func(context.Context, ToolCapabilityWarmupRequest),
) ServiceOption {
	return func(s *Service) {
		s.toolCapabilityWarmup = schedule
	}
}

func WithSearchAvailable(available bool) ServiceOption {
	return func(s *Service) {
		s.searchAvailable = func(context.Context) bool { return available }
	}
}

func WithSearchAvailability(resolver func(context.Context) bool) ServiceOption {
	return func(s *Service) {
		s.searchAvailable = resolver
	}
}

func WithProviderSecretVault(vault *providersecrets.Vault) ServiceOption {
	return func(s *Service) {
		s.providerSecrets = vault
	}
}

func WithModelBuiltInSearchTester(tester ModelBuiltInSearchTester) ServiceOption {
	return func(s *Service) {
		s.modelBuiltInSearchTester = tester
	}
}

type ProviderConfigRepository interface {
	GetProviderConfig(ctx context.Context, userID string, providerID string) (StoredProviderConfig, bool, error)
	ListProviderConfigs(ctx context.Context, userID string) ([]StoredProviderConfig, error)
	UpsertProviderConfig(ctx context.Context, input UpsertProviderConfigInput) (StoredProviderConfig, error)
	CommitProviderConnection(ctx context.Context, input CommitProviderConnectionInput) (StoredProviderConfig, error)
	CommitSearchProviderConnection(ctx context.Context, input CommitSearchProviderConnectionInput) (StoredProviderConfig, error)
	CommitVoiceProviderConnection(ctx context.Context, input CommitVoiceProviderConnectionInput) (StoredProviderConfig, error)
	CommitModelBuiltInSearchConnection(ctx context.Context, input CommitModelBuiltInSearchConnectionInput) (StoredProviderConfig, error)
	DeleteProviderConfig(ctx context.Context, userID string, providerID string) error
}

type StoredProviderConfig struct {
	ID                 string
	UserID             string
	ProviderID         string
	Label              string
	EncryptedSecretRef string
	Config             StoredProviderConfigPayload
}

type StoredProviderConfigPayload struct {
	Kind                         string                            `json:"kind,omitempty"`
	Type                         ProviderType                      `json:"type"`
	SearchProvider               string                            `json:"searchProvider,omitempty"`
	RAGProvider                  string                            `json:"ragProvider,omitempty"`
	VoiceProvider                string                            `json:"voiceProvider,omitempty"`
	VoiceModel                   string                            `json:"voiceModel,omitempty"`
	VoiceID                      string                            `json:"voiceId,omitempty"`
	BaseURL                      string                            `json:"baseUrl"`
	Models                       []string                          `json:"models"`
	Enabled                      bool                              `json:"enabled"`
	ConnectionTestSHA256         string                            `json:"connectionTestSha256,omitempty"`
	ConnectionTestedAt           string                            `json:"connectionTestedAt,omitempty"`
	ModelBuiltInSearchProtocol   string                            `json:"modelBuiltInSearchProtocol,omitempty"`
	ModelBuiltInSearchModel      string                            `json:"modelBuiltInSearchModel,omitempty"`
	ModelBuiltInSearchTestSHA256 string                            `json:"modelBuiltInSearchTestSha256,omitempty"`
	ModelBuiltInSearchTestedAt   string                            `json:"modelBuiltInSearchTestedAt,omitempty"`
	ToolCapabilityDefault        ToolCapabilityOverride            `json:"toolCapabilityDefault,omitempty"`
	ToolCapabilityModelOverrides map[string]ToolCapabilityOverride `json:"toolCapabilityModelOverrides,omitempty"`
}

type UpsertProviderConfigInput struct {
	UserID             string
	ProviderID         string
	Label              string
	EncryptedSecretRef string
	Config             StoredProviderConfigPayload
}

type CommitProviderConnectionInput struct {
	ID                         string
	UserID                     string
	ProviderID                 string
	ExpectedEncryptedSecretRef string
	ExpectedType               ProviderType
	ExpectedBaseURL            string
	ExpectedEnabled            bool
	ConnectionTestSHA256       string
	ConnectionTestedAt         time.Time
	Enabled                    bool
}

type CommitModelBuiltInSearchConnectionInput struct {
	ID                         string
	UserID                     string
	ProviderID                 string
	ExpectedEncryptedSecretRef string
	ExpectedType               ProviderType
	ExpectedBaseURL            string
	ExpectedProtocol           string
	ExpectedModel              string
	ConnectionTestSHA256       string
	ConnectionTestedAt         time.Time
}

type ModelBuiltInSearchTestInput struct {
	ProviderID string
	Type       ProviderType
	BaseURL    string
	APIKey     string
	Protocol   string
	Model      string
}

type ModelBuiltInSearchTester interface {
	TestModelBuiltInSearch(context.Context, ModelBuiltInSearchTestInput) (int, error)
}

type EncryptedSecretEnvelope struct {
	V          int    `json:"v"`
	KID        string `json:"kid"`
	Alg        string `json:"alg"`
	IV         string `json:"iv"`
	WrappedKey string `json:"wrappedKey"`
	Ciphertext string `json:"ciphertext"`
	Context    string `json:"context"`
}

func NewService(cfg config.Config, opts ...ServiceOption) *Service {
	service := &Service{cfg: cfg}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *Service) PublicConfig() PublicConfig {
	return s.PublicConfigForContext(context.Background())
}

func (s *Service) PublicConfigForContext(ctx context.Context) PublicConfig {
	provider := s.serverDefaultProviderForContext(ctx)
	defaultModels, defaultModelsConfigured := s.publicTaskModels(ctx)
	voice := s.publicVoiceConfig(ctx)
	searchAvailable := false
	if s.searchAvailable != nil {
		searchAvailable = s.searchAvailable(ctx)
	}

	return PublicConfig{
		ModelProvider: ModelProviderConfig{
			Available:               provider.Available,
			ID:                      serverDefaultProviderID,
			Name:                    provider.Name,
			Type:                    provider.Type,
			Models:                  append([]string{}, provider.Models...),
			ModelMetadata:           map[string]any{},
			DefaultModels:           defaultModels,
			DefaultModelsConfigured: defaultModelsConfigured,
		},
		Search: SearchConfig{Available: searchAvailable},
		Voice:  voice,
		Deployment: DeploymentConfig{
			Mode:                    authModeToDeploymentMode(s.cfg.Auth.Mode),
			AccessPasswordEnabled:   false,
			TrustedProxyHeaders:     false,
			BYOKStableKeyConfigured: strings.TrimSpace(s.cfg.BYOK.PrivateKeyPEM) != "",
			BYOKEphemeralAllowed:    s.cfg.BYOK.AllowEphemeralKey,
			RateLimitStore:          publicStoreState(s.cfg.Redis.RateLimitEnabled),
			PluginRegistryStore:     pluginRegistryStoreState(s.cfg.DatabaseURL),
		},
	}
}

func (s *Service) ProviderModels(request ProviderModelsRequest) (ProviderModelsResponse, error) {
	return s.ProviderModelsForContext(context.Background(), request)
}

func (s *Service) ProviderModelsForContext(ctx context.Context, request ProviderModelsRequest) (ProviderModelsResponse, error) {
	provider := request.Provider
	if strings.TrimSpace(provider.APIKey) != "" {
		return ProviderModelsResponse{}, ErrPlaintextProviderSecret
	}

	if provider.Source != "" && provider.Source != "server-default" && provider.Source != "server-stored" {
		return ProviderModelsResponse{}, ErrProviderModelsUnsupported
	}
	if provider.Source == "server-stored" {
		resolved, err := s.ResolveStoredProvider(ctx, provider.ID)
		if err != nil {
			return ProviderModelsResponse{}, err
		}
		models, err := s.fetchResolvedProviderModels(ctx, resolved)
		if err != nil {
			return ProviderModelsResponse{}, err
		}
		return ProviderModelsResponse{Models: models}, nil
	}
	if provider.Source == "server-default" ||
		(provider.Source == "" && strings.TrimSpace(provider.Type) == "") {
		resolved, err := s.ResolveServerDefaultProvider(ctx)
		if err != nil {
			return ProviderModelsResponse{}, err
		}
		models, err := s.fetchResolvedProviderModels(ctx, resolved)
		if err != nil {
			return ProviderModelsResponse{}, err
		}
		return ProviderModelsResponse{Models: models}, nil
	}

	apiKey, err := s.ProviderAPIKey(provider)
	if err != nil {
		return ProviderModelsResponse{}, err
	}
	providerType := normalizeProviderType(provider.Type)
	if providerType != ProviderTypeOpenAI &&
		providerType != ProviderTypeOpenAICompatible &&
		providerType != ProviderTypeGemini &&
		providerType != ProviderTypeAnthropic {
		return ProviderModelsResponse{}, ErrProviderModelsUnsupported
	}

	timeout := s.cfg.Provider.Timeout
	if timeout <= 0 || timeout > maxProviderConnectionTestDuration {
		timeout = maxProviderConnectionTestDuration
	}
	models, err := fetchProviderModelsBounded(
		ctx, providerModelsURL(provider.BaseURL, providerType), providerType, apiKey, timeout,
	)
	if err != nil {
		return ProviderModelsResponse{}, err
	}
	return ProviderModelsResponse{Models: models}, nil
}

func (s *Service) fetchResolvedProviderModels(
	ctx context.Context,
	provider ResolvedProvider,
) ([]string, error) {
	providerType := normalizeProviderType(string(provider.Type))
	if providerType != ProviderTypeOpenAI &&
		providerType != ProviderTypeOpenAICompatible &&
		providerType != ProviderTypeGemini &&
		providerType != ProviderTypeAnthropic {
		return nil, ErrProviderModelsUnsupported
	}
	timeout := s.cfg.Provider.Timeout
	if timeout <= 0 || timeout > maxProviderConnectionTestDuration {
		timeout = maxProviderConnectionTestDuration
	}
	return fetchProviderModelsBounded(
		ctx,
		providerModelsURL(provider.BaseURL, providerType),
		providerType,
		provider.APIKey,
		timeout,
	)
}

type resolvedServerDefaultProvider struct {
	Name                         string
	Type                         ProviderType
	BaseURL                      string
	Models                       []string
	Enabled                      bool
	Available                    bool
	APIKey                       string
	SecretRef                    string
	SecretErr                    error
	ConnectionTestValid          bool
	ConnectionTestedAt           string
	ModelBuiltInSearchProtocol   string
	ModelBuiltInSearchModel      string
	ModelBuiltInSearchTestValid  bool
	ModelBuiltInSearchTestedAt   string
	ToolCapabilityDefault        ToolCapabilityOverride
	ToolCapabilityModelOverrides map[string]ToolCapabilityOverride
	ToolCapabilityConfigHash     string
}

func (s *Service) AdminProviderConfig(ctx context.Context) (AdminProviderConfigResponse, error) {
	provider := s.serverDefaultProviderForContext(ctx)
	return adminProviderResponse(
		provider,
		serverDefaultProviderID,
		"server-default",
	), nil
}

func (s *Service) AdminProviderConfigs(ctx context.Context) (AdminProviderConfigsResponse, error) {
	if s.repo == nil {
		return AdminProviderConfigsResponse{}, ErrDatabaseRequired
	}
	user := auth.UserOrDevelopment(ctx)
	stored, err := s.repo.ListProviderConfigs(ctx, user.ID)
	if err != nil {
		return AdminProviderConfigsResponse{}, err
	}

	providers := make([]AdminProviderConfigResponse, 0, len(stored)+1)
	hasDefault := false
	for _, item := range stored {
		if !IsModelProviderConfig(item) {
			continue
		}
		if item.ProviderID == serverDefaultProviderID {
			hasDefault = true
			resolved := s.resolveStoredServerDefault(item)
			providers = append(providers, adminProviderResponse(resolved, serverDefaultProviderID, "server-default"))
			continue
		}
		resolved := s.resolveStoredProvider(item)
		providers = append(providers, adminProviderResponse(resolved, item.ProviderID, "server-stored"))
	}
	if !hasDefault {
		providers = append([]AdminProviderConfigResponse{
			adminProviderResponse(emptyServerDefaultProvider(), serverDefaultProviderID, "server-default"),
		}, providers...)
	}
	return AdminProviderConfigsResponse{Providers: providers}, nil
}

func (s *Service) UpdateAdminProviderConfig(ctx context.Context, request UpdateAdminProviderConfigRequest) (AdminProviderConfigResponse, error) {
	return s.UpsertAdminProviderConfig(ctx, serverDefaultProviderID, request)
}

func (s *Service) UpsertAdminProviderConfig(
	ctx context.Context,
	providerID string,
	request UpdateAdminProviderConfigRequest,
) (AdminProviderConfigResponse, error) {
	if s.repo == nil {
		return AdminProviderConfigResponse{}, ErrDatabaseRequired
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || len(providerID) > 128 ||
		isReservedSearchProviderRecordID(providerID) ||
		isReservedRAGProviderRecordID(providerID) ||
		isReservedVoiceProviderRecordID(providerID) {
		return AdminProviderConfigResponse{}, ErrProviderConfigUnsupported
	}
	if request.ClearAPIKey && len(request.APIKeySecret) > 0 {
		return AdminProviderConfigResponse{}, ErrProviderConfigUnsupported
	}
	user := auth.UserOrDevelopment(ctx)

	current := resolvedServerDefaultProvider{
		Name: "New Provider", Type: ProviderTypeOpenAICompatible,
	}
	if providerID == serverDefaultProviderID {
		current = emptyServerDefaultProvider()
	}
	var currentStored StoredProviderConfig
	hasCurrent := false
	if stored, ok, err := s.repo.GetProviderConfig(ctx, user.ID, providerID); err != nil {
		return AdminProviderConfigResponse{}, err
	} else if ok {
		if !IsModelProviderConfig(stored) {
			return AdminProviderConfigResponse{}, ErrProviderConfigUnsupported
		}
		currentStored = stored
		hasCurrent = true
		if providerID == serverDefaultProviderID {
			current = s.resolveStoredServerDefault(stored)
		} else {
			current = s.resolveStoredProvider(stored)
		}
	}
	providerType := normalizeProviderType(nonEmpty(request.Type, string(current.Type)))
	name := nonEmpty(request.Name, config.DefaultProviderName)
	baseURL := strings.TrimSpace(request.BaseURL)
	models := normalizeModelList(request.Models)
	toolCapabilityDefault := ToolCapabilityAuto
	toolCapabilityModelOverrides := map[string]ToolCapabilityOverride{}
	if hasCurrent {
		toolCapabilityDefault = normalizeToolCapabilityOverride(
			currentStored.Config.ToolCapabilityDefault,
		)
		toolCapabilityModelOverrides = cloneToolCapabilityOverrides(
			currentStored.Config.ToolCapabilityModelOverrides,
		)
	}
	if request.ToolCapabilityDefault != nil {
		value, ok := parseToolCapabilityOverride(*request.ToolCapabilityDefault)
		if !ok {
			return AdminProviderConfigResponse{}, ErrProviderConfigUnsupported
		}
		toolCapabilityDefault = value
	}
	if request.ToolCapabilityModelOverrides != nil {
		var err error
		toolCapabilityModelOverrides, err = normalizeToolCapabilityModelOverrides(
			request.ToolCapabilityModelOverrides,
			models,
		)
		if err != nil {
			return AdminProviderConfigResponse{}, err
		}
	} else {
		toolCapabilityModelOverrides = filterToolCapabilityModelOverrides(
			toolCapabilityModelOverrides,
			models,
		)
	}
	secretRef := strings.TrimSpace(current.SecretRef)

	if request.ClearAPIKey {
		secretRef = ""
	}
	if len(request.APIKeySecret) > 0 {
		envelope, err := parseEncryptedSecretEnvelope(request.APIKeySecret)
		if err != nil {
			return AdminProviderConfigResponse{}, err
		}
		plaintext, err := s.DecryptOptionalSecret(envelope, "provider:"+string(providerType))
		if err != nil {
			return AdminProviderConfigResponse{}, err
		}
		secretRef, err = s.encryptProviderSecretAtRest(
			user.ID, providerID, plaintext,
		)
		if err != nil {
			return AdminProviderConfigResponse{}, err
		}
	} else if !request.ClearAPIKey {
		if current.SecretErr != nil {
			return AdminProviderConfigResponse{}, current.SecretErr
		}
		if strings.TrimSpace(current.APIKey) != "" &&
			(secretRef == "" || storedSecretAlgorithm(secretRef) == byokAlgorithm) {
			var err error
			secretRef, err = s.encryptProviderSecretAtRest(
				user.ID, providerID, current.APIKey,
			)
			if err != nil {
				return AdminProviderConfigResponse{}, err
			}
		}
	}

	connectionTestSHA256 := ""
	connectionTestedAt := ""
	connectionTestValid := false
	if hasCurrent {
		connectionTestSHA256 = strings.TrimSpace(currentStored.Config.ConnectionTestSHA256)
		connectionTestedAt = strings.TrimSpace(currentStored.Config.ConnectionTestedAt)
		connectionTestValid = providerConnectionTestValidForValues(
			providerID,
			providerType,
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
	builtInProtocol := ""
	builtInModel := ""
	builtInTestSHA256 := ""
	builtInTestedAt := ""
	if hasCurrent && currentStored.Config.Type == ProviderTypeOpenAICompatible {
		builtInProtocol = strings.TrimSpace(currentStored.Config.ModelBuiltInSearchProtocol)
		builtInModel = strings.TrimSpace(currentStored.Config.ModelBuiltInSearchModel)
	}
	if request.ModelBuiltInSearchProtocol != nil {
		builtInProtocol = strings.TrimSpace(*request.ModelBuiltInSearchProtocol)
	}
	if request.ModelBuiltInSearchModel != nil {
		builtInModel = strings.TrimSpace(*request.ModelBuiltInSearchModel)
	}
	if providerType != ProviderTypeOpenAICompatible {
		builtInProtocol = ""
		builtInModel = ""
	} else {
		var err error
		builtInProtocol, builtInModel, err = normalizeCustomModelBuiltInSearch(
			builtInProtocol,
			builtInModel,
		)
		if err != nil || (builtInProtocol != "" && !modelListContains(models, builtInModel)) {
			return AdminProviderConfigResponse{}, ErrModelBuiltInSearchUnsupported
		}
	}
	if hasCurrent {
		candidate := currentStored
		candidate.EncryptedSecretRef = secretRef
		candidate.Config.Type = providerType
		candidate.Config.BaseURL = baseURL
		candidate.Config.Models = models
		candidate.Config.ModelBuiltInSearchProtocol = builtInProtocol
		candidate.Config.ModelBuiltInSearchModel = builtInModel
		if ModelBuiltInSearchConnectionTestValid(candidate) {
			builtInTestSHA256 = currentStored.Config.ModelBuiltInSearchTestSHA256
			builtInTestedAt = currentStored.Config.ModelBuiltInSearchTestedAt
		}
	}
	enabled := hasCurrent && currentStored.Config.Enabled && request.Enabled && connectionTestValid
	stored, err := s.repo.UpsertProviderConfig(ctx, UpsertProviderConfigInput{
		UserID:             user.ID,
		ProviderID:         providerID,
		Label:              name,
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Kind:                         providerConfigKindModel,
			Type:                         providerType,
			BaseURL:                      baseURL,
			Models:                       models,
			Enabled:                      enabled,
			ConnectionTestSHA256:         connectionTestSHA256,
			ConnectionTestedAt:           connectionTestedAt,
			ModelBuiltInSearchProtocol:   builtInProtocol,
			ModelBuiltInSearchModel:      builtInModel,
			ModelBuiltInSearchTestSHA256: builtInTestSHA256,
			ModelBuiltInSearchTestedAt:   builtInTestedAt,
			ToolCapabilityDefault:        toolCapabilityDefault,
			ToolCapabilityModelOverrides: toolCapabilityModelOverrides,
		},
	})
	if err != nil {
		return AdminProviderConfigResponse{}, err
	}

	s.scheduleToolCapabilityWarmup(ctx, stored)
	if providerID == serverDefaultProviderID {
		return adminProviderResponse(s.resolveStoredServerDefault(stored), providerID, "server-default"), nil
	}
	return adminProviderResponse(s.resolveStoredProvider(stored), providerID, "server-stored"), nil
}

func (s *Service) DeleteAdminProviderConfig(ctx context.Context, providerID string) error {
	if s.repo == nil {
		return ErrDatabaseRequired
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || providerID == serverDefaultProviderID ||
		isReservedSearchProviderRecordID(providerID) ||
		isReservedRAGProviderRecordID(providerID) ||
		isReservedVoiceProviderRecordID(providerID) {
		return ErrProviderConfigUnsupported
	}
	return s.repo.DeleteProviderConfig(ctx, auth.UserOrDevelopment(ctx).ID, providerID)
}

func (s *Service) serverDefaultProviderForContext(ctx context.Context) resolvedServerDefaultProvider {
	if s.repo == nil {
		return emptyServerDefaultProvider()
	}
	stored, ok, err := s.repo.GetProviderConfig(ctx, auth.UserOrDevelopment(ctx).ID, serverDefaultProviderID)
	if err != nil || !ok || !IsModelProviderConfig(stored) {
		return emptyServerDefaultProvider()
	}
	return s.resolveStoredServerDefault(stored)
}

func emptyServerDefaultProvider() resolvedServerDefaultProvider {
	return resolvedServerDefaultProvider{
		Name: config.DefaultProviderName,
		Type: ProviderTypeOpenAICompatible,
	}
}

func (s *Service) resolveStoredServerDefault(stored StoredProviderConfig) resolvedServerDefaultProvider {
	name := nonEmpty(stored.Label, config.DefaultProviderName)
	providerType := stored.Config.Type
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	baseURL := strings.TrimSpace(stored.Config.BaseURL)
	models := normalizeModelList(stored.Config.Models)
	secretRef := strings.TrimSpace(stored.EncryptedSecretRef)
	apiKey := ""
	var secretErr error
	if secretRef != "" {
		apiKey, secretErr = s.decryptStoredProviderSecret(stored, providerType)
	}
	connectionValid := ProviderConnectionTestValid(stored)
	enabled := stored.Config.Enabled && connectionValid
	available := enabled && secretErr == nil && len(models) > 0 && apiKey != ""
	return resolvedServerDefaultProvider{
		Name:                         name,
		Type:                         providerType,
		BaseURL:                      baseURL,
		Models:                       models,
		Enabled:                      enabled,
		Available:                    available,
		APIKey:                       apiKey,
		SecretRef:                    secretRef,
		SecretErr:                    secretErr,
		ConnectionTestValid:          connectionValid,
		ConnectionTestedAt:           stored.Config.ConnectionTestedAt,
		ModelBuiltInSearchProtocol:   stored.Config.ModelBuiltInSearchProtocol,
		ModelBuiltInSearchModel:      stored.Config.ModelBuiltInSearchModel,
		ModelBuiltInSearchTestValid:  ModelBuiltInSearchConnectionTestValid(stored),
		ModelBuiltInSearchTestedAt:   stored.Config.ModelBuiltInSearchTestedAt,
		ToolCapabilityDefault:        normalizeToolCapabilityOverride(stored.Config.ToolCapabilityDefault),
		ToolCapabilityModelOverrides: cloneToolCapabilityOverrides(stored.Config.ToolCapabilityModelOverrides),
		ToolCapabilityConfigHash:     toolCapabilityConfigHash(stored),
	}
}

func (s *Service) resolveStoredProvider(stored StoredProviderConfig) resolvedServerDefaultProvider {
	providerType := stored.Config.Type
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	secretRef := strings.TrimSpace(stored.EncryptedSecretRef)
	apiKey := ""
	var secretErr error
	if secretRef != "" {
		apiKey, secretErr = s.decryptStoredProviderSecret(stored, providerType)
	}
	models := normalizeModelList(stored.Config.Models)
	connectionValid := ProviderConnectionTestValid(stored)
	enabled := stored.Config.Enabled && connectionValid
	return resolvedServerDefaultProvider{
		Name:                         nonEmpty(stored.Label, "New Provider"),
		Type:                         providerType,
		BaseURL:                      strings.TrimSpace(stored.Config.BaseURL),
		Models:                       models,
		Enabled:                      enabled,
		Available:                    enabled && secretErr == nil && len(models) > 0 && apiKey != "",
		APIKey:                       apiKey,
		SecretRef:                    secretRef,
		SecretErr:                    secretErr,
		ConnectionTestValid:          connectionValid,
		ConnectionTestedAt:           stored.Config.ConnectionTestedAt,
		ModelBuiltInSearchProtocol:   stored.Config.ModelBuiltInSearchProtocol,
		ModelBuiltInSearchModel:      stored.Config.ModelBuiltInSearchModel,
		ModelBuiltInSearchTestValid:  ModelBuiltInSearchConnectionTestValid(stored),
		ModelBuiltInSearchTestedAt:   stored.Config.ModelBuiltInSearchTestedAt,
		ToolCapabilityDefault:        normalizeToolCapabilityOverride(stored.Config.ToolCapabilityDefault),
		ToolCapabilityModelOverrides: cloneToolCapabilityOverrides(stored.Config.ToolCapabilityModelOverrides),
		ToolCapabilityConfigHash:     toolCapabilityConfigHash(stored),
	}
}

func adminProviderResponse(
	provider resolvedServerDefaultProvider,
	providerID string,
	source string,
) AdminProviderConfigResponse {
	connectionTestedAt, connectionTestValid := parseProviderConnectionTestState(provider)
	builtIn := adminModelBuiltInSearchResponse(provider)
	return AdminProviderConfigResponse{
		ID:                  providerID,
		Name:                provider.Name,
		Type:                provider.Type,
		BaseURL:             provider.BaseURL,
		Models:              append([]string{}, provider.Models...),
		Enabled:             provider.Enabled,
		HasAPIKey:           strings.TrimSpace(provider.APIKey) != "" || strings.TrimSpace(provider.SecretRef) != "",
		Source:              source,
		ConnectionTestValid: connectionTestValid,
		ConnectionTestedAt:  connectionTestedAt,
		ModelBuiltInSearch:  builtIn,
		ToolCapability: AdminToolCapabilityConfigResponse{
			Default:        normalizeToolCapabilityOverride(provider.ToolCapabilityDefault),
			ModelOverrides: cloneToolCapabilityOverrides(provider.ToolCapabilityModelOverrides),
		},
	}
}

func adminModelBuiltInSearchResponse(
	provider resolvedServerDefaultProvider,
) AdminModelBuiltInSearchConfigResponse {
	if protocol := officialModelBuiltInSearchProtocol(provider.Type); protocol != "" {
		return AdminModelBuiltInSearchConfigResponse{
			Protocol:            protocol,
			Source:              "official",
			ConnectionTestValid: provider.ConnectionTestValid,
		}
	}
	response := AdminModelBuiltInSearchConfigResponse{
		Protocol:            strings.TrimSpace(provider.ModelBuiltInSearchProtocol),
		Model:               strings.TrimSpace(provider.ModelBuiltInSearchModel),
		Source:              "none",
		ConnectionTestValid: provider.ModelBuiltInSearchTestValid,
	}
	if response.Protocol != "" {
		response.Source = "custom"
	}
	if response.ConnectionTestValid {
		if testedAt, err := time.Parse(time.RFC3339Nano, provider.ModelBuiltInSearchTestedAt); err == nil {
			testedAt = testedAt.UTC()
			response.ConnectionTestedAt = &testedAt
		} else {
			response.ConnectionTestValid = false
		}
	}
	return response
}

func (s *Service) decryptStoredProviderSecret(
	stored StoredProviderConfig,
	providerType ProviderType,
) (string, error) {
	encoded := strings.TrimSpace(stored.EncryptedSecretRef)
	if encoded == "" {
		return "", nil
	}
	switch storedSecretAlgorithm(encoded) {
	case providersecrets.Algorithm:
		if s.providerSecrets == nil {
			return "", ErrProviderSecretVaultUnavailable
		}
		envelope, err := providersecrets.ParseEnvelope(encoded)
		if err != nil {
			return "", ErrProviderSecretInvalid
		}
		plaintext, err := s.providerSecrets.Decrypt(
			envelope,
			modelProviderSecretContext(stored.UserID, stored.ProviderID),
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
	case byokAlgorithm:
		envelope, err := parseStoredLegacySecretRef(encoded)
		if err != nil {
			return "", ErrProviderSecretInvalid
		}
		plaintext, err := s.DecryptOptionalSecret(
			&envelope,
			"provider:"+string(providerType),
		)
		if err != nil || strings.TrimSpace(plaintext) == "" {
			return "", ErrProviderSecretInvalid
		}
		return strings.TrimSpace(plaintext), nil
	default:
		return "", ErrProviderSecretInvalid
	}
}

func (s *Service) encryptProviderSecretAtRest(
	userID string,
	providerID string,
	plaintext string,
) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", ErrProviderSecretRequired
	}
	if s.providerSecrets == nil {
		return "", ErrProviderSecretVaultUnavailable
	}
	secretBytes := []byte(plaintext)
	plaintext = ""
	envelope, err := s.providerSecrets.Encrypt(
		secretBytes,
		modelProviderSecretContext(userID, providerID),
	)
	clear(secretBytes)
	if err != nil {
		return "", ErrProviderSecretInvalid
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", ErrProviderSecretInvalid
	}
	return string(encoded), nil
}

func storedSecretAlgorithm(encoded string) string {
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(encoded)), &header); err != nil {
		return ""
	}
	return header.Alg
}

func parseStoredLegacySecretRef(encoded string) (EncryptedSecretEnvelope, error) {
	var envelope EncryptedSecretEnvelope
	encoded = strings.TrimSpace(encoded)
	if encoded == "" || len(encoded) > maxStoredProviderSecretRefBytes {
		return envelope, ErrProviderSecretInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return EncryptedSecretEnvelope{}, ErrProviderSecretInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return EncryptedSecretEnvelope{}, ErrProviderSecretInvalid
	}
	return envelope, nil
}

func modelProviderSecretContext(userID string, providerID string) string {
	return "provider:model:" + strings.TrimSpace(userID) + ":" + strings.TrimSpace(providerID)
}

func normalizeModelList(models []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

type ResolvedProvider struct {
	ID                           string
	Name                         string
	Type                         ProviderType
	BaseURL                      string
	APIKey                       string
	Models                       []string
	ModelBuiltInSearchProtocol   string
	ModelBuiltInSearchModel      string
	ModelBuiltInSearchTestValid  bool
	ToolCapabilityDefault        ToolCapabilityOverride
	ToolCapabilityModelOverrides map[string]ToolCapabilityOverride
	ToolCapabilityConfigHash     string
}

func (s *Service) ResolveServerDefaultProvider(ctx context.Context) (ResolvedProvider, error) {
	if s.repo == nil {
		return ResolvedProvider{}, ErrDatabaseRequired
	}
	stored, ok, err := s.repo.GetProviderConfig(
		ctx,
		auth.UserOrDevelopment(ctx).ID,
		serverDefaultProviderID,
	)
	if err != nil {
		return ResolvedProvider{}, err
	}
	if !ok {
		return ResolvedProvider{}, ErrProviderConfigNotFound
	}
	if !IsModelProviderConfig(stored) {
		return ResolvedProvider{}, ErrProviderConfigUnsupported
	}
	if !stored.Config.Enabled {
		return ResolvedProvider{}, ErrProviderDisabled
	}
	if !ProviderConnectionTestValid(stored) {
		return ResolvedProvider{}, ErrProviderActivationRequired
	}
	provider := s.resolveStoredServerDefault(stored)
	if provider.SecretErr != nil {
		return ResolvedProvider{}, provider.SecretErr
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		return ResolvedProvider{}, ErrProviderSecretRequired
	}
	return ResolvedProvider{
		ID:                           serverDefaultProviderID,
		Name:                         provider.Name,
		Type:                         provider.Type,
		BaseURL:                      provider.BaseURL,
		APIKey:                       provider.APIKey,
		Models:                       append([]string(nil), provider.Models...),
		ModelBuiltInSearchProtocol:   provider.ModelBuiltInSearchProtocol,
		ModelBuiltInSearchModel:      provider.ModelBuiltInSearchModel,
		ModelBuiltInSearchTestValid:  provider.ModelBuiltInSearchTestValid,
		ToolCapabilityDefault:        provider.ToolCapabilityDefault,
		ToolCapabilityModelOverrides: cloneToolCapabilityOverrides(provider.ToolCapabilityModelOverrides),
		ToolCapabilityConfigHash:     provider.ToolCapabilityConfigHash,
	}, nil
}

func (s *Service) ResolveStoredProvider(ctx context.Context, providerID string) (ResolvedProvider, error) {
	if s.repo == nil {
		return ResolvedProvider{}, ErrDatabaseRequired
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || providerID == serverDefaultProviderID {
		return ResolvedProvider{}, ErrProviderConfigUnsupported
	}
	stored, ok, err := s.repo.GetProviderConfig(ctx, auth.UserOrDevelopment(ctx).ID, providerID)
	if err != nil {
		return ResolvedProvider{}, err
	}
	if !ok {
		return ResolvedProvider{}, ErrProviderConfigNotFound
	}
	if !IsModelProviderConfig(stored) {
		return ResolvedProvider{}, ErrProviderConfigUnsupported
	}
	if !stored.Config.Enabled {
		return ResolvedProvider{}, ErrProviderDisabled
	}
	if !ProviderConnectionTestValid(stored) {
		return ResolvedProvider{}, ErrProviderActivationRequired
	}
	provider := s.resolveStoredProvider(stored)
	if provider.SecretErr != nil {
		return ResolvedProvider{}, provider.SecretErr
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		return ResolvedProvider{}, ErrProviderSecretRequired
	}
	return ResolvedProvider{
		ID:                           providerID,
		Name:                         provider.Name,
		Type:                         provider.Type,
		BaseURL:                      provider.BaseURL,
		APIKey:                       provider.APIKey,
		Models:                       append([]string(nil), provider.Models...),
		ModelBuiltInSearchProtocol:   provider.ModelBuiltInSearchProtocol,
		ModelBuiltInSearchModel:      provider.ModelBuiltInSearchModel,
		ModelBuiltInSearchTestValid:  provider.ModelBuiltInSearchTestValid,
		ToolCapabilityDefault:        provider.ToolCapabilityDefault,
		ToolCapabilityModelOverrides: cloneToolCapabilityOverrides(provider.ToolCapabilityModelOverrides),
		ToolCapabilityConfigHash:     provider.ToolCapabilityConfigHash,
	}, nil
}

func (s *Service) ProviderAPIKey(provider ProviderRuntimeConfig) (string, error) {
	if strings.TrimSpace(provider.APIKey) != "" {
		return "", ErrPlaintextProviderSecret
	}
	envelope, err := parseEncryptedSecretEnvelope(provider.APIKeySecret)
	if err != nil {
		return "", err
	}
	apiKey, err := s.DecryptOptionalSecret(envelope, "provider:"+string(normalizeProviderType(provider.Type)))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(apiKey) == "" {
		return "", ErrProviderSecretRequired
	}
	return apiKey, nil
}

func (s *Service) BYOKPublicKey() (BYOKPublicKeyResponse, error) {
	key, err := s.byokPrivateKey()
	if err != nil {
		return BYOKPublicKeyResponse{}, err
	}

	kid := strings.TrimSpace(s.cfg.BYOK.KeyID)
	if kid == "" {
		kid, err = deriveKeyID(&key.PublicKey)
		if err != nil {
			return BYOKPublicKeyResponse{}, err
		}
	}

	return BYOKPublicKeyResponse{
		KID: kid,
		Alg: byokAlgorithm,
		PublicKeyJWK: map[string]any{
			"kty":     "RSA",
			"n":       base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":       base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			"alg":     "RSA-OAEP-256",
			"key_ops": []string{"wrapKey"},
		},
	}, nil
}

func (s *Service) DecryptOptionalSecret(
	envelope *EncryptedSecretEnvelope,
	expectedContext string,
) (string, error) {
	if envelope == nil {
		return "", nil
	}
	return s.DecryptSecretEnvelope(*envelope, expectedContext)
}

func (s *Service) DecryptSecretEnvelope(
	envelope EncryptedSecretEnvelope,
	expectedContext string,
) (string, error) {
	if envelope.V != 1 || envelope.Alg != byokAlgorithm {
		return "", ErrBYOKNotConfigured
	}
	if envelope.Context != expectedContext {
		return "", ErrBYOKNotConfigured
	}

	key, err := s.byokPrivateKey()
	if err != nil {
		return "", err
	}
	kid := strings.TrimSpace(s.cfg.BYOK.KeyID)
	if kid == "" {
		kid, err = deriveKeyID(&key.PublicKey)
		if err != nil {
			return "", err
		}
	}
	if envelope.KID != kid {
		return "", ErrBYOKNotConfigured
	}

	wrappedKey, err := base64.RawURLEncoding.DecodeString(envelope.WrappedKey)
	if err != nil {
		return "", ErrBYOKNotConfigured
	}
	iv, err := base64.RawURLEncoding.DecodeString(envelope.IV)
	if err != nil {
		return "", ErrBYOKNotConfigured
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return "", ErrBYOKNotConfigured
	}

	aesKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, key, wrappedKey, nil)
	if err != nil {
		return "", ErrBYOKNotConfigured
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", ErrBYOKNotConfigured
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrBYOKNotConfigured
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, []byte(envelope.Context))
	if err != nil {
		return "", ErrBYOKNotConfigured
	}
	return string(plaintext), nil
}

func (s *Service) byokPrivateKey() (*rsa.PrivateKey, error) {
	pemValue := strings.TrimSpace(s.cfg.BYOK.PrivateKeyPEM)
	if pemValue != "" {
		return parseRSAPrivateKeyPEM(pemValue)
	}
	if !s.cfg.BYOK.AllowEphemeralKey {
		return nil, ErrBYOKNotConfigured
	}

	s.byokMu.Lock()
	defer s.byokMu.Unlock()
	if s.ephemeralBYOK != nil {
		return s.ephemeralBYOK, nil
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	s.ephemeralBYOK = key
	return key, nil
}

func parseRSAPrivateKeyPEM(value string) (*rsa.PrivateKey, error) {
	decoded := strings.ReplaceAll(value, `\n`, "\n")
	block, _ := pem.Decode([]byte(decoded))
	if block == nil {
		return nil, ErrBYOKNotConfigured
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, ErrBYOKNotConfigured
	}
	return key, nil
}

func deriveKeyID(publicKey *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func splitModels(value string) []string {
	parts := strings.Split(value, ",")
	models := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		model := strings.TrimSpace(part)
		if model == "" {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	return models
}

func normalizeProviderType(value string) ProviderType {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "gemini", "google_gemini":
		return ProviderTypeGemini
	case "anthropic", "anthropic_claude", "claude":
		return ProviderTypeAnthropic
	case "openai":
		return ProviderTypeOpenAI
	case "openai_compatible", "openai-compatible", "openai compatible":
		return ProviderTypeOpenAICompatible
	default:
		return ProviderTypeOpenAICompatible
	}
}

func parseEncryptedSecretEnvelope(raw map[string]any) (*EncryptedSecretEnvelope, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, ErrBYOKNotConfigured
	}
	var envelope EncryptedSecretEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return nil, ErrBYOKNotConfigured
	}
	return &envelope, nil
}

func providerModelsURL(baseURL string, providerType ProviderType) string {
	normalized := normalizeProviderBaseURL(baseURL, providerType)
	if providerType == ProviderTypeGemini {
		return normalized + "/v1beta/models"
	}
	if providerType == ProviderTypeAnthropic {
		return normalized + "/v1/models"
	}
	return normalized + "/models"
}

func normalizeProviderBaseURL(baseURL string, providerType ProviderType) string {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" || normalized == "default" {
		if providerType == ProviderTypeGemini {
			return "https://generativelanguage.googleapis.com"
		}
		if providerType == ProviderTypeAnthropic {
			return "https://api.anthropic.com"
		}
		return "https://api.openai.com/v1"
	}

	normalized = strings.TrimSuffix(normalized, "#")
	normalized = strings.TrimRight(normalized, "/")
	if providerType == ProviderTypeGemini {
		normalized = strings.TrimSuffix(normalized, "/v1beta/models")
		normalized = strings.TrimSuffix(normalized, "/v1beta/openai")
		return strings.TrimSuffix(normalized, "/v1beta")
	}
	if providerType == ProviderTypeAnthropic {
		normalized = strings.TrimSuffix(normalized, "/v1/messages")
		normalized = strings.TrimSuffix(normalized, "/v1/models")
		return strings.TrimSuffix(normalized, "/v1")
	}
	if providerType == ProviderTypeOpenAI || providerType == ProviderTypeOpenAICompatible {
		if strings.HasSuffix(normalized, "/v1") {
			return normalized
		}
		return normalized + "/v1"
	}
	return normalized
}

type openAIModelsResponse struct {
	Data   []openAIModelItem `json:"data"`
	Models []string          `json:"models"`
}

type openAIModelItem struct {
	ID string `json:"id"`
}

func authModeToDeploymentMode(mode string) string {
	if mode == config.AuthModeRequired {
		return "hosted"
	}
	return "local"
}

func publicStoreState(enabled bool) string {
	if enabled {
		return "shared"
	}
	return "memory"
}

func pluginRegistryStoreState(databaseURL string) string {
	if strings.TrimSpace(databaseURL) != "" {
		return "shared"
	}
	return "memory"
}

func nonEmpty(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return fallback
}
