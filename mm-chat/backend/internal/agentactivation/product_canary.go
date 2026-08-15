package agentactivation

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrootcanary"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const StageProductCanary = "product_canary"

var productActivationPattern = regexp.MustCompile(`^activation_[a-z0-9]{16,64}$`)

var productCanaryChecks = []string{
	"crash_restart_recovery",
	"exact_host_isolation",
	"fixed_plan_ready",
	"prior_activation_chain",
	"private_mtls_canary",
	"product_request_authority",
	"worker_least_privilege",
	"zero_residue",
}

type ProductCanaryConfig struct {
	Config
	ActivationID           string
	CanaryPlanFile         string
	AuthorityPublicKeyFile string
}

type productCanaryWiring struct {
	ActivationID             string `json:"activationId"`
	EndpointSHA256           string `json:"endpointSha256"`
	ClientCertificateSHA256  string `json:"clientCertificateSha256"`
	ServerCASHA256           string `json:"serverCASha256"`
	ServerName               string `json:"serverName"`
	CallerIdentity           string `json:"callerIdentity"`
	CanaryPlanSHA256         string `json:"canaryPlanSha256"`
	AuthorityPublicKeySHA256 string `json:"authorityPublicKeySha256"`
}

type activationChain struct {
	ControlPlane    string `json:"controlPlane"`
	RootRun         string `json:"rootRun"`
	BrokerArtifact  string `json:"brokerArtifact"`
	ProjectMutation string `json:"projectMutation"`
	DepthOneChild   string `json:"depthOneChild"`
	CronWorker      string `json:"cronWorker"`
	DraftLearning   string `json:"draftLearning"`
}

type productCanaryAuthorization struct {
	ProductCanary  bool `json:"productCanary"`
	GenericRuntime bool `json:"genericRuntime"`
	BrokerEffects  bool `json:"brokerEffects"`
	Delegation     bool `json:"delegation"`
	Scheduler      bool `json:"scheduler"`
	Learning       bool `json:"learning"`
	Egress         bool `json:"egress"`
	Secrets        bool `json:"secrets"`
}

type productCanaryCleanup struct {
	QueuedRequests  int `json:"queuedRequests"`
	ClaimedRequests int `json:"claimedRequests"`
	OrphanSandboxes int `json:"orphanSandboxes"`
	ScratchResidue  int `json:"scratchResidue"`
}

type productCanaryRecord struct {
	SchemaVersion string                     `json:"schemaVersion"`
	EvidenceClass string                     `json:"evidenceClass"`
	Stage         string                     `json:"stage"`
	Release       release                    `json:"release"`
	Target        target                     `json:"target"`
	Wiring        productCanaryWiring        `json:"wiring"`
	Prerequisites activationChain            `json:"prerequisites"`
	Window        window                     `json:"window"`
	Checks        []check                    `json:"checks"`
	Authorization productCanaryAuthorization `json:"authorization"`
	Cleanup       productCanaryCleanup       `json:"cleanup"`
	Review        review                     `json:"review"`
}

func VerifyProductCanary(config ProductCanaryConfig, now time.Time) (Decision, error) {
	if validateProductCanaryConfig(config) != nil || now.IsZero() {
		return Decision{ReasonCode: "CONFIG_INVALID"}, ErrInvalid
	}
	policyRaw, err := readRegular(config.PolicyFile, false)
	if err != nil {
		return Decision{ReasonCode: "POLICY_UNREADABLE"}, ErrInvalid
	}
	var policy map[string]any
	if strictjson.Decode(policyRaw, maxDocument, &policy) != nil ||
		policy["schemaVersion"] != "neo.agent-production-policy/v1" || policy["migrationHead"] != float64(95) {
		return Decision{ReasonCode: "POLICY_INVALID"}, ErrInvalid
	}
	policyFingerprint := fingerprint(policyRaw)
	recordRaw, err := readRegular(config.RecordFile, true)
	if err != nil {
		return Decision{PolicyFingerprint: policyFingerprint, ReasonCode: "ACTIVATION_UNREADABLE"}, ErrInvalid
	}
	decision := Decision{PolicyFingerprint: policyFingerprint, EvidenceFingerprint: fingerprint(recordRaw)}
	var evidence productCanaryRecord
	if validateProductCanaryShape(recordRaw) != nil ||
		strictjson.Decode(recordRaw, maxDocument, &evidence) != nil {
		decision.ReasonCode = "ACTIVATION_ROOT_INVALID"
		return decision, ErrInvalid
	}
	if reason := validateProductCanaryRecord(evidence, config, policyFingerprint, now.UTC()); reason != "" {
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
	manifest, parseErr := agentrunner.ParseReleaseManifest(manifestRaw)
	if decision.ManifestFingerprint != evidence.Release.RunnerManifestSHA256 || parseErr != nil ||
		!manifest.Approved || manifest.RunnerID != config.RunnerID || manifestHasPlaceholder(manifest) {
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
	decision.Ready, decision.ReasonCode = true, "PRODUCT_CANARY_GATES_PASSED"
	return decision, nil
}

func validateProductCanaryShape(raw []byte) error {
	root, err := rawObject(raw, "schemaVersion", "evidenceClass", "stage", "release", "target",
		"wiring", "prerequisites", "window", "checks", "authorization", "cleanup", "review")
	if err != nil {
		return err
	}
	for key, fields := range map[string][]string{
		"release":       {"gitCommit", "migrationHead", "runnerManifestSha256", "runnerBinarySha256", "operationsPolicySha256"},
		"target":        {"deploymentFingerprint", "runnerId"},
		"wiring":        {"activationId", "endpointSha256", "clientCertificateSha256", "serverCASha256", "serverName", "callerIdentity", "canaryPlanSha256", "authorityPublicKeySha256"},
		"prerequisites": {"controlPlane", "rootRun", "brokerArtifact", "projectMutation", "depthOneChild", "cronWorker", "draftLearning"},
		"window":        {"startedAt", "completedAt", "expiresAt"},
		"authorization": {"productCanary", "genericRuntime", "brokerEffects", "delegation", "scheduler", "learning", "egress", "secrets"},
		"cleanup":       {"queuedRequests", "claimedRequests", "orphanSandboxes", "scratchResidue"},
		"review":        {"decision", "reviewedAt", "reviewerFingerprint"},
	} {
		if _, objectErr := rawObject(root[key], fields...); objectErr != nil {
			return objectErr
		}
	}
	var checks []any
	if strictjson.Decode(root["checks"], maxDocument, &checks) != nil || len(checks) != len(productCanaryChecks) {
		return ErrInvalid
	}
	return nil
}

func validateProductCanaryRecord(value productCanaryRecord, config ProductCanaryConfig,
	policyFingerprint string, now time.Time,
) string {
	if value.SchemaVersion != SchemaVersion || value.Stage != StageProductCanary ||
		!member(value.EvidenceClass, "template", "production") {
		return "ACTIVATION_VERSION_INVALID"
	}
	if !commitPattern.MatchString(value.Release.GitCommit) || value.Release.MigrationHead != 95 ||
		value.Release.GitCommit != config.ReleaseCommit ||
		!validFingerprint(value.Release.RunnerManifestSHA256) ||
		!validFingerprint(value.Release.RunnerBinarySHA256) ||
		value.Release.OperationsPolicySHA256 != policyFingerprint {
		return "RELEASE_INVALID"
	}
	if value.Target.RunnerID != config.RunnerID || !validFingerprint(value.Target.DeploymentFingerprint) {
		return "TARGET_INVALID"
	}
	if value.Wiring.ActivationID != config.ActivationID ||
		value.Wiring.CallerIdentity != agentrootcanary.ProductCallerIdentity ||
		value.Wiring.CallerIdentity != config.CallerIdentity || value.Wiring.ServerName != config.ServerName ||
		value.Wiring.EndpointSHA256 != EndpointFingerprint(config.Endpoint) {
		return "WIRING_INVALID"
	}
	prerequisiteFingerprints := []string{value.Prerequisites.ControlPlane, value.Prerequisites.RootRun,
		value.Prerequisites.BrokerArtifact, value.Prerequisites.ProjectMutation,
		value.Prerequisites.DepthOneChild, value.Prerequisites.CronWorker,
		value.Prerequisites.DraftLearning}
	fingerprints := []string{value.Release.RunnerManifestSHA256, value.Release.RunnerBinarySHA256,
		value.Target.DeploymentFingerprint, value.Wiring.EndpointSHA256,
		value.Wiring.ClientCertificateSHA256, value.Wiring.ServerCASHA256,
		value.Wiring.CanaryPlanSHA256, value.Wiring.AuthorityPublicKeySHA256}
	fingerprints = append(fingerprints, prerequisiteFingerprints...)
	for _, item := range fingerprints {
		if !validFingerprint(item) {
			return "BINDING_INVALID"
		}
	}
	if value.Window.StartedAt.IsZero() || value.Window.CompletedAt.Before(value.Window.StartedAt) ||
		!value.Window.ExpiresAt.After(value.Window.CompletedAt) ||
		value.Window.ExpiresAt.Sub(value.Window.StartedAt) > 24*time.Hour {
		return "WINDOW_INVALID"
	}
	results := make(map[string]string, len(value.Checks))
	for _, item := range value.Checks {
		if !contains(productCanaryChecks, item.ID) || results[item.ID] != "" ||
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
	if !sameStrings(ids, productCanaryChecks) {
		return "CHECK_SET_INCOMPLETE"
	}
	authorization := value.Authorization
	if !authorization.ProductCanary || authorization.GenericRuntime || authorization.BrokerEffects ||
		authorization.Delegation || authorization.Scheduler || authorization.Learning ||
		authorization.Egress || authorization.Secrets {
		return "AUTHORIZATION_WIDENED"
	}
	cleanup := value.Cleanup
	if cleanup.QueuedRequests < 0 || cleanup.ClaimedRequests < 0 || cleanup.OrphanSandboxes < 0 ||
		cleanup.ScratchResidue < 0 || cleanup.QueuedRequests > 1_000_000 ||
		cleanup.ClaimedRequests > 1_000_000 || cleanup.OrphanSandboxes > 1_000_000 ||
		cleanup.ScratchResidue > 1_000_000 {
		return "CLEANUP_INVALID"
	}
	if !member(value.Review.Decision, "approved", "held") ||
		value.Review.ReviewedAt.Before(value.Window.CompletedAt) ||
		!value.Review.ReviewedAt.Before(value.Window.ExpiresAt) || value.Review.ReviewedAt.After(now) ||
		!validFingerprint(value.Review.ReviewerFingerprint) {
		return "REVIEW_INVALID"
	}
	if results["exact_host_isolation"] == "isolation_unavailable" {
		return "ISOLATION_UNAVAILABLE"
	}
	if value.EvidenceClass != "production" {
		return "NON_PRODUCTION_EVIDENCE"
	}
	if now.Before(value.Window.StartedAt) || !now.Before(value.Window.ExpiresAt) ||
		value.Window.CompletedAt.After(now) {
		return "EVIDENCE_STALE"
	}
	if value.Release.GitCommit == strings.Repeat("0", 40) {
		return "PLACEHOLDER_BINDING_FORBIDDEN"
	}
	for _, item := range fingerprints {
		if item == "sha256:"+strings.Repeat("0", 64) {
			return "PLACEHOLDER_BINDING_FORBIDDEN"
		}
	}
	seenPrerequisites := make(map[string]struct{}, len(prerequisiteFingerprints))
	for _, item := range prerequisiteFingerprints {
		seenPrerequisites[item] = struct{}{}
	}
	if len(seenPrerequisites) != len(prerequisiteFingerprints) {
		return "PREREQUISITE_BINDING_INVALID"
	}
	for _, item := range value.Checks {
		if item.Result != "passed" {
			return "LIVE_CHECK_NOT_PASSED"
		}
	}
	if cleanup.QueuedRequests != 0 || cleanup.ClaimedRequests != 0 ||
		cleanup.OrphanSandboxes != 0 || cleanup.ScratchResidue != 0 {
		return "RUNTIME_RESIDUE_REMAINS"
	}
	if value.Review.Decision != "approved" {
		return "REVIEW_HELD"
	}
	return ""
}

func validateProductCanaryConfig(config ProductCanaryConfig) error {
	for _, path := range []string{config.PolicyFile, config.RecordFile, config.ReleaseManifestFile,
		config.ClientCertificateFile, config.ServerCAFile, config.CanaryPlanFile,
		config.AuthorityPublicKeyFile} {
		if len(path) < 2 || path[0] != '/' {
			return ErrInvalid
		}
	}
	if !productActivationPattern.MatchString(config.ActivationID) || !identityPattern.MatchString(config.RunnerID) ||
		!identityPattern.MatchString(config.ServerName) || config.CallerIdentity != agentrootcanary.ProductCallerIdentity ||
		!commitPattern.MatchString(config.ReleaseCommit) || config.Endpoint == "" {
		return ErrInvalid
	}
	return nil
}
