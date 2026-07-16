package ragsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	testJobID             = "11111111-1111-4111-8111-111111111111"
	testWorkerID          = "22222222-2222-4222-8222-222222222222"
	testLeaseToken        = "33333333-3333-4333-8333-333333333333"
	testFileID            = "44444444-4444-4444-8444-444444444444"
	testMaterializationID = "55555555-5555-4555-8555-555555555555"
	testObjectKey         = "users/user-1/files/file-1"
	testInternalToken     = "unit-test-internal-token"
)

var testBody = []byte("%PDF-1.7 mm-chat private source gateway")

func TestServiceFetchReturnsVerifiedObject(t *testing.T) {
	repo := &fakeSourceRepository{metadata: testSourceMetadata(testBody)}
	store := newFakeSourceObjectStore()
	store.objects[testObjectKey] = fakeSourceObject{payload: testBody, contentType: "application/pdf"}
	service := NewService(repo, store, WithInternalToken(testInternalToken))

	object, err := service.Fetch(context.Background(), testSourceInput())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if !bytes.Equal(object.Body, testBody) {
		t.Fatalf("body = %q, want %q", object.Body, testBody)
	}
	if object.FileID != testFileID || object.SHA256 != sha256Hex(testBody) || object.ByteSize != int64(len(testBody)) || object.ContentType != "application/pdf" {
		t.Fatalf("object = %#v", object)
	}
	if repo.input != testSourceInput() {
		t.Fatalf("repository input = %#v", repo.input)
	}
	if store.getKey != testObjectKey {
		t.Fatalf("object key = %q, want %q", store.getKey, testObjectKey)
	}
}

func TestServiceRequiresConfiguredDependencies(t *testing.T) {
	store := newFakeSourceObjectStore()
	repo := &fakeSourceRepository{metadata: testSourceMetadata(testBody)}

	for _, tt := range []struct {
		name    string
		service *Service
	}{
		{name: "nil service", service: nil},
		{name: "missing repository", service: NewService(nil, store, WithInternalToken(testInternalToken))},
		{name: "missing store", service: NewService(repo, nil, WithInternalToken(testInternalToken))},
		{name: "missing token", service: NewService(repo, store)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.service.Fetch(context.Background(), testSourceInput())
			if !errors.Is(err, ErrServiceUnavailable) {
				t.Fatalf("Fetch() error = %v, want ErrServiceUnavailable", err)
			}
		})
	}
}

func TestServicePropagatesRepositoryServiceUnavailable(t *testing.T) {
	service := NewService(
		&fakeSourceRepository{err: ErrServiceUnavailable},
		newFakeSourceObjectStore(),
		WithInternalToken(testInternalToken),
	)

	_, err := service.Fetch(context.Background(), testSourceInput())

	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("Fetch() error = %v, want ErrServiceUnavailable", err)
	}
}

func TestServiceRedactsMismatches(t *testing.T) {
	repo := &fakeSourceRepository{metadata: testSourceMetadata(testBody)}
	store := newFakeSourceObjectStore()
	store.objects[testObjectKey] = fakeSourceObject{payload: []byte("tampered"), contentType: "application/pdf"}
	service := NewService(repo, store, WithInternalToken(testInternalToken))

	_, err := service.Fetch(context.Background(), testSourceInput())
	if err == nil {
		t.Fatal("Fetch() error = nil, want mismatch")
	}
	var sourceErr *Error
	if !errors.As(err, &sourceErr) || sourceErr.Code != "RAG_SOURCE_OBJECT_MISMATCH" {
		t.Fatalf("Fetch() error = %#v, want redacted mismatch", err)
	}
	if strings.Contains(err.Error(), testObjectKey) || strings.Contains(err.Error(), string(testBody)) {
		t.Fatalf("error leaked object key or payload: %v", err)
	}
}

func TestServiceRejectsUnsafeMetadataBeforeObjectStore(t *testing.T) {
	for _, objectKey := range []string{
		" users/user-1/files/file-1",
		"users/user-1/files/file-1/",
		"users//file-1",
		"users/../file-1",
		"users/./file-1",
		"/users/file-1",
		"users\\file-1",
		"users:file-1",
	} {
		t.Run(objectKey, func(t *testing.T) {
			metadata := testSourceMetadata(testBody)
			metadata.ObjectKey = objectKey
			repo := &fakeSourceRepository{metadata: metadata}
			store := newFakeSourceObjectStore()
			service := NewService(repo, store, WithInternalToken(testInternalToken))

			_, err := service.Fetch(context.Background(), testSourceInput())
			if err == nil {
				t.Fatal("Fetch() error = nil, want metadata mismatch")
			}
			var sourceErr *Error
			if !errors.As(err, &sourceErr) || sourceErr.Code != "RAG_SOURCE_OBJECT_MISMATCH" {
				t.Fatalf("Fetch() error = %#v, want metadata mismatch", err)
			}
			if store.getKey != "" {
				t.Fatalf("object store was called for unsafe key %q", objectKey)
			}
		})
	}
}

func TestHandlerReturnsRawBytesWithRedactedTokenGate(t *testing.T) {
	repo := &fakeSourceRepository{metadata: testSourceMetadata(testBody)}
	store := newFakeSourceObjectStore()
	store.objects[testObjectKey] = fakeSourceObject{payload: testBody, contentType: "application/pdf"}
	handler := NewHandler(NewService(repo, store, WithInternalToken(testInternalToken)))

	rec := performSourceRequest(handler, testInternalToken, testSourceRequestJSON())

	assertSourceStatus(t, rec, http.StatusOK)
	if !bytes.Equal(rec.Body.Bytes(), testBody) {
		t.Fatalf("body = %q, want source bytes", rec.Body.Bytes())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("Content-Type = %q, want application/pdf", got)
	}
	if got := rec.Header().Get("X-MM-Chat-Source-SHA256"); got != sha256Hex(testBody) {
		t.Fatalf("source hash = %q", got)
	}
	if got := rec.Header().Get("X-MM-Chat-File-ID"); got != testFileID {
		t.Fatalf("file id header = %q", got)
	}
}

func TestHandlerRejectsMissingWrongAndUnknownRequests(t *testing.T) {
	handler := NewHandler(NewService(
		&fakeSourceRepository{metadata: testSourceMetadata(testBody)},
		newFakeSourceObjectStore(),
		WithInternalToken(testInternalToken),
	))

	t.Run("missing token", func(t *testing.T) {
		rec := performSourceRequest(handler, "", testSourceRequestJSON())
		assertSourceStatus(t, rec, http.StatusUnauthorized)
		assertSourceErrorCode(t, rec, "RAG_SOURCE_OBJECT_UNAUTHORIZED")
	})

	t.Run("wrong token redacted", func(t *testing.T) {
		rec := performSourceRequest(handler, "wrong-secret-token", testSourceRequestJSON())
		assertSourceStatus(t, rec, http.StatusUnauthorized)
		assertSourceErrorCode(t, rec, "RAG_SOURCE_OBJECT_UNAUTHORIZED")
		if strings.Contains(rec.Body.String(), "wrong-secret-token") || strings.Contains(rec.Body.String(), testInternalToken) {
			t.Fatalf("token leaked in response: %s", rec.Body.String())
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		rec := performSourceRequest(handler, testInternalToken, strings.TrimSuffix(testSourceRequestJSON(), "}")+`,"objectKey":"`+testObjectKey+`"}`)
		assertSourceStatus(t, rec, http.StatusBadRequest)
		assertSourceErrorCode(t, rec, "INVALID_SOURCE_OBJECT_REQUEST")
		if strings.Contains(rec.Body.String(), testObjectKey) {
			t.Fatalf("object key leaked in invalid request response: %s", rec.Body.String())
		}
	})
}

func TestHandlerIsDefaultOffWhenServiceUnavailable(t *testing.T) {
	rec := performSourceRequest(NewHandler(nil), testInternalToken, testSourceRequestJSON())

	assertSourceStatus(t, rec, http.StatusServiceUnavailable)
	assertSourceErrorCode(t, rec, "RAG_SOURCE_OBJECT_UNAVAILABLE")
}

func testSourceInput() SourceObjectInput {
	return SourceObjectInput{
		JobID:             testJobID,
		WorkerID:          testWorkerID,
		LeaseToken:        testLeaseToken,
		FileID:            testFileID,
		MaterializationID: testMaterializationID,
	}
}

func testSourceMetadata(body []byte) SourceMetadata {
	return SourceMetadata{
		FileID:         testFileID,
		StorageBackend: "minio",
		ObjectKey:      testObjectKey,
		SHA256:         sha256Hex(body),
		ByteSize:       int64(len(body)),
		ContentType:    "application/pdf",
	}
}

func testSourceRequestJSON() string {
	payload := SourceObjectRequest{
		JobID:             testJobID,
		WorkerID:          testWorkerID,
		LeaseToken:        testLeaseToken,
		FileID:            testFileID,
		MaterializationID: testMaterializationID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func sha256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func performSourceRequest(handler http.Handler, token string, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, InternalSourceObjectPath, strings.NewReader(body))
	if token != "" {
		req.Header.Set(InternalTokenHeader, token)
	}
	handler.ServeHTTP(rec, req)
	return rec
}

func assertSourceStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
}

func assertSourceErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	if body.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body=%#v", body.Error.Code, want, body)
	}
}

type fakeSourceRepository struct {
	metadata SourceMetadata
	err      error
	input    SourceObjectInput
}

func (r *fakeSourceRepository) FetchParseSourceMetadata(_ context.Context, input SourceObjectInput) (SourceMetadata, error) {
	r.input = input
	if r.err != nil {
		return SourceMetadata{}, r.err
	}
	return r.metadata, nil
}

type fakeSourceObjectStore struct {
	objects map[string]fakeSourceObject
	getKey  string
	info    storage.ObjectInfo
	err     error
}

type fakeSourceObject struct {
	payload     []byte
	contentType string
}

func newFakeSourceObjectStore() *fakeSourceObjectStore {
	return &fakeSourceObjectStore{objects: map[string]fakeSourceObject{}}
}

func (s *fakeSourceObjectStore) Put(context.Context, string, io.Reader, int64, string) error {
	return nil
}

func (s *fakeSourceObjectStore) Get(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	s.getKey = key
	if s.err != nil {
		return nil, storage.ObjectInfo{}, s.err
	}
	object, ok := s.objects[key]
	if !ok {
		return nil, storage.ObjectInfo{}, storage.ErrObjectNotFound
	}
	info := storage.ObjectInfo{
		Key:         key,
		Size:        int64(len(object.payload)),
		ContentType: object.contentType,
		UpdatedAt:   time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC),
	}
	if s.info.Key != "" || s.info.Size != 0 || s.info.ContentType != "" {
		info = s.info
	}
	return io.NopCloser(bytes.NewReader(object.payload)), info, nil
}

func (s *fakeSourceObjectStore) Delete(context.Context, string) error {
	return nil
}
