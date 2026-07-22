package chat

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const (
	GeminiProviderID        = "gemini"
	defaultGeminiServiceURL = "https://generativelanguage.googleapis.com"
	geminiOpenAIPath        = "/v1beta/openai"
)

// GeminiProvider uses Google's documented OpenAI-compatible chat surface while
// the runtime configuration service keeps using the native Gemini Models API.
// Embedding the shared provider preserves streaming, images, and tool planning
// without creating a second Chat Completions parser.
type GeminiProvider struct {
	*OpenAICompatibleProvider
	nativeBaseURL string
	nativeClient  *http.Client
}

func NewGeminiProvider(
	cfg OpenAICompatibleProviderConfig,
) (*GeminiProvider, error) {
	baseURL, err := normalizeGeminiOpenAIBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	nativeBaseURL, err := normalizeGeminiNativeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	cfg.BaseURL = baseURL
	provider, err := NewOpenAICompatibleProvider(cfg)
	if err != nil {
		return nil, err
	}
	clientCopy := *provider.client
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &GeminiProvider{
		OpenAICompatibleProvider: provider,
		nativeBaseURL:            nativeBaseURL,
		nativeClient:             &clientCopy,
	}, nil
}

func normalizeGeminiNativeBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" || value == "default" {
		value = defaultGeminiServiceURL
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("gemini provider base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("gemini provider base url must use http or https")
	}
	path := strings.TrimRight(parsed.Path, "/")
	path = strings.TrimSuffix(path, "/v1beta/openai")
	path = strings.TrimSuffix(path, "/v1beta/models")
	path = strings.TrimSuffix(path, "/v1beta")
	parsed.Path = strings.TrimRight(path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeGeminiOpenAIBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" || value == "default" {
		value = defaultGeminiServiceURL
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("gemini provider base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("gemini provider base url must use http or https")
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, geminiOpenAIPath):
	case strings.HasSuffix(path, "/v1beta"):
		path += "/openai"
	default:
		path += geminiOpenAIPath
	}
	parsed.Path = path
	return strings.TrimRight(parsed.String(), "/"), nil
}
