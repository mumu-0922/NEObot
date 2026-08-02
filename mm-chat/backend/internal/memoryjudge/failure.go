package memoryjudge

import (
	"errors"
	"sort"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	FailureTaxonomyVersion = "memory-candidate-judge-failure-taxonomy-v1"
	FailureTaxonomySHA256  = "c22cb137da8b5fda87526237446519dd9abe2c8d221ad703c5445358d9059f8d"

	FailureInputInvalid          = "CANDIDATE_JUDGE_INPUT_INVALID"
	FailureOutputTooLarge        = "CANDIDATE_JUDGE_OUTPUT_TOO_LARGE"
	FailureEventInvalid          = "CANDIDATE_JUDGE_EVENT_INVALID"
	FailureOutputJSONInvalid     = "CANDIDATE_JUDGE_OUTPUT_JSON_INVALID"
	FailureOutputSchemaInvalid   = "CANDIDATE_JUDGE_OUTPUT_SCHEMA_INVALID"
	FailureOutputOrdinalInvalid  = "CANDIDATE_JUDGE_OUTPUT_ORDINAL_INVALID"
	FailureProvenanceDrift       = "CANDIDATE_JUDGE_PROVENANCE_DRIFT"
	FailureRecorderStateConflict = "CANDIDATE_JUDGE_RECORDER_STATE_CONFLICT"
	FailureUnclassified          = "CANDIDATE_JUDGE_FAILURE_UNCLASSIFIED"
)

var localFailureCategories = map[string]struct{}{
	FailureInputInvalid:          {},
	FailureOutputTooLarge:        {},
	FailureEventInvalid:          {},
	FailureOutputJSONInvalid:     {},
	FailureOutputSchemaInvalid:   {},
	FailureOutputOrdinalInvalid:  {},
	FailureProvenanceDrift:       {},
	FailureRecorderStateConflict: {},
	FailureUnclassified:          {},
}

type failureError struct {
	category string
	cause    error
}

func (failure *failureError) Error() string {
	return "Memory candidate judge failed"
}

func (failure *failureError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

// NewFailure returns a typed, plaintext-free Judge-local failure. Invalid
// categories collapse to the fixed unclassified value.
func NewFailure(category string, cause error) error {
	if !ValidFailureCategory(category) {
		category = FailureUnclassified
	}
	return &failureError{category: category, cause: cause}
}

// FailureCategory classifies a failed Provider/adapter/capture boundary. It
// preserves the canonical Provider taxonomy and never parses error text.
func FailureCategory(err error) string {
	if err == nil {
		return ""
	}
	var failure *failureError
	if errors.As(err, &failure) && failure != nil &&
		ValidFailureCategory(failure.category) {
		return failure.category
	}
	if category, ok := chat.ProviderFailureCategoryOf(err); ok &&
		chat.ValidProviderFailureCategory(category) {
		return string(category)
	}
	if kind, ok := usermemory.HybridCandidateJudgeOutputErrorKindOf(err); ok {
		switch kind {
		case usermemory.HybridCandidateJudgeOutputJSONInvalid:
			return FailureOutputJSONInvalid
		case usermemory.HybridCandidateJudgeOutputSchemaInvalid:
			return FailureOutputSchemaInvalid
		case usermemory.HybridCandidateJudgeOutputOrdinalInvalid:
			return FailureOutputOrdinalInvalid
		}
	}
	return FailureUnclassified
}

func ValidFailureCategory(category string) bool {
	if _, ok := localFailureCategories[category]; ok {
		return true
	}
	return chat.ValidProviderFailureCategory(chat.ProviderFailureCategory(category))
}

// AttemptFailureCategory reports whether the category can originate inside a
// Provider/adapter attempt. Provenance and Recorder conflicts occur only after
// or outside that attempt and are reconciled separately.
func AttemptFailureCategory(category string) bool {
	if !ValidFailureCategory(category) {
		return false
	}
	return category != FailureProvenanceDrift &&
		category != FailureRecorderStateConflict
}

func FailureCategories() []string {
	categories := make([]string, 0, len(localFailureCategories)+len(chat.ProviderFailureCategories()))
	for category := range localFailureCategories {
		categories = append(categories, category)
	}
	for _, category := range chat.ProviderFailureCategories() {
		categories = append(categories, string(category))
	}
	sort.Strings(categories)
	return categories
}
