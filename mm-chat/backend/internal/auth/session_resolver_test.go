package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/sessioncache"
)

func TestSessionResolverUsesCacheHitWithoutRepository(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	cache := newFakeSessionCache()
	cache.snapshot = sessioncache.Snapshot{
		SessionID:   "session-1",
		UserID:      "user-1",
		DisplayName: "User One",
		Role:        "user",
		ExpiresAt:   now.Add(time.Hour),
	}
	cache.hit = true
	repo := &fakeSessionRepository{
		session: Session{ID: "repo-session", ExpiresAt: now.Add(time.Hour)},
	}
	resolver := NewSessionResolver(repo, WithSessionCache(cache), WithClock(func() time.Time { return now }))

	session, err := resolver.ResolveByTokenHash(context.Background(), "token-hash")
	if err != nil {
		t.Fatalf("ResolveByTokenHash() error = %v", err)
	}
	if session.ID != "session-1" || session.UserID != "user-1" || session.DisplayName != "User One" {
		t.Fatalf("ResolveByTokenHash() session = %#v", session)
	}
	if repo.calls != 0 {
		t.Fatalf("repo calls = %d, want 0 on cache hit", repo.calls)
	}
}

func TestSessionResolverCachesRepositoryLookupOnMiss(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	cache := newFakeSessionCache()
	repo := &fakeSessionRepository{
		session: Session{
			ID:          "session-1",
			UserID:      "user-1",
			DisplayName: "User One",
			Role:        "user",
			ExpiresAt:   now.Add(time.Hour),
		},
	}
	resolver := NewSessionResolver(repo, WithSessionCache(cache), WithClock(func() time.Time { return now }))

	session, err := resolver.ResolveByTokenHash(context.Background(), " token-hash ")
	if err != nil {
		t.Fatalf("ResolveByTokenHash() error = %v", err)
	}
	if session.ID != repo.session.ID {
		t.Fatalf("ResolveByTokenHash() session = %#v", session)
	}
	if repo.calls != 1 {
		t.Fatalf("repo calls = %d, want 1", repo.calls)
	}
	if cache.cacheCalls != 1 || cache.cachedTokenHash != "token-hash" {
		t.Fatalf("cache calls/hash = %d/%q", cache.cacheCalls, cache.cachedTokenHash)
	}
	if cache.cachedSnapshot.SessionID != repo.session.ID || cache.cachedSnapshot.UserID != repo.session.UserID {
		t.Fatalf("cached snapshot = %#v", cache.cachedSnapshot)
	}
}

func TestSessionResolverFallsBackToRepositoryOnCacheErrors(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	cache := newFakeSessionCache()
	cache.lookupErr = errors.New("redis down")
	cache.cacheErr = errors.New("redis write down")
	repo := &fakeSessionRepository{
		session: Session{ID: "session-1", UserID: "user-1", ExpiresAt: now.Add(time.Hour)},
	}
	resolver := NewSessionResolver(repo, WithSessionCache(cache), WithClock(func() time.Time { return now }))

	session, err := resolver.ResolveByTokenHash(context.Background(), "token-hash")
	if err != nil {
		t.Fatalf("ResolveByTokenHash() error = %v", err)
	}
	if session.ID != "session-1" {
		t.Fatalf("ResolveByTokenHash() session = %#v", session)
	}
	if repo.calls != 1 || cache.cacheCalls != 1 {
		t.Fatalf("repo/cache calls = %d/%d, want 1/1", repo.calls, cache.cacheCalls)
	}
}

func TestSessionResolverFallsBackToRepositoryWhenCachedSessionIsExpired(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	cache := newFakeSessionCache()
	cache.hit = true
	cache.snapshot = sessioncache.Snapshot{
		SessionID: "cached-session",
		UserID:    "user-1",
		ExpiresAt: now.Add(-time.Second),
	}
	repo := &fakeSessionRepository{
		session: Session{ID: "session-1", UserID: "user-1", ExpiresAt: now.Add(time.Hour)},
	}
	resolver := NewSessionResolver(repo, WithSessionCache(cache), WithClock(func() time.Time { return now }))

	session, err := resolver.ResolveByTokenHash(context.Background(), "token-hash")
	if err != nil {
		t.Fatalf("ResolveByTokenHash() error = %v", err)
	}
	if session.ID != "session-1" {
		t.Fatalf("ResolveByTokenHash() session = %#v", session)
	}
	if cache.deleteCalls != 1 || repo.calls != 1 {
		t.Fatalf("delete/repo calls = %d/%d, want 1/1", cache.deleteCalls, repo.calls)
	}
}

func TestSessionResolverFailsClosedOnRepositoryError(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	repoErr := errors.New("database down")
	resolver := NewSessionResolver(
		&fakeSessionRepository{err: repoErr},
		WithSessionCache(newFakeSessionCache()),
		WithClock(func() time.Time { return now }),
	)

	_, err := resolver.ResolveByTokenHash(context.Background(), "token-hash")
	if !errors.Is(err, repoErr) {
		t.Fatalf("ResolveByTokenHash() error = %v, want repo error", err)
	}
}

func TestSessionResolverRequiresRepository(t *testing.T) {
	resolver := NewSessionResolver(nil)

	_, err := resolver.ResolveByTokenHash(context.Background(), "token-hash")
	if !errors.Is(err, ErrDatabaseRequired) {
		t.Fatalf("ResolveByTokenHash() error = %v, want ErrDatabaseRequired", err)
	}
}

func TestSessionResolverVerifiesRevocationHintAgainstRepository(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	cache := newFakeSessionCache()
	cache.hit = true
	cache.snapshot = sessioncache.Snapshot{SessionID: "session-1", UserID: "user-1", ExpiresAt: now.Add(time.Hour)}
	cache.revoked = true
	repo := &fakeSessionRepository{
		session: Session{ID: "session-1", UserID: "user-1", ExpiresAt: now.Add(time.Hour)},
	}
	resolver := NewSessionResolver(
		repo,
		WithSessionCache(cache),
		WithClock(func() time.Time { return now }),
	)

	session, err := resolver.ResolveByTokenHash(context.Background(), "token-hash")
	if err != nil {
		t.Fatalf("ResolveByTokenHash() error = %v", err)
	}
	if session.ID != "session-1" {
		t.Fatalf("ResolveByTokenHash() session = %#v", session)
	}
	if repo.calls != 1 || cache.deleteCalls != 1 || cache.clearRevokedCalls != 1 {
		t.Fatalf(
			"repo/delete/clear calls = %d/%d/%d, want 1/1/1",
			repo.calls,
			cache.deleteCalls,
			cache.clearRevokedCalls,
		)
	}
}

func TestSessionResolverDoesNotCacheExpiredOrRevokedRepositorySessions(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	revokedAt := now.Add(-time.Second)
	tests := []struct {
		name    string
		session Session
		wantErr error
	}{
		{
			name:    "expired",
			session: Session{ID: "session-1", UserID: "user-1", ExpiresAt: now.Add(-time.Second)},
			wantErr: ErrSessionExpired,
		},
		{
			name:    "revoked",
			session: Session{ID: "session-2", UserID: "user-1", ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt},
			wantErr: ErrSessionRevoked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newFakeSessionCache()
			resolver := NewSessionResolver(
				&fakeSessionRepository{session: tt.session},
				WithSessionCache(cache),
				WithClock(func() time.Time { return now }),
			)

			_, err := resolver.ResolveByTokenHash(context.Background(), "token-hash")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ResolveByTokenHash() error = %v, want %v", err, tt.wantErr)
			}
			if cache.cacheCalls != 0 {
				t.Fatalf("cache calls = %d, want 0", cache.cacheCalls)
			}
			if tt.wantErr == ErrSessionRevoked && cache.markRevokedCalls != 1 {
				t.Fatalf("mark revoked calls = %d, want 1", cache.markRevokedCalls)
			}
		})
	}
}

func TestSessionResolverRejectsBlankOrWhitespaceTokenHash(t *testing.T) {
	resolver := NewSessionResolver(&fakeSessionRepository{})
	for _, tokenHash := range []string{"", " ", "token hash"} {
		_, err := resolver.ResolveByTokenHash(context.Background(), tokenHash)
		if err == nil {
			t.Fatalf("ResolveByTokenHash(%q) error = nil, want validation error", tokenHash)
		}
	}
}

type fakeSessionRepository struct {
	session Session
	err     error
	calls   int
}

func (f *fakeSessionRepository) LookupSessionByTokenHash(context.Context, string) (Session, error) {
	f.calls++
	if f.err != nil {
		return Session{}, f.err
	}
	return f.session, nil
}

type fakeSessionCache struct {
	snapshot          sessioncache.Snapshot
	hit               bool
	lookupErr         error
	cacheErr          error
	revoked           bool
	cacheCalls        int
	cachedTokenHash   string
	cachedSnapshot    sessioncache.Snapshot
	deleteCalls       int
	markRevokedCalls  int
	clearRevokedCalls int
}

func newFakeSessionCache() *fakeSessionCache {
	return &fakeSessionCache{}
}

func (f *fakeSessionCache) CacheSession(_ context.Context, tokenHash string, snapshot sessioncache.Snapshot) error {
	f.cacheCalls++
	f.cachedTokenHash = tokenHash
	f.cachedSnapshot = snapshot
	return f.cacheErr
}

func (f *fakeSessionCache) LookupSession(context.Context, string) (sessioncache.Snapshot, bool, error) {
	if f.lookupErr != nil {
		return sessioncache.Snapshot{}, false, f.lookupErr
	}
	return f.snapshot, f.hit, nil
}

func (f *fakeSessionCache) DeleteSession(context.Context, string) error {
	f.deleteCalls++
	return nil
}

func (f *fakeSessionCache) MarkSessionRevoked(context.Context, string) error {
	f.markRevokedCalls++
	return nil
}

func (f *fakeSessionCache) IsSessionRevoked(context.Context, string) (bool, error) {
	return f.revoked, nil
}

func (f *fakeSessionCache) ClearSessionRevoked(context.Context, string) error {
	f.clearRevokedCalls++
	return nil
}
