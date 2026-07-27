package memoryeval

import (
	"fmt"
	"testing"
)

func passingEvaluationInput(t *testing.T) EvaluationInput {
	t.Helper()
	golden := GoldenSet{
		SchemaVersion:     GoldenSchemaVersion,
		ID:                "memory-v2-synthetic-contract-v1",
		Description:       "Synthetic contract fixture; never production benchmark evidence.",
		PromotionEligible: boolPointer(false),
		DataPolicy: DataPolicy{
			SyntheticOnly: true,
		},
		FixtureManifestSHA256: benchmarkFixtureHash,
		Lifecycle: GoldenLifecycle{
			State:        "frozen",
			FrozenAt:     "2026-07-28T01:00:00Z",
			HoldoutRunID: benchmarkHoldoutRunID,
		},
		Criteria: benchmarkCriteria(),
	}
	for index := 0; index < 500; index++ {
		golden.Cases = append(golden.Cases, benchmarkCase(index, "human_reviewed"))
	}
	refreezeGolden(t, &golden)
	observations := passingObservations(golden)
	return EvaluationInput{
		Golden:             golden,
		GoldenRawSHA256:    benchmarkGoldenRawHash,
		Observations:       observations,
		ObservationsSHA256: benchmarkObservationHash,
	}
}

func benchmarkCase(index int, reviewState string) GoldenCase {
	// Interleave groups so every critical slice is represented by the frozen
	// 60/20/20 split rather than existing only in a tuneable partition.
	group := index % 10
	caseID := fmt.Sprintf("memory-case-%03d", index)
	memoryID := fmt.Sprintf("memory-current-%03d", index)
	item := GoldenCase{
		ID:           caseID,
		Query:        "Synthetic Memory benchmark query",
		Split:        benchmarkSplit(index),
		Language:     "en",
		FixtureAlias: fmt.Sprintf("fixture-%03d", index),
		Scope:        Scope{UserAlias: "user-a"},
		Review:       Review{State: reviewState},
	}
	if reviewState == "human_reviewed" {
		item.Review.ReviewerID = benchmarkReviewerID
		item.Review.ReviewedAt = "2026-07-28T00:30:00Z"
	}
	switch group {
	case 0:
		item.Language = "zh"
		item.Slices = []string{"stable_fact", "chinese_paraphrase"}
		item.ExpectedRelevantMemoryIDs = []string{memoryID}
		item.ExpectedCurrentMemoryIDs = []string{memoryID}
	case 1:
		item.Language = "mixed"
		item.Slices = []string{"preference_instruction", "mixed_language_entity"}
		item.ExpectedRelevantMemoryIDs = []string{memoryID}
		item.ExpectedCurrentMemoryIDs = []string{memoryID}
	case 2:
		item.Slices = []string{"project_decision", "multi_hop"}
		item.Scope.ProjectAlias = "project-a"
		item.ExpectedRelevantMemoryIDs = []string{memoryID, fmt.Sprintf("memory-related-%03d", index)}
		item.ExpectedCurrentMemoryIDs = append([]string(nil), item.ExpectedRelevantMemoryIDs...)
	case 3:
		item.Language = "zh"
		item.Slices = []string{"temporal_correction", "chinese_paraphrase"}
		item.ExpectedRelevantMemoryIDs = []string{memoryID}
		item.ExpectedCurrentMemoryIDs = []string{memoryID}
		item.Exclusions = []Exclusion{{
			MemoryID: fmt.Sprintf("memory-superseded-%03d", index),
			Reason:   "superseded",
		}}
	case 4:
		item.Slices = []string{"unrelated_negative", "failure_fallback"}
		item.ExpectedNoMemory = true
		item.Exclusions = []Exclusion{{MemoryID: fmt.Sprintf("memory-irrelevant-%03d", index), Reason: "irrelevant"}}
	case 5:
		item.Slices = []string{"untrusted_source", "unrelated_negative"}
		item.ExpectedNoMemory = true
		item.Exclusions = []Exclusion{{MemoryID: fmt.Sprintf("memory-untrusted-%03d", index), Reason: "untrusted_source"}}
	case 6:
		item.Slices = []string{"secret_rejection", "untrusted_source"}
		item.ExpectedNoMemory = true
		item.Exclusions = []Exclusion{
			{MemoryID: fmt.Sprintf("memory-secret-%03d", index), Reason: "secret"},
			{MemoryID: fmt.Sprintf("memory-untrusted-%03d", index), Reason: "untrusted_source"},
		}
	case 7:
		item.Slices = []string{"scope_isolation", "unrelated_negative"}
		item.ExpectedNoMemory = true
		item.Exclusions = []Exclusion{{MemoryID: fmt.Sprintf("memory-user-b-%03d", index), Reason: "cross_user"}}
	case 8:
		item.Slices = []string{"deletion", "failure_fallback"}
		item.ExpectedNoMemory = true
		item.Exclusions = []Exclusion{{MemoryID: fmt.Sprintf("memory-deleted-%03d", index), Reason: "deleted"}}
	case 9:
		item.Slices = []string{"stable_fact", "failure_fallback"}
		item.ExpectedRelevantMemoryIDs = []string{memoryID}
		item.ExpectedCurrentMemoryIDs = []string{memoryID}
	}
	return item
}

func benchmarkSplit(index int) string {
	if index < 300 {
		return "development"
	}
	if index < 400 {
		return "validation"
	}
	return "holdout"
}

func benchmarkCriteria() Criteria {
	return Criteria{
		MinimumCandidateRecallAt20:       0.95,
		MinimumFinalRecallAt5:            0.90,
		MinimumCurrentFactAccuracy:       0.95,
		MaximumFalseInjectionRate:        0.02,
		MaximumP95LatencyMilliseconds:    900,
		MaximumP99LatencyMilliseconds:    1500,
		HardCutoffMilliseconds:           2000,
		MaximumAveragePromptMemoryTokens: 600,
		MaximumPromptMemoryTokens:        900,
		MaximumProviderCostRatio:         0.15,
	}
}

func passingObservations(golden GoldenSet) ObservationSet {
	value := ObservationSet{
		SchemaVersion:         ObservationSchemaVersion,
		GoldenSetID:           golden.ID,
		GoldenCorpusSHA256:    golden.Lifecycle.FrozenContentSHA256,
		FixtureManifestSHA256: golden.FixtureManifestSHA256,
		CapturedAt:            "2026-07-28T02:00:00Z",
		CaptureID:             benchmarkCaptureID,
		Profile: Profile{
			ID:                  "memory-v2-contract-profile",
			Role:                "candidate",
			ReaderVersion:       "synthetic-no-network",
			ConfigurationSHA256: benchmarkConfigHash,
			CandidateLimit:      20,
			FinalLimit:          5,
		},
		HoldoutRun: HoldoutRun{
			ID:         benchmarkHoldoutRunID,
			Ordinal:    1,
			ExecutedAt: "2026-07-28T01:30:00Z",
		},
		Costs: ProviderCosts{
			Unit:                         "synthetic-microunit",
			MemoryProviderCostMicrounits: 100,
			ChatProviderCostMicrounits:   1000,
		},
	}
	for _, goldenCase := range golden.Cases {
		memoryIDs := append([]string(nil), goldenCase.ExpectedRelevantMemoryIDs...)
		value.Cases = append(value.Cases, CaseObservation{
			CaseID:                goldenCase.ID,
			CandidateMemoryIDs:    append([]string(nil), memoryIDs...),
			FinalMemoryIDs:        append([]string(nil), memoryIDs...),
			InjectedMemoryIDs:     append([]string(nil), memoryIDs...),
			ProviderSentMemoryIDs: append([]string(nil), memoryIDs...),
			LatencyMilliseconds:   25,
			PromptMemoryTokens:    200,
			Fallback:              "none",
		})
	}
	return value
}

func refreezeGolden(t *testing.T, golden *GoldenSet) {
	t.Helper()
	golden.Lifecycle.FrozenContentSHA256 = ""
	digest, err := GoldenContentSHA256(*golden)
	if err != nil {
		t.Fatal(err)
	}
	golden.Lifecycle.FrozenContentSHA256 = digest
}

func draftGoldenFixture() GoldenSet {
	return GoldenSet{
		SchemaVersion:     GoldenSchemaVersion,
		ID:                "memory-benchmark-draft-example",
		Description:       "Synthetic draft example; not reviewed or promotion evidence.",
		PromotionEligible: boolPointer(false),
		DataPolicy: DataPolicy{
			SyntheticOnly: true,
		},
		Lifecycle: GoldenLifecycle{State: "draft"},
		Criteria:  benchmarkCriteria(),
		Cases: []GoldenCase{
			benchmarkCase(0, "draft"),
		},
	}
}

func exclusionID(item GoldenCase, reason string) string {
	for _, exclusion := range item.Exclusions {
		if exclusion.Reason == reason {
			return exclusion.MemoryID
		}
	}
	return "missing-exclusion"
}

func boolPointer(value bool) *bool {
	return &value
}
