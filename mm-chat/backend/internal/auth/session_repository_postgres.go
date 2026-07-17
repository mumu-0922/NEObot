package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const developmentSessionUserAgent = "mm-chat-development-internal"

type PostgresSessionRepository struct {
	db *sql.DB
}

func NewPostgresSessionRepository(db *sql.DB) *PostgresSessionRepository {
	return &PostgresSessionRepository{db: db}
}

// EnsureDevelopmentSession creates the database identity used by the fixed
// single-user development owner. The random token hash has no corresponding
// browser token and is rotated on every process start, so it cannot be used as
// an authentication credential.
func (r *PostgresSessionRepository) EnsureDevelopmentSession(
	ctx context.Context,
	expiresAt time.Time,
) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, ErrDatabaseRequired
	}
	if !expiresAt.After(timeNow()) {
		return Session{}, errors.New("development session expiry must be in the future")
	}
	randomHash := make([]byte, 32)
	if _, err := rand.Read(randomHash); err != nil {
		return Session{}, fmt.Errorf("generate development session token hash: %w", err)
	}

	session, err := scanSession(r.db.QueryRowContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, user_agent, expires_at)
SELECT $1, u.id, $2, $3, $4
FROM users u
WHERE u.id = $5
  AND u.account_status = 'active'
  AND u.deleted_at IS NULL
ON CONFLICT (id) DO UPDATE SET
  user_id = EXCLUDED.user_id,
  token_hash = EXCLUDED.token_hash,
  user_agent = EXCLUDED.user_agent,
  expires_at = EXCLUDED.expires_at,
  revoked_at = NULL,
  updated_at = now()
RETURNING
  id,
  user_id,
  $6::text AS display_name,
  $7::text AS role,
  expires_at,
  revoked_at,
  created_at,
  updated_at
`, DevelopmentSessionID, hex.EncodeToString(randomHash), developmentSessionUserAgent,
		expiresAt.UTC(), DevelopmentUserID, DevelopmentDisplayName, defaultUserRole))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvalidCredential
	}
	if err != nil {
		return Session{}, fmt.Errorf("ensure development session: %w", err)
	}
	return session, nil
}

func (r *PostgresSessionRepository) LookupSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, ErrDatabaseRequired
	}
	tokenHash, err := cleanTokenHash(tokenHash)
	if err != nil {
		return Session{}, err
	}

	row := r.db.QueryRowContext(ctx, `
SELECT
  s.id,
  s.user_id,
  COALESCE(u.display_name, '') AS display_name,
  $2::text AS role,
  s.expires_at,
  s.revoked_at,
  s.created_at,
  s.updated_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND u.deleted_at IS NULL
  AND u.account_status = 'active'
`, tokenHash, defaultUserRole)

	var session Session
	if err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.DisplayName,
		&session.Role,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("lookup session by token hash: %w", err)
	}

	return session, nil
}

func (r *PostgresSessionRepository) CreateSession(ctx context.Context, input CreateSessionInput) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, ErrDatabaseRequired
	}
	input.SessionID = strings.TrimSpace(input.SessionID)
	if !isUUID(input.SessionID) {
		return Session{}, errors.New("session id must be a UUID")
	}
	input.UserID = strings.TrimSpace(input.UserID)
	if !isUUID(input.UserID) {
		return Session{}, errors.New("user id must be a UUID")
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	tokenHash, err := cleanTokenHash(input.TokenHash)
	if err != nil {
		return Session{}, err
	}
	if !input.ExpiresAt.After(timeNow()) {
		return Session{}, errors.New("session expiry must be in the future")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, fmt.Errorf("begin create session: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(display_name, '')
FROM users
WHERE id = $1
  AND account_status = 'active'
  AND deleted_at IS NULL
FOR SHARE
`, input.UserID).Scan(&input.DisplayName); errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrInvalidCredential
	} else if err != nil {
		return Session{}, fmt.Errorf("lock auth user: %w", err)
	}

	session, err := scanSession(tx.QueryRowContext(ctx, `
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
`, input.SessionID, input.UserID, tokenHash, strings.TrimSpace(input.UserAgent), input.ExpiresAt.UTC(), input.DisplayName, defaultUserRole))
	if err != nil {
		return Session{}, fmt.Errorf("insert auth session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Session{}, fmt.Errorf("commit create session: %w", err)
	}
	return session, nil
}

func (r *PostgresSessionRepository) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, ErrDatabaseRequired
	}
	tokenHash, err := cleanTokenHash(tokenHash)
	if err != nil {
		return Session{}, err
	}

	session, err := scanSession(r.db.QueryRowContext(ctx, `
UPDATE sessions s
SET revoked_at = COALESCE(s.revoked_at, now()),
    updated_at = now()
FROM users u
WHERE s.user_id = u.id
  AND s.token_hash = $1
  AND u.deleted_at IS NULL
  AND u.account_status = 'active'
RETURNING
  s.id,
  s.user_id,
  COALESCE(u.display_name, '') AS display_name,
  $2::text AS role,
  s.expires_at,
  s.revoked_at,
  s.created_at,
  s.updated_at
`, tokenHash, defaultUserRole))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("revoke session by token hash: %w", err)
	}
	return session, nil
}

func scanSession(row rowScanner) (Session, error) {
	var session Session
	if err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.DisplayName,
		&session.Role,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	); err != nil {
		return Session{}, err
	}
	return session, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func timeNow() time.Time {
	return time.Now().UTC()
}
