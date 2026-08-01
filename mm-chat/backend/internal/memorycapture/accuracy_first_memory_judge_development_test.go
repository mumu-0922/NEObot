package memorycapture

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestAccuracyFirstProfileSeparatesRegressionV2V3AndV4(t *testing.T) {
	tests := []struct {
		name     string
		generate func() (memoryauthor.RegressionPool, error)
	}{
		{name: "v2", generate: memoryauthor.GenerateRegression},
		{name: "v3", generate: memoryauthor.GenerateRegressionV3},
		{name: "v4", generate: memoryauthor.GenerateRegressionV4},
	}
	hashes := make(map[string]string, len(tests))
	for _, test := range tests {
		pool, err := test.generate()
		if err != nil {
			t.Fatal(err)
		}
		root := filepath.Join(t.TempDir(), test.name+"-regression")
		if err := memoryauthor.PublishRegression(root, pool); err != nil {
			t.Fatal(err)
		}
		protected, err := LoadProtectedRegression(root)
		if err != nil {
			t.Fatal(err)
		}
		config, err := BuildAccuracyFirstMemoryJudgeDevelopmentProfileConfig(
			protected,
			strings.Repeat("5", 64),
			ProviderModeFakeProtocol,
			FixedMemoryJudgeAuthority(),
			ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		)
		if err != nil {
			t.Fatal(err)
		}
		hashes[test.name], err = ConfigurationSHA256(config)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range [][2]string{{"v2", "v3"}, {"v2", "v4"}, {"v3", "v4"}} {
		if hashes[pair[0]] == hashes[pair[1]] {
			t.Fatalf("%s and %s regression pools produced the same capture configuration hash", pair[0], pair[1])
		}
	}
}

func TestBuildAccuracyFirstMemoryJudgeProfileBindsSchemaV12Execution(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildAccuracyFirstMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	policy := config.AccuracyFirstExecutionPolicy
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v12" ||
		config.ReaderVersion != AccuracyFirstMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeAccuracyFirstMemoryJudge ||
		config.HardCutoffMillis != 0 || config.MaximumP95LatencyMillis != 0 ||
		config.MaximumP99LatencyMillis != 0 ||
		config.EvaluationCriteriaVersion !=
			memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3 ||
		config.RelevancePolicyID != usermemory.HybridRelevanceAccuracyFirstJudgePolicyID ||
		policy == nil || policy.GlobalProviderRequestConcurrency != 1 ||
		policy.ApplicationDeadlineMode != memoryeval.MemoryJudgeApplicationDeadlineNoneV1 ||
		policy.ProviderElapsedTimeoutMode != memoryeval.MemoryJudgeApplicationDeadlineNoneV1 ||
		policy.LatencyEvaluationMode != memoryeval.MemoryJudgeLatencyDiagnosticOnlyV1 ||
		policy.InterCaseCooldownMilliseconds != 1000 ||
		policy.InterCaseCooldownClock != AccuracyFirstCooldownVirtualProtocolV1 ||
		policy.MaximumRetriesPerProviderRequest != 1 ||
		policy.RetryFallbackDelayMilliseconds != 5000 {
		t.Fatalf("accuracy-first profile = %#v", config)
	}
	live, err := BuildAccuracyFirstMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeLiveSiliconFlow,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil || live.AccuracyFirstExecutionPolicy == nil ||
		live.AccuracyFirstExecutionPolicy.InterCaseCooldownClock !=
			AccuracyFirstCooldownWallClockV1 {
		t.Fatalf("live accuracy-first profile = %#v, %v", live, err)
	}
}

func TestAccuracyFirstMemoryJudgeReportUsesDiagnosticOnlyLatencyAndRetryCost(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	logicalJudgeRequests := 0
	logicalInputTokens := 0
	for index := range traces {
		traces[index].FullObservation.LatencyMilliseconds = 120_000
		traces[index].FullObservation.HardCutoffApplied = false
		if traces[index].CloudJudgeInputTokenUpperBound > 0 {
			logicalJudgeRequests++
			logicalInputTokens += traces[index].CloudJudgeInputTokenUpperBound
		}
	}
	profile := cloudJudgeDevelopmentProfile(traces)
	profile.Profile.ID = FakeCandidateProfileID
	profile.Profile.ReaderVersion = AccuracyFirstMemoryJudgeReaderVersion
	profile.Costs = accuracyFirstMemoryJudgeTestCostBasis().Candidate
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalJudgeRequests,
		logicalInputTokens,
		1,
		123,
	)
	report, reportBody, err := BuildAccuracyFirstMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		accuracyFirstMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.SchemaVersion != AccuracyFirstMemoryJudgeReportSchemaVersion ||
		report.AdmissionMode != AccuracyFirstMemoryJudgeAdmissionMode ||
		report.EvaluationCriteriaVersion !=
			memoryeval.MemoryJudgeAccuracyFirstCriteriaVersionV3 ||
		report.Evaluation.Budgets.P95LatencyMilliseconds != 120_000 ||
		report.CostAuthority.ActualRequestCount != logicalJudgeRequests+1 ||
		report.CostAuthority.ActualInputTokenUpperBound != uint64(logicalInputTokens+123) ||
		report.ProviderAttempts.JudgeInputTokenUpperBound != logicalInputTokens+123 ||
		report.CostAuthority.ActualOutputTokenUpperBound !=
			uint64(logicalJudgeRequests+1)*usermemory.HybridCandidateJudgeMaximumOutputTokens ||
		report.ProviderAttempts.InterCaseCooldownCount != 299 ||
		report.ExecutionPolicy.InterCaseCooldownClock != AccuracyFirstCooldownVirtualProtocolV1 {
		t.Fatalf("accuracy-first report = %#v", report)
	}
	for _, forbidden := range [][]byte{
		[]byte(`"latencyPassed"`),
		[]byte(`"hardCutoffPassed"`),
		[]byte(`"hardCutoffViolationCount"`),
	} {
		if bytes.Contains(reportBody, forbidden) {
			t.Fatalf("accuracy-first report retained legacy verdict %s", forbidden)
		}
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(reportBody, &encoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"executionPolicy", "providerAttempts", "evaluationCriteria", "costAuthority",
	} {
		if _, ok := encoded[field]; !ok {
			t.Fatalf("accuracy-first report field %q missing", field)
		}
	}

	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture"),
		CorpusRawSHA256:   sha256String("corpus"),
		AuditRawSHA256:    sha256String("audit"),
		ManifestRawSHA256: sha256String("manifest"),
	}
	startedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	manifest, _, err := BuildAccuracyFirstMemoryJudgeDevelopmentRunManifest(
		"run-accuracy-first-memory-judge",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost"),
		report,
		[]Artifact{{Name: AccuracyFirstMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeAccuracyFirstMemoryJudge ||
		manifest.AdmissionMode != AccuracyFirstMemoryJudgeAdmissionMode {
		t.Fatalf("accuracy-first manifest = %#v", manifest)
	}
	tampered := report
	tampered.ProviderAttempts.JudgeInputTokenUpperBound++
	if _, _, err := BuildAccuracyFirstMemoryJudgeDevelopmentRunManifest(
		"run-accuracy-first-memory-judge",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost"),
		tampered,
		[]Artifact{{Name: AccuracyFirstMemoryJudgeArtifactName, Body: reportBody}},
	); err == nil {
		t.Fatal("accuracy-first manifest accepted drifted input-token telemetry")
	}
}

func TestAccuracyFirstMemoryJudgeReportRejectsHardCutoffEvidence(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	logicalJudgeRequests := 0
	logicalInputTokens := 0
	for index := range traces {
		if traces[index].CloudJudgeInputTokenUpperBound > 0 {
			logicalJudgeRequests++
			logicalInputTokens += traces[index].CloudJudgeInputTokenUpperBound
		}
	}
	traces[0].FullObservation.HardCutoffApplied = true
	profile := cloudJudgeDevelopmentProfile(traces)
	profile.Profile.ID = FakeCandidateProfileID
	profile.Profile.ReaderVersion = AccuracyFirstMemoryJudgeReaderVersion
	profile.Costs = accuracyFirstMemoryJudgeTestCostBasis().Candidate
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalJudgeRequests,
		logicalInputTokens,
		0,
		0,
	)
	if _, _, err := BuildAccuracyFirstMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		accuracyFirstMemoryJudgeTestCostBasis(),
	); err == nil {
		t.Fatal("accuracy-first report accepted hard-cutoff evidence")
	}
}

func TestAccuracyFirstCostBasisAllowsOnlySchemaV8RetryAuthority(t *testing.T) {
	cost := accuracyFirstMemoryJudgeTestCostBasis()
	if _, err := CostBasisSHA256(cost); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAccuracyFirstMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	v7 := cost
	v7.SchemaVersion = "neo-chat.memory-regression-cost-basis.v7"
	if err := ValidateFixedMemoryJudgeCostAuthority(
		v7,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v7 accepted the schema-v8 600-request retry authority")
	}
	v8WithoutRetry := cost
	authority := *cost.ConfiguredCandidateJudgeAuthority
	v8WithoutRetry.ConfiguredCandidateJudgeAuthority = &authority
	v8WithoutRetry.ConfiguredCandidateJudgeAuthority.RequestCount = 300
	v8WithoutRetry.ConfiguredCandidateJudgeAuthority.MaximumOutputTokens =
		300 * usermemory.HybridCandidateJudgeMaximumOutputTokens
	if err := ValidateAccuracyFirstMemoryJudgeCostAuthority(
		v8WithoutRetry,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v8 accepted a 300-request authority")
	}
}

func accuracyFirstTelemetry(
	logicalJudgeRequests int,
	logicalInputTokens int,
	judgeRetries int,
	judgeRetryInputTokens int,
) AccuracyFirstProviderTelemetry {
	rerrankAttempts := logicalJudgeRequests
	judgeAttempts := logicalJudgeRequests + judgeRetries
	return AccuracyFirstProviderTelemetry{
		PassageEmbeddingAttempts:       1,
		QueryEmbeddingAttempts:         300,
		RerankAttempts:                 rerrankAttempts,
		JudgeAttempts:                  judgeAttempts,
		JudgeRetries:                   judgeRetries,
		JudgeInputTokenUpperBound:      logicalInputTokens + judgeRetryInputTokens,
		JudgeRetryInputTokenUpperBound: judgeRetryInputTokens,
		InterCaseCooldownCount:         299,
		InterCaseCooldownMilliseconds:  299_000,
		PassageEmbeddingLatency:        testAccuracyFirstLatency(1),
		QueryEmbeddingLatency:          testAccuracyFirstLatency(300),
		RerankLatency:                  testAccuracyFirstLatency(rerrankAttempts),
		JudgeLatency:                   testAccuracyFirstLatency(judgeAttempts),
	}
}

func testAccuracyFirstLatency(count int) AccuracyFirstLatencyDiagnostics {
	if count == 0 {
		return AccuracyFirstLatencyDiagnostics{}
	}
	return AccuracyFirstLatencyDiagnostics{
		SampleCount:                count,
		TotalMilliseconds:          int64(count),
		P95LatencyMilliseconds:     1,
		P99LatencyMilliseconds:     1,
		MaximumLatencyMilliseconds: 1,
	}
}

func accuracyFirstMemoryJudgeTestCostBasis() CostBasis {
	authority := FixedMemoryJudgeAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v8",
		ProviderCostPolicy: ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 50,
			ChatProviderCostMicrounits: 100,
		},
		Source: "test", EffectiveAt: "2026-07-31T00:00:00Z",
		ConfiguredCandidateJudgeAuthority: &ConfiguredCandidateJudgeCostAuthority{
			ProviderID:                       authority.ProviderID,
			ProviderType:                     authority.ProviderType,
			BaseURLSHA256:                    authority.BaseURLSHA256,
			ModelID:                          authority.ModelID,
			RequestCount:                     600,
			MaximumInputTokens:               600_000,
			MaximumOutputTokens:              600 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            2,
		},
	}
}
