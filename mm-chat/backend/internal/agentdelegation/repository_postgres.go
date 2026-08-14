package agentdelegation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (repository *PostgresRepository) RegisterRoot(ctx context.Context, authority Authority) (Authority, bool, error) {
	if repository == nil || repository.db == nil {
		return Authority{}, false, ErrDatabaseRequired
	}
	subject, _ := json.Marshal(authority.Subject)
	model, _ := json.Marshal(authority.Model)
	grant, _ := json.Marshal(authority.Grant)
	registry, _ := json.Marshal(authority.Registry)
	var created bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_delegation_register_root(
  $1,$2::uuid,$3::jsonb,$4::jsonb,$5,$6,$7,$8,$9::jsonb,$10,$11::jsonb,$12
)`, authority.RunID, authority.UserID, string(subject), string(model), authority.PackageFingerprint,
		authority.RuntimeBundleFingerprint, authority.Grant.GrantID, authority.GrantFingerprint,
		string(grant), authority.RegistryFingerprint, string(registry), authority.ExpiresAt).Scan(&created)
	if err != nil {
		return Authority{}, false, mapPostgresError("register root authority", err)
	}
	result, err := repository.GetAuthority(ctx, authority.UserID, authority.RunID)
	return result, created, err
}

func (repository *PostgresRepository) GetAuthority(ctx context.Context, userID, runID string) (Authority, error) {
	if repository == nil || repository.db == nil {
		return Authority{}, ErrDatabaseRequired
	}
	var result Authority
	var subject, model, grant, registry, budget, reserved, consumed []byte
	err := repository.db.QueryRowContext(ctx, `
SELECT run_id,root_run_id,COALESCE(parent_run_id,''),depth,user_id::text,snapshot_id,snapshot_fingerprint,
 subject,model,package_fingerprint,runtime_bundle_fingerprint,grant_fingerprint,grant_json,
 registry_fingerprint,registry_json,expires_at,budget,reserved_budget,consumed_budget,state,created_at
FROM agent_delegation_authorities WHERE run_id=$1 AND user_id=$2::uuid
`, runID, userID).Scan(&result.RunID, &result.RootRunID, &result.ParentRunID, &result.Depth,
		&result.UserID, &result.SnapshotID, &result.SnapshotFingerprint, &subject, &model,
		&result.PackageFingerprint, &result.RuntimeBundleFingerprint, &result.GrantFingerprint,
		&grant, &result.RegistryFingerprint, &registry, &result.ExpiresAt, &budget, &reserved,
		&consumed, &result.State, &result.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Authority{}, ErrNotFound
	}
	if err != nil {
		return Authority{}, fmt.Errorf("get delegation authority: %w", err)
	}
	for _, target := range []struct {
		body  []byte
		value any
	}{{subject, &result.Subject}, {model, &result.Model}, {grant, &result.Grant}, {registry, &result.Registry}, {budget, &result.Budget}, {reserved, &result.Reserved}, {consumed, &result.Consumed}} {
		if json.Unmarshal(target.body, target.value) != nil {
			return Authority{}, fmt.Errorf("decode delegation authority: %w", ErrInvalidInput)
		}
	}
	// PostgreSQL timestamptz is microsecond precision, while the immutable Grant
	// JSON retains RFC3339 nanoseconds. The Grant is the fingerprint authority.
	result.ExpiresAt = result.Grant.ExpiresAt.UTC()
	return result, nil
}

func (repository *PostgresRepository) EnqueueChild(ctx context.Context, derivation Derivation, proposal ChildProposal) (Authority, bool, error) {
	if repository == nil || repository.db == nil {
		return Authority{}, false, ErrDatabaseRequired
	}
	steps, _ := json.Marshal(derivation.Steps)
	grant, _ := json.Marshal(derivation.ChildGrant)
	registry, _ := json.Marshal(derivation.ChildRegistry)
	digest := leaseTokenDigest(proposal.ParentAttempt.LeaseToken)
	var runID string
	var created bool
	err := repository.db.QueryRowContext(ctx, `SELECT run_id,created FROM agent_delegation_enqueue_child(
 $1,$2,$3::uuid,$4,$5,$6,$7::jsonb,$8::jsonb,$9::text[],$10::text[],$11,$12,$13,$14,$15,$16,$17,$18::jsonb,$19,$20::jsonb
)`, derivation.ChildRunID, derivation.ChildSnapshotID, derivation.Parent.UserID, proposal.IdempotencyKey,
		derivation.ChildSnapshotFingerprint, derivation.RequestFingerprint, string(derivation.ChildSnapshot), string(steps),
		derivation.ScopeKeys, derivation.EventIDs, derivation.Parent.RunID, proposal.ParentAttempt.StepID,
		proposal.ParentAttempt.AttemptID, proposal.ParentAttempt.Generation, proposal.ParentAttempt.LeaseOwner, digest,
		derivation.ChildGrantFingerprint, string(grant), derivation.ChildRegistry.Fingerprint, string(registry)).Scan(&runID, &created)
	if err != nil {
		return Authority{}, false, mapPostgresError("enqueue child", err)
	}
	result, err := repository.GetAuthority(ctx, derivation.Parent.UserID, runID)
	return result, created, err
}

func (repository *PostgresRepository) AdmitLaunch(ctx context.Context, input LaunchAdmissionInput, tokenHash string) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	tools, _ := json.Marshal(input.RegistryTools)
	var accepted bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_delegation_admit_launch($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)`,
		input.UserID, input.RunID, input.AttemptID, input.Generation, input.LeaseOwner, tokenHash, input.SnapshotFingerprint,
		input.GrantFingerprint, input.RegistryFingerprint, string(tools)).Scan(&accepted)
	if err != nil {
		return mapPostgresError("admit child launch", err)
	}
	if !accepted {
		return ErrSnapshotMismatch
	}
	return nil
}

func (repository *PostgresRepository) Settle(ctx context.Context, input SettleInput, settlementID string) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, ErrDatabaseRequired
	}
	usage, _ := json.Marshal(input.Usage)
	var created bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_delegation_settle($1,$2::uuid,$3,$4::jsonb,$5)`, settlementID, input.UserID, input.ChildRunID, string(usage), input.Outcome).Scan(&created)
	if err != nil {
		return false, mapPostgresError("settle child budget", err)
	}
	return created, nil
}

func (repository *PostgresRepository) Cascade(ctx context.Context, input CascadeInput) ([]ReapTarget, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDatabaseRequired
	}
	rows, err := repository.db.QueryContext(ctx, `SELECT reap_id,parent_run_id,child_run_id,attempt_id,generation,lease_owner,mode,state,retry_count
FROM agent_delegation_cascade($1::uuid,$2,$3,$4,$5,$6)`, input.UserID, input.ParentRunID, input.Mode, input.ActorType, input.ActorID, input.ReasonCode)
	if err != nil {
		return nil, mapPostgresError("cascade child runs", err)
	}
	defer rows.Close()
	return scanReaps(rows)
}

func (repository *PostgresRepository) Recover(ctx context.Context, limit int) (int, error) {
	if repository == nil || repository.db == nil {
		return 0, ErrDatabaseRequired
	}
	var recovered int
	if err := repository.db.QueryRowContext(ctx, `SELECT agent_delegation_reconcile($1)`, limit).Scan(&recovered); err != nil {
		return 0, mapPostgresError("reconcile child runs", err)
	}
	return recovered, nil
}

func (repository *PostgresRepository) ListPendingReaps(ctx context.Context, limit int) ([]ReapTarget, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDatabaseRequired
	}
	rows, err := repository.db.QueryContext(ctx, `SELECT reap_id,parent_run_id,child_run_id,attempt_id,generation,lease_owner,mode,state,retry_count
FROM agent_delegation_reaps WHERE state IN ('pending','failed') ORDER BY created_at,reap_id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list delegation reaps: %w", err)
	}
	defer rows.Close()
	return scanReaps(rows)
}

func scanReaps(rows *sql.Rows) ([]ReapTarget, error) {
	var result []ReapTarget
	for rows.Next() {
		var item ReapTarget
		if err := rows.Scan(&item.ReapID, &item.ParentRunID, &item.ChildRunID, &item.AttemptID, &item.Generation, &item.LeaseOwner, &item.Mode, &item.State, &item.RetryCount); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) CompleteReap(ctx context.Context, reapID string, succeeded bool, errorCode string) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	var changed bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_delegation_complete_reap($1,$2,$3)`, reapID, succeeded, errorCode).Scan(&changed)
	if err != nil {
		return mapPostgresError("complete child reap", err)
	}
	return nil
}

func leaseTokenDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)
}

func mapPostgresError(action string, err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		message := strings.ToUpper(pgError.Message)
		switch {
		case strings.Contains(message, "PARENT_INVALID"):
			return ErrParentInvalid
		case strings.Contains(message, "PARENT_STALE"):
			return ErrParentStale
		case strings.Contains(message, "DEPTH_EXCEEDED"):
			return ErrDepthExceeded
		case strings.Contains(message, "SUBSET_VIOLATION"):
			return ErrSubsetViolation
		case strings.Contains(message, "BUDGET_EXCEEDED"):
			return ErrBudgetExceeded
		case strings.Contains(message, "SNAPSHOT_MISMATCH"):
			return ErrSnapshotMismatch
		case strings.Contains(message, "LEASE_STALE"):
			return ErrLeaseStale
		case strings.Contains(message, "KILL_SWITCH_ACTIVE"):
			return ErrKillSwitchActive
		case strings.Contains(message, "IDEMPOTENCY_CONFLICT"):
			return ErrIdempotencyConflict
		case strings.Contains(message, "SETTLEMENT_INVALID"):
			return ErrSettlementInvalid
		case strings.Contains(message, "NOT_FOUND"):
			return ErrNotFound
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
