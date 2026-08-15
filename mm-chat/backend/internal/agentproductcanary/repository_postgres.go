package agentproductcanary

import (
	"context"
	"database/sql"
	"time"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (repository *PostgresRepository) GetActivation(ctx context.Context, activationID string) (Activation, error) {
	if repository == nil || repository.db == nil {
		return Activation{}, ErrUnavailable
	}
	var result Activation
	err := repository.db.QueryRowContext(ctx, `
SELECT activation_id,release_commit,policy_revision,admission_id::text,
       package_fingerprint,runtime_bundle_fingerprint,plan_fingerprint,
       max_requests,valid_from,valid_until,enabled
FROM agent_product_canary_worker_get_activation($1)
`, activationID).Scan(&result.ID, &result.ReleaseCommit, &result.PolicyRevision,
		&result.AdmissionID, &result.PackageFingerprint, &result.RuntimeBundleFingerprint,
		&result.PlanFingerprint, &result.MaxRequests, &result.ValidFrom, &result.ValidUntil,
		&result.Enabled)
	if err != nil {
		return Activation{}, ErrUnavailable
	}
	return result, nil
}

func (repository *PostgresRepository) Health(ctx context.Context, activationID string,
	now time.Time,
) (HealthStatus, error) {
	if repository == nil || repository.db == nil {
		return HealthStatus{}, ErrUnavailable
	}
	var result HealthStatus
	if err := repository.db.QueryRowContext(ctx, `
SELECT stale_claims,pending_terminalizations
FROM agent_product_canary_worker_health($1,$2)
`, activationID, now.UTC()).Scan(&result.StaleClaims, &result.PendingTerminalizations); err != nil {
		return HealthStatus{}, ErrUnavailable
	}
	return result, nil
}

func (repository *PostgresRepository) Claim(ctx context.Context, activationID, owner string,
	now time.Time, ttl time.Duration, limit int,
) ([]Request, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT request_id,activation_id,user_id::text,policy_revision,opt_generation,
       package_fingerprint,runtime_bundle_fingerprint,plan_fingerprint,request_fingerprint,
       state,claim_generation,claim_owner,claim_expires_at,failure_count
FROM agent_product_canary_worker_claim_requests($1,$2,$3,$4,$5)
`, activationID, owner, now.UTC(), int(ttl/time.Second), limit)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	result := make([]Request, 0, limit)
	for rows.Next() {
		var item Request
		if err := rows.Scan(&item.ID, &item.ActivationID, &item.UserID,
			&item.PolicyRevision, &item.OptGeneration, &item.PackageFingerprint,
			&item.RuntimeBundleFingerprint, &item.PlanFingerprint,
			&item.RequestFingerprint, &item.State, &item.ClaimGeneration,
			&item.ClaimOwner, &item.ClaimExpiresAt, &item.FailureCount); err != nil {
			return nil, ErrUnavailable
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrUnavailable
	}
	return result, nil
}

func (repository *PostgresRepository) Complete(ctx context.Context, request Request,
	result ExecutionResult,
) (Receipt, error) {
	if repository == nil || repository.db == nil {
		return Receipt{}, ErrUnavailable
	}
	var receipt Receipt
	err := repository.db.QueryRowContext(ctx, `
SELECT request_id,activation_id,run_id,attempt_id,run_snapshot_fingerprint,
       plan_fingerprint,receipt_fingerprint,outcome,created_at
FROM agent_product_canary_worker_complete_request(
  $1,$2,$3,$4,$5,$6,$7,$8,$9,'bounded_canary_passed'
)
`, request.ActivationID, request.ID, request.ClaimOwner, request.ClaimGeneration,
		result.RunID, result.AttemptID, result.SnapshotFingerprint,
		request.PlanFingerprint, result.ReceiptFingerprint,
	).Scan(&receipt.RequestID, &receipt.ActivationID, &receipt.RunID,
		&receipt.AttemptID, &receipt.SnapshotFingerprint, &receipt.PlanFingerprint,
		&receipt.ReceiptFingerprint, &receipt.Outcome, &receipt.CreatedAt)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	return receipt, nil
}

func (repository *PostgresRepository) Release(ctx context.Context, request Request,
	errorCode string, retry bool,
) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, ErrUnavailable
	}
	var terminal bool
	if err := repository.db.QueryRowContext(ctx, `
SELECT agent_product_canary_worker_release_request($1,$2,$3,$4,$5,$6)
`, request.ActivationID, request.ID, request.ClaimOwner, request.ClaimGeneration,
		errorCode, retry).Scan(&terminal); err != nil {
		return false, ErrUnavailable
	}
	return terminal, nil
}

func (repository *PostgresRepository) Reconcile(ctx context.Context, activationID string,
	now time.Time, limit int,
) (int, error) {
	if repository == nil || repository.db == nil {
		return 0, ErrUnavailable
	}
	var changed int
	if err := repository.db.QueryRowContext(ctx,
		`SELECT agent_product_canary_worker_reconcile($1,$2,$3)`,
		activationID, now.UTC(), limit).Scan(&changed); err != nil {
		return 0, ErrUnavailable
	}
	return changed, nil
}

var _ Repository = (*PostgresRepository)(nil)
