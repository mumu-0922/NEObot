package rageval

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEvaluatePassingGoldenFixture(t *testing.T) {
	golden := loadGoldenFixture(t)
	observations := loadObservationFixture(t)

	report, err := Evaluate(golden, observations)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !report.Passed || len(report.Failures) != 0 {
		t.Fatalf("report gate = %#v", report)
	}
	if report.CaseCount != 7 || report.RelevantCaseCount != 5 || report.NegativeCaseCount != 2 {
		t.Fatalf("case counts = %#v", report)
	}
	if report.FinalContextPrecision != 1 || report.NegativeFalseCitationRate != 0 || report.NoEvidenceAccuracy != 1 || report.P95LatencyMilliseconds != 47 {
		t.Fatalf("metrics = %#v", report)
	}
	wantRecall := map[string]float64{
		"dense": 1, "lexical": 1, "rewrite_dense": 1,
	}
	for _, lane := range report.LaneRecall {
		if lane.Recall != wantRecall[lane.Lane] {
			t.Fatalf("lane %s recall = %v", lane.Lane, lane.Recall)
		}
		delete(wantRecall, lane.Lane)
	}
	if len(wantRecall) != 0 {
		t.Fatalf("missing lane metrics = %#v", wantRecall)
	}
}

func TestEvaluateRecordedCurrentEngineBaseline(t *testing.T) {
	golden := loadGoldenFixture(t)
	file, err := os.Open("testdata/baseline-current-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	observations, err := DecodeObservationSet(file)
	if err != nil {
		t.Fatal(err)
	}

	report, err := Evaluate(golden, observations)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !report.Passed || report.ProfileID != "tsrank-real-array-jina-v1" ||
		report.P95LatencyMilliseconds != 25402 {
		t.Fatalf("baseline report = %#v", report)
	}
}

func TestGoldenFixtureCoversRequiredRetrievalCategories(t *testing.T) {
	golden := loadGoldenFixture(t)
	want := map[string]bool{
		"exact_identifier":   false,
		"zh_lexical":         false,
		"zh_semantic":        false,
		"context_rewrite":    false,
		"cross_collection":   false,
		"unrelated_negative": false,
	}
	for _, item := range golden.Cases {
		if _, ok := want[item.Category]; ok {
			want[item.Category] = true
		}
	}
	for category, covered := range want {
		if !covered {
			t.Fatalf("Golden Set does not cover %s", category)
		}
	}
}

func TestEvaluateRejectsNegativeCitationAndMissingRelevantEvidence(t *testing.T) {
	golden := loadGoldenFixture(t)
	observations := loadObservationFixture(t)
	for index := range observations.Cases {
		switch observations.Cases[index].CaseID {
		case "zh-semantic-retry":
			observations.Cases[index].FinalContextEvidenceIDs = nil
			observations.Cases[index].CitationEvidenceIDs = nil
			observations.Cases[index].NoEvidence = true
		case "negative-weather":
			observations.Cases[index].FinalContextEvidenceIDs = []string{"e-weather"}
			observations.Cases[index].CitationEvidenceIDs = []string{"e-weather"}
			observations.Cases[index].NoEvidence = false
		}
	}

	report, err := Evaluate(golden, observations)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if report.Passed || report.NegativeFalseCitationRate != 0.5 || report.NoEvidenceAccuracy != 5.0/7.0 {
		t.Fatalf("report = %#v", report)
	}
	joined := strings.Join(report.Failures, "\n")
	if !strings.Contains(joined, "approved evidence was not retained") || !strings.Contains(joined, "unrelated case was not rejected") {
		t.Fatalf("failures = %q", joined)
	}
}

func TestDecodeRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	_, err := DecodeGoldenSet(strings.NewReader(`{
  "schemaVersion":"mm-chat.rag-golden.v1",
  "id":"x",
  "description":"x",
  "criteria":{},
  "cases":[],
  "unexpected":"forbidden"
}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}

	raw, err := os.ReadFile("testdata/golden-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeGoldenSet(strings.NewReader(string(raw) + `{}`))
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("trailing value error = %v", err)
	}
}

func TestReportJSONIsDeterministic(t *testing.T) {
	report, err := Evaluate(loadGoldenFixture(t), loadObservationFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("report JSON changed: %s != %s", first, second)
	}
}

func loadGoldenFixture(t *testing.T) GoldenSet {
	t.Helper()
	file, err := os.Open("testdata/golden-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := DecodeGoldenSet(file)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func loadObservationFixture(t *testing.T) ObservationSet {
	t.Helper()
	file, err := os.Open("testdata/passing-observations-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	value, err := DecodeObservationSet(file)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
