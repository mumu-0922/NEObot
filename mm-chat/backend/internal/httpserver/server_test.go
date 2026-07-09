package httpserver

import (
	"context"
	"encoding/json"
	"errors"
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

func TestMiddlewareRecoversPanicsWithJSON(t *testing.T) {
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}), withRecover, withSecurityHeaders)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)

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
	}), withSessionIdentity(resolver))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer raw-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if resolver.tokenHash != bearerTokenHash("raw-token") {
		t.Fatalf("token hash = %q, want %q", resolver.tokenHash, bearerTokenHash("raw-token"))
	}
	if gotUser.ID != resolver.session.UserID || gotUser.DisplayName != "User Seven" || gotUser.Role != "owner" {
		t.Fatalf("context user = %#v", gotUser)
	}
}

func TestSessionIdentityMiddlewareRejectsInvalidSession(t *testing.T) {
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run for invalid bearer token")
	}), withSessionIdentity(&fakeSessionResolver{err: auth.ErrSessionExpired}))
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

func TestRateLimitMiddlewareExemptsHealthReadyAndVersionRoutes(t *testing.T) {
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
