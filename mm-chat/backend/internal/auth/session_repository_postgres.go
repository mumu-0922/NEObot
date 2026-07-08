package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type PostgresSessionRepository struct {
	db *sql.DB
}

func NewPostgresSessionRepository(db *sql.DB) *PostgresSessionRepository {
	return &PostgresSessionRepository{db: db}
}

func (r *PostgresSessionRepository) LookupSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	if r == nil || r.db == nil {
		return Session{}, errors.New("database is required")
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
