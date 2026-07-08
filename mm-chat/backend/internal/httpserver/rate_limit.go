package httpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/ratelimit"
)

const (
	rateLimitCode    = "RATE_LIMITED"
	rateLimitMessage = "too many requests"
)

type rateLimitClock func() time.Time

func withRateLimit(
	store ratelimit.Store,
	redisCfg config.RedisConfig,
	now rateLimitClock,
) Middleware {
	limit := redisCfg.RateLimitRequests
	if limit <= 0 {
		limit = config.DefaultRedisRateLimitRequests
	}
	window := redisCfg.RateLimitWindow
	if window <= 0 {
		window = config.DefaultRedisRateLimitWindow
	}
	if now == nil {
		now = time.Now
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if store == nil || !redisCfg.RateLimitEnabled || isRateLimitExemptPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			result, err := store.Allow(r.Context(), rateLimitKey(r), limit, window, now())
			if err != nil {
				// Redis is non-authoritative temporary state. Runtime Redis errors must
				// not block canonical API reads/writes.
				next.ServeHTTP(w, r)
				return
			}

			writeRateLimitHeaders(w, result)
			if !result.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(result.RetryAfter)))
				writeJSON(w, http.StatusTooManyRequests, ErrorResponse{
					Error: ErrorBody{Code: rateLimitCode, Message: rateLimitMessage},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isRateLimitExemptPath(path string) bool {
	switch path {
	case "/health", "/ready", "/v1/version":
		return true
	default:
		return false
	}
}

func rateLimitKey(r *http.Request) string {
	client := clientAddress(r.RemoteAddr)
	sum := sha256.Sum256([]byte("http:" + client))
	return "http:" + hex.EncodeToString(sum[:])
}

func clientAddress(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	if remoteAddr != "" {
		return remoteAddr
	}

	return "unknown"
}

func writeRateLimitHeaders(w http.ResponseWriter, result ratelimit.Result) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	if !result.ResetAt.IsZero() {
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetAt.Unix()))
	}
}

func retryAfterSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 1
	}

	seconds := int(math.Ceil(duration.Seconds()))
	if seconds < 1 {
		return 1
	}

	return seconds
}
