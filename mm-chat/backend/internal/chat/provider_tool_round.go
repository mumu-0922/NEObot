package chat

import (
	"context"
	"strings"

	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	ProviderEventToolCallDelta     = "tool.call.delta"
	ProviderEventToolCallCompleted = "tool.call.completed"
	ProviderEventToolExecution     = "tool.execution"

	ProviderToolChoiceAuto     = "auto"
	ProviderToolChoiceRequired = "required"
)

type ToolRoundProvider interface {
	Provider
	StreamToolRound(
		context.Context,
		ProviderRoundRequest,
	) (<-chan ProviderEvent, error)
}

type ProviderRoundRequest struct {
	ProviderRequest
	Tools        []ToolDefinition
	ToolChoice   string
	Continuation []ProviderToolExchange
}

type ProviderToolExchange struct {
	AssistantContent   string
	AssistantReasoning string
	Calls              []ProviderToolCall
	Results            []ProviderToolResult
}

type ProviderToolCallDelta struct {
	ChoiceIndex    int
	CallIndex      int
	ID             string
	NameDelta      string
	ArgumentsDelta string
}

type ProviderToolCall struct {
	ChoiceIndex     int
	CallIndex       int
	ID              string
	Name            string
	Arguments       string
	FailureCategory string
}

type ProviderToolResult struct {
	CallID  string
	Name    string
	Content string
}

type ProviderToolExecutionEvent struct {
	ExecutionID     string
	CallID          string
	Name            string
	Status          string
	Round           int
	Arguments       map[string]any
	Query           string
	Search          *websearch.Result
	CitationMarkers []string
	FailureCategory string
	Mode            string
}

func normalizeProviderToolChoice(value string) string {
	switch strings.TrimSpace(value) {
	case ProviderToolChoiceRequired:
		return ProviderToolChoiceRequired
	default:
		return ProviderToolChoiceAuto
	}
}
