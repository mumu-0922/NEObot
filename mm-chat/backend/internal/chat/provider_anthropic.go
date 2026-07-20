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
	modelRef, err := p.ResolveModelRef(input.ModelRef)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelRef.ModelID) == "" {
		return nil, errors.New("anthropic provider model is required")
	}
	messages, err := anthropicMessages(providerMessagesOrPrompt(input))
	if err != nil {
		return nil, err
	}
	request := anthropicMessagesRequest{
		Model:     modelRef.ModelID,
		MaxTokens: defaultAnthropicMaxTokens,
		Stream:    true,
		System:    strings.TrimSpace(input.SystemPrompt),
		Messages:  messages,
	}
	if input.UseReasoning {
		request.Thinking = &anthropicThinkingConfig{
			Type: "enabled", BudgetTokens: defaultAnthropicThinkingTokens,
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
}

type anthropicMessagesResponse struct {
	Content []struct {
		Type  string         `json:"type"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	} `json:"content"`
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
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"delta"`
	ContentBlock *struct {
		Type string `json:"type"`
		Text string `json:"text"`
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
}

func streamAnthropicEvents(ctx context.Context, reader io.Reader, events chan<- ProviderEvent) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	state := anthropicStreamState{}
	dataLines := make([]string, 0, 1)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			keepReading, done := dispatchAnthropicData(
				ctx, strings.Join(dataLines, "\n"), events, &state,
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
			ctx, strings.Join(dataLines, "\n"), events, &state,
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
		return false, true
	}
	var envelope anthropicStreamEnvelope
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		sendProviderEvent(ctx, events, ProviderEvent{Error: errAnthropicFrame})
		return false, false
	}
	switch envelope.Type {
	case "ping", "content_block_start", "content_block_stop":
		if envelope.Type == "content_block_start" && envelope.ContentBlock != nil &&
			envelope.ContentBlock.Type == "text" && envelope.ContentBlock.Text != "" {
			return sendProviderEvent(ctx, events, ProviderEvent{
				Type: ProviderEventDelta, Delta: envelope.ContentBlock.Text,
			}), false
		}
		return true, false
	case "message_start":
		if envelope.Message != nil {
			state.promptTokens = envelope.Message.Usage.InputTokens
			state.completionTokens = envelope.Message.Usage.OutputTokens
		}
		return true, false
	case "content_block_delta":
		if envelope.Delta.Type != "text_delta" || envelope.Delta.Text == "" {
			return true, false
		}
		return sendProviderEvent(ctx, events, ProviderEvent{
			Type: ProviderEventDelta, Delta: envelope.Delta.Text,
		}), false
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
		return false, true
	case "error":
		sendProviderEvent(ctx, events, ProviderEvent{Error: errAnthropicStream})
		return false, false
	default:
		return true, false
	}
}

var _ Provider = (*AnthropicProvider)(nil)
var _ ToolPlanner = (*AnthropicProvider)(nil)
var _ ModelRefValidator = (*AnthropicProvider)(nil)
var _ ModelRefResolver = (*AnthropicProvider)(nil)
