package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeReadyChecker struct {
	err error
}

func (f fakeReadyChecker) CheckReady(context.Context) error {
	return f.err
}

func TestHealthReturnsHealthyJSON(t *testing.T) {
	h := New("test-version")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	h.Health(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)
	assertJSONBody(t, rec, map[string]string{"status": "healthy"})
}

func TestReadyReturnsReadyJSONWhenDatabaseDisabled(t *testing.T) {
	h := New("test-version")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)
	assertJSONBody(t, rec, map[string]string{"status": "ready"})
}

func TestReadyReturnsReadyJSONWhenDatabaseReady(t *testing.T) {
	h := New("test-version", fakeReadyChecker{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)
	assertJSONBody(t, rec, map[string]string{"status": "ready"})
}

func TestReadyReturnsUnavailableWhenDatabaseFails(t *testing.T) {
	h := New("test-version", fakeReadyChecker{err: errors.New("ping failed")})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertJSONContentType(t, rec)

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "DATABASE_NOT_READY" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "DATABASE_NOT_READY")
	}
}

func TestVersionReturnsConfiguredVersionJSON(t *testing.T) {
	h := New("test-version")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)

	h.Version(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertJSONContentType(t, rec)
	assertJSONBody(t, rec, map[string]string{"version": "test-version"})
}

func TestHandlerRejectsNonGETWithJSONError(t *testing.T) {
	h := New("test-version")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/health", nil)

	h.Health(rec, req)

	assertStatus(t, rec, http.StatusMethodNotAllowed)
	assertJSONContentType(t, rec)
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", rec.Header().Get("Allow"), http.MethodGet)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "METHOD_NOT_ALLOWED")
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
}

func assertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
}

func assertJSONBody(t *testing.T, rec *httptest.ResponseRecorder, want map[string]string) {
	t.Helper()

	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	for key, value := range want {
		if got[key] != value {
			t.Fatalf("body[%q] = %q, want %q; body=%v", key, got[key], value, got)
		}
	}
}
