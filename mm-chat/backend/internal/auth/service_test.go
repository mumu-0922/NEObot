package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/sessioncache"
)

const validTestPassword = "not-the-user-password"

func TestServiceLoginCreatesRevisionFencedSession(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeAuthRepository{
		credential: LoginCredential{
			UserID:             "11111111-1111-4111-8111-111111111111",
			Email:              "owner@example.test",
			DisplayName:        "Owner",
			PasswordHash:       defaultDummyPasswordHash,
			CredentialRevision: 7,
		},
	}
	service := NewService(repo, WithServiceClock(func() time.Time { return now }))
	service.newID = func() (string, error) { return "22222222-2222-4222-8222-222222222222", nil }
	service.newToken = func() (string, error) { return testRawToken('a'), nil }

	result, err := service.Login(context.Background(), LoginInput{
		Email:     " Owner@Example.Test ",
		Password:  validTestPassword,
		UserAgent: "agent",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if repo.lookupEmail != "owner@example.test" {
		t.Fatalf("lookup email = %q", repo.lookupEmail)
	}
	if repo.sessionInput.CredentialRevision != 7 ||
		repo.sessionInput.TokenHash != HashSessionToken(testRawToken('a')) ||
		repo.sessionInput.UserID != repo.credential.UserID {
		t.Fatalf("session input = %#v", repo.sessionInput)
	}
	if result.Token != testRawToken('a') || result.User.ID != repo.credential.UserID {
		t.Fatalf("Login() result = %#v", result)
	}
}

func TestServiceLoginCollapsesUnknownAndWrongPassword(t *testing.T) {
	repo := &fakeAuthRepository{lookupErr: ErrInvalidCredential}
	service := NewService(repo)
	_, err := service.Login(context.Background(), LoginInput{
		Email:    "missing@example.test",
		Password: validTestPassword,
	})
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("unknown Login() error = %v", err)
	}

	repo.lookupErr = nil
	repo.credential = LoginCredential{
		UserID:             "11111111-1111-4111-8111-111111111111",
		PasswordHash:       defaultDummyPasswordHash,
		CredentialRevision: 1,
	}
	_, err = service.Login(context.Background(), LoginInput{
		Email:    "owner@example.test",
		Password: "different-password-value",
	})
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("wrong password Login() error = %v", err)
	}
}

func TestServiceRecoveryDeliveryAndCompletion(t *testing.T) {
	repo := &fakeAuthRepository{
		recoveryTarget: RecoveryTarget{
			Email:     "owner@example.test",
			ExpiresAt: time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
		},
		deliverRecovery: true,
		completeRevoked: []RevokedSession{{
			ID:        "22222222-2222-4222-8222-222222222222",
			TokenHash: "session-hash",
		}},
	}
	delivery := &fakeRecoveryDelivery{}
	cache := &fakeAuthCache{}
	service := NewService(repo, WithRecoveryDelivery(delivery), WithAuthSessionCache(cache))
	ids := []string{"33333333-3333-4333-8333-333333333333"}
	service.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	service.newToken = func() (string, error) { return testRawToken('b'), nil }

	if err := service.RequestRecovery(context.Background(), RecoveryRequestInput{
		Email: " Owner@Example.Test ",
	}); err != nil {
		t.Fatalf("RequestRecovery() error = %v", err)
	}
	if repo.recoveryInput.CanonicalEmail != "owner@example.test" ||
		repo.recoveryInput.TokenHash != HashSessionToken(testRawToken('b')) {
		t.Fatalf("recovery input = %#v", repo.recoveryInput)
	}
	if delivery.message.Token != testRawToken('b') || delivery.message.Email != "owner@example.test" {
		t.Fatalf("delivery message = %#v", delivery.message)
	}

	if err := service.CompleteRecovery(context.Background(), RecoveryCompleteInput{
		Token:       testRawToken('c'),
		NewPassword: "replacement-password-value",
	}); err != nil {
		t.Fatalf("CompleteRecovery() error = %v", err)
	}
	if repo.completeInput.TokenHash != HashSessionToken(testRawToken('c')) {
		t.Fatalf("complete input = %#v", repo.completeInput)
	}
	if cache.deletedTokenHash != "session-hash" || cache.revokedSessionID != repo.completeRevoked[0].ID {
		t.Fatalf("cache invalidation = %#v", cache)
	}
}

func TestServiceRevokeAllSessionsUsesContextIdentity(t *testing.T) {
	repo := &fakeAuthRepository{}
	service := NewService(repo)
	ctx := WithUser(context.Background(), User{ID: "11111111-1111-4111-8111-111111111111"})
	if err := service.RevokeAllSessions(ctx); err != nil {
		t.Fatalf("RevokeAllSessions() error = %v", err)
	}
	if repo.revokedUserID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("revoked user = %q", repo.revokedUserID)
	}
}

func TestServiceLogoutRevokesSessionAndClearsCache(t *testing.T) {
	repo := &fakeAuthRepository{}
	cache := &fakeAuthCache{}
	service := NewService(repo, WithAuthSessionCache(cache))
	if err := service.Logout(context.Background(), testRawToken('d')); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	wantHash := HashSessionToken(testRawToken('d'))
	if repo.revokedTokenHash != wantHash || cache.deletedTokenHash != wantHash {
		t.Fatalf("revocation repo/cache = %q/%q", repo.revokedTokenHash, cache.deletedTokenHash)
	}
}

type fakeAuthRepository struct {
	credential       LoginCredential
	lookupEmail      string
	lookupErr        error
	sessionInput     CreateCredentialSessionInput
	recoveryInput    CreateRecoveryTokenInput
	recoveryTarget   RecoveryTarget
	deliverRecovery  bool
	completeInput    CompleteRecoveryRepositoryInput
	completeRevoked  []RevokedSession
	revokedTokenHash string
	revokedUserID    string
}

func (r *fakeAuthRepository) LookupLoginCredential(_ context.Context, email string) (LoginCredential, error) {
	r.lookupEmail = email
	return r.credential, r.lookupErr
}

func (r *fakeAuthRepository) CreateCredentialSession(_ context.Context, input CreateCredentialSessionInput) (Session, error) {
	r.sessionInput = input
	return Session{
		ID:          input.SessionID,
		UserID:      input.UserID,
		DisplayName: r.credential.DisplayName,
		Role:        defaultUserRole,
		ExpiresAt:   input.ExpiresAt,
	}, nil
}

func (r *fakeAuthRepository) AcceptInvite(_ context.Context, input AcceptInviteRepositoryInput) (Session, error) {
	return Session{ID: input.SessionID, UserID: input.UserID, ExpiresAt: input.SessionExpiresAt}, nil
}

func (r *fakeAuthRepository) CreateRecoveryToken(_ context.Context, input CreateRecoveryTokenInput) (RecoveryTarget, bool, error) {
	r.recoveryInput = input
	return r.recoveryTarget, r.deliverRecovery, nil
}

func (r *fakeAuthRepository) CompleteRecovery(_ context.Context, input CompleteRecoveryRepositoryInput) ([]RevokedSession, error) {
	r.completeInput = input
	return r.completeRevoked, nil
}

func (r *fakeAuthRepository) RevokeSessionByTokenHash(_ context.Context, tokenHash string) (Session, error) {
	r.revokedTokenHash = tokenHash
	return Session{ID: "22222222-2222-4222-8222-222222222222"}, nil
}

func (r *fakeAuthRepository) RevokeSessionsByUserID(_ context.Context, userID string) ([]RevokedSession, error) {
	r.revokedUserID = userID
	return nil, nil
}

func (r *fakeAuthRepository) BootstrapIdentity(context.Context, BootstrapIdentityInput) error {
	return nil
}

type fakeRecoveryDelivery struct{ message RecoveryMessage }

func (d *fakeRecoveryDelivery) EnqueueRecovery(message RecoveryMessage) bool {
	d.message = message
	return true
}

type fakeAuthCache struct {
	deletedTokenHash string
	revokedSessionID string
}

func (c *fakeAuthCache) CacheSession(context.Context, string, sessioncache.Snapshot) error {
	return nil
}
func (c *fakeAuthCache) LookupSession(context.Context, string) (sessioncache.Snapshot, bool, error) {
	return sessioncache.Snapshot{}, false, nil
}
func (c *fakeAuthCache) DeleteSession(_ context.Context, tokenHash string) error {
	c.deletedTokenHash = tokenHash
	return nil
}
func (c *fakeAuthCache) MarkSessionRevoked(_ context.Context, sessionID string) error {
	c.revokedSessionID = sessionID
	return nil
}
func (c *fakeAuthCache) IsSessionRevoked(context.Context, string) (bool, error) { return false, nil }
func (c *fakeAuthCache) ClearSessionRevoked(context.Context, string) error      { return nil }

func testRawToken(character byte) string {
	value := make([]byte, 64)
	for i := range value {
		value[i] = character
	}
	return string(value)
}
