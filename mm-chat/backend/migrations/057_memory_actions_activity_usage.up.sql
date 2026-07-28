-- Memory v2 PR6 direct-user actions, answer Usage links, Activity polling,
-- and revision-safe undo. Model output is proposal-only: every identity,
-- scope, target, and revision is rebound inside the narrow capabilities below.

ALTER TABLE user_memories
  DROP CONSTRAINT user_memories_source_allowed,
  ADD CONSTRAINT user_memories_source_allowed
    CHECK (source IN ('manual', 'ai', 'direct_user'));

ALTER TABLE user_memory_revisions
  ADD COLUMN prior_snapshot_schema_major SMALLINT,
  ADD COLUMN prior_memory_type TEXT,
  ADD COLUMN prior_normalized_content TEXT,
  ADD COLUMN prior_importance SMALLINT,
  ADD COLUMN prior_tags TEXT[],
  ADD COLUMN prior_source TEXT,
  ADD COLUMN prior_source_conversation_id UUID,
  ADD COLUMN prior_source_message_id UUID,
  ADD COLUMN prior_enabled BOOLEAN,
  ADD COLUMN prior_last_used_at TIMESTAMPTZ,
  ADD COLUMN prior_scope_type TEXT,
  ADD COLUMN prior_project_id UUID,
  ADD COLUMN prior_scope_conversation_id UUID,
  ADD COLUMN prior_scope_generation BIGINT,
  ADD COLUMN prior_visibility_epoch BIGINT,
  ADD COLUMN prior_authority_kind TEXT,
  ADD COLUMN prior_extraction_profile_id TEXT,
  ADD COLUMN prior_lifecycle_status TEXT,
  ADD COLUMN prior_subject_key TEXT,
  ADD COLUMN prior_fact_key TEXT,
  ADD COLUMN prior_confidence DOUBLE PRECISION,
  ADD COLUMN prior_observed_at TIMESTAMPTZ,
  ADD COLUMN prior_valid_from TIMESTAMPTZ,
  ADD COLUMN prior_valid_to TIMESTAMPTZ,
  ADD COLUMN prior_expires_at TIMESTAMPTZ,
  ADD COLUMN prior_superseded_by_memory_id UUID,
  ADD COLUMN prior_sensitivity TEXT,
  ADD COLUMN prior_temporal_basis TEXT,
  ADD COLUMN prior_temporal_parser_version TEXT,
  ADD CONSTRAINT user_memory_revisions_snapshot_schema_supported CHECK (
    prior_snapshot_schema_major IS NULL OR prior_snapshot_schema_major = 1
  ),
  ADD CONSTRAINT user_memory_revisions_full_snapshot_shape CHECK (
    prior_snapshot_schema_major IS NULL
    OR (
      purged_at IS NULL
      AND prior_content_snapshot IS NOT NULL
      AND prior_memory_type IS NOT NULL
      AND prior_normalized_content IS NOT NULL
      AND prior_importance BETWEEN 1 AND 5
      AND prior_tags IS NOT NULL
      AND prior_source IN ('manual', 'ai', 'direct_user')
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

DROP TRIGGER user_memory_revisions_append_only ON user_memory_revisions;
DROP FUNCTION user_memory_revision_append_only_guard();

CREATE FUNCTION user_memory_revision_append_only_guard()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF NOT EXISTS (
      SELECT 1 FROM user_memories memory
      WHERE memory.id = OLD.memory_id AND memory.user_id = OLD.user_id
    ) THEN
      RETURN OLD;
    END IF;
    RAISE EXCEPTION USING
      ERRCODE = '55000', MESSAGE = 'MEMORY_REVISION_APPEND_ONLY';
  END IF;

  IF OLD.prior_content_snapshot IS NOT NULL
    AND NEW.prior_content_snapshot IS NULL
    AND OLD.purged_at IS NULL
    AND NEW.purged_at IS NOT NULL
    AND OLD.result_code IS NULL
    AND NEW.result_code = 'ONLINE_PURGED'
    AND ROW(
      NEW.memory_id, NEW.revision, NEW.user_id, NEW.operation,
      NEW.old_content_hash, NEW.new_content_hash, NEW.actor_type,
      NEW.job_id, NEW.created_at, NEW.prior_snapshot_schema_major
    ) IS NOT DISTINCT FROM ROW(
      OLD.memory_id, OLD.revision, OLD.user_id, OLD.operation,
      OLD.old_content_hash, OLD.new_content_hash, OLD.actor_type,
      OLD.job_id, OLD.created_at, OLD.prior_snapshot_schema_major
    )
  THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION USING
    ERRCODE = '55000', MESSAGE = 'MEMORY_REVISION_APPEND_ONLY';
END
$function$;

CREATE TRIGGER user_memory_revisions_append_only
BEFORE UPDATE OR DELETE ON user_memory_revisions
FOR EACH ROW EXECUTE FUNCTION user_memory_revision_append_only_guard();

CREATE TABLE memory_user_actions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_conversation_id UUID NOT NULL,
  source_message_id UUID NOT NULL,
  assistant_message_id UUID NOT NULL,
  schema_major SMALLINT NOT NULL,
  requested_action TEXT NOT NULL,
  proposed_scope_type TEXT NOT NULL,
  resolved_project_id UUID,
  resolved_conversation_id UUID,
  candidate_hash TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL,
  status TEXT NOT NULL,
  result_code TEXT NOT NULL,
  result_memory_id UUID,
  result_memory_revision BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_user_actions_id_user_unique UNIQUE (id, user_id),
  CONSTRAINT memory_user_actions_source_unique UNIQUE (user_id, source_message_id),
  CONSTRAINT memory_user_actions_source_conversation_owner_fk
    FOREIGN KEY (source_conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE CASCADE,
  CONSTRAINT memory_user_actions_source_message_owner_fk
    FOREIGN KEY (source_message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT memory_user_actions_assistant_owner_fk
    FOREIGN KEY (assistant_message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT memory_user_actions_project_owner_fk
    FOREIGN KEY (resolved_project_id, user_id)
    REFERENCES projects(id, user_id) ON DELETE RESTRICT,
  CONSTRAINT memory_user_actions_resolved_conversation_owner_fk
    FOREIGN KEY (resolved_conversation_id, user_id)
    REFERENCES conversations(id, user_id) ON DELETE RESTRICT,
  CONSTRAINT memory_user_actions_result_memory_owner_fk
    FOREIGN KEY (result_memory_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE CASCADE,
  CONSTRAINT memory_user_actions_schema_supported CHECK (schema_major = 1),
  CONSTRAINT memory_user_actions_action_allowed
    CHECK (requested_action IN ('remember', 'correct', 'forget')),
  CONSTRAINT memory_user_actions_scope_allowed
    CHECK (proposed_scope_type IN ('global', 'project', 'conversation')),
  CONSTRAINT memory_user_actions_scope_shape CHECK (
    (proposed_scope_type = 'global'
      AND resolved_project_id IS NULL AND resolved_conversation_id IS NULL)
    OR (proposed_scope_type = 'project'
      AND resolved_project_id IS NOT NULL AND resolved_conversation_id IS NULL)
    OR (proposed_scope_type = 'conversation'
      AND resolved_project_id IS NULL AND resolved_conversation_id IS NOT NULL)
    OR (status IN ('review_required', 'rejected', 'failed')
      AND resolved_project_id IS NULL AND resolved_conversation_id IS NULL)
  ),
  CONSTRAINT memory_user_actions_candidate_hash_check
    CHECK (candidate_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_user_actions_confidence_range CHECK (confidence BETWEEN 0 AND 1),
  CONSTRAINT memory_user_actions_status_allowed CHECK (
    status IN ('applied', 'noop', 'review_required', 'rejected', 'failed')
  ),
  CONSTRAINT memory_user_actions_result_code_sanitized
    CHECK (result_code ~ '^[A-Z0-9_]{1,64}$'),
  CONSTRAINT memory_user_actions_result_shape CHECK (
    (status IN ('applied', 'noop')
      AND result_memory_id IS NOT NULL AND result_memory_revision >= 1)
    OR status IN ('review_required', 'rejected', 'failed')
  ),
  CONSTRAINT memory_user_actions_completed_order CHECK (completed_at >= created_at)
);

CREATE INDEX idx_memory_user_actions_user_created
  ON memory_user_actions(user_id, created_at DESC, id);

CREATE TABLE memory_user_action_targets (
  action_id UUID NOT NULL,
  memory_id UUID NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expected_revision BIGINT NOT NULL,
  applied_revision BIGINT,
  resolution TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (action_id, memory_id),
  CONSTRAINT memory_user_action_targets_action_owner_fk
    FOREIGN KEY (action_id, user_id)
    REFERENCES memory_user_actions(id, user_id) ON DELETE CASCADE,
  CONSTRAINT memory_user_action_targets_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE CASCADE,
  CONSTRAINT memory_user_action_targets_expected_revision_positive
    CHECK (expected_revision >= 1),
  CONSTRAINT memory_user_action_targets_applied_revision_positive
    CHECK (applied_revision IS NULL OR applied_revision >= 1),
  CONSTRAINT memory_user_action_targets_resolution_allowed
    CHECK (resolution IN ('current', 'stale', 'applied'))
);

CREATE TABLE message_memory_activities (
  id UUID PRIMARY KEY,
  assistant_message_id UUID NOT NULL,
  ordinal SMALLINT NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  subject_type TEXT NOT NULL,
  subject_id UUID NOT NULL,
  subject_revision BIGINT,
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  undo_kind TEXT NOT NULL DEFAULT 'none',
  undo_status TEXT NOT NULL DEFAULT 'unavailable',
  source_kind TEXT NOT NULL,
  source_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT message_memory_activities_message_ordinal_unique
    UNIQUE (assistant_message_id, ordinal),
  CONSTRAINT message_memory_activities_source_unique
    UNIQUE (source_kind, source_id),
  CONSTRAINT message_memory_activities_assistant_owner_fk
    FOREIGN KEY (assistant_message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT message_memory_activities_ordinal_bounded
    CHECK (ordinal BETWEEN 1 AND 64),
  CONSTRAINT message_memory_activities_subject_allowed
    CHECK (subject_type IN ('memory', 'review_suggestion', 'job', 'action')),
  CONSTRAINT message_memory_activities_subject_revision_positive
    CHECK (subject_revision IS NULL OR subject_revision >= 1),
  CONSTRAINT message_memory_activities_action_allowed CHECK (
    action IN ('created', 'corrected', 'forgotten', 'review_required', 'rejected', 'failed')
  ),
  CONSTRAINT message_memory_activities_status_allowed CHECK (
    status IN ('pending', 'completed', 'review_required', 'failed', 'undone')
  ),
  CONSTRAINT message_memory_activities_reason_sanitized
    CHECK (reason_code ~ '^[A-Z0-9_]{1,64}$'),
  CONSTRAINT message_memory_activities_undo_kind_allowed
    CHECK (undo_kind IN ('none', 'created', 'corrected')),
  CONSTRAINT message_memory_activities_undo_status_allowed
    CHECK (undo_status IN ('unavailable', 'available', 'undone', 'review_required')),
  CONSTRAINT message_memory_activities_source_kind_allowed CHECK (
    source_kind IN ('direct_action', 'review_suggestion', 'memory_job')
  ),
  CONSTRAINT message_memory_activities_undo_shape CHECK (
    (undo_kind = 'none' AND undo_status = 'unavailable')
    OR (undo_kind IN ('created', 'corrected')
      AND undo_status IN ('available', 'undone', 'review_required'))
  ),
  CONSTRAINT message_memory_activities_timestamps_order CHECK (updated_at >= created_at)
);

CREATE INDEX idx_message_memory_activities_user_cursor
  ON message_memory_activities(user_id, created_at, id);

CREATE TABLE message_memory_usages (
  assistant_message_id UUID NOT NULL,
  ordinal SMALLINT NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entity_type TEXT NOT NULL,
  entity_id UUID NOT NULL,
  entity_revision BIGINT NOT NULL,
  layer TEXT NOT NULL,
  scope_type TEXT NOT NULL,
  purpose TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (assistant_message_id, ordinal),
  CONSTRAINT message_memory_usages_assistant_owner_fk
    FOREIGN KEY (assistant_message_id, user_id)
    REFERENCES messages(id, user_id) ON DELETE CASCADE,
  CONSTRAINT message_memory_usages_memory_owner_fk
    FOREIGN KEY (entity_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE CASCADE,
  CONSTRAINT message_memory_usages_ordinal_bounded CHECK (ordinal BETWEEN 1 AND 5),
  CONSTRAINT message_memory_usages_entity_type_allowed
    CHECK (entity_type = 'l1_memory'),
  CONSTRAINT message_memory_usages_revision_positive CHECK (entity_revision >= 1),
  CONSTRAINT message_memory_usages_layer_allowed CHECK (layer = 'l1'),
  CONSTRAINT message_memory_usages_scope_allowed
    CHECK (scope_type IN ('global', 'project', 'conversation')),
  CONSTRAINT message_memory_usages_purpose_allowed CHECK (purpose = 'answer_context'),
  CONSTRAINT message_memory_usages_entity_unique
    UNIQUE (assistant_message_id, entity_type, entity_id)
);

CREATE INDEX idx_message_memory_usages_user_entity
  ON message_memory_usages(user_id, entity_id, created_at DESC);

CREATE FUNCTION memory_next_activity_ordinal(p_assistant_message_id UUID)
RETURNS SMALLINT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_ordinal SMALLINT;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtextextended(p_assistant_message_id::TEXT, 0));
  SELECT COALESCE(max(activity.ordinal), 0) + 1
  INTO v_ordinal
  FROM message_memory_activities activity
  WHERE activity.assistant_message_id = p_assistant_message_id;
  IF v_ordinal > 64 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_ACTIVITY_LIMIT_EXCEEDED';
  END IF;
  RETURN v_ordinal;
END
$function$;

CREATE FUNCTION memory_hydrate_direct_user_action(
  p_user_id UUID,
  p_conversation_id UUID,
  p_source_message_id UUID,
  p_assistant_message_id UUID
) RETURNS TABLE (
  project_id UUID,
  conversation_scope_generation BIGINT,
  project_scope_generation BIGINT,
  sensitive_memory_enabled BOOLEAN,
  current_memories JSONB
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_conversation conversations%ROWTYPE;
  v_sensitive BOOLEAN := false;
  v_project_generation BIGINT;
  v_memories JSONB;
BEGIN
  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = p_conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_DIRECT_CONVERSATION_INVALID';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM messages source
    WHERE source.id = p_source_message_id
      AND source.conversation_id = p_conversation_id
      AND source.user_id = p_user_id
      AND source.role = 'user'
      AND source.status = 'completed'
      AND source.deleted_at IS NULL
  ) OR NOT EXISTS (
    SELECT 1 FROM messages assistant
    WHERE assistant.id = p_assistant_message_id
      AND assistant.conversation_id = p_conversation_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.status IN ('pending', 'streaming')
      AND assistant.parent_message_id = p_source_message_id
      AND assistant.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_DIRECT_MESSAGE_INVALID';
  END IF;

  SELECT settings.sensitive_memory_enabled INTO v_sensitive
  FROM user_memory_settings settings WHERE settings.user_id = p_user_id;
  v_sensitive := COALESCE(v_sensitive, false);

  IF v_conversation.project_id IS NOT NULL THEN
    SELECT project.scope_generation INTO v_project_generation
    FROM projects project
    WHERE project.id = v_conversation.project_id
      AND project.user_id = p_user_id
      AND project.lifecycle_status = 'active'
      AND project.deleted_at IS NULL;
  END IF;

  SELECT COALESCE(jsonb_agg(
    memory_row.payload
    ORDER BY memory_row.rank, memory_row.updated_at DESC, memory_row.id
  ), '[]'::JSONB)
  INTO v_memories
  FROM (
    SELECT
      CASE memory.scope_type WHEN 'conversation' THEN 1 WHEN 'project' THEN 2 ELSE 3 END AS rank,
      memory.id,
      memory.updated_at,
      jsonb_build_object(
        'id', memory.id::TEXT,
        'revision', memory.revision,
        'type', memory.memory_type,
        'content', memory.content,
        'authorityKind', memory.authority_kind,
        'scopeType', memory.scope_type,
        'projectId', CASE WHEN memory.project_id IS NULL THEN NULL ELSE memory.project_id::TEXT END,
        'conversationId', CASE WHEN memory.scope_conversation_id IS NULL THEN NULL ELSE memory.scope_conversation_id::TEXT END,
        'sensitivity', memory.sensitivity
      ) AS payload
    FROM user_memories memory
    JOIN user_memory_state state
      ON state.user_id = memory.user_id
      AND state.visibility_epoch = memory.visibility_epoch
    WHERE memory.user_id = p_user_id
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND (memory.sensitivity = 'normal' OR v_sensitive)
      AND (
        (memory.scope_type = 'global' AND memory.scope_generation = 1)
        OR (
          memory.scope_type = 'project'
          AND memory.project_id = v_conversation.project_id
          AND v_project_generation IS NOT NULL
          AND memory.scope_generation = v_project_generation
        )
        OR (
          memory.scope_type = 'conversation'
          AND memory.scope_conversation_id = p_conversation_id
          AND memory.scope_generation = v_conversation.memory_scope_generation
        )
      )
    ORDER BY rank, memory.updated_at DESC, memory.id
    LIMIT 20
  ) memory_row;

  RETURN QUERY SELECT
    v_conversation.project_id,
    v_conversation.memory_scope_generation,
    v_project_generation,
    v_sensitive,
    v_memories;
END
$function$;

CREATE FUNCTION memory_delete_direct_scoped(
  p_user_id UUID,
  p_memory_id UUID,
  p_expected_revision BIGINT,
  p_event_id UUID,
  p_job_id UUID,
  p_tombstone_id UUID,
  p_manifest_id UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory user_memories%ROWTYPE;
  v_epoch BIGINT;
  v_source_hash TEXT;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT memory.* INTO v_memory
  FROM user_memories memory
  WHERE memory.id = p_memory_id
    AND memory.user_id = p_user_id
    AND memory.deleted_at IS NULL
  FOR UPDATE;
  IF NOT FOUND OR v_memory.revision <> p_expected_revision THEN
    RETURN false;
  END IF;

  INSERT INTO user_memory_state(user_id) VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_epoch
  FROM user_memory_state state
  WHERE state.user_id = p_user_id FOR UPDATE;

  SELECT evidence.source_content_hash INTO v_source_hash
  FROM user_memory_evidence evidence
  WHERE evidence.memory_id = v_memory.id
    AND evidence.source_message_id = v_memory.source_message_id
    AND evidence.evidence_role = 'user'
  LIMIT 1;

  INSERT INTO user_memory_revisions (
    memory_id, revision, user_id, operation, old_content_hash,
    new_content_hash, prior_content_snapshot, actor_type,
    prior_snapshot_schema_major, prior_memory_type,
    prior_normalized_content, prior_importance, prior_tags, prior_source,
    prior_source_conversation_id, prior_source_message_id, prior_enabled,
    prior_last_used_at, prior_scope_type, prior_project_id,
    prior_scope_conversation_id, prior_scope_generation,
    prior_visibility_epoch, prior_authority_kind, prior_extraction_profile_id,
    prior_lifecycle_status, prior_subject_key, prior_fact_key,
    prior_confidence, prior_observed_at, prior_valid_from, prior_valid_to,
    prior_expires_at, prior_superseded_by_memory_id, prior_sensitivity,
    prior_temporal_basis, prior_temporal_parser_version
  ) VALUES (
    v_memory.id, v_memory.revision + 1, p_user_id, 'delete',
    v_memory.content_hash, v_memory.content_hash, v_memory.content, 'user',
    1, v_memory.memory_type, v_memory.normalized_content,
    v_memory.importance, v_memory.tags, v_memory.source,
    v_memory.source_conversation_id, v_memory.source_message_id,
    v_memory.enabled, v_memory.last_used_at, v_memory.scope_type,
    v_memory.project_id, v_memory.scope_conversation_id,
    v_memory.scope_generation, v_memory.visibility_epoch,
    v_memory.authority_kind, v_memory.extraction_profile_id,
    v_memory.lifecycle_status, v_memory.subject_key, v_memory.fact_key,
    v_memory.confidence, v_memory.observed_at, v_memory.valid_from,
    v_memory.valid_to, v_memory.expires_at,
    v_memory.superseded_by_memory_id, v_memory.sensitivity,
    v_memory.temporal_basis, v_memory.temporal_parser_version
  );

  UPDATE user_memories memory
  SET enabled = false, deleted_at = v_now,
      revision = memory.revision + 1, updated_at = v_now
  WHERE memory.id = v_memory.id;

  INSERT INTO user_memory_tombstones (
    id, user_id, memory_id, content_hash,
    source_conversation_id, source_message_id, source_content_hash,
    reason, created_at
  ) VALUES (
    p_tombstone_id, p_user_id, v_memory.id, v_memory.content_hash,
    v_memory.source_conversation_id,
    CASE WHEN v_source_hash IS NULL THEN NULL ELSE v_memory.source_message_id END,
    v_source_hash, 'user_delete', v_now
  );

  INSERT INTO user_memory_deletion_manifests (
    manifest_id, event_id, user_id, memory_id, tombstone_id,
    content_hash, scope_generation, visibility_epoch, deleted_at, created_at
  ) VALUES (
    p_manifest_id, p_event_id, p_user_id, v_memory.id, p_tombstone_id,
    v_memory.content_hash, v_memory.scope_generation, v_epoch, v_now, v_now
  );

  INSERT INTO memory_outbox (
    event_id, user_id, event_schema_major, event_type, aggregate_id,
    visibility_epoch, payload, created_at, updated_at, available_at,
    max_attempts
  ) VALUES (
    p_event_id, p_user_id, 2, 'memory.deleted', v_memory.id, v_epoch,
    jsonb_build_object(
      'schemaMajor', 2, 'memoryId', v_memory.id::TEXT,
      'tombstoneId', p_tombstone_id::TEXT,
      'contentHash', v_memory.content_hash,
      'scopeGeneration', v_memory.scope_generation,
      'visibilityEpoch', v_epoch, 'deletedAt', v_now
    ), v_now, v_now, v_now, 128
  );

  INSERT INTO memory_jobs (
    job_id, user_id, event_id, stage, idempotency_key,
    scope_generation, visibility_epoch, target_memory_id,
    target_tombstone_id, created_at, updated_at, available_at, max_attempts
  ) VALUES (
    p_job_id, p_user_id, p_event_id, 'purge',
    'memory:purge:v2:' || v_memory.id::TEXT,
    v_memory.scope_generation, v_epoch, v_memory.id,
    p_tombstone_id, v_now, v_now, v_now, 128
  );
  RETURN true;
END
$function$;

CREATE FUNCTION memory_apply_direct_user_action(
  p_action_id UUID,
  p_activity_id UUID,
  p_memory_id UUID,
  p_event_id UUID,
  p_job_id UUID,
  p_tombstone_id UUID,
  p_manifest_id UUID,
  p_user_id UUID,
  p_conversation_id UUID,
  p_source_message_id UUID,
  p_assistant_message_id UUID,
  p_schema_major SMALLINT,
  p_requested_action TEXT,
  p_memory_type TEXT,
  p_content TEXT,
  p_normalized_content TEXT,
  p_candidate_hash TEXT,
  p_importance SMALLINT,
  p_tags TEXT[],
  p_sensitivity TEXT,
  p_proposed_scope_type TEXT,
  p_confidence DOUBLE PRECISION,
  p_targets JSONB,
  p_preflight_status TEXT,
  p_preflight_result_code TEXT
) RETURNS TABLE (
  action_id UUID,
  action_status TEXT,
  action_result_code TEXT,
  result_memory_id UUID,
  result_memory_revision BIGINT,
  resolved_scope_type TEXT,
  activity_id UUID
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
#variable_conflict use_column
DECLARE
  v_conversation conversations%ROWTYPE;
  v_source messages%ROWTYPE;
  v_existing_action memory_user_actions%ROWTYPE;
  v_target user_memories%ROWTYPE;
  v_exact user_memories%ROWTYPE;
  v_epoch BIGINT;
  v_project_generation BIGINT;
  v_scope_generation BIGINT;
  v_resolved_project_id UUID;
  v_resolved_conversation_id UUID;
  v_target_count INTEGER := 0;
  v_target_id UUID;
  v_expected_revision BIGINT;
  v_target_resolution TEXT;
  v_status TEXT;
  v_result_code TEXT;
  v_result_memory_id UUID;
  v_result_revision BIGINT;
  v_activity_action TEXT;
  v_activity_status TEXT;
  v_activity_subject_type TEXT;
  v_activity_subject_id UUID;
  v_undo_kind TEXT := 'none';
  v_undo_status TEXT := 'unavailable';
  v_now TIMESTAMPTZ := clock_timestamp();
  v_observed_at TIMESTAMPTZ;
BEGIN
  SELECT action.* INTO v_existing_action
  FROM memory_user_actions action
  WHERE action.user_id = p_user_id
    AND action.source_message_id = p_source_message_id;
  IF FOUND THEN
    RETURN QUERY
    SELECT
      v_existing_action.id, v_existing_action.status,
      v_existing_action.result_code, v_existing_action.result_memory_id,
      v_existing_action.result_memory_revision,
      v_existing_action.proposed_scope_type,
      activity.id
    FROM (SELECT 1) singleton
    LEFT JOIN message_memory_activities activity
      ON activity.source_kind = 'direct_action'
      AND activity.source_id = v_existing_action.id;
    RETURN;
  END IF;

  IF p_schema_major <> 1
    OR lower(trim(p_requested_action)) NOT IN ('remember', 'correct', 'forget')
    OR lower(trim(p_proposed_scope_type)) NOT IN ('global', 'project', 'conversation')
    OR p_candidate_hash !~ '^[0-9a-f]{64}$'
    OR p_confidence NOT BETWEEN 0 AND 1
    OR jsonb_typeof(p_targets) <> 'array'
    OR jsonb_array_length(p_targets) > 5
    OR NOT (
      (p_preflight_status IS NULL AND p_preflight_result_code IS NULL)
      OR (
        p_preflight_status IN ('review_required', 'rejected', 'failed')
        AND p_preflight_result_code ~ '^[A-Z0-9_]{1,64}$'
      )
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_DIRECT_ACTION_INVALID';
  END IF;

  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = p_conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_DIRECT_CONVERSATION_INVALID';
  END IF;

  SELECT source.* INTO v_source
  FROM messages source
  WHERE source.id = p_source_message_id
    AND source.conversation_id = p_conversation_id
    AND source.user_id = p_user_id
    AND source.role = 'user'
    AND source.status = 'completed'
    AND source.deleted_at IS NULL;
  IF NOT FOUND OR NOT EXISTS (
    SELECT 1 FROM messages assistant
    WHERE assistant.id = p_assistant_message_id
      AND assistant.conversation_id = p_conversation_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.status IN ('pending', 'streaming')
      AND assistant.parent_message_id = p_source_message_id
      AND assistant.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_DIRECT_MESSAGE_INVALID';
  END IF;
  v_observed_at := COALESCE(v_source.completed_at, v_source.created_at);

  INSERT INTO user_memory_state(user_id) VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_epoch
  FROM user_memory_state state
  WHERE state.user_id = p_user_id FOR UPDATE;

  p_requested_action := lower(trim(p_requested_action));
  p_proposed_scope_type := lower(trim(p_proposed_scope_type));
  CASE p_proposed_scope_type
    WHEN 'global' THEN
      v_scope_generation := 1;
    WHEN 'project' THEN
      IF v_conversation.project_id IS NOT NULL THEN
        SELECT project.scope_generation INTO v_project_generation
        FROM projects project
        WHERE project.id = v_conversation.project_id
          AND project.user_id = p_user_id
          AND project.lifecycle_status = 'active'
          AND project.deleted_at IS NULL
        FOR UPDATE;
      END IF;
      IF v_project_generation IS NOT NULL THEN
        v_resolved_project_id := v_conversation.project_id;
        v_scope_generation := v_project_generation;
      END IF;
    WHEN 'conversation' THEN
      v_resolved_conversation_id := p_conversation_id;
      v_scope_generation := v_conversation.memory_scope_generation;
  END CASE;

  FOR v_target_count, v_target_id, v_expected_revision IN
    SELECT
      row_number() OVER ()::INTEGER,
      CASE
        WHEN target->>'memoryId' ~
          '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$'
        THEN (target->>'memoryId')::UUID
        ELSE NULL
      END,
      CASE
        WHEN target->>'expectedRevision' ~ '^[1-9][0-9]*$'
        THEN (target->>'expectedRevision')::BIGINT
        ELSE NULL
      END
    FROM jsonb_array_elements(p_targets) target
    WHERE jsonb_typeof(target) = 'object'
      AND target ?& ARRAY['memoryId', 'expectedRevision']
      AND NOT target ?| ARRAY(
        SELECT key FROM jsonb_object_keys(target) key
        WHERE key NOT IN ('memoryId', 'expectedRevision')
      )
  LOOP
    -- The loop intentionally leaves only the final target values. Mutations
    -- below require the original JSON array to contain exactly one object.
    NULL;
  END LOOP;
  v_target_count := jsonb_array_length(p_targets);

  IF p_preflight_status IS NOT NULL THEN
    v_status := p_preflight_status;
    v_result_code := p_preflight_result_code;
  ELSIF v_scope_generation IS NULL THEN
    v_status := 'review_required';
    v_result_code := 'SCOPE_UNAVAILABLE';
  ELSIF p_confidence < 0.80 THEN
    v_status := 'review_required';
    v_result_code := 'LOW_CONFIDENCE';
  ELSIF p_sensitivity = 'secret' THEN
    v_status := 'rejected';
    v_result_code := 'SECRET_REJECTED';
  ELSIF (p_requested_action = 'remember' AND v_target_count <> 0)
    OR (p_requested_action IN ('correct', 'forget') AND v_target_count <> 1)
  THEN
    v_status := 'review_required';
    v_result_code := 'AMBIGUOUS_TARGET';
  ELSIF p_requested_action IN ('remember', 'correct') AND (
    p_memory_type NOT IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    )
    OR p_content IS NULL OR length(trim(p_content)) = 0
    OR char_length(p_content) > 2000
    OR p_normalized_content IS NULL OR length(trim(p_normalized_content)) = 0
    OR char_length(p_normalized_content) > 2000
    OR p_importance NOT BETWEEN 1 AND 5
    OR p_tags IS NULL OR cardinality(p_tags) > 12
    OR p_sensitivity NOT IN ('normal', 'sensitive')
    OR encode(sha256(convert_to(p_content, 'UTF8')), 'hex') <> p_candidate_hash
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_DIRECT_CANDIDATE_INVALID';
  END IF;

  IF v_status IS NULL AND p_requested_action IN ('correct', 'forget') THEN
    SELECT memory.* INTO v_target
    FROM user_memories memory
    WHERE memory.id = v_target_id
      AND memory.user_id = p_user_id
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND memory.visibility_epoch = v_epoch
      AND memory.scope_generation = v_scope_generation
      AND memory.scope_type = p_proposed_scope_type
      AND (memory.scope_type <> 'project' OR memory.project_id = v_resolved_project_id)
      AND (memory.scope_type <> 'conversation'
        OR memory.scope_conversation_id = v_resolved_conversation_id)
    FOR UPDATE;
    IF NOT FOUND THEN
      v_status := 'review_required';
      v_result_code := 'TARGET_INVALID';
    ELSIF v_target.revision <> v_expected_revision THEN
      v_status := 'review_required';
      v_result_code := 'REVISION_STALE';
      v_target_resolution := 'stale';
      v_result_memory_id := v_target.id;
      v_result_revision := v_target.revision;
    ELSE
      v_target_resolution := 'current';
      v_result_memory_id := v_target.id;
      v_result_revision := v_target.revision;
    END IF;
  END IF;

  IF v_status IS NULL AND p_requested_action = 'remember' THEN
    SELECT memory.* INTO v_exact
    FROM user_memories memory
    WHERE memory.user_id = p_user_id
      AND memory.normalized_content = p_normalized_content
      AND memory.scope_type = p_proposed_scope_type
      AND (memory.scope_type <> 'project' OR memory.project_id = v_resolved_project_id)
      AND (memory.scope_type <> 'conversation'
        OR memory.scope_conversation_id = v_resolved_conversation_id)
      AND memory.deleted_at IS NULL
    FOR UPDATE;
    IF FOUND
      AND v_exact.scope_generation = v_scope_generation
      AND v_exact.visibility_epoch = v_epoch
      AND v_exact.enabled
      AND v_exact.lifecycle_status = 'active'
    THEN
      v_status := 'noop';
      v_result_code := 'EXACT_NOOP';
      v_result_memory_id := v_exact.id;
      v_result_revision := v_exact.revision;
    ELSIF FOUND THEN
      v_status := 'review_required';
      v_result_code := 'EXACT_CONFLICT';
    ELSE
      INSERT INTO user_memories (
        id, user_id, memory_type, content, normalized_content, importance,
        tags, source, source_conversation_id, source_message_id, enabled,
        scope_type, project_id, scope_conversation_id, scope_generation,
        revision, visibility_epoch, content_hash, authority_kind,
        extraction_profile_id, lifecycle_status, confidence, observed_at,
        sensitivity, temporal_basis
      ) VALUES (
        p_memory_id, p_user_id, p_memory_type, p_content,
        p_normalized_content, p_importance, p_tags, 'direct_user',
        p_conversation_id, p_source_message_id, true,
        p_proposed_scope_type, v_resolved_project_id,
        v_resolved_conversation_id, v_scope_generation,
        1, v_epoch, p_candidate_hash, 'direct_user', NULL,
        'active', 1.0, v_observed_at, p_sensitivity, 'none'
      );
      INSERT INTO user_memory_evidence (
        memory_id, source_message_id, user_id, source_conversation_id,
        evidence_role, source_content_hash, observed_at
      ) VALUES (
        p_memory_id, p_source_message_id, p_user_id, p_conversation_id,
        'user', encode(sha256(convert_to(v_source.content, 'UTF8')), 'hex'),
        v_observed_at
      );
      v_status := 'applied';
      v_result_code := 'DIRECT_CREATED';
      v_result_memory_id := p_memory_id;
      v_result_revision := 1;
    END IF;
  ELSIF v_status IS NULL AND p_requested_action = 'correct' THEN
    SELECT memory.* INTO v_exact
    FROM user_memories memory
    WHERE memory.user_id = p_user_id
      AND memory.normalized_content = p_normalized_content
      AND memory.scope_type = v_target.scope_type
      AND (memory.scope_type <> 'project' OR memory.project_id = v_target.project_id)
      AND (memory.scope_type <> 'conversation'
        OR memory.scope_conversation_id = v_target.scope_conversation_id)
      AND memory.id <> v_target.id
      AND memory.deleted_at IS NULL
    FOR UPDATE;
    IF FOUND THEN
      v_status := 'review_required';
      v_result_code := 'EXACT_CONFLICT';
    ELSE
      INSERT INTO user_memory_revisions (
        memory_id, revision, user_id, operation, old_content_hash,
        new_content_hash, prior_content_snapshot, actor_type,
        prior_snapshot_schema_major, prior_memory_type,
        prior_normalized_content, prior_importance, prior_tags, prior_source,
        prior_source_conversation_id, prior_source_message_id, prior_enabled,
        prior_last_used_at, prior_scope_type, prior_project_id,
        prior_scope_conversation_id, prior_scope_generation,
        prior_visibility_epoch, prior_authority_kind,
        prior_extraction_profile_id, prior_lifecycle_status,
        prior_subject_key, prior_fact_key, prior_confidence,
        prior_observed_at, prior_valid_from, prior_valid_to, prior_expires_at,
        prior_superseded_by_memory_id, prior_sensitivity,
        prior_temporal_basis, prior_temporal_parser_version
      ) VALUES (
        v_target.id, v_target.revision + 1, p_user_id, 'update',
        v_target.content_hash, p_candidate_hash, v_target.content, 'user',
        1, v_target.memory_type, v_target.normalized_content,
        v_target.importance, v_target.tags, v_target.source,
        v_target.source_conversation_id, v_target.source_message_id,
        v_target.enabled, v_target.last_used_at, v_target.scope_type,
        v_target.project_id, v_target.scope_conversation_id,
        v_target.scope_generation, v_target.visibility_epoch,
        v_target.authority_kind, v_target.extraction_profile_id,
        v_target.lifecycle_status, v_target.subject_key, v_target.fact_key,
        v_target.confidence, v_target.observed_at, v_target.valid_from,
        v_target.valid_to, v_target.expires_at,
        v_target.superseded_by_memory_id, v_target.sensitivity,
        v_target.temporal_basis, v_target.temporal_parser_version
      );
      UPDATE user_memories memory
      SET memory_type = p_memory_type,
          content = p_content,
          normalized_content = p_normalized_content,
          importance = p_importance,
          tags = p_tags,
          source = 'direct_user',
          source_conversation_id = p_conversation_id,
          source_message_id = p_source_message_id,
          enabled = true,
          revision = memory.revision + 1,
          visibility_epoch = v_epoch,
          content_hash = p_candidate_hash,
          authority_kind = 'direct_user',
          extraction_profile_id = NULL,
          lifecycle_status = 'active',
          subject_key = NULL,
          fact_key = NULL,
          confidence = 1.0,
          observed_at = v_observed_at,
          valid_from = NULL,
          valid_to = NULL,
          expires_at = NULL,
          superseded_by_memory_id = NULL,
          sensitivity = p_sensitivity,
          temporal_basis = 'none',
          temporal_parser_version = NULL,
          updated_at = v_now
      WHERE memory.id = v_target.id;
      INSERT INTO user_memory_evidence (
        memory_id, source_message_id, user_id, source_conversation_id,
        evidence_role, source_content_hash, observed_at
      ) VALUES (
        v_target.id, p_source_message_id, p_user_id, p_conversation_id,
        'user', encode(sha256(convert_to(v_source.content, 'UTF8')), 'hex'),
        v_observed_at
      ) ON CONFLICT (memory_id, source_message_id) DO NOTHING;
      v_status := 'applied';
      v_result_code := 'DIRECT_CORRECTED';
      v_result_memory_id := v_target.id;
      v_result_revision := v_target.revision + 1;
      v_target_resolution := 'applied';
    END IF;
  ELSIF v_status IS NULL AND p_requested_action = 'forget' THEN
    IF memory_delete_direct_scoped(
      p_user_id, v_target.id, v_target.revision,
      p_event_id, p_job_id, p_tombstone_id, p_manifest_id
    ) THEN
      v_status := 'applied';
      v_result_code := 'DIRECT_FORGOTTEN';
      v_result_memory_id := v_target.id;
      v_result_revision := v_target.revision + 1;
      v_target_resolution := 'applied';
    ELSE
      v_status := 'review_required';
      v_result_code := 'REVISION_STALE';
      v_target_resolution := 'stale';
    END IF;
  END IF;

  INSERT INTO memory_user_actions (
    id, user_id, source_conversation_id, source_message_id,
    assistant_message_id, schema_major, requested_action,
    proposed_scope_type, resolved_project_id, resolved_conversation_id,
    candidate_hash, confidence, status, result_code,
    result_memory_id, result_memory_revision, created_at, completed_at
  ) VALUES (
    p_action_id, p_user_id, p_conversation_id, p_source_message_id,
    p_assistant_message_id, p_schema_major, p_requested_action,
    p_proposed_scope_type, v_resolved_project_id, v_resolved_conversation_id,
    p_candidate_hash, p_confidence, v_status, v_result_code,
    v_result_memory_id, v_result_revision, v_now, v_now
  );

  IF v_target.id IS NOT NULL AND v_expected_revision IS NOT NULL THEN
    INSERT INTO memory_user_action_targets (
      action_id, memory_id, user_id, expected_revision,
      applied_revision, resolution, created_at
    ) VALUES (
      p_action_id, v_target.id, p_user_id, v_expected_revision,
      CASE WHEN v_target_resolution = 'applied' THEN v_result_revision ELSE NULL END,
      COALESCE(v_target_resolution, 'current'), v_now
    );
  END IF;

  IF v_status <> 'noop' THEN
    CASE
      WHEN v_status = 'applied' AND p_requested_action = 'remember' THEN
        v_activity_action := 'created';
        v_activity_status := 'completed';
        v_activity_subject_type := 'memory';
        v_activity_subject_id := v_result_memory_id;
        v_undo_kind := 'created';
        v_undo_status := 'available';
      WHEN v_status = 'applied' AND p_requested_action = 'correct' THEN
        v_activity_action := 'corrected';
        v_activity_status := 'completed';
        v_activity_subject_type := 'memory';
        v_activity_subject_id := v_result_memory_id;
        v_undo_kind := 'corrected';
        v_undo_status := 'available';
      WHEN v_status = 'applied' AND p_requested_action = 'forget' THEN
        v_activity_action := 'forgotten';
        v_activity_status := 'completed';
        v_activity_subject_type := 'memory';
        v_activity_subject_id := v_result_memory_id;
      WHEN v_status = 'review_required' THEN
        v_activity_action := 'review_required';
        v_activity_status := 'review_required';
        v_activity_subject_type := 'action';
        v_activity_subject_id := p_action_id;
      WHEN v_status = 'rejected' THEN
        v_activity_action := 'rejected';
        v_activity_status := 'completed';
        v_activity_subject_type := 'action';
        v_activity_subject_id := p_action_id;
      ELSE
        v_activity_action := 'failed';
        v_activity_status := 'failed';
        v_activity_subject_type := 'action';
        v_activity_subject_id := p_action_id;
    END CASE;
    INSERT INTO message_memory_activities (
      id, assistant_message_id, ordinal, user_id, subject_type,
      subject_id, subject_revision, action, status, reason_code,
      undo_kind, undo_status, source_kind, source_id,
      created_at, updated_at
    ) VALUES (
      p_activity_id, p_assistant_message_id,
      memory_next_activity_ordinal(p_assistant_message_id), p_user_id,
      v_activity_subject_type, v_activity_subject_id,
      CASE WHEN v_activity_subject_type = 'memory' THEN v_result_revision ELSE NULL END,
      v_activity_action, v_activity_status, v_result_code,
      v_undo_kind, v_undo_status, 'direct_action', p_action_id,
      v_now, v_now
    );
  END IF;

  RETURN QUERY SELECT
    p_action_id, v_status, v_result_code, v_result_memory_id,
    v_result_revision, p_proposed_scope_type,
    CASE WHEN v_status = 'noop' THEN NULL::UUID ELSE p_activity_id END;
END
$function$;

CREATE FUNCTION memory_record_message_usages(
  p_user_id UUID,
  p_conversation_id UUID,
  p_assistant_message_id UUID,
  p_usages JSONB
) RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_usage JSONB;
  v_count INTEGER := 0;
  v_existing_count INTEGER;
  v_ordinal SMALLINT;
  v_memory user_memories%ROWTYPE;
BEGIN
  IF jsonb_typeof(p_usages) <> 'array' OR jsonb_array_length(p_usages) > 5 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_USAGE_BATCH_INVALID';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM messages assistant
    WHERE assistant.id = p_assistant_message_id
      AND assistant.conversation_id = p_conversation_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.status = 'completed'
      AND assistant.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_USAGE_MESSAGE_INVALID';
  END IF;

  -- Usage is immutable answer provenance. Serialize retries for one assistant
  -- message, then accept only an exact replay of the already-recorded list.
  PERFORM pg_advisory_xact_lock(hashtextextended(p_assistant_message_id::TEXT, 0));
  SELECT count(*) INTO v_existing_count
  FROM message_memory_usages usage
  WHERE usage.assistant_message_id = p_assistant_message_id
    AND usage.user_id = p_user_id;

  FOR v_usage IN SELECT value FROM jsonb_array_elements(p_usages)
  LOOP
    v_count := v_count + 1;
    IF jsonb_typeof(v_usage) <> 'object'
      OR NOT (v_usage ?& ARRAY['memoryId', 'revision', 'scopeType'])
      OR (SELECT count(*) FROM jsonb_object_keys(v_usage)) <> 3
      OR v_usage->>'memoryId' !~
        '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$'
      OR v_usage->>'revision' !~ '^[1-9][0-9]*$'
      OR v_usage->>'scopeType' NOT IN ('global', 'project', 'conversation')
    THEN
      RAISE EXCEPTION USING
        ERRCODE = '22023', MESSAGE = 'MEMORY_USAGE_INVALID';
    END IF;
    v_ordinal := v_count::SMALLINT;

    IF v_existing_count > 0 THEN
      IF NOT EXISTS (
        SELECT 1 FROM message_memory_usages usage
        WHERE usage.assistant_message_id = p_assistant_message_id
          AND usage.ordinal = v_ordinal
          AND usage.user_id = p_user_id
          AND usage.entity_type = 'l1_memory'
          AND usage.entity_id = (v_usage->>'memoryId')::UUID
          AND usage.entity_revision = (v_usage->>'revision')::BIGINT
          AND usage.layer = 'l1'
          AND usage.scope_type = v_usage->>'scopeType'
          AND usage.purpose = 'answer_context'
      ) THEN
        RAISE EXCEPTION USING
          ERRCODE = '55000', MESSAGE = 'MEMORY_USAGE_REPLAY_CONFLICT';
      END IF;
      CONTINUE;
    END IF;

    SELECT memory.* INTO v_memory
    FROM user_memories memory
    JOIN user_memory_state state
      ON state.user_id = memory.user_id
      AND state.visibility_epoch = memory.visibility_epoch
    JOIN conversations conversation
      ON conversation.id = p_conversation_id
      AND conversation.user_id = memory.user_id
      AND conversation.deleted_at IS NULL
    LEFT JOIN projects project
      ON project.id = memory.project_id
      AND project.user_id = memory.user_id
    WHERE memory.id = (v_usage->>'memoryId')::UUID
      AND memory.user_id = p_user_id
      AND memory.revision = (v_usage->>'revision')::BIGINT
      AND memory.scope_type = v_usage->>'scopeType'
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND (
        (memory.scope_type = 'global' AND memory.scope_generation = 1)
        OR (
          memory.scope_type = 'project'
          AND memory.project_id = conversation.project_id
          AND project.lifecycle_status = 'active'
          AND project.deleted_at IS NULL
          AND project.scope_generation = memory.scope_generation
        )
        OR (
          memory.scope_type = 'conversation'
          AND memory.scope_conversation_id = conversation.id
          AND memory.scope_generation = conversation.memory_scope_generation
        )
      );
    IF NOT FOUND THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001', MESSAGE = 'MEMORY_USAGE_TARGET_INVALID';
    END IF;
    INSERT INTO message_memory_usages (
      assistant_message_id, ordinal, user_id, entity_type,
      entity_id, entity_revision, layer, scope_type, purpose
    ) VALUES (
      p_assistant_message_id, v_ordinal, p_user_id, 'l1_memory',
      v_memory.id, v_memory.revision, 'l1', v_memory.scope_type,
      'answer_context'
    );
  END LOOP;
  IF v_existing_count > 0 AND v_existing_count <> v_count THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000', MESSAGE = 'MEMORY_USAGE_REPLAY_CONFLICT';
  END IF;
  RETURN v_count;
END
$function$;

CREATE FUNCTION memory_list_activities(
  p_user_id UUID,
  p_cursor UUID,
  p_limit INTEGER
) RETURNS TABLE (
  id UUID,
  assistant_message_id UUID,
  ordinal SMALLINT,
  subject_type TEXT,
  subject_id UUID,
  subject_revision BIGINT,
  action TEXT,
  status TEXT,
  reason_code TEXT,
  undo_kind TEXT,
  undo_status TEXT,
  memory_type TEXT,
  memory_content TEXT,
  memory_revision BIGINT,
  memory_deleted BOOLEAN,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_cursor_created_at TIMESTAMPTZ;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 100 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_ACTIVITY_LIMIT_INVALID';
  END IF;
  IF p_cursor IS NOT NULL THEN
    SELECT activity.created_at INTO v_cursor_created_at
    FROM message_memory_activities activity
    WHERE activity.id = p_cursor AND activity.user_id = p_user_id;
    IF NOT FOUND THEN
      RAISE EXCEPTION USING
        ERRCODE = '22023', MESSAGE = 'MEMORY_ACTIVITY_CURSOR_INVALID';
    END IF;
  END IF;
  RETURN QUERY
  SELECT
    activity.id, activity.assistant_message_id, activity.ordinal,
    activity.subject_type, activity.subject_id, activity.subject_revision,
    activity.action, activity.status, activity.reason_code,
    activity.undo_kind, activity.undo_status,
    CASE WHEN activity.subject_type = 'memory' AND current_memory.is_current
      THEN memory.memory_type ELSE NULL END,
    CASE WHEN activity.subject_type = 'memory' AND current_memory.is_current
      THEN memory.content ELSE NULL END,
    CASE WHEN activity.subject_type = 'memory' AND current_memory.is_current
      THEN memory.revision ELSE NULL END,
    CASE WHEN activity.subject_type = 'memory'
      THEN NOT current_memory.is_current ELSE false END,
    activity.created_at, activity.updated_at
  FROM message_memory_activities activity
  LEFT JOIN user_memories memory
    ON activity.subject_type = 'memory'
    AND memory.id = activity.subject_id
    AND memory.user_id = activity.user_id
  LEFT JOIN user_memory_state state
    ON state.user_id = memory.user_id
    AND state.visibility_epoch = memory.visibility_epoch
  LEFT JOIN projects scoped_project
    ON memory.scope_type = 'project'
    AND scoped_project.id = memory.project_id
    AND scoped_project.user_id = memory.user_id
  LEFT JOIN conversations scoped_conversation
    ON memory.scope_type = 'conversation'
    AND scoped_conversation.id = memory.scope_conversation_id
    AND scoped_conversation.user_id = memory.user_id
  LEFT JOIN LATERAL (
    SELECT COALESCE(
      memory.id IS NOT NULL
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND state.user_id IS NOT NULL
      AND (
        (memory.scope_type = 'global' AND memory.scope_generation = 1)
        OR (
          memory.scope_type = 'project'
          AND scoped_project.id IS NOT NULL
          AND scoped_project.lifecycle_status = 'active'
          AND scoped_project.deleted_at IS NULL
          AND scoped_project.scope_generation = memory.scope_generation
        )
        OR (
          memory.scope_type = 'conversation'
          AND scoped_conversation.id IS NOT NULL
          AND scoped_conversation.deleted_at IS NULL
          AND scoped_conversation.memory_scope_generation = memory.scope_generation
        )
      ),
      false
    ) AS is_current
  ) current_memory ON true
  WHERE activity.user_id = p_user_id
    AND (
      p_cursor IS NULL
      OR activity.created_at > v_cursor_created_at
      OR (activity.created_at = v_cursor_created_at AND activity.id > p_cursor)
    )
  ORDER BY activity.created_at, activity.id
  LIMIT p_limit;
END
$function$;

CREATE FUNCTION memory_list_message_usages(
  p_user_id UUID,
  p_assistant_message_id UUID
) RETURNS TABLE (
  assistant_message_id UUID,
  ordinal SMALLINT,
  memory_id UUID,
  memory_revision BIGINT,
  scope_type TEXT,
  memory_type TEXT,
  memory_content TEXT,
  memory_deleted BOOLEAN,
  created_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM messages assistant
    WHERE assistant.id = p_assistant_message_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant'
      AND assistant.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_USAGE_MESSAGE_INVALID';
  END IF;
  RETURN QUERY
  SELECT
    usage.assistant_message_id, usage.ordinal, usage.entity_id,
    usage.entity_revision, usage.scope_type,
    CASE WHEN current_memory.is_current THEN memory.memory_type ELSE NULL END,
    CASE WHEN current_memory.is_current THEN memory.content ELSE NULL END,
    NOT current_memory.is_current,
    usage.created_at
  FROM message_memory_usages usage
  LEFT JOIN user_memories memory
    ON memory.id = usage.entity_id AND memory.user_id = usage.user_id
  LEFT JOIN user_memory_state state
    ON state.user_id = memory.user_id
    AND state.visibility_epoch = memory.visibility_epoch
  LEFT JOIN projects scoped_project
    ON memory.scope_type = 'project'
    AND scoped_project.id = memory.project_id
    AND scoped_project.user_id = memory.user_id
  LEFT JOIN conversations scoped_conversation
    ON memory.scope_type = 'conversation'
    AND scoped_conversation.id = memory.scope_conversation_id
    AND scoped_conversation.user_id = memory.user_id
  LEFT JOIN LATERAL (
    SELECT COALESCE(
      memory.id IS NOT NULL
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND state.user_id IS NOT NULL
      AND (
        (memory.scope_type = 'global' AND memory.scope_generation = 1)
        OR (
          memory.scope_type = 'project'
          AND scoped_project.id IS NOT NULL
          AND scoped_project.lifecycle_status = 'active'
          AND scoped_project.deleted_at IS NULL
          AND scoped_project.scope_generation = memory.scope_generation
        )
        OR (
          memory.scope_type = 'conversation'
          AND scoped_conversation.id IS NOT NULL
          AND scoped_conversation.deleted_at IS NULL
          AND scoped_conversation.memory_scope_generation = memory.scope_generation
        )
      ),
      false
    ) AS is_current
  ) current_memory ON true
  WHERE usage.user_id = p_user_id
    AND usage.assistant_message_id = p_assistant_message_id
  ORDER BY usage.ordinal;
END
$function$;

CREATE FUNCTION memory_undo_activity(
  p_user_id UUID,
  p_activity_id UUID,
  p_expected_revision BIGINT,
  p_event_id UUID,
  p_job_id UUID,
  p_tombstone_id UUID,
  p_manifest_id UUID
) RETURNS TABLE (
  undo_status TEXT,
  result_code TEXT,
  memory_id UUID,
  memory_revision BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_activity message_memory_activities%ROWTYPE;
  v_memory user_memories%ROWTYPE;
  v_change user_memory_revisions%ROWTYPE;
  v_exact user_memories%ROWTYPE;
  v_new_revision BIGINT;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT activity.* INTO v_activity
  FROM message_memory_activities activity
  WHERE activity.id = p_activity_id
    AND activity.user_id = p_user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_ACTIVITY_NOT_FOUND';
  END IF;
  IF v_activity.source_kind <> 'direct_action'
    OR v_activity.undo_kind NOT IN ('created', 'corrected')
    OR v_activity.undo_status <> 'available'
    OR v_activity.subject_type <> 'memory'
    OR v_activity.subject_revision IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_ACTIVITY_UNDO_UNAVAILABLE';
  END IF;
  IF p_expected_revision <> v_activity.subject_revision THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_ACTIVITY_REVISION_INVALID';
  END IF;

  SELECT memory.* INTO v_memory
  FROM user_memories memory
  WHERE memory.id = v_activity.subject_id
    AND memory.user_id = p_user_id
  FOR UPDATE;
  IF NOT FOUND OR v_memory.deleted_at IS NOT NULL
    OR v_memory.revision <> p_expected_revision
    OR NOT v_memory.enabled
    OR v_memory.lifecycle_status <> 'active'
    OR NOT EXISTS (
      SELECT 1 FROM user_memory_state state
      WHERE state.user_id = p_user_id
        AND state.visibility_epoch = v_memory.visibility_epoch
    )
    OR NOT (
      (v_memory.scope_type = 'global' AND v_memory.scope_generation = 1)
      OR (
        v_memory.scope_type = 'project'
        AND EXISTS (
          SELECT 1 FROM projects project
          WHERE project.id = v_memory.project_id
            AND project.user_id = p_user_id
            AND project.lifecycle_status = 'active'
            AND project.deleted_at IS NULL
            AND project.scope_generation = v_memory.scope_generation
        )
      )
      OR (
        v_memory.scope_type = 'conversation'
        AND EXISTS (
          SELECT 1 FROM conversations conversation
          WHERE conversation.id = v_memory.scope_conversation_id
            AND conversation.user_id = p_user_id
            AND conversation.deleted_at IS NULL
            AND conversation.memory_scope_generation = v_memory.scope_generation
        )
      )
    )
  THEN
    UPDATE message_memory_activities activity
    SET status = 'review_required', reason_code = 'UNDO_STALE',
        undo_status = 'review_required', updated_at = v_now
    WHERE activity.id = p_activity_id;
    RETURN QUERY SELECT
      'review_required'::TEXT, 'UNDO_STALE'::TEXT,
      v_activity.subject_id,
      CASE WHEN v_memory.id IS NULL THEN NULL::BIGINT ELSE v_memory.revision END;
    RETURN;
  END IF;

  IF v_activity.undo_kind = 'created' THEN
    IF NOT memory_delete_direct_scoped(
      p_user_id, v_memory.id, v_memory.revision,
      p_event_id, p_job_id, p_tombstone_id, p_manifest_id
    ) THEN
      UPDATE message_memory_activities activity
      SET status = 'review_required', reason_code = 'UNDO_STALE',
          undo_status = 'review_required', updated_at = v_now
      WHERE activity.id = p_activity_id;
      RETURN QUERY SELECT
        'review_required'::TEXT, 'UNDO_STALE'::TEXT,
        v_memory.id, v_memory.revision;
      RETURN;
    END IF;
    v_new_revision := v_memory.revision + 1;
  ELSE
    SELECT revision.* INTO v_change
    FROM user_memory_revisions revision
    WHERE revision.memory_id = v_memory.id
      AND revision.user_id = p_user_id
      AND revision.revision = v_activity.subject_revision
      AND revision.prior_snapshot_schema_major = 1
      AND revision.purged_at IS NULL;
    IF NOT FOUND
      OR (v_change.prior_source_conversation_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM conversations conversation
        WHERE conversation.id = v_change.prior_source_conversation_id
          AND conversation.user_id = p_user_id
          AND conversation.deleted_at IS NULL
      ))
      OR (v_change.prior_source_message_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM messages message
        WHERE message.id = v_change.prior_source_message_id
          AND message.user_id = p_user_id
          AND message.conversation_id = v_change.prior_source_conversation_id
          AND message.role = 'user'
          AND message.status = 'completed'
          AND message.deleted_at IS NULL
      ))
    THEN
      UPDATE message_memory_activities activity
      SET status = 'review_required', reason_code = 'UNDO_SNAPSHOT_UNAVAILABLE',
          undo_status = 'review_required', updated_at = v_now
      WHERE activity.id = p_activity_id;
      RETURN QUERY SELECT
        'review_required'::TEXT, 'UNDO_SNAPSHOT_UNAVAILABLE'::TEXT,
        v_memory.id, v_memory.revision;
      RETURN;
    END IF;

    SELECT memory.* INTO v_exact
    FROM user_memories memory
    WHERE memory.user_id = p_user_id
      AND memory.id <> v_memory.id
      AND memory.normalized_content = v_change.prior_normalized_content
      AND memory.scope_type = v_change.prior_scope_type
      AND (memory.scope_type <> 'project'
        OR memory.project_id = v_change.prior_project_id)
      AND (memory.scope_type <> 'conversation'
        OR memory.scope_conversation_id = v_change.prior_scope_conversation_id)
      AND memory.deleted_at IS NULL
    FOR UPDATE;
    IF FOUND THEN
      UPDATE message_memory_activities activity
      SET status = 'review_required', reason_code = 'UNDO_EXACT_CONFLICT',
          undo_status = 'review_required', updated_at = v_now
      WHERE activity.id = p_activity_id;
      RETURN QUERY SELECT
        'review_required'::TEXT, 'UNDO_EXACT_CONFLICT'::TEXT,
        v_memory.id, v_memory.revision;
      RETURN;
    END IF;

    v_new_revision := v_memory.revision + 1;
    INSERT INTO user_memory_revisions (
      memory_id, revision, user_id, operation, old_content_hash,
      new_content_hash, prior_content_snapshot, actor_type,
      prior_snapshot_schema_major, prior_memory_type,
      prior_normalized_content, prior_importance, prior_tags, prior_source,
      prior_source_conversation_id, prior_source_message_id, prior_enabled,
      prior_last_used_at, prior_scope_type, prior_project_id,
      prior_scope_conversation_id, prior_scope_generation,
      prior_visibility_epoch, prior_authority_kind,
      prior_extraction_profile_id, prior_lifecycle_status,
      prior_subject_key, prior_fact_key, prior_confidence,
      prior_observed_at, prior_valid_from, prior_valid_to, prior_expires_at,
      prior_superseded_by_memory_id, prior_sensitivity,
      prior_temporal_basis, prior_temporal_parser_version
    ) VALUES (
      v_memory.id, v_new_revision, p_user_id, 'restore',
      v_memory.content_hash, v_change.old_content_hash,
      v_memory.content, 'user', 1,
      v_memory.memory_type, v_memory.normalized_content,
      v_memory.importance, v_memory.tags, v_memory.source,
      v_memory.source_conversation_id, v_memory.source_message_id,
      v_memory.enabled, v_memory.last_used_at, v_memory.scope_type,
      v_memory.project_id, v_memory.scope_conversation_id,
      v_memory.scope_generation, v_memory.visibility_epoch,
      v_memory.authority_kind, v_memory.extraction_profile_id,
      v_memory.lifecycle_status, v_memory.subject_key, v_memory.fact_key,
      v_memory.confidence, v_memory.observed_at, v_memory.valid_from,
      v_memory.valid_to, v_memory.expires_at,
      v_memory.superseded_by_memory_id, v_memory.sensitivity,
      v_memory.temporal_basis, v_memory.temporal_parser_version
    );
    UPDATE user_memories memory
    SET memory_type = v_change.prior_memory_type,
        content = v_change.prior_content_snapshot,
        normalized_content = v_change.prior_normalized_content,
        importance = v_change.prior_importance,
        tags = v_change.prior_tags,
        source = v_change.prior_source,
        source_conversation_id = v_change.prior_source_conversation_id,
        source_message_id = v_change.prior_source_message_id,
        enabled = v_change.prior_enabled,
        last_used_at = v_change.prior_last_used_at,
        scope_type = v_change.prior_scope_type,
        project_id = v_change.prior_project_id,
        scope_conversation_id = v_change.prior_scope_conversation_id,
        scope_generation = v_change.prior_scope_generation,
        visibility_epoch = v_change.prior_visibility_epoch,
        authority_kind = v_change.prior_authority_kind,
        extraction_profile_id = v_change.prior_extraction_profile_id,
        lifecycle_status = v_change.prior_lifecycle_status,
        subject_key = v_change.prior_subject_key,
        fact_key = v_change.prior_fact_key,
        confidence = v_change.prior_confidence,
        observed_at = v_change.prior_observed_at,
        valid_from = v_change.prior_valid_from,
        valid_to = v_change.prior_valid_to,
        expires_at = v_change.prior_expires_at,
        superseded_by_memory_id = v_change.prior_superseded_by_memory_id,
        sensitivity = v_change.prior_sensitivity,
        temporal_basis = v_change.prior_temporal_basis,
        temporal_parser_version = v_change.prior_temporal_parser_version,
        revision = v_new_revision,
        content_hash = v_change.old_content_hash,
        updated_at = v_now
    WHERE memory.id = v_memory.id;
  END IF;

  UPDATE message_memory_activities activity
  SET status = 'undone', reason_code = 'UNDO_APPLIED',
      undo_status = 'undone', updated_at = v_now
  WHERE activity.id = p_activity_id;
  RETURN QUERY SELECT
    'undone'::TEXT, 'UNDO_APPLIED'::TEXT,
    v_memory.id, v_new_revision;
END
$function$;

CREATE FUNCTION memory_review_activity_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_assistant_message_id UUID;
  v_action TEXT;
  v_status TEXT;
BEGIN
  IF NEW.status NOT IN ('pending', 'rejected') THEN
    RETURN NEW;
  END IF;
  SELECT job.assistant_message_id INTO v_assistant_message_id
  FROM memory_jobs job
  WHERE job.job_id = NEW.capture_job_id
    AND job.user_id = NEW.user_id;
  IF v_assistant_message_id IS NULL THEN
    RETURN NEW;
  END IF;
  IF NEW.status = 'pending' THEN
    v_action := 'review_required';
    v_status := 'pending';
  ELSE
    v_action := 'rejected';
    v_status := 'completed';
  END IF;
  INSERT INTO message_memory_activities (
    id, assistant_message_id, ordinal, user_id,
    subject_type, subject_id, action, status, reason_code,
    source_kind, source_id
  ) VALUES (
    gen_random_uuid(), v_assistant_message_id,
    memory_next_activity_ordinal(v_assistant_message_id), NEW.user_id,
    'review_suggestion', NEW.id, v_action, v_status,
    NEW.decision_reason_code, 'review_suggestion', NEW.id
  ) ON CONFLICT (source_kind, source_id) DO NOTHING;
  RETURN NEW;
END
$function$;

CREATE TRIGGER user_memory_review_suggestions_activity
AFTER INSERT ON user_memory_review_suggestions
FOR EACH ROW EXECUTE FUNCTION memory_review_activity_trigger();

CREATE FUNCTION memory_dead_letter_activity_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NEW.status = 'dead_letter'
    AND OLD.status IS DISTINCT FROM NEW.status
    AND NEW.assistant_message_id IS NOT NULL
  THEN
    INSERT INTO message_memory_activities (
      id, assistant_message_id, ordinal, user_id,
      subject_type, subject_id, action, status, reason_code,
      source_kind, source_id
    ) VALUES (
      gen_random_uuid(), NEW.assistant_message_id,
      memory_next_activity_ordinal(NEW.assistant_message_id), NEW.user_id,
      'job', NEW.job_id, 'failed', 'failed', NEW.error_code,
      'memory_job', NEW.job_id
    ) ON CONFLICT (source_kind, source_id) DO NOTHING;
  END IF;
  RETURN NEW;
END
$function$;

CREATE TRIGGER memory_jobs_dead_letter_activity
AFTER UPDATE OF status ON memory_jobs
FOR EACH ROW EXECUTE FUNCTION memory_dead_letter_activity_trigger();

-- Backfill proposal/dead-letter links already present when PR6 is deployed.
WITH suggestion_rows AS (
  SELECT
    suggestion.id,
    suggestion.user_id,
    job.assistant_message_id,
    suggestion.status,
    suggestion.decision_reason_code,
    row_number() OVER (
      PARTITION BY job.assistant_message_id
      ORDER BY suggestion.created_at, suggestion.id
    )::SMALLINT AS ordinal
  FROM user_memory_review_suggestions suggestion
  JOIN memory_jobs job
    ON job.job_id = suggestion.capture_job_id
    AND job.user_id = suggestion.user_id
  WHERE suggestion.status IN ('pending', 'rejected')
    AND job.assistant_message_id IS NOT NULL
)
INSERT INTO message_memory_activities (
  id, assistant_message_id, ordinal, user_id,
  subject_type, subject_id, action, status, reason_code,
  source_kind, source_id, created_at, updated_at
)
SELECT
  gen_random_uuid(), row.assistant_message_id, row.ordinal, row.user_id,
  'review_suggestion', row.id,
  CASE WHEN row.status = 'pending' THEN 'review_required' ELSE 'rejected' END,
  CASE WHEN row.status = 'pending' THEN 'pending' ELSE 'completed' END,
  row.decision_reason_code, 'review_suggestion', row.id, now(), now()
FROM suggestion_rows row;

WITH job_rows AS (
  SELECT
    job.job_id,
    job.user_id,
    job.assistant_message_id,
    job.error_code,
    COALESCE(existing.max_ordinal, 0) + row_number() OVER (
      PARTITION BY job.assistant_message_id
      ORDER BY job.completed_at, job.job_id
    ) AS ordinal
  FROM memory_jobs job
  LEFT JOIN LATERAL (
    SELECT max(activity.ordinal) AS max_ordinal
    FROM message_memory_activities activity
    WHERE activity.assistant_message_id = job.assistant_message_id
  ) existing ON true
  WHERE job.status = 'dead_letter'
    AND job.assistant_message_id IS NOT NULL
)
INSERT INTO message_memory_activities (
  id, assistant_message_id, ordinal, user_id,
  subject_type, subject_id, action, status, reason_code,
  source_kind, source_id, created_at, updated_at
)
SELECT
  gen_random_uuid(), row.assistant_message_id, row.ordinal::SMALLINT,
  row.user_id, 'job', row.job_id, 'failed', 'failed', row.error_code,
  'memory_job', row.job_id, now(), now()
FROM job_rows row;

CREATE OR REPLACE FUNCTION memory_worker_purge_memory(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_memory user_memories%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT job.* INTO v_job
  FROM memory_jobs job
  JOIN memory_outbox outbox
    ON outbox.event_id = job.event_id
    AND outbox.user_id = job.user_id
    AND outbox.event_type = 'memory.deleted'
    AND outbox.status = 'processing'
    AND outbox.lease_owner = p_worker_id
    AND outbox.lease_token = p_lease_token
    AND outbox.lease_expires_at > v_now
    AND outbox.visibility_epoch = job.visibility_epoch
  WHERE job.job_id = p_job_id
    AND job.stage = 'purge'
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  PERFORM 1 FROM user_memory_state state
  WHERE state.user_id = v_job.user_id
    AND state.visibility_epoch = v_job.visibility_epoch
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_VISIBILITY_EPOCH_DRIFT';
  END IF;

  SELECT memory.* INTO v_memory
  FROM user_memories memory
  JOIN user_memory_tombstones tombstone
    ON tombstone.id = v_job.target_tombstone_id
    AND tombstone.memory_id = memory.id
    AND tombstone.user_id = memory.user_id
    AND tombstone.content_hash = memory.content_hash
  WHERE memory.id = v_job.target_memory_id
    AND memory.user_id = v_job.user_id
    AND memory.scope_generation = v_job.scope_generation
    AND memory.deleted_at IS NOT NULL
  FOR UPDATE OF memory;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_PURGE_TARGET_DRIFT';
  END IF;

  DELETE FROM user_memory_evidence evidence
  WHERE evidence.memory_id = v_memory.id
    AND evidence.user_id = v_job.user_id;

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
      purged_at = v_now
  WHERE revision.memory_id = v_memory.id
    AND revision.user_id = v_job.user_id
    AND revision.prior_content_snapshot IS NOT NULL
    AND revision.purged_at IS NULL;

  UPDATE user_memories memory
  SET content = '', normalized_content = '', tags = '{}',
      source_conversation_id = NULL, source_message_id = NULL,
      extraction_profile_id = NULL,
      subject_key = NULL, fact_key = NULL,
      temporal_parser_version = NULL,
      updated_at = GREATEST(memory.updated_at, v_now)
  WHERE memory.id = v_memory.id
    AND (
      memory.content <> '' OR memory.normalized_content <> ''
      OR cardinality(memory.tags) > 0
      OR memory.source_conversation_id IS NOT NULL
      OR memory.source_message_id IS NOT NULL
      OR memory.extraction_profile_id IS NOT NULL
      OR memory.subject_key IS NOT NULL
      OR memory.fact_key IS NOT NULL
      OR memory.temporal_parser_version IS NOT NULL
    );

  UPDATE user_memory_deletion_manifests manifest
  SET result_code = 'ONLINE_PURGED', purged_at = v_now
  WHERE manifest.event_id = v_job.event_id
    AND manifest.user_id = v_job.user_id
    AND manifest.memory_id = v_memory.id
    AND manifest.tombstone_id = v_job.target_tombstone_id
    AND manifest.result_code = 'PENDING';
  RETURN true;
END
$function$;

DO $harden_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'user_memory_revision_append_only_guard()',
    'memory_next_activity_ordinal(uuid)',
    'memory_hydrate_direct_user_action(uuid,uuid,uuid,uuid)',
    'memory_delete_direct_scoped(uuid,uuid,bigint,uuid,uuid,uuid,uuid)',
    'memory_apply_direct_user_action(uuid,uuid,uuid,uuid,uuid,uuid,uuid,uuid,uuid,uuid,uuid,smallint,text,text,text,text,text,smallint,text[],text,text,double precision,jsonb,text,text)',
    'memory_record_message_usages(uuid,uuid,uuid,jsonb)',
    'memory_list_activities(uuid,uuid,integer)',
    'memory_list_message_usages(uuid,uuid)',
    'memory_undo_activity(uuid,uuid,bigint,uuid,uuid,uuid,uuid)',
    'memory_review_activity_trigger()',
    'memory_dead_letter_activity_trigger()',
    'memory_worker_purge_memory(uuid,uuid,uuid)'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name, function_identity, schema_name
    );
  END LOOP;
END
$harden_functions$;

GRANT SELECT, INSERT, UPDATE ON
  memory_user_actions,
  memory_user_action_targets,
  message_memory_activities,
  message_memory_usages
TO memory_runtime_owner;

ALTER FUNCTION memory_next_activity_ordinal(UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_hydrate_direct_user_action(UUID, UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_delete_direct_scoped(
  UUID, UUID, BIGINT, UUID, UUID, UUID, UUID
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_apply_direct_user_action(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID,
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, TEXT, TEXT,
  TEXT, SMALLINT, TEXT[], TEXT, TEXT, DOUBLE PRECISION, JSONB, TEXT, TEXT
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_record_message_usages(UUID, UUID, UUID, JSONB)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_list_activities(UUID, UUID, INTEGER)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_list_message_usages(UUID, UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_undo_activity(
  UUID, UUID, BIGINT, UUID, UUID, UUID, UUID
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_review_activity_trigger()
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_dead_letter_activity_trigger()
  OWNER TO memory_runtime_owner;

REVOKE ALL ON
  memory_user_actions,
  memory_user_action_targets,
  message_memory_activities,
  message_memory_usages
FROM PUBLIC, go_api_runtime, memory_worker_runtime;

REVOKE ALL ON FUNCTION memory_next_activity_ordinal(UUID) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_hydrate_direct_user_action(
  UUID, UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_delete_direct_scoped(
  UUID, UUID, BIGINT, UUID, UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_apply_direct_user_action(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID,
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, TEXT, TEXT,
  TEXT, SMALLINT, TEXT[], TEXT, TEXT, DOUBLE PRECISION, JSONB, TEXT, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_record_message_usages(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_list_activities(
  UUID, UUID, INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_list_message_usages(UUID, UUID) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_undo_activity(
  UUID, UUID, BIGINT, UUID, UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_review_activity_trigger() FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_dead_letter_activity_trigger() FROM PUBLIC;

GRANT EXECUTE ON FUNCTION memory_hydrate_direct_user_action(
  UUID, UUID, UUID, UUID
), memory_apply_direct_user_action(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID,
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, TEXT, TEXT,
  TEXT, SMALLINT, TEXT[], TEXT, TEXT, DOUBLE PRECISION, JSONB, TEXT, TEXT
), memory_record_message_usages(
  UUID, UUID, UUID, JSONB
), memory_list_activities(
  UUID, UUID, INTEGER
), memory_list_message_usages(
  UUID, UUID
), memory_undo_activity(
  UUID, UUID, BIGINT, UUID, UUID, UUID, UUID
) TO go_api_runtime;

DO $owner_create_revocation$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner',
    current_schema()
  );
END
$owner_create_revocation$;
