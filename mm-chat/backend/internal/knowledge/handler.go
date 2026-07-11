package knowledge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	collectionsPath       = "/v1/knowledge/collections"
	collectionsPathBase   = collectionsPath + "/"
	documentsPathBase     = "/v1/knowledge/documents/"
	maxCollectionBodySize = 8 << 10
)

type Handler struct{ service *Service }

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type collectionDTO struct {
	ID                           string      `json:"id"`
	Name                         string      `json:"name"`
	Description                  string      `json:"description"`
	Icon                         string      `json:"icon"`
	Color                        string      `json:"color"`
	Scope                        string      `json:"scope"`
	TeamID                       string      `json:"teamId,omitempty"`
	Permissions                  Permissions `json:"permissions"`
	ACLRevision                  int64       `json:"aclRevision"`
	VisibilityEpoch              int64       `json:"visibilityEpoch"`
	CollectionProcessingRevision int64       `json:"collectionProcessingRevision"`
	CreatedAt                    string      `json:"createdAt"`
	UpdatedAt                    string      `json:"updatedAt"`
}

type pageDTO struct {
	Items      []collectionDTO `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type documentDTO struct {
	ID             string              `json:"id"`
	CollectionID   string              `json:"collectionId"`
	Status         string              `json:"status"`
	CurrentVersion *documentVersionDTO `json:"currentVersion,omitempty"`
	PendingVersion *documentVersionDTO `json:"pendingVersion,omitempty"`
	CreatedAt      string              `json:"createdAt"`
	UpdatedAt      string              `json:"updatedAt"`
}

type documentVersionDTO struct {
	ID            string       `json:"id"`
	SourceVersion int64        `json:"sourceVersion"`
	File          DocumentFile `json:"file"`
	Status        string       `json:"status"`
	CreatedAt     string       `json:"createdAt"`
	UpdatedAt     string       `json:"updatedAt"`
	ErrorCode     string       `json:"errorCode,omitempty"`
}

type documentPageDTO struct {
	Items      []documentDTO `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService(nil)
	}
	return &Handler{service: service}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if _, err := url.ParseQuery(request.URL.RawQuery); err != nil {
		writeServiceError(w, invalidCollectionPayload("query parameters are invalid"))
		return
	}
	switch {
	case request.URL.Path == collectionsPath:
		handler.handleCollectionRoot(w, request)
	case strings.HasPrefix(request.URL.Path, collectionsPathBase):
		id := strings.TrimPrefix(request.URL.Path, collectionsPathBase)
		if collectionID, ok := strings.CutSuffix(id, "/documents"); ok && collectionID != "" && !strings.Contains(collectionID, "/") {
			handler.handleCollectionDocuments(w, request, collectionID)
			return
		}
		if id == "" || strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
			return
		}
		handler.handleCollection(w, request, id)
	case strings.HasPrefix(request.URL.Path, documentsPathBase):
		handler.handleDocument(w, request)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (handler *Handler) handleCollectionDocuments(w http.ResponseWriter, request *http.Request, collectionID string) {
	switch request.Method {
	case http.MethodGet:
		input, err := parseDocumentListQuery(request.URL.Query())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		page, err := handler.service.ListDocuments(request.Context(), collectionID, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		items := make([]documentDTO, 0, len(page.Items))
		for _, document := range page.Items {
			items = append(items, newDocumentDTO(document))
		}
		writeJSON(w, http.StatusOK, documentPageDTO{Items: items, NextCursor: page.NextCursor})
	case http.MethodPost:
		if err := requireNoQuery(request.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		var input BindDocumentInput
		if err := decodeStrictJSON(w, request, &input); err != nil {
			writeError(w, http.StatusBadRequest, ErrorCodeInvalidDocumentPayload, "document request body is invalid")
			return
		}
		document, err := handler.service.CreateDocument(request.Context(), collectionID, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, newDocumentDTO(document))
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (handler *Handler) handleDocument(w http.ResponseWriter, request *http.Request) {
	remainder := strings.TrimPrefix(request.URL.Path, documentsPathBase)
	parts := strings.Split(remainder, "/")
	if len(parts) < 1 || parts[0] == "" || len(parts) > 2 || (len(parts) == 2 && parts[1] != "content") {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if request.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if err := requireNoQuery(request.URL.Query()); err != nil {
		writeServiceError(w, err)
		return
	}
	if len(parts) == 1 {
		document, err := handler.service.GetDocument(request.Context(), parts[0])
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newDocumentDTO(document))
		return
	}
	metadata, reader, err := handler.service.GetDocumentContent(request.Context(), parts[0])
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer reader.Close()
	contentType := metadata.MIMEType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", metadata.ByteSize))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": metadata.FileName}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func (handler *Handler) handleCollectionRoot(w http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		input, err := parseListQuery(request.URL.Query())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		page, err := handler.service.ListCollections(request.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		items := make([]collectionDTO, 0, len(page.Items))
		for _, collection := range page.Items {
			items = append(items, newCollectionDTO(collection))
		}
		writeJSON(w, http.StatusOK, pageDTO{Items: items, NextCursor: page.NextCursor})
	case http.MethodPost:
		if err := requireNoQuery(request.URL.Query()); err != nil {
			writeServiceError(w, err)
			return
		}
		var input CreateCollectionInput
		if err := decodeStrictJSON(w, request, &input); err != nil {
			writeDecodeError(w, err)
			return
		}
		collection, err := handler.service.CreateCollection(request.Context(), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, newCollectionDTO(collection))
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
	}
}

func (handler *Handler) handleCollection(w http.ResponseWriter, request *http.Request, id string) {
	if err := requireNoQuery(request.URL.Query()); err != nil {
		writeServiceError(w, err)
		return
	}
	switch request.Method {
	case http.MethodGet:
		collection, err := handler.service.GetCollection(request.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newCollectionDTO(collection))
	case http.MethodPatch:
		var input UpdateCollectionInput
		if err := decodeStrictJSON(w, request, &input); err != nil {
			writeDecodeError(w, err)
			return
		}
		collection, err := handler.service.UpdateCollection(request.Context(), id, input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, newCollectionDTO(collection))
	case http.MethodDelete:
		if err := requireEmptyBody(w, request); err != nil {
			writeDecodeError(w, err)
			return
		}
		if err := handler.service.DeleteCollection(request.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeNoContent(w)
	default:
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPatch+", "+http.MethodDelete)
	}
}

func parseListQuery(query url.Values) (ListCollectionsInput, error) {
	input := ListCollectionsInput{}
	for key, values := range query {
		if forbiddenRequestField(key) {
			return input, forbiddenIdentityPayload()
		}
		if key != "scope" && key != "teamId" && key != "cursor" && key != "limit" {
			return input, invalidCollectionPayload("query parameters are invalid")
		}
		if len(values) != 1 {
			return input, invalidCollectionPayload("query parameters are invalid")
		}
		switch key {
		case "scope":
			input.Scope = values[0]
		case "teamId":
			input.TeamID = values[0]
		case "cursor":
			input.Cursor = values[0]
		case "limit":
			limit, err := strconv.Atoi(strings.TrimSpace(values[0]))
			if err != nil {
				return input, invalidCollectionPayload("query parameters are invalid")
			}
			input.Limit = limit
		}
	}
	return input, nil
}

func parseDocumentListQuery(query url.Values) (ListDocumentsInput, error) {
	input := ListDocumentsInput{}
	for key, values := range query {
		if forbiddenRequestField(key) {
			return input, forbiddenIdentityPayload()
		}
		if (key != "cursor" && key != "limit") || len(values) != 1 {
			return input, invalidCollectionPayload("query parameters are invalid")
		}
		if key == "cursor" {
			input.Cursor = values[0]
		} else {
			limit, err := strconv.Atoi(strings.TrimSpace(values[0]))
			if err != nil {
				return input, invalidCollectionPayload("query parameters are invalid")
			}
			input.Limit = limit
		}
	}
	return input, nil
}

func requireNoQuery(query url.Values) error {
	for key := range query {
		if forbiddenRequestField(key) {
			return forbiddenIdentityPayload()
		}
	}
	if len(query) != 0 {
		return invalidCollectionPayload("query parameters are invalid")
	}
	return nil
}

type forbiddenFieldError struct{ field string }

func (err forbiddenFieldError) Error() string { return "forbidden field " + err.field }

func decodeStrictJSON(w http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(w, request.Body, maxCollectionBodySize)
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	for key := range object {
		if forbiddenRequestField(key) {
			return forbiddenFieldError{field: key}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func requireEmptyBody(w http.ResponseWriter, request *http.Request) error {
	request.Body = http.MaxBytesReader(w, request.Body, maxCollectionBodySize)
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		for key := range object {
			if forbiddenRequestField(key) {
				return forbiddenFieldError{field: key}
			}
		}
	}
	return invalidCollectionPayload("request body must be empty")
}

func forbiddenRequestField(field string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(field), "_", ""), "-", ""))
	for _, token := range []string{"actor", "owner", "userid", "createdby", "acl", "revision", "epoch", "permission", "governance", "profile", "allowedcollection"} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return normalized == "role" || normalized == "teamrole"
}

func newCollectionDTO(value Collection) collectionDTO {
	return collectionDTO{ID: value.ID, Name: value.Name, Description: value.Description,
		Icon: value.Icon, Color: value.Color, Scope: value.Scope, TeamID: value.TeamID,
		Permissions: value.Permissions, ACLRevision: value.ACLRevision,
		VisibilityEpoch:              value.VisibilityEpoch,
		CollectionProcessingRevision: value.CollectionProcessingRevision,
		CreatedAt:                    value.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:                    value.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}

func newDocumentDTO(value Document) documentDTO {
	return documentDTO{ID: value.ID, CollectionID: value.CollectionID, Status: value.Status,
		CurrentVersion: newDocumentVersionDTO(value.CurrentVersion),
		PendingVersion: newDocumentVersionDTO(value.PendingVersion),
		CreatedAt:      value.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      value.UpdatedAt.UTC().Format(time.RFC3339Nano)}
}

func newDocumentVersionDTO(value *DocumentVersion) *documentVersionDTO {
	if value == nil {
		return nil
	}
	return &documentVersionDTO{ID: value.ID, SourceVersion: value.SourceVersion,
		File: value.File, Status: value.Status,
		CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: value.UpdatedAt.UTC().Format(time.RFC3339Nano), ErrorCode: value.ErrorCode}
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	var forbidden forbiddenFieldError
	switch {
	case errors.As(err, &maxErr):
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
	case errors.As(err, &forbidden):
		writeError(w, http.StatusBadRequest, ErrorCodeForbiddenIdentityField, "caller identity and authorization fields are not accepted")
	default:
		writeError(w, http.StatusBadRequest, ErrorCodeInvalidCollectionPayload, "collection request body is invalid")
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorFor(err)
	writeError(w, status, body.Code, body.Message)
}

func serviceErrorFor(err error) (int, ErrorBody) {
	switch {
	case errors.Is(err, ErrDatabaseRequired):
		return http.StatusServiceUnavailable, ErrorBody{"DATABASE_REQUIRED", "database is required for knowledge endpoints"}
	case errors.Is(err, ErrCursorCodecRequired):
		return http.StatusServiceUnavailable, ErrorBody{"CURSOR_CODEC_REQUIRED", "cursor codec is required for knowledge list endpoints"}
	case errors.Is(err, ErrUnauthenticated):
		return http.StatusUnauthorized, ErrorBody{"UNAUTHENTICATED", "session is invalid or expired"}
	case errors.Is(err, ErrCollectionNotFound):
		return http.StatusNotFound, ErrorBody{"COLLECTION_NOT_FOUND", "collection not found"}
	case errors.Is(err, ErrDocumentNotFound):
		return http.StatusNotFound, ErrorBody{"DOCUMENT_NOT_FOUND", "document not found"}
	case errors.Is(err, ErrStorageRequired):
		return http.StatusServiceUnavailable, ErrorBody{"STORAGE_REQUIRED", "storage is required for knowledge content"}
	case errors.Is(err, ErrTeamAdminRequired):
		return http.StatusForbidden, ErrorBody{"TEAM_ADMIN_REQUIRED", "team admin role is required"}
	case errors.Is(err, ErrIdempotencyConflict):
		return http.StatusConflict, ErrorBody{"IDEMPOTENCY_CONFLICT", "idempotency key conflicts with an existing request"}
	case errors.Is(err, ErrFileNotFound):
		return http.StatusNotFound, ErrorBody{"FILE_NOT_FOUND", "file not found"}
	case errors.Is(err, ErrProcessingConsent):
		return http.StatusForbidden, ErrorBody{"PROCESSING_CONSENT_REQUIRED", "processing consent is required"}
	}
	var validation ValidationError
	if errors.As(err, &validation) {
		return http.StatusBadRequest, ErrorBody{validation.Code, validation.Message}
	}
	return http.StatusInternalServerError, ErrorBody{"INTERNAL_ERROR", "internal server error"}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}
func writeNoContent(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNoContent)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{code, message}})
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
