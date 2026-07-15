package jobcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	contentTypeJSON = "application/json; charset=utf-8"
	jobsPathBase    = "/v1/jobs/"
	cancelSegment   = "cancel"
	maxJobIDLength  = 128
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

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	jobID, ok := parseCancelPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	if err := h.service.Cancel(r.Context(), jobID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": jobID, "status": "cancelled"})
}

func parseCancelPath(path string) (string, bool) {
	if !strings.HasPrefix(path, jobsPathBase) {
		return "", false
	}
	remainder := strings.TrimPrefix(path, jobsPathBase)
	jobID, tail, ok := strings.Cut(remainder, "/")
	if !ok || tail != cancelSegment || !validJobID(jobID) {
		return "", false
	}
	return jobID, true
}

func validJobID(value string) bool {
	if value == "" || len(value) > maxJobIDLength {
		return false
	}
	for _, current := range value {
		if current >= 'a' && current <= 'z' ||
			current >= 'A' && current <= 'Z' ||
			current >= '0' && current <= '9' ||
			current == '-' || current == '_' || current == '.' {
			continue
		}
		return false
	}
	return true
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrJobCancellationUnavailable):
		writeError(w, http.StatusNotImplemented, "JOB_CANCELLATION_UNAVAILABLE", "job cancellation is not configured")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", "request was cancelled")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "job cancellation request failed")
	}
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
