package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

type teamMembershipChangedPayload struct {
	TeamID             string `json:"teamId"`
	UserID             string `json:"userId"`
	Operation          string `json:"operation"`
	TeamRole           string `json:"teamRole"`
	Status             string `json:"status"`
	MembershipRevision int64  `json:"membershipRevision"`
}

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
	inviteEmail := "invited-" + invitedUserID + "@example.test"
	passwordHash, err := hashPassword(ctx, "invite-password-value")
	if err != nil {
		t.Fatalf("hash invite password: %v", err)
	}

	insertUserFixture(t, ctx, db, inviterID, "inviter-"+inviterID+"@example.test", "Inviter")
	insertTeamFixture(t, ctx, db, teamID, inviterID)
	insertInviteFixture(t, ctx, db, inviteID, teamID, inviterID, HashSessionToken(rawInviteToken), inviteEmail, "member")
	insertInviteMailOutboxFixture(t, ctx, db, inviteID, teamID, "sent")

	input := AcceptInviteRepositoryInput{
		InviteTokenHash:  HashSessionToken(rawInviteToken),
		InviteTeamID:     teamID,
		InviteEmail:      inviteEmail,
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

	var inviteStatus, membershipRole, membershipStatus, passwordPHC string
	var membershipRevision int64
	if err := db.QueryRowContext(ctx, `SELECT status FROM team_invites WHERE id = $1`, inviteID).Scan(&inviteStatus); err != nil {
		t.Fatalf("query invite: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT role, status FROM team_memberships WHERE team_id = $1 AND user_id = $2
`, teamID, invitedUserID).Scan(&membershipRole, &membershipStatus); err != nil {
		t.Fatalf("query membership: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT membership_revision FROM teams WHERE id = $1`, teamID).Scan(&membershipRevision); err != nil {
		t.Fatalf("query membership revision: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT password_hash FROM user_credentials WHERE user_id = $1`, invitedUserID).Scan(&passwordPHC); err != nil {
		t.Fatalf("query credential: %v", err)
	}
	if inviteStatus != "accepted" || membershipRole != "member" || membershipStatus != "active" || membershipRevision != 2 || passwordPHC != passwordHash {
		t.Fatalf(
			"accepted state status/role/membership/revision/hash = %q/%q/%q/%d/%t",
			inviteStatus,
			membershipRole,
			membershipStatus,
			membershipRevision,
			passwordPHC == passwordHash,
		)
	}
	payloads := loadTeamMembershipChangedPayloads(t, ctx, db, teamID)
	if len(payloads) != 1 {
		t.Fatalf("membership outbox payloads = %#v", payloads)
	}
	if payloads[0].UserID != invitedUserID ||
		payloads[0].Operation != "added" ||
		payloads[0].TeamRole != "member" ||
		payloads[0].Status != "active" ||
		payloads[0].MembershipRevision != 2 {
		t.Fatalf("membership outbox payload = %#v", payloads[0])
	}

	input.UserID = mustSessionTestUUID(t)
	input.SessionID = mustSessionTestUUID(t)
	input.SessionTokenHash = HashSessionToken(testRawToken('3'))
	_, err = repo.AcceptInvite(ctx, input)
	if !errors.Is(err, ErrInviteNotActive) {
		t.Fatalf("second AcceptInvite() error = %v, want ErrInviteNotActive", err)
	}
}

func TestPostgresIdentityInviteAcceptanceExistingAccountPreservesCredential(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	inviterID := mustSessionTestUUID(t)
	teamID := mustSessionTestUUID(t)
	inviteID := mustSessionTestUUID(t)
	existingUserID := mustSessionTestUUID(t)
	sessionID := mustSessionTestUUID(t)
	rawInviteToken := testRawToken('4')
	inviteEmail := "existing-" + existingUserID + "@example.test"
	passwordHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash existing password: %v", err)
	}

	insertUserFixture(t, ctx, db, inviterID, "inviter-"+inviterID+"@example.test", "Inviter")
	insertIdentityFixture(t, ctx, db, existingUserID, inviteEmail, passwordHash)
	insertTeamFixture(t, ctx, db, teamID, inviterID)
	insertInviteFixture(t, ctx, db, inviteID, teamID, inviterID, HashSessionToken(rawInviteToken), inviteEmail, "member")
	insertInviteMailOutboxFixture(t, ctx, db, inviteID, teamID, "sent")

	now := time.Now().UTC()
	service := NewService(
		repo,
		WithSessionTTL(time.Hour),
		WithServiceClock(func() time.Time { return now }),
	)
	ids := []string{sessionID}
	service.newID = func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	service.newToken = func() (string, error) { return testRawToken('5'), nil }

	result, err := service.AcceptInvite(ctx, AcceptInviteInput{
		Token:     rawInviteToken,
		Password:  validTestPassword,
		UserAgent: "existing-account-test",
	})
	if err != nil {
		t.Fatalf("Service.AcceptInvite() error = %v", err)
	}
	if result.User.ID != existingUserID || result.Token != testRawToken('5') {
		t.Fatalf("accept result = %#v", result)
	}

	credential, err := repo.LookupLoginCredential(ctx, inviteEmail)
	if err != nil {
		t.Fatalf("LookupLoginCredential() error = %v", err)
	}
	if credential.UserID != existingUserID || credential.CredentialRevision != 1 || credential.PasswordHash != passwordHash {
		t.Fatalf("existing credential mutated = %#v", credential)
	}

	var inviteStatus, membershipRole, membershipStatus string
	var membershipRevision int64
	if err := db.QueryRowContext(ctx, `SELECT status FROM team_invites WHERE id = $1`, inviteID).Scan(&inviteStatus); err != nil {
		t.Fatalf("query invite: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT role, status FROM team_memberships WHERE team_id = $1 AND user_id = $2
`, teamID, existingUserID).Scan(&membershipRole, &membershipStatus); err != nil {
		t.Fatalf("query membership: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT membership_revision FROM teams WHERE id = $1`, teamID).Scan(&membershipRevision); err != nil {
		t.Fatalf("query membership revision: %v", err)
	}
	if inviteStatus != "accepted" || membershipRole != "member" || membershipStatus != "active" || membershipRevision != 2 {
		t.Fatalf("existing accept state = %q/%q/%q/%d", inviteStatus, membershipRole, membershipStatus, membershipRevision)
	}
	payloads := loadTeamMembershipChangedPayloads(t, ctx, db, teamID)
	if len(payloads) != 1 || payloads[0].Operation != "added" || payloads[0].UserID != existingUserID {
		t.Fatalf("membership outbox payloads = %#v", payloads)
	}
}

func TestPostgresIdentityInviteAcceptanceReactivatesRemovedMembership(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	inviterID := mustSessionTestUUID(t)
	teamID := mustSessionTestUUID(t)
	inviteID := mustSessionTestUUID(t)
	userID := mustSessionTestUUID(t)
	sessionID := mustSessionTestUUID(t)
	rawInviteToken := testRawToken('6')
	inviteEmail := "reactivate-" + userID + "@example.test"
	passwordHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	insertUserFixture(t, ctx, db, inviterID, "inviter-"+inviterID+"@example.test", "Inviter")
	insertIdentityFixture(t, ctx, db, userID, inviteEmail, passwordHash)
	insertTeamFixture(t, ctx, db, teamID, inviterID)
	if _, err := db.ExecContext(ctx, `
INSERT INTO team_memberships (team_id, user_id, role, status, removed_at, created_at, updated_at)
VALUES ($1, $2, 'member', 'removed', now() - interval '1 day', now() - interval '2 days', now() - interval '1 day')
`, teamID, userID); err != nil {
		t.Fatalf("insert removed membership: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE teams SET membership_revision = 7 WHERE id = $1`, teamID); err != nil {
		t.Fatalf("seed membership revision: %v", err)
	}
	insertInviteFixture(t, ctx, db, inviteID, teamID, inviterID, HashSessionToken(rawInviteToken), inviteEmail, "admin")
	insertInviteMailOutboxFixture(t, ctx, db, inviteID, teamID, "sent")

	session, err := repo.AcceptInvite(ctx, AcceptInviteRepositoryInput{
		InviteTokenHash:    HashSessionToken(rawInviteToken),
		InviteTeamID:       teamID,
		InviteEmail:        inviteEmail,
		UserID:             userID,
		CredentialRevision: 1,
		SessionID:          sessionID,
		SessionTokenHash:   HashSessionToken(testRawToken('7')),
		UserAgent:          "reactivate-test",
		SessionExpiresAt:   time.Now().Add(time.Hour).UTC(),
	})
	if err != nil {
		t.Fatalf("AcceptInvite() error = %v", err)
	}
	if session.UserID != userID || session.DisplayName != "Identity User" {
		t.Fatalf("reactivated session = %#v", session)
	}

	var membershipRole, membershipStatus string
	var removedAt sql.NullTime
	var membershipRevision int64
	if err := db.QueryRowContext(ctx, `
SELECT role, status, removed_at FROM team_memberships WHERE team_id = $1 AND user_id = $2
`, teamID, userID).Scan(&membershipRole, &membershipStatus, &removedAt); err != nil {
		t.Fatalf("query membership: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT membership_revision FROM teams WHERE id = $1`, teamID).Scan(&membershipRevision); err != nil {
		t.Fatalf("query membership revision: %v", err)
	}
	if membershipRole != "admin" || membershipStatus != "active" || removedAt.Valid || membershipRevision != 8 {
		t.Fatalf("reactivated membership role/status/removed/revision = %q/%q/%v/%d", membershipRole, membershipStatus, removedAt.Valid, membershipRevision)
	}
	payloads := loadTeamMembershipChangedPayloads(t, ctx, db, teamID)
	if len(payloads) != 1 || payloads[0].Operation != "reactivated" || payloads[0].TeamRole != "admin" || payloads[0].MembershipRevision != 8 {
		t.Fatalf("membership outbox payloads = %#v", payloads)
	}
}

func TestPostgresIdentityInviteAcceptanceRequiresSentMail(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inviterID := mustSessionTestUUID(t)
	teamID := mustSessionTestUUID(t)
	inviteID := mustSessionTestUUID(t)
	invitedUserID := mustSessionTestUUID(t)
	rawInviteToken := testRawToken('8')
	inviteEmail := "pending-" + invitedUserID + "@example.test"
	passwordHash, err := hashPassword(ctx, "pending-invite-password")
	if err != nil {
		t.Fatalf("hash invite password: %v", err)
	}

	insertUserFixture(t, ctx, db, inviterID, "inviter-"+inviterID+"@example.test", "Inviter")
	insertTeamFixture(t, ctx, db, teamID, inviterID)
	insertInviteFixture(t, ctx, db, inviteID, teamID, inviterID, HashSessionToken(rawInviteToken), inviteEmail, "member")
	insertInviteMailOutboxFixture(t, ctx, db, inviteID, teamID, "pending")

	_, err = repo.AcceptInvite(ctx, AcceptInviteRepositoryInput{
		InviteTokenHash:  HashSessionToken(rawInviteToken),
		InviteTeamID:     teamID,
		InviteEmail:      inviteEmail,
		PasswordHash:     passwordHash,
		UserID:           invitedUserID,
		SessionID:        mustSessionTestUUID(t),
		SessionTokenHash: HashSessionToken(testRawToken('9')),
		UserAgent:        "pending-mail",
		SessionExpiresAt: time.Now().Add(time.Hour).UTC(),
	})
	if !errors.Is(err, ErrInviteNotActive) {
		t.Fatalf("unsent mail AcceptInvite() error = %v, want ErrInviteNotActive", err)
	}
}

func TestPostgresIdentityInviteAcceptanceRejectsStaleCredentialFence(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	inviterID := mustSessionTestUUID(t)
	teamID := mustSessionTestUUID(t)
	inviteID := mustSessionTestUUID(t)
	userID := mustSessionTestUUID(t)
	rawInviteToken := testRawToken('a')
	inviteEmail := "stale-" + userID + "@example.test"
	passwordHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	insertUserFixture(t, ctx, db, inviterID, "inviter-"+inviterID+"@example.test", "Inviter")
	insertIdentityFixture(t, ctx, db, userID, inviteEmail, passwordHash)
	insertTeamFixture(t, ctx, db, teamID, inviterID)
	insertInviteFixture(t, ctx, db, inviteID, teamID, inviterID, HashSessionToken(rawInviteToken), inviteEmail, "member")
	insertInviteMailOutboxFixture(t, ctx, db, inviteID, teamID, "sent")
	if _, err := db.ExecContext(ctx, `
UPDATE user_credentials
SET credential_revision = 2,
    updated_at = now()
WHERE user_id = $1
`, userID); err != nil {
		t.Fatalf("advance credential revision: %v", err)
	}

	_, err = repo.AcceptInvite(ctx, AcceptInviteRepositoryInput{
		InviteTokenHash:    HashSessionToken(rawInviteToken),
		InviteTeamID:       teamID,
		InviteEmail:        inviteEmail,
		UserID:             userID,
		CredentialRevision: 1,
		SessionID:          mustSessionTestUUID(t),
		SessionTokenHash:   HashSessionToken(testRawToken('b')),
		UserAgent:          "stale-fence",
		SessionExpiresAt:   time.Now().Add(time.Hour).UTC(),
	})
	if !errors.Is(err, ErrInviteNotActive) {
		t.Fatalf("stale fence AcceptInvite() error = %v, want ErrInviteNotActive", err)
	}
}

func TestPostgresIdentityInviteAcceptanceConcurrentConsumptionHasOneWinner(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	inviterID := mustSessionTestUUID(t)
	teamID := mustSessionTestUUID(t)
	inviteID := mustSessionTestUUID(t)
	rawInviteToken := testRawToken('c')
	inviteEmail := "race-" + mustSessionTestUUID(t) + "@example.test"
	passwordHash, err := hashPassword(ctx, "race-invite-password")
	if err != nil {
		t.Fatalf("hash invite password: %v", err)
	}

	insertUserFixture(t, ctx, db, inviterID, "inviter-"+inviterID+"@example.test", "Inviter")
	insertTeamFixture(t, ctx, db, teamID, inviterID)
	insertInviteFixture(t, ctx, db, inviteID, teamID, inviterID, HashSessionToken(rawInviteToken), inviteEmail, "member")
	insertInviteMailOutboxFixture(t, ctx, db, inviteID, teamID, "sent")

	start := make(chan struct{})
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		userID := mustSessionTestUUID(t)
		sessionID := mustSessionTestUUID(t)
		wait.Add(1)
		go func(userID string, sessionID string, tokenSuffix byte) {
			defer wait.Done()
			<-start
			_, acceptErr := repo.AcceptInvite(ctx, AcceptInviteRepositoryInput{
				InviteTokenHash:  HashSessionToken(rawInviteToken),
				InviteTeamID:     teamID,
				InviteEmail:      inviteEmail,
				PasswordHash:     passwordHash,
				UserID:           userID,
				SessionID:        sessionID,
				SessionTokenHash: HashSessionToken(testRawToken(tokenSuffix)),
				UserAgent:        "concurrent-accept",
				SessionExpiresAt: time.Now().Add(time.Hour).UTC(),
			})
			errorsCh <- acceptErr
		}(userID, sessionID, byte('d'+i))
	}
	close(start)
	wait.Wait()
	close(errorsCh)

	var success, inactive int
	for acceptErr := range errorsCh {
		switch {
		case acceptErr == nil:
			success++
		case errors.Is(acceptErr, ErrInviteNotActive):
			inactive++
		default:
			t.Fatalf("unexpected AcceptInvite() error = %v", acceptErr)
		}
	}
	if success != 1 || inactive != 1 {
		t.Fatalf("invite race success/inactive = %d/%d, want 1/1", success, inactive)
	}

	var membershipRevision int64
	var membershipCount int
	if err := db.QueryRowContext(ctx, `SELECT membership_revision FROM teams WHERE id = $1`, teamID).Scan(&membershipRevision); err != nil {
		t.Fatalf("query membership revision: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM team_memberships WHERE team_id = $1 AND status = 'active'
`, teamID).Scan(&membershipCount); err != nil {
		t.Fatalf("count active memberships: %v", err)
	}
	if membershipRevision != 2 || membershipCount != 1 {
		t.Fatalf("invite race revision/count = %d/%d", membershipRevision, membershipCount)
	}
	payloads := loadTeamMembershipChangedPayloads(t, ctx, db, teamID)
	if len(payloads) != 1 || payloads[0].Operation != "added" {
		t.Fatalf("membership outbox payloads = %#v", payloads)
	}
}

func TestPostgresCredentialSessionConcurrentWithExistingInviteUsesStableRowOrder(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	inviterID := mustSessionTestUUID(t)
	teamID := mustSessionTestUUID(t)
	inviteID := mustSessionTestUUID(t)
	userID := mustSessionTestUUID(t)
	rawInviteToken := testRawToken('l')
	inviteEmail := "existing-race-" + userID + "@example.test"
	passwordHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	insertUserFixture(t, ctx, db, inviterID, "inviter-"+inviterID+"@example.test", "Inviter")
	insertIdentityFixture(t, ctx, db, userID, inviteEmail, passwordHash)
	insertTeamFixture(t, ctx, db, teamID, inviterID)
	insertInviteFixture(t, ctx, db, inviteID, teamID, inviterID, HashSessionToken(rawInviteToken), inviteEmail, "member")
	insertInviteMailOutboxFixture(t, ctx, db, inviteID, teamID, "sent")
	loginSessionID := mustSessionTestUUID(t)
	inviteSessionID := mustSessionTestUUID(t)

	start := make(chan struct{})
	var wait sync.WaitGroup
	var loginErr error
	var inviteErr error

	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, loginErr = repo.CreateCredentialSession(ctx, CreateCredentialSessionInput{
			SessionID:          loginSessionID,
			UserID:             userID,
			TokenHash:          HashSessionToken(testRawToken('m')),
			UserAgent:          "login-race",
			ExpiresAt:          time.Now().Add(time.Hour).UTC(),
			CredentialRevision: 1,
		})
	}()
	go func() {
		defer wait.Done()
		<-start
		_, inviteErr = repo.AcceptInvite(ctx, AcceptInviteRepositoryInput{
			InviteTokenHash:    HashSessionToken(rawInviteToken),
			InviteTeamID:       teamID,
			InviteEmail:        inviteEmail,
			UserID:             userID,
			CredentialRevision: 1,
			SessionID:          inviteSessionID,
			SessionTokenHash:   HashSessionToken(testRawToken('n')),
			UserAgent:          "invite-race",
			SessionExpiresAt:   time.Now().Add(time.Hour).UTC(),
		})
	}()
	close(start)
	wait.Wait()

	if loginErr != nil {
		t.Fatalf("CreateCredentialSession() error = %v", loginErr)
	}
	if inviteErr != nil {
		t.Fatalf("AcceptInvite() error = %v", inviteErr)
	}

	var membershipRevision int64
	var activeMemberships int
	var activeSessions int
	if err := db.QueryRowContext(ctx, `SELECT membership_revision FROM teams WHERE id = $1`, teamID).Scan(&membershipRevision); err != nil {
		t.Fatalf("query membership revision: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM team_memberships WHERE team_id = $1 AND user_id = $2 AND status = 'active'
`, teamID, userID).Scan(&activeMemberships); err != nil {
		t.Fatalf("count active memberships: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL
`, userID).Scan(&activeSessions); err != nil {
		t.Fatalf("count active sessions: %v", err)
	}
	if membershipRevision != 2 || activeMemberships != 1 || activeSessions != 2 {
		t.Fatalf("post race revision/memberships/sessions = %d/%d/%d", membershipRevision, activeMemberships, activeSessions)
	}
	payloads := loadTeamMembershipChangedPayloads(t, ctx, db, teamID)
	if len(payloads) != 1 || payloads[0].Operation != "added" || payloads[0].UserID != userID {
		t.Fatalf("membership outbox payloads = %#v", payloads)
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
`, sessionID, userID, HashSessionToken(testRawToken(byte('f'+index)))); err != nil {
			t.Fatalf("insert session %d: %v", index, err)
		}
	}

	recoveryToken := testRawToken('h')
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
		TokenHash:          HashSessionToken(testRawToken('i')),
		ExpiresAt:          time.Now().Add(time.Hour),
		CredentialRevision: 1,
	})
	if !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("stale credential revision session error = %v", err)
	}
}

func TestPostgresCredentialSessionConcurrentWithRecoveryUsesStableRowOrder(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	userID := mustSessionTestUUID(t)
	email := "login-recovery-" + userID + "@example.test"
	passwordHash, err := hashPassword(ctx, validTestPassword)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	newHash, err := hashPassword(ctx, "replacement-password-value")
	if err != nil {
		t.Fatalf("hash replacement password: %v", err)
	}
	insertIdentityFixture(t, ctx, db, userID, email, passwordHash)

	sessionID := mustSessionTestUUID(t)
	rawRecoveryToken := testRawToken('o')
	if _, _, err := repo.CreateRecoveryToken(ctx, CreateRecoveryTokenInput{
		CanonicalEmail: email,
		TokenID:        mustSessionTestUUID(t),
		TokenHash:      HashSessionToken(rawRecoveryToken),
		TTL:            30 * time.Minute,
	}); err != nil {
		t.Fatalf("CreateRecoveryToken() error = %v", err)
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	var loginErr error
	var recoveryErr error
	var revoked []RevokedSession

	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, loginErr = repo.CreateCredentialSession(ctx, CreateCredentialSessionInput{
			SessionID:          sessionID,
			UserID:             userID,
			TokenHash:          HashSessionToken(testRawToken('p')),
			UserAgent:          "login-vs-recovery",
			ExpiresAt:          time.Now().Add(time.Hour).UTC(),
			CredentialRevision: 1,
		})
	}()
	go func() {
		defer wait.Done()
		<-start
		revoked, recoveryErr = repo.CompleteRecovery(ctx, CompleteRecoveryRepositoryInput{
			TokenHash:    HashSessionToken(rawRecoveryToken),
			PasswordHash: newHash,
		})
	}()
	close(start)
	wait.Wait()

	if recoveryErr != nil {
		t.Fatalf("CompleteRecovery() error = %v", recoveryErr)
	}
	switch {
	case loginErr == nil:
		var revokedAt sql.NullTime
		if err := db.QueryRowContext(ctx, `SELECT revoked_at FROM sessions WHERE id = $1`, sessionID).Scan(&revokedAt); err != nil {
			t.Fatalf("query raced session: %v", err)
		}
		if !revokedAt.Valid {
			t.Fatalf("raced session should be revoked, got %+v", revokedAt)
		}
		if len(revoked) != 1 || revoked[0].ID != sessionID {
			t.Fatalf("revoked sessions = %#v", revoked)
		}
	case errors.Is(loginErr, ErrInvalidCredential):
		var sessionCount int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE id = $1`, sessionID).Scan(&sessionCount); err != nil {
			t.Fatalf("count raced session: %v", err)
		}
		if sessionCount != 0 {
			t.Fatalf("unexpected persisted session count = %d", sessionCount)
		}
		if len(revoked) != 0 {
			t.Fatalf("unexpected revoked sessions = %#v", revoked)
		}
	default:
		t.Fatalf("CreateCredentialSession() error = %v", loginErr)
	}

	credential, err := repo.LookupLoginCredential(ctx, email)
	if err != nil {
		t.Fatalf("LookupLoginCredential() error = %v", err)
	}
	if credential.CredentialRevision != 2 || credential.PasswordHash != newHash {
		t.Fatalf("recovered credential = %#v", credential)
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
	rawToken := testRawToken('j')
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
	tokenHash := HashSessionToken(testRawToken('k'))
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

func insertInviteMailOutboxFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	inviteID string,
	teamID string,
	status string,
) {
	t.Helper()
	attemptCount := 0
	if status == "sent" {
		attemptCount = 1
	}
	outboxID := mustSessionTestUUID(t)
	if _, err := db.ExecContext(ctx, `
INSERT INTO identity_mail_outbox (
  id,
  team_id,
  invite_id,
  key_id,
  payload_version,
  nonce,
  ciphertext,
  message_id,
  status,
  attempt_count,
  max_attempts,
  available_at,
  terminal_at,
  error_code
) VALUES (
  $1,
  $2,
  $3,
  'test-key',
  1,
  decode(substr(replace($1::text, '-', ''), 1, 24), 'hex'),
  decode('deadc0de', 'hex'),
  $4,
  $5,
  $6,
  5,
  now(),
  CASE WHEN $5 = 'sent' THEN now() ELSE NULL END,
  NULL
)
`, outboxID, teamID, inviteID, "<"+inviteID+"@example.test>", status, attemptCount); err != nil {
		t.Fatalf("insert identity_mail_outbox: %v", err)
	}
}

func insertUserFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	userID string,
	email string,
	displayName string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status)
VALUES ($1, $2, $3, 'active')
`, userID, email, displayName); err != nil {
		t.Fatalf("insert user fixture: %v", err)
	}
}

func insertTeamFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	teamID string,
	inviterID string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO teams (id, name, created_by_user_id)
VALUES ($1, 'Identity Test Team', $2)
`, teamID, inviterID); err != nil {
		t.Fatalf("insert team: %v", err)
	}
}

func insertInviteFixture(
	t *testing.T,
	ctx context.Context,
	db interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	inviteID string,
	teamID string,
	inviterID string,
	tokenHash string,
	email string,
	role string,
) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
INSERT INTO team_invites (
  id,
  team_id,
  invited_by_user_id,
  token_hash,
  email,
  role,
  expires_at
) VALUES ($1, $2, $3, $4, $5, $6, now() + interval '1 hour')
`, inviteID, teamID, inviterID, tokenHash, email, role); err != nil {
		t.Fatalf("insert invite: %v", err)
	}
}

func loadTeamMembershipChangedPayloads(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	teamID string,
) []teamMembershipChangedPayload {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
SELECT payload
FROM knowledge_outbox
WHERE aggregate_type = 'team'
  AND aggregate_key = $1
  AND event_type = 'team.membership.changed'
ORDER BY id ASC
`, teamID)
	if err != nil {
		t.Fatalf("query knowledge outbox: %v", err)
	}
	defer rows.Close()

	var payloads []teamMembershipChangedPayload
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan knowledge outbox payload: %v", err)
		}
		var payload teamMembershipChangedPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal knowledge outbox payload: %v", err)
		}
		payloads = append(payloads, payload)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate knowledge outbox payloads: %v", err)
	}
	return payloads
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
