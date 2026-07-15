package voicejobs

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		{name: "empty text", body: `{"text":" ","provider":"default"}`, status: http.StatusBadRequest, code: "TEXT_REQUIRED"},
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
