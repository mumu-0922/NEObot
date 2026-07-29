package memoryeval

import (
	"errors"
	"fmt"
)

func validateRegressionCorpus(value RegressionCorpus) error {
	if value.SchemaVersion != RegressionCorpusSchemaVersion ||
		!validText(value.ID, 128) || !validText(value.Description, 4096) ||
		value.CorpusClass != RegressionCorpusClass ||
		value.AdmissionMode != RegressionAdmissionMode ||
		value.PromotionEligible == nil || *value.PromotionEligible ||
		!value.DataPolicy.SyntheticOnly || value.DataPolicy.ContainsRealUserData ||
		value.DataPolicy.ContainsSensitiveData ||
		!validSHA256(value.FixtureManifestSHA256) ||
		!validSHA256(value.CorpusContentSHA256) || len(value.Cases) == 0 {
		return errors.New("Memory regression corpus header is invalid")
	}
	if value.MachineAudit.SchemaVersion != RegressionAuditSchemaVersion ||
		value.MachineAudit.Verdict != "passed" ||
		!validText(value.MachineAudit.Auditor, 128) ||
		!validSHA256(value.MachineAudit.ContentSHA256) {
		return errors.New("Memory regression corpus audit binding is invalid")
	}
	if _, err := parseTimestamp(value.MachineAudit.AuditedAt); err != nil {
		return errors.New("Memory regression corpus auditedAt is invalid")
	}
	if err := validateCriteria(value.Criteria); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(value.Cases))
	for _, item := range value.Cases {
		if err := validateGoldenCase(item); err != nil {
			return err
		}
		if item.Review.State != "draft" || item.Review.ReviewerID != "" ||
			item.Review.ReviewedAt != "" {
			return fmt.Errorf("Memory regression case %q contains human attestation", item.ID)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("duplicate Memory regression case %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	digest, err := RegressionCorpusContentSHA256(value)
	if err != nil {
		return err
	}
	if digest != value.CorpusContentSHA256 {
		return errors.New("Memory regression corpus content hash does not match")
	}
	return nil
}

func validateRegressionAudit(value RegressionAudit) error {
	if value.SchemaVersion != RegressionAuditSchemaVersion ||
		!validText(value.CorpusID, 128) ||
		value.CorpusClass != RegressionCorpusClass ||
		value.AdmissionMode != RegressionAdmissionMode ||
		value.PromotionEligible == nil || *value.PromotionEligible ||
		!validSHA256(value.CorpusContentSHA256) ||
		!validSHA256(value.FixtureManifestSHA256) ||
		!validText(value.Auditor, 128) || value.Verdict != "passed" ||
		!validSHA256(value.ContentSHA256) || value.CaseCount != requiredBenchmarkCases {
		return errors.New("Memory regression audit header is invalid")
	}
	if _, err := parseTimestamp(value.AuditedAt); err != nil {
		return errors.New("Memory regression audit auditedAt is invalid")
	}
	if value.SplitCounts != (RegressionSplitCounts{Development: 300, Validation: 100, Holdout: 100}) ||
		value.LanguageCounts != (RegressionLanguageCounts{Chinese: 350, Mixed: 100, English: 50}) {
		return errors.New("Memory regression audit distribution is invalid")
	}
	if len(value.SliceCounts) != len(criticalSlices) {
		return errors.New("Memory regression audit slice counts are incomplete")
	}
	for index, count := range value.SliceCounts {
		if count.Name != criticalSlices[index] || count.Total < requiredCriticalSliceCases ||
			count.Development < 30 || count.Validation < 10 || count.Holdout < 10 ||
			count.Total != count.Development+count.Validation+count.Holdout {
			return fmt.Errorf("Memory regression audit slice %q is invalid", count.Name)
		}
	}
	semantic := value.Semantic
	if semantic.QuerySkeletonCount < 100 ||
		semantic.NormalizedDuplicateCount != 0 ||
		semantic.OrdinalShortcutCount != 0 ||
		semantic.IdentifierShortcutCount != 0 ||
		semantic.FixtureBindingFailureCount != 0 ||
		semantic.SliceSemanticFailureCount != 0 ||
		semantic.LanguageMismatchCount != 0 ||
		semantic.ScopeTextMismatchCount != 0 ||
		semantic.PreferenceSemanticFailureCount != 0 ||
		semantic.FallbackSemanticFailureCount != 0 ||
		semantic.MultiHopSemanticFailureCount != 0 {
		return errors.New("Memory regression audit semantic gates failed")
	}
	digest, err := RegressionAuditContentSHA256(value)
	if err != nil {
		return err
	}
	if digest != value.ContentSHA256 {
		return errors.New("Memory regression audit content hash does not match")
	}
	return nil
}

func validateRegressionAdmission(corpus RegressionCorpus, audit RegressionAudit) error {
	if len(corpus.Cases) != requiredBenchmarkCases {
		return fmt.Errorf(
			"Memory regression corpus has %d cases; exactly %d are required",
			len(corpus.Cases),
			requiredBenchmarkCases,
		)
	}
	splits := splitCounts(corpus.Cases)
	if splits["development"] != 300 || splits["validation"] != 100 ||
		splits["holdout"] != 100 {
		return errors.New("Memory regression corpus must use an exact 300/100/100 split")
	}
	languages := map[string]int{}
	for _, item := range corpus.Cases {
		languages[item.Language]++
	}
	if languages["zh"] != 350 || languages["mixed"] != 100 || languages["en"] != 50 {
		return errors.New("Memory regression corpus must use an exact 350/100/50 language profile")
	}
	for _, name := range criticalSlices {
		if countSlice(corpus.Cases, name) < requiredCriticalSliceCases {
			return fmt.Errorf("Memory regression corpus slice %q has fewer than %d cases", name, requiredCriticalSliceCases)
		}
		counts := countSliceBySplit(corpus.Cases, name)
		if counts["development"] < 30 || counts["validation"] < 10 ||
			counts["holdout"] < 10 {
			return fmt.Errorf("Memory regression corpus slice %q lacks 30/10/10 coverage", name)
		}
	}
	if audit.CorpusID != corpus.ID ||
		audit.CorpusContentSHA256 != corpus.CorpusContentSHA256 ||
		audit.FixtureManifestSHA256 != corpus.FixtureManifestSHA256 ||
		audit.Auditor != corpus.MachineAudit.Auditor ||
		audit.AuditedAt != corpus.MachineAudit.AuditedAt ||
		audit.Verdict != corpus.MachineAudit.Verdict ||
		audit.ContentSHA256 != corpus.MachineAudit.ContentSHA256 {
		return errors.New("Memory regression corpus and audit binding does not match")
	}
	return nil
}

func validateRegressionObservationSet(value RegressionObservationSet) error {
	if value.SchemaVersion != RegressionObservationSchemaVersion ||
		!validText(value.CorpusID, 128) ||
		!validSHA256(value.CorpusContentSHA256) ||
		!validSHA256(value.AuditContentSHA256) ||
		!validSHA256(value.FixtureManifestSHA256) ||
		!validUUID(value.CaptureID) || len(value.Cases) == 0 {
		return errors.New("Memory regression observation header is invalid")
	}
	if _, err := parseTimestamp(value.CapturedAt); err != nil {
		return errors.New("Memory regression observation capturedAt is invalid")
	}
	return validateObservationPayload(value.Profile, value.Costs, value.Cases)
}
