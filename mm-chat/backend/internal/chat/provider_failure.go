package chat

import (
	"context"
	"errors"
	"net/http"
)

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

type providerFailureError struct {
	category ProviderFailureCategory
	message  string
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
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		return ProviderFailureRequestRejected
	default:
		return ProviderFailureUpstreamFailed
	}
}
