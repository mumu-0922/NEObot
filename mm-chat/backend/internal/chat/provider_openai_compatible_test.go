package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleProviderStreamsDeltasAndUsage(t *testing.T) {
	const apiKey = "example-fixture-provider-credential"

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
		if payload.ReasoningEffort != "high" {
			t.Fatalf("reasoning_effort = %q, want high", payload.ReasoningEffort)
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
		UseReasoning: true,
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

func TestOpenAICompatibleProviderSendsImageAttachments(t *testing.T) {
	const imageBytes = "\x89PNG\r\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if len(payload.Messages) != 1 {
			t.Fatalf("messages len = %d, want 1", len(payload.Messages))
		}
		parts, ok := payload.Messages[0].Content.([]any)
		if !ok {
			t.Fatalf("user content = %#v, want multimodal parts", payload.Messages[0].Content)
		}
		if len(parts) != 2 {
			t.Fatalf("content parts len = %d, want 2", len(parts))
		}
		textPart, ok := parts[0].(map[string]any)
		if !ok || textPart["type"] != "text" || textPart["text"] != "who is this?" {
			t.Fatalf("text part = %#v", parts[0])
		}
		imagePart, ok := parts[1].(map[string]any)
		if !ok || imagePart["type"] != "image_url" {
			t.Fatalf("image part = %#v", parts[1])
		}
		imageURL, ok := imagePart["image_url"].(map[string]any)
		if !ok {
			t.Fatalf("image_url part = %#v", imagePart["image_url"])
		}
		wantURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(imageBytes))
		if imageURL["url"] != wantURL {
			t.Fatalf("image url = %#v, want %q", imageURL["url"], wantURL)
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
		Prompt: "who is this?",
		Attachments: []ProviderAttachment{{
			FileID:   testFileID,
			FileName: "fixture.png",
			MimeType: "image/png",
			Data:     []byte(imageBytes),
		}},
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

func TestOpenAICompatibleProviderSendsConversationHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if len(payload.Messages) != 4 {
			t.Fatalf("messages = %#v, want system plus three conversation messages", payload.Messages)
		}
		wantRoles := []string{"system", "user", "assistant", "user"}
		wantContent := []string{"be consistent", "remember violet", "noted", "what color?"}
		for index := range wantRoles {
			if payload.Messages[index].Role != wantRoles[index] || payload.Messages[index].Content != wantContent[index] {
				t.Fatalf("message[%d] = %#v, want %s/%q", index, payload.Messages[index], wantRoles[index], wantContent[index])
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL, APIKey: "test-secret-token", DefaultModel: "gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		SystemPrompt: "be consistent",
		Messages: []ProviderMessage{
			{Role: "user", Content: "remember violet"},
			{Role: "assistant", Content: "noted"},
			{Role: "user", Content: "what color?"},
		},
		ModelRef: ModelRef{ProviderID: OpenAICompatibleProviderID},
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
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
		if payload.ReasoningEffort != "" {
			t.Fatalf("reasoning_effort = %q, want omitted", payload.ReasoningEffort)
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

func TestOpenAICompatibleProviderPlansFunctionCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if payload.Stream || payload.ToolChoice != "auto" || len(payload.Tools) != 1 {
			t.Fatalf("tool plan payload = %#v", payload)
		}
		if payload.Tools[0].Function.Name != "lookup_weather" {
			t.Fatalf("tool name = %q", payload.Tools[0].Function.Name)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup_weather","arguments":"{\"city\":\"Shanghai\"}"}}]}}]}`))
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
	calls, err := provider.PlanTools(context.Background(), ToolPlanRequest{
		Prompt:   "weather in Shanghai",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:       "lookup_weather",
				Parameters: map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("PlanTools() error = %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "lookup_weather" || calls[0].Args["city"] != "Shanghai" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestOpenAICompatibleProviderRejectsInvalidToolArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup_weather","arguments":"[]"}}]}}]}`))
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
	_, err = provider.PlanTools(context.Background(), ToolPlanRequest{
		Prompt:   "weather",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:       "lookup_weather",
				Parameters: map[string]any{"type": "object"},
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("PlanTools() error = %v, want invalid arguments", err)
	}
}

func TestOpenAICompatibleToolPlanNon200DoesNotLeakKey(t *testing.T) {
	const apiKey = "example-tool-plan-provider-credential"
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
	_, err = provider.PlanTools(context.Background(), ToolPlanRequest{
		Prompt:   "weather",
		ModelRef: ModelRef{ProviderID: "openai_compatible"},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:       "lookup_weather",
				Parameters: map[string]any{"type": "object"},
			},
		}},
	})
	if err == nil {
		t.Fatal("PlanTools() error = nil, want error")
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("tool plan error leaked api key: %v", err)
	}
}

func TestOpenAICompatibleProviderNon200DoesNotLeakKey(t *testing.T) {
	const apiKey = "example-stream-provider-credential"

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
