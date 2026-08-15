// Package agentactivation verifies the short-lived, control-only production
// activation evidence consumed by the dedicated Agent Runtime control worker.
package agentactivation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"

	"golang.org/x/sys/unix"
)

const (
	SchemaVersion         = "neo.agent-production-activation/v1"
	StageControl          = "control_plane"
	ControlCallerIdentity = "spiffe://neo-chat/agent-runtime-control"
	maxDocument           = 1 << 20
)

var (
	ErrInvalid = errors.New("ACTIVATION_EVIDENCE_INVALID")
	ErrHeld    = errors.New("ACTIVATION_HELD")

	fingerprintPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	identityPattern    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:@/-]{0,127}$`)
	detailPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

var requiredChecks = []string{
	"clean_copy_install",
	"exact_host_isolation",
	"private_mtls_probe",
	"rollback_kill_switch_ready",
	"zero_inventory_reconcile",
}

type Config struct {
	PolicyFile            string
	RecordFile            string
	ReleaseManifestFile   string
	ClientCertificateFile string
	ServerCAFile          string
	Endpoint              string
	RunnerID              string
	ServerName            string
	CallerIdentity        string
	ReleaseCommit         string
}

type Decision struct {
	Ready               bool
	ReasonCode          string
	EvidenceFingerprint string
	PolicyFingerprint   string
	ManifestFingerprint string
}

type record struct {
	SchemaVersion string        `json:"schemaVersion"`
	EvidenceClass string        `json:"evidenceClass"`
	Stage         string        `json:"stage"`
	Release       release       `json:"release"`
	Target        target        `json:"target"`
	Wiring        wiring        `json:"wiring"`
	Window        window        `json:"window"`
	Checks        []check       `json:"checks"`
	Authorization authorization `json:"authorization"`
	Cleanup       cleanup       `json:"cleanup"`
	Review        review        `json:"review"`
}

type release struct {
	GitCommit              string `json:"gitCommit"`
	MigrationHead          int    `json:"migrationHead"`
	RunnerManifestSHA256   string `json:"runnerManifestSha256"`
	RunnerBinarySHA256     string `json:"runnerBinarySha256"`
	OperationsPolicySHA256 string `json:"operationsPolicySha256"`
}

type target struct {
	DeploymentFingerprint string `json:"deploymentFingerprint"`
	RunnerID              string `json:"runnerId"`
}

type wiring struct {
	EndpointSHA256          string `json:"endpointSha256"`
	ClientCertificateSHA256 string `json:"clientCertificateSha256"`
	ServerCASHA256          string `json:"serverCASha256"`
	ServerName              string `json:"serverName"`
	CallerIdentity          string `json:"callerIdentity"`
}

type window struct {
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type check struct {
	ID             string    `json:"id"`
	Result         string    `json:"result"`
	ObservedAt     time.Time `json:"observedAt"`
	EvidenceSHA256 string    `json:"evidenceSha256"`
	DetailCode     string    `json:"detailCode"`
}

type authorization struct {
	ControlPlane   bool `json:"controlPlane"`
	RootRuns       bool `json:"rootRuns"`
	BrokerReadOnly bool `json:"brokerReadOnly"`
	BrokerMutable  bool `json:"brokerMutable"`
	Delegation     bool `json:"delegation"`
	Scheduler      bool `json:"scheduler"`
	Learning       bool `json:"learning"`
}

type cleanup struct {
	OrphanSandboxes int `json:"orphanSandboxes"`
	ScratchResidue  int `json:"scratchResidue"`
}

type review struct {
	Decision            string    `json:"decision"`
	ReviewedAt          time.Time `json:"reviewedAt"`
	ReviewerFingerprint string    `json:"reviewerFingerprint"`
}

func Verify(config Config, now time.Time) (Decision, error) {
	if validateConfig(config) != nil || now.IsZero() {
		return Decision{ReasonCode: "CONFIG_INVALID"}, ErrInvalid
	}
	policyRaw, err := readRegular(config.PolicyFile, false)
	if err != nil {
		return Decision{ReasonCode: "POLICY_UNREADABLE"}, ErrInvalid
	}
	var policyDocument map[string]any
	if strictjson.Decode(policyRaw, maxDocument, &policyDocument) != nil {
		return Decision{ReasonCode: "POLICY_INVALID"}, ErrInvalid
	}
	if schema, ok := policyDocument["schemaVersion"].(string); !ok || schema != "neo.agent-production-policy/v1" {
		return Decision{ReasonCode: "POLICY_INVALID"}, ErrInvalid
	}
	migrationHead, ok := policyDocument["migrationHead"].(float64)
	if !ok || migrationHead != 93 {
		return Decision{ReasonCode: "POLICY_INVALID"}, ErrInvalid
	}
	policyFingerprint := fingerprint(policyRaw)

	recordRaw, err := readRegular(config.RecordFile, true)
	if err != nil {
		return Decision{PolicyFingerprint: policyFingerprint, ReasonCode: "ACTIVATION_UNREADABLE"}, ErrInvalid
	}
	decision := Decision{PolicyFingerprint: policyFingerprint, EvidenceFingerprint: fingerprint(recordRaw)}
	var evidence record
	if validateRecordShape(recordRaw) != nil || strictjson.Decode(recordRaw, maxDocument, &evidence) != nil {
		decision.ReasonCode = "ACTIVATION_ROOT_INVALID"
		return decision, ErrInvalid
	}
	if reason := validateRecord(evidence, config, policyFingerprint, now.UTC()); reason != "" {
		decision.ReasonCode = reason
		if reason == "NON_PRODUCTION_EVIDENCE" || reason == "EVIDENCE_STALE" ||
			reason == "ISOLATION_UNAVAILABLE" || reason == "LIVE_CHECK_NOT_PASSED" ||
			reason == "RUNTIME_RESIDUE_REMAINS" || reason == "REVIEW_HELD" {
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
	clientCertificate, err := readRegular(config.ClientCertificateFile, true)
	if err != nil || fingerprint(clientCertificate) != evidence.Wiring.ClientCertificateSHA256 {
		decision.ReasonCode = "CLIENT_CERTIFICATE_DRIFT"
		return decision, ErrInvalid
	}
	serverCA, err := readRegular(config.ServerCAFile, true)
	if err != nil || fingerprint(serverCA) != evidence.Wiring.ServerCASHA256 {
		decision.ReasonCode = "SERVER_CA_DRIFT"
		return decision, ErrInvalid
	}
	decision.Ready = true
	decision.ReasonCode = "CONTROL_PLANE_GATES_PASSED"
	return decision, nil
}

func manifestHasPlaceholder(manifest agentrunner.ReleaseManifest) bool {
	fingerprints := []string{
		manifest.ProbeSuiteFingerprint,
		manifest.SeccompProfile.SHA256,
		manifest.IsolationAcceptance.SHA256,
	}
	for _, binary := range manifest.Binaries {
		fingerprints = append(fingerprints, binary.SHA256)
	}
	for _, value := range fingerprints {
		if value == "sha256:0000000000000000000000000000000000000000000000000000000000000000" {
			return true
		}
	}
	return false
}

func validateRecordShape(raw []byte) error {
	root, err := rawObject(raw,
		"schemaVersion", "evidenceClass", "stage", "release", "target", "wiring",
		"window", "checks", "authorization", "cleanup", "review")
	if err != nil {
		return err
	}
	for key, fields := range map[string][]string{
		"release":       {"gitCommit", "migrationHead", "runnerManifestSha256", "runnerBinarySha256", "operationsPolicySha256"},
		"target":        {"deploymentFingerprint", "runnerId"},
		"wiring":        {"endpointSha256", "clientCertificateSha256", "serverCASha256", "serverName", "callerIdentity"},
		"window":        {"startedAt", "completedAt", "expiresAt"},
		"authorization": {"controlPlane", "rootRuns", "brokerReadOnly", "brokerMutable", "delegation", "scheduler", "learning"},
		"cleanup":       {"orphanSandboxes", "scratchResidue"},
		"review":        {"decision", "reviewedAt", "reviewerFingerprint"},
	} {
		if _, objectErr := rawObject(root[key], fields...); objectErr != nil {
			return objectErr
		}
	}
	var checks []json.RawMessage
	if strictjson.Decode(root["checks"], maxDocument, &checks) != nil {
		return ErrInvalid
	}
	for _, item := range checks {
		if _, itemErr := rawObject(item, "id", "result", "observedAt", "evidenceSha256", "detailCode"); itemErr != nil {
			return itemErr
		}
	}
	return nil
}

func rawObject(raw []byte, fields ...string) (map[string]json.RawMessage, error) {
	var value map[string]json.RawMessage
	if strictjson.Decode(raw, maxDocument, &value) != nil || len(value) != len(fields) {
		return nil, ErrInvalid
	}
	for _, field := range fields {
		if _, ok := value[field]; !ok {
			return nil, ErrInvalid
		}
	}
	return value, nil
}

func validateRecord(value record, config Config, policyFingerprint string, now time.Time) string {
	if value.SchemaVersion != SchemaVersion || value.Stage != StageControl ||
		!member(value.EvidenceClass, "template", "production") {
		return "ACTIVATION_VERSION_INVALID"
	}
	if !commitPattern.MatchString(value.Release.GitCommit) || value.Release.MigrationHead != 93 ||
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
	if !validFingerprint(value.Wiring.EndpointSHA256) || !validFingerprint(value.Wiring.ClientCertificateSHA256) ||
		!validFingerprint(value.Wiring.ServerCASHA256) || value.Wiring.EndpointSHA256 != EndpointFingerprint(config.Endpoint) ||
		value.Wiring.ServerName != config.ServerName || value.Wiring.CallerIdentity != config.CallerIdentity {
		return "WIRING_INVALID"
	}
	if value.Window.StartedAt.IsZero() || value.Window.CompletedAt.IsZero() || value.Window.ExpiresAt.IsZero() ||
		value.Window.CompletedAt.Before(value.Window.StartedAt) || !value.Window.ExpiresAt.After(value.Window.CompletedAt) ||
		value.Window.ExpiresAt.Sub(value.Window.StartedAt) > 24*time.Hour {
		return "WINDOW_INVALID"
	}
	results := make(map[string]string, len(value.Checks))
	fingerprints := []string{
		value.Release.RunnerManifestSHA256, value.Release.RunnerBinarySHA256,
		value.Target.DeploymentFingerprint, value.Wiring.EndpointSHA256,
		value.Wiring.ClientCertificateSHA256, value.Wiring.ServerCASHA256,
	}
	for _, item := range value.Checks {
		if !contains(requiredChecks, item.ID) || results[item.ID] != "" ||
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
	if !sameStrings(ids, requiredChecks) {
		return "CHECK_SET_INCOMPLETE"
	}
	if !value.Authorization.ControlPlane || value.Authorization.RootRuns ||
		value.Authorization.BrokerReadOnly || value.Authorization.BrokerMutable ||
		value.Authorization.Delegation || value.Authorization.Scheduler || value.Authorization.Learning {
		return "AUTHORIZATION_WIDENED"
	}
	if value.Cleanup.OrphanSandboxes < 0 || value.Cleanup.ScratchResidue < 0 ||
		value.Cleanup.OrphanSandboxes > 1_000_000 || value.Cleanup.ScratchResidue > 1_000_000 {
		return "CLEANUP_INVALID"
	}
	if !member(value.Review.Decision, "approved", "held") ||
		value.Review.ReviewedAt.Before(value.Window.CompletedAt) || !value.Review.ReviewedAt.Before(value.Window.ExpiresAt) ||
		value.Review.ReviewedAt.After(now) ||
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
	if value.Cleanup.OrphanSandboxes != 0 || value.Cleanup.ScratchResidue != 0 {
		return "RUNTIME_RESIDUE_REMAINS"
	}
	if value.Review.Decision != "approved" {
		return "REVIEW_HELD"
	}
	return ""
}

func EndpointFingerprint(endpoint string) string {
	return fingerprint(append([]byte("neo-agent-runner-endpoint-v1\x00"), []byte(endpoint)...))
}

func fingerprint(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func readRegular(path string, private bool) ([]byte, error) {
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	file := os.NewFile(uintptr(descriptor), path)
	if file == nil {
		_ = unix.Close(descriptor)
		return nil, ErrInvalid
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 2 || info.Size() > maxDocument {
		return nil, ErrInvalid
	}
	if info.Mode().Perm()&0o022 != 0 || private && info.Mode().Perm()&0o077 != 0 {
		return nil, ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxDocument+1))
	if err != nil || int64(len(raw)) != info.Size() || len(raw) > maxDocument {
		return nil, ErrInvalid
	}
	return raw, nil
}

func validateConfig(config Config) error {
	for _, path := range []string{config.PolicyFile, config.RecordFile, config.ReleaseManifestFile, config.ClientCertificateFile, config.ServerCAFile} {
		if len(path) < 2 || path[0] != '/' {
			return ErrInvalid
		}
	}
	if config.Endpoint == "" || !identityPattern.MatchString(config.RunnerID) ||
		!identityPattern.MatchString(config.ServerName) || !identityPattern.MatchString(config.CallerIdentity) ||
		config.CallerIdentity != ControlCallerIdentity || !commitPattern.MatchString(config.ReleaseCommit) {
		return ErrInvalid
	}
	return nil
}

func validFingerprint(value string) bool { return fingerprintPattern.MatchString(value) }
func member(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (decision Decision) Error() string {
	return fmt.Sprintf("Agent Runtime activation: %s", decision.ReasonCode)
}
