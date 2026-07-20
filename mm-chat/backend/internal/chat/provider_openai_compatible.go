package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	OpenAICompatibleProviderID              = "openai_compatible"
	openAICompatibleChatCompletionsPath     = "/chat/completions"
	openAICompatibleProviderIDOpenAI        = "openai"
	openAICompatibleProviderIDHyphenVariant = "openai-compatible"
	maxOpenAICompatibleToolPlanBytes        = 2 << 20
)

var (
	errOpenAICompatibleFrame  = errors.New("openai-compatible provider stream parse failed")
	errOpenAICompatibleStream = errors.New("openai-compatible provider stream failed")
)

type OpenAICompatibleProviderConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	ProviderID   string
	Timeout      time.Duration
	HTTPClient   *http.Client
}

type OpenAICompatibleProvider struct {
	endpoint     string
	apiKey       string
	defaultModel string
	providerID   string
	timeout      time.Duration
	client       *http.Client
}

func NewOpenAICompatibleProvider(
	cfg OpenAICompatibleProviderConfig,
) (*OpenAICompatibleProvider, error) {
	baseURL, err := normalizeOpenAICompatibleBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("openai-compatible provider api key is required")
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	return &OpenAICompatibleProvider{
		endpoint:     baseURL + openAICompatibleChatCompletionsPath,
		apiKey:       apiKey,
		defaultModel: strings.TrimSpace(cfg.DefaultModel),
		providerID:   strings.TrimSpace(cfg.ProviderID),
		timeout:      cfg.Timeout,
		client:       client,
	}, nil
}

func (p *OpenAICompatibleProvider) StreamChat(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}

	model := modelRef.ModelID
	if model == "" {
		return nil, errors.New("openai-compatible provider model is required")
	}

	payload, err := json.Marshal(openAICompatibleChatCompletionRequest{
		Model:  model,
		Stream: true,
		Messages: openAICompatibleMessages(
			input.SystemPrompt,
			providerMessagesOrPrompt(input),
		),
		ReasoningEffort: openAICompatibleReasoningEffort(input.UseReasoning),
	})
	if err != nil {
		return nil, fmt.Errorf("openai-compatible provider request encode failed: %w", err)
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
	}

	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		p.endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("openai-compatible provider request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("openai-compatible provider request failed: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("openai-compatible provider returned status %d", resp.StatusCode)
	}

	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		if cancel != nil {
			defer cancel()
		}

		streamOpenAICompatibleEvents(ctx, resp.Body, events)
	}()

	return events, nil
}

func (p *OpenAICompatibleProvider) ValidateModelRef(modelRef ModelRef) error {
	_, err := p.ResolveModelRef(modelRef)
	return err
}

func (p *OpenAICompatibleProvider) ResolveModelRef(modelRef ModelRef) (ModelRef, error) {
	providerID := strings.ToLower(strings.TrimSpace(modelRef.ProviderID))
	if allowedProviderID := strings.TrimSpace(p.providerID); allowedProviderID != "" &&
		providerID == strings.ToLower(allowedProviderID) {
		modelID := strings.TrimSpace(modelRef.ModelID)
		if modelID == "" {
			modelID = p.defaultModel
		}
		return ModelRef{
			ProviderID: allowedProviderID,
			ModelID:    modelID,
		}, nil
	}
	switch providerID {
	case OpenAICompatibleProviderID, openAICompatibleProviderIDOpenAI, openAICompatibleProviderIDHyphenVariant:
		modelID := strings.TrimSpace(modelRef.ModelID)
		if modelID == "" {
			modelID = p.defaultModel
		}
		return ModelRef{
			ProviderID: OpenAICompatibleProviderID,
			ModelID:    modelID,
		}, nil
	default:
		return ModelRef{}, ValidationError{
			Code:    "UNSUPPORTED_PROVIDER",
			Message: "modelRef.providerId is not supported by the configured provider",
		}
	}
}

type openAICompatibleChatCompletionRequest struct {
	Model           string                    `json:"model"`
	Stream          bool                      `json:"stream"`
	Messages        []openAICompatibleMessage `json:"messages"`
	ReasoningEffort string                    `json:"reasoning_effort,omitempty"`
	Tools           []ToolDefinition          `json:"tools,omitempty"`
	ToolChoice      string                    `json:"tool_choice,omitempty"`
}

type openAICompatibleMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAICompatibleStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content *string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAICompatibleToolPlanResponse struct {
	Choices []struct {
		Message struct {
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *OpenAICompatibleProvider) PlanTools(
	ctx context.Context,
	input ToolPlanRequest,
) ([]ToolCall, error) {
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelRef.ModelID) == "" {
		return nil, errors.New("openai-compatible provider model is required")
	}
	if len(input.Tools) == 0 {
		return []ToolCall{}, nil
	}

	payload, err := json.Marshal(openAICompatibleChatCompletionRequest{
		Model:  modelRef.ModelID,
		Stream: false,
		Messages: openAICompatibleMessages("", []ProviderMessage{{
			Role: "user", Content: input.Prompt,
		}}),
		Tools:      input.Tools,
		ToolChoice: "auto",
	})
	if err != nil {
		return nil, fmt.Errorf("openai-compatible tool plan encode failed: %w", err)
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(
		requestCtx,
		http.MethodPost,
		p.endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible tool plan request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai-compatible tool plan request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai-compatible provider returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenAICompatibleToolPlanBytes+1))
	if err != nil {
		return nil, fmt.Errorf("openai-compatible tool plan read failed: %w", err)
	}
	if len(body) > maxOpenAICompatibleToolPlanBytes {
		return nil, errors.New("openai-compatible tool plan response is too large")
	}

	var planned openAICompatibleToolPlanResponse
	if err := json.Unmarshal(body, &planned); err != nil {
		return nil, fmt.Errorf("openai-compatible tool plan decode failed: %w", err)
	}

	calls := make([]ToolCall, 0)
	for _, choice := range planned.Choices {
		for _, rawCall := range choice.Message.ToolCalls {
			if rawCall.Type != "" && rawCall.Type != "function" {
				return nil, errors.New("openai-compatible tool plan returned unsupported call type")
			}
			name := strings.TrimSpace(rawCall.Function.Name)
			if name == "" {
				return nil, errors.New("openai-compatible tool plan returned an empty function name")
			}
			args := map[string]any{}
			if strings.TrimSpace(rawCall.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(rawCall.Function.Arguments), &args); err != nil {
					return nil, errors.New("openai-compatible tool plan returned invalid arguments")
				}
			}
			calls = append(calls, ToolCall{
				ID:   strings.TrimSpace(rawCall.ID),
				Name: name,
				Args: args,
			})
		}
	}
	return calls, nil
}

func normalizeOpenAICompatibleBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return "", errors.New("openai-compatible provider base url is required")
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("openai-compatible provider base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("openai-compatible provider base url must use http or https")
	}

	return value, nil
}

type openAICompatibleContentPart struct {
	Type     string                        `json:"type"`
	Text     string                        `json:"text,omitempty"`
	ImageURL *openAICompatibleImageURLPart `json:"image_url,omitempty"`
}

type openAICompatibleImageURLPart struct {
	URL string `json:"url"`
}

func openAICompatibleMessages(systemPrompt string, providerMessages []ProviderMessage) []openAICompatibleMessage {
	messages := make([]openAICompatibleMessage, 0, len(providerMessages)+1)
	if systemPrompt = strings.TrimSpace(systemPrompt); systemPrompt != "" {
		messages = append(messages, openAICompatibleMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	for _, message := range providerMessages {
		content := any(message.Content)
		if message.Role == "user" {
			content = openAICompatibleUserContent(message.Content, message.Attachments)
		}
		messages = append(messages, openAICompatibleMessage{
			Role:    message.Role,
			Content: content,
		})
	}

	return messages
}

func providerMessagesOrPrompt(input ProviderRequest) []ProviderMessage {
	if len(input.Messages) > 0 {
		return input.Messages
	}
	return []ProviderMessage{{
		Role:        "user",
		Content:     input.Prompt,
		Attachments: input.Attachments,
	}}
}

func openAICompatibleUserContent(prompt string, attachments []ProviderAttachment) any {
	if len(attachments) == 0 {
		return prompt
	}

	parts := make([]openAICompatibleContentPart, 0, len(attachments)+1)
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, openAICompatibleContentPart{
			Type: "text",
			Text: prompt,
		})
	}
	for _, attachment := range attachments {
		mimeType := strings.TrimSpace(attachment.MimeType)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		parts = append(parts, openAICompatibleContentPart{
			Type: "image_url",
			ImageURL: &openAICompatibleImageURLPart{
				URL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(attachment.Data),
			},
		})
	}
	return parts
}

func openAICompatibleReasoningEffort(enabled bool) string {
	if enabled {
		return "high"
	}
	return ""
}

func streamOpenAICompatibleEvents(
	ctx context.Context,
	reader io.Reader,
	events chan<- ProviderEvent,
) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	dataLines := make([]string, 0, 1)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			keepReading, done := dispatchOpenAICompatibleData(ctx, strings.Join(dataLines, "\n"), events)
			if done || !keepReading {
				return
			}
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimPrefix(data, " ")
			dataLines = append(dataLines, data)
		}
	}

	if len(dataLines) > 0 {
		keepReading, done := dispatchOpenAICompatibleData(ctx, strings.Join(dataLines, "\n"), events)
		if done || !keepReading {
			return
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAICompatibleStream})
		return
	}

	if ctx.Err() == nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAICompatibleStream})
	}
}

func dispatchOpenAICompatibleData(
	ctx context.Context,
	data string,
	events chan<- ProviderEvent,
) (bool, bool) {
	data = strings.TrimSpace(data)
	if data == "" {
		return true, false
	}
	if data == "[DONE]" {
		return false, true
	}

	var chunk openAICompatibleStreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAICompatibleFrame})
		return false, false
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.Content == nil || *choice.Delta.Content == "" {
			continue
		}
		if !sendProviderEvent(ctx, events, ProviderEvent{
			Type:  ProviderEventDelta,
			Delta: *choice.Delta.Content,
		}) {
			return false, false
		}
	}

	if chunk.Usage != nil {
		if !sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventUsage,
			Usage: &TokenUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			},
		}) {
			return false, false
		}
	}

	return true, false
}

func sendProviderEvent(ctx context.Context, events chan<- ProviderEvent, event ProviderEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
