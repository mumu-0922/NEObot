package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicProviderStreamsHistoryImageThinkingAndUsage(t *testing.T) {
	var request anthropicMessagesRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "anthropic-key" ||
			r.Header.Get("anthropic-version") != anthropicVersion ||
			r.Header.Get("Authorization") != "" {
			t.Fatalf("Anthropic headers are invalid")
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hidden\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Cla\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ude\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: upstream.URL + "/v1/messages", APIKey: "anthropic-key",
		ProviderID: "CLAUDE-STORED",
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}
	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		ModelRef:     ModelRef{ProviderID: "CLAUDE-STORED", ModelID: "claude-sonnet-4-5"},
		SystemPrompt: "answer safely", UseReasoning: true,
		Messages: []ProviderMessage{
			{Role: "assistant", Content: "previous"},
			{
				Role: "user", Content: "inspect image",
				Attachments: []ProviderAttachment{{
					MimeType: "image/webp", Data: []byte("image-fixture"),
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	var content strings.Builder
	var usage *TokenUsage
	for event := range events {
		if event.Error != nil {
			t.Fatalf("provider event error = %v", event.Error)
		}
		switch event.Type {
		case ProviderEventDelta:
			content.WriteString(event.Delta)
		case ProviderEventUsage:
			usage = event.Usage
		}
	}
	if content.String() != "Claude" {
		t.Fatalf("content = %q", content.String())
	}
	if usage == nil || usage.PromptTokens != 11 || usage.CompletionTokens != 3 || usage.TotalTokens != 14 {
		t.Fatalf("usage = %#v", usage)
	}
	if request.Model != "claude-sonnet-4-5" || request.System != "answer safely" ||
		!request.Stream || request.MaxTokens != defaultAnthropicMaxTokens ||
		request.Thinking == nil || request.Thinking.BudgetTokens != defaultAnthropicThinkingTokens {
		t.Fatalf("request = %#v", request)
	}
	if len(request.Messages) != 2 || request.Messages[0].Role != "assistant" {
		t.Fatalf("messages = %#v", request.Messages)
	}
	parts, ok := request.Messages[1].Content.([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("image content = %#v", request.Messages[1].Content)
	}
	imageBlock, ok := parts[1].(map[string]any)
	if !ok || imageBlock["type"] != "image" {
		t.Fatalf("image block = %#v", imageBlock)
	}
	source, ok := imageBlock["source"].(map[string]any)
	if !ok || source["type"] != "base64" || source["media_type"] != "image/webp" || source["data"] == "" {
		t.Fatalf("image source = %#v", source)
	}
}

func TestAnthropicProviderMapsMaximumReasoningToBoundedBudget(t *testing.T) {
	var request anthropicMessagesRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: upstream.URL, APIKey: "anthropic-key", ProviderID: "ANTHROPIC",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt: "hard problem", UseReasoning: true, ReasoningEffort: ReasoningEffortMax,
		ModelRef: ModelRef{ProviderID: "ANTHROPIC", ModelID: "claude-opus"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	if request.Thinking == nil || request.Thinking.BudgetTokens != 16_384 {
		t.Fatalf("thinking = %#v", request.Thinking)
	}
	if request.MaxTokens != 20_480 || request.Thinking.BudgetTokens >= request.MaxTokens {
		t.Fatalf("max_tokens/budget = %d/%d", request.MaxTokens, request.Thinking.BudgetTokens)
	}
}

func TestAnthropicProviderPlansNativeTools(t *testing.T) {
	var request anthropicMessagesRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"toolu_1","name":"lookup_weather","input":{"city":"Shanghai"}}]}`))
	}))
	defer upstream.Close()
	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: upstream.URL, APIKey: "anthropic-key", ProviderID: "ANTHROPIC",
	})
	if err != nil {
		t.Fatal(err)
	}
	calls, err := provider.PlanTools(context.Background(), ToolPlanRequest{
		Prompt: "weather", ModelRef: ModelRef{ProviderID: "ANTHROPIC", ModelID: "claude"},
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunctionDefinition{
			Name: "lookup_weather", Description: "Look up weather",
			Parameters: map[string]any{"type": "object"},
		}}},
	})
	if err != nil {
		t.Fatalf("PlanTools() error = %v", err)
	}
	if len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Name != "lookup_weather" ||
		calls[0].Args["city"] != "Shanghai" {
		t.Fatalf("calls = %#v", calls)
	}
	if request.Stream || len(request.Tools) != 1 || request.Tools[0].Name != "lookup_weather" ||
		request.ToolChoice == nil || request.ToolChoice.Type != "auto" {
		t.Fatalf("tool request = %#v", request)
	}
}

func TestAnthropicProviderRejectsUnsupportedImagesBeforeRequest(t *testing.T) {
	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: "https://anthropic.example", APIKey: "anthropic-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.StreamChat(context.Background(), ProviderRequest{
		ModelRef: ModelRef{ProviderID: "anthropic", ModelID: "claude"},
		Messages: []ProviderMessage{{Role: "user", Attachments: []ProviderAttachment{{
			MimeType: "image/svg+xml", Data: []byte("svg"),
		}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported image") {
		t.Fatalf("StreamChat() error = %v", err)
	}
}

func TestAnthropicProviderDoesNotFollowRedirects(t *testing.T) {
	redirectFollowed := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectFollowed <- struct{}{}
		if r.Header.Get("x-api-key") != "" {
			t.Fatal("Anthropic API key reached redirect target")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()

	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: upstream.URL, APIKey: "anthropic-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.StreamChat(context.Background(), ProviderRequest{
		ModelRef: ModelRef{ProviderID: "anthropic", ModelID: "claude"},
		Prompt:   "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "status 307") {
		t.Fatalf("StreamChat() error = %v", err)
	}
	select {
	case <-redirectFollowed:
		t.Fatal("Anthropic provider followed upstream redirect")
	default:
	}
}

func TestNormalizeAnthropicServiceBaseURL(t *testing.T) {
	tests := map[string]string{
		"":                                      defaultAnthropicServiceURL,
		"default":                               defaultAnthropicServiceURL,
		"https://api.anthropic.com/v1":          defaultAnthropicServiceURL,
		"https://api.anthropic.com/v1/messages": defaultAnthropicServiceURL,
		"https://proxy.example/anthropic/v1/models": "https://proxy.example/anthropic",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := normalizeAnthropicServiceBaseURL(input)
			if err != nil {
				t.Fatalf("normalizeAnthropicServiceBaseURL() error = %v", err)
			}
			if got != want {
				t.Fatalf("normalizeAnthropicServiceBaseURL(%q) = %q, want %q", input, got, want)
			}
		})
	}
	for _, input := range []string{
		"ftp://anthropic.example", "https://user@anthropic.example", "https://anthropic.example?key=x",
	} {
		if _, err := normalizeAnthropicServiceBaseURL(input); err == nil {
			t.Fatalf("normalizeAnthropicServiceBaseURL(%q) error = nil", input)
		}
	}
}
