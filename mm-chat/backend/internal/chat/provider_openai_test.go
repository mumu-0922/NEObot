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

	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestOpenAIProviderStreamsResponsesWebSearch(t *testing.T) {
	const apiKey = "fixture-openai-responses-key"
	const imageBytes = "\x89PNG\r\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want /v1/responses", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Fatal("Authorization header mismatch")
		}
		var request openAIResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "gpt-search" || !request.Stream || request.Instructions != "cite sources" {
			t.Fatalf("request = %#v", request)
		}
		if len(request.Tools) != 1 || request.Tools[0].Type != "web_search_preview" {
			t.Fatalf("tools = %#v", request.Tools)
		}
		if len(request.Include) != 2 || request.Reasoning == nil || request.Reasoning.Effort != "high" {
			t.Fatalf("include/reasoning = %#v/%#v", request.Include, request.Reasoning)
		}
		if len(request.Input) != 1 || len(request.Input[0].Content) != 2 {
			t.Fatalf("input = %#v", request.Input)
		}
		if request.Input[0].Content[0].Type != "input_text" || request.Input[0].Content[0].Text != "latest fixture" {
			t.Fatalf("text input = %#v", request.Input[0].Content[0])
		}
		wantImage := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(imageBytes))
		if request.Input[0].Content[1].Type != "input_image" || request.Input[0].Content[1].ImageURL != wantImage {
			t.Fatalf("image input = %#v", request.Input[0].Content[1])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer \"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.annotation.added\",\"annotation\":{\"type\":\"url_citation\",\"title\":\"Alpha\",\"url\":\"https://alpha.example/result#fragment\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"web_search_call\",\"action\":{\"sources\":[{\"title\":\"Alpha duplicate\",\"url\":\"https://alpha.example/result\",\"snippet\":\"duplicate\"},{\"title\":\"Beta\",\"url\":\"https://beta.example/result\",\"snippet\":\"beta source\"}]}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"complete\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":11,\"output_tokens\":7,\"total_tokens\":18}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL + "/v1", APIKey: apiKey, DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}
	if provider.ModelBuiltInSearchID() != websearch.ModelBuiltInOpenAI {
		t.Fatalf("built-in id = %q", provider.ModelBuiltInSearchID())
	}

	events, err := provider.StreamChatWithModelBuiltInSearch(context.Background(), ProviderRequest{
		Prompt: " latest fixture ", SystemPrompt: "cite sources", UseReasoning: true,
		Attachments: []ProviderAttachment{{MimeType: "image/png", Data: []byte(imageBytes)}},
		ModelRef:    ModelRef{ProviderID: "openai", ModelID: "gpt-search"},
	})
	if err != nil {
		t.Fatalf("StreamChatWithModelBuiltInSearch() error = %v", err)
	}

	var content strings.Builder
	var sources []websearch.Source
	var usage *TokenUsage
	for event := range events {
		if event.Error != nil {
			t.Fatalf("event error = %v", event.Error)
		}
		switch event.Type {
		case ProviderEventDelta:
			content.WriteString(event.Delta)
		case ProviderEventSearch:
			if event.Search == nil {
				t.Fatal("search event result is nil")
			}
			sources = append(sources, event.Search.Sources...)
		case ProviderEventUsage:
			usage = event.Usage
		}
	}
	if content.String() != "answer complete" {
		t.Fatalf("content = %q", content.String())
	}
	if len(sources) != 2 || sources[0].URL != "https://alpha.example/result" || sources[1].Title != "Beta" {
		t.Fatalf("sources = %#v", sources)
	}
	if usage == nil || usage.PromptTokens != 11 || usage.CompletionTokens != 7 || usage.TotalTokens != 18 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestOpenAIProviderOrdinaryStreamStillUsesChatCompletions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ordinary\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL + "/v1", APIKey: "fixture-key", DefaultModel: "gpt-default",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		Prompt: "ordinary", ModelRef: ModelRef{ProviderID: "openai"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for event := range events {
		if event.Error != nil {
			t.Fatalf("event error = %v", event.Error)
		}
	}
}

func TestOpenAIResponsesWebSearchSendsConversationHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openAIResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Input) != 3 {
			t.Fatalf("input = %#v, want three history items", request.Input)
		}
		wantRoles := []string{"user", "assistant", "user"}
		wantText := []string{"remember amber", "noted", "what color?"}
		for index := range wantRoles {
			if request.Input[index].Role != wantRoles[index] ||
				len(request.Input[index].Content) != 1 ||
				request.Input[index].Content[0].Text != wantText[index] {
				t.Fatalf("input[%d] = %#v", index, request.Input[index])
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{}}\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(OpenAICompatibleProviderConfig{
		BaseURL: server.URL, APIKey: strings.Repeat("x", 32), DefaultModel: "gpt-search",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.StreamChatWithModelBuiltInSearch(context.Background(), ProviderRequest{
		Messages: []ProviderMessage{
			{Role: "user", Content: "remember amber"},
			{Role: "assistant", Content: "noted"},
			{Role: "user", Content: "what color?"},
		},
		ModelRef: ModelRef{ProviderID: "openai", ModelID: "gpt-search"},
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

func TestOpenAIResponsesFailuresRemainRedacted(t *testing.T) {
	const apiKey = "responses-key-must-not-leak"
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "upstream status", status: http.StatusUnauthorized, body: "bad key " + apiKey},
		{name: "malformed frame", status: http.StatusOK, body: "data: {not-json-" + apiKey + "}\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			provider, err := NewOpenAIProvider(OpenAICompatibleProviderConfig{
				BaseURL: server.URL, APIKey: apiKey, DefaultModel: "gpt-search",
			})
			if err != nil {
				t.Fatal(err)
			}
			events, err := provider.StreamChatWithModelBuiltInSearch(context.Background(), ProviderRequest{
				Prompt: "fixture", ModelRef: ModelRef{ProviderID: "openai"},
			})
			if tt.status != http.StatusOK {
				if err == nil || strings.Contains(err.Error(), apiKey) {
					t.Fatalf("startup error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var streamErr error
			for event := range events {
				if event.Error != nil {
					streamErr = event.Error
				}
			}
			if !errors.Is(streamErr, errOpenAIResponsesFrame) || strings.Contains(streamErr.Error(), apiKey) {
				t.Fatalf("stream error = %v", streamErr)
			}
		})
	}
}
