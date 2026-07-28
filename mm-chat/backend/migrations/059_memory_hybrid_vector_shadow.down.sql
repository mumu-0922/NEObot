DO $memory_hybrid_rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1 FROM user_memory_state
    WHERE active_retrieval_profile_id IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_HYBRID_ROLLBACK_REQUIRES_V1_READER';
  END IF;
  IF EXISTS (SELECT 1 FROM message_memory_hybrid_shadow_observations) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_HYBRID_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS';
  END IF;
END
$memory_hybrid_rollback_guard$;

DROP TRIGGER user_memories_vector_projection_invalidate ON user_memories;
DROP TRIGGER user_memory_search_projection_embedding_queue
  ON user_memory_search_projections;

DROP FUNCTION memory_record_hybrid_shadow(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, BOOLEAN, INTEGER
);
DROP FUNCTION memory_prepare_hybrid_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, REAL[], TEXT
);
DROP FUNCTION memory_worker_retry_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
);
DROP FUNCTION memory_worker_complete_embedding_job(UUID, UUID, UUID, REAL[]);
DROP FUNCTION memory_worker_hydrate_embedding_job(UUID, UUID, UUID);
DROP FUNCTION memory_worker_claim_embedding_job(UUID, UUID, INTEGER);
DROP FUNCTION memory_invalidate_vector_projection();
DROP FUNCTION memory_queue_embedding_projection();

DROP TABLE message_memory_hybrid_shadow_results;
DROP TABLE message_memory_hybrid_shadow_observations;
DROP TABLE user_memory_embedding_jobs;

DROP INDEX idx_user_memory_search_projection_vector;
ALTER TABLE user_memory_search_projections
  DROP CONSTRAINT user_memory_search_projection_embedding_shape_check,
  DROP CONSTRAINT user_memory_search_projection_embedding_error_check,
  DROP CONSTRAINT user_memory_search_projection_embedding_status_check,
  DROP CONSTRAINT user_memory_search_projection_embedding_model_check,
  DROP CONSTRAINT user_memory_search_projection_embedding_profile_check,
  DROP COLUMN embedding_updated_at,
  DROP COLUMN embedding_error_code,
  DROP COLUMN embedding_vector,
  DROP COLUMN embedding_status,
  DROP COLUMN embedding_dimensions,
  DROP COLUMN embedding_model_id,
  DROP COLUMN embedding_profile_id;
