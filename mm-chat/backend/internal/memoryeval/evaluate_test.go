package memoryeval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	benchmarkFixtureHash     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	benchmarkConfigHash      = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	benchmarkGoldenRawHash   = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	benchmarkObservationHash = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	benchmarkReviewerID      = "50000000-0000-4000-8000-000000000001"
	benchmarkHoldoutRunID    = "50000000-0000-4000-8000-000000000002"
	benchmarkCaptureID       = "50000000-0000-4000-8000-000000000003"
)

func TestEvaluatePassingFrozenBenchmark(t *testing.T) {
	input := passingEvaluationInput(t)

	report, err := Evaluate(input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !report.Passed || len(report.Failures) != 0 {
		t.Fatalf("report = %#v", report)
	}
	if report.SchemaVersion != ReportSchemaVersion ||
		report.Evaluation.EvaluatorVersion != EvaluatorVersion ||
		report.Evaluation.GoldenCorpusRawSHA256 != benchmarkGoldenRawHash ||
		report.Evaluation.ObservationsRawSHA256 != benchmarkObservationHash {
		t.Fatalf("evaluation provenance = %#v", report.Evaluation)
	}
	if report.Golden.TotalReviewed != 500 ||
		report.Golden.DevelopmentCount != 300 ||
		report.Golden.ValidationCount != 100 ||
		report.Golden.HoldoutCount != 100 || report.Golden.HoldoutRuns != 1 {
		t.Fatalf("Golden summary = %#v", report.Golden)
	}
	if report.Profile.Metrics.CandidateRecallAt20 != 1 ||
		report.Profile.Metrics.FinalRecallAt5 != 1 ||
		report.Profile.Metrics.CurrentFactAccuracy != 1 ||
		report.Profile.Metrics.FalseInjectionRate != 0 ||
		report.Profile.RankingDiagnostics.NDCGAt5 != 1 ||
		report.Profile.RankingDiagnostics.MRRAt5 != 1 ||
		report.Profile.ProviderCostRatio != 0.1 ||
		!report.Profile.ProviderCostPassed || !report.Profile.Safety.Passed {
		t.Fatalf("profile summary = %#v", report.Profile)
	}
	for _, name := range CriticalSlices() {
		result, ok := report.Slices[name]
		if !ok || result.Cases < 50 || !result.Passed || len(result.Failures) != 0 {
			t.Fatalf("slice %q = %#v", name, result)
		}
	}
}

func TestDecodeRejectsUnknownDuplicateAndTrailingJSON(t *testing.T) {
	valid, err := json.Marshal(draftGoldenFixture())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown",
			body: strings.Replace(string(valid), `"cases":`, `"unknown":true,"cases":`, 1),
			want: "unknown field",
		},
		{
			name: "duplicate",
			body: strings.Replace(string(valid), `"schemaVersion":`, `"schemaVersion":"shadowed","schemaVersion":`, 1),
			want: "duplicate JSON object key",
		},
		{
			name: "trailing",
			body: string(valid) + `{}`,
			want: "trailing JSON token",
		},
		{
			name: "missing explicit promotion denial",
			body: strings.Replace(
				string(valid),
				`"promotionEligible":false,`,
				"",
				1,
			),
			want: "cannot claim promotion eligibility",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeGoldenSet(strings.NewReader(test.body))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeGoldenSet() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDraftIsValidButNeverAdmitted(t *testing.T) {
	draft := draftGoldenFixture()
	body, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeGoldenSet(strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("DecodeGoldenSet() error = %v", err)
	}
	if decoded.PromotionEligible == nil || *decoded.PromotionEligible ||
		decoded.Lifecycle.State != "draft" {
		t.Fatalf("draft = %#v", decoded)
	}
	if err := ValidateGoldenAdmission(decoded); err == nil ||
		!strings.Contains(err.Error(), "not frozen") {
		t.Fatalf("ValidateGoldenAdmission() error = %v", err)
	}
	freeze, err := NewFreezeHashReport(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if freeze.PromotionEligible || len(freeze.FrozenContentSHA256) != 64 {
		t.Fatalf("freeze report = %#v", freeze)
	}
}

func TestCheckedInDraftTemplateIsValidAndCoversEveryCriticalSlice(t *testing.T) {
	path := filepath.Join(
		"..", "..", "..", "docs", "contracts",
		"memory-benchmark-golden-draft-template.json",
	)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	draft, err := DecodeGoldenSet(file)
	if err != nil {
		t.Fatalf("DecodeGoldenSet(%s) error = %v", path, err)
	}
	if draft.PromotionEligible == nil || *draft.PromotionEligible ||
		draft.Lifecycle.State != "draft" ||
		draft.DataPolicy.ContainsRealUserData || draft.DataPolicy.ContainsSensitiveData {
		t.Fatalf("checked-in draft policy = %#v", draft)
	}
	for _, name := range CriticalSlices() {
		if countSlice(draft.Cases, name) == 0 {
			t.Fatalf("checked-in draft does not demonstrate slice %q", name)
		}
	}
	if err := ValidateGoldenAdmission(draft); err == nil {
		t.Fatal("checked-in draft was admitted as frozen evidence")
	}
}

func TestAdmissionRejectsIncompleteOrFabricatedCorpus(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*EvaluationInput)
		message string
	}{
		{
			name: "499 cases",
			mutate: func(input *EvaluationInput) {
				input.Golden.Cases = input.Golden.Cases[:499]
				input.Observations.Cases = input.Observations.Cases[:499]
				refreezeGolden(t, &input.Golden)
			},
			message: "exactly 500",
		},
		{
			name: "wrong split",
			mutate: func(input *EvaluationInput) {
				input.Golden.Cases[0].Split = "validation"
				refreezeGolden(t, &input.Golden)
			},
			message: "300/100/100",
		},
		{
			name: "critical slice below 50",
			mutate: func(input *EvaluationInput) {
				input.Golden.Cases[1].Slices = []string{"mixed_language_entity"}
				refreezeGolden(t, &input.Golden)
			},
			message: `slice "preference_instruction" has fewer than 50`,
		},
		{
			name: "critical slice missing Holdout coverage",
			mutate: func(input *EvaluationInput) {
				removed := 0
				for index := range input.Golden.Cases {
					item := &input.Golden.Cases[index]
					if item.Split != "holdout" {
						continue
					}
					if _, ok := stringSet(item.Slices)["preference_instruction"]; !ok {
						continue
					}
					item.Slices = []string{"mixed_language_entity"}
					removed++
					if removed == 1 {
						break
					}
				}
				for index := 0; index < 20; index++ {
					item := &input.Golden.Cases[index*10]
					item.Slices = append(item.Slices, "preference_instruction")
				}
				refreezeGolden(t, &input.Golden)
			},
			message: "is not split across Development/Validation/Holdout",
		},
		{
			name: "draft review",
			mutate: func(input *EvaluationInput) {
				input.Golden.Cases[0].Review = Review{State: "draft"}
				refreezeGolden(t, &input.Golden)
			},
			message: "is not human-reviewed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := passingEvaluationInput(t)
			test.mutate(&input)
			_, err := Evaluate(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestGoldenCaseSliceLabelsRequireMatchingSemantics(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*GoldenCase)
		message string
	}{
		{
			name: "secret without exclusion",
			mutate: func(item *GoldenCase) {
				item.Slices = []string{"secret_rejection"}
			},
			message: "lacks a matching exclusion",
		},
		{
			name: "scope without authority exclusion",
			mutate: func(item *GoldenCase) {
				item.Slices = []string{"scope_isolation"}
			},
			message: "lacks an authority exclusion",
		},
		{
			name: "temporal without superseded",
			mutate: func(item *GoldenCase) {
				item.Slices = []string{"temporal_correction"}
			},
			message: "lacks current/superseded evidence",
		},
		{
			name: "project without Project scope",
			mutate: func(item *GoldenCase) {
				item.Slices = []string{"project_decision"}
			},
			message: "has no Project scope",
		},
		{
			name: "negative with relevant Memory",
			mutate: func(item *GoldenCase) {
				item.Slices = []string{"unrelated_negative"}
			},
			message: "expects Memory",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := draftGoldenFixture()
			test.mutate(&draft.Cases[0])
			body, err := json.Marshal(draft)
			if err != nil {
				t.Fatal(err)
			}
			_, err = DecodeGoldenSet(strings.NewReader(string(body)))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("DecodeGoldenSet() error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestObservationRankingStagesCannotHideUnlistedInjection(t *testing.T) {
	input := passingEvaluationInput(t)
	input.Observations.Cases[0].FinalMemoryIDs = []string{"memory-not-in-candidates"}
	input.Observations.Cases[0].InjectedMemoryIDs = []string{"memory-not-in-candidates"}

	_, err := Evaluate(input)
	if err == nil || !strings.Contains(err.Error(), "ranking stages are inconsistent") {
		t.Fatalf("Evaluate() error = %v", err)
	}
}

func TestEvaluateRejectsFrozenHashOrderAndHoldoutDrift(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*EvaluationInput)
		message string
	}{
		{
			name: "frozen content drift",
			mutate: func(input *EvaluationInput) {
				input.Golden.Cases[0].Query = "changed after freeze"
			},
			message: "frozen hash does not match",
		},
		{
			name: "case order drift",
			mutate: func(input *EvaluationInput) {
				input.Observations.Cases[0], input.Observations.Cases[1] =
					input.Observations.Cases[1], input.Observations.Cases[0]
			},
			message: "order differs",
		},
		{
			name: "Holdout rerun",
			mutate: func(input *EvaluationInput) {
				input.Observations.HoldoutRun.Ordinal = 2
			},
			message: "exactly one",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := passingEvaluationInput(t)
			test.mutate(&input)
			_, err := Evaluate(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestEvaluateFailsAuthorityPrivacyAndCurrentFactGates(t *testing.T) {
	input := passingEvaluationInput(t)

	// temporal_correction: enough cases inject the superseded fact to cross both
	// the current-fact and false-injection gates.
	for offset := 0; offset < 13; offset++ {
		temporal := 3 + offset*10
		oldID := input.Golden.Cases[temporal].Exclusions[0].MemoryID
		input.Observations.Cases[temporal].CandidateMemoryIDs = []string{oldID}
		input.Observations.Cases[temporal].FinalMemoryIDs = []string{oldID}
		input.Observations.Cases[temporal].InjectedMemoryIDs = []string{oldID}
	}

	// secret_rejection: model output caused a forbidden secret sentinel to persist
	// and cross the provider boundary.
	secret := 6
	secretID := exclusionID(input.Golden.Cases[secret], "secret")
	input.Observations.Cases[secret].PersistedMemoryIDs = []string{secretID}
	input.Observations.Cases[secret].ProviderSentMemoryIDs = []string{secretID}

	// untrusted_source: an assistant/document assertion was persisted as user
	// authority without direct user confirmation.
	untrusted := 5
	untrustedID := exclusionID(input.Golden.Cases[untrusted], "untrusted_source")
	input.Observations.Cases[untrusted].PersistedMemoryIDs = []string{untrustedID}

	// scope_isolation: the other user's Memory reached the candidate lane.
	scope := 7
	crossUserID := exclusionID(input.Golden.Cases[scope], "cross_user")
	input.Observations.Cases[scope].CandidateMemoryIDs = []string{crossUserID}

	// deletion: a tombstoned Memory became visible after a fallback.
	deleted := 8
	deletedID := exclusionID(input.Golden.Cases[deleted], "deleted")
	input.Observations.Cases[deleted].CandidateMemoryIDs = []string{deletedID}

	report, err := Evaluate(input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if report.Passed || report.Profile.Metrics.CurrentFactAccuracy >= 0.95 ||
		report.Profile.Metrics.FalseInjectionCases == 0 ||
		report.Profile.Safety.CrossUserLeakCount != 1 ||
		report.Profile.Safety.DeletedMemoryLeakCount != 1 ||
		report.Profile.Safety.SecretLeakCount != 1 ||
		report.Profile.Safety.UntrustedSourceLeakCount != 1 ||
		report.Profile.Safety.UnauthorizedProviderEgressCount != 1 ||
		report.Profile.Safety.Passed {
		t.Fatalf("failed profile = %#v", report.Profile)
	}
	joined := strings.Join(report.Failures, "\n")
	for _, expected := range []string{
		"current-fact accuracy below criterion",
		"Memory safety or authority leakage was observed",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("failures = %s, want %q", joined, expected)
		}
	}
}

func TestEvaluateFailsLatencyTokenCutoffAndCostBudgets(t *testing.T) {
	input := passingEvaluationInput(t)
	for index := 0; index < 30; index++ {
		input.Observations.Cases[index].LatencyMilliseconds = 1700
	}
	for index := 30; index < 40; index++ {
		input.Observations.Cases[index].LatencyMilliseconds = 2100
		input.Observations.Cases[index].HardCutoffApplied = true
	}
	input.Observations.Cases[40].PromptMemoryTokens = 901
	input.Observations.Costs.MemoryProviderCostMicrounits = 151
	input.Observations.Costs.ChatProviderCostMicrounits = 1000

	report, err := Evaluate(input)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if report.Passed || report.Profile.Budgets.LatencyPassed ||
		report.Profile.Budgets.PromptTokenPassed ||
		report.Profile.Budgets.HardCutoffPassed ||
		report.Profile.ProviderCostPassed {
		t.Fatalf("budget profile = %#v", report.Profile)
	}
	joined := strings.Join(report.Failures, "\n")
	for _, expected := range []string{
		"latency exceeds criterion",
		"token budget exceeds criterion",
		"hard cutoff was violated",
		"provider cost ratio exceeds criterion",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("failures = %s, want %q", joined, expected)
		}
	}
}

func TestProviderCostGateUsesExactV1Ratio(t *testing.T) {
	if !providerCostWithinV1Limit(15, 100) {
		t.Fatal("exact 15% cost ratio failed")
	}
	if providerCostWithinV1Limit(16, 100) {
		t.Fatal("cost ratio above 15% passed")
	}
	if providerCostWithinV1Limit(^uint64(0), ^uint64(0)) {
		t.Fatal("overflow-sized 100% cost ratio passed")
	}
}
