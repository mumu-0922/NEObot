package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiProviderStreamsThroughGoogleOpenAIEndpoint(t *testing.T) {
	var requestPath string
	var authorization string
	var payload openAICompatibleChatCompletionRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Gem\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ini\"}}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	provider, err := NewGeminiProvider(OpenAICompatibleProviderConfig{
		BaseURL: upstream.URL, APIKey: "gemini-key", ProviderID: "GEMINI-STORED",
	})
	if err != nil {
		t.Fatalf("NewGeminiProvider() error = %v", err)
	}
	events, err := provider.StreamChat(context.Background(), ProviderRequest{
		ModelRef:     ModelRef{ProviderID: "GEMINI-STORED", ModelID: "gemini-2.5-flash"},
		SystemPrompt: "answer briefly",
		Messages: []ProviderMessage{{
			Role: "user", Content: "inspect",
			Attachments: []ProviderAttachment{{
				MimeType: "image/png", Data: []byte("fixture-image"),
			}},
		}},
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
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.Type == ProviderEventUsage {
			usage = event.Usage
		}
	}

	if requestPath != "/v1beta/openai/chat/completions" {
		t.Fatalf("request path = %q", requestPath)
	}
	if authorization != "Bearer gemini-key" {
		t.Fatalf("Authorization = %q", authorization)
	}
	if payload.Model != "gemini-2.5-flash" || len(payload.Messages) != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	parts, ok := payload.Messages[1].Content.([]any)
	imagePart, imagePartOK := mapValue(parts, 1)
	imageURL, imageURLOK := imagePart["image_url"].(map[string]any)
	encodedImage, encodedImageOK := imageURL["url"].(string)
	if !ok || len(parts) != 2 || !imagePartOK || !imageURLOK || !encodedImageOK ||
		!strings.HasPrefix(encodedImage, "data:image/png;base64,") {
		t.Fatalf("Gemini image content = %#v", payload.Messages[1].Content)
	}
	if content.String() != "Gemini" || usage == nil || usage.TotalTokens != 6 {
		t.Fatalf("content/usage = %q/%#v", content.String(), usage)
	}
}

func mapValue(values []any, index int) (map[string]any, bool) {
	if index < 0 || index >= len(values) {
		return nil, false
	}
	value, ok := values[index].(map[string]any)
	return value, ok
}

func TestNormalizeGeminiOpenAIBaseURL(t *testing.T) {
	tests := map[string]string{
		"":        defaultGeminiServiceURL + geminiOpenAIPath,
		"default": defaultGeminiServiceURL + geminiOpenAIPath,
		"https://generativelanguage.googleapis.com": defaultGeminiServiceURL + geminiOpenAIPath,
		"https://gemini.example/v1beta":             "https://gemini.example/v1beta/openai",
		"https://gemini.example/v1beta/openai/":     "https://gemini.example/v1beta/openai",
		"https://proxy.example/google":              "https://proxy.example/google/v1beta/openai",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := normalizeGeminiOpenAIBaseURL(input)
			if err != nil {
				t.Fatalf("normalizeGeminiOpenAIBaseURL() error = %v", err)
			}
			if got != want {
				t.Fatalf("normalizeGeminiOpenAIBaseURL(%q) = %q, want %q", input, got, want)
			}
		})
	}

	for _, input := range []string{
		"ftp://gemini.example", "https://user@gemini.example", "https://gemini.example?key=x",
	} {
		if _, err := normalizeGeminiOpenAIBaseURL(input); err == nil {
			t.Fatalf("normalizeGeminiOpenAIBaseURL(%q) error = nil", input)
		}
	}
}

func TestGeminiProviderRetainsToolPlanner(t *testing.T) {
	var _ Provider = (*GeminiProvider)(nil)
	var _ ToolPlanner = (*GeminiProvider)(nil)
	var _ ModelRefResolver = (*GeminiProvider)(nil)
}
