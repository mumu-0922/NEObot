package codejobs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerExecuteFailsClosedAfterAdmission(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		executionsPath,
		strings.NewReader(`{"modelRef":{"providerId":"gemini","modelId":"gemini-code"},"language":"python","code":"print('hi')"}`),
	)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusNotImplemented, "CODE_EXECUTION_UNAVAILABLE")
}

func TestHandlerExecuteDefaultsLanguageAndStillFailsClosed(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		executionsPath,
		strings.NewReader(`{"modelRef":{"providerId":"gemini","modelId":"gemini-code"},"code":"print('hi')"}`),
	)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusNotImplemented, "CODE_EXECUTION_UNAVAILABLE")
}

func TestHandlerExecuteValidatesRequestBeforeAdmission(t *testing.T) {
	handler := NewHandler(nil)
	largeCode := strings.Repeat("x", maxCodeCharacters+1)
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "bad json", body: `{`, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "unknown provider object", body: `{"provider":{"apiKey":"sk-secret"},"modelRef":{"providerId":"gemini","modelId":"gemini-code"},"code":"print('hi')"}`, status: http.StatusBadRequest, code: "INVALID_REQUEST"},
		{name: "missing model", body: `{"modelRef":{"providerId":"gemini"},"code":"print('hi')"}`, status: http.StatusBadRequest, code: "MODEL_REF_REQUIRED"},
		{name: "empty code", body: `{"modelRef":{"providerId":"gemini","modelId":"gemini-code"},"code":" "}`, status: http.StatusBadRequest, code: "CODE_REQUIRED"},
		{name: "large code", body: `{"modelRef":{"providerId":"gemini","modelId":"gemini-code"},"code":"` + largeCode + `"}`, status: http.StatusBadRequest, code: "CODE_TOO_LARGE"},
		{name: "unsupported language", body: `{"modelRef":{"providerId":"gemini","modelId":"gemini-code"},"language":"javascript","code":"console.log(1)"}`, status: http.StatusBadRequest, code: "UNSUPPORTED_CODE_LANGUAGE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, executionsPath, strings.NewReader(tt.body))

			handler.ServeHTTP(rec, req)

			assertError(t, rec, tt.status, tt.code)
			if strings.Contains(rec.Body.String(), "sk-secret") {
				t.Fatalf("response leaked request secret: %s", rec.Body.String())
			}
		})
	}
}

func TestHandlerRequiresPost(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, executionsPath, nil)

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
