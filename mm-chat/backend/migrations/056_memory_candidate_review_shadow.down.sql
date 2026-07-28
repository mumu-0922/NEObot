DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM memory_capture_candidate_batches)
    OR EXISTS (SELECT 1 FROM user_memory_review_suggestions)
    OR EXISTS (SELECT 1 FROM user_memory_review_targets)
    OR EXISTS (SELECT 1 FROM user_memory_review_evidence)
    OR EXISTS (SELECT 1 FROM memory_jobs WHERE stage = 'review_expire')
    OR EXISTS (
      SELECT 1 FROM memory_outbox
      WHERE event_type = 'turn.completed' AND max_attempts > 32
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_REVIEW_ROLLBACK_REQUIRES_EMPTY_HISTORY';
  END IF;

  IF EXISTS (
    SELECT 1 FROM user_memories memory
    WHERE memory.lifecycle_status <> 'active'
      OR memory.subject_key IS NOT NULL
      OR memory.fact_key IS NOT NULL
      OR memory.valid_from IS NOT NULL
      OR memory.valid_to IS NOT NULL
      OR memory.expires_at IS NOT NULL
      OR memory.superseded_by_memory_id IS NOT NULL
      OR memory.sensitivity <> 'normal'
      OR memory.temporal_basis <> 'none'
      OR memory.temporal_parser_version IS NOT NULL
      OR memory.observed_at <> memory.created_at
      OR (
        memory.confidence IS NOT NULL
        AND NOT (
          memory.authority_kind IN ('manual', 'direct_user', 'confirmed', 'import')
          AND memory.confidence = 1.0
        )
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_REVIEW_ROLLBACK_REQUIRES_DEFAULT_CANONICAL_METADATA';
  END IF;
END
$guard$;

REVOKE EXECUTE ON FUNCTION memory_worker_hydrate_capture_v2(UUID, UUID, UUID),
  memory_worker_propose_capture_candidates(
    UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, JSONB
  ),
  memory_worker_expire_capture_reviews(UUID, UUID, UUID)
FROM memory_worker_runtime;

DROP FUNCTION memory_worker_expire_capture_reviews(UUID, UUID, UUID);
DROP FUNCTION memory_worker_propose_capture_candidates(
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, JSONB
);
DROP FUNCTION memory_worker_hydrate_capture_v2(UUID, UUID, UUID);

DROP INDEX idx_memory_jobs_review_expire;

ALTER TABLE memory_jobs
  DROP CONSTRAINT memory_jobs_stage_allowed,
  DROP CONSTRAINT memory_jobs_attempts_bounded,
  DROP CONSTRAINT memory_jobs_stage_shape_check,
  ADD CONSTRAINT memory_jobs_stage_allowed CHECK (
    stage IN (
      'extract', 'resolve', 'embed', 'l2_refresh', 'l3_refresh',
      'purge', 'rebuild'
    )
  ),
  ADD CONSTRAINT memory_jobs_attempts_bounded CHECK (
    (
      (stage = 'purge' AND max_attempts = 128)
      OR (stage <> 'purge' AND max_attempts BETWEEN 1 AND 32)
    )
    AND attempt_count BETWEEN 0 AND max_attempts
  ),
  ADD CONSTRAINT memory_jobs_stage_shape_check CHECK (
    (
      stage = 'extract'
      AND source_conversation_id IS NOT NULL
      AND source_message_id IS NOT NULL
      AND assistant_message_id IS NOT NULL
      AND source_hash IS NOT NULL
      AND provider_source IS NOT NULL
      AND provider_id IS NOT NULL
      AND model_id IS NOT NULL
      AND processing_profile IS NOT NULL
      AND target_memory_id IS NULL
      AND target_tombstone_id IS NULL
    )
    OR (
      stage = 'purge'
      AND source_conversation_id IS NULL
      AND source_message_id IS NULL
      AND assistant_message_id IS NULL
      AND source_hash IS NULL
      AND provider_source IS NULL
      AND provider_id IS NULL
      AND provider_record_id IS NULL
      AND provider_config_updated_at IS NULL
      AND model_id IS NULL
      AND processing_profile IS NULL
      AND project_scope_generation IS NULL
      AND target_memory_id IS NOT NULL
      AND target_tombstone_id IS NOT NULL
    )
    OR (
      stage NOT IN ('extract', 'purge')
      AND source_conversation_id IS NOT NULL
      AND source_message_id IS NOT NULL
      AND assistant_message_id IS NOT NULL
      AND source_hash IS NOT NULL
      AND provider_source IS NOT NULL
      AND provider_id IS NOT NULL
      AND model_id IS NOT NULL
      AND processing_profile IS NOT NULL
      AND target_memory_id IS NULL
      AND target_tombstone_id IS NULL
    )
  );

ALTER TABLE memory_outbox
  DROP CONSTRAINT memory_outbox_attempts_bounded,
  ADD CONSTRAINT memory_outbox_attempts_bounded CHECK (
    (
      (event_type = 'turn.completed' AND max_attempts BETWEEN 1 AND 32)
      OR (event_type = 'memory.deleted' AND max_attempts = 128)
    )
    AND attempt_count BETWEEN 0 AND max_attempts
  );

DROP TABLE user_memory_review_evidence;
DROP TABLE user_memory_review_targets;
DROP TABLE user_memory_review_suggestions;
DROP TABLE memory_capture_candidate_batches;

ALTER TABLE memory_jobs
  DROP CONSTRAINT memory_jobs_job_event_user_unique;

DROP INDEX idx_user_memories_current_fact;
ALTER TABLE user_memories
  DROP CONSTRAINT user_memories_superseded_owner_fk,
  DROP CONSTRAINT user_memories_lifecycle_status_allowed,
  DROP CONSTRAINT user_memories_subject_key_bounded,
  DROP CONSTRAINT user_memories_fact_key_bounded,
  DROP CONSTRAINT user_memories_confidence_range,
  DROP CONSTRAINT user_memories_validity_order,
  DROP CONSTRAINT user_memories_expiry_order,
  DROP CONSTRAINT user_memories_supersede_shape,
  DROP CONSTRAINT user_memories_sensitivity_allowed,
  DROP CONSTRAINT user_memories_temporal_basis_allowed,
  DROP CONSTRAINT user_memories_temporal_parser_bounded,
  DROP COLUMN lifecycle_status,
  DROP COLUMN subject_key,
  DROP COLUMN fact_key,
  DROP COLUMN confidence,
  DROP COLUMN observed_at,
  DROP COLUMN valid_from,
  DROP COLUMN valid_to,
  DROP COLUMN expires_at,
  DROP COLUMN superseded_by_memory_id,
  DROP COLUMN sensitivity,
  DROP COLUMN temporal_basis,
  DROP COLUMN temporal_parser_version;

GRANT EXECUTE ON FUNCTION memory_worker_hydrate_capture(UUID, UUID, UUID),
  memory_worker_apply_capture_candidate(
    UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[]
  )
TO memory_worker_runtime;
