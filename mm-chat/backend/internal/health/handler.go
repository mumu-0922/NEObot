package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const (
	contentTypeJSON = "application/json; charset=utf-8"
	readyTimeout    = 2 * time.Second
)

type ReadinessChecker interface {
	CheckReady(ctx context.Context) error
}

type Handler struct {
	version      string
	readyChecker ReadinessChecker
}

type StatusResponse struct {
	Status string `json:"status"`
}

type VersionResponse struct {
	Version string `json:"version"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(version string, readyChecker ...ReadinessChecker) *Handler {
	if version == "" {
		version = "dev"
	}

	var checker ReadinessChecker
	if len(readyChecker) > 0 {
		checker = readyChecker[0]
	}

	return &Handler{version: version, readyChecker: checker}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{Status: "healthy"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	if h.readyChecker != nil {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()

		if err := h.readyChecker.CheckReady(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{
				Error: ErrorBody{
					Code:    "DATABASE_NOT_READY",
					Message: "database readiness check failed",
				},
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, StatusResponse{Status: "ready"})
}

func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	writeJSON(w, http.StatusOK, VersionResponse{Version: h.version})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}

	w.Header().Set("Allow", method)
	writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{
		Error: ErrorBody{
			Code:    "METHOD_NOT_ALLOWED",
			Message: "method not allowed",
		},
	})
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}
