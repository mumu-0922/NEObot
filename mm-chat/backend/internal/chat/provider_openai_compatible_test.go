package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleProviderStreamsDeltasAndUsage(t *testing.T) {
	const apiKey = "test-secret-token-1234567890"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Fatalf("Authorization header mismatch")
		}

		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if payload.Model != "gpt-5.5" {
			t.Fatalf("model = %q, want gpt-5.5", payload.Model)
		}
		if !payload.Stream {
			t.Fatalf("stream = false, want true")
		}
		if len(payload.Messages) != 2 {
			t.Fatalf("messages len = %d, want 2", len(payload.Messages))
		}
		if payload.Messages[0].Role != "system" || payload.Messages[0].Content != "be terse" {
			t.Fatalf("system message = %#v", payload.Messages[0])
		}
		if payload.Messages[1].Role != "user" || payload.Messages[1].Content != "hello" {
			t.Fatalf("user message = %#v", payload.Messages[1])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      server.URL + "/v1/",
		APIKey:       apiKey,
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:       "hello",
		SystemPrompt: "be terse",
		ModelRef:     ModelRef{ProviderID: "openai_compatible", ModelID: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	var deltas []string
	var usage *TokenUsage
	for event := range events {
		if event.Error != nil {
			t.Fatalf("provider event error = %v", event.Error)
		}
		switch event.Type {
		case ProviderEventDelta:
			deltas = append(deltas, event.Delta)
		case ProviderEventUsage:
			usage = event.Usage
		}
	}

	if strings.Join(deltas, "") != "pong" {
		t.Fatalf("deltas = %q, want pong", strings.Join(deltas, ""))
	}
	if usage == nil || usage.PromptTokens != 2 || usage.CompletionTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestOpenAICompatibleProviderUsesDefaultModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if payload.Model != "gpt-default" {
			t.Fatalf("model = %q, want gpt-default", payload.Model)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-secret-token",
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:   "hello",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	for event := range events {
		if event.Error != nil {
			t.Fatalf("provider event error = %v", event.Error)
		}
	}
}

func TestOpenAICompatibleProviderNon200DoesNotLeakKey(t *testing.T) {
	const apiKey = "test-secret-token-should-not-leak"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key: "+apiKey, http.StatusUnauthorized)
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      server.URL,
		APIKey:       apiKey,
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	_, err = provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:   "hello",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
	})
	if err == nil {
		t.Fatalf("StreamChat() error = nil, want error")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("provider error leaked api key: %v", err)
	}
}

func TestOpenAICompatibleProviderInvalidStreamFrameYieldsErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: not-json\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-secret-token",
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:   "hello",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	var gotError bool
	for event := range events {
		if event.Error != nil {
			gotError = true
		}
	}
	if !gotError {
		t.Fatalf("gotError = false, want true")
	}
}

func TestOpenAICompatibleProviderEOFWithoutDoneYieldsErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-secret-token",
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:   "hello",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	var deltas []string
	var gotError bool
	for event := range events {
		if event.Error != nil {
			gotError = true
			continue
		}
		if event.Type == ProviderEventDelta {
			deltas = append(deltas, event.Delta)
		}
	}
	if strings.Join(deltas, "") != "partial" {
		t.Fatalf("deltas = %q, want partial", strings.Join(deltas, ""))
	}
	if !gotError {
		t.Fatalf("gotError = false, want true")
	}
}

func TestOpenAICompatibleProviderNonSSE200YieldsErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"not a stream"}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      server.URL,
		APIKey:       "test-secret-token",
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:   "hello",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	var gotError bool
	for event := range events {
		if event.Error != nil {
			gotError = true
		}
	}
	if !gotError {
		t.Fatalf("gotError = false, want true")
	}
}

func TestOpenAICompatibleProviderRejectsUnsupportedProviderID(t *testing.T) {
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      "https://example.test/v1",
		APIKey:       "test-secret-token",
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	_, err = provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:   "hello",
		ModelRef: ModelRef{ProviderID: "anthropic", ModelID: "claude-test"},
	})
	if err == nil {
		t.Fatalf("StreamChat() error = nil, want unsupported provider error")
	}

	var validationError ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error type = %T, want ValidationError", err)
	}
	if validationError.Code != "UNSUPPORTED_PROVIDER" {
		t.Fatalf("validation code = %q, want UNSUPPORTED_PROVIDER", validationError.Code)
	}
}

func TestOpenAICompatibleProviderResolvesAliasesToCanonicalModelRef(t *testing.T) {
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      "https://example.test/v1",
		APIKey:       "test-secret-token",
		DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	resolved, err := provider.ResolveModelRef(ModelRef{
		ProviderID: " openai-compatible ",
	})
	if err != nil {
		t.Fatalf("ResolveModelRef() error = %v", err)
	}
	if resolved.ProviderID != OpenAICompatibleProviderID {
		t.Fatalf("ProviderID = %q, want %q", resolved.ProviderID, OpenAICompatibleProviderID)
	}
	if resolved.ModelID != "gpt-default" {
		t.Fatalf("ModelID = %q, want gpt-default", resolved.ModelID)
	}
}
