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

func TestBuildConfiguredCandidateJudgeDevelopmentProfileBindsExactProvider(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	authority := configuredCandidateJudgeTestAuthority()
	config, err := BuildConfiguredCandidateJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		authority,
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v10" ||
		config.ReaderVersion != ConfiguredCandidateJudgeReaderVersion ||
		config.CaptureMode != CaptureModeConfiguredCandidateJudge ||
		config.EvaluationSplit != DevelopmentCalibrationSplit ||
		config.ConfiguredCandidateJudgeProviderID != authority.ProviderID ||
		config.ConfiguredCandidateJudgeProviderType != authority.ProviderType ||
		config.ConfiguredCandidateJudgeBaseURLSHA256 != authority.BaseURLSHA256 ||
		config.ConfiguredCandidateJudgeAdapter != memoryjudge.ChatAdapterVersion ||
		config.CloudCandidateJudgeModelID != authority.ModelID ||
		!config.CloudCandidateJudgeRequired ||
		config.ProviderEgressPolicy !=
			memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1 ||
		config.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 {
		t.Fatalf("configured candidate-judge profile = %#v", config)
	}
	firstHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	config.ConfiguredCandidateJudgeAdapter = "drifted"
	secondHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("configured candidate-judge adapter drift did not change configuration hash")
	}

	invalidAuthority := authority
	invalidAuthority.ProviderID = " configured-gpt"
	if _, err := BuildConfiguredCandidateJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		invalidAuthority,
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	); err == nil {
		t.Fatal("padded configured candidate-judge authority was accepted")
	}
}

func TestBuildConfiguredCandidateJudgeDevelopmentReportAndManifest(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	authority := configuredCandidateJudgeTestAuthority()
	report, reportBody, err := BuildConfiguredCandidateJudgeDevelopmentReport(
		pool,
		configuredCandidateJudgeDevelopmentProfile(
			passingCloudJudgeDevelopmentTraces(pool),
		),
		authority,
		configuredCandidateJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.SchemaVersion != ConfiguredCandidateJudgeReportSchemaVersion ||
		report.AdmissionMode != ConfiguredCandidateJudgeAdmissionMode ||
		report.PromotionEligible || report.JudgeProviderID != authority.ProviderID ||
		report.JudgeProviderType != authority.ProviderType ||
		report.JudgeBaseURLSHA256 != authority.BaseURLSHA256 ||
		report.JudgeModelID != authority.ModelID ||
		report.JudgeAdapter != memoryjudge.ChatAdapterVersion ||
		report.PolicyID != usermemory.HybridRelevanceCloudJudgeCalibrationPolicyID {
		t.Fatalf("configured candidate-judge report = %#v", report)
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal(reportBody, &encoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"schemaVersion", "evaluation", "diagnostics", "costAuthority",
		"judgeProviderId", "judgeProviderType", "judgeBaseUrlSha256",
		"judgeModelId", "judgeAdapter",
	} {
		if _, ok := encoded[field]; !ok {
			t.Fatalf("flattened report field %q missing from %s", field, reportBody)
		}
	}
	if _, nested := encoded["CloudJudgeDevelopmentReport"]; nested {
		t.Fatal("embedded cloud report was serialized as a nested object")
	}
	if !validConfiguredCandidateJudgeDevelopmentReport(report) {
		t.Fatalf("configured candidate-judge report failed manifest validation: %#v", report)
	}

	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture"),
		CorpusRawSHA256:   sha256String("corpus"),
		AuditRawSHA256:    sha256String("audit"),
		ManifestRawSHA256: sha256String("manifest"),
	}
	startedAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	manifest, manifestBody, err := BuildConfiguredCandidateJudgeDevelopmentRunManifest(
		"run-configured-judge",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost"),
		report,
		[]Artifact{{Name: ConfiguredCandidateJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeConfiguredCandidateJudge ||
		manifest.AdmissionMode != ConfiguredCandidateJudgeAdmissionMode ||
		manifest.ProviderCostPolicy != ProviderCostPolicyOwnerAuthorizedAbsoluteV1 ||
		len(manifest.Artifacts) != 1 ||
		manifest.Artifacts[0].Name != ConfiguredCandidateJudgeArtifactName ||
		len(manifestBody) == 0 {
		t.Fatalf("configured candidate-judge manifest = %#v", manifest)
	}

	drifted := authority
	drifted.ModelID = "other-model"
	if _, _, err := BuildConfiguredCandidateJudgeDevelopmentReport(
		pool,
		configuredCandidateJudgeDevelopmentProfile(
			passingCloudJudgeDevelopmentTraces(pool),
		),
		drifted,
		configuredCandidateJudgeTestCostBasis(),
	); err == nil {
		t.Fatal("configured candidate-judge Provider drift was accepted")
	}

	driftedReport := report
	driftedReport.JudgeBaseURLSHA256 = strings.Repeat("z", 64)
	if _, _, err := BuildConfiguredCandidateJudgeDevelopmentRunManifest(
		"run-configured-judge-drift",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost"),
		driftedReport,
		[]Artifact{{Name: ConfiguredCandidateJudgeArtifactName, Body: reportBody}},
	); err == nil {
		t.Fatal("non-hex configured candidate-judge report authority was accepted")
	}
}

func TestConfiguredCandidateJudgeReportRetainsPreJudgeRetrievalFailure(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	trace := &traces[0]
	trace.AdmissionReady = false
	trace.RerankReady = false
	trace.CloudJudgeReady = false
	trace.CloudJudgeInputTokenUpperBound = 0
	trace.AbstentionCode = "RELEVANCE_ADMISSION_UNAVAILABLE"
	trace.FullObservation.ProviderSentMemoryIDs = []string{}
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	trace.FullObservation.Fallback = "no_memory"
	trace.FinalRelevanceScores = []float64{}

	profile := configuredCandidateJudgeDevelopmentProfile(traces)
	report, _, err := BuildConfiguredCandidateJudgeDevelopmentReport(
		pool,
		profile,
		configuredCandidateJudgeTestAuthority(),
		configuredCandidateJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Diagnostics.FailedCaseCount != 1 ||
		report.Diagnostics.FailureCodeCounts["RELEVANCE_ADMISSION_UNAVAILABLE"] != 1 ||
		report.CostAuthority.ActualRequestCount != 194 {
		t.Fatalf("pre-judge retrieval failure report = %#v", report)
	}

	legacy := profile
	legacy.Profile.ReaderVersion = CloudJudgeReaderVersion
	if _, _, err := BuildCloudJudgeDevelopmentReport(
		pool,
		legacy,
		configuredCandidateJudgeTestAuthority().ModelID,
		configuredCandidateJudgeLegacyCostBasis(configuredCandidateJudgeTestCostBasis()),
	); err == nil {
		t.Fatal("historical cloud-judge report accepted a pre-judge retrieval failure")
	}
}

func configuredCandidateJudgeDevelopmentProfile(
	traces []CandidateCalibrationTrace,
) CapturedProfile {
	profile := cloudJudgeDevelopmentProfile(traces)
	profile.Profile.ID = FakeCandidateProfileID
	profile.Profile.ReaderVersion = ConfiguredCandidateJudgeReaderVersion
	profile.Costs = configuredCandidateJudgeTestCostBasis().Candidate
	return profile
}

func configuredCandidateJudgeTestAuthority() ConfiguredCandidateJudgeProfileAuthority {
	return ConfiguredCandidateJudgeProfileAuthority{
		ProviderID:    "configured-gpt",
		ProviderType:  "openai_compatible",
		BaseURLSHA256: strings.Repeat("b", 64),
		ModelID:       hybridJudgeTestModelID,
	}
}

func configuredCandidateJudgeTestCostBasis() CostBasis {
	authority := configuredCandidateJudgeTestAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v6",
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
