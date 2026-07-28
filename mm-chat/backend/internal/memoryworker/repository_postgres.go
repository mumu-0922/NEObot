package memoryworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neo-chat/mm-chat/backend/internal/usermemory"
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
	var secretRef sql.NullString
	var providerConfig []byte
	err := r.db.QueryRowContext(ctx, `
SELECT
  user_id::text, user_message_content, provider_record_id::text,
  provider_id, provider_label, encrypted_secret_ref, provider_config,
  model_id, processing_profile
FROM memory_worker_hydrate_capture($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&capture.UserID,
		&capture.UserMessageContent,
		&capture.ProviderRecordID,
		&capture.ProviderID,
		&capture.ProviderLabel,
		&secretRef,
		&providerConfig,
		&capture.ModelID,
		&capture.ProcessingProfile,
	)
	if err != nil {
		return Capture{}, fmt.Errorf("hydrate memory capture: %w", err)
	}
	capture.EncryptedSecretRef = secretRef.String
	capture.ProviderConfig = append(json.RawMessage(nil), providerConfig...)
	return capture, nil
}

func (r *PostgresRepository) ApplyCandidate(
	ctx context.Context,
	job Job,
	input usermemory.CreateInput,
) (usermemory.Memory, error) {
	if r == nil || r.db == nil {
		return usermemory.Memory{}, ErrDatabaseRequired
	}
	row := r.db.QueryRowContext(ctx, `
SELECT
  id::text, user_id::text, memory_type, content, normalized_content,
  importance, tags_json, source, source_conversation_id::text,
  source_message_id::text, enabled, last_used_at, created_at, updated_at,
  deleted_at
FROM memory_worker_apply_capture_candidate(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8::smallint, $9
)
`,
		job.JobID,
		job.WorkerID,
		job.LeaseToken,
		input.ID,
		input.Type,
		input.Content,
		input.NormalizedContent,
		input.Importance,
		input.Tags,
	)
	var memory usermemory.Memory
	var tagsJSON string
	var sourceConversationID, sourceMessageID sql.NullString
	err := row.Scan(
		&memory.ID,
		&memory.UserID,
		&memory.Type,
		&memory.Content,
		&memory.NormalizedContent,
		&memory.Importance,
		&tagsJSON,
		&memory.Source,
		&sourceConversationID,
		&sourceMessageID,
		&memory.Enabled,
		&memory.LastUsedAt,
		&memory.CreatedAt,
		&memory.UpdatedAt,
		&memory.DeletedAt,
	)
	if err != nil {
		return usermemory.Memory{}, fmt.Errorf("apply memory capture candidate: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &memory.Tags); err != nil {
		return usermemory.Memory{}, fmt.Errorf("decode memory capture tags: %w", err)
	}
	memory.SourceConversationID = sourceConversationID.String
	memory.SourceMessageID = sourceMessageID.String
	return memory, nil
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
