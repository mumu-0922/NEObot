package redisstate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"neo-chat/mm-chat/backend/internal/ratelimit"
)

var rateLimitIncrementScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return current
`)

type RateLimitStore struct {
	rdb       *redis.Client
	keyPrefix string
}

func (c *Client) RateLimitStore() *RateLimitStore {
	if c == nil || c.rdb == nil {
		return nil
	}

	return &RateLimitStore{
		rdb:       c.rdb,
		keyPrefix: c.keyPrefix,
	}
}

func (s *RateLimitStore) Allow(
	ctx context.Context,
	key string,
	limit int,
	window time.Duration,
	now time.Time,
) (ratelimit.Result, error) {
	if err := s.requireReady(limit, window); err != nil {
		return ratelimit.Result{}, err
	}
	redisKey, err := s.rateLimitKey(key, window, now)
	if err != nil {
		return ratelimit.Result{}, err
	}

	ttl := window + time.Second
	count, err := rateLimitIncrementScript.Run(
		ctx,
		s.rdb,
		[]string{redisKey},
		int64(ttl/time.Millisecond),
	).Int64()
	if err != nil {
		return ratelimit.Result{}, err
	}

	resetAt := nextWindowReset(now, window)
	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	retryAfter := resetAt.Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}

	return ratelimit.Result{
		Allowed:    int(count) <= limit,
		Limit:      limit,
		Remaining:  remaining,
		RetryAfter: retryAfter,
		ResetAt:    resetAt,
	}, nil
}

func (s *RateLimitStore) requireReady(limit int, window time.Duration) error {
	if s == nil || s.rdb == nil {
		return errors.New("redis rate limit store is not initialized")
	}
	if limit <= 0 {
		return errors.New("redis rate limit must be positive")
	}
	if window <= 0 {
		return errors.New("redis rate limit window must be positive")
	}

	return nil
}

func (s *RateLimitStore) rateLimitKey(key string, window time.Duration, now time.Time) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("rate limit key is required")
	}
	if strings.ContainsAny(key, " \t\r\n") {
		return "", errors.New("rate limit key must not contain whitespace")
	}

	prefix := normalizeKeyPrefix(s.keyPrefix)
	bucket := windowBucket(now, window)
	return fmt.Sprintf("%s:rate_limit:%s:%d", prefix, key, bucket), nil
}

func windowBucket(now time.Time, window time.Duration) int64 {
	windowNanos := int64(window)
	if windowNanos <= 0 {
		return now.UnixNano()
	}

	return now.UnixNano() / windowNanos
}

func nextWindowReset(now time.Time, window time.Duration) time.Time {
	windowNanos := int64(window)
	if windowNanos <= 0 {
		return now
	}

	bucketStart := (now.UnixNano() / windowNanos) * windowNanos
	return time.Unix(0, bucketStart+windowNanos).UTC()
}
