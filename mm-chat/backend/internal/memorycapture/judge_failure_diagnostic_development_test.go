package memorycapture

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
)

func TestJudgeFailureDiagnosticProfileIsSchemaSeparatedFromV12(t *testing.T) {
	protected := ProtectedRegression{
		FixtureRawSHA256:  strings.Repeat("1", 64),
		CorpusRawSHA256:   strings.Repeat("2", 64),
		AuditRawSHA256:    strings.Repeat("3", 64),
		ManifestRawSHA256: strings.Repeat("4", 64),
	}
	v12, err := BuildAccuracyFirstMemoryJudgeDevelopmentProfileConfig(
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
	if diagnostic.SchemaVersion != "neo-chat.memory-regression-profile-config.v13" ||
		diagnostic.ReaderVersion != JudgeFailureDiagnosticReaderVersion ||
		diagnostic.CaptureMode != CaptureModeJudgeFailureDiagnostic ||
		diagnostic.CandidateJudgeFailureTaxonomyVersion != memoryjudge.FailureTaxonomyVersion ||
		diagnostic.CandidateJudgeFailureTaxonomySHA256 != memoryjudge.FailureTaxonomySHA256 ||
		diagnostic.CandidateJudgeDiagnosticCompleteness !=
			JudgeFailureDiagnosticCompletenessPolicy {
		t.Fatalf("diagnostic profile=%#v", diagnostic)
	}
	v12Body, err := json.Marshal(v12)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{
		[]byte("candidateJudgeFailureTaxonomyVersion"),
		[]byte("candidateJudgeFailureTaxonomySha256"),
		[]byte("candidateJudgeDiagnosticCompleteness"),
	} {
		if bytes.Contains(v12Body, forbidden) {
			t.Fatalf("schema-v12 bytes gained schema-v13 field %q", forbidden)
		}
	}
	v12Hash, err := ConfigurationSHA256(v12)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticHash, err := ConfigurationSHA256(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	if v12Hash == diagnosticHash {
		t.Fatal("schema-v12 and schema-v13 configuration identities collided")
	}
}

func TestJudgeFailureDiagnosticBuildsContentFree300CaseReplay(t *testing.T) {
	pool, err := memoryauthor.GenerateRegressionV5()
	if err != nil {
		t.Fatal(err)
	}
	traces := passingCloudJudgeDevelopmentTraces(pool)
	profile, logicalRequests, logicalInputTokens := judgeFailureDiagnosticProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalRequests,
		logicalInputTokens,
		0,
		0,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{}
	report, reportBody, err := BuildJudgeFailureDiagnosticDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		accuracyFirstMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != JudgeFailureDiagnosticReportSchemaVersion ||
		report.AdmissionMode != JudgeFailureDiagnosticAdmissionMode ||
		report.PromotionEligible || report.PolicySelected || report.Passed ||
		!report.DiagnosticComplete ||
		report.FailureTaxonomySHA256 != memoryjudge.FailureTaxonomySHA256 ||
		report.ProviderAttempts.JudgeAttemptFailureCategoryCounts == nil ||
		len(report.ProviderAttempts.JudgeAttemptFailureCategoryCounts) != 0 ||
		report.Diagnostics.JudgeTerminalFailureCategoryCounts == nil ||
		len(report.Diagnostics.JudgeTerminalFailureCategoryCounts) != 0 {
		t.Fatalf("diagnostic report=%#v", report)
	}
	for _, forbidden := range [][]byte{
		[]byte(pool.Corpus.Cases[0].ID),
		[]byte(`"caseId"`),
		[]byte(`"query"`),
		[]byte(`"memory"`),
		[]byte(`"response"`),
		[]byte(`"selectedOrdinals"`),
	} {
		if bytes.Contains(bytes.ToLower(reportBody), bytes.ToLower(forbidden)) {
			t.Fatalf("diagnostic report retained forbidden surface %q", forbidden)
		}
	}
	if bytes.Count(reportBody, []byte(`"judgeAttemptFailureCategoryCounts"`)) != 1 ||
		bytes.Count(reportBody, []byte(`"judgeTerminalFailureCategoryCounts"`)) != 1 {
		t.Fatalf("diagnostic maps missing or duplicated: %s", reportBody)
	}
	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture-v5"),
		CorpusRawSHA256:   sha256String("corpus-v5"),
		AuditRawSHA256:    sha256String("audit-v5"),
		ManifestRawSHA256: sha256String("manifest-v5"),
	}
	startedAt := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	build := func() (RelevanceRunManifest, []byte, error) {
		return BuildJudgeFailureDiagnosticRunManifest(
			"run-judge-failure-diagnostic",
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			ProviderModeFakeProtocol,
			startedAt,
			startedAt.Add(time.Minute),
			protected,
			sha256String("cost-v8"),
			report,
			[]Artifact{{Name: JudgeFailureDiagnosticArtifactName, Body: reportBody}},
		)
	}
	manifest, firstBody, err := build()
	if err != nil {
		t.Fatal(err)
	}
	_, secondBody, err := build()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBody, secondBody) || manifest.Passed ||
		manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeJudgeFailureDiagnostic ||
		manifest.AdmissionMode != JudgeFailureDiagnosticAdmissionMode {
		t.Fatalf("manifest=%#v replayEqual=%t", manifest, bytes.Equal(firstBody, secondBody))
	}
}

func TestJudgeFailureDiagnosticReconcilesRetryAndTerminalCounts(t *testing.T) {
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
		t.Fatal("candidate-bearing Development trace missing")
	}
	trace := &traces[failed]
	trace.CloudJudgeReady = false
	trace.CloudJudgeFailureCategory = string(chat.ProviderFailureRateLimited)
	trace.AbstentionCode = "CANDIDATE_JUDGE_FAILED"
	trace.ResultCode = "CANDIDATE_JUDGE_FAILED"
	trace.FullObservation.FinalMemoryIDs = []string{}
	trace.FullObservation.InjectedMemoryIDs = []string{}
	trace.FullObservation.PromptMemoryTokens = 0
	trace.FullObservation.Fallback = "no_memory"
	trace.FinalRelevanceScores = []float64{}
	profile, logicalRequests, logicalInputTokens := judgeFailureDiagnosticProfile(traces)
	profile.ProviderAttempts = accuracyFirstTelemetry(
		logicalRequests,
		logicalInputTokens,
		1,
		trace.CloudJudgeInputTokenUpperBound,
	)
	profile.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{
		string(chat.ProviderFailureRateLimited): 2,
	}
	report, reportBody, err := BuildJudgeFailureDiagnosticDevelopmentReport(
		pool,
		profile,
		FixedMemoryJudgeAuthority(),
		accuracyFirstMemoryJudgeTestCostBasis(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if report.Diagnostics.FailureCodeCounts["CANDIDATE_JUDGE_FAILED"] != 1 ||
		report.Diagnostics.JudgeTerminalFailureCategoryCounts[string(chat.ProviderFailureRateLimited)] != 1 ||
		report.ProviderAttempts.JudgeAttemptFailureCategoryCounts[string(chat.ProviderFailureRateLimited)] != 2 ||
		report.ProviderAttempts.JudgeRetries != 1 {
		t.Fatalf("reconciliation report=%#v", report)
	}
	tampered := report
	tampered.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{
		string(chat.ProviderFailureRateLimited): 1,
	}
	if validJudgeFailureDiagnosticDevelopmentReport(tampered) {
		t.Fatal("diagnostic accepted an unreconciled attempt map")
	}
	tampered = report
	tampered.Diagnostics.JudgeTerminalFailureCategoryCounts = map[string]int{}
	if validJudgeFailureDiagnosticDevelopmentReport(tampered) {
		t.Fatal("diagnostic accepted a missing terminal category")
	}
	tampered = report
	tampered.ProviderAttempts.JudgeAttemptFailureCategoryCounts = map[string]int{
		memoryjudge.FailureProvenanceDrift: 2,
	}
	if validJudgeFailureDiagnosticDevelopmentReport(tampered) {
		t.Fatal("diagnostic accepted a capture-local attempt category")
	}
	tampered = report
	tampered.Diagnostics.JudgeCompletedCaseCount--
	tampered.Diagnostics.EmptyCandidateCaseCount++
	if validJudgeFailureDiagnosticDevelopmentReport(tampered) {
		t.Fatal("diagnostic accepted a logical Judge request reconciliation drift")
	}
	if bytes.Contains(reportBody, []byte(trace.CaseID)) {
		t.Fatal("diagnostic report retained failed case identity")
	}
}

func judgeFailureDiagnosticProfile(
	traces []CandidateCalibrationTrace,
) (CapturedProfile, int, int) {
	logicalRequests := 0
	logicalInputTokens := 0
	for _, trace := range traces {
		if trace.CloudJudgeInputTokenUpperBound > 0 {
			logicalRequests++
			logicalInputTokens += trace.CloudJudgeInputTokenUpperBound
		}
	}
	profile := cloudJudgeDevelopmentProfile(traces)
	profile.Profile.ID = FakeCandidateProfileID
	profile.Profile.ReaderVersion = JudgeFailureDiagnosticReaderVersion
	profile.Costs = accuracyFirstMemoryJudgeTestCostBasis().Candidate
	return profile, logicalRequests, logicalInputTokens
}

func TestJudgeFailureDiagnosticRejectsCategoryOutsideJudgeFailure(t *testing.T) {
	diagnostics := CloudJudgeDevelopmentDiagnostics{
		FailureCodeCounts: map[string]int{},
	}
	traces := []CandidateCalibrationTrace{{
		AbstentionCode:            "NONE",
		ResultCode:                "OK",
		CloudJudgeFailureCategory: memoryjudge.FailureUnclassified,
	}}
	if _, _, err := aggregateJudgeTerminalFailureCategories(traces, diagnostics); err == nil {
		t.Fatal("non-failed trace retained a terminal Judge category")
	}
}
