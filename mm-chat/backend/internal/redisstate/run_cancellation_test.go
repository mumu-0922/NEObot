package redisstate

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestNormalizeKeyPrefix(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: defaultKeyPrefix},
		{name: "spaces", in: "  ", want: defaultKeyPrefix},
		{name: "trim colons", in: ":neo-chat:", want: "neo-chat"},
		{name: "trim spaces", in: " neo:test ", want: "neo:test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeKeyPrefix(tt.in); got != tt.want {
				t.Fatalf("normalizeKeyPrefix(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunCancellationKeyValidation(t *testing.T) {
	store := &RunCancellationStore{
		rdb:       redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}),
		keyPrefix: "test",
		ttl:       time.Minute,
	}
	t.Cleanup(func() {
		_ = store.rdb.Close()
	})

	if key, err := store.runCancelKey(" run-1 "); err != nil || key != "test:runs:run-1:cancelled" {
		t.Fatalf("runCancelKey() = %q/%v, want normalized key", key, err)
	}
	if _, err := store.runCancelKey("run 1"); err == nil {
		t.Fatal("runCancelKey() error = nil, want whitespace error")
	}
	if _, err := store.runCancelKey(" "); err == nil {
		t.Fatal("runCancelKey() error = nil, want required error")
	}
}

func TestOpenReturnsNilWhenRedisURLIsEmpty(t *testing.T) {
	client, err := Open(context.Background(), config.RedisConfig{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if client != nil {
		t.Fatalf("Open() client = %#v, want nil", client)
	}
}

func TestOpenRejectsInvalidURLWithoutLeakingSecret(t *testing.T) {
	secret := "top-secret-redis-password"
	_, err := Open(context.Background(), config.RedisConfig{
		URL: "redis://:" + secret + "@[::1",
	})
	if err == nil {
		t.Fatal("Open() error = nil, want parse error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Open() error leaks secret: %v", err)
	}
}

func TestRunCancellationStoreIntegration(t *testing.T) {
	redisURL := os.Getenv("MM_CHAT_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set MM_CHAT_TEST_REDIS_URL to run Redis integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Open(ctx, config.RedisConfig{
		URL:          redisURL,
		KeyPrefix:    "mm-chat-test",
		RunCancelTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer client.Close()

	store := client.RunCancellationStore(time.Minute)
	runID := "11111111-1111-4111-8111-111111111111"
	if err := store.ClearRunCancelled(ctx, runID); err != nil {
		t.Fatalf("ClearRunCancelled() before mark error = %v", err)
	}
	cancelled, err := store.IsRunCancelled(ctx, runID)
	if err != nil {
		t.Fatalf("IsRunCancelled() before mark error = %v", err)
	}
	if cancelled {
		t.Fatal("IsRunCancelled() before mark = true, want false")
	}
	if err := store.MarkRunCancelled(ctx, runID); err != nil {
		t.Fatalf("MarkRunCancelled() error = %v", err)
	}
	cancelled, err = store.IsRunCancelled(ctx, runID)
	if err != nil {
		t.Fatalf("IsRunCancelled() after mark error = %v", err)
	}
	if !cancelled {
		t.Fatal("IsRunCancelled() after mark = false, want true")
	}
	if err := store.ClearRunCancelled(ctx, runID); err != nil {
		t.Fatalf("ClearRunCancelled() after mark error = %v", err)
	}
	cancelled, err = store.IsRunCancelled(ctx, runID)
	if err != nil {
		t.Fatalf("IsRunCancelled() after clear error = %v", err)
	}
	if cancelled {
		t.Fatal("IsRunCancelled() after clear = true, want false")
	}
}
