package chat

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ChatAgentEventTurnStarted      = "turn.started"
	ChatAgentEventTurnEnded        = "turn.ended"
	ChatAgentEventStepStarted      = "step.started"
	ChatAgentEventStepEnded        = "step.ended"
	ChatAgentEventAssistantMessage = "assistant.message"
	ChatAgentEventToolCalled       = "tool.called"
	ChatAgentEventToolResult       = "tool.result"
	ChatAgentEventGoalChanged      = "goal.changed"
	ChatAgentEventGoalRoundStarted = "goal.round.started"
	ChatAgentEventContextReplaced  = "context.replaced"

	ChatAgentTurnRunning     = "running"
	ChatAgentTurnCompleted   = "completed"
	ChatAgentTurnFailed      = "failed"
	ChatAgentTurnCancelled   = "cancelled"
	ChatAgentTurnInterrupted = "interrupted"

	maxChatAgentEventPayloadBytes = 256 * 1024
	maxChatAgentMessageEventBytes = 192 * 1024
)

var chatAgentEventTypes = map[string]struct{}{
	ChatAgentEventTurnStarted:      {},
	ChatAgentEventTurnEnded:        {},
	ChatAgentEventStepStarted:      {},
	ChatAgentEventStepEnded:        {},
	ChatAgentEventAssistantMessage: {},
	ChatAgentEventToolCalled:       {},
	ChatAgentEventToolResult:       {},
	ChatAgentEventGoalChanged:      {},
	ChatAgentEventGoalRoundStarted: {},
	ChatAgentEventContextReplaced:  {},
}

type ChatAgentEvent struct {
	EventID        string         `json:"eventId"`
	TurnID         string         `json:"turnId"`
	UserID         string         `json:"-"`
	ConversationID string         `json:"conversationId"`
	MessageID      string         `json:"messageId"`
	RunID          string         `json:"runId"`
	Sequence       int64          `json:"sequence"`
	Type           string         `json:"type"`
	StepSequence   int            `json:"stepSequence,omitempty"`
	Payload        map[string]any `json:"payload"`
	OccurredAt     time.Time      `json:"occurredAt"`
}

type StartChatAgentTurnInput struct {
	TurnID         string
	EventID        string
	ConversationID string
	MessageID      string
	RunID          string
	OccurredAt     time.Time
}

type AppendChatAgentEventInput struct {
	EventID      string
	Type         string
	StepSequence int
	Payload      map[string]any
	OccurredAt   time.Time
}

type ChatAgentEventRepository interface {
	StartChatAgentTurn(context.Context, StartChatAgentTurnInput) (ChatAgentEvent, error)
	AppendChatAgentEvent(context.Context, string, AppendChatAgentEventInput) (ChatAgentEvent, error)
	ListChatAgentEvents(context.Context, string) ([]ChatAgentEvent, error)
}

type ChatAgentEventLookupRepository interface {
	GetChatAgentEvent(context.Context, string) (ChatAgentEvent, error)
}

type ChatAgentTurnRecoveryRepository interface {
	RecoverIncompleteChatAgentTurns(context.Context, time.Time) (int, error)
}

func (s *Service) StartChatAgentTurn(
	ctx context.Context,
	input StartChatAgentTurnInput,
) (ChatAgentEvent, error) {
	repository, err := s.chatAgentEventRepository()
	if err != nil {
		return ChatAgentEvent{}, err
	}
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.MessageID = strings.TrimSpace(input.MessageID)
	input.RunID = strings.TrimSpace(input.RunID)
	if !isUUID(input.TurnID) || !isUUID(input.EventID) ||
		!isUUID(input.ConversationID) || !isUUID(input.MessageID) ||
		!isUUID(input.RunID) || input.OccurredAt.IsZero() {
		return ChatAgentEvent{}, newValidationError(
			"INVALID_CHAT_AGENT_TURN", "chat Agent turn identifiers and occurredAt are required",
		)
	}
	return repository.StartChatAgentTurn(ctx, input)
}

func (s *Service) AppendChatAgentEvent(
	ctx context.Context,
	turnID string,
	input AppendChatAgentEventInput,
) (ChatAgentEvent, error) {
	repository, err := s.chatAgentEventRepository()
	if err != nil {
		return ChatAgentEvent{}, err
	}
	turnID = strings.TrimSpace(turnID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.Type = strings.TrimSpace(input.Type)
	if !isUUID(turnID) || !isUUID(input.EventID) || input.OccurredAt.IsZero() {
		return ChatAgentEvent{}, newValidationError(
			"INVALID_CHAT_AGENT_EVENT", "chat Agent event identifiers and occurredAt are required",
		)
	}
	if _, ok := chatAgentEventTypes[input.Type]; !ok || input.Type == ChatAgentEventTurnStarted {
		return ChatAgentEvent{}, newValidationError(
			"INVALID_CHAT_AGENT_EVENT", "chat Agent event type is invalid",
		)
	}
	if input.StepSequence < 0 {
		return ChatAgentEvent{}, newValidationError(
			"INVALID_CHAT_AGENT_EVENT", "chat Agent event step sequence is invalid",
		)
	}
	input.Payload, err = normalizeChatAgentEventPayload(input.Type, input.Payload)
	if err != nil {
		return ChatAgentEvent{}, err
	}
	return repository.AppendChatAgentEvent(ctx, turnID, input)
}

func (s *Service) chatAgentEventRepository() (ChatAgentEventRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repository, ok := s.repo.(ChatAgentEventRepository)
	if !ok {
		return nil, ErrDatabaseRequired
	}
	return repository, nil
}

func (s *Service) GetChatAgentEvent(
	ctx context.Context,
	eventID string,
) (ChatAgentEvent, error) {
	if err := s.requireRepository(); err != nil {
		return ChatAgentEvent{}, err
	}
	eventID = strings.TrimSpace(eventID)
	if !isUUID(eventID) {
		return ChatAgentEvent{}, newValidationError(
			"INVALID_CHAT_AGENT_EVENT_ID", "chat Agent event id must be a UUID",
		)
	}
	repository, ok := s.repo.(ChatAgentEventLookupRepository)
	if !ok {
		return ChatAgentEvent{}, ErrDatabaseRequired
	}
	return repository.GetChatAgentEvent(ctx, eventID)
}

func normalizeChatAgentEventPayload(
	eventType string,
	payload map[string]any,
) (map[string]any, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	if eventType == ChatAgentEventContextReplaced {
		var err error
		payload, err = normalizeChatAgentContextReplacementPayload(payload)
		if err != nil {
			return nil, err
		}
	}
	if eventType == ChatAgentEventAssistantMessage {
		bounded := cloneJSONObject(payload)
		if content, ok := bounded["content"].(string); ok {
			bounded["content"] = truncateChatAgentUTF8(content, maxChatAgentMessageEventBytes)
		}
		payload = bounded
	}
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > maxChatAgentEventPayloadBytes {
		return nil, newValidationError(
			"INVALID_CHAT_AGENT_EVENT_PAYLOAD", "chat Agent event payload is invalid or too large",
		)
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil || normalized == nil {
		return nil, newValidationError(
			"INVALID_CHAT_AGENT_EVENT_PAYLOAD", "chat Agent event payload must be an object",
		)
	}
	encoded, err = json.Marshal(normalized)
	if err != nil || len(encoded) > maxChatAgentEventPayloadBytes {
		return nil, newValidationError(
			"INVALID_CHAT_AGENT_EVENT_PAYLOAD", "chat Agent event payload is invalid or too large",
		)
	}
	return normalized, nil
}

func normalizeChatAgentContextReplacementPayload(
	payload map[string]any,
) (map[string]any, error) {
	if len(payload) != 5 {
		return nil, newValidationError(
			"INVALID_CHAT_AGENT_EVENT_PAYLOAD", "context replacement payload is invalid",
		)
	}
	reason := chatAgentPayloadString(payload, "reason")
	switch reason {
	case "tool_result_pruning", "turn_summary_compaction", "provider_context_overflow":
	default:
		return nil, newValidationError(
			"INVALID_CHAT_AGENT_EVENT_PAYLOAD", "context replacement reason is invalid",
		)
	}
	before, beforeOK := exactNonnegativeChatAgentPayloadInt(payload["beforeBytes"])
	after, afterOK := exactNonnegativeChatAgentPayloadInt(payload["afterBytes"])
	pruned, prunedOK := exactNonnegativeChatAgentPayloadInt(payload["resultsPruned"])
	replaced, replacedOK := exactNonnegativeChatAgentPayloadInt(payload["exchangesReplaced"])
	if !beforeOK || !afterOK || !prunedOK || !replacedOK || before <= after ||
		(pruned == 0 && replaced == 0) {
		return nil, newValidationError(
			"INVALID_CHAT_AGENT_EVENT_PAYLOAD", "context replacement counts are invalid",
		)
	}
	return map[string]any{
		"reason": reason, "beforeBytes": before, "afterBytes": after,
		"resultsPruned": pruned, "exchangesReplaced": replaced,
	}, nil
}

func exactNonnegativeChatAgentPayloadInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed >= 0
	case int64:
		if typed < 0 || int64(int(typed)) != typed {
			return 0, false
		}
		return int(typed), true
	case float64:
		converted := int(typed)
		if typed < 0 || float64(converted) != typed {
			return 0, false
		}
		return converted, true
	default:
		return 0, false
	}
}

func chatAgentProcessStepPayload(step ProcessStep) map[string]any {
	return map[string]any{"processStep": cloneProcessStep(step)}
}

func chatAgentToolEventPayload(
	execution *ProviderToolExecutionEvent,
	processSteps []ProcessStep,
) map[string]any {
	if execution == nil {
		return map[string]any{}
	}
	toolCall := map[string]any{
		"executionId":    truncateChatAgentUTF8(execution.ExecutionID, 256),
		"toolName":       truncateChatAgentUTF8(execution.Name, maxToolNameBytes),
		"processStatus":  normalizeProcessStepStatus(execution.Status),
		"round":          max(execution.Round, 0),
		"mode":           truncateChatAgentUTF8(execution.Mode, 64),
		"classification": truncateChatAgentUTF8(execution.Classification, 64),
	}
	if callID := strings.TrimSpace(execution.CallID); callID != "" {
		toolCall["callId"] = truncateChatAgentUTF8(redactProcessSecrets(callID), 256)
	}
	if retryOf := strings.TrimSpace(execution.RetryOf); retryOf != "" {
		toolCall["retryOf"] = truncateChatAgentUTF8(redactProcessSecrets(retryOf), 256)
	}
	if callStatus := strings.TrimSpace(execution.CallStatus); callStatus != "" {
		toolCall["status"] = truncateChatAgentUTF8(callStatus, 64)
	}
	if serverName := strings.TrimSpace(execution.ServerName); serverName != "" {
		toolCall["serverName"] = truncateChatAgentUTF8(redactProcessSecrets(serverName), 256)
	}
	if failure := strings.TrimSpace(execution.FailureCategory); failure != "" {
		toolCall["failureCategory"] = truncateChatAgentUTF8(redactProcessSecrets(failure), 256)
	}
	if execution.DurationMillis > 0 {
		toolCall["durationMillis"] = execution.DurationMillis
	}
	if durability := strings.TrimSpace(execution.Durability); durability == "process_local" {
		toolCall["durability"] = durability
	}
	payload := map[string]any{"toolCall": toolCall}
	if len(processSteps) > 0 {
		steps := make([]ProcessStep, 0, len(processSteps))
		for _, step := range processSteps {
			steps = append(steps, cloneProcessStep(step))
		}
		payload["processSteps"] = steps
	}
	return payload
}

func projectChatAgentToolExecution(event ChatAgentEvent) *ProviderToolExecutionEvent {
	toolCall, ok := event.Payload["toolCall"].(map[string]any)
	if !ok {
		return nil
	}
	executionID := chatAgentPayloadString(toolCall, "executionId")
	toolName := chatAgentPayloadString(toolCall, "toolName")
	processStatus := normalizeProcessStepStatus(
		chatAgentPayloadString(toolCall, "processStatus"),
	)
	mode := chatAgentPayloadString(toolCall, "mode")
	if executionID == "" || toolName == "" || processStatus == "" || mode == "" {
		return nil
	}
	return &ProviderToolExecutionEvent{
		ExecutionID:     executionID,
		CallID:          chatAgentPayloadString(toolCall, "callId"),
		RetryOf:         chatAgentPayloadString(toolCall, "retryOf"),
		Name:            toolName,
		ServerName:      chatAgentPayloadString(toolCall, "serverName"),
		Classification:  chatAgentPayloadString(toolCall, "classification"),
		Status:          processStatus,
		CallStatus:      chatAgentPayloadString(toolCall, "status"),
		Round:           chatAgentPayloadInt(toolCall, "round"),
		FailureCategory: chatAgentPayloadString(toolCall, "failureCategory"),
		DurationMillis:  int64(chatAgentPayloadInt(toolCall, "durationMillis")),
		Mode:            mode,
		Durability:      chatAgentPayloadString(toolCall, "durability"),
	}
}

func projectChatAgentProcessTrace(
	events []ChatAgentEvent,
	legacy []ProcessStep,
) []ProcessStep {
	if len(events) == 0 {
		return cloneProcessSteps(legacy)
	}
	ordered := append([]ChatAgentEvent(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Sequence != ordered[j].Sequence {
			return ordered[i].Sequence < ordered[j].Sequence
		}
		return ordered[i].EventID < ordered[j].EventID
	})
	messageID := ordered[0].MessageID
	trace := newProcessTrace(messageID)
	interruptedAt := time.Time{}
	for _, event := range ordered {
		if step, ok := processStepFromChatAgentPayload(event.Payload["processStep"]); ok {
			trace.add(step)
		}
		if rawSteps, ok := event.Payload["processSteps"].([]any); ok {
			for _, rawStep := range rawSteps {
				if step, valid := processStepFromChatAgentPayload(rawStep); valid {
					trace.add(step)
				}
			}
		}
		if event.Type == ChatAgentEventTurnEnded &&
			chatAgentPayloadString(event.Payload, "status") == ChatAgentTurnInterrupted {
			interruptedAt = event.OccurredAt
		}
	}
	if !interruptedAt.IsZero() {
		for _, step := range trace.snapshot() {
			if reconciled, ok := interruptProcessLocalJobStep(step, interruptedAt); ok {
				trace.add(reconciled)
				continue
			}
			if !isActiveProcessStepStatus(step.Status) {
				continue
			}
			detail := cloneProcessDetail(step.Detail)
			if detail == nil {
				detail = map[string]any{}
			}
			detail["failureCategory"] = ChatAgentTurnInterrupted
			detail["outcome"] = ChatAgentTurnInterrupted
			trace.transitionID(step.ID, ProcessStepStatusInterrupted, interruptedAt, detail)
		}
	}
	projected := trace.snapshot()
	if len(projected) == 0 {
		return cloneProcessSteps(legacy)
	}
	return projected
}

func processStepsFromChatAgentEvent(event ChatAgentEvent) []ProcessStep {
	rawSteps, ok := event.Payload["processSteps"].([]any)
	if !ok {
		return nil
	}
	steps := make([]ProcessStep, 0, len(rawSteps))
	for _, rawStep := range rawSteps {
		if step, valid := processStepFromChatAgentPayload(rawStep); valid {
			steps = append(steps, step)
		}
	}
	return steps
}

func interruptProcessLocalJobStep(step ProcessStep, interruptedAt time.Time) (ProcessStep, bool) {
	if step.Presentation == nil || step.Presentation.Card != "job" ||
		processDetailString(step.Detail, "durability") != "process_local" {
		return ProcessStep{}, false
	}
	switch step.Presentation.JobStatus {
	case "running", "stopping":
	default:
		return ProcessStep{}, false
	}
	reconciled := cloneProcessStep(step)
	reconciled.Status = ProcessStepStatusInterrupted
	reconciled.CompletedAt = formatTime(interruptedAt)
	reconciled.DurationMS = processStepDurationMillis(reconciled.StartedAt, interruptedAt)
	reconciled.Detail = cloneProcessDetail(reconciled.Detail)
	if reconciled.Detail == nil {
		reconciled.Detail = map[string]any{}
	}
	reconciled.Detail["failureCategory"] = ChatAgentTurnInterrupted
	reconciled.Detail["outcome"] = ChatAgentTurnInterrupted
	reconciled.Presentation.JobStatus = ChatAgentTurnInterrupted
	return reconciled, true
}

func processStepFromChatAgentPayload(value any) (ProcessStep, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProcessStep{}, false
	}
	var step ProcessStep
	if err := json.Unmarshal(encoded, &step); err != nil {
		return ProcessStep{}, false
	}
	trace := newProcessTrace(strings.SplitN(step.ID, ":", 2)[0])
	normalized := trace.add(step)
	return normalized, normalized.ID != ""
}

func cloneProcessSteps(steps []ProcessStep) []ProcessStep {
	if len(steps) == 0 {
		return nil
	}
	cloned := make([]ProcessStep, 0, len(steps))
	for _, step := range steps {
		cloned = append(cloned, cloneProcessStep(step))
	}
	return cloned
}

func isActiveProcessStepStatus(status string) bool {
	switch status {
	case ProcessStepStatusPending, ProcessStepStatusRunning, ProcessStepStatusAwaitingApproval:
		return true
	default:
		return false
	}
}

func chatAgentPayloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func chatAgentPayloadInt(payload map[string]any, key string) int {
	switch value := payload[key].(type) {
	case int:
		return max(value, 0)
	case int64:
		return max(int(value), 0)
	case float64:
		if value >= 0 {
			return int(value)
		}
	}
	return 0
}

func truncateChatAgentUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for value != "" && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func isChatAgentTurnTerminalError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "CHAT_AGENT_TURN_TERMINAL")
}

var errChatAgentEventPersistence = errors.New("chat Agent event persistence failed")
