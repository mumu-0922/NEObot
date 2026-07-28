DO $memory_l2_scene_rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1 FROM memory_l2_scene_profiles
    WHERE profile_id <> 'memory_l2_scene_v1'
      OR synthesis_profile_id <> 'memory_l2_scene_synthesis_v1'
      OR retrieval_profile_id <> 'memory_l2_scene_hybrid_bge_m3_rrf60_v1'
      OR lifecycle_status <> 'shadow'
      OR benchmark_report_sha256 IS NOT NULL
      OR canary_report_sha256 IS NOT NULL
      OR activated_at IS NOT NULL
      OR rolled_back_at IS NOT NULL
  ) OR NOT EXISTS (
    SELECT 1 FROM memory_l2_scene_profiles
    WHERE profile_id = 'memory_l2_scene_v1'
      AND lifecycle_status = 'shadow'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L2_SCENE_ROLLBACK_REQUIRES_SHADOW_PROFILE';
  END IF;
  IF EXISTS (SELECT 1 FROM memory_l2_scene_promotion_events) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L2_SCENE_ROLLBACK_REQUIRES_NO_PROMOTION_HISTORY';
  END IF;
  IF EXISTS (SELECT 1 FROM message_memory_l2_scene_observations) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L2_SCENE_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS';
  END IF;
  IF EXISTS (SELECT 1 FROM user_memory_scenes)
    OR EXISTS (SELECT 1 FROM user_memory_scene_members)
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_L2_SCENE_ROLLBACK_REQUIRES_EMPTY_DERIVED_STATE';
  END IF;
END
$memory_l2_scene_rollback_guard$;

DROP TRIGGER memory_l2_scene_promotion_events_append_only
  ON memory_l2_scene_promotion_events;
DROP TRIGGER user_memory_derived_search_embedding_queue
  ON user_memory_derived_search_projections;
DROP TRIGGER user_memory_state_l2_scene_update ON user_memory_state;
DROP TRIGGER projects_l2_scene_update ON projects;
DROP TRIGGER user_memory_settings_l2_scene_update ON user_memory_settings;
DROP TRIGGER user_memories_l2_scene_update ON user_memories;
DROP TRIGGER user_memories_l2_scene_insert ON user_memories;

DROP FUNCTION memory_l2_scene_promotion_append_only_guard();
DROP FUNCTION memory_operator_rollback_l2_scene(UUID, TEXT);
DROP FUNCTION memory_operator_promote_l2_scene(UUID, JSONB, JSONB);
DROP FUNCTION memory_governance_rebuild_l2_scenes(UUID);
DROP FUNCTION memory_governance_rebuild_l2_scene(UUID, UUID);
DROP FUNCTION memory_governance_set_l2_scene_enabled(UUID, UUID, BIGINT, BOOLEAN);
DROP FUNCTION memory_governance_l2_scene_detail(UUID, UUID);
DROP FUNCTION memory_governance_l2_scene_snapshot(UUID);
DROP FUNCTION memory_governance_l2_scene_json(UUID, UUID);
DROP FUNCTION memory_record_l2_scene_search(
  UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, INTEGER, INTEGER
);
DROP FUNCTION memory_prepare_l2_scene_search(
  UUID, UUID, UUID, UUID, TEXT, TEXT, REAL[], TEXT, BOOLEAN
);
DROP FUNCTION memory_l2_scene_reader_authority(UUID, UUID, UUID, BOOLEAN);
DROP FUNCTION memory_worker_retry_l2_scene_embedding_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
);
DROP FUNCTION memory_worker_complete_l2_scene_embedding_job(
  UUID, UUID, UUID, REAL[]
);
DROP FUNCTION memory_worker_hydrate_l2_scene_embedding_job(UUID, UUID, UUID);
DROP FUNCTION memory_worker_claim_l2_scene_embedding_job(UUID, UUID, INTEGER);
DROP FUNCTION memory_worker_retry_l2_scene_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
);
DROP FUNCTION memory_worker_complete_l2_scene_purge(UUID, UUID, UUID);
DROP FUNCTION memory_worker_complete_l2_scene_refresh(UUID, UUID, UUID, JSONB);
DROP FUNCTION memory_worker_hydrate_l2_scene_refresh(UUID, UUID, UUID);
DROP FUNCTION memory_worker_claim_l2_scene_job(UUID, UUID, INTEGER, BOOLEAN);
DROP FUNCTION memory_queue_l2_scene_embedding();
DROP FUNCTION memory_l2_scene_state_changed();
DROP FUNCTION memory_l2_scene_project_changed();
DROP FUNCTION memory_l2_scene_settings_changed();
DROP FUNCTION memory_l2_scene_memory_changed();
DROP FUNCTION memory_l2_scene_invalidate_all(UUID);
DROP FUNCTION memory_l2_scene_invalidate_scope_at_generation(
  UUID, TEXT, UUID, BIGINT
);
DROP FUNCTION memory_l2_scene_enqueue_scope(UUID, TEXT, UUID);
DROP FUNCTION memory_l2_scene_advance_generation(UUID);
DROP FUNCTION memory_l2_scene_reconcile_user(UUID);
DROP FUNCTION memory_l2_scene_source_watermark(UUID, TEXT, UUID);

DROP TABLE message_memory_l2_scene_results;
DROP TABLE message_memory_l2_scene_observations;
DROP TABLE memory_l2_scene_promotion_events;
DROP TABLE user_memory_derived_embedding_jobs;
DROP TABLE user_memory_scene_jobs;
DROP TABLE user_memory_derived_search_projections;
DROP TABLE user_memory_scene_members;
DROP TABLE user_memory_scenes;
DROP TABLE memory_l2_scene_profiles;
