package usermemory

import (
	"context"
	"errors"
	"sort"
)

const (
	HybridMemoryToolName                = "search_memory"
	HybridMemoryToolContractVersion     = "memory-search-tool-v1"
	HybridMemoryToolContractSHA256      = "f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6"
	HybridMemoryToolDecodingProfile     = "memory-search-tool-decoding-v1"
	HybridMemoryToolMaximumOutputTokens = 128
	HybridMemoryToolTemperature         = 0.0
	HybridMemoryToolDisableThinking     = true

	HybridMemoryToolRouteFailureContextDeadline       = "CONTEXT_DEADLINE"
	HybridMemoryToolRouteFailureContextCanceled       = "CONTEXT_CANCELED"
	HybridMemoryToolRouteFailureRequestBuild          = "PROVIDER_REQUEST_BUILD_FAILED"
	HybridMemoryToolRouteFailureTransport             = "PROVIDER_TRANSPORT_FAILED"
	HybridMemoryToolRouteFailureResponseInvalid       = "PROVIDER_RESPONSE_INVALID"
	HybridMemoryToolRouteFailureAuthentication        = "PROVIDER_AUTHENTICATION_FAILED"
	HybridMemoryToolRouteFailureQuotaExhausted        = "PROVIDER_QUOTA_EXHAUSTED"
	HybridMemoryToolRouteFailureRequestTimeout        = "PROVIDER_REQUEST_TIMEOUT"
	HybridMemoryToolRouteFailureRateLimited           = "PROVIDER_RATE_LIMITED"
	HybridMemoryToolRouteFailureRequestRejected       = "PROVIDER_REQUEST_REJECTED"
	HybridMemoryToolRouteFailureUpstream              = "PROVIDER_UPSTREAM_FAILED"
	HybridMemoryToolRouteFailureStreamParse           = "PROVIDER_STREAM_PARSE_FAILED"
	HybridMemoryToolRouteFailureStreamRead            = "PROVIDER_STREAM_READ_FAILED"
	HybridMemoryToolRouteFailureStreamIncomplete      = "PROVIDER_STREAM_INCOMPLETE"
	HybridMemoryToolRouteFailureStreamRemote          = "PROVIDER_STREAM_REMOTE_ERROR"
	HybridMemoryToolRouteFailureEvent                 = "PROVIDER_EVENT_FAILED"
	HybridMemoryToolRouteFailureNilStream             = "PROVIDER_STREAM_NIL"
	HybridMemoryToolRouteFailureInvalidEvent          = "PROVIDER_EVENT_INVALID"
	HybridMemoryToolRouteFailureInvalidCall           = "TOOL_CALL_INVALID"
	HybridMemoryToolRouteFailureRejectedCall          = "TOOL_CALL_REJECTED"
	HybridMemoryToolRouteFailureProvenanceDrift       = "PROVENANCE_DRIFT"
	HybridMemoryToolRouteFailureRecorderStateConflict = "RECORDER_STATE_CONFLICT"
	HybridMemoryToolRouteFailureUnclassified          = "ROUTER_FAILURE_UNCLASSIFIED"
)

var hybridMemoryToolRouteFailureCategories = map[string]struct{}{
	HybridMemoryToolRouteFailureContextDeadline:       {},
	HybridMemoryToolRouteFailureContextCanceled:       {},
	HybridMemoryToolRouteFailureRequestBuild:          {},
	HybridMemoryToolRouteFailureTransport:             {},
	HybridMemoryToolRouteFailureResponseInvalid:       {},
	HybridMemoryToolRouteFailureAuthentication:        {},
	HybridMemoryToolRouteFailureQuotaExhausted:        {},
	HybridMemoryToolRouteFailureRequestTimeout:        {},
	HybridMemoryToolRouteFailureRateLimited:           {},
	HybridMemoryToolRouteFailureRequestRejected:       {},
	HybridMemoryToolRouteFailureUpstream:              {},
	HybridMemoryToolRouteFailureStreamParse:           {},
	HybridMemoryToolRouteFailureStreamRead:            {},
	HybridMemoryToolRouteFailureStreamIncomplete:      {},
	HybridMemoryToolRouteFailureStreamRemote:          {},
	HybridMemoryToolRouteFailureEvent:                 {},
	HybridMemoryToolRouteFailureNilStream:             {},
	HybridMemoryToolRouteFailureInvalidEvent:          {},
	HybridMemoryToolRouteFailureInvalidCall:           {},
	HybridMemoryToolRouteFailureRejectedCall:          {},
	HybridMemoryToolRouteFailureProvenanceDrift:       {},
	HybridMemoryToolRouteFailureRecorderStateConflict: {},
	HybridMemoryToolRouteFailureUnclassified:          {},
}

type HybridMemoryToolRouteError struct {
	Category string
}

func (failure *HybridMemoryToolRouteError) Error() string {
	return "hybrid Memory Tool route failed"
}

func NewHybridMemoryToolRouteError(category string) error {
	if !ValidHybridMemoryToolRouteFailureCategory(category) {
		category = HybridMemoryToolRouteFailureUnclassified
	}
	return &HybridMemoryToolRouteError{Category: category}
}

func HybridMemoryToolRouteFailureCategory(err error) string {
	var failure *HybridMemoryToolRouteError
	if !errors.As(err, &failure) || failure == nil ||
		!ValidHybridMemoryToolRouteFailureCategory(failure.Category) {
		return ""
	}
	return failure.Category
}

func ValidHybridMemoryToolRouteFailureCategory(category string) bool {
	_, ok := hybridMemoryToolRouteFailureCategories[category]
	return ok
}

func HybridMemoryToolRouteFailureCategories() []string {
	categories := make([]string, 0, len(hybridMemoryToolRouteFailureCategories))
	for category := range hybridMemoryToolRouteFailureCategories {
		categories = append(categories, category)
	}
	sort.Strings(categories)
	return categories
}

// HybridMemoryToolRouter represents the current chat model's automatic Tool
// decision. The router receives only the already-secret-redacted query and
// the fixed Tool definition; Memory candidate bodies are never part of this
// boundary.
type HybridMemoryToolRouter interface {
	RouteHybridMemory(
		context.Context,
		HybridMemoryToolRouteInput,
	) (HybridMemoryToolRouteResult, error)
}

type HybridMemoryToolRouteInput struct {
	Query string
}

type HybridMemoryToolRouteResult struct {
	UseMemory             bool
	ModelID               string
	ContractVersion       string
	ContractSHA256        string
	OutputTokenUpperBound int
}
