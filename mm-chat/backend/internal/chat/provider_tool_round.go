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
	ProviderEventRoundCompleted    = "round.completed"

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
	ProviderState      any
	FollowupPrompt     string
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
	SyntheticID     bool
	Name            string
	Arguments       string
	FailureCategory string
}

type ProviderToolResult struct {
	CallID  string
	Name    string
	Content string
	IsError bool
}

type ProviderToolExecutionEvent struct {
	ExecutionID     string            `json:"executionId"`
	CallID          string            `json:"callId,omitempty"`
	Name            string            `json:"toolName"`
	Server          string            `json:"server,omitempty"`
	ServerName      string            `json:"serverName,omitempty"`
	Classification  string            `json:"classification,omitempty"`
	Status          string            `json:"processStatus"`
	CallStatus      string            `json:"status,omitempty"`
	Round           int               `json:"round"`
	Arguments       map[string]any    `json:"argumentsSummary,omitempty"`
	Query           string            `json:"-"`
	Search          *websearch.Result `json:"-"`
	Knowledge       *autoRAGDecision  `json:"-"`
	CitationMarkers []string          `json:"-"`
	FailureCategory string            `json:"failureCategory,omitempty"`
	DurationMillis  int64             `json:"durationMillis,omitempty"`
	Mode            string            `json:"mode"`
}

func normalizeProviderToolChoice(value string) string {
	switch strings.TrimSpace(value) {
	case ProviderToolChoiceRequired:
		return ProviderToolChoiceRequired
	default:
		return ProviderToolChoiceAuto
	}
}

func nonEmptyAgentContinuationContent(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "Continuing the task."
}
