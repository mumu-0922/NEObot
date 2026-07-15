package jobcontrol

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerCancelFailsClosedAfterAdmission(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/cancel", nil)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusNotImplemented, "JOB_CANCELLATION_UNAVAILABLE")
}

func TestHandlerCancelValidatesRoute(t *testing.T) {
	handler := NewHandler(nil)
	for _, path := range []string{
		"/v1/jobs//cancel",
		"/v1/jobs/job/with/slash/cancel",
		"/v1/jobs/job-1/status",
		"/v1/jobs/" + strings.Repeat("x", maxJobIDLength+1) + "/cancel",
		"/v1/jobs/job$secret/cancel",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, nil)

			handler.ServeHTTP(rec, req)

			assertError(t, rec, http.StatusNotFound, "NOT_FOUND")
		})
	}
}

func TestHandlerRequiresPost(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/job-1/cancel", nil)

	handler.ServeHTTP(rec, req)

	assertError(t, rec, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}

func TestParseCancelPath(t *testing.T) {
	jobID, ok := parseCancelPath("/v1/jobs/job_1.cancel-2/cancel")
	if !ok || jobID != "job_1.cancel-2" {
		t.Fatalf("parseCancelPath() = %q, %v", jobID, ok)
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
