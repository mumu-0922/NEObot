package agentrunner

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateLocalPodmanInfoRequiresExactRootlessTuple(t *testing.T) {
	valid := []byte(`{"host":{"security":{"rootless":true,"seccompEnabled":true,"cgroupVersion":"v2"},"ociRuntime":{"name":"crun"},"cgroupManager":"systemd"},"store":{"graphDriverName":"overlay"},"version":{"Version":"6.1.0"}}`)
	if info, err := validateLocalPodmanInfo(valid); err != nil || info.Version.Version != "6.1.0" {
		t.Fatalf("valid local Podman info rejected: %#v, %v", info, err)
	}
	var value map[string]any
	if err := json.Unmarshal(valid, &value); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(map[string]any){
		"rootful": func(input map[string]any) {
			input["host"].(map[string]any)["security"].(map[string]any)["rootless"] = false
		},
		"cgroupfs": func(input map[string]any) {
			input["host"].(map[string]any)["cgroupManager"] = "cgroupfs"
		},
		"old-version": func(input map[string]any) {
			input["version"].(map[string]any)["Version"] = "3.4.4"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var candidate map[string]any
			if err := json.Unmarshal(valid, &candidate); err != nil {
				t.Fatal(err)
			}
			mutate(candidate)
			body, _ := json.Marshal(candidate)
			if _, err := validateLocalPodmanInfo(body); !errors.Is(err, ErrIsolationUnavailable) {
				t.Fatalf("drift accepted: %v", err)
			}
		})
	}
}

func TestLocalSmokeConfigRejectsWritableOrRelativeInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "bin"), filepath.Join(root, "config"), filepath.Join(root, "smoke-state")} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{
		filepath.Join(root, "bin", "podman"),
		filepath.Join(root, "bin", "neo-skill-local-test-workload"),
		filepath.Join(root, "config", "seccomp-agent-v1.json"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := LocalTestSmokeConfig{PodmanPath: paths[0], WorkloadPath: paths[1], SeccompPath: paths[2], StateRoot: filepath.Join(root, "smoke-state")}
	for _, path := range []string{root, filepath.Join(root, "bin"), filepath.Join(root, "config"), config.StateRoot} {
		if err := secureLocalDirectory(path); err != nil {
			resolved, _ := filepath.EvalSymlinks(path)
			info, _ := os.Lstat(path)
			t.Fatalf("valid private directory rejected: path=%q resolved=%q mode=%v", path, resolved, info.Mode())
		}
	}
	for _, path := range paths {
		if err := secureLocalRegularFile(path, 512<<20); err != nil {
			resolved, _ := filepath.EvalSymlinks(path)
			info, _ := os.Lstat(path)
			t.Fatalf("valid private file rejected: path=%q resolved=%q mode=%v", path, resolved, info.Mode())
		}
	}
	if err := validateLocalTestSmokeConfig(config); err != nil {
		t.Fatal(err)
	}
	config.PodmanPath = "podman"
	if err := validateLocalTestSmokeConfig(config); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("relative Podman path accepted: %v", err)
	}
	config.PodmanPath = paths[0]
	if err := os.Chmod(paths[1], 0o622); err != nil {
		t.Fatal(err)
	}
	if err := validateLocalTestSmokeConfig(config); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("writable workload accepted: %v", err)
	}
	if err := os.Chmod(paths[1], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(config.StateRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), config.StateRoot); err != nil {
		t.Fatal(err)
	}
	if err := validateLocalTestSmokeConfig(config); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("symlinked state root accepted: %v", err)
	}
}

func TestSignedLocalCancelBindsSnapshotAndLease(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	attempt := AttemptRef{RunID: "run_1111111111111111", StepID: "step_1111111111111111",
		AttemptID: "attempt_1111111111111111", LeaseGeneration: 1,
		LeaseOwner: localTestRunnerID, LeaseToken: "lease_local_test_111111111111111111111111"}
	snapshot := localFingerprint("snapshot")
	body := CancelRequest{Attempt: attempt, Mode: "kill", ReasonCode: "local_smoke_complete"}
	body.Authority.SnapshotFingerprint = snapshot
	request, err := signedLocalRequest(MethodCancel, "rpc_1111111111111111", strings.Repeat("n", 32), body, attempt, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier, _ := NewSignedAuthorityVerifier(publicKey)
	if err := verifier.Verify(localTestCaller, localTestRunnerID, MethodCancel, request.RequestID,
		request.Nonce, authorityFingerprintForRequest(request), snapshot, attempt,
		request.Cancel.Authority, time.Now()); err != nil {
		t.Fatalf("signed local cancel did not bind exact authority: %v", err)
	}
	forged := attempt
	forged.LeaseToken = "lease_forged_local_test_11111111111111111111"
	if err := verifier.Verify(localTestCaller, localTestRunnerID, MethodCancel, request.RequestID,
		request.Nonce, authorityFingerprintForRequest(request), snapshot, forged,
		request.Cancel.Authority, time.Now()); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("forged local lease was not rejected: %v", err)
	}
}

func TestLocalSmokeReportIsExplicitlyNonProduction(t *testing.T) {
	report := LocalTestSmokeReport{
		SchemaVersion: "neo.agent-runner-local-test-report/v1", EvidenceClass: "local_test",
		ProductionEligible: false, Outcome: "LOCAL_SKILL_SMOKE_PASSED", RuntimeVersion: "6.1.0",
		Checks:  append([]string(nil), localWorkloadChecks...),
		Cleanup: LocalTestSmokeCleanup{ManagedSandboxes: 0}, GeneratedAt: time.Now().UTC(),
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "PROMOTION_READY") || strings.Contains(string(body), "ACTIVATION_READY") ||
		!strings.Contains(string(body), `"productionEligible":false`) {
		t.Fatalf("local report crossed production boundary: %s", body)
	}
}
