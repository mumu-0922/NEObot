package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestMetricsEndpointExposesHTTPDependencyAndDBStats(t *testing.T) {
	metrics := NewMetrics("metrics-test", "minio")
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "metrics-test",
			Storage: config.StorageConfig{Backend: "minio"},
		},
		WithMetrics(metrics),
		WithReadyCheck("storage", readyCheckerFunc(func(context.Context) error { return nil })),
		WithDatabaseStatsProvider(fakeDBStatsProvider{stats: sql.DBStats{
			MaxOpenConnections: 10,
			OpenConnections:    3,
			InUse:              2,
			Idle:               1,
			WaitCount:          4,
		}}),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/files/33333333-3333-4333-8333-333333333333/content", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	body := rec.Body.String()
	assertContains(t, body, `mm_chat_build_info{version="metrics-test",storage_backend="minio"} 1`)
	assertContains(t, body, `mm_chat_dependency_ready{dependency="storage"} 1`)
	assertContains(t, body, `mm_chat_postgres_max_open_connections 10`)
	assertContains(t, body, `mm_chat_postgres_open_connections 3`)
	assertContains(t, body, `mm_chat_http_requests_total{method="GET",path="/v1/files/{id}/content",status="503"} 1`)
	if strings.Contains(body, "33333333-3333-4333-8333-333333333333") {
		t.Fatalf("metrics body leaks raw UUID path label: %s", body)
	}
}

func TestMetricsEndpointReportsFailedDependencyGauge(t *testing.T) {
	metrics := NewMetrics("metrics-test", "local")
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "metrics-test"},
		WithMetrics(metrics),
		WithReadyCheck("redis", readyCheckerFunc(func(context.Context) error { return errors.New("redis down") })),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	assertContains(t, rec.Body.String(), `mm_chat_dependency_ready{dependency="redis"} 0`)
}

func TestMetricsEndpointIsPublicInRequiredAuthMode(t *testing.T) {
	handler := NewHandler(config.Config{
		Addr:    ":0",
		Version: "metrics-test",
		Auth:    config.AuthConfig{Mode: config.AuthModeRequired},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want public 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetricsEndpointRejectsNonGETWithJSONError(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "metrics-test"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("metrics status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestRequestMetricsPreservesFlusher(t *testing.T) {
	metrics := NewMetrics("metrics-test", "local")
	flushAvailable := false
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		flushAvailable = ok
		if ok {
			flusher.Flush()
		}
		w.WriteHeader(http.StatusNoContent)
	}), withRequestMetrics(metrics))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)

	handler.ServeHTTP(rec, req)

	if !flushAvailable {
		t.Fatal("metrics response writer does not preserve http.Flusher")
	}
}

func TestRequestMetricsRecordsRecoveredPanicStatus(t *testing.T) {
	metrics := NewMetrics("metrics-test", "local")
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("metrics-panic")
	}), withRequestMetrics(metrics), withRecover(nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	assertContains(
		t,
		metrics.Render(context.Background()),
		`mm_chat_http_requests_total{method="GET",path="/__unknown__",status="500"} 1`,
	)
}

func TestNormalizeMetricPathBoundsKnownDynamicRoutes(t *testing.T) {
	tests := map[string]string{
		"/v1/auth/invites/accept":                                   "/v1/auth/invites/accept",
		"/v1/auth/recovery/request":                                 "/v1/auth/recovery/request",
		"/v1/auth/recovery/complete":                                "/v1/auth/recovery/complete",
		"/v1/me/sessions":                                           "/v1/me/sessions",
		"/v1/teams":                                                 "/v1/teams",
		"/v1/teams/11111111-1111-4111-8111-111111111111":            "/v1/teams/{teamId}",
		"/v1/teams/11111111-1111-4111-8111-111111111111/members":    "/v1/teams/{teamId}/members",
		"/v1/teams/11111111-1111-4111-8111-111111111111/membership": "/v1/teams/{teamId}/membership",
		"/v1/teams/11111111-1111-4111-8111-111111111111/members/22222222-2222-4222-8222-222222222222": "/v1/teams/{teamId}/members/{userId}",
		"/v1/teams/11111111-1111-4111-8111-111111111111/invites":                                      "/v1/teams/{teamId}/invites",
		"/v1/teams/11111111-1111-4111-8111-111111111111/invites/33333333-3333-4333-8333-333333333333": "/v1/teams/{teamId}/invites/{inviteId}",
		"/v1/knowledge/collections":                                      "/v1/knowledge/collections",
		"/v1/knowledge/collections/11111111-1111-4111-8111-111111111111": "/v1/knowledge/collections/{collectionId}",
		"/v1/chat/conversations/anything/messages":                       "/v1/chat/conversations/{id}/messages",
		"/v1/chat/conversations/anything/stream":                         "/v1/chat/conversations/{id}/stream",
		"/v1/chat/runs/non-uuid-run-id/cancel":                           "/v1/chat/runs/{id}/cancel",
		"/v1/files/33333333-3333-4333-8333-333333333333":                 "/v1/files/{id}",
		"/v1/import/browser/preview":                                     "/v1/import/browser/preview",
		"/v1/import/browser/import-batch-id":                             "/v1/import/browser/{id}",
		"/unknown/33333333-3333-4333-8333-333333333333/detail":           unknownMetricPath,
		"/missing/sk_live_secret_token":                                  unknownMetricPath,
		"//missing/sk_live_secret_token":                                 unknownMetricPath,
		"/%2Fmissing/sk_live_secret_token":                               unknownMetricPath,
	}

	for input, want := range tests {
		if got := normalizeMetricPath(input); got != want {
			t.Fatalf("normalizeMetricPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMetricsEndpointBoundsTeamDynamicLabels(t *testing.T) {
	metrics := NewMetrics("metrics-test", "local")
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "metrics-test"},
		WithMetrics(metrics),
	)
	teamID := "11111111-1111-4111-8111-111111111111"
	inviteID := "33333333-3333-4333-8333-333333333333"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodDelete,
		"/v1/teams/"+teamID+"/invites/"+inviteID,
		nil,
	)
	handler.ServeHTTP(rec, req)

	body := metrics.Render(context.Background())
	assertContains(
		t,
		body,
		`mm_chat_http_requests_total{method="DELETE",path="/v1/teams/{teamId}/invites/{inviteId}",status="401"} 1`,
	)
	if strings.Contains(body, teamID) || strings.Contains(body, inviteID) {
		t.Fatalf("metrics body leaks raw Team UUID: %s", body)
	}
}

func TestMetricsEndpointBoundsUnknownPathAndMethodLabels(t *testing.T) {
	metrics := NewMetrics("metrics-test", "local")
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "metrics-test"},
		WithMetrics(metrics),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("BREW", "/missing/sk_live_secret_token?api_key=hidden", nil)
	handler.ServeHTTP(rec, req)

	body := metrics.Render(context.Background())
	assertContains(t, body, `mm_chat_http_requests_total{method="OTHER",path="/__unknown__",status="404"} 1`)
	if strings.Contains(body, "sk_live_secret_token") || strings.Contains(body, "api_key=hidden") {
		t.Fatalf("metrics body leaks unknown path or query secret: %s", body)
	}
}

func TestMetricsEndpointBoundsEscapedSlashUnknownPathLabels(t *testing.T) {
	metrics := NewMetrics("metrics-test", "local")
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "metrics-test"},
		WithMetrics(metrics),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/%2Fmissing/sk_live_secret_token?api_key=hidden", nil)
	handler.ServeHTTP(rec, req)

	body := metrics.Render(context.Background())
	assertContains(t, body, `path="/__unknown__"`)
	if strings.Contains(body, "sk_live_secret_token") || strings.Contains(body, "%2Fmissing") {
		t.Fatalf("metrics body leaks escaped-slash unknown path: %s", body)
	}
}

type readyCheckerFunc func(context.Context) error

func (f readyCheckerFunc) CheckReady(ctx context.Context) error {
	return f(ctx)
}

type fakeDBStatsProvider struct {
	stats sql.DBStats
}

func (p fakeDBStatsProvider) Stats() sql.DBStats {
	return p.stats
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("body missing %q:\n%s", needle, haystack)
	}
}
