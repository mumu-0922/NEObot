package ragproviders

import (
	"context"
	"encoding/json"
	"net/http"
)

const contentTypeJSON = "application/json; charset=utf-8"

type Handler struct {
	resolve StatusResolver
}

type StatusResolver func(context.Context) (StatusResponse, error)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewHandler(resolve StatusResolver) *Handler {
	if resolve == nil {
		resolve = StaticStatusResolver()
	}
	return &Handler{resolve: resolve}
}

func StaticStatusResolver() StatusResolver {
	return func(context.Context) (StatusResponse, error) {
		return Status(), nil
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/rag/provider-status" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	status, err := h.resolve(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "RAG_PROVIDER_STATUS_UNAVAILABLE", "RAG provider status is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
