package agentrunner

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type PostgresControlRepository struct{ db *sql.DB }

func NewPostgresControlRepository(db *sql.DB) *PostgresControlRepository {
	return &PostgresControlRepository{db: db}
}

func (repository *PostgresControlRepository) IssueAuthority(ctx context.Context, input IssueAuthorityInput, tokenHash string) (AuthorityRecord, error) {
	if repository == nil || repository.db == nil {
		return AuthorityRecord{}, ErrRuntimeUnavailable
	}
	var record AuthorityRecord
	var response []byte
	err := repository.db.QueryRowContext(ctx, `
SELECT runner_id,caller_identity,request_id,method,run_id,step_id,attempt_id,
       lease_generation,lease_owner,lease_token_hash,snapshot_fingerprint,
       kill_switch_epoch,issued_at,expires_at,replayed,COALESCE(response_body,'null'::jsonb)
FROM agent_runner_issue_authority(
  $1,$2,$3,$4,$5,$6,$7::uuid,$8,$9,$10,$11,$12,$13,$14,$15
)`, input.CallerIdentity, input.RequestID, input.Nonce, input.Method, input.RequestFingerprint,
		input.RunnerID, input.UserID, input.Attempt.RunID, input.Attempt.StepID, input.Attempt.AttemptID,
		input.Attempt.LeaseGeneration, input.Attempt.LeaseOwner, tokenHash, input.SnapshotFingerprint,
		int(input.TTL/time.Second)).Scan(&record.RunnerID, &record.CallerIdentity, &record.RequestID, &record.Method,
		&record.RunID, &record.StepID, &record.AttemptID, &record.LeaseGeneration, &record.LeaseOwner,
		&record.LeaseTokenDigest, &record.SnapshotFingerprint, &record.KillSwitchEpoch, &record.IssuedAt,
		&record.ExpiresAt, &record.Replay, &response)
	if err != nil {
		return AuthorityRecord{}, mapControlError(fmt.Errorf("issue Runner authority: %w", err))
	}
	record.SchemaVersion = AuthorityVersion
	record.Nonce = input.Nonce
	record.RequestFingerprint = input.RequestFingerprint
	if record.Replay && string(response) != "null" {
		record.Response = append([]byte(nil), response...)
	}
	return record, nil
}

func (repository *PostgresControlRepository) CompleteRequest(ctx context.Context, caller, requestID, nonce, requestFingerprint, responseFingerprint string, response []byte) error {
	var accepted bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_runner_complete_request($1,$2,$3,$4,$5,$6::jsonb)`, caller, requestID, nonce, requestFingerprint, responseFingerprint, string(response)).Scan(&accepted)
	if err != nil {
		return mapControlError(err)
	}
	if !accepted {
		return ErrInvalidTransition
	}
	return nil
}
func (repository *PostgresControlRepository) ExpectSandbox(ctx context.Context, input ExpectedSandbox) error {
	var ok bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_runner_expect_sandbox($1,$2,$3,$4,$5::uuid,$6,$7,$8,$9,$10,$11,$12,$13)`,
		input.SandboxID, input.CallerIdentity, input.RequestID, input.Nonce, input.UserID,
		input.Attempt.RunID, input.Attempt.StepID, input.Attempt.AttemptID,
		input.Attempt.LeaseGeneration, input.RunnerID, input.SnapshotFingerprint,
		input.SpecFingerprint, input.ProbeFingerprint).Scan(&ok)
	if err != nil {
		return mapControlError(err)
	}
	if !ok {
		return ErrInvalidTransition
	}
	return nil
}
func (repository *PostgresControlRepository) UpdateSandbox(ctx context.Context, sandboxID, attemptID string, generation int64, expected, to, terminal string) error {
	var ok bool
	err := repository.db.QueryRowContext(ctx, `SELECT agent_runner_update_sandbox($1,$2,$3,$4,$5,NULLIF($6,''))`, sandboxID, attemptID, generation, expected, to, terminal).Scan(&ok)
	if err != nil {
		return mapControlError(err)
	}
	if !ok {
		return ErrInvalidTransition
	}
	return nil
}
func (repository *PostgresControlRepository) RecoverySandboxes(ctx context.Context, limit int) ([]RecoverySandbox, error) {
	if limit < 1 || limit > 1000 {
		return nil, ErrInvalidInput
	}
	rows, err := repository.db.QueryContext(ctx, `SELECT sandbox_id,user_id::text,run_id,step_id,attempt_id,lease_generation,runner_id,snapshot_fingerprint,spec_fingerprint,probe_fingerprint,sandbox_state,attempt_state,lease_expires_at,lease_expired FROM agent_runner_recovery_sandboxes($1)`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RecoverySandbox{}
	for rows.Next() {
		var item RecoverySandbox
		if err := rows.Scan(&item.SandboxID, &item.UserID, &item.Attempt.RunID, &item.Attempt.StepID, &item.Attempt.AttemptID, &item.Attempt.LeaseGeneration, &item.RunnerID, &item.SnapshotFingerprint, &item.SpecFingerprint, &item.ProbeFingerprint, &item.SandboxState, &item.AttemptState, &item.LeaseExpiresAt, &item.LeaseExpired); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
func (repository *PostgresControlRepository) Prune(ctx context.Context, cutoff time.Time, limit int) (int, int, error) {
	if cutoff.IsZero() || limit < 1 || limit > 1000 {
		return 0, 0, ErrInvalidInput
	}
	var requests, sandboxes int
	err := repository.db.QueryRowContext(ctx, `SELECT requests_deleted,sandboxes_deleted FROM agent_runner_prune($1,$2)`, cutoff, limit).Scan(&requests, &sandboxes)
	return requests, sandboxes, err
}
