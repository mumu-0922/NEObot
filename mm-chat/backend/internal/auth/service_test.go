package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceLoginCreatesSessionForBootstrapToken(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	repo := &fakeAuthRepository{}
	service := NewService(
		repo,
		WithBootstrapToken("bootstrap-secret"),
		WithBootstrapUser("77777777-7777-4777-8777-777777777777", "Server Owner"),
		WithSessionTTL(time.Hour),
		WithServiceClock(func() time.Time { return now }),
	)
	service.newID = func() (string, error) { return "88888888-8888-4888-8888-888888888888", nil }
	service.newToken = func() (string, error) { return "raw-session-token", nil }

	result, err := service.Login(context.Background(), LoginInput{Token: " bootstrap-secret ", UserAgent: "agent"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.Token != "raw-session-token" {
		t.Fatalf("Token = %q, want raw-session-token", result.Token)
	}
	if result.User.ID != "77777777-7777-4777-8777-777777777777" || result.User.DisplayName != "Server Owner" {
		t.Fatalf("User = %#v", result.User)
	}
	if !result.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("ExpiresAt = %s, want %s", result.ExpiresAt, now.Add(time.Hour))
	}
	if repo.createInput.TokenHash != HashSessionToken("raw-session-token") {
		t.Fatalf("TokenHash = %q, want hash of raw session token", repo.createInput.TokenHash)
	}
	if repo.createInput.UserAgent != "agent" {
		t.Fatalf("UserAgent = %q, want agent", repo.createInput.UserAgent)
	}
}

func TestServiceLoginRejectsMissingOrInvalidBootstrapToken(t *testing.T) {
	repo := &fakeAuthRepository{}
	service := NewService(repo)
	_, err := service.Login(context.Background(), LoginInput{Token: "anything"})
	if !errors.Is(err, ErrAuthNotConfigured) {
		t.Fatalf("Login() without config error = %v, want ErrAuthNotConfigured", err)
	}

	service = NewService(repo, WithBootstrapToken("bootstrap-secret"))
	_, err = service.Login(context.Background(), LoginInput{Token: "wrong"})
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("Login() invalid error = %v, want ErrInvalidCredential", err)
	}
	if repo.createCalls != 0 {
		t.Fatalf("CreateSession calls = %d, want 0", repo.createCalls)
	}
}

func TestServiceLogoutRevokesSessionAndClearsCache(t *testing.T) {
	repo := &fakeAuthRepository{
		revokeSession: Session{ID: "session-1", UserID: DevelopmentUserID, ExpiresAt: time.Now().Add(time.Hour)},
	}
	cache := newFakeSessionCache()
	service := NewService(repo, WithAuthSessionCache(cache))

	if err := service.Logout(context.Background(), "raw-session-token"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if repo.revokedTokenHash != HashSessionToken("raw-session-token") {
		t.Fatalf("revoked token hash = %q", repo.revokedTokenHash)
	}
	if cache.deleteCalls != 1 || cache.markRevokedCalls != 1 {
		t.Fatalf("cache delete/mark calls = %d/%d, want 1/1", cache.deleteCalls, cache.markRevokedCalls)
	}
}

type fakeAuthRepository struct {
	createInput      CreateSessionInput
	createCalls      int
	createErr        error
	revokedTokenHash string
	revokeSession    Session
	revokeErr        error
}

func (r *fakeAuthRepository) CreateSession(_ context.Context, input CreateSessionInput) (Session, error) {
	r.createCalls++
	r.createInput = input
	if r.createErr != nil {
		return Session{}, r.createErr
	}
	return Session{
		ID:          input.SessionID,
		UserID:      input.UserID,
		DisplayName: input.DisplayName,
		Role:        "user",
		ExpiresAt:   input.ExpiresAt,
	}, nil
}

func (r *fakeAuthRepository) RevokeSessionByTokenHash(_ context.Context, tokenHash string) (Session, error) {
	r.revokedTokenHash = tokenHash
	if r.revokeErr != nil {
		return Session{}, r.revokeErr
	}
	return r.revokeSession, nil
}
