package imagejobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type providerHTTPError struct {
	Stage      string
	StatusCode int
	ErrorCode  string
	ErrorType  string
}

func (e *providerHTTPError) Error() string {
	return fmt.Sprintf("openai-compatible image %s returned status %d", e.Stage, e.StatusCode)
}

func (e *providerHTTPError) Unwrap() error { return ErrImageProviderFailed }

type providerStageError struct {
	reason  string
	message string
	cause   error
}

func (e *providerStageError) Error() string { return e.message }

func (e *providerStageError) Unwrap() error { return e.cause }

func newProviderStageError(reason string, message string, cause error) error {
	return &providerStageError{reason: reason, message: message, cause: cause}
}

func FailureReason(err error) string {
	if errors.Is(err, ErrImageJobsUnavailable) {
		return "IMAGE_EXECUTOR_UNAVAILABLE"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "IMAGE_PROVIDER_TIMEOUT"
	}
	if errors.Is(err, context.Canceled) {
		return "IMAGE_PROVIDER_CANCELLED"
	}
	var httpErr *providerHTTPError
	if errors.As(err, &httpErr) {
		stage := "REQUEST"
		if httpErr.Stage == "image fetch" {
			stage = "FETCH"
		}
		reason := fmt.Sprintf("IMAGE_PROVIDER_%s_HTTP_%d", stage, httpErr.StatusCode)
		if httpErr.ErrorCode != "" {
			return reason + "_CODE_" + httpErr.ErrorCode
		}
		if httpErr.ErrorType != "" {
			return reason + "_TYPE_" + httpErr.ErrorType
		}
		return reason
	}
	var stageErr *providerStageError
	if errors.As(err, &stageErr) {
		return stageErr.reason
	}
	if errors.Is(err, ErrImageProviderFailed) {
		return "IMAGE_PROVIDER_REJECTED"
	}
	return "IMAGE_PROVIDER_RESPONSE_INVALID"
}

func IsContentPolicyViolation(err error) bool {
	var httpErr *providerHTTPError
	return errors.As(err, &httpErr) && httpErr.ErrorCode == "CONTENT_POLICY_VIOLATION"
}

func isRetryableImageProviderError(err error) bool {
	var httpErr *providerHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusRequestTimeout ||
			httpErr.StatusCode == http.StatusTooManyRequests ||
			httpErr.StatusCode >= http.StatusInternalServerError
	}
	var stageErr *providerStageError
	if !errors.As(err, &stageErr) {
		return false
	}
	switch stageErr.reason {
	case "IMAGE_PROVIDER_REQUEST_FAILED",
		"IMAGE_PROVIDER_RESPONSE_READ_FAILED",
		"IMAGE_PROVIDER_RESPONSE_DECODE_FAILED",
		"IMAGE_PROVIDER_RESPONSE_EMPTY",
		"IMAGE_PROVIDER_IMAGE_CONTENT_MISSING",
		"IMAGE_PROVIDER_FETCH_FAILED",
		"IMAGE_PROVIDER_FETCH_READ_FAILED",
		"IMAGE_PROVIDER_FETCH_EMPTY":
		return true
	default:
		return false
	}
}

func providerErrorIdentity(body []byte) (string, string) {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return "", ""
	}
	return normalizeProviderErrorLabel(envelope.Error.Code),
		normalizeProviderErrorLabel(envelope.Error.Type)
}

func normalizeProviderErrorLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var normalized strings.Builder
	for _, current := range value {
		if normalized.Len() >= 64 {
			break
		}
		switch {
		case current >= 'a' && current <= 'z':
			normalized.WriteRune(current - ('a' - 'A'))
		case current >= 'A' && current <= 'Z', current >= '0' && current <= '9':
			normalized.WriteRune(current)
		case current == '-', current == '.', current == '_':
			normalized.WriteByte('_')
		}
	}
	switch label := normalized.String(); label {
	case "CONTENT_POLICY_VIOLATION",
		"INVALID_API_KEY",
		"AUTHENTICATION_ERROR",
		"INVALID_REQUEST_ERROR",
		"RATE_LIMIT_EXCEEDED",
		"RATE_LIMIT_ERROR",
		"INSUFFICIENT_QUOTA",
		"SERVER_ERROR",
		"INTERNAL_SERVER_ERROR",
		"SERVICE_UNAVAILABLE":
		return label
	default:
		return "UNRECOGNIZED"
	}
}
