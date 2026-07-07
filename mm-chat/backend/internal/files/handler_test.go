package files

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/storage"
)

const testFileID = "11111111-1111-4111-8111-111111111111"

func TestHandlerUploadsGetsDownloadsAndDeletesFile(t *testing.T) {
	repo := newFakeRepository()
	store := newFakeObjectStore()
	service := NewService(repo, store)
	service.newID = func() (string, error) { return testFileID, nil }
	handler := NewHandler(service, WithMaxUploadBytes(1024))

	rec := performMultipartRequest(
		handler,
		http.MethodPost,
		filesPath,
		"hello.txt",
		"text/plain",
		"hello file",
		map[string]string{"purpose": "chat"},
	)
	assertStatus(t, rec, http.StatusCreated)

	var uploaded FileRecordDTO
	decodeBody(t, rec, &uploaded)
	if uploaded.ID != testFileID || uploaded.FileName != "hello.txt" || uploaded.MimeType != "text/plain" {
		t.Fatalf("uploaded = %#v", uploaded)
	}
	if uploaded.SHA256 != "f3877e8a3d98f809d9f844060fbea2864a4b66980a22ff22297014d0c168db2e" {
		t.Fatalf("sha256 = %q", uploaded.SHA256)
	}
	if uploaded.DownloadURL != "/v1/files/"+testFileID+"/content" {
		t.Fatalf("downloadUrl = %q", uploaded.DownloadURL)
	}

	rec = performRequest(handler, http.MethodGet, filesPath+"/"+testFileID, "")
	assertStatus(t, rec, http.StatusOK)
	var metadata FileRecordDTO
	decodeBody(t, rec, &metadata)
	if metadata.ID != uploaded.ID || metadata.Purpose != "chat" {
		t.Fatalf("metadata = %#v", metadata)
	}

	rec = performRequest(handler, http.MethodGet, filesPath+"/"+testFileID+"/content?disposition=attachment", "")
	assertStatus(t, rec, http.StatusOK)
	if rec.Body.String() != "hello file" {
		t.Fatalf("download body = %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") || !strings.Contains(got, "hello.txt") {
		t.Fatalf("Content-Disposition = %q", got)
	}

	rec = performRequest(handler, http.MethodDelete, filesPath+"/"+testFileID, "")
	assertStatus(t, rec, http.StatusNoContent)
	if _, ok := store.objects[objectKeyFor(testFileID)]; ok {
		t.Fatalf("object key %q still exists after delete", objectKeyFor(testFileID))
	}

	rec = performRequest(handler, http.MethodGet, filesPath+"/"+testFileID, "")
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorCode(t, rec, "FILE_NOT_FOUND")
}

func TestHandlerUploadErrors(t *testing.T) {
	repo := newFakeRepository()
	store := newFakeObjectStore()
	service := NewService(repo, store)
	service.newID = func() (string, error) { return testFileID, nil }
	handler := NewHandler(service, WithMaxUploadBytes(5))

	t.Run("missing file", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("purpose", "chat"); err != nil {
			t.Fatalf("WriteField() error = %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, filesPath, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		handler.ServeHTTP(rec, req)
		assertStatus(t, rec, http.StatusBadRequest)
		assertErrorCode(t, rec, "FILE_REQUIRED")
	})

	t.Run("invalid purpose", func(t *testing.T) {
		rec := performMultipartRequest(handler, http.MethodPost, filesPath, "a.txt", "text/plain", "abc", map[string]string{"purpose": "bad"})
		assertStatus(t, rec, http.StatusBadRequest)
		assertErrorCode(t, rec, "INVALID_FILE_PURPOSE")
	})

	t.Run("too large", func(t *testing.T) {
		rec := performMultipartRequest(handler, http.MethodPost, filesPath, "a.txt", "text/plain", "abcdef", map[string]string{"purpose": "chat"})
		assertStatus(t, rec, http.StatusRequestEntityTooLarge)
		assertErrorCode(t, rec, "FILE_TOO_LARGE")
	})
}

func TestHandlerMetadataErrors(t *testing.T) {
	handler := NewHandler(NewService(newFakeRepository(), newFakeObjectStore()))

	rec := performRequest(handler, http.MethodGet, filesPath+"/not-a-uuid", "")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, "INVALID_FILE_ID")

	rec = performRequest(handler, http.MethodGet, filesPath+"/22222222-2222-4222-8222-222222222222", "")
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorCode(t, rec, "FILE_NOT_FOUND")

	rec = performRequest(handler, http.MethodPost, filesPath+"/"+testFileID, "")
	assertStatus(t, rec, http.StatusMethodNotAllowed)
	assertErrorCode(t, rec, "METHOD_NOT_ALLOWED")
}

func TestHandlerDownloadReturnsNotFoundWhenObjectMissing(t *testing.T) {
	repo := newFakeRepository()
	store := newFakeObjectStore()
	_, err := repo.CreateFile(context.Background(), CreateFileInput{
		ID:               testFileID,
		OriginalFilename: "missing.txt",
		MimeType:         "text/plain",
		ByteSize:         7,
		SHA256:           "2f05d4b689d270568c3d99898977a1f8aebf77292f41f63d326d9d116e01e0dc",
		StorageBackend:   "local",
		ObjectKey:        objectKeyFor(testFileID),
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	handler := NewHandler(NewService(repo, store))

	rec := performRequest(handler, http.MethodGet, filesPath+"/"+testFileID+"/content", "")
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorCode(t, rec, "FILE_NOT_FOUND")
}

func TestServiceUploadDeletesObjectWhenRepositoryInsertFails(t *testing.T) {
	repo := newFakeRepository()
	repo.createErr = errors.New("insert failed")
	store := newFakeObjectStore()
	service := NewService(repo, store)
	service.newID = func() (string, error) { return testFileID, nil }

	_, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "rollback.txt",
		MimeType:         "text/plain",
		Size:             8,
		Purpose:          "chat",
		Body:             strings.NewReader("rollback"),
	})
	if err == nil {
		t.Fatal("Upload() error = nil, want repository insert failure")
	}
	if _, ok := store.objects[objectKeyFor(testFileID)]; ok {
		t.Fatalf("object key %q still exists after repository insert failure", objectKeyFor(testFileID))
	}
}

func TestHandlerRequiresDatabaseAndStorage(t *testing.T) {
	rec := performMultipartRequest(
		NewHandler(NewService(nil, newFakeObjectStore())),
		http.MethodPost,
		filesPath,
		"a.txt",
		"text/plain",
		"abc",
		map[string]string{"purpose": "chat"},
	)
	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "DATABASE_REQUIRED")

	rec = performMultipartRequest(
		NewHandler(NewService(newFakeRepository(), nil)),
		http.MethodPost,
		filesPath,
		"a.txt",
		"text/plain",
		"abc",
		map[string]string{"purpose": "chat"},
	)
	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "STORAGE_REQUIRED")
}

func performRequest(handler http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	handler.ServeHTTP(rec, req)
	return rec
}

func performMultipartRequest(
	handler http.Handler,
	method string,
	path string,
	filename string,
	contentType string,
	payload string,
	fields map[string]string,
) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			panic(err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		panic(err)
	}
	if _, err := io.Copy(part, strings.NewReader(payload)); err != nil {
		panic(err)
	}
	if err := writer.Close(); err != nil {
		panic(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	handler.ServeHTTP(rec, req)
	return rec
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body ErrorResponse
	decodeBody(t, rec, &body)
	if body.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body=%s", body.Error.Code, want, rec.Body.String())
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(destination); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
	}
}

type fakeRepository struct {
	records   map[string]FileRecord
	createErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{records: map[string]FileRecord{}}
}

func (r *fakeRepository) CreateFile(_ context.Context, input CreateFileInput) (FileRecord, error) {
	if r.createErr != nil {
		return FileRecord{}, r.createErr
	}
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	record := FileRecord{
		ID:               input.ID,
		UserID:           DevUserID,
		OriginalFilename: input.OriginalFilename,
		MimeType:         input.MimeType,
		ByteSize:         input.ByteSize,
		SHA256:           input.SHA256,
		StorageBackend:   input.StorageBackend,
		ObjectKey:        input.ObjectKey,
		UploadStatus:     "available",
		Metadata:         input.Metadata,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.records[record.ID] = record
	return record, nil
}

func (r *fakeRepository) GetFile(_ context.Context, fileID string) (FileRecord, error) {
	record, ok := r.records[fileID]
	if !ok || record.DeletedAt != nil {
		return FileRecord{}, ErrFileNotFound
	}
	return record, nil
}

func (r *fakeRepository) MarkFileDeleted(_ context.Context, fileID string) (FileRecord, error) {
	record, ok := r.records[fileID]
	if !ok || record.DeletedAt != nil {
		return FileRecord{}, ErrFileNotFound
	}
	now := time.Date(2026, 7, 7, 12, 1, 0, 0, time.UTC)
	record.UploadStatus = "deleted"
	record.DeletedAt = &now
	record.UpdatedAt = now
	r.records[fileID] = record
	return record, nil
}

type fakeObjectStore struct {
	objects map[string]fakeObject
}

type fakeObject struct {
	payload     []byte
	contentType string
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{objects: map[string]fakeObject{}}
}

func (s *fakeObjectStore) Put(_ context.Context, key string, body io.Reader, size int64, contentType string) error {
	payload, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(payload)) != size {
		return errors.New("size mismatch")
	}
	s.objects[key] = fakeObject{payload: payload, contentType: contentType}
	return nil
}

func (s *fakeObjectStore) Get(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	object, ok := s.objects[key]
	if !ok {
		return nil, storage.ObjectInfo{}, storage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(object.payload)), storage.ObjectInfo{
		Key:         key,
		Size:        int64(len(object.payload)),
		ContentType: object.contentType,
		UpdatedAt:   time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (s *fakeObjectStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}
