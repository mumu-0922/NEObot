package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentbrokerrelay"
)

func TestLoadWorkerConfigAcceptsOnlyBrokerArtifactCanary(t *testing.T) {
	values := validSettings()
	resolved, err := loadWorkerConfig(mapLookup(values))
	if err != nil {
		t.Fatalf("loadWorkerConfig() error = %v", err)
	}
	if !resolved.canaryEnabled || resolved.runtimeEnabled || resolved.schedulerEnabled ||
		resolved.skillInstallEnabled || resolved.learningEnabled || resolved.delegationEnabled ||
		resolved.brokerReadEnabled || resolved.brokerWriteEnabled {
		t.Fatalf("feature flags = %#v", resolved)
	}
	if resolved.rpcTimeout != 10*time.Second || resolved.authorityTTL != 15*time.Second ||
		resolved.batchSize != 100 || resolved.s3UseSSL || !resolved.s3ForcePathStyle {
		t.Fatalf("bounded defaults = %#v", resolved)
	}

	for _, broadFlag := range []string{envRuntimeEnabled, envSchedulerEnabled, envSkillInstallEnabled,
		envLearningEnabled, envDelegationEnabled, envBrokerReadEnabled, envBrokerWriteEnabled} {
		widened := validSettings()
		widened[broadFlag] = "true"
		if _, err := loadWorkerConfig(mapLookup(widened)); err == nil || !strings.Contains(err.Error(), "synthetic") {
			t.Fatalf("%s widening error = %v", broadFlag, err)
		}
	}
}

func TestLoadWorkerConfigRejectsIdentityReuseAndNonPrivateRelay(t *testing.T) {
	for _, identity := range []string{
		"spiffe://neo-chat/agent-runtime-control",
		"spiffe://neo-chat/agent-runtime-root-canary",
		agentactivation.RunnerRelayIdentity,
	} {
		values := validSettings()
		values[envCallerIdentity] = identity
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "identities") {
			t.Fatalf("caller identity %q error = %v", identity, err)
		}
	}
	values := validSettings()
	values[envRunnerRelayIdentity] = agentactivation.BrokerCanaryCallerIdentity
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "identities") {
		t.Fatalf("relay identity reuse error = %v", err)
	}

	for name, value := range map[string]string{
		envRelayListen:   "0.0.0.0:9444",
		envRelayEndpoint: "https://broker-canary.internal:9444" + agentbrokerrelay.Path,
	} {
		values = validSettings()
		values[name] = value
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "private literal") {
			t.Fatalf("%s=%q error = %v", name, value, err)
		}
	}
	values = validSettings()
	values[envRelayEndpoint] = "https://127.0.0.2:9444" + agentbrokerrelay.Path
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "private literal") {
		t.Fatalf("relay endpoint mismatch error = %v", err)
	}
}

func TestLoadWorkerConfigRejectsSharedOrUnsafeResources(t *testing.T) {
	if _, err := loadWorkerConfig(func(string) (string, bool) { return "", false }); err == nil ||
		!strings.Contains(err.Error(), envCanaryEnabled) {
		t.Fatalf("disabled error = %v", err)
	}

	cases := []struct {
		name  string
		value string
		want  string
	}{
		{envDatabaseURL, "postgres://agent_broker_canary@postgres:5432/neo_chat", envDatabaseURL},
		{"STORAGE_BACKEND", "filesystem", "S3-compatible"},
		{"S3_BUCKET_AUTO_CREATE", "true", "auto-create"},
		{envRPCTimeout, "11s", "out of range"},
		{envAuthorityTTL, "9s", "out of range"},
		{envAuthorityTTL, "16s", "out of range"},
	}
	for _, tc := range cases {
		values := validSettings()
		values[tc.name] = tc.value
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s=%q error = %v, want %q", tc.name, tc.value, err, tc.want)
		}
	}
}

func TestDatabaseRoleQueryRequiresExactFourRoleFunctionOnlyContract(t *testing.T) {
	for _, signature := range []string{
		"ARRAY['agent_artifact_control','agent_effect_control','agent_orchestrator_runtime','agent_runner_control']::name[]",
		"NOT has_table_privilege(current_user,'agent_artifacts','INSERT,UPDATE,DELETE')",
		"NOT has_table_privilege(current_user,'agent_effect_intents','INSERT,UPDATE,DELETE')",
		"NOT has_table_privilege(current_user,'agent_attempts','INSERT,UPDATE,DELETE')",
		"agent_artifact_authorize(text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)",
		"agent_artifact_attach(text,text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)",
	} {
		if !strings.Contains(databaseRoleQuery, signature) {
			t.Fatalf("database role query missing %q", signature)
		}
	}
	for _, forbidden := range []string{"agent_product_owner", "agent_effect_owner", "agent_orchestrator_owner"} {
		if strings.Contains(databaseRoleQuery, "'"+forbidden+"'") {
			t.Fatalf("database role query admits owner %q", forbidden)
		}
	}
}

func TestValidRelayEndpointRequiresExactLiteralPrivateListener(t *testing.T) {
	if !validRelayEndpoint("https://127.0.0.1:9444"+agentbrokerrelay.Path, "127.0.0.1:9444") {
		t.Fatal("exact IPv4 loopback relay rejected")
	}
	if !validRelayEndpoint("https://[::1]:9444"+agentbrokerrelay.Path, "[::1]:9444") {
		t.Fatal("exact IPv6 loopback relay rejected")
	}
	for _, endpoint := range []string{
		"http://127.0.0.1:9444" + agentbrokerrelay.Path,
		"https://127.0.0.1:9444" + agentbrokerrelay.Path + "?wide=true",
		"https://user@127.0.0.1:9444" + agentbrokerrelay.Path,
		"https://127.0.0.1:9444/",
	} {
		if validRelayEndpoint(endpoint, "127.0.0.1:9444") {
			t.Fatalf("unsafe relay endpoint accepted: %s", endpoint)
		}
	}
}

func TestReadMCPRunnerTokenRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "token")
	if err := os.WriteFile(target, []byte(strings.Repeat("a", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	if token, err := readMCPRunnerToken(target); err != nil || len(token) != 32 {
		t.Fatalf("readMCPRunnerToken() = %q, %v", token, err)
	}
	link := filepath.Join(root, "token-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readMCPRunnerToken(link); err == nil {
		t.Fatal("readMCPRunnerToken() accepted a symlink")
	}
}

func validSettings() map[string]string {
	return map[string]string{
		envCanaryEnabled: "true", envRuntimeEnabled: "false", envSchedulerEnabled: "false",
		envSkillInstallEnabled: "false", envLearningEnabled: "false", envDelegationEnabled: "false",
		envBrokerReadEnabled: "false", envBrokerWriteEnabled: "false",
		envDatabaseURL: "postgres://agent_broker_canary:secret@postgres:5432/neo_chat?sslmode=disable",
		envRunnerURL:   "https://127.0.0.1:9443/internal/neo-runner/v1/rpc", envRunnerID: "neo-runner-primary",
		envRunnerServerName: "neo-runner.internal", envCallerIdentity: agentactivation.BrokerCanaryCallerIdentity,
		envRunnerClientCert: "/run/broker-canary/client.crt", envRunnerClientKey: "/run/broker-canary/client.key",
		envRunnerServerCA: "/run/broker-canary/server-ca.crt", envReleaseManifest: "/run/broker-canary/release.json",
		envProductionPolicy: "/etc/broker-canary/policy.json", envActivationRecord: "/run/broker-canary/activation.json",
		envCanaryPlan: "/run/broker-canary/plan.json", envAuthorityPrivateKey: "/run/broker-canary/authority-private-key",
		envAuthorityPublicKey: "/run/broker-canary/authority-public-key", envReleaseCommit: strings.Repeat("a", 40),
		envRelayListen: "127.0.0.1:9444", envRelayEndpoint: "https://127.0.0.1:9444" + agentbrokerrelay.Path,
		envRelayServerCert: "/run/broker-canary/relay-server.crt", envRelayServerKey: "/run/broker-canary/relay-server.key",
		envRelayClientCA: "/run/broker-canary/relay-client-ca.crt", envRunnerRelayIdentity: agentactivation.RunnerRelayIdentity,
		envQuarantineRoot: "/var/lib/neo-chat/agent-artifacts/quarantine",
		envMCPRunnerURL:   "http://mcp-runner:8090", envMCPRunnerTokenFile: "/run/secrets/mm_chat_mcp_runner_token",
		"STORAGE_BACKEND": "minio", "S3_ENDPOINT": "http://minio:9000", "S3_BUCKET": "neo-chat",
		"S3_REGION": "us-east-1", "S3_ACCESS_KEY_ID": "agent-broker-canary", "S3_SECRET_ACCESS_KEY": "secret",
		"S3_BUCKET_AUTO_CREATE": "false",
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}
