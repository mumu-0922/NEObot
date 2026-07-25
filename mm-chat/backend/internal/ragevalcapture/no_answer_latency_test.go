package ragevalcapture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureSupplementalNoAnswerLatencyDiagnosticUsesSeparateOrderedPhases(
	t *testing.T,
) {
	input := newSupplementalLatencyDiagnosticFixture(t)
	diagnostic, err := CaptureSupplementalNoAnswerLatencyDiagnostic(
		context.Background(),
		input,
	)
	if err != nil {
		t.Fatalf("CaptureSupplementalNoAnswerLatencyDiagnostic() error = %v", err)
	}
	wantCold := []string{
		input.LoadedSuite.Suite.Cases[0].ID,
		input.LoadedSuite.Suite.Cases[10].ID,
		input.LoadedSuite.Suite.Cases[20].ID,
		input.LoadedSuite.Suite.Cases[30].ID,
	}
	wantWarm := []string{
		input.LoadedSuite.Suite.Cases[1].ID,
		input.LoadedSuite.Suite.Cases[11].ID,
		input.LoadedSuite.Suite.Cases[21].ID,
		input.LoadedSuite.Suite.Cases[31].ID,
	}
	if strings.Join(diagnostic.Cold.CaseIDs, ",") != strings.Join(wantCold, ",") ||
		strings.Join(diagnostic.Warm.CaseIDs, ",") != strings.Join(wantWarm, ",") {
		t.Fatalf(
			"diagnostic cases = cold %#v / warm %#v",
			diagnostic.Cold.CaseIDs,
			diagnostic.Warm.CaseIDs,
		)
	}
	if !diagnostic.DiagnosticIntegrityPassed ||
		diagnostic.PromotionEvidence || len(diagnostic.Failures) != 0 ||
		diagnostic.Conclusion != "warm_state_within_budget_cold_reproduction_inconclusive" ||
		len(diagnostic.Cold.Cases) != 4 || len(diagnostic.Warm.Cases) != 4 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestCaptureSupplementalNoAnswerLatencyDiagnosticRejectsSourceDrift(
	t *testing.T,
) {
	for _, test := range []struct {
		name   string
		mutate func(*SupplementalNoAnswerLatencyDiagnosticInput)
	}{
		{
			name: "source hash",
			mutate: func(input *SupplementalNoAnswerLatencyDiagnosticInput) {
				input.SourceReport.RawSHA256 = "invalid"
			},
		},
		{
			name: "suite binding",
			mutate: func(input *SupplementalNoAnswerLatencyDiagnosticInput) {
				input.SourceReport.Report.Suite.RawSHA256 = strings.Repeat("7", 64)
			},
		},
		{
			name: "non latency failure",
			mutate: func(input *SupplementalNoAnswerLatencyDiagnosticInput) {
				input.SourceReport.Report.Cases[0].Answered = true
				rebuildSupplementalReportSummaries(&input.SourceReport.Report)
			},
		},
		{
			name: "concurrency",
			mutate: func(input *SupplementalNoAnswerLatencyDiagnosticInput) {
				input.Concurrency = 3
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := newSupplementalLatencyDiagnosticFixture(t)
			test.mutate(&input)
			if _, err := CaptureSupplementalNoAnswerLatencyDiagnostic(
				context.Background(),
				input,
			); err == nil {
				t.Fatal("drifted diagnostic source was accepted")
			}
		})
	}
}

func TestSupplementalLatencyConclusionSeparatesColdAndPersistentLatency(
	t *testing.T,
) {
	for _, test := range []struct {
		name      string
		integrity bool
		cold      int64
		warm      int64
		want      string
	}{
		{
			name: "cold effect", integrity: true, cold: 1600, warm: 600,
			want: "cold_start_effect_observed",
		},
		{
			name: "persistent", integrity: true, cold: 1600, warm: 1200,
			want: "warm_state_latency_exceeded",
		},
		{
			name: "not reproduced", integrity: true, cold: 700, warm: 600,
			want: "warm_state_within_budget_cold_reproduction_inconclusive",
		},
		{
			name: "integrity failure", integrity: false, cold: 1600, warm: 600,
			want: "diagnostic_integrity_failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := supplementalLatencyConclusion(
				test.integrity,
				test.cold,
				test.warm,
				1000,
			); got != test.want {
				t.Fatalf("supplementalLatencyConclusion() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWriteSupplementalLatencyDiagnosticExclusiveNeverOverwrites(t *testing.T) {
	diagnostic, err := CaptureSupplementalNoAnswerLatencyDiagnostic(
		context.Background(),
		newSupplementalLatencyDiagnosticFixture(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "latency.json")
	if err := WriteSupplementalNoAnswerLatencyDiagnosticExclusive(
		path,
		diagnostic,
		true,
	); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSupplementalNoAnswerLatencyDiagnosticExclusive(
		path,
		diagnostic,
		true,
	); err == nil {
		t.Fatal("existing latency diagnostic was overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("existing latency diagnostic changed")
	}
}

func TestValidateSupplementalNoAnswerReportRejectsTamperedSummary(t *testing.T) {
	input := newSupplementalLatencyDiagnosticFixture(t)
	report := input.SourceReport.Report
	report.Summary.FalseAnswers++
	if err := validateSupplementalNoAnswerReport(report); err == nil {
		t.Fatal("tampered supplemental report was accepted")
	}
}

func newSupplementalLatencyDiagnosticFixture(
	t *testing.T,
) SupplementalNoAnswerLatencyDiagnosticInput {
	t.Helper()
	input := newSupplementalNoAnswerFixture(t)
	report, err := CaptureSupplementalNoAnswer(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4; index++ {
		report.Cases[index].LatencyMilliseconds = 1500
		report.Cases[index].LatencyBreakdown.PipelineTotalMilliseconds = 1500
	}
	rebuildSupplementalReportSummaries(&report)
	if report.Passed {
		t.Fatal("latency source fixture unexpectedly passed")
	}
	return SupplementalNoAnswerLatencyDiagnosticInput{
		SupplementalNoAnswerInput: input,
		SourceReport: LoadedSupplementalNoAnswerReport{
			Report: report, RawSHA256: strings.Repeat("8", 64),
		},
	}
}

func rebuildSupplementalReportSummaries(report *SupplementalNoAnswerReport) {
	report.Summary, report.Slices, report.Failures =
		supplementalNoAnswerReportSummaries(report.Cases, report.Criteria)
	report.Passed = len(report.Failures) == 0
}
