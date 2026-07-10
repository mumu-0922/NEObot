package ratelimit

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// MemoryStore is a bounded, process-local fixed-window rate-limit store. Keys
// are expected to be opaque hashes produced by the caller; the store does not
// retain canonical identity data.
type MemoryStore struct {
	mu         sync.Mutex
	maxEntries int
	entries    map[string]memoryEntry
}

type memoryEntry struct {
	window  time.Duration
	bucket  int64
	count   uint64
	resetAt time.Time
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore creates a bounded in-memory rate-limit store. A non-positive
// capacity stores no entries and therefore fails closed for every valid key.
func NewMemoryStore(maxEntries int) *MemoryStore {
	if maxEntries < 0 {
		maxEntries = 0
	}

	return &MemoryStore{
		maxEntries: maxEntries,
		entries:    make(map[string]memoryEntry),
	}
}

// Allow records one attempt and returns its fixed-window decision.
func (s *MemoryStore) Allow(
	ctx context.Context,
	key string,
	limit int,
	window time.Duration,
	now time.Time,
) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("rate limit context is required")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return Result{}, errors.New("rate limit key is required")
	}
	if limit <= 0 {
		return Result{}, errors.New("memory rate limit must be positive")
	}
	if window <= 0 {
		return Result{}, errors.New("memory rate limit window must be positive")
	}
	if s == nil {
		return Result{}, errors.New("memory rate limit store is not initialized")
	}

	bucket, resetAt := memoryWindow(now, window)

	s.mu.Lock()
	defer s.mu.Unlock()

	// A caller may be canceled while waiting for another decision to finish.
	// Recheck under the lock so a canceled call never mutates a counter.
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	entry, exists := s.entries[key]
	if !exists {
		// Keep established-key decisions O(1). New keys opportunistically
		// reclaim expired slots before the bounded-capacity check.
		s.deleteExpired(now)
		if len(s.entries) >= s.maxEntries {
			capacityResetAt := s.nextExpiry(resetAt)
			return blockedResult(limit, now, capacityResetAt), nil
		}
	}

	if !exists || entry.window != window || entry.bucket != bucket {
		entry = memoryEntry{
			window:  window,
			bucket:  bucket,
			resetAt: resetAt,
		}
	}
	if entry.count < ^uint64(0) {
		entry.count++
	}
	s.entries[key] = entry

	allowed := entry.count <= uint64(limit)
	remaining := 0
	if entry.count < uint64(limit) {
		remaining = limit - int(entry.count)
	}

	return Result{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		RetryAfter: retryAfter(now, resetAt),
		ResetAt:    resetAt,
	}, nil
}

func (s *MemoryStore) deleteExpired(now time.Time) {
	for key, entry := range s.entries {
		if !now.Before(entry.resetAt) {
			delete(s.entries, key)
		}
	}
}

func (s *MemoryStore) nextExpiry(fallback time.Time) time.Time {
	var earliest time.Time
	for _, entry := range s.entries {
		if earliest.IsZero() || entry.resetAt.Before(earliest) {
			earliest = entry.resetAt
		}
	}
	if earliest.IsZero() {
		return fallback
	}

	return earliest
}

func blockedResult(limit int, now, resetAt time.Time) Result {
	return Result{
		Allowed:    false,
		Limit:      limit,
		Remaining:  0,
		RetryAfter: retryAfter(now, resetAt),
		ResetAt:    resetAt,
	}
}

func retryAfter(now, resetAt time.Time) time.Duration {
	retry := resetAt.Sub(now)
	if retry < 0 {
		return 0
	}

	return retry
}

func memoryWindow(now time.Time, window time.Duration) (int64, time.Time) {
	windowNanos := int64(window)
	unixNanos := now.UnixNano()
	bucket := unixNanos / windowNanos
	remainder := unixNanos % windowNanos
	if unixNanos < 0 && remainder != 0 {
		bucket--
	}

	untilReset := windowNanos
	if remainder > 0 {
		untilReset -= remainder
	} else if remainder < 0 {
		untilReset = -remainder
	}
	resetAt := now.Add(time.Duration(untilReset)).UTC()

	return bucket, resetAt
}
