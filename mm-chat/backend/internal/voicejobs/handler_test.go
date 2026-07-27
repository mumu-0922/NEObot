package voicejobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

func TestHandlerSynthesizeFailsClosedAfterAdmission(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		synthesizePath,
		strings.NewReader(`{"text":"hello","provider":"default","voiceId":"voice-1"}`),
	)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusNotImplemented, "VOICE_JOBS_UNAVAILABLE")
}

func TestHandlerSynthesizeReturnsStoredArtifactWhenExecutorConfigured(t *testing.T) {
	executor := &fakeVoiceExecutor{synthesizeResult: SynthesizeResult{
		JobID:       "job-1",
		Filename:    "speech.webm",
		ContentType: "audio/webm",
		Size:        5,
		Body:        strings.NewReader("audio"),
	}}
	store := &fakeArtifactStore{artifact: jobartifacts.Artifact{
		FileID:      "file-1",
		Purpose:     "audio",
		ContentType: "audio/webm",
		Size:        5,
	}}
	handler := NewHandler(NewService(WithExecutor(executor), WithArtifactStore(store), WithAuditRecorder(noopAuditRecorder())))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		synthesizePath,
		strings.NewReader(`{"text":"hello","provider":"default","jobId":"job-1","voiceId":"voice-1"}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response SynthesizeResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response != (SynthesizeResponse{FileID: "file-1", Purpose: "audio", ContentType: "audio/webm", Size: 5}) {
		t.Fatalf("response = %#v", response)
	}
	if !executor.synthesizeCalled {
		t.Fatal("executor was not called")
	}
}

func TestHandlerSynthesizeMapsProviderErrorsWithoutLeakingText(t *testing.T) {
	executor := &fakeVoiceExecutor{err: ErrVoiceProviderFailed}
	store := &fakeArtifactStore{artifact: jobartifacts.Artifact{
		FileID:      "file-1",
		Purpose:     "audio",
		ContentType: "audio/mpeg",
		Size:        5,
	}}
	handler := NewHandler(NewService(WithExecutor(executor), WithArtifactStore(store), WithAuditRecorder(noopAuditRecorder())))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		synthesizePath,
		strings.NewReader(`{"text":"private speech","provider":"default"}`),
	)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusBadGateway, "VOICE_PROVIDER_ERROR")
	if strings.Contains(rec.Body.String(), "private speech") {
		t.Fatalf("response leaked synthesis text: %s", rec.Body.String())
	}
}

func TestHandlerSynthesizeValidatesRequestBeforeAdmission(t *testing.T) {
	handler := NewHandler(nil)
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "bad json", body: `{`, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "unknown field", body: `{"text":"hello","provider":"default","secret":"leak"}`, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "unsupported provider", body: `{"text":"hello","provider":"unknown"}`, status: http.StatusBadRequest, code: "UNSUPPORTED_VOICE_PROVIDER"},
		{name: "legacy provider", body: `{"text":"hello","provider":"model"}`, status: http.StatusBadRequest, code: "UNSUPPORTED_VOICE_PROVIDER"},
		{name: "empty text", body: `{"text":" ","provider":"default"}`, status: http.StatusBadRequest, code: "TEXT_REQUIRED"},
		{name: "text too long", body: `{"text":"` + strings.Repeat("界", maxSynthesisTextBytes/3+1) + `","provider":"default"}`, status: http.StatusRequestEntityTooLarge, code: "TEXT_TOO_LONG"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, synthesizePath, strings.NewReader(tt.body))

			handler.ServeHTTP(rec, req)

			assertError(t, rec, tt.status, tt.code)
			if strings.Contains(rec.Body.String(), "leak") {
				t.Fatalf("response leaked request field: %s", rec.Body.String())
			}
		})
	}
}

func TestHandlerSynthesizeRequiresMessageIDForCachedProductionPath(t *testing.T) {
	handler := NewHandler(NewService(
		WithSynthesisExecutorResolver(staticSynthesisResolver{execution: SynthesisExecution{
			Executor: &fakeVoiceExecutor{}, ProviderID: "siliconflow", ModelID: "cosy", VoiceID: "claire",
		}}),
		WithArtifactStore(&fakeArtifactStore{}),
		WithArtifactDeleter(&recordingArtifactDeleter{}),
		WithSynthesisCache(&memorySynthesisCache{}),
		WithAuditRecorder(noopAuditRecorder()),
	))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodPost,
		synthesizePath,
		strings.NewReader(`{"text":"hello","provider":"default"}`),
	))

	assertError(t, recorder, http.StatusNotFound, "VOICE_SOURCE_MESSAGE_NOT_FOUND")
}

func TestHandlerTranscribeFailsClosedAfterAdmission(t *testing.T) {
	handler := NewHandler(nil)
	body, contentType := multipartAudioBody(t, map[string]string{
		"provider": "model",
		"modelId":  "audio-model",
		"language": "auto",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, transcribePath, body)
	req.Header.Set("Content-Type", contentType)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusNotImplemented, "VOICE_JOBS_UNAVAILABLE")
}

func TestHandlerTranscribePassesAudioToOptInExecutor(t *testing.T) {
	executor := &fakeVoiceExecutor{transcribeResponse: TranscribeResponse{Text: "hello"}}
	handler := NewHandler(NewService(WithExecutor(executor), WithAuditRecorder(noopAuditRecorder())))
	body, contentType := multipartAudioBody(t, map[string]string{
		"provider": "model",
		"modelId":  "audio-model",
		"language": "en",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, transcribePath, body)
	req.Header.Set("Content-Type", contentType)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response TranscribeResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Text != "hello" {
		t.Fatalf("text = %q, want hello", response.Text)
	}
	if !executor.transcribeCalled {
		t.Fatal("executor was not called")
	}
	if executor.transcribeRequest.AudioFilename != "audio.webm" ||
		executor.transcribeRequest.AudioContentType != "application/octet-stream" ||
		executor.transcribeRequest.AudioSize <= 0 ||
		executor.transcribeBody != "audio-bytes" {
		t.Fatalf("transcribe request = %#v body=%q", executor.transcribeRequest, executor.transcribeBody)
	}
}

func TestHandlerTranscribeValidatesRequestBeforeAdmission(t *testing.T) {
	handler := NewHandler(nil)

	t.Run("missing audio", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("provider", "default"); err != nil {
			t.Fatalf("write field: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart: %v", err)
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, transcribePath, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		handler.ServeHTTP(rec, req)

		assertError(t, rec, http.StatusBadRequest, "AUDIO_REQUIRED")
	})

	t.Run("unsupported provider", func(t *testing.T) {
		body, contentType := multipartAudioBody(t, map[string]string{"provider": "unknown"})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, transcribePath, body)
		req.Header.Set("Content-Type", contentType)

		handler.ServeHTTP(rec, req)

		assertError(t, rec, http.StatusBadRequest, "UNSUPPORTED_VOICE_PROVIDER")
	})
}

func TestHandlerReturns503WhenAuditUnavailable(t *testing.T) {
	handler := NewHandler(NewService(WithAuditRecorder(jobaudit.RecorderFunc(func(context.Context, jobaudit.Event) error {
		return errors.New("audit sink down")
	}))))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		synthesizePath,
		strings.NewReader(`{"text":"hello","provider":"default"}`),
	)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusServiceUnavailable, "JOB_AUDIT_UNAVAILABLE")
}

func TestHandlerRequiresPost(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, synthesizePath, nil)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}

func multipartAudioBody(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "audio.webm")
	if err != nil {
		t.Fatalf("create audio part: %v", err)
	}
	if _, err := part.Write([]byte("audio-bytes")); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &body, writer.FormDataContentType()
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
