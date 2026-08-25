package resourceorchestrator

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

const ResourcesPath = "/v1/resources"

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	user := auth.UserOrDevelopment(request.Context())
	switch request.URL.Path {
	case ResourcesPath:
		if request.Method != http.MethodGet {
			writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		conversationID := strings.TrimSpace(request.URL.Query().Get("conversationId"))
		if conversationID == "" {
			writeError(writer, http.StatusBadRequest, "INVALID_RESOURCE_QUERY", "conversation id is required")
			return
		}
		catalog, err := handler.service.Catalog(request.Context(), user.ID, conversationID)
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "RESOURCE_CATALOG_UNAVAILABLE", "resource catalog is unavailable")
			return
		}
		writeJSON(writer, http.StatusOK, catalog)
	case ResourcesPath + "/search":
		if request.Method != http.MethodGet {
			writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		kind := request.URL.Query().Get("kind")
		query := strings.Join(strings.Fields(request.URL.Query().Get("q")), " ")
		if len(query) > 200 {
			writeError(writer, http.StatusBadRequest, "INVALID_RESOURCE_QUERY", "resource query is invalid")
			return
		}
		result, err := handler.service.Search(request.Context(), user.ID, kind, query)
		if errors.Is(err, ErrInvalidQuery) {
			writeError(writer, http.StatusBadRequest, "INVALID_RESOURCE_QUERY", "resource query is invalid")
			return
		}
		if err != nil {
			writeError(writer, http.StatusServiceUnavailable, "RESOURCE_SEARCH_UNAVAILABLE", "resource search is unavailable")
			return
		}
		writeJSON(writer, http.StatusOK, result)
	case ResourcesPath + "/install":
		if request.Method != http.MethodPost {
			writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var input struct {
			Kind           string `json:"kind"`
			ID             string `json:"id"`
			Version        string `json:"version"`
			ExactRevision  string `json:"exactRevision"`
			ConversationID string `json:"conversationId"`
		}
		request.Body = http.MaxBytesReader(writer, request.Body, 16<<10)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(writer, http.StatusBadRequest, "INVALID_RESOURCE_REQUEST", "resource install request is invalid")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeError(writer, http.StatusBadRequest, "INVALID_RESOURCE_REQUEST", "resource install request is invalid")
			return
		}
		result, err := handler.service.InstallExplicit(request.Context(), InstallRequest{
			Kind: input.Kind, ID: input.ID, Version: input.Version,
			ExactRevision: input.ExactRevision, UserID: user.ID,
			ConversationID: input.ConversationID, EntryPoint: "control_plane",
		})
		switch {
		case errors.Is(err, ErrInvalidQuery):
			writeError(writer, http.StatusBadRequest, "INVALID_RESOURCE_REQUEST", "resource install request is invalid")
			return
		case errors.Is(err, ErrRevisionChanged):
			writeError(writer, http.StatusConflict, "RESOURCE_REVISION_CHANGED", "resource candidate changed; search again")
			return
		case errors.Is(err, ErrConfigurationRequired):
			writeError(
				writer, http.StatusConflict, "RESOURCE_CONFIGURATION_REQUIRED",
				"resource requires configuration in its management page",
			)
			return
		case errors.Is(err, ErrDisabled):
			writeError(
				writer, http.StatusServiceUnavailable, "RESOURCE_ORCHESTRATION_DISABLED",
				"resource installation is disabled",
			)
			return
		case errors.Is(err, ErrAuditUnavailable):
			writeError(
				writer, http.StatusServiceUnavailable, "RESOURCE_AUDIT_UNAVAILABLE",
				"resource installation audit is unavailable",
			)
			return
		case err != nil:
			writeError(writer, http.StatusServiceUnavailable, "RESOURCE_INSTALL_FAILED", "resource installation failed")
			return
		}
		writeJSON(writer, http.StatusOK, result)
	default:
		writeError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
