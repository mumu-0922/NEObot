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
		if payload.ReasoningEffort != "xhigh" {
			t.Fatalf("reasoning_effort = %q, want xhigh", payload.ReasoningEffort)
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
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"checked\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning\":{\"opaque\":true}}}]}\n\n"))
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
		Prompt:          "hello",
		SystemPrompt:    "be terse",
		UseReasoning:    true,
		ReasoningEffort: ReasoningEffortXHigh,
		ModelRef:        ModelRef{ProviderID: "openai_compatible", ModelID: "gpt-5.5"},
	})
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}

	var deltas []string
	var reasoning []string
	var usage *TokenUsage
	for event := range events {
		if event.Error != nil {
			t.Fatalf("provider event error = %v", event.Error)
		}
		switch event.Type {
		case ProviderEventDelta:
			deltas = append(deltas, event.Delta)
		case ProviderEventReasoningDelta:
			reasoning = append(reasoning, event.ReasoningDelta)
		case ProviderEventUsage:
			usage = event.Usage
		}
	}

	if strings.Join(deltas, "") != "pong" {
		t.Fatalf("deltas = %q, want pong", strings.Join(deltas, ""))
	}
	if strings.Join(reasoning, "") != "checked" {
		t.Fatalf("reasoning = %q, want checked", strings.Join(reasoning, ""))
	}
	if usage == nil || usage.PromptTokens != 2 || usage.CompletionTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestOpenAICompatibleProviderStreamsFragmentedToolCallAndNativeContinuation(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if len(payload.Tools) != 1 || payload.Tools[0].Function.Name != searchWebToolName {
			t.Fatalf("tools = %#v", payload.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount {
		case 1:
			choice, ok := payload.ToolChoice.(map[string]any)
			if !ok || choice["type"] != "function" {
				t.Fatalf("forced tool_choice = %#v", payload.ToolChoice)
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"search_\",\"arguments\":\"{\\\"query\\\":\\\"latest \"}}]}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"web\",\"arguments\":\"fixture\\\"}\"}}]}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case 2:
			if payload.ToolChoice != ProviderToolChoiceAuto {
				t.Fatalf("continuation tool_choice = %#v, want auto", payload.ToolChoice)
			}
			if len(payload.Messages) != 3 {
				t.Fatalf("continuation messages = %#v", payload.Messages)
			}
			assistant := payload.Messages[1]
			if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 ||
				assistant.ToolCalls[0].ID != "call-1" ||
				assistant.ToolCalls[0].Function.Name != searchWebToolName {
				t.Fatalf("assistant continuation = %#v", assistant)
			}
			tool := payload.Messages[2]
			if tool.Role != "tool" || tool.ToolCallID != "call-1" ||
				!strings.Contains(tool.Content.(string), "[W1]") {
				t.Fatalf("tool continuation = %#v", tool)
			}
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"final [W1]\"}}]}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected provider request %d", requestCount)
		}
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL, APIKey: "test-secret-token", DefaultModel: "gpt-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	base := ProviderRequest{
		Prompt: "latest fixture",
		ModelRef: ModelRef{
			ProviderID: OpenAICompatibleProviderID,
			ModelID:    "gpt-test",
		},
	}
	first, err := provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: base,
		Tools:           []ToolDefinition{searchWebToolDefinition()},
		ToolChoice:      ProviderToolChoiceRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	var fragments []ProviderToolCallDelta
	var call ProviderToolCall
	for event := range first {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.ToolCallDelta != nil {
			fragments = append(fragments, *event.ToolCallDelta)
		}
		if event.ToolCall != nil {
			call = *event.ToolCall
		}
	}
	if len(fragments) != 2 || call.ID != "call-1" || call.Name != searchWebToolName ||
		call.Arguments != `{"query":"latest fixture"}` {
		t.Fatalf("fragments/call = %#v / %#v", fragments, call)
	}

	second, err := provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: base,
		Tools:           []ToolDefinition{searchWebToolDefinition()},
		ToolChoice:      ProviderToolChoiceAuto,
		Continuation: []ProviderToolExchange{{
			Calls: []ProviderToolCall{call},
			Results: []ProviderToolResult{{
				CallID:  call.ID,
				Name:    call.Name,
				Content: `{"ok":true,"sources":[{"marker":"[W1]"}]}`,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var content strings.Builder
	for event := range second {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
	}
	if content.String() != "final [W1]" || requestCount != 2 {
		t.Fatalf("content/requests = %q / %d", content.String(), requestCount)
	}
}

func TestOpenAICompatibleProviderRejectsOversizedStreamedToolArguments(t *testing.T) {
	arguments := strings.Repeat("x", maxOpenAICompatibleToolArgumentsBytes+1)
	reader := strings.NewReader("data: " + mustJSON(t, map[string]any{
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0,
				"id":    "call-large",
				"type":  "function",
				"function": map[string]any{
					"name":      searchWebToolName,
					"arguments": arguments,
				},
			}}},
		}},
	}) + "\n\ndata: [DONE]\n\n")
	events := make(chan ProviderEvent, 8)
	streamOpenAICompatibleEvents(context.Background(), reader, events)
	close(events)
	var completed *ProviderToolCall
	for event := range events {
		if event.ToolCall != nil {
			completed = event.ToolCall
		}
	}
	if completed == nil || completed.FailureCategory != "arguments_too_large" ||
		len(completed.Arguments) != maxOpenAICompatibleToolArgumentsBytes {
		t.Fatalf("completed tool call = %#v", completed)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
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
