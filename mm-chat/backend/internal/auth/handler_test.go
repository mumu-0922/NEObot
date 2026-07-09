package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerLoginMeAndLogout(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeAuthRepository{
		revokeSession: Session{ID: "session-1", UserID: DevelopmentUserID, ExpiresAt: now.Add(time.Hour)},
	}
	service := NewService(
		repo,
		WithBootstrapToken("bootstrap-secret"),
		WithBootstrapUser("77777777-7777-4777-8777-777777777777", "Server Owner"),
		WithSessionTTL(time.Hour),
		WithServiceClock(func() time.Time { return now }),
	)
	service.newID = func() (string, error) { return "88888888-8888-4888-8888-888888888888", nil }
	service.newToken = func() (string, error) { return "raw-session-token", nil }
	handler := NewHandler(service)

	rec := performAuthRequest(handler, http.MethodPost, authLoginPath, `{"token":"bootstrap-secret"}`, "")
	assertAuthStatus(t, rec, http.StatusOK)
	var login LoginResponse
	decodeAuthBody(t, rec, &login)
	if login.Token != "raw-session-token" || login.User.ID != "77777777-7777-4777-8777-777777777777" {
		t.Fatalf("login response = %#v", login)
	}
	if strings.Contains(rec.Body.String(), "bootstrap-secret") {
		t.Fatalf("login response leaked bootstrap token: %s", rec.Body.String())
	}

	rec = performAuthRequest(handler, http.MethodGet, mePath, "", "")
	assertAuthStatus(t, rec, http.StatusOK)
	var me CurrentUserDTO
	decodeAuthBody(t, rec, &me)
	if me.ID != DevelopmentUserID {
		t.Fatalf("me without context = %#v, want development user", me)
	}

	rec = performAuthRequest(handler, http.MethodPost, authLogoutPath, "", "Bearer raw-session-token")
	assertAuthStatus(t, rec, http.StatusNoContent)
	if repo.revokedTokenHash != HashSessionToken("raw-session-token") {
		t.Fatalf("logout revoked hash = %q", repo.revokedTokenHash)
	}
}

func TestHandlerLoginErrors(t *testing.T) {
	handler := NewHandler(NewService(&fakeAuthRepository{}, WithBootstrapToken("bootstrap-secret")))

	rec := performAuthRequest(handler, http.MethodPost, authLoginPath, `{"token":"wrong"}`, "")
	assertAuthStatus(t, rec, http.StatusUnauthorized)
	assertAuthErrorCode(t, rec, "INVALID_CREDENTIALS")

	rec = performAuthRequest(handler, http.MethodPost, authLoginPath, `{"token":"bootstrap-secret","userId":"bad"}`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "INVALID_AUTH_PAYLOAD")
}

func TestHandlerLogoutRequiresBearer(t *testing.T) {
	handler := NewHandler(NewService(&fakeAuthRepository{}))
	rec := performAuthRequest(handler, http.MethodPost, authLogoutPath, "", "")
	assertAuthStatus(t, rec, http.StatusUnauthorized)
	assertAuthErrorCode(t, rec, "UNAUTHENTICATED")
}

func performAuthRequest(handler http.Handler, method string, path string, body string, authorization string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	handler.ServeHTTP(rec, req)
	return rec
}

func assertAuthStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
}

func assertAuthErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body ErrorResponse
	decodeAuthBody(t, rec, &body)
	if body.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body=%s", body.Error.Code, want, rec.Body.String())
	}
}

func decodeAuthBody(t *testing.T, rec *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(destination); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, rec.Body.String())
	}
}
