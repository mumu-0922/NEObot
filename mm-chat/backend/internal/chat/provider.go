package chat

import (
	"context"
	"strings"

	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	ProviderEventDelta          = "delta"
	ProviderEventReasoningDelta = "reasoning.delta"
	ProviderEventUsage          = "usage"
	ProviderEventSearch         = "search"
)

type Provider interface {
	StreamChat(ctx context.Context, input ProviderRequest) (<-chan ProviderEvent, error)
}

type ModelBuiltInSearchProvider interface {
	Provider
	ModelBuiltInSearchID() websearch.ModelBuiltInProviderID
	StreamChatWithModelBuiltInSearch(
		context.Context,
		ProviderRequest,
	) (<-chan ProviderEvent, error)
}

type ToolPlanner interface {
	PlanTools(ctx context.Context, input ToolPlanRequest) ([]ToolCall, error)
}

type ModelRefValidator interface {
	ValidateModelRef(modelRef ModelRef) error
}

type ModelRefResolver interface {
	ResolveModelRef(modelRef ModelRef) (ModelRef, error)
}

type ProviderRequest struct {
	RunID              string
	ConversationID     string
	UserMessageID      string
	AssistantMessageID string
	Prompt             string
	SystemPrompt       string
	Messages           []ProviderMessage
	Attachments        []ProviderAttachment
	UseReasoning       bool
	ReasoningEffort    ReasoningEffort
	ModelRef           ModelRef
	Metadata           map[string]any
}

type ProviderMessage struct {
	MessageID   string `json:"-"`
	Role        string
	Content     string
	Attachments []ProviderAttachment
}

type ProviderAttachment struct {
	FileID   string
	FileName string
	MimeType string
	Size     int64
	SHA256   string
	Purpose  string
	Data     []byte
}

type ToolPlanRequest struct {
	Prompt   string
	ModelRef ModelRef
	Tools    []ToolDefinition
}

type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type ProviderEvent struct {
	Type           string
	Delta          string
	ReasoningDelta string
	Usage          *TokenUsage
	Search         *websearch.Result
	Error          error
}

type TokenUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

type MockProvider struct{}

func NewMockProvider() MockProvider {
	return MockProvider{}
}

func (p MockProvider) StreamChat(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	events := make(chan ProviderEvent)
	go func() {
		defer close(events)

		prompt := strings.TrimSpace(input.Prompt)
		chunks := []string{"Mock response: ", prompt}
		for _, chunk := range chunks {
			if chunk == "" {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case events <- ProviderEvent{Type: ProviderEventDelta, Delta: chunk}:
			}
		}

		usage := &TokenUsage{
			PromptTokens:     countMockTokens(prompt),
			CompletionTokens: countMockTokens(strings.Join(chunks, "")),
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		select {
		case <-ctx.Done():
			return
		case events <- ProviderEvent{Type: ProviderEventUsage, Usage: usage}:
		}
	}()

	return events, nil
}

func countMockTokens(value string) int {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0
	}

	return len(fields)
}
