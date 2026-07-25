package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/rageval"
)

func TestRunWritesPassingReport(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{
		"-golden", filepath.Join("..", "..", "internal", "rageval", "testdata", "golden-v1.json"),
		"-observations", filepath.Join("..", "..", "internal", "rageval", "testdata", "passing-observations-v1.json"),
	}, &output)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var report rageval.Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !report.Passed || report.CaseCount != 7 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunRequiresBothInputs(t *testing.T) {
	if err := run([]string{"-golden", "golden.json"}, &bytes.Buffer{}); err == nil {
		t.Fatal("run() error = nil, want invalid arguments")
	}
}

func TestRunPromotionWritesActivationCompatibleReport(t *testing.T) {
	golden, candidate := promotionCommandFixtures(t)
	directory := t.TempDir()
	goldenPath := filepath.Join(directory, "golden.json")
	candidatePath := filepath.Join(directory, "candidate.json")
	writeJSONFixture(t, goldenPath, golden)
	writeJSONFixture(t, candidatePath, candidate)
	var freezeOutput bytes.Buffer
	if err := run([]string{
		"-promotion-golden", goldenPath,
		"-print-promotion-freeze-hash",
	}, &freezeOutput); err != nil {
		t.Fatalf("freeze-hash run() error = %v", err)
	}
	var freezeReport struct {
		FrozenContentSHA256 string `json:"frozenContentSha256"`
		PromotionEligible   bool   `json:"promotionEligible"`
	}
	if err := json.Unmarshal(freezeOutput.Bytes(), &freezeReport); err != nil {
		t.Fatalf("decode freeze report: %v", err)
	}
	if freezeReport.FrozenContentSHA256 !=
		golden.Lifecycle.FrozenContentSHA256 || freezeReport.PromotionEligible {
		t.Fatalf("freeze report = %#v", freezeReport)
	}

	var output bytes.Buffer
	err := run([]string{
		"-promotion-golden", goldenPath,
		"-candidate-observations", candidatePath,
		"-candidate-generation-id", candidate.GenerationID,
		"-artifact-manifest-hash", candidate.ArtifactManifestHash,
	}, &output)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var report rageval.PromotionGateReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !report.Passed ||
		report.SchemaVersion != rageval.PromotionGateReportSchemaVersion ||
		report.Golden.TotalReviewed != 500 ||
		report.Evaluation.GoldenCorpusRawSHA256 == "" ||
		report.Evaluation.CandidateObservationsSHA256 == "" {
		t.Fatalf("promotion report = %#v", report)
	}
}

func promotionCommandFixtures(t *testing.T) (
	rageval.PromotionGoldenSet,
	rageval.PromotionObservationSet,
) {
	t.Helper()
	golden := rageval.PromotionGoldenSet{
		SchemaVersion: rageval.PromotionGoldenSchemaVersion,
		ID:            "promotion-command-fixture",
		Description:   "Synthetic command contract fixture; never production evidence.",
		Lifecycle: rageval.PromotionGoldenLifecycle{
			State:        "frozen",
			FrozenAt:     "2026-07-24T01:00:00Z",
			HoldoutRunID: "50000000-0000-4000-8000-000000000030",
		},
		Criteria: rageval.PromotionCriteria{
			MaximumP95LatencyMilliseconds:      1000,
			MaximumAverageContextTokens:        4096,
			MinimumAggregateQualityImprovement: 0.005,
		},
	}
	criticalSlices := rageval.PromotionCriticalSlices()
	for index := 0; index < 500; index++ {
		split := "development"
		if index >= 300 && index < 400 {
			split = "validation"
		} else if index >= 400 {
			split = "holdout"
		}
		slice := criticalSlices[index/50]
		noAnswer := index >= 450
		evidence := []string{fmt.Sprintf("evidence-%03d", index)}
		if noAnswer {
			evidence = nil
		}
		golden.Cases = append(golden.Cases, rageval.PromotionGoldenCase{
			ID:                          fmt.Sprintf("case-%03d", index),
			Query:                       "synthetic command query",
			Split:                       split,
			Slices:                      []string{slice},
			SelectedCollectionAliases:   []string{"synthetic"},
			ExpectedRelevantEvidenceIDs: evidence,
			ExpectedNoAnswer:            noAnswer,
			TableExactAnswerRequired:    slice == "xlsx_table",
			Review: rageval.PromotionReview{
				State:      "human_reviewed",
				ReviewerID: "50000000-0000-4000-8000-000000000003",
				ReviewedAt: "2026-07-24T00:30:00Z",
			},
		})
	}
	digest, err := rageval.PromotionGoldenContentSHA256(golden)
	if err != nil {
		t.Fatal(err)
	}
	golden.Lifecycle.FrozenContentSHA256 = digest
	candidate := promotionCommandObservations(golden, "candidate")
	return golden, candidate
}

func promotionCommandObservations(
	golden rageval.PromotionGoldenSet,
	role string,
) rageval.PromotionObservationSet {
	result := rageval.PromotionObservationSet{
		SchemaVersion:        rageval.PromotionObservationSchemaVersion,
		GoldenSetID:          golden.ID,
		GoldenCorpusSHA256:   golden.Lifecycle.FrozenContentSHA256,
		CapturedAt:           "2026-07-24T02:00:00Z",
		CaptureID:            "50000000-0000-4000-8000-000000000021",
		ProfileRole:          role,
		GenerationID:         "50000000-0000-4000-8000-000000000020",
		ArtifactManifestHash: strings.Repeat("b", 64),
		ProfileID:            role + "-profile",
		HoldoutRun: rageval.PromotionHoldoutRun{
			ID:         "50000000-0000-4000-8000-000000000030",
			Ordinal:    1,
			ExecutedAt: "2026-07-24T01:30:00Z",
		},
	}
	for index, goldenCase := range golden.Cases {
		correctness := 0.0
		faithfulness := 0.0
		if !goldenCase.ExpectedNoAnswer {
			correctness = 1
			faithfulness = 1
		}
		evidence := append([]string(nil), goldenCase.ExpectedRelevantEvidenceIDs...)
		result.Cases = append(result.Cases, rageval.PromotionCaseObservation{
			CaseID:               fmt.Sprintf("case-%03d", index),
			RetrievedEvidenceIDs: evidence,
			FinalEvidenceIDs:     evidence,
			CitationEvidenceIDs:  evidence,
			Answered:             !goldenCase.ExpectedNoAnswer,
			AnswerCorrectness:    correctness,
			Faithfulness:         faithfulness,
			TableExactAnswer:     goldenCase.TableExactAnswerRequired,
			LatencyMilliseconds:  25,
			ContextTokens:        512,
			Integrity: rageval.PromotionCaseIntegrity{
				CitationLocatorValid: true,
				ProvenanceValid:      true,
				CellLineageValid:     true,
			},
		})
	}
	return result
}

func writeJSONFixture(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
