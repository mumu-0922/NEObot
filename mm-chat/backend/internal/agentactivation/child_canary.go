package agentactivation

import (
	"sort"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	StageChildCanary          = "depth_one_child_canary"
	ChildCanaryCallerIdentity = "spiffe://neo-chat/agent-runtime-child-canary"
)

var childCanaryChecks = []string{
	"broker_artifact_canary_ready",
	"child_first_reap_ready",
	"control_plane_ready",
	"credential_absence",
	"delegate_task_removed",
	"depth_one_subset_ready",
	"exact_host_isolation",
	"late_launch_fenced",
	"least_privilege_login",
	"private_runner_mtls",
	"project_mutation_canary_ready",
	"restart_recovery_ready",
	"root_run_canary_ready",
	"signed_authority_ready",
	"synthetic_plan_ready",
	"zero_inventory",
}

type ChildCanaryConfig struct {
	Config
	CanaryPlanFile         string
	AuthorityPublicKeyFile string
}

type childCanaryWiring struct {
	EndpointSHA256           string `json:"endpointSha256"`
	ClientCertificateSHA256  string `json:"clientCertificateSha256"`
	ServerCASHA256           string `json:"serverCASha256"`
	ServerName               string `json:"serverName"`
	CallerIdentity           string `json:"callerIdentity"`
	CanaryPlanSHA256         string `json:"canaryPlanSha256"`
	AuthorityPublicKeySHA256 string `json:"authorityPublicKeySha256"`
}

type childCanaryAuthorization struct {
	ControlPlane        bool `json:"controlPlane"`
	RootRuns            bool `json:"rootRuns"`
	ChildAgents         bool `json:"childAgents"`
	BrokerReadOnly      bool `json:"brokerReadOnly"`
	BrokerMutable       bool `json:"brokerMutable"`
	ArtifactPublication bool `json:"artifactPublication"`
	ProjectMutation     bool `json:"projectMutation"`
	Scheduler           bool `json:"scheduler"`
	SkillInstall        bool `json:"skillInstall"`
	Learning            bool `json:"learning"`
	Egress              bool `json:"egress"`
	Secrets             bool `json:"secrets"`
	Provider            bool `json:"provider"`
	MCPWrite            bool `json:"mcpWrite"`
}

type childCanaryCleanup struct {
	OrphanSandboxes int `json:"orphanSandboxes"`
	ScratchResidue  int `json:"scratchResidue"`
	PendingReaps    int `json:"pendingReaps"`
	Descendants     int `json:"descendants"`
	Credentials     int `json:"credentials"`
}

type childCanaryRecord struct {
	SchemaVersion string                   `json:"schemaVersion"`
	EvidenceClass string                   `json:"evidenceClass"`
	Stage         string                   `json:"stage"`
	Release       release                  `json:"release"`
	Target        target                   `json:"target"`
	Wiring        childCanaryWiring        `json:"wiring"`
	Window        window                   `json:"window"`
	Checks        []check                  `json:"checks"`
	Authorization childCanaryAuthorization `json:"authorization"`
	Cleanup       childCanaryCleanup       `json:"cleanup"`
	Review        review                   `json:"review"`
}

func VerifyChildCanary(config ChildCanaryConfig, now time.Time) (Decision, error) {
	if validateChildCanaryConfig(config) != nil || now.IsZero() {
		return Decision{ReasonCode: "CONFIG_INVALID"}, ErrInvalid
	}
	policyRaw, err := readRegular(config.PolicyFile, false)
	if err != nil {
		return Decision{ReasonCode: "POLICY_UNREADABLE"}, ErrInvalid
	}
	var policyDocument map[string]any
	if strictjson.Decode(policyRaw, maxDocument, &policyDocument) != nil ||
		policyDocument["schemaVersion"] != "neo.agent-production-policy/v1" ||
		policyDocument["migrationHead"] != float64(93) {
		return Decision{ReasonCode: "POLICY_INVALID"}, ErrInvalid
	}
	policyFingerprint := fingerprint(policyRaw)
	recordRaw, err := readRegular(config.RecordFile, true)
	if err != nil {
		return Decision{PolicyFingerprint: policyFingerprint, ReasonCode: "ACTIVATION_UNREADABLE"}, ErrInvalid
	}
	decision := Decision{PolicyFingerprint: policyFingerprint, EvidenceFingerprint: fingerprint(recordRaw)}
	var evidence childCanaryRecord
	if validateChildCanaryShape(recordRaw) != nil || strictjson.Decode(recordRaw, maxDocument, &evidence) != nil {
		decision.ReasonCode = "ACTIVATION_CHILD_INVALID"
		return decision, ErrInvalid
	}
	if reason := validateChildCanaryRecord(evidence, config, policyFingerprint, now.UTC()); reason != "" {
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
	for _, binding := range []struct{ path, expected, code string }{
		{config.ClientCertificateFile, evidence.Wiring.ClientCertificateSHA256, "CLIENT_CERTIFICATE_DRIFT"},
		{config.ServerCAFile, evidence.Wiring.ServerCASHA256, "SERVER_CA_DRIFT"},
		{config.CanaryPlanFile, evidence.Wiring.CanaryPlanSHA256, "CANARY_PLAN_DRIFT"},
		{config.AuthorityPublicKeyFile, evidence.Wiring.AuthorityPublicKeySHA256, "AUTHORITY_KEY_DRIFT"},
	} {
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
	decision.Ready, decision.ReasonCode = true, "DEPTH_ONE_CHILD_CANARY_GATES_PASSED"
	return decision, nil
}

func validateChildCanaryShape(raw []byte) error {
	root, err := rawObject(raw, "schemaVersion", "evidenceClass", "stage", "release", "target", "wiring",
		"window", "checks", "authorization", "cleanup", "review")
	if err != nil {
		return err
	}
	for key, fields := range map[string][]string{
		"release": {"gitCommit", "migrationHead", "runnerManifestSha256", "runnerBinarySha256", "operationsPolicySha256"},
		"target":  {"deploymentFingerprint", "runnerId"},
		"wiring": {"endpointSha256", "clientCertificateSha256", "serverCASha256", "serverName",
			"callerIdentity", "canaryPlanSha256", "authorityPublicKeySha256"},
		"window": {"startedAt", "completedAt", "expiresAt"},
		"authorization": {"controlPlane", "rootRuns", "childAgents", "brokerReadOnly", "brokerMutable",
			"artifactPublication", "projectMutation", "scheduler", "skillInstall", "learning", "egress",
			"secrets", "provider", "mcpWrite"},
		"cleanup": {"orphanSandboxes", "scratchResidue", "pendingReaps", "descendants", "credentials"},
		"review":  {"decision", "reviewedAt", "reviewerFingerprint"},
	} {
		if _, objectErr := rawObject(root[key], fields...); objectErr != nil {
			return objectErr
		}
	}
	var checks []any
	if strictjson.Decode(root["checks"], maxDocument, &checks) != nil || len(checks) != len(childCanaryChecks) {
		return ErrInvalid
	}
	return nil
}

func validateChildCanaryRecord(value childCanaryRecord, config ChildCanaryConfig,
	policyFingerprint string, now time.Time,
) string {
	if value.SchemaVersion != SchemaVersion || value.Stage != StageChildCanary ||
		!member(value.EvidenceClass, "template", "production") {
		return "ACTIVATION_VERSION_INVALID"
	}
	if !commitPattern.MatchString(value.Release.GitCommit) || value.Release.MigrationHead != 93 ||
		!validFingerprint(value.Release.RunnerManifestSHA256) || !validFingerprint(value.Release.RunnerBinarySHA256) ||
		!validFingerprint(value.Release.OperationsPolicySHA256) ||
		value.Release.OperationsPolicySHA256 != policyFingerprint {
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
		value.Wiring.ServerCASHA256, value.Wiring.CanaryPlanSHA256, value.Wiring.AuthorityPublicKeySHA256}
	if !validFingerprint(value.Wiring.EndpointSHA256) || !validFingerprint(value.Wiring.ClientCertificateSHA256) ||
		!validFingerprint(value.Wiring.ServerCASHA256) || !validFingerprint(value.Wiring.CanaryPlanSHA256) ||
		!validFingerprint(value.Wiring.AuthorityPublicKeySHA256) ||
		value.Wiring.EndpointSHA256 != EndpointFingerprint(config.Endpoint) ||
		value.Wiring.ServerName != config.ServerName || value.Wiring.CallerIdentity != config.CallerIdentity {
		return "WIRING_INVALID"
	}
	if value.Window.StartedAt.IsZero() || value.Window.CompletedAt.IsZero() || value.Window.ExpiresAt.IsZero() ||
		value.Window.CompletedAt.Before(value.Window.StartedAt) || !value.Window.ExpiresAt.After(value.Window.CompletedAt) ||
		value.Window.ExpiresAt.Sub(value.Window.StartedAt) > 24*time.Hour {
		return "WINDOW_INVALID"
	}
	results := make(map[string]string, len(value.Checks))
	for _, item := range value.Checks {
		if !contains(childCanaryChecks, item.ID) || results[item.ID] != "" ||
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
	if !sameStrings(ids, childCanaryChecks) {
		return "CHECK_SET_INCOMPLETE"
	}
	authorization := value.Authorization
	if authorization.ControlPlane || !authorization.RootRuns || !authorization.ChildAgents ||
		authorization.BrokerReadOnly || authorization.BrokerMutable || authorization.ArtifactPublication ||
		authorization.ProjectMutation || authorization.Scheduler || authorization.SkillInstall ||
		authorization.Learning || authorization.Egress || authorization.Secrets ||
		authorization.Provider || authorization.MCPWrite {
		return "AUTHORIZATION_WIDENED"
	}
	cleanup := value.Cleanup
	if cleanup.OrphanSandboxes < 0 || cleanup.ScratchResidue < 0 || cleanup.PendingReaps < 0 ||
		cleanup.Descendants < 0 || cleanup.Credentials < 0 || cleanup.OrphanSandboxes > 1_000_000 ||
		cleanup.ScratchResidue > 1_000_000 || cleanup.PendingReaps > 1_000_000 ||
		cleanup.Descendants > 1_000_000 || cleanup.Credentials > 1_000_000 {
		return "CLEANUP_INVALID"
	}
	if !member(value.Review.Decision, "approved", "held") ||
		value.Review.ReviewedAt.Before(value.Window.CompletedAt) ||
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
	if cleanup.OrphanSandboxes != 0 || cleanup.ScratchResidue != 0 || cleanup.PendingReaps != 0 ||
		cleanup.Descendants != 0 || cleanup.Credentials != 0 {
		return "RUNTIME_RESIDUE_REMAINS"
	}
	if value.Review.Decision != "approved" {
		return "REVIEW_HELD"
	}
	return ""
}

func validateChildCanaryConfig(config ChildCanaryConfig) error {
	for _, path := range []string{config.PolicyFile, config.RecordFile, config.ReleaseManifestFile,
		config.ClientCertificateFile, config.ServerCAFile, config.CanaryPlanFile,
		config.AuthorityPublicKeyFile} {
		if len(path) < 2 || path[0] != '/' {
			return ErrInvalid
		}
	}
	if config.Endpoint == "" || !identityPattern.MatchString(config.RunnerID) ||
		!identityPattern.MatchString(config.ServerName) || config.CallerIdentity != ChildCanaryCallerIdentity ||
		!commitPattern.MatchString(config.ReleaseCommit) {
		return ErrInvalid
	}
	return nil
}
