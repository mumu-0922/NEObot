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
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("run(%q) error = nil", args)
		}
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
