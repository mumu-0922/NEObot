package agentlearningworker

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
)

type BeginInput struct {
	DraftID             string
	ClaimOwner          string
	CheckGeneration     int64
	Kind                string
	Attempt             agentrunner.AttemptRef
	SnapshotFingerprint string
	Now                 time.Time
	LeaseDuration       time.Duration
}

type BeginResult struct {
	AttemptID string
	State     string
	Created   bool
}

type RunnerResult struct {
	ArtifactName        string
	ArtifactBytes       int64
	ArtifactFingerprint string
	Payload             json.RawMessage
}

// InventoryAttempt is the content-free durable Runner-check projection used
// after a worker restart. Lease tokens are deliberately not persisted; stale
// Sandboxes are removed only through the caller-scoped list/reconcile path
// after both the attempt lease and every issued authority have expired.
type InventoryAttempt struct {
	Attempt             agentrunner.AttemptIdentity
	DraftID             string
	UserID              string
	CheckGeneration     int64
	Kind                string
	LeaseOwner          string
	SnapshotFingerprint string
	State               string
	SandboxID           string
	SpecFingerprint     string
	ProbeFingerprint    string
	CleanupState        string
	CleanupAttempts     int
	ErrorCode           string
	LeaseExpiresAt      time.Time
	AuthorityExpiresAt  *time.Time
}

type PostgresRepository struct {
	db           *sql.DB
	activationID string
}

func NewPostgresRepository(database *sql.DB, activationID string) *PostgresRepository {
	return &PostgresRepository{db: database, activationID: strings.TrimSpace(activationID)}
}

func (repository *PostgresRepository) VerifyTarget(ctx context.Context, plan Plan) error {
	if repository == nil || repository.db == nil || ValidatePlan(plan) != nil ||
		repository.activationID != plan.ActivationID {
		return ErrInvalidPlan
	}
	isolation, _ := plan.Check("isolation")
	evaluation, _ := plan.Check("evaluation")
	var activationID, draftID, userID, draftFingerprint, proposedFingerprint string
	var runtimeFingerprint, archiveFingerprint, snapshotFingerprint string
	var workspaceID, workspaceFingerprint, isolationSuite, evaluationSuite string
	var planFingerprint string
	var enabled bool
	err := repository.db.QueryRowContext(ctx, `
SELECT activation_id,draft_id,user_id::text,draft_fingerprint,
  proposed_package_fingerprint,runtime_bundle_fingerprint,archive_fingerprint,
  runner_snapshot_fingerprint,workspace_snapshot_id,workspace_fingerprint,
  isolation_suite_fingerprint,evaluation_suite_fingerprint,plan_fingerprint,enabled
FROM agent_learning_worker_get_target($1)
`, plan.ActivationID).Scan(&activationID, &draftID, &userID, &draftFingerprint,
		&proposedFingerprint, &runtimeFingerprint, &archiveFingerprint,
		&snapshotFingerprint, &workspaceID, &workspaceFingerprint,
		&isolationSuite, &evaluationSuite, &planFingerprint, &enabled)
	if err != nil || activationID != plan.ActivationID || draftID != plan.DraftID ||
		userID != plan.UserID || draftFingerprint != plan.DraftFingerprint ||
		proposedFingerprint != plan.ProposedPackageFingerprint ||
		runtimeFingerprint != plan.RuntimeBundleFingerprint ||
		archiveFingerprint != plan.ArchiveFingerprint ||
		snapshotFingerprint != plan.RunnerSnapshotFingerprint ||
		workspaceID != plan.Sandbox.WorkspaceSnapshotID ||
		workspaceFingerprint != plan.Sandbox.WorkspaceFingerprint ||
		isolationSuite != isolation.SuiteFingerprint ||
		evaluationSuite != evaluation.SuiteFingerprint ||
		planFingerprint != plan.DocumentFingerprint || !enabled {
		return ErrInvalidPlan
	}
	return nil
}

func (repository *PostgresRepository) Begin(
	ctx context.Context,
	input BeginInput,
) (BeginResult, error) {
	if repository == nil || repository.db == nil {
		return BeginResult{}, errors.New("Draft learning database unavailable")
	}
	token := sha256.Sum256([]byte(input.Attempt.LeaseToken))
	var result BeginResult
	err := repository.db.QueryRowContext(ctx, `
SELECT attempt_id,state,created FROM agent_learning_worker_begin_runner_check(
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
`, repository.activationID, input.DraftID, input.ClaimOwner, input.CheckGeneration,
		input.Kind, input.Attempt.AttemptID, input.Attempt.RunID, input.Attempt.StepID,
		input.Attempt.LeaseGeneration, input.Attempt.LeaseOwner, hex.EncodeToString(token[:]),
		input.SnapshotFingerprint, input.Now.UTC(), int(input.LeaseDuration/time.Second)).Scan(
		&result.AttemptID, &result.State, &result.Created)
	if err != nil {
		return BeginResult{}, mapDatabaseError("begin Draft Runner check", err)
	}
	return result, nil
}

func (repository *PostgresRepository) RecordLaunch(
	ctx context.Context,
	attempt agentrunner.AttemptRef,
	sandboxID, specFingerprint, probeFingerprint string,
) error {
	var changed bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_worker_record_runner_launch($1,$2,$3,$4,$5,$6)
`, repository.activationID, attempt.AttemptID, attempt.LeaseGeneration,
		sandboxID, specFingerprint, probeFingerprint).Scan(&changed)
	if err != nil {
		return mapDatabaseError("record Draft Runner launch", err)
	}
	return nil
}

func (repository *PostgresRepository) RecordResult(
	ctx context.Context,
	attempt agentrunner.AttemptRef,
	result RunnerResult,
) error {
	var changed bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_worker_record_runner_result($1,$2,$3,$4,$5,$6,$7::jsonb)
`, repository.activationID, attempt.AttemptID, attempt.LeaseGeneration,
		result.ArtifactName, result.ArtifactBytes, result.ArtifactFingerprint,
		string(result.Payload)).Scan(&changed)
	if err != nil {
		return mapDatabaseError("record Draft Runner result", err)
	}
	return nil
}

func (repository *PostgresRepository) MarkCancelPending(
	ctx context.Context,
	attempt agentrunner.AttemptRef,
	reasonCode string,
) error {
	var changed bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_worker_mark_runner_cancel_pending($1,$2,$3,$4)
`, repository.activationID, attempt.AttemptID, attempt.LeaseGeneration, reasonCode).Scan(&changed)
	if err != nil {
		return mapDatabaseError("mark Draft Runner cancel pending", err)
	}
	return nil
}

func (repository *PostgresRepository) CompleteCleanup(
	ctx context.Context,
	attempt agentrunner.AttemptRef,
	succeeded bool,
	reasonCode string,
) error {
	var changed bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_worker_complete_runner_cleanup($1,$2,$3,$4,NULLIF($5,''))
`, repository.activationID, attempt.AttemptID, attempt.LeaseGeneration,
		succeeded, reasonCode).Scan(&changed)
	if err != nil {
		return mapDatabaseError("complete Draft Runner cleanup", err)
	}
	return nil
}

func (repository *PostgresRepository) RunnerInventory(
	ctx context.Context,
	limit int,
) ([]InventoryAttempt, error) {
	if repository == nil || repository.db == nil || limit < 1 || limit > 1000 {
		return nil, errors.New("Draft learning database unavailable")
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT attempt_id,draft_id,user_id::text,check_generation,kind,transport_run_id,
  step_id,lease_generation,lease_owner,snapshot_fingerprint,state,
  COALESCE(sandbox_id,''),COALESCE(spec_fingerprint,''),COALESCE(probe_fingerprint,''),
  cleanup_state,cleanup_attempts,COALESCE(error_code,''),lease_expires_at,
  authority_expires_at
FROM agent_learning_worker_runner_inventory($1,$2)
`, repository.activationID, limit)
	if err != nil {
		return nil, mapDatabaseError("inventory Draft Runner checks", err)
	}
	defer rows.Close()
	result := make([]InventoryAttempt, 0)
	for rows.Next() {
		var item InventoryAttempt
		var authorityExpiry sql.NullTime
		if err := rows.Scan(&item.Attempt.AttemptID, &item.DraftID, &item.UserID,
			&item.CheckGeneration, &item.Kind, &item.Attempt.RunID, &item.Attempt.StepID,
			&item.Attempt.LeaseGeneration, &item.LeaseOwner,
			&item.SnapshotFingerprint, &item.State, &item.SandboxID,
			&item.SpecFingerprint, &item.ProbeFingerprint, &item.CleanupState,
			&item.CleanupAttempts, &item.ErrorCode, &item.LeaseExpiresAt,
			&authorityExpiry); err != nil {
			return nil, fmt.Errorf("scan Draft Runner inventory: %w", err)
		}
		if authorityExpiry.Valid {
			value := authorityExpiry.Time
			item.AuthorityExpiresAt = &value
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Draft Runner inventory: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ReconcileRunner(
	ctx context.Context,
	now time.Time,
	limit int,
) (int, error) {
	if repository == nil || repository.db == nil || now.IsZero() || limit < 1 || limit > 1000 {
		return 0, errors.New("Draft learning database unavailable")
	}
	var reconciled int
	if err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_worker_reconcile_runner($1,$2,$3)
`, repository.activationID, now.UTC(), limit).Scan(&reconciled); err != nil {
		return 0, mapDatabaseError("reconcile Draft Runner checks", err)
	}
	return reconciled, nil
}

func (repository *PostgresRepository) PruneRunner(
	ctx context.Context,
	cutoff time.Time,
	limit int,
) (requests, results, attempts int, err error) {
	if repository == nil || repository.db == nil || cutoff.IsZero() || limit < 1 || limit > 1000 {
		return 0, 0, 0, errors.New("Draft learning database unavailable")
	}
	err = repository.db.QueryRowContext(ctx, `
SELECT requests_pruned,results_pruned,attempts_pruned
FROM agent_learning_worker_prune_runner($1,$2,$3)
`, repository.activationID, cutoff.UTC(), limit).Scan(&requests, &results, &attempts)
	if err != nil {
		return 0, 0, 0, mapDatabaseError("prune Draft Runner checks", err)
	}
	return requests, results, attempts, nil
}

func (repository *PostgresRepository) IssueAuthority(
	ctx context.Context,
	input agentrunner.IssueAuthorityInput,
	tokenHash string,
) (agentrunner.AuthorityRecord, error) {
	if repository == nil || repository.db == nil {
		return agentrunner.AuthorityRecord{}, agentrunner.ErrRuntimeUnavailable
	}
	var record agentrunner.AuthorityRecord
	var response []byte
	err := repository.db.QueryRowContext(ctx, `
SELECT runner_id,caller_identity,request_id,method,run_id,step_id,attempt_id,
  lease_generation,lease_owner,lease_token_hash,snapshot_fingerprint,
  kill_switch_epoch,issued_at,expires_at,replayed,COALESCE(response_body,'null'::jsonb)
FROM agent_learning_worker_issue_runner_authority($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
`, repository.activationID, input.CallerIdentity, input.RunnerID, input.RequestID,
		input.Nonce, input.Method, input.RequestFingerprint, input.Attempt.AttemptID,
		tokenHash, int(input.TTL/time.Second)).Scan(&record.RunnerID,
		&record.CallerIdentity, &record.RequestID, &record.Method, &record.RunID,
		&record.StepID, &record.AttemptID, &record.LeaseGeneration, &record.LeaseOwner,
		&record.LeaseTokenDigest, &record.SnapshotFingerprint, &record.KillSwitchEpoch,
		&record.IssuedAt, &record.ExpiresAt, &record.Replay, &response)
	if err != nil {
		return agentrunner.AuthorityRecord{}, mapDatabaseError("issue Draft Runner authority", err)
	}
	record.SchemaVersion = agentrunner.AuthorityVersion
	record.Nonce = input.Nonce
	record.RequestFingerprint = input.RequestFingerprint
	if record.Replay && string(response) != "null" {
		record.Response = append(json.RawMessage(nil), response...)
	}
	return record, nil
}

func (repository *PostgresRepository) CompleteRequest(
	ctx context.Context,
	caller, requestID, nonce, requestFingerprint, responseFingerprint string,
	response []byte,
) error {
	var accepted bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_learning_worker_complete_runner_request($1,$2,$3,$4,$5,$6::jsonb)
`, caller, requestID, nonce, requestFingerprint, responseFingerprint,
		string(response)).Scan(&accepted)
	if err != nil {
		return mapDatabaseError("complete Draft Runner request", err)
	}
	if !accepted {
		return agentrunner.ErrInvalidTransition
	}
	return nil
}

func (*PostgresRepository) ExpectSandbox(context.Context, agentrunner.ExpectedSandbox) error {
	return agentrunner.ErrInvalidTransition
}

func (*PostgresRepository) UpdateSandbox(context.Context, string, string, int64, string, string, string) error {
	return agentrunner.ErrInvalidTransition
}

func (*PostgresRepository) RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error) {
	return nil, agentrunner.ErrInvalidTransition
}

func (*PostgresRepository) Prune(context.Context, time.Time, int) (int, int, error) {
	return 0, 0, agentrunner.ErrInvalidTransition
}

func mapDatabaseError(operation string, err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToUpper(err.Error())
	for _, candidate := range []error{
		agentrunner.ErrReplayDetected,
		agentrunner.ErrLeaseStale,
		agentrunner.ErrSnapshotMismatch,
		agentrunner.ErrKillSwitchActive,
		agentrunner.ErrInvalidTransition,
	} {
		if strings.Contains(message, strings.ToUpper(candidate.Error())) {
			return candidate
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
