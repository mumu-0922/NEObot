package auth

import (
	"context"
	"errors"
	"strings"
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

func TestServiceAcceptInviteCreatesNewIdentityFromSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeAuthRepository{
		lookupErr: ErrInvalidCredential,
		inviteSnapshot: InviteAcceptanceSnapshot{
			TeamID: "33333333-3333-4333-8333-333333333333",
			Email:  "new-member@example.test",
		},
	}
	service := NewService(repo, WithServiceClock(func() time.Time { return now }))
	ids := []string{
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
	}
	service.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	service.newToken = func() (string, error) { return testRawToken('e'), nil }

	result, err := service.AcceptInvite(context.Background(), AcceptInviteInput{
		Token:     testRawToken('f'),
		Password:  "new-invite-password",
		UserAgent: "invite-agent",
	})
	if err != nil {
		t.Fatalf("AcceptInvite() error = %v", err)
	}
	if repo.inviteLookupTokenHash != HashSessionToken(testRawToken('f')) {
		t.Fatalf("invite lookup token hash = %q", repo.inviteLookupTokenHash)
	}
	if repo.lookupEmail != repo.inviteSnapshot.Email {
		t.Fatalf("lookup email = %q", repo.lookupEmail)
	}
	if repo.acceptInviteInput.UserID != "44444444-4444-4444-8444-444444444444" ||
		repo.acceptInviteInput.CredentialRevision != 0 ||
		repo.acceptInviteInput.InviteTeamID != repo.inviteSnapshot.TeamID ||
		repo.acceptInviteInput.InviteEmail != repo.inviteSnapshot.Email {
		t.Fatalf("accept invite input = %#v", repo.acceptInviteInput)
	}
	if !strings.HasPrefix(repo.acceptInviteInput.PasswordHash, "$argon2id$") {
		t.Fatalf("password hash = %q", repo.acceptInviteInput.PasswordHash)
	}
	valid, verifyErr := verifyPassword(context.Background(), "new-invite-password", repo.acceptInviteInput.PasswordHash)
	if verifyErr != nil || !valid {
		t.Fatalf("verify new invite hash = %v/%v", valid, verifyErr)
	}
	if repo.acceptInviteInput.SessionID != "55555555-5555-4555-8555-555555555555" ||
		repo.acceptInviteInput.SessionTokenHash != HashSessionToken(testRawToken('e')) {
		t.Fatalf("session input = %#v", repo.acceptInviteInput)
	}
	if result.Token != testRawToken('e') || result.User.ID != repo.acceptInviteInput.UserID {
		t.Fatalf("AcceptInvite() result = %#v", result)
	}
}

func TestServiceAcceptInviteExistingCredentialUsesFence(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	repo := &fakeAuthRepository{
		credential: LoginCredential{
			UserID:             "66666666-6666-4666-8666-666666666666",
			Email:              "existing-member@example.test",
			DisplayName:        "Existing Member",
			PasswordHash:       defaultDummyPasswordHash,
			CredentialRevision: 9,
		},
		inviteSnapshot: InviteAcceptanceSnapshot{
			TeamID: "77777777-7777-4777-8777-777777777777",
			Email:  "existing-member@example.test",
		},
	}
	service := NewService(repo, WithServiceClock(func() time.Time { return now }))
	ids := []string{"88888888-8888-4888-8888-888888888888"}
	service.newID = func() (string, error) {
		if len(ids) == 0 {
			return "", errors.New("unexpected user id generation")
		}
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	service.newToken = func() (string, error) { return testRawToken('a'), nil }

	result, err := service.AcceptInvite(context.Background(), AcceptInviteInput{
		Token:     testRawToken('b'),
		Password:  validTestPassword,
		UserAgent: "existing-agent",
	})
	if err != nil {
		t.Fatalf("AcceptInvite() error = %v", err)
	}
	if repo.acceptInviteInput.UserID != repo.credential.UserID ||
		repo.acceptInviteInput.CredentialRevision != repo.credential.CredentialRevision ||
		repo.acceptInviteInput.PasswordHash != "" {
		t.Fatalf("accept invite input = %#v", repo.acceptInviteInput)
	}
	if repo.acceptInviteInput.InviteTeamID != repo.inviteSnapshot.TeamID ||
		repo.acceptInviteInput.InviteEmail != repo.inviteSnapshot.Email {
		t.Fatalf("invite snapshot not forwarded = %#v", repo.acceptInviteInput)
	}
	if result.Token != testRawToken('a') || result.User.ID != repo.credential.UserID {
		t.Fatalf("AcceptInvite() result = %#v", result)
	}
}

func TestServiceAcceptInviteCollapsesWrongExistingPassword(t *testing.T) {
	repo := &fakeAuthRepository{
		credential: LoginCredential{
			UserID:             "99999999-9999-4999-8999-999999999999",
			Email:              "existing-member@example.test",
			PasswordHash:       defaultDummyPasswordHash,
			CredentialRevision: 2,
		},
		inviteSnapshot: InviteAcceptanceSnapshot{
			TeamID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Email:  "existing-member@example.test",
		},
	}
	service := NewService(repo)

	_, err := service.AcceptInvite(context.Background(), AcceptInviteInput{
		Token:    testRawToken('c'),
		Password: "definitely-the-wrong-password",
	})
	if !errors.Is(err, ErrInviteNotActive) {
		t.Fatalf("wrong existing password AcceptInvite() error = %v", err)
	}
	if repo.acceptInviteCalled {
		t.Fatalf("AcceptInvite() should not reach repository, input = %#v", repo.acceptInviteInput)
	}
}

func TestServiceAcceptInviteRejectsMalformedPasswordBeforeTokenSnapshot(t *testing.T) {
	tests := []struct {
		name string
		repo *fakeAuthRepository
	}{
		{
			name: "active invite",
			repo: &fakeAuthRepository{
				inviteSnapshot: InviteAcceptanceSnapshot{
					TeamID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
					Email:  "invitee@example.test",
				},
			},
		},
		{
			name: "inactive invite",
			repo: &fakeAuthRepository{
				inviteSnapshotErr: ErrInviteNotActive,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(tt.repo)

			_, err := service.AcceptInvite(context.Background(), AcceptInviteInput{
				Token:    testRawToken('d'),
				Password: "too-short",
			})
			if !errors.Is(err, ErrInvalidIdentityInput) {
				t.Fatalf("AcceptInvite() error = %v, want ErrInvalidIdentityInput", err)
			}
			if tt.repo.inviteLookupTokenHash != "" {
				t.Fatalf("invite snapshot should not be queried, got %q", tt.repo.inviteLookupTokenHash)
			}
			if tt.repo.lookupEmail != "" || tt.repo.acceptInviteCalled {
				t.Fatalf("unexpected downstream calls lookup=%q accept=%v", tt.repo.lookupEmail, tt.repo.acceptInviteCalled)
			}
		})
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
	credential            LoginCredential
	lookupEmail           string
	lookupErr             error
	inviteSnapshot        InviteAcceptanceSnapshot
	inviteSnapshotErr     error
	inviteLookupTokenHash string
	sessionInput          CreateCredentialSessionInput
	acceptInviteInput     AcceptInviteRepositoryInput
	acceptInviteCalled    bool
	recoveryInput         CreateRecoveryTokenInput
	recoveryTarget        RecoveryTarget
	deliverRecovery       bool
	completeInput         CompleteRecoveryRepositoryInput
	completeRevoked       []RevokedSession
	revokedTokenHash      string
	revokedUserID         string
}

func (r *fakeAuthRepository) LookupLoginCredential(_ context.Context, email string) (LoginCredential, error) {
	r.lookupEmail = email
	if r.lookupErr != nil {
		return LoginCredential{}, r.lookupErr
	}
	if r.credential.Email != "" && !strings.EqualFold(r.credential.Email, email) {
		return LoginCredential{}, ErrInvalidCredential
	}
	return r.credential, nil
}

func (r *fakeAuthRepository) LookupInviteAcceptanceSnapshot(_ context.Context, inviteTokenHash string) (InviteAcceptanceSnapshot, error) {
	r.inviteLookupTokenHash = inviteTokenHash
	if r.inviteSnapshotErr != nil {
		return InviteAcceptanceSnapshot{}, r.inviteSnapshotErr
	}
	if r.inviteSnapshot.TeamID == "" {
		return InviteAcceptanceSnapshot{
			TeamID: "12345678-1234-4234-8234-123456789012",
			Email:  "invitee@example.test",
		}, nil
	}
	return r.inviteSnapshot, nil
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
	r.acceptInviteCalled = true
	r.acceptInviteInput = input
	return Session{
		ID:          input.SessionID,
		UserID:      input.UserID,
		DisplayName: r.credential.DisplayName,
		Role:        defaultUserRole,
		ExpiresAt:   input.SessionExpiresAt,
	}, nil
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
