package auth

import (
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/sessioncache"
)

const defaultUserRole = "user"

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrSessionRevoked  = errors.New("session revoked")
)

// Session is the canonical application view of a Postgres session row joined to
// browser-safe user metadata. It never contains raw bearer tokens or token
// hashes.
type Session struct {
	ID          string
	UserID      string
	DisplayName string
	Role        string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (s Session) Snapshot() sessioncache.Snapshot {
	role := s.Role
	if role == "" {
		role = defaultUserRole
	}

	return sessioncache.Snapshot{
		SessionID:   s.ID,
		UserID:      s.UserID,
		DisplayName: s.DisplayName,
		Role:        role,
		ExpiresAt:   s.ExpiresAt,
	}
}

func sessionFromSnapshot(snapshot sessioncache.Snapshot) Session {
	role := snapshot.Role
	if role == "" {
		role = defaultUserRole
	}

	return Session{
		ID:          snapshot.SessionID,
		UserID:      snapshot.UserID,
		DisplayName: snapshot.DisplayName,
		Role:        role,
		ExpiresAt:   snapshot.ExpiresAt,
	}
}
