package agentdelegation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

var (
	ErrDatabaseRequired    = errors.New("agent delegation database is required")
	ErrInvalidInput        = errors.New("agent delegation input is invalid")
	ErrNotFound            = errors.New("agent delegation record was not found")
	ErrParentInvalid       = errors.New("PARENT_INVALID")
	ErrParentStale         = errors.New("PARENT_STALE")
	ErrDepthExceeded       = errors.New("DEPTH_EXCEEDED")
	ErrSubsetViolation     = errors.New("SUBSET_VIOLATION")
	ErrBudgetExceeded      = errors.New("BUDGET_EXCEEDED")
	ErrSnapshotMismatch    = errors.New("SNAPSHOT_MISMATCH")
	ErrLeaseStale          = errors.New("LEASE_STALE")
	ErrKillSwitchActive    = errors.New("KILL_SWITCH_ACTIVE")
	ErrSettlementInvalid   = errors.New("SETTLEMENT_INVALID")
	ErrIdempotencyConflict = errors.New("IDEMPOTENCY_CONFLICT")
)

type ModelBinding struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

type Authority struct {
	RunID                    string
	RootRunID                string
	ParentRunID              string
	Depth                    int
	UserID                   string
	SnapshotID               string
	SnapshotFingerprint      string
	Subject                  agentbroker.Subject
	Model                    ModelBinding
	PackageFingerprint       string
	RuntimeBundleFingerprint string
	Grant                    agentbroker.CapabilityGrant
	GrantFingerprint         string
	Registry                 agentbroker.ToolRegistry
	RegistryFingerprint      string
	ExpiresAt                time.Time
	Budget                   agentbroker.Budget
	Reserved                 agentbroker.Budget
	Consumed                 agentbroker.Budget
	State                    string
	CreatedAt                time.Time
}

type ParentAttempt struct {
	StepID     string
	AttemptID  string
	Generation int64
	LeaseOwner string
	LeaseToken string
}

type RegisterRootInput struct {
	UserID   string
	RunID    string
	Grant    agentbroker.CapabilityGrant
	Registry agentbroker.ToolRegistry
	Model    ModelBinding
}

type ChildProposal struct {
	UserID         string
	IdempotencyKey string
	ParentRunID    string
	ParentAttempt  ParentAttempt
	Model          ModelBinding
	Grant          agentbroker.CapabilityGrant
	RequestedTools []string
	Catalog        []agentbroker.ToolDefinition
	Steps          []agentorchestrator.StepPlan
}

type Derivation struct {
	Parent                   Authority
	ChildRunID               string
	ChildSnapshotID          string
	ChildSnapshot            json.RawMessage
	ChildSnapshotFingerprint string
	ChildGrant               agentbroker.CapabilityGrant
	ChildGrantFingerprint    string
	ChildRegistry            agentbroker.ToolRegistry
	Reservation              agentbroker.Budget
	Steps                    []agentorchestrator.StepPlan
	ScopeKeys                []string
	EventIDs                 []string
	RequestFingerprint       string
}

type EnqueueResult struct {
	Authority Authority
	Created   bool
}

type LaunchAdmissionInput struct {
	UserID              string
	RunID               string
	AttemptID           string
	Generation          int64
	LeaseOwner          string
	LeaseToken          string
	SnapshotFingerprint string
	GrantFingerprint    string
	RegistryFingerprint string
	RegistryTools       []string
}

type ReapTarget struct {
	ReapID      string
	ParentRunID string
	ChildRunID  string
	AttemptID   string
	Generation  int64
	LeaseOwner  string
	Mode        string
	State       string
	RetryCount  int
}

type CascadeInput struct {
	UserID      string
	ParentRunID string
	Mode        string
	ActorType   string
	ActorID     string
	ReasonCode  string
}

type SettleInput struct {
	UserID     string
	ChildRunID string
	Usage      agentbroker.Budget
	Outcome    string
}

type Repository interface {
	RegisterRoot(context.Context, Authority) (Authority, bool, error)
	GetAuthority(context.Context, string, string) (Authority, error)
	EnqueueChild(context.Context, Derivation, ChildProposal) (Authority, bool, error)
	AdmitLaunch(context.Context, LaunchAdmissionInput, string) error
	Settle(context.Context, SettleInput, string) (bool, error)
	Cascade(context.Context, CascadeInput) ([]ReapTarget, error)
	Recover(context.Context, int) (int, error)
	ListPendingReaps(context.Context, int) ([]ReapTarget, error)
	CompleteReap(context.Context, string, bool, string) error
}

type Reaper interface {
	Reap(context.Context, ReapTarget) error
}
