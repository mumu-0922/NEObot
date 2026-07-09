package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/ratelimit"
)

func TestNewHandlerRoutesHealthReadyAndVersion(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})

	tests := []struct {
		name string
		path string
		want map[string]string
	}{
		{name: "health", path: "/health", want: map[string]string{"status": "healthy"}},
		{name: "ready", path: "/ready", want: map[string]string{"status": "ready"}},
		{name: "version", path: "/v1/version", want: map[string]string{"version": "route-test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}

			var got map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response body: %v", err)
			}
			for key, value := range tt.want {
				if got[key] != value {
					t.Fatalf("body[%q] = %q, want %q; body=%v", key, got[key], value, got)
				}
			}
		})
	}
}

func TestMiddlewareSetsSecurityHeaders(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestMiddlewareSetsAndPropagatesRequestID(t *testing.T) {
	var contextRequestID string
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}), withRequestID)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/request-id", nil)
	req.Header.Set(requestIDHeader, "client-request-1")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get(requestIDHeader); got != "client-request-1" {
		t.Fatalf("%s = %q, want client-request-1", requestIDHeader, got)
	}
	if contextRequestID != "client-request-1" {
		t.Fatalf("context request id = %q, want client-request-1", contextRequestID)
	}
}

func TestMiddlewareGeneratesRequestIDWhenMissingOrInvalid(t *testing.T) {
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Fatal("request id missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}), withRequestID)
	for _, headerValue := range []string{"", "bad request id"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/request-id", nil)
		req.Header.Set(requestIDHeader, headerValue)

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if got := rec.Header().Get(requestIDHeader); got == "" || got == headerValue {
			t.Fatalf("%s = %q, want generated value", requestIDHeader, got)
		}
	}
}

func TestMiddlewareLogsStructuredRequest(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}), withRequestID, withRequestLogging(logger))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/example?secret=hidden", nil)
	req.Header.Set(requestIDHeader, "log-request-1")
	req.RemoteAddr = "127.0.0.1:12345"

	handler.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("decode structured log: %v; log=%s", err, logs.String())
	}
	if entry["msg"] != "http_request" ||
		entry["request_id"] != "log-request-1" ||
		entry["method"] != http.MethodPost ||
		entry["path"] != "/v1/example" {
		t.Fatalf("structured log = %#v", entry)
	}
	if entry["status"] != float64(http.StatusCreated) {
		t.Fatalf("log status = %#v, want %d", entry["status"], http.StatusCreated)
	}
	if strings.Contains(logs.String(), "secret=hidden") {
		t.Fatalf("structured log includes query string: %s", logs.String())
	}
}

func TestRequestLoggingPreservesFlusher(t *testing.T) {
	flushAvailable := false
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		flushAvailable = ok
		if ok {
			flusher.Flush()
		}
		w.WriteHeader(http.StatusNoContent)
	}), withRequestID, withRequestLogging(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)

	handler.ServeHTTP(rec, req)

	if !flushAvailable {
		t.Fatal("logging response writer does not preserve http.Flusher")
	}
}

func TestMiddlewareRecoversPanicsWithJSON(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom-secret")
	}), withRequestID, withRecover(logger), withSecurityHeaders)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(requestIDHeader, "panic-request-1")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "INTERNAL_ERROR")
	}
	if !strings.Contains(logs.String(), "panic-request-1") {
		t.Fatalf("panic log missing request id: %s", logs.String())
	}
	if strings.Contains(logs.String(), "boom-secret") {
		t.Fatalf("panic log leaks panic payload: %s", logs.String())
	}
}

func TestMiddlewareChainLogsPanicAndRequestWithRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("chain-secret")
	}), withRequestID, withRequestLogging(logger), withRecover(logger), withSecurityHeaders)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic-chain?token=hidden", nil)
	req.Header.Set(requestIDHeader, "chain-request-1")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if got := rec.Header().Get(requestIDHeader); got != "chain-request-1" {
		t.Fatalf("%s = %q, want chain-request-1", requestIDHeader, got)
	}

	entries := decodeJSONLogLines(t, logs.Bytes())
	if len(entries) != 2 {
		t.Fatalf("log entries = %#v, want panic and request entries; raw=%s", entries, logs.String())
	}
	if entries[0]["msg"] != "http_panic" || entries[0]["request_id"] != "chain-request-1" {
		t.Fatalf("panic log = %#v", entries[0])
	}
	if entries[1]["msg"] != "http_request" ||
		entries[1]["request_id"] != "chain-request-1" ||
		entries[1]["status"] != float64(http.StatusInternalServerError) ||
		entries[1]["path"] != "/panic-chain" {
		t.Fatalf("request log = %#v", entries[1])
	}
	if strings.Contains(logs.String(), "chain-secret") || strings.Contains(logs.String(), "token=hidden") {
		t.Fatalf("chain logs leak secret payload or query: %s", logs.String())
	}
}

func TestSessionIdentityMiddlewareSetsRequestUser(t *testing.T) {
	resolver := &fakeSessionResolver{
		session: auth.Session{
			ID:          "session-1",
			UserID:      "77777777-7777-4777-8777-777777777777",
			DisplayName: "User Seven",
			Role:        "owner",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}
	var gotUser auth.User
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = auth.UserOrDevelopment(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}), withSessionIdentity(resolver, false))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer raw-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if resolver.tokenHash != auth.HashSessionToken("raw-token") {
		t.Fatalf("token hash = %q, want %q", resolver.tokenHash, auth.HashSessionToken("raw-token"))
	}
	if gotUser.ID != resolver.session.UserID || gotUser.DisplayName != "User Seven" || gotUser.Role != "owner" {
		t.Fatalf("context user = %#v", gotUser)
	}
}

func TestSessionIdentityMiddlewareRejectsInvalidSession(t *testing.T) {
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run for invalid bearer token")
	}), withSessionIdentity(&fakeSessionResolver{err: auth.ErrSessionExpired}, false))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer expired-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("error code = %q, want UNAUTHENTICATED", body.Error.Code)
	}
}

func TestSessionIdentityMiddlewareKeepsDevelopmentFallbackWhenMissingBearer(t *testing.T) {
	resolver := &fakeSessionResolver{err: auth.ErrSessionExpired}
	nextCalled := false
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		user := auth.UserOrDevelopment(r.Context())
		if user.ID != auth.DevelopmentUserID {
			t.Fatalf("user = %#v, want development fallback", user)
		}
		w.WriteHeader(http.StatusNoContent)
	}), withSessionIdentity(resolver, false))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if !nextCalled {
		t.Fatal("next handler was not called")
	}
	if resolver.tokenHash != "" {
		t.Fatalf("resolver tokenHash = %q, want blank when bearer is missing", resolver.tokenHash)
	}
}

func TestSessionIdentityMiddlewareSkipsLoginRoute(t *testing.T) {
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "route-test"},
		WithSessionResolver(&fakeSessionResolver{err: auth.ErrSessionExpired}),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"token":"x"}`))
	req.Header.Set("Authorization", "Bearer expired-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("error code = %q, want DATABASE_REQUIRED", body.Error.Code)
	}
}

func TestAuthRequiredModeRejectsMissingCredentialsAndKeepsPublicRoutes(t *testing.T) {
	handler := NewHandler(config.Config{
		Addr:    ":0",
		Version: "route-test",
		Auth:    config.AuthConfig{Mode: config.AuthModeRequired},
	})

	publicRoutes := []struct {
		method string
		path   string
		want   int
	}{
		{method: http.MethodGet, path: "/health", want: http.StatusOK},
		{method: http.MethodGet, path: "/ready", want: http.StatusOK},
		{method: http.MethodGet, path: "/metrics", want: http.StatusOK},
		{method: http.MethodGet, path: "/v1/version", want: http.StatusOK},
		{method: http.MethodPost, path: "/v1/auth/login", want: http.StatusServiceUnavailable},
	}
	for _, route := range publicRoutes {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"token":"x"}`))

		handler.ServeHTTP(rec, req)

		if rec.Code != route.want {
			t.Fatalf("%s %s status = %d, want %d; body=%s", route.method, route.path, rec.Code, route.want, rec.Body.String())
		}
	}

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/me"},
		{method: http.MethodGet, path: "/v1/chat/conversations"},
		{method: http.MethodGet, path: "/v1/files/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodGet, path: "/v1/import/browser/33333333-3333-4333-8333-333333333333"},
	}
	for _, route := range protectedRoutes {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(route.method, route.path, nil)

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401; body=%s", route.method, route.path, rec.Code, rec.Body.String())
		}
		var body ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s %s response: %v", route.method, route.path, err)
		}
		if body.Error.Code != "UNAUTHENTICATED" {
			t.Fatalf("%s %s error code = %q, want UNAUTHENTICATED", route.method, route.path, body.Error.Code)
		}
	}
}

func TestAuthRequiredModeFailsClosedWhenResolverIsMissing(t *testing.T) {
	handler := NewHandler(config.Config{
		Addr:    ":0",
		Version: "route-test",
		Auth:    config.AuthConfig{Mode: config.AuthModeRequired},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer raw-session-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("error code = %q, want DATABASE_REQUIRED", body.Error.Code)
	}
}

func TestNewHandlerRejectsNonGETWithJSONError(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/health", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "METHOD_NOT_ALLOWED")
	}
}

func TestNewHandlerReturnsJSONNotFound(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "NOT_FOUND")
	}
}

func TestNewHandlerRegistersChatRoutesWithDatabaseRequired(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/conversations", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/runs/33333333-3333-4333-8333-333333333333/cancel",
		nil,
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("cancel status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode cancel error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("cancel error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/files/33333333-3333-4333-8333-333333333333",
		nil,
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode file error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("file error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/import/browser/33333333-3333-4333-8333-333333333333",
		nil,
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("import status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode import error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("import error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"token":"x"}`))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("auth login status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode auth login error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("auth login error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/me", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestRateLimitMiddlewareLimitsNonExemptRoutes(t *testing.T) {
	store := newFakeRateLimitStore()
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Redis: config.RedisConfig{
				RateLimitEnabled:  true,
				RateLimitRequests: 2,
				RateLimitWindow:   time.Minute,
			},
		},
		WithRateLimitStore(store),
	)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		req.RemoteAddr = "203.0.113.10:4444"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("request %d status = %d, want 404; body=%s", i+1, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-RateLimit-Limit"); got != "2" {
			t.Fatalf("request %d X-RateLimit-Limit = %q, want 2", i+1, got)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "203.0.113.10:4444"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header is blank")
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Fatalf("X-RateLimit-Limit = %q, want 2", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Fatalf("X-RateLimit-Remaining = %q, want 0", got)
	}
	if got := rec.Header().Get("X-RateLimit-Reset"); got == "" {
		t.Fatal("X-RateLimit-Reset header is blank")
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode rate limit response: %v", err)
	}
	if body.Error.Code != "RATE_LIMITED" {
		t.Fatalf("error code = %q, want RATE_LIMITED", body.Error.Code)
	}
}

func TestRateLimitMiddlewareExemptsHealthReadyMetricsAndVersionRoutes(t *testing.T) {
	store := newFakeRateLimitStore()
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Redis: config.RedisConfig{
				RateLimitEnabled:  true,
				RateLimitRequests: 1,
				RateLimitWindow:   time.Minute,
			},
		},
		WithRateLimitStore(store),
	)

	tests := []struct {
		path string
		code int
	}{
		{path: "/health", code: http.StatusOK},
		{path: "/ready", code: http.StatusOK},
		{path: "/metrics", code: http.StatusOK},
		{path: "/v1/version", code: http.StatusOK},
	}

	for _, tt := range tests {
		for i := 0; i < 3; i++ {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.RemoteAddr = "203.0.113.10:4444"
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.code {
				t.Fatalf("%s request %d status = %d, want %d", tt.path, i+1, rec.Code, tt.code)
			}
		}
	}
	if store.calls != 0 {
		t.Fatalf("rate limit store calls = %d, want 0 for exempt routes", store.calls)
	}
}

func TestRateLimitMiddlewareFailsOpenOnStoreError(t *testing.T) {
	store := newFakeRateLimitStore()
	store.err = errors.New("redis down")
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Redis: config.RedisConfig{
				RateLimitEnabled:  true,
				RateLimitRequests: 1,
				RateLimitWindow:   time.Minute,
			},
		},
		WithRateLimitStore(store),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "203.0.113.10:4444"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 fail-open; body=%s", rec.Code, rec.Body.String())
	}
}

type fakeRateLimitStore struct {
	calls  int
	counts map[string]int
	err    error
}

func newFakeRateLimitStore() *fakeRateLimitStore {
	return &fakeRateLimitStore{counts: map[string]int{}}
}

func decodeJSONLogLines(t *testing.T, payload []byte) []map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(payload), []byte("\n"))
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func (s *fakeRateLimitStore) Allow(
	_ context.Context,
	key string,
	limit int,
	window time.Duration,
	now time.Time,
) (ratelimit.Result, error) {
	s.calls++
	if s.err != nil {
		return ratelimit.Result{}, s.err
	}
	s.counts[key]++
	remaining := limit - s.counts[key]
	if remaining < 0 {
		remaining = 0
	}
	return ratelimit.Result{
		Allowed:    s.counts[key] <= limit,
		Limit:      limit,
		Remaining:  remaining,
		RetryAfter: window,
		ResetAt:    now.Add(window),
	}, nil
}

type fakeSessionResolver struct {
	session   auth.Session
	err       error
	tokenHash string
}

func (r *fakeSessionResolver) ResolveByTokenHash(_ context.Context, tokenHash string) (auth.Session, error) {
	r.tokenHash = tokenHash
	if r.err != nil {
		return auth.Session{}, r.err
	}
	return r.session, nil
}
