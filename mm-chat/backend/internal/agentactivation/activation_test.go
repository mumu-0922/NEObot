package agentactivation

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifyRootCanaryRequiresIndependentStageAndBindings(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	config, evidence := rootCanaryActivationFixture(t, now)
	decision, err := VerifyRootCanary(config, now)
	if err != nil || !decision.Ready || decision.ReasonCode != "ROOT_RUN_CANARY_GATES_PASSED" {
		t.Fatalf("VerifyRootCanary() = %#v, %v", decision, err)
	}
	controlConfig := config.Config
	controlConfig.CallerIdentity = ControlCallerIdentity
	if decision, err := Verify(controlConfig, now); !errors.Is(err, ErrInvalid) || decision.ReasonCode != "ACTIVATION_ROOT_INVALID" {
		t.Fatalf("control accepted canary evidence: %#v, %v", decision, err)
	}
	evidence.Authorization.BrokerReadOnly = true
	writeJSON(t, config.RecordFile, evidence)
	decision, err = VerifyRootCanary(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "AUTHORIZATION_WIDENED" {
		t.Fatalf("widened canary = %#v, %v", decision, err)
	}

	config, _ = rootCanaryActivationFixture(t, now)
	writePrivate(t, config.CanaryPlanFile, []byte(`{"schemaVersion":"drift"}`))
	decision, err = VerifyRootCanary(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "CANARY_PLAN_DRIFT" {
		t.Fatalf("plan drift = %#v, %v", decision, err)
	}
}

func TestVerifyAcceptsOnlyExactControlPlaneEvidence(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	config, evidence := activationFixture(t, now)
	decision, err := Verify(config, now)
	if err != nil || !decision.Ready || decision.ReasonCode != "CONTROL_PLANE_GATES_PASSED" {
		t.Fatalf("Verify() = %#v, %v", decision, err)
	}

	evidence.Authorization.RootRuns = true
	writeJSON(t, config.RecordFile, evidence)
	decision, err = Verify(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "AUTHORIZATION_WIDENED" {
		t.Fatalf("widened Verify() = %#v, %v", decision, err)
	}

	evidence.Authorization.RootRuns = false
	evidence.EvidenceClass = "template"
	writeJSON(t, config.RecordFile, evidence)
	decision, err = Verify(config, now)
	if !errors.Is(err, ErrHeld) || decision.ReasonCode != "NON_PRODUCTION_EVIDENCE" {
		t.Fatalf("template Verify() = %#v, %v", decision, err)
	}
}

func TestVerifyRejectsStaleAndMountedFileDrift(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	config, evidence := activationFixture(t, now)
	evidence.Window.ExpiresAt = now
	writeJSON(t, config.RecordFile, evidence)
	decision, err := Verify(config, now)
	if !errors.Is(err, ErrHeld) || decision.ReasonCode != "EVIDENCE_STALE" {
		t.Fatalf("stale Verify() = %#v, %v", decision, err)
	}

	config, _ = activationFixture(t, now)
	if err := os.WriteFile(config.ClientCertificateFile, []byte("drifted certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	decision, err = Verify(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "CLIENT_CERTIFICATE_DRIFT" {
		t.Fatalf("drift Verify() = %#v, %v", decision, err)
	}
}

func TestVerifyRejectsFutureReview(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	config, evidence := activationFixture(t, now)
	evidence.Review.ReviewedAt = now.Add(10 * time.Minute)
	writeJSON(t, config.RecordFile, evidence)
	decision, err := Verify(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "REVIEW_INVALID" {
		t.Fatalf("future review Verify() = %#v, %v", decision, err)
	}
}

func TestVerifyRejectsSymlinkedEvidence(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	config, _ := activationFixture(t, now)
	realRecord := config.RecordFile
	config.RecordFile = filepath.Join(filepath.Dir(realRecord), "activation-link.json")
	if err := os.Symlink(realRecord, config.RecordFile); err != nil {
		t.Fatal(err)
	}
	decision, err := Verify(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "ACTIVATION_UNREADABLE" {
		t.Fatalf("symlinked Verify() = %#v, %v", decision, err)
	}
}

func TestVerifyRejectsMissingExplicitDenyAndCleanupFields(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	config, _ := activationFixture(t, now)
	for name, mutate := range map[string]func(map[string]any){
		"cleanup": func(value map[string]any) { delete(value, "cleanup") },
		"explicit broker deny": func(value map[string]any) {
			delete(value["authorization"].(map[string]any), "brokerMutable")
		},
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			raw, err := os.ReadFile(config.RecordFile)
			if err != nil || json.Unmarshal(raw, &value) != nil {
				t.Fatal("read activation fixture")
			}
			mutate(value)
			writeJSON(t, config.RecordFile, value)
			decision, verifyErr := Verify(config, now)
			if !errors.Is(verifyErr, ErrInvalid) || decision.ReasonCode != "ACTIVATION_ROOT_INVALID" {
				t.Fatalf("Verify() = %#v, %v", decision, verifyErr)
			}
		})
		config, _ = activationFixture(t, now)
	}
}

func TestVerifyRejectsApprovedManifestWithPlaceholderFingerprint(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	config, evidence := activationFixture(t, now)
	var manifest map[string]any
	raw, err := os.ReadFile(config.ReleaseManifestFile)
	if err != nil || json.Unmarshal(raw, &manifest) != nil {
		t.Fatal("read release manifest")
	}
	manifest["probeSuiteFingerprint"] = "sha256:" + strings.Repeat("0", 64)
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writePrivate(t, config.ReleaseManifestFile, manifestRaw)
	evidence.Release.RunnerManifestSHA256 = fingerprint(manifestRaw)
	writeJSON(t, config.RecordFile, evidence)
	decision, verifyErr := Verify(config, now)
	if !errors.Is(verifyErr, ErrInvalid) || decision.ReasonCode != "RUNNER_MANIFEST_INVALID" {
		t.Fatalf("Verify() = %#v, %v", decision, verifyErr)
	}
}

func activationFixture(t *testing.T, now time.Time) (Config, record) {
	t.Helper()
	root := t.TempDir()
	policyFile := filepath.Join(root, "policy.json")
	policyRaw := []byte(`{"schemaVersion":"neo.agent-production-policy/v1","migrationHead":90}`)
	writePrivate(t, policyFile, policyRaw)
	manifestFile := filepath.Join(root, "release-manifest.json")
	manifest := map[string]any{
		"schemaVersion": "neo.agent-runner-release/v1", "approved": true,
		"runnerId": "neo-runner-primary", "runnerVersion": "g21.0-test",
		"protocolVersion": "neo.runner-rpc/v1", "storageDriver": "overlay",
		"networkMode": "none", "userNamespaceSize": 65536,
		"requiredControllers":   []string{"cpu", "memory", "pids"},
		"probeSuiteFingerprint": testFingerprint('8'),
		"seccompProfile":        map[string]any{"path": "/etc/neo-runner/seccomp.json", "sha256": testFingerprint('7')},
		"isolationAcceptance":   map[string]any{"path": "/etc/neo-runner/isolation.json", "sha256": testFingerprint('6')},
	}
	binaries := make([]map[string]any, 0, 5)
	for index, name := range []string{"podman", "crun", "conmon", "newuidmap", "newgidmap"} {
		binaries = append(binaries, map[string]any{
			"name": name, "path": "/opt/neo-runner/bin/" + name,
			"version": "test", "sha256": testFingerprint(byte('1' + index)),
		})
	}
	manifest["binaries"] = binaries
	manifestRaw, _ := json.Marshal(manifest)
	writePrivate(t, manifestFile, manifestRaw)
	certificateFile := filepath.Join(root, "client.crt")
	serverCAFile := filepath.Join(root, "server-ca.crt")
	certificateRaw := []byte("test client certificate bytes")
	serverCARaw := []byte("test server ca bytes")
	writePrivate(t, certificateFile, certificateRaw)
	writePrivate(t, serverCAFile, serverCARaw)
	recordFile := filepath.Join(root, "activation.json")
	endpoint := "https://10.0.0.8:9443/internal/neo-runner/v1/rpc"
	value := record{
		SchemaVersion: SchemaVersion, EvidenceClass: "production", Stage: StageControl,
		Release: release{GitCommit: strings.Repeat("a", 40), MigrationHead: 90,
			RunnerManifestSHA256: fingerprint(manifestRaw), RunnerBinarySHA256: testFingerprint('9'),
			OperationsPolicySHA256: fingerprint(policyRaw)},
		Target: target{DeploymentFingerprint: testFingerprint('a'), RunnerID: "neo-runner-primary"},
		Wiring: wiring{EndpointSHA256: EndpointFingerprint(endpoint),
			ClientCertificateSHA256: fingerprint(certificateRaw), ServerCASHA256: fingerprint(serverCARaw),
			ServerName: "neo-runner.internal", CallerIdentity: "spiffe://neo-chat/agent-runtime-control"},
		Window:        window{StartedAt: now.Add(-10 * time.Minute), CompletedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(time.Hour)},
		Authorization: authorization{ControlPlane: true}, Cleanup: cleanup{},
		Review: review{Decision: "approved", ReviewedAt: now.Add(-time.Minute), ReviewerFingerprint: testFingerprint('b')},
	}
	checkFingerprints := []byte{'c', 'd', 'e', 'f', '1'}
	for index, id := range requiredChecks {
		value.Checks = append(value.Checks, check{ID: id, Result: "passed", ObservedAt: now.Add(-5 * time.Minute),
			EvidenceSHA256: testFingerprint(checkFingerprints[index]), DetailCode: "PASS"})
	}
	writeJSON(t, recordFile, value)
	return Config{PolicyFile: policyFile, RecordFile: recordFile, ReleaseManifestFile: manifestFile,
		ClientCertificateFile: certificateFile, ServerCAFile: serverCAFile, Endpoint: endpoint,
		RunnerID: "neo-runner-primary", ServerName: "neo-runner.internal",
		CallerIdentity: "spiffe://neo-chat/agent-runtime-control", ReleaseCommit: strings.Repeat("a", 40)}, value
}

func rootCanaryActivationFixture(t *testing.T, now time.Time) (RootCanaryConfig, rootCanaryRecord) {
	t.Helper()
	controlConfig, controlEvidence := activationFixture(t, now)
	planFile := filepath.Join(filepath.Dir(controlConfig.RecordFile), "root-canary-plan.json")
	planRaw := []byte(`{"schemaVersion":"neo.agent-root-run-canary-plan/v1","synthetic":true}`)
	writePrivate(t, planFile, planRaw)
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyFile := filepath.Join(filepath.Dir(controlConfig.RecordFile), "authority-public-key")
	publicKeyRaw := []byte(base64.RawURLEncoding.EncodeToString(publicKey))
	writePrivate(t, publicKeyFile, publicKeyRaw)
	controlConfig.CallerIdentity = RootCanaryCallerIdentity
	evidence := rootCanaryRecord{
		SchemaVersion: SchemaVersion, EvidenceClass: "production", Stage: StageRootCanary,
		Release: controlEvidence.Release, Target: controlEvidence.Target,
		Wiring: rootCanaryWiring{EndpointSHA256: controlEvidence.Wiring.EndpointSHA256,
			ClientCertificateSHA256: controlEvidence.Wiring.ClientCertificateSHA256,
			ServerCASHA256:          controlEvidence.Wiring.ServerCASHA256, ServerName: controlEvidence.Wiring.ServerName,
			CallerIdentity: RootCanaryCallerIdentity, CanaryPlanSHA256: fingerprint(planRaw),
			AuthorityPublicKeySHA256: fingerprint(publicKeyRaw)},
		Window: controlEvidence.Window, Authorization: authorization{RootRuns: true}, Cleanup: cleanup{}, Review: controlEvidence.Review,
	}
	for index, id := range rootCanaryChecks {
		evidence.Checks = append(evidence.Checks, check{ID: id, Result: "passed", ObservedAt: now.Add(-5 * time.Minute),
			EvidenceSHA256: testFingerprint(byte('1' + index)), DetailCode: "PASS"})
	}
	writeJSON(t, controlConfig.RecordFile, evidence)
	return RootCanaryConfig{Config: controlConfig, CanaryPlanFile: planFile, AuthorityPublicKeyFile: publicKeyFile}, evidence
}

func testFingerprint(value byte) string { return "sha256:" + strings.Repeat(string(value), 64) }
func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writePrivate(t, path, body)
}
func writePrivate(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
