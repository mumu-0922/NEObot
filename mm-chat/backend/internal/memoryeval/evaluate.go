package memoryeval

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sort"
)

const (
	// ProviderEgressPolicyLegacyRelevanceGated preserves the historical
	// evaluator contract: every excluded Memory sent to a Provider is an
	// unauthorized egress event. An omitted policy has the same meaning so old
	// artifacts retain byte and scoring compatibility.
	ProviderEgressPolicyLegacyRelevanceGated = "legacy_relevance_gated"
	// ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 permits only the
	// ordinary irrelevant exclusion to cross the Provider boundary. Every
	// authority/privacy exclusion remains forbidden and false-injection scoring
	// is deliberately unaffected.
	ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 = "owner_authorized_normal_candidates_v1"
)

type metricAccumulator struct {
	cases                 int
	relevantCases         int
	negativeCases         int
	currentFactCases      int
	currentFactCorrect    int
	falseInjectionCases   int
	expectedRelevant      int
	candidateRelevantHits int
	finalRelevantHits     int
	ndcg                  float64
	mrr                   float64
	latencies             []int64
	promptTokens          int
	maximumPromptTokens   int
	hardCutoffViolations  int
	safety                SafetyMetrics
}

type evaluatedProfile struct {
	caseCount int
	metrics   Metrics
	ranking   RankingMetrics
	budgets   Budgets
	safety    SafetyMetrics
}

type scoredEvaluation struct {
	profile  ProfileSummary
	slices   map[string]SliceResult
	failures []string
}

func Evaluate(input EvaluationInput) (Report, error) {
	if err := validateGoldenSet(input.Golden); err != nil {
		return Report{}, err
	}
	if err := validateGoldenAdmission(input.Golden); err != nil {
		return Report{}, err
	}
	if err := validateObservationSet(input.Observations); err != nil {
		return Report{}, err
	}
	if err := validateBindings(input); err != nil {
		return Report{}, err
	}

	scored, err := scoreEvaluation(
		input.Golden.Cases,
		input.Observations.Cases,
		input.Golden.Criteria,
		input.Observations.Profile,
		input.Observations.Costs,
	)
	if err != nil {
		return Report{}, err
	}

	split := splitCounts(input.Golden.Cases)
	return Report{
		SchemaVersion: ReportSchemaVersion,
		Passed:        len(scored.failures) == 0,
		Evaluation: EvaluationProvenance{
			EvaluatorVersion:          EvaluatorVersion,
			GoldenCorpusRawSHA256:     input.GoldenRawSHA256,
			GoldenFrozenContentSHA256: input.Golden.Lifecycle.FrozenContentSHA256,
			ObservationsRawSHA256:     input.ObservationsSHA256,
			CaptureID:                 input.Observations.CaptureID,
			HoldoutRunID:              input.Observations.HoldoutRun.ID,
			FixtureManifestSHA256:     input.Golden.FixtureManifestSHA256,
		},
		Golden: GoldenSummary{
			CorpusID:         input.Golden.ID,
			State:            input.Golden.Lifecycle.State,
			FrozenAt:         input.Golden.Lifecycle.FrozenAt,
			TotalReviewed:    len(input.Golden.Cases),
			DevelopmentCount: split["development"],
			ValidationCount:  split["validation"],
			HoldoutCount:     split["holdout"],
			HoldoutRuns:      input.Observations.HoldoutRun.Ordinal,
		},
		Profile:  scored.profile,
		Slices:   scored.slices,
		Failures: scored.failures,
	}, nil
}

func scoreEvaluation(
	cases []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
	profile Profile,
	costs ProviderCosts,
) (scoredEvaluation, error) {
	evaluated, slices, failures, err := scoreCasesAndSlices(
		cases,
		observations,
		criteria,
		profile.ProviderEgressPolicy,
	)
	if err != nil {
		return scoredEvaluation{}, err
	}
	costRatio := float64(costs.MemoryProviderCostMicrounits) /
		float64(costs.ChatProviderCostMicrounits)
	costPassed := providerCostWithinV1Limit(
		costs.MemoryProviderCostMicrounits,
		costs.ChatProviderCostMicrounits,
	)
	if !costPassed {
		failures = append(failures, "Memory provider cost ratio exceeds criterion")
	}
	sort.Strings(failures)

	return scoredEvaluation{
		profile: ProfileSummary{
			ProfileID:          profile.ID,
			ProfileRole:        profile.Role,
			ReaderVersion:      profile.ReaderVersion,
			Metrics:            evaluated.metrics,
			RankingDiagnostics: evaluated.ranking,
			Budgets:            evaluated.budgets,
			Safety:             evaluated.safety,
			ProviderCostRatio:  costRatio,
			ProviderCostPassed: costPassed,
		},
		slices:   slices,
		failures: failures,
	}, nil
}

func scoreCasesAndSlices(
	cases []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
	providerEgressPolicy string,
) (evaluatedProfile, map[string]SliceResult, []string, error) {
	if !validProviderEgressPolicy(providerEgressPolicy) {
		return evaluatedProfile{}, nil, nil, errors.New("Memory Provider-egress policy is invalid")
	}
	evaluated, evaluatedSlices, err := evaluateCasesWithSlices(
		cases,
		observations,
		criteria,
		providerEgressPolicy,
	)
	if err != nil {
		return evaluatedProfile{}, nil, nil, err
	}
	failures := profileFailures(evaluated, criteria)
	slices := make(map[string]SliceResult, len(criticalSlices))
	for _, name := range criticalSlices {
		sliceProfile, ok := evaluatedSlices[name]
		if !ok {
			continue
		}
		sliceFailures := profileFailures(sliceProfile, criteria)
		sort.Strings(sliceFailures)
		slices[name] = SliceResult{
			Cases:              sliceProfile.caseCount,
			Metrics:            sliceProfile.metrics,
			RankingDiagnostics: sliceProfile.ranking,
			Budgets:            sliceProfile.budgets,
			Safety:             sliceProfile.safety,
			Passed:             len(sliceFailures) == 0,
			Failures:           sliceFailures,
		}
		for _, failure := range sliceFailures {
			failures = append(failures, name+": "+failure)
		}
	}
	sort.Strings(failures)
	return evaluated, slices, failures, nil
}

// EvaluateCalibrationSelection scores one already-admitted split through the
// exact benchmark metric/slice implementation without creating a report or
// any promotion authority.
func EvaluateCalibrationSelection(
	cases []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
) (CalibrationEvaluation, error) {
	return EvaluateCalibrationSelectionWithProviderEgressPolicy(
		cases,
		observations,
		criteria,
		"",
	)
}

// EvaluateCalibrationSelectionWithProviderEgressPolicy scores a calibration
// split using one explicit, versioned Provider-egress authority profile. It is
// additive so existing calibration callers and artifacts retain legacy safety
// semantics.
func EvaluateCalibrationSelectionWithProviderEgressPolicy(
	cases []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
	providerEgressPolicy string,
) (CalibrationEvaluation, error) {
	evaluated, slices, failures, err := scoreCasesAndSlices(
		cases,
		observations,
		criteria,
		providerEgressPolicy,
	)
	if err != nil {
		return CalibrationEvaluation{}, err
	}
	return CalibrationEvaluation{
		Passed:             len(failures) == 0,
		Metrics:            evaluated.metrics,
		RankingDiagnostics: evaluated.ranking,
		Budgets:            evaluated.budgets,
		Safety:             evaluated.safety,
		Slices:             slices,
		Failures:           failures,
	}, nil
}

// EvaluateAccuracyFirstCalibrationSelectionWithProviderEgressPolicy reuses
// the exact metric, slice, safety, and token accumulators while excluding
// latency and hard-cutoff verdicts. P95/P99 remain diagnostic output.
func EvaluateAccuracyFirstCalibrationSelectionWithProviderEgressPolicy(
	cases []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
	providerEgressPolicy string,
) (AccuracyFirstCalibrationEvaluation, error) {
	if _, err := MemoryJudgeAccuracyFirstCriteriaV3(criteria); err != nil {
		return AccuracyFirstCalibrationEvaluation{}, err
	}
	if !validProviderEgressPolicy(providerEgressPolicy) {
		return AccuracyFirstCalibrationEvaluation{}, errors.New("Memory Provider-egress policy is invalid")
	}
	evaluated, evaluatedSlices, err := evaluateCasesWithSlices(
		cases,
		observations,
		criteria,
		providerEgressPolicy,
	)
	if err != nil {
		return AccuracyFirstCalibrationEvaluation{}, err
	}
	failures := accuracyFirstProfileFailures(evaluated, criteria)
	slices := make(map[string]AccuracyFirstSliceResult, len(criticalSlices))
	for _, name := range criticalSlices {
		sliceProfile, ok := evaluatedSlices[name]
		if !ok {
			continue
		}
		sliceFailures := accuracyFirstProfileFailures(sliceProfile, criteria)
		sort.Strings(sliceFailures)
		slices[name] = AccuracyFirstSliceResult{
			Cases:              sliceProfile.caseCount,
			Metrics:            sliceProfile.metrics,
			RankingDiagnostics: sliceProfile.ranking,
			Budgets:            accuracyFirstBudgets(sliceProfile.budgets),
			Safety:             sliceProfile.safety,
			Passed:             len(sliceFailures) == 0,
			Failures:           sliceFailures,
		}
		for _, failure := range sliceFailures {
			failures = append(failures, name+": "+failure)
		}
	}
	sort.Strings(failures)
	return AccuracyFirstCalibrationEvaluation{
		Passed:             len(failures) == 0,
		Metrics:            evaluated.metrics,
		RankingDiagnostics: evaluated.ranking,
		Budgets:            accuracyFirstBudgets(evaluated.budgets),
		Safety:             evaluated.safety,
		Slices:             slices,
		Failures:           failures,
	}, nil
}

func accuracyFirstBudgets(value Budgets) AccuracyFirstBudgets {
	return AccuracyFirstBudgets{
		P95LatencyMilliseconds:    value.P95LatencyMilliseconds,
		P99LatencyMilliseconds:    value.P99LatencyMilliseconds,
		AveragePromptMemoryTokens: value.AveragePromptMemoryTokens,
		MaximumPromptMemoryTokens: value.MaximumPromptMemoryTokens,
		PromptTokenPassed:         value.PromptTokenPassed,
	}
}

func accuracyFirstProfileFailures(value evaluatedProfile, criteria Criteria) []string {
	failures := make([]string, 0)
	if value.metrics.CandidateRecallAt20 < criteria.MinimumCandidateRecallAt20 {
		failures = append(failures, "candidate recall@20 below criterion")
	}
	if value.metrics.FinalRecallAt5 < criteria.MinimumFinalRecallAt5 {
		failures = append(failures, "final recall@5 below criterion")
	}
	if value.metrics.CurrentFactAccuracy < criteria.MinimumCurrentFactAccuracy {
		failures = append(failures, "current-fact accuracy below criterion")
	}
	if value.metrics.FalseInjectionRate > criteria.MaximumFalseInjectionRate {
		failures = append(failures, "false-injection rate above criterion")
	}
	if !value.budgets.PromptTokenPassed {
		failures = append(failures, "prompt Memory token budget exceeds criterion")
	}
	if !value.safety.Passed {
		failures = append(failures, "Memory safety or authority leakage was observed")
	}
	return failures
}

// EvaluateValidationSelection applies the same split metrics, slice gates,
// safety checks, and exact v1 Provider-cost gate used by the full evaluator.
func EvaluateValidationSelection(
	cases []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
	costs ProviderCosts,
) (ValidationEvaluation, error) {
	return EvaluateValidationSelectionWithProviderEgressPolicy(
		cases,
		observations,
		criteria,
		costs,
		"",
	)
}

// EvaluateValidationSelectionWithProviderEgressPolicy applies the exact
// benchmark gates with an explicit Provider-egress authority profile.
func EvaluateValidationSelectionWithProviderEgressPolicy(
	cases []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
	costs ProviderCosts,
	providerEgressPolicy string,
) (ValidationEvaluation, error) {
	scored, err := scoreEvaluation(
		cases,
		observations,
		criteria,
		Profile{ProviderEgressPolicy: providerEgressPolicy},
		costs,
	)
	if err != nil {
		return ValidationEvaluation{}, err
	}
	return ValidationEvaluation{
		CalibrationEvaluation: CalibrationEvaluation{
			Passed:             len(scored.failures) == 0,
			Metrics:            scored.profile.Metrics,
			RankingDiagnostics: scored.profile.RankingDiagnostics,
			Budgets:            scored.profile.Budgets,
			Safety:             scored.profile.Safety,
			Slices:             scored.slices,
			Failures:           scored.failures,
		},
		ProviderCostRatio:  scored.profile.ProviderCostRatio,
		ProviderCostPassed: scored.profile.ProviderCostPassed,
	}, nil
}

func validateBindings(input EvaluationInput) error {
	if !validSHA256(input.GoldenRawSHA256) ||
		!validSHA256(input.ObservationsSHA256) {
		return errors.New("Memory evaluation raw hashes are invalid")
	}
	if input.Observations.GoldenSetID != input.Golden.ID ||
		input.Observations.GoldenCorpusSHA256 != input.Golden.Lifecycle.FrozenContentSHA256 {
		return errors.New("Memory observations are not bound to the frozen corpus")
	}
	if input.Observations.FixtureManifestSHA256 != input.Golden.FixtureManifestSHA256 {
		return errors.New("Memory observations are not bound to the fixture manifest")
	}
	if input.Observations.HoldoutRun.ID != input.Golden.Lifecycle.HoldoutRunID ||
		input.Observations.HoldoutRun.Ordinal != 1 {
		return errors.New("Memory Holdout must have exactly one precommitted run")
	}
	frozenAt, _ := parseTimestamp(input.Golden.Lifecycle.FrozenAt)
	holdoutAt, _ := parseTimestamp(input.Observations.HoldoutRun.ExecutedAt)
	capturedAt, _ := parseTimestamp(input.Observations.CapturedAt)
	if holdoutAt.Before(frozenAt) || holdoutAt.After(capturedAt) {
		return errors.New("Memory Holdout timestamp is outside the frozen capture window")
	}
	if len(input.Observations.Cases) != len(input.Golden.Cases) {
		return errors.New("Memory observations do not exactly match the Golden corpus")
	}
	for index, goldenCase := range input.Golden.Cases {
		if input.Observations.Cases[index].CaseID != goldenCase.ID {
			return fmt.Errorf("Memory observation order differs at case %q", goldenCase.ID)
		}
	}
	return nil
}

func evaluateCases(
	golden []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
) (evaluatedProfile, error) {
	evaluated, _, err := evaluateCasesWithSlices(golden, observations, criteria, "")
	return evaluated, err
}

func evaluateCasesWithSlices(
	golden []GoldenCase,
	observations []CaseObservation,
	criteria Criteria,
	providerEgressPolicy string,
) (evaluatedProfile, map[string]evaluatedProfile, error) {
	observedByCase := make(map[string]CaseObservation, len(observations))
	for _, item := range observations {
		if _, duplicate := observedByCase[item.CaseID]; duplicate {
			return evaluatedProfile{}, nil, fmt.Errorf("duplicate Memory observation case %q", item.CaseID)
		}
		observedByCase[item.CaseID] = item
	}
	if len(observedByCase) != len(golden) {
		return evaluatedProfile{}, nil, errors.New("Memory observations do not exactly match selected Golden cases")
	}
	accumulator := metricAccumulator{}
	sliceAccumulators := make(map[string]*metricAccumulator, len(criticalSlices))
	for _, goldenCase := range golden {
		observed, ok := observedByCase[goldenCase.ID]
		if !ok {
			return evaluatedProfile{}, nil, fmt.Errorf("missing Memory observation case %q", goldenCase.ID)
		}
		delete(observedByCase, goldenCase.ID)
		accumulateCase(
			&accumulator,
			goldenCase,
			observed,
			criteria,
			providerEgressPolicy,
		)
		memberships := stringSet(goldenCase.Slices)
		for _, name := range criticalSlices {
			if _, selected := memberships[name]; !selected {
				continue
			}
			sliceAccumulator := sliceAccumulators[name]
			if sliceAccumulator == nil {
				sliceAccumulator = &metricAccumulator{}
				sliceAccumulators[name] = sliceAccumulator
			}
			accumulateCase(
				sliceAccumulator,
				goldenCase,
				observed,
				criteria,
				providerEgressPolicy,
			)
		}
	}
	if len(observedByCase) != 0 {
		return evaluatedProfile{}, nil, errors.New("Memory observations contain unknown cases")
	}
	evaluatedSlices := make(map[string]evaluatedProfile, len(sliceAccumulators))
	for name, sliceAccumulator := range sliceAccumulators {
		sliceResult := sliceAccumulator.result(criteria)
		sliceResult.caseCount = sliceAccumulator.cases
		evaluatedSlices[name] = sliceResult
	}
	result := accumulator.result(criteria)
	result.caseCount = accumulator.cases
	return result, evaluatedSlices, nil
}

func accumulateCase(
	accumulator *metricAccumulator,
	golden GoldenCase,
	observed CaseObservation,
	criteria Criteria,
	providerEgressPolicy string,
) {
	accumulator.cases++
	accumulator.latencies = append(accumulator.latencies, observed.LatencyMilliseconds)
	accumulator.promptTokens += observed.PromptMemoryTokens
	accumulator.maximumPromptTokens = max(
		accumulator.maximumPromptTokens,
		observed.PromptMemoryTokens,
	)
	if observed.LatencyMilliseconds > criteria.HardCutoffMilliseconds ||
		(observed.LatencyMilliseconds >= criteria.HardCutoffMilliseconds &&
			!observed.HardCutoffApplied) {
		accumulator.hardCutoffViolations++
	}

	relevant := stringSet(golden.ExpectedRelevantMemoryIDs)
	if golden.ExpectedNoMemory {
		accumulator.negativeCases++
	} else {
		accumulator.relevantCases++
		accumulator.ndcg += ndcgAt5(observed.FinalMemoryIDs, relevant)
		accumulator.mrr += reciprocalRankAt5(observed.FinalMemoryIDs, relevant)
	}
	accumulator.expectedRelevant += len(relevant)
	accumulator.candidateRelevantHits += countAllowed(observed.CandidateMemoryIDs, relevant)
	accumulator.finalRelevantHits += countAllowed(observed.FinalMemoryIDs, relevant)
	if containsUnexpected(observed.InjectedMemoryIDs, relevant) {
		accumulator.falseInjectionCases++
	}

	if len(golden.ExpectedCurrentMemoryIDs) > 0 {
		accumulator.currentFactCases++
		if currentFactCorrect(golden, observed) {
			accumulator.currentFactCorrect++
		}
	}
	accumulateSafety(
		&accumulator.safety,
		golden,
		observed,
		providerEgressPolicy,
	)
}

func currentFactCorrect(golden GoldenCase, observed CaseObservation) bool {
	injected := stringSet(observed.InjectedMemoryIDs)
	for _, expected := range golden.ExpectedCurrentMemoryIDs {
		if _, ok := injected[expected]; !ok {
			return false
		}
	}
	for _, exclusion := range golden.Exclusions {
		if exclusion.Reason == "superseded" {
			if _, leaked := injected[exclusion.MemoryID]; leaked {
				return false
			}
		}
	}
	return true
}

func accumulateSafety(
	safety *SafetyMetrics,
	golden GoldenCase,
	observed CaseObservation,
	providerEgressPolicy string,
) {
	allSurfaces := append([]string(nil), observed.CandidateMemoryIDs...)
	allSurfaces = append(allSurfaces, observed.FinalMemoryIDs...)
	allSurfaces = append(allSurfaces, observed.InjectedMemoryIDs...)
	allSurfaces = append(allSurfaces, observed.PersistedMemoryIDs...)
	allSurfaces = append(allSurfaces, observed.ProviderSentMemoryIDs...)
	all := stringSet(allSurfaces)
	providerSent := stringSet(observed.ProviderSentMemoryIDs)

	leakedReasons := make(map[string]bool)
	providerLeak := false
	for _, exclusion := range golden.Exclusions {
		if _, leaked := all[exclusion.MemoryID]; leaked {
			leakedReasons[exclusion.Reason] = true
		}
		if _, leaked := providerSent[exclusion.MemoryID]; leaked &&
			!providerEgressAuthorized(providerEgressPolicy, exclusion.Reason) {
			providerLeak = true
		}
	}
	if leakedReasons["cross_user"] || leakedReasons["out_of_scope"] {
		safety.CrossUserLeakCount++
	}
	if leakedReasons["deleted"] {
		safety.DeletedMemoryLeakCount++
	}
	if leakedReasons["secret"] {
		safety.SecretLeakCount++
	}
	if leakedReasons["untrusted_source"] {
		safety.UntrustedSourceLeakCount++
	}
	if providerLeak {
		safety.UnauthorizedProviderEgressCount++
	}
}

func validProviderEgressPolicy(value string) bool {
	return value == "" ||
		value == ProviderEgressPolicyLegacyRelevanceGated ||
		value == ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1
}

func providerEgressAuthorized(policy string, exclusionReason string) bool {
	return policy == ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 &&
		exclusionReason == "irrelevant"
}

func (value metricAccumulator) result(criteria Criteria) evaluatedProfile {
	metrics := Metrics{
		CandidateRecallAt20:  ratio(value.candidateRelevantHits, value.expectedRelevant),
		FinalRecallAt5:       ratio(value.finalRelevantHits, value.expectedRelevant),
		CurrentFactAccuracy:  ratio(value.currentFactCorrect, value.currentFactCases),
		FalseInjectionRate:   ratio(value.falseInjectionCases, value.cases),
		RelevantCaseCount:    value.relevantCases,
		NegativeCaseCount:    value.negativeCases,
		CurrentFactCaseCount: value.currentFactCases,
		FalseInjectionCases:  value.falseInjectionCases,
	}
	ranking := RankingMetrics{
		NDCGAt5: ratioFloat(value.ndcg, value.relevantCases),
		MRRAt5:  ratioFloat(value.mrr, value.relevantCases),
	}
	budgets := Budgets{
		P95LatencyMilliseconds:    percentile(value.latencies, 0.95),
		P99LatencyMilliseconds:    percentile(value.latencies, 0.99),
		AveragePromptMemoryTokens: ratioFloat(float64(value.promptTokens), value.cases),
		MaximumPromptMemoryTokens: value.maximumPromptTokens,
		HardCutoffViolationCount:  value.hardCutoffViolations,
	}
	budgets.LatencyPassed = budgets.P95LatencyMilliseconds <=
		criteria.MaximumP95LatencyMilliseconds &&
		budgets.P99LatencyMilliseconds <= criteria.MaximumP99LatencyMilliseconds
	budgets.PromptTokenPassed = budgets.AveragePromptMemoryTokens <=
		criteria.MaximumAveragePromptMemoryTokens &&
		budgets.MaximumPromptMemoryTokens <= criteria.MaximumPromptMemoryTokens
	budgets.HardCutoffPassed = budgets.HardCutoffViolationCount == 0
	value.safety.Passed = value.safety.CrossUserLeakCount == 0 &&
		value.safety.DeletedMemoryLeakCount == 0 &&
		value.safety.SecretLeakCount == 0 &&
		value.safety.UntrustedSourceLeakCount == 0 &&
		value.safety.UnauthorizedProviderEgressCount == 0
	return evaluatedProfile{
		metrics: metrics,
		ranking: ranking,
		budgets: budgets,
		safety:  value.safety,
	}
}

func profileFailures(value evaluatedProfile, criteria Criteria) []string {
	failures := make([]string, 0)
	if value.metrics.CandidateRecallAt20 < criteria.MinimumCandidateRecallAt20 {
		failures = append(failures, "candidate recall@20 below criterion")
	}
	if value.metrics.FinalRecallAt5 < criteria.MinimumFinalRecallAt5 {
		failures = append(failures, "final recall@5 below criterion")
	}
	if value.metrics.CurrentFactAccuracy < criteria.MinimumCurrentFactAccuracy {
		failures = append(failures, "current-fact accuracy below criterion")
	}
	if value.metrics.FalseInjectionRate > criteria.MaximumFalseInjectionRate {
		failures = append(failures, "false-injection rate above criterion")
	}
	if !value.budgets.LatencyPassed {
		failures = append(failures, "Memory recall latency exceeds criterion")
	}
	if !value.budgets.PromptTokenPassed {
		failures = append(failures, "prompt Memory token budget exceeds criterion")
	}
	if !value.budgets.HardCutoffPassed {
		failures = append(failures, "Memory recall hard cutoff was violated")
	}
	if !value.safety.Passed {
		failures = append(failures, "Memory safety or authority leakage was observed")
	}
	return failures
}

func countAllowed(values []string, allowed map[string]struct{}) int {
	count := 0
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			count++
		}
	}
	return count
}

func containsUnexpected(values []string, allowed map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := allowed[value]; !ok {
			return true
		}
	}
	return false
}

func reciprocalRankAt5(values []string, relevant map[string]struct{}) float64 {
	for index, value := range values {
		if index >= 5 {
			break
		}
		if _, ok := relevant[value]; ok {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func ndcgAt5(values []string, relevant map[string]struct{}) float64 {
	dcg := 0.0
	for index, value := range values {
		if index >= 5 {
			break
		}
		if _, ok := relevant[value]; ok {
			dcg += 1 / math.Log2(float64(index)+2)
		}
	}
	ideal := 0.0
	for index := 0; index < min(len(relevant), 5); index++ {
		ideal += 1 / math.Log2(float64(index)+2)
	}
	if ideal == 0 {
		return 1
	}
	return dcg / ideal
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}

func ratioFloat(numerator float64, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return numerator / float64(denominator)
}

func percentile(values []int64, quantile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := int(math.Ceil(quantile*float64(len(ordered)))) - 1
	return ordered[max(index, 0)]
}

func providerCostWithinV1Limit(memoryCost, chatCost uint64) bool {
	// The v1 criterion is frozen at 0.15 = 3/20. Compare 128-bit products so
	// large integer microunit totals cannot cross the boundary through float64
	// rounding or uint64 multiplication overflow.
	memoryHigh, memoryLow := bits.Mul64(memoryCost, 20)
	chatHigh, chatLow := bits.Mul64(chatCost, 3)
	if memoryHigh != chatHigh {
		return memoryHigh < chatHigh
	}
	return memoryLow <= chatLow
}
