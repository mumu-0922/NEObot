package usermemory

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	memoriesPath             = "/v1/memories"
	memoriesPathBase         = memoriesPath + "/"
	memorySettingsPath       = "/v1/memory-settings"
	memoryActivitiesPath     = "/v1/memory-activities"
	memoryActivitiesPathBase = memoryActivitiesPath + "/"
	memoryUsagesPath         = "/v1/memory-usages"
	maxRequestBytes          = 64 * 1024
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
	Items []T `json:"items"`
}

type MemoryDTO struct {
	ID               string   `json:"id"`
	Type             string   `json:"type"`
	Content          string   `json:"content"`
	CreatedAt        int64    `json:"createdAt"`
	UpdatedAt        int64    `json:"updatedAt"`
	LastUsedAt       *int64   `json:"lastUsedAt,omitempty"`
	Importance       int      `json:"importance"`
	Tags             []string `json:"tags"`
	Source           string   `json:"source"`
	SourceSessionID  string   `json:"sourceSessionId,omitempty"`
	SourceMessageIDs []string `json:"sourceMessageIds,omitempty"`
}

type memoryRequest struct {
	Type       string   `json:"type"`
	Content    string   `json:"content"`
	Importance int      `json:"importance"`
	Tags       []string `json:"tags"`
}

type MemoryActivityDTO struct {
	ID                 string `json:"id"`
	AssistantMessageID string `json:"assistantMessageId"`
	Ordinal            int    `json:"ordinal"`
	SubjectType        string `json:"subjectType"`
	SubjectID          string `json:"subjectId"`
	SubjectRevision    *int64 `json:"subjectRevision,omitempty"`
	Action             string `json:"action"`
	Status             string `json:"status"`
	ReasonCode         string `json:"reasonCode"`
	UndoKind           string `json:"undoKind"`
	UndoStatus         string `json:"undoStatus"`
	MemoryType         string `json:"memoryType,omitempty"`
	MemoryContent      string `json:"memoryContent,omitempty"`
	MemoryRevision     *int64 `json:"memoryRevision,omitempty"`
	MemoryDeleted      bool   `json:"memoryDeleted,omitempty"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
}

type MemoryActivityPage struct {
	Items      []MemoryActivityDTO `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

type MessageMemoryUsageDTO struct {
	AssistantMessageID string `json:"assistantMessageId"`
	Ordinal            int    `json:"ordinal"`
	MemoryID           string `json:"memoryId"`
	MemoryRevision     int64  `json:"memoryRevision"`
	ScopeType          string `json:"scopeType"`
	MemoryType         string `json:"memoryType,omitempty"`
	MemoryContent      string `json:"memoryContent,omitempty"`
	MemoryDeleted      bool   `json:"memoryDeleted"`
	CreatedAt          int64  `json:"createdAt"`
}

type undoActivityRequest struct {
	ExpectedRevision int64 `json:"expectedRevision"`
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == memoryActivitiesPath:
		h.handleActivities(w, r)
	case strings.HasPrefix(r.URL.Path, memoryActivitiesPathBase):
		h.handleActivity(w, r)
	case r.URL.Path == memoryUsagesPath:
		h.handleUsages(w, r)
	case r.URL.Path == memoriesPath:
		h.handleCollection(w, r)
	case strings.HasPrefix(r.URL.Path, memoriesPathBase):
		memoryID := strings.TrimPrefix(r.URL.Path, memoriesPathBase)
		if memoryID == "" || strings.Contains(memoryID, "/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
			return
		}
		h.handleMemory(w, r, memoryID)
	case r.URL.Path == memorySettingsPath:
		h.handleSettings(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handleActivities(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_MEMORY_ACTIVITY_LIMIT", "activity limit is invalid")
			return
		}
		limit = parsed
	}
	items, err := h.service.ListActivities(
		r.Context(), r.URL.Query().Get("cursor"), limit,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response := MemoryActivityPage{Items: make([]MemoryActivityDTO, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, newMemoryActivityDTO(item))
	}
	if len(items) == limit && len(items) > 0 {
		response.NextCursor = items[len(items)-1].ID
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleActivity(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	resource := strings.TrimPrefix(r.URL.Path, memoryActivitiesPathBase)
	parts := strings.Split(resource, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "undo" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request undoActivityRequest
	if err := decodeJSON(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	result, err := h.service.UndoActivity(
		r.Context(), parts[0], request.ExpectedRevision,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleUsages(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	items, err := h.service.ListMessageUsages(
		r.Context(), r.URL.Query().Get("assistantMessageId"),
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	response := Page[MessageMemoryUsageDTO]{
		Items: make([]MessageMemoryUsageDTO, 0, len(items)),
	}
	for _, item := range items {
		response.Items = append(response.Items, MessageMemoryUsageDTO{
			AssistantMessageID: item.AssistantMessageID,
			Ordinal:            item.Ordinal,
			MemoryID:           item.MemoryID,
			MemoryRevision:     item.MemoryRevision,
			ScopeType:          item.ScopeType,
			MemoryType:         item.MemoryType,
			MemoryContent:      item.MemoryContent,
			MemoryDeleted:      item.MemoryDeleted,
			CreatedAt:          item.CreatedAt.UnixMilli(),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleCollection(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	switch r.Method {
	case http.MethodGet:
		memories, err := h.service.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		items := make([]MemoryDTO, 0, len(memories))
		for _, memory := range memories {
			items = append(items, newMemoryDTO(memory))
		}
		writeJSON(w, http.StatusOK, Page[MemoryDTO]{Items: items})
	case http.MethodPost:
		var request memoryRequest
		if err := decodeJSON(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		memory, err := h.service.CreateManual(r.Context(), Candidate{
			Type: request.Type, Content: request.Content,
			Importance: request.Importance, Tags: request.Tags,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, newMemoryDTO(memory))
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (h *Handler) handleMemory(w http.ResponseWriter, r *http.Request, memoryID string) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var request memoryRequest
		if err := decodeJSON(r, &request); err != nil {
			writeDecodeError(w, err)
			return
		}
		memory, err := h.service.Update(r.Context(), memoryID, Candidate{
			Type: request.Type, Content: request.Content,
			Importance: request.Importance, Tags: request.Tags,
		})
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newMemoryDTO(memory))
	case http.MethodDelete:
		if err := h.service.Delete(r.Context(), memoryID); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodPatch+", "+http.MethodDelete)
	}
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		writeServiceError(w, ErrDatabaseRequired)
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := h.service.GetSettings(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPatch:
		var patch SettingsPatch
		if err := decodeJSON(r, &patch); err != nil {
			writeDecodeError(w, err)
			return
		}
		settings, err := h.service.UpdateSettings(r.Context(), patch)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch)
	}
}

func newMemoryDTO(memory Memory) MemoryDTO {
	dto := MemoryDTO{
		ID: memory.ID, Type: memory.Type, Content: memory.Content,
		CreatedAt: memory.CreatedAt.UnixMilli(), UpdatedAt: memory.UpdatedAt.UnixMilli(),
		Importance: memory.Importance, Tags: memory.Tags, Source: memory.Source,
		SourceSessionID: memory.SourceConversationID,
	}
	if dto.Tags == nil {
		dto.Tags = []string{}
	}
	if memory.SourceMessageID != "" {
		dto.SourceMessageIDs = []string{memory.SourceMessageID}
	}
	if memory.LastUsedAt != nil {
		value := memory.LastUsedAt.UnixMilli()
		dto.LastUsedAt = &value
	}
	return dto
}

func newMemoryActivityDTO(activity MemoryActivity) MemoryActivityDTO {
	return MemoryActivityDTO{
		ID:                 activity.ID,
		AssistantMessageID: activity.AssistantMessageID,
		Ordinal:            activity.Ordinal,
		SubjectType:        activity.SubjectType,
		SubjectID:          activity.SubjectID,
		SubjectRevision:    activity.SubjectRevision,
		Action:             activity.Action,
		Status:             activity.Status,
		ReasonCode:         activity.ReasonCode,
		UndoKind:           activity.UndoKind,
		UndoStatus:         activity.UndoStatus,
		MemoryType:         activity.MemoryType,
		MemoryContent:      activity.MemoryContent,
		MemoryRevision:     activity.MemoryRevision,
		MemoryDeleted:      activity.MemoryDeleted,
		CreatedAt:          activity.CreatedAt.UnixMilli(),
		UpdatedAt:          activity.UpdatedAt.UnixMilli(),
	}
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain one JSON object")
		}
		return err
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	message := "request body must be valid JSON"
	if strings.Contains(err.Error(), "unknown field") {
		message = "request body contains an unsupported field"
	}
	writeError(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", message)
}

func writeServiceError(w http.ResponseWriter, err error) {
	var validationError ValidationError
	switch {
	case errors.As(err, &validationError):
		writeError(w, http.StatusBadRequest, validationError.Code, validationError.Message)
	case errors.Is(err, ErrMemoryNotFound):
		writeError(w, http.StatusNotFound, "MEMORY_NOT_FOUND", "memory not found")
	case errors.Is(err, ErrMemoryConflict):
		writeError(w, http.StatusConflict, "MEMORY_CONFLICT", "memory content already exists")
	case errors.Is(err, ErrActivityNotFound):
		writeError(w, http.StatusNotFound, "MEMORY_ACTIVITY_NOT_FOUND", "memory activity not found")
	case errors.Is(err, ErrActivityUndoUnavailable):
		writeError(w, http.StatusConflict, "MEMORY_ACTIVITY_UNDO_UNAVAILABLE", "memory activity undo is unavailable")
	case errors.Is(err, ErrActionRepositoryRequired):
		writeError(w, http.StatusServiceUnavailable, "MEMORY_ACTIONS_UNAVAILABLE", "memory actions are unavailable")
	case errors.Is(err, ErrDatabaseRequired):
		writeError(w, http.StatusServiceUnavailable, "DATABASE_REQUIRED", "memory database is required")
	default:
		writeError(w, http.StatusInternalServerError, "MEMORY_OPERATION_FAILED", "memory operation failed")
	}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
