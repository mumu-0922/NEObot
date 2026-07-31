package ragproviders

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const providerRetryFallbackDelay = 5 * time.Second

type providerRetryError struct {
	delay time.Duration
}

func (failure *providerRetryError) Error() string {
	return ErrProviderGatewayUpstream.Error()
}

func (failure *providerRetryError) Unwrap() error {
	return ErrProviderGatewayUpstream
}

// ProviderRetryDelay returns bounded, plaintext-free advice only for a
// transient transport/read failure or explicit 408/429/5xx response. Invalid
// JSON, schema drift, and deterministic 4xx responses do not implement this
// contract and therefore cannot be retried by the accuracy-first runner.
func ProviderRetryDelay(err error) (time.Duration, bool) {
	var failure *providerRetryError
	if !errors.As(err, &failure) || failure == nil || failure.delay < 0 {
		return 0, false
	}
	return failure.delay, true
}

func newProviderRetryError(retryAfter string) error {
	delay, ok := parseProviderRetryAfter(retryAfter, time.Now())
	if !ok {
		delay = providerRetryFallbackDelay
	}
	return &providerRetryError{delay: delay}
}

func providerGatewayRetryableStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError
}

func parseProviderRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(value, 10, 63); err == nil {
		maximumSeconds := uint64((time.Duration(1<<63 - 1)) / time.Second)
		if seconds > maximumSeconds {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	parsed, err := http.ParseTime(value)
	if err != nil || parsed.Before(now) {
		return 0, false
	}
	return parsed.Sub(now), true
}
