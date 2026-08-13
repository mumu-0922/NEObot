package agentbroker

import (
	"context"
	"encoding/json"
	"time"
)

const (
	GrantVersion    = "neo.capability-grant/v1"
	RegistryVersion = "neo.tool-registry/v1"
	IntentVersion   = "neo.effect-intent/v1"

	ApprovalAutomatic = "automatic"
	ApprovalOnce      = "once"
	ApprovalPerCommit = "per_commit"
	ApprovalDenied    = "denied"

	IntentPrepared         = "prepared"
	IntentAwaitingApproval = "awaiting_approval"
	IntentApproved         = "approved"
	IntentCommitting       = "committing"
	IntentCommitted        = "committed"
	IntentFailed           = "failed"
	IntentRejected         = "rejected"
	IntentCanceled         = "canceled"
	IntentExpired          = "expired"
	IntentOutcomeUnknown   = "outcome_unknown"

	OutcomeCommitted = "committed"
	OutcomeReplayed  = "replayed"
	OutcomeRejected  = "rejected"
	OutcomeUnknown   = "outcome_unknown"

	ClassificationRead    = "read"
	ClassificationMutable = "mutable"
	ClassificationUnknown = "unknown"
)

type Subject struct {
	UserID      string `json:"userId"`
	ProjectID   string `json:"projectId"`
	AssistantID string `json:"assistantId"`
}

type RunBinding struct {
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId,omitempty"`
	Depth       int    `json:"depth"`
}

type Selector struct {
	Kind   string   `json:"kind"`
	Values []string `json:"values"`
}

type Capability struct {
	Capability string   `json:"capability"`
	Actions    []string `json:"actions"`
	Resources  Selector `json:"resources"`
	Approval   string   `json:"approval"`
	MaxCalls   int      `json:"maxCalls"`
}

type EgressRule struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Ports  []int  `json:"ports"`
}

type EgressPolicy struct {
	Mode  string       `json:"mode"`
	Rules []EgressRule `json:"rules"`
}

type SecretGrant struct {
	Slot       string   `json:"slot"`
	BrokerRef  string   `json:"brokerRef"`
	Actions    []string `json:"actions"`
	TTLSeconds int      `json:"ttlSeconds"`
}

type Budget struct {
	MaxWallSeconds   int   `json:"maxWallSeconds"`
	MaxModelTokens   int64 `json:"maxModelTokens"`
	MaxToolCalls     int   `json:"maxToolCalls"`
	MaxArtifactBytes int64 `json:"maxArtifactBytes"`
}

type CapabilityGrant struct {
	SchemaVersion            string        `json:"schemaVersion"`
	GrantID                  string        `json:"grantId"`
	Subject                  Subject       `json:"subject"`
	Run                      RunBinding    `json:"run"`
	PackageFingerprint       string        `json:"packageFingerprint"`
	RuntimeBundleFingerprint string        `json:"runtimeBundleFingerprint"`
	IssuedAt                 time.Time     `json:"issuedAt"`
	ExpiresAt                time.Time     `json:"expiresAt"`
	Capabilities             []Capability  `json:"capabilities"`
	Egress                   EgressPolicy  `json:"egress"`
	Secrets                  []SecretGrant `json:"secrets"`
	Budget                   Budget        `json:"budget"`
}

type ToolDefinition struct {
	Identity       string   `json:"identity"`
	Capability     string   `json:"capability"`
	Actions        []string `json:"actions"`
	Classification string   `json:"classification"`
	Idempotent     bool     `json:"idempotent"`
}

type RegistryTool struct {
	Identity       string   `json:"identity"`
	Capability     string   `json:"capability"`
	Actions        []string `json:"actions"`
	Resources      Selector `json:"resources"`
	Approval       string   `json:"approval"`
	MaxCalls       int      `json:"maxCalls"`
	Classification string   `json:"classification"`
	Idempotent     bool     `json:"idempotent"`
}

type ToolRegistry struct {
	SchemaVersion            string         `json:"schemaVersion"`
	Subject                  Subject        `json:"subject"`
	RunID                    string         `json:"runId"`
	Depth                    int            `json:"depth"`
	GrantID                  string         `json:"grantId"`
	PackageFingerprint       string         `json:"packageFingerprint"`
	RuntimeBundleFingerprint string         `json:"runtimeBundleFingerprint"`
	ExpiresAt                time.Time      `json:"expiresAt"`
	Egress                   EgressPolicy   `json:"egress"`
	Secrets                  []SecretGrant  `json:"secrets"`
	Budget                   Budget         `json:"budget"`
	Tools                    []RegistryTool `json:"tools"`
	Fingerprint              string         `json:"registryFingerprint"`
}

type AttemptAuthority struct {
	UserID              string
	RunID               string
	StepID              string
	AttemptID           string
	Generation          int64
	LeaseOwner          string
	LeaseToken          string
	SnapshotFingerprint string
	GrantFingerprint    string
	RegistryFingerprint string
	KillSwitchEpoch     int64
}

type PrepareInput struct {
	RequestID    string
	Attempt      AttemptAuthority
	Grant        CapabilityGrant
	Registry     ToolRegistry
	ToolIdentity string
	Action       string
	Resource     string
	Arguments    json.RawMessage
	BaseRevision string
	TTL          time.Duration
}

type PreparedIntent struct {
	SchemaVersion        string
	IntentID             string
	RequestID            string
	RequestFingerprint   string
	IntentFingerprint    string
	IdempotencyKey       string
	Subject              Subject
	RunID                string
	StepID               string
	AttemptID            string
	Generation           int64
	LeaseOwner           string
	LeaseTokenDigest     string
	SnapshotFingerprint  string
	GrantID              string
	GrantFingerprint     string
	RegistryFingerprint  string
	ToolIdentity         string
	Capability           string
	Action               string
	Resource             string
	ArgumentsFingerprint string
	CanonicalArguments   json.RawMessage
	BaseRevision         string
	ApprovalClass        string
	CapabilityMaxCalls   int
	GrantMaxToolCalls    int
	State                string
	KillSwitchEpoch      int64
	CreatedAt            time.Time
	ExpiresAt            time.Time
	ApprovedAt           *time.Time
	ApprovalRevision     int64
	CommittingAt         *time.Time
	TerminalAt           *time.Time
	ReceiptFingerprint   string
	ExecutorStatusDigest string
	ErrorCode            string
	Replay               bool
}

type ApprovalInput struct {
	ApprovalID        string
	UserID            string
	IntentID          string
	IntentFingerprint string
	Decision          string
	ActorType         string
	ActorID           string
	ReasonCode        string
	ExpectedRevision  int64
}

type GrantRevocationInput struct {
	GrantID          string
	GrantFingerprint string
	ActorType        string
	ActorID          string
	ReasonCode       string
}

type CancelInput struct {
	CancellationID    string
	UserID            string
	IntentID          string
	IntentFingerprint string
	ActorType         string
	ActorID           string
	ReasonCode        string
}

type CommitInput struct {
	Attempt           AttemptAuthority
	IntentID          string
	IntentFingerprint string
	ApprovalID        string
	IdempotencyKey    string
}

type CommitClaim struct {
	Intent PreparedIntent
	Replay bool
}

type CommitResult struct {
	Outcome            string
	IdempotencyKey     string
	ReceiptFingerprint string
	ErrorCode          string
	Replay             bool
}

type ExecutionRequest struct {
	IntentID             string
	IdempotencyKey       string
	ToolIdentity         string
	Capability           string
	Action               string
	Resource             string
	Arguments            json.RawMessage
	ArgumentsFingerprint string
	BaseRevision         string
}

type ExecutorReceipt struct {
	ReceiptFingerprint string
	StatusToken        string
}

type DispatchError struct {
	Cause        error
	PossibleSend bool
}

func (e *DispatchError) Error() string {
	if e == nil || e.Cause == nil {
		return ErrExecutorUnavailable.Error()
	}
	return e.Cause.Error()
}

func (e *DispatchError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type ExecutorStatus struct {
	Outcome            string
	ReceiptFingerprint string
	StatusToken        string
}

type EffectExecutor interface {
	Commit(context.Context, ExecutionRequest) (ExecutorReceipt, error)
	Status(context.Context, ExecutionRequest) (ExecutorStatus, error)
}

// ReadOnlyExecutor is deliberately separate from EffectExecutor. A caller may
// use it only after live Broker authorization and only for a Registry Tool
// whose classification is read and whose policy is explicitly idempotent.
type ReadOnlyExecutor interface {
	Read(context.Context, ExecutionRequest) (json.RawMessage, error)
}

func ReadRetryAllowed(tool RegistryTool, observedResult bool) bool {
	return tool.Classification == ClassificationRead && tool.Idempotent &&
		tool.Approval == ApprovalAutomatic && !observedResult
}

type Repository interface {
	Prepare(context.Context, PreparedIntent) (PreparedIntent, error)
	DecideApproval(context.Context, ApprovalInput) (PreparedIntent, error)
	Cancel(context.Context, CancelInput) (PreparedIntent, error)
	ClaimCommit(context.Context, CommitInput, string) (CommitClaim, error)
	CompleteCommit(context.Context, string, string, string, string, string) (PreparedIntent, error)
	RevokeGrant(context.Context, GrantRevocationInput) (bool, error)
	GetIntent(context.Context, string, string) (PreparedIntent, error)
	ListCommitting(context.Context, int) ([]PreparedIntent, error)
	Expire(context.Context, time.Time, int) (int, error)
}
