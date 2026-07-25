package rageval

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const (
	promotionCandidateGenerationID = "50000000-0000-4000-8000-000000000001"
	promotionReviewerID            = "50000000-0000-4000-8000-000000000003"
	promotionHoldoutRunID          = "50000000-0000-4000-8000-000000000004"
	promotionCandidateCaptureID    = "50000000-0000-4000-8000-000000000006"
	promotionCandidateManifestHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestEvaluatePromotionProducesBoundPassingReport(t *testing.T) {
	input := passingPromotionInput(t)

	report, err := EvaluatePromotion(input)
	if err != nil {
		t.Fatalf("EvaluatePromotion() error = %v", err)
	}
	if !report.Passed || len(report.Failures) != 0 {
		t.Fatalf("report gate = %#v", report)
	}
	if report.Golden.TotalReviewed != 500 ||
		report.Golden.DevelopmentCount != 300 ||
		report.Golden.ValidationCount != 100 ||
		report.Golden.HoldoutCount != 100 ||
		report.Golden.HoldoutRuns != 1 {
		t.Fatalf("Golden summary = %#v", report.Golden)
	}
	if report.Evaluation.GoldenFrozenContentSHA256 !=
		input.Golden.Lifecycle.FrozenContentSHA256 ||
		report.Evaluation.CandidateObservationsSHA256 != input.CandidateRawSHA256 {
		t.Fatalf("evaluation provenance = %#v", report.Evaluation)
	}
	if report.Metrics.RecallAt50 != 1 ||
		report.Metrics.FinalRecallAt10 != 1 ||
		report.Metrics.NDCGAt10 != 1 ||
		report.Metrics.MRRAt10 != 1 ||
		report.Metrics.CitationCorrectness != 1 ||
		report.Metrics.CitationCompleteness != 1 ||
		report.Metrics.Faithfulness != 1 ||
		report.Metrics.TableExactAnswer != 1 ||
		report.Integrity.CitationLocatorRate != 1 {
		t.Fatalf("candidate metrics = %#v", report.Metrics)
	}
	for name, result := range report.Slices {
		if result.Cases != 50 || !result.Passed || len(result.Failures) != 0 ||
			result.Metrics.RecallAt50 != 1 || !result.Integrity.Passed {
			t.Fatalf("slice %s = %#v", name, result)
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, retiredField := range []string{
		`"activeMetrics"`, `"comparison"`, `"activeObservationsSha256"`,
	} {
		if strings.Contains(string(encoded), retiredField) {
			t.Fatalf("Candidate-only report contains %s: %s", retiredField, encoded)
		}
	}
}

func TestEvaluatePromotionIgnoresLegacyRelativeImprovementCriterion(t *testing.T) {
	input := passingPromotionInput(t)
	input.Golden.Criteria.MinimumAggregateQualityImprovement = 0
	refreezePromotionGolden(t, &input.Golden)
	input.Candidate.GoldenCorpusSHA256 = input.Golden.Lifecycle.FrozenContentSHA256

	report, err := EvaluatePromotion(input)
	if err != nil || !report.Passed {
		t.Fatalf("Candidate-only EvaluatePromotion() = %#v, %v", report, err)
	}
}

func TestDecodePromotionRejectsDuplicateJSONKeys(t *testing.T) {
	_, err := DecodePromotionGoldenSet(strings.NewReader(`{
  "schemaVersion":"neo-chat.rag-promotion-golden.v1",
  "schemaVersion":"shadowed"
}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("DecodePromotionGoldenSet() error = %v", err)
	}
}

func TestEvaluatePromotionRejectsIncompleteGoldenAdmission(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*PromotionEvaluationInput)
		message string
	}{
		{
			name: "499 cases",
			mutate: func(input *PromotionEvaluationInput) {
				input.Golden.Cases = input.Golden.Cases[:499]
				input.Candidate.Cases = input.Candidate.Cases[:499]
				refreezePromotionGolden(t, &input.Golden)
			},
			message: "at least 500",
		},
		{
			name: "critical slice below 50",
			mutate: func(input *PromotionEvaluationInput) {
				input.Golden.Cases[0].Slices = []string{"non_critical_draft_slice"}
				refreezePromotionGolden(t, &input.Golden)
			},
			message: `slice "pdf" has fewer than 50`,
		},
		{
			name: "split counts are not exact",
			mutate: func(input *PromotionEvaluationInput) {
				input.Golden.Cases[0].Split = "validation"
				refreezePromotionGolden(t, &input.Golden)
			},
			message: "exact 60/20/20 split",
		},
		{
			name: "draft case",
			mutate: func(input *PromotionEvaluationInput) {
				input.Golden.Cases[0].Review = PromotionReview{State: "draft"}
				refreezePromotionGolden(t, &input.Golden)
			},
			message: "is not human-reviewed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := passingPromotionInput(t)
			test.mutate(&input)
			_, err := EvaluatePromotion(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("EvaluatePromotion() error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestEvaluatePromotionRejectsFrozenHashDrift(t *testing.T) {
	input := passingPromotionInput(t)
	input.Golden.Cases[0].Query = "changed after freeze"

	_, err := EvaluatePromotion(input)
	if err == nil || !strings.Contains(err.Error(), "frozen hash does not match") {
		t.Fatalf("EvaluatePromotion() error = %v", err)
	}
}

func TestEvaluatePromotionRejectsMissingOrRepeatedHoldout(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*PromotionEvaluationInput)
	}{
		{
			name: "missing",
			mutate: func(input *PromotionEvaluationInput) {
				input.Candidate.HoldoutRun.ID = ""
			},
		},
		{
			name: "rerun",
			mutate: func(input *PromotionEvaluationInput) {
				input.Candidate.HoldoutRun.Ordinal = 2
			},
		},
		{
			name: "not precommitted",
			mutate: func(input *PromotionEvaluationInput) {
				input.Candidate.HoldoutRun.ID = "50000000-0000-4000-8000-000000000099"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := passingPromotionInput(t)
			test.mutate(&input)
			if _, err := EvaluatePromotion(input); err == nil {
				t.Fatal("EvaluatePromotion() error = nil")
			}
		})
	}
}

func TestEvaluatePromotionFailsLocatorProvenanceAndCellLineage(t *testing.T) {
	input := passingPromotionInput(t)
	input.Candidate.Cases[0].Integrity = PromotionCaseIntegrity{}

	report, err := EvaluatePromotion(input)
	if err != nil {
		t.Fatalf("EvaluatePromotion() error = %v", err)
	}
	if report.Passed || report.Integrity.Passed ||
		report.Integrity.CitationLocatorRate == 1 ||
		report.Integrity.ProvenanceRate == 1 ||
		report.Integrity.CellLineageRate == 1 {
		t.Fatalf("integrity gate = %#v", report.Integrity)
	}
	if !strings.Contains(strings.Join(report.Failures, "\n"), "integrity") {
		t.Fatalf("failures = %#v", report.Failures)
	}
}

func TestEvaluatePromotionFailsCriticalSliceAbsoluteGate(t *testing.T) {
	input := passingPromotionInput(t)
	for index := 0; index < 50; index++ {
		input.Candidate.Cases[index].AnswerCorrectness = 0
	}

	report, err := EvaluatePromotion(input)
	if err != nil {
		t.Fatalf("EvaluatePromotion() error = %v", err)
	}
	if report.Passed || report.Slices["pdf"].Passed {
		t.Fatalf("pdf slice = %#v", report.Slices["pdf"])
	}
	if !strings.Contains(
		strings.Join(report.Failures, "\n"),
		"pdf: answer correctness below promotion threshold",
	) {
		t.Fatalf("failures = %#v", report.Failures)
	}
}

func TestPromotionNoAnswerErrorRateIsZeroWithoutNegativeCases(t *testing.T) {
	metrics := promotionMetricAccumulator{
		cases: 10, relevantCases: 10,
	}.metrics()
	if metrics.NoAnswerFalseAnswerRate != 0 {
		t.Fatalf(
			"NoAnswerFalseAnswerRate = %v, want zero without negative cases",
			metrics.NoAnswerFalseAnswerRate,
		)
	}
}

func passingPromotionInput(t *testing.T) PromotionEvaluationInput {
	t.Helper()
	golden := PromotionGoldenSet{
		SchemaVersion: PromotionGoldenSchemaVersion,
		ID:            "structure-candidate-golden-v1",
		Description:   "Synthetic contract fixture; not a production reviewed corpus.",
		Lifecycle: PromotionGoldenLifecycle{
			State:        "frozen",
			FrozenAt:     "2026-07-24T01:00:00Z",
			HoldoutRunID: promotionHoldoutRunID,
		},
		Criteria: PromotionCriteria{
			MaximumP95LatencyMilliseconds:      1000,
			MaximumAverageContextTokens:        4096,
			MinimumAggregateQualityImprovement: 0.005,
		},
	}
	for index := 0; index < 500; index++ {
		split := "development"
		if index >= 300 && index < 400 {
			split = "validation"
		} else if index >= 400 {
			split = "holdout"
		}
		slice := criticalPromotionSlices[index/50]
		noAnswer := index >= 250 && index < 260
		evidence := []string{promotionEvidenceID(index)}
		if noAnswer {
			evidence = nil
		}
		golden.Cases = append(golden.Cases, PromotionGoldenCase{
			ID:                          promotionCaseID(index),
			Query:                       "synthetic contract query",
			Split:                       split,
			Slices:                      []string{slice},
			SelectedCollectionAliases:   []string{"synthetic-collection"},
			ExpectedRelevantEvidenceIDs: evidence,
			ExpectedNoAnswer:            noAnswer,
			TableExactAnswerRequired:    slice == "xlsx_table",
			Review: PromotionReview{
				State:      "human_reviewed",
				ReviewerID: promotionReviewerID,
				ReviewedAt: "2026-07-24T00:30:00Z",
			},
		})
	}
	refreezePromotionGolden(t, &golden)
	candidate := passingPromotionObservations(golden, "candidate")
	return PromotionEvaluationInput{
		Golden:                        golden,
		GoldenRawSHA256:               strings.Repeat("c", 64),
		Candidate:                     candidate,
		CandidateRawSHA256:            strings.Repeat("e", 64),
		CandidateGenerationID:         promotionCandidateGenerationID,
		CandidateArtifactManifestHash: promotionCandidateManifestHash,
	}
}

func passingPromotionObservations(
	golden PromotionGoldenSet,
	role string,
) PromotionObservationSet {
	result := PromotionObservationSet{
		SchemaVersion:        PromotionObservationSchemaVersion,
		GoldenSetID:          golden.ID,
		GoldenCorpusSHA256:   golden.Lifecycle.FrozenContentSHA256,
		CapturedAt:           "2026-07-24T02:00:00Z",
		CaptureID:            promotionCandidateCaptureID,
		ProfileRole:          role,
		GenerationID:         promotionCandidateGenerationID,
		ArtifactManifestHash: promotionCandidateManifestHash,
		ProfileID:            role + "-profile",
		HoldoutRun: PromotionHoldoutRun{
			ID:         promotionHoldoutRunID,
			Ordinal:    1,
			ExecutedAt: "2026-07-24T01:30:00Z",
		},
	}
	for index, goldenCase := range golden.Cases {
		evidence := append([]string(nil), goldenCase.ExpectedRelevantEvidenceIDs...)
		answered := !goldenCase.ExpectedNoAnswer
		answerCorrectness := 0.0
		faithfulness := 0.0
		if answered {
			answerCorrectness = 1
			faithfulness = 1
		}
		result.Cases = append(result.Cases, PromotionCaseObservation{
			CaseID:               promotionCaseID(index),
			RetrievedEvidenceIDs: evidence,
			FinalEvidenceIDs:     evidence,
			CitationEvidenceIDs:  evidence,
			Answered:             answered,
			AnswerCorrectness:    answerCorrectness,
			Faithfulness:         faithfulness,
			TableExactAnswer:     goldenCase.TableExactAnswerRequired,
			LatencyMilliseconds:  25,
			ContextTokens:        512,
			Integrity: PromotionCaseIntegrity{
				CitationLocatorValid: true,
				ProvenanceValid:      true,
				CellLineageValid:     true,
			},
		})
	}
	return result
}

func refreezePromotionGolden(t *testing.T, golden *PromotionGoldenSet) {
	t.Helper()
	golden.Lifecycle.FrozenContentSHA256 = ""
	digest, err := PromotionGoldenContentSHA256(*golden)
	if err != nil {
		t.Fatal(err)
	}
	golden.Lifecycle.FrozenContentSHA256 = digest
}

func promotionCaseID(index int) string {
	return "case-" + leftPadPromotionNumber(index)
}

func promotionEvidenceID(index int) string {
	return "evidence-" + leftPadPromotionNumber(index)
}

func leftPadPromotionNumber(value int) string {
	return fmt.Sprintf("%03d", value)
}
