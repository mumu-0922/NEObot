package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type chatAgentEventRecorder struct {
	service      *Service
	turnID       string
	startedEvent ChatAgentEvent
	mu           sync.Mutex
	ended        bool
}

func startChatAgentEventRecorder(
	ctx context.Context,
	service *Service,
	conversationID string,
	messageID string,
	runID string,
	at time.Time,
) (*chatAgentEventRecorder, error) {
	turnID, err := NewUUID()
	if err != nil {
		return nil, fmt.Errorf("%w: create turn id: %v", errChatAgentEventPersistence, err)
	}
	eventID, err := NewUUID()
	if err != nil {
		return nil, fmt.Errorf("%w: create event id: %v", errChatAgentEventPersistence, err)
	}
	startedEvent, err := service.StartChatAgentTurn(ctx, StartChatAgentTurnInput{
		TurnID: turnID, EventID: eventID, ConversationID: conversationID,
		MessageID: messageID, RunID: runID, OccurredAt: at.UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errChatAgentEventPersistence, err)
	}
	return &chatAgentEventRecorder{
		service: service, turnID: turnID, startedEvent: startedEvent,
	}, nil
}

func (recorder *chatAgentEventRecorder) append(
	ctx context.Context,
	eventType string,
	stepSequence int,
	payload map[string]any,
	at time.Time,
) (ChatAgentEvent, error) {
	if recorder == nil || recorder.service == nil || recorder.turnID == "" {
		return ChatAgentEvent{}, errChatAgentEventPersistence
	}
	eventID, err := NewUUID()
	if err != nil {
		return ChatAgentEvent{}, fmt.Errorf("%w: create event id: %v", errChatAgentEventPersistence, err)
	}
	return recorder.appendWithEventID(ctx, eventID, eventType, stepSequence, payload, at)
}

func (recorder *chatAgentEventRecorder) appendWithEventID(
	ctx context.Context,
	eventID string,
	eventType string,
	stepSequence int,
	payload map[string]any,
	at time.Time,
) (ChatAgentEvent, error) {
	if recorder == nil || recorder.service == nil || recorder.turnID == "" {
		return ChatAgentEvent{}, errChatAgentEventPersistence
	}
	event, err := recorder.service.AppendChatAgentEvent(ctx, recorder.turnID, AppendChatAgentEventInput{
		EventID: eventID, Type: eventType, StepSequence: stepSequence,
		Payload: payload, OccurredAt: at.UTC(),
	})
	if err != nil {
		return ChatAgentEvent{}, fmt.Errorf("%w: %v", errChatAgentEventPersistence, err)
	}
	return event, nil
}

func (recorder *chatAgentEventRecorder) recordProcessStep(
	ctx context.Context,
	step ProcessStep,
	at time.Time,
) (ChatAgentEvent, ProcessStep, error) {
	eventType := ChatAgentEventStepStarted
	if isTerminalProcessStepStatus(step.Status) {
		eventType = ChatAgentEventStepEnded
	}
	stepSequence := 0
	if round, ok := step.Detail["round"].(int); ok && round > 0 {
		stepSequence = round
	} else if round, ok := step.Detail["round"].(float64); ok && round >= 1 {
		stepSequence = int(round)
	}
	event, err := recorder.append(
		ctx, eventType, stepSequence, chatAgentProcessStepPayload(step), at,
	)
	if err != nil {
		return ChatAgentEvent{}, ProcessStep{}, err
	}
	projected, ok := processStepFromChatAgentPayload(event.Payload["processStep"])
	if !ok {
		return ChatAgentEvent{}, ProcessStep{}, fmt.Errorf(
			"%w: process projection invalid", errChatAgentEventPersistence,
		)
	}
	return event, projected, nil
}

func (recorder *chatAgentEventRecorder) recordToolExecution(
	ctx context.Context,
	execution *ProviderToolExecutionEvent,
	processSteps []ProcessStep,
	at time.Time,
) (ChatAgentEvent, error) {
	if execution == nil {
		return ChatAgentEvent{}, fmt.Errorf("%w: Tool event missing", errChatAgentEventPersistence)
	}
	eventID, err := NewUUID()
	if err != nil {
		return ChatAgentEvent{}, fmt.Errorf("%w: create event id: %v", errChatAgentEventPersistence, err)
	}
	eventType := ChatAgentEventToolCalled
	if isTerminalProcessStepStatus(execution.Status) {
		eventType = ChatAgentEventToolResult
	}
	steps := cloneProcessSteps(processSteps)
	if chatAgentToolRetryEligible(execution) {
		retryOf := truncateChatAgentUTF8(redactProcessSecrets(execution.CallID), 256)
		for index := range steps {
			if steps[index].Presentation == nil ||
				steps[index].Presentation.Card != "file" ||
				steps[index].Presentation.Operation != "read" {
				continue
			}
			steps[index].Presentation.Retry = &ProcessRetryPresentation{
				EventID: eventID, RetryOf: retryOf,
			}
		}
	}
	return recorder.appendWithEventID(
		ctx,
		eventID,
		eventType,
		max(execution.Round, 0),
		chatAgentToolEventPayload(execution, steps),
		at,
	)
}

func chatAgentToolRetryEligible(execution *ProviderToolExecutionEvent) bool {
	if execution == nil || execution.Status != ProcessStepStatusFailed ||
		execution.Mode != "local_direct" || !isLocalFileReadToolName(execution.Name) ||
		strings.TrimSpace(execution.CallID) == "" || execution.Presentation == nil ||
		execution.Presentation.Card != "file" || execution.Presentation.Operation != "read" ||
		strings.TrimSpace(execution.Presentation.Path) == "" {
		return false
	}
	switch strings.TrimSpace(execution.FailureCategory) {
	case "file_not_found", "execution_failed":
		return true
	default:
		return false
	}
}

func (recorder *chatAgentEventRecorder) recordContextReplacement(
	ctx context.Context,
	replacement *ProviderContextReplacementEvent,
	at time.Time,
) (ChatAgentEvent, error) {
	if replacement == nil || replacement.BeforeBytes <= replacement.AfterBytes ||
		replacement.AfterBytes < 0 || replacement.ResultsPruned < 0 ||
		replacement.ExchangesReplaced < 0 {
		return ChatAgentEvent{}, fmt.Errorf(
			"%w: context replacement invalid", errChatAgentEventPersistence,
		)
	}
	return recorder.append(ctx, ChatAgentEventContextReplaced, 0, map[string]any{
		"reason":      strings.TrimSpace(replacement.Reason),
		"beforeBytes": replacement.BeforeBytes, "afterBytes": replacement.AfterBytes,
		"resultsPruned":     replacement.ResultsPruned,
		"exchangesReplaced": replacement.ExchangesReplaced,
	}, at)
}

func (recorder *chatAgentEventRecorder) recordContextInjection(
	ctx context.Context,
	source string,
	label string,
	content string,
	at time.Time,
) (ChatAgentEvent, error) {
	return recorder.append(ctx, ChatAgentEventContextInjected, 0, map[string]any{
		"source": source, "label": label, "content": content,
	}, at)
}

func (recorder *chatAgentEventRecorder) recordAssistantBlockStart(
	ctx context.Context,
	blockIndex int,
	stepSequence int,
	at time.Time,
) (ChatAgentEvent, error) {
	return recorder.append(ctx, ChatAgentEventAssistantChunk, stepSequence, map[string]any{
		"chunkType": "block-start", "blockType": "reasoning", "blockIndex": blockIndex,
	}, at)
}

func (recorder *chatAgentEventRecorder) recordAssistantReasoningDelta(
	ctx context.Context,
	blockIndex int,
	stepSequence int,
	content string,
	at time.Time,
) (ChatAgentEvent, error) {
	return recorder.append(ctx, ChatAgentEventAssistantChunk, stepSequence, map[string]any{
		"chunkType": "reasoning-delta", "blockType": "reasoning",
		"blockIndex": blockIndex, "content": content,
	}, at)
}

func (recorder *chatAgentEventRecorder) recordAssistantBlockCompleted(
	ctx context.Context,
	blockIndex int,
	stepSequence int,
	at time.Time,
) (ChatAgentEvent, error) {
	return recorder.append(ctx, ChatAgentEventBlockCompleted, stepSequence, map[string]any{
		"blockType": "reasoning", "blockIndex": blockIndex,
	}, at)
}

func (recorder *chatAgentEventRecorder) recordAssistantNarrationBlockStart(
	ctx context.Context,
	blockIndex int,
	stepSequence int,
	at time.Time,
) (ChatAgentEvent, error) {
	return recorder.append(ctx, ChatAgentEventAssistantChunk, stepSequence, map[string]any{
		"chunkType": "block-start", "blockType": "narration", "blockIndex": blockIndex,
	}, at)
}

func (recorder *chatAgentEventRecorder) recordAssistantNarrationDelta(
	ctx context.Context,
	blockIndex int,
	stepSequence int,
	content string,
	at time.Time,
) (ChatAgentEvent, error) {
	return recorder.append(ctx, ChatAgentEventAssistantChunk, stepSequence, map[string]any{
		"chunkType": "narration-delta", "blockType": "narration",
		"blockIndex": blockIndex, "content": content,
	}, at)
}

func (recorder *chatAgentEventRecorder) recordAssistantNarrationBlockCompleted(
	ctx context.Context,
	blockIndex int,
	stepSequence int,
	at time.Time,
) (ChatAgentEvent, error) {
	return recorder.append(ctx, ChatAgentEventBlockCompleted, stepSequence, map[string]any{
		"blockType": "narration", "blockIndex": blockIndex,
	}, at)
}

func (recorder *chatAgentEventRecorder) finish(
	ctx context.Context,
	status string,
	content string,
	errorCode string,
	at time.Time,
) ([]ChatAgentEvent, error) {
	if recorder == nil {
		return nil, nil
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.ended {
		return nil, nil
	}
	status = normalizeChatAgentTurnStatus(status)
	messagePayload := map[string]any{
		"status":  status,
		"content": content,
	}
	if errorCode = strings.TrimSpace(errorCode); errorCode != "" {
		messagePayload["errorCode"] = truncateChatAgentUTF8(
			redactProcessSecrets(errorCode), 256,
		)
	}
	messageEvent, messageErr := recorder.append(
		ctx, ChatAgentEventAssistantMessage, 0, messagePayload, at,
	)
	endEvent, endErr := recorder.append(
		ctx,
		ChatAgentEventTurnEnded,
		0,
		map[string]any{"status": status},
		at,
	)
	if endErr == nil {
		recorder.ended = true
	}
	events := make([]ChatAgentEvent, 0, 2)
	if messageErr == nil {
		events = append(events, messageEvent)
	}
	if endErr == nil {
		events = append(events, endEvent)
	}
	return events, errors.Join(messageErr, endErr)
}

func normalizeChatAgentTurnStatus(status string) string {
	switch strings.TrimSpace(status) {
	case ChatAgentTurnCompleted:
		return ChatAgentTurnCompleted
	case ChatAgentTurnFailed:
		return ChatAgentTurnFailed
	case ChatAgentTurnCancelled:
		return ChatAgentTurnCancelled
	default:
		return ChatAgentTurnInterrupted
	}
}

func chatAgentErrorCode(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata["errorCode"].(string)
	return strings.TrimSpace(value)
}
