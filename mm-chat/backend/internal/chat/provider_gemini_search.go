package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"neo-chat/mm-chat/backend/internal/websearch"
)

var errGeminiBuiltInSearchStream = errors.New("gemini built-in search stream failed")

func (p *GeminiProvider) ModelBuiltInSearchID() websearch.ModelBuiltInProviderID {
	return websearch.ModelBuiltInGemini
}

func (p *GeminiProvider) StreamChatWithModelBuiltInSearch(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelRef.ModelID) == "" {
		return nil, errors.New("gemini provider model is required")
	}
	contents := geminiBuiltInSearchContents(providerMessagesOrPrompt(input))
	payload := map[string]any{
		"contents": contents,
		"tools":    []map[string]any{{"google_search": map[string]any{}}},
	}
	if system := strings.TrimSpace(input.SystemPrompt); system != "" {
		payload["system_instruction"] = map[string]any{
			"parts": []map[string]any{{"text": system}},
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("gemini built-in search request encode failed")
	}
	endpoint := p.nativeBaseURL + "/v1beta/models/" +
		url.PathEscape(modelRef.ModelID) + ":streamGenerateContent?alt=sse"
	requestCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
	}
	req, err := http.NewRequestWithContext(
		requestCtx, http.MethodPost, endpoint, bytes.NewReader(encoded),
	)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, errors.New("gemini built-in search request build failed")
	}
	req.Header.Set("x-goog-api-key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	resp, err := p.nativeClient.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, errors.New("gemini built-in search request failed")
	}
	if resp == nil || resp.Body == nil || resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
		}
		if cancel != nil {
			cancel()
		}
		return nil, errors.New("gemini built-in search provider returned a non-success status")
	}
	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		if cancel != nil {
			defer cancel()
		}
		streamGeminiBuiltInSearchEvents(ctx, resp.Body, events)
	}()
	return events, nil
}

func geminiBuiltInSearchContents(messages []ProviderMessage) []map[string]any {
	contents := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		role := "user"
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			role = "model"
		}
		parts := make([]map[string]any, 0, len(message.Attachments)+1)
		if text := strings.TrimSpace(message.Content); text != "" {
			parts = append(parts, map[string]any{"text": text})
		}
		for _, attachment := range message.Attachments {
			if len(attachment.Data) == 0 {
				continue
			}
			parts = append(parts, map[string]any{
				"inline_data": map[string]any{
					"mime_type": attachment.MimeType,
					"data":      base64.StdEncoding.EncodeToString(attachment.Data),
				},
			})
		}
		if len(parts) > 0 {
			contents = append(contents, map[string]any{"role": role, "parts": parts})
		}
	}
	return contents
}

type geminiBuiltInSearchEnvelope struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
		GroundingMetadata struct {
			GroundingChunks []struct {
				Web struct {
					URI   string `json:"uri"`
					Title string `json:"title"`
				} `json:"web"`
			} `json:"groundingChunks"`
		} `json:"groundingMetadata"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func streamGeminiBuiltInSearchEvents(
	ctx context.Context,
	reader io.Reader,
	events chan<- ProviderEvent,
) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	dataLines := make([]string, 0, 1)
	seenSources := map[string]struct{}{}
	sawPayload := false
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if !dispatchGeminiBuiltInSearchData(
				ctx, strings.Join(dataLines, "\n"), events, seenSources, &sawPayload,
			) {
				return
			}
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) > 0 && !dispatchGeminiBuiltInSearchData(
		ctx, strings.Join(dataLines, "\n"), events, seenSources, &sawPayload,
	) {
		return
	}
	if ctx.Err() == nil && (scanner.Err() != nil || !sawPayload) {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errGeminiBuiltInSearchStream})
	}
}

func dispatchGeminiBuiltInSearchData(
	ctx context.Context,
	data string,
	events chan<- ProviderEvent,
	seenSources map[string]struct{},
	sawPayload *bool,
) bool {
	data = strings.TrimSpace(data)
	if data == "" || data == "[DONE]" {
		return true
	}
	var envelope geminiBuiltInSearchEnvelope
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errGeminiBuiltInSearchStream})
		return false
	}
	*sawPayload = true
	newSources := make([]websearch.Source, 0)
	for _, candidate := range envelope.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text == "" {
				continue
			}
			event := ProviderEvent{Type: ProviderEventDelta, Delta: part.Text}
			if part.Thought {
				event = ProviderEvent{Type: ProviderEventReasoningDelta, ReasoningDelta: part.Text}
			}
			if !sendProviderEvent(ctx, events, event) {
				return false
			}
		}
		for _, chunk := range candidate.GroundingMetadata.GroundingChunks {
			uri := strings.TrimSpace(chunk.Web.URI)
			if uri == "" {
				continue
			}
			if _, exists := seenSources[uri]; exists {
				continue
			}
			seenSources[uri] = struct{}{}
			title := strings.TrimSpace(chunk.Web.Title)
			if title == "" {
				title = "Google Search source"
			}
			newSources = append(newSources, websearch.Source{
				Title: title, URL: uri, Content: title,
			})
		}
	}
	if len(newSources) > 0 && !sendProviderEvent(ctx, events, ProviderEvent{
		Type:   ProviderEventSearch,
		Search: &websearch.Result{Sources: newSources, Images: []websearch.Image{}},
	}) {
		return false
	}
	if envelope.UsageMetadata.TotalTokenCount > 0 {
		return sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventUsage,
			Usage: &TokenUsage{
				PromptTokens:     envelope.UsageMetadata.PromptTokenCount,
				CompletionTokens: envelope.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      envelope.UsageMetadata.TotalTokenCount,
			},
		})
	}
	return true
}
