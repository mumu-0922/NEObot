package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/hindsightfixture"
)

func TestRunRejectsOpenOrIncompleteInvocationBeforeNetwork(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"-manifest", "fixture.json"},
		{"-manifest", "fixture.json", "-golden", "golden.json"},
		{"-manifest", "fixture.json", "-golden", "golden.json", "-mode", "live"},
		{"-manifest", "fixture.json", "-golden", "golden.json", "-mode", "end_to_end", "extra"},
	} {
		if err := run(context.Background(), args, &bytes.Buffer{}); err == nil {
			t.Fatalf("run(%q) error = nil", args)
		}
	}
}

func TestRunManifestHashIsOfflineAndNonPromotional(t *testing.T) {
	manifest, err := os.ReadFile("../../../docs/contracts/memory-hindsight-fixture-draft.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{
		"-manifest", path,
		"-print-manifest-hash",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"promotionEligible":false`) ||
		!strings.Contains(output.String(), `"contentSha256":"f829b91807d412dff4a9eab5b823f6f0176f3d3937c6869aefcfa59ffe4e45ad"`) {
		t.Fatalf("hash output = %s", output.String())
	}
}

func TestWriteReportIsContentFreeAndMarksNonPromotion(t *testing.T) {
	report := hindsightfixture.Report{
		SchemaVersion:     hindsightfixture.ReportSchemaVersion,
		PromotionEligible: false,
		Passed:            false,
		ErrorCode:         "timeout",
	}
	var output bytes.Buffer
	if err := writeReport(&output, report, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"promotionEligible": false`) ||
		!strings.Contains(output.String(), `"errorCode": "timeout"`) {
		t.Fatalf("report output = %s", output.String())
	}
}
