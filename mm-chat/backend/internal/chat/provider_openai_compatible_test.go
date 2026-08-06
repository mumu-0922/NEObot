package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/usermemory"
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

func TestOpenAICompatibleProviderSendsDeterministicJudgeControls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider payload: %v", err)
		}
		if payload.EnableThinking == nil || *payload.EnableThinking ||
			payload.MaxTokens != 128 || payload.Temperature == nil ||
			*payload.Temperature != 0 {
			t.Fatalf("deterministic controls = %#v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{}\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL, APIKey: "fixture-judge-credential", DefaultModel: "judge-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	temperature := 0.0
	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt:          "judge",
		DisableThinking: true,
		MaxOutputTokens: 128,
		Temperature:     &temperature,
		ModelRef: ModelRef{
			ProviderID: OpenAICompatibleProviderID,
			ModelID:    "judge-model",
		},
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

func TestOpenAICompatibleProviderCompletesBufferedChatWithDeterministicControls(t *testing.T) {
	const apiKey = "example-fixture-buffered-credential"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" ||
			r.Header.Get("Authorization") != "Bearer "+apiKey ||
			r.Header.Get("Accept") != "application/json" {
			t.Fatalf("request path/auth/accept drifted")
		}
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Stream || payload.Model != "gpt-5.6-luna" ||
			payload.EnableThinking == nil || *payload.EnableThinking ||
			payload.MaxTokens != usermemory.HybridCandidateJudgeMaximumOutputTokens ||
			payload.Temperature == nil || *payload.Temperature != 0 ||
			len(payload.Messages) != 2 || payload.Messages[0].Role != "system" ||
			payload.Messages[1].Role != "user" {
			t.Fatalf("buffered payload=%#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schemaVersion\":\"neo-chat.memory-cloud-candidate-judge-output.v1\",\"selectedOrdinals\":[0]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":5,"total_tokens":14}}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL + "/v1", APIKey: apiKey, ProviderID: "SERVER_DEFAULT",
	})
	if err != nil {
		t.Fatal(err)
	}
	temperature := 0.0
	completed, err := provider.CompleteChat(context.Background(), ProviderRequest{
		Prompt: "candidate JSON", SystemPrompt: "strict judge",
		DisableThinking: true,
		MaxOutputTokens: usermemory.HybridCandidateJudgeMaximumOutputTokens,
		Temperature:     &temperature,
		ModelRef: ModelRef{
			ProviderID: "SERVER_DEFAULT", ModelID: "gpt-5.6-luna",
		},
	})
	if err != nil || completed.Content == "" || completed.Usage == nil ||
		completed.Usage.PromptTokens != 9 || completed.Usage.CompletionTokens != 5 ||
		completed.Usage.TotalTokens != 14 {
		t.Fatalf("completion=%#v err=%v", completed, err)
	}
}

func TestOpenAICompatibleBufferedAndStreamingJudgeRequestsDifferOnlyByTransport(t *testing.T) {
	payloads := make(chan map[string]any, 2)
	accepts := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		payloads <- payload
		accepts <- r.Header.Get("Accept")
		if stream, _ := payload["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{}\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL + "/v1", APIKey: "fixture-buffered-wire-key",
		ProviderID: "SERVER_DEFAULT",
	})
	if err != nil {
		t.Fatal(err)
	}
	temperature := 0.0
	request := ProviderRequest{
		Prompt: "candidate JSON", SystemPrompt: "strict judge",
		DisableThinking: true,
		MaxOutputTokens: usermemory.HybridCandidateJudgeMaximumOutputTokens,
		Temperature:     &temperature,
		ModelRef: ModelRef{
			ProviderID: "SERVER_DEFAULT", ModelID: "gpt-5.6-luna",
		},
	}
	events, err := provider.StreamChat(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}
	if _, err := provider.CompleteChat(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	streaming := <-payloads
	buffered := <-payloads
	if streaming["stream"] != true || buffered["stream"] != false {
		t.Fatalf("streaming=%#v buffered=%#v", streaming, buffered)
	}
	delete(streaming, "stream")
	delete(buffered, "stream")
	if !reflect.DeepEqual(streaming, buffered) {
		t.Fatalf("streaming=%#v buffered=%#v", streaming, buffered)
	}
	if streamingAccept, bufferedAccept := <-accepts, <-accepts; streamingAccept != "text/event-stream" || bufferedAccept != "application/json" {
		t.Fatalf("accepts=%q/%q", streamingAccept, bufferedAccept)
	}
}

func TestOpenAICompatibleProviderBufferedChatClassifiesBoundedFailures(t *testing.T) {
	tests := []struct {
		name string
		body string
		want ProviderFailureCategory
	}{
		{name: "malformed", body: `{`, want: ProviderFailureResponseInvalid},
		{name: "multiple choices", body: `{"choices":[{"message":{"content":"{}"},"finish_reason":"stop"},{"message":{"content":"{}"},"finish_reason":"stop"}]}`, want: ProviderFailureResponseInvalid},
		{name: "missing content", body: `{"choices":[{"message":{},"finish_reason":"stop"}]}`, want: ProviderFailureResponseInvalid},
		{name: "missing finish", body: `{"choices":[{"message":{"content":"{}"}}]}`, want: ProviderFailureResponseInvalid},
		{name: "nonexact finish", body: `{"choices":[{"message":{"content":"{}"},"finish_reason":" stop "}]}`, want: ProviderFailureResponseInvalid},
		{name: "incomplete length finish", body: `{"choices":[{"message":{"content":"{}"},"finish_reason":"length"}]}`, want: ProviderFailureResponseInvalid},
		{name: "oversize", body: strings.Repeat("x", maxOpenAICompatibleBufferedChatBytes+1), want: ProviderFailureResponseInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
				BaseURL: server.URL, APIKey: "example-fixture-buffered-credential",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.CompleteChat(context.Background(), ProviderRequest{
				Prompt: "fixture", ModelRef: ModelRef{
					ProviderID: OpenAICompatibleProviderID, ModelID: "fixture-model",
				},
			})
			category, ok := ProviderFailureCategoryOf(err)
			if !ok || category != test.want {
				t.Fatalf("category=%q/%t want=%q err=%v", category, ok, test.want, err)
			}
		})
	}

	t.Run("read interruption", func(t *testing.T) {
		client := &http.Client{Transport: providerRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(failingSSEReader{}),
			}, nil
		})}
		provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
			BaseURL: "https://provider.example/v1",
			APIKey:  "example-fixture-buffered-credential", HTTPClient: client,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = provider.CompleteChat(context.Background(), ProviderRequest{
			Prompt: "fixture", ModelRef: ModelRef{
				ProviderID: OpenAICompatibleProviderID, ModelID: "fixture-model",
			},
		})
		category, ok := ProviderFailureCategoryOf(err)
		if !ok || category != ProviderFailureTransportFailed ||
			strings.Contains(err.Error(), "private stream read failure") {
			t.Fatalf("category=%q/%t err=%v", category, ok, err)
		}
	})

	t.Run("typed status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"private rate detail"}}`))
		}))
		defer server.Close()
		provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
			BaseURL: server.URL, APIKey: "example-fixture-buffered-credential",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = provider.CompleteChat(context.Background(), ProviderRequest{
			Prompt: "fixture", ModelRef: ModelRef{
				ProviderID: OpenAICompatibleProviderID, ModelID: "fixture-model",
			},
		})
		category, ok := ProviderFailureCategoryOf(err)
		delay, retryable := ProviderRetryDelay(err)
		if !ok || category != ProviderFailureRateLimited || !retryable ||
			delay != 7*time.Second || strings.Contains(err.Error(), "private rate detail") {
			t.Fatalf("category=%q/%t delay=%s/%t err=%v", category, ok, delay, retryable, err)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
			BaseURL: "https://provider.example/v1",
			APIKey:  "example-fixture-buffered-credential",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = provider.CompleteChat(ctx, ProviderRequest{
			Prompt: "fixture", ModelRef: ModelRef{
				ProviderID: OpenAICompatibleProviderID, ModelID: "fixture-model",
			},
		})
		category, ok := ProviderFailureCategoryOf(err)
		if !ok || category != ProviderFailureContextCanceled {
			t.Fatalf("category=%q/%t err=%v", category, ok, err)
		}
	})
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
			_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"web\",\"arguments\":\"fixture\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n"))
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
	completedCalls := 0
	for event := range first {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.ToolCallDelta != nil {
			fragments = append(fragments, *event.ToolCallDelta)
		}
		if event.ToolCall != nil {
			completedCalls++
			call = *event.ToolCall
		}
	}
	if len(fragments) != 2 || completedCalls != 1 || call.ID != "call-1" || call.Name != searchWebToolName ||
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

func TestDeepSeekCompatibleProviderCanonicalizesOnlyDeclaredZeroArgumentToolCalls(t *testing.T) {
	const returnedArguments = `{"query":"forbidden"}`
	tests := []struct {
		name         string
		deepSeek     bool
		tool         ToolDefinition
		returnedArgs string
		wantArgs     string
		wantMemoryOK bool
	}{
		{
			name:     "official DeepSeek zero-argument Tool",
			deepSeek: true, tool: SearchMemoryToolDefinition(),
			returnedArgs: returnedArguments, wantArgs: `{}`, wantMemoryOK: true,
		},
		{
			name:     "generic compatible zero-argument Tool",
			deepSeek: false, tool: SearchMemoryToolDefinition(),
			returnedArgs: returnedArguments, wantArgs: returnedArguments,
		},
		{
			name:     "official DeepSeek argument-bearing Tool",
			deepSeek: true, tool: searchWebToolDefinition(),
			returnedArgs: returnedArguments, wantArgs: returnedArguments,
		},
		{
			name:     "official DeepSeek malformed zero-argument Tool",
			deepSeek: true, tool: SearchMemoryToolDefinition(),
			returnedArgs: `not-json`, wantArgs: `not-json`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(
					w,
					"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":%q,\"arguments\":%q}}]},\"finish_reason\":\"tool_calls\"}]}\n\n",
					test.tool.Function.Name,
					test.returnedArgs,
				)
				_, _ = w.Write([]byte("data: [DONE]\n\n"))
			}))
			defer server.Close()

			provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
				BaseURL: server.URL, APIKey: "fixture-token", DefaultModel: "fixture-model",
			})
			if err != nil {
				t.Fatal(err)
			}
			provider.deepSeek = test.deepSeek
			events, err := provider.StreamToolRound(context.Background(), ProviderRoundRequest{
				ProviderRequest: ProviderRequest{
					Prompt:   "call the Tool",
					ModelRef: ModelRef{ProviderID: OpenAICompatibleProviderID, ModelID: "fixture-model"},
				},
				Tools: []ToolDefinition{test.tool}, ToolChoice: ProviderToolChoiceRequired,
			})
			if err != nil {
				t.Fatal(err)
			}
			var completed *ProviderToolCall
			for event := range events {
				if event.Error != nil {
					t.Fatal(event.Error)
				}
				if event.Type == ProviderEventToolCallCompleted {
					completed = event.ToolCall
				}
			}
			if completed == nil || completed.Name != test.tool.Function.Name ||
				completed.Arguments != test.wantArgs {
				t.Fatalf("completed Tool Call = %#v, want arguments %q", completed, test.wantArgs)
			}
			if completed.Name == usermemory.HybridMemoryToolName {
				_, failure := validateSearchMemoryToolCall(*completed, 1, true)
				if got := failure == ""; got != test.wantMemoryOK {
					t.Fatalf("Memory validation success = %t, want %t (%q)", got, test.wantMemoryOK, failure)
				}
			}
		})
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
	streamOpenAICompatibleEvents(context.Background(), reader, events, nil)
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

func TestOpenAICompatibleStreamMarksSyntheticToolCallID(t *testing.T) {
	reader := strings.NewReader("data: " + mustJSON(t, map[string]any{
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0,
				"type":  "function",
				"function": map[string]any{
					"name":      searchWebToolName,
					"arguments": `{}`,
				},
			}}},
			"finish_reason": "tool_calls",
		}},
	}) + "\n\ndata: [DONE]\n\n")
	events := make(chan ProviderEvent, 8)
	streamOpenAICompatibleEvents(context.Background(), reader, events, nil)
	close(events)
	var completed *ProviderToolCall
	for event := range events {
		if event.ToolCall != nil {
			completed = event.ToolCall
		}
	}
	if completed == nil || completed.ID != "call_0_0" ||
		!completed.SyntheticID || completed.FailureCategory != "" {
		t.Fatalf("completed tool call = %#v", completed)
	}
}

func TestOpenAICompatibleStreamClassifiesParseRemoteAndIncompleteFailures(t *testing.T) {
	tests := []struct {
		name string
		body string
		want ProviderFailureCategory
	}{
		{name: "parse", body: "data: {invalid}\n\n", want: ProviderFailureStreamParseFailed},
		{name: "remote", body: "data: {\"error\":{\"message\":\"private\"}}\n\n", want: ProviderFailureStreamRemoteError},
		{name: "incomplete", body: "data: {\"choices\":[]}\n\n", want: ProviderFailureStreamIncomplete},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := make(chan ProviderEvent, 4)
			streamOpenAICompatibleEvents(
				context.Background(), strings.NewReader(test.body), events, nil,
			)
			close(events)
			var failure error
			for event := range events {
				if event.Error != nil {
					failure = event.Error
				}
			}
			category, ok := ProviderFailureCategoryOf(failure)
			if !ok || category != test.want {
				t.Fatalf("failure category = %q/%t (%v)", category, ok, failure)
			}
		})
	}
}

func TestOpenAICompatibleStreamClassifiesReadFailure(t *testing.T) {
	events := make(chan ProviderEvent, 2)
	streamOpenAICompatibleEvents(context.Background(), failingSSEReader{}, events, nil)
	close(events)
	var failure error
	for event := range events {
		failure = event.Error
	}
	category, ok := ProviderFailureCategoryOf(failure)
	if !ok || category != ProviderFailureStreamReadFailed {
		t.Fatalf("stream read category = %q/%t (%v)", category, ok, failure)
	}
}

type failingSSEReader struct{}

func (failingSSEReader) Read([]byte) (int, error) {
	return 0, errors.New("private stream read failure")
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
		if payload.Stream || payload.ToolChoice != "auto" || len(payload.Tools) != 1 ||
			payload.EnableThinking == nil || *payload.EnableThinking ||
			payload.MaxTokens != 128 || payload.Temperature == nil ||
			*payload.Temperature != 0 {
			t.Fatalf("tool plan payload = %#v", payload)
		}
		if payload.Tools[0].Function.Name != "lookup_weather" ||
			!payload.Tools[0].Function.Strict {
			t.Fatalf("tool definition = %#v", payload.Tools[0].Function)
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
	temperature := 0.0
	calls, err := provider.PlanTools(context.Background(), ToolPlanRequest{
		Prompt:          "weather in Shanghai",
		ModelRef:        ModelRef{ProviderID: "openai_compatible"},
		DisableThinking: true,
		MaxOutputTokens: 128,
		Temperature:     &temperature,
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name:       "lookup_weather",
				Parameters: map[string]any{"type": "object"},
				Strict:     true,
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

func TestDeepSeekCompatibleProviderDisablesThinkingForToolProtocolOnly(t *testing.T) {
	requestCount := 0
	client := &http.Client{Transport: providerRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestCount++
		if r.URL.String() != "https://api.deepseek.com/v1/chat/completions" {
			t.Fatalf("request URL = %q", r.URL.String())
		}
		var payload openAICompatibleChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if requestCount <= 3 {
			if payload.EnableThinking != nil || payload.Thinking == nil ||
				payload.Thinking.Type != "disabled" || payload.ReasoningEffort != "" {
				t.Fatalf(
					"DeepSeek Tool thinking controls = %#v/%#v/%q",
					payload.EnableThinking,
					payload.Thinking,
					payload.ReasoningEffort,
				)
			}
		} else if payload.EnableThinking != nil || payload.Thinking != nil ||
			payload.ReasoningEffort != string(ReasoningEffortHigh) {
			t.Fatalf(
				"DeepSeek plain-chat thinking controls = %#v/%#v/%q",
				payload.EnableThinking,
				payload.Thinking,
				payload.ReasoningEffort,
			)
		}
		switch requestCount {
		case 2:
			choice, ok := payload.ToolChoice.(map[string]any)
			function, _ := choice["function"].(map[string]any)
			if !ok || choice["type"] != "function" ||
				function["name"] != "search_memory" || len(payload.Tools) != 1 {
				t.Fatalf("DeepSeek forced Tool payload = %#v", payload)
			}
		case 3, 4:
			if payload.ToolChoice != nil || len(payload.Tools) != 0 {
				t.Fatalf("DeepSeek continuation/plain Tools = %#v", payload)
			}
		}
		if payload.Stream {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
				},
				Body: io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"choices":[{"message":{"content":"","tool_calls":[]}}]}`,
			)),
		}, nil
	})}
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: "https://api.deepseek.com/v1", APIKey: "fixture-deepseek-key",
		DefaultModel: "deepseek-v4-flash", HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	temperature := 0.0
	calls, err := provider.PlanTools(context.Background(), ToolPlanRequest{
		Prompt: "memory?", DisableThinking: true, MaxOutputTokens: 128,
		Temperature: &temperature,
		ModelRef:    ModelRef{ProviderID: "openai_compatible", ModelID: "deepseek-v4-flash"},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name: "search_memory", Parameters: map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil || len(calls) != 0 {
		t.Fatalf("PlanTools() = %#v/%v", calls, err)
	}
	tool := ToolDefinition{
		Type: "function",
		Function: ToolFunctionDefinition{
			Name: "search_memory", Parameters: map[string]any{"type": "object"},
		},
	}
	events, err := provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt: "memory?", UseReasoning: true, ReasoningEffort: ReasoningEffortMax,
			ModelRef: ModelRef{
				ProviderID: "openai_compatible", ModelID: "deepseek-v4-flash",
			},
		},
		Tools: []ToolDefinition{tool}, ToolChoice: ProviderToolChoiceRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	events, err = provider.StreamToolRound(context.Background(), ProviderRoundRequest{
		ProviderRequest: ProviderRequest{
			Prompt: "memory?", UseReasoning: true, ReasoningEffort: ReasoningEffortMax,
			ModelRef: ModelRef{
				ProviderID: "openai_compatible", ModelID: "deepseek-v4-flash",
			},
		},
		Continuation: []ProviderToolExchange{{
			Calls: []ProviderToolCall{{ID: "call-1", Name: "search_memory", Arguments: `{}`}},
			Results: []ProviderToolResult{{
				CallID: "call-1", Name: "search_memory", Content: `{"ok":true}`,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	events, err = provider.StreamChat(context.Background(), ProviderRequest{
		Prompt: "plain reasoning", UseReasoning: true, ReasoningEffort: ReasoningEffortHigh,
		ModelRef: ModelRef{
			ProviderID: "openai_compatible", ModelID: "deepseek-v4-flash",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	if requestCount != 4 {
		t.Fatalf("request count = %d, want 4", requestCount)
	}
}

func TestOfficialDeepSeekBaseURLDetection(t *testing.T) {
	tests := []struct {
		baseURL string
		want    bool
	}{
		{baseURL: "https://api.deepseek.com/v1", want: true},
		{baseURL: "https://API.DEEPSEEK.COM/v1", want: true},
		{baseURL: "https://api.deepseek.com.evil.example/v1", want: false},
		{baseURL: "https://deepseek.example/v1", want: false},
		{baseURL: "://invalid", want: false},
	}
	for _, test := range tests {
		if got := isOfficialDeepSeekBaseURL(test.baseURL); got != test.want {
			t.Errorf("isOfficialDeepSeekBaseURL(%q) = %t, want %t", test.baseURL, got, test.want)
		}
	}
}

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn providerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
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

func TestOpenAICompatibleProviderRejectsAmbiguousToolPlanResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "zero choices",
			body: `{"choices":[]}`,
			want: "choice count",
		},
		{
			name: "multiple choices",
			body: `{"choices":[{"message":{}},{"message":{}}]}`,
			want: "choice count",
		},
		{
			name: "missing arguments",
			body: `{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup_weather"}}]}}]}`,
			want: "missing arguments",
		},
		{
			name: "null arguments",
			body: `{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup_weather","arguments":"null"}}]}}]}`,
			want: "invalid arguments",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
				BaseURL: server.URL, APIKey: "test-secret-token",
				DefaultModel: "gpt-default",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.PlanTools(context.Background(), ToolPlanRequest{
				Prompt: "weather", ModelRef: ModelRef{ProviderID: "openai_compatible"},
				Tools: []ToolDefinition{{
					Type: "function",
					Function: ToolFunctionDefinition{
						Name: "lookup_weather", Parameters: map[string]any{"type": "object"},
					},
				}},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("PlanTools() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestOpenAICompatibleProviderAcceptsOneNoCallToolPlan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{}}]}`))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL, APIKey: "test-secret-token", DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatal(err)
	}
	calls, err := provider.PlanTools(context.Background(), ToolPlanRequest{
		Prompt: "hello", ModelRef: ModelRef{ProviderID: "openai_compatible"},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionDefinition{
				Name: "lookup_weather", Parameters: map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil || len(calls) != 0 {
		t.Fatalf("PlanTools() = %#v/%v", calls, err)
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

func TestOpenAICompatibleProviderFinishReasonAllowsCleanEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"complete\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
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
	for event := range events {
		if event.Error != nil {
			t.Fatalf("provider event error = %v", event.Error)
		}
		if event.Type == ProviderEventDelta {
			deltas = append(deltas, event.Delta)
		}
	}
	if strings.Join(deltas, "") != "complete" {
		t.Fatalf("deltas = %q, want complete", strings.Join(deltas, ""))
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

func TestOpenAICompatibleProviderResolvesCanonicalAliasToConfiguredProviderID(t *testing.T) {
	provider, err := NewOpenAICompatibleProvider(OpenAICompatibleProviderConfig{
		BaseURL:      "https://example.test/v1",
		APIKey:       "test-secret-token",
		DefaultModel: "gpt-default",
		ProviderID:   "SERVER_DEFAULT",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	resolved, err := provider.ResolveModelRef(ModelRef{
		ProviderID: OpenAICompatibleProviderID,
		ModelID:    "gpt-search",
	})
	if err != nil {
		t.Fatalf("ResolveModelRef() error = %v", err)
	}
	if resolved.ProviderID != "SERVER_DEFAULT" {
		t.Fatalf("ProviderID = %q, want SERVER_DEFAULT", resolved.ProviderID)
	}
	if resolved.ModelID != "gpt-search" {
		t.Fatalf("ModelID = %q, want gpt-search", resolved.ModelID)
	}
}
