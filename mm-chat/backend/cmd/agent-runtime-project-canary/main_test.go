package main

import (
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentactivation"
	"neo-chat/mm-chat/backend/internal/agentbrokerrelay"
)

func TestLoadWorkerConfigAcceptsOnlyProjectMutationCanary(t *testing.T) {
	resolved, err := loadWorkerConfig(mapLookup(validSettings()))
	if err != nil {
		t.Fatalf("loadWorkerConfig() error = %v", err)
	}
	if !resolved.canaryEnabled || !resolved.controlEnabled || !resolved.rootCanaryEnabled ||
		!resolved.brokerCanaryEnabled || resolved.runtimeEnabled || resolved.schedulerEnabled ||
		resolved.skillInstallEnabled || resolved.learningEnabled || resolved.delegationEnabled ||
		resolved.brokerReadEnabled || resolved.brokerWriteEnabled {
		t.Fatalf("feature flags = %#v", resolved)
	}
	if resolved.rpcTimeout != 10*time.Second || resolved.authorityTTL != 15*time.Second || resolved.batchSize != 100 {
		t.Fatalf("bounded defaults = %#v", resolved)
	}
	for _, broadFlag := range []string{envRuntimeEnabled, envSchedulerEnabled, envSkillInstallEnabled,
		envLearningEnabled, envDelegationEnabled, envBrokerReadEnabled, envBrokerWriteEnabled} {
		values := validSettings()
		values[broadFlag] = "true"
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "synthetic") {
			t.Fatalf("%s widening error = %v", broadFlag, err)
		}
	}
}

func TestLoadWorkerConfigRequiresEarlierCanaries(t *testing.T) {
	for _, prerequisite := range []string{envControlEnabled, envRootCanaryEnabled, envBrokerCanaryEnabled} {
		values := validSettings()
		values[prerequisite] = "false"
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "requires ready") {
			t.Fatalf("%s prerequisite error = %v", prerequisite, err)
		}
	}
}

func TestLoadWorkerConfigRejectsIdentityPathAndEndpointReuse(t *testing.T) {
	for _, identity := range []string{agentactivation.ControlCallerIdentity,
		agentactivation.RootCanaryCallerIdentity, agentactivation.BrokerCanaryCallerIdentity,
		agentactivation.ProjectRunnerRelayIdentity} {
		values := validSettings()
		values[envCallerIdentity] = identity
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "identities") {
			t.Fatalf("caller identity %q error = %v", identity, err)
		}
	}
	values := validSettings()
	values[envApprovalPublicKey] = values[envAuthorityPublicKey]
	if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "reuse") {
		t.Fatalf("approval/authority path reuse = %v", err)
	}
	for name, value := range map[string]string{
		envRelayListen:   "0.0.0.0:9445",
		envRelayEndpoint: "https://project-canary.internal:9445" + agentbrokerrelay.Path,
	} {
		values := validSettings()
		values[name] = value
		if _, err := loadWorkerConfig(mapLookup(values)); err == nil || !strings.Contains(err.Error(), "private literal") {
			t.Fatalf("%s=%q error = %v", name, value, err)
		}
	}
}

func TestDatabaseRoleQueryRequiresExactFourRoleFunctionOnlyContract(t *testing.T) {
	for _, signature := range []string{
		"ARRAY['agent_effect_control','agent_orchestrator_runtime','agent_project_mutation_control','agent_runner_control']::name[]",
		"NOT has_table_privilege(current_user,'agent_project_canary_resources','INSERT,UPDATE,DELETE')",
		"NOT has_table_privilege(current_user,'agent_project_mutation_receipts','INSERT,UPDATE,DELETE')",
		"agent_project_mutation_commit(text,uuid,text,text,bigint,text,text,text,text,text,text,text,bytea,text,text)",
		"agent_project_mutation_status(text,uuid,text,text,text,text)",
		"agent_project_mutation_cleanup(text,uuid,text,text,text)",
	} {
		if !strings.Contains(databaseRoleQuery, signature) {
			t.Fatalf("database role query missing %q", signature)
		}
	}
	for _, forbidden := range []string{"agent_project_mutation_owner", "agent_effect_owner", "agent_orchestrator_owner", "agent_artifact_control"} {
		if strings.Contains(databaseRoleQuery, "'"+forbidden+"'") {
			t.Fatalf("database role query admits %q", forbidden)
		}
	}
}

func TestValidRelayEndpointRequiresExactLiteralPrivateListener(t *testing.T) {
	if !validRelayEndpoint("https://127.0.0.1:9445"+agentbrokerrelay.Path, "127.0.0.1:9445") {
		t.Fatal("exact IPv4 relay rejected")
	}
	for _, endpoint := range []string{
		"http://127.0.0.1:9445" + agentbrokerrelay.Path,
		"https://127.0.0.1:9445" + agentbrokerrelay.Path + "?wide=true",
		"https://user@127.0.0.1:9445" + agentbrokerrelay.Path,
		"https://169.254.1.2:9445" + agentbrokerrelay.Path,
	} {
		if validRelayEndpoint(endpoint, "127.0.0.1:9445") {
			t.Fatalf("unsafe relay endpoint accepted: %s", endpoint)
		}
	}
}

func validSettings() map[string]string {
	return map[string]string{
		envCanaryEnabled: "true", envControlEnabled: "true", envRootCanaryEnabled: "true",
		envBrokerCanaryEnabled: "true", envRuntimeEnabled: "false", envSchedulerEnabled: "false",
		envSkillInstallEnabled: "false", envLearningEnabled: "false", envDelegationEnabled: "false",
		envBrokerReadEnabled: "false", envBrokerWriteEnabled: "false",
		envDatabaseURL: "postgres://agent_project_canary:secret@postgres:5432/neo_chat?sslmode=disable",
		envRunnerURL:   "https://127.0.0.1:9443/internal/neo-runner/v1/rpc", envRunnerID: "neo-runner-primary",
		envRunnerServerName: "neo-runner.internal", envCallerIdentity: agentactivation.ProjectCanaryCallerIdentity,
		envRunnerClientCert: "/run/project-canary/client.crt", envRunnerClientKey: "/run/project-canary/client.key",
		envRunnerServerCA: "/run/project-canary/server-ca.crt", envReleaseManifest: "/run/project-canary/release.json",
		envProductionPolicy: "/etc/project-canary/policy.json", envActivationRecord: "/run/project-canary/activation.json",
		envCanaryPlan: "/run/project-canary/plan.json", envAuthorityPrivateKey: "/run/project-canary/authority-private-key",
		envAuthorityPublicKey: "/run/project-canary/authority-public-key",
		envApprovalDocument:   "/run/project-canary/approval.json", envApprovalPublicKey: "/run/project-canary/approval-public-key",
		envReleaseCommit: strings.Repeat("a", 40), envRelayListen: "127.0.0.1:9445",
		envRelayEndpoint:   "https://127.0.0.1:9445" + agentbrokerrelay.Path,
		envRelayServerCert: "/run/project-canary/relay-server.crt", envRelayServerKey: "/run/project-canary/relay-server.key",
		envRelayClientCA:       "/run/project-canary/relay-client-ca.crt",
		envRunnerRelayIdentity: agentactivation.ProjectRunnerRelayIdentity,
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}
