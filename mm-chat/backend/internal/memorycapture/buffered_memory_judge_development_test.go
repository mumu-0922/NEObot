package memorycapture

import (
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
)

func TestBufferedMemoryJudgeProfileAndCostAreSchemaSeparated(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildBufferedMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	executionPolicy, err := BufferedMemoryJudgeDevelopmentExecutionPolicy(
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v17" ||
		config.ReaderVersion != BufferedMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeBufferedMemoryJudge ||
		config.ConfiguredCandidateJudgeAdapter != memoryjudge.BufferedChatAdapterVersion ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != executionPolicy ||
		config.RelevancePolicyDescriptorSHA256 != negativePolicyGuardDescriptorSHA256 {
		t.Fatalf("buffered config=%#v", config)
	}
	stablePolicy, err := TransportStableDevelopmentExecutionPolicy(ProviderModeFakeProtocol)
	if err != nil {
		t.Fatal(err)
	}
	if executionPolicy.SequenceVersion == stablePolicy.SequenceVersion ||
		executionPolicy.RetryPolicyVersion != stablePolicy.RetryPolicyVersion ||
		executionPolicy.MaximumJudgeRetriesPerRequest !=
			stablePolicy.MaximumJudgeRetriesPerRequest ||
		executionPolicy.SecondJudgeRetryDelayMilliseconds !=
			stablePolicy.SecondJudgeRetryDelayMilliseconds {
		t.Fatalf("buffered policy=%#v stable=%#v", executionPolicy, stablePolicy)
	}

	cost := bufferedMemoryJudgeTestCostBasis()
	if err := ValidateBufferedMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := CostBasisSHA256(cost); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNegativePolicyGuardMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v16 accepted schema-v17 cost authority")
	}
	oldCost := negativePolicyGuardMemoryJudgeTestCostBasis()
	if err := ValidateBufferedMemoryJudgeCostAuthority(
		oldCost,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v17 accepted schema-v16 cost authority")
	}
}

func TestBufferedMemoryJudgeReportAndManifestBindTransportIdentity(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	setNegativePolicyGuardTrace(t, pool, traces)
	profile, logicalRequests, logicalInputTokens := negativePolicyGuardProfile(traces)
	profile.Profile.ReaderVersion = BufferedMemoryJudgeReaderVersion
	profile.Costs = bufferedMemoryJudgeTestCostBasis().Candidate
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalRequests,
		logicalInputTokens,
		0,
		0,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, reportBody, err := BuildBufferedMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		bufferedMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.SchemaVersion != BufferedMemoryJudgeReportSchemaVersion ||
		report.AdmissionMode != BufferedMemoryJudgeAdmissionMode ||
		report.JudgeAdapter != memoryjudge.BufferedChatAdapterVersion ||
		report.ExecutionPolicy.SequenceVersion != BufferedMemoryJudgeExecutionSequenceV1 ||
		report.Diagnostics.NegativePolicyQueryAbstainedCaseCount != 1 {
		t.Fatalf("buffered report=%#v", report)
	}

	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture-v5"),
		CorpusRawSHA256:   sha256String("corpus-v5"),
		AuditRawSHA256:    sha256String("audit-v5"),
		ManifestRawSHA256: sha256String("manifest-v5"),
	}
	startedAt := time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC)
	manifest, _, err := BuildBufferedMemoryJudgeRunManifest(
		"run-buffered-judge",
		"dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost-v12"),
		report,
		[]Artifact{{Name: BufferedMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeBufferedMemoryJudge ||
		manifest.AdmissionMode != BufferedMemoryJudgeAdmissionMode ||
		manifest.NegativePolicyQueryGuardSHA256 == "" {
		t.Fatalf("buffered manifest=%#v", manifest)
	}
}

func bufferedMemoryJudgeTestCostBasis() CostBasis {
	cost := negativePolicyGuardMemoryJudgeTestCostBasis()
	cost.SchemaVersion = "neo-chat.memory-regression-cost-basis.v12"
	return cost
}
