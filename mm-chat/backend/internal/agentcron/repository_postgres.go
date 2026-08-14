package agentcron

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

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (repository *PostgresRepository) CreateRevision(ctx context.Context, prepared preparedRevision) (Template, bool, error) {
	if repository == nil || repository.db == nil {
		return Template{}, false, ErrDatabaseRequired
	}
	var templateID, state string
	var revision int64
	var next time.Time
	var created bool
	err := repository.db.QueryRowContext(ctx, `
SELECT template_id,current_revision,state,next_trigger_at,created
FROM agent_cron_create_revision($1,$2::uuid,$3,$4,$5::jsonb,$6,$7,$8,$9,$10)
`, prepared.TemplateID, prepared.Spec.Owner.UserID, prepared.ExpectedRevision,
		prepared.RevisionFingerprint, string(prepared.CanonicalSpec), prepared.NextTriggerAt,
		prepared.Approval.ActorType, prepared.Approval.ActorID, prepared.Approval.ReasonCode,
		prepared.AuditEventID).Scan(&templateID, &revision, &state, &next, &created)
	if err != nil {
		return Template{}, false, mapPostgresError("create agent Cron revision", err)
	}
	template, err := repository.GetTemplate(ctx, prepared.Spec.Owner.UserID, templateID)
	return template, created, err
}

func (repository *PostgresRepository) GetTemplate(ctx context.Context, userID, templateID string) (Template, error) {
	if repository == nil || repository.db == nil {
		return Template{}, ErrDatabaseRequired
	}
	var result Template
	var next sql.NullTime
	var spec []byte
	err := repository.db.QueryRowContext(ctx, `
SELECT template.id,template.user_id::text,template.current_revision,template.current_revision_fingerprint,
       template.state,template.next_trigger_at,template.created_at,template.updated_at,revision.spec
FROM agent_cron_templates template
JOIN agent_cron_revisions revision ON revision.template_id=template.id AND revision.revision=template.current_revision
WHERE template.id=$1 AND template.user_id=$2::uuid
`, templateID, userID).Scan(&result.ID, &result.UserID, &result.CurrentRevision,
		&result.RevisionFingerprint, &result.State, &next, &result.CreatedAt, &result.UpdatedAt, &spec)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	if err != nil {
		return Template{}, fmt.Errorf("get agent Cron template: %w", err)
	}
	if next.Valid {
		value := next.Time.UTC()
		result.NextTriggerAt = &value
	}
	if err := json.Unmarshal(spec, &result.Spec); err != nil {
		return Template{}, fmt.Errorf("decode agent Cron revision: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) SetLifecycle(ctx context.Context, prepared preparedLifecycle) (Template, error) {
	if repository == nil || repository.db == nil {
		return Template{}, ErrDatabaseRequired
	}
	var next any
	if prepared.NextTriggerAt != nil {
		next = prepared.NextTriggerAt.UTC()
	}
	var result Template
	var nextResult sql.NullTime
	var spec []byte
	err := repository.db.QueryRowContext(ctx, `
WITH changed AS (
  SELECT (agent_cron_set_lifecycle($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10)).*
)
SELECT changed.id,changed.user_id::text,changed.current_revision,changed.current_revision_fingerprint,
       changed.state,changed.next_trigger_at,changed.created_at,changed.updated_at,revision.spec
FROM changed JOIN agent_cron_revisions revision
  ON revision.template_id=changed.id AND revision.revision=changed.current_revision
`, prepared.TemplateID, prepared.UserID, prepared.ExpectedRevision, prepared.To,
		next, prepared.MissedCount, prepared.ActorType, prepared.ActorID,
		prepared.ReasonCode, prepared.AuditEventID).Scan(&result.ID, &result.UserID,
		&result.CurrentRevision, &result.RevisionFingerprint, &result.State,
		&nextResult, &result.CreatedAt, &result.UpdatedAt, &spec)
	if err != nil {
		return Template{}, mapPostgresError("change agent Cron lifecycle", err)
	}
	if nextResult.Valid {
		value := nextResult.Time.UTC()
		result.NextTriggerAt = &value
	}
	if err := json.Unmarshal(spec, &result.Spec); err != nil {
		return Template{}, fmt.Errorf("decode agent Cron lifecycle revision: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) RevokeApproval(ctx context.Context, input RevokeApprovalInput) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, ErrDatabaseRequired
	}
	var created bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_cron_revoke_approval($1,$2::uuid,$3,$4,$5,$6,$7)
`, input.RevocationID, input.UserID, input.ApprovalID, input.ActorType,
		input.ActorID, input.ReasonCode, input.AuditEventID).Scan(&created)
	if err != nil {
		return false, mapPostgresError("revoke agent Cron approval", err)
	}
	return created, nil
}

func (repository *PostgresRepository) ClaimDue(ctx context.Context, request ClaimRequest) ([]DueClaim, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDatabaseRequired
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT template_id,user_id::text,revision,revision_fingerprint,next_trigger_at,
       claim_generation,claim_owner,claim_expires_at,spec
FROM agent_cron_claim_due($1,$2,$3,$4)
`, request.Owner, request.Now, int(request.LeaseDuration/time.Second), request.Limit)
	if err != nil {
		return nil, mapPostgresError("claim due agent Cron templates", err)
	}
	defer rows.Close()
	var result []DueClaim
	for rows.Next() {
		var item DueClaim
		var spec []byte
		if err := rows.Scan(&item.TemplateID, &item.UserID, &item.Revision,
			&item.RevisionFingerprint, &item.NextTriggerAt, &item.ClaimGeneration,
			&item.ClaimOwner, &item.ClaimExpiresAt, &spec); err != nil {
			return nil, fmt.Errorf("scan due agent Cron template: %w", err)
		}
		if err := json.Unmarshal(spec, &item.Spec); err != nil {
			return nil, fmt.Errorf("decode due agent Cron revision: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) Advance(ctx context.Context, input AdvanceInput) ([]Trigger, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDatabaseRequired
	}
	decisions, err := json.Marshal(input.Decisions)
	if err != nil {
		return nil, ErrInvalidInput
	}
	rows, err := repository.db.QueryContext(ctx, `
WITH advanced AS (
  SELECT * FROM agent_cron_advance_cursor($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
)
SELECT advanced.id,advanced.template_id,advanced.user_id::text,advanced.revision,
       advanced.revision_fingerprint,advanced.scheduled_for,advanced.occurrence_fingerprint,
       advanced.state,advanced.reason_code,COALESCE(advanced.run_id,''),advanced.retry_count,
       advanced.next_attempt_at,advanced.claim_generation,COALESCE(advanced.claim_owner,''),
       advanced.claim_expires_at,revision.spec
FROM advanced JOIN agent_cron_revisions revision
  ON revision.template_id=advanced.template_id AND revision.revision=advanced.revision
ORDER BY advanced.scheduled_for,advanced.id
`, input.Claim.TemplateID, input.Claim.Revision, input.Claim.RevisionFingerprint,
		input.Claim.ClaimOwner, input.Claim.ClaimGeneration, input.Claim.NextTriggerAt,
		input.ObservedAt, input.NextTriggerAt, string(decisions))
	if err != nil {
		return nil, mapPostgresError("advance agent Cron cursor", err)
	}
	defer rows.Close()
	return scanTriggers(rows)
}

func (repository *PostgresRepository) ClaimTriggers(ctx context.Context, request ClaimRequest) ([]Trigger, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDatabaseRequired
	}
	rows, err := repository.db.QueryContext(ctx, `
SELECT trigger_id,template_id,user_id::text,revision,revision_fingerprint,scheduled_for,
       occurrence_fingerprint,state,reason_code,run_id,retry_count,next_attempt_at,
       claim_generation,claim_owner,claim_expires_at,spec
FROM agent_cron_claim_triggers($1,$2,$3,$4)
`, request.Owner, request.Now, int(request.LeaseDuration/time.Second), request.Limit)
	if err != nil {
		return nil, mapPostgresError("claim agent Cron triggers", err)
	}
	defer rows.Close()
	return scanTriggers(rows)
}

type rowScanner interface{ Scan(...any) error }

func scanTrigger(row rowScanner) (Trigger, error) {
	var result Trigger
	var claimExpires sql.NullTime
	var spec []byte
	if err := row.Scan(&result.ID, &result.TemplateID, &result.UserID, &result.Revision,
		&result.RevisionFingerprint, &result.ScheduledFor, &result.OccurrenceFingerprint,
		&result.State, &result.ReasonCode, &result.RunID, &result.RetryCount,
		&result.NextAttemptAt, &result.ClaimGeneration, &result.ClaimOwner,
		&claimExpires, &spec); err != nil {
		return Trigger{}, err
	}
	if claimExpires.Valid {
		value := claimExpires.Time.UTC()
		result.ClaimExpiresAt = &value
	}
	if err := json.Unmarshal(spec, &result.Spec); err != nil {
		return Trigger{}, err
	}
	return result, nil
}

func scanTriggers(rows *sql.Rows) ([]Trigger, error) {
	var result []Trigger
	for rows.Next() {
		item, err := scanTrigger(rows)
		if err != nil {
			return nil, fmt.Errorf("scan agent Cron trigger: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (repository *PostgresRepository) EnqueueTrigger(ctx context.Context, envelope EnqueueEnvelope) (TriggerResult, error) {
	if repository == nil || repository.db == nil {
		return TriggerResult{}, ErrDatabaseRequired
	}
	var result TriggerResult
	err := repository.db.QueryRowContext(ctx, `
SELECT trigger_id,state,reason_code,run_id,created
FROM agent_cron_enqueue_trigger($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10::text[],$11)
`, envelope.Trigger.ID, envelope.Trigger.ClaimOwner, envelope.Trigger.ClaimGeneration,
		envelope.RunID, envelope.SnapshotID, envelope.SnapshotFingerprint,
		envelope.RequestFingerprint, string(envelope.CanonicalSnapshot), string(envelope.Steps),
		envelope.EventIDs, envelope.AuditEventID).Scan(&result.TriggerID, &result.State,
		&result.ReasonCode, &result.RunID, &result.Created)
	if err != nil {
		return TriggerResult{}, mapPostgresError("enqueue agent Cron trigger", err)
	}
	return result, nil
}

func (repository *PostgresRepository) ReleaseTrigger(ctx context.Context, input ReleaseInput) error {
	if repository == nil || repository.db == nil {
		return ErrDatabaseRequired
	}
	var terminal bool
	err := repository.db.QueryRowContext(ctx, `
SELECT agent_cron_release_trigger($1,$2,$3,$4,$5,$6)
`, input.TriggerID, input.ClaimOwner, input.ClaimGeneration, input.ErrorCode,
		input.RetryAt, input.AuditEventID).Scan(&terminal)
	if err != nil {
		return mapPostgresError("release agent Cron trigger", err)
	}
	return nil
}

func (repository *PostgresRepository) Reconcile(ctx context.Context, now time.Time, limit int) (CleanupResult, error) {
	if repository == nil || repository.db == nil {
		return CleanupResult{}, ErrDatabaseRequired
	}
	var result CleanupResult
	err := repository.db.QueryRowContext(ctx, `
SELECT cursor_claims_reclaimed,trigger_claims_reclaimed FROM agent_cron_reconcile($1,$2)
`, now, limit).Scan(&result.CursorClaimsReclaimed, &result.TriggerClaimsReclaimed)
	if err != nil {
		return CleanupResult{}, mapPostgresError("reconcile agent Cron claims", err)
	}
	return result, nil
}

func (repository *PostgresRepository) Prune(ctx context.Context, cutoff time.Time, limit int) (CleanupResult, error) {
	if repository == nil || repository.db == nil {
		return CleanupResult{}, ErrDatabaseRequired
	}
	var result CleanupResult
	err := repository.db.QueryRowContext(ctx, `
SELECT triggers_pruned,audits_pruned,templates_pruned FROM agent_cron_prune($1,$2)
`, cutoff, limit).Scan(&result.TriggersPruned, &result.AuditsPruned, &result.TemplatesPruned)
	if err != nil {
		return CleanupResult{}, mapPostgresError("prune agent Cron history", err)
	}
	return result, nil
}

func mapPostgresError(operation string, err error) error {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch strings.ToUpper(pgError.Message) {
		case "AGENT_CRON_NOT_FOUND":
			return ErrNotFound
		case "REVISION_CONFLICT":
			return ErrRevisionConflict
		case "STALE_CLAIM":
			return ErrStaleClaim
		case "STALE_TEMPLATE":
			return ErrStaleTemplate
		case "OWNER_REVOKED":
			return ErrOwnerRevoked
		case "SKILL_REVOKED":
			return ErrSkillRevoked
		case "GRANT_REVOKED":
			return ErrGrantRevoked
		case "APPROVAL_REVOKED":
			return ErrApprovalRevoked
		case "TEMPLATE_EXPIRED":
			return ErrExpired
		case "KILL_SWITCH_ACTIVE", "SECRET_REVOKED":
			return ErrKillSwitchActive
		case "IDEMPOTENCY_CONFLICT":
			return ErrIdempotencyConflict
		}
		if pgError.Code == "22023" || pgError.Code == "23514" {
			return fmt.Errorf("%w: %s", ErrInvalidInput, pgError.Message)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
