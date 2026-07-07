package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	contentTypeJSON        = "application/json; charset=utf-8"
	filesPath              = "/v1/files"
	filePathBase           = filesPath + "/"
	defaultMaxUploadBytes  = int64(25 << 20)
	multipartOverheadBytes = int64(1 << 20)
)

type Handler struct {
	service        *Service
	maxUploadBytes int64
}

type HandlerOption func(*Handler)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type FileRecordDTO struct {
	ID          string `json:"id"`
	FileName    string `json:"fileName"`
	MimeType    string `json:"mimeType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Purpose     string `json:"purpose"`
	CreatedAt   string `json:"createdAt"`
	DownloadURL string `json:"downloadUrl"`
}

func WithMaxUploadBytes(maxUploadBytes int64) HandlerOption {
	return func(h *Handler) {
		if maxUploadBytes > 0 {
			h.maxUploadBytes = maxUploadBytes
		}
	}
}

func NewHandler(service *Service, opts ...HandlerOption) *Handler {
	if service == nil {
		service = NewService(nil, nil)
	}
	handler := &Handler{
		service:        service,
		maxUploadBytes: defaultMaxUploadBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == filesPath:
		h.handleFiles(w, r)
	case strings.HasPrefix(r.URL.Path, filePathBase):
		h.handleFileChild(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	h.uploadFile(w, r)
}

func (h *Handler) handleFileChild(w http.ResponseWriter, r *http.Request) {
	fileID, child, ok := parseFileChildPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}

	if child == "content" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		h.downloadFile(w, r, fileID)
		return
	}
	if child != "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getFile(w, r, fileID)
	case http.MethodDelete:
		h.deleteFile(w, r, fileID)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodDelete)
	}
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	if err := h.service.requireReady(); err != nil {
		writeServiceError(w, err)
		return
	}

	maxBytes := h.maxUploadBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxUploadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+multipartOverheadBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file exceeds upload limit")
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_MULTIPART", "multipart upload body is invalid")
		return
	}
	if r.MultipartForm != nil {
		defer func() {
			_ = r.MultipartForm.RemoveAll()
		}()
	}

	file, header, err := r.FormFile("file")
	if errors.Is(err, http.ErrMissingFile) {
		writeError(w, http.StatusBadRequest, "FILE_REQUIRED", "file is required")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_MULTIPART", "file part is invalid")
		return
	}
	defer file.Close()
	if header.Size > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file exceeds upload limit")
		return
	}

	record, err := h.service.Upload(r.Context(), UploadInput{
		OriginalFilename: header.Filename,
		MimeType:         header.Header.Get("Content-Type"),
		Size:             header.Size,
		Purpose:          r.FormValue("purpose"),
		ConversationID:   r.FormValue("conversationId"),
		WorkspaceID:      r.FormValue("workspaceId"),
		CollectionID:     r.FormValue("knowledgeCollectionId"),
		ClientFileID:     r.FormValue("clientFileId"),
		Body:             file,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, newFileRecordDTO(record))
}

func (h *Handler) getFile(w http.ResponseWriter, r *http.Request, fileID string) {
	record, err := h.service.GetMetadata(r.Context(), fileID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newFileRecordDTO(record))
}

func (h *Handler) downloadFile(w http.ResponseWriter, r *http.Request, fileID string) {
	record, reader, err := h.service.GetContent(r.Context(), fileID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer reader.Close()

	contentType := record.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", record.ByteSize))
	if r.URL.Query().Get("disposition") == "attachment" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": record.OriginalFilename}))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func (h *Handler) deleteFile(w http.ResponseWriter, r *http.Request, fileID string) {
	if err := h.service.Delete(r.Context(), fileID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseFileChildPath(path string) (string, string, bool) {
	remainder := strings.TrimPrefix(path, filePathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func newFileRecordDTO(record FileRecord) FileRecordDTO {
	return FileRecordDTO{
		ID:          record.ID,
		FileName:    record.OriginalFilename,
		MimeType:    record.MimeType,
		Size:        record.ByteSize,
		SHA256:      record.SHA256,
		Purpose:     purposeFromMetadata(record.Metadata),
		CreatedAt:   formatTime(record.CreatedAt),
		DownloadURL: "/v1/files/" + record.ID + "/content",
	}
}

func purposeFromMetadata(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata["purpose"].(string)
	return value
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorFor(err)
	writeError(w, status, body.Code, body.Message)
}

func serviceErrorFor(err error) (int, ErrorBody) {
	if errors.Is(err, ErrDatabaseRequired) {
		return http.StatusServiceUnavailable, ErrorBody{Code: "DATABASE_REQUIRED", Message: "database is required for file endpoints"}
	}
	if errors.Is(err, ErrStorageRequired) {
		return http.StatusServiceUnavailable, ErrorBody{Code: "STORAGE_REQUIRED", Message: "storage is required for file endpoints"}
	}
	if errors.Is(err, ErrFileNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "FILE_NOT_FOUND", Message: "file not found"}
	}
	if errors.Is(err, storage.ErrObjectNotFound) {
		return http.StatusNotFound, ErrorBody{Code: "FILE_NOT_FOUND", Message: "file not found"}
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
