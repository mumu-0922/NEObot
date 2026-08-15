package main

import "testing"

func TestPrivateAddressAllowsOnlyLiteralLoopbackOrPrivateIP(t *testing.T) {
	for _, value := range []string{"127.0.0.1:9443", "[::1]:9443", "10.20.30.40:9443", "172.16.0.8:9443", "192.168.10.8:9443", "[fd00::8]:9443"} {
		if !privateAddress(value) {
			t.Fatalf("privateAddress(%q) = false", value)
		}
	}
	for _, value := range []string{"0.0.0.0:9443", "[::]:9443", "neo-runner.internal:9443", "8.8.8.8:9443", "169.254.1.1:9443", "[fe80::1]:9443", "127.0.0.1", "10.0.0.8:0", "10.0.0.8:https", ":9443"} {
		if privateAddress(value) {
			t.Fatalf("privateAddress(%q) = true", value)
		}
	}
}

func TestRunnerCallerIdentitiesRemainSeparate(t *testing.T) {
	seen := map[string]struct{}{}
	for _, identity := range []string{controlCallerIdentity, rootCanaryCallerIdentity,
		brokerCanaryCallerIdentity, brokerRelayIdentity, projectCanaryCallerIdentity,
		projectRelayIdentity, childCanaryCallerIdentity, draftLearningCallerIdentity} {
		if _, duplicate := seen[identity]; duplicate {
			t.Fatal("Runner caller and relay identities must remain distinct")
		}
		seen[identity] = struct{}{}
	}
}

func TestProjectRelayConfigurationCannotPartiallyEnable(t *testing.T) {
	for _, name := range []string{"NEO_RUNNER_PROJECT_RELAY_URL", "NEO_RUNNER_PROJECT_RELAY_CLIENT_CERT_FILE",
		"NEO_RUNNER_PROJECT_RELAY_CLIENT_KEY_FILE", "NEO_RUNNER_PROJECT_RELAY_SERVER_CA_FILE",
		"NEO_RUNNER_PROJECT_RELAY_SERVER_NAME", "NEO_RUNNER_PROJECT_RELAY_CLIENT_IDENTITY"} {
		t.Setenv(name, "")
	}
	if projectRelayConfigured() {
		t.Fatal("empty Project relay configuration was enabled")
	}
	t.Setenv("NEO_RUNNER_PROJECT_RELAY_URL", "https://10.0.0.10:9445/internal/agent-broker/v1/relay")
	if !projectRelayConfigured() {
		t.Fatal("partial Project relay configuration was not detected")
	}
}

func TestBrokerRelayConfigurationCannotPartiallyEnable(t *testing.T) {
	t.Setenv("NEO_RUNNER_BROKER_RELAY_URL", "")
	t.Setenv("NEO_RUNNER_BROKER_RELAY_CLIENT_CERT_FILE", "")
	t.Setenv("NEO_RUNNER_BROKER_RELAY_CLIENT_KEY_FILE", "")
	t.Setenv("NEO_RUNNER_BROKER_RELAY_SERVER_CA_FILE", "")
	t.Setenv("NEO_RUNNER_BROKER_RELAY_SERVER_NAME", "")
	t.Setenv("NEO_RUNNER_BROKER_RELAY_CLIENT_IDENTITY", "")
	if brokerRelayConfigured() {
		t.Fatal("empty Broker relay configuration was enabled")
	}
	t.Setenv("NEO_RUNNER_BROKER_RELAY_URL", "https://10.0.0.9:9444/internal/agent-broker/v1/relay")
	if !brokerRelayConfigured() {
		t.Fatal("partial Broker relay configuration was not detected")
	}
}
