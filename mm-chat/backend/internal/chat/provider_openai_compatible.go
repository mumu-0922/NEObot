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
	maxOpenAICompatibleToolArgumentsBytes   = 64 << 10
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
	deepSeek     bool
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
		deepSeek:     isOfficialDeepSeekBaseURL(baseURL),
		timeout:      cfg.Timeout,
		client:       client,
	}, nil
}

func (p *OpenAICompatibleProvider) StreamChat(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	return p.StreamToolRound(ctx, ProviderRoundRequest{
		ProviderRequest: input,
	})
}

func (p *OpenAICompatibleProvider) StreamToolRound(
	ctx context.Context,
	input ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}

	model := modelRef.ModelID
	if model == "" {
		return nil, errors.New("openai-compatible provider model is required")
	}

	messages := openAICompatibleMessages(
		input.SystemPrompt,
		providerMessagesOrPrompt(input.ProviderRequest),
	)
	messages = appendOpenAICompatibleContinuation(messages, input.Continuation)
	enableThinking, thinking := p.thinkingControls(input.DisableThinking, true)
	payload, err := json.Marshal(openAICompatibleChatCompletionRequest{
		Model:    model,
		Stream:   true,
		Messages: messages,
		Tools:    input.Tools,
		ToolChoice: openAICompatibleToolChoice(
			normalizeProviderToolChoice(input.ToolChoice),
			input.Tools,
		),
		ReasoningEffort: openAIReasoningEffort(
			model,
			input.UseReasoning,
			effectiveReasoningEffort(input.ProviderRequest),
		),
		EnableThinking: enableThinking,
		Thinking:       thinking,
		MaxTokens:      input.MaxOutputTokens,
		Temperature:    input.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("openai-compatible provider request encode failed: %w", err)
	}

	return p.streamChatCompletion(ctx, payload)
}

func (p *OpenAICompatibleProvider) streamChatCompletion(
	ctx context.Context,
	payload []byte,
) (<-chan ProviderEvent, error) {
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
	if allowedProviderID := strings.TrimSpace(p.providerID); allowedProviderID != "" {
		switch providerID {
		case strings.ToLower(allowedProviderID), OpenAICompatibleProviderID,
			openAICompatibleProviderIDOpenAI, openAICompatibleProviderIDHyphenVariant:
			modelID := strings.TrimSpace(modelRef.ModelID)
			if modelID == "" {
				modelID = p.defaultModel
			}
			return ModelRef{
				ProviderID: allowedProviderID,
				ModelID:    modelID,
			}, nil
		default:
			return ModelRef{}, ValidationError{
				Code:    "UNSUPPORTED_PROVIDER",
				Message: "modelRef.providerId is not supported by the configured provider",
			}
		}
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
	EnableThinking  *bool                     `json:"enable_thinking,omitempty"`
	Thinking        *openAICompatibleThinking `json:"thinking,omitempty"`
	MaxTokens       int                       `json:"max_tokens,omitempty"`
	Temperature     *float64                  `json:"temperature,omitempty"`
	Tools           []ToolDefinition          `json:"tools,omitempty"`
	ToolChoice      any                       `json:"tool_choice,omitempty"`
}

type openAICompatibleThinking struct {
	Type string `json:"type"`
}

func disabledThinkingValue(disabled bool) *bool {
	if !disabled {
		return nil
	}
	value := false
	return &value
}

func (p *OpenAICompatibleProvider) thinkingControls(
	disabled bool,
	includeCompatibleExtension bool,
) (*bool, *openAICompatibleThinking) {
	if !disabled || !includeCompatibleExtension {
		return nil, nil
	}
	if p != nil && p.deepSeek {
		return nil, &openAICompatibleThinking{Type: "disabled"}
	}
	return disabledThinkingValue(true), nil
}

func isOfficialDeepSeekBaseURL(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(parsed.Hostname(), "api.deepseek.com")
}

type openAICompatibleMessage struct {
	Role             string                            `json:"role"`
	Content          any                               `json:"content"`
	ReasoningContent string                            `json:"reasoning_content,omitempty"`
	ToolCalls        []openAICompatibleMessageToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string                            `json:"tool_call_id,omitempty"`
}

type openAICompatibleMessageToolCall struct {
	ID       string                              `json:"id"`
	Type     string                              `json:"type"`
	Function openAICompatibleMessageToolFunction `json:"function"`
}

type openAICompatibleMessageToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAICompatibleStreamChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Content          *string         `json:"content"`
			ReasoningContent json.RawMessage `json:"reasoning_content"`
			Reasoning        json.RawMessage `json:"reasoning"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
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
	return p.planTools(ctx, input, true)
}

func (p *OpenAICompatibleProvider) planTools(
	ctx context.Context,
	input ToolPlanRequest,
	includeEnableThinking bool,
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

	enableThinking, thinking := p.thinkingControls(
		input.DisableThinking,
		includeEnableThinking,
	)
	payload, err := json.Marshal(openAICompatibleChatCompletionRequest{
		Model:          modelRef.ModelID,
		Stream:         false,
		EnableThinking: enableThinking,
		Thinking:       thinking,
		MaxTokens:      input.MaxOutputTokens,
		Temperature:    input.Temperature,
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
	if len(planned.Choices) != 1 {
		return nil, errors.New("openai-compatible tool plan returned an invalid choice count")
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
			rawArguments := strings.TrimSpace(rawCall.Function.Arguments)
			if rawArguments == "" {
				return nil, errors.New("openai-compatible tool plan returned missing arguments")
			}
			var args map[string]any
			if err := json.Unmarshal([]byte(rawArguments), &args); err != nil || args == nil {
				return nil, errors.New("openai-compatible tool plan returned invalid arguments")
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

func appendOpenAICompatibleContinuation(
	messages []openAICompatibleMessage,
	exchanges []ProviderToolExchange,
) []openAICompatibleMessage {
	for _, exchange := range exchanges {
		if len(exchange.Calls) == 0 {
			continue
		}
		toolCalls := make([]openAICompatibleMessageToolCall, 0, len(exchange.Calls))
		for _, call := range exchange.Calls {
			toolCalls = append(toolCalls, openAICompatibleMessageToolCall{
				ID:   strings.TrimSpace(call.ID),
				Type: "function",
				Function: openAICompatibleMessageToolFunction{
					Name:      strings.TrimSpace(call.Name),
					Arguments: strings.TrimSpace(call.Arguments),
				},
			})
		}
		messages = append(messages, openAICompatibleMessage{
			Role:             "assistant",
			Content:          nullableOpenAICompatibleContent(exchange.AssistantContent),
			ReasoningContent: strings.TrimSpace(exchange.AssistantReasoning),
			ToolCalls:        toolCalls,
		})
		for _, result := range exchange.Results {
			messages = append(messages, openAICompatibleMessage{
				Role:       "tool",
				Content:    result.Content,
				ToolCallID: strings.TrimSpace(result.CallID),
			})
		}
	}
	return messages
}

func nullableOpenAICompatibleContent(value string) any {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return nil
}

func openAICompatibleToolChoice(value string, tools []ToolDefinition) any {
	if len(tools) == 0 {
		return nil
	}
	if value == ProviderToolChoiceRequired && len(tools) > 0 {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": strings.TrimSpace(tools[0].Function.Name),
			},
		}
	}
	if value == ProviderToolChoiceAuto {
		return ProviderToolChoiceAuto
	}
	return nil
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

func streamOpenAICompatibleEvents(
	ctx context.Context,
	reader io.Reader,
	events chan<- ProviderEvent,
) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	dataLines := make([]string, 0, 1)
	toolCalls := newOpenAICompatibleToolCallAccumulator()
	completed := false
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			keepReading, eventCompleted := dispatchOpenAICompatibleData(
				ctx,
				strings.Join(dataLines, "\n"),
				events,
				toolCalls,
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
			data = strings.TrimPrefix(data, " ")
			dataLines = append(dataLines, data)
		}
	}

	if len(dataLines) > 0 {
		keepReading, eventCompleted := dispatchOpenAICompatibleData(
			ctx,
			strings.Join(dataLines, "\n"),
			events,
			toolCalls,
		)
		completed = completed || eventCompleted
		if !keepReading {
			return
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAICompatibleStream})
		return
	}

	if ctx.Err() == nil && !completed {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAICompatibleStream})
	}
}

func dispatchOpenAICompatibleData(
	ctx context.Context,
	data string,
	events chan<- ProviderEvent,
	toolCalls *openAICompatibleToolCallAccumulator,
) (bool, bool) {
	data = strings.TrimSpace(data)
	if data == "" {
		return true, false
	}
	if data == "[DONE]" {
		if !toolCalls.complete(ctx, events) {
			return false, false
		}
		return false, true
	}

	var chunk openAICompatibleStreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errOpenAICompatibleFrame})
		return false, false
	}

	completed := false
	for _, choice := range chunk.Choices {
		for _, toolCall := range choice.Delta.ToolCalls {
			if !toolCalls.append(ctx, events, choice.Index, toolCall.Index, toolCall.ID,
				toolCall.Type, toolCall.Function.Name, toolCall.Function.Arguments) {
				return false, false
			}
		}
		reasoning := openAICompatibleReasoningDelta(
			choice.Delta.ReasoningContent,
			choice.Delta.Reasoning,
		)
		if reasoning != "" {
			if !sendProviderEvent(ctx, events, ProviderEvent{
				Type: ProviderEventReasoningDelta, ReasoningDelta: reasoning,
			}) {
				return false, false
			}
		}
		if strings.TrimSpace(choice.FinishReason) != "" {
			completed = true
		}
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

	if completed && !toolCalls.complete(ctx, events) {
		return false, false
	}
	return true, completed
}

type openAICompatibleToolCallKey struct {
	choiceIndex int
	callIndex   int
}

type openAICompatibleToolCallAccumulator struct {
	order     []openAICompatibleToolCallKey
	calls     map[openAICompatibleToolCallKey]*ProviderToolCall
	completed bool
}

func newOpenAICompatibleToolCallAccumulator() *openAICompatibleToolCallAccumulator {
	return &openAICompatibleToolCallAccumulator{
		order: []openAICompatibleToolCallKey{},
		calls: map[openAICompatibleToolCallKey]*ProviderToolCall{},
	}
}

func (accumulator *openAICompatibleToolCallAccumulator) append(
	ctx context.Context,
	events chan<- ProviderEvent,
	choiceIndex int,
	callIndex int,
	id string,
	callType string,
	nameDelta string,
	argumentsDelta string,
) bool {
	key := openAICompatibleToolCallKey{
		choiceIndex: choiceIndex,
		callIndex:   callIndex,
	}
	call, ok := accumulator.calls[key]
	if !ok {
		call = &ProviderToolCall{
			ChoiceIndex: choiceIndex,
			CallIndex:   callIndex,
		}
		accumulator.calls[key] = call
		accumulator.order = append(accumulator.order, key)
	}
	if callType != "" && callType != "function" {
		call.FailureCategory = "unsupported_call_type"
	}
	if id = strings.TrimSpace(id); id != "" {
		switch {
		case call.ID == "":
			call.ID = id
		case call.ID != id && !strings.HasSuffix(call.ID, id):
			call.ID += id
		}
		if len(call.ID) > 256 {
			call.ID = truncateProcessUTF8(call.ID, 256)
			call.FailureCategory = "invalid_call_id"
		}
	}
	if nameDelta != "" {
		call.Name += nameDelta
		if len(call.Name) > maxToolNameBytes {
			call.Name = truncateProcessUTF8(call.Name, maxToolNameBytes)
			call.FailureCategory = "invalid_tool_name"
		}
	}
	if argumentsDelta != "" {
		remaining := maxOpenAICompatibleToolArgumentsBytes - len(call.Arguments)
		if remaining <= 0 {
			call.FailureCategory = "arguments_too_large"
		} else {
			call.Arguments += truncateProcessUTF8(argumentsDelta, remaining)
			if len(argumentsDelta) > remaining {
				call.FailureCategory = "arguments_too_large"
			}
		}
	}
	delta := ProviderToolCallDelta{
		ChoiceIndex:    choiceIndex,
		CallIndex:      callIndex,
		ID:             id,
		NameDelta:      nameDelta,
		ArgumentsDelta: argumentsDelta,
	}
	return sendProviderEvent(ctx, events, ProviderEvent{
		Type:          ProviderEventToolCallDelta,
		ToolCallDelta: &delta,
	})
}

func (accumulator *openAICompatibleToolCallAccumulator) complete(
	ctx context.Context,
	events chan<- ProviderEvent,
) bool {
	if accumulator.completed {
		return true
	}
	for _, key := range accumulator.order {
		call := *accumulator.calls[key]
		call.ID = strings.TrimSpace(call.ID)
		call.Name = strings.TrimSpace(call.Name)
		call.Arguments = strings.TrimSpace(call.Arguments)
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d_%d", call.ChoiceIndex, call.CallIndex)
		}
		if call.Name == "" && call.FailureCategory == "" {
			call.FailureCategory = "invalid_tool_name"
		}
		if !sendProviderEvent(ctx, events, ProviderEvent{
			Type:     ProviderEventToolCallCompleted,
			ToolCall: &call,
		}) {
			return false
		}
	}
	accumulator.completed = true
	return true
}

func openAICompatibleReasoningDelta(values ...json.RawMessage) string {
	for _, value := range values {
		if len(value) == 0 || string(value) == "null" {
			continue
		}
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			return text
		}
	}
	return ""
}

func sendProviderEvent(ctx context.Context, events chan<- ProviderEvent, event ProviderEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case events <- event:
		return true
	}
}
