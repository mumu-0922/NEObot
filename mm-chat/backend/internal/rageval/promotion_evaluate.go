package rageval

import (
	"errors"
	"fmt"
	"sort"
)

const (
	minimumPromotionCases          = 500
	minimumPromotionSliceCases     = 50
	minimumRecallAt50              = 0.95
	minimumFinalRecallAt10         = 0.90
	minimumNDCGAt10                = 0.85
	minimumMRRAt10                 = 0.80
	minimumCitationCorrectness     = 0.95
	minimumCitationCompleteness    = 0.90
	minimumFaithfulness            = 0.95
	minimumAnswerCorrectness       = 0.95
	maximumNoAnswerFalseAnswerRate = 0.02
	minimumTableExactAnswer        = 0.95
)

func EvaluatePromotion(input PromotionEvaluationInput) (PromotionGateReport, error) {
	if err := validatePromotionGoldenSet(input.Golden); err != nil {
		return PromotionGateReport{}, err
	}
	if err := validatePromotionAdmission(input.Golden); err != nil {
		return PromotionGateReport{}, err
	}
	if err := validatePromotionObservationSet(input.Candidate); err != nil {
		return PromotionGateReport{}, err
	}
	if err := validatePromotionBindings(input); err != nil {
		return PromotionGateReport{}, err
	}

	candidate, err := evaluatePromotionProfile(input.Golden, input.Candidate)
	if err != nil {
		return PromotionGateReport{}, err
	}

	splitCounts := promotionSplitCounts(input.Golden.Cases)
	slices := make(map[string]PromotionSliceResult, len(criticalPromotionSlices))
	failures := make([]string, 0)
	for _, name := range criticalPromotionSlices {
		sliceMetrics := candidate.sliceMetrics[name]
		sliceIntegrity := promotionSliceIntegrity(
			input.Golden.Cases,
			input.Candidate.Cases,
			name,
		)
		sliceFailures := PromotionAbsoluteFailures(sliceMetrics)
		if !sliceIntegrity.Passed {
			sliceFailures = append(
				sliceFailures,
				"citation/locator integrity is not complete",
			)
		}
		sort.Strings(sliceFailures)
		slices[name] = PromotionSliceResult{
			Metrics:   sliceMetrics,
			Integrity: sliceIntegrity,
			Cases:     promotionSliceCount(input.Golden.Cases, name),
			Passed:    len(sliceFailures) == 0,
			Failures:  sliceFailures,
		}
		for _, failure := range sliceFailures {
			failures = append(failures, name+": "+failure)
		}
	}

	failures = append(failures, PromotionAbsoluteFailures(candidate.metrics)...)
	latencyPassed := candidate.p95Latency <=
		input.Golden.Criteria.MaximumP95LatencyMilliseconds
	contextPassed := candidate.averageContextTokens <=
		input.Golden.Criteria.MaximumAverageContextTokens
	if !latencyPassed {
		failures = append(failures, "candidate P95 latency exceeds the frozen budget")
	}
	if !contextPassed {
		failures = append(
			failures,
			"candidate average context tokens exceed the frozen budget",
		)
	}
	integrity := PromotionIntegrity{
		Passed: candidate.metrics.ProvenanceCellLineage == 1 &&
			candidate.metrics.CitationCorrectness >= minimumCitationCorrectness,
		CitationLocatorRate: candidateMetricRate(
			input.Candidate.Cases,
			func(item PromotionCaseObservation) bool {
				return item.Integrity.CitationLocatorValid
			},
		),
		ProvenanceRate: candidateMetricRate(
			input.Candidate.Cases,
			func(item PromotionCaseObservation) bool {
				return item.Integrity.ProvenanceValid
			},
		),
		CellLineageRate: candidateMetricRate(
			input.Candidate.Cases,
			func(item PromotionCaseObservation) bool {
				return item.Integrity.CellLineageValid
			},
		),
	}
	if integrity.CitationLocatorRate != 1 ||
		integrity.ProvenanceRate != 1 ||
		integrity.CellLineageRate != 1 {
		integrity.Passed = false
		failures = append(failures, "citation/locator integrity is not complete")
	}

	sort.Strings(failures)
	report := PromotionGateReport{
		SchemaVersion:         PromotionGateReportSchemaVersion,
		CandidateGenerationID: input.CandidateGenerationID,
		ArtifactManifestHash:  input.CandidateArtifactManifestHash,
		Passed:                len(failures) == 0,
		Evaluation: PromotionEvaluationProvenance{
			EvaluatorVersion:            PromotionEvaluatorVersion,
			GoldenCorpusRawSHA256:       input.GoldenRawSHA256,
			GoldenFrozenContentSHA256:   input.Golden.Lifecycle.FrozenContentSHA256,
			CandidateObservationsSHA256: input.CandidateRawSHA256,
			CandidateCaptureID:          input.Candidate.CaptureID,
			HoldoutRunID:                input.Candidate.HoldoutRun.ID,
		},
		Golden: PromotionGoldenSummary{
			CorpusID:         input.Golden.ID,
			State:            input.Golden.Lifecycle.State,
			FrozenAt:         input.Golden.Lifecycle.FrozenAt,
			TotalReviewed:    len(input.Golden.Cases),
			DevelopmentCount: splitCounts["development"],
			ValidationCount:  splitCounts["validation"],
			HoldoutCount:     splitCounts["holdout"],
			HoldoutRuns:      input.Candidate.HoldoutRun.Ordinal,
		},
		Slices:  slices,
		Metrics: candidate.metrics,
		Budgets: PromotionBudgets{
			CandidateP95LatencyMilliseconds: candidate.p95Latency,
			MaximumP95LatencyMilliseconds:   input.Golden.Criteria.MaximumP95LatencyMilliseconds,
			CandidateAverageContextTokens:   candidate.averageContextTokens,
			MaximumAverageContextTokens:     input.Golden.Criteria.MaximumAverageContextTokens,
			LatencyPassed:                   latencyPassed,
			ContextTokenCostPassed:          contextPassed,
		},
		Integrity: integrity,
		Failures:  failures,
	}
	return report, nil
}

// PromotionAbsoluteFailures applies the single frozen Candidate-only metric
// contract used by Development, Validation, and Holdout evaluation.
func PromotionAbsoluteFailures(metrics PromotionMetrics) []string {
	failures := make([]string, 0)
	appendMinimumFailure := func(actual, minimum float64, message string) {
		if actual < minimum {
			failures = append(failures, message)
		}
	}
	appendMinimumFailure(metrics.RecallAt50, minimumRecallAt50,
		"recall@50 below promotion threshold")
	appendMinimumFailure(metrics.FinalRecallAt10, minimumFinalRecallAt10,
		"final recall@10 below promotion threshold")
	appendMinimumFailure(metrics.NDCGAt10, minimumNDCGAt10,
		"nDCG@10 below promotion threshold")
	appendMinimumFailure(metrics.MRRAt10, minimumMRRAt10,
		"MRR@10 below promotion threshold")
	appendMinimumFailure(metrics.CitationCorrectness, minimumCitationCorrectness,
		"citation correctness below promotion threshold")
	appendMinimumFailure(metrics.CitationCompleteness, minimumCitationCompleteness,
		"citation completeness below promotion threshold")
	appendMinimumFailure(metrics.Faithfulness, minimumFaithfulness,
		"faithfulness below promotion threshold")
	appendMinimumFailure(metrics.AnswerCorrectness, minimumAnswerCorrectness,
		"answer correctness below promotion threshold")
	appendMinimumFailure(metrics.TableExactAnswer, minimumTableExactAnswer,
		"table exact-answer below promotion threshold")
	if metrics.NoAnswerFalseAnswerRate > maximumNoAnswerFalseAnswerRate {
		failures = append(
			failures,
			"no-answer false-answer rate above promotion threshold",
		)
	}
	if metrics.ProvenanceCellLineage != 1 {
		failures = append(failures, "provenance/cell lineage is not complete")
	}
	if metrics.ACLLeakCount != 0 ||
		metrics.DeletionLeakCount != 0 ||
		metrics.SecretLeakCount != 0 ||
		metrics.UnauthorizedEvidenceLeakCount != 0 {
		failures = append(failures, "security or authority leakage was observed")
	}
	return failures
}

func promotionSliceIntegrity(
	golden []PromotionGoldenCase,
	observations []PromotionCaseObservation,
	slice string,
) PromotionIntegrity {
	caseIDs := make(map[string]struct{})
	for _, item := range golden {
		for _, name := range item.Slices {
			if name == slice {
				caseIDs[item.ID] = struct{}{}
				break
			}
		}
	}
	selected := make([]PromotionCaseObservation, 0, len(caseIDs))
	for _, item := range observations {
		if _, ok := caseIDs[item.CaseID]; ok {
			selected = append(selected, item)
		}
	}
	return SummarizePromotionIntegrity(selected)
}

// SummarizePromotionIntegrity computes the exact Citation/locator integrity
// rates used by both preflight capture and the formal Holdout evaluator.
func SummarizePromotionIntegrity(
	selected []PromotionCaseObservation,
) PromotionIntegrity {
	integrity := PromotionIntegrity{
		CitationLocatorRate: candidateMetricRate(selected, func(
			item PromotionCaseObservation,
		) bool {
			return item.Integrity.CitationLocatorValid
		}),
		ProvenanceRate: candidateMetricRate(selected, func(
			item PromotionCaseObservation,
		) bool {
			return item.Integrity.ProvenanceValid
		}),
		CellLineageRate: candidateMetricRate(selected, func(
			item PromotionCaseObservation,
		) bool {
			return item.Integrity.CellLineageValid
		}),
	}
	integrity.Passed = integrity.CitationLocatorRate == 1 &&
		integrity.ProvenanceRate == 1 && integrity.CellLineageRate == 1
	return integrity
}

// ValidatePromotionGoldenAdmission exposes the closed frozen-corpus admission
// check to the capture command. Capture must reject an incomplete corpus before
// it spends provider calls or creates any evaluation artifact.
func ValidatePromotionGoldenAdmission(golden PromotionGoldenSet) error {
	if err := validatePromotionGoldenSet(golden); err != nil {
		return err
	}
	return validatePromotionAdmission(golden)
}

func validatePromotionAdmission(golden PromotionGoldenSet) error {
	if golden.Lifecycle.State != "frozen" ||
		golden.Lifecycle.FrozenAt == "" ||
		!validUUID(golden.Lifecycle.HoldoutRunID) ||
		golden.Lifecycle.FrozenContentSHA256 == "" {
		return errors.New("promotion Golden corpus is not frozen")
	}
	frozenAt, _ := parsePromotionTimestamp(golden.Lifecycle.FrozenAt)
	contentHash, err := PromotionGoldenContentSHA256(golden)
	if err != nil {
		return err
	}
	if contentHash != golden.Lifecycle.FrozenContentSHA256 {
		return errors.New("promotion Golden corpus frozen hash does not match")
	}
	if len(golden.Cases) < minimumPromotionCases {
		return fmt.Errorf(
			"promotion Golden corpus has %d cases; at least %d are required",
			len(golden.Cases),
			minimumPromotionCases,
		)
	}
	counts := promotionSplitCounts(golden.Cases)
	if counts["development"]*100 != len(golden.Cases)*60 ||
		counts["validation"]*100 != len(golden.Cases)*20 ||
		counts["holdout"]*100 != len(golden.Cases)*20 {
		return errors.New(
			"promotion Golden corpus must use an exact 60/20/20 split",
		)
	}
	for _, name := range criticalPromotionSlices {
		if promotionSliceCount(golden.Cases, name) < minimumPromotionSliceCases {
			return fmt.Errorf(
				"promotion Golden corpus slice %q has fewer than %d cases",
				name,
				minimumPromotionSliceCases,
			)
		}
	}
	tableCases := 0
	for _, item := range golden.Cases {
		if item.Review.State != "human_reviewed" ||
			!validUUID(item.Review.ReviewerID) ||
			item.Review.ReviewedAt == "" {
			return fmt.Errorf(
				"promotion Golden case %q is not human-reviewed",
				item.ID,
			)
		}
		reviewedAt, _ := parsePromotionTimestamp(item.Review.ReviewedAt)
		if reviewedAt.After(frozenAt) {
			return fmt.Errorf(
				"promotion Golden case %q was reviewed after corpus freeze",
				item.ID,
			)
		}
		if item.TableExactAnswerRequired {
			tableCases++
		}
	}
	if tableCases < minimumPromotionSliceCases {
		return fmt.Errorf(
			"promotion Golden corpus has fewer than %d table exact-answer cases",
			minimumPromotionSliceCases,
		)
	}
	return nil
}

func validatePromotionBindings(input PromotionEvaluationInput) error {
	if !validSHA256(input.GoldenRawSHA256) ||
		!validSHA256(input.CandidateRawSHA256) ||
		!validSHA256(input.CandidateArtifactManifestHash) ||
		!validUUID(input.CandidateGenerationID) {
		return errors.New("promotion evaluation hash or candidate binding is invalid")
	}
	if input.Candidate.ProfileRole != "candidate" {
		return errors.New("candidate observations have an incorrect profile role")
	}
	if input.Candidate.GoldenSetID != input.Golden.ID {
		return errors.New("promotion observation Golden set id does not match")
	}
	frozenHash := input.Golden.Lifecycle.FrozenContentSHA256
	if input.Candidate.GoldenCorpusSHA256 != frozenHash {
		return errors.New("candidate observations are not bound to the frozen corpus")
	}
	if input.Candidate.GenerationID != input.CandidateGenerationID ||
		input.Candidate.ArtifactManifestHash != input.CandidateArtifactManifestHash {
		return errors.New("candidate observations are not bound to the verified artifact")
	}
	if input.Candidate.HoldoutRun.ID != input.Golden.Lifecycle.HoldoutRunID ||
		input.Candidate.HoldoutRun.Ordinal != 1 {
		return errors.New("Holdout must have exactly one Candidate run")
	}
	frozenAt, _ := parsePromotionTimestamp(input.Golden.Lifecycle.FrozenAt)
	holdoutAt, _ := parsePromotionTimestamp(input.Candidate.HoldoutRun.ExecutedAt)
	candidateCapturedAt, _ := parsePromotionTimestamp(input.Candidate.CapturedAt)
	if holdoutAt.Before(frozenAt) ||
		holdoutAt.After(candidateCapturedAt) {
		return errors.New("Holdout run timestamp is outside the frozen capture window")
	}
	return nil
}
