package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerIdentityLifecycleRoutes(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeAuthRepository{
		credential: LoginCredential{
			UserID:             "77777777-7777-4777-8777-777777777777",
			DisplayName:        "Server Owner",
			PasswordHash:       defaultDummyPasswordHash,
			CredentialRevision: 1,
		},
	}
	service := NewService(repo, WithSessionTTL(time.Hour), WithServiceClock(func() time.Time { return now }))
	ids := []string{
		"88888888-8888-4888-8888-888888888888",
		"99999999-9999-4999-8999-999999999999",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
	}
	service.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	tokens := []string{testRawToken('a'), testRawToken('b'), testRawToken('c')}
	service.newToken = func() (string, error) {
		token := tokens[0]
		tokens = tokens[1:]
		return token, nil
	}
	handler := NewHandler(service)

	rec := performAuthRequest(handler, http.MethodPost, authLoginPath,
		`{"email":"Owner@Example.Test","password":"not-the-user-password"}`, "")
	assertAuthStatus(t, rec, http.StatusOK)
	var login LoginResponse
	decodeAuthBody(t, rec, &login)
	if login.Token != testRawToken('a') || login.User.ID != repo.credential.UserID {
		t.Fatalf("login response = %#v", login)
	}
	if strings.Contains(rec.Body.String(), "not-the-user-password") ||
		strings.Contains(rec.Body.String(), "Owner@Example.Test") {
		t.Fatalf("login response leaked credential input: %s", rec.Body.String())
	}

	rec = performAuthRequest(handler, http.MethodPost, authInviteAcceptPath,
		`{"token":"`+testRawToken('d')+`","password":"invite-password-value"}`, "")
	assertAuthStatus(t, rec, http.StatusOK)

	repo.deliverRecovery = false
	rec = performAuthRequest(handler, http.MethodPost, authRecoveryRequestPath,
		`{"email":"owner@example.test"}`, "")
	assertAuthStatus(t, rec, http.StatusAccepted)
	if rec.Body.String() != "{\"status\":\"accepted\"}\n" {
		t.Fatalf("recovery response = %q", rec.Body.String())
	}

	rec = performAuthRequest(handler, http.MethodPost, authRecoveryCompletePath,
		`{"token":"`+testRawToken('e')+`","newPassword":"replacement-password-value"}`, "")
	assertAuthStatus(t, rec, http.StatusNoContent)

	rec = performAuthRequest(handler, http.MethodGet, mePath, "", "")
	assertAuthStatus(t, rec, http.StatusOK)
	var me CurrentUserDTO
	decodeAuthBody(t, rec, &me)
	if me.ID != DevelopmentUserID {
		t.Fatalf("me without context = %#v, want development user", me)
	}

	rec = performAuthRequest(handler, http.MethodPost, authLogoutPath, "", "Bearer "+testRawToken('f'))
	assertAuthStatus(t, rec, http.StatusNoContent)
}

func TestHandlerRejectsLegacyAndCallerIdentityPayloads(t *testing.T) {
	handler := NewHandler(NewService(&fakeAuthRepository{}))

	rec := performAuthRequest(handler, http.MethodPost, authLoginPath, `{"token":"legacy"}`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "INVALID_AUTH_PAYLOAD")

	rec = performAuthRequest(handler, http.MethodPost, authLoginPath,
		`{"email":"owner@example.test","password":"not-the-user-password","userId":"bad"}`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "FORBIDDEN_IDENTITY_FIELD")

	rec = performAuthRequest(handler, http.MethodPost, authInviteAcceptPath,
		`{"token":"`+testRawToken('d')+`","password":"invite-password-value","teamRole":"admin"}`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "FORBIDDEN_IDENTITY_FIELD")

	rec = performAuthRequest(handler, http.MethodPost, authInviteAcceptPath,
		`{"token":"`+testRawToken('d')+`","password":"invite-password-value"} trailing`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "INVALID_AUTH_PAYLOAD")

	rec = performAuthRequest(handler, http.MethodPost,
		authInviteAcceptPath+"?token="+testRawToken('d'),
		`{"token":"`+testRawToken('d')+`","password":"invite-password-value"}`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "INVALID_AUTH_PAYLOAD")

	rec = performAuthRequest(handler, http.MethodPost,
		authLoginPath+"?allowedCollectionIds=bad",
		`{"email":"owner@example.test","password":"not-the-user-password"}`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "FORBIDDEN_IDENTITY_FIELD")

	rec = performAuthRequest(handler, http.MethodPost,
		authLoginPath+"?ignored=one;ignored=two",
		`{"email":"owner@example.test","password":"not-the-user-password"}`, "")
	assertAuthStatus(t, rec, http.StatusBadRequest)
	assertAuthErrorCode(t, rec, "INVALID_AUTH_PAYLOAD")
}

func TestHandlerBoundsAuthBodyAndRateLimits(t *testing.T) {
	handler := NewHandler(NewService(&fakeAuthRepository{lookupErr: ErrInvalidCredential}))
	oversized := `{"email":"` + strings.Repeat("a", maxAuthRequestBytes) + `"}`
	rec := performAuthRequest(handler, http.MethodPost, authLoginPath, oversized, "")
	assertAuthStatus(t, rec, http.StatusRequestEntityTooLarge)
	assertAuthErrorCode(t, rec, "PAYLOAD_TOO_LARGE")

	for i := 0; i < loginRateLimitPolicy.subjectLimit; i++ {
		email := "missing@example.test"
		if i%2 == 1 {
			email = " MISSING@Example.Test "
		}
		rec = performAuthRequest(handler, http.MethodPost, authLoginPath,
			`{"email":"`+email+`","password":"not-the-user-password"}`, "")
		assertAuthStatus(t, rec, http.StatusUnauthorized)
	}
	rec = performAuthRequest(handler, http.MethodPost, authLoginPath,
		`{"email":"missing@example.test","password":"not-the-user-password"}`, "")
	assertAuthStatus(t, rec, http.StatusTooManyRequests)
	assertAuthErrorCode(t, rec, "RATE_LIMITED")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("rate limit response is missing Retry-After")
	}
}

func TestHandlerLogoutAndRevokeAllRequireIdentity(t *testing.T) {
	handler := NewHandler(NewService(&fakeAuthRepository{}))
	rec := performAuthRequest(handler, http.MethodPost, authLogoutPath, "", "")
	assertAuthStatus(t, rec, http.StatusUnauthorized)
	assertAuthErrorCode(t, rec, "UNAUTHENTICATED")

	rec = performAuthRequest(handler, http.MethodDelete, meSessionsPath, "", "")
	assertAuthStatus(t, rec, http.StatusUnauthorized)
	assertAuthErrorCode(t, rec, "UNAUTHENTICATED")
}

func TestAuthClientAddressTrustsForwardedHeaderOnlyFromLoopback(t *testing.T) {
	loopback := httptest.NewRequest(http.MethodPost, authLoginPath, nil)
	loopback.RemoteAddr = "127.0.0.1:1234"
	loopback.Header.Set("X-Forwarded-For", "198.51.100.10, invalid, 203.0.113.44")
	if got := authClientAddress(loopback); got != "203.0.113.44" {
		t.Fatalf("loopback forwarded client = %q", got)
	}

	external := httptest.NewRequest(http.MethodPost, authLoginPath, nil)
	external.RemoteAddr = "192.0.2.20:1234"
	external.Header.Set("X-Forwarded-For", "198.51.100.99")
	if got := authClientAddress(external); got != "192.0.2.20" {
		t.Fatalf("external spoofed forwarded client = %q", got)
	}
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
