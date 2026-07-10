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

// ResolveByTokenHash always rechecks Postgres before authorizing a request.
// Redis is populated only as a non-authoritative snapshot and revocation hint;
// a positive cache hit cannot bypass a password recovery, account disable, or
// revoke-all transaction.
func (r *SessionResolver) ResolveByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	tokenHash, err := cleanTokenHash(tokenHash)
	if err != nil {
		return Session{}, err
	}
	if r == nil || r.repo == nil {
		return Session{}, ErrDatabaseRequired
	}

	session, err := r.repo.LookupSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if r.cache != nil {
			_ = r.cache.DeleteSession(ctx, tokenHash)
		}
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
