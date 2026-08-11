package memoryeval

import (
	"encoding/json"
	"testing"
)

func TestMemoryJudgeDevelopmentCriteriaV2ChangesOnlyLatency(t *testing.T) {
	base := benchmarkCriteria()
	criteria, err := MemoryJudgeDevelopmentCriteriaV2(base)
	if err != nil {
		t.Fatal(err)
	}
	if criteria.MaximumP95LatencyMilliseconds != 1500 ||
		criteria.MaximumP99LatencyMilliseconds != 2500 ||
		criteria.HardCutoffMilliseconds != 3000 {
		t.Fatalf("v2 latency criteria = %#v", criteria)
	}
	criteria.MaximumP95LatencyMilliseconds = base.MaximumP95LatencyMilliseconds
	criteria.MaximumP99LatencyMilliseconds = base.MaximumP99LatencyMilliseconds
	criteria.HardCutoffMilliseconds = base.HardCutoffMilliseconds
	if criteria != base {
		t.Fatalf("v2 non-latency criteria drifted: %#v / %#v", criteria, base)
	}
}

func TestMemoryJudgeDevelopmentCriteriaV2RejectsBaseAndDerivedDrift(t *testing.T) {
	base := benchmarkCriteria()
	base.MinimumFinalRecallAt5 = 0.89
	if _, err := MemoryJudgeDevelopmentCriteriaV2(base); err == nil {
		t.Fatal("drifted v1 base was accepted")
	}

	criteria, err := MemoryJudgeDevelopmentCriteriaV2(benchmarkCriteria())
	if err != nil {
		t.Fatal(err)
	}
	criteria.MaximumP99LatencyMilliseconds++
	if err := ValidateMemoryJudgeDevelopmentCriteriaV2(criteria); err == nil {
		t.Fatal("drifted v2 latency was accepted")
	}
	criteria, _ = MemoryJudgeDevelopmentCriteriaV2(benchmarkCriteria())
	criteria.MaximumFalseInjectionRate = 0.03
	if err := ValidateMemoryJudgeDevelopmentCriteriaV2(criteria); err == nil {
		t.Fatal("drifted v2 safety criterion was accepted")
	}
}

func TestMemoryJudgeAccuracyFirstCriteriaV3OmitsLatencyGates(t *testing.T) {
	criteria, err := MemoryJudgeAccuracyFirstCriteriaV3(benchmarkCriteria())
	if err != nil {
		t.Fatal(err)
	}
	if criteria.MinimumFinalRecallAt5 != 0.90 ||
		criteria.MaximumFalseInjectionRate != 0.02 ||
		criteria.MaximumAveragePromptMemoryTokens != 600 ||
		criteria.LatencyEvaluationMode != MemoryJudgeLatencyDiagnosticOnlyV1 ||
		criteria.ApplicationDeadlineMode != MemoryJudgeApplicationDeadlineNoneV1 {
		t.Fatalf("accuracy-first criteria = %#v", criteria)
	}
	drifted := criteria
	drifted.LatencyEvaluationMode = "gated"
	if ValidateMemoryJudgeAccuracyFirstCriteriaV3(drifted) == nil {
		t.Fatal("accuracy-first latency mode drift was accepted")
	}
	drifted = criteria
	drifted.MaximumFalseInjectionRate = 0.03
	if ValidateMemoryJudgeAccuracyFirstCriteriaV3(drifted) == nil {
		t.Fatal("accuracy-first safety drift was accepted")
	}
}

func TestMemoryJudgeSingleUserBoundedMissCriteriaV4IsSeparateAndExact(t *testing.T) {
	base := benchmarkCriteria()
	v3, err := MemoryJudgeAccuracyFirstCriteriaV3(base)
	if err != nil {
		t.Fatal(err)
	}
	v3JSONBefore, err := json.Marshal(v3)
	if err != nil {
		t.Fatal(err)
	}
	v4, err := MemoryJudgeSingleUserBoundedMissCriteriaV4(base)
	if err != nil {
		t.Fatal(err)
	}
	if v4.MinimumCurrentFactAccuracy != 0.95 ||
		v4.MinimumRequiredSliceCurrentFactAccuracy != 0.90 ||
		v4.MaximumFalseInjectionRate != 0 ||
		v4.MaximumFalseInjectionCases != 0 ||
		v4.MinimumCandidateRecallAt20 != v3.MinimumCandidateRecallAt20 ||
		v4.MinimumFinalRecallAt5 != v3.MinimumFinalRecallAt5 ||
		v4.MaximumAveragePromptMemoryTokens != v3.MaximumAveragePromptMemoryTokens ||
		v4.MaximumPromptMemoryTokens != v3.MaximumPromptMemoryTokens ||
		v4.MaximumProviderCostRatio != v3.MaximumProviderCostRatio ||
		v4.LatencyEvaluationMode != v3.LatencyEvaluationMode ||
		v4.ApplicationDeadlineMode != v3.ApplicationDeadlineMode {
		t.Fatalf("bounded-miss criteria = %#v", v4)
	}
	v3JSONAfter, err := json.Marshal(v3)
	if err != nil {
		t.Fatal(err)
	}
	if string(v3JSONBefore) != string(v3JSONAfter) ||
		string(v3JSONAfter) != `{"minimumCandidateRecallAt20":0.95,"minimumFinalRecallAt5":0.9,"minimumCurrentFactAccuracy":0.95,"maximumFalseInjectionRate":0.02,"maximumAveragePromptMemoryTokens":600,"maximumPromptMemoryTokens":900,"maximumProviderCostRatio":0.15,"latencyEvaluationMode":"diagnostic_only_v1","applicationDeadlineMode":"none_v1"}` {
		t.Fatalf("historical v3 JSON drifted: %s", v3JSONAfter)
	}

	drifted := v4
	drifted.MinimumRequiredSliceCurrentFactAccuracy = 0.89
	if ValidateMemoryJudgeSingleUserBoundedMissCriteriaV4(drifted) == nil {
		t.Fatal("bounded-miss required-slice drift was accepted")
	}
	drifted = v4
	drifted.MaximumFalseInjectionCases = 1
	if ValidateMemoryJudgeSingleUserBoundedMissCriteriaV4(drifted) == nil {
		t.Fatal("bounded-miss false-injection drift was accepted")
	}
}
