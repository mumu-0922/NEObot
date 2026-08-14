package agentbroker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgresRepository is the only production persistence seam for effect
// intents. Callers must connect with a session that may SET ROLE to the narrow
// agent_effect_control role; it never performs direct table mutations.
type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (repository *PostgresRepository) Prepare(ctx context.Context, input PreparedIntent) (PreparedIntent, error) {
	if repository == nil || repository.db == nil {
		return PreparedIntent{}, ErrDatabaseRequired
	}
	row := repository.db.QueryRowContext(ctx, `
SELECT id,request_id,request_fingerprint,intent_fingerprint,idempotency_key,
       user_id::text,project_id,assistant_id,run_id,step_id,attempt_id,
       lease_generation,lease_owner,lease_token_hash,snapshot_fingerprint,
       grant_id,grant_fingerprint,registry_fingerprint,tool_identity,capability,
       action,resource,arguments_fingerprint,canonical_arguments,base_revision,
       approval_class,capability_max_calls,grant_max_tool_calls,state,
       kill_switch_epoch,created_at,expires_at,approved_at,approval_revision,
       committing_at,terminal_at,COALESCE(receipt_fingerprint,''),
       COALESCE(executor_status_digest,''),COALESCE(error_code,'')
FROM agent_effect_prepare(
  $1,$2,$3,$4,$5,$6::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
  $19,$20,$21,$22,$23,$24::jsonb,$25,$26,$27,$28,$29,$30,$31
)`, input.IntentID, input.RequestID, input.RequestFingerprint, input.IntentFingerprint,
		input.IdempotencyKey, input.Subject.UserID, input.Subject.ProjectID, input.Subject.AssistantID,
		input.RunID, input.StepID, input.AttemptID, input.Generation, input.LeaseOwner,
		input.LeaseTokenDigest, input.SnapshotFingerprint, input.GrantID, input.GrantFingerprint,
		input.RegistryFingerprint, input.ToolIdentity, input.Capability, input.Action, input.Resource,
		input.ArgumentsFingerprint, string(input.CanonicalArguments), input.BaseRevision,
		input.ApprovalClass, input.CapabilityMaxCalls, input.GrantMaxToolCalls,
		input.KillSwitchEpoch, input.CreatedAt, input.ExpiresAt)
	prepared, err := scanIntent(row)
	if err != nil {
		return PreparedIntent{}, mapPostgresError("prepare agent effect", err)
	}
	prepared.Replay = prepared.IntentID != input.IntentID
	return prepared, nil
}

func (repository *PostgresRepository) DecideApproval(ctx context.Context, input ApprovalInput) (PreparedIntent, error) {
	if repository == nil || repository.db == nil {
		return PreparedIntent{}, ErrDatabaseRequired
	}
	prepared, err := scanIntent(repository.db.QueryRowContext(ctx, `
SELECT id,request_id,request_fingerprint,intent_fingerprint,idempotency_key,
       user_id::text,project_id,assistant_id,run_id,step_id,attempt_id,
       lease_generation,lease_owner,lease_token_hash,snapshot_fingerprint,
       grant_id,grant_fingerprint,registry_fingerprint,tool_identity,capability,
       action,resource,arguments_fingerprint,canonical_arguments,base_revision,
       approval_class,capability_max_calls,grant_max_tool_calls,state,
       kill_switch_epoch,created_at,expires_at,approved_at,approval_revision,
       committing_at,terminal_at,COALESCE(receipt_fingerprint,''),
       COALESCE(executor_status_digest,''),COALESCE(error_code,'')
FROM agent_effect_decide_approval($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9)
`, input.ApprovalID, input.UserID, input.IntentID, input.IntentFingerprint,
		input.Decision, input.ActorType, input.ActorID, input.ReasonCode, input.ExpectedRevision))
	if err != nil {
		return PreparedIntent{}, mapPostgresError("decide agent effect approval", err)
	}
	return prepared, nil
}

func (repository *PostgresRepository) Cancel(ctx context.Context, input CancelInput) (PreparedIntent, error) {
	if repository == nil || repository.db == nil {
		return PreparedIntent{}, ErrDatabaseRequired
	}
	intent, err := scanIntent(repository.db.QueryRowContext(ctx, intentSelect+`
FROM agent_effect_cancel($1,$2::uuid,$3,$4,$5,$6,$7)
`, input.CancellationID, input.UserID, input.IntentID, input.IntentFingerprint,
		input.ActorType, input.ActorID, input.ReasonCode))
	if err != nil {
		return PreparedIntent{}, mapPostgresError("cancel agent effect", err)
	}
	return intent, nil
}

func (repository *PostgresRepository) ClaimCommit(ctx context.Context, input CommitInput, tokenHash string) (CommitClaim, error) {
	if repository == nil || repository.db == nil {
		return CommitClaim{}, ErrDatabaseRequired
	}
	row := repository.db.QueryRowContext(ctx, `
SELECT (result.intent).id,(result.intent).request_id,(result.intent).request_fingerprint,
       (result.intent).intent_fingerprint,(result.intent).idempotency_key,
       (result.intent).user_id::text,(result.intent).project_id,(result.intent).assistant_id,
       (result.intent).run_id,(result.intent).step_id,(result.intent).attempt_id,
       (result.intent).lease_generation,(result.intent).lease_owner,(result.intent).lease_token_hash,
       (result.intent).snapshot_fingerprint,(result.intent).grant_id,
       (result.intent).grant_fingerprint,(result.intent).registry_fingerprint,
       (result.intent).tool_identity,(result.intent).capability,(result.intent).action,
       (result.intent).resource,(result.intent).arguments_fingerprint,
       (result.intent).canonical_arguments,(result.intent).base_revision,
       (result.intent).approval_class,(result.intent).capability_max_calls,
       (result.intent).grant_max_tool_calls,(result.intent).state,
       (result.intent).kill_switch_epoch,(result.intent).created_at,(result.intent).expires_at,
       (result.intent).approved_at,(result.intent).approval_revision,
       (result.intent).committing_at,(result.intent).terminal_at,
       COALESCE((result.intent).receipt_fingerprint,''),
       COALESCE((result.intent).executor_status_digest,''),
       COALESCE((result.intent).error_code,''),result.replayed
FROM agent_effect_claim_commit(
  $1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15
) result`, input.Attempt.UserID, input.Attempt.RunID, input.Attempt.StepID,
		input.Attempt.AttemptID, input.Attempt.Generation, input.Attempt.LeaseOwner,
		tokenHash, input.Attempt.SnapshotFingerprint, input.Attempt.GrantFingerprint,
		input.Attempt.RegistryFingerprint, input.Attempt.KillSwitchEpoch, input.IntentID,
		input.IntentFingerprint, input.ApprovalID, input.IdempotencyKey)
	intent, replayed, err := scanIntentWithReplay(row)
	if err != nil {
		return CommitClaim{}, mapPostgresError("claim agent effect Commit", err)
	}
	return CommitClaim{Intent: intent, Replay: replayed}, nil
}

func (repository *PostgresRepository) CompleteCommit(ctx context.Context, intentID, state, receipt, statusDigest, code string) (PreparedIntent, error) {
	if repository == nil || repository.db == nil {
		return PreparedIntent{}, ErrDatabaseRequired
	}
	intent, err := scanIntent(repository.db.QueryRowContext(ctx, intentSelect+`
FROM agent_effect_complete_commit($1,$2,$3,$4,$5)
`, intentID, state, receipt, statusDigest, code))
	if err != nil {
		return PreparedIntent{}, mapPostgresError("complete agent effect Commit", err)
	}
	return intent, nil
}

func (repository *PostgresRepository) RevokeGrant(ctx context.Context, input GrantRevocationInput) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, ErrDatabaseRequired
	}
	var created bool
	if err := repository.db.QueryRowContext(ctx, `SELECT agent_effect_revoke_grant($1,$2,$3,$4,$5)`,
		input.GrantID, input.GrantFingerprint, input.ActorType, input.ActorID, input.ReasonCode).
		Scan(&created); err != nil {
		return false, mapPostgresError("revoke agent capability Grant", err)
	}
	return created, nil
}

func (repository *PostgresRepository) GetIntent(ctx context.Context, userID, intentID string) (PreparedIntent, error) {
	if repository == nil || repository.db == nil {
		return PreparedIntent{}, ErrDatabaseRequired
	}
	intent, err := scanIntent(repository.db.QueryRowContext(ctx, intentSelect+`
FROM agent_effect_get_intent($1::uuid,$2)
`, userID, intentID))
	if errors.Is(err, sql.ErrNoRows) {
		return PreparedIntent{}, ErrNotFound
	}
	if err != nil {
		return PreparedIntent{}, fmt.Errorf("get agent effect intent: %w", err)
	}
	return intent, nil
}

func (repository *PostgresRepository) ListCommitting(ctx context.Context, limit int) ([]PreparedIntent, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDatabaseRequired
	}
	rows, err := repository.db.QueryContext(ctx, intentSelect+`
FROM agent_effect_list_committing($1)
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list committing agent effects: %w", err)
	}
	defer rows.Close()
	var result []PreparedIntent
	for rows.Next() {
		intent, scanErr := scanIntent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan committing agent effect: %w", scanErr)
		}
		result = append(result, intent)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) Expire(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	if repository == nil || repository.db == nil {
		return 0, ErrDatabaseRequired
	}
	var changed int
	if err := repository.db.QueryRowContext(ctx, `SELECT agent_effect_expire($1,$2)`, cutoff, limit).Scan(&changed); err != nil {
		return 0, mapPostgresError("expire agent effects", err)
	}
	return changed, nil
}

func (repository *PostgresRepository) CreateSecretHandle(ctx context.Context, record SecretHandleRecord, binding SecretBinding) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	encodedBinding, err := json.Marshal(binding)
	if err != nil {
		return ErrSecretDenied
	}
	var created bool
	err = repository.db.QueryRowContext(ctx, `SELECT agent_secret_handle_create($1,$2,$3,$4,$5,$6::jsonb)`,
		record.HandleDigest, record.IntentID, record.SecretRef, record.BindingFingerprint,
		record.ExpiresAt, string(encodedBinding)).Scan(&created)
	if err != nil {
		return mapPostgresError("create agent secret handle", err)
	}
	if !created {
		return ErrSecretDenied
	}
	return nil
}

func (repository *PostgresRepository) RevokeSecretHandle(ctx context.Context, digest string, revokedAt time.Time) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	var revoked bool
	if err := repository.db.QueryRowContext(ctx, `SELECT agent_secret_handle_revoke($1,$2)`,
		digest, revokedAt).Scan(&revoked); err != nil {
		return mapPostgresError("revoke agent secret handle", err)
	}
	if !revoked {
		return ErrSecretDenied
	}
	return nil
}

func (repository *PostgresRepository) ConsumeSecretHandle(ctx context.Context, digest, binding string, usedAt time.Time) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	var consumed bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_secret_handle_consume($1,$2,$3)`, digest, binding, usedAt).Scan(&consumed)
	if err != nil {
		return mapPostgresError("consume agent secret handle", err)
	}
	if !consumed {
		return ErrSecretDenied
	}
	return nil
}

func (repository *PostgresRepository) RevokeIntentHandles(ctx context.Context, intentID string, revokedAt time.Time) (int, error) {
	if repository == nil || repository.db == nil {
		return 0, ErrDatabaseRequired
	}
	var changed int
	if err := repository.db.QueryRowContext(ctx, `SELECT agent_secret_handle_revoke_intent($1,$2)`, intentID, revokedAt).Scan(&changed); err != nil {
		return 0, mapPostgresError("revoke agent secret handles", err)
	}
	return changed, nil
}

const intentSelect = `
SELECT id,request_id,request_fingerprint,intent_fingerprint,idempotency_key,
       user_id::text,project_id,assistant_id,run_id,step_id,attempt_id,
       lease_generation,lease_owner,lease_token_hash,snapshot_fingerprint,
       grant_id,grant_fingerprint,registry_fingerprint,tool_identity,capability,
       action,resource,arguments_fingerprint,canonical_arguments,base_revision,
       approval_class,capability_max_calls,grant_max_tool_calls,state,
       kill_switch_epoch,created_at,expires_at,approved_at,approval_revision,
       committing_at,terminal_at,COALESCE(receipt_fingerprint,''),
       COALESCE(executor_status_digest,''),COALESCE(error_code,'')`

type rowScanner interface{ Scan(...any) error }

func scanIntent(row rowScanner) (PreparedIntent, error) {
	var intent PreparedIntent
	var arguments []byte
	err := row.Scan(&intent.IntentID, &intent.RequestID, &intent.RequestFingerprint,
		&intent.IntentFingerprint, &intent.IdempotencyKey, &intent.Subject.UserID,
		&intent.Subject.ProjectID, &intent.Subject.AssistantID, &intent.RunID, &intent.StepID,
		&intent.AttemptID, &intent.Generation, &intent.LeaseOwner, &intent.LeaseTokenDigest,
		&intent.SnapshotFingerprint, &intent.GrantID, &intent.GrantFingerprint,
		&intent.RegistryFingerprint, &intent.ToolIdentity, &intent.Capability, &intent.Action,
		&intent.Resource, &intent.ArgumentsFingerprint, &arguments, &intent.BaseRevision,
		&intent.ApprovalClass, &intent.CapabilityMaxCalls, &intent.GrantMaxToolCalls,
		&intent.State, &intent.KillSwitchEpoch, &intent.CreatedAt, &intent.ExpiresAt,
		&intent.ApprovedAt, &intent.ApprovalRevision, &intent.CommittingAt, &intent.TerminalAt,
		&intent.ReceiptFingerprint, &intent.ExecutorStatusDigest, &intent.ErrorCode)
	if err != nil {
		return PreparedIntent{}, err
	}
	if !json.Valid(arguments) {
		return PreparedIntent{}, ErrInvalidInput
	}
	intent.SchemaVersion = IntentVersion
	intent.CanonicalArguments = append(json.RawMessage(nil), arguments...)
	return intent, nil
}

func scanIntentWithReplay(row rowScanner) (PreparedIntent, bool, error) {
	// Composite-return queries have the same ordered intent columns plus replay.
	var replay bool
	wrapped := &trailingScanner{row: row, trailing: &replay}
	intent, err := scanIntent(wrapped)
	return intent, replay, err
}

type trailingScanner struct {
	row      rowScanner
	trailing any
}

func (scanner *trailingScanner) Scan(dest ...any) error {
	return scanner.row.Scan(append(dest, scanner.trailing)...)
}

func mapPostgresError(action string, err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		message := strings.ToUpper(pgError.Message)
		for _, mapping := range []struct {
			code string
			err  error
		}{
			{"REPLAY_DETECTED", ErrReplayDetected}, {"LEASE_STALE", ErrLeaseStale},
			{"GRANT_DENIED", ErrGrantDenied},
			{"SNAPSHOT_MISMATCH", ErrSnapshotMismatch}, {"KILL_SWITCH_ACTIVE", ErrKillSwitchActive},
			{"BUDGET_EXHAUSTED", ErrBudgetExhausted}, {"APPROVAL_REQUIRED", ErrApprovalRequired},
			{"APPROVAL_DENIED", ErrApprovalDenied}, {"INTENT_EXPIRED", ErrIntentExpired},
			{"SECRET_DENIED", ErrSecretDenied},
			{"ARTIFACT_DENIED", ErrArtifactDenied},
			{"INVALID_TRANSITION", ErrInvalidTransition}, {"AGENT_EFFECT_NOT_FOUND", ErrNotFound},
		} {
			if strings.Contains(message, mapping.code) {
				return mapping.err
			}
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
