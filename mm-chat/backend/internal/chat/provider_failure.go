package chat

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const providerRetryFallbackDelay = 5 * time.Second

// ProviderFailureCategory is a bounded, plaintext-free classification for
// Provider request and stream failures. It is safe to aggregate, but it is not
// intended to expose upstream response bodies or error messages.
type ProviderFailureCategory string

const (
	ProviderFailureRequestBuildFailed ProviderFailureCategory = "PROVIDER_REQUEST_BUILD_FAILED"
	ProviderFailureTransportFailed    ProviderFailureCategory = "PROVIDER_TRANSPORT_FAILED"
	ProviderFailureResponseInvalid    ProviderFailureCategory = "PROVIDER_RESPONSE_INVALID"
	ProviderFailureAuthentication     ProviderFailureCategory = "PROVIDER_AUTHENTICATION_FAILED"
	ProviderFailureQuotaExhausted     ProviderFailureCategory = "PROVIDER_QUOTA_EXHAUSTED"
	ProviderFailureRequestTimeout     ProviderFailureCategory = "PROVIDER_REQUEST_TIMEOUT"
	ProviderFailureRateLimited        ProviderFailureCategory = "PROVIDER_RATE_LIMITED"
	ProviderFailureRequestRejected    ProviderFailureCategory = "PROVIDER_REQUEST_REJECTED"
	ProviderFailureUpstreamFailed     ProviderFailureCategory = "PROVIDER_UPSTREAM_FAILED"
	ProviderFailureStreamParseFailed  ProviderFailureCategory = "PROVIDER_STREAM_PARSE_FAILED"
	ProviderFailureStreamReadFailed   ProviderFailureCategory = "PROVIDER_STREAM_READ_FAILED"
	ProviderFailureStreamIncomplete   ProviderFailureCategory = "PROVIDER_STREAM_INCOMPLETE"
	ProviderFailureStreamRemoteError  ProviderFailureCategory = "PROVIDER_STREAM_REMOTE_ERROR"
	ProviderFailureContextDeadline    ProviderFailureCategory = "CONTEXT_DEADLINE"
	ProviderFailureContextCanceled    ProviderFailureCategory = "CONTEXT_CANCELED"
)

var providerFailureCategories = map[ProviderFailureCategory]struct{}{
	ProviderFailureRequestBuildFailed: {},
	ProviderFailureTransportFailed:    {},
	ProviderFailureResponseInvalid:    {},
	ProviderFailureAuthentication:     {},
	ProviderFailureQuotaExhausted:     {},
	ProviderFailureRequestTimeout:     {},
	ProviderFailureRateLimited:        {},
	ProviderFailureRequestRejected:    {},
	ProviderFailureUpstreamFailed:     {},
	ProviderFailureStreamParseFailed:  {},
	ProviderFailureStreamReadFailed:   {},
	ProviderFailureStreamIncomplete:   {},
	ProviderFailureStreamRemoteError:  {},
	ProviderFailureContextDeadline:    {},
	ProviderFailureContextCanceled:    {},
}

type providerFailureError struct {
	category      ProviderFailureCategory
	message       string
	retryAfter    time.Duration
	hasRetryAfter bool
}

func (failure *providerFailureError) Error() string {
	if failure == nil || failure.message == "" {
		return "provider request failed"
	}
	return failure.message
}

func newProviderFailure(
	category ProviderFailureCategory,
	message string,
) error {
	return &providerFailureError{category: category, message: message}
}

func newProviderHTTPFailure(
	category ProviderFailureCategory,
	message string,
	retryAfter string,
) error {
	failure := &providerFailureError{category: category, message: message}
	if delay, ok := parseProviderRetryAfter(retryAfter, time.Now()); ok {
		failure.retryAfter = delay
		failure.hasRetryAfter = true
	}
	return failure
}

// ProviderFailureCategoryOf returns only a fixed category. It deliberately
// does not return or persist the wrapped Provider error text.
func ProviderFailureCategoryOf(err error) (ProviderFailureCategory, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ProviderFailureContextDeadline, true
	}
	if errors.Is(err, context.Canceled) {
		return ProviderFailureContextCanceled, true
	}
	var failure *providerFailureError
	if !errors.As(err, &failure) || failure == nil || failure.category == "" {
		return "", false
	}
	return failure.category, true
}

// ValidProviderFailureCategory reports whether category belongs to the fixed,
// plaintext-free Provider taxonomy.
func ValidProviderFailureCategory(category ProviderFailureCategory) bool {
	_, ok := providerFailureCategories[category]
	return ok
}

// ProviderFailureCategories returns a sorted copy of the fixed Provider
// taxonomy so higher-level diagnostic contracts can bind it by hash without
// duplicating the HTTP/SSE category list.
func ProviderFailureCategories() []ProviderFailureCategory {
	categories := make([]ProviderFailureCategory, 0, len(providerFailureCategories))
	for category := range providerFailureCategories {
		categories = append(categories, category)
	}
	sort.Slice(categories, func(i, j int) bool { return categories[i] < categories[j] })
	return categories
}

// ProviderRetryDelay exposes only a duration and retryability decision. It
// accepts explicit 408/429/5xx classes plus transport/stream interruptions;
// invalid response/schema/stream syntax and deterministic 4xx classes remain
// non-retryable.
func ProviderRetryDelay(err error) (time.Duration, bool) {
	category, ok := ProviderFailureCategoryOf(err)
	if !ok {
		return 0, false
	}
	switch category {
	case ProviderFailureRequestTimeout,
		ProviderFailureRateLimited,
		ProviderFailureUpstreamFailed,
		ProviderFailureTransportFailed,
		ProviderFailureStreamReadFailed,
		ProviderFailureStreamIncomplete:
	default:
		return 0, false
	}
	var failure *providerFailureError
	if errors.As(err, &failure) && failure != nil && failure.hasRetryAfter {
		return failure.retryAfter, true
	}
	return providerRetryFallbackDelay, true
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

func providerHTTPFailureCategory(statusCode int) ProviderFailureCategory {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return ProviderFailureAuthentication
	case statusCode == http.StatusPaymentRequired:
		return ProviderFailureQuotaExhausted
	case statusCode == http.StatusRequestTimeout:
		return ProviderFailureRequestTimeout
	case statusCode == http.StatusTooManyRequests:
		return ProviderFailureRateLimited
	case statusCode >= http.StatusInternalServerError:
		return ProviderFailureUpstreamFailed
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		return ProviderFailureRequestRejected
	default:
		return ProviderFailureRequestRejected
	}
}
