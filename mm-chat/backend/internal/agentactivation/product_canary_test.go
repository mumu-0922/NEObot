package agentactivation

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrootcanary"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

func TestProductCanaryTemplateIsHeldAtExactHostIsolation(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/contracts/fixtures/agent-runtime/neo-agent-product-canary-activation.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var record productCanaryRecord
	if err := strictjson.Decode(raw, maxDocument, &record); err != nil {
		t.Fatal(err)
	}
	config := productCanaryTestConfig(record.Release.GitCommit)
	config.ActivationID = record.Wiring.ActivationID
	record.Wiring.EndpointSHA256 = EndpointFingerprint(config.Endpoint)
	if reason := validateProductCanaryRecord(record, config, record.Release.OperationsPolicySHA256,
		time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)); reason != "ISOLATION_UNAVAILABLE" {
		t.Fatalf("validateProductCanaryRecord() reason = %q", reason)
	}
}

func TestProductCanaryRejectsInvalidConfigAndBindings(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	config := productCanaryTestConfig("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	record := productCanaryReadyRecord(now, config)
	if reason := validateProductCanaryRecord(record, config,
		record.Release.OperationsPolicySHA256, now); reason != "" {
		t.Fatalf("ready record reason = %q", reason)
	}

	badConfig := config
	badConfig.ActivationID = "activation_short"
	if validateProductCanaryConfig(badConfig) == nil {
		t.Fatal("short activation ID was accepted")
	}

	duplicate := record
	duplicate.Prerequisites.DraftLearning = duplicate.Prerequisites.CronWorker
	if reason := validateProductCanaryRecord(duplicate, config,
		record.Release.OperationsPolicySHA256, now); reason != "PREREQUISITE_BINDING_INVALID" {
		t.Fatalf("duplicate prerequisite reason = %q", reason)
	}

	placeholder := record
	placeholder.Checks = append([]check(nil), record.Checks...)
	placeholder.Checks[0].EvidenceSHA256 = testFingerprint('0')
	if reason := validateProductCanaryRecord(placeholder, config,
		record.Release.OperationsPolicySHA256, now); reason != "PLACEHOLDER_BINDING_FORBIDDEN" {
		t.Fatalf("placeholder reason = %q", reason)
	}

	if reason := validateProductCanaryRecord(record, config, testFingerprint('f'), now); reason != "RELEASE_INVALID" {
		t.Fatalf("policy drift reason = %q", reason)
	}
}

func TestVerifyProductCanaryBindsPlanAndPolicyFiles(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	base, _ := activationFixture(t, now)
	directory := filepath.Dir(base.RecordFile)
	planFile := filepath.Join(directory, "product-canary-plan.json")
	planRaw := []byte(`{"schemaVersion":"neo.agent-product-canary-plan/v1"}`)
	writePrivate(t, planFile, planRaw)
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKeyFile := filepath.Join(directory, "authority-public-key")
	publicKeyRaw := []byte(base64.RawURLEncoding.EncodeToString(public))
	writePrivate(t, publicKeyFile, publicKeyRaw)

	config := ProductCanaryConfig{Config: base, ActivationID: "activation_0123456789abcdef",
		CanaryPlanFile: planFile, AuthorityPublicKeyFile: publicKeyFile}
	config.CallerIdentity = agentrootcanary.ProductCallerIdentity
	record := productCanaryReadyRecord(now, config)
	policyRaw, err := os.ReadFile(config.PolicyFile)
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(config.ReleaseManifestFile)
	if err != nil {
		t.Fatal(err)
	}
	clientRaw, err := os.ReadFile(config.ClientCertificateFile)
	if err != nil {
		t.Fatal(err)
	}
	caRaw, err := os.ReadFile(config.ServerCAFile)
	if err != nil {
		t.Fatal(err)
	}
	record.Release.OperationsPolicySHA256 = fingerprint(policyRaw)
	record.Release.RunnerManifestSHA256 = fingerprint(manifestRaw)
	record.Wiring.ClientCertificateSHA256 = fingerprint(clientRaw)
	record.Wiring.ServerCASHA256 = fingerprint(caRaw)
	record.Wiring.CanaryPlanSHA256 = fingerprint(planRaw)
	record.Wiring.AuthorityPublicKeySHA256 = fingerprint(publicKeyRaw)
	writeJSON(t, config.RecordFile, record)

	decision, err := VerifyProductCanary(config, now)
	if err != nil || !decision.Ready {
		t.Fatalf("VerifyProductCanary() = %#v, %v", decision, err)
	}

	writePrivate(t, planFile, []byte(`{"schemaVersion":"drift"}`))
	decision, err = VerifyProductCanary(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "CANARY_PLAN_DRIFT" {
		t.Fatalf("plan drift = %#v, %v", decision, err)
	}

	writePrivate(t, planFile, planRaw)
	writePrivate(t, config.PolicyFile, []byte(`{"schemaVersion":"neo.agent-production-policy/v1","migrationHead":95,"drift":true}`))
	decision, err = VerifyProductCanary(config, now)
	if !errors.Is(err, ErrInvalid) || decision.ReasonCode != "RELEASE_INVALID" {
		t.Fatalf("policy drift = %#v, %v", decision, err)
	}
}

func productCanaryTestConfig(commit string) ProductCanaryConfig {
	return ProductCanaryConfig{Config: Config{
		PolicyFile: "/tmp/policy.json", RecordFile: "/tmp/activation.json",
		ReleaseManifestFile: "/tmp/release.json", ClientCertificateFile: "/tmp/client.crt",
		ServerCAFile: "/tmp/server-ca.crt", Endpoint: "https://10.0.0.8:9443/internal/neo-runner/v1/rpc",
		RunnerID: "neo-runner-primary", ServerName: "neo-runner.internal",
		CallerIdentity: agentrootcanary.ProductCallerIdentity, ReleaseCommit: commit,
	}, ActivationID: "activation_0123456789abcdef", CanaryPlanFile: "/tmp/plan.json",
		AuthorityPublicKeyFile: "/tmp/authority.pub"}
}

func productCanaryReadyRecord(now time.Time, config ProductCanaryConfig) productCanaryRecord {
	fp := func(value string) string { return fingerprint([]byte(value)) }
	record := productCanaryRecord{
		SchemaVersion: SchemaVersion, EvidenceClass: "production", Stage: StageProductCanary,
		Release: release{GitCommit: config.ReleaseCommit, MigrationHead: 95,
			RunnerManifestSHA256: fp("manifest"), RunnerBinarySHA256: fp("binary"),
			OperationsPolicySHA256: fp("policy")},
		Target: target{DeploymentFingerprint: fp("deployment"), RunnerID: config.RunnerID},
		Wiring: productCanaryWiring{ActivationID: config.ActivationID,
			EndpointSHA256: EndpointFingerprint(config.Endpoint), ClientCertificateSHA256: fp("client"),
			ServerCASHA256: fp("ca"), ServerName: config.ServerName, CallerIdentity: config.CallerIdentity,
			CanaryPlanSHA256: fp("plan"), AuthorityPublicKeySHA256: fp("authority")},
		Prerequisites: activationChain{ControlPlane: fp("control"), RootRun: fp("root"),
			BrokerArtifact: fp("broker"), ProjectMutation: fp("project"), DepthOneChild: fp("child"),
			CronWorker: fp("cron"), DraftLearning: fp("learning")},
		Window:        window{StartedAt: now.Add(-10 * time.Minute), CompletedAt: now.Add(-2 * time.Minute), ExpiresAt: now.Add(time.Hour)},
		Authorization: productCanaryAuthorization{ProductCanary: true}, Cleanup: productCanaryCleanup{},
		Review: review{Decision: "approved", ReviewedAt: now.Add(-time.Minute), ReviewerFingerprint: fp("reviewer")},
	}
	for index, id := range productCanaryChecks {
		record.Checks = append(record.Checks, check{ID: id, Result: "passed",
			ObservedAt: now.Add(-5 * time.Minute), EvidenceSHA256: fp(fmt.Sprintf("check-%d", index)), DetailCode: "PASS"})
	}
	return record
}
