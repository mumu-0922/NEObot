package main

import (
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrootcanary"
)

func TestLoadWorkerConfigRequiresExactPrerequisitesAndProductIdentity(t *testing.T) {
	values := validEnvironment()
	config, err := loadWorkerConfig(mapLookup(values))
	if err != nil {
		t.Fatal(err)
	}
	if config.callerIdentity != agentrootcanary.ProductCallerIdentity || config.claimTTL != 5*time.Minute {
		t.Fatalf("config=%#v", config)
	}
	for _, testCase := range []struct{ name, value string }{
		{envEnabled, "false"}, {prerequisiteFlags[0], "false"},
		{envRuntimeEnabled, "true"}, {envCallerIdentity, "spiffe://neo-chat/agent-runtime-root-canary"},
		{envActivationID, "activation_short"}, {envClaimOwner, ""},
	} {
		copy := validEnvironment()
		copy[testCase.name] = testCase.value
		if _, err := loadWorkerConfig(mapLookup(copy)); err == nil {
			t.Fatalf("accepted %s=%s", testCase.name, testCase.value)
		}
	}
}

func validEnvironment() map[string]string {
	values := map[string]string{
		envEnabled: "true", envRuntimeEnabled: "false", envSchedulerEnabled: "false",
		envLearningEnabled: "false", envSkillInstallEnabled: "false",
		envDatabaseURL:  "postgres://product:secret@postgres:5432/neo_chat?sslmode=require",
		envActivationID: "activation_1234567890abcdef", envRunnerURL: "https://10.0.0.2:9443/v1/runner/rpc",
		envRunnerID: "neo-runner-primary", envServerName: "neo-runner.internal",
		envCallerIdentity:    agentrootcanary.ProductCallerIdentity,
		envClientCertificate: "/run/product/client.crt", envClientKey: "/run/product/client.key",
		envServerCA: "/run/product/ca.crt", envReleaseManifest: "/run/product/release.json",
		envProductionPolicy: "/run/product/policy.json", envActivationRecord: "/run/product/activation.json",
		envCanaryPlan: "/run/product/plan.json", envAuthorityPrivateKey: "/run/product/authority.key",
		envAuthorityPublicKey: "/run/product/authority.pub", envReleaseCommit: strings.Repeat("a", 40),
		envClaimOwner: "product-canary-worker",
	}
	for _, name := range prerequisiteFlags {
		values[name] = "true"
	}
	return values
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) { value, ok := values[name]; return value, ok }
}
