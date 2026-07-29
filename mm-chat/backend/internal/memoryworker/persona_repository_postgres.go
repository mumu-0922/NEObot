package memoryworker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (r *PostgresRepository) ClaimPersona(
	ctx context.Context,
	workerID string,
	leaseToken string,
	leaseDuration time.Duration,
	refreshEnabled bool,
) (PersonaJob, bool, error) {
	if r == nil || r.db == nil {
		return PersonaJob{}, false, ErrDatabaseRequired
	}
	var job PersonaJob
	var targetPersonaID, sourceWatermark sql.NullString
	var providerRecordID, modelID sql.NullString
	var providerUpdatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT job_id::text, stage, user_id::text, target_persona_id::text,
       visibility_epoch, generation, profile_id, source_watermark,
       attempt_count, max_attempts, provider_record_id::text,
       provider_config_updated_at, model_id, lease_expires_at
FROM memory_worker_claim_l3_persona_job($1::uuid, $2::uuid, $3, $4)
`, workerID, leaseToken, int(leaseDuration/time.Second), refreshEnabled).Scan(
		&job.JobID,
		&job.Stage,
		&job.UserID,
		&targetPersonaID,
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
		return PersonaJob{}, false, nil
	}
	if err != nil {
		return PersonaJob{}, false, fmt.Errorf("claim L3 Persona job: %w", err)
	}
	job.TargetPersonaID = targetPersonaID.String
	job.SourceWatermark = sourceWatermark.String
	job.ProviderRecordID = providerRecordID.String
	job.ProviderConfigUpdatedAt = providerUpdatedAt.Time
	job.ModelID = modelID.String
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *PostgresRepository) HydratePersonaRefresh(
	ctx context.Context,
	job PersonaJob,
) (PersonaCapture, error) {
	if r == nil || r.db == nil {
		return PersonaCapture{}, ErrDatabaseRequired
	}
	var capture PersonaCapture
	var providerRecordID, providerID, providerLabel sql.NullString
	var encryptedSecretRef, modelID sql.NullString
	var providerConfig, memories []byte
	var providerUpdatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
SELECT user_id::text, visibility_epoch, generation, profile_id,
       source_watermark, sensitive_memory_enabled, memories,
       provider_record_id::text, provider_id, provider_label,
       encrypted_secret_ref, provider_config, provider_config_updated_at,
       model_id
FROM memory_worker_hydrate_l3_persona_refresh($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&capture.UserID,
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
		return PersonaCapture{}, fmt.Errorf("hydrate L3 Persona refresh: %w", err)
	}
	if !json.Valid(memories) {
		return PersonaCapture{}, errors.New("hydrate L3 Persona refresh: invalid memories")
	}
	if len(providerConfig) > 0 && !json.Valid(providerConfig) {
		return PersonaCapture{}, errors.New("hydrate L3 Persona refresh: invalid provider config")
	}
	if err := json.Unmarshal(memories, &capture.Memories); err != nil {
		return PersonaCapture{}, fmt.Errorf("decode L3 Persona memories: %w", err)
	}
	capture.ProviderRecordID = providerRecordID.String
	capture.ProviderID = providerID.String
	capture.ProviderLabel = providerLabel.String
	capture.EncryptedSecretRef = encryptedSecretRef.String
	capture.ProviderConfig = append(json.RawMessage(nil), providerConfig...)
	capture.ProviderConfigUpdatedAt = providerUpdatedAt.Time
	capture.ModelID = modelID.String
	return capture, nil
}

func (r *PostgresRepository) CompletePersonaRefresh(
	ctx context.Context,
	job PersonaJob,
	proposal *PersonaProposal,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	payload, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("encode L3 Persona proposal: %w", err)
	}
	var result []byte
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_refresh(
  $1::uuid, $2::uuid, $3::uuid, $4::jsonb
)
`, job.JobID, job.WorkerID, job.LeaseToken, string(payload)).Scan(&result); err != nil {
		return fmt.Errorf("complete L3 Persona refresh: %w", err)
	}
	if !json.Valid(result) {
		return errors.New("complete L3 Persona refresh returned invalid result")
	}
	return nil
}

func (r *PostgresRepository) CompletePersonaPurge(
	ctx context.Context,
	job PersonaJob,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	var completed bool
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_purge($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(&completed); err != nil {
		return fmt.Errorf("complete L3 Persona purge: %w", err)
	}
	if !completed {
		return errors.New("complete L3 Persona purge returned false")
	}
	return nil
}

func (r *PostgresRepository) RetryPersona(
	ctx context.Context,
	job PersonaJob,
	errorCode string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrDatabaseRequired
	}
	var status string
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_retry_l3_persona_job(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6
)
`, job.JobID, job.WorkerID, job.LeaseToken, errorCode, availableAt, terminal).Scan(
		&status,
	); err != nil {
		return "", fmt.Errorf("retry L3 Persona job: %w", err)
	}
	return status, nil
}

func (r *PostgresRepository) ClaimPersonaEmbedding(
	ctx context.Context,
	workerID string,
	leaseToken string,
	leaseDuration time.Duration,
) (PersonaEmbeddingJob, bool, error) {
	if r == nil || r.db == nil {
		return PersonaEmbeddingJob{}, false, ErrDatabaseRequired
	}
	var job PersonaEmbeddingJob
	err := r.db.QueryRowContext(ctx, `
SELECT job_id::text, user_id::text, persona_id::text, persona_revision,
       content_hash, source_watermark, visibility_epoch, generation,
       embedding_profile_id, embedding_model_id, embedding_dimensions,
       attempt_count, max_attempts, provider_record_id::text,
       provider_config_updated_at, lease_expires_at
FROM memory_worker_claim_l3_persona_embedding_job($1::uuid, $2::uuid, $3)
`, workerID, leaseToken, int(leaseDuration/time.Second)).Scan(
		&job.JobID,
		&job.UserID,
		&job.PersonaID,
		&job.PersonaRevision,
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
		return PersonaEmbeddingJob{}, false, nil
	}
	if err != nil {
		return PersonaEmbeddingJob{}, false, fmt.Errorf("claim L3 Persona embedding job: %w", err)
	}
	job.WorkerID = workerID
	job.LeaseToken = leaseToken
	return job, true, nil
}

func (r *PostgresRepository) HydratePersonaEmbedding(
	ctx context.Context,
	job PersonaEmbeddingJob,
) (PersonaEmbeddingCapture, error) {
	if r == nil || r.db == nil {
		return PersonaEmbeddingCapture{}, ErrDatabaseRequired
	}
	var capture PersonaEmbeddingCapture
	var providerConfig []byte
	err := r.db.QueryRowContext(ctx, `
SELECT user_id::text, persona_id::text, content, content_hash,
       persona_revision, source_watermark, visibility_epoch, generation,
       embedding_profile_id, embedding_model_id, embedding_dimensions,
       provider_record_id::text, provider_id, provider_label,
       encrypted_secret_ref, provider_config, provider_config_updated_at
FROM memory_worker_hydrate_l3_persona_embedding_job($1::uuid, $2::uuid, $3::uuid)
`, job.JobID, job.WorkerID, job.LeaseToken).Scan(
		&capture.UserID,
		&capture.PersonaID,
		&capture.Content,
		&capture.ContentHash,
		&capture.PersonaRevision,
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
		return PersonaEmbeddingCapture{}, fmt.Errorf("hydrate L3 Persona embedding job: %w", err)
	}
	if !json.Valid(providerConfig) {
		return PersonaEmbeddingCapture{}, errors.New("hydrate L3 Persona embedding: invalid provider config")
	}
	capture.ProviderConfig = append(json.RawMessage(nil), providerConfig...)
	return capture, nil
}

func (r *PostgresRepository) CompletePersonaEmbedding(
	ctx context.Context,
	job PersonaEmbeddingJob,
	vector []float32,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	if !validMemoryEmbeddingVector(vector, job.EmbeddingDimensions) {
		return errors.New("complete L3 Persona embedding: invalid vector")
	}
	var completed bool
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_complete_l3_persona_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4::real[]
)
`, job.JobID, job.WorkerID, job.LeaseToken, memoryEmbeddingRealArray(vector)).Scan(
		&completed,
	); err != nil {
		return fmt.Errorf("complete L3 Persona embedding job: %w", err)
	}
	if !completed {
		return errors.New("complete L3 Persona embedding returned false")
	}
	return nil
}

func (r *PostgresRepository) RetryPersonaEmbedding(
	ctx context.Context,
	job PersonaEmbeddingJob,
	errorCode string,
	availableAt time.Time,
	terminal bool,
) (string, error) {
	if r == nil || r.db == nil {
		return "", ErrDatabaseRequired
	}
	var status string
	if err := r.db.QueryRowContext(ctx, `
SELECT memory_worker_retry_l3_persona_embedding_job(
  $1::uuid, $2::uuid, $3::uuid, $4, $5, $6
)
`, job.JobID, job.WorkerID, job.LeaseToken, errorCode, availableAt, terminal).Scan(
		&status,
	); err != nil {
		return "", fmt.Errorf("retry L3 Persona embedding job: %w", err)
	}
	return status, nil
}

var _ PersonaRepository = (*PostgresRepository)(nil)
