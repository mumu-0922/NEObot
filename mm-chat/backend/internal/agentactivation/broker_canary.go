package agentactivation

import (
	"sort"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	StageBrokerCanary          = "broker_artifact_canary"
	BrokerCanaryCallerIdentity = "spiffe://neo-chat/agent-runtime-broker-canary"
	RunnerRelayIdentity        = "spiffe://neo-chat/neo-runner-broker-relay"
)

var brokerCanaryChecks = []string{
	"artifact_authority_ready",
	"artifact_cleanup_ready",
	"control_plane_ready",
	"exact_host_isolation",
	"mcp_read_ready",
	"outcome_unknown_no_retry",
	"private_broker_relay_mtls",
	"private_runner_mtls",
	"project_read_ready",
	"restart_recovery_ready",
	"signed_authority_ready",
	"synthetic_plan_ready",
	"workspace_read_ready",
	"zero_inventory",
}

type BrokerCanaryConfig struct {
	Config
	CanaryPlanFile             string
	AuthorityPublicKeyFile     string
	RelayEndpoint              string
	RelayServerCertificateFile string
	RelayClientCAFile          string
	RunnerRelayIdentity        string
}

type brokerCanaryWiring struct {
	EndpointSHA256               string `json:"endpointSha256"`
	ClientCertificateSHA256      string `json:"clientCertificateSha256"`
	ServerCASHA256               string `json:"serverCASha256"`
	ServerName                   string `json:"serverName"`
	CallerIdentity               string `json:"callerIdentity"`
	CanaryPlanSHA256             string `json:"canaryPlanSha256"`
	AuthorityPublicKeySHA256     string `json:"authorityPublicKeySha256"`
	RelayEndpointSHA256          string `json:"relayEndpointSha256"`
	RelayServerCertificateSHA256 string `json:"relayServerCertificateSha256"`
	RelayClientCASHA256          string `json:"relayClientCASha256"`
	RunnerRelayIdentity          string `json:"runnerRelayIdentity"`
}

type brokerCanaryAuthorization struct {
	ControlPlane        bool `json:"controlPlane"`
	RootRuns            bool `json:"rootRuns"`
	BrokerReadOnly      bool `json:"brokerReadOnly"`
	ArtifactPublication bool `json:"artifactPublication"`
	BrokerMutable       bool `json:"brokerMutable"`
	Delegation          bool `json:"delegation"`
	Scheduler           bool `json:"scheduler"`
	Learning            bool `json:"learning"`
}

type brokerCanaryCleanup struct {
	OrphanSandboxes   int `json:"orphanSandboxes"`
	ScratchResidue    int `json:"scratchResidue"`
	ArtifactResidue   int `json:"artifactResidue"`
	QuarantineResidue int `json:"quarantineResidue"`
}

type brokerCanaryRecord struct {
	SchemaVersion string                    `json:"schemaVersion"`
	EvidenceClass string                    `json:"evidenceClass"`
	Stage         string                    `json:"stage"`
	Release       release                   `json:"release"`
	Target        target                    `json:"target"`
	Wiring        brokerCanaryWiring        `json:"wiring"`
	Window        window                    `json:"window"`
	Checks        []check                   `json:"checks"`
	Authorization brokerCanaryAuthorization `json:"authorization"`
	Cleanup       brokerCanaryCleanup       `json:"cleanup"`
	Review        review                    `json:"review"`
}

func VerifyBrokerCanary(config BrokerCanaryConfig, now time.Time) (Decision, error) {
	if validateBrokerCanaryConfig(config) != nil || now.IsZero() {
		return Decision{ReasonCode: "CONFIG_INVALID"}, ErrInvalid
	}
	policyRaw, err := readRegular(config.PolicyFile, false)
	if err != nil {
		return Decision{ReasonCode: "POLICY_UNREADABLE"}, ErrInvalid
	}
	var policyDocument map[string]any
	if strictjson.Decode(policyRaw, maxDocument, &policyDocument) != nil ||
		policyDocument["schemaVersion"] != "neo.agent-production-policy/v1" ||
		policyDocument["migrationHead"] != float64(95) {
		return Decision{ReasonCode: "POLICY_INVALID"}, ErrInvalid
	}
	policyFingerprint := fingerprint(policyRaw)
	recordRaw, err := readRegular(config.RecordFile, true)
	if err != nil {
		return Decision{PolicyFingerprint: policyFingerprint, ReasonCode: "ACTIVATION_UNREADABLE"}, ErrInvalid
	}
	decision := Decision{PolicyFingerprint: policyFingerprint, EvidenceFingerprint: fingerprint(recordRaw)}
	var evidence brokerCanaryRecord
	if validateBrokerCanaryShape(recordRaw) != nil || strictjson.Decode(recordRaw, maxDocument, &evidence) != nil {
		decision.ReasonCode = "ACTIVATION_ROOT_INVALID"
		return decision, ErrInvalid
	}
	if reason := validateBrokerCanaryRecord(evidence, config, policyFingerprint, now.UTC()); reason != "" {
		decision.ReasonCode = reason
		if member(reason, "NON_PRODUCTION_EVIDENCE", "EVIDENCE_STALE", "ISOLATION_UNAVAILABLE",
			"LIVE_CHECK_NOT_PASSED", "RUNTIME_RESIDUE_REMAINS", "REVIEW_HELD") {
			return decision, ErrHeld
		}
		return decision, ErrInvalid
	}
	manifestRaw, err := readRegular(config.ReleaseManifestFile, true)
	if err != nil {
		decision.ReasonCode = "RUNNER_MANIFEST_UNREADABLE"
		return decision, ErrInvalid
	}
	decision.ManifestFingerprint = fingerprint(manifestRaw)
	if decision.ManifestFingerprint != evidence.Release.RunnerManifestSHA256 {
		decision.ReasonCode = "RUNNER_MANIFEST_DRIFT"
		return decision, ErrInvalid
	}
	manifest, err := agentrunner.ParseReleaseManifest(manifestRaw)
	if err != nil || !manifest.Approved || manifest.RunnerID != config.RunnerID || manifestHasPlaceholder(manifest) {
		decision.ReasonCode = "RUNNER_MANIFEST_INVALID"
		return decision, ErrInvalid
	}
	bindings := []struct {
		path, expected, code string
	}{
		{config.ClientCertificateFile, evidence.Wiring.ClientCertificateSHA256, "CLIENT_CERTIFICATE_DRIFT"},
		{config.ServerCAFile, evidence.Wiring.ServerCASHA256, "SERVER_CA_DRIFT"},
		{config.CanaryPlanFile, evidence.Wiring.CanaryPlanSHA256, "CANARY_PLAN_DRIFT"},
		{config.AuthorityPublicKeyFile, evidence.Wiring.AuthorityPublicKeySHA256, "AUTHORITY_KEY_DRIFT"},
		{config.RelayServerCertificateFile, evidence.Wiring.RelayServerCertificateSHA256, "RELAY_CERTIFICATE_DRIFT"},
		{config.RelayClientCAFile, evidence.Wiring.RelayClientCASHA256, "RELAY_CLIENT_CA_DRIFT"},
	}
	for _, binding := range bindings {
		raw, readErr := readRegular(binding.path, true)
		if readErr != nil || fingerprint(raw) != binding.expected {
			decision.ReasonCode = binding.code
			return decision, ErrInvalid
		}
	}
	if _, err := agentrunner.LoadEd25519PublicKey(config.AuthorityPublicKeyFile); err != nil {
		decision.ReasonCode = "AUTHORITY_KEY_INVALID"
		return decision, ErrInvalid
	}
	decision.Ready, decision.ReasonCode = true, "BROKER_ARTIFACT_CANARY_GATES_PASSED"
	return decision, nil
}

func validateBrokerCanaryShape(raw []byte) error {
	root, err := rawObject(raw, "schemaVersion", "evidenceClass", "stage", "release", "target", "wiring",
		"window", "checks", "authorization", "cleanup", "review")
	if err != nil {
		return err
	}
	for key, fields := range map[string][]string{
		"release": {"gitCommit", "migrationHead", "runnerManifestSha256", "runnerBinarySha256", "operationsPolicySha256"},
		"target":  {"deploymentFingerprint", "runnerId"},
		"wiring": {"endpointSha256", "clientCertificateSha256", "serverCASha256", "serverName", "callerIdentity",
			"canaryPlanSha256", "authorityPublicKeySha256", "relayEndpointSha256",
			"relayServerCertificateSha256", "relayClientCASha256", "runnerRelayIdentity"},
		"window": {"startedAt", "completedAt", "expiresAt"},
		"authorization": {"controlPlane", "rootRuns", "brokerReadOnly", "artifactPublication",
			"brokerMutable", "delegation", "scheduler", "learning"},
		"cleanup": {"orphanSandboxes", "scratchResidue", "artifactResidue", "quarantineResidue"},
		"review":  {"decision", "reviewedAt", "reviewerFingerprint"},
	} {
		if _, objectErr := rawObject(root[key], fields...); objectErr != nil {
			return objectErr
		}
	}
	var checks []any
	if strictjson.Decode(root["checks"], maxDocument, &checks) != nil || len(checks) != len(brokerCanaryChecks) {
		return ErrInvalid
	}
	return nil
}

func validateBrokerCanaryRecord(value brokerCanaryRecord, config BrokerCanaryConfig,
	policyFingerprint string, now time.Time,
) string {
	if value.SchemaVersion != SchemaVersion || value.Stage != StageBrokerCanary ||
		!member(value.EvidenceClass, "template", "production") {
		return "ACTIVATION_VERSION_INVALID"
	}
	if !commitPattern.MatchString(value.Release.GitCommit) || value.Release.MigrationHead != 95 ||
		!validFingerprint(value.Release.RunnerManifestSHA256) || !validFingerprint(value.Release.RunnerBinarySHA256) ||
		!validFingerprint(value.Release.OperationsPolicySHA256) || value.Release.OperationsPolicySHA256 != policyFingerprint {
		return "RELEASE_INVALID"
	}
	if value.Release.GitCommit != config.ReleaseCommit {
		return "RELEASE_COMMIT_DRIFT"
	}
	if !validFingerprint(value.Target.DeploymentFingerprint) || value.Target.RunnerID != config.RunnerID ||
		!identityPattern.MatchString(value.Target.RunnerID) {
		return "TARGET_INVALID"
	}
	fingerprints := []string{value.Release.RunnerManifestSHA256, value.Release.RunnerBinarySHA256,
		value.Target.DeploymentFingerprint, value.Wiring.EndpointSHA256, value.Wiring.ClientCertificateSHA256,
		value.Wiring.ServerCASHA256, value.Wiring.CanaryPlanSHA256, value.Wiring.AuthorityPublicKeySHA256,
		value.Wiring.RelayEndpointSHA256, value.Wiring.RelayServerCertificateSHA256,
		value.Wiring.RelayClientCASHA256}
	if !validFingerprint(value.Wiring.EndpointSHA256) || !validFingerprint(value.Wiring.ClientCertificateSHA256) ||
		!validFingerprint(value.Wiring.ServerCASHA256) || !validFingerprint(value.Wiring.CanaryPlanSHA256) ||
		!validFingerprint(value.Wiring.AuthorityPublicKeySHA256) || !validFingerprint(value.Wiring.RelayEndpointSHA256) ||
		!validFingerprint(value.Wiring.RelayServerCertificateSHA256) || !validFingerprint(value.Wiring.RelayClientCASHA256) ||
		value.Wiring.EndpointSHA256 != EndpointFingerprint(config.Endpoint) ||
		value.Wiring.RelayEndpointSHA256 != BrokerRelayEndpointFingerprint(config.RelayEndpoint) ||
		value.Wiring.ServerName != config.ServerName || value.Wiring.CallerIdentity != config.CallerIdentity ||
		value.Wiring.RunnerRelayIdentity != config.RunnerRelayIdentity {
		return "WIRING_INVALID"
	}
	if value.Window.StartedAt.IsZero() || value.Window.CompletedAt.IsZero() || value.Window.ExpiresAt.IsZero() ||
		value.Window.CompletedAt.Before(value.Window.StartedAt) || !value.Window.ExpiresAt.After(value.Window.CompletedAt) ||
		value.Window.ExpiresAt.Sub(value.Window.StartedAt) > 24*time.Hour {
		return "WINDOW_INVALID"
	}
	results := make(map[string]string, len(value.Checks))
	for _, item := range value.Checks {
		if !contains(brokerCanaryChecks, item.ID) || results[item.ID] != "" ||
			!member(item.Result, "passed", "failed", "not_run", "isolation_unavailable") ||
			item.ObservedAt.Before(value.Window.StartedAt) || item.ObservedAt.After(value.Window.CompletedAt) ||
			!validFingerprint(item.EvidenceSHA256) || !detailPattern.MatchString(item.DetailCode) {
			return "CHECK_SET_INVALID"
		}
		results[item.ID] = item.Result
		fingerprints = append(fingerprints, item.EvidenceSHA256)
	}
	ids := make([]string, 0, len(results))
	for id := range results {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if !sameStrings(ids, brokerCanaryChecks) {
		return "CHECK_SET_INCOMPLETE"
	}
	if value.Authorization.ControlPlane || !value.Authorization.RootRuns || !value.Authorization.BrokerReadOnly ||
		!value.Authorization.ArtifactPublication || value.Authorization.BrokerMutable ||
		value.Authorization.Delegation || value.Authorization.Scheduler || value.Authorization.Learning {
		return "AUTHORIZATION_WIDENED"
	}
	cleanup := value.Cleanup
	if cleanup.OrphanSandboxes < 0 || cleanup.ScratchResidue < 0 || cleanup.ArtifactResidue < 0 ||
		cleanup.QuarantineResidue < 0 || cleanup.OrphanSandboxes > 1_000_000 || cleanup.ScratchResidue > 1_000_000 ||
		cleanup.ArtifactResidue > 1_000_000 || cleanup.QuarantineResidue > 1_000_000 {
		return "CLEANUP_INVALID"
	}
	if !member(value.Review.Decision, "approved", "held") || value.Review.ReviewedAt.Before(value.Window.CompletedAt) ||
		!value.Review.ReviewedAt.Before(value.Window.ExpiresAt) || value.Review.ReviewedAt.After(now) ||
		!validFingerprint(value.Review.ReviewerFingerprint) {
		return "REVIEW_INVALID"
	}
	fingerprints = append(fingerprints, value.Review.ReviewerFingerprint)
	if results["exact_host_isolation"] == "isolation_unavailable" {
		return "ISOLATION_UNAVAILABLE"
	}
	if value.EvidenceClass != "production" {
		return "NON_PRODUCTION_EVIDENCE"
	}
	if now.Before(value.Window.StartedAt) || !now.Before(value.Window.ExpiresAt) || value.Window.CompletedAt.After(now) {
		return "EVIDENCE_STALE"
	}
	if value.Release.GitCommit == "0000000000000000000000000000000000000000" {
		return "PLACEHOLDER_BINDING_FORBIDDEN"
	}
	for _, item := range fingerprints {
		if item == "sha256:0000000000000000000000000000000000000000000000000000000000000000" {
			return "PLACEHOLDER_BINDING_FORBIDDEN"
		}
	}
	for _, result := range results {
		if result != "passed" {
			return "LIVE_CHECK_NOT_PASSED"
		}
	}
	if cleanup.OrphanSandboxes != 0 || cleanup.ScratchResidue != 0 ||
		cleanup.ArtifactResidue != 0 || cleanup.QuarantineResidue != 0 {
		return "RUNTIME_RESIDUE_REMAINS"
	}
	if value.Review.Decision != "approved" {
		return "REVIEW_HELD"
	}
	return ""
}

func validateBrokerCanaryConfig(config BrokerCanaryConfig) error {
	for _, path := range []string{config.PolicyFile, config.RecordFile, config.ReleaseManifestFile,
		config.ClientCertificateFile, config.ServerCAFile, config.CanaryPlanFile,
		config.AuthorityPublicKeyFile, config.RelayServerCertificateFile, config.RelayClientCAFile} {
		if len(path) < 2 || path[0] != '/' {
			return ErrInvalid
		}
	}
	if config.Endpoint == "" || config.RelayEndpoint == "" || !identityPattern.MatchString(config.RunnerID) ||
		!identityPattern.MatchString(config.ServerName) || config.CallerIdentity != BrokerCanaryCallerIdentity ||
		config.RunnerRelayIdentity != RunnerRelayIdentity || !commitPattern.MatchString(config.ReleaseCommit) {
		return ErrInvalid
	}
	return nil
}

func BrokerRelayEndpointFingerprint(endpoint string) string {
	return fingerprint(append([]byte("neo-agent-broker-relay-endpoint-v1\x00"), []byte(endpoint)...))
}
