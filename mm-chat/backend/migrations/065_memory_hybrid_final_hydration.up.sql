DO $migration$
BEGIN
  IF to_regprocedure(
    'memory_record_hybrid_shadow(uuid,uuid,uuid,text,text,jsonb,jsonb,integer,boolean,integer)'
  ) IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_HYBRID_FINAL_HYDRATION_REQUIRES_PR8';
  END IF;
END
$migration$;

CREATE FUNCTION memory_hydrate_hybrid_final(
  p_observation_id UUID,
  p_user_id UUID,
  p_assistant_message_id UUID
) RETURNS TABLE (
  ordinal INTEGER,
  memory_id UUID,
  memory_revision BIGINT,
  scope_type TEXT,
  memory_type TEXT,
  content TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_observation message_memory_hybrid_shadow_observations%ROWTYPE;
  v_conversation conversations%ROWTYPE;
  v_expected INTEGER;
  v_returned INTEGER;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_observation_id IS NULL OR p_user_id IS NULL
    OR p_assistant_message_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_HYBRID_FINAL_HYDRATION_ARGUMENT_INVALID';
  END IF;

  SELECT observation.* INTO v_observation
  FROM message_memory_hybrid_shadow_observations observation
  WHERE observation.id = p_observation_id
    AND observation.user_id = p_user_id
    AND observation.assistant_message_id = p_assistant_message_id
    AND observation.retrieval_profile_id = 'memory_hybrid_bge_m3_rrf60_v1'
    AND observation.embedding_profile_id = 'siliconflow_bge_m3_v1'
    AND observation.status = 'completed'
    AND observation.result_code = 'OK'
    AND observation.rerank_status = 'applied'
    AND observation.final_count BETWEEN 1 AND 5
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_HYBRID_FINAL_HYDRATION_OBSERVATION_INVALID';
  END IF;

  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = v_observation.conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL
    AND conversation.memory_use_mode <> 'off';
  IF NOT FOUND OR NOT EXISTS (
    SELECT 1
    FROM messages assistant
    JOIN messages source
      ON source.id = assistant.parent_message_id
     AND source.conversation_id = assistant.conversation_id
     AND source.user_id = assistant.user_id
     AND source.role = 'user'
     AND source.status = 'completed'
     AND source.deleted_at IS NULL
     AND encode(sha256(convert_to(source.content, 'UTF8')), 'hex') =
       v_observation.query_sha256
    JOIN user_memory_state state
      ON state.user_id = assistant.user_id
     AND state.visibility_epoch > 0
     AND state.active_projection_generation = v_observation.projection_generation
    JOIN user_memory_settings settings
      ON settings.user_id = assistant.user_id
     AND settings.enabled
     AND settings.search_enabled
    WHERE assistant.id = p_assistant_message_id
      AND assistant.conversation_id = v_observation.conversation_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.status IN ('pending', 'streaming')
      AND assistant.deleted_at IS NULL
  ) OR (
    v_conversation.project_id IS NOT NULL AND NOT EXISTS (
      SELECT 1
      FROM projects project
      WHERE project.id = v_conversation.project_id
        AND project.user_id = p_user_id
        AND project.lifecycle_status = 'active'
    )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_HYBRID_FINAL_HYDRATION_SOURCE_STALE';
  END IF;

  SELECT count(*)::INTEGER INTO v_expected
  FROM message_memory_hybrid_shadow_results result
  WHERE result.observation_id = p_observation_id
    AND result.user_id = p_user_id
    AND result.lane = 'final';
  IF v_expected <> v_observation.final_count OR v_expected NOT BETWEEN 1 AND 5 THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_HYBRID_FINAL_HYDRATION_RESULT_INVALID';
  END IF;

  RETURN QUERY
  SELECT result.ordinal::INTEGER, memory.id, memory.revision,
    memory.scope_type, memory.memory_type, memory.content
  FROM message_memory_hybrid_shadow_results result
  JOIN user_memory_search_projections projection
    ON projection.memory_id = result.memory_id
   AND projection.user_id = result.user_id
   AND projection.memory_revision = result.memory_revision
   AND projection.scope_type = result.scope_type
   AND projection.projection_generation = v_observation.projection_generation
   AND projection.retrieval_profile_id = 'memory_lexical_cjk_bm25_v1'
   AND projection.lexical_status = 'ready'
   AND projection.embedding_profile_id = 'siliconflow_bge_m3_v1'
   AND projection.embedding_model_id = 'Pro/BAAI/bge-m3'
   AND projection.embedding_dimensions = 1024
   AND projection.embedding_status = 'ready'
   AND projection.embedding_vector IS NOT NULL
  JOIN user_memories memory
    ON memory.id = projection.memory_id
   AND memory.user_id = projection.user_id
   AND memory.revision = projection.memory_revision
   AND memory.content_hash = projection.content_hash
   AND memory.visibility_epoch = projection.visibility_epoch
   AND memory.scope_type = projection.scope_type
   AND memory.scope_generation = projection.scope_generation
   AND memory.sensitivity = projection.sensitivity
  JOIN user_memory_state state
    ON state.user_id = memory.user_id
   AND state.visibility_epoch = memory.visibility_epoch
   AND state.active_projection_generation = projection.projection_generation
  JOIN user_memory_settings settings
    ON settings.user_id = memory.user_id
   AND settings.enabled
   AND settings.search_enabled
  LEFT JOIN projects scoped_project
    ON projection.scope_type = 'project'
   AND scoped_project.id = projection.project_id
   AND scoped_project.user_id = projection.user_id
   AND scoped_project.lifecycle_status = 'active'
   AND scoped_project.scope_generation = projection.scope_generation
  LEFT JOIN conversations scoped_conversation
    ON projection.scope_type = 'conversation'
   AND scoped_conversation.id = projection.scope_conversation_id
   AND scoped_conversation.user_id = projection.user_id
   AND scoped_conversation.deleted_at IS NULL
   AND scoped_conversation.memory_scope_generation = projection.scope_generation
  WHERE result.observation_id = p_observation_id
    AND result.user_id = p_user_id
    AND result.lane = 'final'
    AND memory.deleted_at IS NULL
    AND memory.enabled
    AND memory.lifecycle_status = 'active'
    AND (memory.valid_from IS NULL OR memory.valid_from <= v_now)
    AND (memory.valid_to IS NULL OR v_now < memory.valid_to)
    AND (memory.expires_at IS NULL OR v_now < memory.expires_at)
    AND (memory.sensitivity = 'normal' OR settings.sensitive_memory_enabled)
    AND (
      (projection.scope_type = 'global' AND projection.scope_generation = 1)
      OR (projection.scope_type = 'project'
        AND v_conversation.project_id IS NOT NULL
        AND projection.project_id = v_conversation.project_id
        AND scoped_project.id IS NOT NULL)
      OR (projection.scope_type = 'conversation'
        AND projection.scope_conversation_id = v_conversation.id
        AND scoped_conversation.id IS NOT NULL)
    )
  ORDER BY result.ordinal;
  GET DIAGNOSTICS v_returned = ROW_COUNT;
  IF v_returned <> v_expected THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_HYBRID_FINAL_HYDRATION_AUTHORITY_STALE';
  END IF;
END
$function$;

ALTER FUNCTION memory_hydrate_hybrid_final(UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;

REVOKE ALL ON FUNCTION memory_hydrate_hybrid_final(UUID, UUID, UUID)
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION memory_hydrate_hybrid_final(UUID, UUID, UUID)
  TO go_api_runtime;
