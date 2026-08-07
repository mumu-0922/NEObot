package memorycapture

import (
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const accuracyRepairPolicyDescriptorSHA256 = "bf60e88192731881dac91df11a089f73b413fae14e03a96794a80c09004b53d6"

func TestAccuracyRepairMemoryJudgeProfileAndCostAreSchemaSeparated(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildAccuracyRepairMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	executionPolicy, err := AccuracyRepairMemoryJudgeDevelopmentExecutionPolicy(
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v20" ||
		config.ReaderVersion != AccuracyRepairMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeAccuracyRepairMemoryJudge ||
		config.ConfiguredCandidateJudgeAdapter != memoryjudge.BufferedChatAccuracyAdapterVersion ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != executionPolicy ||
		config.RelevancePolicyDescriptorSHA256 != accuracyRepairPolicyDescriptorSHA256 {
		t.Fatalf("accuracy-repair config=%#v", config)
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
		t.Fatalf("accuracy-repair policy=%#v stable=%#v", executionPolicy, stablePolicy)
	}

	cost := accuracyRepairMemoryJudgeTestCostBasis()
	if err := ValidateAccuracyRepairMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := CostBasisSHA256(cost); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBufferedMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v12 accepted schema-v15 cost authority")
	}
	oldCost := bufferedMemoryJudgeTestCostBasis()
	if err := ValidateAccuracyRepairMemoryJudgeCostAuthority(
		oldCost,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v15 accepted schema-v12 cost authority")
	}
}

func TestAccuracyRepairMemoryJudgeReportAndManifestBindPromptIdentity(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	setNegativePolicyGuardTrace(t, pool, traces)
	profile, logicalRequests, logicalInputTokens := negativePolicyGuardProfile(traces)
	profile.Profile.ReaderVersion = AccuracyRepairMemoryJudgeReaderVersion
	profile.Costs = accuracyRepairMemoryJudgeTestCostBasis().Candidate
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalRequests,
		logicalInputTokens,
		0,
		0,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, reportBody, err := BuildAccuracyRepairMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		accuracyRepairMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.SchemaVersion != AccuracyRepairMemoryJudgeReportSchemaVersion ||
		report.AdmissionMode != AccuracyRepairMemoryJudgeAdmissionMode ||
		report.JudgeAdapter != memoryjudge.BufferedChatAccuracyAdapterVersion ||
		report.ExecutionPolicy.SequenceVersion != AccuracyRepairMemoryJudgeExecutionSequenceV1 ||
		report.Diagnostics.NegativePolicyQueryAbstainedCaseCount != 1 ||
		report.JudgePromptVersion != usermemory.HybridCandidateJudgeAccuracyPromptVersion ||
		report.JudgePromptSHA256 != usermemory.HybridCandidateJudgeAccuracyPromptSHA256 ||
		report.PolicyID != usermemory.HybridRelevanceAccuracyRepairDevelopmentPolicyID {
		t.Fatalf("accuracy-repair report=%#v", report)
	}

	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture-v5"),
		CorpusRawSHA256:   sha256String("corpus-v5"),
		AuditRawSHA256:    sha256String("audit-v5"),
		ManifestRawSHA256: sha256String("manifest-v5"),
	}
	startedAt := time.Date(2026, 8, 6, 13, 0, 0, 0, time.UTC)
	manifest, _, err := BuildAccuracyRepairMemoryJudgeRunManifest(
		"run-accuracy-repair",
		"dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost-v15"),
		report,
		[]Artifact{{Name: AccuracyRepairMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeAccuracyRepairMemoryJudge ||
		manifest.AdmissionMode != AccuracyRepairMemoryJudgeAdmissionMode ||
		manifest.NegativePolicyQueryGuardSHA256 == "" {
		t.Fatalf("accuracy-repair manifest=%#v", manifest)
	}
}

func accuracyRepairMemoryJudgeTestCostBasis() CostBasis {
	cost := negativePolicyGuardMemoryJudgeTestCostBasis()
	cost.SchemaVersion = "neo-chat.memory-regression-cost-basis.v15"
	return cost
}
