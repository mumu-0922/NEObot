-- PR4 rollback is schema-only. Once provenance, revisions, tombstones, purge,
-- or a non-default epoch has been used, preserve the authority and roll back
-- runtime flags/processes instead of dropping deletion history.

DO $guard$
BEGIN
  IF EXISTS (
      SELECT 1 FROM user_memory_revisions
    ) OR EXISTS (
      SELECT 1 FROM user_memory_tombstones
    ) OR EXISTS (
      SELECT 1 FROM user_memory_deletion_manifests
    ) OR EXISTS (
      SELECT 1 FROM user_memory_evidence
    ) OR EXISTS (
      SELECT 1 FROM memory_jobs WHERE stage = 'purge'
    ) OR EXISTS (
      SELECT 1 FROM memory_outbox WHERE event_type = 'memory.deleted'
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_PROVENANCE_ROLLBACK_REQUIRES_EMPTY_HISTORY';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM user_memory_state
    WHERE visibility_epoch <> 1
      OR active_projection_generation <> 1
      OR active_retrieval_profile_id IS NOT NULL
      OR active_l2_generation <> 1
      OR active_l3_generation <> 1
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_PROVENANCE_ROLLBACK_REQUIRES_DEFAULT_STATE';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM user_memories
    WHERE source = 'ai'
      OR revision <> 1
      OR content = ''
      OR normalized_content = ''
      OR visibility_epoch <> 1
      OR authority_kind <> CASE source
        WHEN 'manual' THEN 'manual'
        ELSE 'auto'
      END
      OR extraction_profile_id IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_PROVENANCE_ROLLBACK_REQUIRES_V1_COMPATIBLE_MEMORY';
  END IF;
END
$guard$;

REVOKE ALL ON FUNCTION memory_worker_purge_memory(UUID, UUID, UUID)
FROM memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_upsert_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN
), memory_update_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN
), memory_delete_global(
  UUID, UUID, UUID, UUID, UUID, UUID
) FROM go_api_runtime;

DROP FUNCTION memory_worker_purge_memory(UUID, UUID, UUID);
DROP FUNCTION memory_delete_global(UUID, UUID, UUID, UUID, UUID, UUID);
DROP FUNCTION memory_update_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN
);
DROP FUNCTION memory_upsert_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN
);

DROP INDEX idx_memory_jobs_purge_target;
ALTER TABLE memory_jobs
  DROP CONSTRAINT memory_jobs_stage_shape_check,
  DROP CONSTRAINT memory_jobs_attempts_bounded,
  DROP CONSTRAINT memory_jobs_target_tombstone_owner_fk,
  DROP CONSTRAINT memory_jobs_target_memory_owner_fk,
  DROP COLUMN target_tombstone_id,
  DROP COLUMN target_memory_id,
  ALTER COLUMN source_conversation_id SET NOT NULL,
  ALTER COLUMN source_message_id SET NOT NULL,
  ALTER COLUMN assistant_message_id SET NOT NULL,
  ALTER COLUMN source_hash SET NOT NULL,
  ALTER COLUMN provider_source SET NOT NULL,
  ALTER COLUMN provider_id SET NOT NULL,
  ALTER COLUMN model_id SET NOT NULL,
  ALTER COLUMN processing_profile SET NOT NULL,
  ADD CONSTRAINT memory_jobs_attempts_bounded CHECK (
    max_attempts BETWEEN 1 AND 32
    AND attempt_count BETWEEN 0 AND max_attempts
  );

ALTER TABLE memory_outbox
  DROP CONSTRAINT memory_outbox_payload_id_only,
  DROP CONSTRAINT memory_outbox_event_type_allowed,
  DROP CONSTRAINT memory_outbox_attempts_bounded,
  ADD CONSTRAINT memory_outbox_event_type_allowed
    CHECK (event_type = 'turn.completed'),
  ADD CONSTRAINT memory_outbox_attempts_bounded CHECK (
    max_attempts BETWEEN 1 AND 32
    AND attempt_count BETWEEN 0 AND max_attempts
  ),
  ADD CONSTRAINT memory_outbox_payload_id_only CHECK (
    payload ? 'schemaMajor'
    AND payload ? 'conversationId'
    AND payload ? 'userMessageId'
    AND payload ? 'assistantMessageId'
    AND payload ? 'sourceHash'
    AND payload ? 'memoryScopeGeneration'
    AND payload ? 'visibilityEpoch'
    AND payload ? 'providerProfile'
    AND (payload - ARRAY[
      'schemaMajor',
      'conversationId',
      'userMessageId',
      'assistantMessageId',
      'sourceHash',
      'projectId',
      'memoryScopeGeneration',
      'projectScopeGeneration',
      'visibilityEpoch',
      'providerProfile'
    ]::TEXT[]) = '{}'::JSONB
    AND jsonb_typeof(payload->'providerProfile') = 'object'
    AND (payload->'providerProfile') ? 'providerSource'
    AND (payload->'providerProfile') ? 'providerId'
    AND (payload->'providerProfile') ? 'modelId'
    AND (payload->'providerProfile') ? 'profileHash'
    AND ((payload->'providerProfile') - ARRAY[
      'providerSource',
      'providerId',
      'modelId',
      'profileHash',
      'providerConfigId'
    ]::TEXT[]) = '{}'::JSONB
  );

DROP TABLE user_memory_deletion_manifests;
DROP TABLE user_memory_tombstones;
DROP TABLE user_memory_revisions;
DROP FUNCTION user_memory_revision_append_only_guard();
DROP TABLE user_memory_evidence;

ALTER TABLE user_memories
  DROP CONSTRAINT user_memories_content_bounded,
  ADD CONSTRAINT user_memories_content_bounded CHECK (
    length(trim(content)) > 0 AND char_length(content) <= 2000
  ),
  ADD CONSTRAINT user_memories_normalized_content_bounded CHECK (
    length(trim(normalized_content)) > 0
    AND char_length(normalized_content) <= 2000
  ),
  DROP CONSTRAINT user_memories_extraction_profile_check,
  DROP CONSTRAINT user_memories_authority_kind_allowed,
  DROP CONSTRAINT user_memories_content_hash_check,
  DROP CONSTRAINT user_memories_visibility_epoch_positive,
  DROP CONSTRAINT user_memories_revision_positive,
  DROP COLUMN extraction_profile_id,
  DROP COLUMN authority_kind,
  DROP COLUMN content_hash,
  DROP COLUMN visibility_epoch,
  DROP COLUMN revision,
  DROP CONSTRAINT user_memories_id_user_unique;

ALTER TABLE messages
  DROP CONSTRAINT messages_id_user_unique;

DROP TABLE user_memory_state;

GRANT DELETE ON user_memories TO go_api_runtime;


CREATE OR REPLACE FUNCTION memory_append_turn_completed_event(
  p_event_id UUID,
  p_job_id UUID,
  p_user_id UUID,
  p_conversation_id UUID,
  p_user_message_id UUID,
  p_assistant_message_id UUID,
  p_provider_source TEXT,
  p_provider_id TEXT,
  p_model_id TEXT,
  p_event_schema_major SMALLINT
) RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_conversation conversations%ROWTYPE;
  v_source messages%ROWTYPE;
  v_assistant messages%ROWTYPE;
  v_project_scope_generation BIGINT;
  v_source_hash TEXT;
  v_task_model TEXT;
  v_provider_source TEXT := lower(trim(p_provider_source));
  v_provider_id TEXT := trim(p_provider_id);
  v_model_id TEXT := trim(p_model_id);
  v_provider provider_configs%ROWTYPE;
  v_provider_found BOOLEAN := false;
  v_profile_hash TEXT;
  v_payload JSONB;
  v_existing_payload JSONB;
  v_existing_event_id UUID;
  v_job_event_id UUID;
BEGIN
  IF p_event_schema_major NOT IN (1, 2) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_EVENT_SCHEMA_UNSUPPORTED';
  END IF;
  IF v_provider_source NOT IN ('server-default', 'server-stored', 'request', 'legacy') THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_PROVIDER_SOURCE_INVALID';
  END IF;
  IF v_provider_id = '' OR v_model_id = '' THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_PROVIDER_PROFILE_INVALID';
  END IF;

  SELECT * INTO v_conversation
  FROM conversations
  WHERE id = p_conversation_id
    AND user_id = p_user_id
    AND deleted_at IS NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_CONVERSATION_INVALID';
  END IF;

  SELECT * INTO v_source
  FROM messages
  WHERE id = p_user_message_id
    AND conversation_id = p_conversation_id
    AND user_id = p_user_id
    AND role = 'user'
    AND status = 'completed'
    AND deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_SOURCE_INVALID';
  END IF;

  SELECT * INTO v_assistant
  FROM messages
  WHERE id = p_assistant_message_id
    AND conversation_id = p_conversation_id
    AND user_id = p_user_id
    AND parent_message_id = p_user_message_id
    AND role = 'assistant'
    AND status = 'completed'
    AND deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_ASSISTANT_INVALID';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM user_memory_settings settings
    LEFT JOIN projects project
      ON project.id = v_conversation.project_id
      AND project.user_id = settings.user_id
      AND project.deleted_at IS NULL
    WHERE settings.user_id = p_user_id
      AND settings.enabled
      AND CASE v_conversation.memory_learn_mode
        WHEN 'on' THEN true
        WHEN 'off' THEN false
        ELSE settings.auto_record_enabled
      END
      AND (
        v_conversation.project_id IS NULL
        OR project.lifecycle_status = 'active'
      )
  ) THEN
    RETURN NULL;
  END IF;

  IF v_conversation.project_id IS NOT NULL THEN
    SELECT scope_generation INTO v_project_scope_generation
    FROM projects
    WHERE id = v_conversation.project_id
      AND user_id = p_user_id
      AND deleted_at IS NULL
      AND lifecycle_status = 'active';
    IF NOT FOUND THEN
      RETURN NULL;
    END IF;
  END IF;

  SELECT memory INTO v_task_model
  FROM task_model_settings
  WHERE user_id = p_user_id;
  v_task_model := trim(COALESCE(v_task_model, ''));
  IF v_task_model <> '' AND position(':' IN v_task_model) > 1 THEN
    v_provider_source := 'server-stored';
    v_provider_id := split_part(v_task_model, ':', 1);
    v_model_id := substring(v_task_model FROM position(':' IN v_task_model) + 1);
    IF v_provider_id = 'SERVER_DEFAULT' THEN
      v_provider_source := 'server-default';
    END IF;
  ELSIF v_provider_source = 'server-default' THEN
    v_provider_id := 'SERVER_DEFAULT';
  END IF;

  SELECT * INTO v_provider
  FROM provider_configs
  WHERE user_id = p_user_id
    AND provider_id = v_provider_id
    AND deleted_at IS NULL
  ORDER BY updated_at DESC, created_at DESC
  LIMIT 1;
  v_provider_found := FOUND;

  v_source_hash := encode(sha256(convert_to(v_source.content, 'UTF8')), 'hex');
  v_profile_hash := encode(sha256(convert_to(
    v_provider_source || chr(31) || v_provider_id || chr(31) || v_model_id || chr(31)
    || CASE WHEN v_provider_found
      THEN v_provider.id::TEXT || chr(31)
        || extract(epoch FROM v_provider.updated_at)::TEXT
      ELSE 'missing'
    END || chr(31) || p_event_schema_major::TEXT,
    'UTF8'
  )), 'hex');

  v_payload := jsonb_strip_nulls(jsonb_build_object(
    'schemaMajor', p_event_schema_major,
    'conversationId', p_conversation_id::TEXT,
    'userMessageId', p_user_message_id::TEXT,
    'assistantMessageId', p_assistant_message_id::TEXT,
    'sourceHash', v_source_hash,
    'projectId', v_conversation.project_id::TEXT,
    'memoryScopeGeneration', v_conversation.memory_scope_generation,
    'projectScopeGeneration', v_project_scope_generation,
    'visibilityEpoch', 1,
    'providerProfile', jsonb_strip_nulls(jsonb_build_object(
      'providerSource', v_provider_source,
      'providerId', v_provider_id,
      'modelId', v_model_id,
      'profileHash', v_profile_hash,
      'providerConfigId', CASE WHEN v_provider_found THEN v_provider.id::TEXT END
    ))
  ));

  INSERT INTO memory_outbox (
    event_id,
    user_id,
    event_schema_major,
    event_type,
    aggregate_id,
    visibility_epoch,
    payload
  ) VALUES (
    p_event_id,
    p_user_id,
    p_event_schema_major,
    'turn.completed',
    p_assistant_message_id,
    1,
    v_payload
  )
  ON CONFLICT (user_id, event_type, aggregate_id) DO NOTHING;

  SELECT event_id, payload INTO v_existing_event_id, v_existing_payload
  FROM memory_outbox
  WHERE user_id = p_user_id
    AND event_type = 'turn.completed'
    AND aggregate_id = p_assistant_message_id;
  IF NOT FOUND OR v_existing_payload <> v_payload THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_EVENT_CONFLICT';
  END IF;

  INSERT INTO memory_jobs (
    job_id,
    user_id,
    event_id,
    stage,
    idempotency_key,
    source_conversation_id,
    source_message_id,
    assistant_message_id,
    source_hash,
    provider_source,
    provider_id,
    provider_record_id,
    provider_config_updated_at,
    model_id,
    processing_profile,
    scope_generation,
    project_scope_generation,
    visibility_epoch
  ) VALUES (
    p_job_id,
    p_user_id,
    v_existing_event_id,
    'extract',
    'memory:extract:v' || p_event_schema_major::TEXT || ':' || v_existing_event_id::TEXT,
    p_conversation_id,
    p_user_message_id,
    p_assistant_message_id,
    v_source_hash,
    v_provider_source,
    v_provider_id,
    CASE WHEN v_provider_found THEN v_provider.id END,
    CASE WHEN v_provider_found THEN v_provider.updated_at END,
    v_model_id,
    v_profile_hash,
    v_conversation.memory_scope_generation,
    v_project_scope_generation,
    1
  )
  ON CONFLICT (event_id, stage) DO NOTHING;

  SELECT event_id INTO v_job_event_id
  FROM memory_jobs
  WHERE event_id = v_existing_event_id AND stage = 'extract';
  IF v_job_event_id IS DISTINCT FROM v_existing_event_id THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_JOB_CONFLICT';
  END IF;

  RETURN v_existing_event_id;
END
$function$;


CREATE OR REPLACE FUNCTION memory_worker_hydrate_capture(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS TABLE (
  user_id UUID,
  user_message_content TEXT,
  provider_record_id UUID,
  provider_id TEXT,
  provider_label TEXT,
  encrypted_secret_ref TEXT,
  provider_config JSONB,
  model_id TEXT,
  processing_profile TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
#variable_conflict use_column
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_source messages%ROWTYPE;
  v_provider provider_configs%ROWTYPE;
  v_profile_hash TEXT;
BEGIN
  SELECT * INTO v_job
  FROM memory_jobs job
  WHERE job.job_id = p_job_id
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM memory_outbox outbox
    WHERE outbox.event_id = v_job.event_id
      AND outbox.user_id = v_job.user_id
      AND outbox.status = 'processing'
      AND outbox.lease_owner = p_worker_id
      AND outbox.lease_token = p_lease_token
      AND outbox.lease_expires_at > clock_timestamp()
      AND outbox.visibility_epoch = v_job.visibility_epoch
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_OUTBOX_LEASE_LOST';
  END IF;

  SELECT source.* INTO v_source
  FROM messages source
  JOIN conversations conversation
    ON conversation.id = source.conversation_id
    AND conversation.user_id = v_job.user_id
    AND conversation.deleted_at IS NULL
  JOIN messages assistant
    ON assistant.id = v_job.assistant_message_id
    AND assistant.conversation_id = conversation.id
    AND assistant.user_id = v_job.user_id
    AND assistant.parent_message_id = source.id
    AND assistant.role = 'assistant'
    AND assistant.status = 'completed'
    AND assistant.deleted_at IS NULL
  WHERE source.id = v_job.source_message_id
    AND source.conversation_id = v_job.source_conversation_id
    AND source.user_id = v_job.user_id
    AND source.role = 'user'
    AND source.status = 'completed'
    AND source.deleted_at IS NULL
    AND conversation.memory_scope_generation = v_job.scope_generation
    AND encode(sha256(convert_to(source.content, 'UTF8')), 'hex') = v_job.source_hash
    AND EXISTS (
      SELECT 1
      FROM user_memory_settings settings
      LEFT JOIN projects project
        ON project.id = conversation.project_id
        AND project.user_id = settings.user_id
        AND project.deleted_at IS NULL
      WHERE settings.user_id = v_job.user_id
        AND settings.enabled
        AND CASE conversation.memory_learn_mode
          WHEN 'on' THEN true
          WHEN 'off' THEN false
          ELSE settings.auto_record_enabled
        END
        AND (
          conversation.project_id IS NULL
          OR (
            project.lifecycle_status = 'active'
            AND project.scope_generation = v_job.project_scope_generation
          )
        )
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_SOURCE_DRIFT';
  END IF;

  IF v_job.provider_record_id IS NULL OR v_job.provider_config_updated_at IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_PROVIDER_UNAVAILABLE';
  END IF;

  SELECT * INTO v_provider
  FROM provider_configs provider
  WHERE provider.id = v_job.provider_record_id
    AND provider.user_id = v_job.user_id
    AND provider.provider_id = v_job.provider_id
    AND provider.updated_at = v_job.provider_config_updated_at
    AND provider.deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_PROFILE_DRIFT';
  END IF;
  IF octet_length(v_provider.config::TEXT) > 65536
    OR octet_length(COALESCE(v_provider.encrypted_secret_ref, '')) > 98304
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '54000',
      MESSAGE = 'MEMORY_PROVIDER_PROFILE_TOO_LARGE';
  END IF;

  v_profile_hash := encode(sha256(convert_to(
    v_job.provider_source || chr(31) || v_job.provider_id || chr(31)
    || v_job.model_id || chr(31) || v_provider.id::TEXT || chr(31)
    || extract(epoch FROM v_provider.updated_at)::TEXT || chr(31)
    || (SELECT event_schema_major::TEXT FROM memory_outbox WHERE event_id = v_job.event_id),
    'UTF8'
  )), 'hex');
  IF v_profile_hash <> v_job.processing_profile THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_PROFILE_DRIFT';
  END IF;

  RETURN QUERY SELECT
    v_job.user_id,
    left(v_source.content, 12000),
    v_provider.id,
    v_provider.provider_id,
    v_provider.label,
    v_provider.encrypted_secret_ref,
    v_provider.config,
    v_job.model_id,
    v_job.processing_profile;
END
$function$;


CREATE OR REPLACE FUNCTION memory_worker_apply_capture_candidate(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_memory_id UUID,
  p_memory_type TEXT,
  p_content TEXT,
  p_normalized_content TEXT,
  p_importance SMALLINT,
  p_tags TEXT[]
) RETURNS TABLE (
  id UUID,
  user_id UUID,
  memory_type TEXT,
  content TEXT,
  normalized_content TEXT,
  importance SMALLINT,
  tags_json TEXT,
  source TEXT,
  source_conversation_id UUID,
  source_message_id UUID,
  enabled BOOLEAN,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
#variable_conflict use_column
DECLARE
  v_job memory_jobs%ROWTYPE;
BEGIN
  PERFORM 1
  FROM memory_worker_hydrate_capture(p_job_id, p_worker_id, p_lease_token);

  SELECT * INTO v_job
  FROM memory_jobs
  WHERE job_id = p_job_id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  IF p_memory_type NOT IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    )
    OR length(trim(p_content)) = 0
    OR char_length(p_content) > 2000
    OR length(trim(p_normalized_content)) = 0
    OR char_length(p_normalized_content) > 2000
    OR p_importance NOT BETWEEN 1 AND 5
    OR cardinality(p_tags) > 12
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_CANDIDATE_INVALID';
  END IF;

  RETURN QUERY
  INSERT INTO user_memories (
    id,
    user_id,
    memory_type,
    content,
    normalized_content,
    importance,
    tags,
    source,
    source_conversation_id,
    source_message_id,
    enabled,
    scope_type
  ) VALUES (
    p_memory_id,
    v_job.user_id,
    p_memory_type,
    p_content,
    p_normalized_content,
    p_importance,
    p_tags,
    'ai',
    v_job.source_conversation_id,
    v_job.source_message_id,
    true,
    'global'
  )
  ON CONFLICT (user_id, normalized_content)
  WHERE deleted_at IS NULL AND scope_type = 'global' DO UPDATE SET
    memory_type = EXCLUDED.memory_type,
    content = EXCLUDED.content,
    importance = GREATEST(user_memories.importance, EXCLUDED.importance),
    tags = EXCLUDED.tags,
    source = EXCLUDED.source,
    source_conversation_id = EXCLUDED.source_conversation_id,
    source_message_id = EXCLUDED.source_message_id,
    enabled = true,
    updated_at = now()
  RETURNING
    user_memories.id,
    user_memories.user_id,
    user_memories.memory_type,
    user_memories.content,
    user_memories.normalized_content,
    user_memories.importance,
    to_json(user_memories.tags)::TEXT,
    user_memories.source,
    user_memories.source_conversation_id,
    user_memories.source_message_id,
    user_memories.enabled,
    user_memories.last_used_at,
    user_memories.created_at,
    user_memories.updated_at,
    user_memories.deleted_at;
END
$function$;

DO $harden_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_append_turn_completed_event(uuid,uuid,uuid,uuid,uuid,uuid,text,text,text,smallint)',
    'memory_worker_hydrate_capture(uuid,uuid,uuid)',
    'memory_worker_apply_capture_candidate(uuid,uuid,uuid,uuid,text,text,text,smallint,text[])'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,
      function_identity,
      schema_name
    );
  END LOOP;
END
$harden_functions$;
