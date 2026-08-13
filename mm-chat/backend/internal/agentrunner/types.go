// Package agentrunner implements the private, fail-closed control boundary for
// the host-side Neo Agent Runner. It is deliberately not wired into the public
// API or chat runtime.
package agentrunner

import (
	"encoding/json"
	"errors"
	"time"
)

const (
	ProtocolVersion  = "neo.runner-rpc/v1"
	AuthorityVersion = "neo.runner-authority/v1"
	ReleaseVersion   = "neo.agent-runner-release/v1"

	MethodProbe     = "probe"
	MethodLaunch    = "launch"
	MethodHeartbeat = "heartbeat"
	MethodCancel    = "cancel"
	MethodPrepare   = "prepare"
	MethodCommit    = "commit"
	MethodList      = "list"
	MethodReconcile = "reconcile"

	ErrorAuthFailed           = "AUTH_FAILED"
	ErrorReplayDetected       = "REPLAY_DETECTED"
	ErrorVersionUnsupported   = "VERSION_UNSUPPORTED"
	ErrorRuntimeUnavailable   = "RUNTIME_UNAVAILABLE"
	ErrorIsolationUnavailable = "ISOLATION_UNAVAILABLE"
	ErrorSnapshotMismatch     = "SNAPSHOT_MISMATCH"
	ErrorGrantDenied          = "GRANT_DENIED"
	ErrorLeaseStale           = "LEASE_STALE"
	ErrorKillSwitchActive     = "KILL_SWITCH_ACTIVE"
	ErrorBudgetExhausted      = "BUDGET_EXHAUSTED"
	ErrorApprovalRequired     = "APPROVAL_REQUIRED"
	ErrorApprovalDenied       = "APPROVAL_DENIED"
	ErrorIntentExpired        = "INTENT_EXPIRED"
	ErrorEgressDenied         = "EGRESS_DENIED"
	ErrorSecretDenied         = "SECRET_DENIED"
	ErrorProjectConflict      = "PROJECT_CONFLICT"
	ErrorArtifactDenied       = "ARTIFACT_DENIED"
	ErrorExecutorUnavailable  = "EXECUTOR_UNAVAILABLE"
	ErrorInvalidTransition    = "INVALID_TRANSITION"
	ErrorOutcomeUnknown       = "OUTCOME_UNKNOWN"
	ErrorInternal             = "INTERNAL_ERROR"
)

var (
	ErrInvalidInput         = errors.New("agent Runner input is invalid")
	ErrAuthFailed           = errors.New(ErrorAuthFailed)
	ErrReplayDetected       = errors.New(ErrorReplayDetected)
	ErrVersionUnsupported   = errors.New(ErrorVersionUnsupported)
	ErrRuntimeUnavailable   = errors.New(ErrorRuntimeUnavailable)
	ErrIsolationUnavailable = errors.New(ErrorIsolationUnavailable)
	ErrSnapshotMismatch     = errors.New(ErrorSnapshotMismatch)
	ErrGrantDenied          = errors.New(ErrorGrantDenied)
	ErrLeaseStale           = errors.New(ErrorLeaseStale)
	ErrKillSwitchActive     = errors.New(ErrorKillSwitchActive)
	ErrBudgetExhausted      = errors.New(ErrorBudgetExhausted)
	ErrApprovalRequired     = errors.New(ErrorApprovalRequired)
	ErrApprovalDenied       = errors.New(ErrorApprovalDenied)
	ErrIntentExpired        = errors.New(ErrorIntentExpired)
	ErrEgressDenied         = errors.New(ErrorEgressDenied)
	ErrSecretDenied         = errors.New(ErrorSecretDenied)
	ErrProjectConflict      = errors.New(ErrorProjectConflict)
	ErrArtifactDenied       = errors.New(ErrorArtifactDenied)
	ErrExecutorUnavailable  = errors.New(ErrorExecutorUnavailable)
	ErrInvalidTransition    = errors.New(ErrorInvalidTransition)
	ErrOutcomeUnknown       = errors.New(ErrorOutcomeUnknown)
	ErrNotFound             = errors.New("agent Runner record was not found")
)

type Envelope struct {
	SchemaVersion string          `json:"schemaVersion"`
	Method        string          `json:"method"`
	RequestID     string          `json:"requestId"`
	SentAt        time.Time       `json:"sentAt"`
	Nonce         string          `json:"nonce"`
	Body          json.RawMessage `json:"body"`
}

type AttemptRef struct {
	RunID           string `json:"runId"`
	StepID          string `json:"stepId"`
	AttemptID       string `json:"attemptId"`
	LeaseGeneration int64  `json:"leaseGeneration"`
	LeaseOwner      string `json:"leaseOwner"`
	LeaseToken      string `json:"leaseToken"`
}

// AttemptIdentity is safe to return and persist. It intentionally omits the
// opaque lease credential.
type AttemptIdentity struct {
	RunID           string `json:"runId"`
	StepID          string `json:"stepId"`
	AttemptID       string `json:"attemptId"`
	LeaseGeneration int64  `json:"leaseGeneration"`
}

func (attempt AttemptRef) Identity() AttemptIdentity {
	return AttemptIdentity{
		RunID: attempt.RunID, StepID: attempt.StepID, AttemptID: attempt.AttemptID,
		LeaseGeneration: attempt.LeaseGeneration,
	}
}

type AuthorityClaims struct {
	SchemaVersion       string    `json:"schemaVersion"`
	RunnerID            string    `json:"runnerId"`
	CallerIdentity      string    `json:"callerIdentity"`
	Method              string    `json:"method"`
	RequestID           string    `json:"requestId"`
	Nonce               string    `json:"nonce"`
	RequestFingerprint  string    `json:"requestFingerprint"`
	RunID               string    `json:"runId"`
	StepID              string    `json:"stepId"`
	AttemptID           string    `json:"attemptId"`
	LeaseGeneration     int64     `json:"leaseGeneration"`
	LeaseOwner          string    `json:"leaseOwner"`
	LeaseTokenDigest    string    `json:"leaseTokenDigest"`
	SnapshotFingerprint string    `json:"snapshotFingerprint"`
	KillSwitchEpoch     int64     `json:"killSwitchEpoch"`
	IssuedAt            time.Time `json:"issuedAt"`
	ExpiresAt           time.Time `json:"expiresAt"`
}

type AuthorityTicket struct {
	AuthorityClaims
	Signature string `json:"signature"`
}

type ResourceLimits struct {
	CPUMillis    int64 `json:"cpuMillis"`
	MemoryMiB    int64 `json:"memoryMiB"`
	PIDs         int64 `json:"pids"`
	WallSeconds  int64 `json:"wallSeconds"`
	OutputBytes  int64 `json:"outputBytes"`
	ScratchBytes int64 `json:"scratchBytes"`
}

type SandboxSpec struct {
	RuntimeBundleFingerprint  string         `json:"runtimeBundleFingerprint"`
	PackageFingerprint        string         `json:"packageFingerprint"`
	Image                     string         `json:"image"`
	UID                       int            `json:"uid"`
	GID                       int            `json:"gid"`
	RootfsReadOnly            bool           `json:"rootfsReadOnly"`
	NoNewPrivileges           bool           `json:"noNewPrivileges"`
	Capabilities              []string       `json:"capabilities"`
	SeccompProfileFingerprint string         `json:"seccompProfileFingerprint"`
	NetworkMode               string         `json:"networkMode"`
	WorkspaceSnapshotID       string         `json:"workspaceSnapshotId"`
	WorkspaceFingerprint      string         `json:"workspaceFingerprint"`
	Resources                 ResourceLimits `json:"resources"`
}

type ToolRegistry struct {
	Depth               int      `json:"depth"`
	Tools               []string `json:"tools"`
	RegistryFingerprint string   `json:"registryFingerprint"`
}

type ProbeRequest struct {
	RequiredFeatures []string `json:"requiredFeatures"`
}

type LaunchRequest struct {
	Attempt             AttemptRef      `json:"attempt"`
	Authority           AuthorityTicket `json:"authority"`
	GrantID             string          `json:"grantId"`
	GrantFingerprint    string          `json:"grantFingerprint"`
	SnapshotFingerprint string          `json:"snapshotFingerprint"`
	Sandbox             SandboxSpec     `json:"sandbox"`
	ToolRegistry        ToolRegistry    `json:"toolRegistry"`
	Argv                []string        `json:"argv"`
}

type HeartbeatRequest struct {
	Attempt           AttemptRef      `json:"attempt"`
	Authority         AuthorityTicket `json:"authority"`
	ObservedState     string          `json:"observedState"`
	LastEventSequence int64           `json:"lastEventSequence"`
}

type CancelRequest struct {
	Attempt    AttemptRef      `json:"attempt"`
	Authority  AuthorityTicket `json:"authority"`
	Mode       string          `json:"mode"`
	ReasonCode string          `json:"reasonCode"`
}

type PrepareRequest struct {
	Attempt              AttemptRef      `json:"attempt"`
	Authority            AuthorityTicket `json:"authority"`
	SnapshotFingerprint  string          `json:"snapshotFingerprint"`
	GrantID              string          `json:"grantId"`
	GrantFingerprint     string          `json:"grantFingerprint"`
	RegistryFingerprint  string          `json:"registryFingerprint"`
	ToolIdentity         string          `json:"toolIdentity"`
	Capability           string          `json:"capability"`
	Action               string          `json:"action"`
	Resource             string          `json:"resource"`
	Arguments            json.RawMessage `json:"arguments"`
	ArgumentsFingerprint string          `json:"argumentsFingerprint"`
	BaseRevision         string          `json:"baseRevision,omitempty"`
	TTLSeconds           int             `json:"ttlSeconds"`
}

type CommitRequest struct {
	Attempt             AttemptRef      `json:"attempt"`
	Authority           AuthorityTicket `json:"authority"`
	SnapshotFingerprint string          `json:"snapshotFingerprint"`
	GrantFingerprint    string          `json:"grantFingerprint"`
	RegistryFingerprint string          `json:"registryFingerprint"`
	IntentID            string          `json:"intentId"`
	IntentFingerprint   string          `json:"intentFingerprint"`
	ApprovalID          string          `json:"approvalId"`
	IdempotencyKey      string          `json:"idempotencyKey"`
}

type ListRequest struct {
	RunnerID string `json:"runnerId"`
}

type ReconcileRequest struct {
	RunnerID string              `json:"runnerId"`
	Expected []SandboxDescriptor `json:"expected"`
}

type Request struct {
	Envelope
	Probe     *ProbeRequest
	Launch    *LaunchRequest
	Heartbeat *HeartbeatRequest
	Cancel    *CancelRequest
	Prepare   *PrepareRequest
	Commit    *CommitRequest
	List      *ListRequest
	Reconcile *ReconcileRequest
	Canonical []byte
}

type RPCError struct {
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
}

type ErrorResult struct {
	Accepted bool     `json:"accepted"`
	Error    RPCError `json:"error"`
}

type ProbeResult struct {
	Ready            bool      `json:"ready"`
	Runtime          string    `json:"runtime"`
	RuntimeVersion   string    `json:"runtimeVersion"`
	Features         []string  `json:"features"`
	ProbeFingerprint string    `json:"probeFingerprint"`
	Error            *RPCError `json:"error,omitempty"`
}

type LaunchResult struct {
	Accepted  bool            `json:"accepted"`
	Attempt   AttemptIdentity `json:"attempt"`
	SandboxID string          `json:"sandboxId,omitempty"`
	StartedAt *time.Time      `json:"startedAt,omitempty"`
	Error     *RPCError       `json:"error,omitempty"`
}

type HeartbeatResult struct {
	Accepted       bool       `json:"accepted"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt,omitempty"`
	KillRequested  bool       `json:"killRequested"`
	Error          *RPCError  `json:"error,omitempty"`
}

type CancelResult struct {
	Accepted         bool      `json:"accepted"`
	ObservedTerminal string    `json:"observedTerminal"`
	Error            *RPCError `json:"error,omitempty"`
}

type PrepareResult struct {
	Prepared          bool       `json:"prepared"`
	IntentID          string     `json:"intentId,omitempty"`
	IntentFingerprint string     `json:"intentFingerprint,omitempty"`
	IdempotencyKey    string     `json:"idempotencyKey,omitempty"`
	Approval          string     `json:"approval,omitempty"`
	ExpiresAt         *time.Time `json:"expiresAt,omitempty"`
	Replay            bool       `json:"replay"`
	Error             *RPCError  `json:"error,omitempty"`
}

type CommitResult struct {
	Outcome            string    `json:"outcome"`
	IdempotencyKey     string    `json:"idempotencyKey"`
	ReceiptFingerprint string    `json:"receiptFingerprint,omitempty"`
	Replay             bool      `json:"replay"`
	Error              *RPCError `json:"error,omitempty"`
}

type SandboxDescriptor struct {
	SandboxID           string          `json:"sandboxId"`
	Attempt             AttemptIdentity `json:"attempt"`
	SnapshotFingerprint string          `json:"snapshotFingerprint"`
	SpecFingerprint     string          `json:"specFingerprint"`
	ProbeFingerprint    string          `json:"probeFingerprint"`
	State               string          `json:"state"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type ListResult struct {
	RunnerID  string              `json:"runnerId"`
	Sandboxes []SandboxDescriptor `json:"sandboxes"`
}

type ReconcileResult struct {
	RunnerID string `json:"runnerId"`
	Cleaned  int    `json:"cleaned"`
}

type Response struct {
	SchemaVersion string    `json:"schemaVersion"`
	Method        string    `json:"method"`
	RequestID     string    `json:"requestId"`
	SentAt        time.Time `json:"sentAt"`
	Nonce         string    `json:"nonce"`
	Body          any       `json:"body"`
}

type ReleaseManifest struct {
	SchemaVersion         string          `json:"schemaVersion"`
	Approved              bool            `json:"approved"`
	RunnerID              string          `json:"runnerId"`
	RunnerVersion         string          `json:"runnerVersion"`
	ProtocolVersion       string          `json:"protocolVersion"`
	Binaries              []ReleaseBinary `json:"binaries"`
	StorageDriver         string          `json:"storageDriver"`
	NetworkMode           string          `json:"networkMode"`
	UserNamespaceSize     int             `json:"userNamespaceSize"`
	RequiredControllers   []string        `json:"requiredControllers"`
	SeccompProfile        ReleaseFile     `json:"seccompProfile"`
	ProbeSuiteFingerprint string          `json:"probeSuiteFingerprint"`
	IsolationAcceptance   ReleaseFile     `json:"isolationAcceptance"`
}

type ReleaseBinary struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type ReleaseFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ProbeEvidence struct {
	Ready            bool     `json:"ready"`
	RunnerID         string   `json:"runnerId"`
	RunnerVersion    string   `json:"runnerVersion"`
	Runtime          string   `json:"runtime"`
	RuntimeVersion   string   `json:"runtimeVersion"`
	KernelClass      string   `json:"kernelClass"`
	StorageDriver    string   `json:"storageDriver"`
	NetworkMode      string   `json:"networkMode"`
	Features         []string `json:"features"`
	FailureClasses   []string `json:"failureClasses"`
	ProbeFingerprint string   `json:"probeFingerprint"`
}

type IsolationAcceptanceReport struct {
	SchemaVersion             string    `json:"schemaVersion"`
	Approved                  bool      `json:"approved"`
	RunnerID                  string    `json:"runnerId"`
	RunnerUID                 int       `json:"runnerUid"`
	RunnerGID                 int       `json:"runnerGid"`
	RunnerVersion             string    `json:"runnerVersion"`
	ProtocolVersion           string    `json:"protocolVersion"`
	PodmanVersion             string    `json:"podmanVersion"`
	CrunVersion               string    `json:"crunVersion"`
	StorageDriver             string    `json:"storageDriver"`
	NetworkMode               string    `json:"networkMode"`
	SeccompProfileFingerprint string    `json:"seccompProfileFingerprint"`
	ProbeSuiteFingerprint     string    `json:"probeSuiteFingerprint"`
	PassedFeatures            []string  `json:"passedFeatures"`
	GeneratedAt               time.Time `json:"generatedAt"`
	ExpiresAt                 time.Time `json:"expiresAt"`
}
