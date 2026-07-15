package imagejobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"neo-chat/mm-chat/backend/internal/jobaudit"
)

const (
	contentTypeJSON     = "application/json; charset=utf-8"
	generationsPath     = "/v1/images/generations"
	maxJSONRequestBytes = 32 << 10
	maxPromptCharacters = 20_000
	defaultImageCount   = 1
	maxImageCount       = 4
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
	switch r.URL.Path {
	case generationsPath:
		h.requireMethod(w, r, http.MethodPost, h.generate)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) requireMethod(
	w http.ResponseWriter,
	r *http.Request,
	method string,
	next func(http.ResponseWriter, *http.Request),
) {
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	next(w, r)
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	var request GenerateRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	if err := normalizeGenerateRequest(&request); err != nil {
		writeAdmissionError(w, err)
		return
	}

	response, err := h.service.Generate(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

var (
	errPromptRequired = errors.New("prompt is required")
	errPromptTooLarge = errors.New("prompt is too large")
	errModelRequired  = errors.New("model ref is required")
	errCountInvalid   = errors.New("image count is invalid")
)

func normalizeGenerateRequest(request *GenerateRequest) error {
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.ModelRef.ProviderID = strings.TrimSpace(request.ModelRef.ProviderID)
	request.ModelRef.ModelID = strings.TrimSpace(request.ModelRef.ModelID)
	request.Size = strings.TrimSpace(request.Size)
	if request.Prompt == "" {
		return errPromptRequired
	}
	if len([]rune(request.Prompt)) > maxPromptCharacters {
		return errPromptTooLarge
	}
	if request.ModelRef.ProviderID == "" || request.ModelRef.ModelID == "" {
		return errModelRequired
	}
	if request.Count == 0 {
		request.Count = defaultImageCount
	}
	if request.Count < 1 || request.Count > maxImageCount {
		return errCountInvalid
	}
	return nil
}

func writeAdmissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPromptRequired):
		writeError(w, http.StatusBadRequest, "PROMPT_REQUIRED", "prompt is required")
	case errors.Is(err, errPromptTooLarge):
		writeError(w, http.StatusBadRequest, "PROMPT_TOO_LARGE", "prompt is too large")
	case errors.Is(err, errModelRequired):
		writeError(w, http.StatusBadRequest, "MODEL_REF_REQUIRED", "modelRef.providerId and modelRef.modelId are required")
	case errors.Is(err, errCountInvalid):
		writeError(w, http.StatusBadRequest, "COUNT_INVALID", "count must be between 1 and 4")
	default:
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, jobaudit.ErrAuditUnavailable):
		writeError(w, http.StatusServiceUnavailable, "JOB_AUDIT_UNAVAILABLE", "job audit is unavailable")
	case errors.Is(err, ErrImageArtifactStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "IMAGE_ARTIFACT_STORE_UNAVAILABLE", "image artifact storage is not configured")
	case errors.Is(err, ErrImageJobsUnavailable):
		writeError(w, http.StatusNotImplemented, "IMAGE_JOBS_UNAVAILABLE", "image generation jobs are not configured")
	case errors.Is(err, ErrImageProviderFailed):
		writeError(w, http.StatusBadGateway, "IMAGE_PROVIDER_ERROR", "image provider request failed")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", "request was cancelled")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "image generation request failed")
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
