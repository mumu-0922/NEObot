package memoryworker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
)

const (
	memoryExtractionToolName = "propose_memory_candidates"
	memoryDecisionToolName   = "propose_memory_candidate_decisions"
)

var errExtractionInvalid = errors.New("memory extraction protocol is invalid")

type classifiedExtractionError struct {
	category string
}

func (failure classifiedExtractionError) Error() string {
	return errExtractionInvalid.Error()
}

func (failure classifiedExtractionError) Unwrap() error {
	return errExtractionInvalid
}

func memoryExtractionToolDefinition(
	sourceMessageID string,
	messages []providerMessage,
) chat.ToolDefinition {
	nullableString := []any{"string", "null"}
	stringArray := func(maxItems int) map[string]any {
		return map[string]any{
			"type": "array", "maxItems": maxItems,
			"items": map[string]any{"type": "string"},
		}
	}
	userMessageIDs := make([]string, 0, len(messages))
	assistantMessageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "user":
			userMessageIDs = append(userMessageIDs, message.ID)
		case "assistant":
			assistantMessageIDs = append(assistantMessageIDs, message.ID)
		}
	}
	authorityIDs := stringArray(len(userMessageIDs))
	authorityIDs["description"] = "Use only supplied user-role message IDs and " +
		"always include sourceMessageId " + sourceMessageID + "."
	authorityIDs["items"] = map[string]any{
		"type": "string", "enum": userMessageIDs,
	}
	contextIDs := stringArray(len(assistantMessageIDs))
	contextIDs["description"] = "Use only supplied assistant-role message IDs; " +
		"use [] when assistant context is unnecessary."
	if len(assistantMessageIDs) > 0 {
		contextIDs["items"] = map[string]any{
			"type": "string", "enum": assistantMessageIDs,
		}
	}
	confirmationKinds := []string{"explicit_user"}
	if len(assistantMessageIDs) > 0 {
		confirmationKinds = append(confirmationKinds, "confirmed_assistant")
	}
	candidateProperties := map[string]any{
		"type": map[string]any{
			"type": "string", "enum": []string{
				"fact", "preference", "instruction", "project",
				"warning", "decision", "context",
			},
		},
		"content":                 map[string]any{"type": "string", "maxLength": 2000},
		"importance":              map[string]any{"type": "integer", "minimum": 1, "maximum": 5},
		"confidence":              map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"tags":                    stringArray(12),
		"subjectKey":              map[string]any{"type": nullableString, "maxLength": 256},
		"factKey":                 map[string]any{"type": nullableString, "maxLength": 256},
		"sensitivity":             map[string]any{"type": "string", "enum": []string{"normal", "sensitive", "secret"}},
		"authorityUserMessageIds": authorityIDs,
		"contextMessageIds":       contextIDs,
		"confirmationKind": map[string]any{
			"type": "string", "enum": confirmationKinds,
		},
		"proposedScopeType": map[string]any{
			"type": "string", "enum": []string{"global", "project", "conversation"},
		},
		"scopeConfidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"temporalBasis": map[string]any{
			"type": "string", "enum": []string{
				"none", "source_timestamp", "explicit_absolute",
				"relative_ambiguous", "model_inferred",
			},
		},
		"validFrom":     map[string]any{"type": nullableString},
		"validTo":       map[string]any{"type": nullableString},
		"factExpiresAt": map[string]any{"type": nullableString},
	}
	return chat.ToolDefinition{
		Type: "function",
		Function: chat.ToolFunctionDefinition{
			Name:   memoryExtractionToolName,
			Strict: true,
			Description: "Submit zero to five durable Memory candidates extracted from " +
				"the supplied untrusted conversation data.",
			Parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"memories"},
				"properties": map[string]any{
					"memories": map[string]any{
						"type": "array", "maxItems": 5,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             rawCaptureCandidateKeys[:],
							"properties":           candidateProperties,
						},
					},
				},
			},
		},
	}
}

func memoryDecisionToolDefinition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Type: "function",
		Function: chat.ToolFunctionDefinition{
			Name:   memoryDecisionToolName,
			Strict: true,
			Description: "Submit one bounded conflict decision for every supplied " +
				"Memory candidate ordinal.",
			Parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"decisions"},
				"properties": map[string]any{
					"decisions": map[string]any{
						"type": "array", "maxItems": 5,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required": []string{
								"ordinal", "action", "targetMemoryIds",
							},
							"properties": map[string]any{
								"ordinal": map[string]any{
									"type": "integer", "minimum": 1, "maximum": 5,
								},
								"action": map[string]any{
									"type": "string", "enum": []string{
										"ADD", "NOOP", "MERGE", "SUPERSEDE", "REJECT",
									},
								},
								"targetMemoryIds": map[string]any{
									"type": "array", "maxItems": 5,
									"items": map[string]any{"type": "string"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func streamRequiredToolArguments(
	ctx context.Context,
	provider chat.ToolRoundProvider,
	toolName string,
	request chat.ProviderRoundRequest,
) ([]byte, error) {
	events, err := provider.StreamToolRound(ctx, request)
	if err != nil {
		return nil, err
	}
	if events == nil {
		return nil, fmt.Errorf("%w: nil Tool Round stream", errExtractionInvalid)
	}
	calls := make([]chat.ProviderToolCall, 0, 1)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case event, ok := <-events:
			if !ok {
				if len(calls) != 1 {
					return nil, fmt.Errorf("%w: Tool Call count", errExtractionInvalid)
				}
				call := calls[0]
				if strings.TrimSpace(call.ID) == "" ||
					call.SyntheticID ||
					call.Name != toolName || call.FailureCategory != "" ||
					len(call.Arguments) == 0 || len(call.Arguments) > memoryExtractionOutputBytes {
					return nil, fmt.Errorf("%w: completed Tool Call", errExtractionInvalid)
				}
				return []byte(call.Arguments), nil
			}
			if event.Error != nil {
				return nil, event.Error
			}
			switch event.Type {
			case chat.ProviderEventToolCallCompleted:
				if event.ToolCall == nil || len(calls) == 1 {
					return nil, fmt.Errorf("%w: completed Tool Call batch", errExtractionInvalid)
				}
				calls = append(calls, *event.ToolCall)
			case chat.ProviderEventToolCallDelta,
				chat.ProviderEventRoundCompleted,
				chat.ProviderEventUsage,
				chat.ProviderEventDelta,
				chat.ProviderEventReasoningDelta:
				// Transport-only events carry no candidate authority.
			default:
				return nil, fmt.Errorf("%w: unexpected Tool Round event", errExtractionInvalid)
			}
		}
	}
}

func extractionRetryExhausted(job Job) bool {
	// The first invalid response plus two bounded retries is the complete
	// protocol-error budget. Provider/transport failures retain the job's
	// separately configured retry authority.
	return job.AttemptCount >= 3
}

func extractionFailureCategory(err error) string {
	var classified classifiedExtractionError
	if errors.As(err, &classified) && classified.category != "" {
		return classified.category
	}
	value := strings.ToLower(err.Error())
	switch {
	case strings.Contains(value, "decision tool arguments"):
		return "DECISION_ARGUMENTS_INVALID"
	case strings.Contains(value, "decode tool arguments"):
		return "CANDIDATE_ARGUMENTS_INVALID"
	case strings.Contains(value, "decision count"):
		return "DECISION_COUNT_INVALID"
	case strings.Contains(value, "decision ordinal"):
		return "DECISION_ORDINAL_INVALID"
	case strings.Contains(value, "decision action"):
		return "DECISION_ACTION_INVALID"
	case strings.Contains(value, "decision target"):
		return "DECISION_TARGET_INVALID"
	case strings.Contains(value, "candidate count"):
		return "CANDIDATE_COUNT_INVALID"
	case strings.Contains(value, "proposal validation"):
		return "PROPOSAL_VALIDATION_INVALID"
	case strings.Contains(value, "tool round is unsupported"):
		return "TOOL_ROUND_UNSUPPORTED"
	case strings.Contains(value, "nil tool round stream"):
		return "TOOL_STREAM_NIL"
	case strings.Contains(value, "tool call count"):
		return "TOOL_CALL_COUNT_INVALID"
	case strings.Contains(value, "completed tool call"):
		return "TOOL_CALL_INVALID"
	case strings.Contains(value, "unexpected tool round event"):
		return "TOOL_EVENT_INVALID"
	default:
		return "EXTRACTION_PROTOCOL_INVALID"
	}
}
