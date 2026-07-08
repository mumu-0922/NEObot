package redisstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/sessioncache"
)

type sessionCachePayload struct {
	sessioncache.Snapshot
	CachedAt time.Time `json:"cachedAt"`
}

// SessionCacheStore stores non-authoritative session lookup results and
// revocation hints. Postgres remains the source of truth for session validity.
type SessionCacheStore struct {
	rdb       *redis.Client
	keyPrefix string
	ttl       time.Duration
	now       func() time.Time
}

func (c *Client) SessionCacheStore(ttl time.Duration) *SessionCacheStore {
	if c == nil || c.rdb == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = config.DefaultRedisSessionCacheTTL
	}

	return &SessionCacheStore{
		rdb:       c.rdb,
		keyPrefix: c.keyPrefix,
		ttl:       ttl,
		now:       time.Now,
	}
}

func (s *SessionCacheStore) CacheSession(
	ctx context.Context,
	tokenHash string,
	snapshot sessioncache.Snapshot,
) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	key, err := s.sessionTokenKey(tokenHash)
	if err != nil {
		return err
	}
	if err := s.validateSnapshot(snapshot); err != nil {
		return err
	}

	ttl := s.cacheTTL(snapshot.ExpiresAt)
	if ttl <= 0 {
		return errors.New("session snapshot is already expired")
	}

	payload, err := json.Marshal(sessionCachePayload{
		Snapshot: snapshot,
		CachedAt: s.clock().UTC(),
	})
	if err != nil {
		return err
	}

	return s.rdb.Set(ctx, key, payload, ttl).Err()
}

func (s *SessionCacheStore) LookupSession(
	ctx context.Context,
	tokenHash string,
) (sessioncache.Snapshot, bool, error) {
	if err := s.requireReady(); err != nil {
		return sessioncache.Snapshot{}, false, err
	}
	key, err := s.sessionTokenKey(tokenHash)
	if err != nil {
		return sessioncache.Snapshot{}, false, err
	}

	value, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return sessioncache.Snapshot{}, false, nil
	}
	if err != nil {
		return sessioncache.Snapshot{}, false, err
	}

	var payload sessionCachePayload
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		_ = s.rdb.Del(ctx, key).Err()
		return sessioncache.Snapshot{}, false, fmt.Errorf("decode cached session: %w", err)
	}
	if err := s.validateSnapshot(payload.Snapshot); err != nil {
		_ = s.rdb.Del(ctx, key).Err()
		return sessioncache.Snapshot{}, false, fmt.Errorf("invalid cached session: %w", err)
	}
	if !payload.ExpiresAt.After(s.clock()) {
		_ = s.rdb.Del(ctx, key).Err()
		return sessioncache.Snapshot{}, false, nil
	}

	return payload.Snapshot, true, nil
}

func (s *SessionCacheStore) DeleteSession(ctx context.Context, tokenHash string) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	key, err := s.sessionTokenKey(tokenHash)
	if err != nil {
		return err
	}

	return s.rdb.Del(ctx, key).Err()
}

func (s *SessionCacheStore) MarkSessionRevoked(ctx context.Context, sessionID string) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	key, err := s.sessionRevokedKey(sessionID)
	if err != nil {
		return err
	}

	return s.rdb.Set(ctx, key, "1", s.ttl).Err()
}

func (s *SessionCacheStore) IsSessionRevoked(ctx context.Context, sessionID string) (bool, error) {
	if err := s.requireReady(); err != nil {
		return false, err
	}
	key, err := s.sessionRevokedKey(sessionID)
	if err != nil {
		return false, err
	}

	value, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return value != "", nil
}

func (s *SessionCacheStore) ClearSessionRevoked(ctx context.Context, sessionID string) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	key, err := s.sessionRevokedKey(sessionID)
	if err != nil {
		return err
	}

	return s.rdb.Del(ctx, key).Err()
}

func (s *SessionCacheStore) requireReady() error {
	if s == nil || s.rdb == nil {
		return errors.New("redis session cache store is not initialized")
	}
	if s.ttl <= 0 {
		return errors.New("redis session cache ttl is invalid")
	}

	return nil
}

func (s *SessionCacheStore) validateSnapshot(snapshot sessioncache.Snapshot) error {
	if _, err := cleanSessionID(snapshot.SessionID); err != nil {
		return fmt.Errorf("session id: %w", err)
	}
	if _, err := cleanUserID(snapshot.UserID); err != nil {
		return fmt.Errorf("user id: %w", err)
	}
	if snapshot.ExpiresAt.IsZero() {
		return errors.New("expires_at is required")
	}

	return nil
}

func (s *SessionCacheStore) sessionTokenKey(tokenHash string) (string, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return "", errors.New("token hash is required")
	}
	if strings.ContainsAny(tokenHash, " \t\r\n") {
		return "", errors.New("token hash must not contain whitespace")
	}

	sum := sha256.Sum256([]byte(tokenHash))
	prefix := normalizeKeyPrefix(s.keyPrefix)
	return fmt.Sprintf("%s:sessions:token:%s", prefix, hex.EncodeToString(sum[:])), nil
}

func (s *SessionCacheStore) sessionRevokedKey(sessionID string) (string, error) {
	cleaned, err := cleanSessionID(sessionID)
	if err != nil {
		return "", err
	}

	prefix := normalizeKeyPrefix(s.keyPrefix)
	return fmt.Sprintf("%s:sessions:%s:revoked", prefix, cleaned), nil
}

func (s *SessionCacheStore) cacheTTL(expiresAt time.Time) time.Duration {
	remaining := expiresAt.Sub(s.clock())
	if remaining <= 0 {
		return 0
	}
	if remaining < s.ttl {
		return remaining
	}

	return s.ttl
}

func (s *SessionCacheStore) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}

	return time.Now()
}

func cleanSessionID(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", errors.New("session id is required")
	}
	if strings.ContainsAny(sessionID, " \t\r\n") {
		return "", errors.New("session id must not contain whitespace")
	}

	return sessionID, nil
}

func cleanUserID(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", errors.New("user id is required")
	}
	if strings.ContainsAny(userID, " \t\r\n") {
		return "", errors.New("user id must not contain whitespace")
	}

	return userID, nil
}
