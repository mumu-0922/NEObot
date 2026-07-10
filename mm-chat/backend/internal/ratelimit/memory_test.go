package ratelimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryStoreFixedWindow(t *testing.T) {
	store := NewMemoryStore(10)
	now := time.Unix(125, 250*time.Millisecond.Nanoseconds()).UTC()
	resetAt := time.Unix(180, 0).UTC()
	wantRetryAfter := resetAt.Sub(now)

	tests := []struct {
		name      string
		allowed   bool
		remaining int
	}{
		{name: "first", allowed: true, remaining: 1},
		{name: "last allowed", allowed: true, remaining: 0},
		{name: "blocked", allowed: false, remaining: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := store.Allow(
				context.Background(),
				"sha256:account",
				2,
				time.Minute,
				now,
			)
			if err != nil {
				t.Fatalf("Allow() error = %v", err)
			}
			if result.Allowed != test.allowed || result.Remaining != test.remaining {
				t.Fatalf(
					"Allow() = %#v, want allowed=%v remaining=%d",
					result,
					test.allowed,
					test.remaining,
				)
			}
			if result.Limit != 2 {
				t.Fatalf("Allow().Limit = %d, want 2", result.Limit)
			}
			if !result.ResetAt.Equal(resetAt) {
				t.Fatalf("Allow().ResetAt = %s, want %s", result.ResetAt, resetAt)
			}
			if result.RetryAfter != wantRetryAfter {
				t.Fatalf(
					"Allow().RetryAfter = %s, want %s",
					result.RetryAfter,
					wantRetryAfter,
				)
			}
		})
	}
}

func TestMemoryStoreWindowReset(t *testing.T) {
	store := NewMemoryStore(1)
	window := 10 * time.Second
	now := time.Unix(21, 0).UTC()

	first, err := store.Allow(context.Background(), "sha256:ip", 1, window, now)
	if err != nil {
		t.Fatalf("Allow() first error = %v", err)
	}
	if !first.Allowed || !first.ResetAt.Equal(time.Unix(30, 0).UTC()) {
		t.Fatalf("Allow() first = %#v, want allowed until second 30", first)
	}

	blocked, err := store.Allow(
		context.Background(),
		"sha256:ip",
		1,
		window,
		time.Unix(29, 999_999_999).UTC(),
	)
	if err != nil {
		t.Fatalf("Allow() blocked error = %v", err)
	}
	if blocked.Allowed || blocked.RetryAfter != time.Nanosecond {
		t.Fatalf("Allow() blocked = %#v, want one nanosecond retry", blocked)
	}

	reset, err := store.Allow(
		context.Background(),
		"sha256:ip",
		1,
		window,
		time.Unix(30, 0).UTC(),
	)
	if err != nil {
		t.Fatalf("Allow() reset error = %v", err)
	}
	if !reset.Allowed || reset.Remaining != 0 {
		t.Fatalf("Allow() reset = %#v, want new allowed window", reset)
	}
	if !reset.ResetAt.Equal(time.Unix(40, 0).UTC()) || reset.RetryAfter != window {
		t.Fatalf("Allow() reset = %#v, want reset at second 40", reset)
	}
}

func TestMemoryStoreConcurrentLimit(t *testing.T) {
	const (
		attempts = 128
		limit    = 25
	)

	store := NewMemoryStore(1)
	now := time.Unix(120, 0).UTC()
	start := make(chan struct{})
	results := make(chan Result, attempts)
	errs := make(chan error, attempts)
	var wg sync.WaitGroup

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := store.Allow(
				context.Background(),
				"sha256:concurrent",
				limit,
				time.Minute,
				now,
			)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}

	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Errorf("Allow() concurrent error = %v", err)
	}

	allowed := 0
	for result := range results {
		if result.Allowed {
			allowed++
		}
		if result.Remaining < 0 || result.Remaining >= limit {
			t.Errorf("Allow() concurrent result = %#v, invalid remaining", result)
		}
	}
	if allowed != limit {
		t.Fatalf("Allow() concurrent allowed = %d, want %d", allowed, limit)
	}
}

func TestMemoryStoreCapacityFailsClosedAndCleansExpiredEntries(t *testing.T) {
	store := NewMemoryStore(1)
	window := 10 * time.Second
	now := time.Unix(21, 0).UTC()

	first, err := store.Allow(context.Background(), "sha256:first", 2, window, now)
	if err != nil {
		t.Fatalf("Allow() first error = %v", err)
	}
	if !first.Allowed {
		t.Fatalf("Allow() first = %#v, want allowed", first)
	}

	full, err := store.Allow(context.Background(), "sha256:new", 3, window, now)
	if err != nil {
		t.Fatalf("Allow() at capacity error = %v", err)
	}
	if full.Allowed || full.Limit != 3 || full.Remaining != 0 {
		t.Fatalf("Allow() at capacity = %#v, want fail closed", full)
	}
	if !full.ResetAt.Equal(time.Unix(30, 0).UTC()) || full.RetryAfter != 9*time.Second {
		t.Fatalf("Allow() at capacity = %#v, want retry at existing expiry", full)
	}

	existing, err := store.Allow(context.Background(), "sha256:first", 2, window, now)
	if err != nil {
		t.Fatalf("Allow() existing error = %v", err)
	}
	if !existing.Allowed || existing.Remaining != 0 {
		t.Fatalf("Allow() existing = %#v, want existing key to continue", existing)
	}

	afterExpiry, err := store.Allow(
		context.Background(),
		"sha256:new",
		3,
		window,
		time.Unix(30, 0).UTC(),
	)
	if err != nil {
		t.Fatalf("Allow() after expiry error = %v", err)
	}
	if !afterExpiry.Allowed || afterExpiry.Remaining != 2 {
		t.Fatalf("Allow() after expiry = %#v, want cleaned capacity", afterExpiry)
	}
}

func TestMemoryStoreNonPositiveCapacityFailsClosed(t *testing.T) {
	store := NewMemoryStore(0)
	now := time.Unix(21, 0).UTC()

	result, err := store.Allow(
		context.Background(),
		"sha256:key",
		2,
		10*time.Second,
		now,
	)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if result.Allowed || result.Remaining != 0 {
		t.Fatalf("Allow() = %#v, want fail closed", result)
	}
	if !result.ResetAt.Equal(time.Unix(30, 0).UTC()) || result.RetryAfter != 9*time.Second {
		t.Fatalf("Allow() = %#v, want requested window retry", result)
	}
}

func TestMemoryStoreCanceledContextDoesNotConsumeCapacity(t *testing.T) {
	store := NewMemoryStore(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.Allow(
		ctx,
		"sha256:canceled",
		1,
		time.Minute,
		time.Unix(120, 0).UTC(),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Allow() error = %v, want context.Canceled", err)
	}

	result, err := store.Allow(
		context.Background(),
		"sha256:valid",
		1,
		time.Minute,
		time.Unix(120, 0).UTC(),
	)
	if err != nil {
		t.Fatalf("Allow() after cancellation error = %v", err)
	}
	if !result.Allowed {
		t.Fatalf("Allow() after cancellation = %#v, want capacity available", result)
	}
}

func TestMemoryStoreValidation(t *testing.T) {
	store := NewMemoryStore(1)
	now := time.Unix(120, 0).UTC()

	tests := []struct {
		name   string
		key    string
		limit  int
		window time.Duration
	}{
		{name: "empty key", key: " ", limit: 1, window: time.Minute},
		{name: "zero limit", key: "sha256:key", limit: 0, window: time.Minute},
		{name: "negative limit", key: "sha256:key", limit: -1, window: time.Minute},
		{name: "zero window", key: "sha256:key", limit: 1, window: 0},
		{name: "negative window", key: "sha256:key", limit: 1, window: -time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Allow(
				context.Background(),
				test.key,
				test.limit,
				test.window,
				now,
			)
			if err == nil {
				t.Fatal("Allow() error = nil, want validation error")
			}
		})
	}
}
