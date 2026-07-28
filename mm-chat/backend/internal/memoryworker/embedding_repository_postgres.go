package memoryworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (r *PostgresRepository) ClaimEmbedding(
	ctx context.Context,
	workerID string,
	leaseToken string,
	leaseDuration time.Duration,
) (EmbeddingJob, bool, error) {
	if r == nil || r.db == nil {
		return EmbeddingJob{}, false, ErrDatabaseRequired
	}
	var job EmbeddingJob
	err := r.db.QueryRowContext(ctx, `
SELECT job_id::text, user_id::text, memory_id::text,
       projection_generation, memory_revision, content_hash,
       visibility_epoch, scope_type, COALESCE(project_id::text, ''),
       COALESCE(scope_conversation_id::text, ''), scope_generation,
       embedding_profile_id, embedding_model_id, embedding_dimensions,
       attempt_count, max_attempts, provider_record_id::text,
       provider_config_updated_at, lease_expires_at
FROM memory_worker_claim_embedding_job($1::uuid, $2::uuid, $3)
`, workerID, leaseToken, int(leaseDuration/time.Second)).Scan(
		&job.JobID,
		&job.UserID,
		&job.MemoryID,
		&job.ProjectionGeneration,
		&job.MemoryRevision,
		&job.ContentHash,
		&job.VisibilityEpoch,
		&job.ScopeType,
		&job.ProjectID,
		&job.ScopeConversationID,
		&job.ScopeGeneration,
		&job.EmbeddingProfileID,
		&job.EmbeddingModelID,
		&job.EmbeddingDimensions,
		&job.AttemptCount,
		&job.MaxAttempts,
		&job.ProviderRecordID,
		&job.ProviderConfigUpdatedAt,
		&job.LeaseExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return EmbeddingJob{}, false, nil
	}
	if err != nil {
		return EmbeddingJob{}, false, fmt.Errorf("claim memory embedding job: %w", err)
	}
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *PostgresRepository) HydrateEmbedding(
	ctx context.Context,
	job EmbeddingJob,
) (EmbeddingCapture, error) {
	if r == nil || r.db == nil {
		return EmbeddingCapture{}, ErrDatabaseRequired
	}
	var capture EmbeddingCapture
	var providerConfig []byte
	err := r.db.QueryRowContext(ctx, `
SELECT user_id::text, memory_id::text, content, content_hash,
       memory_revision, projection_generation, visibility_epoch, scope_type,
       COALESCE(project_id::text, ''),
       COALESCE(scope_conversation_id::text, ''), scope_generation,
       embedding_profile_id,
       embedding_model_id, embedding_dimensions, provider_record_id::text,
       provider_id, provider_label, encrypted_secret_ref, provider_config,
       provider_config_updated_at
FROM memory_worker_hydrate_embedding_job($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&capture.UserID,
		&capture.MemoryID,
		&capture.Content,
		&capture.ContentHash,
		&capture.MemoryRevision,
		&capture.ProjectionGeneration,
		&capture.VisibilityEpoch,
		&capture.ScopeType,
		&capture.ProjectID,
		&capture.ScopeConversationID,
		&capture.ScopeGeneration,
		&capture.EmbeddingProfileID,
		&capture.EmbeddingModelID,
		&capture.EmbeddingDimensions,
		&capture.ProviderRecordID,
		&capture.ProviderID,
		&capture.ProviderLabel,
		&capture.EncryptedSecretRef,
		&providerConfig,
		&capture.ProviderConfigUpdatedAt,
	)
	if err != nil {
		return EmbeddingCapture{}, fmt.Errorf("hydrate memory embedding job: %w", err)
	}
	if !json.Valid(providerConfig) {
		return EmbeddingCapture{}, errors.New("hydrate memory embedding job: invalid provider config")
	}
	capture.ProviderConfig = append(json.RawMessage(nil), providerConfig...)
	return capture, nil
}

func (r *PostgresRepository) CompleteEmbedding(
	ctx context.Context,
	job EmbeddingJob,
	vector []float32,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	if !validMemoryEmbeddingVector(vector, job.EmbeddingDimensions) {
		return errors.New("complete memory embedding job: invalid vector")
	}
	var completed bool
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_complete_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4::real[]
)
`, job.JobID, job.WorkerID, job.LeaseToken, memoryEmbeddingRealArray(vector)).Scan(&completed); err != nil {
		return fmt.Errorf("complete memory embedding job: %w", err)
	}
	if !completed {
		return errors.New("complete memory embedding job returned false")
	}
	return nil
}

func (r *PostgresRepository) RetryEmbedding(
	ctx context.Context,
	job EmbeddingJob,
	errorCode string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrDatabaseRequired
	}
	var status string
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_retry_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6
)
`, job.JobID, job.WorkerID, job.LeaseToken, errorCode, availableAt, terminal).Scan(&status); err != nil {
		return "", fmt.Errorf("retry memory embedding job: %w", err)
	}
	return status, nil
}

func memoryEmbeddingRealArray(values []float32) string {
	var builder strings.Builder
	builder.Grow(len(values) * 12)
	builder.WriteByte('{')
	for index, value := range values {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte('}')
	return builder.String()
}

var _ EmbeddingRepository = (*PostgresRepository)(nil)
