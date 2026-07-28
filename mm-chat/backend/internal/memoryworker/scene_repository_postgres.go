package memoryworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (r *PostgresRepository) ClaimScene(
	ctx context.Context,
	workerID string,
	leaseToken string,
	leaseDuration time.Duration,
	refreshEnabled bool,
) (SceneJob, bool, error) {
	if r == nil || r.db == nil {
		return SceneJob{}, false, ErrDatabaseRequired
	}
	var job SceneJob
	var scopeType, projectID, targetSceneID, sourceWatermark sql.NullString
	var scopeGeneration sql.NullInt64
	var providerRecordID, modelID sql.NullString
	var providerUpdatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT job_id::text, stage, user_id::text, scope_type,
       project_id::text, target_scene_id::text, scope_generation,
       visibility_epoch, generation, profile_id, source_watermark,
       attempt_count, max_attempts, provider_record_id::text,
       provider_config_updated_at, model_id, lease_expires_at
FROM memory_worker_claim_l2_scene_job($1::uuid, $2::uuid, $3, $4)
`, workerID, leaseToken, int(leaseDuration/time.Second), refreshEnabled).Scan(
		&job.JobID,
		&job.Stage,
		&job.UserID,
		&scopeType,
		&projectID,
		&targetSceneID,
		&scopeGeneration,
		&job.VisibilityEpoch,
		&job.Generation,
		&job.ProfileID,
		&sourceWatermark,
		&job.AttemptCount,
		&job.MaxAttempts,
		&providerRecordID,
		&providerUpdatedAt,
		&modelID,
		&job.LeaseExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SceneJob{}, false, nil
	}
	if err != nil {
		return SceneJob{}, false, fmt.Errorf("claim L2 Scene job: %w", err)
	}
	job.ScopeType = scopeType.String
	job.ProjectID = projectID.String
	job.TargetSceneID = targetSceneID.String
	if scopeGeneration.Valid {
		job.ScopeGeneration = scopeGeneration.Int64
	}
	job.SourceWatermark = sourceWatermark.String
	job.ProviderRecordID = providerRecordID.String
	job.ProviderConfigUpdatedAt = providerUpdatedAt.Time
	job.ModelID = modelID.String
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *PostgresRepository) HydrateSceneRefresh(
	ctx context.Context,
	job SceneJob,
) (SceneCapture, error) {
	if r == nil || r.db == nil {
		return SceneCapture{}, ErrDatabaseRequired
	}
	var capture SceneCapture
	var projectID, providerRecordID, providerID, providerLabel sql.NullString
	var encryptedSecretRef, modelID sql.NullString
	var providerConfig, memories []byte
	var providerUpdatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT user_id::text, scope_type, project_id::text, scope_generation,
       visibility_epoch, generation, profile_id, source_watermark,
       sensitive_memory_enabled, memories, provider_record_id::text,
       provider_id, provider_label, encrypted_secret_ref, provider_config,
       provider_config_updated_at, model_id
FROM memory_worker_hydrate_l2_scene_refresh($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&capture.UserID,
		&capture.ScopeType,
		&projectID,
		&capture.ScopeGeneration,
		&capture.VisibilityEpoch,
		&capture.Generation,
		&capture.ProfileID,
		&capture.SourceWatermark,
		&capture.SensitiveMemoryEnabled,
		&memories,
		&providerRecordID,
		&providerID,
		&providerLabel,
		&encryptedSecretRef,
		&providerConfig,
		&providerUpdatedAt,
		&modelID,
	)
	if err != nil {
		return SceneCapture{}, fmt.Errorf("hydrate L2 Scene refresh: %w", err)
	}
	if !json.Valid(memories) {
		return SceneCapture{}, errors.New("hydrate L2 Scene refresh: invalid memories")
	}
	if len(providerConfig) > 0 && !json.Valid(providerConfig) {
		return SceneCapture{}, errors.New("hydrate L2 Scene refresh: invalid provider config")
	}
	if err := json.Unmarshal(memories, &capture.Memories); err != nil {
		return SceneCapture{}, fmt.Errorf("decode L2 Scene memories: %w", err)
	}
	capture.ProjectID = projectID.String
	capture.ProviderRecordID = providerRecordID.String
	capture.ProviderID = providerID.String
	capture.ProviderLabel = providerLabel.String
	capture.EncryptedSecretRef = encryptedSecretRef.String
	capture.ProviderConfig = append(json.RawMessage(nil), providerConfig...)
	capture.ProviderConfigUpdatedAt = providerUpdatedAt.Time
	capture.ModelID = modelID.String
	return capture, nil
}

func (r *PostgresRepository) CompleteSceneRefresh(
	ctx context.Context,
	job SceneJob,
	proposals []SceneProposal,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	payload, err := json.Marshal(proposals)
	if err != nil {
		return fmt.Errorf("encode L2 Scene proposals: %w", err)
	}
	var result []byte
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_refresh(
  $1::uuid, $2::uuid, $3::uuid, $4::jsonb
)
`, job.JobID, job.WorkerID, job.LeaseToken, string(payload)).Scan(&result); err != nil {
		return fmt.Errorf("complete L2 Scene refresh: %w", err)
	}
	if !json.Valid(result) {
		return errors.New("complete L2 Scene refresh returned invalid result")
	}
	return nil
}

func (r *PostgresRepository) CompleteScenePurge(
	ctx context.Context,
	job SceneJob,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	var completed bool
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_purge($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(&completed); err != nil {
		return fmt.Errorf("complete L2 Scene purge: %w", err)
	}
	if !completed {
		return errors.New("complete L2 Scene purge returned false")
	}
	return nil
}

func (r *PostgresRepository) RetryScene(
	ctx context.Context,
	job SceneJob,
	errorCode string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrDatabaseRequired
	}
	var status string
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_retry_l2_scene_job(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6
)
`, job.JobID, job.WorkerID, job.LeaseToken, errorCode, availableAt, terminal).Scan(
		&status,
	); err != nil {
		return "", fmt.Errorf("retry L2 Scene job: %w", err)
	}
	return status, nil
}

func (r *PostgresRepository) ClaimSceneEmbedding(
	ctx context.Context,
	workerID string,
	leaseToken string,
	leaseDuration time.Duration,
) (SceneEmbeddingJob, bool, error) {
	if r == nil || r.db == nil {
		return SceneEmbeddingJob{}, false, ErrDatabaseRequired
	}
	var job SceneEmbeddingJob
	err := r.db.QueryRowContext(ctx, `
SELECT job_id::text, user_id::text, scene_id::text, scene_revision,
       content_hash, source_watermark, visibility_epoch, generation,
       embedding_profile_id, embedding_model_id, embedding_dimensions,
       attempt_count, max_attempts, provider_record_id::text,
       provider_config_updated_at, lease_expires_at
FROM memory_worker_claim_l2_scene_embedding_job($1::uuid, $2::uuid, $3)
`, workerID, leaseToken, int(leaseDuration/time.Second)).Scan(
		&job.JobID,
		&job.UserID,
		&job.SceneID,
		&job.SceneRevision,
		&job.ContentHash,
		&job.SourceWatermark,
		&job.VisibilityEpoch,
		&job.Generation,
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
		return SceneEmbeddingJob{}, false, nil
	}
	if err != nil {
		return SceneEmbeddingJob{}, false, fmt.Errorf("claim L2 Scene embedding job: %w", err)
	}
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *PostgresRepository) HydrateSceneEmbedding(
	ctx context.Context,
	job SceneEmbeddingJob,
) (SceneEmbeddingCapture, error) {
	if r == nil || r.db == nil {
		return SceneEmbeddingCapture{}, ErrDatabaseRequired
	}
	var capture SceneEmbeddingCapture
	var providerConfig []byte
	err := r.db.QueryRowContext(ctx, `
SELECT user_id::text, scene_id::text, content, content_hash, scene_revision,
       source_watermark, visibility_epoch, generation, embedding_profile_id,
       embedding_model_id, embedding_dimensions, provider_record_id::text,
       provider_id, provider_label, encrypted_secret_ref, provider_config,
       provider_config_updated_at
FROM memory_worker_hydrate_l2_scene_embedding_job($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&capture.UserID,
		&capture.SceneID,
		&capture.Content,
		&capture.ContentHash,
		&capture.SceneRevision,
		&capture.SourceWatermark,
		&capture.VisibilityEpoch,
		&capture.Generation,
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
		return SceneEmbeddingCapture{}, fmt.Errorf("hydrate L2 Scene embedding job: %w", err)
	}
	if !json.Valid(providerConfig) {
		return SceneEmbeddingCapture{}, errors.New("hydrate L2 Scene embedding: invalid provider config")
	}
	capture.ProviderConfig = append(json.RawMessage(nil), providerConfig...)
	return capture, nil
}

func (r *PostgresRepository) CompleteSceneEmbedding(
	ctx context.Context,
	job SceneEmbeddingJob,
	vector []float32,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	if !validMemoryEmbeddingVector(vector, job.EmbeddingDimensions) {
		return errors.New("complete L2 Scene embedding: invalid vector")
	}
	var completed bool
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l2_scene_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4::real[]
)
`, job.JobID, job.WorkerID, job.LeaseToken, memoryEmbeddingRealArray(vector)).Scan(
		&completed,
	); err != nil {
		return fmt.Errorf("complete L2 Scene embedding job: %w", err)
	}
	if !completed {
		return errors.New("complete L2 Scene embedding returned false")
	}
	return nil
}

func (r *PostgresRepository) RetrySceneEmbedding(
	ctx context.Context,
	job SceneEmbeddingJob,
	errorCode string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrDatabaseRequired
	}
	var status string
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_retry_l2_scene_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6
)
`, job.JobID, job.WorkerID, job.LeaseToken, errorCode, availableAt, terminal).Scan(
		&status,
	); err != nil {
		return "", fmt.Errorf("retry L2 Scene embedding job: %w", err)
	}
	return status, nil
}

var _ SceneRepository = (*PostgresRepository)(nil)
