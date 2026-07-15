package codejobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const (
	contentTypeJSON     = "application/json; charset=utf-8"
	executionsPath      = "/v1/code/executions"
	maxJSONRequestBytes = 128 << 10
	maxCodeCharacters   = 100_000
	defaultLanguage     = "python"
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
	case executionsPath:
		h.requireMethod(w, r, http.MethodPost, h.execute)
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

func (h *Handler) execute(w http.ResponseWriter, r *http.Request) {
	var request ExecuteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	if err := normalizeExecuteRequest(&request); err != nil {
		writeAdmissionError(w, err)
		return
	}

	response, err := h.service.Execute(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

var (
	errCodeRequired        = errors.New("code is required")
	errCodeTooLarge        = errors.New("code is too large")
	errModelRequired       = errors.New("model ref is required")
	errLanguageUnsupported = errors.New("code language is not supported")
)

func normalizeExecuteRequest(request *ExecuteRequest) error {
	codeText := strings.TrimSpace(request.Code)
	request.ModelRef.ProviderID = strings.TrimSpace(request.ModelRef.ProviderID)
	request.ModelRef.ModelID = strings.TrimSpace(request.ModelRef.ModelID)
	request.Language = strings.ToLower(strings.TrimSpace(request.Language))
	if request.Language == "" {
		request.Language = defaultLanguage
	}
	if codeText == "" {
		return errCodeRequired
	}
	if len([]rune(request.Code)) > maxCodeCharacters {
		return errCodeTooLarge
	}
	if request.ModelRef.ProviderID == "" || request.ModelRef.ModelID == "" {
		return errModelRequired
	}
	if request.Language != defaultLanguage {
		return errLanguageUnsupported
	}
	return nil
}

func writeAdmissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errCodeRequired):
		writeError(w, http.StatusBadRequest, "CODE_REQUIRED", "code is required")
	case errors.Is(err, errCodeTooLarge):
		writeError(w, http.StatusBadRequest, "CODE_TOO_LARGE", "code is too large")
	case errors.Is(err, errModelRequired):
		writeError(w, http.StatusBadRequest, "MODEL_REF_REQUIRED", "modelRef.providerId and modelRef.modelId are required")
	case errors.Is(err, errLanguageUnsupported):
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_CODE_LANGUAGE", "code language is not supported")
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
	case errors.Is(err, ErrCodeExecutionUnavailable):
		writeError(w, http.StatusNotImplemented, "CODE_EXECUTION_UNAVAILABLE", "code execution jobs are not configured")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", "request was cancelled")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "code execution request failed")
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
