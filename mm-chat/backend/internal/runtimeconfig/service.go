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
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
)

const (
	serverDefaultProviderID = "SERVER_DEFAULT"
	byokAlgorithm           = "RSA-OAEP-256+A256GCM"
)

var (
	ErrBYOKNotConfigured         = errors.New("byok key is not configured")
	ErrProviderModelsUnsupported = errors.New("provider model listing is only available for server-default providers")
	ErrPlaintextProviderSecret   = errors.New("plaintext provider secrets are not accepted")
	ErrProviderSecretRequired    = errors.New("provider api key is required")
	ErrProviderConfigUnsupported = errors.New("provider configuration is unsupported")
	ErrDatabaseRequired          = errors.New("database is required")
	ErrProviderConfigNotFound    = errors.New("provider configuration was not found")
)

const maxProviderModelsResponseBytes = 2 << 20

type Service struct {
	cfg  config.Config
	repo ProviderConfigRepository

	byokMu        sync.Mutex
	ephemeralBYOK *rsa.PrivateKey
}

type ServiceOption func(*Service)

func WithProviderConfigRepository(repo ProviderConfigRepository) ServiceOption {
	return func(s *Service) {
		s.repo = repo
	}
}

type ProviderConfigRepository interface {
	GetProviderConfig(ctx context.Context, userID string, providerID string) (StoredProviderConfig, bool, error)
	ListProviderConfigs(ctx context.Context, userID string) ([]StoredProviderConfig, error)
	UpsertProviderConfig(ctx context.Context, input UpsertProviderConfigInput) (StoredProviderConfig, error)
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
	Type    ProviderType `json:"type"`
	BaseURL string       `json:"baseUrl"`
	Models  []string     `json:"models"`
	Enabled bool         `json:"enabled"`
}

type UpsertProviderConfigInput struct {
	UserID             string
	ProviderID         string
	Label              string
	EncryptedSecretRef string
	Config             StoredProviderConfigPayload
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

	return PublicConfig{
		ModelProvider: ModelProviderConfig{
			Available:     provider.Available,
			ID:            serverDefaultProviderID,
			Name:          provider.Name,
			Type:          provider.Type,
			Models:        append([]string(nil), provider.Models...),
			ModelMetadata: map[string]any{},
			DefaultModels: map[string]string{},
		},
		Search: SearchConfig{Available: false},
		RAG: RAGConfig{
			VectorStoreAvailable:        false,
			DocumentProcessingAvailable: false,
		},
		Voice: VoiceConfig{
			ElevenLabsAvailable: false,
			MimoAvailable:       false,
			DefaultSTTAvailable: false,
			DefaultTTSAvailable: false,
		},
		Deployment: DeploymentConfig{
			Mode:                    authModeToDeploymentMode(s.cfg.Auth.Mode),
			AccessPasswordEnabled:   false,
			TrustedProxyHeaders:     false,
			BYOKStableKeyConfigured: strings.TrimSpace(s.cfg.BYOK.PrivateKeyPEM) != "",
			BYOKEphemeralAllowed:    s.cfg.BYOK.AllowEphemeralKey,
			RateLimitStore:          publicStoreState(s.cfg.Redis.RateLimitEnabled),
			DocumentParseJobStore:   "memory",
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
		if resolved.Type != ProviderTypeOpenAI && resolved.Type != ProviderTypeOpenAICompatible {
			return ProviderModelsResponse{}, ErrProviderModelsUnsupported
		}
		models, err := fetchOpenAICompatibleModels(providerModelsURL(resolved.BaseURL, resolved.Type), resolved.APIKey, s.cfg.Provider.Timeout)
		if err != nil {
			return ProviderModelsResponse{}, err
		}
		return ProviderModelsResponse{Models: models}, nil
	}
	if provider.Source == "server-default" ||
		(provider.Source == "" && strings.TrimSpace(provider.Type) == "") {
		resolved := s.serverDefaultProviderForContext(ctx)
		if strings.TrimSpace(resolved.APIKey) == "" {
			return ProviderModelsResponse{Models: append([]string(nil), resolved.Models...)}, nil
		}
		if resolved.Type != ProviderTypeOpenAI && resolved.Type != ProviderTypeOpenAICompatible {
			return ProviderModelsResponse{}, ErrProviderModelsUnsupported
		}
		models, err := fetchOpenAICompatibleModels(providerModelsURL(resolved.BaseURL, resolved.Type), resolved.APIKey, s.cfg.Provider.Timeout)
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
	if providerType != ProviderTypeOpenAI && providerType != ProviderTypeOpenAICompatible {
		return ProviderModelsResponse{}, ErrProviderModelsUnsupported
	}

	models, err := fetchOpenAICompatibleModels(providerModelsURL(provider.BaseURL, providerType), apiKey, s.cfg.Provider.Timeout)
	if err != nil {
		return ProviderModelsResponse{}, err
	}
	return ProviderModelsResponse{Models: models}, nil
}

type resolvedServerDefaultProvider struct {
	Name      string
	Type      ProviderType
	BaseURL   string
	Models    []string
	Enabled   bool
	Available bool
	APIKey    string
	SecretRef string
}

func (s *Service) AdminProviderConfig(ctx context.Context) (AdminProviderConfigResponse, error) {
	provider := s.serverDefaultProviderForContext(ctx)
	return AdminProviderConfigResponse{
		ID:        serverDefaultProviderID,
		Name:      provider.Name,
		Type:      provider.Type,
		BaseURL:   provider.BaseURL,
		Models:    append([]string(nil), provider.Models...),
		Enabled:   provider.Enabled,
		HasAPIKey: strings.TrimSpace(provider.APIKey) != "" || strings.TrimSpace(provider.SecretRef) != "",
		Source:    "server-default",
	}, nil
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
			adminProviderResponse(s.envServerDefaultProvider(), serverDefaultProviderID, "server-default"),
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
	if providerID == "" || len(providerID) > 128 {
		return AdminProviderConfigResponse{}, ErrProviderConfigUnsupported
	}

	current := s.envServerDefaultProvider()
	if stored, ok, err := s.repo.GetProviderConfig(ctx, auth.UserOrDevelopment(ctx).ID, providerID); err != nil {
		return AdminProviderConfigResponse{}, err
	} else if ok {
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
	secretRef := strings.TrimSpace(current.SecretRef)

	if request.ClearAPIKey {
		secretRef = ""
	}
	if len(request.APIKeySecret) > 0 {
		envelope, err := parseEncryptedSecretEnvelope(request.APIKeySecret)
		if err != nil {
			return AdminProviderConfigResponse{}, err
		}
		if _, err := s.DecryptOptionalSecret(envelope, "provider:"+string(providerType)); err != nil {
			return AdminProviderConfigResponse{}, err
		}
		encoded, err := json.Marshal(envelope)
		if err != nil {
			return AdminProviderConfigResponse{}, ErrProviderConfigUnsupported
		}
		secretRef = string(encoded)
	}

	user := auth.UserOrDevelopment(ctx)
	enabled := request.Enabled
	if providerID == serverDefaultProviderID {
		enabled = true
	}
	stored, err := s.repo.UpsertProviderConfig(ctx, UpsertProviderConfigInput{
		UserID:             user.ID,
		ProviderID:         providerID,
		Label:              name,
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Type:    providerType,
			BaseURL: baseURL,
			Models:  models,
			Enabled: enabled,
		},
	})
	if err != nil {
		return AdminProviderConfigResponse{}, err
	}

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
	if providerID == "" || providerID == serverDefaultProviderID {
		return ErrProviderConfigUnsupported
	}
	return s.repo.DeleteProviderConfig(ctx, auth.UserOrDevelopment(ctx).ID, providerID)
}

func (s *Service) serverDefaultProviderForContext(ctx context.Context) resolvedServerDefaultProvider {
	base := s.envServerDefaultProvider()
	if s.repo == nil {
		return base
	}
	stored, ok, err := s.repo.GetProviderConfig(ctx, auth.UserOrDevelopment(ctx).ID, serverDefaultProviderID)
	if err != nil || !ok {
		return base
	}
	return s.resolveStoredServerDefault(stored)
}

func (s *Service) envServerDefaultProvider() resolvedServerDefaultProvider {
	models := splitModels(s.cfg.Provider.Model)
	apiKey := strings.TrimSpace(s.cfg.Provider.APIKey)
	available := len(models) > 0 || apiKey != ""
	return resolvedServerDefaultProvider{
		Name:      nonEmpty(s.cfg.Provider.Name, config.DefaultProviderName),
		Type:      normalizeProviderType(s.cfg.Provider.Type),
		BaseURL:   strings.TrimSpace(s.cfg.Provider.BaseURL),
		Models:    models,
		Enabled:   true,
		Available: available,
		APIKey:    apiKey,
	}
}

func (s *Service) resolveStoredServerDefault(stored StoredProviderConfig) resolvedServerDefaultProvider {
	base := s.envServerDefaultProvider()
	name := nonEmpty(stored.Label, base.Name)
	providerType := stored.Config.Type
	if providerType == "" {
		providerType = base.Type
	}
	baseURL := strings.TrimSpace(stored.Config.BaseURL)
	if baseURL == "" {
		baseURL = base.BaseURL
	}
	models := normalizeModelList(stored.Config.Models)
	if len(models) == 0 {
		models = append([]string(nil), base.Models...)
	}
	secretRef := strings.TrimSpace(stored.EncryptedSecretRef)
	apiKey := base.APIKey
	if secretRef != "" {
		if envelope, err := parseStoredSecretRef(secretRef); err == nil {
			if plaintext, err := s.DecryptOptionalSecret(envelope, "provider:"+string(providerType)); err == nil {
				apiKey = strings.TrimSpace(plaintext)
			}
		}
	}
	available := len(models) > 0 || apiKey != "" || secretRef != ""
	return resolvedServerDefaultProvider{
		Name:      name,
		Type:      providerType,
		BaseURL:   baseURL,
		Models:    models,
		Enabled:   stored.Config.Enabled,
		Available: available,
		APIKey:    apiKey,
		SecretRef: secretRef,
	}
}

func (s *Service) resolveStoredProvider(stored StoredProviderConfig) resolvedServerDefaultProvider {
	providerType := stored.Config.Type
	if providerType == "" {
		providerType = ProviderTypeOpenAICompatible
	}
	secretRef := strings.TrimSpace(stored.EncryptedSecretRef)
	apiKey := ""
	if secretRef != "" {
		if envelope, err := parseStoredSecretRef(secretRef); err == nil {
			if plaintext, err := s.DecryptOptionalSecret(envelope, "provider:"+string(providerType)); err == nil {
				apiKey = strings.TrimSpace(plaintext)
			}
		}
	}
	models := normalizeModelList(stored.Config.Models)
	return resolvedServerDefaultProvider{
		Name:      nonEmpty(stored.Label, "New Provider"),
		Type:      providerType,
		BaseURL:   strings.TrimSpace(stored.Config.BaseURL),
		Models:    models,
		Enabled:   stored.Config.Enabled,
		Available: len(models) > 0 || apiKey != "" || secretRef != "",
		APIKey:    apiKey,
		SecretRef: secretRef,
	}
}

func adminProviderResponse(
	provider resolvedServerDefaultProvider,
	providerID string,
	source string,
) AdminProviderConfigResponse {
	return AdminProviderConfigResponse{
		ID:        providerID,
		Name:      provider.Name,
		Type:      provider.Type,
		BaseURL:   provider.BaseURL,
		Models:    append([]string(nil), provider.Models...),
		Enabled:   provider.Enabled,
		HasAPIKey: strings.TrimSpace(provider.APIKey) != "" || strings.TrimSpace(provider.SecretRef) != "",
		Source:    source,
	}
}

func parseStoredSecretRef(value string) (*EncryptedSecretEnvelope, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var envelope EncryptedSecretEnvelope
	if err := json.Unmarshal([]byte(value), &envelope); err != nil {
		return nil, ErrBYOKNotConfigured
	}
	return &envelope, nil
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
	ID      string
	Name    string
	Type    ProviderType
	BaseURL string
	APIKey  string
}

func (s *Service) ResolveServerDefaultProvider(ctx context.Context) (ResolvedProvider, error) {
	provider := s.serverDefaultProviderForContext(ctx)
	if strings.TrimSpace(provider.APIKey) == "" {
		return ResolvedProvider{}, ErrProviderSecretRequired
	}
	return ResolvedProvider{
		ID:      serverDefaultProviderID,
		Name:    provider.Name,
		Type:    provider.Type,
		BaseURL: provider.BaseURL,
		APIKey:  provider.APIKey,
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
	provider := s.resolveStoredProvider(stored)
	if strings.TrimSpace(provider.APIKey) == "" {
		return ResolvedProvider{}, ErrProviderSecretRequired
	}
	return ResolvedProvider{
		ID:      providerID,
		Name:    provider.Name,
		Type:    provider.Type,
		BaseURL: provider.BaseURL,
		APIKey:  provider.APIKey,
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
	return normalized + "/models"
}

func normalizeProviderBaseURL(baseURL string, providerType ProviderType) string {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" || normalized == "default" {
		if providerType == ProviderTypeGemini {
			return "https://generativelanguage.googleapis.com"
		}
		return "https://api.openai.com/v1"
	}

	normalized = strings.TrimSuffix(normalized, "#")
	normalized = strings.TrimRight(normalized, "/")
	if providerType == ProviderTypeGemini {
		return strings.TrimSuffix(normalized, "/v1beta")
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

func fetchOpenAICompatibleModels(rawURL string, apiKey string, timeout time.Duration) ([]string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, ErrProviderConfigUnsupported
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrProviderConfigUnsupported
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrProviderConfigUnsupported
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider model listing request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("provider model listing returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderModelsResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("provider model listing read failed: %w", err)
	}
	if len(body) > maxProviderModelsResponseBytes {
		return nil, ErrProviderConfigUnsupported
	}

	var decoded openAIModelsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("provider model listing decode failed: %w", err)
	}
	models := make([]string, 0, len(decoded.Data)+len(decoded.Models))
	seen := map[string]struct{}{}
	addModel := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	for _, item := range decoded.Data {
		addModel(item.ID)
	}
	for _, model := range decoded.Models {
		addModel(model)
	}
	return models, nil
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
