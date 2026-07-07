package chat

import (
	"encoding/json"
	"errors"
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
)

type Handler struct {
	service *Service
}

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

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService(nil)
	}

	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == conversationsPath:
		h.handleConversations(w, r)
	case strings.HasPrefix(r.URL.Path, conversationPathBase):
		h.handleConversationChild(w, r)
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

func (h *Handler) handleConversationChild(w http.ResponseWriter, r *http.Request) {
	conversationID, ok := parseMessagesPath(r.URL.Path)
	if !ok {
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

func parseMessagesPath(path string) (string, bool) {
	remainder := strings.TrimPrefix(path, conversationPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "messages" {
		return "", false
	}

	return parts[0], true
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
	if errors.Is(err, ErrDatabaseRequired) {
		writeError(w, http.StatusServiceUnavailable, "DATABASE_REQUIRED", "database is required for chat endpoints")
		return
	}
	if errors.Is(err, ErrConversationNotFound) {
		writeError(w, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")
		return
	}
	if errors.Is(err, ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key already exists")
		return
	}

	var validationError ValidationError
	if errors.As(err, &validationError) {
		writeError(w, http.StatusBadRequest, validationError.Code, validationError.Message)
		return
	}

	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
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
