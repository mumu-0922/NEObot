package browserimport

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerPreviewReturnsIssuesWithoutWriting(t *testing.T) {
	repo := &fakeRepository{}
	handler := NewHandler(NewService(repo, WithNow(fixedNow)))
	manifest := validManifest()
	manifest.Messages[1].SequenceNo = 3

	rec := performMultipartRequest(t, handler, http.MethodPost, previewPath, testImportZip(t, manifest))
	assertStatus(t, rec, http.StatusOK)
	if repo.commits != 0 {
		t.Fatalf("repo commits = %d, want 0", repo.commits)
	}

	var response PreviewResponse
	decodeBody(t, rec, &response)
	if response.CommitAllowed {
		t.Fatalf("commitAllowed = true, want false; response=%#v", response)
	}
	assertIssueCode(t, response.Errors, "INVALID_MESSAGE_TREE")
}

func TestHandlerCommitCallsRepositoryForValidPackage(t *testing.T) {
	repo := &fakeRepository{response: CommitResponse{
		BatchID: "77777777-7777-4777-8777-777777777777",
		Status:  "completed",
		Created: CreatedCounts{Conversations: 1, Messages: 2},
		Mappings: ImportMappings{
			Conversations: map[string]string{"conversation-client-1": "11111111-1111-4111-8111-111111111111"},
			Messages:      map[string]string{"message-client-1": "22222222-2222-4222-8222-222222222222"},
			Files:         map[string]string{},
		},
	}}
	handler := NewHandler(NewService(repo, WithNow(fixedNow)))

	rec := performMultipartRequest(t, handler, http.MethodPost, browserImportPath, testImportZip(t, validManifest()))
	assertStatus(t, rec, http.StatusCreated)
	if repo.commits != 1 {
		t.Fatalf("repo commits = %d, want 1", repo.commits)
	}

	var response CommitResponse
	decodeBody(t, rec, &response)
	if response.Status != "completed" || response.BatchID != repo.response.BatchID {
		t.Fatalf("response = %#v", response)
	}
}

func TestHandlerCommitRequiresDatabase(t *testing.T) {
	handler := NewHandler(NewService(nil, WithNow(fixedNow)))
	rec := performMultipartRequest(t, handler, http.MethodPost, browserImportPath, testImportZip(t, validManifest()))
	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertErrorCode(t, rec, "DATABASE_REQUIRED")
}

func TestHandlerPreviewDoesNotRequireDatabase(t *testing.T) {
	handler := NewHandler(NewService(nil, WithNow(fixedNow)))
	rec := performMultipartRequest(t, handler, http.MethodPost, previewPath, testImportZip(t, validManifest()))
	assertStatus(t, rec, http.StatusOK)

	var response PreviewResponse
	decodeBody(t, rec, &response)
	if !response.CommitAllowed {
		t.Fatalf("commitAllowed = false; response=%#v", response)
	}
}

func TestHandlerRouteMatrix(t *testing.T) {
	repo := &fakeRepository{
		response: CommitResponse{
			BatchID:  "77777777-7777-4777-8777-777777777777",
			Status:   "completed",
			Mappings: ImportMappings{Conversations: map[string]string{}, Messages: map[string]string{}, Files: map[string]string{}},
		},
	}
	handler := NewHandler(NewService(repo, WithNow(fixedNow)))

	tests := []struct {
		name       string
		method     string
		path       string
		body       []byte
		wantStatus int
		wantCode   string
	}{
		{name: "preview method", method: http.MethodGet, path: previewPath, wantStatus: http.StatusMethodNotAllowed, wantCode: "METHOD_NOT_ALLOWED"},
		{name: "commit method", method: http.MethodGet, path: browserImportPath, wantStatus: http.StatusMethodNotAllowed, wantCode: "METHOD_NOT_ALLOWED"},
		{name: "get batch", method: http.MethodGet, path: browserImportPath + "/77777777-7777-4777-8777-777777777777", wantStatus: http.StatusOK},
		{name: "delete batch", method: http.MethodDelete, path: browserImportPath + "/77777777-7777-4777-8777-777777777777", wantStatus: http.StatusNoContent},
		{name: "invalid batch uuid", method: http.MethodGet, path: browserImportPath + "/bad", wantStatus: http.StatusBadRequest, wantCode: "INVALID_IMPORT_PAYLOAD"},
		{name: "missing batch", method: http.MethodGet, path: browserImportPath + "/88888888-8888-4888-8888-888888888888", wantStatus: http.StatusNotFound, wantCode: "IMPORT_BATCH_NOT_FOUND"},
	}
	repo.statusErrFor = "88888888-8888-4888-8888-888888888888"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(tt.body))
			handler.ServeHTTP(rec, req)
			assertStatus(t, rec, tt.wantStatus)
			if tt.wantCode != "" {
				assertErrorCode(t, rec, tt.wantCode)
			}
		})
	}
}

func TestHandlerRejectsMissingPackagePart(t *testing.T) {
	handler := NewHandler(NewService(nil, WithNow(fixedNow)))
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, previewPath, &buffer)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	handler.ServeHTTP(rec, req)

	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorCode(t, rec, "INVALID_IMPORT_PAYLOAD")
}

func performMultipartRequest(t *testing.T, handler http.Handler, method string, path string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	part, err := writer.CreateFormFile("package", "neo-chat-browser-import-v2.zip")
	if err != nil {
		t.Fatalf("create package part: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write package part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, &buffer)
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

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(destination); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
	}
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body ErrorResponse
	decodeBody(t, rec, &body)
	if body.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body=%#v", body.Error.Code, want, body)
	}
}

type fakeRepository struct {
	commits      int
	response     CommitResponse
	statusErrFor string
}

func (r *fakeRepository) Commit(_ context.Context, _ Package) (CommitResponse, error) {
	r.commits++
	return r.response, nil
}

func (r *fakeRepository) GetBatchStatus(_ context.Context, batchID string) (BatchStatusResponse, error) {
	if batchID == r.statusErrFor {
		return BatchStatusResponse{}, ErrBatchNotFound
	}
	return BatchStatusResponse{BatchID: batchID, Status: "completed", CreatedAt: "2026-07-07T00:00:00Z"}, nil
}

func (r *fakeRepository) Rollback(_ context.Context, _ string) error {
	return nil
}
