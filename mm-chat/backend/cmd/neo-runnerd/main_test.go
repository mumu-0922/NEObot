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
	if controlCallerIdentity == canaryCallerIdentity {
		t.Fatal("control and canary identities must remain distinct")
	}
}
