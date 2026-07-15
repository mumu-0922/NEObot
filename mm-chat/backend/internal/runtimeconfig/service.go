package runtimeconfig

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"sync"

	"neo-chat/mm-chat/backend/internal/config"
)

const (
	serverDefaultProviderID = "SERVER_DEFAULT"
	byokAlgorithm           = "RSA-OAEP-256"
)

var (
	ErrBYOKNotConfigured         = errors.New("byok key is not configured")
	ErrProviderModelsUnsupported = errors.New("provider model listing is only available for server-default providers")
	ErrPlaintextProviderSecret   = errors.New("plaintext provider secrets are not accepted")
)

type Service struct {
	cfg config.Config

	byokMu        sync.Mutex
	ephemeralBYOK *rsa.PrivateKey
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
			PluginRegistryStore:     "memory",
		},
	}
}

func (s *Service) ProviderModels(request ProviderModelsRequest) (ProviderModelsResponse, error) {
	provider := request.Provider
	if strings.TrimSpace(provider.APIKey) != "" {
		return ProviderModelsResponse{}, ErrPlaintextProviderSecret
	}
	if len(provider.APIKeySecret) > 0 {
		return ProviderModelsResponse{}, ErrProviderModelsUnsupported
	}

	if provider.Source != "" && provider.Source != "server-default" {
		return ProviderModelsResponse{}, ErrProviderModelsUnsupported
	}
	if provider.Source == "" && strings.TrimSpace(provider.Type) != "" {
		return ProviderModelsResponse{}, ErrProviderModelsUnsupported
	}

	return ProviderModelsResponse{Models: splitModels(s.cfg.Provider.Model)}, nil
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

func nonEmpty(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return fallback
}
