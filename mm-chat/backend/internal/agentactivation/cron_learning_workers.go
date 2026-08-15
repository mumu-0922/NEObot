package agentactivation

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	StageCronWorker          = "cron_worker"
	StageDraftLearningWorker = "draft_learning_worker"
	DraftLearningCaller      = "spiffe://neo-chat/agent-runtime-draft-learning"
	workerZeroFingerprint    = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

var workerPrerequisites = []string{
	"broker_artifact_canary", "child_run_canary", "control_plane",
	"project_mutation_canary", "root_run_canary",
}

var cronWorkerChecks = []string{
	"broker_artifact_canary_ready", "child_run_canary_ready", "control_plane_ready",
	"crash_restart_ready", "credential_absence", "exact_host_isolation",
	"exact_target_plan", "least_privilege_login", "project_mutation_canary_ready",
	"root_run_canary_ready",
}

var draftLearningWorkerChecks = []string{
	"broker_artifact_canary_ready", "child_run_canary_ready", "cleanup_recovery_ready",
	"control_plane_ready", "crash_restart_ready", "exact_host_isolation",
	"exact_target_plan", "human_promote_separate", "least_privilege_login",
	"object_store_separation", "project_mutation_canary_ready", "root_run_canary_ready",
	"runner_result_acl", "rootless_isolation_evaluation",
}

type WorkerConfig struct {
	PolicyFile        string
	RecordFile        string
	PlanFile          string
	ReleaseCommit     string
	PrerequisiteFiles map[string]string
}

type DraftLearningWorkerConfig struct {
	WorkerConfig
	ReleaseManifestFile      string
	ClientCertificateFile    string
	ServerCAFile             string
	AuthorityPublicKeyFile   string
	ObjectCredentialMetaFile string
	Endpoint                 string
	RunnerID                 string
	ServerName               string
	CallerIdentity           string
}

type workerPrerequisite struct {
	Stage          string `json:"stage"`
	EvidenceSHA256 string `json:"evidenceSha256"`
}

type cronWorkerTarget struct {
	ActivationID        string `json:"activationId"`
	TemplateID          string `json:"templateId"`
	TemplateRevision    int64  `json:"templateRevision"`
	TemplateFingerprint string `json:"templateFingerprint"`
	PlanSHA256          string `json:"planSha256"`
}

type cronWorkerAuthorization struct {
	ExactCronTarget  bool `json:"exactCronTarget"`
	Runtime          bool `json:"runtime"`
	GenericScheduler bool `json:"genericScheduler"`
	Learning         bool `json:"learning"`
	SkillInstall     bool `json:"skillInstall"`
	BrokerMutation   bool `json:"brokerMutation"`
	Delegation       bool `json:"delegation"`
	RunnerCredential bool `json:"runnerCredential"`
	ObjectCredential bool `json:"objectCredential"`
}

type cronWorkerCleanup struct {
	StaleClaims     int `json:"staleClaims"`
	PendingTriggers int `json:"pendingTriggers"`
}

type cronWorkerRecord struct {
	SchemaVersion string                  `json:"schemaVersion"`
	EvidenceClass string                  `json:"evidenceClass"`
	Stage         string                  `json:"stage"`
	Release       release                 `json:"release"`
	Prerequisites []workerPrerequisite    `json:"prerequisites"`
	Target        cronWorkerTarget        `json:"target"`
	Window        window                  `json:"window"`
	Checks        []check                 `json:"checks"`
	Authorization cronWorkerAuthorization `json:"authorization"`
	Cleanup       cronWorkerCleanup       `json:"cleanup"`
	Review        review                  `json:"review"`
}

type draftLearningTarget struct {
	ActivationID         string `json:"activationId"`
	DraftID              string `json:"draftId"`
	DraftFingerprint     string `json:"draftFingerprint"`
	PackageFingerprint   string `json:"packageFingerprint"`
	RuntimeFingerprint   string `json:"runtimeFingerprint"`
	ArchiveFingerprint   string `json:"archiveFingerprint"`
	WorkspaceFingerprint string `json:"workspaceFingerprint"`
	PlanSHA256           string `json:"planSha256"`
}

type draftLearningWiring struct {
	EndpointSHA256             string `json:"endpointSha256"`
	ClientCertificateSHA256    string `json:"clientCertificateSha256"`
	ServerCASHA256             string `json:"serverCASha256"`
	ServerName                 string `json:"serverName"`
	CallerIdentity             string `json:"callerIdentity"`
	AuthorityPublicKeySHA256   string `json:"authorityPublicKeySha256"`
	ObjectCredentialMetaSHA256 string `json:"objectCredentialMetaSha256"`
}

type draftLearningAuthorization struct {
	DraftChecks             bool `json:"draftChecks"`
	Cleanup                 bool `json:"cleanup"`
	RunnerLifecycle         bool `json:"runnerLifecycle"`
	Runtime                 bool `json:"runtime"`
	GenericScheduler        bool `json:"genericScheduler"`
	GenericLearning         bool `json:"genericLearning"`
	SkillInstall            bool `json:"skillInstall"`
	BrokerMutation          bool `json:"brokerMutation"`
	Delegation              bool `json:"delegation"`
	Promote                 bool `json:"promote"`
	AdministratorCredential bool `json:"administratorCredential"`
}

type draftLearningCleanup struct {
	OrphanSandboxes     int `json:"orphanSandboxes"`
	PendingRunnerChecks int `json:"pendingRunnerChecks"`
	PendingObjects      int `json:"pendingObjects"`
}

type humanDecision struct {
	DecisionID                 string `json:"decisionId"`
	ActorClass                 string `json:"actorClass"`
	PromotedPackageFingerprint string `json:"promotedPackageFingerprint"`
}

type draftLearningWorkerRecord struct {
	SchemaVersion string                     `json:"schemaVersion"`
	EvidenceClass string                     `json:"evidenceClass"`
	Stage         string                     `json:"stage"`
	Release       release                    `json:"release"`
	Prerequisites []workerPrerequisite       `json:"prerequisites"`
	Target        draftLearningTarget        `json:"target"`
	Wiring        draftLearningWiring        `json:"wiring"`
	Window        window                     `json:"window"`
	Checks        []check                    `json:"checks"`
	Authorization draftLearningAuthorization `json:"authorization"`
	Cleanup       draftLearningCleanup       `json:"cleanup"`
	HumanDecision humanDecision              `json:"humanDecision"`
	Review        review                     `json:"review"`
}

func VerifyCronWorker(config WorkerConfig, now time.Time) (Decision, error) {
	policyRaw, policyFingerprint, recordRaw, decision, err := readWorkerInputs(config, now)
	if err != nil {
		return decision, err
	}
	_ = policyRaw
	var evidence cronWorkerRecord
	if validateCronWorkerShape(recordRaw) != nil || strictjson.Decode(recordRaw, maxDocument, &evidence) != nil {
		decision.ReasonCode = "ACTIVATION_ROOT_INVALID"
		return decision, ErrInvalid
	}
	if reason := validateCronWorkerRecord(evidence, config, policyFingerprint, now.UTC()); reason != "" {
		decision.ReasonCode = reason
		return heldOrInvalid(decision, reason)
	}
	if !workerBindings(config, evidence.Prerequisites, evidence.Target.PlanSHA256) {
		decision.ReasonCode = "WORKER_BINDING_DRIFT"
		return decision, ErrInvalid
	}
	decision.Ready, decision.ReasonCode = true, "CRON_WORKER_GATES_PASSED"
	return decision, nil
}

func VerifyDraftLearningWorker(config DraftLearningWorkerConfig, now time.Time) (Decision, error) {
	if validateDraftLearningWorkerConfig(config) != nil {
		return Decision{ReasonCode: "CONFIG_INVALID"}, ErrInvalid
	}
	_, policyFingerprint, recordRaw, decision, err := readWorkerInputs(config.WorkerConfig, now)
	if err != nil {
		return decision, err
	}
	var evidence draftLearningWorkerRecord
	if validateDraftLearningWorkerShape(recordRaw) != nil ||
		strictjson.Decode(recordRaw, maxDocument, &evidence) != nil {
		decision.ReasonCode = "ACTIVATION_ROOT_INVALID"
		return decision, ErrInvalid
	}
	if reason := validateDraftLearningWorkerRecord(evidence, config, policyFingerprint, now.UTC()); reason != "" {
		decision.ReasonCode = reason
		return heldOrInvalid(decision, reason)
	}
	if !workerBindings(config.WorkerConfig, evidence.Prerequisites, evidence.Target.PlanSHA256) {
		decision.ReasonCode = "WORKER_BINDING_DRIFT"
		return decision, ErrInvalid
	}
	manifestRaw, readErr := readRegular(config.ReleaseManifestFile, true)
	if readErr != nil || fingerprint(manifestRaw) != evidence.Release.RunnerManifestSHA256 {
		decision.ReasonCode = "RUNNER_MANIFEST_DRIFT"
		return decision, ErrInvalid
	}
	manifest, parseErr := agentrunner.ParseReleaseManifest(manifestRaw)
	if parseErr != nil || !manifest.Approved || manifest.RunnerID != config.RunnerID || manifestHasPlaceholder(manifest) {
		decision.ReasonCode = "RUNNER_MANIFEST_INVALID"
		return decision, ErrInvalid
	}
	for _, binding := range []struct{ path, expected string }{
		{config.ClientCertificateFile, evidence.Wiring.ClientCertificateSHA256},
		{config.ServerCAFile, evidence.Wiring.ServerCASHA256},
		{config.AuthorityPublicKeyFile, evidence.Wiring.AuthorityPublicKeySHA256},
		{config.ObjectCredentialMetaFile, evidence.Wiring.ObjectCredentialMetaSHA256},
	} {
		raw, readErr := readRegular(binding.path, true)
		if readErr != nil || fingerprint(raw) != binding.expected {
			decision.ReasonCode = "WORKER_BINDING_DRIFT"
			return decision, ErrInvalid
		}
	}
	if _, err := agentrunner.LoadEd25519PublicKey(config.AuthorityPublicKeyFile); err != nil {
		decision.ReasonCode = "AUTHORITY_KEY_INVALID"
		return decision, ErrInvalid
	}
	decision.ManifestFingerprint = fingerprint(manifestRaw)
	decision.Ready, decision.ReasonCode = true, "DRAFT_LEARNING_WORKER_GATES_PASSED"
	return decision, nil
}

func readWorkerInputs(config WorkerConfig, now time.Time) ([]byte, string, []byte, Decision, error) {
	if validateWorkerConfig(config) != nil || now.IsZero() {
		return nil, "", nil, Decision{ReasonCode: "CONFIG_INVALID"}, ErrInvalid
	}
	policyRaw, err := readRegular(config.PolicyFile, false)
	if err != nil {
		return nil, "", nil, Decision{ReasonCode: "POLICY_UNREADABLE"}, ErrInvalid
	}
	var policy map[string]any
	if strictjson.Decode(policyRaw, maxDocument, &policy) != nil ||
		policy["schemaVersion"] != "neo.agent-production-policy/v1" ||
		policy["migrationHead"] != float64(94) {
		return nil, "", nil, Decision{ReasonCode: "POLICY_INVALID"}, ErrInvalid
	}
	policyFingerprint := fingerprint(policyRaw)
	recordRaw, err := readRegular(config.RecordFile, true)
	decision := Decision{PolicyFingerprint: policyFingerprint}
	if err != nil {
		decision.ReasonCode = "ACTIVATION_UNREADABLE"
		return nil, policyFingerprint, nil, decision, ErrInvalid
	}
	decision.EvidenceFingerprint = fingerprint(recordRaw)
	return policyRaw, policyFingerprint, recordRaw, decision, nil
}

func validateCronWorkerRecord(value cronWorkerRecord, config WorkerConfig,
	policyFingerprint string, now time.Time,
) string {
	if reason := validateWorkerBase(value.SchemaVersion, value.EvidenceClass, value.Stage,
		StageCronWorker, value.Release, value.Prerequisites, value.Window, value.Checks,
		value.Review, cronWorkerChecks, config.ReleaseCommit, policyFingerprint, now); reason != "" {
		return reason
	}
	if !identityPattern.MatchString(value.Target.ActivationID) ||
		!identityPattern.MatchString(value.Target.TemplateID) || value.Target.TemplateRevision < 1 ||
		!validFingerprint(value.Target.TemplateFingerprint) || !validFingerprint(value.Target.PlanSHA256) {
		return "TARGET_INVALID"
	}
	a := value.Authorization
	if !a.ExactCronTarget || a.Runtime || a.GenericScheduler || a.Learning || a.SkillInstall ||
		a.BrokerMutation || a.Delegation || a.RunnerCredential || a.ObjectCredential {
		return "AUTHORIZATION_WIDENED"
	}
	if value.Cleanup.StaleClaims != 0 || value.Cleanup.PendingTriggers != 0 {
		return "RUNTIME_RESIDUE_REMAINS"
	}
	return ""
}

func validateDraftLearningWorkerRecord(value draftLearningWorkerRecord,
	config DraftLearningWorkerConfig, policyFingerprint string, now time.Time,
) string {
	if reason := validateWorkerBase(value.SchemaVersion, value.EvidenceClass, value.Stage,
		StageDraftLearningWorker, value.Release, value.Prerequisites, value.Window, value.Checks,
		value.Review, draftLearningWorkerChecks, config.ReleaseCommit, policyFingerprint, now); reason != "" {
		return reason
	}
	t := value.Target
	if !identityPattern.MatchString(t.ActivationID) || !identityPattern.MatchString(t.DraftID) ||
		!validFingerprint(t.DraftFingerprint) || !validFingerprint(t.PackageFingerprint) ||
		!validFingerprint(t.RuntimeFingerprint) || !validFingerprint(t.ArchiveFingerprint) ||
		!validFingerprint(t.WorkspaceFingerprint) || !validFingerprint(t.PlanSHA256) {
		return "TARGET_INVALID"
	}
	w := value.Wiring
	if !validFingerprint(w.EndpointSHA256) || !validFingerprint(w.ClientCertificateSHA256) ||
		!validFingerprint(w.ServerCASHA256) || !validFingerprint(w.AuthorityPublicKeySHA256) ||
		!validFingerprint(w.ObjectCredentialMetaSHA256) ||
		w.EndpointSHA256 != EndpointFingerprint(config.Endpoint) || w.ServerName != config.ServerName ||
		w.CallerIdentity != DraftLearningCaller || config.CallerIdentity != DraftLearningCaller {
		return "WIRING_INVALID"
	}
	a := value.Authorization
	if !a.DraftChecks || !a.Cleanup || !a.RunnerLifecycle || a.Runtime || a.GenericScheduler ||
		a.GenericLearning || a.SkillInstall || a.BrokerMutation || a.Delegation || a.Promote ||
		a.AdministratorCredential {
		return "AUTHORIZATION_WIDENED"
	}
	if value.Cleanup.OrphanSandboxes != 0 || value.Cleanup.PendingRunnerChecks != 0 ||
		value.Cleanup.PendingObjects != 0 {
		return "RUNTIME_RESIDUE_REMAINS"
	}
	if !identityPattern.MatchString(value.HumanDecision.DecisionID) ||
		value.HumanDecision.ActorClass != "human_operator" ||
		!validFingerprint(value.HumanDecision.PromotedPackageFingerprint) {
		return "HUMAN_DECISION_INVALID"
	}
	return ""
}

func validateWorkerBase(schema, evidenceClass, stage, expectedStage string, releaseValue release,
	prerequisites []workerPrerequisite, windowValue window, checks []check, reviewValue review,
	required []string, releaseCommit, policyFingerprint string, now time.Time,
) string {
	if schema != SchemaVersion || stage != expectedStage || !member(evidenceClass, "template", "production") {
		return "ACTIVATION_VERSION_INVALID"
	}
	if !commitPattern.MatchString(releaseValue.GitCommit) || releaseValue.MigrationHead != 94 ||
		releaseValue.GitCommit != releaseCommit ||
		!validFingerprint(releaseValue.RunnerManifestSHA256) ||
		!validFingerprint(releaseValue.RunnerBinarySHA256) ||
		!validFingerprint(releaseValue.OperationsPolicySHA256) ||
		releaseValue.OperationsPolicySHA256 != policyFingerprint {
		return "RELEASE_INVALID"
	}
	if !validPrerequisites(prerequisites) {
		return "PREREQUISITE_INVALID"
	}
	if windowValue.StartedAt.IsZero() || windowValue.CompletedAt.IsZero() || windowValue.ExpiresAt.IsZero() ||
		windowValue.CompletedAt.Before(windowValue.StartedAt) ||
		!windowValue.ExpiresAt.After(windowValue.CompletedAt) ||
		windowValue.ExpiresAt.Sub(windowValue.StartedAt) > 24*time.Hour {
		return "WINDOW_INVALID"
	}
	results := map[string]string{}
	for _, item := range checks {
		if !contains(required, item.ID) || results[item.ID] != "" ||
			!member(item.Result, "passed", "failed", "not_run", "isolation_unavailable") ||
			item.ObservedAt.Before(windowValue.StartedAt) || item.ObservedAt.After(windowValue.CompletedAt) ||
			!validFingerprint(item.EvidenceSHA256) || !detailPattern.MatchString(item.DetailCode) {
			return "CHECK_SET_INVALID"
		}
		results[item.ID] = item.Result
	}
	ids := make([]string, 0, len(results))
	for id := range results {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	expected := append([]string(nil), required...)
	sort.Strings(expected)
	if !sameStrings(ids, expected) {
		return "CHECK_SET_INCOMPLETE"
	}
	if !member(reviewValue.Decision, "approved", "held") ||
		reviewValue.ReviewedAt.Before(windowValue.CompletedAt) ||
		!reviewValue.ReviewedAt.Before(windowValue.ExpiresAt) || reviewValue.ReviewedAt.After(now) ||
		!validFingerprint(reviewValue.ReviewerFingerprint) {
		return "REVIEW_INVALID"
	}
	if results["exact_host_isolation"] == "isolation_unavailable" {
		return "ISOLATION_UNAVAILABLE"
	}
	if evidenceClass != "production" {
		return "NON_PRODUCTION_EVIDENCE"
	}
	if now.Before(windowValue.StartedAt) || !now.Before(windowValue.ExpiresAt) || windowValue.CompletedAt.After(now) {
		return "EVIDENCE_STALE"
	}
	if releaseValue.GitCommit == strings.Repeat("0", 40) ||
		releaseValue.RunnerManifestSHA256 == workerZeroFingerprint ||
		releaseValue.RunnerBinarySHA256 == workerZeroFingerprint ||
		releaseValue.OperationsPolicySHA256 == workerZeroFingerprint ||
		reviewValue.ReviewerFingerprint == workerZeroFingerprint {
		return "PLACEHOLDER_BINDING_FORBIDDEN"
	}
	for _, prerequisite := range prerequisites {
		if prerequisite.EvidenceSHA256 == workerZeroFingerprint {
			return "PLACEHOLDER_BINDING_FORBIDDEN"
		}
	}
	for _, item := range checks {
		if item.EvidenceSHA256 == workerZeroFingerprint {
			return "PLACEHOLDER_BINDING_FORBIDDEN"
		}
	}
	for _, result := range results {
		if result != "passed" {
			return "LIVE_CHECK_NOT_PASSED"
		}
	}
	if reviewValue.Decision != "approved" {
		return "REVIEW_HELD"
	}
	return ""
}

func validPrerequisites(values []workerPrerequisite) bool {
	stages := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if !contains(workerPrerequisites, value.Stage) || !validFingerprint(value.EvidenceSHA256) {
			return false
		}
		if _, duplicate := seen[value.Stage]; duplicate {
			return false
		}
		seen[value.Stage] = struct{}{}
		stages = append(stages, value.Stage)
	}
	sort.Strings(stages)
	return sameStrings(stages, workerPrerequisites)
}

func workerBindings(config WorkerConfig, prerequisites []workerPrerequisite, planSHA string) bool {
	plan, err := readRegular(config.PlanFile, true)
	if err != nil || fingerprint(plan) != planSHA {
		return false
	}
	expected := map[string]string{}
	for _, item := range prerequisites {
		expected[item.Stage] = item.EvidenceSHA256
	}
	for _, stage := range workerPrerequisites {
		path := config.PrerequisiteFiles[stage]
		raw, readErr := readRegular(path, true)
		if readErr != nil || fingerprint(raw) != expected[stage] {
			return false
		}
	}
	return true
}

func validateWorkerConfig(config WorkerConfig) error {
	if !commitPattern.MatchString(config.ReleaseCommit) || len(config.PrerequisiteFiles) != len(workerPrerequisites) {
		return ErrInvalid
	}
	for _, path := range []string{config.PolicyFile, config.RecordFile, config.PlanFile} {
		if len(path) < 2 || path[0] != '/' {
			return ErrInvalid
		}
	}
	for _, stage := range workerPrerequisites {
		path := config.PrerequisiteFiles[stage]
		if len(path) < 2 || path[0] != '/' {
			return ErrInvalid
		}
	}
	return nil
}

func validateDraftLearningWorkerConfig(config DraftLearningWorkerConfig) error {
	if validateWorkerConfig(config.WorkerConfig) != nil {
		return ErrInvalid
	}
	for _, path := range []string{
		config.ReleaseManifestFile, config.ClientCertificateFile, config.ServerCAFile,
		config.AuthorityPublicKeyFile, config.ObjectCredentialMetaFile,
	} {
		if !filepath.IsAbs(path) {
			return ErrInvalid
		}
	}
	parsed, err := url.Parse(config.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Path != "/internal/neo-runner/v1/rpc" ||
		!identityPattern.MatchString(config.RunnerID) ||
		!identityPattern.MatchString(config.ServerName) ||
		config.CallerIdentity != DraftLearningCaller {
		return ErrInvalid
	}
	return nil
}

func heldOrInvalid(decision Decision, reason string) (Decision, error) {
	if member(reason, "NON_PRODUCTION_EVIDENCE", "EVIDENCE_STALE", "ISOLATION_UNAVAILABLE",
		"LIVE_CHECK_NOT_PASSED", "RUNTIME_RESIDUE_REMAINS", "REVIEW_HELD") {
		return decision, ErrHeld
	}
	return decision, ErrInvalid
}

func validateCronWorkerShape(raw []byte) error {
	root, err := rawObject(raw, "schemaVersion", "evidenceClass", "stage", "release",
		"prerequisites", "target", "window", "checks", "authorization", "cleanup", "review")
	if err != nil {
		return err
	}
	for key, fields := range map[string][]string{
		"release":       {"gitCommit", "migrationHead", "runnerManifestSha256", "runnerBinarySha256", "operationsPolicySha256"},
		"target":        {"activationId", "templateId", "templateRevision", "templateFingerprint", "planSha256"},
		"window":        {"startedAt", "completedAt", "expiresAt"},
		"authorization": {"exactCronTarget", "runtime", "genericScheduler", "learning", "skillInstall", "brokerMutation", "delegation", "runnerCredential", "objectCredential"},
		"cleanup":       {"staleClaims", "pendingTriggers"},
		"review":        {"decision", "reviewedAt", "reviewerFingerprint"},
	} {
		if _, shapeErr := rawObject(root[key], fields...); shapeErr != nil {
			return shapeErr
		}
	}
	return validateWorkerArrays(root, len(cronWorkerChecks))
}

func validateDraftLearningWorkerShape(raw []byte) error {
	root, err := rawObject(raw, "schemaVersion", "evidenceClass", "stage", "release",
		"prerequisites", "target", "wiring", "window", "checks", "authorization", "cleanup",
		"humanDecision", "review")
	if err != nil {
		return err
	}
	for key, fields := range map[string][]string{
		"release":       {"gitCommit", "migrationHead", "runnerManifestSha256", "runnerBinarySha256", "operationsPolicySha256"},
		"target":        {"activationId", "draftId", "draftFingerprint", "packageFingerprint", "runtimeFingerprint", "archiveFingerprint", "workspaceFingerprint", "planSha256"},
		"wiring":        {"endpointSha256", "clientCertificateSha256", "serverCASha256", "serverName", "callerIdentity", "authorityPublicKeySha256", "objectCredentialMetaSha256"},
		"window":        {"startedAt", "completedAt", "expiresAt"},
		"authorization": {"draftChecks", "cleanup", "runnerLifecycle", "runtime", "genericScheduler", "genericLearning", "skillInstall", "brokerMutation", "delegation", "promote", "administratorCredential"},
		"cleanup":       {"orphanSandboxes", "pendingRunnerChecks", "pendingObjects"},
		"humanDecision": {"decisionId", "actorClass", "promotedPackageFingerprint"},
		"review":        {"decision", "reviewedAt", "reviewerFingerprint"},
	} {
		if _, shapeErr := rawObject(root[key], fields...); shapeErr != nil {
			return shapeErr
		}
	}
	return validateWorkerArrays(root, len(draftLearningWorkerChecks))
}

func validateWorkerArrays(root map[string]json.RawMessage, checkCount int) error {
	var prerequisites, checks []json.RawMessage
	if strictjson.Decode(root["prerequisites"], maxDocument, &prerequisites) != nil ||
		len(prerequisites) != len(workerPrerequisites) ||
		strictjson.Decode(root["checks"], maxDocument, &checks) != nil || len(checks) != checkCount {
		return ErrInvalid
	}
	for _, item := range prerequisites {
		if _, err := rawObject(item, "stage", "evidenceSha256"); err != nil {
			return err
		}
	}
	for _, item := range checks {
		if _, err := rawObject(item, "id", "result", "observedAt", "evidenceSha256", "detailCode"); err != nil {
			return err
		}
	}
	return nil
}
