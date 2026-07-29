DO $memory_l3_persona_rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1 FROM memory_l3_persona_profiles
    WHERE profile_id <> 'memory_l3_persona_v1'
      OR synthesis_profile_id <> 'memory_l3_persona_synthesis_v1'
      OR retrieval_profile_id <> 'memory_l3_persona_hybrid_bge_m3_rrf60_v1'
      OR lifecycle_status <> 'shadow'
      OR benchmark_report_sha256 IS NOT NULL
      OR canary_report_sha256 IS NOT NULL
      OR activated_at IS NOT NULL
      OR rolled_back_at IS NOT NULL
  ) OR NOT EXISTS (
    SELECT 1 FROM memory_l3_persona_profiles
    WHERE profile_id = 'memory_l3_persona_v1'
      AND lifecycle_status = 'shadow'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_ROLLBACK_REQUIRES_SHADOW_PROFILE';
  END IF;
  IF EXISTS (SELECT 1 FROM memory_l3_persona_promotion_events) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_ROLLBACK_REQUIRES_NO_PROMOTION_HISTORY';
  END IF;
  IF EXISTS (SELECT 1 FROM message_memory_l3_persona_observations) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS';
  END IF;
  IF EXISTS (SELECT 1 FROM user_memory_persona_versions)
    OR EXISTS (SELECT 1 FROM user_memory_persona_members)
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L3_PERSONA_ROLLBACK_REQUIRES_EMPTY_DERIVED_STATE';
  END IF;
END
$memory_l3_persona_rollback_guard$;

DROP TRIGGER memory_l3_persona_promotion_events_append_only
  ON memory_l3_persona_promotion_events;
DROP TRIGGER user_memory_persona_search_embedding_queue
  ON user_memory_persona_search_projections;
DROP TRIGGER user_memory_state_l3_persona_update ON user_memory_state;
DROP TRIGGER user_memory_settings_l3_persona_update ON user_memory_settings;
DROP TRIGGER user_memories_l3_persona_delete ON user_memories;
DROP TRIGGER user_memories_l3_persona_update ON user_memories;
DROP TRIGGER user_memories_l3_persona_insert ON user_memories;

DROP FUNCTION memory_l3_persona_promotion_append_only_guard();
DROP FUNCTION memory_operator_rollback_l3_persona(UUID, TEXT);
DROP FUNCTION memory_operator_promote_l3_persona(UUID, JSONB, JSONB);
DROP FUNCTION memory_governance_rebuild_l3_personas(UUID);
DROP FUNCTION memory_governance_rebuild_l3_persona(UUID, UUID);
DROP FUNCTION memory_governance_set_l3_persona_enabled(UUID, UUID, BIGINT, BOOLEAN);
DROP FUNCTION memory_governance_l3_persona_detail(UUID, UUID);
DROP FUNCTION memory_governance_l3_persona_snapshot(UUID);
DROP FUNCTION memory_governance_l3_persona_json(UUID, UUID);
DROP FUNCTION memory_record_l3_persona_search(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, INTEGER
);
DROP FUNCTION memory_prepare_l3_persona_search(
  UUID, UUID, UUID, UUID, TEXT, TEXT, REAL[], TEXT, BOOLEAN
);
DROP FUNCTION memory_l3_persona_reader_authority(UUID, UUID, UUID, BOOLEAN);
DROP FUNCTION memory_l3_persona_version_current(UUID, UUID, BIGINT, BOOLEAN);
DROP FUNCTION memory_worker_retry_l3_persona_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
);
DROP FUNCTION memory_worker_complete_l3_persona_embedding_job(
  UUID, UUID, UUID, REAL[]
);
DROP FUNCTION memory_worker_hydrate_l3_persona_embedding_job(UUID, UUID, UUID);
DROP FUNCTION memory_worker_claim_l3_persona_embedding_job(UUID, UUID, INTEGER);
DROP FUNCTION memory_worker_retry_l3_persona_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
);
DROP FUNCTION memory_worker_complete_l3_persona_purge(UUID, UUID, UUID);
DROP FUNCTION memory_worker_complete_l3_persona_refresh(UUID, UUID, UUID, JSONB);
DROP FUNCTION memory_worker_hydrate_l3_persona_refresh(UUID, UUID, UUID);
DROP FUNCTION memory_worker_claim_l3_persona_job(UUID, UUID, INTEGER, BOOLEAN);
DROP FUNCTION memory_queue_l3_persona_embedding();
DROP FUNCTION memory_l3_persona_state_changed();
DROP FUNCTION memory_l3_persona_settings_changed();
DROP FUNCTION memory_l3_persona_memory_changed();
DROP FUNCTION memory_l3_persona_invalidate_user(UUID);
DROP FUNCTION memory_l3_persona_invalidate_user_at_generation(UUID, BIGINT);
DROP FUNCTION memory_l3_persona_enqueue_user(UUID);
DROP FUNCTION memory_l3_persona_advance_generation(UUID);
DROP FUNCTION memory_l3_persona_reconcile_user(UUID);
DROP FUNCTION memory_l3_persona_source_watermark(UUID);
DROP FUNCTION memory_l3_persona_estimated_tokens(TEXT);

DROP TABLE message_memory_l3_persona_results;
DROP TABLE message_memory_l3_persona_observations;
DROP TABLE memory_l3_persona_promotion_events;
DROP TABLE user_memory_persona_embedding_jobs;
DROP TABLE user_memory_persona_jobs;
DROP TABLE user_memory_persona_search_projections;
DROP TABLE user_memory_persona_members;
DROP TABLE user_memory_persona_versions;
DROP TABLE memory_l3_persona_profiles;
