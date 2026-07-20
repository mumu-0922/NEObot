package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchProviderModelsBoundedRejectsUnsafeURLs(t *testing.T) {
	for name, rawURL := range map[string]string{
		"scheme":   "ftp://provider.example/models",
		"userinfo": "https://user@provider.example/models",
		"query":    "https://provider.example/models?key=value",
		"fragment": "https://provider.example/models#private",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := fetchProviderModelsBounded(
				context.Background(),
				rawURL,
				ProviderTypeOpenAICompatible,
				"fixture-credential",
				time.Second,
			)
			if !errors.Is(err, ErrProviderConfigUnsupported) {
				t.Fatalf("fetchProviderModelsBounded() error = %v", err)
			}
		})
	}
}

func TestFetchProviderModelsBoundedDoesNotFollowRedirects(t *testing.T) {
	var targetCalled atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled.Store(true)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	_, err := fetchProviderModelsBounded(
		context.Background(),
		redirect.URL,
		ProviderTypeOpenAICompatible,
		"fixture-credential",
		time.Second,
	)
	if !errors.Is(err, ErrProviderConnectionTestFailed) {
		t.Fatalf("fetchProviderModelsBounded() error = %v", err)
	}
	if targetCalled.Load() {
		t.Fatal("connection test followed an upstream redirect")
	}
}

func TestFetchProviderModelsBoundedRejectsOversizedResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxProviderModelsResponseBytes+1)))
	}))
	defer upstream.Close()

	_, err := fetchProviderModelsBounded(
		context.Background(),
		upstream.URL,
		ProviderTypeOpenAICompatible,
		"fixture-credential",
		time.Second,
	)
	if !errors.Is(err, ErrProviderConnectionTestFailed) {
		t.Fatalf("fetchProviderModelsBounded() error = %v", err)
	}
}

func TestFetchProviderModelsBoundedCapsNormalizedModels(t *testing.T) {
	modelIDs := make([]map[string]string, 0, maxProviderConnectionModels+3)
	modelIDs = append(modelIDs, map[string]string{
		"id": strings.Repeat("x", maxProviderConnectionModelIDBytes+1),
	})
	for index := 0; index < maxProviderConnectionModels+2; index++ {
		modelIDs = append(modelIDs, map[string]string{
			"id": fmt.Sprintf("model-%04d", index),
		})
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": modelIDs})
	}))
	defer upstream.Close()

	models, err := fetchProviderModelsBounded(
		context.Background(),
		upstream.URL,
		ProviderTypeOpenAICompatible,
		"fixture-credential",
		time.Second,
	)
	if err != nil {
		t.Fatalf("fetchProviderModelsBounded() error = %v", err)
	}
	if len(models) != maxProviderConnectionModels {
		t.Fatalf("models length = %d, want %d", len(models), maxProviderConnectionModels)
	}
	if models[0] != "model-0000" || models[len(models)-1] != "model-2047" {
		t.Fatalf("bounded models = first %q, last %q", models[0], models[len(models)-1])
	}
}

func TestAnthropicProviderEndpointNormalization(t *testing.T) {
	for input, want := range map[string]string{
		"":                                      "https://api.anthropic.com",
		"default":                               "https://api.anthropic.com",
		"https://api.anthropic.com/v1":          "https://api.anthropic.com",
		"https://api.anthropic.com/v1/messages": "https://api.anthropic.com",
		"https://proxy.example/anthropic/v1/models": "https://proxy.example/anthropic",
	} {
		if got := normalizeProviderBaseURL(input, ProviderTypeAnthropic); got != want {
			t.Fatalf("normalizeProviderBaseURL(%q) = %q, want %q", input, got, want)
		}
	}
	if got := providerModelsURL("https://api.anthropic.com/v1/messages", ProviderTypeAnthropic); got != "https://api.anthropic.com/v1/models" {
		t.Fatalf("providerModelsURL() = %q", got)
	}
}
