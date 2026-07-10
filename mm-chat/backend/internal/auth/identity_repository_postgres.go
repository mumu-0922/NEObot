package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func (r *PostgresSessionRepository) LookupLoginCredential(
	ctx context.Context,
	canonicalEmail string,
) (LoginCredential, error) {
	if r == nil || r.db == nil {
		return LoginCredential{}, ErrDatabaseRequired
	}
	email, err := canonicalizeEmail(canonicalEmail)
	if err != nil || email != canonicalEmail {
		return LoginCredential{}, ErrInvalidCredential
	}

	var credential LoginCredential
	err = r.db.QueryRowContext(ctx, `
SELECT
  u.id,
  u.email,
  COALESCE(u.display_name, ''),
  c.password_hash,
  c.credential_revision
FROM users u
JOIN user_credentials c ON c.user_id = u.id
WHERE lower(u.email) = $1
  AND u.account_status = 'active'
  AND u.deleted_at IS NULL
  AND c.email_verified_at IS NOT NULL
`, email).Scan(
		&credential.UserID,
		&credential.Email,
		&credential.DisplayName,
		&credential.PasswordHash,
		&credential.CredentialRevision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return LoginCredential{}, ErrInvalidCredential
	}
	if err != nil {
		return LoginCredential{}, fmt.Errorf("lookup login credential: %w", err)
	}
	return credential, nil
}

func (r *PostgresSessionRepository) CreateCredentialSession(
	ctx context.Context,
	input CreateCredentialSessionInput,
) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, ErrDatabaseRequired
	}
	if !isUUID(strings.TrimSpace(input.SessionID)) || !isUUID(strings.TrimSpace(input.UserID)) {
		return Session{}, errors.New("session and user ids must be UUIDs")
	}
	tokenHash, err := cleanTokenHash(input.TokenHash)
	if err != nil {
		return Session{}, err
	}
	if input.CredentialRevision < 1 {
		return Session{}, errors.New("credential revision must be positive")
	}
	if !input.ExpiresAt.After(timeNow()) {
		return Session{}, errors.New("session expiry must be in the future")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, fmt.Errorf("begin credential session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var displayName string
	displayName, err = lockCredentialSessionUser(ctx, tx, input.UserID)
	if err != nil {
		return Session{}, err
	}
	if err := lockCredentialSessionRevision(ctx, tx, input.UserID, input.CredentialRevision); err != nil {
		return Session{}, err
	}

	session, err := insertSession(ctx, tx, insertSessionInput{
		SessionID:   input.SessionID,
		UserID:      input.UserID,
		DisplayName: displayName,
		TokenHash:   tokenHash,
		UserAgent:   input.UserAgent,
		ExpiresAt:   input.ExpiresAt,
	})
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, fmt.Errorf("commit credential session: %w", err)
	}
	return session, nil
}

func (r *PostgresSessionRepository) LookupInviteAcceptanceSnapshot(
	ctx context.Context,
	inviteTokenHash string,
) (InviteAcceptanceSnapshot, error) {
	if r == nil || r.db == nil {
		return InviteAcceptanceSnapshot{}, ErrDatabaseRequired
	}
	inviteHash, err := cleanTokenHash(inviteTokenHash)
	if err != nil {
		return InviteAcceptanceSnapshot{}, ErrInviteNotActive
	}

	var snapshot InviteAcceptanceSnapshot
	err = r.db.QueryRowContext(ctx, `
SELECT i.team_id, i.email
FROM team_invites i
JOIN teams t ON t.id = i.team_id
WHERE i.token_hash = $1
  AND i.status = 'pending'
  AND i.expires_at > now()
  AND t.deleted_at IS NULL
`, inviteHash).Scan(&snapshot.TeamID, &snapshot.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return InviteAcceptanceSnapshot{}, ErrInviteNotActive
	}
	if err != nil {
		return InviteAcceptanceSnapshot{}, fmt.Errorf("lookup invite acceptance snapshot: %w", err)
	}
	return snapshot, nil
}

func (r *PostgresSessionRepository) AcceptInvite(
	ctx context.Context,
	input AcceptInviteRepositoryInput,
) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, ErrDatabaseRequired
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	if !isUUID(input.UserID) || !isUUID(input.SessionID) {
		return Session{}, errors.New("invite user and session ids must be UUIDs")
	}
	input.InviteTeamID = strings.TrimSpace(input.InviteTeamID)
	if !isUUID(input.InviteTeamID) {
		return Session{}, errors.New("invite team id must be a UUID")
	}
	inviteEmail, err := canonicalizeEmail(input.InviteEmail)
	if err != nil || inviteEmail != strings.TrimSpace(input.InviteEmail) {
		return Session{}, errors.New("invite email must be canonical")
	}
	inviteHash, err := cleanTokenHash(input.InviteTokenHash)
	if err != nil {
		return Session{}, ErrInviteNotActive
	}
	sessionHash, err := cleanTokenHash(input.SessionTokenHash)
	if err != nil {
		return Session{}, err
	}
	existingCredential := input.CredentialRevision > 0
	switch {
	case existingCredential:
		if strings.TrimSpace(input.PasswordHash) != "" {
			return Session{}, errors.New("existing invite acceptance must not replace credential hash")
		}
	case !strings.HasPrefix(input.PasswordHash, "$argon2id$"):
		return Session{}, errors.New("invite password hash must be Argon2id")
	}
	if !input.SessionExpiresAt.After(timeNow()) {
		return Session{}, errors.New("invite session expiry must be in the future")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, fmt.Errorf("begin accept invite: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	displayName := ""
	if existingCredential {
		displayName, err = lockInviteAcceptanceUser(ctx, tx, input.UserID, inviteEmail)
		if err != nil {
			return Session{}, err
		}
	} else if err := lockInviteAcceptanceCanonicalEmail(ctx, tx, inviteEmail); err != nil {
		return Session{}, err
	}
	if err := lockInviteAcceptanceTeam(ctx, tx, input.InviteTeamID); err != nil {
		return Session{}, err
	}
	invite, err := lockInviteAcceptanceInvite(ctx, tx, inviteHash, input.InviteTeamID, inviteEmail)
	if err != nil {
		return Session{}, err
	}
	if err := lockInviteAcceptanceMailOutbox(ctx, tx, invite.ID); err != nil {
		return Session{}, err
	}
	if existingCredential {
		if err := lockInviteAcceptanceCredential(ctx, tx, input.UserID, input.CredentialRevision); err != nil {
			return Session{}, err
		}
	} else {
		if err := insertInviteAcceptanceUser(ctx, tx, input.UserID, inviteEmail); err != nil {
			return Session{}, err
		}
		if err := insertInviteAcceptanceCredential(ctx, tx, input.UserID, input.PasswordHash); err != nil {
			return Session{}, err
		}
	}

	operation, err := upsertInviteAcceptanceMembership(ctx, tx, input.InviteTeamID, input.UserID, invite.Role)
	if err != nil {
		return Session{}, err
	}
	revision, err := advanceInviteAcceptanceMembershipRevision(ctx, tx, input.InviteTeamID)
	if err != nil {
		return Session{}, err
	}
	if err := insertInviteAcceptanceMembershipOutbox(ctx, tx, input.InviteTeamID, input.UserID, invite.Role, operation, revision); err != nil {
		return Session{}, err
	}
	if err := consumeInviteAcceptanceInvite(ctx, tx, invite.ID); err != nil {
		return Session{}, err
	}

	session, err := insertSession(ctx, tx, insertSessionInput{
		SessionID:   input.SessionID,
		UserID:      input.UserID,
		DisplayName: displayName,
		TokenHash:   sessionHash,
		UserAgent:   input.UserAgent,
		ExpiresAt:   input.SessionExpiresAt,
	})
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		if isSerializationFailure(err) || isUniqueViolation(err) {
			return Session{}, ErrInviteNotActive
		}
		return Session{}, fmt.Errorf("commit accept invite: %w", err)
	}
	return session, nil
}

func (r *PostgresSessionRepository) CreateRecoveryToken(
	ctx context.Context,
	input CreateRecoveryTokenInput,
) (RecoveryTarget, bool, error) {
	if r == nil || r.db == nil {
		return RecoveryTarget{}, false, ErrDatabaseRequired
	}
	email, err := canonicalizeEmail(input.CanonicalEmail)
	if err != nil || email != input.CanonicalEmail {
		return RecoveryTarget{}, false, ErrInvalidIdentityInput
	}
	if !isUUID(input.TokenID) || input.TTL <= 0 {
		return RecoveryTarget{}, false, errors.New("recovery token id and TTL are required")
	}
	tokenHash, err := cleanTokenHash(input.TokenHash)
	if err != nil {
		return RecoveryTarget{}, false, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RecoveryTarget{}, false, fmt.Errorf("begin recovery request: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userID, verifiedEmail string
	err = tx.QueryRowContext(ctx, `
SELECT id, email
FROM users
WHERE lower(email) = $1
  AND account_status = 'active'
  AND deleted_at IS NULL
FOR UPDATE
`, email).Scan(&userID, &verifiedEmail)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return RecoveryTarget{}, false, nil
	}
	if err != nil {
		return RecoveryTarget{}, false, fmt.Errorf("lock recovery user: %w", err)
	}
	var credentialUserID string
	err = tx.QueryRowContext(ctx, `
SELECT user_id
FROM user_credentials
WHERE user_id = $1
  AND email_verified_at IS NOT NULL
FOR UPDATE
`, userID).Scan(&credentialUserID)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return RecoveryTarget{}, false, nil
	}
	if err != nil {
		return RecoveryTarget{}, false, fmt.Errorf("lock recovery credential: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE credential_recovery_tokens
SET status = 'revoked',
    revoked_at = now(),
    updated_at = now()
WHERE user_id = $1
  AND status = 'active'
`, userID); err != nil {
		return RecoveryTarget{}, false, fmt.Errorf("revoke previous recovery tokens: %w", err)
	}

	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
INSERT INTO credential_recovery_tokens (
  id,
  user_id,
  token_hash,
  status,
  expires_at
) VALUES ($1, $2, $3, 'active', now() + make_interval(secs => $4))
RETURNING expires_at
`, input.TokenID, userID, tokenHash, input.TTL.Seconds()).Scan(&expiresAt)
	if err != nil {
		return RecoveryTarget{}, false, fmt.Errorf("insert recovery token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RecoveryTarget{}, false, fmt.Errorf("commit recovery request: %w", err)
	}
	return RecoveryTarget{Email: verifiedEmail, ExpiresAt: expiresAt.UTC()}, true, nil
}

func (r *PostgresSessionRepository) CompleteRecovery(
	ctx context.Context,
	input CompleteRecoveryRepositoryInput,
) ([]RevokedSession, error) {
	if r == nil || r.db == nil {
		return nil, ErrDatabaseRequired
	}
	tokenHash, err := cleanTokenHash(input.TokenHash)
	if err != nil || !strings.HasPrefix(input.PasswordHash, "$argon2id$") {
		return nil, ErrInvalidCredential
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin complete recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var tokenID, userID string
	err = tx.QueryRowContext(ctx, `
SELECT id, user_id
FROM credential_recovery_tokens
WHERE token_hash = $1
  AND status = 'active'
  AND expires_at > now()
`, tokenHash).Scan(&tokenID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredential
	}
	if err != nil {
		return nil, fmt.Errorf("lookup active recovery token: %w", err)
	}
	var lockedUserID string
	err = tx.QueryRowContext(ctx, `
SELECT id
FROM users
WHERE id = $1
  AND account_status = 'active'
  AND deleted_at IS NULL
FOR UPDATE
`, userID).Scan(&lockedUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredential
	}
	if err != nil {
		return nil, fmt.Errorf("lock recovery user: %w", err)
	}
	var credentialUserID string
	err = tx.QueryRowContext(ctx, `
SELECT user_id
FROM user_credentials
WHERE user_id = $1
  AND email_verified_at IS NOT NULL
FOR UPDATE
`, userID).Scan(&credentialUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredential
	}
	if err != nil {
		return nil, fmt.Errorf("lock recovery credential: %w", err)
	}

	credentialResult, err := tx.ExecContext(ctx, `
UPDATE user_credentials
SET password_hash = $2,
    credential_revision = credential_revision + 1,
    updated_at = now()
WHERE user_id = $1
`, userID, input.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("update recovered credential: %w", err)
	}
	if rows, rowsErr := credentialResult.RowsAffected(); rowsErr != nil {
		return nil, fmt.Errorf("update recovered credential rows affected: %w", rowsErr)
	} else if rows != 1 {
		return nil, ErrInvalidCredential
	}
	result, err := tx.ExecContext(ctx, `
UPDATE credential_recovery_tokens
SET status = 'used',
    used_at = now(),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND token_hash = $3
  AND status = 'active'
  AND expires_at > now()
`, tokenID, userID, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("consume recovery token: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return nil, ErrInvalidCredential
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE credential_recovery_tokens
SET status = 'revoked',
    revoked_at = now(),
    updated_at = now()
WHERE user_id = $1
  AND status = 'active'
  AND id <> $2
`, userID, tokenID); err != nil {
		return nil, fmt.Errorf("revoke sibling recovery tokens: %w", err)
	}

	revoked, err := revokeSessionsForUser(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit complete recovery: %w", err)
	}
	return revoked, nil
}

func (r *PostgresSessionRepository) RevokeSessionsByUserID(
	ctx context.Context,
	userID string,
) ([]RevokedSession, error) {
	if r == nil || r.db == nil {
		return nil, ErrDatabaseRequired
	}
	if !isUUID(strings.TrimSpace(userID)) {
		return nil, errors.New("user id must be a UUID")
	}
	return revokeSessionsForUser(ctx, r.db, userID)
}

func (r *PostgresSessionRepository) BootstrapIdentity(
	ctx context.Context,
	input BootstrapIdentityInput,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	if !isUUID(strings.TrimSpace(input.UserID)) {
		return errors.New("bootstrap user id must be a UUID")
	}
	email, err := canonicalizeEmail(input.Email)
	if err != nil || email != input.Email {
		return ErrInvalidIdentityInput
	}
	if !strings.HasPrefix(input.PasswordHash, "$argon2id$") {
		return errors.New("bootstrap password hash must be Argon2id")
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin bootstrap identity: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE user_credentials IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock bootstrap credentials: %w", err)
	}

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM user_credentials)`).Scan(&exists); err != nil {
		return fmt.Errorf("check bootstrap identity: %w", err)
	}
	if exists {
		return ErrBootstrapAlreadyCompleted
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status)
VALUES ($1, $2, $3, 'active')
ON CONFLICT (id) DO UPDATE
SET email = EXCLUDED.email,
    display_name = EXCLUDED.display_name,
    account_status = 'active',
    deleted_at = NULL,
    updated_at = now()
`, input.UserID, email, strings.TrimSpace(input.DisplayName)); err != nil {
		return fmt.Errorf("upsert bootstrap user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_credentials (
  user_id,
  password_hash,
  credential_revision,
  email_verified_at
) VALUES ($1, $2, 1, now())
`, input.UserID, input.PasswordHash); err != nil {
		if isUniqueViolation(err) {
			return ErrBootstrapAlreadyCompleted
		}
		return fmt.Errorf("insert bootstrap credential: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now()),
    updated_at = now()
WHERE user_id = $1
  AND revoked_at IS NULL
`, input.UserID); err != nil {
		return fmt.Errorf("revoke pre-credential bootstrap sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		if isSerializationFailure(err) || isUniqueViolation(err) {
			return ErrBootstrapAlreadyCompleted
		}
		return fmt.Errorf("commit bootstrap identity: %w", err)
	}
	return nil
}

type lockedInviteAcceptance struct {
	ID   string
	Role string
}

func lockCredentialSessionUser(ctx context.Context, tx *sql.Tx, userID string) (string, error) {
	var displayName string
	err := tx.QueryRowContext(ctx, `
SELECT COALESCE(display_name, '')
FROM users
WHERE id = $1
  AND account_status = 'active'
  AND deleted_at IS NULL
FOR SHARE
`, userID).Scan(&displayName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidCredential
	}
	if err != nil {
		return "", fmt.Errorf("lock credential session user: %w", err)
	}
	return displayName, nil
}

func lockCredentialSessionRevision(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	credentialRevision int64,
) error {
	var credentialUserID string
	err := tx.QueryRowContext(ctx, `
SELECT user_id
FROM user_credentials
WHERE user_id = $1
  AND credential_revision = $2
  AND email_verified_at IS NOT NULL
FOR SHARE
`, userID, credentialRevision).Scan(&credentialUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidCredential
	}
	if err != nil {
		return fmt.Errorf("lock credential session revision: %w", err)
	}
	return nil
}

func lockInviteAcceptanceCanonicalEmail(ctx context.Context, tx *sql.Tx, canonicalEmail string) error {
	rows, err := tx.QueryContext(ctx, `
SELECT pg_advisory_xact_lock(hashtextextended($1, 0::bigint))
`, canonicalEmail)
	if err != nil {
		return fmt.Errorf("lock invite canonical email fence: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("lock invite canonical email fence: %w", err)
	}
	return nil
}

func lockInviteAcceptanceUser(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	canonicalEmail string,
) (string, error) {
	var displayName string
	err := tx.QueryRowContext(ctx, `
SELECT COALESCE(display_name, '')
FROM users
WHERE id = $1
  AND email = $2
  AND account_status = 'active'
  AND deleted_at IS NULL
FOR UPDATE
`, userID, canonicalEmail).Scan(&displayName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInviteNotActive
	}
	if err != nil {
		return "", fmt.Errorf("lock invite user: %w", err)
	}
	return displayName, nil
}

func lockInviteAcceptanceTeam(ctx context.Context, tx *sql.Tx, teamID string) error {
	var lockedTeamID string
	err := tx.QueryRowContext(ctx, `
SELECT id
FROM teams
WHERE id = $1
  AND deleted_at IS NULL
FOR UPDATE
`, teamID).Scan(&lockedTeamID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInviteNotActive
	}
	if err != nil {
		return fmt.Errorf("lock invite team: %w", err)
	}
	return nil
}

func lockInviteAcceptanceInvite(
	ctx context.Context,
	tx *sql.Tx,
	inviteHash string,
	teamID string,
	canonicalEmail string,
) (lockedInviteAcceptance, error) {
	var invite lockedInviteAcceptance
	err := tx.QueryRowContext(ctx, `
SELECT id, role
FROM team_invites
WHERE token_hash = $1
  AND team_id = $2
  AND email = $3
  AND status = 'pending'
  AND expires_at > now()
FOR UPDATE
`, inviteHash, teamID, canonicalEmail).Scan(&invite.ID, &invite.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return lockedInviteAcceptance{}, ErrInviteNotActive
	}
	if err != nil {
		return lockedInviteAcceptance{}, fmt.Errorf("lock invite row: %w", err)
	}
	return invite, nil
}

func lockInviteAcceptanceMailOutbox(ctx context.Context, tx *sql.Tx, inviteID string) error {
	var lockedInviteID string
	err := tx.QueryRowContext(ctx, `
SELECT invite_id
FROM identity_mail_outbox
WHERE invite_id = $1
  AND status = 'sent'
FOR UPDATE
`, inviteID).Scan(&lockedInviteID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInviteNotActive
	}
	if err != nil {
		return fmt.Errorf("lock invite mail outbox: %w", err)
	}
	return nil
}

func lockInviteAcceptanceCredential(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	credentialRevision int64,
) error {
	var credentialUserID string
	err := tx.QueryRowContext(ctx, `
SELECT user_id
FROM user_credentials
WHERE user_id = $1
  AND credential_revision = $2
  AND email_verified_at IS NOT NULL
FOR UPDATE
`, userID, credentialRevision).Scan(&credentialUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInviteNotActive
	}
	if err != nil {
		return fmt.Errorf("lock invite credential: %w", err)
	}
	return nil
}

func insertInviteAcceptanceUser(ctx context.Context, tx *sql.Tx, userID string, canonicalEmail string) error {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status)
VALUES ($1, $2, '', 'active')
`, userID, canonicalEmail); err != nil {
		if isUniqueViolation(err) {
			return ErrInviteNotActive
		}
		return fmt.Errorf("insert invited user: %w", err)
	}
	return nil
}

func insertInviteAcceptanceCredential(ctx context.Context, tx *sql.Tx, userID string, passwordHash string) error {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_credentials (
  user_id,
  password_hash,
  credential_revision,
  email_verified_at
) VALUES ($1, $2, 1, now())
`, userID, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return ErrInviteNotActive
		}
		return fmt.Errorf("insert invited credential: %w", err)
	}
	return nil
}

func upsertInviteAcceptanceMembership(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	userID string,
	role string,
) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx, `
SELECT status
FROM team_memberships
WHERE team_id = $1
  AND user_id = $2
FOR UPDATE
`, teamID, userID).Scan(&status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `
INSERT INTO team_memberships (team_id, user_id, role, status)
VALUES ($1, $2, $3, 'active')
`, teamID, userID, role); err != nil {
			if isUniqueViolation(err) {
				return "", ErrInviteNotActive
			}
			return "", fmt.Errorf("insert invited membership: %w", err)
		}
		return "added", nil
	case err != nil:
		return "", fmt.Errorf("lock invite membership: %w", err)
	case status == "active":
		return "", ErrInviteNotActive
	case status != "removed":
		return "", ErrInviteNotActive
	}
	result, err := tx.ExecContext(ctx, `
UPDATE team_memberships
SET role = $3,
    status = 'active',
    removed_at = NULL,
    updated_at = now()
WHERE team_id = $1
  AND user_id = $2
  AND status = 'removed'
`, teamID, userID, role)
	if err != nil {
		return "", fmt.Errorf("reactivate invited membership: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return "", ErrInviteNotActive
	}
	return "reactivated", nil
}

func advanceInviteAcceptanceMembershipRevision(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `
UPDATE teams
SET membership_revision = membership_revision + 1,
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING membership_revision
`, teamID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrInviteNotActive
	}
	if err != nil {
		return 0, fmt.Errorf("advance membership revision: %w", err)
	}
	return revision, nil
}

func insertInviteAcceptanceMembershipOutbox(
	ctx context.Context,
	tx *sql.Tx,
	teamID string,
	userID string,
	role string,
	operation string,
	revision int64,
) error {
	eventID, err := newUUID()
	if err != nil {
		return fmt.Errorf("generate membership outbox event id: %w", err)
	}
	payload, err := json.Marshal(map[string]any{
		"teamId":             teamID,
		"userId":             userID,
		"operation":          operation,
		"teamRole":           role,
		"status":             "active",
		"membershipRevision": revision,
	})
	if err != nil {
		return fmt.Errorf("marshal membership outbox payload: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO knowledge_outbox (
  event_id,
  aggregate_type,
  aggregate_key,
  event_type,
  payload
) VALUES ($1, 'team', $2, 'team.membership.changed', $3::jsonb)
`, eventID, teamID, string(payload)); err != nil {
		return fmt.Errorf("insert membership outbox: %w", err)
	}
	return nil
}

func consumeInviteAcceptanceInvite(ctx context.Context, tx *sql.Tx, inviteID string) error {
	result, err := tx.ExecContext(ctx, `
UPDATE team_invites
SET status = 'accepted',
    accepted_at = now(),
    updated_at = now()
WHERE id = $1
  AND status = 'pending'
`, inviteID)
	if err != nil {
		return fmt.Errorf("consume invite: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return ErrInviteNotActive
	}
	return nil
}

type insertSessionInput struct {
	SessionID   string
	UserID      string
	DisplayName string
	TokenHash   string
	UserAgent   string
	ExpiresAt   time.Time
}

func insertSession(ctx context.Context, db queryRower, input insertSessionInput) (Session, error) {
	session, err := scanSession(db.QueryRowContext(ctx, `
INSERT INTO sessions (
  id,
  user_id,
  token_hash,
  user_agent,
  expires_at
) VALUES ($1, $2, $3, NULLIF($4, ''), $5)
RETURNING
  id,
  user_id,
  $6::text AS display_name,
  $7::text AS role,
  expires_at,
  revoked_at,
  created_at,
  updated_at
`,
		input.SessionID,
		input.UserID,
		input.TokenHash,
		strings.TrimSpace(input.UserAgent),
		input.ExpiresAt.UTC(),
		input.DisplayName,
		defaultUserRole,
	))
	if err != nil {
		return Session{}, fmt.Errorf("insert auth session: %w", err)
	}
	return session, nil
}

func revokeSessionsForUser(
	ctx context.Context,
	db queryExecer,
	userID string,
) ([]RevokedSession, error) {
	rows, err := db.QueryContext(ctx, `
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now()),
    updated_at = now()
WHERE user_id = $1
  AND revoked_at IS NULL
RETURNING id, token_hash
`, userID)
	if err != nil {
		return nil, fmt.Errorf("revoke user sessions: %w", err)
	}
	defer rows.Close()

	var revoked []RevokedSession
	for rows.Next() {
		var session RevokedSession
		if err := rows.Scan(&session.ID, &session.TokenHash); err != nil {
			return nil, fmt.Errorf("scan revoked session: %w", err)
		}
		revoked = append(revoked, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate revoked sessions: %w", err)
	}
	return revoked, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type queryExecer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
