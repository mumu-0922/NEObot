package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadWorkerConfigRequiresExactG21PointFourBoundary(t *testing.T) {
	values := validSettings()
	resolved, err := loadWorkerConfig(mapLookup(values))
	if err != nil || !resolved.canaryEnabled || !resolved.controlEnabled || !resolved.rootCanaryEnabled ||
		!resolved.brokerCanaryEnabled || !resolved.projectCanaryEnabled || !resolved.delegationEnabled ||
		resolved.runtimeEnabled || resolved.brokerReadEnabled || resolved.brokerWriteEnabled ||
		resolved.authorityTTL != 15*time.Second || resolved.batchSize != 100 {
		t.Fatalf("loadWorkerConfig() = %#v, %v", resolved, err)
	}
	for _, name := range []string{envControlEnabled, envRootCanaryEnabled, envBrokerCanaryEnabled,
		envProjectCanaryEnabled, envDelegationEnabled} {
		t.Run("requires "+name, func(t *testing.T) {
			candidate := validSettings()
			candidate[name] = "false"
			if _, err := loadWorkerConfig(mapLookup(candidate)); err == nil || !strings.Contains(err.Error(), "G21.4") {
				t.Fatalf("dependency error = %v", err)
			}
		})
	}
	for _, name := range []string{envRuntimeEnabled, envSchedulerEnabled, envSkillInstallEnabled,
		envLearningEnabled, envBrokerReadEnabled, envBrokerWriteEnabled} {
		t.Run("denies "+name, func(t *testing.T) {
			candidate := validSettings()
			candidate[name] = "true"
			if _, err := loadWorkerConfig(mapLookup(candidate)); err == nil || !strings.Contains(err.Error(), "synthetic") {
				t.Fatalf("widening error = %v", err)
			}
		})
	}
}

func TestLoadWorkerConfigRejectsIdentityAndCredentialDrift(t *testing.T) {
	values := validSettings()
	values[envCallerIdentity] = "spiffe://neo-chat/agent-runtime-project-canary"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), envCallerIdentity) {
		t.Fatalf("identity reuse error = %v", err)
	}
	values = validSettings()
	values[envDatabaseURL] = "postgres://agent_child_canary@postgres:5432/neo_chat"
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), envDatabaseURL) {
		t.Fatalf("database URL error = %v", err)
	}
}

func validSettings() map[string]string {
	return map[string]string{
		envCanaryEnabled: "true", envControlEnabled: "true", envRootCanaryEnabled: "true",
		envBrokerCanaryEnabled: "true", envProjectCanaryEnabled: "true", envDelegationEnabled: "true",
		envRuntimeEnabled: "false", envSchedulerEnabled: "false", envSkillInstallEnabled: "false",
		envLearningEnabled: "false", envBrokerReadEnabled: "false", envBrokerWriteEnabled: "false",
		envDatabaseURL: "postgres://agent_child_canary:secret@postgres:5432/neo_chat?sslmode=disable",
		envRunnerURL:   "https://10.0.0.8:9443/internal/neo-runner/v1/rpc", envRunnerID: "neo-runner-primary",
		envServerName: "neo-runner.internal", envCallerIdentity: "spiffe://neo-chat/agent-runtime-child-canary",
		envClientCertificate: "/run/child-canary/client.crt", envClientKey: "/run/child-canary/client.key",
		envServerCA: "/run/child-canary/server-ca.crt", envReleaseManifest: "/run/child-canary/release.json",
		envProductionPolicy: "/etc/child-canary/policy.json", envActivationRecord: "/run/child-canary/activation.json",
		envCanaryPlan: "/run/child-canary/plan.json", envAuthorityPrivateKey: "/run/child-canary/authority-private-key",
		envAuthorityPublicKey: "/run/child-canary/authority-public-key", envReleaseCommit: strings.Repeat("a", 40),
		envAuthorityTTL: "15s",
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}
