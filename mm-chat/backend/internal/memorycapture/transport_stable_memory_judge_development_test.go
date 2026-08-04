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

func TestTransportStableProfileAndCostAreSeparatedFromV12V13(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256: strings.Repeat("1", 64), CorpusRawSHA256: strings.Repeat("2", 64),
		AuditRawSHA256: strings.Repeat("3", 64), ManifestRawSHA256: strings.Repeat("4", 64),
	}
	config, err := BuildTransportStableMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := TransportStableDevelopmentExecutionPolicy(ProviderModeFakeProtocol)
	if err != nil {
		t.Fatal(err)
	}
	if config.SchemaVersion != "neo-chat.memory-regression-profile-config.v14" ||
		config.ReaderVersion != TransportStableMemoryJudgeReaderVersion ||
		config.CaptureMode != CaptureModeTransportStableMemoryJudge ||
		config.AccuracyFirstExecutionPolicy == nil ||
		*config.AccuracyFirstExecutionPolicy != policy ||
		policy.MaximumRetriesPerProviderRequest != 1 ||
		policy.MaximumJudgeRetriesPerRequest != 2 ||
		policy.RetryFallbackDelayMilliseconds != 5000 ||
		policy.SecondJudgeRetryDelayMilliseconds != 10000 ||
		config.CandidateJudgeFailureTaxonomySHA256 != memoryjudge.FailureTaxonomySHA256 {
		t.Fatalf("transport-stable config=%#v", config)
	}
	if err := ValidateTransportStableMemoryJudgeCostAuthority(
		transportStableMemoryJudgeTestCostBasis(),
		FixedMemoryJudgeAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAccuracyFirstMemoryJudgeCostAuthority(
		transportStableMemoryJudgeTestCostBasis(),
		FixedMemoryJudgeAuthority(),
	); err == nil {
		t.Fatal("schema-v12 accepted schema-v14 cost authority")
	}
	accuracyFirst, err := BuildAccuracyFirstMemoryJudgeDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := BuildJudgeFailureDiagnosticDevelopmentProfileConfig(
		protected,
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, historical := range []ProfileConfig{accuracyFirst, diagnostic} {
		body, marshalErr := json.Marshal(historical)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, forbidden := range [][]byte{
			[]byte(`"maximumJudgeRetriesPerRequest"`),
			[]byte(`"secondJudgeRetryDelayMilliseconds"`),
		} {
			if bytes.Contains(body, forbidden) {
				t.Fatalf("historical profile gained schema-v14 field %s: %s", forbidden, body)
			}
		}
	}
}

func TestTransportStableReportPassesOnlyWithoutTerminalJudgeFailure(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	profile, logicalRequests, logicalInputTokens := transportStableProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalRequests,
		logicalInputTokens,
		0,
		0,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, reportBody, err := BuildTransportStableMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		transportStableMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || !report.Evaluation.Passed || report.PolicySelected ||
		report.PromotionEligible || !report.DiagnosticComplete ||
		report.SchemaVersion != TransportStableMemoryJudgeReportSchemaVersion ||
		report.AdmissionMode != TransportStableMemoryJudgeAdmissionMode ||
		len(report.Diagnostics.JudgeTerminalFailureCategoryCounts) != 0 ||
		len(report.ProviderAttempts.JudgeAttemptFailureCategoryCounts) != 0 {
		t.Fatalf("transport-stable report=%#v", report)
	}
	for _, forbidden := range [][]byte{
		[]byte(pool.Corpus.Cases[0].ID), []byte(`"caseId"`), []byte(`"query"`),
		[]byte(`"memory"`), []byte(`"response"`), []byte(`"selectedOrdinals"`),
	} {
		if bytes.Contains(bytes.ToLower(reportBody), bytes.ToLower(forbidden)) {
			t.Fatalf("report retained forbidden surface %q", forbidden)
		}
	}
	protected := ProtectedRegression{
		Pool: pool, FixtureRawSHA256: sha256String("fixture-v5"),
		CorpusRawSHA256: sha256String("corpus-v5"), AuditRawSHA256: sha256String("audit-v5"),
		ManifestRawSHA256: sha256String("manifest-v5"),
	}
	startedAt := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	manifest, firstBody, err := BuildTransportStableMemoryJudgeRunManifest(
		"run-transport-stable",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost-v9"),
		report,
		[]Artifact{{Name: TransportStableMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, secondBody, err := BuildTransportStableMemoryJudgeRunManifest(
		"run-transport-stable",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ProviderModeFakeProtocol,
		startedAt,
		startedAt.Add(time.Minute),
		protected,
		sha256String("cost-v9"),
		report,
		[]Artifact{{Name: TransportStableMemoryJudgeArtifactName, Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeTransportStableMemoryJudge ||
		!bytes.Equal(firstBody, secondBody) {
		t.Fatalf("manifest=%#v replayEqual=%t", manifest, bytes.Equal(firstBody, secondBody))
	}
}

func TestTransportStableReportReconcilesTwoRetriesAndFailsClosed(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	failed := -1
	for index := range traces {
		if traces[index].CloudJudgeInputTokenUpperBound > 0 {
			failed = index
			break
		}
	}
	if failed < 0 {
		t.Fatal("candidate-bearing trace missing")
	}
	trace := &traces[failed]
	trace.CloudJudgeReady = false
	trace.CloudJudgeFailureCategory = string(chat.ProviderFailureTransportFailed)
	trace.AbstentionCode = "CANDIDATE_JUDGE_FAILED"
	trace.ResultCode = "CANDIDATE_JUDGE_FAILED"
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	trace.FullObservation.Fallback = "no_memory"
	trace.FinalRelevanceScores = []float64{}
	profile, logicalRequests, logicalInputTokens := transportStableProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalRequests,
		logicalInputTokens,
		2,
		2*trace.CloudJudgeInputTokenUpperBound,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{
		string(chat.ProviderFailureStreamReadFailed): 1,
		string(chat.ProviderFailureTransportFailed):  2,
	}
	report, _, err := BuildTransportStableMemoryJudgeDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		transportStableMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || !report.Evaluation.Passed ||
		report.Diagnostics.FailureCodeCounts["CANDIDATE_JUDGE_FAILED"] != 1 ||
		report.Diagnostics.JudgeTerminalFailureCategoryCounts[string(chat.ProviderFailureTransportFailed)] != 1 ||
		report.ProviderAttempts.JudgeRetries != 2 ||
		report.ProviderAttempts.JudgeAttemptFailureCategoryCounts[string(chat.ProviderFailureTransportFailed)] != 2 {
		t.Fatalf("transport-stable failed report=%#v", report)
	}
	tampered := report
	tampered.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{
		string(chat.ProviderFailureTransportFailed): 2,
	}
	if validTransportStableMemoryJudgeDevelopmentReport(tampered) {
		t.Fatal("transport-stable report accepted unreconciled attempt counts")
	}
}

func transportStableProfile(
	traces []CandidateCalibrationTrace,
) (CapturedProfile, int, int) {
	profile, logicalRequests, logicalInputTokens := judgeFailureDiagnosticProfile(traces)
	profile.Profile.ReaderVersion = TransportStableMemoryJudgeReaderVersion
	profile.Costs = transportStableMemoryJudgeTestCostBasis().Candidate
	return profile, logicalRequests, logicalInputTokens
}

func transportStableMemoryJudgeTestCostBasis() CostBasis {
	authority := FixedMemoryJudgeAuthority()
	return CostBasis{
		SchemaVersion:      "neo-chat.memory-regression-cost-basis.v9",
		ProviderCostPolicy: ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
		Baseline: memoryeval.ProviderCosts{
			Unit: "cny_microunits", ChatProviderCostMicrounits: 100,
		},
		Candidate: memoryeval.ProviderCosts{
			Unit: "cny_microunits", MemoryProviderCostMicrounits: 50,
			ChatProviderCostMicrounits: 100,
		},
		Source: "test", EffectiveAt: "2026-08-04T00:00:00Z",
		ConfiguredCandidateJudgeAuthority: &ConfiguredCandidateJudgeCostAuthority{
			ProviderID: authority.ProviderID, ProviderType: authority.ProviderType,
			BaseURLSHA256: authority.BaseURLSHA256, ModelID: authority.ModelID,
			RequestCount: 900, MaximumInputTokens: 1_500_000,
			MaximumOutputTokens:              900 * usermemory.HybridCandidateJudgeMaximumOutputTokens,
			InputMicrounitsPerMillionTokens:  1,
			OutputMicrounitsPerMillionTokens: 1,
			MaximumCostMicrounits:            3,
		},
	}
}

func TestTransportStableProfileJSONContainsVersionedRetryFields(t *testing.T) {
	config, err := BuildTransportStableMemoryJudgeDevelopmentProfileConfig(
		ProtectedRegression{
			FixtureRawSHA256: strings.Repeat("1", 64), CorpusRawSHA256: strings.Repeat("2", 64),
			AuditRawSHA256: strings.Repeat("3", 64), ManifestRawSHA256: strings.Repeat("4", 64),
		},
		strings.Repeat("5", 64),
		ProviderModeFakeProtocol,
		FixedMemoryJudgeAuthority(),
		ProviderCostPolicyOwnerAuthorizedAbsoluteV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range [][]byte{
		[]byte(`"maximumJudgeRetriesPerRequest":2`),
		[]byte(`"secondJudgeRetryDelayMilliseconds":10000`),
	} {
		if !bytes.Contains(body, required) {
			t.Fatalf("profile missing %s: %s", required, body)
		}
	}
}
