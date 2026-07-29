package memorycapture

import (
	"encoding/json"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestBuildRunManifestIsContentFreeAndNonPromotional(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	protected := ProtectedRegression{
		Pool:             pool,
		FixtureRawSHA256: sha256String("fixture"), CorpusRawSHA256: sha256String("corpus"),
		AuditRawSHA256: sha256String("audit"), ManifestRawSHA256: sha256String("manifest"),
	}
	baseline := protocolRegressionReport(BaselineProfileID, "baseline", true)
	candidate := protocolRegressionReport(FakeCandidateProfileID, "candidate", false)
	artifacts := []Artifact{
		{Name: "baseline.observations.json", Body: []byte("{\"ids\":[]}\n")},
		{Name: "candidate.report.json", Body: []byte("{\"passed\":false}\n")},
	}
	started := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	manifest, body, err := BuildRunManifest(
		"run-1", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol, started, started.Add(time.Minute), protected,
		sha256String("cost"), ProfileHashes{
			Baseline: sha256String("baseline"), Candidate: sha256String("candidate"),
			BaselineProfileID: BaselineProfileID, CandidateProfileID: FakeCandidateProfileID,
		}, baseline, candidate, artifacts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.PromotionEligible || manifest.CorpusClass != memoryeval.RegressionCorpusClass ||
		manifest.AdmissionMode != memoryeval.RegressionAdmissionMode || len(body) == 0 {
		t.Fatalf("manifest = %#v", manifest)
	}
	all := append(append([]Artifact(nil), artifacts...), Artifact{Name: "run-manifest.json", Body: body})
	if err := VerifyRetainedArtifactsLeakFree(pool, all, []byte("fixture-live-key")); err != nil {
		t.Fatalf("content-free manifest leak check: %v", err)
	}
}

func TestBuildCalibrationRunManifestBindsSplitPolicyAndAggregateArtifact(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture"),
		CorpusRawSHA256:   sha256String("corpus"),
		AuditRawSHA256:    sha256String("audit"),
		ManifestRawSHA256: sha256String("manifest"),
	}
	report := CalibrationReport{
		SchemaVersion:                 CalibrationReportSchemaVersion,
		CorpusClass:                   memoryeval.RegressionCorpusClass,
		AdmissionMode:                 CalibrationAdmissionMode,
		PromotionEligible:             false,
		Split:                         DevelopmentCalibrationSplit,
		CaseCount:                     300,
		PolicyID:                      usermemory.HybridRelevanceIntentCalibrationPolicyID,
		ProfileID:                     FakeCandidateProfileID,
		ConfigurationSHA256:           sha256String("calibration-config"),
		EvaluatedPairCount:            20301,
		FeasiblePairCount:             1,
		ProviderCostPassed:            true,
		IntentSelectionAlgorithm:      calibrationIntentSelectionAlgorithm,
		IntentEvaluatedThresholdCount: 201,
		IntentFeasibleThresholdCount:  1,
		Selected: &CalibrationSelection{Evaluation: memoryeval.CalibrationEvaluation{
			Passed: true,
		}},
		IntentSelected: &CalibrationIntentSelection{Evaluation: memoryeval.CalibrationEvaluation{
			Passed: true,
		}},
		Diagnostics: CalibrationDiagnostics{
			Version:                      calibrationDiagnosticsVersion,
			OtherCaseCount:               300,
			FailurePairCounts:            map[string]int{},
			IntentFailureThresholdCounts: map[string]int{},
			BestSafetyAttempt:            &CalibrationSelection{},
			BestRecallAttempt:            &CalibrationSelection{},
			BestIntentSafetyAttempt:      &CalibrationIntentSelection{},
			BestIntentRecallAttempt:      &CalibrationIntentSelection{},
			MemoryIntentMarginCurve: newCalibrationThresholdCurve(
				calibrationMinimumAdmissionBP,
				calibrationMaximumAdmissionBP,
			),
			AdmissionSimilarityCurve: newCalibrationThresholdCurve(
				calibrationMinimumAdmissionBP,
				calibrationMaximumAdmissionBP,
			),
			MaximumRerankScoreCurve: newCalibrationThresholdCurve(
				calibrationMinimumFinalBP,
				calibrationMaximumFinalBP,
			),
			TopTwoRerankMarginCurve: newCalibrationThresholdCurve(
				calibrationMinimumFinalBP,
				calibrationMaximumFinalBP,
			),
		},
	}
	artifact := Artifact{Name: "relevance-calibration.json", Body: []byte("{\"aggregate\":true}\n")}
	started := time.Date(2026, 7, 29, 14, 0, 0, 0, time.UTC)
	manifest, body, err := BuildCalibrationRunManifest(
		"run-calibration",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeFakeProtocol,
		started,
		started.Add(time.Minute),
		protected,
		sha256String("cost"),
		report,
		[]Artifact{artifact},
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.PromotionEligible || !manifest.Passed ||
		manifest.CaptureMode != CaptureModeCalibration ||
		manifest.Split != DevelopmentCalibrationSplit ||
		manifest.PolicyID != usermemory.HybridRelevanceIntentCalibrationPolicyID ||
		len(manifest.Artifacts) != 1 || len(body) == 0 {
		t.Fatalf("calibration manifest = %#v", manifest)
	}
	report.Selected = nil
	report.FeasiblePairCount = 0
	report.IntentSelected = nil
	report.IntentFeasibleThresholdCount = 0
	manifest, _, err = BuildCalibrationRunManifest(
		"run-calibration-no-feasible",
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		ProviderModeFakeProtocol,
		started,
		started.Add(time.Minute),
		protected,
		sha256String("cost"),
		report,
		[]Artifact{artifact},
	)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Passed {
		t.Fatalf("no-feasible calibration manifest passed = %#v", manifest)
	}

	report.Diagnostics.AdmissionSimilarityCurve.UnrelatedNegativePassingCaseCounts = nil
	if _, _, err := BuildCalibrationRunManifest(
		"run-calibration-invalid-diagnostics",
		"cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		ProviderModeFakeProtocol,
		started,
		started.Add(time.Minute),
		protected,
		sha256String("cost"),
		report,
		[]Artifact{artifact},
	); err == nil {
		t.Fatal("incomplete aggregate diagnostics were accepted")
	}
}

func TestBuildCloudJudgeDevelopmentRunManifestBindsPolicy(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	protected := ProtectedRegression{
		Pool:              pool,
		FixtureRawSHA256:  sha256String("fixture"),
		CorpusRawSHA256:   sha256String("corpus"),
		AuditRawSHA256:    sha256String("audit"),
		ManifestRawSHA256: sha256String("manifest"),
	}
	cost := cloudJudgeTestCostBasis()
	cost.CloudJudgeAuthority.InputMicrounitsPerMillionTokens = 0
	cost.CloudJudgeAuthority.OutputMicrounitsPerMillionTokens = 0
	cost.CloudJudgeAuthority.MaximumCostMicrounits = 0
	report, reportBody, err := BuildCloudJudgeDevelopmentReport(
		pool,
		cloudJudgeDevelopmentProfile(passingCloudJudgeDevelopmentTraces(pool)),
		hybridJudgeTestModelID,
		cost,
	)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 7, 29, 15, 0, 0, 0, time.UTC)
	manifest, body, err := BuildCloudJudgeDevelopmentRunManifest(
		"run-cloud-judge",
		"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		ProviderModeLiveSiliconFlow,
		started,
		started.Add(time.Minute),
		protected,
		sha256String("cost"),
		report,
		[]Artifact{{Name: "cloud-judge-development.json", Body: reportBody}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Passed || manifest.PromotionEligible ||
		manifest.CaptureMode != CaptureModeCloudJudgeDevelopment ||
		manifest.AdmissionMode != CloudJudgeDevelopmentAdmissionMode ||
		manifest.PolicyID != usermemory.HybridRelevanceCloudJudgeCalibrationPolicyID ||
		len(body) == 0 {
		t.Fatalf("cloud manifest = %#v", manifest)
	}
}

func TestVerifyRetainedArtifactsLeakFreeRejectsPlaintextAndCredential(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	query := pool.Corpus.Cases[0].Query
	memoryText := pool.Fixtures.Fixtures[0].Memories[0].CanonicalContent
	credential := []byte("fixture-live-key")
	for name, body := range map[string][]byte{
		"query":      []byte(`{"query":` + quoteJSON(query) + `}`),
		"memory":     []byte(memoryText),
		"credential": credential,
	} {
		t.Run(name, func(t *testing.T) {
			if err := VerifyRetainedArtifactsLeakFree(pool, []Artifact{{
				Name: "leak.json", Body: body,
			}}, credential); err == nil {
				t.Fatal("protected plaintext was accepted")
			}
		})
	}
}

func protocolRegressionReport(profileID, role string, passed bool) memoryeval.RegressionReport {
	return memoryeval.RegressionReport{
		Passed: passed,
		Profile: memoryeval.ProfileSummary{
			ProfileID: profileID, ProfileRole: role, ReaderVersion: ReaderVersion,
		},
	}
}

func quoteJSON(value string) string {
	body, _ := json.Marshal(value)
	return string(body)
}
