package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	contentTypeJSON      = "application/json; charset=utf-8"
	maxRequestBodyBytes  = 1 << 20
	conversationsPath    = "/v1/chat/conversations"
	conversationPathBase = conversationsPath + "/"
	runsPathBase         = "/v1/chat/runs/"
)

type Handler struct {
	service    *Service
	provider   Provider
	activeRuns *activeRunRegistry
}

type HandlerOption func(*Handler)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type ConversationDTO struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Status       string         `json:"status"`
	ModelRef     *ModelRef      `json:"modelRef,omitempty"`
	MessageCount int            `json:"messageCount"`
	Config       map[string]any `json:"config"`
	CreatedAt    string         `json:"createdAt"`
	UpdatedAt    string         `json:"updatedAt"`
}

type ChatMessageDTO struct {
	ID              string         `json:"id"`
	ConversationID  string         `json:"conversationId"`
	SequenceNo      int            `json:"sequenceNo"`
	Role            string         `json:"role"`
	Status          string         `json:"status"`
	Content         string         `json:"content"`
	ModelRef        *ModelRef      `json:"modelRef,omitempty"`
	OutputBlocks    []any          `json:"outputBlocks"`
	Metadata        map[string]any `json:"metadata"`
	ParentMessageID string         `json:"parentMessageId,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
	CompletedAt     string         `json:"completedAt,omitempty"`
}

type createConversationRequest struct {
	Title             string         `json:"title"`
	ModelRef          *ModelRef      `json:"modelRef"`
	SystemInstruction string         `json:"systemInstruction"`
	SystemPrompt      string         `json:"systemPrompt"`
	Config            map[string]any `json:"config"`
	Metadata          map[string]any `json:"metadata"`
	IdempotencyKey    string         `json:"idempotencyKey"`
}

type createMessageRequest struct {
	Role            string         `json:"role"`
	Content         string         `json:"content"`
	ParentMessageID string         `json:"parentMessageId"`
	Metadata        map[string]any `json:"metadata"`
	IdempotencyKey  string         `json:"idempotencyKey"`
}

type fieldViolation struct {
	Code    string
	Message string
}

type streamMessageRequest struct {
	UserMessageID     string         `json:"userMessageId"`
	ModelRef          *ModelRef      `json:"modelRef"`
	SystemInstruction string         `json:"systemInstruction"`
	SystemPrompt      string         `json:"systemPrompt"`
	Config            map[string]any `json:"config"`
	Metadata          map[string]any `json:"metadata"`
	IdempotencyKey    string         `json:"idempotencyKey"`
}

type streamEvent struct {
	Type           string          `json:"type"`
	RunID          string          `json:"runId"`
	ConversationID string          `json:"conversationId"`
	MessageID      string          `json:"messageId,omitempty"`
	Sequence       int             `json:"sequence"`
	CreatedAt      string          `json:"createdAt"`
	Role           string          `json:"role,omitempty"`
	ModelRef       *ModelRef       `json:"modelRef,omitempty"`
	Delta          string          `json:"delta,omitempty"`
	Usage          *TokenUsage     `json:"usage,omitempty"`
	Message        *ChatMessageDTO `json:"message,omitempty"`
	Error          *ErrorBody      `json:"error,omitempty"`
}

type cancelRunResponse struct {
	RunID   string         `json:"runId"`
	Status  string         `json:"status"`
	Message ChatMessageDTO `json:"message"`
}

func WithProvider(provider Provider) HandlerOption {
	return func(h *Handler) {
		if provider != nil {
			h.provider = provider
		}
	}
}

func NewHandler(service *Service, opts ...HandlerOption) *Handler {
	if service == nil {
		service = NewService(nil)
	}

	handler := &Handler{
		service:    service,
		activeRuns: newActiveRunRegistry(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}

	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == conversationsPath:
		h.handleConversations(w, r)
	case strings.HasPrefix(r.URL.Path, conversationPathBase):
		h.handleConversationChild(w, r)
	case strings.HasPrefix(r.URL.Path, runsPathBase):
		h.handleRunChild(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handleConversations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createConversation(w, r)
	case http.MethodGet:
		h.listConversations(w, r)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleRunChild(w http.ResponseWriter, r *http.Request) {
	runID, child, ok := parseRunChildPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if child != "cancel" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	h.cancelRun(w, r, runID)
}

func (h *Handler) cancelRun(w http.ResponseWriter, r *http.Request, runID string) {
	h.activeRuns.cancel(runID)

	message, err := h.service.CancelRun(r.Context(), runID, CancelRunInput{
		Metadata: map[string]any{
			"runId":       runID,
			"cancelledBy": "api",
		},
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	h.activeRuns.cancel(runID)

	writeJSON(w, http.StatusOK, cancelRunResponse{
		RunID:   runID,
		Status:  message.Status,
		Message: newMessageDTO(message),
	})
}

func (h *Handler) handleConversationChild(w http.ResponseWriter, r *http.Request) {
	conversationID, child, ok := parseConversationChildPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}

	if child == "stream" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.streamAssistantMessage(w, r, conversationID)
		return
	}
	if child != "messages" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.listMessages(w, r, conversationID)
	case http.MethodPost:
		h.createMessage(w, r, conversationID)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) createConversation(w http.ResponseWriter, r *http.Request) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}

	var request createConversationRequest
	if err := decodeJSONWithForbiddenFields(
		w,
		r,
		&request,
		forbiddenConversationFields(),
	); err != nil {
		writeRequestDecodeError(w, err)
		return
	}

	metadata := request.Config
	if metadata == nil {
		metadata = request.Metadata
	}
	input := CreateConversationInput{
		Title:          request.Title,
		SystemPrompt:   request.SystemInstruction,
		Metadata:       metadata,
		IdempotencyKey: request.IdempotencyKey,
	}
	if input.SystemPrompt == "" {
		input.SystemPrompt = request.SystemPrompt
	}
	if request.ModelRef != nil {
		input.ModelProvider = request.ModelRef.ProviderID
		input.ModelID = request.ModelRef.ModelID
	}

	conversation, err := h.service.CreateConversation(r.Context(), input)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newConversationDTO(conversation))
}

func (h *Handler) listConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := h.service.ListConversations(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	items := make([]ConversationDTO, 0, len(conversations))
	for _, conversation := range conversations {
		items = append(items, newConversationDTO(conversation))
	}

	writeJSON(w, http.StatusOK, Page[ConversationDTO]{Items: items})
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request, conversationID string) {
	messages, err := h.service.ListMessages(r.Context(), conversationID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	items := make([]ChatMessageDTO, 0, len(messages))
	for _, message := range messages {
		items = append(items, newMessageDTO(message))
	}

	writeJSON(w, http.StatusOK, Page[ChatMessageDTO]{Items: items})
}

func (h *Handler) createMessage(w http.ResponseWriter, r *http.Request, conversationID string) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}

	var request createMessageRequest
	if err := decodeJSONWithForbiddenFields(w, r, &request, forbiddenMessageFields()); err != nil {
		writeRequestDecodeError(w, err)
		return
	}

	message, err := h.service.CreateMessage(r.Context(), conversationID, CreateMessageInput{
		Role:            request.Role,
		Content:         request.Content,
		ParentMessageID: request.ParentMessageID,
		Metadata:        request.Metadata,
		IdempotencyKey:  request.IdempotencyKey,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newMessageDTO(message))
}

func (h *Handler) streamAssistantMessage(w http.ResponseWriter, r *http.Request, conversationID string) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}
	if h.provider == nil {
		writeServiceError(w, ErrProviderRequired)
		return
	}

	var request streamMessageRequest
	if err := decodeJSONWithForbiddenFields(w, r, &request, forbiddenStreamFields()); err != nil {
		writeRequestDecodeError(w, err)
		return
	}

	modelRef := request.ModelRef
	if modelRef == nil {
		writeError(w, http.StatusBadRequest, "MODEL_REF_REQUIRED", "modelRef is required")
		return
	}
	if resolver, ok := h.provider.(ModelRefResolver); ok {
		resolvedModelRef, err := resolver.ResolveModelRef(*modelRef)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		modelRef = &resolvedModelRef
	} else if validator, ok := h.provider.(ModelRefValidator); ok {
		if err := validator.ValidateModelRef(*modelRef); err != nil {
			writeServiceError(w, err)
			return
		}
	}
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "idempotencyKey is required")
		return
	}
	systemPrompt := request.SystemInstruction
	if systemPrompt == "" {
		systemPrompt = request.SystemPrompt
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "streaming is not supported")
		return
	}

	userMessage, err := h.service.GetMessage(r.Context(), conversationID, request.UserMessageID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if userMessage.Role != "user" {
		writeRequestDecodeError(w, newValidationError("INVALID_USER_MESSAGE_ID", "userMessageId must reference a user message"))
		return
	}

	runID, err := NewUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	assistantMessageID, err := NewUUID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}

	assistantMessage, err := h.service.CreateAssistantMessage(
		r.Context(),
		conversationID,
		CreateAssistantMessageInput{
			ID:              assistantMessageID,
			ParentMessageID: userMessage.ID,
			ModelProvider:   modelRef.ProviderID,
			ModelID:         modelRef.ModelID,
			Metadata: map[string]any{
				"runId":  runID,
				"config": ensureObject(request.Config),
			},
			IdempotencyKey: request.IdempotencyKey,
		},
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	streamCtx, streamCancel := context.WithCancel(r.Context())
	unregisterRun := h.activeRuns.register(runID, streamCancel)
	defer unregisterRun()
	defer streamCancel()

	events, err := h.provider.StreamChat(streamCtx, ProviderRequest{
		RunID:              runID,
		ConversationID:     conversationID,
		UserMessageID:      userMessage.ID,
		AssistantMessageID: assistantMessage.ID,
		Prompt:             userMessage.Content,
		SystemPrompt:       systemPrompt,
		ModelRef:           *modelRef,
		Metadata:           request.Metadata,
	})
	if err != nil {
		if streamCtx.Err() != nil || r.Context().Err() != nil || errors.Is(err, context.Canceled) {
			h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
				Status:   "cancelled",
				Metadata: map[string]any{"runId": runID},
			})
			return
		}
		h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
			Status:   "failed",
			Metadata: map[string]any{"runId": runID, "errorCode": "PROVIDER_ERROR"},
		})
		writeError(w, http.StatusBadGateway, "PROVIDER_ERROR", "provider stream failed")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	sequence := 1
	if err := writeSSEEvent(w, "message.started", streamEvent{
		Type:           "message.started",
		RunID:          runID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		Sequence:       sequence,
		CreatedAt:      formatTime(time.Now()),
		Role:           "assistant",
		ModelRef:       modelRef,
	}); err != nil {
		h.cancelAssistantAfterWriteError(conversationID, assistantMessage.ID, runID, "")
		return
	}
	flusher.Flush()

	var content strings.Builder
	for providerEvent := range events {
		if providerEvent.Error != nil {
			if streamCtx.Err() != nil {
				sequence++
				_ = writeSSEEvent(w, "message.cancelled", streamEvent{
					Type:           "message.cancelled",
					RunID:          runID,
					ConversationID: conversationID,
					MessageID:      assistantMessage.ID,
					Sequence:       sequence,
					CreatedAt:      formatTime(time.Now()),
				})
				h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
					Status:   "cancelled",
					Content:  content.String(),
					Metadata: map[string]any{"runId": runID},
				})
				flusher.Flush()
				return
			}
			sequence++
			_ = writeSSEEvent(w, "message.error", streamEvent{
				Type:           "message.error",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
				Error:          &ErrorBody{Code: "PROVIDER_ERROR", Message: "provider stream failed"},
			})
			h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
				Status:   "failed",
				Content:  content.String(),
				Metadata: map[string]any{"runId": runID, "errorCode": "PROVIDER_ERROR"},
			})
			flusher.Flush()
			return
		}

		switch providerEvent.Type {
		case ProviderEventDelta:
			content.WriteString(providerEvent.Delta)
			sequence++
			if err := writeSSEEvent(w, "message.delta", streamEvent{
				Type:           "message.delta",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
				Delta:          providerEvent.Delta,
			}); err != nil {
				h.cancelAssistantAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
				return
			}
			flusher.Flush()
		case ProviderEventUsage:
			sequence++
			if err := writeSSEEvent(w, "usage.updated", streamEvent{
				Type:           "usage.updated",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
				Usage:          providerEvent.Usage,
			}); err != nil {
				h.cancelAssistantAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
				return
			}
			flusher.Flush()
		}
	}

	if err := streamCtx.Err(); err != nil {
		sequence++
		_ = writeSSEEvent(w, "message.cancelled", streamEvent{
			Type:           "message.cancelled",
			RunID:          runID,
			ConversationID: conversationID,
			MessageID:      assistantMessage.ID,
			Sequence:       sequence,
			CreatedAt:      formatTime(time.Now()),
		})
		h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
			Status:   "cancelled",
			Content:  content.String(),
			Metadata: map[string]any{"runId": runID},
		})
		flusher.Flush()
		return
	}

	assistantMessage, err = h.finalizeAssistantMessage(
		context.Background(),
		conversationID,
		assistantMessage.ID,
		FinalizeAssistantMessageInput{
			Status:  "completed",
			Content: content.String(),
			Metadata: map[string]any{
				"runId": runID,
			},
		},
	)
	if err != nil {
		sequence++
		_, errorBody := serviceErrorFor(err)
		_ = writeSSEEvent(w, "message.error", streamEvent{
			Type:           "message.error",
			RunID:          runID,
			ConversationID: conversationID,
			MessageID:      assistantMessageID,
			Sequence:       sequence,
			CreatedAt:      formatTime(time.Now()),
			Error:          &errorBody,
		})
		flusher.Flush()
		return
	}

	sequence++
	assistantDTO := newMessageDTO(assistantMessage)
	if err := writeSSEEvent(w, "message.completed", streamEvent{
		Type:           "message.completed",
		RunID:          runID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		Sequence:       sequence,
		CreatedAt:      formatTime(time.Now()),
		Message:        &assistantDTO,
	}); err != nil {
		return
	}
	flusher.Flush()
}

func parseConversationChildPath(path string) (string, string, bool) {
	remainder := strings.TrimPrefix(path, conversationPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func parseRunChildPath(path string) (string, string, bool) {
	remainder := strings.TrimPrefix(path, runsPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	return decodeJSONWithForbiddenFields(w, r, destination, nil)
}

func decodeJSONWithForbiddenFields(
	w http.ResponseWriter,
	r *http.Request,
	destination any,
	forbiddenFields map[string]fieldViolation,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	var trailing struct{}
	if err := decoder.Decode(&trailing); err != nil {
		if errors.Is(err, io.EOF) {
			return decodeRawObject(raw, destination, forbiddenFields)
		}
		return err
	}

	return errors.New("request body must contain a single JSON value")
}

func decodeRawObject(
	raw map[string]json.RawMessage,
	destination any,
	forbiddenFields map[string]fieldViolation,
) error {
	for field, violation := range forbiddenFields {
		if _, ok := raw[field]; ok {
			return ValidationError{Code: violation.Code, Message: violation.Message}
		}
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return err
	}

	return nil
}

func writeRequestDecodeError(w http.ResponseWriter, err error) {
	var validationError ValidationError
	if errors.As(err, &validationError) {
		writeError(w, http.StatusBadRequest, validationError.Code, validationError.Message)
		return
	}

	writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request body")
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorFor(err)
	writeError(w, status, body.Code, body.Message)
}

func serviceErrorFor(err error) (int, ErrorBody) {
	if errors.Is(err, ErrDatabaseRequired) {
		return http.StatusServiceUnavailable, ErrorBody{Code: "DATABASE_REQUIRED", Message: "database is required for chat endpoints"}
	}
	if errors.Is(err, ErrProviderRequired) {
		return http.StatusServiceUnavailable, ErrorBody{Code: "PROVIDER_REQUIRED", Message: "provider is required for streaming endpoints"}
	}
	if errors.Is(err, ErrConversationNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "CONVERSATION_NOT_FOUND", Message: "conversation not found"}
	}
	if errors.Is(err, ErrIdempotencyConflict) {
		return http.StatusConflict, ErrorBody{Code: "IDEMPOTENCY_CONFLICT", Message: "idempotency key already exists"}
	}
	if errors.Is(err, ErrRunNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "RUN_NOT_FOUND", Message: "run not found"}
	}
	if errors.Is(err, ErrRunNotCancellable) {
		return http.StatusConflict, ErrorBody{Code: "RUN_NOT_CANCELLABLE", Message: "run is not cancellable"}
	}

	var validationError ValidationError
	if errors.As(err, &validationError) {
		return http.StatusBadRequest, ErrorBody{Code: validationError.Code, Message: validationError.Message}
	}

	return http.StatusInternalServerError, ErrorBody{Code: "INTERNAL_ERROR", Message: "internal server error"}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

func writeSSEEvent(w io.Writer, event string, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
	return err
}

func forbiddenConversationFields() map[string]fieldViolation {
	identity := validationField("caller identity fields are not accepted")
	return map[string]fieldViolation{
		"id":                validationField("conversation field is server-managed"),
		"userId":            identity,
		"ownerId":           identity,
		"sessionId":         identity,
		"session":           identity,
		"bearerToken":       identity,
		"accessToken":       identity,
		"authorization":     identity,
		"impersonateUserId": identity,
		"status":            validationField("conversation field is server-managed"),
		"messageCount":      validationField("conversation field is server-managed"),
		"modelProvider":     validationField("use modelRef.providerId instead of modelProvider"),
		"modelId":           validationField("use modelRef.modelId instead of modelId"),
		"createdAt":         validationField("conversation field is server-managed"),
		"updatedAt":         validationField("conversation field is server-managed"),
		"deletedAt":         validationField("conversation field is server-managed"),
	}
}

func forbiddenMessageFields() map[string]fieldViolation {
	violation := fieldViolation{
		Code:    "FORBIDDEN_MESSAGE_FIELD",
		Message: "message field is server-managed",
	}
	identity := fieldViolation{
		Code:    "FORBIDDEN_MESSAGE_FIELD",
		Message: "caller identity fields are not accepted",
	}

	return map[string]fieldViolation{
		"id":                violation,
		"conversationId":    violation,
		"userId":            identity,
		"ownerId":           identity,
		"sessionId":         identity,
		"session":           identity,
		"bearerToken":       identity,
		"accessToken":       identity,
		"authorization":     identity,
		"impersonateUserId": identity,
		"sequenceNo":        violation,
		"status":            violation,
		"modelRef":          violation,
		"modelProvider":     violation,
		"modelId":           violation,
		"providerMessageId": violation,
		"outputBlocks":      violation,
		"errorCode":         violation,
		"errorMessage":      violation,
		"createdAt":         violation,
		"updatedAt":         violation,
		"completedAt":       violation,
		"deletedAt":         violation,
	}
}

func forbiddenStreamFields() map[string]fieldViolation {
	fields := forbiddenMessageFields()
	delete(fields, "modelRef")
	fields["role"] = fieldViolation{Code: "FORBIDDEN_MESSAGE_FIELD", Message: "message field is server-managed"}
	fields["content"] = validationField("content is not supported in this streaming phase")
	fields["attachments"] = validationField("attachments are not supported in this streaming phase")
	return fields
}

func (h *Handler) finalizeAssistantMessage(
	ctx context.Context,
	conversationID string,
	messageID string,
	input FinalizeAssistantMessageInput,
) (Message, error) {
	finalizeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return h.service.FinalizeAssistantMessage(finalizeCtx, conversationID, messageID, input)
}

func (h *Handler) cancelAssistantAfterWriteError(
	conversationID string,
	messageID string,
	runID string,
	content string,
) {
	_, _ = h.finalizeAssistantMessage(
		context.Background(),
		conversationID,
		messageID,
		FinalizeAssistantMessageInput{
			Status:  "cancelled",
			Content: content,
			Metadata: map[string]any{
				"runId":     runID,
				"errorCode": "SSE_WRITE_FAILED",
			},
		},
	)
}

func validationField(message string) fieldViolation {
	return fieldViolation{Code: "VALIDATION_ERROR", Message: message}
}

func newConversationDTO(conversation Conversation) ConversationDTO {
	return ConversationDTO{
		ID:           conversation.ID,
		Title:        conversation.Title,
		Status:       conversation.Status,
		ModelRef:     newModelRef(conversation.ModelProvider, conversation.ModelID),
		MessageCount: conversation.MessageCount,
		Config:       ensureObject(conversation.Metadata),
		CreatedAt:    formatTime(conversation.CreatedAt),
		UpdatedAt:    formatTime(conversation.UpdatedAt),
	}
}

func newMessageDTO(message Message) ChatMessageDTO {
	return ChatMessageDTO{
		ID:              message.ID,
		ConversationID:  message.ConversationID,
		SequenceNo:      message.SequenceNo,
		Role:            message.Role,
		Status:          message.Status,
		Content:         message.Content,
		ModelRef:        newModelRef(message.ModelProvider, message.ModelID),
		OutputBlocks:    ensureArray(message.OutputBlocks),
		Metadata:        ensureObject(message.Metadata),
		ParentMessageID: message.ParentMessageID,
		CreatedAt:       formatTime(message.CreatedAt),
		UpdatedAt:       formatTime(message.UpdatedAt),
		CompletedAt:     formatOptionalTime(message.CompletedAt),
	}
}

func newModelRef(providerID string, modelID string) *ModelRef {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" && modelID == "" {
		return nil
	}

	return &ModelRef{ProviderID: providerID, ModelID: modelID}
}

func ensureObject(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}

	return value
}

func ensureArray(value []any) []any {
	if value == nil {
		return []any{}
	}

	return value
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}

	return formatTime(*value)
}
