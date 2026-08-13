package agentorchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	RunPending        = "pending"
	RunAdmitted       = "admitted"
	RunQueued         = "queued"
	RunRunning        = "running"
	RunSucceeded      = "succeeded"
	RunFailed         = "failed"
	RunCanceled       = "canceled"
	RunKilled         = "killed"
	RunOutcomeUnknown = "outcome_unknown"

	StepPending        = "pending"
	StepReady          = "ready"
	StepRunning        = "running"
	StepSucceeded      = "succeeded"
	StepFailed         = "failed"
	StepSkipped        = "skipped"
	StepCanceled       = "canceled"
	StepKilled         = "killed"
	StepOutcomeUnknown = "outcome_unknown"

	AttemptLeased         = "leased"
	AttemptStarting       = "starting"
	AttemptRunning        = "running"
	AttemptPrepared       = "prepared"
	AttemptCommitting     = "committing"
	AttemptSucceeded      = "succeeded"
	AttemptFailed         = "failed"
	AttemptCanceled       = "canceled"
	AttemptKilled         = "killed"
	AttemptOutcomeUnknown = "outcome_unknown"
	AttemptLeaseExpired   = "lease_expired"

	KillDenyNew = "deny_new"
	KillCancel  = "cancel"
	KillKill    = "kill"
)

var (
	ErrDatabaseRequired    = errors.New("agent orchestrator database is required")
	ErrInvalidInput        = errors.New("agent orchestrator input is invalid")
	ErrNotFound            = errors.New("agent orchestrator record was not found")
	ErrInvalidTransition   = errors.New("INVALID_TRANSITION")
	ErrLeaseStale          = errors.New("LEASE_STALE")
	ErrKillSwitchActive    = errors.New("KILL_SWITCH_ACTIVE")
	ErrIdempotencyConflict = errors.New("IDEMPOTENCY_CONFLICT")
	ErrRevisionConflict    = errors.New("REVISION_CONFLICT")
)

type StepPlan struct {
	ID   string `json:"stepId"`
	Kind string `json:"kind"`
}

type ScopeBinding struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type EnqueueInput struct {
	UserID         string
	IdempotencyKey string
	Snapshot       json.RawMessage
	Steps          []StepPlan
	ScopeBindings  []ScopeBinding
}

type EnqueueResult struct {
	Run     Run
	Created bool
}

type Run struct {
	ID                  string
	UserID              string
	SnapshotID          string
	SnapshotFingerprint string
	RequestFingerprint  string
	State               string
	NextSequence        int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	TerminalAt          *time.Time
	Steps               []Step
	Attempts            []Attempt
}

type Step struct {
	ID                string
	RunID             string
	Ordinal           int
	Kind              string
	State             string
	CurrentGeneration int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	TerminalAt        *time.Time
}

type Attempt struct {
	ID             string
	RunID          string
	StepID         string
	Generation     int64
	State          string
	LeaseOwner     string
	LeaseExpiresAt time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	TerminalAt     *time.Time
}

// Lease contains the opaque token only at acquisition time. The database stores
// its SHA-256 digest and events never contain it.
type Lease struct {
	Attempt
	Token string
}

type Actor struct {
	Type string
	ID   string
}

type TransitionInput struct {
	EventID    string
	UserID     string
	RunID      string
	StepID     string
	AttemptID  string
	Generation int64
	LeaseOwner string
	LeaseToken string
	Expected   string
	To         string
	Actor      Actor
	ReasonCode string
	Detail     map[string]any
}

type AcquireInput struct {
	UserID        string
	RunID         string
	StepID        string
	LeaseOwner    string
	LeaseDuration time.Duration
	Actor         Actor
	ReasonCode    string
}

type HeartbeatInput struct {
	EventID       string
	UserID        string
	RunID         string
	StepID        string
	AttemptID     string
	Generation    int64
	LeaseOwner    string
	LeaseToken    string
	LeaseDuration time.Duration
	Actor         Actor
	ReasonCode    string
}

type RecoveryRun struct {
	RunID          string
	UserID         string
	State          string
	SnapshotID     string
	AttemptID      string
	StepID         string
	Generation     int64
	AttemptState   string
	LeaseExpiresAt *time.Time
	LeaseExpired   bool
}

type KillSwitchInput struct {
	ID               string
	ScopeType        string
	ScopeValue       string
	Mode             string
	Active           bool
	ExpectedRevision int64
	Actor            Actor
	ReasonCode       string
}

type KillSwitch struct {
	ID         string
	ScopeType  string
	ScopeValue string
	Mode       string
	Active     bool
	Revision   int64
	Epoch      int64
	Actor      Actor
	ReasonCode string
	CreatedAt  time.Time
}

type KillResolution struct {
	Active    bool
	Mode      string
	Epoch     int64
	SwitchIDs []string
}

type Event struct {
	ID             string
	RunID          string
	StepID         string
	AttemptID      string
	Sequence       int64
	OccurredAt     time.Time
	Kind           string
	Entity         string
	From           string
	To             string
	Actor          Actor
	ReasonCode     string
	Generation     int64
	LeaseExpiresAt *time.Time
	Detail         map[string]any
}

type Repository interface {
	EnqueueRun(context.Context, preparedEnqueue) (string, bool, error)
	GetRun(context.Context, string, string) (Run, error)
	TransitionRun(context.Context, TransitionInput) error
	TransitionStep(context.Context, TransitionInput) error
	TransitionAttempt(context.Context, TransitionInput) error
	ObserveTerminalConflict(context.Context, TransitionInput) error
	AcquireStep(context.Context, AcquireInput, preparedLease) (Lease, error)
	HeartbeatAttempt(context.Context, HeartbeatInput, string) (time.Time, error)
	ListRecoveryRuns(context.Context, int) ([]RecoveryRun, error)
	RebuildProjection(context.Context, string, string) error
	AppendKillSwitch(context.Context, KillSwitchInput) (KillSwitch, error)
	ResolveKillSwitch(context.Context, string, string) (KillResolution, error)
	PruneTerminalRuns(context.Context, time.Time, int) (int, error)
}

type preparedEnqueue struct {
	Input               EnqueueInput
	RunID               string
	SnapshotID          string
	SnapshotFingerprint string
	RequestFingerprint  string
	CanonicalSnapshot   []byte
	StepsJSON           []byte
	ScopeKeys           []string
	EventIDs            []string
}

type preparedLease struct {
	AttemptID string
	Token     string
	TokenHash string
	EventIDs  []string
}
