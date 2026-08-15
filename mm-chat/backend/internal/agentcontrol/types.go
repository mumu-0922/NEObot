package agentcontrol

import (
	"context"
	"errors"
	"io"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentcron"
	"neo-chat/mm-chat/backend/internal/agentlearning"
	"neo-chat/mm-chat/backend/internal/storage"
)

const (
	RuntimeHeldReason        = "ISOLATION_UNAVAILABLE"
	RuntimeLocalDirectReason = "LOCAL_DIRECT_EXECUTION"

	ShadowModeSynthetic = "synthetic"
	ShadowModeReadOnly  = "read_only"
)

var (
	ErrDatabaseRequired     = errors.New("agent product database is required")
	ErrInvalidInput         = errors.New("agent product input is invalid")
	ErrNotFound             = errors.New("agent product record was not found")
	ErrAdministratorNeeded  = errors.New("agent product administrator access is required")
	ErrIsolationUnavailable = errors.New(RuntimeHeldReason)
	ErrRevisionConflict     = errors.New("REVISION_CONFLICT")
	ErrGenerationStale      = errors.New("GENERATION_STALE")
	ErrFingerprintDrift     = errors.New("FINGERPRINT_DRIFT")
	ErrBudgetExceeded       = errors.New("BUDGET_EXCEEDED")
	ErrKillSwitchActive     = errors.New("KILL_SWITCH_ACTIVE")
	ErrRunCancelBlocked     = errors.New("RUN_CANCEL_BLOCKED")
)

type RuntimeStatus struct {
	State          string `json:"state"`
	ReasonCode     string `json:"reasonCode"`
	Executable     bool   `json:"executable"`
	ProductCanary  bool   `json:"productCanary"`
	Scheduler      bool   `json:"scheduler"`
	LearningWorker bool   `json:"learningWorker"`
}

type CenterStatus struct {
	IsAdministrator bool           `json:"isAdministrator"`
	Runtime         RuntimeStatus  `json:"runtime"`
	Shadow          ShadowSnapshot `json:"shadow"`
}

type RunSummary struct {
	ID                  string     `json:"id"`
	State               string     `json:"state"`
	SnapshotFingerprint string     `json:"snapshotFingerprint"`
	RequestFingerprint  string     `json:"requestFingerprint"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	TerminalAt          *time.Time `json:"terminalAt,omitempty"`
	CancellationState   string     `json:"cancellationState,omitempty"`
	CancellationMode    string     `json:"cancellationMode,omitempty"`
	CancellationReason  string     `json:"cancellationReason,omitempty"`
}

type RunStep struct {
	ID                string     `json:"id"`
	Ordinal           int        `json:"ordinal"`
	Kind              string     `json:"kind"`
	State             string     `json:"state"`
	CurrentGeneration int64      `json:"currentGeneration"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	TerminalAt        *time.Time `json:"terminalAt,omitempty"`
}

type RunAttempt struct {
	ID             string     `json:"id"`
	StepID         string     `json:"stepId"`
	Generation     int64      `json:"generation"`
	State          string     `json:"state"`
	LeaseExpiresAt time.Time  `json:"leaseExpiresAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	TerminalAt     *time.Time `json:"terminalAt,omitempty"`
}

type RunEvent struct {
	ID             string     `json:"id"`
	StepID         string     `json:"stepId,omitempty"`
	AttemptID      string     `json:"attemptId,omitempty"`
	Sequence       int64      `json:"sequence"`
	OccurredAt     time.Time  `json:"occurredAt"`
	Kind           string     `json:"kind"`
	Entity         string     `json:"entity"`
	From           string     `json:"from,omitempty"`
	To             string     `json:"to"`
	ActorType      string     `json:"actorType"`
	ReasonCode     string     `json:"reasonCode"`
	Generation     int64      `json:"generation,omitempty"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt,omitempty"`
}

type Approval struct {
	IntentID             string     `json:"intentId"`
	IntentFingerprint    string     `json:"intentFingerprint"`
	ToolIdentity         string     `json:"toolIdentity"`
	Capability           string     `json:"capability"`
	Action               string     `json:"action"`
	ArgumentsFingerprint string     `json:"argumentsFingerprint"`
	ApprovalClass        string     `json:"approvalClass"`
	ApprovalRevision     int64      `json:"approvalRevision"`
	State                string     `json:"state"`
	ExpiresAt            time.Time  `json:"expiresAt"`
	ApprovedAt           *time.Time `json:"approvedAt,omitempty"`
	TerminalAt           *time.Time `json:"terminalAt,omitempty"`
	ErrorCode            string     `json:"errorCode,omitempty"`
}

type ChildRun struct {
	RunID                    string    `json:"runId"`
	ParentRunID              string    `json:"parentRunId,omitempty"`
	Depth                    int       `json:"depth"`
	State                    string    `json:"state"`
	PackageFingerprint       string    `json:"packageFingerprint"`
	RuntimeBundleFingerprint string    `json:"runtimeBundleFingerprint"`
	GrantFingerprint         string    `json:"grantFingerprint"`
	RegistryFingerprint      string    `json:"registryFingerprint"`
	ExpiresAt                time.Time `json:"expiresAt"`
	CreatedAt                time.Time `json:"createdAt"`
}

type Artifact struct {
	ID          string    `json:"id"`
	AttemptID   string    `json:"attemptId"`
	Generation  int64     `json:"generation"`
	Name        string    `json:"name"`
	MediaType   string    `json:"mediaType"`
	Size        int64     `json:"size"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"createdAt"`
	DownloadURL string    `json:"downloadUrl"`
}

type ArtifactSource struct {
	Artifact
	ObjectKey string `json:"-"`
}

type RunDetail struct {
	Run       RunSummary   `json:"run"`
	Steps     []RunStep    `json:"steps"`
	Attempts  []RunAttempt `json:"attempts"`
	Events    []RunEvent   `json:"events"`
	Approvals []Approval   `json:"approvals"`
	Children  []ChildRun   `json:"children"`
	Artifacts []Artifact   `json:"artifacts"`
}

type CancelRunInput struct {
	UserID              string
	RunID               string
	ExpectedState       string
	SnapshotFingerprint string
	CancellationID      string
	Mode                string
	ReasonCode          string
}

type RunCancellation struct {
	ID                  string    `json:"id"`
	RunID               string    `json:"runId"`
	Mode                string    `json:"mode"`
	State               string    `json:"state"`
	ExpectedRunState    string    `json:"expectedRunState"`
	SnapshotFingerprint string    `json:"snapshotFingerprint"`
	ReasonCode          string    `json:"reasonCode"`
	CreatedAt           time.Time `json:"createdAt"`
}

type ScheduleSummary struct {
	ID                       string     `json:"id"`
	State                    string     `json:"state"`
	CurrentRevision          int64      `json:"currentRevision"`
	RevisionFingerprint      string     `json:"revisionFingerprint"`
	NextTriggerAt            *time.Time `json:"nextTriggerAt,omitempty"`
	ScheduleExpression       string     `json:"scheduleExpression"`
	Timezone                 string     `json:"timezone"`
	Calculator               string     `json:"calculator"`
	AutomationClass          string     `json:"automationClass"`
	ApprovalID               string     `json:"approvalId"`
	PackageFingerprint       string     `json:"packageFingerprint"`
	RuntimeBundleFingerprint string     `json:"runtimeBundleFingerprint"`
	MissedPolicy             string     `json:"missedPolicy"`
	OverlapPolicy            string     `json:"overlapPolicy"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type DraftSummary struct {
	ID                         string                       `json:"id"`
	OwnerUserID                string                       `json:"ownerUserId"`
	State                      string                       `json:"state"`
	Revision                   int64                        `json:"revision"`
	DraftFingerprint           string                       `json:"draftFingerprint"`
	BasePackageFingerprint     string                       `json:"basePackageFingerprint"`
	ProposedPackageFingerprint string                       `json:"proposedPackageFingerprint"`
	RuntimeBundleFingerprint   string                       `json:"runtimeBundleFingerprint"`
	Name                       string                       `json:"name"`
	Version                    string                       `json:"version"`
	CheckGeneration            int64                        `json:"checkGeneration"`
	CheckAttempts              int                          `json:"checkAttempts"`
	AdmissionID                string                       `json:"admissionId,omitempty"`
	CreatedAt                  time.Time                    `json:"createdAt"`
	UpdatedAt                  time.Time                    `json:"updatedAt"`
	ObjectDeletedAt            *time.Time                   `json:"objectDeletedAt,omitempty"`
	Checks                     []agentlearning.CheckReceipt `json:"checks"`
}

type DraftReviewResult struct {
	Decision  string                   `json:"decision"`
	Draft     *DraftSummary            `json:"draft,omitempty"`
	Promotion *agentlearning.Promotion `json:"promotion,omitempty"`
	Created   bool                     `json:"created"`
}

type ShadowPolicy struct {
	Revision                 int64      `json:"revision"`
	Enabled                  bool       `json:"enabled"`
	Mode                     string     `json:"mode"`
	AdmissionID              string     `json:"admissionId,omitempty"`
	PackageFingerprint       string     `json:"packageFingerprint,omitempty"`
	RuntimeBundleFingerprint string     `json:"runtimeBundleFingerprint,omitempty"`
	CohortBasisPoints        int        `json:"cohortBasisPoints"`
	MaxObservations          int        `json:"maxObservations"`
	MaxErrors                int        `json:"maxErrors"`
	StartsAt                 *time.Time `json:"startsAt,omitempty"`
	ExpiresAt                *time.Time `json:"expiresAt,omitempty"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type ShadowOptIn struct {
	OptedIn        bool      `json:"optedIn"`
	Generation     int64     `json:"generation"`
	PolicyRevision int64     `json:"policyRevision"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ShadowSnapshot struct {
	Policy           ShadowPolicy        `json:"policy"`
	OptIn            ShadowOptIn         `json:"optIn"`
	CohortSelected   bool                `json:"cohortSelected"`
	Eligible         bool                `json:"eligible"`
	Effective        bool                `json:"effective"`
	HeldReasonCode   string              `json:"heldReasonCode"`
	ObservationCount int                 `json:"observationCount"`
	ErrorCount       int                 `json:"errorCount"`
	ProductCanary    ProductCanaryStatus `json:"productCanary"`
}

type ProductCanaryStatus struct {
	ActivationID      string `json:"activationId,omitempty"`
	PlanFingerprint   string `json:"planFingerprint,omitempty"`
	RemainingRequests int    `json:"remainingRequests"`
}

type EnqueueProductCanaryInput struct {
	UserID                 string
	ExpectedPolicyRevision int64
	ExpectedGeneration     int64
	RequestID              string
	RequestFingerprint     string
}

type ProductCanaryRequest struct {
	ID                 string     `json:"id"`
	ActivationID       string     `json:"activationId"`
	State              string     `json:"state"`
	PolicyRevision     int64      `json:"policyRevision"`
	OptGeneration      int64      `json:"optGeneration"`
	RequestFingerprint string     `json:"requestFingerprint"`
	FailureCount       int        `json:"failureCount"`
	ErrorCode          string     `json:"errorCode,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	TerminalAt         *time.Time `json:"terminalAt,omitempty"`
}

type UpdateShadowPolicyInput struct {
	AdministratorID          string
	ExpectedRevision         int64
	Enabled                  bool
	Mode                     string
	AdmissionID              string
	PackageFingerprint       string
	RuntimeBundleFingerprint string
	CohortBasisPoints        int
	MaxObservations          int
	MaxErrors                int
	StartsAt                 *time.Time
	ExpiresAt                *time.Time
	ReasonCode               string
}

type SetShadowOptInInput struct {
	UserID             string
	ExpectedGeneration int64
	PolicyRevision     int64
	OptedIn            bool
	ReasonCode         string
}

type ShadowRequest struct {
	UserID                   string
	PolicyRevision           int64
	Generation               int64
	AdmissionID              string
	PackageFingerprint       string
	RuntimeBundleFingerprint string
	Mode                     string
}

type ShadowMeasurement struct {
	Outcome       string
	ReasonCode    string
	LatencyBucket string
	RunCount      int
	StepCount     int
	AttemptCount  int
	ErrorCount    int
}

type ShadowObservation struct {
	ID                       string    `json:"id"`
	PolicyRevision           int64     `json:"policyRevision"`
	Generation               int64     `json:"generation"`
	AdmissionID              string    `json:"admissionId"`
	PackageFingerprint       string    `json:"packageFingerprint"`
	RuntimeBundleFingerprint string    `json:"runtimeBundleFingerprint"`
	Mode                     string    `json:"mode"`
	Outcome                  string    `json:"outcome"`
	ReasonCode               string    `json:"reasonCode"`
	LatencyBucket            string    `json:"latencyBucket"`
	RunCount                 int       `json:"runCount"`
	StepCount                int       `json:"stepCount"`
	AttemptCount             int       `json:"attemptCount"`
	ErrorCount               int       `json:"errorCount"`
	CreatedAt                time.Time `json:"createdAt"`
}

type ShadowAdapter interface {
	Observe(context.Context, ShadowRequest) (ShadowMeasurement, error)
}

type Repository interface {
	ListRuns(context.Context, string, int) ([]RunSummary, error)
	GetRun(context.Context, string, string) (RunDetail, error)
	GetArtifact(context.Context, string, string, string) (ArtifactSource, error)
	CancelRun(context.Context, CancelRunInput) (RunCancellation, error)
	ListSchedules(context.Context, string, int) ([]ScheduleSummary, error)
	ListDrafts(context.Context, int) ([]DraftSummary, error)
	GetDraft(context.Context, string) (DraftSummary, error)
	GetShadow(context.Context, string) (ShadowSnapshot, error)
	EnqueueProductCanary(context.Context, EnqueueProductCanaryInput) (ProductCanaryRequest, error)
	UpdateShadowPolicy(context.Context, UpdateShadowPolicyInput) (ShadowPolicy, error)
	SetShadowOptIn(context.Context, SetShadowOptInInput) (ShadowOptIn, error)
	RegisterShadowBoot(context.Context, string) (int64, error)
	AppendShadowObservation(context.Context, string, ShadowRequest, ShadowMeasurement) (ShadowObservation, error)
}

type ArtifactStore interface {
	Get(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error)
}

type CronService interface {
	GetTemplate(context.Context, string, string) (agentcron.Template, error)
	CreateRevision(context.Context, agentcron.CreateInput) (agentcron.Template, bool, error)
	SetLifecycle(context.Context, agentcron.LifecycleInput) (agentcron.Template, error)
	Resume(context.Context, agentcron.Template, agentcron.Approval) (agentcron.Template, error)
}

type BrokerService interface {
	DecideApproval(context.Context, agentbroker.ApprovalInput) (agentbroker.PreparedIntent, error)
	Cancel(context.Context, agentbroker.CancelInput) (agentbroker.PreparedIntent, error)
}

type LearningService interface {
	GetDiff(context.Context, string, string) ([]agentlearning.FileDiff, error)
	Reject(context.Context, string, agentlearning.ReviewInput) (agentlearning.Draft, bool, error)
	Promote(context.Context, string, agentlearning.ReviewInput) (agentlearning.Promotion, error)
}
