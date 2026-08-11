package memorycapture

import (
	"bytes"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestSingleUserBoundedMissDevelopmentUsesFreshCriteriaAndCostIdentity(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	protected := doubleConfirmationValidationProtected(pool)
	cost := singleUserBoundedMissDevelopmentTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildSingleUserBoundedMissDevelopmentProfileConfig(
		protected,
		costSHA256,
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	criteriaSHA256, err := singleUserBoundedMissCriteriaSHA256(pool.Corpus.Criteria)
	if err != nil {
		t.Fatal(err)
	}
	legacyCriteriaSHA256, err := productionValidationCriteriaSHA256(pool.Corpus.Criteria)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v24-single-user-bounded-miss-development.v1" ||
		config.ReaderVersion != SingleUserBoundedMissDevelopmentReaderVersion ||
		config.CaptureMode != CaptureModeSingleUserBoundedMissDevelopment ||
		config.EvaluationCriteriaVersion != memoryeval.MemoryJudgeSingleUserBoundedMissCriteriaVersionV4 ||
		config.EvaluationCriteriaSHA256 != criteriaSHA256 ||
		criteriaSHA256 == legacyCriteriaSHA256 ||
		config.AccuracyFirstExecutionPolicy == nil ||
		config.AccuracyFirstExecutionPolicy.SequenceVersion != SingleUserBoundedMissDevelopmentExecutionSequenceV1 {
		t.Fatalf("bounded-miss Development config=%#v", config)
	}
	if ValidateSingleUserBoundedMissDevelopmentCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	) != nil {
		t.Fatal("fresh Development cost authority was rejected")
	}
	if ValidateDoubleConfirmationDevelopmentCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	) == nil {
		t.Fatal("consumed v22 validator accepted v24 cost authority")
	}

	traces := passingCloudJudgeDevelopmentTraces(pool)
	setNegativePolicyGuardTrace(t, pool, traces)
	confirmationInputTokens := addTwoConfirmationAttempts(t, traces)
	profile, logicalRequests, logicalInputTokens := negativePolicyGuardProfile(traces)
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	profile.Profile.ReaderVersion = SingleUserBoundedMissDevelopmentReaderVersion
	profile.Profile.ConfigurationSHA256 = configurationSHA256
	profile.Costs = cost.Candidate
	profile.ProviderAttempts = accuracyFirstTelemetry(logicalRequests, logicalInputTokens, 0, 0)
	profile.ProviderAttempts.JudgeAttempts += 2
	profile.ProviderAttempts.JudgeConfirmationAttempts = 2
	profile.ProviderAttempts.JudgeConfirmationInputTokenUpperBound = 2 * confirmationInputTokens
	profile.ProviderAttempts.JudgeLatency = testAccuracyFirstLatency(
		profile.ProviderAttempts.JudgeAttempts,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, reportBody, err := BuildSingleUserBoundedMissDevelopmentReport(
		pool,
		profile,
		config,
		FixedMemoryJudgeAuthority(),
		cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed ||
		report.SchemaVersion != SingleUserBoundedMissDevelopmentReportSchemaVersion ||
		report.EvaluationCriteriaSHA256 != criteriaSHA256 ||
		report.EvaluationCriteria.MinimumRequiredSliceCurrentFactAccuracy != 0.90 ||
		report.EvaluationCriteria.MaximumFalseInjectionCases != 0 ||
		bytes.Count(reportBody, []byte(`"evaluationCriteria":`)) != 1 ||
		bytes.Count(reportBody, []byte(`"evaluation":`)) != 1 {
		t.Fatalf("bounded-miss Development report=%#v body=%s", report, reportBody)
	}
	startedAt := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	manifest, _, err := BuildSingleUserBoundedMissDevelopmentRunManifest(
		"run-bounded-miss-development",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		costSHA256,
		report,
		[]Artifact{{Name: SingleUserBoundedMissDevelopmentArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeSingleUserBoundedMissDevelopment ||
		manifest.EvaluationCriteriaSHA256 != criteriaSHA256 {
		t.Fatalf("bounded-miss Development manifest=%#v", manifest)
	}
}

func TestSingleUserBoundedMissValidationAcceptsOneBoundedMissButNoFalseMemory(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	protected := doubleConfirmationValidationProtected(pool)
	cost := singleUserBoundedMissValidationTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildSingleUserBoundedMissValidationProfileConfig(
		protected,
		costSHA256,
		ProviderModeLiveSiliconFlow,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v25-single-user-bounded-miss-validation.v1" ||
		config.ReaderVersion != SingleUserBoundedMissValidationReaderVersion ||
		config.CaptureMode != CaptureModeSingleUserBoundedMissValidation ||
		config.EvaluationCriteriaVersion != memoryeval.MemoryJudgeSingleUserBoundedMissCriteriaVersionV4 ||
		config.AccuracyFirstExecutionPolicy == nil ||
		config.AccuracyFirstExecutionPolicy.SequenceVersion != SingleUserBoundedMissValidationExecutionSequenceV1 {
		t.Fatalf("bounded-miss Validation config=%#v", config)
	}
	if ValidateSingleUserBoundedMissValidationCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	) != nil {
		t.Fatal("fresh Validation cost authority was rejected")
	}
	if ValidateDoubleConfirmationValidationCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	) == nil {
		t.Fatal("consumed schema-v23 validator accepted v25 cost authority")
	}

	profile := doubleConfirmationValidationProfile(
		t,
		pool,
		config,
		cost,
		ProviderModeLiveSiliconFlow,
	)
	profile.Profile.ReaderVersion = SingleUserBoundedMissValidationReaderVersion
	omitOneCurrentFact(t, pool, &profile, "stable_fact")
	report, reportBody, err := BuildSingleUserBoundedMissValidationReport(
		pool,
		profile,
		config,
		FixedMemoryJudgeAuthority(),
		cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	stable := report.Evaluation.Slices["stable_fact"]
	if !report.Passed || !report.Evaluation.Passed ||
		stable.Metrics.CurrentFactAccuracy != 0.90 || !stable.Passed ||
		report.Evaluation.Metrics.CurrentFactAccuracy < 0.95 ||
		report.Evaluation.Metrics.FalseInjectionCases != 0 ||
		report.Outcome.Severity != ProductionValidationSeverityNone ||
		bytes.Contains(reportBody, []byte(`"minimumRequiredSliceCurrentFactAccuracy":0.95`)) {
		t.Fatalf("bounded-miss Validation report=%#v", report)
	}
	startedAt := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	manifest, _, err := BuildSingleUserBoundedMissValidationRunManifest(
		"run-bounded-miss-validation",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ProviderModeLiveSiliconFlow,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		costSHA256,
		report,
		[]Artifact{{Name: SingleUserBoundedMissValidationArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible || manifest.ReleaseEligible ||
		manifest.CaptureMode != CaptureModeSingleUserBoundedMissValidation {
		t.Fatalf("bounded-miss Validation manifest=%#v", manifest)
	}

	falseMemory := profile
	falseMemory.Cases = append([]memoryeval.CaseObservation(nil), profile.Cases...)
	falseMemory.Calibration = append([]CandidateCalibrationTrace(nil), profile.Calibration...)
	falseMemory.Cases[0].InjectedMemoryIDs = append(
		append([]string(nil), falseMemory.Cases[0].InjectedMemoryIDs...),
		"unexpected-memory",
	)
	falseMemory.Calibration[0].FullObservation = falseMemory.Cases[0]
	failed, _, err := BuildSingleUserBoundedMissValidationReport(
		pool,
		falseMemory,
		config,
		FixedMemoryJudgeAuthority(),
		cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Passed || failed.Evaluation.Passed ||
		failed.Evaluation.Metrics.FalseInjectionCases != 1 ||
		failed.Outcome.Severity != ProductionValidationSeverityOrange ||
		len(failed.Outcome.Reasons) != 1 ||
		failed.Outcome.Reasons[0] != singleUserBoundedMissValidationReasonInjection {
		t.Fatalf("false Memory was accepted: %#v", failed)
	}
}

func addTwoConfirmationAttempts(
	t *testing.T,
	traces []CandidateCalibrationTrace,
) int {
	t.Helper()
	for index := range traces {
		if traces[index].CloudJudgeInputTokenUpperBound <= 0 {
			continue
		}
		inputTokens := traces[index].CloudJudgeInputTokenUpperBound
		traces[index].CloudJudgeInputTokenUpperBound += 2 * inputTokens
		return inputTokens
	}
	t.Fatal("fixture has no Judge-completed case")
	return 0
}

func omitOneCurrentFact(
	t *testing.T,
	pool memoryauthor.RegressionPool,
	profile *CapturedProfile,
	sliceName string,
) {
	t.Helper()
	caseByID := make(map[string]memoryeval.GoldenCase, len(pool.Corpus.Cases))
	for _, item := range pool.Corpus.Cases {
		caseByID[item.ID] = item
	}
	for index := range profile.Cases {
		item := caseByID[profile.Cases[index].CaseID]
		if len(item.ExpectedCurrentMemoryIDs) == 0 ||
			len(item.ExpectedRelevantMemoryIDs) != 1 ||
			!containsText(item.Slices, sliceName) {
			continue
		}
		profile.Cases[index].FinalMemoryIDs = nil
		profile.Cases[index].InjectedMemoryIDs = nil
		profile.Cases[index].PromptMemoryTokens = 0
		profile.Calibration[index].FullObservation = profile.Cases[index]
		profile.Calibration[index].FinalRelevanceScores = nil
		return
	}
	t.Fatalf("no current-fact case found for slice %q", sliceName)
}

func containsText(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func singleUserBoundedMissDevelopmentTestCostBasis() CostBasis {
	cost := doubleConfirmationDevelopmentTestCostBasis()
	cost.SchemaVersion =
		"neo-chat.memory-regression-cost-basis.v24-single-user-bounded-miss-development.v1"
	return cost
}

func singleUserBoundedMissValidationTestCostBasis() CostBasis {
	cost := doubleConfirmationValidationTestCostBasis()
	cost.SchemaVersion =
		"neo-chat.memory-regression-cost-basis.v25-single-user-bounded-miss-validation.v1"
	return cost
}

func TestSingleUserBoundedMissValidationFakeEvidenceCannotPass(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	protected := doubleConfirmationValidationProtected(pool)
	cost := singleUserBoundedMissValidationTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildSingleUserBoundedMissValidationProfileConfig(
		protected,
		costSHA256,
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	profile := doubleConfirmationValidationProfile(
		t,
		pool,
		config,
		cost,
		ProviderModeFakeProtocol,
	)
	profile.Profile.ReaderVersion = SingleUserBoundedMissValidationReaderVersion
	report, _, err := BuildSingleUserBoundedMissValidationReport(
		pool,
		profile,
		config,
		FixedMemoryJudgeAuthority(),
		cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || !report.Evaluation.Passed ||
		report.EvidenceClass != ProductionValidationEvidenceFake ||
		report.Outcome.RequiredAction != ProductionValidationActionRetainBeta {
		t.Fatalf("Fake evidence gained launch authority: %#v", report)
	}
}
