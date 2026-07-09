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
	assertReadyBody(t, rec, "ready", map[string]string{"database": "ready"}, "")
}

func TestReadyReturnsUnavailableWhenDatabaseFails(t *testing.T) {
	h := New("test-version", fakeReadyChecker{err: errors.New("ping failed")})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertJSONContentType(t, rec)
	assertReadyBody(t, rec, "not_ready", map[string]string{"database": "not_ready"}, "DEPENDENCY_NOT_READY")
}

func TestReadyReportsMultipleNamedChecksWithoutLeakingErrors(t *testing.T) {
	h := NewWithChecks(
		"test-version",
		Check{Name: "database", Checker: fakeReadyChecker{}},
		Check{Name: "redis", Checker: fakeReadyChecker{err: errors.New("redis password leaked if exposed")}},
		Check{Name: "object storage", Checker: fakeReadyChecker{}},
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(rec, req)

	assertStatus(t, rec, http.StatusServiceUnavailable)
	assertJSONContentType(t, rec)
	assertReadyBody(t, rec, "not_ready", map[string]string{
		"database":       "ready",
		"redis":          "not_ready",
		"object-storage": "ready",
	}, "DEPENDENCY_NOT_READY")
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("ready response leaks dependency error: %s", rec.Body.String())
	}
}

func TestReadyNormalizesDuplicateCheckNames(t *testing.T) {
	h := NewWithChecks(
		"test-version",
		Check{Name: "Storage", Checker: fakeReadyChecker{}},
		Check{Name: "storage", Checker: fakeReadyChecker{}},
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	h.Ready(rec, req)

	assertStatus(t, rec, http.StatusOK)
	assertReadyBody(t, rec, "ready", map[string]string{
		"storage":   "ready",
		"storage-2": "ready",
	}, "")
}

func assertReadyBody(t *testing.T, rec *httptest.ResponseRecorder, status string, checks map[string]string, errorCode string) {
	t.Helper()

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if body.Status != status {
		t.Fatalf("status body = %q, want %q; body=%#v", body.Status, status, body)
	}
	for name, want := range checks {
		got, ok := body.Checks[name]
		if !ok {
			t.Fatalf("checks[%q] missing; checks=%#v", name, body.Checks)
		}
		if got.Status != want {
			t.Fatalf("checks[%q].status = %q, want %q", name, got.Status, want)
		}
	}
	if len(body.Checks) != len(checks) {
		t.Fatalf("checks = %#v, want keys %#v", body.Checks, checks)
	}
	if errorCode == "" {
		if body.Error != nil {
			t.Fatalf("error = %#v, want nil", body.Error)
		}
		return
	}
	if body.Error == nil {
		t.Fatalf("error = nil, want code %s", errorCode)
	}
	if body.Error.Code != errorCode {
		t.Fatalf("error code = %q, want %q", body.Error.Code, errorCode)
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
