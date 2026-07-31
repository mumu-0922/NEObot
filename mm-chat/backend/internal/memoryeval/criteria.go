package memoryeval

import "errors"

const (
	CriteriaVersionV1                         = "neo-chat.memory-benchmark-criteria.v1"
	MemoryJudgeDevelopmentCriteriaVersionV2   = "neo-chat.memory-benchmark-criteria.v2"
	MemoryJudgeDevelopmentMaximumP95MillisV2  = int64(1500)
	MemoryJudgeDevelopmentMaximumP99MillisV2  = int64(2500)
	MemoryJudgeDevelopmentHardCutoffMillisV2  = int64(3000)
	MemoryJudgeAccuracyFirstCriteriaVersionV3 = "neo-chat.memory-benchmark-criteria.v3"
	MemoryJudgeLatencyDiagnosticOnlyV1        = "diagnostic_only_v1"
	MemoryJudgeApplicationDeadlineNoneV1      = "none_v1"
)

// MemoryJudgeDevelopmentCriteriaV2 derives the owner-selected complete-flow
// latency budget from one already-admitted v1 regression corpus. The corpus
// remains byte-authoritative; only the separately versioned Development
// evaluation receives these three replacement values.
func MemoryJudgeDevelopmentCriteriaV2(base Criteria) (Criteria, error) {
	if err := validateCriteria(base); err != nil {
		return Criteria{}, err
	}
	criteria := base
	criteria.MaximumP95LatencyMilliseconds = MemoryJudgeDevelopmentMaximumP95MillisV2
	criteria.MaximumP99LatencyMilliseconds = MemoryJudgeDevelopmentMaximumP99MillisV2
	criteria.HardCutoffMilliseconds = MemoryJudgeDevelopmentHardCutoffMillisV2
	if err := ValidateMemoryJudgeDevelopmentCriteriaV2(criteria); err != nil {
		return Criteria{}, err
	}
	return criteria, nil
}

func MemoryJudgeAccuracyFirstCriteriaV3(base Criteria) (AccuracyFirstCriteria, error) {
	if err := validateCriteria(base); err != nil {
		return AccuracyFirstCriteria{}, err
	}
	criteria := AccuracyFirstCriteria{
		MinimumCandidateRecallAt20:       base.MinimumCandidateRecallAt20,
		MinimumFinalRecallAt5:            base.MinimumFinalRecallAt5,
		MinimumCurrentFactAccuracy:       base.MinimumCurrentFactAccuracy,
		MaximumFalseInjectionRate:        base.MaximumFalseInjectionRate,
		MaximumAveragePromptMemoryTokens: base.MaximumAveragePromptMemoryTokens,
		MaximumPromptMemoryTokens:        base.MaximumPromptMemoryTokens,
		MaximumProviderCostRatio:         base.MaximumProviderCostRatio,
		LatencyEvaluationMode:            MemoryJudgeLatencyDiagnosticOnlyV1,
		ApplicationDeadlineMode:          MemoryJudgeApplicationDeadlineNoneV1,
	}
	if err := ValidateMemoryJudgeAccuracyFirstCriteriaV3(criteria); err != nil {
		return AccuracyFirstCriteria{}, err
	}
	return criteria, nil
}

func ValidateMemoryJudgeAccuracyFirstCriteriaV3(value AccuracyFirstCriteria) error {
	legacy := Criteria{
		MinimumCandidateRecallAt20:       value.MinimumCandidateRecallAt20,
		MinimumFinalRecallAt5:            value.MinimumFinalRecallAt5,
		MinimumCurrentFactAccuracy:       value.MinimumCurrentFactAccuracy,
		MaximumFalseInjectionRate:        value.MaximumFalseInjectionRate,
		MaximumP95LatencyMilliseconds:    900,
		MaximumP99LatencyMilliseconds:    1500,
		HardCutoffMilliseconds:           2000,
		MaximumAveragePromptMemoryTokens: value.MaximumAveragePromptMemoryTokens,
		MaximumPromptMemoryTokens:        value.MaximumPromptMemoryTokens,
		MaximumProviderCostRatio:         value.MaximumProviderCostRatio,
	}
	if value.LatencyEvaluationMode != MemoryJudgeLatencyDiagnosticOnlyV1 ||
		value.ApplicationDeadlineMode != MemoryJudgeApplicationDeadlineNoneV1 ||
		validateCriteria(legacy) != nil {
		return errors.New("Memory judge accuracy-first Development criteria drifted")
	}
	return nil
}

// ValidateMemoryJudgeDevelopmentCriteriaV2 rejects any non-latency drift and
// keeps the historical v1 validator as the single authority for every other
// quality, safety, token, and cost criterion.
func ValidateMemoryJudgeDevelopmentCriteriaV2(value Criteria) error {
	if value.MaximumP95LatencyMilliseconds != MemoryJudgeDevelopmentMaximumP95MillisV2 ||
		value.MaximumP99LatencyMilliseconds != MemoryJudgeDevelopmentMaximumP99MillisV2 ||
		value.HardCutoffMilliseconds != MemoryJudgeDevelopmentHardCutoffMillisV2 {
		return errors.New("Memory judge Development criteria do not match v2")
	}
	legacy := value
	legacy.MaximumP95LatencyMilliseconds = 900
	legacy.MaximumP99LatencyMilliseconds = 1500
	legacy.HardCutoffMilliseconds = 2000
	if err := validateCriteria(legacy); err != nil {
		return errors.New("Memory judge Development criteria drifted from v1")
	}
	return nil
}
