package agentactivation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

func TestCronWorkerTemplateIsHeldAndAuthorityIsExact(t *testing.T) {
	raw := readWorkerFixture(t, "neo-agent-cron-worker-activation.valid.json")
	if err := validateCronWorkerShape(raw); err != nil {
		t.Fatal(err)
	}
	var record cronWorkerRecord
	if err := strictjson.Decode(raw, maxDocument, &record); err != nil {
		t.Fatal(err)
	}
	config := WorkerConfig{ReleaseCommit: record.Release.GitCommit}
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	if reason := validateCronWorkerRecord(record, config,
		record.Release.OperationsPolicySHA256, now); reason != "ISOLATION_UNAVAILABLE" {
		t.Fatalf("reason = %s", reason)
	}
	passWorkerChecks(record.Checks)
	record.EvidenceClass, record.Review.Decision = "production", "approved"
	record.Release.GitCommit = "1111111111111111111111111111111111111111"
	config.ReleaseCommit = record.Release.GitCommit
	record.Authorization.RunnerCredential = true
	if reason := validateCronWorkerRecord(record, config,
		record.Release.OperationsPolicySHA256, now); reason != "AUTHORIZATION_WIDENED" {
		t.Fatalf("widened reason = %s", reason)
	}
}

func TestDraftLearningWorkerTemplateIsHeldAndCannotPromote(t *testing.T) {
	raw := readWorkerFixture(t, "neo-agent-draft-learning-worker-activation.valid.json")
	if err := validateDraftLearningWorkerShape(raw); err != nil {
		t.Fatal(err)
	}
	var record draftLearningWorkerRecord
	if err := strictjson.Decode(raw, maxDocument, &record); err != nil {
		t.Fatal(err)
	}
	config := DraftLearningWorkerConfig{WorkerConfig: WorkerConfig{
		ReleaseCommit: record.Release.GitCommit}, Endpoint: "https://10.0.0.8:9443/internal/neo-runner/v1/rpc",
		ServerName: "neo-runner.internal", CallerIdentity: DraftLearningCaller}
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	if reason := validateDraftLearningWorkerRecord(record, config,
		record.Release.OperationsPolicySHA256, now); reason != "ISOLATION_UNAVAILABLE" {
		t.Fatalf("reason = %s", reason)
	}
	passWorkerChecks(record.Checks)
	record.EvidenceClass, record.Review.Decision = "production", "approved"
	record.Release.GitCommit = "1111111111111111111111111111111111111111"
	config.ReleaseCommit = record.Release.GitCommit
	record.Authorization.Promote = true
	if reason := validateDraftLearningWorkerRecord(record, config,
		record.Release.OperationsPolicySHA256, now); reason != "AUTHORIZATION_WIDENED" {
		t.Fatalf("widened reason = %s", reason)
	}
}

func passWorkerChecks(checks []check) {
	for index := range checks {
		checks[index].Result = "passed"
		checks[index].DetailCode = "PASS"
	}
}

func readWorkerFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "contracts", "fixtures",
		"agent-runtime", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
