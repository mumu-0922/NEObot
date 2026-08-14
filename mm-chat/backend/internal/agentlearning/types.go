package agentlearning

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/skillsupply"
)

const (
	SchemaVersion = "neo.skill-draft/v1"

	StateQuarantined = "quarantined"
	StateChecking    = "checking"
	StateReviewable  = "reviewable"
	StateCheckFailed = "check_failed"
	StateRejected    = "rejected"
	StatePromoted    = "promoted"

	CheckStatic     = "static"
	CheckIsolation  = "isolation"
	CheckEvaluation = "evaluation"

	CheckPassed = "passed"
	CheckFailed = "failed"

	EvidenceSourcePackage = "source_package"
	EvidenceRunEvent      = "run_event"
)

var (
	ErrDatabaseRequired    = errors.New("agent learning database is required")
	ErrObjectStoreRequired = errors.New("agent learning object store is required")
	ErrLearningDisabled    = errors.New("LEARNING_DISABLED")
	ErrAdministratorNeeded = errors.New("agent learning administrator access is required")
	ErrInvalidInput        = errors.New("agent learning input is invalid")
	ErrNotFound            = errors.New("agent learning Draft was not found")
	ErrSourceInvalid       = errors.New("SOURCE_RUN_INVALID")
	ErrSourceDrift         = errors.New("SOURCE_DRIFT")
	ErrAuthorityWidened    = errors.New("AUTHORITY_WIDENED")
	ErrPackageExists       = errors.New("PACKAGE_ALREADY_EXISTS")
	ErrStaleClaim          = errors.New("STALE_CLAIM")
	ErrRevisionConflict    = errors.New("REVISION_CONFLICT")
	ErrChecksIncomplete    = errors.New("CHECKS_INCOMPLETE")
	ErrPromotionDenied     = errors.New("PROMOTION_DENIED")
	ErrKillSwitchActive    = errors.New("KILL_SWITCH_ACTIVE")
	ErrObjectDrift         = errors.New("DRAFT_OBJECT_DRIFT")
	ErrCheckUnavailable    = errors.New("CHECK_UNAVAILABLE")
)

type EvidenceRef struct {
	Kind        string   `json:"kind"`
	Ref         string   `json:"ref"`
	Fingerprint string   `json:"fingerprint"`
	Paths       []string `json:"paths"`
}

type TestFile struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Fingerprint string `json:"fingerprint"`
}

type DraftSpec struct {
	SchemaVersion              string        `json:"schemaVersion"`
	SourceRunID                string        `json:"sourceRunId"`
	SourceSnapshotID           string        `json:"sourceSnapshotId"`
	SourceSnapshotFingerprint  string        `json:"sourceSnapshotFingerprint"`
	BasePackageFingerprint     string        `json:"basePackageFingerprint"`
	ProposedPackageFingerprint string        `json:"proposedPackageFingerprint"`
	RuntimeBundleFingerprint   string        `json:"runtimeBundleFingerprint"`
	SBOMFingerprint            string        `json:"sbomFingerprint"`
	ArchiveFingerprint         string        `json:"archiveFingerprint"`
	EvidenceFingerprint        string        `json:"evidenceFingerprint"`
	TestFingerprint            string        `json:"testFingerprint"`
	Name                       string        `json:"name"`
	Version                    string        `json:"version"`
	ArchiveBytes               int64         `json:"archiveBytes"`
	Evidence                   []EvidenceRef `json:"evidence"`
	Tests                      []TestFile    `json:"tests"`
	ChangedPaths               []string      `json:"changedPaths"`
}

type ProposeInput struct {
	UserID                 string
	SourceRunID            string
	BasePackageFingerprint string
	Archive                []byte
	Evidence               []EvidenceRef
}

type Draft struct {
	ID                   string
	UserID               string
	DraftFingerprint     string
	Spec                 DraftSpec
	State                string
	Revision             int64
	CheckAttempts        int
	CheckGeneration      int64
	CheckOwner           string
	CheckExpiresAt       *time.Time
	AdmissionID          string
	PromotedFingerprint  string
	DraftObjectKey       string
	BasePackageObjectKey string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ObjectDeletedAt      *time.Time
}

type BasePackage struct {
	Package skillsupply.PackageVersion
}

type SourceRun struct {
	RunID               string
	UserID              string
	SnapshotID          string
	SnapshotFingerprint string
	State               string
	Depth               int
}

type ClaimRequest struct {
	Owner         string
	Now           time.Time
	LeaseDuration time.Duration
	Limit         int
}

type CheckClaim struct {
	Draft
	ClaimGeneration int64
	ClaimOwner      string
	ClaimExpiresAt  time.Time
}

type CheckInput struct {
	Draft   Draft
	Archive []byte
}

type CheckResult struct {
	Status           string
	ReasonCode       string
	SuiteFingerprint string
	DurationMillis   int64
	Metrics          map[string]int64
}

type CheckReceipt struct {
	ID                  string           `json:"id"`
	Kind                string           `json:"kind"`
	Status              string           `json:"status"`
	ReasonCode          string           `json:"reasonCode"`
	SuiteFingerprint    string           `json:"suiteFingerprint"`
	EvidenceFingerprint string           `json:"evidenceFingerprint"`
	DurationMillis      int64            `json:"durationMillis"`
	Metrics             map[string]int64 `json:"metrics"`
}

type Checker interface {
	Check(context.Context, CheckInput) (CheckResult, error)
}

type ReviewInput struct {
	DraftID                    string
	ExpectedRevision           int64
	DraftFingerprint           string
	ProposedPackageFingerprint string
	ReasonCode                 string
}

type Promotion struct {
	DraftID            string
	AdmissionID        string
	PackageFingerprint string
	Created            bool
}

type CleanupClaim struct {
	DraftID           string
	ObjectKey         string
	ObjectFingerprint string
	Generation        int64
	Owner             string
	ExpiresAt         time.Time
	Attempts          int
}

type CleanupResult struct {
	Completed int
	Failed    int
}

type ReconcileResult struct {
	ChecksReclaimed  int
	CleanupReclaimed int
}

type PruneResult struct {
	DraftsPruned int
	AuditsPruned int
}

type FileDiff struct {
	Path              string
	Change            string
	BeforeFingerprint string
	AfterFingerprint  string
	BeforeSize        int64
	AfterSize         int64
	BeforeText        string
	AfterText         string
	Binary            bool
}

type preparedDraft struct {
	Draft
	EvidenceJSON []byte
	TestsJSON    []byte
}

type preparedPromotion struct {
	ReviewInput
	AdministratorID  string
	CandidateID      string
	DecisionID       string
	SourceRef        string
	SourceObjectKey  string
	Package          skillsupply.PackageVersion
	AllowedToolsJSON []byte
	CapabilitiesJSON []byte
}

type Repository interface {
	GetSourceRun(context.Context, string, string, string) (SourceRun, error)
	GetBasePackage(context.Context, string) (BasePackage, error)
	CreateDraft(context.Context, preparedDraft) (Draft, bool, error)
	GetDraft(context.Context, string, string) (Draft, error)
	ClaimChecks(context.Context, ClaimRequest) ([]CheckClaim, error)
	CompleteChecks(context.Context, CheckClaim, []CheckReceipt) (Draft, error)
	ReleaseCheck(context.Context, CheckClaim, string, time.Time) (bool, error)
	Reject(context.Context, string, ReviewInput, string, string) (Draft, bool, error)
	ReplayPromotion(context.Context, string, ReviewInput) (Promotion, error)
	Promote(context.Context, preparedPromotion) (Promotion, error)
	ClaimCleanup(context.Context, ClaimRequest) ([]CleanupClaim, error)
	CompleteCleanup(context.Context, CleanupClaim) error
	ReleaseCleanup(context.Context, CleanupClaim, string, time.Time) (bool, error)
	Reconcile(context.Context, time.Time, int) (ReconcileResult, error)
	Prune(context.Context, time.Time, int) (PruneResult, error)
}

func nonNilMetrics(value map[string]int64) map[string]int64 {
	if value == nil {
		return map[string]int64{}
	}
	return value
}

func rawJSON(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}
