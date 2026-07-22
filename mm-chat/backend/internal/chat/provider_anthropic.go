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
	AnthropicProviderID            = "anthropic"
	defaultAnthropicServiceURL     = "https://api.anthropic.com"
	anthropicMessagesPath          = "/v1/messages"
	anthropicVersion               = "2023-06-01"
	defaultAnthropicMaxTokens      = 8_192
	defaultAnthropicThinkingTokens = 4_096
	maxAnthropicToolPlanBytes      = 2 << 20
	maxAnthropicToolArgumentsBytes = 64 << 10
)

var (
	errAnthropicFrame  = errors.New("anthropic provider stream parse failed")
	errAnthropicStream = errors.New("anthropic provider stream failed")
)

type AnthropicProviderConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	ProviderID   string
	Timeout      time.Duration
	HTTPClient   *http.Client
}

type AnthropicProvider struct {
	endpoint     string
	apiKey       string
	defaultModel string
	providerID   string
	timeout      time.Duration
	client       *http.Client
}

func NewAnthropicProvider(cfg AnthropicProviderConfig) (*AnthropicProvider, error) {
	baseURL, err := normalizeAnthropicServiceBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("anthropic provider api key is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &AnthropicProvider{
		endpoint:     baseURL + anthropicMessagesPath,
		apiKey:       apiKey,
		defaultModel: strings.TrimSpace(cfg.DefaultModel),
		providerID:   strings.TrimSpace(cfg.ProviderID),
		timeout:      cfg.Timeout,
		client:       &clientCopy,
	}, nil
}

func (p *AnthropicProvider) StreamChat(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	return p.StreamToolRound(ctx, ProviderRoundRequest{
		ProviderRequest: input,
	})
}

func (p *AnthropicProvider) StreamToolRound(
	ctx context.Context,
	input ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelRef.ModelID) == "" {
		return nil, errors.New("anthropic provider model is required")
	}
	messages, err := anthropicMessages(providerMessagesOrPrompt(input.ProviderRequest))
	if err != nil {
		return nil, err
	}
	messages, err = appendAnthropicContinuation(messages, input.Continuation)
	if err != nil {
		return nil, err
	}
	tools := anthropicTools(input.Tools)
	request := anthropicMessagesRequest{
		Model:     modelRef.ModelID,
		MaxTokens: defaultAnthropicMaxTokens,
		Stream:    true,
		System:    strings.TrimSpace(input.SystemPrompt),
		Messages:  messages,
		Tools:     tools,
		ToolChoice: anthropicToolChoiceForRound(
			normalizeProviderToolChoice(input.ToolChoice),
			tools,
			input.UseReasoning,
		),
	}
	if input.UseReasoning {
		budgetTokens := anthropicThinkingBudget(effectiveReasoningEffort(input.ProviderRequest))
		request.MaxTokens = anthropicMaxTokens(budgetTokens)
		request.Thinking = &anthropicThinkingConfig{
			Type: "enabled", BudgetTokens: budgetTokens,
		}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("anthropic provider request encode failed: %w", err)
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
	}
	req, err := http.NewRequestWithContext(
		requestCtx, http.MethodPost, p.endpoint, bytes.NewReader(payload),
	)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("anthropic provider request build failed: %w", err)
	}
	p.setHeaders(req, "text/event-stream")
	resp, err := p.client.Do(req)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("anthropic provider request failed: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4_096))
		_ = resp.Body.Close()
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("anthropic provider returned status %d", resp.StatusCode)
	}

	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		if cancel != nil {
			defer cancel()
		}
		streamAnthropicEvents(ctx, resp.Body, events)
	}()
	return events, nil
}

func (p *AnthropicProvider) ValidateModelRef(modelRef ModelRef) error {
	_, err := p.ResolveModelRef(modelRef)
	return err
}

func (p *AnthropicProvider) ResolveModelRef(modelRef ModelRef) (ModelRef, error) {
	providerID := strings.ToLower(strings.TrimSpace(modelRef.ProviderID))
	if allowedProviderID := strings.TrimSpace(p.providerID); allowedProviderID != "" &&
		providerID == strings.ToLower(allowedProviderID) {
		modelID := strings.TrimSpace(modelRef.ModelID)
		if modelID == "" {
			modelID = p.defaultModel
		}
		return ModelRef{ProviderID: allowedProviderID, ModelID: modelID}, nil
	}
	if providerID != AnthropicProviderID && providerID != "claude" {
		return ModelRef{}, ValidationError{
			Code:    "UNSUPPORTED_PROVIDER",
			Message: "modelRef.providerId is not supported by the configured provider",
		}
	}
	modelID := strings.TrimSpace(modelRef.ModelID)
	if modelID == "" {
		modelID = p.defaultModel
	}
	return ModelRef{ProviderID: AnthropicProviderID, ModelID: modelID}, nil
}

func (p *AnthropicProvider) PlanTools(
	ctx context.Context,
	input ToolPlanRequest,
) ([]ToolCall, error) {
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelRef.ModelID) == "" {
		return nil, errors.New("anthropic provider model is required")
	}
	if len(input.Tools) == 0 {
		return []ToolCall{}, nil
	}
	tools := make([]anthropicTool, 0, len(input.Tools))
	for _, tool := range input.Tools {
		tools = append(tools, anthropicTool{
			Name: tool.Function.Name, Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}
	payload, err := json.Marshal(anthropicMessagesRequest{
		Model: modelRef.ModelID, MaxTokens: 1_024,
		Messages: []anthropicMessage{{Role: "user", Content: input.Prompt}},
		Tools:    tools, ToolChoice: &anthropicToolChoice{Type: "auto"},
	})
	if err != nil {
		return nil, fmt.Errorf("anthropic tool plan encode failed: %w", err)
	}
	requestCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(
		requestCtx, http.MethodPost, p.endpoint, bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("anthropic tool plan request build failed: %w", err)
	}
	p.setHeaders(req, "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic tool plan request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4_096))
		return nil, fmt.Errorf("anthropic provider returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnthropicToolPlanBytes+1))
	if err != nil {
		return nil, fmt.Errorf("anthropic tool plan read failed: %w", err)
	}
	if len(body) > maxAnthropicToolPlanBytes {
		return nil, errors.New("anthropic tool plan response is too large")
	}
	var decoded anthropicMessagesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("anthropic tool plan decode failed: %w", err)
	}
	calls := make([]ToolCall, 0)
	for _, block := range decoded.Content {
		if block.Type != "tool_use" {
			continue
		}
		name := strings.TrimSpace(block.Name)
		if name == "" {
			return nil, errors.New("anthropic tool plan returned an empty tool name")
		}
		args := block.Input
		if args == nil {
			args = map[string]any{}
		}
		calls = append(calls, ToolCall{ID: strings.TrimSpace(block.ID), Name: name, Args: args})
	}
	return calls, nil
}

func (p *AnthropicProvider) setHeaders(req *http.Request, accept string) {
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)
}

type anthropicMessagesRequest struct {
	Model      string                   `json:"model"`
	MaxTokens  int                      `json:"max_tokens"`
	Stream     bool                     `json:"stream,omitempty"`
	System     string                   `json:"system,omitempty"`
	Messages   []anthropicMessage       `json:"messages"`
	Thinking   *anthropicThinkingConfig `json:"thinking,omitempty"`
	Tools      []anthropicTool          `json:"tools,omitempty"`
	ToolChoice *anthropicToolChoice     `json:"tool_choice,omitempty"`
}

type anthropicThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicContentBlock struct {
	Type   string                `json:"type"`
	Text   string                `json:"text,omitempty"`
	Source *anthropicImageSource `json:"source,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type anthropicToolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicRoundState struct {
	AssistantBlocks []map[string]any
}

type anthropicMessagesResponse struct {
	Content []struct {
		Type  string         `json:"type"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"content"`
}

func anthropicTools(definitions []ToolDefinition) []anthropicTool {
	tools := make([]anthropicTool, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Type != "" && definition.Type != "function" {
			continue
		}
		name := strings.TrimSpace(definition.Function.Name)
		if name == "" {
			continue
		}
		inputSchema := definition.Function.Parameters
		if inputSchema == nil {
			inputSchema = map[string]any{"type": "object"}
		}
		tools = append(tools, anthropicTool{
			Name:        name,
			Description: strings.TrimSpace(definition.Function.Description),
			InputSchema: inputSchema,
		})
	}
	return tools
}

func anthropicToolChoiceForRound(
	choice string,
	tools []anthropicTool,
	thinkingEnabled bool,
) *anthropicToolChoice {
	if len(tools) == 0 {
		return nil
	}
	if choice == ProviderToolChoiceRequired && !thinkingEnabled {
		return &anthropicToolChoice{Type: "tool", Name: tools[0].Name}
	}
	return &anthropicToolChoice{Type: "auto"}
}

func appendAnthropicContinuation(
	messages []anthropicMessage,
	exchanges []ProviderToolExchange,
) ([]anthropicMessage, error) {
	for _, exchange := range exchanges {
		if len(exchange.Calls) == 0 {
			continue
		}
		blocks := anthropicContinuationBlocks(exchange)
		if len(blocks) == 0 {
			return nil, errors.New("anthropic continuation assistant blocks are required")
		}
		messages = append(messages, anthropicMessage{
			Role:    "assistant",
			Content: blocks,
		})
		results := make([]anthropicToolResultBlock, 0, len(exchange.Results))
		for _, result := range exchange.Results {
			callID := strings.TrimSpace(result.CallID)
			if callID == "" {
				return nil, errors.New("anthropic continuation tool result id is required")
			}
			results = append(results, anthropicToolResultBlock{
				Type:      "tool_result",
				ToolUseID: callID,
				Content:   result.Content,
				IsError:   result.IsError,
			})
		}
		if len(results) == 0 {
			return nil, errors.New("anthropic continuation tool results are required")
		}
		messages = append(messages, anthropicMessage{
			Role:    "user",
			Content: results,
		})
	}
	return messages, nil
}

func anthropicContinuationBlocks(exchange ProviderToolExchange) []map[string]any {
	if state, ok := exchange.ProviderState.(anthropicRoundState); ok &&
		len(state.AssistantBlocks) > 0 {
		return cloneAnthropicAssistantBlocks(state.AssistantBlocks)
	}
	if state, ok := exchange.ProviderState.(*anthropicRoundState); ok &&
		state != nil && len(state.AssistantBlocks) > 0 {
		return cloneAnthropicAssistantBlocks(state.AssistantBlocks)
	}

	blocks := make([]map[string]any, 0, len(exchange.Calls)+1)
	if content := strings.TrimSpace(exchange.AssistantContent); content != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": content})
	}
	for _, call := range exchange.Calls {
		input := map[string]any{}
		if err := json.Unmarshal([]byte(call.Arguments), &input); err != nil || input == nil {
			input = map[string]any{}
		}
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    strings.TrimSpace(call.ID),
			"name":  strings.TrimSpace(call.Name),
			"input": input,
		})
	}
	return blocks
}

func cloneAnthropicAssistantBlocks(blocks []map[string]any) []map[string]any {
	encoded, err := json.Marshal(blocks)
	if err != nil {
		return nil
	}
	var cloned []map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil
	}
	return cloned
}

func anthropicMessages(providerMessages []ProviderMessage) ([]anthropicMessage, error) {
	messages := make([]anthropicMessage, 0, len(providerMessages))
	for _, message := range providerMessages {
		role := "user"
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			role = "assistant"
		}
		content := any(message.Content)
		if len(message.Attachments) > 0 {
			if role != "user" {
				return nil, errors.New("anthropic assistant image history is unsupported")
			}
			blocks := make([]anthropicContentBlock, 0, len(message.Attachments)+1)
			if strings.TrimSpace(message.Content) != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: message.Content})
			}
			for _, attachment := range message.Attachments {
				mimeType := strings.ToLower(strings.TrimSpace(attachment.MimeType))
				if !anthropicImageMIMETypeSupported(mimeType) || len(attachment.Data) == 0 {
					return nil, errors.New("anthropic provider received an unsupported image attachment")
				}
				blocks = append(blocks, anthropicContentBlock{
					Type: "image",
					Source: &anthropicImageSource{
						Type: "base64", MediaType: mimeType,
						Data: base64.StdEncoding.EncodeToString(attachment.Data),
					},
				})
			}
			content = blocks
		}
		messages = append(messages, anthropicMessage{Role: role, Content: content})
	}
	return messages, nil
}

func anthropicImageMIMETypeSupported(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func normalizeAnthropicServiceBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" || value == "default" {
		value = defaultAnthropicServiceURL
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("anthropic provider base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("anthropic provider base url must use http or https")
	}
	path := strings.TrimRight(parsed.Path, "/")
	path = strings.TrimSuffix(path, "/v1/messages")
	path = strings.TrimSuffix(path, "/v1/models")
	path = strings.TrimSuffix(path, "/v1")
	parsed.Path = strings.TrimRight(path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

type anthropicStreamEnvelope struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		Thinking    string `json:"thinking"`
		Signature   string `json:"signature"`
		Data        string `json:"data"`
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	ContentBlock *struct {
		Type      string          `json:"type"`
		Text      string          `json:"text"`
		Thinking  string          `json:"thinking"`
		Signature string          `json:"signature"`
		Data      string          `json:"data"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Input     json.RawMessage `json:"input"`
	} `json:"content_block"`
	Message *struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage anthropicUsage `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamState struct {
	promptTokens     int
	completionTokens int
	assistantBlocks  []map[string]any
	blockPositions   map[int]int
	toolCalls        map[int]*ProviderToolCall
	toolOrder        []int
	completedTools   map[int]bool
}

func newAnthropicStreamState() *anthropicStreamState {
	return &anthropicStreamState{
		assistantBlocks: []map[string]any{},
		blockPositions:  map[int]int{},
		toolCalls:       map[int]*ProviderToolCall{},
		toolOrder:       []int{},
		completedTools:  map[int]bool{},
	}
}

func streamAnthropicEvents(ctx context.Context, reader io.Reader, events chan<- ProviderEvent) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	state := newAnthropicStreamState()
	dataLines := make([]string, 0, 1)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			keepReading, done := dispatchAnthropicData(
				ctx, strings.Join(dataLines, "\n"), events, state,
			)
			if done || !keepReading {
				return
			}
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
			dataLines = append(dataLines, data)
		}
	}
	if len(dataLines) > 0 {
		keepReading, done := dispatchAnthropicData(
			ctx, strings.Join(dataLines, "\n"), events, state,
		)
		if done || !keepReading {
			return
		}
	}
	if scanner.Err() != nil && ctx.Err() == nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errAnthropicStream})
		return
	}
	if ctx.Err() == nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errAnthropicStream})
	}
}

func dispatchAnthropicData(
	ctx context.Context,
	data string,
	events chan<- ProviderEvent,
	state *anthropicStreamState,
) (bool, bool) {
	data = strings.TrimSpace(data)
	if data == "" {
		return true, false
	}
	if data == "[DONE]" {
		if !state.completeRound(ctx, events) {
			return false, false
		}
		return false, true
	}
	var envelope anthropicStreamEnvelope
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errAnthropicFrame})
		return false, false
	}
	switch envelope.Type {
	case "ping":
		return true, false
	case "content_block_start":
		return state.startContentBlock(ctx, events, envelope), false
	case "content_block_stop":
		return state.stopContentBlock(ctx, events, envelope.Index), false
	case "message_start":
		if envelope.Message != nil {
			state.promptTokens = envelope.Message.Usage.InputTokens
			state.completionTokens = envelope.Message.Usage.OutputTokens
		}
		return true, false
	case "content_block_delta":
		return state.applyContentBlockDelta(ctx, events, envelope), false
	case "message_delta":
		if envelope.Usage.InputTokens > 0 {
			state.promptTokens = envelope.Usage.InputTokens
		}
		if envelope.Usage.OutputTokens > 0 {
			state.completionTokens = envelope.Usage.OutputTokens
		}
		usage := &TokenUsage{
			PromptTokens: state.promptTokens, CompletionTokens: state.completionTokens,
			TotalTokens: state.promptTokens + state.completionTokens,
		}
		return sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventUsage, Usage: usage,
		}), false
	case "message_stop":
		if !state.completeRound(ctx, events) {
			return false, false
		}
		return false, true
	case "error":
		sendProviderEvent(ctx, events, ProviderEvent{Error: errAnthropicStream})
		return false, false
	default:
		return true, false
	}
}

func (state *anthropicStreamState) startContentBlock(
	ctx context.Context,
	events chan<- ProviderEvent,
	envelope anthropicStreamEnvelope,
) bool {
	if envelope.ContentBlock == nil {
		return true
	}
	block := envelope.ContentBlock
	position := len(state.assistantBlocks)
	state.blockPositions[envelope.Index] = position
	switch block.Type {
	case "text":
		state.assistantBlocks = append(state.assistantBlocks, map[string]any{
			"type": "text",
			"text": block.Text,
		})
		if block.Text != "" {
			return sendProviderEvent(ctx, events, ProviderEvent{
				Type: ProviderEventDelta, Delta: block.Text,
			})
		}
	case "thinking":
		state.assistantBlocks = append(state.assistantBlocks, map[string]any{
			"type":      "thinking",
			"thinking":  block.Thinking,
			"signature": block.Signature,
		})
		if block.Thinking != "" {
			return sendProviderEvent(ctx, events, ProviderEvent{
				Type:           ProviderEventReasoningDelta,
				ReasoningDelta: block.Thinking,
			})
		}
	case "redacted_thinking":
		state.assistantBlocks = append(state.assistantBlocks, map[string]any{
			"type": "redacted_thinking",
			"data": block.Data,
		})
	case "tool_use":
		id := strings.TrimSpace(block.ID)
		name := strings.TrimSpace(block.Name)
		call := &ProviderToolCall{
			CallIndex: envelope.Index,
			ID:        id,
			Name:      name,
		}
		if len(call.ID) > 256 {
			call.ID = truncateProcessUTF8(call.ID, 256)
			call.FailureCategory = "invalid_call_id"
		}
		if len(call.Name) > maxToolNameBytes {
			call.Name = truncateProcessUTF8(call.Name, maxToolNameBytes)
			call.FailureCategory = "invalid_tool_name"
		}
		input := anthropicToolInput(block.Input)
		if len(input) > 0 {
			encoded, _ := json.Marshal(input)
			if string(encoded) != "{}" {
				call.Arguments = string(encoded)
			}
		}
		state.toolCalls[envelope.Index] = call
		state.toolOrder = append(state.toolOrder, envelope.Index)
		state.assistantBlocks = append(state.assistantBlocks, map[string]any{
			"type":  "tool_use",
			"id":    call.ID,
			"name":  call.Name,
			"input": input,
		})
		delta := ProviderToolCallDelta{
			CallIndex: envelope.Index,
			ID:        call.ID,
			NameDelta: call.Name,
		}
		return sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventToolCallDelta, ToolCallDelta: &delta,
		})
	default:
		delete(state.blockPositions, envelope.Index)
	}
	return true
}

func (state *anthropicStreamState) applyContentBlockDelta(
	ctx context.Context,
	events chan<- ProviderEvent,
	envelope anthropicStreamEnvelope,
) bool {
	position, hasBlock := state.blockPositions[envelope.Index]
	switch envelope.Delta.Type {
	case "text_delta":
		if envelope.Delta.Text == "" {
			return true
		}
		if hasBlock {
			state.assistantBlocks[position]["text"] =
				stringValue(state.assistantBlocks[position]["text"]) + envelope.Delta.Text
		}
		return sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventDelta, Delta: envelope.Delta.Text,
		})
	case "thinking_delta":
		if envelope.Delta.Thinking == "" {
			return true
		}
		if hasBlock {
			state.assistantBlocks[position]["thinking"] =
				stringValue(state.assistantBlocks[position]["thinking"]) + envelope.Delta.Thinking
		}
		return sendProviderEvent(ctx, events, ProviderEvent{
			Type:           ProviderEventReasoningDelta,
			ReasoningDelta: envelope.Delta.Thinking,
		})
	case "signature_delta":
		if hasBlock && envelope.Delta.Signature != "" {
			state.assistantBlocks[position]["signature"] =
				stringValue(state.assistantBlocks[position]["signature"]) + envelope.Delta.Signature
		}
		return true
	case "redacted_thinking_delta":
		if hasBlock && envelope.Delta.Data != "" {
			state.assistantBlocks[position]["data"] =
				stringValue(state.assistantBlocks[position]["data"]) + envelope.Delta.Data
		}
		return true
	case "input_json_delta", "tool_use_delta":
		call := state.toolCalls[envelope.Index]
		if call == nil || envelope.Delta.PartialJSON == "" {
			return true
		}
		fragment := envelope.Delta.PartialJSON
		remaining := maxAnthropicToolArgumentsBytes - len(call.Arguments)
		if remaining <= 0 {
			call.FailureCategory = "arguments_too_large"
			fragment = ""
		} else if len(fragment) > remaining {
			call.Arguments += truncateProcessUTF8(fragment, remaining)
			call.FailureCategory = "arguments_too_large"
			fragment = truncateProcessUTF8(fragment, remaining)
		} else {
			call.Arguments += fragment
		}
		delta := ProviderToolCallDelta{
			CallIndex:      envelope.Index,
			ArgumentsDelta: fragment,
		}
		return sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventToolCallDelta, ToolCallDelta: &delta,
		})
	default:
		return true
	}
}

func (state *anthropicStreamState) stopContentBlock(
	ctx context.Context,
	events chan<- ProviderEvent,
	index int,
) bool {
	call := state.toolCalls[index]
	if call == nil || state.completedTools[index] {
		return true
	}
	call.ID = strings.TrimSpace(call.ID)
	call.Name = strings.TrimSpace(call.Name)
	call.Arguments = strings.TrimSpace(call.Arguments)
	if call.ID == "" && call.FailureCategory == "" {
		call.FailureCategory = "invalid_call_id"
	}
	if call.Name == "" && call.FailureCategory == "" {
		call.FailureCategory = "invalid_tool_name"
	}
	if call.Arguments == "" {
		call.Arguments = "{}"
	}
	if position, ok := state.blockPositions[index]; ok {
		input := map[string]any{}
		if err := json.Unmarshal([]byte(call.Arguments), &input); err != nil || input == nil {
			input = map[string]any{}
		}
		state.assistantBlocks[position]["input"] = input
	}
	state.completedTools[index] = true
	copy := *call
	return sendProviderEvent(ctx, events, ProviderEvent{
		Type: ProviderEventToolCallCompleted, ToolCall: &copy,
	})
}

func (state *anthropicStreamState) completeRound(
	ctx context.Context,
	events chan<- ProviderEvent,
) bool {
	for _, index := range state.toolOrder {
		if !state.stopContentBlock(ctx, events, index) {
			return false
		}
	}
	return sendProviderEvent(ctx, events, ProviderEvent{
		Type: ProviderEventRoundCompleted,
		RoundState: anthropicRoundState{
			AssistantBlocks: cloneAnthropicAssistantBlocks(state.assistantBlocks),
		},
	})
}

func anthropicToolInput(raw json.RawMessage) map[string]any {
	input := map[string]any{}
	if len(raw) == 0 || string(raw) == "null" {
		return input
	}
	if err := json.Unmarshal(raw, &input); err != nil || input == nil {
		return map[string]any{}
	}
	return input
}

var _ Provider = (*AnthropicProvider)(nil)
var _ ToolRoundProvider = (*AnthropicProvider)(nil)
var _ ToolPlanner = (*AnthropicProvider)(nil)
var _ ModelRefValidator = (*AnthropicProvider)(nil)
var _ ModelRefResolver = (*AnthropicProvider)(nil)
