package memorycapture

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestAbstentionConfirmationDevelopmentProfileAndCostAreSeparated(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildAbstentionConfirmationMemoryJudgeDevelopmentProfileConfig(
		protected, strings.Repeat("5", 64), ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(), ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := AbstentionConfirmationDevelopmentExecutionPolicy(
		ProviderModeFakeProtocol,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion !=
		"neo-chat.memory-regression-profile-config.v20-confirmation-development.v1" ||
		config.ReaderVersion != AbstentionConfirmationMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeAbstentionConfirmationMemoryJudge ||
		config.ConfiguredCandidateJudgeAdapter !=
			memoryjudge.BufferedChatAbstentionConfirmationAdapterVersion ||
		!config.CloudCandidateJudgeAbstentionConfirmationRequired ||
		config.CloudCandidateJudgeConfirmationPromptVersion !=
			usermemory.HybridCandidateJudgeConfirmationPromptVersion ||
		config.CloudCandidateJudgeConfirmationPromptSHA256 !=
			usermemory.HybridCandidateJudgeConfirmationPromptSHA256 ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != execution ||
		execution.MaximumAbstentionConfirmationsPerLogicalRequest != 1 {
		t.Fatalf("confirmation config=%#v execution=%#v", config, execution)
	}
	cost := abstentionConfirmationDevelopmentTestCostBasis()
	if err := ValidateAbstentionConfirmationDevelopmentCostAuthority(
		cost, FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	costBody, err := json.Marshal(cost)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := DecodeCostBasis(costBody); err != nil {
		t.Fatalf("decode confirmation cost basis: %v", err)
	}
	if err := ValidateAccuracyRepairMemoryJudgeCostAuthority(
		cost, FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v15 accepted confirmation cost authority")
	}
	for name, mutate := range map[string]func(*CostBasis){
		"request count": func(value *CostBasis) {
			value.ConfiguredCandidateJudgeAuthority.RequestCount--
		},
		"output ceiling": func(value *CostBasis) {
			value.ConfiguredCandidateJudgeAuthority.MaximumOutputTokens--
		},
		"judge cost": func(value *CostBasis) {
			value.ConfiguredCandidateJudgeAuthority.MaximumCostMicrounits--
		},
		"memory cost": func(value *CostBasis) {
			value.Candidate.MemoryProviderCostMicrounits--
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := abstentionConfirmationDevelopmentTestCostBasis()
			mutate(&candidate)
			if err := ValidateAbstentionConfirmationDevelopmentCostAuthority(
				candidate, FixedMemoryJudgeAuthority(),
			); err == nil {
				t.Fatal("drifted confirmation cost authority was accepted")
			}
		})
	}
}

func TestDoubleConfirmationDevelopmentExecutionPolicyChangesOnlyBoundedCount(t *testing.T) {
	single, err := AbstentionConfirmationDevelopmentExecutionPolicy(ProviderModeFakeProtocol)
	if err != nil {
		t.Fatal(err)
	}
	double, err := DoubleConfirmationDevelopmentExecutionPolicy(ProviderModeFakeProtocol)
	if err != nil {
		t.Fatal(err)
	}
	single.SequenceVersion = double.SequenceVersion
	single.MaximumAbstentionConfirmationsPerLogicalRequest = 2
	if single != double ||
		double.SequenceVersion != DoubleConfirmationDevelopmentExecutionSequenceV1 {
		t.Fatalf("double-confirmation execution drifted: %#v", double)
	}
}

func TestAbstentionConfirmationTelemetryRejectsRetryTokenAndLatencyDrift(t *testing.T) {
	valid := accuracyFirstTelemetry(10, 1000, 1, 100)
	valid.JudgeAttempts += 3
	valid.JudgeRetries++
	valid.JudgeInputTokenUpperBound += 300
	valid.JudgeRetryInputTokenUpperBound += 100
	valid.JudgeConfirmationAttempts = 3
	valid.JudgeConfirmationRetries = 1
	valid.JudgeConfirmationInputTokenUpperBound = 300
	valid.JudgeConfirmationRetryInputTokenUpperBound = 100
	valid.JudgeLatency = testAccuracyFirstLatency(valid.JudgeAttempts)
	if err := validateAbstentionConfirmationProviderTelemetry(valid, 300, 10, 2, 1); err != nil {
		t.Fatalf("valid confirmation telemetry rejected: %v", err)
	}

	mutations := map[string]func(*AccuracyFirstProviderTelemetry){
		"confirmation attempts exceed total": func(value *AccuracyFirstProviderTelemetry) {
			value.JudgeConfirmationAttempts = value.JudgeAttempts + 1
		},
		"confirmation retries exceed total": func(value *AccuracyFirstProviderTelemetry) {
			value.JudgeConfirmationRetries = value.JudgeRetries + 1
		},
		"confirmation tokens exceed total": func(value *AccuracyFirstProviderTelemetry) {
			value.JudgeConfirmationInputTokenUpperBound = value.JudgeInputTokenUpperBound + 1
		},
		"confirmation retry tokens exceed total": func(value *AccuracyFirstProviderTelemetry) {
			value.JudgeConfirmationRetryInputTokenUpperBound =
				value.JudgeRetryInputTokenUpperBound + 1
		},
		"missing primary retry tokens": func(value *AccuracyFirstProviderTelemetry) {
			value.JudgeRetryInputTokenUpperBound =
				value.JudgeConfirmationRetryInputTokenUpperBound
		},
		"query latency count": func(value *AccuracyFirstProviderTelemetry) {
			value.QueryEmbeddingLatency.SampleCount--
		},
		"cooldown duration": func(value *AccuracyFirstProviderTelemetry) {
			value.InterCaseCooldownMilliseconds--
		},
		"aggregate judge latency count": func(value *AccuracyFirstProviderTelemetry) {
			value.JudgeLatency.SampleCount--
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := validateAbstentionConfirmationProviderTelemetry(
				candidate, 300, 10, 2, 1,
			); err == nil {
				t.Fatal("drifted confirmation telemetry was accepted")
			}
		})
	}
}

func TestAbstentionConfirmationTelemetryBindsLogicalConfirmationMaximum(t *testing.T) {
	valid := accuracyFirstTelemetry(1, 100, 0, 0)
	valid.JudgeAttempts += 2
	valid.JudgeInputTokenUpperBound += 200
	valid.JudgeConfirmationAttempts = 2
	valid.JudgeConfirmationInputTokenUpperBound = 200
	valid.JudgeLatency = testAccuracyFirstLatency(valid.JudgeAttempts)
	if err := validateAbstentionConfirmationProviderTelemetry(
		valid, 300, 1, 2, 2,
	); err != nil {
		t.Fatalf("two bounded confirmations rejected: %v", err)
	}
	if err := validateAbstentionConfirmationProviderTelemetry(
		valid, 300, 1, 2, 1,
	); err == nil {
		t.Fatal("single-confirmation authority accepted two logical confirmations")
	}
}

func TestAbstentionConfirmationDevelopmentReportReconcilesOneConfirmation(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	setNegativePolicyGuardTrace(t, pool, traces)
	confirmationInputTokens := 0
	for index := range traces {
		if traces[index].CloudJudgeInputTokenUpperBound > 0 {
			confirmationInputTokens = traces[index].CloudJudgeInputTokenUpperBound
			traces[index].CloudJudgeInputTokenUpperBound += confirmationInputTokens
			break
		}
	}
	if confirmationInputTokens == 0 {
		t.Fatal("fixture has no Judge-completed case")
	}
	profile, logicalRequests, logicalInputTokens := negativePolicyGuardProfile(traces)
	profile.Profile.ReaderVersion = AbstentionConfirmationMemoryJudgeReaderVersion
	profile.Costs = abstentionConfirmationDevelopmentTestCostBasis().Candidate
	profile.ProviderAttempts = accuracyFirstTelemetry(logicalRequests, logicalInputTokens, 0, 0)
	profile.ProviderAttempts.JudgeAttempts++
	profile.ProviderAttempts.JudgeConfirmationAttempts = 1
	profile.ProviderAttempts.JudgeConfirmationInputTokenUpperBound =
		confirmationInputTokens
	profile.ProviderAttempts.JudgeLatency =
		testAccuracyFirstLatency(profile.ProviderAttempts.JudgeAttempts)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, reportBody, err := BuildAbstentionConfirmationMemoryJudgeDevelopmentReport(
		pool, profile, FixedMemoryJudgeAuthority(),
		abstentionConfirmationDevelopmentTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed ||
		report.SchemaVersion != AbstentionConfirmationMemoryJudgeReportSchemaVersion ||
		report.JudgeAdapter != memoryjudge.BufferedChatAbstentionConfirmationAdapterVersion ||
		report.JudgePromptVersion != usermemory.HybridCandidateJudgeAccuracyPromptVersion ||
		report.JudgeConfirmationPromptVersion !=
			usermemory.HybridCandidateJudgeConfirmationPromptVersion ||
		report.ProviderAttempts.JudgeConfirmationAttempts != 1 ||
		report.PolicyID !=
			usermemory.HybridRelevanceAbstentionConfirmationDevelopmentPolicyID {
		t.Fatalf("confirmation report=%#v", report)
	}
	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture-v5"),
		CorpusRawSHA256:   sha256String("corpus-v5"),
		AuditRawSHA256:    sha256String("audit-v5"),
		ManifestRawSHA256: sha256String("manifest-v5"),
	}
	startedAt := time.Date(2026, 8, 7, 16, 0, 0, 0, time.UTC)
	manifest, _, err := BuildAbstentionConfirmationMemoryJudgeRunManifest(
		"run-abstention-confirmation", "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
		ProviderModeFakeProtocol, startedAt, startedAt.Add(time.Minute), protected,
		sha256String("confirmation-cost"), report,
		[]Artifact{{Name: AbstentionConfirmationMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeAbstentionConfirmationMemoryJudge ||
		manifest.AdmissionMode != AbstentionConfirmationMemoryJudgeAdmissionMode {
		t.Fatalf("confirmation manifest=%#v", manifest)
	}
}

func abstentionConfirmationDevelopmentTestCostBasis() CostBasis {
	cost := accuracyRepairMemoryJudgeTestCostBasis()
	cost.SchemaVersion =
		"neo-chat.memory-regression-cost-basis.v20-confirmation-development.v1"
	authority := *cost.ConfiguredCandidateJudgeAuthority
	cost.ConfiguredCandidateJudgeAuthority = &authority
	authority.RequestCount = 1800
	authority.MaximumInputTokens = 2_000_000
	authority.MaximumOutputTokens =
		1800 * usermemory.HybridCandidateJudgeMaximumOutputTokens
	inputCost, _ := tokenCostCeiling(
		authority.MaximumInputTokens,
		authority.InputMicrounitsPerMillionTokens,
	)
	outputCost, _ := tokenCostCeiling(
		authority.MaximumOutputTokens,
		authority.OutputMicrounitsPerMillionTokens,
	)
	authority.MaximumCostMicrounits = inputCost + outputCost
	cost.Candidate.MemoryProviderCostMicrounits = authority.MaximumCostMicrounits
	return cost
}
