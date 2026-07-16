package runtimeconfig

import (
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
)

const maxProviderModelsResponseBytes = 2 << 20

type Service struct {
	cfg config.Config

	byokMu        sync.Mutex
	ephemeralBYOK *rsa.PrivateKey
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

func NewService(cfg config.Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) PublicConfig() PublicConfig {
	models := splitModels(s.cfg.Provider.Model)
	providerType := normalizeProviderType(s.cfg.Provider.Type)
	available := len(models) > 0 || strings.TrimSpace(s.cfg.Provider.APIKey) != ""

	return PublicConfig{
		ModelProvider: ModelProviderConfig{
			Available:     available,
			ID:            serverDefaultProviderID,
			Name:          nonEmpty(s.cfg.Provider.Name, config.DefaultProviderName),
			Type:          providerType,
			Models:        models,
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
	provider := request.Provider
	if strings.TrimSpace(provider.APIKey) != "" {
		return ProviderModelsResponse{}, ErrPlaintextProviderSecret
	}

	if provider.Source != "" && provider.Source != "server-default" {
		return ProviderModelsResponse{}, ErrProviderModelsUnsupported
	}
	if provider.Source == "server-default" ||
		(provider.Source == "" && strings.TrimSpace(provider.Type) == "") {
		return ProviderModelsResponse{Models: splitModels(s.cfg.Provider.Model)}, nil
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
			"alg":     byokAlgorithm,
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
