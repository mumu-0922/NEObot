package memorycapture

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const negativePolicyGuardDescriptorSHA256 = "82341542e46b091521b9f4b8c4eb637d6e732683d9902e0d2e3832a14cb50f9b"

func TestNegativePolicyGuardProfileAndCostAreSchemaSeparated(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildNegativePolicyGuardMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	executionPolicy, err := TransportStableDevelopmentExecutionPolicy(ProviderModeFakeProtocol)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v16" ||
		config.ReaderVersion != NegativePolicyGuardMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeNegativePolicyGuardMemoryJudge ||
		config.EvaluationSplit != DevelopmentCalibrationSplit ||
		config.RelevancePolicyID != usermemory.HybridRelevanceNegativePolicyGuardDevelopmentPolicyID ||
		!config.NegativePolicyQueryGuardRequired ||
		config.NegativePolicyQueryGuardVersion != usermemory.NegativePolicyQueryGuardVersion ||
		config.NegativePolicyQueryGuardSHA256 != usermemory.NegativePolicyQueryGuardSHA256 ||
		config.RelevancePolicyDescriptorSHA256 != negativePolicyGuardDescriptorSHA256 ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != executionPolicy ||
		config.CandidateJudgeFailureTaxonomySHA256 != memoryjudge.FailureTaxonomySHA256 {
		t.Fatalf("negative-guard config=%#v", config)
	}
	cost := negativePolicyGuardMemoryJudgeTestCostBasis()
	if err := ValidateNegativePolicyGuardMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := CostBasisSHA256(cost); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransportStableMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v14 accepted schema-v16 cost authority")
	}
}

func TestNegativePolicyGuardReportAccountsExactAbstentionWithoutFailure(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	guarded := setNegativePolicyGuardTrace(t, pool, traces)
	profile, logicalRequests, logicalInputTokens := negativePolicyGuardProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalRequests,
		logicalInputTokens,
		0,
		0,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, reportBody, err := BuildNegativePolicyGuardMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		negativePolicyGuardMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || !report.Evaluation.Passed || report.PromotionEligible ||
		report.PolicySelected || !report.DiagnosticComplete ||
		report.SchemaVersion != NegativePolicyGuardMemoryJudgeReportSchemaVersion ||
		report.AdmissionMode != NegativePolicyGuardMemoryJudgeAdmissionMode ||
		report.PolicyID != usermemory.HybridRelevanceNegativePolicyGuardDevelopmentPolicyID ||
		report.Diagnostics.NegativePolicyQueryAbstainedCaseCount != 1 ||
		report.Diagnostics.FailedCaseCount != 0 ||
		report.CostAuthority.ActualRequestCount != logicalRequests ||
		report.NegativePolicyQueryGuardVersion != usermemory.NegativePolicyQueryGuardVersion ||
		report.NegativePolicyQueryGuardSHA256 != usermemory.NegativePolicyQueryGuardSHA256 ||
		report.RelevancePolicyDescriptorSHA256 != negativePolicyGuardDescriptorSHA256 {
		t.Fatalf("negative-guard report=%#v", report)
	}
	for _, forbidden := range [][]byte{
		[]byte(guarded.CaseID), []byte(`"caseId"`), []byte(`"query"`),
		[]byte(`"response"`), []byte(`"selectedOrdinals"`),
	} {
		if bytes.Contains(bytes.ToLower(reportBody), bytes.ToLower(forbidden)) {
			t.Fatalf("report retained forbidden surface %q", forbidden)
		}
	}
	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture-v5"),
		CorpusRawSHA256:   sha256String("corpus-v5"),
		AuditRawSHA256:    sha256String("audit-v5"),
		ManifestRawSHA256: sha256String("manifest-v5"),
	}
	startedAt := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	manifest, _, err := BuildNegativePolicyGuardMemoryJudgeRunManifest(
		"run-negative-guard",
		"cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost-v11"),
		report,
		[]Artifact{{Name: NegativePolicyGuardMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeNegativePolicyGuardMemoryJudge ||
		manifest.NegativePolicyQueryGuardVersion != usermemory.NegativePolicyQueryGuardVersion ||
		manifest.NegativePolicyQueryGuardSHA256 != usermemory.NegativePolicyQueryGuardSHA256 ||
		manifest.RelevancePolicyDescriptorSHA256 != negativePolicyGuardDescriptorSHA256 {
		t.Fatalf("negative-guard manifest=%#v", manifest)
	}
}

func TestNegativePolicyGuardReportRejectsEgressAndFailsOtherPreAdmissionCodes(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	setNegativePolicyGuardTrace(t, pool, traces)
	guarded := setNegativePolicyGuardTrace(t, pool, traces)
	guarded.AbstentionCode = "RELEVANCE_ADMISSION_FAILED"
	profile, logicalRequests, logicalInputTokens := negativePolicyGuardProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(logicalRequests, logicalInputTokens, 0, 0)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, _, err := BuildNegativePolicyGuardMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		negativePolicyGuardMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.Diagnostics.FailedCaseCount != 1 ||
		report.Diagnostics.FailureCodeCounts["RELEVANCE_ADMISSION_FAILED"] != 1 ||
		report.Diagnostics.NegativePolicyQueryAbstainedCaseCount != 1 {
		t.Fatalf("other pre-admission result was not failed closed: %#v", report.Diagnostics)
	}

	traces = passingCloudJudgeDevelopmentTraces(pool)
	guarded = setNegativePolicyGuardTrace(t, pool, traces)
	guarded.FullObservation.ProviderSentMemoryIDs = []string{"opaque-egress"}
	profile, logicalRequests, logicalInputTokens = negativePolicyGuardProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(logicalRequests, logicalInputTokens, 0, 0)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	if _, _, err := BuildNegativePolicyGuardMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		negativePolicyGuardMemoryJudgeTestCostBasis(),
	); err == nil {
		t.Fatal("negative guard accepted candidate plaintext Provider egress")
	}

	traces = passingCloudJudgeDevelopmentTraces(pool)
	guarded = setNegativePolicyGuardTrace(t, pool, traces)
	guarded.ResultCode = "OK"
	profile, logicalRequests, logicalInputTokens = negativePolicyGuardProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(logicalRequests, logicalInputTokens, 0, 0)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	if _, _, err := BuildNegativePolicyGuardMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		negativePolicyGuardMemoryJudgeTestCostBasis(),
	); err == nil {
		t.Fatal("negative guard accepted a non-NO_CANDIDATES result")
	}

	traces = passingCloudJudgeDevelopmentTraces(pool)
	profile, logicalRequests, logicalInputTokens = negativePolicyGuardProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(logicalRequests, logicalInputTokens, 0, 0)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	if _, _, err := BuildNegativePolicyGuardMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		negativePolicyGuardMemoryJudgeTestCostBasis(),
	); err == nil {
		t.Fatal("negative-guard report accepted zero guard abstentions")
	}
}

func setNegativePolicyGuardTrace(
	t *testing.T,
	pool memoryauthor.RegressionPool,
	traces []CandidateCalibrationTrace,
) *CandidateCalibrationTrace {
	t.Helper()
	for index := range traces {
		trace := &traces[index]
		if len(trace.FullObservation.CandidateMemoryIDs) == 0 ||
			trace.AbstentionCode == "NEGATIVE_POLICY_QUERY_ABSTAINED" {
			continue
		}
		var expectedNoMemory bool
		for _, item := range pool.Corpus.Cases {
			if item.ID == trace.CaseID {
				expectedNoMemory = item.ExpectedNoMemory
				break
			}
		}
		if !expectedNoMemory {
			continue
		}
		trace.AdmissionReady = false
		trace.RerankReady = false
		trace.CloudJudgeReady = false
		trace.CloudJudgeInputTokenUpperBound = 0
		trace.CloudJudgeFailureCategory = ""
		trace.AbstentionCode = "NEGATIVE_POLICY_QUERY_ABSTAINED"
		trace.ResultCode = "NO_CANDIDATES"
		trace.FullObservation.ProviderSentMemoryIDs = []string{}
		trace.FullObservation.FinalMemoryIDs = []string{}
		trace.FullObservation.InjectedMemoryIDs = []string{}
		trace.FullObservation.PromptMemoryTokens = 0
		trace.FullObservation.HardCutoffApplied = false
		trace.FullObservation.Fallback = "no_memory"
		trace.FinalRelevanceScores = []float64{}
		return trace
	}
	t.Fatal("candidate-bearing expected-no-memory Development trace missing")
	return nil
}

func negativePolicyGuardProfile(
	traces []CandidateCalibrationTrace,
) (CapturedProfile, int, int) {
	profile, logicalRequests, logicalInputTokens := judgeFailureDiagnosticProfile(traces)
	profile.Profile.ReaderVersion = NegativePolicyGuardMemoryJudgeReaderVersion
	profile.Costs = negativePolicyGuardMemoryJudgeTestCostBasis().Candidate
	return profile, logicalRequests, logicalInputTokens
}

func negativePolicyGuardMemoryJudgeTestCostBasis() CostBasis {
	authority := FixedMemoryJudgeAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v11",
		ProviderCostPolicy: ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 50,
			ChatProviderCostMicrounits: 100,
		},
		Source: "test", EffectiveAt: "2026-08-06T00:00:00Z",
		ConfiguredCandidateJudgeAuthority: &ConfiguredCandidateJudgeCostAuthority{
			ProviderID:                       authority.ProviderID,
			ProviderType:                     authority.ProviderType,
			BaseURLSHA256:                    authority.BaseURLSHA256,
			ModelID:                          authority.ModelID,
			RequestCount:                     900,
			MaximumInputTokens:               1_500_000,
			MaximumOutputTokens:              900 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            3,
		},
	}
}
