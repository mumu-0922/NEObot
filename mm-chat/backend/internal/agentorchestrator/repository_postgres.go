package agentorchestrator

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

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (repository *PostgresRepository) EnqueueRun(ctx context.Context, prepared preparedEnqueue) (string, bool, error) {
	if repository == nil || repository.db == nil {
		return "", false, ErrDatabaseRequired
	}
	var runID string
	var created bool
	err := repository.db.QueryRowContext(ctx, `
SELECT run_id, created
FROM agent_orchestrator_enqueue_run(
  $1, $2, $3::uuid, $4, $5, $6, $7::jsonb, $8::jsonb, $9::text[], $10::text[]
)`, prepared.RunID, prepared.SnapshotID, prepared.Input.UserID,
		prepared.Input.IdempotencyKey, prepared.SnapshotFingerprint, prepared.RequestFingerprint,
		string(prepared.CanonicalSnapshot), string(prepared.StepsJSON), prepared.ScopeKeys, prepared.EventIDs,
	).Scan(&runID, &created)
	if err != nil {
		return "", false, mapPostgresError("enqueue agent Run", err)
	}
	return runID, created, nil
}

func (repository *PostgresRepository) GetRun(ctx context.Context, userID, runID string) (Run, error) {
	if repository == nil || repository.db == nil {
		return Run{}, ErrDatabaseRequired
	}
	var run Run
	err := repository.db.QueryRowContext(ctx, `
SELECT id, user_id::text, snapshot_id, snapshot_fingerprint,
       request_fingerprint, state, next_sequence, created_at, updated_at, terminal_at
FROM agent_runs WHERE id=$1 AND user_id=$2::uuid
`, runID, userID).Scan(&run.ID, &run.UserID, &run.SnapshotID,
		&run.SnapshotFingerprint, &run.RequestFingerprint, &run.State, &run.NextSequence,
		&run.CreatedAt, &run.UpdatedAt, &run.TerminalAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("get agent Run: %w", err)
	}
	steps, err := repository.db.QueryContext(ctx, `
SELECT id, run_id, ordinal, kind, state, current_generation,
       created_at, updated_at, terminal_at
FROM agent_steps WHERE run_id=$1 AND user_id=$2::uuid ORDER BY ordinal
`, runID, userID)
	if err != nil {
		return Run{}, fmt.Errorf("list agent Steps: %w", err)
	}
	defer steps.Close()
	for steps.Next() {
		var step Step
		if err := steps.Scan(&step.ID, &step.RunID, &step.Ordinal, &step.Kind, &step.State,
			&step.CurrentGeneration, &step.CreatedAt, &step.UpdatedAt, &step.TerminalAt); err != nil {
			return Run{}, fmt.Errorf("scan agent Step: %w", err)
		}
		run.Steps = append(run.Steps, step)
	}
	if err := steps.Err(); err != nil {
		return Run{}, fmt.Errorf("iterate agent Steps: %w", err)
	}
	attempts, err := repository.db.QueryContext(ctx, `
SELECT id, run_id, step_id, generation, state, lease_owner,
       lease_expires_at, created_at, updated_at, terminal_at
FROM agent_attempts WHERE run_id=$1 AND user_id=$2::uuid
ORDER BY step_id, generation
`, runID, userID)
	if err != nil {
		return Run{}, fmt.Errorf("list agent Attempts: %w", err)
	}
	defer attempts.Close()
	for attempts.Next() {
		var attempt Attempt
		if err := attempts.Scan(&attempt.ID, &attempt.RunID, &attempt.StepID,
			&attempt.Generation, &attempt.State, &attempt.LeaseOwner,
			&attempt.LeaseExpiresAt, &attempt.CreatedAt, &attempt.UpdatedAt,
			&attempt.TerminalAt); err != nil {
			return Run{}, fmt.Errorf("scan agent Attempt: %w", err)
		}
		run.Attempts = append(run.Attempts, attempt)
	}
	if err := attempts.Err(); err != nil {
		return Run{}, fmt.Errorf("iterate agent Attempts: %w", err)
	}
	return run, nil
}

func (repository *PostgresRepository) TransitionRun(ctx context.Context, input TransitionInput) error {
	return repository.transition(ctx, "run", input)
}

func (repository *PostgresRepository) TransitionStep(ctx context.Context, input TransitionInput) error {
	return repository.transition(ctx, "step", input)
}

func (repository *PostgresRepository) TransitionAttempt(ctx context.Context, input TransitionInput) error {
	return repository.transition(ctx, "attempt", input)
}

func (repository *PostgresRepository) transition(ctx context.Context, entity string, input TransitionInput) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	detail, err := marshalEventDetail(input.Detail)
	if err != nil {
		return ErrInvalidInput
	}
	var tokenHash string
	if input.LeaseToken != "" {
		digest := sha256.Sum256([]byte(input.LeaseToken))
		tokenHash = hex.EncodeToString(digest[:])
	}
	var accepted bool
	err = repository.db.QueryRowContext(ctx, `
SELECT agent_orchestrator_transition(
  $1, $2::uuid, $3, NULLIF($4,''), NULLIF($5,''), $6,
  NULLIF($7,''), NULLIF($8,''), $9, $10, $11, $12, $13, $14, $15::jsonb
)`, input.EventID, input.UserID, input.RunID, input.StepID, input.AttemptID,
		input.Generation, input.LeaseOwner, tokenHash, entity, input.Expected,
		input.To, input.Actor.Type, input.Actor.ID, input.ReasonCode, string(detail)).Scan(&accepted)
	if err != nil {
		return mapPostgresError("transition agent "+entity, err)
	}
	if !accepted {
		return ErrInvalidTransition
	}
	return nil
}

func (repository *PostgresRepository) ObserveTerminalConflict(ctx context.Context, input TransitionInput) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	detail, _ := marshalEventDetail(input.Detail)
	var accepted bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_orchestrator_observe_terminal_conflict(
  $1, $2::uuid, $3, NULLIF($4,''), NULLIF($5,''), $6, $7, $8, $9, $10::jsonb
)`, input.EventID, input.UserID, input.RunID, input.StepID, input.AttemptID,
		input.To, input.Actor.Type, input.Actor.ID, input.ReasonCode, string(detail)).Scan(&accepted)
	if err != nil {
		return mapPostgresError("observe terminal conflict", err)
	}
	return nil
}

func marshalEventDetail(detail map[string]any) ([]byte, error) {
	if detail == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(detail)
}

func (repository *PostgresRepository) AcquireStep(ctx context.Context, input AcquireInput, prepared preparedLease) (Lease, error) {
	if repository == nil || repository.db == nil {
		return Lease{}, ErrDatabaseRequired
	}
	var lease Lease
	ttlSeconds := int(input.LeaseDuration / time.Second)
	err := repository.db.QueryRowContext(ctx, `
SELECT attempt_id, run_id, step_id, generation, state, lease_owner,
       lease_expires_at, created_at, updated_at
FROM agent_orchestrator_acquire_step(
  $1::uuid, $2, $3, $4, $5, $6, $7::text[], $8, $9, $10, $11
)`, input.UserID, input.RunID, input.StepID, prepared.AttemptID,
		input.LeaseOwner, prepared.TokenHash, prepared.EventIDs, ttlSeconds,
		input.Actor.Type, input.Actor.ID, input.ReasonCode,
	).Scan(&lease.ID, &lease.RunID, &lease.StepID, &lease.Generation,
		&lease.State, &lease.LeaseOwner, &lease.LeaseExpiresAt, &lease.CreatedAt,
		&lease.UpdatedAt)
	if err != nil {
		return Lease{}, mapPostgresError("acquire agent Step", err)
	}
	lease.Token = prepared.Token
	return lease, nil
}

func (repository *PostgresRepository) HeartbeatAttempt(ctx context.Context, input HeartbeatInput, tokenHash string) (time.Time, error) {
	if repository == nil || repository.db == nil {
		return time.Time{}, ErrDatabaseRequired
	}
	var expiresAt time.Time
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_orchestrator_heartbeat_attempt(
  $1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)`, input.EventID, input.UserID, input.RunID, input.StepID,
		input.AttemptID, input.Generation, input.LeaseOwner, tokenHash,
		int(input.LeaseDuration/time.Second), input.Actor.Type, input.Actor.ID,
		input.ReasonCode).Scan(&expiresAt)
	if err != nil {
		return time.Time{}, mapPostgresError("heartbeat agent Attempt", err)
	}
	return expiresAt, nil
}

func (repository *PostgresRepository) ListRecoveryRuns(ctx context.Context, limit int) ([]RecoveryRun, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDatabaseRequired
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT run_id, user_id::text, run_state, snapshot_id,
       COALESCE(attempt_id,''), COALESCE(step_id,''), COALESCE(generation,0),
       COALESCE(attempt_state,''), lease_expires_at,
       COALESCE(lease_expires_at <= clock_timestamp(), false)
FROM agent_orchestrator_recovery_runs($1)
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recovery Runs: %w", err)
	}
	defer rows.Close()
	var result []RecoveryRun
	for rows.Next() {
		var item RecoveryRun
		if err := rows.Scan(&item.RunID, &item.UserID, &item.State, &item.SnapshotID,
			&item.AttemptID, &item.StepID, &item.Generation, &item.AttemptState,
			&item.LeaseExpiresAt, &item.LeaseExpired); err != nil {
			return nil, fmt.Errorf("scan recovery Run: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) RebuildProjection(ctx context.Context, userID, runID string) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	var rebuilt bool
	if err := repository.db.QueryRowContext(ctx, `SELECT agent_orchestrator_rebuild_projection($1::uuid,$2)`, userID, runID).Scan(&rebuilt); err != nil {
		return mapPostgresError("rebuild agent projection", err)
	}
	return nil
}

func (repository *PostgresRepository) AppendKillSwitch(ctx context.Context, input KillSwitchInput) (KillSwitch, error) {
	if repository == nil || repository.db == nil {
		return KillSwitch{}, ErrDatabaseRequired
	}
	var result KillSwitch
	err := repository.db.QueryRowContext(ctx, `
SELECT switch_id, scope_type, scope_value, mode, active, revision, epoch,
       actor_type, actor_id, reason_code, created_at
FROM agent_orchestrator_append_kill_switch(
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)`, input.ID, input.ScopeType, input.ScopeValue, input.Mode, input.Active,
		input.ExpectedRevision, input.Actor.Type, input.Actor.ID, input.ReasonCode,
	).Scan(&result.ID, &result.ScopeType, &result.ScopeValue, &result.Mode,
		&result.Active, &result.Revision, &result.Epoch, &result.Actor.Type,
		&result.Actor.ID, &result.ReasonCode, &result.CreatedAt)
	if err != nil {
		return KillSwitch{}, mapPostgresError("append agent Kill Switch", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ResolveKillSwitch(ctx context.Context, userID, runID string) (KillResolution, error) {
	if repository == nil || repository.db == nil {
		return KillResolution{}, ErrDatabaseRequired
	}
	var result KillResolution
	var switchIDsJSON []byte
	err := repository.db.QueryRowContext(ctx, `
SELECT active, mode, epoch, to_json(switch_ids)
FROM agent_orchestrator_resolve_kill_switch($1::uuid,$2)
`, userID, runID).Scan(&result.Active, &result.Mode, &result.Epoch, &switchIDsJSON)
	if err != nil {
		return KillResolution{}, mapPostgresError("resolve agent Kill Switch", err)
	}
	if err := json.Unmarshal(switchIDsJSON, &result.SwitchIDs); err != nil {
		return KillResolution{}, fmt.Errorf("decode agent Kill Switch resolution: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) PruneTerminalRuns(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if repository == nil || repository.db == nil {
		return 0, ErrDatabaseRequired
	}
	var deleted int
	if err := repository.db.QueryRowContext(ctx, `SELECT agent_orchestrator_prune_terminal_runs($1,$2)`, cutoff, limit).Scan(&deleted); err != nil {
		return 0, mapPostgresError("prune terminal agent Runs", err)
	}
	return deleted, nil
}

func mapPostgresError(action string, err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		message := strings.ToUpper(pgError.Message)
		switch {
		case strings.Contains(message, "IDEMPOTENCY_CONFLICT"):
			return ErrIdempotencyConflict
		case strings.Contains(message, "INVALID_TRANSITION"):
			return ErrInvalidTransition
		case strings.Contains(message, "LEASE_STALE"):
			return ErrLeaseStale
		case strings.Contains(message, "KILL_SWITCH_ACTIVE"):
			return ErrKillSwitchActive
		case strings.Contains(message, "REVISION_CONFLICT"):
			return ErrRevisionConflict
		case strings.Contains(message, "NOT_FOUND"):
			return ErrNotFound
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
