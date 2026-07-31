package memorycapture

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestBuildFixedMemoryJudgeDevelopmentProfileBindsSchemaV11(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildFixedMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v11" ||
		config.ReaderVersion != FixedMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeFixedMemoryJudge ||
		config.HardCutoffMillis != 3000 ||
		config.EvaluationCriteriaVersion !=
			memoryeval.MemoryJudgeDevelopmentCriteriaVersionV2 ||
		config.MaximumP95LatencyMillis != 1500 ||
		config.MaximumP99LatencyMillis != 2500 ||
		config.RelevancePolicyID != usermemory.HybridRelevanceFixedMemoryJudgePolicyID ||
		config.ConfiguredCandidateJudgeProviderID != FixedMemoryJudgeProviderID ||
		config.ConfiguredCandidateJudgeProviderType != FixedMemoryJudgeProviderType ||
		config.ConfiguredCandidateJudgeBaseURLSHA256 != FixedMemoryJudgeBaseURLSHA256 ||
		config.ConfiguredCandidateJudgeAdapter != memoryjudge.ChatAdapterVersion ||
		config.CloudCandidateJudgeModelID != FixedMemoryJudgeModelID {
		t.Fatalf("fixed Memory Judge profile = %#v", config)
	}

	drifted := FixedMemoryJudgeAuthority()
	drifted.ModelID = "other"
	if _, err := BuildFixedMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		drifted,
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	); err == nil {
		t.Fatal("fixed Memory Judge authority drift was accepted")
	}
}

func TestBuildFixedMemoryJudgeDevelopmentReportUsesV2LatencyCriteria(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	for index := range traces {
		traces[index].FullObservation.LatencyMilliseconds = 1200
		traces[index].FullObservation.HardCutoffApplied = false
	}
	profile := cloudJudgeDevelopmentProfile(traces)
	profile.Profile.ID = FakeCandidateProfileID
	profile.Profile.ReaderVersion = FixedMemoryJudgeReaderVersion
	profile.Costs = fixedMemoryJudgeTestCostBasis().Candidate
	report, reportBody, err := BuildFixedMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		fixedMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.SchemaVersion != FixedMemoryJudgeReportSchemaVersion ||
		report.AdmissionMode != FixedMemoryJudgeAdmissionMode ||
		report.PolicyID != usermemory.HybridRelevanceFixedMemoryJudgePolicyID ||
		report.JudgeModelID != FixedMemoryJudgeModelID ||
		report.EvaluationCriteriaVersion !=
			memoryeval.MemoryJudgeDevelopmentCriteriaVersionV2 ||
		report.EvaluationCriteria.MaximumP95LatencyMilliseconds != 1500 ||
		report.EvaluationCriteria.MaximumP99LatencyMilliseconds != 2500 ||
		report.EvaluationCriteria.HardCutoffMilliseconds != 3000 ||
		report.Evaluation.Budgets.P95LatencyMilliseconds != 1200 {
		t.Fatalf("fixed Memory Judge report = %#v", report)
	}
	if !validFixedMemoryJudgeDevelopmentReport(report) {
		t.Fatal("fixed Memory Judge report failed strict validation")
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(reportBody, &encoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"schemaVersion", "evaluationCriteriaVersion", "evaluationCriteria",
		"judgeProviderId", "judgeProviderType", "judgeBaseUrlSha256",
	} {
		if _, ok := encoded[field]; !ok {
			t.Fatalf("fixed report field %q missing", field)
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
	manifest, _, err := BuildFixedMemoryJudgeDevelopmentRunManifest(
		"run-fixed-memory-judge",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost"),
		report,
		[]Artifact{{Name: FixedMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeFixedMemoryJudge ||
		manifest.AdmissionMode != FixedMemoryJudgeAdmissionMode ||
		manifest.PolicyID != usermemory.HybridRelevanceFixedMemoryJudgePolicyID {
		t.Fatalf("fixed Memory Judge manifest = %#v", manifest)
	}

	drifted := report
	drifted.EvaluationCriteria.MaximumP95LatencyMilliseconds = 900
	if validFixedMemoryJudgeDevelopmentReport(drifted) {
		t.Fatal("fixed Memory Judge report accepted criteria drift")
	}
}

func TestFixedMemoryJudgeCostBasisBindsExactLunaAuthority(t *testing.T) {
	cost := fixedMemoryJudgeTestCostBasis()
	if _, err := CostBasisSHA256(cost); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFixedMemoryJudgeCostAuthority(
		cost,
		FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}

	drifted := cost
	authority := *cost.ConfiguredCandidateJudgeAuthority
	drifted.ConfiguredCandidateJudgeAuthority = &authority
	drifted.ConfiguredCandidateJudgeAuthority.ProviderID = "other"
	if _, err := CostBasisSHA256(drifted); err == nil {
		t.Fatal("schema-v7 accepted non-Luna authority")
	}

	legacy := cost
	legacy.SchemaVersion = "neo-chat.memory-regression-cost-basis.v6"
	if err := ValidateFixedMemoryJudgeCostAuthority(
		legacy,
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v6 was accepted as fixed Memory Judge cost authority")
	}
}

func fixedMemoryJudgeTestCostBasis() CostBasis {
	authority := FixedMemoryJudgeAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v7",
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
			RequestCount:                     300,
			MaximumInputTokens:               300_000,
			MaximumOutputTokens:              300 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            2,
		},
	}
}
