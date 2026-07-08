package sessioncache

import (
	"context"
	"time"
)

// Snapshot is the minimal, browser-safe session data allowed in Redis. It must
// never include raw bearer tokens, token hashes, provider secrets, or request
// metadata.
type Snapshot struct {
	SessionID   string    `json:"sessionId"`
	UserID      string    `json:"userId"`
	DisplayName string    `json:"displayName,omitempty"`
	Role        string    `json:"role,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// Store caches session lookups and short-lived revocation hints. Postgres is
// still authoritative for whether a session exists, is expired, or is revoked.
type Store interface {
	CacheSession(ctx context.Context, tokenHash string, snapshot Snapshot) error
	LookupSession(ctx context.Context, tokenHash string) (Snapshot, bool, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	MarkSessionRevoked(ctx context.Context, sessionID string) error
	IsSessionRevoked(ctx context.Context, sessionID string) (bool, error)
	ClearSessionRevoked(ctx context.Context, sessionID string) error
}
