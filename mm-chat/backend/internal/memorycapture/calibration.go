package memorycapture

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	CalibrationReportSchemaVersion      = "neo-chat.memory-regression-relevance-calibration.v3"
	CalibrationAdmissionMode            = "development_calibration_only"
	calibrationMinimumAdmissionBP       = -100
	calibrationMaximumAdmissionBP       = 100
	calibrationMinimumFinalBP           = 0
	calibrationMaximumFinalBP           = 100
	calibrationStepBP                   = 1
	calibrationSelectionAlgorithm       = "min-false-injection_max-recall_lowest-threshold_v1"
	calibrationIntentSelectionAlgorithm = "zero-egress_max-recall_highest-intent-margin_v1"
	calibrationDiagnosticsVersion       = "aggregate-threshold-curves-intent-and-attempts-v2"
)

type CalibrationGrid struct {
	IntentMinimumBasisPoints    int `json:"intentMinimumBasisPoints"`
	IntentMaximumBasisPoints    int `json:"intentMaximumBasisPoints"`
	AdmissionMinimumBasisPoints int `json:"admissionMinimumBasisPoints"`
	AdmissionMaximumBasisPoints int `json:"admissionMaximumBasisPoints"`
	FinalMinimumBasisPoints     int `json:"finalMinimumBasisPoints"`
	FinalMaximumBasisPoints     int `json:"finalMaximumBasisPoints"`
	StepBasisPoints             int `json:"stepBasisPoints"`
}

type CalibrationPlanConfig struct {
	SchemaVersion            string          `json:"schemaVersion"`
	SelectionAlgorithm       string          `json:"selectionAlgorithm"`
	IntentSelectionAlgorithm string          `json:"intentSelectionAlgorithm"`
	DiagnosticsVersion       string          `json:"diagnosticsVersion"`
	Grid                     CalibrationGrid `json:"grid"`
}

type CalibrationSelection struct {
	ProviderSimilarityBasisPoints int                              `json:"providerSimilarityBasisPoints"`
	FinalRelevanceBasisPoints     int                              `json:"finalRelevanceBasisPoints"`
	Evaluation                    memoryeval.CalibrationEvaluation `json:"evaluation"`
}

type CalibrationIntentSelection struct {
	MinimumMemoryIntentMarginBasisPoints int                              `json:"minimumMemoryIntentMarginBasisPoints"`
	Evaluation                           memoryeval.CalibrationEvaluation `json:"evaluation"`
}

type CalibrationFrontierPoint struct {
	ProviderSimilarityBasisPoints int                      `json:"providerSimilarityBasisPoints"`
	Feasible                      bool                     `json:"feasible"`
	FinalRelevanceBasisPoints     int                      `json:"finalRelevanceBasisPoints,omitempty"`
	Metrics                       memoryeval.Metrics       `json:"metrics,omitempty"`
	Safety                        memoryeval.SafetyMetrics `json:"safety,omitempty"`
}

type CalibrationThresholdCurve struct {
	MinimumBasisPoints                 int   `json:"minimumBasisPoints"`
	MaximumBasisPoints                 int   `json:"maximumBasisPoints"`
	StepBasisPoints                    int   `json:"stepBasisPoints"`
	RelevantEligibleCaseCount          int   `json:"relevantEligibleCaseCount"`
	RelevantMissingCaseCount           int   `json:"relevantMissingCaseCount"`
	UnrelatedNegativeEligibleCaseCount int   `json:"unrelatedNegativeEligibleCaseCount"`
	UnrelatedNegativeMissingCaseCount  int   `json:"unrelatedNegativeMissingCaseCount"`
	RelevantPassingCaseCounts          []int `json:"relevantPassingCaseCounts"`
	UnrelatedNegativePassingCaseCounts []int `json:"unrelatedNegativePassingCaseCounts"`
}

type CalibrationDiagnostics struct {
	Version                      string                      `json:"version"`
	OtherCaseCount               int                         `json:"otherCaseCount"`
	FailurePairCounts            map[string]int              `json:"failurePairCounts"`
	IntentFailureThresholdCounts map[string]int              `json:"intentFailureThresholdCounts"`
	BestSafetyAttempt            *CalibrationSelection       `json:"bestSafetyAttempt,omitempty"`
	BestRecallAttempt            *CalibrationSelection       `json:"bestRecallAttempt,omitempty"`
	BestIntentSafetyAttempt      *CalibrationIntentSelection `json:"bestIntentSafetyAttempt,omitempty"`
	BestIntentRecallAttempt      *CalibrationIntentSelection `json:"bestIntentRecallAttempt,omitempty"`
	MemoryIntentMarginCurve      CalibrationThresholdCurve   `json:"memoryIntentMarginCurve"`
	AdmissionSimilarityCurve     CalibrationThresholdCurve   `json:"admissionSimilarityCurve"`
	MaximumRerankScoreCurve      CalibrationThresholdCurve   `json:"maximumRerankScoreCurve"`
	TopTwoRerankMarginCurve      CalibrationThresholdCurve   `json:"topTwoRerankMarginCurve"`
}

type CalibrationReport struct {
	SchemaVersion                 string                      `json:"schemaVersion"`
	CorpusClass                   string                      `json:"corpusClass"`
	AdmissionMode                 string                      `json:"admissionMode"`
	PromotionEligible             bool                        `json:"promotionEligible"`
	Split                         string                      `json:"split"`
	CaseCount                     int                         `json:"caseCount"`
	PolicyID                      string                      `json:"policyId"`
	ProfileID                     string                      `json:"profileId"`
	ConfigurationSHA256           string                      `json:"configurationSha256"`
	SelectionAlgorithm            string                      `json:"selectionAlgorithm"`
	Grid                          CalibrationGrid             `json:"grid"`
	EvaluatedPairCount            int                         `json:"evaluatedPairCount"`
	FeasiblePairCount             int                         `json:"feasiblePairCount"`
	ProviderCostRatio             float64                     `json:"providerCostRatio"`
	ProviderCostPassed            bool                        `json:"providerCostPassed"`
	Selected                      *CalibrationSelection       `json:"selected,omitempty"`
	IntentSelectionAlgorithm      string                      `json:"intentSelectionAlgorithm"`
	IntentEvaluatedThresholdCount int                         `json:"intentEvaluatedThresholdCount"`
	IntentFeasibleThresholdCount  int                         `json:"intentFeasibleThresholdCount"`
	IntentSelected                *CalibrationIntentSelection `json:"intentSelected,omitempty"`
	Frontier                      []CalibrationFrontierPoint  `json:"frontier"`
	Diagnostics                   CalibrationDiagnostics      `json:"diagnostics"`
}

// BuildDevelopmentCalibration evaluates the precommitted two-stage grid using
// only development cases. Raw query/vector/rerank scores remain in the input
// traces and are never copied into the returned aggregate report.
func BuildDevelopmentCalibration(
	pool memoryauthor.RegressionPool,
	profileID string,
	configurationSHA256 string,
	costs memoryeval.ProviderCosts,
	traces []CandidateCalibrationTrace,
) (CalibrationReport, []byte, error) {
	if (profileID != CandidateProfileID && profileID != FakeCandidateProfileID) ||
		len(configurationSHA256) != 64 || costs.Unit == "" ||
		costs.MemoryProviderCostMicrounits == 0 || costs.ChatProviderCostMicrounits == 0 {
		return CalibrationReport{}, nil, ErrCaptureInvalid
	}
	if _, err := hex.DecodeString(configurationSHA256); err != nil {
		return CalibrationReport{}, nil, ErrCaptureInvalid
	}
	splitCounts := map[string]int{}
	for _, item := range pool.Corpus.Cases {
		splitCounts[item.Split]++
	}
	if len(pool.Corpus.Cases) != 500 || splitCounts["development"] != 300 ||
		splitCounts["validation"] != 100 || splitCounts["holdout"] != 100 {
		return CalibrationReport{}, nil, fmt.Errorf("%w: calibration corpus split", ErrCaptureInvalid)
	}
	development := make([]memoryeval.GoldenCase, 0, 300)
	for _, item := range pool.Corpus.Cases {
		if item.Split == "development" {
			development = append(development, item)
		}
	}
	if len(development) != 300 || len(traces) != len(development) {
		return CalibrationReport{}, nil, fmt.Errorf("%w: development calibration count", ErrCaptureInvalid)
	}
	traceByID := make(map[string]CandidateCalibrationTrace, len(traces))
	for _, trace := range traces {
		if !trace.PreparedReady {
			return CalibrationReport{}, nil, fmt.Errorf(
				"%w: calibration prepare unavailable (%s/%s)",
				ErrCaptureInvalid,
				normalizeCalibrationCode(trace.ResultCode),
				normalizeCalibrationCode(trace.AbstentionCode),
			)
		}
		candidateCount := len(trace.FullObservation.CandidateMemoryIDs)
		emptyCandidateTrace := candidateCount == 0 && !trace.AdmissionReady &&
			!trace.RerankReady && len(trace.FullObservation.FinalMemoryIDs) == 0 &&
			len(trace.FullObservation.InjectedMemoryIDs) == 0 &&
			len(trace.FullObservation.ProviderSentMemoryIDs) == 0 &&
			len(trace.FinalRelevanceScores) == 0
		completeCandidateTrace := candidateCount > 0 && trace.MemoryIntentReady &&
			trace.AdmissionReady && trace.RerankReady
		redactedCandidateTrace := candidateCount > 0 && trace.AdmissionReady &&
			!trace.RerankReady && trace.AbstentionCode == "SECRET_REDACTED" &&
			len(trace.FullObservation.FinalMemoryIDs) == 0 &&
			len(trace.FullObservation.InjectedMemoryIDs) == 0 &&
			len(trace.FullObservation.ProviderSentMemoryIDs) == 0 &&
			len(trace.FinalRelevanceScores) == 0
		if trace.CaseID == "" {
			return CalibrationReport{}, nil, fmt.Errorf("%w: empty calibration case identity", ErrCaptureInvalid)
		}
		if !emptyCandidateTrace && !completeCandidateTrace && !redactedCandidateTrace {
			state := "inconsistent"
			switch {
			case candidateCount == 0:
				state = "empty candidate surface"
			case !trace.AdmissionReady:
				state = "admission unavailable"
			case !trace.RerankReady:
				state = "rerank unavailable"
			}
			return CalibrationReport{}, nil, fmt.Errorf(
				"%w: incomplete calibration trace (%s/%s/%s candidates=%d provider=%d final=%d scores=%d)",
				ErrCaptureInvalid,
				state,
				normalizeCalibrationCode(trace.ResultCode),
				normalizeCalibrationCode(trace.AbstentionCode),
				candidateCount,
				len(trace.FullObservation.ProviderSentMemoryIDs),
				len(trace.FullObservation.FinalMemoryIDs),
				len(trace.FinalRelevanceScores),
			)
		}
		if math.IsNaN(trace.MemoryIntentMargin) || math.IsInf(trace.MemoryIntentMargin, 0) ||
			trace.MemoryIntentMargin < -1 || trace.MemoryIntentMargin > 1 ||
			math.IsNaN(trace.AdmissionSimilarity) || math.IsInf(trace.AdmissionSimilarity, 0) ||
			trace.AdmissionSimilarity < -1 || trace.AdmissionSimilarity > 1 ||
			len(trace.FinalRelevanceScores) != len(trace.FullObservation.FinalMemoryIDs) {
			return CalibrationReport{}, nil, fmt.Errorf("%w: incomplete calibration trace", ErrCaptureInvalid)
		}
		for _, score := range trace.FinalRelevanceScores {
			if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
				return CalibrationReport{}, nil, fmt.Errorf("%w: invalid calibration score", ErrCaptureInvalid)
			}
		}
		if _, duplicate := traceByID[trace.CaseID]; duplicate {
			return CalibrationReport{}, nil, fmt.Errorf("%w: duplicate calibration case", ErrCaptureInvalid)
		}
		traceByID[trace.CaseID] = trace
	}
	ordered := make([]CandidateCalibrationTrace, len(development))
	for index, item := range development {
		trace, ok := traceByID[item.ID]
		if !ok || trace.FullObservation.CaseID != item.ID {
			return CalibrationReport{}, nil, fmt.Errorf("%w: calibration case binding", ErrCaptureInvalid)
		}
		ordered[index] = trace
	}

	report := CalibrationReport{
		SchemaVersion:     CalibrationReportSchemaVersion,
		CorpusClass:       memoryeval.RegressionCorpusClass,
		AdmissionMode:     CalibrationAdmissionMode,
		PromotionEligible: false,
		Split:             "development", CaseCount: len(development),
		PolicyID:  usermemory.HybridRelevanceIntentCalibrationPolicyID,
		ProfileID: profileID, ConfigurationSHA256: configurationSHA256,
		SelectionAlgorithm:       calibrationSelectionAlgorithm,
		IntentSelectionAlgorithm: calibrationIntentSelectionAlgorithm,
		Grid:                     developmentCalibrationPlan().Grid,
		Diagnostics:              buildCalibrationDiagnostics(development, ordered),
		Frontier: make([]CalibrationFrontierPoint, 0,
			calibrationMaximumAdmissionBP-calibrationMinimumAdmissionBP+1),
	}
	var selected *CalibrationSelection
	for admissionBP := calibrationMinimumAdmissionBP; admissionBP <= calibrationMaximumAdmissionBP; admissionBP += calibrationStepBP {
		frontier := CalibrationFrontierPoint{ProviderSimilarityBasisPoints: admissionBP}
		for finalBP := calibrationMinimumFinalBP; finalBP <= calibrationMaximumFinalBP; finalBP += calibrationStepBP {
			observations := simulateCalibrationObservations(ordered, admissionBP, finalBP)
			validation, err := memoryeval.EvaluateValidationSelection(
				development,
				observations,
				pool.Corpus.Criteria,
				costs,
			)
			if err != nil {
				return CalibrationReport{}, nil, err
			}
			evaluation := validation.CalibrationEvaluation
			if report.EvaluatedPairCount == 0 {
				report.ProviderCostRatio = validation.ProviderCostRatio
				report.ProviderCostPassed = validation.ProviderCostPassed
			}
			report.EvaluatedPairCount++
			for _, failure := range evaluation.Failures {
				report.Diagnostics.FailurePairCounts[failure]++
			}
			candidate := CalibrationSelection{
				ProviderSimilarityBasisPoints: admissionBP,
				FinalRelevanceBasisPoints:     finalBP,
				Evaluation:                    evaluation,
			}
			if report.Diagnostics.BestSafetyAttempt == nil ||
				betterSafetyDiagnostic(candidate, *report.Diagnostics.BestSafetyAttempt) {
				copy := candidate
				report.Diagnostics.BestSafetyAttempt = &copy
			}
			if report.Diagnostics.BestRecallAttempt == nil ||
				betterRecallDiagnostic(candidate, *report.Diagnostics.BestRecallAttempt) {
				copy := candidate
				report.Diagnostics.BestRecallAttempt = &copy
			}
			if !evaluation.Passed {
				continue
			}
			report.FeasiblePairCount++
			if !frontier.Feasible {
				frontier.Feasible = true
				frontier.FinalRelevanceBasisPoints = finalBP
				frontier.Metrics = evaluation.Metrics
				frontier.Safety = evaluation.Safety
			}
			if selected == nil || betterCalibrationSelection(candidate, *selected) {
				copy := candidate
				selected = &copy
			}
		}
		report.Frontier = append(report.Frontier, frontier)
	}
	report.Selected = selected
	var intentSelected *CalibrationIntentSelection
	for intentBP := calibrationMinimumAdmissionBP; intentBP <= calibrationMaximumAdmissionBP; intentBP += calibrationStepBP {
		observations := simulateIntentCalibrationObservations(ordered, intentBP)
		validation, err := memoryeval.EvaluateValidationSelection(
			development,
			observations,
			pool.Corpus.Criteria,
			costs,
		)
		if err != nil {
			return CalibrationReport{}, nil, err
		}
		report.IntentEvaluatedThresholdCount++
		for _, failure := range validation.CalibrationEvaluation.Failures {
			report.Diagnostics.IntentFailureThresholdCounts[failure]++
		}
		candidate := CalibrationIntentSelection{
			MinimumMemoryIntentMarginBasisPoints: intentBP,
			Evaluation:                           validation.CalibrationEvaluation,
		}
		if report.Diagnostics.BestIntentSafetyAttempt == nil ||
			betterIntentSafetyDiagnostic(candidate, *report.Diagnostics.BestIntentSafetyAttempt) {
			copy := candidate
			report.Diagnostics.BestIntentSafetyAttempt = &copy
		}
		if report.Diagnostics.BestIntentRecallAttempt == nil ||
			betterIntentRecallDiagnostic(candidate, *report.Diagnostics.BestIntentRecallAttempt) {
			copy := candidate
			report.Diagnostics.BestIntentRecallAttempt = &copy
		}
		if !validation.CalibrationEvaluation.Passed {
			continue
		}
		report.IntentFeasibleThresholdCount++
		if intentSelected == nil || betterIntentCalibrationSelection(candidate, *intentSelected) {
			copy := candidate
			intentSelected = &copy
		}
	}
	report.IntentSelected = intentSelected
	body, err := json.Marshal(report)
	if err != nil {
		return CalibrationReport{}, nil, errors.Join(ErrCaptureInvalid, err)
	}
	return report, append(body, '\n'), nil
}

func developmentCalibrationPlan() *CalibrationPlanConfig {
	return &CalibrationPlanConfig{
		SchemaVersion:            CalibrationReportSchemaVersion,
		SelectionAlgorithm:       calibrationSelectionAlgorithm,
		IntentSelectionAlgorithm: calibrationIntentSelectionAlgorithm,
		DiagnosticsVersion:       calibrationDiagnosticsVersion,
		Grid: CalibrationGrid{
			IntentMinimumBasisPoints:    calibrationMinimumAdmissionBP,
			IntentMaximumBasisPoints:    calibrationMaximumAdmissionBP,
			AdmissionMinimumBasisPoints: calibrationMinimumAdmissionBP,
			AdmissionMaximumBasisPoints: calibrationMaximumAdmissionBP,
			FinalMinimumBasisPoints:     calibrationMinimumFinalBP,
			FinalMaximumBasisPoints:     calibrationMaximumFinalBP,
			StepBasisPoints:             calibrationStepBP,
		},
	}
}

func buildCalibrationDiagnostics(
	cases []memoryeval.GoldenCase,
	traces []CandidateCalibrationTrace,
) CalibrationDiagnostics {
	diagnostics := CalibrationDiagnostics{
		Version:                      calibrationDiagnosticsVersion,
		FailurePairCounts:            make(map[string]int),
		IntentFailureThresholdCounts: make(map[string]int),
		MemoryIntentMarginCurve: newCalibrationThresholdCurve(
			calibrationMinimumAdmissionBP,
			calibrationMaximumAdmissionBP,
		),
		AdmissionSimilarityCurve: newCalibrationThresholdCurve(
			calibrationMinimumAdmissionBP,
			calibrationMaximumAdmissionBP,
		),
		MaximumRerankScoreCurve: newCalibrationThresholdCurve(
			calibrationMinimumFinalBP,
			calibrationMaximumFinalBP,
		),
		TopTwoRerankMarginCurve: newCalibrationThresholdCurve(
			calibrationMinimumFinalBP,
			calibrationMaximumFinalBP,
		),
	}
	for index, item := range cases {
		group := calibrationCaseGroup(item)
		if group == calibrationGroupOther {
			diagnostics.OtherCaseCount++
			continue
		}
		trace := traces[index]
		intentReady := trace.MemoryIntentReady && len(trace.FullObservation.CandidateMemoryIDs) > 0
		recordCalibrationCurveValue(
			&diagnostics.MemoryIntentMarginCurve,
			group,
			trace.MemoryIntentMargin,
			intentReady,
		)
		admissionReady := trace.AdmissionReady && len(trace.FullObservation.CandidateMemoryIDs) > 0
		recordCalibrationCurveValue(
			&diagnostics.AdmissionSimilarityCurve,
			group,
			trace.AdmissionSimilarity,
			admissionReady,
		)
		if !trace.RerankReady || len(trace.FinalRelevanceScores) == 0 {
			recordCalibrationCurveValue(
				&diagnostics.MaximumRerankScoreCurve,
				group,
				0,
				false,
			)
			recordCalibrationCurveValue(
				&diagnostics.TopTwoRerankMarginCurve,
				group,
				0,
				false,
			)
			continue
		}
		maximum, second, hasSecond := topTwoCalibrationScores(trace.FinalRelevanceScores)
		recordCalibrationCurveValue(
			&diagnostics.MaximumRerankScoreCurve,
			group,
			maximum,
			true,
		)
		recordCalibrationCurveValue(
			&diagnostics.TopTwoRerankMarginCurve,
			group,
			maximum-second,
			hasSecond,
		)
	}
	return diagnostics
}

type calibrationGroup int

const (
	calibrationGroupOther calibrationGroup = iota
	calibrationGroupRelevant
	calibrationGroupUnrelatedNegative
)

func calibrationCaseGroup(item memoryeval.GoldenCase) calibrationGroup {
	if !item.ExpectedNoMemory && len(item.ExpectedRelevantMemoryIDs) > 0 {
		return calibrationGroupRelevant
	}
	if item.ExpectedNoMemory {
		for _, exclusion := range item.Exclusions {
			if exclusion.Reason == "irrelevant" {
				return calibrationGroupUnrelatedNegative
			}
		}
	}
	return calibrationGroupOther
}

func newCalibrationThresholdCurve(minimum, maximum int) CalibrationThresholdCurve {
	count := (maximum-minimum)/calibrationStepBP + 1
	return CalibrationThresholdCurve{
		MinimumBasisPoints:                 minimum,
		MaximumBasisPoints:                 maximum,
		StepBasisPoints:                    calibrationStepBP,
		RelevantPassingCaseCounts:          make([]int, count),
		UnrelatedNegativePassingCaseCounts: make([]int, count),
	}
}

func recordCalibrationCurveValue(
	curve *CalibrationThresholdCurve,
	group calibrationGroup,
	value float64,
	ready bool,
) {
	if !ready {
		if group == calibrationGroupRelevant {
			curve.RelevantMissingCaseCount++
		} else {
			curve.UnrelatedNegativeMissingCaseCount++
		}
		return
	}
	if group == calibrationGroupRelevant {
		curve.RelevantEligibleCaseCount++
	} else {
		curve.UnrelatedNegativeEligibleCaseCount++
	}
	for index := range curve.RelevantPassingCaseCounts {
		threshold := float64(curve.MinimumBasisPoints+index*curve.StepBasisPoints) / 100
		if value < threshold {
			continue
		}
		if group == calibrationGroupRelevant {
			curve.RelevantPassingCaseCounts[index]++
		} else {
			curve.UnrelatedNegativePassingCaseCounts[index]++
		}
	}
}

func topTwoCalibrationScores(values []float64) (float64, float64, bool) {
	maximum := -1.0
	second := -1.0
	for _, value := range values {
		if value > maximum {
			second = maximum
			maximum = value
		} else if value > second {
			second = value
		}
	}
	if second < 0 {
		return maximum, 0, false
	}
	return maximum, second, true
}

func simulateCalibrationObservations(
	traces []CandidateCalibrationTrace,
	admissionBP int,
	finalBP int,
) []memoryeval.CaseObservation {
	admissionThreshold := float64(admissionBP) / 100
	finalThreshold := float64(finalBP) / 100
	observations := make([]memoryeval.CaseObservation, len(traces))
	for index, trace := range traces {
		observed := cloneCaseObservation(trace.FullObservation)
		if !trace.AdmissionReady || !trace.RerankReady {
			observed.ProviderSentMemoryIDs = []string{}
			observed.FinalMemoryIDs = []string{}
			observed.InjectedMemoryIDs = []string{}
			observed.PromptMemoryTokens = 0
			observed.Fallback = "no_memory"
			observations[index] = observed
			continue
		}
		if trace.AdmissionSimilarity < admissionThreshold {
			observed.ProviderSentMemoryIDs = []string{}
			observed.FinalMemoryIDs = []string{}
			observed.InjectedMemoryIDs = []string{}
			observed.PromptMemoryTokens = 0
			observed.Fallback = "no_memory"
			observations[index] = observed
			continue
		}
		final := make([]string, 0, len(observed.FinalMemoryIDs))
		for position, memoryID := range observed.FinalMemoryIDs {
			if trace.FinalRelevanceScores[position] >= finalThreshold {
				final = append(final, memoryID)
			}
		}
		observed.FinalMemoryIDs = final
		observed.InjectedMemoryIDs = append([]string(nil), final...)
		if len(final) == 0 {
			observed.PromptMemoryTokens = 0
			observed.Fallback = "no_memory"
		}
		observations[index] = observed
	}
	return observations
}

func simulateIntentCalibrationObservations(
	traces []CandidateCalibrationTrace,
	intentBP int,
) []memoryeval.CaseObservation {
	threshold := float64(intentBP) / 100
	observations := make([]memoryeval.CaseObservation, len(traces))
	for index, trace := range traces {
		observed := cloneCaseObservation(trace.FullObservation)
		if trace.MemoryIntentReady && trace.MemoryIntentMargin >= threshold {
			observations[index] = observed
			continue
		}
		observed.ProviderSentMemoryIDs = []string{}
		observed.FinalMemoryIDs = []string{}
		observed.InjectedMemoryIDs = []string{}
		observed.PromptMemoryTokens = 0
		observed.Fallback = "no_memory"
		observations[index] = observed
	}
	return observations
}

func normalizeCalibrationCode(value string) string {
	if value == "" {
		return "NONE"
	}
	for _, current := range value {
		if (current < 'A' || current > 'Z') && (current < '0' || current > '9') && current != '_' {
			return "INVALID"
		}
	}
	if len(value) > 64 {
		return "INVALID"
	}
	return value
}

func cloneCaseObservation(value memoryeval.CaseObservation) memoryeval.CaseObservation {
	value.CandidateMemoryIDs = append([]string(nil), value.CandidateMemoryIDs...)
	value.FinalMemoryIDs = append([]string(nil), value.FinalMemoryIDs...)
	value.InjectedMemoryIDs = append([]string(nil), value.InjectedMemoryIDs...)
	value.PersistedMemoryIDs = append([]string(nil), value.PersistedMemoryIDs...)
	value.ProviderSentMemoryIDs = append([]string(nil), value.ProviderSentMemoryIDs...)
	return value
}

func betterCalibrationSelection(candidate, current CalibrationSelection) bool {
	left := candidate.Evaluation.Metrics
	right := current.Evaluation.Metrics
	if left.FalseInjectionCases != right.FalseInjectionCases {
		return left.FalseInjectionCases < right.FalseInjectionCases
	}
	if left.FinalRecallAt5 != right.FinalRecallAt5 {
		return left.FinalRecallAt5 > right.FinalRecallAt5
	}
	if left.CurrentFactAccuracy != right.CurrentFactAccuracy {
		return left.CurrentFactAccuracy > right.CurrentFactAccuracy
	}
	if candidate.ProviderSimilarityBasisPoints != current.ProviderSimilarityBasisPoints {
		return candidate.ProviderSimilarityBasisPoints < current.ProviderSimilarityBasisPoints
	}
	return candidate.FinalRelevanceBasisPoints < current.FinalRelevanceBasisPoints
}

func betterIntentCalibrationSelection(
	candidate CalibrationIntentSelection,
	current CalibrationIntentSelection,
) bool {
	left := candidate.Evaluation
	right := current.Evaluation
	if left.Metrics.FalseInjectionCases != right.Metrics.FalseInjectionCases {
		return left.Metrics.FalseInjectionCases < right.Metrics.FalseInjectionCases
	}
	if left.Safety.UnauthorizedProviderEgressCount != right.Safety.UnauthorizedProviderEgressCount {
		return left.Safety.UnauthorizedProviderEgressCount <
			right.Safety.UnauthorizedProviderEgressCount
	}
	if left.Metrics.FinalRecallAt5 != right.Metrics.FinalRecallAt5 {
		return left.Metrics.FinalRecallAt5 > right.Metrics.FinalRecallAt5
	}
	if left.Metrics.CurrentFactAccuracy != right.Metrics.CurrentFactAccuracy {
		return left.Metrics.CurrentFactAccuracy > right.Metrics.CurrentFactAccuracy
	}
	return candidate.MinimumMemoryIntentMarginBasisPoints >
		current.MinimumMemoryIntentMarginBasisPoints
}

func betterIntentSafetyDiagnostic(
	candidate CalibrationIntentSelection,
	current CalibrationIntentSelection,
) bool {
	left := candidate.Evaluation
	right := current.Evaluation
	leftSafety := calibrationSafetyBurden(left)
	rightSafety := calibrationSafetyBurden(right)
	if leftSafety != rightSafety {
		return leftSafety < rightSafety
	}
	if len(left.Failures) != len(right.Failures) {
		return len(left.Failures) < len(right.Failures)
	}
	if left.Metrics.FinalRecallAt5 != right.Metrics.FinalRecallAt5 {
		return left.Metrics.FinalRecallAt5 > right.Metrics.FinalRecallAt5
	}
	return candidate.MinimumMemoryIntentMarginBasisPoints >
		current.MinimumMemoryIntentMarginBasisPoints
}

func betterIntentRecallDiagnostic(
	candidate CalibrationIntentSelection,
	current CalibrationIntentSelection,
) bool {
	left := candidate.Evaluation
	right := current.Evaluation
	if left.Metrics.FinalRecallAt5 != right.Metrics.FinalRecallAt5 {
		return left.Metrics.FinalRecallAt5 > right.Metrics.FinalRecallAt5
	}
	if left.Metrics.CurrentFactAccuracy != right.Metrics.CurrentFactAccuracy {
		return left.Metrics.CurrentFactAccuracy > right.Metrics.CurrentFactAccuracy
	}
	leftSafety := calibrationSafetyBurden(left)
	rightSafety := calibrationSafetyBurden(right)
	if leftSafety != rightSafety {
		return leftSafety < rightSafety
	}
	return candidate.MinimumMemoryIntentMarginBasisPoints >
		current.MinimumMemoryIntentMarginBasisPoints
}

func betterSafetyDiagnostic(candidate, current CalibrationSelection) bool {
	left := candidate.Evaluation
	right := current.Evaluation
	leftSafety := calibrationSafetyBurden(left)
	rightSafety := calibrationSafetyBurden(right)
	if leftSafety != rightSafety {
		return leftSafety < rightSafety
	}
	if len(left.Failures) != len(right.Failures) {
		return len(left.Failures) < len(right.Failures)
	}
	if left.Metrics.FinalRecallAt5 != right.Metrics.FinalRecallAt5 {
		return left.Metrics.FinalRecallAt5 > right.Metrics.FinalRecallAt5
	}
	if left.Metrics.CurrentFactAccuracy != right.Metrics.CurrentFactAccuracy {
		return left.Metrics.CurrentFactAccuracy > right.Metrics.CurrentFactAccuracy
	}
	return lowerCalibrationThresholds(candidate, current)
}

func betterRecallDiagnostic(candidate, current CalibrationSelection) bool {
	left := candidate.Evaluation
	right := current.Evaluation
	if left.Metrics.FinalRecallAt5 != right.Metrics.FinalRecallAt5 {
		return left.Metrics.FinalRecallAt5 > right.Metrics.FinalRecallAt5
	}
	if left.Metrics.CurrentFactAccuracy != right.Metrics.CurrentFactAccuracy {
		return left.Metrics.CurrentFactAccuracy > right.Metrics.CurrentFactAccuracy
	}
	if left.Metrics.CandidateRecallAt20 != right.Metrics.CandidateRecallAt20 {
		return left.Metrics.CandidateRecallAt20 > right.Metrics.CandidateRecallAt20
	}
	leftSafety := calibrationSafetyBurden(left)
	rightSafety := calibrationSafetyBurden(right)
	if leftSafety != rightSafety {
		return leftSafety < rightSafety
	}
	if len(left.Failures) != len(right.Failures) {
		return len(left.Failures) < len(right.Failures)
	}
	return lowerCalibrationThresholds(candidate, current)
}

func calibrationSafetyBurden(value memoryeval.CalibrationEvaluation) int {
	safety := value.Safety
	return value.Metrics.FalseInjectionCases +
		safety.CrossUserLeakCount +
		safety.DeletedMemoryLeakCount +
		safety.SecretLeakCount +
		safety.UntrustedSourceLeakCount +
		safety.UnauthorizedProviderEgressCount
}

func lowerCalibrationThresholds(candidate, current CalibrationSelection) bool {
	if candidate.ProviderSimilarityBasisPoints != current.ProviderSimilarityBasisPoints {
		return candidate.ProviderSimilarityBasisPoints < current.ProviderSimilarityBasisPoints
	}
	return candidate.FinalRelevanceBasisPoints < current.FinalRelevanceBasisPoints
}
