package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/sessioncache"
)

// SessionRepository reads canonical session state from Postgres.
type SessionRepository interface {
	LookupSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error)
}

type SessionResolver struct {
	repo  SessionRepository
	cache sessioncache.Store
	now   func() time.Time
}

type ResolverOption func(*SessionResolver)

func WithSessionCache(cache sessioncache.Store) ResolverOption {
	return func(resolver *SessionResolver) {
		resolver.cache = cache
	}
}

func WithClock(now func() time.Time) ResolverOption {
	return func(resolver *SessionResolver) {
		resolver.now = now
	}
}

func NewSessionResolver(repo SessionRepository, opts ...ResolverOption) *SessionResolver {
	resolver := &SessionResolver{
		repo: repo,
		now:  time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(resolver)
		}
	}
	if resolver.now == nil {
		resolver.now = time.Now
	}

	return resolver
}

// ResolveByTokenHash checks Redis first, then falls back to the canonical
// repository on cache miss or Redis error. Repository errors fail closed.
func (r *SessionResolver) ResolveByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	tokenHash, err := cleanTokenHash(tokenHash)
	if err != nil {
		return Session{}, err
	}
	if r == nil || r.repo == nil {
		return Session{}, ErrDatabaseRequired
	}

	if r.cache != nil {
		snapshot, ok, cacheErr := r.cache.LookupSession(ctx, tokenHash)
		if cacheErr == nil && ok {
			revoked, revokeErr := r.cache.IsSessionRevoked(ctx, snapshot.SessionID)
			if revokeErr == nil && revoked {
				_ = r.cache.DeleteSession(ctx, tokenHash)
			} else if revokeErr == nil {
				session := sessionFromSnapshot(snapshot)
				if err := r.validateActiveSession(session); err == nil {
					return session, nil
				}
				_ = r.cache.DeleteSession(ctx, tokenHash)
			}
		}
	}

	session, err := r.repo.LookupSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return Session{}, err
	}
	if err := r.validateActiveSession(session); err != nil {
		if r.cache != nil {
			_ = r.cache.DeleteSession(ctx, tokenHash)
			if errors.Is(err, ErrSessionRevoked) {
				_ = r.cache.MarkSessionRevoked(ctx, session.ID)
			}
		}
		return Session{}, err
	}

	if r.cache != nil {
		_ = r.cache.ClearSessionRevoked(ctx, session.ID)
		_ = r.cache.CacheSession(ctx, tokenHash, session.Snapshot())
	}

	return session, nil
}

func (r *SessionResolver) validateActiveSession(session Session) error {
	if session.RevokedAt != nil {
		return ErrSessionRevoked
	}
	if !session.ExpiresAt.After(r.clock()) {
		return ErrSessionExpired
	}

	return nil
}

func (r *SessionResolver) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now()
	}

	return time.Now()
}

func cleanTokenHash(tokenHash string) (string, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return "", errors.New("token hash is required")
	}
	if strings.ContainsAny(tokenHash, " \t\r\n") {
		return "", errors.New("token hash must not contain whitespace")
	}

	return tokenHash, nil
}
