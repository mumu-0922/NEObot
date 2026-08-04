package memoryworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrDatabaseRequired = errors.New("memory worker database is required")

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Claim(
	ctx context.Context,
	workerID string,
	leaseToken string,
	leaseDuration time.Duration,
) (Job, bool, error) {
	if r == nil || r.db == nil {
		return Job{}, false, ErrDatabaseRequired
	}
	leaseSeconds := int(leaseDuration / time.Second)
	var job Job
	var sourceConversationID, sourceMessageID, assistantMessageID sql.NullString
	var sourceHash, providerSource, providerID, providerRecordID sql.NullString
	var modelID, processingProfile sql.NullString
	var projectScopeGeneration sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
SELECT
  job_id::text, user_id::text, event_id::text, event_schema_major,
  stage, attempt_count, max_attempts, source_conversation_id::text,
  source_message_id::text, assistant_message_id::text, source_hash,
  provider_source, provider_id, provider_record_id::text, model_id,
  processing_profile, scope_generation, project_scope_generation,
  visibility_epoch, lease_expires_at
FROM memory_worker_claim_job($1::uuid, $2::uuid, $3)
`, workerID, leaseToken, leaseSeconds).Scan(
		&job.JobID,
		&job.UserID,
		&job.EventID,
		&job.EventSchemaMajor,
		&job.Stage,
		&job.AttemptCount,
		&job.MaxAttempts,
		&sourceConversationID,
		&sourceMessageID,
		&assistantMessageID,
		&sourceHash,
		&providerSource,
		&providerID,
		&providerRecordID,
		&modelID,
		&processingProfile,
		&job.ScopeGeneration,
		&projectScopeGeneration,
		&job.VisibilityEpoch,
		&job.LeaseExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("claim memory job: %w", err)
	}
	job.SourceConversationID = sourceConversationID.String
	job.SourceMessageID = sourceMessageID.String
	job.AssistantMessageID = assistantMessageID.String
	job.SourceHash = sourceHash.String
	job.ProviderSource = providerSource.String
	job.ProviderID = providerID.String
	job.ProviderRecordID = providerRecordID.String
	job.ModelID = modelID.String
	job.ProcessingProfile = processingProfile.String
	if projectScopeGeneration.Valid {
		value := projectScopeGeneration.Int64
		job.ProjectScopeGeneration = &value
	}
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *PostgresRepository) Hydrate(ctx context.Context, job Job) (Capture, error) {
	if r == nil || r.db == nil {
		return Capture{}, ErrDatabaseRequired
	}
	var capture Capture
	var projectID, secretRef sql.NullString
	var contextMessages, currentMemories, providerConfig []byte
	err := r.db.QueryRowContext(ctx, `
SELECT
  user_id::text, context_messages, current_memories,
  sensitive_memory_enabled, project_id::text, provider_record_id::text,
  provider_id, provider_label, encrypted_secret_ref, provider_config,
  model_id, processing_profile, proposal_committed
FROM memory_worker_hydrate_capture_v2($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&capture.UserID,
		&contextMessages,
		&currentMemories,
		&capture.SensitiveMemoryEnabled,
		&projectID,
		&capture.ProviderRecordID,
		&capture.ProviderID,
		&capture.ProviderLabel,
		&secretRef,
		&providerConfig,
		&capture.ModelID,
		&capture.ProcessingProfile,
		&capture.ProposalCommitted,
	)
	if err != nil {
		return Capture{}, fmt.Errorf("hydrate memory capture: %w", err)
	}
	capture.EncryptedSecretRef = secretRef.String
	capture.ProjectID = projectID.String
	capture.ProviderConfig = append(json.RawMessage(nil), providerConfig...)
	if err := json.Unmarshal(contextMessages, &capture.Messages); err != nil {
		return Capture{}, fmt.Errorf("decode memory capture context: %w", err)
	}
	if err := json.Unmarshal(currentMemories, &capture.CurrentMemories); err != nil {
		return Capture{}, fmt.Errorf("decode current Memory context: %w", err)
	}
	return capture, nil
}

func (r *PostgresRepository) ProposeCandidates(
	ctx context.Context,
	job Job,
	batch ProposalBatch,
) (ProposalSummary, error) {
	if r == nil || r.db == nil {
		return ProposalSummary{}, ErrDatabaseRequired
	}
	candidates, err := json.Marshal(batch.Candidates)
	if err != nil {
		return ProposalSummary{}, fmt.Errorf("encode memory capture proposals: %w", err)
	}
	var summary ProposalSummary
	row := r.db.QueryRowContext(ctx, `
SELECT
  proposal_count, shadow_count, review_count, rejected_count
FROM memory_worker_propose_capture_candidates(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::smallint, $6, $7, $8::jsonb
)
`,
		job.JobID,
		job.WorkerID,
		job.LeaseToken,
		batch.ExpiryJobID,
		batch.CandidateSchemaMajor,
		batch.ExtractionProfileID,
		batch.DecisionProfileID,
		string(candidates),
	)
	if err := row.Scan(
		&summary.ProposalCount,
		&summary.ShadowCount,
		&summary.ReviewCount,
		&summary.RejectedCount,
	); err != nil {
		return ProposalSummary{}, fmt.Errorf("propose memory capture candidates: %w", err)
	}
	return summary, nil
}

func (r *PostgresRepository) PromoteCandidates(
	ctx context.Context,
	job Job,
) (PromotionSummary, error) {
	if r == nil || r.db == nil {
		return PromotionSummary{}, ErrDatabaseRequired
	}
	var summary PromotionSummary
	if err := r.db.QueryRowContext(ctx, `
SELECT promoted_count, review_count, rejected_count
FROM memory_worker_promote_capture_candidates($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&summary.PromotedCount,
		&summary.ReviewCount,
		&summary.RejectedCount,
	); err != nil {
		return PromotionSummary{}, fmt.Errorf("promote memory capture candidates: %w", err)
	}
	return summary, nil
}

func (r *PostgresRepository) Purge(ctx context.Context, job Job) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	var purged bool
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_purge_memory($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(&purged); err != nil {
		return fmt.Errorf("purge deleted memory: %w", err)
	}
	if !purged {
		return errors.New("purge deleted memory returned false")
	}
	return nil
}

func (r *PostgresRepository) ExpireReviews(ctx context.Context, job Job) (int, error) {
	if r == nil || r.db == nil {
		return 0, ErrDatabaseRequired
	}
	var expired int
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_expire_capture_reviews($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(&expired); err != nil {
		return 0, fmt.Errorf("expire memory capture reviews: %w", err)
	}
	return expired, nil
}

func (r *PostgresRepository) Complete(ctx context.Context, job Job) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	if _, err := r.db.ExecContext(ctx, `
SELECT memory_worker_complete_job($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken); err != nil {
		return fmt.Errorf("complete memory job: %w", err)
	}
	return nil
}

func (r *PostgresRepository) Retry(
	ctx context.Context,
	job Job,
	errorCode string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrDatabaseRequired
	}
	var status string
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_retry_job(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6
)
`, job.JobID, job.WorkerID, job.LeaseToken, errorCode, availableAt, terminal).Scan(&status); err != nil {
		return "", fmt.Errorf("retry memory job: %w", err)
	}
	return status, nil
}

func (r *PostgresRepository) CheckReady(ctx context.Context) (Readiness, error) {
	if r == nil || r.db == nil {
		return Readiness{}, ErrDatabaseRequired
	}
	var readiness Readiness
	var oldest sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT consumer_ready, pending_count, processing_count,
       dead_letter_count, oldest_pending_at
FROM memory_worker_readiness()
`).Scan(
		&readiness.ConsumerReady,
		&readiness.PendingCount,
		&readiness.ProcessingCount,
		&readiness.DeadLetterCount,
		&oldest,
	)
	if err != nil {
		return Readiness{}, fmt.Errorf("check memory worker readiness: %w", err)
	}
	if oldest.Valid {
		value := oldest.Time
		readiness.OldestPendingAt = &value
	}
	return readiness, nil
}
