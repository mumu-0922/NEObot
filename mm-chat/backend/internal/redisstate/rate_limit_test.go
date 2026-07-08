package redisstate

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestRateLimitKeyValidation(t *testing.T) {
	store := &RateLimitStore{
		rdb:       redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}),
		keyPrefix: "test",
	}
	t.Cleanup(func() {
		_ = store.rdb.Close()
	})

	now := time.Unix(100, 0)
	key, err := store.rateLimitKey(" http:abc123 ", time.Minute, now)
	if err != nil {
		t.Fatalf("rateLimitKey() error = %v", err)
	}
	if key != "test:rate_limit:http:abc123:1" {
		t.Fatalf("rateLimitKey() = %q", key)
	}
	if _, err := store.rateLimitKey("http abc", time.Minute, now); err == nil {
		t.Fatal("rateLimitKey() error = nil, want whitespace error")
	}
	if _, err := store.rateLimitKey(" ", time.Minute, now); err == nil {
		t.Fatal("rateLimitKey() error = nil, want required error")
	}
}

func TestRateLimitStoreIntegration(t *testing.T) {
	redisURL := testRedisURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Open(ctx, config.RedisConfig{
		URL:       redisURL,
		KeyPrefix: "mm-chat-rate-test",
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	store := client.RateLimitStore()
	now := time.Unix(120, 0).UTC()
	key := "http:integration"
	redisKey, err := store.rateLimitKey(key, time.Minute, now)
	if err != nil {
		t.Fatalf("rateLimitKey() error = %v", err)
	}
	if err := client.rdb.Del(ctx, redisKey).Err(); err != nil {
		t.Fatalf("Del() before test error = %v", err)
	}

	first, err := store.Allow(ctx, key, 2, time.Minute, now)
	if err != nil {
		t.Fatalf("Allow() first error = %v", err)
	}
	if !first.Allowed || first.Remaining != 1 || first.Limit != 2 {
		t.Fatalf("Allow() first = %#v, want allowed remaining 1", first)
	}
	firstTTL, err := client.rdb.PTTL(ctx, redisKey).Result()
	if err != nil {
		t.Fatalf("PTTL() after first allow error = %v", err)
	}
	if firstTTL <= 0 || firstTTL > time.Minute+time.Second {
		t.Fatalf("PTTL() after first allow = %s, want within configured TTL", firstTTL)
	}
	second, err := store.Allow(ctx, key, 2, time.Minute, now.Add(time.Second))
	if err != nil {
		t.Fatalf("Allow() second error = %v", err)
	}
	if !second.Allowed || second.Remaining != 0 {
		t.Fatalf("Allow() second = %#v, want allowed remaining 0", second)
	}
	secondTTL, err := client.rdb.PTTL(ctx, redisKey).Result()
	if err != nil {
		t.Fatalf("PTTL() after second allow error = %v", err)
	}
	if secondTTL <= 0 || secondTTL > firstTTL {
		t.Fatalf("PTTL() after second allow = %s, want positive and not extended above %s", secondTTL, firstTTL)
	}
	third, err := store.Allow(ctx, key, 2, time.Minute, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Allow() third error = %v", err)
	}
	if third.Allowed || third.Remaining != 0 || third.RetryAfter <= 0 {
		t.Fatalf("Allow() third = %#v, want blocked with retry", third)
	}
}

func testRedisURL(t *testing.T) string {
	t.Helper()

	redisURL := os.Getenv("MM_CHAT_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set MM_CHAT_TEST_REDIS_URL to run Redis integration tests")
	}

	return redisURL
}
