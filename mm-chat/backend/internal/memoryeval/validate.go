package memoryeval

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func validateGoldenSet(value GoldenSet) error {
	if value.SchemaVersion != GoldenSchemaVersion ||
		!validText(value.ID, 128) ||
		!validText(value.Description, 4096) ||
		len(value.Cases) == 0 {
		return errors.New("Memory Golden corpus header is invalid")
	}
	if value.PromotionEligible == nil || *value.PromotionEligible {
		return errors.New("Memory Golden corpus cannot claim promotion eligibility")
	}
	if !value.DataPolicy.SyntheticOnly ||
		value.DataPolicy.ContainsRealUserData ||
		value.DataPolicy.ContainsSensitiveData {
		return errors.New("Memory Golden corpus data policy is invalid")
	}
	if value.FixtureManifestSHA256 != "" && !validSHA256(value.FixtureManifestSHA256) {
		return errors.New("Memory Golden corpus fixture hash is invalid")
	}
	if value.Lifecycle.State != "draft" && value.Lifecycle.State != "frozen" {
		return errors.New("Memory Golden corpus lifecycle is invalid")
	}
	if value.Lifecycle.State == "draft" &&
		(value.Lifecycle.FrozenAt != "" || value.Lifecycle.HoldoutRunID != "" ||
			value.Lifecycle.FrozenContentSHA256 != "") {
		return errors.New("draft Memory Golden corpus contains frozen lifecycle fields")
	}
	if value.Lifecycle.FrozenAt != "" {
		if _, err := parseTimestamp(value.Lifecycle.FrozenAt); err != nil {
			return errors.New("Memory Golden corpus frozenAt is invalid")
		}
	}
	if value.Lifecycle.HoldoutRunID != "" && !validUUID(value.Lifecycle.HoldoutRunID) {
		return errors.New("Memory Golden corpus Holdout run id is invalid")
	}
	if value.Lifecycle.FrozenContentSHA256 != "" &&
		!validSHA256(value.Lifecycle.FrozenContentSHA256) {
		return errors.New("Memory Golden corpus frozen hash is invalid")
	}
	if err := validateCriteria(value.Criteria); err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(value.Cases))
	for _, item := range value.Cases {
		if err := validateGoldenCase(item); err != nil {
			return err
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("duplicate Memory Golden case %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return nil
}

func validateCriteria(value Criteria) error {
	if value.MinimumCandidateRecallAt20 != 0.95 ||
		value.MinimumFinalRecallAt5 != 0.90 ||
		value.MinimumCurrentFactAccuracy != 0.95 ||
		value.MaximumFalseInjectionRate != 0.02 ||
		value.MaximumP95LatencyMilliseconds != 900 ||
		value.MaximumP99LatencyMilliseconds != 1500 ||
		value.HardCutoffMilliseconds != 2000 ||
		value.MaximumAveragePromptMemoryTokens != 600 ||
		value.MaximumPromptMemoryTokens != 900 ||
		value.MaximumProviderCostRatio != 0.15 {
		return errors.New("Memory Golden corpus criteria do not match v1")
	}
	for _, rate := range []float64{
		value.MinimumCandidateRecallAt20,
		value.MinimumFinalRecallAt5,
		value.MinimumCurrentFactAccuracy,
		value.MaximumFalseInjectionRate,
		value.MaximumProviderCostRatio,
	} {
		if !validRate(rate) {
			return errors.New("Memory Golden corpus criteria are invalid")
		}
	}
	if math.IsNaN(value.MaximumAveragePromptMemoryTokens) ||
		math.IsInf(value.MaximumAveragePromptMemoryTokens, 0) {
		return errors.New("Memory Golden corpus criteria are invalid")
	}
	return nil
}

func validateGoldenCase(item GoldenCase) error {
	if !validOpaqueID(item.ID) || !validText(item.Query, 4000) ||
		!validOpaqueID(item.FixtureAlias) || !validOpaqueID(item.Scope.UserAlias) ||
		len(item.Slices) == 0 || len(item.Slices) > len(criticalSlices) {
		return fmt.Errorf("Memory Golden case %q is invalid", item.ID)
	}
	if item.Scope.ProjectAlias != "" && !validOpaqueID(item.Scope.ProjectAlias) {
		return fmt.Errorf("Memory Golden case %q project scope is invalid", item.ID)
	}
	if item.Scope.ConversationAlias != "" && !validOpaqueID(item.Scope.ConversationAlias) {
		return fmt.Errorf("Memory Golden case %q conversation scope is invalid", item.ID)
	}
	if item.Split != "development" && item.Split != "validation" && item.Split != "holdout" {
		return fmt.Errorf("Memory Golden case %q split is invalid", item.ID)
	}
	if item.Language != "zh" && item.Language != "en" && item.Language != "mixed" {
		return fmt.Errorf("Memory Golden case %q language is invalid", item.ID)
	}
	if hasBlankOrDuplicate(item.Slices) || !containsOnlyCriticalSlices(item.Slices) {
		return fmt.Errorf("Memory Golden case %q slices are invalid", item.ID)
	}
	if len(item.ExpectedRelevantMemoryIDs) > 5 ||
		len(item.ExpectedCurrentMemoryIDs) > 5 || len(item.Exclusions) > 100 ||
		hasInvalidOrDuplicateIDs(item.ExpectedRelevantMemoryIDs) ||
		hasInvalidOrDuplicateIDs(item.ExpectedCurrentMemoryIDs) {
		return fmt.Errorf("Memory Golden case %q expectations are invalid", item.ID)
	}
	if item.ExpectedNoMemory == (len(item.ExpectedRelevantMemoryIDs) > 0) {
		return fmt.Errorf("Memory Golden case %q relevance expectation is inconsistent", item.ID)
	}
	relevant := stringSet(item.ExpectedRelevantMemoryIDs)
	for _, current := range item.ExpectedCurrentMemoryIDs {
		if _, ok := relevant[current]; !ok {
			return fmt.Errorf("Memory Golden case %q current fact is not relevant", item.ID)
		}
	}
	excluded := make(map[string]string, len(item.Exclusions))
	for _, exclusion := range item.Exclusions {
		if !validOpaqueID(exclusion.MemoryID) {
			return fmt.Errorf("Memory Golden case %q exclusion id is invalid", item.ID)
		}
		if _, ok := exclusionReasons[exclusion.Reason]; !ok {
			return fmt.Errorf("Memory Golden case %q exclusion reason is invalid", item.ID)
		}
		if _, duplicate := excluded[exclusion.MemoryID]; duplicate {
			return fmt.Errorf("Memory Golden case %q repeats an exclusion", item.ID)
		}
		if _, overlaps := relevant[exclusion.MemoryID]; overlaps {
			return fmt.Errorf("Memory Golden case %q includes and excludes one Memory", item.ID)
		}
		excluded[exclusion.MemoryID] = exclusion.Reason
	}
	if err := validateSliceSemantics(item, excluded); err != nil {
		return err
	}
	if item.Review.State != "draft" && item.Review.State != "human_reviewed" {
		return fmt.Errorf("Memory Golden case %q review is invalid", item.ID)
	}
	if item.Review.State == "draft" {
		if item.Review.ReviewerID != "" || item.Review.ReviewedAt != "" {
			return fmt.Errorf("Memory Golden case %q draft review contains attestation", item.ID)
		}
		return nil
	}
	if !validUUID(item.Review.ReviewerID) {
		return fmt.Errorf("Memory Golden case %q reviewer identity is invalid", item.ID)
	}
	if _, err := parseTimestamp(item.Review.ReviewedAt); err != nil {
		return fmt.Errorf("Memory Golden case %q review timestamp is invalid", item.ID)
	}
	return nil
}

func validateSliceSemantics(item GoldenCase, excluded map[string]string) error {
	hasSlice := func(name string) bool {
		_, ok := stringSet(item.Slices)[name]
		return ok
	}
	hasReason := func(reason string) bool {
		for _, actual := range excluded {
			if actual == reason {
				return true
			}
		}
		return false
	}
	invalid := func(message string) error {
		return fmt.Errorf("Memory Golden case %q %s", item.ID, message)
	}
	if hasSlice("chinese_paraphrase") && item.Language == "en" {
		return invalid("Chinese slice language is invalid")
	}
	if hasSlice("mixed_language_entity") && item.Language != "mixed" {
		return invalid("mixed-language slice is invalid")
	}
	if hasSlice("project_decision") && item.Scope.ProjectAlias == "" {
		return invalid("project decision has no Project scope")
	}
	if hasSlice("temporal_correction") &&
		(len(item.ExpectedCurrentMemoryIDs) == 0 || !hasReason("superseded")) {
		return invalid("temporal correction lacks current/superseded evidence")
	}
	if hasSlice("unrelated_negative") && !item.ExpectedNoMemory {
		return invalid("unrelated negative expects Memory")
	}
	for slice, reason := range map[string]string{
		"untrusted_source": "untrusted_source",
		"secret_rejection": "secret",
		"deletion":         "deleted",
	} {
		if hasSlice(slice) && !hasReason(reason) {
			return invalid(slice + " lacks a matching exclusion")
		}
	}
	if hasSlice("scope_isolation") &&
		!hasReason("cross_user") && !hasReason("out_of_scope") {
		return invalid("scope isolation lacks an authority exclusion")
	}
	return nil
}

func validateGoldenAdmission(value GoldenSet) error {
	if value.Lifecycle.State != "frozen" || value.Lifecycle.FrozenAt == "" ||
		!validUUID(value.Lifecycle.HoldoutRunID) ||
		!validSHA256(value.Lifecycle.FrozenContentSHA256) ||
		!validSHA256(value.FixtureManifestSHA256) {
		return errors.New("Memory Golden corpus is not frozen")
	}
	digest, err := GoldenContentSHA256(value)
	if err != nil {
		return err
	}
	if digest != value.Lifecycle.FrozenContentSHA256 {
		return errors.New("Memory Golden corpus frozen hash does not match")
	}
	if len(value.Cases) != requiredBenchmarkCases {
		return fmt.Errorf(
			"Memory Golden corpus has %d cases; exactly %d are required",
			len(value.Cases),
			requiredBenchmarkCases,
		)
	}
	splits := splitCounts(value.Cases)
	if splits["development"] != 300 ||
		splits["validation"] != 100 ||
		splits["holdout"] != 100 {
		return errors.New("Memory Golden corpus must use an exact 300/100/100 split")
	}
	for _, name := range criticalSlices {
		if countSlice(value.Cases, name) < requiredCriticalSliceCases {
			return fmt.Errorf("Memory Golden corpus slice %q has fewer than %d cases", name, requiredCriticalSliceCases)
		}
		counts := countSliceBySplit(value.Cases, name)
		if counts["development"] < 30 ||
			counts["validation"] < 10 ||
			counts["holdout"] < 10 {
			return fmt.Errorf(
				"Memory Golden corpus slice %q is not split across Development/Validation/Holdout",
				name,
			)
		}
	}
	frozenAt, _ := parseTimestamp(value.Lifecycle.FrozenAt)
	for _, item := range value.Cases {
		if item.Review.State != "human_reviewed" {
			return fmt.Errorf("Memory Golden case %q is not human-reviewed", item.ID)
		}
		reviewedAt, _ := parseTimestamp(item.Review.ReviewedAt)
		if reviewedAt.After(frozenAt) {
			return fmt.Errorf("Memory Golden case %q was reviewed after corpus freeze", item.ID)
		}
	}
	return nil
}

func validateObservationSet(value ObservationSet) error {
	if value.SchemaVersion != ObservationSchemaVersion ||
		!validText(value.GoldenSetID, 128) ||
		!validSHA256(value.GoldenCorpusSHA256) ||
		!validSHA256(value.FixtureManifestSHA256) ||
		!validUUID(value.CaptureID) || len(value.Cases) == 0 {
		return errors.New("Memory observation header is invalid")
	}
	if _, err := parseTimestamp(value.CapturedAt); err != nil {
		return errors.New("Memory observation capturedAt is invalid")
	}
	if !validUUID(value.HoldoutRun.ID) || value.HoldoutRun.Ordinal < 1 {
		return errors.New("Memory observation Holdout run is invalid")
	}
	if _, err := parseTimestamp(value.HoldoutRun.ExecutedAt); err != nil {
		return errors.New("Memory observation Holdout timestamp is invalid")
	}
	return validateObservationPayload(value.Profile, value.Costs, value.Cases)
}

func validateObservationPayload(
	profile Profile,
	costs ProviderCosts,
	cases []CaseObservation,
) error {
	if !validText(profile.ID, 128) ||
		!validText(profile.ReaderVersion, 128) ||
		!validSHA256(profile.ConfigurationSHA256) ||
		(profile.Role != "baseline" && profile.Role != "candidate" &&
			profile.Role != "shadow") ||
		profile.CandidateLimit != 20 || profile.FinalLimit != 5 {
		return errors.New("Memory observation profile is invalid")
	}
	if !validText(costs.Unit, 32) || costs.ChatProviderCostMicrounits == 0 {
		return errors.New("Memory observation provider costs are invalid")
	}
	seen := make(map[string]struct{}, len(cases))
	for _, item := range cases {
		if !validOpaqueID(item.CaseID) || item.LatencyMilliseconds < 0 ||
			item.PromptMemoryTokens < 0 {
			return fmt.Errorf("Memory observation case %q is invalid", item.CaseID)
		}
		if _, duplicate := seen[item.CaseID]; duplicate {
			return fmt.Errorf("duplicate Memory observation case %q", item.CaseID)
		}
		seen[item.CaseID] = struct{}{}
		if len(item.CandidateMemoryIDs) > profile.CandidateLimit ||
			len(item.FinalMemoryIDs) > profile.FinalLimit ||
			len(item.InjectedMemoryIDs) > profile.FinalLimit ||
			len(item.PersistedMemoryIDs) > 100 || len(item.ProviderSentMemoryIDs) > 100 ||
			hasInvalidOrDuplicateIDs(item.CandidateMemoryIDs) ||
			hasInvalidOrDuplicateIDs(item.FinalMemoryIDs) ||
			hasInvalidOrDuplicateIDs(item.InjectedMemoryIDs) ||
			hasInvalidOrDuplicateIDs(item.PersistedMemoryIDs) ||
			hasInvalidOrDuplicateIDs(item.ProviderSentMemoryIDs) {
			return fmt.Errorf("Memory observation case %q identifiers are invalid", item.CaseID)
		}
		if !isSubset(item.FinalMemoryIDs, item.CandidateMemoryIDs) ||
			!isSubset(item.InjectedMemoryIDs, item.FinalMemoryIDs) {
			return fmt.Errorf("Memory observation case %q ranking stages are inconsistent", item.CaseID)
		}
		switch item.Fallback {
		case "none", "exact_bm25", "lexical_v1", "no_memory":
		default:
			return fmt.Errorf("Memory observation case %q fallback is invalid", item.CaseID)
		}
	}
	return nil
}

func containsOnlyCriticalSlices(values []string) bool {
	allowed := stringSet(criticalSlices[:])
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func validOpaqueID(value string) bool {
	return validText(value, 128) && !strings.ContainsAny(value, "\r\n\t")
}

func validText(value string, maximumRunes int) bool {
	if strings.TrimSpace(value) != value || value == "" ||
		!utf8.ValidString(value) || utf8.RuneCountInString(value) > maximumRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validUUID(value string) bool {
	if len(value) != 36 || strings.ToLower(value) != value {
		return false
	}
	for index, character := range value {
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdef", character) {
				return false
			}
		}
	}
	return true
}

func parseTimestamp(value string) (time.Time, error) {
	if strings.TrimSpace(value) != value || value == "" {
		return time.Time{}, errors.New("timestamp is blank or padded")
	}
	return time.Parse(time.RFC3339, value)
}

func validRate(value float64) bool {
	return value >= 0 && value <= 1 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hasBlankOrDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, duplicate := seen[value]; duplicate {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func hasInvalidOrDuplicateIDs(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validOpaqueID(value) {
			return true
		}
		if _, duplicate := seen[value]; duplicate {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func isSubset(values, allowedValues []string) bool {
	allowed := stringSet(allowedValues)
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func splitCounts(cases []GoldenCase) map[string]int {
	counts := map[string]int{"development": 0, "validation": 0, "holdout": 0}
	for _, item := range cases {
		counts[item.Split]++
	}
	return counts
}

func countSlice(cases []GoldenCase, name string) int {
	count := 0
	for _, item := range cases {
		for _, actual := range item.Slices {
			if actual == name {
				count++
				break
			}
		}
	}
	return count
}

func countSliceBySplit(cases []GoldenCase, name string) map[string]int {
	counts := map[string]int{"development": 0, "validation": 0, "holdout": 0}
	for _, item := range cases {
		for _, actual := range item.Slices {
			if actual == name {
				counts[item.Split]++
				break
			}
		}
	}
	return counts
}
