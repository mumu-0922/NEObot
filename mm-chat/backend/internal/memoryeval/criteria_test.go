package memoryeval

import "testing"

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
