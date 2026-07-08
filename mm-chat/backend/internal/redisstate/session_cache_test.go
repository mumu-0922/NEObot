package redisstate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/sessioncache"
)

func TestSessionCacheKeyValidation(t *testing.T) {
	store := &SessionCacheStore{
		rdb:       redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}),
		keyPrefix: "test",
		ttl:       time.Minute,
	}
	t.Cleanup(func() {
		_ = store.rdb.Close()
	})

	key, err := store.sessionTokenKey(" token-hash-1 ")
	if err != nil {
		t.Fatalf("sessionTokenKey() error = %v", err)
	}
	if !strings.HasPrefix(key, "test:sessions:token:") {
		t.Fatalf("sessionTokenKey() = %q, want session-token key", key)
	}
	if strings.Contains(key, "token-hash-1") {
		t.Fatalf("sessionTokenKey() = %q, must not contain raw token hash", key)
	}
	if _, err := store.sessionTokenKey("token hash"); err == nil {
		t.Fatal("sessionTokenKey() error = nil, want whitespace error")
	}
	if _, err := store.sessionTokenKey(" "); err == nil {
		t.Fatal("sessionTokenKey() error = nil, want required error")
	}

	revokedKey, err := store.sessionRevokedKey(" session-1 ")
	if err != nil {
		t.Fatalf("sessionRevokedKey() error = %v", err)
	}
	if revokedKey != "test:sessions:session-1:revoked" {
		t.Fatalf("sessionRevokedKey() = %q", revokedKey)
	}
	if _, err := store.sessionRevokedKey("session 1"); err == nil {
		t.Fatal("sessionRevokedKey() error = nil, want whitespace error")
	}
}

func TestSessionCacheTTLUsesSoonerOfCacheTTLAndSessionExpiry(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	store := &SessionCacheStore{
		ttl: 5 * time.Minute,
		now: func() time.Time { return now },
	}

	shortTTL := store.cacheTTL(now.Add(time.Minute))
	if shortTTL != time.Minute {
		t.Fatalf("cacheTTL(short expiry) = %s, want 1m", shortTTL)
	}
	longTTL := store.cacheTTL(now.Add(30 * time.Minute))
	if longTTL != 5*time.Minute {
		t.Fatalf("cacheTTL(long expiry) = %s, want 5m", longTTL)
	}
	expiredTTL := store.cacheTTL(now.Add(-time.Second))
	if expiredTTL != 0 {
		t.Fatalf("cacheTTL(expired) = %s, want 0", expiredTTL)
	}
}

func TestSessionCacheRejectsInvalidSnapshots(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	store := &SessionCacheStore{
		rdb:       redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}),
		keyPrefix: "test",
		ttl:       time.Minute,
		now:       func() time.Time { return now },
	}
	t.Cleanup(func() {
		_ = store.rdb.Close()
	})

	valid := sessioncache.Snapshot{
		SessionID:   "session-1",
		UserID:      "user-1",
		DisplayName: "User One",
		Role:        "user",
		ExpiresAt:   now.Add(time.Hour),
	}

	tests := []struct {
		name     string
		snapshot sessioncache.Snapshot
	}{
		{name: "missing session", snapshot: sessioncache.Snapshot{UserID: valid.UserID, ExpiresAt: valid.ExpiresAt}},
		{name: "missing user", snapshot: sessioncache.Snapshot{SessionID: valid.SessionID, ExpiresAt: valid.ExpiresAt}},
		{name: "missing expiry", snapshot: sessioncache.Snapshot{SessionID: valid.SessionID, UserID: valid.UserID}},
		{name: "expired", snapshot: sessioncache.Snapshot{SessionID: valid.SessionID, UserID: valid.UserID, ExpiresAt: now.Add(-time.Second)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.CacheSession(context.Background(), "token-hash", tt.snapshot); err == nil {
				t.Fatal("CacheSession() error = nil, want validation error")
			}
		})
	}
}

func TestSessionCacheStoreIntegration(t *testing.T) {
	redisURL := testRedisURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Open(ctx, config.RedisConfig{
		URL:       redisURL,
		KeyPrefix: "mm-chat-session-test",
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	store := client.SessionCacheStore(2 * time.Minute)
	tokenHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	snapshot := sessioncache.Snapshot{
		SessionID:   "11111111-1111-4111-8111-111111111111",
		UserID:      "22222222-2222-4222-8222-222222222222",
		DisplayName: "Integration User",
		Role:        "user",
		ExpiresAt:   time.Now().Add(time.Hour).UTC(),
	}

	tokenKey, err := store.sessionTokenKey(tokenHash)
	if err != nil {
		t.Fatalf("sessionTokenKey() error = %v", err)
	}
	if err := client.rdb.Del(ctx, tokenKey).Err(); err != nil {
		t.Fatalf("Del() token before test error = %v", err)
	}
	if err := store.ClearSessionRevoked(ctx, snapshot.SessionID); err != nil {
		t.Fatalf("ClearSessionRevoked() before test error = %v", err)
	}

	found, ok, err := store.LookupSession(ctx, tokenHash)
	if err != nil {
		t.Fatalf("LookupSession() before cache error = %v", err)
	}
	if ok || found.SessionID != "" {
		t.Fatalf("LookupSession() before cache = %#v/%v, want miss", found, ok)
	}

	if err := store.CacheSession(ctx, tokenHash, snapshot); err != nil {
		t.Fatalf("CacheSession() error = %v", err)
	}
	ttl, err := client.rdb.PTTL(ctx, tokenKey).Result()
	if err != nil {
		t.Fatalf("PTTL() cached session error = %v", err)
	}
	if ttl <= 0 || ttl > 2*time.Minute {
		t.Fatalf("PTTL() cached session = %s, want within configured ttl", ttl)
	}

	found, ok, err = store.LookupSession(ctx, tokenHash)
	if err != nil {
		t.Fatalf("LookupSession() after cache error = %v", err)
	}
	if !ok || found.SessionID != snapshot.SessionID || found.UserID != snapshot.UserID || found.DisplayName != snapshot.DisplayName || found.Role != snapshot.Role || !found.ExpiresAt.Equal(snapshot.ExpiresAt) {
		t.Fatalf("LookupSession() after cache = %#v/%v, want cached snapshot", found, ok)
	}

	if err := store.DeleteSession(ctx, tokenHash); err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	_, ok, err = store.LookupSession(ctx, tokenHash)
	if err != nil {
		t.Fatalf("LookupSession() after delete error = %v", err)
	}
	if ok {
		t.Fatal("LookupSession() after delete ok = true, want false")
	}

	revoked, err := store.IsSessionRevoked(ctx, snapshot.SessionID)
	if err != nil {
		t.Fatalf("IsSessionRevoked() before mark error = %v", err)
	}
	if revoked {
		t.Fatal("IsSessionRevoked() before mark = true, want false")
	}
	if err := store.MarkSessionRevoked(ctx, snapshot.SessionID); err != nil {
		t.Fatalf("MarkSessionRevoked() error = %v", err)
	}
	revoked, err = store.IsSessionRevoked(ctx, snapshot.SessionID)
	if err != nil {
		t.Fatalf("IsSessionRevoked() after mark error = %v", err)
	}
	if !revoked {
		t.Fatal("IsSessionRevoked() after mark = false, want true")
	}
	if err := store.ClearSessionRevoked(ctx, snapshot.SessionID); err != nil {
		t.Fatalf("ClearSessionRevoked() error = %v", err)
	}
	revoked, err = store.IsSessionRevoked(ctx, snapshot.SessionID)
	if err != nil {
		t.Fatalf("IsSessionRevoked() after clear error = %v", err)
	}
	if revoked {
		t.Fatal("IsSessionRevoked() after clear = true, want false")
	}
}
