package agentproductcanary

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalid     = errors.New("PRODUCT_CANARY_INVALID")
	ErrUnavailable = errors.New("PRODUCT_CANARY_UNAVAILABLE")
)

type Activation struct {
	ID                       string
	ReleaseCommit            string
	PolicyRevision           int64
	AdmissionID              string
	PackageFingerprint       string
	RuntimeBundleFingerprint string
	PlanFingerprint          string
	MaxRequests              int
	ValidFrom, ValidUntil    time.Time
	Enabled                  bool
}

type Request struct {
	ID                       string
	ActivationID             string
	UserID                   string
	PolicyRevision           int64
	OptGeneration            int64
	PackageFingerprint       string
	RuntimeBundleFingerprint string
	PlanFingerprint          string
	RequestFingerprint       string
	State                    string
	ClaimGeneration          int64
	ClaimOwner               string
	ClaimExpiresAt           time.Time
	FailureCount             int
}

type ExecutionResult struct {
	RunID, AttemptID    string
	SnapshotFingerprint string
	ReceiptFingerprint  string
}

type Receipt struct {
	RequestID, ActivationID, RunID, AttemptID string
	SnapshotFingerprint, PlanFingerprint      string
	ReceiptFingerprint, Outcome               string
	CreatedAt                                 time.Time
}

type HealthStatus struct {
	StaleClaims, PendingTerminalizations int
}

type Repository interface {
	GetActivation(context.Context, string) (Activation, error)
	Health(context.Context, string, time.Time) (HealthStatus, error)
	Claim(context.Context, string, string, time.Time, time.Duration, int) ([]Request, error)
	Complete(context.Context, Request, ExecutionResult) (Receipt, error)
	Release(context.Context, Request, string, bool) (bool, error)
	Reconcile(context.Context, string, time.Time, int) (int, error)
}

type Executor interface {
	Health(context.Context) error
	Execute(context.Context, Request) (ExecutionResult, error)
}

type Gate interface {
	Verify(time.Time) error
}
