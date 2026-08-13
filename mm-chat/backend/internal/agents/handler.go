package agents

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	contentTypeJSON      = "application/json; charset=utf-8"
	agentsPath           = "/v1/agents"
	agentsPathBase       = agentsPath + "/"
	assistantsPath       = "/v1/assistants"
	libraryPath          = assistantsPath + "/library"
	libraryPathBase      = libraryPath + "/"
	marketPath           = assistantsPath + "/market"
	marketItemPathBase   = marketPath + "/items/"
	maxAssistantBodySize = int64(1 << 20)
)

type Handler struct{ service *Service }

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService()
	}
	return &Handler{service: service}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch {
	case request.URL.Path == agentsPath:
		handler.handleLegacyAgentList(w, request)
	case strings.HasPrefix(request.URL.Path, agentsPathBase):
		handler.handleLegacyAgentDetail(w, request, strings.TrimPrefix(request.URL.Path, agentsPathBase))
	case request.URL.Path == libraryPath:
		handler.handleLibrary(w, request)
	case strings.HasPrefix(request.URL.Path, libraryPathBase):
		handler.handleLibraryEntry(w, request, strings.TrimPrefix(request.URL.Path, libraryPathBase))
	case request.URL.Path == marketPath:
		handler.handleMarket(w, request)
	case strings.HasPrefix(request.URL.Path, marketItemPathBase):
		handler.handleMarketItem(w, request, strings.TrimPrefix(request.URL.Path, marketItemPathBase))
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (handler *Handler) handleLegacyAgentList(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	agents, err := handler.service.ListAgents(request.Context(), NormalizeLocale(request.URL.Query().Get("locale")))
	if err != nil {
		writeJSON(w, http.StatusOK, ListResponse{Agents: []Agent{}, Unavailable: true})
		return
	}
	writeJSON(w, http.StatusOK, ListResponse{Agents: agents})
}

func (handler *Handler) handleLegacyAgentDetail(w http.ResponseWriter, request *http.Request, identifier string) {
	if request.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if identifier == "" || strings.Contains(identifier, "/") {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	agent, err := handler.service.GetAgentDetail(request.Context(), identifier, NormalizeLocale(request.URL.Query().Get("locale")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (handler *Handler) handleLibrary(w http.ResponseWriter, request *http.Request) {
	user := auth.UserOrDevelopment(request.Context())
	switch request.Method {
	case http.MethodGet:
		entries, err := handler.service.ListLibrary(request.Context(), user.ID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"assistants": entries})
	case http.MethodPost:
		var input CreateCustomInput
		if !decodeJSON(w, request, &input) {
			return
		}
		entry, err := handler.service.CreateCustom(request.Context(), user.ID, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"assistant": entry})
	default:
		methodNotAllowed(w, "GET, POST")
	}
}

func (handler *Handler) handleLibraryEntry(w http.ResponseWriter, request *http.Request, suffix string) {
	parts := strings.Split(suffix, "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	entryID := parts[0]
	if len(parts) == 2 {
		handler.handleLibraryAction(w, request, user.ID, entryID, parts[1])
		return
	}
	switch request.Method {
	case http.MethodGet:
		entry, err := handler.service.GetLibrary(request.Context(), user.ID, entryID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"assistant": entry})
	case http.MethodPut:
		var input UpdateCustomInput
		if !decodeJSON(w, request, &input) {
			return
		}
		entry, err := handler.service.UpdateCustom(request.Context(), user.ID, entryID, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"assistant": entry})
	case http.MethodDelete:
		revision, ok := queryRevision(request)
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_ASSISTANT_REVISION", "assistant revision is invalid")
			return
		}
		entry, err := handler.service.GetLibrary(request.Context(), user.ID, entryID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		if entry.Source == SourceCustom {
			err = handler.service.DeleteCustom(request.Context(), user.ID, entryID, revision)
		} else {
			err = handler.service.Uninstall(request.Context(), user.ID, entryID, revision)
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, "GET, PUT, DELETE")
	}
}

func (handler *Handler) handleLibraryAction(w http.ResponseWriter, request *http.Request, userID, entryID, action string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var input struct {
		ExpectedRevision int64 `json:"expectedRevision"`
	}
	if !decodeJSON(w, request, &input) {
		return
	}
	var entry LibraryEntry
	var err error
	switch action {
	case "copy":
		entry, err = handler.service.CopyToCustom(request.Context(), userID, entryID, input.ExpectedRevision)
	case "update":
		entry, err = handler.service.UpdateInstalled(request.Context(), userID, entryID, input.ExpectedRevision)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	status := http.StatusOK
	if action == "copy" {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"assistant": entry})
}

func (handler *Handler) handleMarket(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	page, pageOK := queryInt(request, "page", 1)
	pageSize, pageSizeOK := queryInt(request, "pageSize", 20)
	if !pageOK || !pageSizeOK {
		writeError(w, http.StatusBadRequest, "INVALID_ASSISTANT_SEARCH", "assistant search is invalid")
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	result, err := handler.service.SearchMarket(request.Context(), user.ID, MarketSearchInput{
		Query: request.URL.Query().Get("q"), Category: request.URL.Query().Get("category"),
		Locale: NormalizeLocale(request.URL.Query().Get("locale")), Page: page, PageSize: pageSize,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *Handler) handleMarketItem(w http.ResponseWriter, request *http.Request, suffix string) {
	parts := strings.Split(suffix, "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	identifier := parts[0]
	if len(parts) == 1 && request.Method == http.MethodGet {
		agent, err := handler.service.MarketDetail(request.Context(), user.ID, identifier,
			NormalizeLocale(request.URL.Query().Get("locale")))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"assistant": agent, "canReview": handler.service.IsAdministrator(user.ID)})
		return
	}
	if len(parts) != 2 || request.Method != http.MethodPost {
		methodNotAllowed(w, "GET, POST")
		return
	}
	switch parts[1] {
	case "install":
		var input struct {
			Fingerprint string `json:"fingerprint"`
		}
		if !decodeJSON(w, request, &input) {
			return
		}
		entry, err := handler.service.Install(request.Context(), user.ID, identifier, input.Fingerprint)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"assistant": entry})
	case "review":
		var input struct {
			Status      string `json:"status"`
			Fingerprint string `json:"fingerprint"`
		}
		if !decodeJSON(w, request, &input) {
			return
		}
		err := handler.service.ReviewMarket(request.Context(), user.ID, identifier,
			input.Status, input.Fingerprint, NormalizeLocale(request.URL.Query().Get("locale")))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": input.Status})
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func decodeJSON(w http.ResponseWriter, request *http.Request, destination any) bool {
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxAssistantBodySize+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is invalid")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body is invalid")
		return false
	}
	return true
}

func queryInt(request *http.Request, name string, fallback int) (int, bool) {
	value := strings.TrimSpace(request.URL.Query().Get(name))
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func queryRevision(request *http.Request) (int64, bool) {
	value := strings.TrimSpace(request.URL.Query().Get("revision"))
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed >= 1
}

func writeServiceError(w http.ResponseWriter, err error) {
	var validationError ValidationError
	switch {
	case errors.As(err, &validationError):
		writeError(w, http.StatusBadRequest, validationError.Code, validationError.Message)
	case errors.Is(err, ErrLibraryNotFound):
		writeError(w, http.StatusNotFound, "ASSISTANT_NOT_FOUND", "assistant was not found")
	case errors.Is(err, ErrAdmissionNotFound):
		writeError(w, http.StatusForbidden, "ASSISTANT_NOT_ADMITTED", "assistant is not admitted")
	case errors.Is(err, ErrLibraryConflict):
		writeError(w, http.StatusConflict, "ASSISTANT_ALREADY_INSTALLED", "assistant is already installed")
	case errors.Is(err, ErrRevisionConflict):
		writeError(w, http.StatusConflict, "ASSISTANT_REVISION_CONFLICT", "assistant changed; reload before saving")
	case errors.Is(err, ErrMarketChanged):
		writeError(w, http.StatusConflict, "ASSISTANT_MARKET_CHANGED", "assistant detail changed; review it again")
	case errors.Is(err, ErrAdministratorRequired):
		writeError(w, http.StatusForbidden, "ASSISTANT_ADMIN_REQUIRED", "assistant administrator access is required")
	case errors.Is(err, ErrInvalidRegistryEntry):
		writeError(w, http.StatusBadGateway, "INVALID_AGENT_REGISTRY_RESPONSE", registryErrorMessage(err))
	case errors.Is(err, ErrRepositoryUnavailable):
		writeError(w, http.StatusServiceUnavailable, "ASSISTANT_LIBRARY_UNAVAILABLE", "assistant library is unavailable")
	default:
		writeError(w, http.StatusServiceUnavailable, "ASSISTANT_MARKET_UNAVAILABLE", "assistant market is unavailable")
	}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
