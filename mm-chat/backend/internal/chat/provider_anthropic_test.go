package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	var reasoning strings.Builder
	var usage *TokenUsage
	for event := range events {
		if event.Error != nil {
			t.Fatalf("provider event error = %v", event.Error)
		}
		switch event.Type {
		case ProviderEventDelta:
			content.WriteString(event.Delta)
		case ProviderEventReasoningDelta:
			reasoning.WriteString(event.ReasoningDelta)
		case ProviderEventUsage:
			usage = event.Usage
		}
	}
	if content.String() != "Claude" {
		t.Fatalf("content = %q", content.String())
	}
	if reasoning.String() != "hidden" {
		t.Fatalf("reasoning = %q, want hidden", reasoning.String())
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

func TestAnthropicProviderStreamsToolUseAndPreservesThinkingContinuation(t *testing.T) {
	var requests []anthropicMessagesRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request anthropicMessagesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":0}}}\n\n"))
			_, _ = w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\",\"signature\":\"\"}}\n\n"))
			_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"check sources\"}}\n\n"))
			_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig-123\"}}\n\n"))
			_, _ = w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
			_, _ = w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"search_web\",\"input\":{}}}\n\n"))
			_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\":\"}}\n\n"))
			_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"latest fixture\\\"}\"}}\n\n"))
			_, _ = w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n"))
			_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":3}}\n\n"))
			_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
			return
		}
		_, _ = w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"answer [W1]\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()

	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: upstream.URL, APIKey: "anthropic-key", ProviderID: "ANTHROPIC",
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := ToolDefinition{Type: "function", Function: ToolFunctionDefinition{
		Name: "search_web", Parameters: map[string]any{"type": "object"},
	}}
	firstEvents, err := provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt: "latest fixture", UseReasoning: true,
			ModelRef: ModelRef{ProviderID: "ANTHROPIC", ModelID: "claude-sonnet"},
		},
		Tools: []ToolDefinition{tool}, ToolChoice: ProviderToolChoiceRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	var call *ProviderToolCall
	var roundState any
	var reasoning strings.Builder
	for event := range firstEvents {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		switch event.Type {
		case ProviderEventReasoningDelta:
			reasoning.WriteString(event.ReasoningDelta)
		case ProviderEventToolCallCompleted:
			copy := *event.ToolCall
			call = &copy
		case ProviderEventRoundCompleted:
			roundState = event.RoundState
		}
	}
	if call == nil || call.ID != "toolu_1" || call.Name != "search_web" ||
		call.Arguments != `{"query":"latest fixture"}` || reasoning.String() != "check sources" ||
		roundState == nil {
		t.Fatalf("call/reasoning/state = %#v / %q / %#v", call, reasoning.String(), roundState)
	}

	secondEvents, err := provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt: "latest fixture", UseReasoning: true,
			ModelRef: ModelRef{ProviderID: "ANTHROPIC", ModelID: "claude-sonnet"},
		},
		Tools: []ToolDefinition{tool}, ToolChoice: ProviderToolChoiceAuto,
		Continuation: []ProviderToolExchange{{
			Calls: []ProviderToolCall{*call},
			Results: []ProviderToolResult{{
				CallID: call.ID, Name: call.Name, Content: `{"ok":true}`,
			}},
			ProviderState: roundState,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var answer strings.Builder
	for event := range secondEvents {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			answer.WriteString(event.Delta)
		}
	}
	if answer.String() != "answer [W1]" || len(requests) != 2 {
		t.Fatalf("answer/requests = %q / %d", answer.String(), len(requests))
	}
	if requests[0].ToolChoice == nil || requests[0].ToolChoice.Type != "auto" {
		t.Fatalf("thinking tool choice = %#v", requests[0].ToolChoice)
	}
	if len(requests[1].Messages) != 3 {
		t.Fatalf("continuation messages = %#v", requests[1].Messages)
	}
	assistantBlocks, ok := requests[1].Messages[1].Content.([]any)
	if !ok || len(assistantBlocks) != 2 {
		t.Fatalf("assistant blocks = %#v", requests[1].Messages[1].Content)
	}
	thinking, _ := assistantBlocks[0].(map[string]any)
	toolUse, _ := assistantBlocks[1].(map[string]any)
	if thinking["type"] != "thinking" || thinking["thinking"] != "check sources" ||
		thinking["signature"] != "sig-123" || toolUse["type"] != "tool_use" ||
		toolUse["id"] != "toolu_1" {
		t.Fatalf("preserved blocks = %#v", assistantBlocks)
	}
	input, _ := toolUse["input"].(map[string]any)
	if input["query"] != "latest fixture" {
		t.Fatalf("tool input = %#v", input)
	}
	toolResults, ok := requests[1].Messages[2].Content.([]any)
	if !ok || len(toolResults) != 1 {
		t.Fatalf("tool results = %#v", requests[1].Messages[2].Content)
	}
	result, _ := toolResults[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "toolu_1" ||
		result["content"] != `{"ok":true}` {
		t.Fatalf("tool result = %#v", result)
	}
}

func TestAnthropicProviderForcesNamedToolWithoutThinking(t *testing.T) {
	var request anthropicMessagesRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer upstream.Close()
	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: upstream.URL, APIKey: "anthropic-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt: "search", ModelRef: ModelRef{ProviderID: "anthropic", ModelID: "claude"},
		},
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunctionDefinition{
			Name: "search_web", Parameters: map[string]any{"type": "object"},
		}}},
		ToolChoice: ProviderToolChoiceRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	if request.ToolChoice == nil || request.ToolChoice.Type != "tool" ||
		request.ToolChoice.Name != "search_web" {
		t.Fatalf("tool choice = %#v", request.ToolChoice)
	}
}

func TestAnthropicToolContinuationMarksFailureResult(t *testing.T) {
	messages, err := appendAnthropicContinuation(nil, []ProviderToolExchange{{
		Calls: []ProviderToolCall{{
			ID: "toolu_failure", Name: "search_web", Arguments: `{"query":"fixture"}`,
		}},
		Results: []ProviderToolResult{{
			CallID: "toolu_failure", Name: "search_web", Content: `{"ok":false}`, IsError: true,
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	results, ok := messages[1].Content.([]anthropicToolResultBlock)
	if !ok || len(results) != 1 || !results[0].IsError ||
		results[0].ToolUseID != "toolu_failure" {
		t.Fatalf("failure result = %#v", messages[1].Content)
	}
}

func TestAnthropicStreamCapsFragmentedToolArguments(t *testing.T) {
	ctx := context.Background()
	events := make(chan ProviderEvent, 8)
	state := newAnthropicStreamState()
	start := `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_large","name":"search_web","input":{}}}`
	if keep, done := dispatchAnthropicData(ctx, start, events, state); !keep || done {
		t.Fatalf("start keep/done = %v/%v", keep, done)
	}
	fragment, err := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": strings.Repeat("x", maxAnthropicToolArgumentsBytes+128),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if keep, done := dispatchAnthropicData(ctx, string(fragment), events, state); !keep || done {
		t.Fatalf("delta keep/done = %v/%v", keep, done)
	}
	stop := `{"type":"content_block_stop","index":0}`
	if keep, done := dispatchAnthropicData(ctx, stop, events, state); !keep || done {
		t.Fatalf("stop keep/done = %v/%v", keep, done)
	}
	close(events)
	var completed *ProviderToolCall
	for event := range events {
		if event.Type == ProviderEventToolCallCompleted {
			copy := *event.ToolCall
			completed = &copy
		}
	}
	if completed == nil || completed.FailureCategory != "arguments_too_large" ||
		len(completed.Arguments) != maxAnthropicToolArgumentsBytes {
		t.Fatalf("completed call = %#v", completed)
	}
}

func TestAnthropicProviderCancelsStreamingRequest(t *testing.T) {
	requestCancelled := make(chan struct{})
	var cancellationObserved atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		if cancellationObserved.CompareAndSwap(false, true) {
			close(requestCancelled)
		}
	}))
	defer upstream.Close()
	provider, err := NewAnthropicProvider(AnthropicProviderConfig{
		BaseURL: upstream.URL, APIKey: "anthropic-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	events, err := provider.StreamChat(ctx, ProviderRequest{
		Prompt: "wait", ModelRef: ModelRef{ProviderID: "anthropic", ModelID: "claude"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	for range events {
	}
	select {
	case <-requestCancelled:
	case <-time.After(time.Second):
		t.Fatal("Anthropic upstream request was not cancelled")
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
