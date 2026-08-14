package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadWorkerConfigDefaultsToDisabledAndRejectsExecutionWidening(t *testing.T) {
	if _, err := loadWorkerConfig(func(string) (string, bool) { return "", false }); err == nil || !strings.Contains(err.Error(), envControlEnabled) {
		t.Fatalf("disabled error = %v", err)
	}
	values := validSettings()
	values[envRuntimeEnabled] = "true"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "control-plane") {
		t.Fatalf("widened error = %v", err)
	}
	values = validSettings()
	values[envControlEnabled] = "maybe"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), envControlEnabled) {
		t.Fatalf("invalid bool error = %v", err)
	}
}

func TestLoadWorkerConfigAcceptsExactControlOnlySettings(t *testing.T) {
	resolved, err := loadWorkerConfig(mapLookup(validSettings()))
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.controlEnabled || resolved.runtimeEnabled || resolved.pollInterval != 30*time.Second ||
		resolved.rpcTimeout != 10*time.Second || resolved.batchSize != 100 {
		t.Fatalf("resolved = %#v", resolved)
	}
}

func TestLoadWorkerConfigRejectsDatabaseURLWithoutPassword(t *testing.T) {
	values := validSettings()
	values[envDatabaseURL] = "postgres://agent_runner@postgres:5432/neo_chat?sslmode=disable"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), envDatabaseURL) {
		t.Fatalf("database URL error = %v", err)
	}
}

func TestLoadWorkerConfigRejectsDifferentClientIdentity(t *testing.T) {
	values := validSettings()
	values[envCallerIdentity] = "spiffe://neo-chat/backend"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), envCallerIdentity) {
		t.Fatalf("client identity error = %v", err)
	}
}

func validSettings() map[string]string {
	return map[string]string{
		envControlEnabled: "true", envRuntimeEnabled: "false", envSchedulerEnabled: "false",
		envSkillInstallEnabled: "false", envLearningEnabled: "false", envDelegationEnabled: "false",
		envBrokerReadEnabled: "false", envBrokerMutableEnabled: "false",
		envDatabaseURL: "postgres://agent_runner:secret@postgres:5432/neo_chat?sslmode=disable",
		envRunnerURL:   "https://10.0.0.8:9443/internal/neo-runner/v1/rpc",
		envRunnerID:    "neo-runner-primary", envServerName: "neo-runner.internal",
		envCallerIdentity:    "spiffe://neo-chat/agent-runtime-control",
		envClientCertificate: "/run/secrets/client.crt", envClientKey: "/run/secrets/client.key",
		envServerCA: "/run/secrets/server-ca.crt", envReleaseManifest: "/run/secrets/release.json",
		envProductionPolicy: "/etc/mm-chat/production-policy.json",
		envActivationRecord: "/run/secrets/activation.json", envReleaseCommit: strings.Repeat("a", 40),
	}
}
func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}
