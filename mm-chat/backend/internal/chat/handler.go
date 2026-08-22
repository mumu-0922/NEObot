package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/hostworkspace"
	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/skillsupply"
	"neo-chat/mm-chat/backend/internal/usermemory"
	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	contentTypeJSON       = "application/json; charset=utf-8"
	maxRequestBodyBytes   = 1 << 20
	conversationsPath     = "/v1/chat/conversations"
	conversationPathBase  = conversationsPath + "/"
	generateTextPath      = "/v1/chat/generate"
	runsPathBase          = "/v1/chat/runs/"
	approvalsPathBase     = "/v1/chat/approvals/"
	agentEventsPathBase   = "/v1/chat/agent-events/"
	toolPlanPath          = "/v1/chat/tools/plan"
	maxToolPlanPrompt     = 16 * 1024
	maxGenerateTextPrompt = 128 * 1024
	maxGenerateTextOutput = 256 * 1024
	maxToolPlanTools      = 32
	maxToolNameBytes      = 128
	maxToolDescBytes      = 2048
	maxToolParamsBytes    = 32 * 1024
	defaultChatImageSize  = "1024x1024"

	ImageContentPolicyViolationCode = "IMAGE_CONTENT_POLICY_VIOLATION"
	ImageProviderConnectionCode     = "IMAGE_PROVIDER_CONNECTION_ERROR"
	ImageProviderTimeoutCode        = "IMAGE_PROVIDER_TIMEOUT"
)

type Handler struct {
	service                      *Service
	provider                     Provider
	attachmentResolver           ProviderAttachmentResolver
	providerResolver             RuntimeProviderResolver
	imageGenerator               ImageGenerator
	activeRuns                   *activeRunRegistry
	cancellationRuns             RunCancellationStore
	ragAssembler                 *RAGAnswerAssembler
	ragAnswerGate                RAGAnswerGovernanceGate
	knowledgeCatalogSource       KnowledgeRoutingCatalogSource
	knowledgeCatalogGate         KnowledgeRoutingCatalogGovernanceGate
	webSearchService             *websearch.Service
	userMemoryService            *usermemory.Service
	memoryLexicalShadowEnabled   bool
	memoryHybridShadowEnabled    bool
	memoryToolLoopEnabled        bool
	memoryToolLoopCanaryUserIDs  map[string]struct{}
	memoryL2SceneShadowEnabled   bool
	memoryL2SceneReaderEnabled   bool
	memoryL3PersonaShadowEnabled bool
	memoryL3PersonaReaderEnabled bool
	agentTimelineEnabled         bool
	agentTimelineCanaryUserIDs   map[string]struct{}
	memoryWakePublisher          MemoryWakePublisher
	memoryActionProviderResolver MemoryActionProviderResolver
	contextBudgetPolicy          contextBudgetPolicy
	toolCapabilityCache          ToolCapabilityCache
	toolCapabilityProbes         *toolCapabilityProbeGroup
	mcpService                   *mcpclient.Service
	localSkillCatalog            LocalSkillCatalog
	localSkillExecutor           *localskills.Executor
	hostWorkspaceService         hostWorkspaceExecutionService
	artifactPublisher            WorkspaceArtifactPublisher
	artifactMaxBytes             int64
	approvalWaiters              *chatAgentApprovalWaiters
}

type HandlerOption func(*Handler)

type ProviderAttachmentResolver interface {
	ResolveProviderAttachment(ctx context.Context, attachment Attachment) (ProviderAttachment, error)
}

type MemoryWakePublisher interface {
	PublishMemoryWake(ctx context.Context, eventID string) error
}

type RuntimeProviderResolver interface {
	ResolveRuntimeProvider(ctx context.Context, provider runtimeconfig.ProviderRuntimeConfig) (RuntimeProviderResolution, error)
}

type RuntimeProviderResolution struct {
	Provider                     Provider
	RAGAnswerProcessor           string
	ToolCapabilityPolicy         string
	ToolCapabilityModelOverrides map[string]string
	ToolCapabilityConfigHash     string
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
	WorkspaceID       string         `json:"workspaceId,omitempty"`
	PermissionMode    string         `json:"permissionMode"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
}

type ChatMessageDTO struct {
	ID              string           `json:"id"`
	ConversationID  string           `json:"conversationId"`
	SequenceNo      int              `json:"sequenceNo"`
	Role            string           `json:"role"`
	Status          string           `json:"status"`
	Content         string           `json:"content"`
	ModelRef        *ModelRef        `json:"modelRef,omitempty"`
	Attachments     []AttachmentDTO  `json:"attachments"`
	OutputBlocks    []any            `json:"outputBlocks"`
	Metadata        map[string]any   `json:"metadata"`
	AgentEvents     []ChatAgentEvent `json:"agentEvents,omitempty"`
	ParentMessageID string           `json:"parentMessageId,omitempty"`
	CreatedAt       string           `json:"createdAt"`
	UpdatedAt       string           `json:"updatedAt"`
	CompletedAt     string           `json:"completedAt,omitempty"`
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

type updateConversationPermissionRequest struct {
	PermissionMode         agenthost.PermissionMode `json:"permissionMode"`
	FullAccessAcknowledged bool                     `json:"fullAccessAcknowledged"`
}

type updateConversationMemoryPolicyRequest struct {
	ExpectedScopeGeneration int64  `json:"expectedScopeGeneration"`
	ProjectID               string `json:"projectId"`
	UseMode                 string `json:"useMode"`
	LearnMode               string `json:"learnMode"`
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
	UserMessageID           string                               `json:"userMessageId"`
	ContinuationOfMessageID string                               `json:"continuationOfMessageId"`
	ModelRef                *ModelRef                            `json:"modelRef"`
	Provider                *runtimeconfig.ProviderRuntimeConfig `json:"provider"`
	SystemInstruction       string                               `json:"systemInstruction"`
	SystemPrompt            string                               `json:"systemPrompt"`
	Config                  map[string]any                       `json:"config"`
	Metadata                map[string]any                       `json:"metadata"`
	IdempotencyKey          string                               `json:"idempotencyKey"`
}

type mcpPreflightRequest struct {
	ModelRef *ModelRef                            `json:"modelRef"`
	Provider *runtimeconfig.ProviderRuntimeConfig `json:"provider"`
}

type mcpPreflightResponse struct {
	Enabled bool `json:"enabled"`
}

type toolPlanRequest struct {
	Prompt   string           `json:"prompt"`
	ModelRef *ModelRef        `json:"modelRef"`
	Tools    []ToolDefinition `json:"tools"`
}

type toolPlanResponse struct {
	Calls []ToolCall `json:"calls"`
}

type generateTextRequest struct {
	ModelRef *ModelRef                            `json:"modelRef"`
	Provider *runtimeconfig.ProviderRuntimeConfig `json:"provider"`
	Prompt   string                               `json:"prompt"`
}

type generateTextResponse struct {
	Text string `json:"text"`
}

type streamEvent struct {
	Type           string                      `json:"type"`
	RunID          string                      `json:"runId"`
	ConversationID string                      `json:"conversationId"`
	MessageID      string                      `json:"messageId,omitempty"`
	Sequence       int                         `json:"sequence"`
	CreatedAt      string                      `json:"createdAt"`
	Role           string                      `json:"role,omitempty"`
	ModelRef       *ModelRef                   `json:"modelRef,omitempty"`
	Delta          string                      `json:"delta,omitempty"`
	Usage          *TokenUsage                 `json:"usage,omitempty"`
	Message        *ChatMessageDTO             `json:"message,omitempty"`
	Error          *ErrorBody                  `json:"error,omitempty"`
	Results        *websearch.Result           `json:"results,omitempty"`
	Step           *ProcessStep                `json:"step,omitempty"`
	ToolCall       *ProviderToolExecutionEvent `json:"toolCall,omitempty"`
	AgentEvent     *ChatAgentEvent             `json:"agentEvent,omitempty"`
}

type runStreamGapEvent struct {
	Type           string `json:"type"`
	RunID          string `json:"runId"`
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId,omitempty"`
	CreatedAt      string `json:"createdAt"`
	After          int    `json:"after"`
	OldestSequence int    `json:"oldestSequence"`
	LatestSequence int    `json:"latestSequence"`
	Reason         string `json:"reason"`
}

type cancelRunResponse struct {
	RunID   string         `json:"runId"`
	Status  string         `json:"status"`
	Message ChatMessageDTO `json:"message"`
}

type decideApprovalRequest struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	Decision         string `json:"decision"`
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

func WithKnowledgeRoutingCatalog(
	source KnowledgeRoutingCatalogSource,
	gate KnowledgeRoutingCatalogGovernanceGate,
) HandlerOption {
	return func(h *Handler) {
		h.knowledgeCatalogSource = source
		h.knowledgeCatalogGate = gate
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

func WithMemoryLexicalShadowEnabled(enabled bool) HandlerOption {
	return func(handler *Handler) {
		handler.memoryLexicalShadowEnabled = enabled
	}
}

func WithMemoryHybridShadowEnabled(enabled bool) HandlerOption {
	return func(handler *Handler) {
		handler.memoryHybridShadowEnabled = enabled
	}
}

func WithMemoryToolLoopEnabled(enabled bool) HandlerOption {
	return func(handler *Handler) {
		handler.memoryToolLoopEnabled = enabled
	}
}

func WithMemoryToolLoopCanaryUserIDs(userIDs []string) HandlerOption {
	return func(handler *Handler) {
		handler.memoryToolLoopCanaryUserIDs = make(map[string]struct{}, len(userIDs))
		for _, userID := range userIDs {
			userID = strings.ToLower(strings.TrimSpace(userID))
			if userID != "" {
				handler.memoryToolLoopCanaryUserIDs[userID] = struct{}{}
			}
		}
	}
}

func WithMemoryL2SceneShadowEnabled(enabled bool) HandlerOption {
	return func(handler *Handler) {
		handler.memoryL2SceneShadowEnabled = enabled
	}
}

func WithMemoryL2SceneReaderEnabled(enabled bool) HandlerOption {
	return func(handler *Handler) {
		handler.memoryL2SceneReaderEnabled = enabled
	}
}

func WithMemoryL3PersonaShadowEnabled(enabled bool) HandlerOption {
	return func(handler *Handler) {
		handler.memoryL3PersonaShadowEnabled = enabled
	}
}

func WithMemoryL3PersonaReaderEnabled(enabled bool) HandlerOption {
	return func(handler *Handler) {
		handler.memoryL3PersonaReaderEnabled = enabled
	}
}

func WithAgentTimelineCanary(enabled bool, userIDs []string) HandlerOption {
	return func(handler *Handler) {
		handler.agentTimelineEnabled = enabled
		handler.agentTimelineCanaryUserIDs = make(map[string]struct{}, len(userIDs))
		for _, userID := range userIDs {
			userID = strings.ToLower(strings.TrimSpace(userID))
			if userID != "" {
				handler.agentTimelineCanaryUserIDs[userID] = struct{}{}
			}
		}
	}
}

func (h *Handler) agentTimelineEnabledFor(userID string) bool {
	if h == nil || !h.agentTimelineEnabled {
		return false
	}
	_, allowed := h.agentTimelineCanaryUserIDs[strings.ToLower(strings.TrimSpace(userID))]
	return allowed
}

func WithMemoryWakePublisher(publisher MemoryWakePublisher) HandlerOption {
	return func(handler *Handler) {
		handler.memoryWakePublisher = publisher
	}
}

func WithMemoryActionProviderResolver(
	resolver MemoryActionProviderResolver,
) HandlerOption {
	return func(handler *Handler) {
		handler.memoryActionProviderResolver = resolver
	}
}

func WithRunCancellationStore(store RunCancellationStore) HandlerOption {
	return func(h *Handler) {
		h.cancellationRuns = store
	}
}

func WithToolCapabilityCache(cache ToolCapabilityCache) HandlerOption {
	return func(h *Handler) {
		h.toolCapabilityCache = cache
	}
}

func WithMCPService(service *mcpclient.Service) HandlerOption {
	return func(handler *Handler) {
		handler.mcpService = service
	}
}

func WithLocalSkillRuntime(
	catalog LocalSkillCatalog,
	executor *localskills.Executor,
) HandlerOption {
	return func(handler *Handler) {
		handler.localSkillCatalog = catalog
		handler.localSkillExecutor = executor
	}
}

func WithHostWorkspaceExecution(service *hostworkspace.Service) HandlerOption {
	return func(handler *Handler) {
		handler.hostWorkspaceService = service
	}
}

func WithWorkspaceArtifactPublisher(
	publisher WorkspaceArtifactPublisher,
	maxBytes int64,
) HandlerOption {
	return func(handler *Handler) {
		if publisher != nil && maxBytes > 0 {
			handler.artifactPublisher = publisher
			handler.artifactMaxBytes = maxBytes
		}
	}
}

func NewHandler(service *Service, opts ...HandlerOption) *Handler {
	if service == nil {
		service = NewService(nil)
	}

	handler := &Handler{
		service:              service,
		activeRuns:           newActiveRunRegistry(),
		approvalWaiters:      newChatAgentApprovalWaiters(),
		contextBudgetPolicy:  defaultContextBudgetPolicy(),
		toolCapabilityProbes: newToolCapabilityProbeGroup(),
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
	case strings.HasPrefix(r.URL.Path, approvalsPathBase):
		h.handleApprovalChild(w, r)
	case strings.HasPrefix(r.URL.Path, agentEventsPathBase):
		h.handleAgentEventChild(w, r)
	case r.URL.Path == generateTextPath:
		h.handleGenerateText(w, r)
	case r.URL.Path == toolPlanPath:
		h.handleToolPlan(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handleGenerateText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var request generateTextRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeRequestDecodeError(w, err)
		return
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" || len(prompt) > maxGenerateTextPrompt {
		writeError(
			w,
			http.StatusBadRequest,
			"INVALID_GENERATE_TEXT",
			"prompt is required and must be within limits",
		)
		return
	}
	if request.ModelRef == nil {
		writeError(w, http.StatusBadRequest, "MODEL_REF_REQUIRED", "modelRef is required")
		return
	}

	providerResolution, err := h.resolveStreamProvider(r.Context(), request.Provider)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	provider := providerResolution.Provider
	modelRef := *request.ModelRef
	if resolver, ok := provider.(ModelRefResolver); ok {
		modelRef, err = resolver.ResolveModelRef(modelRef)
	} else if validator, ok := provider.(ModelRefValidator); ok {
		err = validator.ValidateModelRef(modelRef)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	events, err := provider.StreamChat(ctx, ProviderRequest{
		Prompt:   prompt,
		ModelRef: modelRef,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "PROVIDER_ERROR", "provider text generation failed")
		return
	}

	var generated strings.Builder
	for event := range events {
		if event.Error != nil {
			writeError(w, http.StatusBadGateway, "PROVIDER_ERROR", "provider text generation failed")
			return
		}
		if event.Type != ProviderEventDelta || event.Delta == "" {
			continue
		}
		if generated.Len()+len(event.Delta) > maxGenerateTextOutput {
			cancel()
			writeError(
				w,
				http.StatusBadGateway,
				"PROVIDER_RESPONSE_TOO_LARGE",
				"provider text response exceeded the allowed size",
			)
			return
		}
		generated.WriteString(event.Delta)
	}

	writeJSON(w, http.StatusOK, generateTextResponse{Text: generated.String()})
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
	switch child {
	case "cancel":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.cancelRun(w, r, runID)
	case "events":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		h.resumeRunEvents(w, r, runID)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) resumeRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	afterValue := strings.TrimSpace(r.URL.Query().Get("after"))
	if afterValue == "" {
		afterValue = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	after := 0
	var err error
	if afterValue != "" {
		after, err = strconv.Atoi(afterValue)
		if err != nil || after < 0 {
			writeError(w, http.StatusBadRequest, "INVALID_STREAM_CURSOR", "after must be a non-negative integer")
			return
		}
	}
	actor := auth.UserOrDevelopment(r.Context())
	stream, conversationID := h.activeRuns.streamForUser(runID, actor.ID)
	if stream == nil {
		writeError(w, http.StatusNotFound, "RUN_STREAM_NOT_FOUND", "run stream not found")
		return
	}
	if _, err := h.service.GetConversation(r.Context(), conversationID); err != nil {
		writeServiceError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "streaming is not supported")
		return
	}
	subscription := stream.subscribe(after)
	defer subscription.Unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if subscription.Gap != nil {
		gap := subscription.Gap
		if err := writeSSEEvent(w, "stream.gap", runStreamGapEvent{
			Type: "stream.gap", RunID: runID, ConversationID: conversationID,
			MessageID: stream.messageID, CreatedAt: formatTime(time.Now()),
			After: gap.After, OldestSequence: gap.OldestSequence,
			LatestSequence: gap.LatestSequence, Reason: "cursor_evicted",
		}); err != nil {
			return
		}
		flusher.Flush()
	}
	for _, frame := range subscription.Replay {
		if _, err := w.Write(frame.Bytes); err != nil {
			return
		}
		flusher.Flush()
	}
	if subscription.Closed {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case frame, open := <-subscription.Updates:
			if !open {
				return
			}
			if _, err := w.Write(frame.Bytes); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) handleApprovalChild(w http.ResponseWriter, r *http.Request) {
	approvalID, child, ok := parseApprovalChildPath(r.URL.Path)
	if !ok || child != "decision" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request decideApprovalRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeRequestDecodeError(w, err)
		return
	}
	switch strings.TrimSpace(request.Decision) {
	case ChatAgentApprovalAllowOnce, ChatAgentApprovalAllowConversation,
		ChatAgentApprovalDeny:
	default:
		writeError(
			w, http.StatusBadRequest, "INVALID_CHAT_AGENT_APPROVAL_DECISION",
			"approval decision must be allow_once, allow_conversation, or deny",
		)
		return
	}
	approval, err := h.service.DecideChatAgentApproval(r.Context(), DecideChatAgentApprovalInput{
		ApprovalID: approvalID, ExpectedRevision: request.ExpectedRevision,
		Decision: request.Decision, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	h.approvalWaiters.resolve(approval)
	writeJSON(w, http.StatusOK, approval)
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
	if h.mcpService != nil {
		user := auth.UserOrDevelopment(r.Context())
		if err := h.mcpService.DeleteConversationData(r.Context(), user.ID, conversationID); err != nil {
			writeMCPAdmissionError(w, err)
			return
		}
	}
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

	writeJSON(w, http.StatusOK, h.newMessageDTO(r.Context(), message))
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
		Message: h.newMessageDTO(r.Context(), message),
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
	if child == "mcp-preflight" {
		h.handleMCPPreflight(w, r, conversationID)
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
	if child == "memory-policy" {
		h.handleConversationMemoryPolicy(w, r, conversationID)
		return
	}
	if child == "permission" {
		h.handleConversationPermission(w, r, conversationID)
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

func (h *Handler) handleConversationPermission(
	w http.ResponseWriter,
	r *http.Request,
	conversationID string,
) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w, http.MethodPut)
		return
	}
	if h.hostWorkspaceService == nil {
		writeError(w, http.StatusServiceUnavailable, "HOST_EXECUTION_UNAVAILABLE", "Host Workspace execution is unavailable")
		return
	}
	var request updateConversationPermissionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeRequestDecodeError(w, err)
		return
	}
	err := h.hostWorkspaceService.SetConversationPermission(
		r.Context(), conversationID, request.PermissionMode,
		request.FullAccessAcknowledged,
	)
	switch {
	case errors.Is(err, hostworkspace.ErrInvalid):
		writeError(w, http.StatusBadRequest, "INVALID_AGENT_PERMISSION", "Agent permission request is invalid")
		return
	case errors.Is(err, hostworkspace.ErrPermissionAcknowledgement):
		writeError(w, http.StatusBadRequest, "FULL_ACCESS_ACKNOWLEDGEMENT_REQUIRED", "Full access acknowledgement is required")
		return
	case errors.Is(err, hostworkspace.ErrPermissionLocked):
		writeError(w, http.StatusConflict, "CONVERSATION_PERMISSION_LOCKED", "Agent permission cannot change during an active Turn")
		return
	case errors.Is(err, hostworkspace.ErrPermissionUnavailable):
		writeError(w, http.StatusConflict, "AGENT_PERMISSION_UNAVAILABLE", "Agent permission mode is unavailable")
		return
	case errors.Is(err, hostworkspace.ErrDisabled):
		writeError(w, http.StatusServiceUnavailable, "HOST_EXECUTION_UNAVAILABLE", "Host Workspace execution is unavailable")
		return
	case errors.Is(err, hostworkspace.ErrWorkspaceUnbound):
		writeError(w, http.StatusConflict, "WORKSPACE_UNBOUND", "Workspace has no Host directory")
		return
	case errors.Is(err, hostworkspace.ErrConversationNotFound):
		writeError(w, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation was not found")
		return
	case err != nil:
		writeServiceError(w, err)
		return
	}
	conversation, err := h.service.GetConversation(r.Context(), conversationID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, newConversationDTO(conversation))
}

func (h *Handler) handleMCPPreflight(
	w http.ResponseWriter,
	r *http.Request,
	conversationID string,
) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request mcpPreflightRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeRequestDecodeError(w, err)
		return
	}
	if request.ModelRef == nil {
		writeError(w, http.StatusBadRequest, "MODEL_REF_REQUIRED", "modelRef is required")
		return
	}
	if _, err := h.service.GetConversation(r.Context(), conversationID); err != nil {
		writeServiceError(w, err)
		return
	}
	if h.mcpService == nil || !h.mcpService.Config().Enabled {
		writeJSON(w, http.StatusOK, mcpPreflightResponse{Enabled: false})
		return
	}

	user := auth.UserOrDevelopment(r.Context())
	prepared, err := h.mcpService.Preflight(r.Context(), user.ID, conversationID)
	if err != nil {
		writeMCPAdmissionError(w, err)
		return
	}
	if !prepared.Enabled() {
		writeJSON(w, http.StatusOK, mcpPreflightResponse{Enabled: false})
		return
	}

	providerResolution, err := h.resolveStreamProvider(r.Context(), request.Provider)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	modelRef := *request.ModelRef
	if resolver, ok := providerResolution.Provider.(ModelRefResolver); ok {
		modelRef, err = resolver.ResolveModelRef(modelRef)
	} else if validator, ok := providerResolution.Provider.(ModelRefValidator); ok {
		err = validator.ValidateModelRef(modelRef)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if h.resolveToolRoundCapability(
		r.Context(),
		providerResolution.Provider,
		providerResolution,
		modelRef,
	) != ToolCapabilitySupported {
		writeError(
			w,
			http.StatusConflict,
			"MCP_MODEL_UNSUPPORTED",
			"The selected model does not support Tools",
		)
		return
	}
	writeJSON(w, http.StatusOK, mcpPreflightResponse{Enabled: true})
}

func (h *Handler) handleConversationMemoryPolicy(
	w http.ResponseWriter,
	r *http.Request,
	conversationID string,
) {
	if h == nil || h.userMemoryService == nil {
		writeServiceError(w, usermemory.ErrGovernanceRepositoryRequired)
		return
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := h.userMemoryService.GetConversationPolicy(r.Context(), conversationID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, policy)
	case http.MethodPatch:
		var request updateConversationMemoryPolicyRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeRequestDecodeError(w, err)
			return
		}
		policy, err := h.userMemoryService.UpdateConversationPolicy(
			r.Context(),
			usermemory.UpdateConversationPolicyInput{
				ConversationID:          conversationID,
				ExpectedScopeGeneration: request.ExpectedScopeGeneration,
				ProjectID:               request.ProjectID,
				UseMode:                 request.UseMode,
				LearnMode:               request.LearnMode,
			},
		)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, policy)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch)
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
		items = append(items, h.newMessageDTO(r.Context(), message))
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

	writeJSON(w, http.StatusCreated, h.newMessageDTO(r.Context(), message))
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
	// Authentication and request-scoped values remain available, but browser
	// navigation or a closed SSE socket is delivery state, not Run cancellation.
	generationCtx := context.WithoutCancel(r.Context())
	r = r.WithContext(generationCtx)
	delivery := newBestEffortStreamWriter(w)
	w = delivery
	flusher = delivery

	userMessage, err := h.service.GetMessage(r.Context(), conversationID, request.UserMessageID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if userMessage.Role != "user" {
		writeRequestDecodeError(w, newValidationError("INVALID_USER_MESSAGE_ID", "userMessageId must reference a user message"))
		return
	}
	var continuation *answerContinuation
	request.ContinuationOfMessageID = strings.TrimSpace(request.ContinuationOfMessageID)
	if request.ContinuationOfMessageID != "" {
		if !isUUID(request.ContinuationOfMessageID) {
			writeRequestDecodeError(w, newValidationError(
				"INVALID_ANSWER_CONTINUATION_ID",
				"continuationOfMessageId must be a UUID",
			))
			return
		}
		source, sourceErr := h.service.GetMessage(
			r.Context(), conversationID, request.ContinuationOfMessageID,
		)
		if sourceErr != nil {
			writeServiceError(w, sourceErr)
			return
		}
		prepared, prepareErr := prepareAnswerContinuation(source, userMessage)
		if prepareErr != nil {
			writeServiceError(w, prepareErr)
			return
		}
		continuation = &prepared
	}
	ragSelection := ragSelection{}
	if continuation == nil {
		ragSelection, err = h.resolveConversationRAGSelection(
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
	}
	if isImageGenerationModel(modelRef.ModelID) {
		if continuation != nil {
			writeRequestDecodeError(w, newValidationError(
				"ANSWER_CONTINUATION_NOT_ALLOWED",
				"image generation replies must be regenerated",
			))
			return
		}
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

	providerResolution, err := h.resolveStreamProvider(r.Context(), request.Provider)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	streamProvider := providerResolution.Provider
	ragAnswerProcessor := providerResolution.RAGAnswerProcessor
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
	attachmentResolution := providerAttachmentResolution{}
	if continuation == nil {
		attachmentResolution, err = h.resolveProviderMessageAttachments(
			r.Context(),
			userMessage,
		)
		if err != nil {
			writeServiceError(w, err)
			return
		}
	}
	providerAttachments := attachmentResolution.Images
	conversationMessages, err := h.service.ListMessages(r.Context(), conversationID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	conversation, err := h.service.GetConversation(r.Context(), conversationID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	requestedToolMode := requestedChatToolMode(conversation.Metadata)
	searchMode := searchModeFromConfig(request.Config)
	if continuation != nil {
		searchMode = chatSearchModeOff
	}
	toolRoundCapable := false
	if continuation == nil {
		toolRoundCapable = h.resolveToolRoundCapabilityForMode(
			r.Context(),
			streamProvider,
			providerResolution,
			*modelRef,
			requestedToolMode,
		) == ToolCapabilitySupported
	}
	effectiveToolMode := effectiveChatToolMode(conversation.Metadata, toolRoundCapable)
	agentMode := effectiveToolMode == chatToolModeAgent
	useLiveKnowledgeTool := ragSelection.Enabled && toolRoundCapable &&
		searchMode != chatSearchModeModelBuiltIn
	useCompatibilityKnowledge := ragSelection.Enabled && !useLiveKnowledgeTool
	autoDecision := autoRAGDecision{}
	if ragSelection.Enabled {
		autoDecision.Outcome = "not_requested"
	}
	knowledgeStarted := time.Now()
	providerPrompt := appendDirectAttachmentContext(
		userMessage.Content,
		attachmentResolution.DocumentContext,
	)
	providerSystemPrompt := systemPrompt
	if attachmentResolution.HasDocuments {
		providerSystemPrompt = appendDirectAttachmentSystemInstruction(
			providerSystemPrompt,
		)
	}
	providerMetadata := request.Metadata
	routerStarted := time.Now()
	forceExternalSearch := searchMode == chatSearchModeExternal &&
		hasCurrentPublicIntent(userMessage.Content)
	fusionPlan := planSourceFusion(userMessage.Content, searchMode.enabled(), autoDecision)
	if searchMode == chatSearchModeExternal {
		fusionPlan.SearchRequested = false
		fusionPlan.SearchReason = sourceSearchAutoTool
		if autoDecision.ReadyForAnswer() {
			fusionPlan.Authority = sourceAuthorityKnowledge
		} else {
			fusionPlan.Authority = sourceAuthorityModel
		}
	}
	fusionDiagnostics := newSourceFusionDiagnostics(fusionPlan)
	if searchMode == chatSearchModeExternal {
		fusionDiagnostics.WebResolveOutcome = "pending"
		fusionDiagnostics.WebExecuteOutcome = "pending"
		fusionDiagnostics.WebQueryConversationRewriteOutcome = "pending"
	}
	fusionDiagnostics.RouterDurationMillis = sourceFusionDurationMillis(routerStarted)
	resolveStarted := time.Now()
	var searchExecution *websearch.ActiveExecution
	var modelBuiltInSearchProvider ModelBuiltInSearchProvider
	var searchErr error
	switch searchMode {
	case chatSearchModeExternal:
		searchExecution, searchErr = h.resolveExternalSearchExecution(r.Context())
	case chatSearchModeModelBuiltIn:
		searchExecution, modelBuiltInSearchProvider, searchErr =
			h.resolveChatSearchExecution(
				r.Context(), streamProvider, *modelRef, fusionPlan.SearchRequested,
			)
	}
	if searchMode.enabled() {
		fusionDiagnostics.WebResolveDurationMillis = sourceFusionDurationMillis(resolveStarted)
	}
	if searchErr != nil {
		fusionDiagnostics.WebResolveOutcome = "degraded"
		fusionDiagnostics.WebExecuteOutcome = "not_run"
		fusionDiagnostics.WebQueryConversationRewriteOutcome = "not_run"
		fusionDiagnostics.DegradationReason = sourceSearchDegradationReason(searchErr)
		fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
		searchExecution = nil
		modelBuiltInSearchProvider = nil
	} else if searchExecution != nil {
		fusionDiagnostics.WebResolveOutcome = "resolved"
	}
	webSearchResult := websearch.Result{Sources: []websearch.Source{}, Images: []websearch.Image{}}
	if searchExecution != nil &&
		searchExecution.Mode == websearch.ExecutionModelBuiltIn {
		fusionDiagnostics.WebQueryConversationRewriteOutcome = "provider_managed"
		if !useCompatibilityKnowledge {
			fusionDiagnostics.WebExecuteOutcome = "provider_stream"
		}
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
	var preparedMCPRun mcpclient.PreparedRun
	actor := auth.UserOrDevelopment(r.Context())
	agentTimelineEnabled := h.agentTimelineEnabledFor(actor.ID)
	if agentMode && h.mcpService != nil && h.mcpService.Config().Enabled {
		preparedMCPRun, err = h.mcpService.PrepareRun(
			r.Context(), actor.ID, conversationID, "", runID,
		)
		if err != nil {
			writeMCPAdmissionError(w, err)
			return
		}
	}
	var localSkillRuntime *localSkillToolRuntime
	localSkillContextPrompt := ""
	if agentMode && h.localSkillExecutor != nil && h.localSkillExecutor.Enabled() {
		toolExecutor, executorErr := h.localToolExecutorForConversation(
			r.Context(), conversationID, conversation.WorkspaceID,
		)
		if executorErr != nil {
			if errors.Is(executorErr, errHostWorkspaceBinding) {
				writeError(w, http.StatusConflict, "HOST_WORKSPACE_BINDING_FAILED", "Conversation Workspace binding is unavailable")
			} else {
				writeError(w, http.StatusServiceUnavailable, "HOST_EXECUTION_UNAVAILABLE", "Host Workspace execution is unavailable")
			}
			return
		}
		var skills []skillsupply.RuntimeSkill
		if h.localSkillCatalog != nil {
			var prepareErr error
			skills, prepareErr = h.localSkillCatalog.PrepareRuntimeSkills(
				r.Context(), actor.ID, h.localSkillExecutor.Config().RuntimeRoot,
			)
			if prepareErr != nil {
				writeError(w, http.StatusServiceUnavailable, "SKILL_RUNTIME_UNAVAILABLE", "local Skill runtime is unavailable")
				return
			}
		}
		localSkillRuntime = newLocalSkillToolRuntime(toolExecutor, skills)
		localSkillRuntime.bindJobScope(actor.ID, conversationID)
		localSkillRuntime.bindArtifactPublisher(h.artifactPublisher, h.artifactMaxBytes)
		var prepareErr error
		providerPrompt, prepareErr = localSkillRuntime.prepareUserPrompt(
			providerPrompt,
			userMessage.Content,
		)
		if prepareErr != nil {
			writeError(w, http.StatusServiceUnavailable, "SKILL_RUNTIME_UNAVAILABLE", "local Skill runtime is unavailable")
			return
		}
		localSkillContextPrompt = localSkillRuntime.promptInstruction()
		if localSkillContextPrompt != "" {
			providerSystemPrompt = strings.TrimSpace(providerSystemPrompt)
			if providerSystemPrompt != "" {
				providerSystemPrompt += "\n\n"
			}
			providerSystemPrompt += localSkillContextPrompt
		}
	}

	assistantMetadata := map[string]any{
		"runId":             runID,
		"config":            ensureObject(request.Config),
		"requestedToolMode": string(requestedToolMode),
		"toolMode":          string(effectiveToolMode),
	}
	if continuation != nil {
		assistantMetadata[continuationOfMessageIDMetadataKey] = continuation.source.ID
		assistantMetadata[continuationModeMetadataKey] = answerOnlyContinuationMode
	}
	assistantMessage, err := h.service.CreateAssistantMessage(
		r.Context(),
		conversationID,
		CreateAssistantMessageInput{
			ID:              assistantMessageID,
			ParentMessageID: userMessage.ID,
			ModelProvider:   modelRef.ProviderID,
			ModelID:         modelRef.ModelID,
			Metadata:        assistantMetadata,
			IdempotencyKey:  request.IdempotencyKey,
		},
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	agentRecorder, err := startChatAgentEventRecorder(
		r.Context(),
		h.service,
		conversationID,
		assistantMessage.ID,
		runID,
		time.Now(),
	)
	if err != nil {
		_, _ = h.finalizeAssistantMessage(
			context.Background(),
			conversationID,
			assistantMessage.ID,
			FinalizeAssistantMessageInput{
				Status: "failed",
				Metadata: map[string]any{
					"runId": runID, "errorCode": "AGENT_EVENT_PERSISTENCE_FAILED",
				},
			},
		)
		writeError(
			w,
			http.StatusInternalServerError,
			"AGENT_EVENT_PERSISTENCE_FAILED",
			"chat Agent event persistence failed",
		)
		return
	}
	streamedAgentEvents := []ChatAgentEvent{agentRecorder.startedEvent}
	appendStreamedAgentEvent := func(event ChatAgentEvent) {
		if event.EventID == "" {
			return
		}
		for _, existing := range streamedAgentEvents {
			if existing.EventID == event.EventID {
				return
			}
		}
		streamedAgentEvents = append(streamedAgentEvents, event)
	}
	var emitAgentEvent func(ChatAgentEvent) error
	var goalToolRuntime *chatAgentGoalToolRuntime
	if agentMode {
		goalToolRuntime = newChatAgentGoalToolRuntime(
			h.service, agentRecorder.turnID, conversationID,
		)
		if agentTimelineEnabled {
			localSkillRuntime.bindApprovalRuntime(newChatToolApprovalRuntime(
				h.service, h.approvalWaiters, agentRecorder.turnID,
			))
		}
		providerSystemPrompt = appendChatAgentGoalSystemInstruction(
			providerSystemPrompt, goalToolRuntime,
		)
	}
	turnFinalStatus := ChatAgentTurnInterrupted
	turnFinalContent := ""
	turnFinalErrorCode := "AGENT_RUN_INTERRUPTED"
	defer func() {
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer finishCancel()
		_, _ = agentRecorder.finish(
			finishCtx,
			turnFinalStatus,
			turnFinalContent,
			turnFinalErrorCode,
			time.Now(),
		)
	}()
	finalizeTracked := func(
		ctx context.Context,
		conversationID string,
		messageID string,
		input FinalizeAssistantMessageInput,
	) (Message, error) {
		input.Attachments = append(input.Attachments, localSkillRuntime.publishedAttachmentInputs()...)
		ctx = auth.WithUser(ctx, actor)
		message, finalizeErr := h.finalizeAssistantMessage(
			ctx, conversationID, messageID, input,
		)
		if finalizeErr == nil {
			turnFinalStatus = normalizeChatAgentTurnStatus(input.Status)
			turnFinalContent = input.Content
			turnFinalErrorCode = chatAgentErrorCode(input.Metadata)
			finishCtx, finishCancel := context.WithTimeout(
				auth.WithUser(context.Background(), actor),
				5*time.Second,
			)
			finishedEvents, finishErr := agentRecorder.finish(
				finishCtx,
				turnFinalStatus,
				turnFinalContent,
				turnFinalErrorCode,
				time.Now(),
			)
			finishCancel()
			if finishErr != nil {
				finalizeErr = finishErr
			}
			for _, event := range finishedEvents {
				appendStreamedAgentEvent(event)
				if emitAgentEvent != nil {
					_ = emitAgentEvent(event)
				}
			}
			message.AgentEvents = append([]ChatAgentEvent(nil), streamedAgentEvents...)
			if agentTimelineEnabled {
				eventReadCtx, eventReadCancel := context.WithTimeout(
					auth.WithUser(context.Background(), actor),
					5*time.Second,
				)
				if current, readErr := h.service.GetMessage(
					eventReadCtx, conversationID, messageID,
				); readErr == nil {
					message.AgentEvents = append(
						[]ChatAgentEvent(nil), current.AgentEvents...,
					)
				}
				eventReadCancel()
			}
		}
		if finalizeErr != nil {
			reconcileCtx := auth.WithUser(context.Background(), actor)
			if current, readErr := h.service.GetMessage(
				reconcileCtx, conversationID, messageID,
			); readErr == nil {
				localSkillRuntime.discardUnlinkedPublishedArtifacts(
					reconcileCtx, current.Attachments,
				)
			}
		}
		return message, finalizeErr
	}
	cancelTrackedAfterWriteError := func(
		conversationID string,
		messageID string,
		runID string,
		content string,
	) {
		if turnFinalStatus == ChatAgentTurnFailed {
			turnFinalContent = content
			_, _ = finalizeTracked(
				generationCtx,
				conversationID,
				messageID,
				FinalizeAssistantMessageInput{
					Status: "failed", Content: content,
					Metadata: map[string]any{
						"runId": runID, "errorCode": turnFinalErrorCode,
					},
				},
			)
			return
		}
		turnFinalStatus = ChatAgentTurnCancelled
		turnFinalContent = content
		turnFinalErrorCode = "SSE_WRITE_FAILED"
		_, _ = finalizeTracked(generationCtx, conversationID, messageID, FinalizeAssistantMessageInput{
			Status: "cancelled", Content: content,
			Metadata: map[string]any{"runId": runID, "errorCode": "SSE_WRITE_FAILED"},
		})
	}
	memoryToolRuntime := h.newMemoryToolRuntime(
		r.Context(),
		toolRoundCapable,
		searchMode,
		userMessage.Content,
		conversationID,
		assistantMessage.ID,
	)
	mcpRuntime := newMCPToolRuntime(
		h.mcpService,
		preparedMCPRun,
		actor.ID,
		userMessage.Content,
	)
	directMemoryAction := directMemoryActionPreparation{}
	if continuation == nil && !memoryToolRuntime.enabled() {
		directMemoryAction = h.prepareDirectMemoryAction(
			r.Context(),
			conversationID,
			userMessage,
			assistantMessage,
			conversationMessages,
			streamProvider,
			*modelRef,
		)
	}
	providerSystemPrompt = appendDirectMemoryActionAnswerInstruction(
		providerSystemPrompt,
		directMemoryAction,
	)
	trace := newProcessTrace(assistantMessage.ID)
	addLegacyWebProcessStep(
		trace,
		fusionPlan,
		fusionDiagnostics,
		searchExecution,
		"",
		webSearchResult,
		resolveStarted,
		0,
	)
	if searchMode == chatSearchModeExternal && searchExecution == nil &&
		forceExternalSearch {
		trace.add(terminalProcessStep(
			trace.stepID(ProcessStepKindWeb),
			ProcessStepKindWeb,
			ProcessStepStatusFailed,
			resolveStarted,
			fusionDiagnostics.WebResolveDurationMillis,
			map[string]any{
				"mode":            string(chatSearchModeExternal),
				"outcome":         "degraded",
				"failureCategory": fusionDiagnostics.DegradationReason,
			},
		))
		providerSystemPrompt = strings.TrimSpace(providerSystemPrompt)
		if providerSystemPrompt != "" {
			providerSystemPrompt += "\n\n"
		}
		providerSystemPrompt += externalWebUnavailableSystemInstruction
	}
	messageSearchExecution := searchExecution
	if searchMode == chatSearchModeExternal {
		messageSearchExecution = nil
	}
	searchPlannerMessages := buildProviderConversationMessages(
		conversationMessages,
		userMessage.ID,
		userMessage.Content,
		nil,
	)
	var knowledgeRuntime *knowledgeToolRuntime
	if ragSelection.Enabled {
		knowledgeRuntime = h.newKnowledgeToolRuntime(
			r.Context(),
			conversationID,
			userMessage.Content,
			ragSelection,
			*modelRef,
			ragAnswerProcessor,
		)
		prepareKnowledgeRoutingCatalog(
			r.Context(),
			h.knowledgeCatalogSource,
			h.knowledgeCatalogGate,
			knowledgeRuntime,
		)
	}

	memoryPreparation := durableMemoryPreparation{}
	if continuation == nil && !memoryToolRuntime.enabled() {
		providerSystemPrompt, memoryPreparation = h.prepareDurableMemory(
			r.Context(),
			userMessage.Content,
			conversationID,
			assistantMessage.ID,
			providerSystemPrompt,
		)
	}
	providerSystemPromptWithoutFusion := providerSystemPrompt
	providerSystemPrompt = applySourceFusionSystemInstruction(
		providerSystemPrompt,
		fusionPlan,
	)
	providerSystemPrompt = appendAgentFollowupPrompt(
		providerSystemPrompt,
		localSkillRuntime.consumeJobCompletionPrompt(),
	)
	providerMessages := buildProviderConversationMessages(
		conversationMessages, userMessage.ID, providerPrompt, providerAttachments,
	)
	if continuation != nil {
		providerSystemPrompt = appendAnswerContinuationSystemInstruction(providerSystemPrompt)
		providerMessages = buildProviderConversationMessages(
			conversationMessages, continuation.source.ID, continuation.prefix, nil,
		)
		providerMessages = append(providerMessages, ProviderMessage{
			Role: "user", Content: answerContinuationPrompt(*continuation),
		})
		providerPrompt = answerContinuationPrompt(*continuation)
		providerAttachments = nil
	}
	contextPreparation := conversationContextPreparation{
		Messages: providerMessages, SystemPrompt: providerSystemPrompt, Mode: "full",
	}
	if continuation == nil {
		contextPreparation = h.prepareConversationContext(
			r.Context(),
			conversationID,
			*modelRef,
			streamProvider,
			providerSystemPrompt,
			providerMessages,
		)
	}
	providerMessages = contextPreparation.Messages
	providerSystemPrompt = contextPreparation.SystemPrompt
	preludeAgentEvents := make([]ChatAgentEvent, 0, 2)
	recordContextInjection := func(source, label, content string) error {
		if strings.TrimSpace(content) == "" {
			return nil
		}
		if localSkillRuntime != nil {
			hostRoot := strings.TrimSpace(localSkillRuntime.config().WorkspaceHostRoot)
			if hostRoot != "" {
				content = strings.ReplaceAll(content, hostRoot, "$NEO_CHAT_WORKSPACE_HOST")
			}
		}
		recorded, recordErr := agentRecorder.recordContextInjection(
			generationCtx, source, label, content, time.Now(),
		)
		if recordErr != nil {
			return recordErr
		}
		preludeAgentEvents = append(preludeAgentEvents, recorded)
		appendStreamedAgentEvent(recorded)
		return nil
	}
	if err := recordContextInjection(
		"system-prompt", "System prompt", providerSystemPrompt,
	); err != nil {
		turnFinalStatus = ChatAgentTurnFailed
		turnFinalErrorCode = "AGENT_EVENT_PERSISTENCE_FAILED"
		writeError(w, http.StatusInternalServerError, turnFinalErrorCode, "chat Agent event persistence failed")
		return
	}
	if err := recordContextInjection(
		"skill-catalog", "Skill catalog", localSkillContextPrompt,
	); err != nil {
		turnFinalStatus = ChatAgentTurnFailed
		turnFinalErrorCode = "AGENT_EVENT_PERSISTENCE_FAILED"
		writeError(w, http.StatusInternalServerError, turnFinalErrorCode, "chat Agent event persistence failed")
		return
	}
	webMessageMetadata := func(
		decision autoRAGDecision,
		extra map[string]any,
		answerContent string,
	) map[string]any {
		decision = decision.completed(answerContent)
		metadataFusionPlan := reconcileCompletedSourceFusionAuthority(
			fusionPlan,
			answerContent,
			decision,
			webSearchResult,
		)
		metadata := withUsedWebSearchMessageMetadata(
			autoRAGMessageMetadata(runID, ragSelection, decision, extra),
			messageSearchExecution,
			answerContent,
			webSearchResult,
		)
		metadata = withSourceFusionMessageMetadata(
			metadata,
			metadataFusionPlan,
			decision,
			fusionDiagnostics,
		)
		metadata = withConversationContextMetadata(metadata, contextPreparation)
		metadata = withDurableMemoryMetadata(metadata, memoryPreparation)
		metadata["requestedToolMode"] = string(requestedToolMode)
		metadata["toolMode"] = string(effectiveToolMode)
		if continuation != nil {
			metadata[continuationOfMessageIDMetadataKey] = continuation.source.ID
			metadata[continuationModeMetadataKey] = answerOnlyContinuationMode
		}
		return withDirectMemoryActionMetadata(metadata, directMemoryAction)
	}

	var streamCtx context.Context
	var streamCancel context.CancelFunc
	streamTimeout := time.Duration(0)
	streamDeadlineSource := ""
	if mcpRuntime.enabled() {
		streamTimeout = h.mcpService.Config().RunTimeout
		streamDeadlineSource = "mcp"
	}
	if localSkillRuntime.enabled() &&
		(streamTimeout == 0 || localSkillRuntime.config().RunTimeout < streamTimeout) {
		streamTimeout = localSkillRuntime.config().RunTimeout
		streamDeadlineSource = "local_skill"
	}
	if streamTimeout > 0 {
		streamCtx, streamCancel = context.WithTimeout(
			generationCtx,
			streamTimeout,
		)
	} else {
		streamCtx, streamCancel = context.WithCancel(generationCtx)
	}
	runStream := newActiveRunStream(runID, conversationID, assistantMessage.ID)
	delivery.attachStream(runStream)
	unregisterRun := h.activeRuns.registerStream(
		runID, streamCancel, actor.ID, conversationID, runStream,
	)
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
	reasoning := newProcessReasoningStream()
	toolTrace := newToolProcessTrace(trace)
	if modelBuiltInSearchProvider != nil && !useCompatibilityKnowledge {
		builtInSearchStarted = time.Now()
		startBuiltInWebProcessStep(trace, searchExecution, builtInSearchStarted)
	}
	generationStarted := time.Now()
	trace.start(
		ProcessStepKindGeneration,
		"process.generation",
		generationStarted,
		map[string]any{"outcome": "streaming"},
	)
	if mcpRuntime.enabled() || localSkillRuntime.enabled() || memoryToolRuntime.enabled() ||
		goalToolRuntime.enabled() || useLiveKnowledgeTool ||
		(searchMode == chatSearchModeExternal && searchExecution != nil &&
			searchExecution.Mode == websearch.ExecutionExternal &&
			!useCompatibilityKnowledge) {
		toolLoopInput := externalWebToolLoopInput{
			Provider:        streamProvider,
			Request:         providerRequest,
			PlannerMessages: searchPlannerMessages,
			SearchService:   h.webSearchService,
			MaxResults: configIntRange(
				request.Config,
				"searchResultsLimit",
				5,
				1,
				websearch.MaxResults,
			),
			ForceSearch:            forceExternalSearch,
			KnowledgeReady:         autoDecision.ReadyForAnswer(),
			Knowledge:              knowledgeRuntime,
			Memory:                 memoryToolRuntime,
			CapabilityCache:        h.toolCapabilityCache,
			CapabilityConfigHash:   providerResolution.ToolCapabilityConfigHash,
			DisableNativeToolRound: !toolRoundCapable,
			MCP:                    mcpRuntime,
			LocalSkills:            localSkillRuntime,
			Goals:                  goalToolRuntime,
		}
		if searchExecution != nil &&
			searchExecution.Mode == websearch.ExecutionExternal {
			toolLoopInput.Execution = *searchExecution
		}
		events = startRetrievalToolLoop(streamCtx, toolLoopInput)
	} else if useCompatibilityKnowledge {
		compatibilityInput := compatibilityKnowledgeLoopInput{
			Provider:             streamProvider,
			Request:              providerRequest,
			Runtime:              knowledgeRuntime,
			ConversationMessages: conversationMessages,
			PlannerMessages:      searchPlannerMessages,
			BuiltInSearch:        modelBuiltInSearchProvider,
			ForceSearch:          forceExternalSearch,
		}
		if searchExecution != nil &&
			searchExecution.Mode == websearch.ExecutionExternal {
			externalInput := externalWebToolLoopInput{
				Provider:        streamProvider,
				Request:         providerRequest,
				PlannerMessages: searchPlannerMessages,
				SearchService:   h.webSearchService,
				Execution:       *searchExecution,
				MaxResults: configIntRange(
					request.Config,
					"searchResultsLimit",
					5,
					1,
					websearch.MaxResults,
				),
				ForceSearch: forceExternalSearch,
			}
			compatibilityInput.ExternalSearch = &externalInput
		}
		events = startCompatibilityKnowledgeLoop(streamCtx, compatibilityInput)
	} else if modelBuiltInSearchProvider != nil {
		events, err = modelBuiltInSearchProvider.StreamChatWithModelBuiltInSearch(
			streamCtx,
			providerRequest,
		)
		if err != nil && streamCtx.Err() == nil &&
			!errors.Is(err, context.Canceled) {
			fusionDiagnostics.WebExecuteDurationMillis =
				sourceFusionDurationMillis(builtInSearchStarted)
			fusionDiagnostics.WebExecuteOutcome = "degraded"
			fusionDiagnostics.DegradationReason = "provider_failed"
			completeBuiltInWebProcessStep(
				trace,
				ProcessStepStatusFailed,
				time.Now(),
				webSearchResult,
				"provider_failed",
			)
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
		deadlineExceeded := streamDeadlineSource != "" &&
			errors.Is(streamCtx.Err(), context.DeadlineExceeded)
		if !deadlineExceeded && (streamCtx.Err() != nil || errors.Is(err, context.Canceled)) {
			finishProcessTrace(trace, "cancelled", time.Now(), webSearchResult)
			finalizeTracked(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
				Status: "cancelled",
				OutputBlocks: usedWebSearchOutputBlocks(
					assistantMessage.ID,
					"",
					webSearchResult,
				),
				Metadata: withProcessTraceMessageMetadata(
					webMessageMetadata(autoDecision, nil, ""),
					reasoning.String(),
					trace,
				),
			})
			return
		}
		deadlineErr, legacyMCPDeadline := chatStreamDeadlineError(
			streamDeadlineSource, err, deadlineExceeded,
		)
		errorBody := chatStreamErrorBody(deadlineErr, legacyMCPDeadline)
		finishProcessTrace(trace, "failed", time.Now(), webSearchResult)
		finalizeTracked(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
			Status: "failed",
			OutputBlocks: usedWebSearchOutputBlocks(
				assistantMessage.ID,
				"",
				webSearchResult,
			),
			Metadata: withProcessTraceMessageMetadata(
				webMessageMetadata(
					autoDecision,
					map[string]any{"errorCode": errorBody.Code},
					"",
				),
				reasoning.String(),
				trace,
			),
		})
		writeError(w, http.StatusBadGateway, errorBody.Code, errorBody.Message)
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
		cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, "")
		return
	}
	flusher.Flush()
	emitAgentEvent = func(event ChatAgentEvent) error {
		appendStreamedAgentEvent(event)
		if !agentTimelineEnabled {
			return nil
		}
		sequence++
		if err := writeSSEEvent(w, "agent.event", streamEvent{
			Type:           "agent.event",
			RunID:          runID,
			ConversationID: conversationID,
			MessageID:      assistantMessage.ID,
			Sequence:       sequence,
			CreatedAt:      formatTime(time.Now()),
			AgentEvent:     &event,
		}); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := emitAgentEvent(agentRecorder.startedEvent); err != nil {
		cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, "")
		return
	}
	for _, event := range preludeAgentEvents {
		if err := emitAgentEvent(event); err != nil {
			cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, "")
			return
		}
	}
	emitProcessStep := func(step ProcessStep) error {
		eventAt := time.Now()
		recordedEvent, projected, err := agentRecorder.recordProcessStep(
			generationCtx, step, eventAt,
		)
		if err != nil {
			turnFinalStatus = ChatAgentTurnFailed
			turnFinalErrorCode = "AGENT_EVENT_PERSISTENCE_FAILED"
			return err
		}
		if err := emitAgentEvent(recordedEvent); err != nil {
			return err
		}
		if agentTimelineEnabled {
			return nil
		}
		sequence++
		stepCopy := projected
		stepCopy.Presentation = nil
		if err := writeSSEEvent(w, "process.step.updated", streamEvent{
			Type:           "process.step.updated",
			RunID:          runID,
			ConversationID: conversationID,
			MessageID:      assistantMessage.ID,
			Sequence:       sequence,
			CreatedAt:      formatTime(time.Now()),
			Step:           &stepCopy,
		}); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	emitTransientProcessStep := func(step ProcessStep) error {
		sequence++
		stepCopy := cloneProcessStep(step)
		eventType := "agent.progress"
		if !agentTimelineEnabled {
			eventType = "process.step.updated"
			stepCopy.Presentation = nil
		}
		if err := writeSSEEvent(w, eventType, streamEvent{
			Type: eventType, RunID: runID,
			ConversationID: conversationID, MessageID: assistantMessage.ID,
			Sequence: sequence, CreatedAt: formatTime(time.Now()), Step: &stepCopy,
		}); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	reasoningBlockIndex := 0
	reasoningBlockOpen := false
	reasoningPersistedBytes := 0
	providerRound := 1
	persistReasoningDelta := func(delta string) error {
		if delta == "" {
			return nil
		}
		remaining := maxPersistedReasoningBytes - reasoningPersistedBytes
		persistedDelta := truncateChatAgentUTF8(
			delta,
			min(maxChatAgentChunkEventBytes, max(remaining, 0)),
		)
		if remaining > 0 && persistedDelta != "" {
			eventAt := time.Now()
			if !reasoningBlockOpen {
				reasoningBlockIndex++
				started, err := agentRecorder.recordAssistantBlockStart(
					generationCtx, reasoningBlockIndex, providerRound, eventAt,
				)
				if err != nil {
					return err
				}
				if err := emitAgentEvent(started); err != nil {
					return err
				}
				reasoningBlockOpen = true
			}
			recorded, err := agentRecorder.recordAssistantReasoningDelta(
				generationCtx, reasoningBlockIndex, providerRound, persistedDelta, eventAt,
			)
			if err != nil {
				return err
			}
			if err := emitAgentEvent(recorded); err != nil {
				return err
			}
			persistedContent, _ := recorded.Payload["content"].(string)
			reasoningPersistedBytes += len(persistedContent)
		}
		return nil
	}
	var reasoningEventBuffer strings.Builder
	lastReasoningEventFlush := time.Now()
	flushReasoningEvents := func(force bool) error {
		if reasoningEventBuffer.Len() == 0 {
			return nil
		}
		if !force && reasoningEventBuffer.Len() < 16<<10 &&
			time.Since(lastReasoningEventFlush) < 75*time.Millisecond {
			return nil
		}
		buffered := reasoningEventBuffer.String()
		reasoningEventBuffer.Reset()
		lastReasoningEventFlush = time.Now()
		for buffered != "" {
			chunk := truncateChatAgentUTF8(buffered, maxChatAgentChunkEventBytes)
			if chunk == "" {
				break
			}
			if err := persistReasoningDelta(chunk); err != nil {
				return err
			}
			buffered = buffered[len(chunk):]
		}
		return nil
	}
	emitReasoningDelta := func(delta string) error {
		if delta == "" {
			return nil
		}
		if !agentTimelineEnabled {
			sequence++
			if err := writeSSEEvent(w, "reasoning.delta", streamEvent{
				Type:           "reasoning.delta",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
				Delta:          delta,
			}); err != nil {
				return err
			}
			flusher.Flush()
		}
		reasoningEventBuffer.WriteString(delta)
		return flushReasoningEvents(false)
	}
	closeReasoningBlock := func() error {
		if err := flushReasoningEvents(true); err != nil {
			return err
		}
		if !reasoningBlockOpen {
			return nil
		}
		recorded, err := agentRecorder.recordAssistantBlockCompleted(
			generationCtx, reasoningBlockIndex, providerRound, time.Now(),
		)
		if err != nil {
			return err
		}
		reasoningBlockOpen = false
		return emitAgentEvent(recorded)
	}
	for _, step := range trace.snapshot() {
		if err := emitProcessStep(step); err != nil {
			cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, "")
			return
		}
	}
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
			cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, "")
			return
		}
		flusher.Flush()
	}

	var content strings.Builder
	if continuation != nil {
		content.WriteString(continuation.prefix)
		sequence++
		if err := writeSSEEvent(w, "message.delta", streamEvent{
			Type:           "message.delta",
			RunID:          runID,
			ConversationID: conversationID,
			MessageID:      assistantMessage.ID,
			Sequence:       sequence,
			CreatedAt:      formatTime(time.Now()),
			Delta:          continuation.prefix,
		}); err != nil {
			cancelTrackedAfterWriteError(
				conversationID, assistantMessage.ID, runID, content.String(),
			)
			return
		}
		flusher.Flush()
	}
	toolExecutionCancelled := false
	for providerEvent := range events {
		if providerEvent.Round > 0 {
			providerRound = providerEvent.Round
		}
		if providerEvent.Error != nil {
			if modelBuiltInSearchProvider != nil && streamCtx.Err() == nil &&
				!errors.Is(providerEvent.Error, context.Canceled) {
				fusionDiagnostics.WebExecuteDurationMillis =
					sourceFusionDurationMillis(builtInSearchStarted)
				fusionDiagnostics.WebExecuteOutcome = "degraded"
				fusionDiagnostics.DegradationReason = "provider_failed"
				fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
			}
			deadlineExceeded := streamDeadlineSource != "" &&
				errors.Is(streamCtx.Err(), context.DeadlineExceeded)
			if streamCtx.Err() != nil && !deadlineExceeded {
				_ = emitReasoningDelta(reasoning.flush())
				_ = closeReasoningBlock()
				for _, step := range reconcileProcessTraceCitations(
					trace,
					content.String(),
				) {
					_ = emitProcessStep(step)
				}
				for _, step := range finishProcessTrace(
					trace,
					"cancelled",
					time.Now(),
					webSearchResult,
				) {
					_ = emitProcessStep(step)
				}
				finalizeTracked(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
					Status:  "cancelled",
					Content: content.String(),
					OutputBlocks: usedWebSearchOutputBlocks(
						assistantMessage.ID,
						content.String(),
						webSearchResult,
					),
					Metadata: withProcessTraceMessageMetadata(
						webMessageMetadata(autoDecision, nil, content.String()),
						reasoning.String(),
						trace,
					),
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
			deadlineErr, legacyMCPDeadline := chatStreamDeadlineError(
				streamDeadlineSource, providerEvent.Error, deadlineExceeded,
			)
			errorBody := chatStreamErrorBody(deadlineErr, legacyMCPDeadline)
			_ = emitReasoningDelta(reasoning.flush())
			_ = closeReasoningBlock()
			for _, step := range reconcileProcessTraceCitations(
				trace,
				content.String(),
			) {
				_ = emitProcessStep(step)
			}
			for _, step := range finishProcessTrace(
				trace,
				"failed",
				time.Now(),
				webSearchResult,
			) {
				_ = emitProcessStep(step)
			}
			finalizeTracked(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
				Status:  "failed",
				Content: content.String(),
				OutputBlocks: usedWebSearchOutputBlocks(
					assistantMessage.ID,
					content.String(),
					webSearchResult,
				),
				Metadata: withProcessTraceMessageMetadata(
					webMessageMetadata(
						autoDecision,
						map[string]any{"errorCode": errorBody.Code},
						content.String(),
					),
					reasoning.String(),
					trace,
				),
			})
			sequence++
			_ = writeSSEEvent(w, "message.error", streamEvent{
				Type:           "message.error",
				RunID:          runID,
				ConversationID: conversationID,
				MessageID:      assistantMessage.ID,
				Sequence:       sequence,
				CreatedAt:      formatTime(time.Now()),
				Error:          &errorBody,
			})
			flusher.Flush()
			return
		}

		switch providerEvent.Type {
		case ProviderEventSearchStarted:
			builtInSearchStarted = time.Now()
			fusionDiagnostics.WebExecuteOutcome = "provider_stream"
			step := startBuiltInWebProcessStep(
				trace,
				searchExecution,
				builtInSearchStarted,
			)
			if err := emitProcessStep(step); err != nil {
				cancelTrackedAfterWriteError(
					conversationID,
					assistantMessage.ID,
					runID,
					content.String(),
				)
				return
			}
		case ProviderEventSearchDegraded:
			fusionDiagnostics.WebExecuteDurationMillis =
				sourceFusionDurationMillis(builtInSearchStarted)
			fusionDiagnostics.WebExecuteOutcome = "degraded"
			fusionDiagnostics.DegradationReason = strings.TrimSpace(
				providerEvent.FailureCategory,
			)
			if fusionDiagnostics.DegradationReason == "" {
				fusionDiagnostics.DegradationReason = "provider_failed"
			}
			fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
			if step, ok := completeBuiltInWebProcessStep(
				trace,
				ProcessStepStatusFailed,
				time.Now(),
				webSearchResult,
				fusionDiagnostics.DegradationReason,
			); ok {
				if err := emitProcessStep(step); err != nil {
					cancelTrackedAfterWriteError(
						conversationID,
						assistantMessage.ID,
						runID,
						content.String(),
					)
					return
				}
			}
			searchExecution = nil
			modelBuiltInSearchProvider = nil
		case ProviderEventReasoningDelta:
			if providerEvent.ReasoningDelta == "" {
				continue
			}
			if _, exists := trace.get(ProcessStepKindReasoning); !exists {
				step := trace.start(
					ProcessStepKindReasoning,
					"process.reasoning",
					time.Now(),
					map[string]any{"outcome": "streaming"},
				)
				if err := emitProcessStep(step); err != nil {
					cancelTrackedAfterWriteError(
						conversationID,
						assistantMessage.ID,
						runID,
						content.String(),
					)
					return
				}
			}
			if err := emitReasoningDelta(
				reasoning.append(providerEvent.ReasoningDelta),
			); err != nil {
				cancelTrackedAfterWriteError(
					conversationID,
					assistantMessage.ID,
					runID,
					content.String(),
				)
				return
			}
		case ProviderEventDelta:
			if err := emitReasoningDelta(reasoning.flush()); err != nil {
				cancelTrackedAfterWriteError(
					conversationID,
					assistantMessage.ID,
					runID,
					content.String(),
				)
				return
			}
			if err := closeReasoningBlock(); err != nil {
				cancelTrackedAfterWriteError(
					conversationID, assistantMessage.ID, runID, content.String(),
				)
				return
			}
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
				cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
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
				cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
				return
			}
			flusher.Flush()
		case ProviderEventToolExecution:
			if err := emitReasoningDelta(reasoning.flush()); err != nil {
				cancelTrackedAfterWriteError(
					conversationID, assistantMessage.ID, runID, content.String(),
				)
				return
			}
			if err := closeReasoningBlock(); err != nil {
				cancelTrackedAfterWriteError(
					conversationID, assistantMessage.ID, runID, content.String(),
				)
				return
			}
			execution := providerEvent.ToolExecution
			if execution != nil && execution.Round > 0 {
				providerRound = execution.Round
			}
			eventAt := time.Now()
			processUpdates := toolTrace.apply(execution, eventAt)
			if execution != nil && execution.Transient {
				for _, step := range processUpdates {
					if err := emitTransientProcessStep(step); err != nil {
						cancelTrackedAfterWriteError(
							conversationID, assistantMessage.ID, runID, content.String(),
						)
						return
					}
				}
				continue
			}
			recordedTool, recordErr := agentRecorder.recordToolExecution(
				generationCtx, execution, processUpdates, eventAt,
			)
			if recordErr != nil {
				turnFinalStatus = ChatAgentTurnFailed
				turnFinalErrorCode = "AGENT_EVENT_PERSISTENCE_FAILED"
				streamCancel()
				cancelTrackedAfterWriteError(
					conversationID, assistantMessage.ID, runID, content.String(),
				)
				return
			}
			if emitErr := emitAgentEvent(recordedTool); emitErr != nil {
				cancelTrackedAfterWriteError(
					conversationID, assistantMessage.ID, runID, content.String(),
				)
				return
			}
			if recordedSteps := processStepsFromChatAgentEvent(recordedTool); len(recordedSteps) > 0 {
				processUpdates = recordedSteps
			}
			if !agentTimelineEnabled && execution != nil &&
				(execution.Mode == "mcp" || execution.Mode == localskills.RuntimeLocalDirect ||
					execution.Mode == localskills.RuntimeHostWorkspace) &&
				execution.CallStatus != "" {
				projectedTool := projectChatAgentToolExecution(recordedTool)
				if projectedTool == nil {
					turnFinalStatus = ChatAgentTurnFailed
					turnFinalErrorCode = "AGENT_EVENT_PROJECTION_FAILED"
					streamCancel()
					cancelTrackedAfterWriteError(
						conversationID, assistantMessage.ID, runID, content.String(),
					)
					return
				}
				sequence++
				if err := writeSSEEvent(w, "tool.call.updated", streamEvent{
					Type:           "tool.call.updated",
					RunID:          runID,
					ConversationID: conversationID,
					MessageID:      assistantMessage.ID,
					Sequence:       sequence,
					CreatedAt:      formatTime(time.Now()),
					ToolCall:       projectedTool,
				}); err != nil {
					cancelTrackedAfterWriteError(
						conversationID, assistantMessage.ID, runID, content.String(),
					)
					return
				}
				flusher.Flush()
			}
			if execution != nil && execution.Status == ProcessStepStatusCancelled {
				toolExecutionCancelled = true
			}
			for _, step := range processUpdates {
				if err := emitProcessStep(step); err != nil {
					cancelTrackedAfterWriteError(
						conversationID,
						assistantMessage.ID,
						runID,
						content.String(),
					)
					return
				}
			}
			if execution != nil && execution.Name == searchKnowledgeToolName {
				if execution.Knowledge != nil {
					autoDecision = *execution.Knowledge
				}
				switch execution.Status {
				case ProcessStepStatusCompleted, ProcessStepStatusFailed:
					fusionDiagnostics.KnowledgeDurationMillis =
						sourceFusionDurationMillis(knowledgeStarted)
					if autoDecision.ReadyForAnswer() {
						fusionPlan.Authority = sourceAuthorityKnowledge
						if len(webSearchResult.Sources) > 0 {
							fusionPlan.Authority = sourceAuthorityMixed
						}
					} else if len(webSearchResult.Sources) > 0 {
						fusionPlan.Authority = sourceAuthorityWeb
					} else {
						fusionPlan.Authority = sourceAuthorityModel
					}
				}
				continue
			}
			if execution == nil || execution.Name != searchWebToolName {
				continue
			}
			switch execution.Status {
			case ProcessStepStatusRunning:
				messageSearchExecution = searchExecution
				fusionPlan.SearchRequested = true
				if autoDecision.ReadyForAnswer() {
					fusionPlan.Authority = sourceAuthorityMixed
				} else {
					fusionPlan.Authority = sourceAuthorityWeb
				}
				fusionDiagnostics.WebQueryConversationRewriteOutcome = "provider_tool"
				fusionDiagnostics.WebExecuteOutcome = "running"
			case ProcessStepStatusCompleted:
				fusionDiagnostics.WebExecuteDurationMillis =
					sourceFusionDurationMillis(resolveStarted)
				if execution.Search == nil || len(execution.Search.Sources) == 0 {
					fusionDiagnostics.WebExecuteOutcome = "no_results"
				} else {
					fusionDiagnostics.WebExecuteOutcome = "completed"
				}
			case ProcessStepStatusFailed:
				fusionDiagnostics.WebExecuteDurationMillis =
					sourceFusionDurationMillis(resolveStarted)
				fusionDiagnostics.WebExecuteOutcome = "degraded"
				fusionDiagnostics.DegradationReason = strings.TrimSpace(
					execution.FailureCategory,
				)
				if execution.Mode == "compatibility" &&
					execution.FailureCategory == "planner_failed" {
					fusionDiagnostics.WebQueryConversationRewriteOutcome = "failed"
				} else {
					fusionDiagnostics.WebQueryConversationRewriteOutcome = "provider_tool"
					fusionPlan.SearchRequested = true
				}
				fusionPlan = fallbackSourceFusionAuthority(fusionPlan, autoDecision)
			case ProcessStepStatusCancelled:
				fusionDiagnostics.WebExecuteDurationMillis =
					sourceFusionDurationMillis(resolveStarted)
				fusionDiagnostics.WebExecuteOutcome = "cancelled"
				fusionDiagnostics.DegradationReason = ""
				if execution.Mode == "compatibility" &&
					execution.ExecutionID == "compatibility-plan" {
					fusionDiagnostics.WebQueryConversationRewriteOutcome = "not_run"
				} else {
					fusionDiagnostics.WebQueryConversationRewriteOutcome = "provider_tool"
					fusionPlan.SearchRequested = true
				}
			}
		case ProviderEventSearch:
			if providerEvent.Search == nil ||
				(len(providerEvent.Search.Sources) == 0 &&
					len(providerEvent.Search.Images) == 0) {
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
				cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
				return
			}
			flusher.Flush()
			if modelBuiltInSearchProvider != nil {
				if step, ok := completeBuiltInWebProcessStep(
					trace,
					ProcessStepStatusCompleted,
					time.Now(),
					webSearchResult,
					"",
				); ok {
					if err := emitProcessStep(step); err != nil {
						cancelTrackedAfterWriteError(
							conversationID,
							assistantMessage.ID,
							runID,
							content.String(),
						)
						return
					}
				}
			}
		case ProviderEventContextReplaced:
			recordedEvent, recordErr := agentRecorder.recordContextReplacement(
				generationCtx, providerEvent.ContextReplacement, time.Now(),
			)
			if recordErr != nil {
				turnFinalStatus = ChatAgentTurnFailed
				turnFinalErrorCode = "AGENT_EVENT_PERSISTENCE_FAILED"
				streamCancel()
				cancelTrackedAfterWriteError(
					conversationID, assistantMessage.ID, runID, content.String(),
				)
				return
			}
			if emitErr := emitAgentEvent(recordedEvent); emitErr != nil {
				cancelTrackedAfterWriteError(
					conversationID, assistantMessage.ID, runID, content.String(),
				)
				return
			}
		}
	}
	if err := emitReasoningDelta(reasoning.flush()); err != nil {
		cancelTrackedAfterWriteError(
			conversationID, assistantMessage.ID, runID, content.String(),
		)
		return
	}
	if err := closeReasoningBlock(); err != nil {
		cancelTrackedAfterWriteError(
			conversationID, assistantMessage.ID, runID, content.String(),
		)
		return
	}

	if streamDeadlineSource != "" && errors.Is(streamCtx.Err(), context.DeadlineExceeded) {
		_ = emitReasoningDelta(reasoning.flush())
		for _, step := range finishProcessTrace(trace, "failed", time.Now(), webSearchResult) {
			_ = emitProcessStep(step)
		}
		deadlineErr, legacyMCPDeadline := chatStreamDeadlineError(
			streamDeadlineSource, context.DeadlineExceeded, true,
		)
		errorBody := chatStreamErrorBody(deadlineErr, legacyMCPDeadline)
		finalizeTracked(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
			Status: "failed", Content: content.String(),
			OutputBlocks: usedWebSearchOutputBlocks(assistantMessage.ID, content.String(), webSearchResult),
			Metadata: withProcessTraceMessageMetadata(
				webMessageMetadata(autoDecision, map[string]any{"errorCode": errorBody.Code}, content.String()),
				reasoning.String(), trace,
			),
		})
		sequence++
		_ = writeSSEEvent(w, "message.error", streamEvent{
			Type: "message.error", RunID: runID, ConversationID: conversationID,
			MessageID: assistantMessage.ID, Sequence: sequence,
			CreatedAt: formatTime(time.Now()), Error: &errorBody,
		})
		flusher.Flush()
		return
	}
	if streamCtx.Err() != nil || toolExecutionCancelled ||
		h.isRunCancelled(context.Background(), runID) {
		streamCancel()
		_ = emitReasoningDelta(reasoning.flush())
		for _, step := range reconcileProcessTraceCitations(
			trace,
			content.String(),
		) {
			_ = emitProcessStep(step)
		}
		for _, step := range finishProcessTrace(
			trace,
			"cancelled",
			time.Now(),
			webSearchResult,
		) {
			_ = emitProcessStep(step)
		}
		finalizeTracked(context.Background(), conversationID, assistantMessage.ID, FinalizeAssistantMessageInput{
			Status:  "cancelled",
			Content: content.String(),
			OutputBlocks: usedWebSearchOutputBlocks(
				assistantMessage.ID,
				content.String(),
				webSearchResult,
			),
			Metadata: withProcessTraceMessageMetadata(
				webMessageMetadata(autoDecision, nil, content.String()),
				reasoning.String(),
				trace,
			),
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
	if searchMode == chatSearchModeExternal &&
		!fusionPlan.SearchRequested &&
		fusionDiagnostics.WebExecuteOutcome == "pending" {
		fusionDiagnostics.WebQueryConversationRewriteOutcome = "skipped"
		fusionDiagnostics.WebExecuteOutcome = "skipped"
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
				cancelTrackedAfterWriteError(conversationID, assistantMessage.ID, runID, content.String())
				return
			}
			flusher.Flush()
		}
	}

	completedContent := content.String()
	if continuation == nil {
		completedContent = reconcileProviderSourceMarkers(
			completedContent,
			autoDecision,
			webSearchResult,
		)
	}
	completedDecision := autoDecision.completed(completedContent)
	fusionPlan = reconcileCompletedSourceFusionAuthority(
		fusionPlan,
		completedContent,
		completedDecision,
		webSearchResult,
	)
	if err := emitReasoningDelta(reasoning.flush()); err != nil {
		cancelTrackedAfterWriteError(
			conversationID,
			assistantMessage.ID,
			runID,
			content.String(),
		)
		return
	}
	for _, step := range reconcileProcessTraceCitations(trace, completedContent) {
		if err := emitProcessStep(step); err != nil {
			cancelTrackedAfterWriteError(
				conversationID,
				assistantMessage.ID,
				runID,
				content.String(),
			)
			return
		}
	}
	for _, step := range finishProcessTrace(
		trace,
		"completed",
		time.Now(),
		webSearchResult,
	) {
		if err := emitProcessStep(step); err != nil {
			cancelTrackedAfterWriteError(
				conversationID,
				assistantMessage.ID,
				runID,
				content.String(),
			)
			return
		}
	}
	var memoryCapture *MemoryCaptureInput
	if continuation == nil {
		memoryCapture, err = newDurableMemoryCapture(
			userMessage.ID,
			*modelRef,
			request.Provider,
		)
	}
	if err != nil {
		finalizeTracked(
			context.Background(),
			conversationID,
			assistantMessage.ID,
			FinalizeAssistantMessageInput{
				Status:  "failed",
				Content: completedContent,
				Metadata: map[string]any{
					"runId":     runID,
					"errorCode": "MEMORY_CAPTURE_ID_FAILED",
				},
			},
		)
		sequence++
		_ = writeSSEEvent(w, "message.error", streamEvent{
			Type:           "message.error",
			RunID:          runID,
			ConversationID: conversationID,
			MessageID:      assistantMessage.ID,
			Sequence:       sequence,
			CreatedAt:      formatTime(time.Now()),
			Error: &ErrorBody{
				Code:    "INTERNAL_ERROR",
				Message: "internal server error",
			},
		})
		flusher.Flush()
		return
	}
	var memoryUsages []MemoryUsageInput
	if continuation == nil {
		memoryUsages = durableMemoryUsageInputsForRun(memoryPreparation, memoryToolRuntime)
	}
	assistantMessage, err = finalizeTracked(
		context.Background(),
		conversationID,
		assistantMessage.ID,
		FinalizeAssistantMessageInput{
			Status:  "completed",
			Content: completedContent,
			OutputBlocks: usedWebSearchOutputBlocks(
				assistantMessage.ID,
				completedContent,
				webSearchResult,
			),
			Metadata: withProcessTraceMessageMetadata(
				webMessageMetadata(completedDecision, nil, completedContent),
				reasoning.String(),
				trace,
			),
			MemoryCapture: memoryCapture,
			MemoryUsages:  memoryUsages,
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
	assistantDTO := h.newMessageDTO(r.Context(), assistantMessage)
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
	h.publishDurableMemoryWake(assistantMessage.memoryEventID)
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
	assistantDTO := h.newMessageDTO(r.Context(), assistantMessage)
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
	FailureStage   string
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

func (h *Handler) newKnowledgeToolRuntime(
	ctx context.Context,
	conversationID string,
	originalQueryText string,
	selection ragSelection,
	modelRef ModelRef,
	ragAnswerProcessor string,
) *knowledgeToolRuntime {
	if !selection.Enabled {
		return nil
	}
	session, _ := auth.SessionFromContext(ctx)
	governanceModelRef := modelRef
	if processor := strings.TrimSpace(ragAnswerProcessor); processor != "" {
		governanceModelRef.ProviderID = processor
	}
	return &knowledgeToolRuntime{
		Assembler:             h.ragAssembler,
		AnswerGate:            h.ragAnswerGate,
		ActorUserID:           session.UserID,
		SessionID:             session.ID,
		ConversationID:        conversationID,
		OriginalQueryText:     originalQueryText,
		SelectedCollectionIDs: append([]string(nil), selection.CollectionIDs...),
		GovernanceModelRef:    governanceModelRef,
	}
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
	if failureStage := strings.TrimSpace(decision.FailureStage); failureStage != "" {
		knowledgeMetadata["failureStage"] = failureStage
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
) (RuntimeProviderResolution, error) {
	if providerConfig == nil {
		if h.provider == nil {
			return RuntimeProviderResolution{}, ErrProviderRequired
		}
		return RuntimeProviderResolution{Provider: h.provider}, nil
	}
	if strings.TrimSpace(providerConfig.Source) == "server-default" {
		if h.providerResolver != nil {
			return h.providerResolver.ResolveRuntimeProvider(ctx, *providerConfig)
		}
		if h.provider == nil {
			return RuntimeProviderResolution{}, ErrProviderRequired
		}
		return RuntimeProviderResolution{Provider: h.provider}, nil
	}
	if strings.TrimSpace(providerConfig.Source) == "" &&
		strings.TrimSpace(providerConfig.ID) == "" &&
		strings.TrimSpace(providerConfig.Type) == "" &&
		strings.TrimSpace(providerConfig.BaseURL) == "" &&
		len(providerConfig.APIKeySecret) == 0 {
		if h.provider == nil {
			return RuntimeProviderResolution{}, ErrProviderRequired
		}
		return RuntimeProviderResolution{Provider: h.provider}, nil
	}
	if h.providerResolver == nil {
		return RuntimeProviderResolution{}, newValidationError(
			"PROVIDER_CONFIG_UNSUPPORTED",
			"runtime provider configuration is not supported",
		)
	}
	return h.providerResolver.ResolveRuntimeProvider(ctx, *providerConfig)
}

func (h *Handler) resolveChatSearchExecution(
	ctx context.Context,
	provider Provider,
	modelRef ModelRef,
	searchEnabled bool,
) (*websearch.ActiveExecution, ModelBuiltInSearchProvider, error) {
	if !searchEnabled {
		return nil, nil, nil
	}
	if h == nil || h.webSearchService == nil || !h.webSearchService.Configured() {
		return nil, nil, websearch.ErrNotConfigured
	}
	builtIn, ok := provider.(ModelBuiltInSearchProvider)
	if !ok {
		return nil, nil, errModelBuiltInSearchUnsupported
	}
	execution, err := h.webSearchService.ResolveModelBuiltIn(
		ctx,
		websearch.ModelBuiltInResolutionRequest{
			ProviderID: modelRef.ProviderID,
			ModelID:    modelRef.ModelID,
			Protocol:   builtIn.ModelBuiltInSearchID(),
		},
	)
	if err != nil {
		return nil, nil, err
	}
	if execution.Mode != websearch.ExecutionModelBuiltIn {
		return nil, nil, errModelBuiltInSearchUnsupported
	}
	if builtIn.ModelBuiltInSearchID() != execution.ModelBuiltIn {
		return nil, nil, errModelBuiltInSearchUnsupported
	}
	return &execution, builtIn, nil
}

func (h *Handler) resolveExternalSearchExecution(
	ctx context.Context,
) (*websearch.ActiveExecution, error) {
	if h == nil || h.webSearchService == nil || !h.webSearchService.Configured() {
		return nil, websearch.ErrNotConfigured
	}
	execution, err := h.webSearchService.ResolveExternal(ctx)
	if err != nil {
		return nil, err
	}
	if execution.Mode != websearch.ExecutionExternal || execution.External == nil {
		return nil, websearch.ErrInvalidConfig
	}
	return &execution, nil
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

func parseApprovalChildPath(path string) (string, string, bool) {
	remainder := strings.TrimPrefix(path, approvalsPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || !isUUID(parts[0]) || parts[1] == "" {
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

func writeMCPAdmissionError(w http.ResponseWriter, err error) {
	status := http.StatusConflict
	code := "MCP_SELECTION_INVALID"
	message := "The selected Tools configuration is invalid"
	switch {
	case errors.Is(err, mcpclient.ErrCredentialRequired),
		errors.Is(err, mcpclient.ErrCredentialInvalid),
		errors.Is(err, mcpclient.ErrServerNeedsAuth):
		code, message = "MCP_AUTH_REQUIRED", "Tool server authorization is required"
	case errors.Is(err, mcpclient.ErrServerUnavailable),
		errors.Is(err, mcpclient.ErrServerNotReady),
		errors.Is(err, mcpclient.ErrRemoteDisabled),
		errors.Is(err, mcpclient.ErrStdioDisabled),
		errors.Is(err, mcpclient.ErrDisabled):
		status = http.StatusServiceUnavailable
		code, message = "MCP_SERVER_UNAVAILABLE", "A selected Tool server is unavailable"
	case errors.Is(err, mcpclient.ErrSelectionLimit):
		code, message = "MCP_LIMIT_REACHED", "Too many Tool servers are enabled"
	case errors.Is(err, mcpclient.ErrServerNotFound),
		errors.Is(err, mcpclient.ErrToolNotFound),
		errors.Is(err, mcpclient.ErrSelectionInvalid):
		code, message = "MCP_AUTHORIZATION_FAILED", "The selected Tools are no longer authorized"
	}
	writeError(w, status, code, message)
}

func chatStreamErrorBody(err error, deadlineExceeded bool) ErrorBody {
	if deadlineExceeded {
		return ErrorBody{Code: "MCP_BUDGET_EXHAUSTED", Message: "Tools run time limit was reached"}
	}
	var failure *mcpRunFailure
	if errors.As(err, &failure) {
		message := "Tools execution failed"
		switch failure.code {
		case "MCP_OUTCOME_UNKNOWN":
			message = "A write Tool may have completed, so the run was stopped"
		case "MCP_AUTH_REQUIRED":
			message = "Tool server authorization is required"
		case "MCP_SERVER_UNAVAILABLE":
			message = "A selected Tool server became unavailable"
		case "MCP_MODEL_UNSUPPORTED":
			message = "The selected model does not support Tools"
		case "MCP_BUDGET_EXHAUSTED":
			message = "Tools run limit was reached"
		}
		return ErrorBody{Code: failure.code, Message: message}
	}
	var localFailure *localSkillRunFailure
	if errors.As(err, &localFailure) {
		message := "Local Skill execution failed"
		switch localFailure.code {
		case "SKILL_MODEL_UNSUPPORTED":
			message = "The selected model does not support Tools"
		case "LOCAL_SKILL_BUDGET_EXHAUSTED":
			message = "Local Skill run limit was reached"
		case "LOCAL_SKILL_CANCELED":
			message = "Local Skill execution was cancelled"
		case "LOCAL_SKILL_PROVIDER_FAILED":
			message = "The provider could not continue the local Skill Tool loop"
		case "AGENT_APPROVAL_PERSISTENCE_FAILED":
			message = "The Agent could not persist approval state"
		case "LOCAL_SKILL_REQUIRED_CALL_MISSING":
			message = "The provider did not load the required Skill before acting"
		}
		return ErrorBody{Code: localFailure.code, Message: message}
	}
	var agentFailure *chatAgentRunFailure
	if errors.As(err, &agentFailure) {
		message := "Chat Agent run failed"
		switch agentFailure.code {
		case "AGENT_VERIFICATION_REQUIRED":
			message = "The Agent changed state but could not verify the result before stopping"
		case "AGENT_GOAL_PERSISTENCE_FAILED":
			message = "The Agent could not persist Goal state"
		}
		return ErrorBody{Code: agentFailure.code, Message: message}
	}
	if category, ok := ProviderFailureCategoryOf(err); ok {
		switch category {
		case ProviderFailureStreamReadFailed, ProviderFailureStreamIncomplete:
			return ErrorBody{
				Code:    providerStreamInterruptedCode,
				Message: "provider response stream was interrupted; partial output was preserved",
			}
		}
	}
	return ErrorBody{Code: "PROVIDER_ERROR", Message: "provider stream failed"}
}

func chatStreamDeadlineError(
	source string,
	err error,
	deadlineExceeded bool,
) (error, bool) {
	if !deadlineExceeded || source != "local_skill" {
		return err, deadlineExceeded
	}
	return &localSkillRunFailure{
		code: "LOCAL_SKILL_BUDGET_EXHAUSTED", err: context.DeadlineExceeded,
	}, false
}

func serviceErrorFor(err error) (int, ErrorBody) {
	var memoryValidation usermemory.ValidationError
	if usermemory.IsStateConflict(err) && errors.As(err, &memoryValidation) {
		return http.StatusConflict, ErrorBody{Code: memoryValidation.Code, Message: memoryValidation.Message}
	}
	if errors.As(err, &memoryValidation) {
		return http.StatusBadRequest, ErrorBody{Code: memoryValidation.Code, Message: memoryValidation.Message}
	}
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
	if errors.Is(err, usermemory.ErrConversationPolicyNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "CONVERSATION_MEMORY_POLICY_NOT_FOUND", Message: "conversation memory policy not found"}
	}
	if errors.Is(err, usermemory.ErrGovernanceRepositoryRequired) {
		return http.StatusServiceUnavailable, ErrorBody{Code: "MEMORY_GOVERNANCE_UNAVAILABLE", Message: "memory governance is unavailable"}
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
	var approvalError ChatAgentApprovalError
	if errors.As(err, &approvalError) {
		switch approvalError.Code {
		case "CHAT_AGENT_APPROVAL_NOT_FOUND":
			return http.StatusNotFound, ErrorBody{
				Code: approvalError.Code, Message: "approval request not found",
			}
		case "CHAT_AGENT_APPROVAL_STALE_REVISION":
			return http.StatusConflict, ErrorBody{
				Code: approvalError.Code, Message: "approval request revision is stale",
			}
		case "CHAT_AGENT_APPROVAL_AUTHORITY_INVALID":
			return http.StatusNotFound, ErrorBody{
				Code: "CHAT_AGENT_APPROVAL_NOT_FOUND", Message: "approval request not found",
			}
		default:
			return http.StatusConflict, ErrorBody{
				Code: approvalError.Code, Message: "approval request could not be decided",
			}
		}
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
	if writer, ok := w.(interface {
		WriteSSEEvent(string, []byte) error
	}); ok {
		return writer.WriteSSEEvent(event, encoded)
	}
	sequence, _, _ := streamEventIdentity(encoded)
	_, err = w.Write(formatSSEFrame(event, encoded, sequence))
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
	config := stripRetiredLegacySkillSelection(ensureObject(conversation.Metadata))
	delete(config, conversationPermissionMetadataKey)
	permissionMode := conversation.PermissionMode
	if permissionMode != string(agenthost.PermissionReadOnly) &&
		permissionMode != string(agenthost.PermissionWorkspaceWrite) &&
		permissionMode != string(agenthost.PermissionFullAccess) {
		permissionMode = string(agenthost.PermissionWorkspaceWrite)
	}
	return ConversationDTO{
		ID:                conversation.ID,
		Title:             conversation.Title,
		Status:            conversation.Status,
		ModelRef:          newModelRef(conversation.ModelProvider, conversation.ModelID),
		MessageCount:      conversation.MessageCount,
		SystemInstruction: conversation.SystemPrompt,
		Pinned:            configBool(config, "pinned"),
		Config:            config,
		WorkspaceID:       conversation.WorkspaceID,
		PermissionMode:    permissionMode,
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
		AgentEvents:     append([]ChatAgentEvent(nil), message.AgentEvents...),
		ParentMessageID: message.ParentMessageID,
		CreatedAt:       formatTime(message.CreatedAt),
		UpdatedAt:       formatTime(message.UpdatedAt),
		CompletedAt:     formatOptionalTime(message.CompletedAt),
	}
}

func (h *Handler) newMessageDTO(ctx context.Context, message Message) ChatMessageDTO {
	dto := newMessageDTO(message)
	actor := auth.UserOrDevelopment(ctx)
	if h.agentTimelineEnabledFor(actor.ID) {
		dto.Metadata = metadataWithoutProcessTrace(dto.Metadata)
		return dto
	}
	dto.AgentEvents = nil
	dto.Metadata = metadataWithoutAgentTimelinePresentations(dto.Metadata)
	return dto
}

func metadataWithoutProcessTrace(metadata map[string]any) map[string]any {
	cloned := cloneJSONObject(ensureObject(metadata))
	delete(cloned, processTraceMetadataKey)
	return cloned
}

func metadataWithoutAgentTimelinePresentations(metadata map[string]any) map[string]any {
	cloned := cloneJSONObject(ensureObject(metadata))
	rawTrace, ok := cloned[processTraceMetadataKey]
	if !ok {
		return cloned
	}
	encoded, err := json.Marshal(rawTrace)
	if err != nil {
		delete(cloned, processTraceMetadataKey)
		return cloned
	}
	var steps []map[string]any
	if err := json.Unmarshal(encoded, &steps); err != nil {
		delete(cloned, processTraceMetadataKey)
		return cloned
	}
	for _, step := range steps {
		delete(step, "presentation")
	}
	cloned[processTraceMetadataKey] = steps
	return cloned
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
