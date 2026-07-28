-- Memory v2 PR10 portability and restore authority. Portability is additive
-- governance only; the v1 Global Top 5 remains the prompt/Usage reader.

ALTER TABLE user_memories
  DROP CONSTRAINT user_memories_source_allowed,
  ADD CONSTRAINT user_memories_source_allowed
    CHECK (source IN ('manual', 'ai', 'direct_user', 'import'));

ALTER TABLE user_memory_revisions
  DROP CONSTRAINT user_memory_revisions_actor_allowed,
  ADD CONSTRAINT user_memory_revisions_actor_allowed
    CHECK (actor_type IN ('user', 'worker', 'operator', 'import')),
  DROP CONSTRAINT user_memory_revisions_full_snapshot_shape,
  ADD CONSTRAINT user_memory_revisions_full_snapshot_shape CHECK (
    prior_snapshot_schema_major IS NULL
    OR (
      purged_at IS NULL
      AND prior_content_snapshot IS NOT NULL
      AND prior_memory_type IS NOT NULL
      AND prior_normalized_content IS NOT NULL
      AND prior_importance BETWEEN 1 AND 5
      AND prior_tags IS NOT NULL
      AND prior_source IN ('manual', 'ai', 'direct_user', 'import')
      AND prior_enabled IS NOT NULL
      AND prior_scope_type IN ('global', 'project', 'conversation')
      AND prior_scope_generation >= 1
      AND prior_visibility_epoch >= 1
      AND prior_authority_kind IN (
        'manual', 'direct_user', 'confirmed', 'import', 'auto'
      )
      AND prior_lifecycle_status IN ('active', 'superseded', 'expired', 'rejected')
      AND prior_observed_at IS NOT NULL
      AND prior_sensitivity IN ('normal', 'sensitive')
      AND prior_temporal_basis IN (
        'none', 'source_timestamp', 'explicit_absolute',
        'relative_ambiguous', 'model_inferred'
      )
      AND (
        (prior_scope_type = 'global'
          AND prior_project_id IS NULL
          AND prior_scope_conversation_id IS NULL)
        OR (prior_scope_type = 'project'
          AND prior_project_id IS NOT NULL
          AND prior_scope_conversation_id IS NULL)
        OR (prior_scope_type = 'conversation'
          AND prior_project_id IS NULL
          AND prior_scope_conversation_id IS NOT NULL)
      )
    )
    OR (
      prior_snapshot_schema_major = 1
      AND purged_at IS NOT NULL
      AND result_code = 'ONLINE_PURGED'
      AND prior_content_snapshot IS NULL
      AND prior_memory_type IS NULL
      AND prior_normalized_content IS NULL
      AND prior_importance IS NULL
      AND prior_tags IS NULL
      AND prior_source IS NULL
      AND prior_source_conversation_id IS NULL
      AND prior_source_message_id IS NULL
      AND prior_enabled IS NULL
      AND prior_last_used_at IS NULL
      AND prior_scope_type IS NULL
      AND prior_project_id IS NULL
      AND prior_scope_conversation_id IS NULL
      AND prior_scope_generation IS NULL
      AND prior_visibility_epoch IS NULL
      AND prior_authority_kind IS NULL
      AND prior_extraction_profile_id IS NULL
      AND prior_lifecycle_status IS NULL
      AND prior_subject_key IS NULL
      AND prior_fact_key IS NULL
      AND prior_confidence IS NULL
      AND prior_observed_at IS NULL
      AND prior_valid_from IS NULL
      AND prior_valid_to IS NULL
      AND prior_expires_at IS NULL
      AND prior_superseded_by_memory_id IS NULL
      AND prior_sensitivity IS NULL
      AND prior_temporal_basis IS NULL
      AND prior_temporal_parser_version IS NULL
    )
  );

CREATE TABLE memory_import_batches (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  package_hash TEXT NOT NULL,
  manifest_hash TEXT NOT NULL,
  mappings_hash TEXT NOT NULL,
  plan_hash TEXT NOT NULL,
  authority_state_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'applying',
  project_count INTEGER NOT NULL,
  memory_count INTEGER NOT NULL,
  revision_count INTEGER NOT NULL,
  added_project_count INTEGER NOT NULL DEFAULT 0,
  added_memory_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  completed_at TIMESTAMPTZ,
  CONSTRAINT memory_import_batches_id_user_unique UNIQUE (id, user_id),
  CONSTRAINT memory_import_batches_hashes_valid CHECK (
    package_hash ~ '^[0-9a-f]{64}$'
    AND manifest_hash ~ '^[0-9a-f]{64}$'
    AND mappings_hash ~ '^[0-9a-f]{64}$'
    AND plan_hash ~ '^[0-9a-f]{64}$'
    AND authority_state_hash ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT memory_import_batches_status_allowed
    CHECK (status IN ('applying', 'completed')),
  CONSTRAINT memory_import_batches_counts_bounded CHECK (
    project_count BETWEEN 0 AND 1000
    AND memory_count BETWEEN 0 AND 50000
    AND revision_count BETWEEN 0 AND 200000
    AND added_project_count BETWEEN 0 AND project_count
    AND added_memory_count BETWEEN 0 AND memory_count
  ),
  CONSTRAINT memory_import_batches_completion_shape CHECK (
    (status = 'applying' AND completed_at IS NULL)
    OR (status = 'completed' AND completed_at IS NOT NULL
      AND completed_at >= created_at)
  )
);

CREATE INDEX idx_memory_import_batches_user_created
  ON memory_import_batches(user_id, created_at DESC, id);

CREATE TABLE memory_deletion_replay_entries (
  manifest_id UUID PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  memory_id UUID NOT NULL UNIQUE,
  tombstone_id UUID NOT NULL UNIQUE,
  content_hash TEXT NOT NULL,
  entry_hash TEXT NOT NULL,
  result_code TEXT NOT NULL,
  replayed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT memory_deletion_replay_content_hash_valid
    CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_deletion_replay_entry_hash_valid
    CHECK (entry_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_deletion_replay_result_allowed CHECK (
    result_code IN (
      'REPLAYED', 'ALREADY_APPLIED', 'NOT_FOUND', 'HASH_MISMATCH'
    )
  )
);

CREATE INDEX idx_memory_deletion_replay_entries_replayed
  ON memory_deletion_replay_entries(replayed_at, manifest_id);

CREATE FUNCTION memory_portability_authority_state(p_user_id UUID)
RETURNS TEXT
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT encode(sha256(convert_to(jsonb_build_object(
    'state', COALESCE((
      SELECT jsonb_build_object(
        'visibilityEpoch', state.visibility_epoch,
        'activeRetrievalProfileId', COALESCE(state.active_retrieval_profile_id, '')
      ) FROM user_memory_state state WHERE state.user_id = p_user_id
    ), '{}'::JSONB),
    'settings', COALESCE((
      SELECT jsonb_build_object(
        'enabled', settings.enabled,
        'searchEnabled', settings.search_enabled,
        'autoRecordEnabled', settings.auto_record_enabled,
        'sensitiveMemoryEnabled', settings.sensitive_memory_enabled,
        'l2Mode', settings.l2_mode,
        'l3Mode', settings.l3_mode,
        'updatedAt', settings.updated_at
      ) FROM user_memory_settings settings WHERE settings.user_id = p_user_id
    ), '{}'::JSONB),
    'projects', COALESCE((
      SELECT jsonb_agg(jsonb_build_array(
        project.id, project.revision, project.scope_generation,
        project.lifecycle_status, project.deleted_at
      ) ORDER BY project.id)
      FROM projects project WHERE project.user_id = p_user_id
    ), '[]'::JSONB),
    'conversations', COALESCE((
      SELECT jsonb_agg(jsonb_build_array(
        conversation.id, conversation.project_id,
        conversation.memory_scope_generation,
        conversation.memory_use_mode, conversation.memory_learn_mode,
        conversation.deleted_at
      ) ORDER BY conversation.id)
      FROM conversations conversation WHERE conversation.user_id = p_user_id
    ), '[]'::JSONB),
    'memories', COALESCE((
      SELECT jsonb_agg(jsonb_build_array(
        memory.id, memory.revision, memory.content_hash, memory.enabled,
        memory.lifecycle_status, memory.scope_type, memory.project_id,
        memory.scope_conversation_id, memory.scope_generation,
        memory.visibility_epoch, memory.deleted_at
      ) ORDER BY memory.id)
      FROM user_memories memory WHERE memory.user_id = p_user_id
    ), '[]'::JSONB)
  )::TEXT, 'UTF8')), 'hex')
$function$;

CREATE FUNCTION memory_portability_export_records(
  p_user_id UUID,
  p_include_history BOOLEAN
) RETURNS TABLE(record_kind TEXT, sort_group INTEGER, sort_key TEXT, payload JSONB)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
WITH project_refs AS MATERIALIZED (
  SELECT project.*,
    'project-' || lpad(row_number() OVER (ORDER BY project.id)::TEXT, 6, '0') AS portable_ref
  FROM projects project
  WHERE project.user_id = p_user_id AND project.deleted_at IS NULL
), memory_refs AS MATERIALIZED (
  SELECT memory.*,
    'memory-' || lpad(row_number() OVER (ORDER BY memory.id)::TEXT, 6, '0') AS portable_ref
  FROM user_memories memory
  WHERE memory.user_id = p_user_id AND memory.deleted_at IS NULL
), conversation_refs AS MATERIALIZED (
  SELECT source.id,
    'conversation-' || lpad(row_number() OVER (
      ORDER BY source.id
    )::TEXT, 6, '0') AS portable_ref
  FROM (
    SELECT memory.scope_conversation_id AS id
    FROM memory_refs memory
    WHERE memory.scope_type = 'conversation'
    UNION
    SELECT revision.prior_scope_conversation_id AS id
    FROM user_memory_revisions revision
    JOIN memory_refs memory ON memory.id = revision.memory_id
    WHERE p_include_history
      AND revision.prior_scope_type = 'conversation'
      AND revision.prior_scope_conversation_id IS NOT NULL
  ) source
), records AS (
  SELECT 'settings'::TEXT AS record_kind, 0 AS sort_group, 'settings'::TEXT AS sort_key,
    jsonb_build_object(
      'kind', 'settings',
      'settings', jsonb_build_object(
        'enabled', COALESCE(settings.enabled, false),
        'searchEnabled', COALESCE(settings.search_enabled, true),
        'autoRecordEnabled', COALESCE(settings.auto_record_enabled, false),
        'sensitiveMemoryEnabled', COALESCE(settings.sensitive_memory_enabled, false),
        'l2Mode', COALESCE(settings.l2_mode, 'inherit'),
        'l3Mode', COALESCE(settings.l3_mode, 'inherit')
      )
    ) AS payload
  FROM (SELECT 1) singleton
  LEFT JOIN user_memory_settings settings ON settings.user_id = p_user_id

  UNION ALL

  SELECT 'project', 1, project.portable_ref,
    jsonb_build_object(
      'kind', 'project', 'ref', project.portable_ref,
      'name', project.name, 'description', project.description,
      'lifecycleStatus', project.lifecycle_status
    )
  FROM project_refs project

  UNION ALL

  SELECT 'memory', 2, memory.portable_ref,
    jsonb_strip_nulls(jsonb_build_object(
      'kind', 'memory', 'ref', memory.portable_ref,
      'revision', memory.revision, 'type', memory.memory_type,
      'content', memory.content, 'contentHash', memory.content_hash,
      'importance', memory.importance, 'tags', to_jsonb(memory.tags),
      'originalAuthority', memory.authority_kind, 'enabled', memory.enabled,
      'scope', jsonb_strip_nulls(jsonb_build_object(
        'type', memory.scope_type,
        'projectRef', CASE WHEN memory.scope_type = 'project' THEN project.portable_ref END,
        'conversationRef', CASE WHEN memory.scope_type = 'conversation' THEN conversation.portable_ref END
      )),
      'lifecycleStatus', memory.lifecycle_status,
      'supersededByRef', superseded.portable_ref,
      'sensitivity', memory.sensitivity,
      'subjectKey', memory.subject_key, 'factKey', memory.fact_key,
      'confidence', COALESCE(memory.confidence, 0),
      'observedAt', to_char(memory.observed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
      'validFrom', CASE WHEN memory.valid_from IS NOT NULL THEN
        to_char(memory.valid_from AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END,
      'validTo', CASE WHEN memory.valid_to IS NOT NULL THEN
        to_char(memory.valid_to AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END,
      'expiresAt', CASE WHEN memory.expires_at IS NOT NULL THEN
        to_char(memory.expires_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END,
      'createdAt', to_char(memory.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
      'updatedAt', to_char(memory.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
    ))
  FROM memory_refs memory
  LEFT JOIN project_refs project ON project.id = memory.project_id
  LEFT JOIN conversation_refs conversation ON conversation.id = memory.scope_conversation_id
  LEFT JOIN memory_refs superseded ON superseded.id = memory.superseded_by_memory_id

  UNION ALL

  SELECT 'revision', 3,
    memory.portable_ref || ':' || lpad(revision.revision::TEXT, 20, '0'),
    jsonb_strip_nulls(jsonb_build_object(
      'kind', 'revision', 'memoryRef', memory.portable_ref,
      'revision', revision.revision, 'operation', revision.operation,
      'oldContentHash', revision.old_content_hash,
      'newContentHash', revision.new_content_hash,
      'prior', CASE WHEN revision.prior_content_snapshot IS NOT NULL THEN
        jsonb_strip_nulls(jsonb_build_object(
          'type', COALESCE(revision.prior_memory_type, memory.memory_type),
          'content', revision.prior_content_snapshot,
          'contentHash', revision.old_content_hash,
          'importance', COALESCE(revision.prior_importance, memory.importance),
          'tags', to_jsonb(COALESCE(revision.prior_tags, memory.tags)),
          'enabled', COALESCE(revision.prior_enabled, memory.enabled),
          'scope', jsonb_strip_nulls(jsonb_build_object(
            'type', COALESCE(revision.prior_scope_type, memory.scope_type),
            'projectRef', CASE WHEN COALESCE(revision.prior_scope_type, memory.scope_type) = 'project'
              THEN prior_project.portable_ref END,
            'conversationRef', CASE WHEN COALESCE(revision.prior_scope_type, memory.scope_type) = 'conversation'
              THEN prior_conversation.portable_ref END
          )),
          'lifecycleStatus', COALESCE(revision.prior_lifecycle_status, 'active'),
          'supersededByRef', prior_superseded.portable_ref,
          'sensitivity', COALESCE(revision.prior_sensitivity, memory.sensitivity),
          'subjectKey', revision.prior_subject_key,
          'factKey', revision.prior_fact_key,
          'confidence', COALESCE(revision.prior_confidence, 0),
          'observedAt', to_char(COALESCE(revision.prior_observed_at, memory.observed_at)
            AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
          'validFrom', CASE WHEN revision.prior_valid_from IS NOT NULL THEN
            to_char(revision.prior_valid_from AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END,
          'validTo', CASE WHEN revision.prior_valid_to IS NOT NULL THEN
            to_char(revision.prior_valid_to AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END,
          'expiresAt', CASE WHEN revision.prior_expires_at IS NOT NULL THEN
            to_char(revision.prior_expires_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END
        )) END,
      'purged', revision.prior_content_snapshot IS NULL,
      'createdAt', to_char(revision.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
    ))
  FROM user_memory_revisions revision
  JOIN memory_refs memory ON memory.id = revision.memory_id
  LEFT JOIN project_refs prior_project
    ON prior_project.id = COALESCE(revision.prior_project_id, memory.project_id)
  LEFT JOIN conversation_refs prior_conversation
    ON prior_conversation.id = COALESCE(revision.prior_scope_conversation_id, memory.scope_conversation_id)
  LEFT JOIN memory_refs prior_superseded
    ON prior_superseded.id = revision.prior_superseded_by_memory_id
  WHERE p_include_history
)
SELECT record_kind, sort_group, sort_key, payload
FROM records ORDER BY sort_group, sort_key
$function$;

CREATE FUNCTION memory_portability_resolve_project(p_user_id UUID, p_project_id UUID)
RETURNS JSONB
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT jsonb_build_object(
    'id', project.id::TEXT, 'name', project.name,
    'description', project.description,
    'lifecycleStatus', project.lifecycle_status,
    'revision', project.revision, 'scopeGeneration', project.scope_generation,
    'conversationCount', 0, 'memoryCount', 0,
    'createdAt', memory_governance_epoch_millis(project.created_at),
    'updatedAt', memory_governance_epoch_millis(project.updated_at)
  ) FROM projects project
  WHERE project.id = p_project_id AND project.user_id = p_user_id
    AND project.deleted_at IS NULL
$function$;

CREATE FUNCTION memory_portability_resolve_conversation(
  p_user_id UUID,
  p_conversation_id UUID
) RETURNS JSONB
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT jsonb_build_object(
    'conversationId', conversation.id::TEXT,
    'title', conversation.title,
    'projectId', COALESCE(conversation.project_id::TEXT, ''),
    'useMode', conversation.memory_use_mode,
    'learnMode', conversation.memory_learn_mode,
    'effectiveUse', false, 'effectiveLearn', false, 'learnForcedOff', false,
    'scopeGeneration', conversation.memory_scope_generation,
    'updatedAt', memory_governance_epoch_millis(conversation.updated_at)
  ) FROM conversations conversation
  WHERE conversation.id = p_conversation_id
    AND conversation.user_id = p_user_id AND conversation.deleted_at IS NULL
$function$;

CREATE FUNCTION memory_portability_resolve_memory(
  p_user_id UUID,
  p_normalized_content TEXT,
  p_subject_key TEXT,
  p_fact_key TEXT,
  p_scope_type TEXT,
  p_project_id UUID,
  p_conversation_id UUID,
  p_scope_generation BIGINT
) RETURNS JSONB
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory user_memories%ROWTYPE;
BEGIN
  IF memory_governance_scope_generation(
    p_user_id, p_scope_type, p_project_id, p_conversation_id
  ) <> p_scope_generation THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'MEMORY_IMPORT_SCOPE_STALE';
  END IF;
  SELECT memory.* INTO v_memory
  FROM user_memories memory
  WHERE memory.user_id = p_user_id AND memory.deleted_at IS NULL
    AND memory.normalized_content = p_normalized_content
    AND memory.scope_type = p_scope_type
    AND (p_scope_type <> 'project' OR memory.project_id = p_project_id)
    AND (p_scope_type <> 'conversation'
      OR memory.scope_conversation_id = p_conversation_id)
  ORDER BY memory.updated_at DESC, memory.id LIMIT 1;
  IF FOUND THEN
    RETURN jsonb_build_object(
      'result', 'NOOP', 'reasonCode', 'EXACT_DUPLICATE',
      'currentMemoryId', v_memory.id::TEXT,
      'currentRevision', v_memory.revision,
      'currentContentHash', v_memory.content_hash
    );
  END IF;
  IF nullif(trim(p_fact_key), '') IS NOT NULL THEN
    SELECT memory.* INTO v_memory
    FROM user_memories memory
    WHERE memory.user_id = p_user_id AND memory.deleted_at IS NULL
      AND memory.enabled AND memory.lifecycle_status = 'active'
      AND memory.fact_key = trim(p_fact_key)
      AND (nullif(trim(p_subject_key), '') IS NULL
        OR memory.subject_key = trim(p_subject_key))
      AND memory.scope_type = p_scope_type
      AND (p_scope_type <> 'project' OR memory.project_id = p_project_id)
      AND (p_scope_type <> 'conversation'
        OR memory.scope_conversation_id = p_conversation_id)
    ORDER BY memory.updated_at DESC, memory.id LIMIT 1;
    IF FOUND THEN
      RETURN jsonb_build_object(
        'result', 'REVIEW', 'reasonCode', 'CURRENT_FACT_CONFLICT',
        'currentMemoryId', v_memory.id::TEXT,
        'currentRevision', v_memory.revision,
        'currentContentHash', v_memory.content_hash
      );
    END IF;
  END IF;
  RETURN jsonb_build_object('result', 'ADD', 'reasonCode', 'NEW_MEMORY');
END
$function$;

CREATE FUNCTION memory_portability_completed_import(
  p_user_id UUID,
  p_import_id UUID,
  p_package_hash TEXT,
  p_manifest_hash TEXT,
  p_mappings_hash TEXT,
  p_plan_hash TEXT,
  p_authority_state_hash TEXT
) RETURNS JSONB
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_batch memory_import_batches%ROWTYPE;
BEGIN
  SELECT batch.* INTO v_batch
  FROM memory_import_batches batch
  WHERE batch.id = p_import_id;
  IF NOT FOUND THEN
    RETURN NULL;
  END IF;
  IF v_batch.user_id <> p_user_id
    OR v_batch.package_hash <> p_package_hash
    OR v_batch.manifest_hash <> p_manifest_hash
    OR v_batch.mappings_hash <> p_mappings_hash
    OR v_batch.plan_hash <> p_plan_hash
    OR v_batch.authority_state_hash <> p_authority_state_hash
  THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_IMPORT_REPLAY_CONFLICT';
  END IF;
  IF v_batch.status <> 'completed' THEN
    RETURN NULL;
  END IF;
  RETURN jsonb_build_object(
    'importId', v_batch.id::TEXT, 'status', v_batch.status,
    'addedProjects', v_batch.added_project_count,
    'addedMemories', v_batch.added_memory_count,
    'importedAt', memory_governance_epoch_millis(v_batch.completed_at)
  );
END
$function$;

CREATE FUNCTION memory_portability_begin_import(
  p_user_id UUID,
  p_import_id UUID,
  p_package_hash TEXT,
  p_manifest_hash TEXT,
  p_mappings_hash TEXT,
  p_plan_hash TEXT,
  p_authority_state_hash TEXT,
  p_project_count INTEGER,
  p_memory_count INTEGER,
  p_revision_count INTEGER
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_batch memory_import_batches%ROWTYPE;
BEGIN
  SELECT batch.* INTO v_batch FROM memory_import_batches batch
  WHERE batch.id = p_import_id FOR UPDATE;
  IF FOUND THEN
    IF v_batch.user_id <> p_user_id
      OR v_batch.package_hash <> p_package_hash
      OR v_batch.manifest_hash <> p_manifest_hash
      OR v_batch.mappings_hash <> p_mappings_hash
      OR v_batch.plan_hash <> p_plan_hash
      OR v_batch.authority_state_hash <> p_authority_state_hash
    THEN
      RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'MEMORY_IMPORT_REPLAY_CONFLICT';
    END IF;
    RETURN jsonb_build_object(
      'importId', v_batch.id::TEXT, 'status', v_batch.status,
      'addedProjects', v_batch.added_project_count,
      'addedMemories', v_batch.added_memory_count,
      'importedAt', memory_governance_epoch_millis(
        COALESCE(v_batch.completed_at, v_batch.created_at)
      )
    );
  END IF;
  IF memory_portability_authority_state(p_user_id) <> p_authority_state_hash THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'MEMORY_IMPORT_STATE_STALE';
  END IF;
  INSERT INTO memory_import_batches (
    id, user_id, package_hash, manifest_hash, mappings_hash, plan_hash,
    authority_state_hash, project_count, memory_count, revision_count
  ) VALUES (
    p_import_id, p_user_id, p_package_hash, p_manifest_hash, p_mappings_hash,
    p_plan_hash, p_authority_state_hash, p_project_count, p_memory_count,
    p_revision_count
  ) RETURNING * INTO v_batch;
  RETURN jsonb_build_object(
    'importId', v_batch.id::TEXT, 'status', v_batch.status,
    'addedProjects', 0, 'addedMemories', 0,
    'importedAt', memory_governance_epoch_millis(v_batch.created_at)
  );
END
$function$;

CREATE FUNCTION memory_portability_create_project(
  p_user_id UUID,
  p_import_id UUID,
  p_project_id UUID,
  p_name TEXT,
  p_description TEXT,
  p_lifecycle_status TEXT
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM memory_import_batches batch
    WHERE batch.id = p_import_id AND batch.user_id = p_user_id
      AND batch.status = 'applying'
  ) OR length(trim(p_name)) = 0 OR char_length(p_name) > 200
    OR char_length(p_description) > 4000
    OR memory_governance_is_secret(concat_ws(E'\n', p_name, p_description))
    OR p_lifecycle_status NOT IN ('active', 'archived')
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_PROJECT_INVALID';
  END IF;
  IF (SELECT count(*) FROM projects project
      WHERE project.user_id = p_user_id AND project.deleted_at IS NULL) >= 200 THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_PROJECT_LIMIT';
  END IF;
  INSERT INTO projects (
    id, user_id, name, description, lifecycle_status, archived_at
  ) VALUES (
    p_project_id, p_user_id, trim(p_name), p_description,
    p_lifecycle_status,
    CASE WHEN p_lifecycle_status = 'archived' THEN clock_timestamp() END
  );
END
$function$;

CREATE FUNCTION memory_portability_add_memory(
  p_user_id UUID,
  p_import_id UUID,
  p_memory_id UUID,
  p_record JSONB,
  p_normalized_content TEXT,
  p_scope_type TEXT,
  p_project_id UUID,
  p_conversation_id UUID,
  p_scope_generation BIGINT
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_epoch BIGINT;
  v_content TEXT := p_record->>'content';
  v_sensitivity TEXT := p_record->>'sensitivity';
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM memory_import_batches batch
    WHERE batch.id = p_import_id AND batch.user_id = p_user_id
      AND batch.status = 'applying'
  ) OR p_record->>'kind' <> 'memory'
    OR p_record->>'type' NOT IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    ) OR length(trim(v_content)) = 0 OR char_length(v_content) > 2000
    OR length(trim(p_normalized_content)) = 0
    OR char_length(p_normalized_content) > 2000
    OR (p_record->>'importance')::INTEGER NOT BETWEEN 1 AND 5
    OR jsonb_array_length(p_record->'tags') > 12
    OR (p_record->>'revision')::BIGINT < 1
    OR v_sensitivity NOT IN ('normal', 'sensitive')
    OR memory_governance_is_secret(concat_ws(
      E'\n', v_content, p_record->>'subjectKey', p_record->>'factKey',
      (SELECT string_agg(tag, E'\n')
       FROM jsonb_array_elements_text(p_record->'tags') AS tag)
    ))
    OR encode(sha256(convert_to(v_content, 'UTF8')), 'hex') <> p_record->>'contentHash'
    OR memory_governance_scope_generation(
      p_user_id, p_scope_type, p_project_id, p_conversation_id
    ) <> p_scope_generation
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_MEMORY_INVALID';
  END IF;
  IF EXISTS (
    SELECT 1 FROM user_memories memory
    WHERE memory.user_id = p_user_id AND memory.deleted_at IS NULL
      AND memory.normalized_content = p_normalized_content
      AND memory.scope_type = p_scope_type
      AND (p_scope_type <> 'project' OR memory.project_id = p_project_id)
      AND (p_scope_type <> 'conversation'
        OR memory.scope_conversation_id = p_conversation_id)
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'MEMORY_IMPORT_STATE_STALE';
  END IF;
  INSERT INTO user_memory_state(user_id) VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_epoch
  FROM user_memory_state state WHERE state.user_id = p_user_id FOR UPDATE;
  INSERT INTO user_memories (
    id, user_id, memory_type, content, normalized_content, importance,
    tags, source, enabled, scope_type, project_id, scope_conversation_id,
    scope_generation, revision, visibility_epoch, content_hash,
    authority_kind, lifecycle_status, subject_key, fact_key, confidence,
    observed_at, valid_from, valid_to, expires_at, sensitivity,
    temporal_basis
  ) VALUES (
    p_memory_id, p_user_id, p_record->>'type', v_content,
    p_normalized_content, (p_record->>'importance')::SMALLINT,
    ARRAY(SELECT jsonb_array_elements_text(p_record->'tags')),
    'import', COALESCE((p_record->>'enabled')::BOOLEAN, true),
    p_scope_type, p_project_id, p_conversation_id, p_scope_generation,
    (p_record->>'revision')::BIGINT, v_epoch, p_record->>'contentHash',
    'import', 'active', nullif(trim(p_record->>'subjectKey'), ''),
    nullif(trim(p_record->>'factKey'), ''),
    (p_record->>'confidence')::DOUBLE PRECISION,
    (p_record->>'observedAt')::TIMESTAMPTZ,
    nullif(p_record->>'validFrom', '')::TIMESTAMPTZ,
    nullif(p_record->>'validTo', '')::TIMESTAMPTZ,
    nullif(p_record->>'expiresAt', '')::TIMESTAMPTZ,
    CASE WHEN memory_governance_classify_sensitivity(v_content) = 'sensitive'
      THEN 'sensitive' ELSE v_sensitivity END,
    CASE WHEN p_record ? 'validFrom' OR p_record ? 'validTo'
      OR p_record ? 'expiresAt' THEN 'explicit_absolute' ELSE 'none' END
  );
END
$function$;

CREATE FUNCTION memory_portability_add_revision(
  p_user_id UUID,
  p_import_id UUID,
  p_memory_id UUID,
  p_record JSONB,
  p_scope_type TEXT,
  p_project_id UUID,
  p_conversation_id UUID,
  p_scope_generation BIGINT,
  p_prior_superseded_by_memory_id UUID
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory user_memories%ROWTYPE;
  v_prior JSONB := p_record->'prior';
  v_purged BOOLEAN := COALESCE((p_record->>'purged')::BOOLEAN, false);
  v_created TIMESTAMPTZ := (p_record->>'createdAt')::TIMESTAMPTZ;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM memory_import_batches batch
    WHERE batch.id = p_import_id AND batch.user_id = p_user_id
      AND batch.status = 'applying'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_BATCH_INVALID';
  END IF;
  SELECT memory.* INTO v_memory FROM user_memories memory
  WHERE memory.id = p_memory_id AND memory.user_id = p_user_id FOR SHARE;
  IF NOT FOUND OR (p_record->>'revision')::BIGINT > v_memory.revision
    OR (p_record->>'revision')::BIGINT < 2
    OR p_record->>'operation' NOT IN (
      'update', 'merge', 'supersede', 'delete', 'restore', 'move'
    ) OR p_record->>'oldContentHash' !~ '^[0-9a-f]{64}$'
    OR p_record->>'newContentHash' !~ '^[0-9a-f]{64}$'
    OR (v_purged = (v_prior IS NOT NULL))
    OR (v_purged AND p_prior_superseded_by_memory_id IS NOT NULL)
    OR (NOT v_purged AND
      ((v_prior->>'lifecycleStatus' = 'superseded') <>
        (p_prior_superseded_by_memory_id IS NOT NULL)))
    OR (p_prior_superseded_by_memory_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM user_memories target
      WHERE target.id = p_prior_superseded_by_memory_id
        AND target.user_id = p_user_id AND target.deleted_at IS NULL
    ))
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_REVISION_INVALID';
  END IF;
  IF NOT v_purged AND (
    v_prior->>'contentHash' <> p_record->>'oldContentHash'
    OR encode(sha256(convert_to(v_prior->>'content', 'UTF8')), 'hex')
      <> p_record->>'oldContentHash'
    OR memory_governance_is_secret(concat_ws(
      E'\n', v_prior->>'content', v_prior->>'subjectKey', v_prior->>'factKey',
      (SELECT string_agg(tag, E'\n')
       FROM jsonb_array_elements_text(v_prior->'tags') AS tag)
    ))
    OR memory_governance_scope_generation(
      p_user_id, p_scope_type, p_project_id, p_conversation_id
    ) <> p_scope_generation
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_REVISION_INVALID';
  END IF;
  INSERT INTO user_memory_revisions (
    memory_id, revision, user_id, operation, old_content_hash,
    new_content_hash, prior_content_snapshot, actor_type, result_code,
    purged_at, created_at, prior_snapshot_schema_major,
    prior_memory_type, prior_normalized_content, prior_importance,
    prior_tags, prior_source, prior_enabled, prior_scope_type,
    prior_project_id, prior_scope_conversation_id, prior_scope_generation,
    prior_visibility_epoch, prior_authority_kind, prior_lifecycle_status,
    prior_subject_key, prior_fact_key, prior_confidence, prior_observed_at,
    prior_valid_from, prior_valid_to, prior_expires_at,
    prior_superseded_by_memory_id, prior_sensitivity, prior_temporal_basis
  ) VALUES (
    p_memory_id, (p_record->>'revision')::BIGINT, p_user_id,
    p_record->>'operation', p_record->>'oldContentHash',
    p_record->>'newContentHash', CASE WHEN v_purged THEN NULL ELSE v_prior->>'content' END,
    'import', CASE WHEN v_purged THEN 'ONLINE_PURGED' END,
    CASE WHEN v_purged THEN v_created END, v_created,
    CASE WHEN v_purged THEN 1 ELSE 1 END,
    CASE WHEN v_purged THEN NULL ELSE v_prior->>'type' END,
    CASE WHEN v_purged THEN NULL ELSE lower(trim(v_prior->>'content')) END,
    CASE WHEN v_purged THEN NULL ELSE (v_prior->>'importance')::SMALLINT END,
    CASE WHEN v_purged THEN NULL ELSE
      ARRAY(SELECT jsonb_array_elements_text(v_prior->'tags')) END,
    CASE WHEN v_purged THEN NULL ELSE 'import' END,
    CASE WHEN v_purged THEN NULL ELSE (v_prior->>'enabled')::BOOLEAN END,
    CASE WHEN v_purged THEN NULL ELSE p_scope_type END,
    CASE WHEN v_purged THEN NULL ELSE p_project_id END,
    CASE WHEN v_purged THEN NULL ELSE p_conversation_id END,
    CASE WHEN v_purged THEN NULL ELSE p_scope_generation END,
    CASE WHEN v_purged THEN NULL ELSE v_memory.visibility_epoch END,
    CASE WHEN v_purged THEN NULL ELSE 'import' END,
    CASE WHEN v_purged THEN NULL ELSE v_prior->>'lifecycleStatus' END,
    CASE WHEN v_purged THEN NULL ELSE nullif(trim(v_prior->>'subjectKey'), '') END,
    CASE WHEN v_purged THEN NULL ELSE nullif(trim(v_prior->>'factKey'), '') END,
    CASE WHEN v_purged THEN NULL ELSE (v_prior->>'confidence')::DOUBLE PRECISION END,
    CASE WHEN v_purged THEN NULL ELSE (v_prior->>'observedAt')::TIMESTAMPTZ END,
    CASE WHEN v_purged THEN NULL ELSE nullif(v_prior->>'validFrom', '')::TIMESTAMPTZ END,
    CASE WHEN v_purged THEN NULL ELSE nullif(v_prior->>'validTo', '')::TIMESTAMPTZ END,
    CASE WHEN v_purged THEN NULL ELSE nullif(v_prior->>'expiresAt', '')::TIMESTAMPTZ END,
    CASE WHEN v_purged THEN NULL ELSE p_prior_superseded_by_memory_id END,
    CASE WHEN v_purged THEN NULL ELSE v_prior->>'sensitivity' END,
    CASE WHEN v_purged THEN NULL WHEN v_prior ? 'validFrom' OR v_prior ? 'validTo'
      OR v_prior ? 'expiresAt' THEN 'explicit_absolute' ELSE 'none' END
  );
END
$function$;

CREATE FUNCTION memory_portability_finalize_memory(
  p_user_id UUID,
  p_import_id UUID,
  p_memory_id UUID,
  p_lifecycle_status TEXT,
  p_superseded_by_memory_id UUID
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM memory_import_batches batch
    WHERE batch.id = p_import_id AND batch.user_id = p_user_id
      AND batch.status = 'applying'
  ) OR p_lifecycle_status NOT IN ('active', 'superseded', 'expired', 'rejected')
    OR ((p_lifecycle_status = 'superseded') <> (p_superseded_by_memory_id IS NOT NULL))
    OR (p_superseded_by_memory_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM user_memories target
      WHERE target.id = p_superseded_by_memory_id AND target.user_id = p_user_id
        AND target.deleted_at IS NULL
    ))
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_FINAL_STATE_INVALID';
  END IF;
  UPDATE user_memories memory
  SET lifecycle_status = p_lifecycle_status,
      superseded_by_memory_id = p_superseded_by_memory_id,
      enabled = CASE WHEN p_lifecycle_status = 'active' THEN memory.enabled ELSE false END,
      updated_at = GREATEST(memory.updated_at, clock_timestamp())
  WHERE memory.id = p_memory_id AND memory.user_id = p_user_id
    AND memory.source = 'import' AND memory.authority_kind = 'import';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_IMPORT_MEMORY_INVALID';
  END IF;
END
$function$;

CREATE FUNCTION memory_portability_complete_import(
  p_user_id UUID,
  p_import_id UUID,
  p_added_projects INTEGER,
  p_added_memories INTEGER
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_batch memory_import_batches%ROWTYPE;
BEGIN
  UPDATE memory_import_batches batch
  SET status = 'completed', added_project_count = p_added_projects,
      added_memory_count = p_added_memories, completed_at = clock_timestamp()
  WHERE batch.id = p_import_id AND batch.user_id = p_user_id
    AND batch.status = 'applying'
  RETURNING * INTO v_batch;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '40001', MESSAGE = 'MEMORY_IMPORT_REPLAY_CONFLICT';
  END IF;
  RETURN jsonb_build_object(
    'importId', v_batch.id::TEXT, 'status', v_batch.status,
    'addedProjects', v_batch.added_project_count,
    'addedMemories', v_batch.added_memory_count,
    'importedAt', memory_governance_epoch_millis(v_batch.completed_at)
  );
END
$function$;

CREATE FUNCTION memory_portability_export_deletions()
RETURNS TABLE(sort_key TEXT, payload JSONB)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT
    to_char(manifest.deleted_at AT TIME ZONE 'UTC',
      'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || ':' || manifest.manifest_id::TEXT,
    jsonb_strip_nulls(jsonb_build_object(
      'kind', 'deletion',
      'manifestId', manifest.manifest_id::TEXT,
      'eventId', manifest.event_id::TEXT,
      'userId', manifest.user_id::TEXT,
      'memoryId', manifest.memory_id::TEXT,
      'tombstoneId', manifest.tombstone_id::TEXT,
      'contentHash', manifest.content_hash,
      'scopeGeneration', manifest.scope_generation,
      'visibilityEpoch', manifest.visibility_epoch,
      'deletedAt', to_char(manifest.deleted_at AT TIME ZONE 'UTC',
        'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
      'purgedAt', CASE WHEN manifest.purged_at IS NOT NULL THEN
        to_char(manifest.purged_at AT TIME ZONE 'UTC',
          'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END,
      'resultCode', manifest.result_code
    ))
  FROM user_memory_deletion_manifests manifest
  ORDER BY manifest.deleted_at, manifest.manifest_id
$function$;

CREATE FUNCTION memory_portability_replay_deletion(p_entry JSONB)
RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_manifest_id UUID;
  v_event_id UUID;
  v_user_id UUID;
  v_memory_id UUID;
  v_tombstone_id UUID;
  v_content_hash TEXT;
  v_entry_hash TEXT;
  v_deleted_at TIMESTAMPTZ;
  v_purged_at TIMESTAMPTZ;
  v_result_code TEXT;
  v_scope_generation BIGINT;
  v_visibility_epoch BIGINT;
  v_existing memory_deletion_replay_entries%ROWTYPE;
  v_memory user_memories%ROWTYPE;
  v_already_applied BOOLEAN := false;
BEGIN
  IF p_entry IS NULL OR jsonb_typeof(p_entry) <> 'object'
    OR NOT p_entry ?& ARRAY[
      'kind', 'manifestId', 'eventId', 'userId', 'memoryId',
      'tombstoneId', 'contentHash', 'scopeGeneration',
      'visibilityEpoch', 'deletedAt', 'resultCode'
    ]
    OR EXISTS (
      SELECT 1 FROM jsonb_object_keys(p_entry) AS fields(key)
      WHERE key <> ALL (ARRAY[
        'kind', 'manifestId', 'eventId', 'userId', 'memoryId',
        'tombstoneId', 'contentHash', 'scopeGeneration',
        'visibilityEpoch', 'deletedAt', 'purgedAt', 'resultCode'
      ])
    )
    OR p_entry->>'kind' <> 'deletion'
    OR p_entry->>'contentHash' !~ '^[0-9a-f]{64}$'
    OR p_entry->>'manifestId' !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    OR p_entry->>'eventId' !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    OR p_entry->>'userId' !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    OR p_entry->>'memoryId' !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    OR p_entry->>'tombstoneId' !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
    OR p_entry->>'deletedAt' !~
      '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?Z$'
    OR p_entry->>'resultCode' NOT IN ('PENDING', 'ONLINE_PURGED')
    OR (p_entry->>'resultCode' = 'PENDING' AND p_entry ? 'purgedAt')
    OR (p_entry->>'resultCode' = 'ONLINE_PURGED' AND NOT (p_entry ? 'purgedAt'))
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_DELETION_REPLAY_ENTRY_INVALID';
  END IF;

  BEGIN
    v_manifest_id := (p_entry->>'manifestId')::UUID;
    v_event_id := (p_entry->>'eventId')::UUID;
    v_user_id := (p_entry->>'userId')::UUID;
    v_memory_id := (p_entry->>'memoryId')::UUID;
    v_tombstone_id := (p_entry->>'tombstoneId')::UUID;
    v_content_hash := p_entry->>'contentHash';
    v_scope_generation := (p_entry->>'scopeGeneration')::BIGINT;
    v_visibility_epoch := (p_entry->>'visibilityEpoch')::BIGINT;
    v_deleted_at := (p_entry->>'deletedAt')::TIMESTAMPTZ;
    v_purged_at := CASE WHEN p_entry ? 'purgedAt'
      THEN (p_entry->>'purgedAt')::TIMESTAMPTZ END;
    v_result_code := p_entry->>'resultCode';
  EXCEPTION WHEN OTHERS THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_DELETION_REPLAY_ENTRY_INVALID';
  END;

  IF v_scope_generation < 1 OR v_visibility_epoch < 1
    OR (v_result_code = 'ONLINE_PURGED'
      AND (v_purged_at IS NULL OR v_purged_at < v_deleted_at))
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_DELETION_REPLAY_ENTRY_INVALID';
  END IF;

  v_entry_hash := encode(sha256(convert_to(p_entry::TEXT, 'UTF8')), 'hex');
  SELECT authority.* INTO v_existing
  FROM memory_deletion_replay_entries authority
  WHERE authority.manifest_id = v_manifest_id
    OR authority.event_id = v_event_id
    OR authority.memory_id = v_memory_id
    OR authority.tombstone_id = v_tombstone_id
  FOR UPDATE;
  IF FOUND THEN
    IF v_existing.manifest_id <> v_manifest_id
      OR v_existing.event_id <> v_event_id
      OR v_existing.user_id <> v_user_id
      OR v_existing.memory_id <> v_memory_id
      OR v_existing.tombstone_id <> v_tombstone_id
      OR v_existing.content_hash <> v_content_hash
      OR v_existing.entry_hash <> v_entry_hash
    THEN
      RAISE EXCEPTION USING ERRCODE = '40001',
        MESSAGE = 'MEMORY_DELETION_REPLAY_CONFLICT';
    END IF;
    RETURN CASE WHEN v_existing.result_code IN ('REPLAYED', 'ALREADY_APPLIED')
      THEN 'ALREADY_APPLIED' ELSE v_existing.result_code END;
  END IF;

  SELECT memory.* INTO v_memory
  FROM user_memories memory
  WHERE memory.id = v_memory_id AND memory.user_id = v_user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    INSERT INTO memory_deletion_replay_entries (
      manifest_id, event_id, user_id, memory_id, tombstone_id,
      content_hash, entry_hash, result_code
    ) VALUES (
      v_manifest_id, v_event_id, v_user_id, v_memory_id, v_tombstone_id,
      v_content_hash, v_entry_hash, 'NOT_FOUND'
    );
    RETURN 'NOT_FOUND';
  END IF;
  IF v_memory.content_hash <> v_content_hash THEN
    INSERT INTO memory_deletion_replay_entries (
      manifest_id, event_id, user_id, memory_id, tombstone_id,
      content_hash, entry_hash, result_code
    ) VALUES (
      v_manifest_id, v_event_id, v_user_id, v_memory_id, v_tombstone_id,
      v_content_hash, v_entry_hash, 'HASH_MISMATCH'
    );
    RETURN 'HASH_MISMATCH';
  END IF;

  SELECT EXISTS (
    SELECT 1 FROM user_memory_deletion_manifests manifest
    WHERE manifest.manifest_id = v_manifest_id
      AND manifest.event_id = v_event_id
      AND manifest.user_id = v_user_id
      AND manifest.memory_id = v_memory_id
      AND manifest.tombstone_id = v_tombstone_id
      AND manifest.content_hash = v_content_hash
      AND manifest.result_code = 'ONLINE_PURGED'
  ) INTO v_already_applied;

  IF EXISTS (
    SELECT 1 FROM user_memory_tombstones tombstone
    WHERE (tombstone.id = v_tombstone_id OR tombstone.memory_id = v_memory_id)
      AND NOT (
        tombstone.id = v_tombstone_id
        AND tombstone.user_id = v_user_id
        AND tombstone.memory_id = v_memory_id
        AND tombstone.content_hash = v_content_hash
      )
  ) OR EXISTS (
    SELECT 1 FROM user_memory_deletion_manifests manifest
    WHERE (manifest.manifest_id = v_manifest_id
      OR manifest.event_id = v_event_id
      OR manifest.memory_id = v_memory_id
      OR manifest.tombstone_id = v_tombstone_id)
      AND NOT (
        manifest.manifest_id = v_manifest_id
        AND manifest.event_id = v_event_id
        AND manifest.user_id = v_user_id
        AND manifest.memory_id = v_memory_id
        AND manifest.tombstone_id = v_tombstone_id
        AND manifest.content_hash = v_content_hash
        AND manifest.scope_generation = v_scope_generation
        AND manifest.visibility_epoch = v_visibility_epoch
        AND manifest.deleted_at = v_deleted_at
      )
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_DELETION_REPLAY_CONFLICT';
  END IF;

  INSERT INTO user_memory_tombstones (
    id, user_id, memory_id, content_hash, reason, created_at
  ) VALUES (
    v_tombstone_id, v_user_id, v_memory_id, v_content_hash,
    'user_delete', v_deleted_at
  ) ON CONFLICT (id) DO NOTHING;

  v_purged_at := COALESCE(v_purged_at, GREATEST(clock_timestamp(), v_deleted_at));
  INSERT INTO user_memory_deletion_manifests (
    manifest_id, event_id, user_id, memory_id, tombstone_id,
    content_hash, scope_generation, visibility_epoch,
    deleted_at, result_code, purged_at, created_at
  ) VALUES (
    v_manifest_id, v_event_id, v_user_id, v_memory_id, v_tombstone_id,
    v_content_hash, v_scope_generation, v_visibility_epoch,
    v_deleted_at, 'ONLINE_PURGED', v_purged_at,
    GREATEST(v_deleted_at, v_purged_at)
  ) ON CONFLICT (manifest_id) DO UPDATE SET
    result_code = 'ONLINE_PURGED',
    purged_at = GREATEST(
      user_memory_deletion_manifests.deleted_at, v_purged_at
    );

  DELETE FROM user_memory_evidence evidence
  WHERE evidence.memory_id = v_memory_id AND evidence.user_id = v_user_id;

  UPDATE user_memory_revisions revision
  SET prior_content_snapshot = NULL,
      prior_memory_type = NULL,
      prior_normalized_content = NULL,
      prior_importance = NULL,
      prior_tags = NULL,
      prior_source = NULL,
      prior_source_conversation_id = NULL,
      prior_source_message_id = NULL,
      prior_enabled = NULL,
      prior_last_used_at = NULL,
      prior_scope_type = NULL,
      prior_project_id = NULL,
      prior_scope_conversation_id = NULL,
      prior_scope_generation = NULL,
      prior_visibility_epoch = NULL,
      prior_authority_kind = NULL,
      prior_extraction_profile_id = NULL,
      prior_lifecycle_status = NULL,
      prior_subject_key = NULL,
      prior_fact_key = NULL,
      prior_confidence = NULL,
      prior_observed_at = NULL,
      prior_valid_from = NULL,
      prior_valid_to = NULL,
      prior_expires_at = NULL,
      prior_superseded_by_memory_id = NULL,
      prior_sensitivity = NULL,
      prior_temporal_basis = NULL,
      prior_temporal_parser_version = NULL,
      result_code = 'ONLINE_PURGED',
      purged_at = v_purged_at
  WHERE revision.memory_id = v_memory_id
    AND revision.user_id = v_user_id
    AND revision.prior_content_snapshot IS NOT NULL
    AND revision.purged_at IS NULL;

  UPDATE user_memories memory
  SET enabled = false,
      deleted_at = CASE WHEN memory.deleted_at IS NULL THEN v_deleted_at
        ELSE LEAST(memory.deleted_at, v_deleted_at) END,
      content = '',
      normalized_content = '',
      tags = '{}',
      source_conversation_id = NULL,
      source_message_id = NULL,
      extraction_profile_id = NULL,
      subject_key = NULL,
      fact_key = NULL,
      temporal_parser_version = NULL,
      updated_at = GREATEST(memory.updated_at, v_purged_at)
  WHERE memory.id = v_memory_id AND memory.user_id = v_user_id;

  INSERT INTO memory_deletion_replay_entries (
    manifest_id, event_id, user_id, memory_id, tombstone_id,
    content_hash, entry_hash, result_code
  ) VALUES (
    v_manifest_id, v_event_id, v_user_id, v_memory_id, v_tombstone_id,
    v_content_hash, v_entry_hash,
    CASE WHEN v_already_applied THEN 'ALREADY_APPLIED' ELSE 'REPLAYED' END
  );
  RETURN CASE WHEN v_already_applied THEN 'ALREADY_APPLIED' ELSE 'REPLAYED' END;
END
$function$;

CREATE FUNCTION memory_portability_rebuild_projections()
RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory_id UUID;
  v_projection_count INTEGER;
BEGIN
  DELETE FROM user_memory_search_projections;
  FOR v_memory_id IN
    SELECT memory.id FROM user_memories memory ORDER BY memory.id
  LOOP
    PERFORM memory_refresh_lexical_projection(v_memory_id);
  END LOOP;
  SELECT count(*)::INTEGER INTO v_projection_count
  FROM user_memory_search_projections;
  RETURN v_projection_count;
END
$function$;

DO $pin_memory_portability_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_portability_authority_state(uuid)',
    'memory_portability_export_records(uuid,boolean)',
    'memory_portability_resolve_project(uuid,uuid)',
    'memory_portability_resolve_conversation(uuid,uuid)',
    'memory_portability_resolve_memory(uuid,text,text,text,text,uuid,uuid,bigint)',
    'memory_portability_completed_import(uuid,uuid,text,text,text,text,text)',
    'memory_portability_begin_import(uuid,uuid,text,text,text,text,text,integer,integer,integer)',
    'memory_portability_create_project(uuid,uuid,uuid,text,text,text)',
    'memory_portability_add_memory(uuid,uuid,uuid,jsonb,text,text,uuid,uuid,bigint)',
    'memory_portability_add_revision(uuid,uuid,uuid,jsonb,text,uuid,uuid,bigint,uuid)',
    'memory_portability_finalize_memory(uuid,uuid,uuid,text,uuid)',
    'memory_portability_complete_import(uuid,uuid,integer,integer)',
    'memory_portability_export_deletions()',
    'memory_portability_replay_deletion(jsonb)',
    'memory_portability_rebuild_projections()'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name, function_identity, schema_name
    );
    EXECUTE format(
      'ALTER FUNCTION %I.%s OWNER TO memory_runtime_owner',
      schema_name, function_identity
    );
  END LOOP;
END
$pin_memory_portability_functions$;

ALTER TABLE memory_import_batches OWNER TO memory_runtime_owner;
ALTER TABLE memory_deletion_replay_entries OWNER TO memory_runtime_owner;
REVOKE ALL ON memory_import_batches, memory_deletion_replay_entries
FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION
  memory_portability_authority_state(UUID),
  memory_portability_export_records(UUID, BOOLEAN),
  memory_portability_resolve_project(UUID, UUID),
  memory_portability_resolve_conversation(UUID, UUID),
  memory_portability_resolve_memory(UUID, TEXT, TEXT, TEXT, TEXT, UUID, UUID, BIGINT),
  memory_portability_completed_import(UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT),
  memory_portability_begin_import(UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, INTEGER, INTEGER, INTEGER),
  memory_portability_create_project(UUID, UUID, UUID, TEXT, TEXT, TEXT),
  memory_portability_add_memory(UUID, UUID, UUID, JSONB, TEXT, TEXT, UUID, UUID, BIGINT),
  memory_portability_add_revision(UUID, UUID, UUID, JSONB, TEXT, UUID, UUID, BIGINT, UUID),
  memory_portability_finalize_memory(UUID, UUID, UUID, TEXT, UUID),
  memory_portability_complete_import(UUID, UUID, INTEGER, INTEGER),
  memory_portability_export_deletions(),
  memory_portability_replay_deletion(JSONB),
  memory_portability_rebuild_projections()
FROM PUBLIC;

GRANT EXECUTE ON FUNCTION
  memory_portability_authority_state(UUID),
  memory_portability_export_records(UUID, BOOLEAN),
  memory_portability_resolve_project(UUID, UUID),
  memory_portability_resolve_conversation(UUID, UUID),
  memory_portability_resolve_memory(UUID, TEXT, TEXT, TEXT, TEXT, UUID, UUID, BIGINT),
  memory_portability_completed_import(UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT),
  memory_portability_begin_import(UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, INTEGER, INTEGER, INTEGER),
  memory_portability_create_project(UUID, UUID, UUID, TEXT, TEXT, TEXT),
  memory_portability_add_memory(UUID, UUID, UUID, JSONB, TEXT, TEXT, UUID, UUID, BIGINT),
  memory_portability_add_revision(UUID, UUID, UUID, JSONB, TEXT, UUID, UUID, BIGINT, UUID),
  memory_portability_finalize_memory(UUID, UUID, UUID, TEXT, UUID),
  memory_portability_complete_import(UUID, UUID, INTEGER, INTEGER),
  memory_portability_export_deletions(),
  memory_portability_replay_deletion(JSONB),
  memory_portability_rebuild_projections()
TO go_api_runtime;

DO $owner_create_revocation$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner', current_schema()
  );
END
$owner_create_revocation$;
