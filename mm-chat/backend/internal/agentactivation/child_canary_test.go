package agentactivation

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

func TestChildCanaryTemplateIsHeldAtExactHostIsolation(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "contracts", "fixtures",
		"agent-runtime", "neo-agent-child-run-canary-activation.valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateChildCanaryShape(raw); err != nil {
		t.Fatal(err)
	}
	var record childCanaryRecord
	if err := strictjson.Decode(raw, maxDocument, &record); err != nil {
		t.Fatal(err)
	}
	config := ChildCanaryConfig{Config: Config{Endpoint: "https://10.0.0.8:9443/internal/neo-runner/v1/rpc",
		RunnerID: "neo-runner-primary", ServerName: "neo-runner.internal",
		CallerIdentity: ChildCanaryCallerIdentity, ReleaseCommit: record.Release.GitCommit}}
	record.Wiring.EndpointSHA256 = EndpointFingerprint(config.Endpoint)
	reason := validateChildCanaryRecord(record, config, record.Release.OperationsPolicySHA256,
		time.Date(2026, 8, 15, 1, 12, 0, 0, time.UTC))
	if reason != "ISOLATION_UNAVAILABLE" {
		t.Fatalf("reason = %s", reason)
	}
	record.Authorization.ChildAgents = false
	if reason := validateChildCanaryRecord(record, config, record.Release.OperationsPolicySHA256,
		time.Date(2026, 8, 15, 1, 12, 0, 0, time.UTC)); reason != "AUTHORIZATION_WIDENED" {
		t.Fatalf("narrow authorization reason = %s", reason)
	}
}

func TestChildCanaryInvalidFixtureFailsSemanticAuthorization(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "contracts", "fixtures",
		"agent-runtime", "neo-agent-child-run-canary-activation.invalid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record childCanaryRecord
	if err := strictjson.Decode(raw, maxDocument, &record); err != nil {
		t.Fatal(err)
	}
	config := ChildCanaryConfig{Config: Config{Endpoint: "https://10.0.0.8:9443/internal/neo-runner/v1/rpc",
		RunnerID: "neo-runner-primary", ServerName: "neo-runner.internal",
		CallerIdentity: ChildCanaryCallerIdentity, ReleaseCommit: record.Release.GitCommit}}
	record.Wiring.EndpointSHA256 = EndpointFingerprint(config.Endpoint)
	if reason := validateChildCanaryRecord(record, config, record.Release.OperationsPolicySHA256,
		time.Date(2026, 8, 15, 1, 12, 0, 0, time.UTC)); reason != "AUTHORIZATION_WIDENED" {
		t.Fatalf("reason = %s", reason)
	}
}
