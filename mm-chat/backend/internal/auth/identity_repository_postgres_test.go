package auth

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPostgresIdentityInviteAcceptanceIsAtomicAndOneTime(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inviterID := mustSessionTestUUID(t)
	teamID := mustSessionTestUUID(t)
	inviteID := mustSessionTestUUID(t)
	invitedUserID := mustSessionTestUUID(t)
	sessionID := mustSessionTestUUID(t)
	rawInviteToken := testRawToken('1')
	passwordHash, err := hashPassword(ctx, "invite-password-value")
	if err != nil {
		t.Fatalf("hash invite password: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name)
VALUES ($1, $2, 'Inviter')
`, inviterID, "inviter-"+inviterID+"@example.test"); err != nil {
		t.Fatalf("insert inviter: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO teams (id, name, created_by_user_id)
VALUES ($1, 'Identity Test Team', $2)
`, teamID, inviterID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO team_invites (
  id, team_id, invited_by_user_id, token_hash, email, role, expires_at
) VALUES ($1, $2, $3, $4, $5, 'member', now() + interval '1 hour')
`,
		inviteID,
		teamID,
		inviterID,
		HashSessionToken(rawInviteToken),
		"invited-"+invitedUserID+"@example.test",
	); err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	input := AcceptInviteRepositoryInput{
		InviteTokenHash:  HashSessionToken(rawInviteToken),
		PasswordHash:     passwordHash,
		UserID:           invitedUserID,
		SessionID:        sessionID,
		SessionTokenHash: HashSessionToken(testRawToken('2')),
		UserAgent:        "identity-test",
		SessionExpiresAt: time.Now().Add(time.Hour).UTC(),
	}
	session, err := repo.AcceptInvite(ctx, input)
	if err != nil {
		t.Fatalf("AcceptInvite() error = %v", err)
	}
	if session.UserID != invitedUserID || session.ID != sessionID {
		t.Fatalf("accepted session = %#v", session)
	}

	var inviteStatus, membershipRole, passwordPHC string
	var membershipRevision int64
	if err := db.QueryRowContext(ctx, `SELECT status FROM team_invites WHERE id = $1`, inviteID).Scan(&inviteStatus); err != nil {
		t.Fatalf("query invite: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT role FROM team_memberships WHERE team_id = $1 AND user_id = $2
`, teamID, invitedUserID).Scan(&membershipRole); err != nil {
		t.Fatalf("query membership: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT membership_revision FROM teams WHERE id = $1`, teamID).Scan(&membershipRevision); err != nil {
		t.Fatalf("query membership revision: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM user_credentials WHERE user_id = $1`, invitedUserID).Scan(&passwordPHC); err != nil {
		t.Fatalf("query credential: %v", err)
	}
	if inviteStatus != "accepted" || membershipRole != "member" || membershipRevision != 2 || passwordPHC != passwordHash {
		t.Fatalf(
			"accepted state status/role/revision/hash = %q/%q/%d/%t",
			inviteStatus,
			membershipRole,
			membershipRevision,
			passwordPHC == passwordHash,
		)
	}

	input.UserID = mustSessionTestUUID(t)
	input.SessionID = mustSessionTestUUID(t)
	input.SessionTokenHash = HashSessionToken(testRawToken('3'))
	_, err = repo.AcceptInvite(ctx, input)
	if !errors.Is(err, ErrInviteNotActive) {
		t.Fatalf("second AcceptInvite() error = %v, want ErrInviteNotActive", err)
	}
}

func TestPostgresIdentityRecoveryRotatesCredentialAndRevokesSessions(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	userID := mustSessionTestUUID(t)
	email := "recovery-" + userID + "@example.test"
	oldHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	newPassword := "replacement-password-value"
	newHash, err := hashPassword(ctx, newPassword)
	if err != nil {
		t.Fatalf("hash new password: %v", err)
	}
	insertIdentityFixture(t, ctx, db, userID, email, oldHash)

	sessionIDs := []string{mustSessionTestUUID(t), mustSessionTestUUID(t)}
	for index, sessionID := range sessionIDs {
		if _, err := db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, now() + interval '1 hour')
`, sessionID, userID, HashSessionToken(testRawToken(byte('4'+index)))); err != nil {
			t.Fatalf("insert session %d: %v", index, err)
		}
	}

	recoveryToken := testRawToken('6')
	target, deliver, err := repo.CreateRecoveryToken(ctx, CreateRecoveryTokenInput{
		CanonicalEmail: email,
		TokenID:        mustSessionTestUUID(t),
		TokenHash:      HashSessionToken(recoveryToken),
		TTL:            30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CreateRecoveryToken() error = %v", err)
	}
	if !deliver || target.Email != email || !target.ExpiresAt.After(time.Now()) {
		t.Fatalf("recovery target/deliver = %#v/%v", target, deliver)
	}

	revoked, err := repo.CompleteRecovery(ctx, CompleteRecoveryRepositoryInput{
		TokenHash:    HashSessionToken(recoveryToken),
		PasswordHash: newHash,
	})
	if err != nil {
		t.Fatalf("CompleteRecovery() error = %v", err)
	}
	if len(revoked) != len(sessionIDs) {
		t.Fatalf("revoked sessions = %#v, want %d", revoked, len(sessionIDs))
	}

	credential, err := repo.LookupLoginCredential(ctx, email)
	if err != nil {
		t.Fatalf("LookupLoginCredential() error = %v", err)
	}
	if credential.CredentialRevision != 2 || credential.PasswordHash != newHash {
		t.Fatalf("recovered credential = %#v", credential)
	}
	valid, err := verifyPassword(ctx, newPassword, credential.PasswordHash)
	if err != nil || !valid {
		t.Fatalf("verify recovered password = %v/%v", valid, err)
	}

	_, err = repo.CompleteRecovery(ctx, CompleteRecoveryRepositoryInput{
		TokenHash:    HashSessionToken(recoveryToken),
		PasswordHash: newHash,
	})
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("reused recovery token error = %v, want ErrInvalidCredential", err)
	}

	_, err = repo.CreateCredentialSession(ctx, CreateCredentialSessionInput{
		SessionID:          mustSessionTestUUID(t),
		UserID:             userID,
		TokenHash:          HashSessionToken(testRawToken('7')),
		ExpiresAt:          time.Now().Add(time.Hour),
		CredentialRevision: 1,
	})
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("stale credential revision session error = %v", err)
	}
}

func TestPostgresIdentityRecoveryConcurrentConsumptionHasOneWinner(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	userID := mustSessionTestUUID(t)
	email := "recovery-race-" + userID + "@example.test"
	passwordHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	insertIdentityFixture(t, ctx, db, userID, email, passwordHash)
	rawToken := testRawToken('8')
	if _, _, err := repo.CreateRecoveryToken(ctx, CreateRecoveryTokenInput{
		CanonicalEmail: email,
		TokenID:        mustSessionTestUUID(t),
		TokenHash:      HashSessionToken(rawToken),
		TTL:            30 * time.Minute,
	}); err != nil {
		t.Fatalf("create recovery token: %v", err)
	}

	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, completeErr := repo.CompleteRecovery(ctx, CompleteRecoveryRepositoryInput{
				TokenHash:    HashSessionToken(rawToken),
				PasswordHash: passwordHash,
			})
			errorsCh <- completeErr
		}()
	}
	close(start)
	wait.Wait()
	close(errorsCh)

	var success, inactive int
	for completeErr := range errorsCh {
		switch {
		case completeErr == nil:
			success++
		case errors.Is(completeErr, ErrInvalidCredential):
			inactive++
		default:
			t.Fatalf("unexpected CompleteRecovery() error = %v", completeErr)
		}
	}
	if success != 1 || inactive != 1 {
		t.Fatalf("recovery race success/inactive = %d/%d, want 1/1", success, inactive)
	}
}

func TestPostgresSessionResolutionRejectsDisabledAccountDespiteSession(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	userID := mustSessionTestUUID(t)
	email := "disabled-" + userID + "@example.test"
	passwordHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	insertIdentityFixture(t, ctx, db, userID, email, passwordHash)
	tokenHash := HashSessionToken(testRawToken('9'))
	if _, err := db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, now() + interval '1 hour')
`, mustSessionTestUUID(t), userID, tokenHash); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET account_status = 'disabled' WHERE id = $1`, userID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	_, err = repo.LookupSessionByTokenHash(ctx, tokenHash)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("disabled LookupSessionByTokenHash() error = %v", err)
	}
	_, err = repo.LookupLoginCredential(ctx, email)
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("disabled LookupLoginCredential() error = %v", err)
	}
}

func insertIdentityFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	userID string,
	email string,
	passwordHash string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status)
VALUES ($1, $2, 'Identity User', 'active')
`, userID, email); err != nil {
		t.Fatalf("insert identity user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO user_credentials (user_id, password_hash, email_verified_at)
VALUES ($1, $2, now())
`, userID, passwordHash); err != nil {
		t.Fatalf("insert identity credential: %v", err)
	}
}
