package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"neo-chat/mm-chat/backend/internal/rageval"
)

func TestRunWritesPassingReport(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{
		"-golden", filepath.Join("..", "..", "internal", "rageval", "testdata", "golden-v1.json"),
		"-observations", filepath.Join("..", "..", "internal", "rageval", "testdata", "passing-observations-v1.json"),
	}, &output)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var report rageval.Report
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if !report.Passed || report.CaseCount != 7 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunRequiresBothInputs(t *testing.T) {
	if err := run([]string{"-golden", "golden.json"}, &bytes.Buffer{}); err == nil {
		t.Fatal("run() error = nil, want invalid arguments")
	}
}
