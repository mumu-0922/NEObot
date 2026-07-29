package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
)

func TestGenerateVerifyAndReviewCommands(t *testing.T) {
	external, err := os.MkdirTemp("/var/tmp", "memory-author-cli-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(external) })
	root := filepath.Join(external, "v1")
	var output bytes.Buffer
	if err := run(context.Background(), []string{"generate", "-root", root}, &output); err != nil {
		t.Fatal(err)
	}
	var status memoryauthor.Status
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.CandidateCount != 650 || status.Pending != 650 || status.Frozen {
		t.Fatalf("generate status = %+v", status)
	}
	output.Reset()
	if err := run(context.Background(), []string{"verify", "-root", root}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "query") || strings.Contains(output.String(), "expectedMemory") {
		t.Fatalf("verify output leaked corpus content: %s", output.String())
	}

	if err := run(context.Background(), []string{
		"freeze", "-root", root,
		"-holdout-run-id", "22222222-2222-4222-8222-222222222222",
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "exactly 500 accepted") {
		t.Fatalf("incomplete CLI freeze error = %v", err)
	}
	if err := run(context.Background(), []string{
		"review", "-root", root, "-reviewer", "not-a-uuid",
	}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "UUID") {
		t.Fatalf("invalid reviewer error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	output.Reset()
	if err := run(canceled, []string{
		"review", "-root", root,
		"-reviewer", "11111111-1111-4111-8111-111111111111",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "127.0.0.1") || !strings.Contains(output.String(), "#") {
		t.Fatalf("review output lacks loopback URL: %s", output.String())
	}
}

func TestCommandRejectsUnknownArgumentsAndSourceTreeOutput(t *testing.T) {
	if err := run(context.Background(), nil, &bytes.Buffer{}); err == nil {
		t.Fatal("missing subcommand was accepted")
	}
	if err := run(context.Background(), []string{"unknown"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown subcommand was accepted")
	}
	repository, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	unsafeRoot := filepath.Join(repository, "mm-chat", "backend", "formal-corpus")
	if err := run(context.Background(), []string{"generate", "-root", unsafeRoot}, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "mm-chat/data/memory-benchmark") {
		t.Fatalf("source-tree output error = %v", err)
	}
}
