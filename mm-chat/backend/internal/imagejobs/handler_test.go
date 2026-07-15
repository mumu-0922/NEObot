package imagejobs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

func TestHandlerGenerateFailsClosedAfterAdmission(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		generationsPath,
		strings.NewReader(`{"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":"paint a quiet UI","count":1}`),
	)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusNotImplemented, "IMAGE_JOBS_UNAVAILABLE")
}

func TestHandlerGenerateReturnsStoredArtifactsWhenExecutorConfigured(t *testing.T) {
	executor := &fakeImageExecutor{result: GenerateResult{
		Images: []GeneratedImageResult{{
			JobID:       "job-1",
			Filename:    "image.png",
			ContentType: "image/png",
			Size:        5,
			Body:        strings.NewReader("image"),
		}},
		Message: "stored",
	}}
	store := &fakeArtifactStore{artifacts: []jobartifacts.Artifact{{
		FileID:      "file-1",
		Purpose:     "image",
		ContentType: "image/png",
		Size:        5,
	}}}
	handler := NewHandler(NewService(
		WithExecutor(executor),
		WithArtifactStore(store),
		WithAuditRecorder(noopAuditRecorder()),
	))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		generationsPath,
		strings.NewReader(`{"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":"paint","count":1}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response GenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Message != "stored" ||
		len(response.Images) != 1 ||
		response.Images[0] != (GeneratedImage{FileID: "file-1", Purpose: "image", ContentType: "image/png", Size: 5}) {
		t.Fatalf("response = %#v", response)
	}
	if !executor.called {
		t.Fatal("executor was not called")
	}
}

func TestHandlerGenerateValidatesRequestBeforeAdmission(t *testing.T) {
	handler := NewHandler(nil)
	longPrompt := strings.Repeat("x", maxPromptCharacters+1)
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "bad json", body: `{`, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "unknown provider object", body: `{"provider":{"apiKey":"sk-secret"},"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":"hello"}`, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "missing model", body: `{"modelRef":{"providerId":"openai"},"prompt":"hello"}`, status: http.StatusBadRequest, code: "MODEL_REF_REQUIRED"},
		{name: "empty prompt", body: `{"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":" "}`, status: http.StatusBadRequest, code: "PROMPT_REQUIRED"},
		{name: "large prompt", body: `{"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":"` + longPrompt + `"}`, status: http.StatusBadRequest, code: "PROMPT_TOO_LARGE"},
		{name: "bad count", body: `{"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":"hello","count":5}`, status: http.StatusBadRequest, code: "COUNT_INVALID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, generationsPath, strings.NewReader(tt.body))

			handler.ServeHTTP(rec, req)

			assertError(t, rec, tt.status, tt.code)
			if strings.Contains(rec.Body.String(), "sk-secret") {
				t.Fatalf("response leaked request secret: %s", rec.Body.String())
			}
		})
	}
}

func TestHandlerReturns503WhenAuditUnavailable(t *testing.T) {
	handler := NewHandler(NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return errors.New("audit sink down")
	}))))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, generationsPath, strings.NewReader(`{"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":"paint"}`))

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusServiceUnavailable, "JOB_AUDIT_UNAVAILABLE")
}

func TestHandlerRequiresPost(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, generationsPath, nil)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, status, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var response ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("error code = %q, want %q; body=%#v", response.Error.Code, code, response)
	}
}
