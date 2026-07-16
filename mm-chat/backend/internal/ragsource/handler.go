package ragsource

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const maxRequestBytes = 8 << 10
const contentTypeJSON = "application/json; charset=utf-8"

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
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != InternalSourceObjectPath {
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method is not allowed")
		return
	}
	if h == nil || h.service == nil || !h.service.InternalTokenConfigured() {
		writeJSONError(w, http.StatusServiceUnavailable, "RAG_SOURCE_OBJECT_UNAVAILABLE", "source object gateway is unavailable")
		return
	}
	if !constantTimeTokenEqual(r.Header.Get(InternalTokenHeader), h.service.InternalToken()) {
		writeJSONError(w, http.StatusUnauthorized, "RAG_SOURCE_OBJECT_UNAUTHORIZED", "source object gateway token is invalid")
		return
	}

	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var payload SourceObjectRequest
	if err := decoder.Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_SOURCE_OBJECT_REQUEST", "source object request is invalid")
		return
	}
	input := SourceObjectInput{
		JobID:             payload.JobID,
		WorkerID:          payload.WorkerID,
		LeaseToken:        payload.LeaseToken,
		FileID:            payload.FileID,
		MaterializationID: payload.MaterializationID,
	}
	object, err := h.service.Fetch(r.Context(), input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	contentType := object.ContentType
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", object.ByteSize))
	w.Header().Set("X-MM-Chat-Source-SHA256", object.SHA256)
	w.Header().Set("X-MM-Chat-File-ID", object.FileID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(object.Body)
}

func constantTimeTokenEqual(got string, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == "" || want == "" {
		return false
	}
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

func writeServiceError(w http.ResponseWriter, err error) {
	var sourceErr *Error
	if errors.As(err, &sourceErr) {
		switch sourceErr.Code {
		case "INVALID_SOURCE_OBJECT_REQUEST":
			writeJSONError(w, http.StatusBadRequest, sourceErr.Code, sourceErr.Message)
		case "RAG_SOURCE_OBJECT_UNAVAILABLE":
			writeJSONError(w, http.StatusConflict, sourceErr.Code, sourceErr.Message)
		case "RAG_SOURCE_OBJECT_TOO_LARGE":
			writeJSONError(w, http.StatusRequestEntityTooLarge, sourceErr.Code, sourceErr.Message)
		case "RAG_SOURCE_OBJECT_HASH_MISMATCH", "RAG_SOURCE_OBJECT_MISMATCH":
			writeJSONError(w, http.StatusUnprocessableEntity, sourceErr.Code, sourceErr.Message)
		default:
			writeJSONError(w, http.StatusInternalServerError, "RAG_SOURCE_OBJECT_FAILED", "source object gateway failed")
		}
		return
	}
	if errors.Is(err, ErrServiceUnavailable) {
		writeJSONError(w, http.StatusServiceUnavailable, "RAG_SOURCE_OBJECT_UNAVAILABLE", "source object gateway is unavailable")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "RAG_SOURCE_OBJECT_FAILED", "source object gateway failed")
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
