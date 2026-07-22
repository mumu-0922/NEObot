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
	"strings"

	"neo-chat/mm-chat/backend/internal/websearch"
)

const openAIResponsesPath = "/responses"

var (
	errOpenAIResponsesFrame  = errors.New("openai responses stream parse failed")
	errOpenAIResponsesStream = errors.New("openai responses stream failed")
)

// OpenAIProvider keeps ordinary chat on the existing Chat Completions path and
// adds the official Responses Web Search capability only for providers
// explicitly configured as OpenAI.
type OpenAIProvider struct {
	*OpenAICompatibleProvider
	responsesEndpoint string
}

func NewOpenAIProvider(cfg OpenAICompatibleProviderConfig) (*OpenAIProvider, error) {
	baseURL, err := normalizeOpenAICompatibleBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	compatible, err := NewOpenAICompatibleProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &OpenAIProvider{
		OpenAICompatibleProvider: compatible,
		responsesEndpoint:        baseURL + openAIResponsesPath,
	}, nil
}

func (p *OpenAIProvider) ModelBuiltInSearchID() websearch.ModelBuiltInProviderID {
	return websearch.ModelBuiltInOpenAI
}

func (p *OpenAIProvider) StreamChatWithModelBuiltInSearch(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelRef.ModelID) == "" {
		return nil, errors.New("openai provider model is required")
	}

	payload, err := json.Marshal(openAIResponsesRequest{
		Model:        modelRef.ModelID,
		Stream:       true,
		Input:        openAIResponsesInput(providerMessagesOrPrompt(input)),
		Instructions: strings.TrimSpace(input.SystemPrompt),
		Tools:        []openAIResponsesTool{{Type: "web_search_preview"}},
		Include: []string{
			"web_search_call.results",
			"web_search_call.action.sources",
		},
		Reasoning: openAIResponsesReasoning(
			modelRef.ModelID,
			input.UseReasoning,
			effectiveReasoningEffort(input),
		),
	})
	if err != nil {
		return nil, errors.New("openai responses request encode failed")
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
	}
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		p.responsesEndpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, errors.New("openai responses request build failed")
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, errors.New("openai responses request failed")
	}
	if resp == nil || resp.Body == nil {
		if cancel != nil {
			cancel()
		}
		return nil, errors.New("openai responses response is invalid")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if cancel != nil {
			cancel()
		}
		return nil, errors.New("openai responses provider returned a non-success status")
	}

	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		if cancel != nil {
			defer cancel()
		}
		streamOpenAIResponsesEvents(ctx, resp.Body, events)
	}()
	return events, nil
}

type openAIResponsesRequest struct {
	Model        string                          `json:"model"`
	Stream       bool                            `json:"stream"`
	Input        []openAIResponsesInputItem      `json:"input"`
	Instructions string                          `json:"instructions,omitempty"`
	Tools        []openAIResponsesTool           `json:"tools"`
	Include      []string                        `json:"include"`
	Reasoning    *openAIResponsesReasoningConfig `json:"reasoning,omitempty"`
}

type openAIResponsesInputItem struct {
	Role    string                     `json:"role"`
	Content []openAIResponsesInputPart `json:"content"`
}

type openAIResponsesInputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type openAIResponsesTool struct {
	Type string `json:"type"`
}

type openAIResponsesReasoningConfig struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary"`
}

func openAIResponsesInput(messages []ProviderMessage) []openAIResponsesInputItem {
	input := make([]openAIResponsesInputItem, 0, len(messages))
	for _, message := range messages {
		content := make([]openAIResponsesInputPart, 0, len(message.Attachments)+1)
		if text := strings.TrimSpace(message.Content); text != "" {
			content = append(content, openAIResponsesInputPart{Type: "input_text", Text: text})
		}
		for _, attachment := range message.Attachments {
			mimeType := strings.TrimSpace(attachment.MimeType)
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			content = append(content, openAIResponsesInputPart{
				Type: "input_image",
				ImageURL: "data:" + mimeType + ";base64," +
					base64.StdEncoding.EncodeToString(attachment.Data),
			})
		}
		input = append(input, openAIResponsesInputItem{
			Role:    message.Role,
			Content: content,
		})
	}
	return input
}

func openAIResponsesReasoning(
	modelID string,
	enabled bool,
	requested ReasoningEffort,
) *openAIResponsesReasoningConfig {
	if !enabled {
		return nil
	}
	return &openAIResponsesReasoningConfig{
		Effort:  openAIReasoningEffort(modelID, enabled, requested),
		Summary: "auto",
	}
}

type openAIResponsesSourceAccumulator struct {
	seen map[string]struct{}
	used int
}

func streamOpenAIResponsesEvents(
	ctx context.Context,
	reader io.Reader,
	events chan<- ProviderEvent,
) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	dataLines := make([]string, 0, 1)
	sources := &openAIResponsesSourceAccumulator{seen: map[string]struct{}{}}
	completed := false

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			keepReading, eventCompleted := dispatchOpenAIResponsesData(
				ctx, strings.Join(dataLines, "\n"), events, sources,
			)
			completed = completed || eventCompleted
			if !keepReading {
				return
			}
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			dataLines = append(dataLines, strings.TrimPrefix(data, " "))
		}
	}
	if len(dataLines) > 0 {
		keepReading, eventCompleted := dispatchOpenAIResponsesData(
			ctx, strings.Join(dataLines, "\n"), events, sources,
		)
		completed = completed || eventCompleted
		if !keepReading {
			return
		}
	}
	if scanner.Err() != nil && ctx.Err() == nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAIResponsesStream})
		return
	}
	if ctx.Err() == nil && !completed {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAIResponsesStream})
	}
}

func dispatchOpenAIResponsesData(
	ctx context.Context,
	data string,
	events chan<- ProviderEvent,
	sources *openAIResponsesSourceAccumulator,
) (bool, bool) {
	data = strings.TrimSpace(data)
	if data == "" {
		return true, false
	}
	if data == "[DONE]" {
		return true, false
	}
	var event struct {
		Type       string         `json:"type"`
		Delta      string         `json:"delta"`
		Annotation map[string]any `json:"annotation"`
		Item       map[string]any `json:"item"`
		Response   struct {
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
				TotalTokens  int `json:"total_tokens"`
			} `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAIResponsesFrame})
		return false, false
	}

	switch event.Type {
	case "response.output_text.delta":
		if event.Delta != "" && !sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventDelta, Delta: event.Delta,
		}) {
			return false, false
		}
	case "response.reasoning_summary_text.delta":
		if event.Delta != "" && !sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventReasoningDelta, ReasoningDelta: event.Delta,
		}) {
			return false, false
		}
	case "response.output_text.annotation.added":
		if !sendOpenAIResponseSources(ctx, events, sources, []any{event.Annotation}, "") {
			return false, false
		}
	case "response.output_item.done":
		if !dispatchOpenAIResponseItem(ctx, events, sources, event.Item) {
			return false, false
		}
	case "response.completed":
		if event.Response.Usage != nil && !sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventUsage,
			Usage: &TokenUsage{
				PromptTokens:     event.Response.Usage.InputTokens,
				CompletionTokens: event.Response.Usage.OutputTokens,
				TotalTokens:      event.Response.Usage.TotalTokens,
			},
		}) {
			return false, false
		}
		return true, true
	case "response.failed", "response.error":
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAIResponsesStream})
		return false, false
	}
	return true, false
}

func dispatchOpenAIResponseItem(
	ctx context.Context,
	events chan<- ProviderEvent,
	sources *openAIResponsesSourceAccumulator,
	item map[string]any,
) bool {
	switch stringValue(item["type"]) {
	case "web_search_call":
		candidates := append(sourceValues(item["results"]), sourceValues(item["sources"])...)
		if action, ok := item["action"].(map[string]any); ok {
			candidates = append(candidates, sourceValues(action["sources"])...)
		}
		return sendOpenAIResponseSources(ctx, events, sources, candidates, "")
	case "message":
		for _, rawContent := range sourceValues(item["content"]) {
			content, ok := rawContent.(map[string]any)
			if !ok {
				continue
			}
			fallback := extractOpenAIResponseText(content["text"])
			if !sendOpenAIResponseSources(
				ctx, events, sources, sourceValues(content["annotations"]), fallback,
			) {
				return false
			}
		}
	}
	return true
}

func sendOpenAIResponseSources(
	ctx context.Context,
	events chan<- ProviderEvent,
	accumulator *openAIResponsesSourceAccumulator,
	values []any,
	fallbackContent string,
) bool {
	if accumulator == nil || accumulator.used >= websearch.MaxResults {
		return true
	}
	candidates := make([]websearch.Source, 0, len(values))
	for _, value := range values {
		record, ok := value.(map[string]any)
		if !ok {
			continue
		}
		url := firstOpenAIResponseText(record, "url", "uri", "link")
		if url == "" {
			continue
		}
		title := firstOpenAIResponseText(record, "title", "name")
		if title == "" {
			title = "Search source"
		}
		content := firstOpenAIResponseText(record, "content", "snippet", "text", "summary")
		if content == "" {
			content = fallbackContent
		}
		if content == "" {
			content = title
		}
		candidates = append(candidates, websearch.Source{Title: title, URL: url, Content: content})
	}
	normalized := websearch.NormalizeResult(websearch.Result{Sources: candidates}, websearch.MaxResults)
	newSources := make([]websearch.Source, 0, len(normalized.Sources))
	for _, source := range normalized.Sources {
		if accumulator.used >= websearch.MaxResults {
			break
		}
		if _, exists := accumulator.seen[source.URL]; exists {
			continue
		}
		accumulator.seen[source.URL] = struct{}{}
		accumulator.used++
		newSources = append(newSources, source)
	}
	if len(newSources) == 0 {
		return true
	}
	result := websearch.Result{Sources: newSources, Images: []websearch.Image{}}
	return sendProviderEvent(ctx, events, ProviderEvent{
		Type: ProviderEventSearch, Search: &result,
	})
}

func sourceValues(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	if value != nil {
		return []any{value}
	}
	return nil
}

func firstOpenAIResponseText(record map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := extractOpenAIResponseText(record[key]); value != "" {
			return value
		}
	}
	return ""
}

func extractOpenAIResponseText(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var text strings.Builder
		for _, item := range typed {
			text.WriteString(extractOpenAIResponseText(item))
		}
		return text.String()
	case map[string]any:
		return firstOpenAIResponseText(typed, "text", "content", "summary", "delta")
	default:
		return ""
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
