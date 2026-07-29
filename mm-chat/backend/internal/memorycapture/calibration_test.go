package memorycapture

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestBuildDevelopmentCalibrationSelectsAggregateOnlyTwoStagePolicy(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingDevelopmentCalibrationTraces(pool)
	report, body, err := BuildDevelopmentCalibration(
		pool,
		FakeCandidateProfileID,
		strings.Repeat("a", 64),
		passingCalibrationCosts(),
		traces,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != CalibrationReportSchemaVersion ||
		report.AdmissionMode != CalibrationAdmissionMode ||
		report.PromotionEligible || report.Split != "development" ||
		report.CaseCount != 300 || report.Selected == nil || report.IntentSelected == nil {
		t.Fatalf("calibration report authority = %#v", report)
	}
	if report.Selected.ProviderSimilarityBasisPoints != 21 ||
		report.Selected.FinalRelevanceBasisPoints != 0 ||
		!report.Selected.Evaluation.Passed ||
		report.Selected.Evaluation.Metrics.FalseInjectionCases != 0 ||
		report.Selected.Evaluation.Safety.UnauthorizedProviderEgressCount != 0 {
		t.Fatalf("selected policy = %#v", report.Selected)
	}
	if report.IntentSelected.MinimumMemoryIntentMarginBasisPoints != 80 ||
		!report.IntentSelected.Evaluation.Passed ||
		report.IntentEvaluatedThresholdCount != 201 ||
		report.IntentFeasibleThresholdCount == 0 {
		t.Fatalf("selected intent policy = %#v", report.IntentSelected)
	}
	if report.EvaluatedPairCount != 20301 || len(report.Frontier) != 201 ||
		report.FeasiblePairCount == 0 || !report.ProviderCostPassed ||
		report.ProviderCostRatio != 0.01 {
		t.Fatalf("grid result = pairs:%d feasible:%d frontier:%d",
			report.EvaluatedPairCount, report.FeasiblePairCount, len(report.Frontier))
	}
	if report.Diagnostics.Version != calibrationDiagnosticsVersion ||
		report.Diagnostics.BestSafetyAttempt == nil ||
		report.Diagnostics.BestRecallAttempt == nil ||
		report.Diagnostics.BestIntentSafetyAttempt == nil ||
		report.Diagnostics.BestIntentRecallAttempt == nil ||
		len(report.Diagnostics.FailurePairCounts) == 0 ||
		len(report.Diagnostics.AdmissionSimilarityCurve.RelevantPassingCaseCounts) != 201 ||
		len(report.Diagnostics.MaximumRerankScoreCurve.RelevantPassingCaseCounts) != 101 ||
		len(report.Diagnostics.TopTwoRerankMarginCurve.RelevantPassingCaseCounts) != 101 ||
		len(report.Diagnostics.MemoryIntentMarginCurve.RelevantPassingCaseCounts) != 201 ||
		report.Diagnostics.AdmissionSimilarityCurve.RelevantEligibleCaseCount == 0 ||
		report.Diagnostics.AdmissionSimilarityCurve.UnrelatedNegativeEligibleCaseCount == 0 {
		t.Fatalf("aggregate calibration diagnostics = %#v", report.Diagnostics)
	}
	admissionCurve := report.Diagnostics.AdmissionSimilarityCurve
	if admissionCurve.UnrelatedNegativePassingCaseCounts[120] !=
		admissionCurve.UnrelatedNegativeEligibleCaseCount ||
		admissionCurve.UnrelatedNegativePassingCaseCounts[121] != 0 ||
		admissionCurve.RelevantPassingCaseCounts[180] !=
			admissionCurve.RelevantEligibleCaseCount ||
		admissionCurve.RelevantPassingCaseCounts[181] != 0 {
		t.Fatalf("admission threshold curve = %#v", admissionCurve)
	}
	maximumCurve := report.Diagnostics.MaximumRerankScoreCurve
	if maximumCurve.UnrelatedNegativePassingCaseCounts[10] !=
		maximumCurve.UnrelatedNegativeEligibleCaseCount ||
		maximumCurve.UnrelatedNegativePassingCaseCounts[11] != 0 ||
		maximumCurve.RelevantPassingCaseCounts[90] !=
			maximumCurve.RelevantEligibleCaseCount ||
		maximumCurve.RelevantPassingCaseCounts[91] != 0 {
		t.Fatalf("maximum rerank threshold curve = %#v", maximumCurve)
	}
	intentCurve := report.Diagnostics.MemoryIntentMarginCurve
	if intentCurve.UnrelatedNegativePassingCaseCounts[20] !=
		intentCurve.UnrelatedNegativeEligibleCaseCount ||
		intentCurve.UnrelatedNegativePassingCaseCounts[21] != 0 ||
		intentCurve.RelevantPassingCaseCounts[180] !=
			intentCurve.RelevantEligibleCaseCount ||
		intentCurve.RelevantPassingCaseCounts[181] != 0 {
		t.Fatalf("Memory intent threshold curve = %#v", intentCurve)
	}
	for _, forbiddenKey := range []string{
		`"caseId"`, `"query"`, `"canonicalContent"`,
		`"memoryIntentMargin"`, `"admissionSimilarity"`, `"finalRelevanceScores"`,
	} {
		if bytes.Contains(body, []byte(forbiddenKey)) {
			t.Fatalf("aggregate calibration report contains forbidden key %s", forbiddenKey)
		}
	}
	for _, item := range pool.Corpus.Cases {
		if bytes.Contains(body, []byte(item.ID)) || bytes.Contains(body, []byte(item.Query)) {
			t.Fatal("aggregate calibration report contains case identity or query plaintext")
		}
	}
	for _, fixture := range pool.Fixtures.Fixtures {
		for _, memory := range fixture.Memories {
			if bytes.Contains(body, []byte(memory.CanonicalContent)) {
				t.Fatal("aggregate calibration report contains Memory plaintext")
			}
		}
	}
}

func TestBuildDevelopmentCalibrationRejectsIncompleteOrInvalidTraceAuthority(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	valid := passingDevelopmentCalibrationTraces(pool)
	configHash := strings.Repeat("b", 64)
	tests := []struct {
		name      string
		profileID string
		config    string
		mutate    func([]CandidateCalibrationTrace) []CandidateCalibrationTrace
	}{
		{name: "missing", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				return values[:len(values)-1]
			}},
		{name: "prepare unavailable", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				values[0].PreparedReady = false
				values[0].ResultCode = "PREPARE_FAILED"
				return values
			}},
		{name: "duplicate", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				values[1] = values[0]
				return values
			}},
		{name: "missing admission", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				values[0].AdmissionReady = false
				return values
			}},
		{name: "missing Memory intent", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				values[0].MemoryIntentReady = false
				return values
			}},
		{name: "missing rerank", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				values[0].RerankReady = false
				return values
			}},
		{name: "NaN admission", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				values[0].AdmissionSimilarity = math.NaN()
				return values
			}},
		{name: "infinite rerank", profileID: FakeCandidateProfileID, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
				for index := range values {
					if len(values[index].FinalRelevanceScores) > 0 {
						values[index].FinalRelevanceScores[0] = math.Inf(1)
						break
					}
				}
				return values
			}},
		{name: "raw profile", profileID: pool.Corpus.Cases[0].Query, config: configHash,
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace { return values }},
		{name: "non-hex config", profileID: FakeCandidateProfileID, config: strings.Repeat("z", 64),
			mutate: func(values []CandidateCalibrationTrace) []CandidateCalibrationTrace { return values }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traces := cloneCalibrationTraces(valid)
			traces = test.mutate(traces)
			if _, _, err := BuildDevelopmentCalibration(
				pool,
				test.profileID,
				test.config,
				passingCalibrationCosts(),
				traces,
			); err == nil {
				t.Fatal("invalid calibration trace authority was accepted")
			}
		})
	}
}

func TestBuildDevelopmentCalibrationKeepsProviderCostGate(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	costs := passingCalibrationCosts()
	costs.MemoryProviderCostMicrounits = 16
	report, _, err := BuildDevelopmentCalibration(
		pool,
		FakeCandidateProfileID,
		strings.Repeat("c", 64),
		costs,
		passingDevelopmentCalibrationTraces(pool),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.ProviderCostPassed || report.ProviderCostRatio != 0.16 ||
		report.Selected != nil || report.FeasiblePairCount != 0 ||
		report.IntentSelected != nil || report.IntentFeasibleThresholdCount != 0 ||
		report.Diagnostics.BestSafetyAttempt == nil ||
		report.Diagnostics.BestRecallAttempt == nil ||
		report.Diagnostics.BestIntentSafetyAttempt == nil ||
		report.Diagnostics.BestIntentRecallAttempt == nil ||
		report.Diagnostics.FailurePairCounts["Memory provider cost ratio exceeds criterion"] != 20301 ||
		report.Diagnostics.IntentFailureThresholdCounts["Memory provider cost ratio exceeds criterion"] != 201 {
		t.Fatalf("cost gate was weakened = %#v", report)
	}
}

func passingCalibrationCosts() memoryeval.ProviderCosts {
	return memoryeval.ProviderCosts{
		Unit: "cny_microunits", MemoryProviderCostMicrounits: 1,
		ChatProviderCostMicrounits: 100,
	}
}

func passingDevelopmentCalibrationTraces(
	pool memoryauthor.RegressionPool,
) []CandidateCalibrationTrace {
	result := make([]CandidateCalibrationTrace, 0, 300)
	for _, item := range pool.Corpus.Cases {
		if item.Split != "development" {
			continue
		}
		observation := memoryeval.CaseObservation{
			CaseID: item.ID, LatencyMilliseconds: 25, Fallback: "none",
			PersistedMemoryIDs: []string{},
		}
		admission := 0.8
		intentMargin := 0.8
		finalScores := make([]float64, 0, len(item.ExpectedRelevantMemoryIDs))
		if item.ExpectedNoMemory {
			admission = 0.2
			intentMargin = -0.8
			for _, exclusion := range item.Exclusions {
				if exclusion.Reason != "irrelevant" {
					continue
				}
				observation.CandidateMemoryIDs = []string{exclusion.MemoryID}
				observation.FinalMemoryIDs = []string{exclusion.MemoryID}
				observation.InjectedMemoryIDs = []string{exclusion.MemoryID}
				observation.ProviderSentMemoryIDs = []string{exclusion.MemoryID}
				observation.PromptMemoryTokens = 100
				finalScores = []float64{0.1}
				break
			}
		} else {
			observation.CandidateMemoryIDs = append(
				[]string(nil), item.ExpectedRelevantMemoryIDs...,
			)
			observation.FinalMemoryIDs = append(
				[]string(nil), item.ExpectedRelevantMemoryIDs...,
			)
			observation.InjectedMemoryIDs = append(
				[]string(nil), item.ExpectedRelevantMemoryIDs...,
			)
			observation.ProviderSentMemoryIDs = append(
				[]string(nil), item.ExpectedRelevantMemoryIDs...,
			)
			observation.PromptMemoryTokens = 100
			for range item.ExpectedRelevantMemoryIDs {
				finalScores = append(finalScores, 0.9)
			}
		}
		traceReady := len(observation.CandidateMemoryIDs) > 0
		result = append(result, CandidateCalibrationTrace{
			CaseID: item.ID, PreparedReady: true,
			MemoryIntentMargin: intentMargin, MemoryIntentReady: true,
			AdmissionSimilarity: admission, AdmissionReady: true,
			RerankReady:     true,
			FullObservation: observation, FinalRelevanceScores: finalScores,
		})
		if !traceReady {
			result[len(result)-1].AdmissionReady = false
			result[len(result)-1].RerankReady = false
			result[len(result)-1].MemoryIntentReady = false
		}
	}
	return result
}

func cloneCalibrationTraces(values []CandidateCalibrationTrace) []CandidateCalibrationTrace {
	cloned := make([]CandidateCalibrationTrace, len(values))
	for index, value := range values {
		value.FullObservation = cloneCaseObservation(value.FullObservation)
		value.FinalRelevanceScores = append([]float64(nil), value.FinalRelevanceScores...)
		cloned[index] = value
	}
	return cloned
}
