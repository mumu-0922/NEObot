package chat

import (
	"bufio"
	"bytes"
	"context"
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
)

var (
	errOpenAICompatibleFrame  = errors.New("openai-compatible provider stream parse failed")
	errOpenAICompatibleStream = errors.New("openai-compatible provider stream failed")
)

type OpenAICompatibleProviderConfig struct {
	BaseURL      string
	APIKey       string
	DefaultModel string
	Timeout      time.Duration
	HTTPClient   *http.Client
}

type OpenAICompatibleProvider struct {
	endpoint     string
	apiKey       string
	defaultModel string
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
		Model:    model,
		Stream:   true,
		Messages: openAICompatibleMessages(input.SystemPrompt, input.Prompt),
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
	Model    string                    `json:"model"`
	Stream   bool                      `json:"stream"`
	Messages []openAICompatibleMessage `json:"messages"`
}

type openAICompatibleMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

func openAICompatibleMessages(systemPrompt string, prompt string) []openAICompatibleMessage {
	messages := make([]openAICompatibleMessage, 0, 2)
	if systemPrompt = strings.TrimSpace(systemPrompt); systemPrompt != "" {
		messages = append(messages, openAICompatibleMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	messages = append(messages, openAICompatibleMessage{
		Role:    "user",
		Content: prompt,
	})

	return messages
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
