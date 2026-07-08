package ratelimit

import (
	"context"
	"time"
)

// Result describes one fixed-window rate-limit decision.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}

// Store records short-lived rate-limit counters. Implementations must not store
// canonical user data or secrets.
type Store interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration, now time.Time) (Result, error)
}
