package agentcron

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
)

const (
	SchemaVersion     = "neo.cron-template/v1"
	CalculatorVersion = "robfig-cron/v3.0.1+go-tzdata"

	AutomationReadOnly       = "automation_read_only"
	AutomationBrokeredEffect = "automation_brokered_effect"

	MissedSkip     = "skip"
	MissedFireOnce = "fire_once"
	MissedCatchUp  = "catch_up"

	OverlapSkip      = "skip"
	OverlapBufferOne = "buffer_one"
	OverlapAllow     = "allow"

	TemplateActive  = "active"
	TemplatePaused  = "paused"
	TemplateDeleted = "deleted"

	TriggerPending  = "pending"
	TriggerClaimed  = "claimed"
	TriggerEnqueued = "enqueued"
	TriggerSkipped  = "skipped"
	TriggerFailed   = "failed"
)

var (
	ErrDatabaseRequired    = errors.New("agent Cron database is required")
	ErrInvalidInput        = errors.New("agent Cron input is invalid")
	ErrNotFound            = errors.New("agent Cron record was not found")
	ErrRevisionConflict    = errors.New("REVISION_CONFLICT")
	ErrStaleClaim          = errors.New("STALE_CLAIM")
	ErrStaleTemplate       = errors.New("STALE_TEMPLATE")
	ErrOwnerRevoked        = errors.New("OWNER_REVOKED")
	ErrSkillRevoked        = errors.New("SKILL_REVOKED")
	ErrGrantRevoked        = errors.New("GRANT_REVOKED")
	ErrApprovalRevoked     = errors.New("APPROVAL_REVOKED")
	ErrExpired             = errors.New("TEMPLATE_EXPIRED")
	ErrKillSwitchActive    = errors.New("KILL_SWITCH_ACTIVE")
	ErrIdempotencyConflict = errors.New("IDEMPOTENCY_CONFLICT")
)

type InputBinding struct {
	Ref         string `json:"ref"`
	Fingerprint string `json:"fingerprint"`
}

type ScheduleBinding struct {
	Expression string `json:"expression"`
	Timezone   string `json:"timezone"`
	Calculator string `json:"calculator"`
}

type ModelBinding struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

type SkillBinding struct {
	InstallationID           string `json:"installationId"`
	AdmissionID              string `json:"admissionId"`
	PackageFingerprint       string `json:"packageFingerprint"`
	RuntimeBundleFingerprint string `json:"runtimeBundleFingerprint"`
}

type GrantBinding struct {
	GrantID             string                   `json:"grantId"`
	GrantFingerprint    string                   `json:"grantFingerprint"`
	RegistryFingerprint string                   `json:"registryFingerprint"`
	IssuedAt            time.Time                `json:"issuedAt"`
	ExpiresAt           time.Time                `json:"expiresAt"`
	Capabilities        []agentbroker.Capability `json:"capabilities"`
}

type AutomationBinding struct {
	Class      string `json:"class"`
	ApprovalID string `json:"approvalId"`
}

type SchedulingPolicies struct {
	Missed               string `json:"missed"`
	CatchupWindowSeconds int    `json:"catchupWindowSeconds"`
	MaxCatchupRuns       int    `json:"maxCatchupRuns"`
	Overlap              string `json:"overlap"`
	MaxAttempts          int    `json:"maxAttempts"`
	RetryBackoffSeconds  int    `json:"retryBackoffSeconds"`
}

type StepBinding struct {
	Kind string `json:"kind"`
}

type TemplateSpec struct {
	SchemaVersion string                    `json:"schemaVersion"`
	Owner         agentbroker.Subject       `json:"owner"`
	Input         InputBinding              `json:"input"`
	Schedule      ScheduleBinding           `json:"schedule"`
	Model         ModelBinding              `json:"model"`
	Budget        agentbroker.Budget        `json:"budget"`
	Skill         SkillBinding              `json:"skill"`
	Grant         GrantBinding              `json:"grant"`
	Egress        agentbroker.EgressPolicy  `json:"egress"`
	Secrets       []agentbroker.SecretGrant `json:"secrets"`
	Automation    AutomationBinding         `json:"automation"`
	Policies      SchedulingPolicies        `json:"policies"`
	Steps         []StepBinding             `json:"steps"`
	ScopeKeys     []string                  `json:"scopeKeys"`
}

type Approval struct {
	ActorType  string
	ActorID    string
	ReasonCode string
}

type CreateInput struct {
	TemplateID       string
	ExpectedRevision int64
	Spec             TemplateSpec
	Approval         Approval
	ActivateAt       time.Time
}

type Template struct {
	ID                  string       `json:"id"`
	UserID              string       `json:"-"`
	CurrentRevision     int64        `json:"currentRevision"`
	RevisionFingerprint string       `json:"revisionFingerprint"`
	State               string       `json:"state"`
	NextTriggerAt       *time.Time   `json:"nextTriggerAt,omitempty"`
	CreatedAt           time.Time    `json:"createdAt"`
	UpdatedAt           time.Time    `json:"updatedAt"`
	Spec                TemplateSpec `json:"spec"`
}

type LifecycleInput struct {
	TemplateID       string
	UserID           string
	ExpectedRevision int64
	To               string
	ActorType        string
	ActorID          string
	ReasonCode       string
}

type ClaimRequest struct {
	Owner         string
	Now           time.Time
	LeaseDuration time.Duration
	Limit         int
}

type DueClaim struct {
	TemplateID          string
	UserID              string
	Revision            int64
	RevisionFingerprint string
	NextTriggerAt       time.Time
	ClaimGeneration     int64
	ClaimOwner          string
	ClaimExpiresAt      time.Time
	Spec                TemplateSpec
}

type OccurrenceDecision struct {
	TriggerID             string    `json:"triggerId"`
	AuditEventID          string    `json:"auditEventId"`
	ScheduledFor          time.Time `json:"scheduledFor"`
	OccurrenceFingerprint string    `json:"occurrenceFingerprint"`
	State                 string    `json:"state"`
	ReasonCode            string    `json:"reasonCode"`
	MissedCount           int       `json:"missedCount"`
	CountTruncated        bool      `json:"countTruncated"`
}

type AdvanceInput struct {
	Claim         DueClaim
	ObservedAt    time.Time
	NextTriggerAt time.Time
	Decisions     []OccurrenceDecision
}

type Trigger struct {
	ID                    string
	TemplateID            string
	UserID                string
	Revision              int64
	RevisionFingerprint   string
	ScheduledFor          time.Time
	OccurrenceFingerprint string
	State                 string
	ReasonCode            string
	RunID                 string
	RetryCount            int
	NextAttemptAt         time.Time
	ClaimGeneration       int64
	ClaimOwner            string
	ClaimExpiresAt        *time.Time
	Spec                  TemplateSpec
}

type EnqueueEnvelope struct {
	Trigger             Trigger
	RunID               string
	SnapshotID          string
	SnapshotFingerprint string
	RequestFingerprint  string
	CanonicalSnapshot   json.RawMessage
	Steps               json.RawMessage
	EventIDs            []string
	AuditEventID        string
}

type TriggerResult struct {
	TriggerID  string
	State      string
	ReasonCode string
	RunID      string
	Created    bool
}

type ReleaseInput struct {
	TriggerID       string
	ClaimOwner      string
	ClaimGeneration int64
	ErrorCode       string
	RetryAt         time.Time
	AuditEventID    string
}

type RevokeApprovalInput struct {
	RevocationID string
	UserID       string
	ApprovalID   string
	ActorType    string
	ActorID      string
	ReasonCode   string
	AuditEventID string
}

type CycleResult struct {
	ClaimedTemplates int
	Materialized     int
	ClaimedTriggers  int
	Enqueued         int
	Skipped          int
	Failed           int
}

type CleanupResult struct {
	CursorClaimsReclaimed  int
	TriggerClaimsReclaimed int
	TriggersPruned         int
	AuditsPruned           int
	TemplatesPruned        int
}

type Repository interface {
	CreateRevision(context.Context, preparedRevision) (Template, bool, error)
	GetTemplate(context.Context, string, string) (Template, error)
	SetLifecycle(context.Context, preparedLifecycle) (Template, error)
	RevokeApproval(context.Context, RevokeApprovalInput) (bool, error)
	ClaimDue(context.Context, ClaimRequest) ([]DueClaim, error)
	Advance(context.Context, AdvanceInput) ([]Trigger, error)
	ClaimTriggers(context.Context, ClaimRequest) ([]Trigger, error)
	EnqueueTrigger(context.Context, EnqueueEnvelope) (TriggerResult, error)
	ReleaseTrigger(context.Context, ReleaseInput) error
	Reconcile(context.Context, time.Time, int) (CleanupResult, error)
	Prune(context.Context, time.Time, int) (CleanupResult, error)
}

type preparedRevision struct {
	TemplateID          string
	ExpectedRevision    int64
	Spec                TemplateSpec
	CanonicalSpec       json.RawMessage
	RevisionFingerprint string
	NextTriggerAt       time.Time
	Approval            Approval
	AuditEventID        string
}

type preparedLifecycle struct {
	LifecycleInput
	NextTriggerAt *time.Time
	MissedCount   int
	AuditEventID  string
}
