// Package agentbroker owns the G20.4 Agent Tool Registry and durable
// Prepare/approval/Commit control boundary. It deliberately has no HTTP or Chat
// wiring.
package agentbroker

import "errors"

var (
	ErrInvalidInput        = errors.New("agent Broker input is invalid")
	ErrDatabaseRequired    = errors.New("agent Broker database is required")
	ErrNotFound            = errors.New("agent Broker record was not found")
	ErrGrantDenied         = errors.New("GRANT_DENIED")
	ErrLeaseStale          = errors.New("LEASE_STALE")
	ErrSnapshotMismatch    = errors.New("SNAPSHOT_MISMATCH")
	ErrKillSwitchActive    = errors.New("KILL_SWITCH_ACTIVE")
	ErrBudgetExhausted     = errors.New("BUDGET_EXHAUSTED")
	ErrApprovalRequired    = errors.New("APPROVAL_REQUIRED")
	ErrApprovalDenied      = errors.New("APPROVAL_DENIED")
	ErrReplayDetected      = errors.New("REPLAY_DETECTED")
	ErrIntentExpired       = errors.New("INTENT_EXPIRED")
	ErrInvalidTransition   = errors.New("INVALID_TRANSITION")
	ErrOutcomeUnknown      = errors.New("OUTCOME_UNKNOWN")
	ErrEgressDenied        = errors.New("EGRESS_DENIED")
	ErrSecretDenied        = errors.New("SECRET_DENIED")
	ErrProjectConflict     = errors.New("PROJECT_CONFLICT")
	ErrArtifactDenied      = errors.New("ARTIFACT_DENIED")
	ErrExecutorUnavailable = errors.New("EXECUTOR_UNAVAILABLE")
)
