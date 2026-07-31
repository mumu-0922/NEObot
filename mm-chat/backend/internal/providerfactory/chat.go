package providerfactory

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

var ErrUnsupportedChatProvider = errors.New("chat provider type is unsupported")

type ChatConfig struct {
	ProviderID         string
	Type               runtimeconfig.ProviderType
	BaseURL            string
	APIKey             string
	Timeout            time.Duration
	HTTPClient         *http.Client
	UseOpenAIResponses bool
}

func NewChatProvider(cfg ChatConfig) (chat.Provider, error) {
	providerID := strings.TrimSpace(cfg.ProviderID)
	switch normalizeProviderType(cfg.Type) {
	case runtimeconfig.ProviderTypeOpenAI:
		return chat.NewOpenAIProvider(chat.OpenAICompatibleProviderConfig{
			BaseURL: openAIBaseURL(cfg.BaseURL), APIKey: cfg.APIKey,
			ProviderID: providerID, Timeout: cfg.Timeout, HTTPClient: cfg.HTTPClient,
		})
	case runtimeconfig.ProviderTypeOpenAICompatible:
		providerConfig := chat.OpenAICompatibleProviderConfig{
			BaseURL: openAIBaseURL(cfg.BaseURL), APIKey: cfg.APIKey,
			ProviderID: providerID, Timeout: cfg.Timeout, HTTPClient: cfg.HTTPClient,
		}
		if cfg.UseOpenAIResponses {
			return chat.NewOpenAIProvider(providerConfig)
		}
		return chat.NewOpenAICompatibleProvider(providerConfig)
	case runtimeconfig.ProviderTypeGemini:
		return chat.NewGeminiProvider(chat.OpenAICompatibleProviderConfig{
			BaseURL: strings.TrimSpace(cfg.BaseURL), APIKey: cfg.APIKey,
			ProviderID: providerID, Timeout: cfg.Timeout, HTTPClient: cfg.HTTPClient,
		})
	case runtimeconfig.ProviderTypeAnthropic:
		return chat.NewAnthropicProvider(chat.AnthropicProviderConfig{
			BaseURL: strings.TrimSpace(cfg.BaseURL), APIKey: cfg.APIKey,
			ProviderID: providerID, Timeout: cfg.Timeout, HTTPClient: cfg.HTTPClient,
		})
	default:
		return nil, ErrUnsupportedChatProvider
	}
}

func normalizeProviderType(providerType runtimeconfig.ProviderType) runtimeconfig.ProviderType {
	switch strings.ToLower(strings.TrimSpace(string(providerType))) {
	case "openai":
		return runtimeconfig.ProviderTypeOpenAI
	case "openai compatible", "openai_compatible", "openai-compatible", "":
		return runtimeconfig.ProviderTypeOpenAICompatible
	case "gemini", "google gemini", "google_gemini":
		return runtimeconfig.ProviderTypeGemini
	case "anthropic", "anthropic claude", "anthropic_claude", "claude":
		return runtimeconfig.ProviderTypeAnthropic
	default:
		return providerType
	}
}

func openAIBaseURL(raw string) string {
	baseURL := strings.TrimSpace(raw)
	if baseURL == "" || baseURL == "default" {
		return "https://api.openai.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "#")
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return baseURL + "/v1"
}
