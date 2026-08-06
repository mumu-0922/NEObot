package memorycapture

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestProductionMemoryJudgeValidationProfileIsSchemaV15AndHistoricalJSONOmitsFields(
	t *testing.T,
) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	protected := productionValidationProtected(pool)
	cost := productionMemoryJudgeValidationTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildProductionMemoryJudgeValidationProfileConfig(
		protected,
		costSHA256,
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := ProductionMemoryJudgeValidationExecutionPolicy(ProviderModeFakeProtocol)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v15" ||
		config.ReaderVersion != ProductionMemoryJudgeValidationReaderVersion ||
		config.CaptureMode != CaptureModeProductionMemoryJudgeValidation ||
		config.EvaluationSplit != FrozenValidationSplit ||
		config.RelevancePolicyID != usermemory.HybridRelevanceProductionJudgePolicyID ||
		config.RelevancePolicyMode != "fixed_cloud_candidate_judge_production" ||
		config.MemoryReadIntentPolicyVersion != chat.MemoryReadIntentPolicyVersion ||
		config.MemoryReadIntentPolicySHA256 != chat.MemoryReadIntentPolicySHA256 ||
		!validSHA256String(config.ValidationCaseOrderSHA256) ||
		!validSHA256String(config.EvaluationCriteriaSHA256) ||
		!validSHA256String(config.ProductionRelevancePolicySHA256) ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != execution {
		t.Fatalf("production Validation config=%#v", config)
	}

	historicalBuilders := []func() (ProfileConfig, error){
		func() (ProfileConfig, error) {
			return BuildAccuracyFirstMemoryJudgeDevelopmentProfileConfig(
				protected, costSHA256, ProviderModeFakeProtocol,
				FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
			)
		},
		func() (ProfileConfig, error) {
			return BuildJudgeFailureDiagnosticDevelopmentProfileConfig(
				protected, costSHA256, ProviderModeFakeProtocol,
				FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
			)
		},
		func() (ProfileConfig, error) {
			return BuildTransportStableMemoryJudgeDevelopmentProfileConfig(
				protected, costSHA256, ProviderModeFakeProtocol,
				FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
			)
		},
	}
	for index, build := range historicalBuilders {
		historical, buildErr := build()
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		body, marshalErr := json.Marshal(historical)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, field := range []string{
			"validationCaseOrderSha256",
			"evaluationCriteriaSha256",
			"productionRelevancePolicySha256",
			"memoryReadIntentPolicyVersion",
			"memoryReadIntentPolicySha256",
		} {
			if bytes.Contains(body, []byte(field)) {
				t.Fatalf("historical profile[%d] gained schema-v15 field %q", index, field)
			}
		}
	}
}

func TestProductionMemoryJudgeValidationFakeEvidenceNeverPassesAndReplaysDeterministically(
	t *testing.T,
) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	protected := productionValidationProtected(pool)
	cost := productionMemoryJudgeValidationTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildProductionMemoryJudgeValidationProfileConfig(
		protected,
		costSHA256,
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	profile := productionValidationProfile(t, pool, config, cost, ProviderModeFakeProtocol)
	report, firstBody, err := BuildProductionMemoryJudgeValidationReport(
		pool, profile, config, FixedMemoryJudgeAuthority(), cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, secondBody, err := BuildProductionMemoryJudgeValidationReport(
		pool, profile, config, FixedMemoryJudgeAuthority(), cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBody, secondBody) || report.Passed ||
		report.EvidenceClass != ProductionValidationEvidenceFake ||
		report.Outcome.Severity != ProductionValidationSeverityYellow ||
		report.Outcome.RequiredAction != ProductionValidationActionRetainBeta ||
		len(report.Outcome.Reasons) != 1 ||
		report.Outcome.Reasons[0] != productionValidationReasonFake ||
		report.PromotionEligible || report.ReleaseEligible || report.PolicySelected {
		t.Fatalf("fake production Validation report=%#v", report)
	}
	for _, forbidden := range [][]byte{
		[]byte(pool.Corpus.Cases[0].ID),
		[]byte(`"caseId"`),
		[]byte(`"query"`),
		[]byte(`"canonicalContent"`),
		[]byte(`"providerResponse"`),
		[]byte(`"providerError"`),
		[]byte(`"rawScore"`),
		[]byte(`"selectedOrdinals"`),
	} {
		if bytes.Contains(bytes.ToLower(firstBody), bytes.ToLower(forbidden)) {
			t.Fatalf("production Validation retained forbidden surface %q", forbidden)
		}
	}
	startedAt := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	buildManifest := func() (ProductionMemoryJudgeValidationRunManifest, []byte, error) {
		return BuildProductionMemoryJudgeValidationRunManifest(
			"run-production-validation",
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			ProviderModeFakeProtocol,
			startedAt,
			startedAt.Add(time.Minute),
			protected,
			costSHA256,
			report,
			[]Artifact{{Name: ProductionMemoryJudgeValidationArtifactName, Body: firstBody}},
		)
	}
	manifest, firstManifestBody, err := buildManifest()
	if err != nil {
		t.Fatal(err)
	}
	_, secondManifestBody, err := buildManifest()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstManifestBody, secondManifestBody) || manifest.Passed ||
		manifest.PromotionEligible || manifest.ReleaseEligible ||
		manifest.CaptureMode != CaptureModeProductionMemoryJudgeValidation ||
		manifest.EvidenceClass != ProductionValidationEvidenceFake ||
		!equalProductionValidationOutcome(manifest.Outcome, report.Outcome) {
		t.Fatalf("production Validation manifest=%#v", manifest)
	}
}

func TestProductionMemoryJudgeValidationLivePassAndTerminalFailureSemantics(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	protected := productionValidationProtected(pool)
	cost := productionMemoryJudgeValidationTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildProductionMemoryJudgeValidationProfileConfig(
		protected,
		costSHA256,
		ProviderModeLiveSiliconFlow,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	profile := productionValidationProfile(t, pool, config, cost, ProviderModeLiveSiliconFlow)
	report, _, err := BuildProductionMemoryJudgeValidationReport(
		pool, profile, config, FixedMemoryJudgeAuthority(), cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.EvidenceClass != ProductionValidationEvidenceLive ||
		report.Outcome.Severity != ProductionValidationSeverityNone ||
		report.Outcome.RequiredAction != ProductionValidationActionOwnerReview {
		t.Fatalf("live production Validation report=%#v", report)
	}

	failed := -1
	for index := range profile.Calibration {
		if profile.Calibration[index].CloudJudgeInputTokenUpperBound > 0 {
			failed = index
			break
		}
	}
	if failed < 0 {
		t.Fatal("candidate-bearing Validation trace missing")
	}
	trace := &profile.Calibration[failed]
	trace.CloudJudgeReady = false
	trace.CloudJudgeFailureCategory = string(chat.ProviderFailureTransportFailed)
	trace.AbstentionCode = "CANDIDATE_JUDGE_FAILED"
	trace.ResultCode = "CANDIDATE_JUDGE_FAILED"
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	trace.FullObservation.Fallback = "no_memory"
	trace.FinalRelevanceScores = []float64{}
	profile.Cases[failed] = trace.FullObservation
	logicalRequests, logicalInputTokens := productionValidationLogicalJudgeTelemetry(profile.Calibration)
	profile.ProviderAttempts = productionValidationTelemetry(
		logicalRequests,
		logicalInputTokens,
		2,
		2*trace.CloudJudgeInputTokenUpperBound,
		ProviderModeLiveSiliconFlow,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{
		string(chat.ProviderFailureTransportFailed): 3,
	}
	failedReport, _, err := BuildProductionMemoryJudgeValidationReport(
		pool, profile, config, FixedMemoryJudgeAuthority(), cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	if failedReport.Passed || failedReport.Outcome.Severity != ProductionValidationSeverityYellow ||
		failedReport.Outcome.RequiredAction != ProductionValidationActionRetainBeta ||
		len(failedReport.Outcome.Reasons) == 0 ||
		failedReport.Outcome.Reasons[0] != productionValidationReasonProvider ||
		failedReport.Diagnostics.FailedCaseCount != 1 || len(failedReport.Evaluation.Failures) == 0 &&
		failedReport.Evaluation.Passed {
		t.Fatalf("terminal-failure production Validation report=%#v", failedReport)
	}
}

func TestProductionValidationFailureActionPrecedenceIsFrozen(t *testing.T) {
	base := memoryeval.AccuracyFirstCalibrationEvaluation{
		Passed: true,
		Safety: memoryeval.SafetyMetrics{Passed: true},
	}
	tests := []struct {
		name       string
		evidence   string
		evaluation memoryeval.AccuracyFirstCalibrationEvaluation
		failed     int
		severity   string
		action     string
	}{
		{
			name: "fake never quality evidence", evidence: ProductionValidationEvidenceFake,
			evaluation: func() memoryeval.AccuracyFirstCalibrationEvaluation {
				value := base
				value.Safety.Passed = false
				return value
			}(),
			severity: ProductionValidationSeverityYellow,
			action:   ProductionValidationActionRetainBeta,
		},
		{
			name: "privacy red", evidence: ProductionValidationEvidenceLive,
			evaluation: func() memoryeval.AccuracyFirstCalibrationEvaluation {
				value := base
				value.Safety.Passed = false
				return value
			}(),
			severity: ProductionValidationSeverityRed,
			action:   ProductionValidationActionDisableTool,
		},
		{
			name: "false injection orange", evidence: ProductionValidationEvidenceLive,
			evaluation: func() memoryeval.AccuracyFirstCalibrationEvaluation {
				value := base
				value.Metrics.FalseInjectionRate = 0.03
				return value
			}(),
			severity: ProductionValidationSeverityOrange,
			action:   ProductionValidationActionDisableRead,
		},
		{
			name: "provider yellow", evidence: ProductionValidationEvidenceLive,
			evaluation: base, failed: 1,
			severity: ProductionValidationSeverityYellow,
			action:   ProductionValidationActionRetainBeta,
		},
		{
			name: "pass owner review", evidence: ProductionValidationEvidenceLive,
			evaluation: base,
			severity:   ProductionValidationSeverityNone,
			action:     ProductionValidationActionOwnerReview,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := productionValidationOutcome(
				test.evidence,
				test.evaluation,
				test.failed,
				0.02,
			)
			if outcome.Severity != test.severity || outcome.RequiredAction != test.action {
				t.Fatalf("outcome=%#v", outcome)
			}
		})
	}
}

func TestProductionMemoryJudgeValidationCostAuthorityIsSchemaSeparated(t *testing.T) {
	cost := productionMemoryJudgeValidationTestCostBasis()
	if _, err := CostBasisSHA256(cost); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProductionMemoryJudgeValidationCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	for _, legacyValidator := range []func(CostBasis, ConfiguredCandidateJudgeProfileAuthority) error{
		ValidateFixedMemoryJudgeCostAuthority,
		ValidateAccuracyFirstMemoryJudgeCostAuthority,
		ValidateTransportStableMemoryJudgeCostAuthority,
	} {
		if err := legacyValidator(cost, FixedMemoryJudgeAuthority()); err == nil {
			t.Fatal("historical cost validator accepted schema-v10 authority")
		}
	}
	invalid := cost
	authority := *cost.ConfiguredCandidateJudgeAuthority
	invalid.ConfiguredCandidateJudgeAuthority = &authority
	invalid.ConfiguredCandidateJudgeAuthority.RequestCount = 301
	if err := ValidateProductionMemoryJudgeValidationCostAuthority(
		invalid,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("production Validation accepted request authority above 300")
	}
}

func productionValidationProtected(pool memoryauthor.RegressionPool) ProtectedRegression {
	return ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("production-validation-fixture"),
		CorpusRawSHA256:   sha256String("production-validation-corpus"),
		AuditRawSHA256:    sha256String("production-validation-audit"),
		ManifestRawSHA256: sha256String("production-validation-manifest"),
	}
}

func productionValidationProfile(
	t *testing.T,
	pool memoryauthor.RegressionPool,
	config ProfileConfig,
	cost CostBasis,
	providerMode string,
) CapturedProfile {
	t.Helper()
	traces := passingCloudJudgeValidationTraces(pool)
	logicalRequests, logicalInputTokens := productionValidationLogicalJudgeTelemetry(traces)
	configurationSHA256, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	profileID, err := candidateProfileID(providerMode)
	if err != nil {
		t.Fatal(err)
	}
	cases := make([]memoryeval.CaseObservation, len(traces))
	for index := range traces {
		cases[index] = traces[index].FullObservation
	}
	return CapturedProfile{
		Profile: memoryeval.Profile{
			ID:                   profileID,
			Role:                 "candidate",
			ReaderVersion:        ProductionMemoryJudgeValidationReaderVersion,
			ConfigurationSHA256:  configurationSHA256,
			CandidateLimit:       usermemory.MaxHybridShadowResults,
			FinalLimit:           usermemory.HybridShadowFinalLimit,
			ProviderEgressPolicy: memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		},
		Costs:       cost.Candidate,
		Cases:       cases,
		Calibration: traces,
		ProviderAttempts: productionValidationTelemetry(
			logicalRequests,
			logicalInputTokens,
			0,
			0,
			providerMode,
		),
	}
}

func passingCloudJudgeValidationTraces(
	pool memoryauthor.RegressionPool,
) []CandidateCalibrationTrace {
	traces := make([]CandidateCalibrationTrace, 0, 100)
	for _, item := range pool.Corpus.Cases {
		if item.Split != FrozenValidationSplit {
			continue
		}
		observation := memoryeval.CaseObservation{
			CaseID: item.ID, LatencyMilliseconds: 25, Fallback: "none",
			PersistedMemoryIDs: []string{},
		}
		if item.ExpectedNoMemory {
			observation.Fallback = "no_memory"
			for _, exclusion := range item.Exclusions {
				if exclusion.Reason != "irrelevant" {
					continue
				}
				observation.CandidateMemoryIDs = []string{exclusion.MemoryID}
				observation.ProviderSentMemoryIDs = []string{exclusion.MemoryID}
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
		}
		candidateReady := len(observation.CandidateMemoryIDs) > 0
		trace := CandidateCalibrationTrace{
			CaseID: item.ID, PreparedReady: true,
			AdmissionReady:  candidateReady,
			RerankReady:     candidateReady,
			CloudJudgeReady: candidateReady,
			AbstentionCode:  "NONE",
			ResultCode:      "OK",
			FullObservation: observation,
			FinalRelevanceScores: func() []float64 {
				result := make([]float64, len(observation.FinalMemoryIDs))
				for index := range result {
					result[index] = 0.9
				}
				return result
			}(),
		}
		if candidateReady {
			trace.CloudJudgeInputTokenUpperBound = 1000
		} else {
			trace.AbstentionCode = "NO_CANDIDATES"
			trace.ResultCode = "NO_CANDIDATES"
		}
		traces = append(traces, trace)
	}
	return traces
}

func productionValidationLogicalJudgeTelemetry(
	traces []CandidateCalibrationTrace,
) (int, int) {
	logicalRequests := 0
	logicalInputTokens := 0
	for _, trace := range traces {
		if trace.CloudJudgeInputTokenUpperBound > 0 {
			logicalRequests++
			logicalInputTokens += trace.CloudJudgeInputTokenUpperBound
		}
	}
	return logicalRequests, logicalInputTokens
}

func productionValidationTelemetry(
	logicalJudgeRequests int,
	logicalInputTokens int,
	judgeRetries int,
	judgeRetryInputTokens int,
	providerMode string,
) AccuracyFirstProviderTelemetry {
	elapsed := int64(0)
	if providerMode == ProviderModeLiveSiliconFlow {
		elapsed = 99
	}
	judgeAttempts := logicalJudgeRequests + judgeRetries
	return AccuracyFirstProviderTelemetry{
		PassageEmbeddingAttempts:          1,
		QueryEmbeddingAttempts:            100,
		RerankAttempts:                    logicalJudgeRequests,
		JudgeAttempts:                     judgeAttempts,
		JudgeRetries:                      judgeRetries,
		JudgeInputTokenUpperBound:         logicalInputTokens + judgeRetryInputTokens,
		JudgeRetryInputTokenUpperBound:    judgeRetryInputTokens,
		JudgeAttemptFailureCategoryCounts: map[string]int{},
		InterCaseCooldownCount:            99,
		InterCaseCooldownMilliseconds:     99_000,
		InterCaseCooldownElapsedMillis:    elapsed,
		PassageEmbeddingLatency:           testAccuracyFirstLatency(1),
		QueryEmbeddingLatency:             testAccuracyFirstLatency(100),
		RerankLatency:                     testAccuracyFirstLatency(logicalJudgeRequests),
		JudgeLatency:                      testAccuracyFirstLatency(judgeAttempts),
	}
}

func productionMemoryJudgeValidationTestCostBasis() CostBasis {
	authority := FixedMemoryJudgeAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v10",
		ProviderCostPolicy: ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 50,
			ChatProviderCostMicrounits: 100,
		},
		Source: "test", EffectiveAt: "2026-08-05T00:00:00Z",
		ConfiguredCandidateJudgeAuthority: &ConfiguredCandidateJudgeCostAuthority{
			ProviderID: authority.ProviderID, ProviderType: authority.ProviderType,
			BaseURLSHA256: authority.BaseURLSHA256, ModelID: authority.ModelID,
			RequestCount: 300, MaximumInputTokens: 300_000,
			MaximumOutputTokens:              300 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            2,
		},
	}
}

func TestProductionMemoryJudgeValidationHashesAreStableHex(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	values := []func() (string, error){
		func() (string, error) { return validationCaseOrderSHA256(pool) },
		func() (string, error) {
			return productionValidationCriteriaSHA256(pool.Corpus.Criteria)
		},
		productionRelevancePolicySHA256,
	}
	for _, build := range values {
		first, firstErr := build()
		second, secondErr := build()
		if firstErr != nil || secondErr != nil || first != second ||
			len(first) != 64 || strings.Trim(first, "0123456789abcdef") != "" {
			t.Fatalf("stable hash=%q/%q errors=%v/%v", first, second, firstErr, secondErr)
		}
	}
	if judgeFailureTaxonomySHA256() != memoryjudge.FailureTaxonomySHA256 {
		t.Fatal("Judge failure taxonomy drifted")
	}
}
