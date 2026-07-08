package browserimport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	contentTypeJSON        = "application/json; charset=utf-8"
	browserImportPath      = "/v1/import/browser"
	browserImportPathBase  = browserImportPath + "/"
	previewPath            = browserImportPath + "/preview"
	multipartOverheadBytes = int64(1 << 20)
)

type Handler struct {
	service         *Service
	maxPackageBytes int64
}

type HandlerOption func(*Handler)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func WithHandlerMaxPackageBytes(maxPackageBytes int64) HandlerOption {
	return func(h *Handler) {
		if maxPackageBytes > 0 {
			h.maxPackageBytes = maxPackageBytes
		}
	}
}

func NewHandler(service *Service, opts ...HandlerOption) *Handler {
	if service == nil {
		service = NewService(nil)
	}
	handler := &Handler{service: service, maxPackageBytes: defaultMaxPackageBytes}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == previewPath:
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.preview(w, r)
	case r.URL.Path == browserImportPath:
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.commit(w, r)
	case strings.HasPrefix(r.URL.Path, browserImportPathBase):
		h.handleBatch(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	input, cleanup, err := h.packageInput(w, r)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer cleanup()

	response, err := h.service.Preview(r.Context(), input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) commit(w http.ResponseWriter, r *http.Request) {
	input, cleanup, err := h.packageInput(w, r)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer cleanup()

	response, err := h.service.Commit(r.Context(), input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) handleBatch(w http.ResponseWriter, r *http.Request) {
	batchID, ok := parseBatchPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		response, err := h.service.GetBatchStatus(r.Context(), batchID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodDelete:
		if err := h.service.Rollback(r.Context(), batchID); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodDelete)
	}
}

func (h *Handler) packageInput(w http.ResponseWriter, r *http.Request) (PackageInput, func(), error) {
	maxBytes := h.maxPackageBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxPackageBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+multipartOverheadBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return PackageInput{}, func() {}, newValidationError("FILE_TOO_LARGE", "import package exceeds upload limit")
		}
		return PackageInput{}, func() {}, newValidationError("INVALID_IMPORT_PAYLOAD", "multipart import body is invalid")
	}
	cleanup := func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}

	file, _, err := r.FormFile("package")
	if errors.Is(err, http.ErrMissingFile) {
		cleanup()
		return PackageInput{}, func() {}, newValidationError("INVALID_IMPORT_PAYLOAD", "package part is required")
	}
	if err != nil {
		cleanup()
		return PackageInput{}, func() {}, newValidationError("INVALID_IMPORT_PAYLOAD", "package part is invalid")
	}
	wrappedCleanup := func() {
		_ = file.Close()
		cleanup()
	}
	return PackageInput{Reader: file}, wrappedCleanup, nil
}

func parseBatchPath(routePath string) (string, bool) {
	remainder := strings.TrimPrefix(routePath, browserImportPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], true
	}
	return "", false
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorFor(err)
	writeError(w, status, body.Code, body.Message)
}

func serviceErrorFor(err error) (int, ErrorBody) {
	if errors.Is(err, ErrDatabaseRequired) {
		return http.StatusServiceUnavailable, ErrorBody{Code: "DATABASE_REQUIRED", Message: "database is required for import endpoints"}
	}
	if errors.Is(err, ErrStorageRequired) {
		return http.StatusServiceUnavailable, ErrorBody{Code: "STORAGE_REQUIRED", Message: "storage is required for import file attachments"}
	}
	if errors.Is(err, ErrIdempotencyConflict) {
		return http.StatusConflict, ErrorBody{Code: "IDEMPOTENCY_CONFLICT", Message: "idempotency key conflicts with a different import package"}
	}
	if errors.Is(err, ErrBatchNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "IMPORT_BATCH_NOT_FOUND", Message: "import batch not found"}
	}
	if errors.Is(err, ErrBatchModified) {
		return http.StatusConflict, ErrorBody{Code: "IMPORT_BATCH_MODIFIED", Message: "import batch has been modified after import"}
	}
	var validationError ValidationError
	if errors.As(err, &validationError) {
		if validationError.Code == "FILE_TOO_LARGE" {
			return http.StatusRequestEntityTooLarge, ErrorBody{Code: validationError.Code, Message: validationError.Message}
		}
		return http.StatusBadRequest, ErrorBody{Code: validationError.Code, Message: validationError.Message}
	}
	return http.StatusInternalServerError, ErrorBody{Code: "INTERNAL_ERROR", Message: "internal server error"}
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
