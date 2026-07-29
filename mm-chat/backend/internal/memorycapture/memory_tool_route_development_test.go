package memorycapture

import (
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestBuildMemoryToolRouteDevelopmentReportPassesExactRoutePolicy(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingMemoryToolRouteDevelopmentTraces(pool)
	profile := memoryToolRouteDevelopmentProfile(traces)
	report, body, err := BuildMemoryToolRouteDevelopmentReport(
		pool,
		profile,
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || !report.Evaluation.Passed ||
		report.SchemaVersion != MemoryToolRouteDevelopmentReportSchemaVersion ||
		report.PolicyID != usermemory.HybridRelevanceMemoryToolRoutePolicyID ||
		report.ToolName != usermemory.HybridMemoryToolName ||
		report.ToolDecodingProfile != usermemory.HybridMemoryToolDecodingProfile ||
		report.ToolMaximumOutputTokens != usermemory.HybridMemoryToolMaximumOutputTokens ||
		report.ToolTemperature != usermemory.HybridMemoryToolTemperature ||
		report.ToolDisableThinking != usermemory.HybridMemoryToolDisableThinking ||
		report.Diagnostics.RouteCompletedCaseCount != 300 ||
		report.Diagnostics.FailedCaseCount != 0 ||
		report.CostAuthority.ActualRequestCount != 300 ||
		strings.Contains(string(body), "query") ||
		strings.Contains(string(body), "memoryContent") {
		t.Fatalf("report=%#v body=%s", report, body)
	}
}

func TestBuildMemoryToolRouteDevelopmentRunManifestBindsPolicy(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	report, reportBody, err := BuildMemoryToolRouteDevelopmentReport(
		pool,
		memoryToolRouteDevelopmentProfile(passingMemoryToolRouteDevelopmentTraces(pool)),
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	started := time.Date(2026, 7, 29, 16, 0, 0, 0, time.UTC)
	manifest, body, err := BuildMemoryToolRouteDevelopmentRunManifest(
		"run-memory-tool-route",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeLiveSiliconFlow,
		started,
		started.Add(time.Minute),
		protected,
		strings.Repeat("5", 64),
		report,
		[]Artifact{{Name: "memory-tool-route-development.json", Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeMemoryToolRouteDevelopment ||
		manifest.AdmissionMode != MemoryToolRouteDevelopmentAdmissionMode ||
		manifest.PolicyID != usermemory.HybridRelevanceMemoryToolRoutePolicyID ||
		len(body) == 0 {
		t.Fatalf("manifest=%#v", manifest)
	}
}

func TestBuildMemoryToolRouteDevelopmentReportRejectsUnreadyInjectedCase(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingMemoryToolRouteDevelopmentTraces(pool)
	for index := range traces {
		if len(traces[index].FullObservation.FinalMemoryIDs) == 0 {
			continue
		}
		traces[index].MemoryToolRouteReady = false
		break
	}
	if _, _, err := BuildMemoryToolRouteDevelopmentReport(
		pool,
		memoryToolRouteDevelopmentProfile(traces),
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	); err == nil {
		t.Fatal("unready route injected Memory")
	}
}

func TestBuildMemoryToolRouteDevelopmentProfileConfigBindsProviderAndTool(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	cost := memoryToolRouteTestCostBasis()
	costSHA256, err := CostBasisSHA256(cost)
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildMemoryToolRouteDevelopmentProfileConfig(
		protected,
		costSHA256,
		ProviderModeLiveSiliconFlow,
		memoryToolRouteTestAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v6" ||
		config.ReaderVersion != MemoryToolRouteReaderVersion ||
		config.CaptureMode != CaptureModeMemoryToolRouteDevelopment ||
		!config.MemoryToolRouteRequired ||
		config.MemoryToolRouteProviderID != "configured-gpt" ||
		config.MemoryToolRouteModelID != "gpt-test" ||
		config.MemoryToolRouteContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
		config.MemoryToolRouteDecodingProfile != usermemory.HybridMemoryToolDecodingProfile ||
		config.MemoryToolRouteMaximumOutputTokens !=
			usermemory.HybridMemoryToolMaximumOutputTokens ||
		config.MemoryToolRouteTemperature == nil ||
		*config.MemoryToolRouteTemperature != usermemory.HybridMemoryToolTemperature ||
		config.MemoryToolRouteDisableThinking != usermemory.HybridMemoryToolDisableThinking {
		t.Fatalf("config=%#v", config)
	}
	firstHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	config.MemoryToolRouteDecodingProfile = "drifted"
	secondHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("Memory Tool-route decoding drift did not change configuration hash")
	}
}

func passingMemoryToolRouteDevelopmentTraces(
	pool memoryauthor.RegressionPool,
) []CandidateCalibrationTrace {
	traces := passingCloudJudgeDevelopmentTraces(pool)
	for index := range traces {
		trace := &traces[index]
		trace.CloudJudgeReady = false
		trace.CloudJudgeInputTokenUpperBound = 0
		trace.MemoryToolRouteReady = true
		trace.MemoryToolRouteInputTokenUpperBound = 1000
		trace.MemoryToolRouteUsed = len(trace.FullObservation.FinalMemoryIDs) > 0
	}
	return traces
}

func memoryToolRouteDevelopmentProfile(
	traces []CandidateCalibrationTrace,
) CapturedProfile {
	cases := make([]memoryeval.CaseObservation, len(traces))
	for index, trace := range traces {
		cases[index] = trace.FullObservation
	}
	return CapturedProfile{
		Profile: memoryeval.Profile{
			ID: CandidateProfileID, Role: "candidate",
			ReaderVersion:        MemoryToolRouteReaderVersion,
			ConfigurationSHA256:  strings.Repeat("a", 64),
			CandidateLimit:       usermemory.MaxHybridShadowResults,
			FinalLimit:           usermemory.HybridShadowFinalLimit,
			ProviderEgressPolicy: memoryeval.ProviderEgressPolicyOwnerAuthorizedNormalCandidatesV1,
		},
		Costs:       memoryToolRouteTestCostBasis().Candidate,
		Cases:       cases,
		Calibration: traces,
	}
}

func memoryToolRouteTestAuthority() MemoryToolRouteProfileAuthority {
	return MemoryToolRouteProfileAuthority{
		ProviderID:    "configured-gpt",
		ProviderType:  "openai_compatible",
		BaseURLSHA256: strings.Repeat("b", 64),
		ModelID:       "gpt-test",
	}
}

func memoryToolRouteTestCostBasis() CostBasis {
	authority := memoryToolRouteTestAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v4",
		ProviderCostPolicy: ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 50,
			ChatProviderCostMicrounits: 100,
		},
		Source: "test", EffectiveAt: "2026-07-29T00:00:00Z",
		MemoryToolRouteAuthority: &MemoryToolRouteCostAuthority{
			ProviderID:                       authority.ProviderID,
			ProviderType:                     authority.ProviderType,
			BaseURLSHA256:                    authority.BaseURLSHA256,
			ModelID:                          authority.ModelID,
			RequestCount:                     300,
			MaximumInputTokens:               300_000,
			MaximumOutputTokens:              300 * usermemory.HybridMemoryToolMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            2,
		},
	}
}
