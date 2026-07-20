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

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/usermemory"
	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	contentTypeJSON      = "application/json; charset=utf-8"
	maxRequestBodyBytes  = 1 << 20
	conversationsPath    = "/v1/chat/conversations"
	conversationPathBase = conversationsPath + "/"
	runsPathBase         = "/v1/chat/runs/"
	toolPlanPath         = "/v1/chat/tools/plan"
	maxToolPlanPrompt    = 16 * 1024
	maxToolPlanTools     = 32
	maxToolNameBytes     = 128
	maxToolDescBytes     = 2048
	maxToolParamsBytes   = 32 * 1024
	defaultChatImageSize = "1024x1024"

	ImageContentPolicyViolationCode = "IMAGE_CONTENT_POLICY_VIOLATION"
	ImageProviderConnectionCode     = "IMAGE_PROVIDER_CONNECTION_ERROR"
	ImageProviderTimeoutCode        = "IMAGE_PROVIDER_TIMEOUT"
)

type Handler struct {
	service             *Service
	provider            Provider
	attachmentResolver  ProviderAttachmentResolver
	providerResolver    RuntimeProviderResolver
	imageGenerator      ImageGenerator
	activeRuns          *activeRunRegistry
	cancellationRuns    RunCancellationStore
	ragAssembler        *RAGAnswerAssembler
	ragAnswerGate       RAGAnswerGovernanceGate
	webSearchService    *websearch.Service
	userMemoryService   *usermemory.Service
	contextBudgetPolicy contextBudgetPolicy
}

type HandlerOption func(*Handler)

type ProviderAttachmentResolver interface {
	ResolveProviderAttachment(ctx context.Context, attachment Attachment) (ProviderAttachment, error)
}

type RuntimeProviderResolver interface {
	ResolveRuntimeProvider(ctx context.Context, provider runtimeconfig.ProviderRuntimeConfig) (Provider, error)
}

type ImageGenerationRequest struct {
	ModelRef ModelRef
	Prompt   string
	Size     string
}

type GeneratedImageAttachment struct {
	FileID  string
	Purpose string
}

type ImageGenerationResult struct {
	Attachments []GeneratedImageAttachment
	Message     string
}

type ImageGenerationError struct {
	Code string
	Err  error
}

func (e *ImageGenerationError) Error() string {
	if e == nil || e.Err == nil {
		return "image generation failed"
	}
	return e.Err.Error()
}

func (e *ImageGenerationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type ImageGenerator interface {
	GenerateImage(context.Context, ImageGenerationRequest) (ImageGenerationResult, error)
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
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	Status            string         `json:"status"`
	ModelRef          *ModelRef      `json:"modelRef,omitempty"`
	MessageCount      int            `json:"messageCount"`
	SystemInstruction string         `json:"systemInstruction,omitempty"`
	Pinned            bool           `json:"pinned"`
	Config            map[string]any `json:"config"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
}

type ChatMessageDTO struct {
	ID              string          `json:"id"`
	ConversationID  string          `json:"conversationId"`
	SequenceNo      int             `json:"sequenceNo"`
	Role            string          `json:"role"`
	Status          string          `json:"status"`
	Content         string          `json:"content"`
	ModelRef        *ModelRef       `json:"modelRef,omitempty"`
	Attachments     []AttachmentDTO `json:"attachments"`
	OutputBlocks    []any           `json:"outputBlocks"`
	Metadata        map[string]any  `json:"metadata"`
	ParentMessageID string          `json:"parentMessageId,omitempty"`
	CreatedAt       string          `json:"createdAt"`
	UpdatedAt       string          `json:"updatedAt"`
	CompletedAt     string          `json:"completedAt,omitempty"`
}

type AttachmentDTO struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	FileID   string `json:"fileId"`
	FileName string `json:"fileName"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Purpose  string `json:"purpose"`
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

type updateConversationRequest struct {
	Title             *string        `json:"title"`
	ModelRef          *ModelRef      `json:"modelRef"`
	SystemInstruction *string        `json:"systemInstruction"`
	SystemPrompt      *string        `json:"systemPrompt"`
	Config            map[string]any `json:"config"`
	Metadata          map[string]any `json:"metadata"`
	Pinned            *bool          `json:"pinned"`
}

type duplicateConversationRequest struct {
	Title          string `json:"title"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type generateConversationTitleRequest struct {
	ModelRef *ModelRef `json:"modelRef"`
}

type generateConversationTitleResponse struct {
	Title string `json:"title"`
}

type generateRelatedQuestionsRequest struct {
	ModelRef *ModelRef `json:"modelRef"`
}

type generateRelatedQuestionsResponse struct {
	Questions []string `json:"questions"`
}

type createMessageRequest struct {
	Role            string          `json:"role"`
	Content         string          `json:"content"`
	ParentMessageID string          `json:"parentMessageId"`
	Metadata        map[string]any  `json:"metadata"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	Attachments     []AttachmentDTO `json:"attachments"`
}

type updateMessageRequest struct {
	Content *string `json:"content"`
}

type fieldViolation struct {
	Code    string
	Message string
}

type streamMessageRequest struct {
	UserMessageID     string                               `json:"userMessageId"`
	ModelRef          *ModelRef                            `json:"modelRef"`
	Provider          *runtimeconfig.ProviderRuntimeConfig `json:"provider"`
	SystemInstruction string                               `json:"systemInstruction"`
	SystemPrompt      string                               `json:"systemPrompt"`
	Config            map[string]any                       `json:"config"`
	Metadata          map[string]any                       `json:"metadata"`
	IdempotencyKey    string                               `json:"idempotencyKey"`
}

type toolPlanRequest struct {
	Prompt   string           `json:"prompt"`
	ModelRef *ModelRef        `json:"modelRef"`
	Tools    []ToolDefinition `json:"tools"`
}

type toolPlanResponse struct {
	Calls []ToolCall `json:"calls"`
}

type streamEvent struct {
	Type           string            `json:"type"`
	RunID          string            `json:"runId"`
	ConversationID string            `json:"conversationId"`
	MessageID      string            `json:"messageId,omitempty"`
	Sequence       int               `json:"sequence"`
	CreatedAt      string            `json:"createdAt"`
	Role           string            `json:"role,omitempty"`
	ModelRef       *ModelRef         `json:"modelRef,omitempty"`
	Delta          string            `json:"delta,omitempty"`
	Usage          *TokenUsage       `json:"usage,omitempty"`
	Message        *ChatMessageDTO   `json:"message,omitempty"`
	Error          *ErrorBody        `json:"error,omitempty"`
	Results        *websearch.Result `json:"results,omitempty"`
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

func WithAttachmentResolver(resolver ProviderAttachmentResolver) HandlerOption {
	return func(h *Handler) {
		h.attachmentResolver = resolver
	}
}

func WithImageGenerator(generator ImageGenerator) HandlerOption {
	return func(h *Handler) {
		if generator != nil {
			h.imageGenerator = generator
		}
	}
}

func WithRuntimeProviderResolver(resolver RuntimeProviderResolver) HandlerOption {
	return func(h *Handler) {
		h.providerResolver = resolver
	}
}

func WithRAGAnswerAssembler(assembler *RAGAnswerAssembler) HandlerOption {
	return func(h *Handler) {
		h.ragAssembler = assembler
	}
}

func WithRAGAnswerGovernanceGate(gate RAGAnswerGovernanceGate) HandlerOption {
	return func(h *Handler) {
		h.ragAnswerGate = gate
	}
}

func WithWebSearchService(service *websearch.Service) HandlerOption {
	return func(h *Handler) {
		if service != nil && service.Configured() {
			h.webSearchService = service
		}
	}
}

func WithUserMemoryService(service *usermemory.Service) HandlerOption {
	return func(handler *Handler) {
		handler.userMemoryService = service
	}
}

func WithRunCancellationStore(store RunCancellationStore) HandlerOption {
	return func(h *Handler) {
		h.cancellationRuns = store
	}
}

func NewHandler(service *Service, opts ...HandlerOption) *Handler {
	if service == nil {
		service = NewService(nil)
	}

	handler := &Handler{
		service:             service,
		activeRuns:          newActiveRunRegistry(),
		contextBudgetPolicy: defaultContextBudgetPolicy(),
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
		if conversationID, ok := parseConversationResourcePath(r.URL.Path); ok {
			h.handleConversationResource(w, r, conversationID)
			return
		}
		h.handleConversationChild(w, r)
	case strings.HasPrefix(r.URL.Path, runsPathBase):
		h.handleRunChild(w, r)
	case r.URL.Path == toolPlanPath:
		h.handleToolPlan(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handleToolPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if h.provider == nil {
		writeServiceError(w, ErrProviderRequired)
		return
	}
	planner, ok := h.provider.(ToolPlanner)
	if !ok {
		writeError(w, http.StatusNotImplemented, "TOOLS_UNSUPPORTED", "configured provider does not support tool planning")
		return
	}

	var request toolPlanRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeRequestDecodeError(w, err)
		return
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" || len(prompt) > maxToolPlanPrompt {
		writeError(w, http.StatusBadRequest, "INVALID_TOOL_PLAN", "tool plan prompt is required and must be within limits")
		return
	}
	if request.ModelRef == nil {
		writeError(w, http.StatusBadRequest, "MODEL_REF_REQUIRED", "modelRef is required")
		return
	}
	if len(request.Tools) == 0 || len(request.Tools) > maxToolPlanTools {
		writeError(w, http.StatusBadRequest, "INVALID_TOOL_PLAN", "tool plan requires between 1 and 32 tools")
		return
	}
	for _, tool := range request.Tools {
		if err := validateToolDefinition(tool); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_TOOL_PLAN", err.Error())
			return
		}
	}

	calls, err := planner.PlanTools(r.Context(), ToolPlanRequest{
		Prompt:   prompt,
		ModelRef: *request.ModelRef,
		Tools:    request.Tools,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "PROVIDER_ERROR", "provider tool planning failed")
		return
	}
	if err := validatePlannedToolCalls(calls, request.Tools); err != nil {
		writeError(w, http.StatusBadGateway, "PROVIDER_ERROR", "provider returned an invalid tool plan")
		return
	}
	if calls == nil {
		calls = []ToolCall{}
	}
	writeJSON(w, http.StatusOK, toolPlanResponse{Calls: calls})
}

func validatePlannedToolCalls(calls []ToolCall, tools []ToolDefinition) error {
	if len(calls) > maxToolPlanTools {
		return errors.New("tool plan contains too many calls")
	}

	allowedNames := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		allowedNames[strings.TrimSpace(tool.Function.Name)] = struct{}{}
	}
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if _, ok := allowedNames[name]; !ok {
			return errors.New("tool plan contains an unavailable function")
		}
		if call.Args == nil {
			return errors.New("tool plan arguments must be an object")
		}
		encoded, err := json.Marshal(call.Args)
		if err != nil || len(encoded) > maxToolParamsBytes {
			return errors.New("tool plan arguments are invalid or too large")
		}
	}
	return nil
}

func validateToolDefinition(tool ToolDefinition) error {
	if tool.Type != "function" {
		return errors.New("tool type must be function")
	}
	name := strings.TrimSpace(tool.Function.Name)
	if name == "" || len(name) > maxToolNameBytes {
		return errors.New("tool function name is required and must be within limits")
	}
	for index, value := range name {
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9' && index > 0) || value == '_' || value == '-' {
			continue
		}
		return errors.New("tool function name contains unsupported characters")
	}
	if len(tool.Function.Description) > maxToolDescBytes {
		return errors.New("tool function description is too long")
	}
	if tool.Function.Parameters == nil {
		return errors.New("tool function parameters are required")
	}
	encoded, err := json.Marshal(tool.Function.Parameters)
	if err != nil || len(encoded) > maxToolParamsBytes {
		return errors.New("tool function parameters are invalid or too large")
	}
	return nil
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

func (h *Handler) handleConversationResource(w http.ResponseWriter, r *http.Request, conversationID string) {
	switch r.Method {
	case http.MethodPatch:
		h.updateConversation(w, r, conversationID)
	case http.MethodDelete:
		h.deleteConversation(w, r, conversationID)
	default:
		methodNotAllowed(w, http.MethodPatch+", "+http.MethodDelete)
	}
}

func (h *Handler) updateConversation(w http.ResponseWriter, r *http.Request, conversationID string) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}

	var request updateConversationRequest
	if err := decodeJSONWithForbiddenFields(w, r, &request, forbiddenConversationUpdateFields()); err != nil {
		writeRequestDecodeError(w, err)
		return
	}

	metadataMerge := request.Metadata
	if metadataMerge == nil {
		metadataMerge = request.Config
	}
	if metadataMerge == nil {
		metadataMerge = map[string]any{}
	}
	if request.Pinned != nil {
		metadataMerge["pinned"] = *request.Pinned
	}

	input := UpdateConversationInput{
		Title:         request.Title,
		SystemPrompt:  request.SystemInstruction,
		MetadataMerge: metadataMerge,
	}
	if input.SystemPrompt == nil {
		input.SystemPrompt = request.SystemPrompt
	}
	if request.ModelRef != nil {
		input.ModelProvider = &request.ModelRef.ProviderID
		input.ModelID = &request.ModelRef.ModelID
	}

	conversation, err := h.service.UpdateConversation(r.Context(), conversationID, input)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newConversationDTO(conversation))
}

func (h *Handler) deleteConversation(w http.ResponseWriter, r *http.Request, conversationID string) {
	if err := h.service.DeleteConversation(r.Context(), conversationID); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) duplicateConversation(w http.ResponseWriter, r *http.Request, conversationID string) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}

	request := duplicateConversationRequest{}
	if r.Body != nil && r.Body != http.NoBody {
		if err := decodeJSONWithForbiddenFields(w, r, &request, forbiddenConversationFields()); err != nil {
			writeRequestDecodeError(w, err)
			return
		}
	}

	conversation, err := h.service.DuplicateConversation(r.Context(), conversationID, DuplicateConversationInput{
		Title:          request.Title,
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newConversationDTO(conversation))
}

func (h *Handler) generateConversationTitle(w http.ResponseWriter, r *http.Request, conversationID string) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}

	var request generateConversationTitleRequest
	if err := decodeJSONWithForbiddenFields(w, r, &request, forbiddenConversationTitleFields()); err != nil {
		writeRequestDecodeError(w, err)
		return
	}

	messages, err := h.service.ListMessages(r.Context(), conversationID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	fallbackTitle := fallbackTitleFromMessages(messages)
	if h.provider == nil {
		writeJSON(w, http.StatusOK, generateConversationTitleResponse{Title: fallbackTitle})
		return
	}
	if request.ModelRef == nil {
		writeJSON(w, http.StatusOK, generateConversationTitleResponse{Title: fallbackTitle})
		return
	}

	title, err := generateTitleWithProvider(r.Context(), h.provider, *request.ModelRef, messages, fallbackTitle)
	if err != nil {
		title = fallbackTitle
	}
	writeJSON(w, http.StatusOK, generateConversationTitleResponse{Title: title})
}

func (h *Handler) generateRelatedQuestions(w http.ResponseWriter, r *http.Request, conversationID string) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}

	var request generateRelatedQuestionsRequest
	if err := decodeJSONWithForbiddenFields(w, r, &request, forbiddenRelatedQuestionsFields()); err != nil {
		writeRequestDecodeError(w, err)
		return
	}

	messages, err := h.service.ListMessages(r.Context(), conversationID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if h.provider == nil || request.ModelRef == nil {
		writeJSON(w, http.StatusOK, generateRelatedQuestionsResponse{Questions: []string{}})
		return
	}

	questions, err := generateRelatedQuestionsWithProvider(r.Context(), h.provider, *request.ModelRef, messages)
	if err != nil {
		questions = []string{}
	}
	writeJSON(w, http.StatusOK, generateRelatedQuestionsResponse{Questions: questions})
}

func (h *Handler) handleMessageResource(
	w http.ResponseWriter,
	r *http.Request,
	conversationID string,
	messageID string,
) {
	switch r.Method {
	case http.MethodPatch:
		h.updateMessage(w, r, conversationID, messageID)
	case http.MethodDelete:
		h.deleteMessage(w, r, conversationID, messageID)
	default:
		methodNotAllowed(w, http.MethodPatch+", "+http.MethodDelete)
	}
}

func (h *Handler) updateMessage(w http.ResponseWriter, r *http.Request, conversationID string, messageID string) {
	if err := h.service.requireRepository(); err != nil {
		writeServiceError(w, err)
		return
	}

	var request updateMessageRequest
	if err := decodeJSONWithForbiddenFields(w, r, &request, forbiddenMessageUpdateFields()); err != nil {
		writeRequestDecodeError(w, err)
		return
	}

	message, err := h.service.UpdateMessage(r.Context(), conversationID, messageID, UpdateMessageInput{
		Content: request.Content,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newMessageDTO(message))
}

func (h *Handler) deleteMessage(w http.ResponseWriter, r *http.Request, conversationID string, messageID string) {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	input := DeleteMessageInput{}
	switch scope {
	case "", "message":
	case "subsequent":
		input.DeleteSubsequent = true
	default:
		writeError(w, http.StatusBadRequest, "INVALID_DELETE_SCOPE", "delete scope must be message or subsequent")
		return
	}

	if err := h.service.DeleteMessage(r.Context(), conversationID, messageID, input); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) cancelRun(w http.ResponseWriter, r *http.Request, runID string) {
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
	h.markRunCancelled(context.Background(), runID)

	writeJSON(w, http.StatusOK, cancelRunResponse{
		RunID:   runID,
		Status:  message.Status,
		Message: newMessageDTO(message),
	})
}

func (h *Handler) handleConversationChild(w http.ResponseWriter, r *http.Request) {
	if conversationID, messageID, ok := parseConversationMessageResourcePath(r.URL.Path); ok {
		h.handleMessageResource(w, r, conversationID, messageID)
		return
	}

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
	if child == "duplicate" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.duplicateConversation(w, r, conversationID)
		return
	}
	if child == "title" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.generateConversationTitle(w, r, conversationID)
		return
	}
	if child == "related-questions" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.generateRelatedQuestions(w, r, conversationID)
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
		Attachments:     newAttachmentInputs(request.Attachments),
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
	ragSelection, err := h.resolveConversationRAGSelection(
		r.Context(),
		conversationID,
		request.Config,
		request.Metadata,
		userMessage.Metadata,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if isImageGenerationModel(modelRef.ModelID) {
		h.streamImageGeneration(
			w,
			r,
			flusher,
			conversationID,
			userMessage,
			*modelRef,
			request.IdempotencyKey,
		)
		return
	}

	streamProvider, err := h.resolveStreamProvider(r.Context(), request.Provider)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if resolver, ok := streamProvider.(ModelRefResolver); ok {
		resolvedModelRef, err := resolver.ResolveModelRef(*modelRef)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		modelRef = &resolvedModelRef
	} else if validator, ok := streamProvider.(ModelRefValidator); ok {
		if err := validator.ValidateModelRef(*modelRef); err != nil {
			writeServiceError(w, err)
			return
		}
	}
	providerAttachments, err := h.resolveProviderAttachments(r.Context(), userMessage)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	autoDecision := autoRAGDecision{}
	knowledgeStarted := time.Now()
	if ragSelection.Enabled {
		autoDecision = h.decideAutoRAG(
			r.Context(),
			conversationID,
			userMessage,
			modelRef,
			ragSelection,
			streamProvider,
		)
	}
	providerPrompt := userMessage.Content
	providerSystemPrompt := systemPrompt
	providerMetadata := request.Metadata
	if ragSelection.Enabled && autoDecision.ReadyForAnswer() {
		providerPrompt, providerSystemPrompt, err = buildAutoRAGProviderRequest(
			userMessage.Content,
			systemPrompt,
			autoDecision.Evidence,
			autoDecision.Citations,
		)
		if err != nil {
			autoDecision = autoRAGDecision{Outcome: "dependency_unavailable"}
			providerPrompt = userMessage.Content
			providerSystemPrompt = systemPrompt
		} else {
			providerMetadata = mergeAutoRAGProviderMetadata(
				request.Metadata,
				autoDecision,
			)
		}
	}
	knowledgeDurationMillis := int64(0)
	if ragSelection.Enabled {
		knowledgeDurationMillis = sourceFusionDurationMillis(knowledgeStarted)
	}
	routerStarted := time.Now()
	fusionPlan := planSourceFusion(
		userMessage.Content,
		configBool(request.Config, "useSearch"),
		autoDecision,
	)
	fusionDiagnostics := newSourceFusionDiagnostics(fusionPlan)
	fusionDiagnostics.KnowledgeDurationMillis = knowledgeDurationMillis
	fusionDiagnostics.RouterDurationMillis = sourceFusionDurationMillis(routerStarted)
	resolveStarted := time.Now()
	searchExecution, modelBuiltInSearchProvider, searchErr := h.resolveChatSearchExecution(
		r.Context(),
		streamProvider,
		fusionPlan.SearchRequested,
	)
	if fusionPlan.SearchRequested {
		fusionDiagnostics.WebResolveDurationMillis = sourceFusionDurationMillis(resolveStarted)
	}
	if searchErr != nil {
		fusionDiagnostics.WebResolveOutcome = "degraded"
		fusionDiagnostics.WebExecuteOutcome = "not_run"
		fusionDiagnostics.DegradationReason = sourceSearchDegradationReason(searchErr)
		fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
		searchExecution = nil
		modelBuiltInSearchProvider = nil
	} else if searchExecution != nil {
		fusionDiagnostics.WebResolveOutcome = "resolved"
	}
	webSearchResult := websearch.Result{Sources: []websearch.Source{}, Images: []websearch.Image{}}
	if searchExecution != nil && searchExecution.Mode == websearch.ExecutionExternal {
		searchQuery, derived := buildFusionWebSearchQuery(
			userMessage.Content,
			fusionPlan,
			autoDecision,
		)
		fusionDiagnostics.WebQueryDerived = derived
		executeStarted := time.Now()
		webSearchResult, searchErr = h.webSearchService.Execute(
			r.Context(),
			*searchExecution,
			websearch.Request{
				Query:      searchQuery,
				MaxResults: configIntRange(request.Config, "searchResultsLimit", 5, 1, websearch.MaxResults),
			},
		)
		fusionDiagnostics.WebExecuteDurationMillis = sourceFusionDurationMillis(executeStarted)
		if searchErr != nil {
			fusionDiagnostics.WebExecuteOutcome = "degraded"
			fusionDiagnostics.DegradationReason = sourceSearchDegradationReason(searchErr)
			fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
			webSearchResult = websearch.Result{
				Sources: []websearch.Source{}, Images: []websearch.Image{},
			}
		} else {
			webSearchResult, _ = prepareWebSearchResult(webSearchResult)
			if len(webSearchResult.Sources) == 0 {
				fusionDiagnostics.WebExecuteOutcome = "no_results"
				fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
			} else {
				fusionDiagnostics.WebExecuteOutcome = "completed"
			}
		}
	} else if searchExecution != nil &&
		searchExecution.Mode == websearch.ExecutionModelBuiltIn {
		fusionDiagnostics.WebExecuteOutcome = "provider_stream"
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

	if searchExecution != nil && searchExecution.Mode == websearch.ExecutionExternal {
		providerPrompt, providerSystemPrompt = buildWebSearchProviderRequest(
			providerPrompt,
			providerSystemPrompt,
			webSearchResult,
		)
	}
	providerSystemPrompt, memoryPreparation := h.prepareDurableMemory(
		r.Context(),
		userMessage.Content,
		providerSystemPrompt,
	)
	providerSystemPromptWithoutFusion := providerSystemPrompt
	providerSystemPrompt = applySourceFusionSystemInstruction(
		providerSystemPrompt,
		fusionPlan,
	)
	conversationMessages, err := h.service.ListMessages(r.Context(), conversationID)
	if err != nil {
		h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
			Status:   "failed",
			Metadata: map[string]any{"runId": runID, "errorCode": "CONTEXT_READ_FAILED"},
		})
		writeServiceError(w, err)
		return
	}
	providerMessages := buildProviderConversationMessages(
		conversationMessages,
		userMessage.ID,
		providerPrompt,
		providerAttachments,
	)
	contextPreparation := h.prepareConversationContext(
		r.Context(),
		conversationID,
		*modelRef,
		streamProvider,
		providerSystemPrompt,
		providerMessages,
	)
	providerMessages = contextPreparation.Messages
	providerSystemPrompt = contextPreparation.SystemPrompt
	webMessageMetadata := func(decision autoRAGDecision, extra map[string]any) map[string]any {
		metadata := withWebSearchMessageMetadata(
			autoRAGMessageMetadata(runID, ragSelection, decision, extra),
			searchExecution,
			webSearchResult,
		)
		metadata = withSourceFusionMessageMetadata(
			metadata,
			fusionPlan,
			decision,
			fusionDiagnostics,
		)
		metadata = withConversationContextMetadata(metadata, contextPreparation)
		return withDurableMemoryMetadata(metadata, memoryPreparation)
	}

	streamCtx, streamCancel := context.WithCancel(r.Context())
	unregisterRun := h.activeRuns.register(runID, streamCancel)
	stopCancellationWatch := watchRunCancellation(streamCtx, h.cancellationRuns, runID, streamCancel)
	defer unregisterRun()
	defer h.clearRunCancelled(context.Background(), runID)
	defer stopCancellationWatch()
	defer streamCancel()

	useReasoning := configBool(request.Config, "useReasoning")
	providerRequest := ProviderRequest{
		RunID:              runID,
		ConversationID:     conversationID,
		UserMessageID:      userMessage.ID,
		AssistantMessageID: assistantMessage.ID,
		Prompt:             providerPrompt,
		SystemPrompt:       providerSystemPrompt,
		Messages:           providerMessages,
		Attachments:        providerAttachments,
		UseReasoning:       useReasoning,
		ReasoningEffort:    reasoningEffortFromConfig(request.Config, useReasoning),
		ModelRef:           *modelRef,
		Metadata:           providerMetadata,
	}
	var events <-chan ProviderEvent
	var builtInSearchStarted time.Time
	if modelBuiltInSearchProvider != nil {
		builtInSearchStarted = time.Now()
		events, err = modelBuiltInSearchProvider.StreamChatWithModelBuiltInSearch(
			streamCtx,
			providerRequest,
		)
		if err != nil && streamCtx.Err() == nil && r.Context().Err() == nil &&
			!errors.Is(err, context.Canceled) {
			fusionDiagnostics.WebExecuteDurationMillis =
				sourceFusionDurationMillis(builtInSearchStarted)
			fusionDiagnostics.WebExecuteOutcome = "degraded"
			fusionDiagnostics.DegradationReason = "provider_failed"
			fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
			searchExecution = nil
			modelBuiltInSearchProvider = nil
			providerRequest.SystemPrompt = providerSystemPromptWithoutFusion
			if contextPreparation.UsesSummary {
				providerRequest.SystemPrompt = appendContextSummaryRuntimeInstruction(
					providerRequest.SystemPrompt,
				)
			}
			events, err = streamProvider.StreamChat(streamCtx, providerRequest)
		}
	} else {
		events, err = streamProvider.StreamChat(streamCtx, providerRequest)
	}
	if err != nil {
		if streamCtx.Err() != nil || r.Context().Err() != nil || errors.Is(err, context.Canceled) {
			h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
				Status:       "cancelled",
				OutputBlocks: webSearchOutputBlocks(assistantMessage.ID, webSearchResult),
				Metadata:     webMessageMetadata(autoDecision, nil),
			})
			return
		}
		h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
			Status:       "failed",
			OutputBlocks: webSearchOutputBlocks(assistantMessage.ID, webSearchResult),
			Metadata:     webMessageMetadata(autoDecision, map[string]any{"errorCode": "PROVIDER_ERROR"}),
		})
		writeError(w, http.StatusBadGateway, "PROVIDER_ERROR", "provider stream failed")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
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
	if searchExecution != nil && searchExecution.Mode == websearch.ExecutionExternal &&
		(len(webSearchResult.Sources) > 0 || len(webSearchResult.Images) > 0) {
		sequence++
		if err := writeSSEEvent(w, "search.results", streamEvent{
			Type:           "search.results",
			RunID:          runID,
			ConversationID: conversationID,
			MessageID:      assistantMessage.ID,
			Sequence:       sequence,
			CreatedAt:      formatTime(time.Now()),
			Results:        &webSearchResult,
		}); err != nil {
			h.cancelAssistantAfterWriteError(conversationID, assistantMessage.ID, runID, "")
			return
		}
		flusher.Flush()
	}

	var content strings.Builder
	for providerEvent := range events {
		if providerEvent.Error != nil {
			if modelBuiltInSearchProvider != nil && streamCtx.Err() == nil &&
				!errors.Is(providerEvent.Error, context.Canceled) {
				fusionDiagnostics.WebExecuteDurationMillis =
					sourceFusionDurationMillis(builtInSearchStarted)
				fusionDiagnostics.WebExecuteOutcome = "degraded"
				fusionDiagnostics.DegradationReason = "provider_failed"
				fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
			}
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
					Status:       "cancelled",
					Content:      content.String(),
					OutputBlocks: webSearchOutputBlocks(assistantMessage.ID, webSearchResult),
					Metadata:     webMessageMetadata(autoDecision, nil),
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
				Status:       "failed",
				Content:      content.String(),
				OutputBlocks: webSearchOutputBlocks(assistantMessage.ID, webSearchResult),
				Metadata:     webMessageMetadata(autoDecision, map[string]any{"errorCode": "PROVIDER_ERROR"}),
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
		case ProviderEventSearch:
			if providerEvent.Search == nil || len(providerEvent.Search.Sources) == 0 {
				continue
			}
			if modelBuiltInSearchProvider != nil {
				fusionDiagnostics.WebExecuteDurationMillis =
					sourceFusionDurationMillis(builtInSearchStarted)
				fusionDiagnostics.WebExecuteOutcome = "completed"
			}
			webSearchResult = mergeWebSearchResults(webSearchResult, *providerEvent.Search)
			sequence++
			if err := writeSSEEvent(w, "search.results", streamEvent{
				Type:           "search.results",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
				Results:        &webSearchResult,
			}); err != nil {
				h.cancelAssistantAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
				return
			}
			flusher.Flush()
		}
	}

	if streamCtx.Err() != nil || h.isRunCancelled(context.Background(), runID) {
		streamCancel()
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
			Status:       "cancelled",
			Content:      content.String(),
			OutputBlocks: webSearchOutputBlocks(assistantMessage.ID, webSearchResult),
			Metadata:     webMessageMetadata(autoDecision, nil),
		})
		flusher.Flush()
		return
	}
	if modelBuiltInSearchProvider != nil &&
		fusionDiagnostics.WebExecuteOutcome == "provider_stream" {
		fusionDiagnostics.WebExecuteDurationMillis =
			sourceFusionDurationMillis(builtInSearchStarted)
		fusionDiagnostics.WebExecuteOutcome = "no_results"
		fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
	}
	if searchExecution != nil && searchExecution.Mode == websearch.ExecutionModelBuiltIn {
		if delta := missingBuiltInWebCitationDelta(content.String(), webSearchResult); delta != "" {
			content.WriteString(delta)
			sequence++
			if err := writeSSEEvent(w, "message.delta", streamEvent{
				Type:           "message.delta",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
				Delta:          delta,
			}); err != nil {
				h.cancelAssistantAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
				return
			}
			flusher.Flush()
		}
	}

	completedDecision := autoDecision.completed(content.String())
	fusionPlan = reconcileCompletedSourceFusionAuthority(
		fusionPlan,
		content.String(),
		completedDecision,
		webSearchResult,
	)
	assistantMessage, err = h.finalizeAssistantMessage(
		context.Background(),
		conversationID,
		assistantMessage.ID,
		FinalizeAssistantMessageInput{
			Status:       "completed",
			Content:      content.String(),
			OutputBlocks: webSearchOutputBlocks(assistantMessage.ID, webSearchResult),
			Metadata:     webMessageMetadata(completedDecision, nil),
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
	h.queueDurableMemoryExtraction(
		r.Context(),
		streamProvider,
		*modelRef,
		conversationID,
		userMessage.ID,
		userMessage.Content,
	)
}

func (h *Handler) resolveConversationRAGSelection(
	ctx context.Context,
	conversationID string,
	requestConfig map[string]any,
	requestMetadata map[string]any,
	userMessageMetadata map[string]any,
) (ragSelection, error) {
	conversation, err := h.service.GetConversation(ctx, conversationID)
	if err != nil {
		return ragSelection{}, err
	}
	selection, bindingPresent, err := extractConversationRAGSelection(conversation.Metadata)
	if err != nil {
		return ragSelection{}, err
	}
	if bindingPresent {
		return selection, nil
	}

	legacySelection, err := extractRAGSelection(requestConfig, requestMetadata)
	if err != nil {
		return ragSelection{}, err
	}
	if !legacySelection.Enabled {
		legacySelection, err = extractRAGSelection(nil, userMessageMetadata)
		if err != nil {
			return ragSelection{}, err
		}
	}
	if !legacySelection.Enabled {
		return ragSelection{}, nil
	}

	updated, err := h.service.UpdateConversation(ctx, conversationID, UpdateConversationInput{
		MetadataMerge: map[string]any{
			conversationKnowledgeSelectionKey: legacySelection.CollectionIDs,
		},
	})
	if err != nil {
		return ragSelection{}, err
	}
	migratedSelection, _, err := extractConversationRAGSelection(updated.Metadata)
	if err != nil {
		return ragSelection{}, err
	}
	return migratedSelection, nil
}

func isImageGenerationModel(modelID string) bool {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	return strings.HasPrefix(modelID, "gpt-image-") ||
		strings.HasPrefix(modelID, "dall-e-") ||
		strings.HasPrefix(modelID, "imagen-")
}

func (h *Handler) streamImageGeneration(
	w http.ResponseWriter,
	r *http.Request,
	flusher http.Flusher,
	conversationID string,
	userMessage Message,
	modelRef ModelRef,
	idempotencyKey string,
) {
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
				"runId": runID,
				"kind":  "image_generation",
			},
			IdempotencyKey: idempotencyKey,
		},
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
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
		ModelRef:       &modelRef,
	}); err != nil {
		h.cancelAssistantAfterWriteError(conversationID, assistantMessage.ID, runID, "")
		return
	}
	flusher.Flush()

	if h.imageGenerator == nil {
		h.failImageGenerationStream(w, flusher, conversationID, assistantMessage, runID, sequence, "IMAGE_JOBS_UNAVAILABLE")
		return
	}

	streamCtx, streamCancel := context.WithCancel(r.Context())
	unregisterRun := h.activeRuns.register(runID, streamCancel)
	stopCancellationWatch := watchRunCancellation(streamCtx, h.cancellationRuns, runID, streamCancel)
	defer unregisterRun()
	defer h.clearRunCancelled(context.Background(), runID)
	defer stopCancellationWatch()
	defer streamCancel()

	result, err := h.imageGenerator.GenerateImage(streamCtx, ImageGenerationRequest{
		ModelRef: modelRef,
		Prompt:   userMessage.Content,
		Size:     defaultChatImageSize,
	})
	if err != nil {
		if streamCtx.Err() != nil || errors.Is(err, context.Canceled) {
			h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
				Status: "cancelled",
				Metadata: map[string]any{
					"runId":     runID,
					"kind":      "image_generation",
					"errorCode": "REQUEST_CANCELLED",
				},
			})
			sequence++
			_ = writeSSEEvent(w, "message.cancelled", streamEvent{
				Type:           "message.cancelled",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
			})
			flusher.Flush()
			return
		}
		h.failImageGenerationStream(
			w,
			flusher,
			conversationID,
			assistantMessage,
			runID,
			sequence,
			imageGenerationStreamErrorCode(err),
		)
		return
	}
	if len(result.Attachments) == 0 {
		h.failImageGenerationStream(w, flusher, conversationID, assistantMessage, runID, sequence, "IMAGE_RESPONSE_EMPTY")
		return
	}

	attachments := make([]AttachmentInput, 0, len(result.Attachments))
	for _, attachment := range result.Attachments {
		attachments = append(attachments, AttachmentInput{
			Source:  "server",
			FileID:  attachment.FileID,
			Purpose: nonEmptyImagePurpose(attachment.Purpose),
		})
	}
	assistantMessage, err = h.finalizeAssistantMessage(
		context.Background(),
		conversationID,
		assistantMessage.ID,
		FinalizeAssistantMessageInput{
			Status:      "completed",
			Content:     "",
			Attachments: attachments,
			Metadata: map[string]any{
				"runId":      runID,
				"kind":       "image_generation",
				"imageCount": len(attachments),
			},
		},
	)
	if err != nil {
		h.failImageGenerationStream(w, flusher, conversationID, assistantMessage, runID, sequence, "IMAGE_PERSIST_FAILED")
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

func (h *Handler) failImageGenerationStream(
	w http.ResponseWriter,
	flusher http.Flusher,
	conversationID string,
	assistantMessage Message,
	runID string,
	sequence int,
	errorCode string,
) {
	h.finalizeAssistantMessage(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
		Status: "failed",
		Metadata: map[string]any{
			"runId":     runID,
			"kind":      "image_generation",
			"errorCode": errorCode,
		},
	})
	sequence++
	_ = writeSSEEvent(w, "message.error", streamEvent{
		Type:           "message.error",
		RunID:          runID,
		ConversationID: conversationID,
		MessageID:      assistantMessage.ID,
		Sequence:       sequence,
		CreatedAt:      formatTime(time.Now()),
		Error:          &ErrorBody{Code: errorCode, Message: imageGenerationStreamErrorMessage(errorCode)},
	})
	flusher.Flush()
}

func imageGenerationStreamErrorCode(err error) string {
	var imageErr *ImageGenerationError
	if !errors.As(err, &imageErr) {
		return "IMAGE_PROVIDER_ERROR"
	}
	switch strings.TrimSpace(imageErr.Code) {
	case ImageContentPolicyViolationCode:
		return ImageContentPolicyViolationCode
	case ImageProviderConnectionCode:
		return ImageProviderConnectionCode
	case ImageProviderTimeoutCode:
		return ImageProviderTimeoutCode
	default:
		return "IMAGE_PROVIDER_ERROR"
	}
}

func imageGenerationStreamErrorMessage(errorCode string) string {
	switch errorCode {
	case ImageContentPolicyViolationCode:
		return "image request was rejected by provider content policy"
	case ImageProviderConnectionCode:
		return "image provider connection failed after retry"
	case ImageProviderTimeoutCode:
		return "image provider timed out"
	default:
		return "image generation failed"
	}
}

func nonEmptyImagePurpose(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "image"
}

type autoRAGDecision struct {
	Outcome        string
	Evidence       []knowledge.HydratedEvidence
	Citations      []RAGCitation
	Authority      *RAGAnswerAuthority
	QueryRewritten bool
	RerankStatus   string
}

func (d autoRAGDecision) ReadyForAnswer() bool {
	return d.Outcome == "evidence_ready" && len(d.Evidence) > 0 &&
		len(d.Citations) > 0 && d.Authority != nil
}

func (d autoRAGDecision) completed(content string) autoRAGDecision {
	if !d.ReadyForAnswer() {
		return d
	}

	usedCitations := make([]RAGCitation, 0, len(d.Citations))
	for _, citation := range d.Citations {
		if strings.Contains(content, citation.Marker) {
			usedCitations = append(usedCitations, citation)
		}
	}
	d.Evidence = nil
	d.Citations = usedCitations
	if len(usedCitations) == 0 {
		d.Outcome = "answered_without_knowledge"
		d.Authority = nil
		return d
	}
	d.Outcome = "answered"
	return d
}

func (h *Handler) decideAutoRAG(
	ctx context.Context,
	conversationID string,
	userMessage Message,
	modelRef *ModelRef,
	selection ragSelection,
	provider Provider,
) autoRAGDecision {
	session, ok := auth.SessionFromContext(ctx)
	if !ok {
		return autoRAGDecision{Outcome: "dependency_unavailable"}
	}
	rewrittenQuery := ""
	if shouldRewriteRAGQuery(userMessage.Content) {
		if messages, listErr := h.service.ListMessages(ctx, conversationID); listErr == nil {
			rewrittenQuery, _ = rewriteRAGQuery(
				ctx,
				provider,
				*modelRef,
				userMessage.ID,
				userMessage.Content,
				messages,
			)
		}
	}
	result, err := h.ragAssembler.Assemble(ctx, RAGAssemblyInput{
		ActorUserID:           session.UserID,
		SessionID:             session.ID,
		ConversationID:        conversationID,
		QueryText:             userMessage.Content,
		RewrittenQueryText:    rewrittenQuery,
		SelectedCollectionIDs: selection.CollectionIDs,
	})
	if err != nil {
		if errors.Is(err, ErrRAGInsufficientEvidence) {
			return autoRAGDecision{Outcome: "no_evidence"}
		}
		return autoRAGDecision{Outcome: "dependency_unavailable"}
	}
	if len(result.Evidence) == 0 || len(result.Citations) == 0 {
		return autoRAGDecision{Outcome: "no_evidence"}
	}
	authority, err := h.authorizeAutoRAGAnswer(ctx, selection, modelRef, result.Citations)
	if err != nil {
		if errors.Is(err, ErrRAGAnswerGovernanceRequired) {
			return autoRAGDecision{Outcome: "answer_governance_required"}
		}
		return autoRAGDecision{Outcome: "dependency_unavailable"}
	}
	return autoRAGDecision{
		Outcome:        "evidence_ready",
		Evidence:       result.Evidence,
		Citations:      result.Citations,
		Authority:      &authority,
		QueryRewritten: rewrittenQuery != "",
		RerankStatus:   result.RerankStatus,
	}
}

func (h *Handler) authorizeAutoRAGAnswer(
	ctx context.Context,
	selection ragSelection,
	modelRef *ModelRef,
	citations []RAGCitation,
) (RAGAnswerAuthority, error) {
	if h == nil || h.ragAnswerGate == nil || modelRef == nil {
		return RAGAnswerAuthority{}, ErrRAGDependencyUnavailable
	}
	return h.ragAnswerGate.AuthorizeRAGAnswer(ctx, RAGAnswerGovernanceInput{
		ModelRef:              *modelRef,
		SelectedCollectionIDs: append([]string(nil), selection.CollectionIDs...),
		Citations:             append([]RAGCitation(nil), citations...),
	})
}

func mergeAutoRAGProviderMetadata(
	base map[string]any,
	decision autoRAGDecision,
) map[string]any {
	metadata := make(map[string]any, len(base)+1)
	for key, value := range base {
		metadata[key] = value
	}
	metadata["knowledge"] = autoRAGAnswerProviderMetadata(decision)
	return metadata
}

func autoRAGMessageMetadata(
	runID string,
	selection ragSelection,
	decision autoRAGDecision,
	extra map[string]any,
) map[string]any {
	metadata := map[string]any{"runId": runID}
	for key, value := range extra {
		metadata[key] = value
	}
	if !selection.Enabled {
		return metadata
	}
	knowledgeMetadata := map[string]any{
		"mode":                  "auto",
		"outcome":               decision.Outcome,
		"selectedCollectionIds": append([]string(nil), selection.CollectionIDs...),
		"citationCount":         len(decision.Citations),
		"evidenceUsed":          len(decision.Citations) > 0,
		"queryRewritten":        decision.QueryRewritten,
		"rerankStatus":          decision.RerankStatus,
	}
	if len(decision.Citations) > 0 {
		knowledgeMetadata["citations"] = append([]RAGCitation(nil), decision.Citations...)
	}
	if decision.Authority != nil {
		knowledgeMetadata["answerGovernance"] = *decision.Authority
	}
	if decision.Outcome == "dependency_unavailable" ||
		decision.Outcome == "answer_governance_required" {
		knowledgeMetadata["degradationReason"] = decision.Outcome
	}
	metadata["knowledge"] = knowledgeMetadata
	return metadata
}

func (h *Handler) resolveStreamProvider(
	ctx context.Context,
	providerConfig *runtimeconfig.ProviderRuntimeConfig,
) (Provider, error) {
	if providerConfig == nil {
		if h.provider == nil {
			return nil, ErrProviderRequired
		}
		return h.provider, nil
	}
	if strings.TrimSpace(providerConfig.Source) == "server-default" {
		if h.providerResolver != nil {
			return h.providerResolver.ResolveRuntimeProvider(ctx, *providerConfig)
		}
		if h.provider == nil {
			return nil, ErrProviderRequired
		}
		return h.provider, nil
	}
	if strings.TrimSpace(providerConfig.Source) == "" &&
		strings.TrimSpace(providerConfig.ID) == "" &&
		strings.TrimSpace(providerConfig.Type) == "" &&
		strings.TrimSpace(providerConfig.BaseURL) == "" &&
		len(providerConfig.APIKeySecret) == 0 {
		if h.provider == nil {
			return nil, ErrProviderRequired
		}
		return h.provider, nil
	}
	if h.providerResolver == nil {
		return nil, newValidationError(
			"PROVIDER_CONFIG_UNSUPPORTED",
			"runtime provider configuration is not supported",
		)
	}
	return h.providerResolver.ResolveRuntimeProvider(ctx, *providerConfig)
}

func (h *Handler) resolveChatSearchExecution(
	ctx context.Context,
	provider Provider,
	searchEnabled bool,
) (*websearch.ActiveExecution, ModelBuiltInSearchProvider, error) {
	if !searchEnabled {
		return nil, nil, nil
	}
	if h == nil || h.webSearchService == nil || !h.webSearchService.Configured() {
		return nil, nil, websearch.ErrNotConfigured
	}
	execution, err := h.webSearchService.ResolveActive(ctx)
	if err != nil {
		return nil, nil, err
	}
	if execution.Mode != websearch.ExecutionModelBuiltIn {
		return &execution, nil, nil
	}
	builtIn, ok := provider.(ModelBuiltInSearchProvider)
	if !ok || builtIn.ModelBuiltInSearchID() != execution.ModelBuiltIn {
		return nil, nil, errModelBuiltInSearchUnsupported
	}
	return &execution, builtIn, nil
}

func (h *Handler) resolveProviderAttachments(ctx context.Context, message Message) ([]ProviderAttachment, error) {
	if len(message.Attachments) == 0 {
		return nil, nil
	}

	providerAttachments := make([]ProviderAttachment, 0, len(message.Attachments))
	for _, attachment := range message.Attachments {
		if !isProviderImageAttachment(attachment) {
			continue
		}
		if h.attachmentResolver == nil {
			return nil, newValidationError(
				"ATTACHMENT_CONTENT_UNAVAILABLE",
				"image attachment content is not available for provider streaming",
			)
		}
		providerAttachment, err := h.attachmentResolver.ResolveProviderAttachment(ctx, attachment)
		if err != nil {
			return nil, err
		}
		if len(providerAttachment.Data) == 0 {
			return nil, newValidationError(
				"ATTACHMENT_CONTENT_EMPTY",
				"image attachment content is empty",
			)
		}
		if strings.TrimSpace(providerAttachment.FileID) == "" {
			providerAttachment.FileID = attachment.FileID
		}
		if strings.TrimSpace(providerAttachment.FileName) == "" {
			providerAttachment.FileName = attachment.FileName
		}
		if strings.TrimSpace(providerAttachment.MimeType) == "" {
			providerAttachment.MimeType = attachment.MimeType
		}
		if providerAttachment.Size == 0 {
			providerAttachment.Size = attachment.Size
		}
		if strings.TrimSpace(providerAttachment.SHA256) == "" {
			providerAttachment.SHA256 = attachment.SHA256
		}
		if strings.TrimSpace(providerAttachment.Purpose) == "" {
			providerAttachment.Purpose = attachment.Purpose
		}
		providerAttachments = append(providerAttachments, providerAttachment)
	}
	return providerAttachments, nil
}

func isProviderImageAttachment(attachment Attachment) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(attachment.MimeType)), "image/")
}

func configBool(config map[string]any, key string) bool {
	value, ok := config[key].(bool)
	return ok && value
}

func configIntRange(config map[string]any, key string, fallback int, minimum int, maximum int) int {
	value, ok := config[key].(float64)
	if !ok || value != float64(int(value)) {
		return fallback
	}
	integer := int(value)
	if integer < minimum || integer > maximum {
		return fallback
	}
	return integer
}

func boundedWebSearchQuery(value string) string {
	return truncateWebUTF8(value, websearch.MaxQueryBytes)
}

func generateTitleWithProvider(
	ctx context.Context,
	provider Provider,
	modelRef ModelRef,
	messages []Message,
	fallbackTitle string,
) (string, error) {
	events, err := provider.StreamChat(ctx, ProviderRequest{
		Prompt:       buildTitlePrompt(messages),
		SystemPrompt: "You generate short, useful chat titles.",
		ModelRef:     modelRef,
	})
	if err != nil {
		return fallbackTitle, err
	}

	var builder strings.Builder
	for event := range events {
		if event.Error != nil {
			return fallbackTitle, event.Error
		}
		if event.Type == ProviderEventDelta {
			builder.WriteString(event.Delta)
			if builder.Len() > 1024 {
				break
			}
		}
	}

	return normalizeGeneratedTitle(builder.String(), fallbackTitle), nil
}

func generateRelatedQuestionsWithProvider(
	ctx context.Context,
	provider Provider,
	modelRef ModelRef,
	messages []Message,
) ([]string, error) {
	prompt, ok := buildRelatedQuestionsPrompt(messages)
	if !ok {
		return []string{}, nil
	}

	events, err := provider.StreamChat(ctx, ProviderRequest{
		Prompt:       prompt,
		SystemPrompt: "You generate short related follow-up questions.",
		ModelRef:     modelRef,
	})
	if err != nil {
		return []string{}, err
	}

	var builder strings.Builder
	for event := range events {
		if event.Error != nil {
			return []string{}, event.Error
		}
		if event.Type == ProviderEventDelta {
			builder.WriteString(event.Delta)
			if builder.Len() > 4096 {
				break
			}
		}
	}

	return parseAuxiliaryStringList(builder.String(), 5, 240), nil
}

func buildRelatedQuestionsPrompt(messages []Message) (string, bool) {
	userMessage, assistantMessage := latestRelatedQuestionMessages(messages)
	if userMessage == "" || assistantMessage == "" {
		return "", false
	}

	return fmt.Sprintf(
		"Based on the following conversation, suggest 3 to 5 related follow-up questions the user might want to ask.\nEach question must be short (less than 24 words).\nReturn the result as a JSON array of strings.\n\nUser: %q\nModel: %q",
		clipAuxiliaryPromptContent(userMessage),
		clipAuxiliaryPromptContent(assistantMessage),
	), true
}

func latestRelatedQuestionMessages(messages []Message) (string, string) {
	var assistantMessage string
	var assistantIndex = -1
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == "assistant" && strings.TrimSpace(message.Content) != "" {
			assistantMessage = message.Content
			assistantIndex = index
			break
		}
	}
	if assistantIndex < 0 {
		return "", ""
	}

	for index := assistantIndex - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			return message.Content, assistantMessage
		}
	}

	return "", ""
}

func buildTitlePrompt(messages []Message) string {
	userMessage, assistantMessage := firstTitleMessages(messages)
	userContent := clipTitlePromptContent(userMessage)
	assistantContent := clipTitlePromptContent(assistantMessage)

	return fmt.Sprintf(
		"Summarize the following conversation into a short, concise title (3-6 words).\nDo not use quotes.\nUse the same language as the user's question.\n\nUser: %q\nAI: %q\n\nTitle:",
		userContent,
		assistantContent,
	)
}

func firstTitleMessages(messages []Message) (string, string) {
	var firstUser string
	var firstAssistant string
	for _, message := range messages {
		if firstUser == "" && message.Role == "user" {
			firstUser = message.Content
		}
		if firstAssistant == "" && message.Role == "assistant" {
			firstAssistant = message.Content
		}
		if firstUser != "" && firstAssistant != "" {
			break
		}
	}

	return firstUser, firstAssistant
}

func fallbackTitleFromMessages(messages []Message) string {
	userMessage, _ := firstTitleMessages(messages)
	return normalizeGeneratedTitle(userMessage, "New Chat")
}

func clipTitlePromptContent(value string) string {
	value = strings.TrimSpace(value)
	const maxRunes = 1200
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func clipAuxiliaryPromptContent(value string) string {
	value = strings.TrimSpace(value)
	const maxRunes = 4000
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func normalizeGeneratedTitle(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	value = strings.Trim(value, "\"'`“”‘’")
	value = strings.TrimSpace(strings.Split(value, "\n")[0])
	value = strings.Trim(value, "\"'`“”‘’")
	value = strings.TrimSpace(value)
	if value == "" {
		value = "New Chat"
	}

	const maxRunes = 80
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}

func parseAuxiliaryStringList(value string, maxItems int, maxChars int) []string {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```JSON")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var parsed []string
	if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil {
		return normalizeAuxiliaryStringList(parsed, maxItems, maxChars)
	}

	return normalizeAuxiliaryStringList(strings.Split(cleaned, "\n"), maxItems, maxChars)
}

func normalizeAuxiliaryStringList(values []string, maxItems int, maxChars int) []string {
	if maxItems <= 0 || maxChars <= 0 {
		return []string{}
	}

	items := make([]string, 0, maxItems)
	seen := map[string]struct{}{}
	for _, raw := range values {
		item := stripAuxiliaryListMarker(raw)
		runes := []rune(item)
		if len(runes) > maxChars {
			item = string(runes[:maxChars])
		}
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}

		items = append(items, item)
		seen[key] = struct{}{}
		if len(items) >= maxItems {
			break
		}
	}

	if items == nil {
		return []string{}
	}
	return items
}

func stripAuxiliaryListMarker(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, "-*• \t")
	value = strings.TrimSpace(value)
	if dotIndex := strings.IndexAny(value, ".)"); dotIndex > 0 && dotIndex <= 3 {
		prefix := value[:dotIndex]
		allDigits := true
		for _, r := range prefix {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			value = strings.TrimSpace(value[dotIndex+1:])
		}
	}
	value = strings.Trim(value, "\"'")
	return strings.TrimSpace(value)
}

func parseConversationMessageResourcePath(path string) (string, string, bool) {
	remainder := strings.TrimPrefix(path, conversationPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "messages" || parts[2] == "" {
		return "", "", false
	}

	return parts[0], parts[2], true
}

func parseConversationResourcePath(path string) (string, bool) {
	remainder := strings.TrimPrefix(path, conversationPathBase)
	if remainder == "" || strings.Contains(remainder, "/") {
		return "", false
	}

	return remainder, true
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
	if errors.Is(err, ErrMessageNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "MESSAGE_NOT_FOUND", Message: "message not found"}
	}
	if errors.Is(err, ErrIdempotencyConflict) {
		return http.StatusConflict, ErrorBody{Code: "IDEMPOTENCY_CONFLICT", Message: "idempotency key already exists"}
	}
	if errors.Is(err, ErrFileNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "FILE_NOT_FOUND", Message: "file not found"}
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

func forbiddenConversationUpdateFields() map[string]fieldViolation {
	return forbiddenConversationFields()
}

func forbiddenConversationTitleFields() map[string]fieldViolation {
	fields := forbiddenConversationFields()
	fields["history"] = validationField("conversation title history is server-managed")
	fields["messages"] = validationField("conversation title history is server-managed")
	fields["provider"] = validationField("provider configuration is server-managed")
	fields["modelName"] = validationField("use modelRef instead of modelName")
	return fields
}

func forbiddenRelatedQuestionsFields() map[string]fieldViolation {
	fields := forbiddenConversationFields()
	fields["history"] = validationField("related-question history is server-managed")
	fields["messages"] = validationField("related-question history is server-managed")
	fields["provider"] = validationField("provider configuration is server-managed")
	fields["modelName"] = validationField("use modelRef instead of modelName")
	return fields
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

func forbiddenMessageUpdateFields() map[string]fieldViolation {
	fields := cloneFieldViolations(forbiddenMessageFields())
	violation := fieldViolation{
		Code:    "FORBIDDEN_MESSAGE_FIELD",
		Message: "message field is server-managed",
	}
	fields["role"] = violation
	fields["parentMessageId"] = violation
	fields["metadata"] = violation
	fields["idempotencyKey"] = violation
	fields["attachments"] = violation
	return fields
}

func cloneFieldViolations(fields map[string]fieldViolation) map[string]fieldViolation {
	cloned := make(map[string]fieldViolation, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
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

func (h *Handler) markRunCancelled(ctx context.Context, runID string) {
	if h == nil || h.cancellationRuns == nil || !isUUID(strings.TrimSpace(runID)) {
		return
	}
	_ = h.cancellationRuns.MarkRunCancelled(ctx, runID)
}

func (h *Handler) clearRunCancelled(ctx context.Context, runID string) {
	if h == nil || h.cancellationRuns == nil || !isUUID(strings.TrimSpace(runID)) {
		return
	}
	_ = h.cancellationRuns.ClearRunCancelled(ctx, runID)
}

func (h *Handler) isRunCancelled(ctx context.Context, runID string) bool {
	if h == nil || h.cancellationRuns == nil || !isUUID(strings.TrimSpace(runID)) {
		return false
	}
	cancelled, err := h.cancellationRuns.IsRunCancelled(ctx, runID)
	return err == nil && cancelled
}

func newConversationDTO(conversation Conversation) ConversationDTO {
	config := ensureObject(conversation.Metadata)
	return ConversationDTO{
		ID:                conversation.ID,
		Title:             conversation.Title,
		Status:            conversation.Status,
		ModelRef:          newModelRef(conversation.ModelProvider, conversation.ModelID),
		MessageCount:      conversation.MessageCount,
		SystemInstruction: conversation.SystemPrompt,
		Pinned:            configBool(config, "pinned"),
		Config:            config,
		CreatedAt:         formatTime(conversation.CreatedAt),
		UpdatedAt:         formatTime(conversation.UpdatedAt),
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
		Attachments:     newAttachmentDTOs(message.Attachments),
		OutputBlocks:    ensureArray(message.OutputBlocks),
		Metadata:        ensureObject(message.Metadata),
		ParentMessageID: message.ParentMessageID,
		CreatedAt:       formatTime(message.CreatedAt),
		UpdatedAt:       formatTime(message.UpdatedAt),
		CompletedAt:     formatOptionalTime(message.CompletedAt),
	}
}

func newAttachmentInputs(attachments []AttachmentDTO) []AttachmentInput {
	if len(attachments) == 0 {
		return nil
	}
	inputs := make([]AttachmentInput, 0, len(attachments))
	for _, attachment := range attachments {
		inputs = append(inputs, AttachmentInput{
			Source:  attachment.Source,
			FileID:  attachment.FileID,
			Purpose: attachment.Purpose,
		})
	}
	return inputs
}

func newAttachmentDTOs(attachments []Attachment) []AttachmentDTO {
	items := make([]AttachmentDTO, 0, len(attachments))
	for _, attachment := range attachments {
		items = append(items, AttachmentDTO{
			ID:       attachment.ID,
			Source:   "server",
			FileID:   attachment.FileID,
			FileName: attachment.FileName,
			MimeType: attachment.MimeType,
			Size:     attachment.Size,
			SHA256:   attachment.SHA256,
			Purpose:  attachment.Purpose,
		})
	}
	return items
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
