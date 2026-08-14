package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadWorkerConfigAcceptsOnlyNarrowRootCanary(t *testing.T) {
	values := validSettings()
	resolved, err := loadWorkerConfig(mapLookup(values))
	if err != nil || !resolved.canaryEnabled || resolved.runtimeEnabled ||
		resolved.authorityTTL != 10*time.Second || resolved.batchSize != 100 {
		t.Fatalf("loadWorkerConfig() = %#v, %v", resolved, err)
	}
	values[envBrokerReadEnabled] = "true"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "synthetic") {
		t.Fatalf("Broker widening error = %v", err)
	}
	values = validSettings()
	values[envCallerIdentity] = "spiffe://neo-chat/agent-runtime-control"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), envCallerIdentity) {
		t.Fatalf("identity reuse error = %v", err)
	}
}

func TestLoadWorkerConfigRejectsSharedOrIncompleteSettings(t *testing.T) {
	if _, err := loadWorkerConfig(func(string) (string, bool) { return "", false }); err == nil ||
		!strings.Contains(err.Error(), envCanaryEnabled) {
		t.Fatalf("disabled error = %v", err)
	}
	values := validSettings()
	values[envDatabaseURL] = "postgres://agent_root_canary@postgres:5432/neo_chat"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), envDatabaseURL) {
		t.Fatalf("database URL error = %v", err)
	}
}

func validSettings() map[string]string {
	return map[string]string{
		envCanaryEnabled: "true", envRuntimeEnabled: "false", envSchedulerEnabled: "false",
		envSkillInstallEnabled: "false", envLearningEnabled: "false", envDelegationEnabled: "false",
		envBrokerReadEnabled: "false", envBrokerWriteEnabled: "false",
		envDatabaseURL: "postgres://agent_root_canary:secret@postgres:5432/neo_chat?sslmode=disable",
		envRunnerURL:   "https://10.0.0.8:9443/internal/neo-runner/v1/rpc", envRunnerID: "neo-runner-primary",
		envServerName: "neo-runner.internal", envCallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary",
		envClientCertificate: "/run/root-canary/client.crt", envClientKey: "/run/root-canary/client.key",
		envServerCA: "/run/root-canary/server-ca.crt", envReleaseManifest: "/run/root-canary/release.json",
		envProductionPolicy: "/etc/root-canary/policy.json", envActivationRecord: "/run/root-canary/activation.json",
		envCanaryPlan: "/run/root-canary/plan.json", envAuthorityPrivateKey: "/run/root-canary/authority-private-key",
		envAuthorityPublicKey: "/run/root-canary/authority-public-key", envReleaseCommit: strings.Repeat("a", 40),
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}
