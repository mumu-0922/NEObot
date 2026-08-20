package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

type retryChatAgentToolRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
}

func (h *Handler) handleAgentEventChild(w http.ResponseWriter, r *http.Request) {
	eventID, child, ok := parseAgentEventChildPath(r.URL.Path)
	if !ok || child != "retry" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	h.retryChatAgentTool(w, r, eventID)
}

func (h *Handler) retryChatAgentTool(w http.ResponseWriter, r *http.Request, eventID string) {
	actor := auth.UserOrDevelopment(r.Context())
	if !h.agentTimelineEnabledFor(actor.ID) {
		writeError(w, http.StatusNotFound, "CHAT_AGENT_TOOL_RETRY_NOT_FOUND", "Tool retry not found")
		return
	}
	if h.localSkillExecutor == nil || !h.localSkillExecutor.Enabled() {
		writeError(w, http.StatusConflict, "CHAT_AGENT_TOOL_RETRY_UNAVAILABLE", "Tool retry is unavailable")
		return
	}
	var request retryChatAgentToolRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeRequestDecodeError(w, err)
		return
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" || len(request.IdempotencyKey) > 255 {
		writeError(
			w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY",
			"idempotencyKey is required and must be within limits",
		)
		return
	}

	source, err := h.service.GetChatAgentEvent(r.Context(), eventID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	call, retryOf, ok := authorizedChatAgentToolRetry(source)
	if !ok {
		writeError(
			w, http.StatusConflict, "CHAT_AGENT_TOOL_RETRY_NOT_AUTHORIZED",
			"Tool retry is not authorized",
		)
		return
	}
	sourceMessage, err := h.service.GetMessage(r.Context(), source.ConversationID, source.MessageID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if sourceMessage.Role != "assistant" || !isUUID(sourceMessage.ParentMessageID) ||
		source.EventID == sourceMessage.ID {
		writeError(
			w, http.StatusConflict, "CHAT_AGENT_TOOL_RETRY_NOT_AUTHORIZED",
			"Tool retry is not authorized",
		)
		return
	}

	// One failed event owns at most one retry Message. The durable source event
	// UUID is safe to reuse as the Message UUID because the tables have separate
	// key spaces; this makes response-loss and double-click replay fail closed
	// without executing the Tool twice.
	messageID := source.EventID
	if replayed, readErr := h.service.GetMessage(
		r.Context(), source.ConversationID, messageID,
	); readErr == nil {
		writeJSON(w, http.StatusOK, h.newMessageDTO(r.Context(), replayed))
		return
	}
	runID, err := NewUUID()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	call.ID, err = NewUUID()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	retryMetadata := map[string]any{
		"runId": runID,
		"agentToolRetry": map[string]any{
			"sourceEventId": source.EventID,
			"retryOf":       retryOf,
		},
	}
	retryMessage, err := h.service.CreateAssistantMessage(
		r.Context(), source.ConversationID, CreateAssistantMessageInput{
			ID: messageID, ParentMessageID: sourceMessage.ParentMessageID,
			ModelProvider: sourceMessage.ModelProvider, ModelID: sourceMessage.ModelID,
			Metadata: retryMetadata, IdempotencyKey: request.IdempotencyKey,
		},
	)
	if err != nil {
		if replayed, readErr := h.service.GetMessage(
			r.Context(), source.ConversationID, messageID,
		); readErr == nil {
			writeJSON(w, http.StatusOK, h.newMessageDTO(r.Context(), replayed))
			return
		}
		writeServiceError(w, err)
		return
	}

	executionCtx, cancel := context.WithTimeout(
		auth.WithUser(context.WithoutCancel(r.Context()), actor),
		h.localSkillExecutor.Config().RunTimeout,
	)
	defer cancel()
	recorder, err := startChatAgentEventRecorder(
		executionCtx, h.service, source.ConversationID, retryMessage.ID, runID, time.Now(),
	)
	if err != nil {
		h.failChatAgentToolRetry(
			executionCtx, source.ConversationID, retryMessage.ID,
			retryMetadata, "AGENT_EVENT_PERSISTENCE_FAILED",
		)
		writeServiceError(w, err)
		return
	}
	turnStatus := ChatAgentTurnInterrupted
	turnErrorCode := "AGENT_TOOL_RETRY_INTERRUPTED"
	defer func() {
		finishCtx, finishCancel := context.WithTimeout(
			auth.WithUser(context.Background(), actor), 5*time.Second,
		)
		defer finishCancel()
		_, _ = recorder.finish(finishCtx, turnStatus, "", turnErrorCode, time.Now())
	}()

	runtime := newLocalSkillToolRuntime(h.localSkillExecutor, nil)
	runtime.bindJobScope(actor.ID, source.ConversationID)
	events := make(chan ProviderEvent, 8)
	_, executeErr := runtime.execute(executionCtx, events, call, 1, 1)
	close(events)
	trace := newProcessTrace(retryMessage.ID)
	toolTrace := newToolProcessTrace(trace)
	for providerEvent := range events {
		if providerEvent.Type != ProviderEventToolExecution ||
			providerEvent.ToolExecution == nil {
			continue
		}
		execution := providerEvent.ToolExecution
		execution.RetryOf = retryOf
		updates := toolTrace.apply(execution, time.Now())
		if _, recordErr := recorder.recordToolExecution(
			executionCtx, execution, updates, time.Now(),
		); recordErr != nil {
			h.failChatAgentToolRetry(
				executionCtx, source.ConversationID, retryMessage.ID,
				retryMetadata, "AGENT_EVENT_PERSISTENCE_FAILED",
			)
			writeServiceError(w, recordErr)
			return
		}
	}
	status := "completed"
	turnStatus = ChatAgentTurnCompleted
	turnErrorCode = ""
	if executeErr != nil {
		status = "failed"
		turnStatus = ChatAgentTurnFailed
		turnErrorCode = "AGENT_TOOL_RETRY_FAILED"
		retryMetadata["errorCode"] = turnErrorCode
	}
	retryMetadata = withProcessTraceMessageMetadata(retryMetadata, "", trace)
	if _, err := h.finalizeAssistantMessage(
		executionCtx, source.ConversationID, retryMessage.ID,
		FinalizeAssistantMessageInput{
			Status: status, Content: "", OutputBlocks: []any{}, Metadata: retryMetadata,
		},
	); err != nil {
		writeServiceError(w, err)
		return
	}
	if _, err := recorder.finish(
		executionCtx, turnStatus, "", turnErrorCode, time.Now(),
	); err != nil {
		writeServiceError(w, err)
		return
	}
	completed, err := h.service.GetMessage(
		executionCtx, source.ConversationID, retryMessage.ID,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.newMessageDTO(r.Context(), completed))
}

func (h *Handler) failChatAgentToolRetry(
	ctx context.Context,
	conversationID string,
	messageID string,
	metadata map[string]any,
	code string,
) {
	failed := cloneJSONObject(metadata)
	failed["errorCode"] = code
	_, _ = h.finalizeAssistantMessage(ctx, conversationID, messageID, FinalizeAssistantMessageInput{
		Status: "failed", Content: "", OutputBlocks: []any{}, Metadata: failed,
	})
}

func authorizedChatAgentToolRetry(event ChatAgentEvent) (ProviderToolCall, string, bool) {
	if event.Type != ChatAgentEventToolResult {
		return ProviderToolCall{}, "", false
	}
	execution := projectChatAgentToolExecution(event)
	if execution == nil {
		return ProviderToolCall{}, "", false
	}
	for _, step := range processStepsFromChatAgentEvent(event) {
		presentation := step.Presentation
		if presentation == nil || presentation.Card != "file" || presentation.Operation != "read" ||
			presentation.Retry == nil || presentation.Retry.EventID != event.EventID ||
			presentation.Retry.RetryOf != execution.CallID || strings.TrimSpace(presentation.Path) == "" {
			continue
		}
		execution.Presentation = presentation
		if !chatAgentToolRetryEligible(execution) {
			continue
		}
		arguments, err := json.Marshal(map[string]any{
			"path": presentation.Path, "offset": presentation.Offset, "limit": nil,
		})
		if err != nil {
			return ProviderToolCall{}, "", false
		}
		return ProviderToolCall{
			Name: localFileReadToolName, Arguments: string(arguments),
		}, execution.CallID, true
	}
	return ProviderToolCall{}, "", false
}

func parseAgentEventChildPath(path string) (string, string, bool) {
	remainder := strings.TrimPrefix(path, agentEventsPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || !isUUID(parts[0]) || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
