package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestRunFreezeHashValidatesDraftWithoutClaimingPromotion(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "draft.json")
	writeJSON(t, path, commandDraftGolden())

	var output bytes.Buffer
	if err := run([]string{
		"-golden", path,
		"-print-freeze-hash",
	}, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var report memoryeval.FreezeHashReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode freeze report: %v", err)
	}
	if report.SchemaVersion != memoryeval.FreezeHashSchemaVersion ||
		report.State != "draft" || report.CaseCount != 1 ||
		len(report.FrozenContentSHA256) != 64 || report.PromotionEligible {
		t.Fatalf("freeze report = %#v", report)
	}
}

func TestRunRequiresClosedEvaluationInputs(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"-golden", "golden.json"},
		{"-golden", "golden.json", "-observations", "observations.json"},
		{"-golden", "golden.json", "-print-freeze-hash", "-output", "report.json"},
		{"-regression-corpus", "corpus.json", "-observations", "observations.json", "-output", "report.json"},
		{"-golden", "golden.json", "-regression-corpus", "corpus.json", "-regression-audit", "audit.json", "-observations", "observations.json", "-output", "report.json"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("run(%q) error = nil", args)
		}
	}
}

func TestRunRegressionEvaluationPublishesExplicitNonPromotionalReport(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	corpusPath := filepath.Join(directory, "corpus.json")
	auditPath := filepath.Join(directory, "audit.json")
	observationsPath := filepath.Join(directory, "observations.json")
	outputPath := filepath.Join(directory, "report.json")
	if err := os.WriteFile(corpusPath, pool.CorpusJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(auditPath, pool.AuditJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, observationsPath, commandRegressionObservations(pool))

	var output bytes.Buffer
	args := []string{
		"-regression-corpus", corpusPath,
		"-regression-audit", auditPath,
		"-observations", observationsPath,
		"-output", outputPath,
	}
	if err := run(args, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		SchemaVersion     string `json:"schemaVersion"`
		Passed            bool   `json:"passed"`
		PromotionEligible bool   `json:"promotionEligible"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.PromotionEligible ||
		result.SchemaVersion != "neo-chat.memory-benchmark-regression-report-output.v1" {
		t.Fatalf("regression command output = %+v", result)
	}
	body, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var report memoryeval.RegressionReport
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.PromotionEligible ||
		report.AdmissionMode != memoryeval.RegressionAdmissionMode {
		t.Fatalf("regression report = %+v", report)
	}
	before := append([]byte(nil), body...)
	if err := run(args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second regression evaluation error = %v", err)
	}
	after, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("existing regression report changed after refused overwrite")
	}

	failing := commandRegressionObservations(pool)
	for index := range failing.Cases {
		failing.Cases[index].FinalMemoryIDs = nil
		failing.Cases[index].InjectedMemoryIDs = nil
	}
	failingObservationsPath := filepath.Join(directory, "failing-observations.json")
	failingOutputPath := filepath.Join(directory, "failing-report.json")
	writeJSON(t, failingObservationsPath, failing)
	if err := run([]string{
		"-regression-corpus", corpusPath,
		"-regression-audit", auditPath,
		"-observations", failingObservationsPath,
		"-output", failingOutputPath,
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "gate failed") {
		t.Fatalf("failing regression evaluation error = %v", err)
	}
	failingBody, err := os.ReadFile(failingOutputPath)
	if err != nil {
		t.Fatalf("failed gate did not publish report: %v", err)
	}
	if err := json.Unmarshal(failingBody, &report); err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Failures) == 0 || report.PromotionEligible {
		t.Fatalf("failed regression report = %+v", report)
	}
}

func TestWriteReportExclusiveNeverOverwrites(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "nested", "report.json")
	report := memoryeval.Report{
		SchemaVersion: memoryeval.ReportSchemaVersion,
		Passed:        true,
		Failures:      []string{},
	}
	digest, err := writeReportExclusive(path, report, true)
	if err != nil {
		t.Fatalf("writeReportExclusive() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(body)
	if digest != hex.EncodeToString(expected[:]) || !bytes.HasSuffix(body, []byte("\n")) {
		t.Fatalf("digest/body = %q, %q", digest, body)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("report mode = %o", info.Mode().Perm())
	}
	before := append([]byte(nil), body...)
	if _, err := writeReportExclusive(path, memoryeval.Report{Passed: false}, false); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second write error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("existing report changed after refused overwrite")
	}
}

func commandDraftGolden() memoryeval.GoldenSet {
	return memoryeval.GoldenSet{
		SchemaVersion:     memoryeval.GoldenSchemaVersion,
		ID:                "memory-command-draft",
		Description:       "Synthetic command draft; not reviewed promotion evidence.",
		PromotionEligible: commandBoolPointer(false),
		DataPolicy: memoryeval.DataPolicy{
			SyntheticOnly: true,
		},
		Lifecycle: memoryeval.GoldenLifecycle{State: "draft"},
		Criteria: memoryeval.Criteria{
			MinimumCandidateRecallAt20:       0.95,
			MinimumFinalRecallAt5:            0.90,
			MinimumCurrentFactAccuracy:       0.95,
			MaximumFalseInjectionRate:        0.02,
			MaximumP95LatencyMilliseconds:    900,
			MaximumP99LatencyMilliseconds:    1500,
			HardCutoffMilliseconds:           2000,
			MaximumAveragePromptMemoryTokens: 600,
			MaximumPromptMemoryTokens:        900,
			MaximumProviderCostRatio:         0.15,
		},
		Cases: []memoryeval.GoldenCase{
			{
				ID:                        "draft-stable-fact",
				Query:                     "我的合成偏好是什么？",
				Split:                     "development",
				Language:                  "zh",
				Slices:                    []string{"stable_fact"},
				FixtureAlias:              "synthetic-user-a",
				Scope:                     memoryeval.Scope{UserAlias: "user-a"},
				ExpectedRelevantMemoryIDs: []string{"memory-a"},
				ExpectedCurrentMemoryIDs:  []string{"memory-a"},
				Review:                    memoryeval.Review{State: "draft"},
			},
		},
	}
}

func commandRegressionObservations(pool memoryauthor.RegressionPool) memoryeval.RegressionObservationSet {
	cases := make([]memoryeval.CaseObservation, 0, len(pool.Corpus.Cases))
	for _, item := range pool.Corpus.Cases {
		fallback := "none"
		if item.ExpectedNoMemory {
			fallback = "no_memory"
		}
		cases = append(cases, memoryeval.CaseObservation{
			CaseID:              item.ID,
			CandidateMemoryIDs:  append([]string(nil), item.ExpectedRelevantMemoryIDs...),
			FinalMemoryIDs:      append([]string(nil), item.ExpectedRelevantMemoryIDs...),
			InjectedMemoryIDs:   append([]string(nil), item.ExpectedRelevantMemoryIDs...),
			LatencyMilliseconds: 20,
			PromptMemoryTokens:  20 * len(item.ExpectedRelevantMemoryIDs),
			Fallback:            fallback,
		})
	}
	return memoryeval.RegressionObservationSet{
		SchemaVersion:         memoryeval.RegressionObservationSchemaVersion,
		CorpusID:              pool.Corpus.ID,
		CorpusContentSHA256:   pool.Corpus.CorpusContentSHA256,
		AuditContentSHA256:    pool.Audit.ContentSHA256,
		FixtureManifestSHA256: pool.Corpus.FixtureManifestSHA256,
		CapturedAt:            "2026-07-29T13:00:00Z",
		CaptureID:             "33333333-3333-4333-8333-333333333333",
		Profile: memoryeval.Profile{
			ID:                  "command-regression-profile",
			Role:                "baseline",
			ReaderVersion:       "fixture-reader-v1",
			ConfigurationSHA256: strings.Repeat("e", 64),
			CandidateLimit:      20,
			FinalLimit:          5,
		},
		Costs: memoryeval.ProviderCosts{
			Unit:                         "synthetic-microunit",
			MemoryProviderCostMicrounits: 1,
			ChatProviderCostMicrounits:   100,
		},
		Cases: cases,
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func commandBoolPointer(value bool) *bool {
	return &value
}
