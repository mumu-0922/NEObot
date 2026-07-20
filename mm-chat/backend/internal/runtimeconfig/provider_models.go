package runtimeconfig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxProviderConnectionTestDuration = 15 * time.Second
	maxProviderConnectionModels       = 2_048
	maxProviderConnectionModelIDBytes = 512
)

type geminiModelsResponse struct {
	Models []geminiModelItem `json:"models"`
}

type geminiModelItem struct {
	Name string `json:"name"`
}

func (s *Service) fetchProviderModelsForConnectionTest(
	ctx context.Context,
	provider resolvedServerDefaultProvider,
) ([]string, error) {
	providerType := normalizeProviderType(string(provider.Type))
	if providerType != ProviderTypeOpenAI &&
		providerType != ProviderTypeOpenAICompatible &&
		providerType != ProviderTypeGemini &&
		providerType != ProviderTypeAnthropic {
		return nil, ErrProviderConfigUnsupported
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

func fetchProviderModelsBounded(
	ctx context.Context,
	rawURL string,
	providerType ProviderType,
	apiKey string,
	timeout time.Duration,
) ([]string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, ErrProviderConfigUnsupported
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrProviderConfigUnsupported
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, ErrProviderSecretRequired
	}

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, ErrProviderConfigUnsupported
	}
	req.Header.Set("Accept", "application/json")
	if providerType == ProviderTypeGemini {
		req.Header.Set("x-goog-api-key", apiKey)
	} else if providerType == ProviderTypeAnthropic {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrProviderConnectionTestFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4_096))
		return nil, ErrProviderConnectionTestFailed
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderModelsResponseBytes+1))
	if err != nil || len(body) > maxProviderModelsResponseBytes {
		return nil, ErrProviderConnectionTestFailed
	}

	models := make([]string, 0)
	if providerType == ProviderTypeGemini {
		var decoded geminiModelsResponse
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, ErrProviderConnectionTestFailed
		}
		for _, item := range decoded.Models {
			models = append(models, strings.TrimPrefix(strings.TrimSpace(item.Name), "models/"))
		}
	} else {
		var decoded openAIModelsResponse
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, ErrProviderConnectionTestFailed
		}
		for _, item := range decoded.Data {
			models = append(models, item.ID)
		}
		models = append(models, decoded.Models...)
	}

	models = normalizeBoundedConnectionModels(models)
	if len(models) == 0 {
		return nil, ErrProviderConnectionTestFailed
	}
	return models, nil
}

func normalizeBoundedConnectionModels(models []string) []string {
	seen := make(map[string]struct{}, min(len(models), maxProviderConnectionModels))
	normalized := make([]string, 0, min(len(models), maxProviderConnectionModels))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > maxProviderConnectionModelIDBytes {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		normalized = append(normalized, model)
		if len(normalized) == maxProviderConnectionModels {
			break
		}
	}
	return normalized
}
