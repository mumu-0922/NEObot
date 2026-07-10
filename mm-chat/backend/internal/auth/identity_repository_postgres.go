package auth

import (
	"context"
	"database/sql"
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
	err = tx.QueryRowContext(ctx, `
SELECT COALESCE(u.display_name, '')
FROM users u
JOIN user_credentials c ON c.user_id = u.id
WHERE u.id = $1
  AND u.account_status = 'active'
  AND u.deleted_at IS NULL
  AND c.credential_revision = $2
  AND c.email_verified_at IS NOT NULL
FOR SHARE OF u, c
`, input.UserID, input.CredentialRevision).Scan(&displayName)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvalidCredential
	}
	if err != nil {
		return Session{}, fmt.Errorf("lock credential for session: %w", err)
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

func (r *PostgresSessionRepository) AcceptInvite(
	ctx context.Context,
	input AcceptInviteRepositoryInput,
) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, ErrDatabaseRequired
	}
	if !isUUID(input.UserID) || !isUUID(input.SessionID) {
		return Session{}, errors.New("invite user and session ids must be UUIDs")
	}
	inviteHash, err := cleanTokenHash(input.InviteTokenHash)
	if err != nil {
		return Session{}, ErrInviteNotActive
	}
	sessionHash, err := cleanTokenHash(input.SessionTokenHash)
	if err != nil {
		return Session{}, err
	}
	if !strings.HasPrefix(input.PasswordHash, "$argon2id$") {
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

	var teamID, email, role string
	err = tx.QueryRowContext(ctx, `
SELECT i.team_id, i.email, i.role
FROM team_invites i
JOIN teams t ON t.id = i.team_id
WHERE i.token_hash = $1
  AND i.status = 'pending'
  AND i.expires_at > now()
  AND t.deleted_at IS NULL
FOR UPDATE OF i, t
`, inviteHash).Scan(&teamID, &email, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInviteNotActive
	}
	if err != nil {
		return Session{}, fmt.Errorf("lock active invite: %w", err)
	}

	var identityExists bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM users
  WHERE lower(email) = lower($1)
)
`, email).Scan(&identityExists); err != nil {
		return Session{}, fmt.Errorf("check invite identity: %w", err)
	}
	if identityExists {
		return Session{}, ErrInviteNotActive
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO users (id, email, display_name, account_status)
VALUES ($1, lower(trim($2)), '', 'active')
`, input.UserID, email); err != nil {
		if isUniqueViolation(err) {
			return Session{}, ErrInviteNotActive
		}
		return Session{}, fmt.Errorf("insert invited user: %w", err)
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
			return Session{}, ErrInviteNotActive
		}
		return Session{}, fmt.Errorf("insert invited credential: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO team_memberships (team_id, user_id, role, status)
VALUES ($1, $2, $3, 'active')
`, teamID, input.UserID, role); err != nil {
		if isUniqueViolation(err) {
			return Session{}, ErrInviteNotActive
		}
		return Session{}, fmt.Errorf("insert invited membership: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE teams
SET membership_revision = membership_revision + 1,
    updated_at = now()
WHERE id = $1
  AND deleted_at IS NULL
`, teamID); err != nil {
		return Session{}, fmt.Errorf("advance membership revision: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE team_invites
SET status = 'accepted',
    accepted_at = now(),
    updated_at = now()
WHERE token_hash = $1
  AND status = 'pending'
`, inviteHash)
	if err != nil {
		return Session{}, fmt.Errorf("consume invite: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return Session{}, ErrInviteNotActive
	}

	session, err := insertSession(ctx, tx, insertSessionInput{
		SessionID:   input.SessionID,
		UserID:      input.UserID,
		DisplayName: "",
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
SELECT u.id, u.email
FROM users u
JOIN user_credentials c ON c.user_id = u.id
WHERE lower(u.email) = $1
  AND u.account_status = 'active'
  AND u.deleted_at IS NULL
  AND c.email_verified_at IS NOT NULL
FOR UPDATE OF u, c
`, email).Scan(&userID, &verifiedEmail)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return RecoveryTarget{}, false, nil
	}
	if err != nil {
		return RecoveryTarget{}, false, fmt.Errorf("lock recovery identity: %w", err)
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
SELECT t.id, t.user_id
FROM credential_recovery_tokens t
JOIN users u ON u.id = t.user_id
JOIN user_credentials c ON c.user_id = t.user_id
WHERE t.token_hash = $1
  AND t.status = 'active'
  AND t.expires_at > now()
  AND u.account_status = 'active'
  AND u.deleted_at IS NULL
  AND c.email_verified_at IS NOT NULL
FOR UPDATE OF t, u, c
`, tokenHash).Scan(&tokenID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredential
	}
	if err != nil {
		return nil, fmt.Errorf("lock active recovery token: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE user_credentials
SET password_hash = $2,
    credential_revision = credential_revision + 1,
    updated_at = now()
WHERE user_id = $1
`, userID, input.PasswordHash); err != nil {
		return nil, fmt.Errorf("update recovered credential: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE credential_recovery_tokens
SET status = 'used',
    used_at = now(),
    updated_at = now()
WHERE id = $1
`, tokenID); err != nil {
		return nil, fmt.Errorf("consume recovery token: %w", err)
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
