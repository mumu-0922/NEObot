package main

import (
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/agentlearningworker"
)

func TestLoadWorkerConfigRequiresIndependentExactStage(t *testing.T) {
	values := validWorkerEnvironment()
	config, err := loadWorkerConfig(mapLookup(values))
	if err != nil || !config.enabled || config.callerIdentity != agentlearningworker.CallerIdentity {
		t.Fatalf("loadWorkerConfig() = %#v, %v", config, err)
	}
	for name, value := range map[string]string{
		envEnabled: "false", envRuntimeEnabled: "true", envLearningEnabled: "true",
		envSchedulerEnabled: "true", envSkillInstallEnabled: "true",
		envBrokerWriteEnabled: "true", envDelegationEnabled: "true",
		"S3_BUCKET_AUTO_CREATE": "true",
	} {
		t.Run(name, func(t *testing.T) {
			candidate := validWorkerEnvironment()
			candidate[name] = value
			if _, err := loadWorkerConfig(mapLookup(candidate)); err == nil {
				t.Fatal("loadWorkerConfig() error = nil")
			}
		})
	}
}

func TestSanitizeDoesNotLeakObjectStoreSecret(t *testing.T) {
	secret := "draft-learning-secret-value"
	if value := sanitize(assertError(secret)); strings.Contains(value, secret) {
		t.Fatalf("sanitize() leaked %q", value)
	}
}

type assertError string

func (value assertError) Error() string { return string(value) }

func validWorkerEnvironment() map[string]string {
	return map[string]string{
		envEnabled: "true", envRuntimeEnabled: "false", envSchedulerEnabled: "false",
		envLearningEnabled: "false", envSkillInstallEnabled: "false",
		envBrokerReadEnabled: "false", envBrokerWriteEnabled: "false", envDelegationEnabled: "false",
		envDatabaseURL: "postgres://draft-worker:secret@postgres:5432/mmchat?sslmode=require",
		envPlanFile:    "/run/agent-draft-learning/plan.json",
		envRunnerURL:   "https://runner:9443/internal/neo-runner/v1/rpc",
		envRunnerID:    "neo-runner-primary", envServerName: "neo-runner.internal",
		envCallerIdentity:       agentlearningworker.CallerIdentity,
		envClientCertificate:    "/run/agent-draft-learning/client.crt",
		envClientKey:            "/run/agent-draft-learning/client.key",
		envServerCA:             "/run/agent-draft-learning/ca.crt",
		envAuthorityPrivateKey:  "/run/agent-draft-learning/authority.key",
		envAuthorityPublicKey:   "/run/agent-draft-learning/authority.pub",
		envReleaseManifest:      "/run/agent-draft-learning/release-manifest.json",
		envProductionPolicy:     "/run/agent-draft-learning/policy.json",
		envActivationRecord:     "/run/agent-draft-learning/activation.json",
		envReleaseCommit:        strings.Repeat("a", 40),
		envObjectCredentialMeta: "/run/agent-draft-learning/object-credential.json",
		"AGENT_DRAFT_LEARNING_WORKER_CONTROL_ACTIVATION_FILE": "/run/agent-draft-learning/control.json",
		"AGENT_DRAFT_LEARNING_WORKER_ROOT_ACTIVATION_FILE":    "/run/agent-draft-learning/root.json",
		"AGENT_DRAFT_LEARNING_WORKER_BROKER_ACTIVATION_FILE":  "/run/agent-draft-learning/broker.json",
		"AGENT_DRAFT_LEARNING_WORKER_PROJECT_ACTIVATION_FILE": "/run/agent-draft-learning/project.json",
		"AGENT_DRAFT_LEARNING_WORKER_CHILD_ACTIVATION_FILE":   "/run/agent-draft-learning/child.json",
		envOwner: "draft-learning-worker-primary", "STORAGE_BACKEND": "minio",
		"S3_ENDPOINT": "minio:9000", "S3_BUCKET": "mm-chat",
		"S3_REGION": "us-east-1", "S3_ACCESS_KEY_ID": "draft-worker",
		"S3_SECRET_ACCESS_KEY": "fixture-secret", "S3_USE_SSL": "false",
		"S3_FORCE_PATH_STYLE": "true", "S3_BUCKET_AUTO_CREATE": "false",
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}
