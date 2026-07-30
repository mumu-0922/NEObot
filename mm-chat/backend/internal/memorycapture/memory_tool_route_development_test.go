package memorycapture

import (
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
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
		report.SchemaVersion != MemoryToolFirstRoundDevelopmentReportSchemaVersion ||
		report.PolicyID != usermemory.HybridRelevanceMemoryFirstToolRoundPolicyID ||
		report.ToolName != usermemory.HybridMemoryToolName ||
		report.ToolAdapterVersion != chat.MemoryToolFirstRoundAdapterVersion ||
		report.ToolDecodingProfile != "" || report.ToolMaximumOutputTokens != 0 ||
		report.ToolTemperature != 0 || report.ToolDisableThinking ||
		report.Diagnostics.RouteCompletedCaseCount != 300 ||
		report.Diagnostics.FailedCaseCount != 0 ||
		report.CostAuthority.ActualRequestCount != 300 ||
		strings.Contains(string(body), "failureTaxonomy") ||
		strings.Contains(string(body), "routeFailureCategoryCounts") ||
		strings.Contains(string(body), "retrievalIncompleteCaseCount") ||
		strings.Contains(string(body), "retrievalFailureCodeCounts") ||
		strings.Contains(string(body), "diagnosticCompleteness") ||
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
		[]Artifact{{Name: "memory-first-tool-round-development.json", Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeMemoryToolRouteDevelopment ||
		manifest.AdmissionMode != MemoryToolFirstRoundDevelopmentAdmissionMode ||
		manifest.PolicyID != usermemory.HybridRelevanceMemoryFirstToolRoundPolicyID ||
		len(body) == 0 {
		t.Fatalf("manifest=%#v", manifest)
	}
}

func TestBuildMemoryToolRouteRunManifestReturnsBoundedIntegrityReason(t *testing.T) {
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
	report.CaseCount--
	started := time.Date(2026, 7, 29, 16, 0, 0, 0, time.UTC)
	_, _, err = BuildMemoryToolRouteDevelopmentRunManifest(
		"run-memory-tool-route",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeLiveSiliconFlow,
		started,
		started.Add(time.Minute),
		ProtectedRegression{
			FixtureRawSHA256:  strings.Repeat("1", 64),
			CorpusRawSHA256:   strings.Repeat("2", 64),
			AuditRawSHA256:    strings.Repeat("3", 64),
			ManifestRawSHA256: strings.Repeat("4", 64),
		},
		strings.Repeat("5", 64),
		report,
		[]Artifact{{Name: "memory-first-tool-round-development.json", Body: reportBody}},
	)
	if err == nil || !errors.Is(err, ErrCaptureInvalid) ||
		err.Error() != "native Memory capture is invalid: Memory Tool-route manifest authority" {
		t.Fatalf("bounded manifest error = %v", err)
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

func TestBuildMemoryToolRouteDiagnosticReportAggregatesOnlyBoundedFailures(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingMemoryToolRouteDevelopmentTraces(pool)
	trace := &traces[0]
	trace.MemoryToolRouteReady = false
	trace.MemoryToolRouteUsed = false
	trace.MemoryToolRouteFailureCategory =
		usermemory.HybridMemoryToolRouteFailureRateLimited
	trace.MemoryToolRouteOutputTokenUpperBound = 0
	trace.AbstentionCode = "MEMORY_TOOL_ROUTE_FAILED"
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	trace.FullObservation.Fallback = "no_memory"
	profile := memoryToolRouteDevelopmentProfile(traces)
	profile.Profile.ReaderVersion = MemoryToolFirstRoundDiagnosticReaderVersion
	report, body, err := BuildMemoryToolRouteDiagnosticReport(
		pool,
		profile,
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != MemoryToolFirstRoundDiagnosticReportSchemaVersion ||
		report.AdmissionMode != MemoryToolFirstRoundDiagnosticAdmissionMode ||
		report.FailureTaxonomyVersion != MemoryToolRouteFailureTaxonomyVersion ||
		report.FailureTaxonomySHA256 != MemoryToolRouteFailureTaxonomySHA256 ||
		report.DiagnosticCompleteness != MemoryToolRouteDiagnosticCompletenessPolicy ||
		report.Diagnostics.FailedCaseCount != 1 ||
		report.Diagnostics.RouteFailureCategoryCounts[usermemory.HybridMemoryToolRouteFailureRateLimited] != 1 ||
		sumDiagnosticCounts(report.Diagnostics.RouteFailureCategoryCounts) != 1 ||
		report.Diagnostics.RetrievalIncompleteCount != 0 ||
		report.Diagnostics.RetrievalFailureCodeCounts == nil ||
		!strings.Contains(string(body), `"retrievalIncompleteCaseCount":0`) ||
		!strings.Contains(string(body), `"retrievalFailureCodeCounts":{}`) ||
		strings.Contains(string(body), "private query") ||
		strings.Contains(string(body), "private upstream") {
		t.Fatalf("diagnostic report=%#v body=%s", report, body)
	}

	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	started := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	manifest, _, err := BuildMemoryToolRouteDiagnosticRunManifest(
		"run-memory-tool-route-diagnostic",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ProviderModeLiveSiliconFlow,
		started,
		started.Add(time.Minute),
		protected,
		strings.Repeat("5", 64),
		report,
		[]Artifact{{
			Name: "memory-first-tool-round-route-diagnostic-development.json",
			Body: body,
		}},
	)
	if err != nil || manifest.CaptureMode != CaptureModeMemoryToolRouteDiagnostic ||
		manifest.AdmissionMode != MemoryToolFirstRoundDiagnosticAdmissionMode {
		t.Fatalf("diagnostic manifest=%#v err=%v", manifest, err)
	}
}

func TestBuildMemoryToolRouteDiagnosticReportRejectsMissingFailureCategory(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingMemoryToolRouteDevelopmentTraces(pool)
	trace := &traces[0]
	trace.MemoryToolRouteReady = false
	trace.MemoryToolRouteOutputTokenUpperBound = 0
	trace.AbstentionCode = "MEMORY_TOOL_ROUTE_FAILED"
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	profile := memoryToolRouteDevelopmentProfile(traces)
	profile.Profile.ReaderVersion = MemoryToolFirstRoundDiagnosticReaderVersion
	if _, _, err := BuildMemoryToolRouteDiagnosticReport(
		pool,
		profile,
		memoryToolRouteTestAuthority(),
		memoryToolRouteTestCostBasis(),
	); err == nil || !errors.Is(err, ErrCaptureInvalid) ||
		!strings.Contains(err.Error(), "report failure_category") {
		t.Fatalf("missing failure category error = %v", err)
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
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v7" ||
		config.ReaderVersion != MemoryToolFirstRoundReaderVersion ||
		config.CaptureMode != CaptureModeMemoryToolRouteDevelopment ||
		!config.MemoryToolRouteRequired ||
		config.MemoryToolRouteProviderID != "configured-gpt" ||
		config.MemoryToolRouteModelID != "gpt-test" ||
		config.MemoryToolRouteContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
		config.MemoryToolRouteAdapterVersion != chat.MemoryToolFirstRoundAdapterVersion ||
		config.MemoryToolRouteDecodingProfile != "" ||
		config.MemoryToolRouteMaximumOutputTokens != 0 ||
		config.MemoryToolRouteTemperature != nil || config.MemoryToolRouteDisableThinking {
		t.Fatalf("config=%#v", config)
	}
	firstHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	config.MemoryToolRouteAdapterVersion = "drifted"
	secondHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("Memory first Tool-round adapter drift did not change configuration hash")
	}
}

func TestBuildMemoryToolRouteDiagnosticProfileConfigBindsFailureTaxonomy(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	costSHA256, err := CostBasisSHA256(memoryToolRouteTestCostBasis())
	if err != nil {
		t.Fatal(err)
	}
	config, err := BuildMemoryToolRouteDiagnosticProfileConfig(
		protected,
		costSHA256,
		ProviderModeLiveSiliconFlow,
		memoryToolRouteTestAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v9" ||
		config.ReaderVersion != MemoryToolFirstRoundDiagnosticReaderVersion ||
		config.CaptureMode != CaptureModeMemoryToolRouteDiagnostic ||
		config.MemoryToolRouteFailureTaxonomyVersion !=
			MemoryToolRouteFailureTaxonomyVersion ||
		config.MemoryToolRouteFailureTaxonomySHA256 !=
			MemoryToolRouteFailureTaxonomySHA256 ||
		config.MemoryToolRouteDiagnosticCompleteness !=
			MemoryToolRouteDiagnosticCompletenessPolicy {
		t.Fatalf("diagnostic config=%#v", config)
	}
	firstHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	config.MemoryToolRouteFailureTaxonomyVersion = "drifted"
	secondHash, err := ConfigurationSHA256(config)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("failure taxonomy drift did not change configuration hash")
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
		trace.MemoryToolRouteOutputTokenUpperBound = 64
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
			ReaderVersion:        MemoryToolFirstRoundReaderVersion,
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
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v5",
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
			MaximumOutputTokens:              300 * 8192,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            4,
		},
	}
}
