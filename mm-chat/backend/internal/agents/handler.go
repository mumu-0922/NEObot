package agents

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	contentTypeJSON = "application/json; charset=utf-8"
	agentsPath      = "/v1/agents"
	agentsPathBase  = agentsPath + "/"
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

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService()
	}
	return &Handler{service: service}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch {
	case request.URL.Path == agentsPath:
		handler.handleAgentList(w, request)
	case strings.HasPrefix(request.URL.Path, agentsPathBase):
		identifier := strings.TrimPrefix(request.URL.Path, agentsPathBase)
		if identifier == "" || strings.Contains(identifier, "/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
			return
		}
		handler.handleAgentDetail(w, request, identifier)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (handler *Handler) handleAgentList(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	locale := NormalizeLocale(request.URL.Query().Get("locale"))
	agents, err := handler.service.ListAgents(request.Context(), locale)
	if err != nil {
		writeJSON(w, http.StatusOK, ListResponse{Agents: []Agent{}, Unavailable: true})
		return
	}
	writeJSON(w, http.StatusOK, ListResponse{Agents: agents})
}

func (handler *Handler) handleAgentDetail(w http.ResponseWriter, request *http.Request, identifier string) {
	if request.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	locale := NormalizeLocale(request.URL.Query().Get("locale"))
	agent, err := handler.service.GetAgentDetail(request.Context(), identifier, locale)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func writeServiceError(w http.ResponseWriter, err error) {
	var validationError ValidationError
	if errors.As(err, &validationError) {
		writeError(w, http.StatusBadRequest, validationError.Code, validationError.Message)
		return
	}
	if errors.Is(err, ErrInvalidRegistryEntry) {
		writeError(w, http.StatusBadGateway, "INVALID_AGENT_REGISTRY_RESPONSE", registryErrorMessage(err))
		return
	}
	writeError(w, http.StatusInternalServerError, "AGENT_REGISTRY_UNAVAILABLE", registryErrorMessage(err))
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
	_ = json.NewEncoder(w).Encode(payload)
}
