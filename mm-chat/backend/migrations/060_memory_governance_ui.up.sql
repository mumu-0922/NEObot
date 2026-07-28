-- Memory v2 PR9 governance capabilities. The v1 Global reader remains the
-- only prompt authority; scoped Memory is user-governable but not promoted.

ALTER TABLE user_memory_revisions
  DROP CONSTRAINT user_memory_revisions_operation_allowed,
  ADD CONSTRAINT user_memory_revisions_operation_allowed
    CHECK (operation IN (
      'update', 'merge', 'supersede', 'delete', 'restore', 'move'
    ));

ALTER TABLE user_memory_review_suggestions
  ADD COLUMN decision_kind TEXT,
  ADD COLUMN result_memory_id UUID,
  ADD CONSTRAINT user_memory_review_suggestions_decision_kind_allowed CHECK (
    decision_kind IS NULL OR decision_kind IN (
      'keep_current', 'accept_new', 'edit_merge', 'keep_both', 'reject'
    )
  ),
  ADD CONSTRAINT user_memory_review_suggestions_result_memory_owner_fk
    FOREIGN KEY (result_memory_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE RESTRICT;

ALTER TABLE user_memory_review_suggestions
  DROP CONSTRAINT user_memory_review_suggestions_status_allowed,
  DROP CONSTRAINT user_memory_review_suggestions_plaintext_shape,
  DROP CONSTRAINT user_memory_review_suggestions_state_shape,
  ADD CONSTRAINT user_memory_review_suggestions_status_allowed
    CHECK (status IN ('shadow', 'pending', 'accepted', 'rejected', 'expired')),
  ADD CONSTRAINT user_memory_review_suggestions_plaintext_shape CHECK (
    (
      status IN ('shadow', 'pending')
      AND candidate_content IS NOT NULL
      AND length(trim(candidate_content)) > 0
      AND char_length(candidate_content) <= 2000
      AND normalized_content IS NOT NULL
      AND length(trim(normalized_content)) > 0
      AND char_length(normalized_content) <= 2000
      AND purged_at IS NULL
      AND result_code IS NULL
      AND decision_kind IS NULL
      AND result_memory_id IS NULL
    )
    OR (
      status IN ('accepted', 'rejected', 'expired')
      AND candidate_content IS NULL
      AND normalized_content IS NULL
      AND tags = '{}'::TEXT[]
      AND subject_key IS NULL
      AND fact_key IS NULL
      AND purged_at IS NOT NULL
      AND result_code IN (
        'SECRET_REJECTED', 'SENSITIVE_DISABLED', 'MODEL_REJECTED',
        'TOMBSTONED', 'PLAINTEXT_EXPIRED', 'USER_KEPT_CURRENT',
        'USER_REJECTED', 'USER_ACCEPTED', 'USER_EDIT_MERGED',
        'USER_KEPT_BOTH'
      )
    )
  ),
  ADD CONSTRAINT user_memory_review_suggestions_state_shape CHECK (
    (status = 'shadow' AND disposition = 'shadow' AND decided_at IS NULL
      AND decision_kind IS NULL AND result_memory_id IS NULL)
    OR (status = 'pending' AND disposition = 'review' AND decided_at IS NULL
      AND decision_kind IS NULL AND result_memory_id IS NULL)
    OR (status = 'accepted' AND disposition = 'review' AND decided_at IS NOT NULL
      AND decision_kind IN ('accept_new', 'edit_merge', 'keep_both')
      AND result_memory_id IS NOT NULL)
    OR (status = 'rejected' AND disposition = 'rejected' AND decided_at IS NOT NULL
      AND decision_kind IN ('keep_current', 'reject')
      AND result_memory_id IS NULL)
    OR (status = 'expired' AND disposition IN ('shadow', 'review')
      AND decided_at IS NOT NULL AND decision_kind IS NULL
      AND result_memory_id IS NULL)
  );

CREATE TABLE user_memory_review_decisions (
  id UUID PRIMARY KEY,
  suggestion_id UUID NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  decision_kind TEXT NOT NULL,
  decision_hash TEXT NOT NULL,
  result_code TEXT NOT NULL,
  result_memory_id UUID,
  result_memory_revision BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_review_decisions_suggestion_unique
    UNIQUE (suggestion_id),
  CONSTRAINT user_memory_review_decisions_suggestion_owner_fk
    FOREIGN KEY (suggestion_id, user_id)
    REFERENCES user_memory_review_suggestions(id, user_id) ON DELETE CASCADE,
  CONSTRAINT user_memory_review_decisions_result_memory_owner_fk
    FOREIGN KEY (result_memory_id, user_id)
    REFERENCES user_memories(id, user_id) ON DELETE RESTRICT,
  CONSTRAINT user_memory_review_decisions_kind_allowed CHECK (
    decision_kind IN (
      'keep_current', 'accept_new', 'edit_merge', 'keep_both', 'reject'
    )
  ),
  CONSTRAINT user_memory_review_decisions_hash_check
    CHECK (decision_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT user_memory_review_decisions_result_sanitized
    CHECK (result_code ~ '^[A-Z0-9_]{1,64}$'),
  CONSTRAINT user_memory_review_decisions_result_shape CHECK (
    (decision_kind IN ('keep_current', 'reject')
      AND result_memory_id IS NULL AND result_memory_revision IS NULL)
    OR (decision_kind IN ('accept_new', 'edit_merge', 'keep_both')
      AND result_memory_id IS NOT NULL AND result_memory_revision >= 1)
  )
);

CREATE INDEX idx_user_memory_review_decisions_user_created
  ON user_memory_review_decisions(user_id, created_at DESC, id);

CREATE FUNCTION memory_governance_epoch_millis(p_value TIMESTAMPTZ)
RETURNS BIGINT
LANGUAGE sql
IMMUTABLE
STRICT
SET search_path FROM CURRENT
AS $function$
  SELECT floor(extract(epoch FROM p_value) * 1000)::BIGINT
$function$;

CREATE FUNCTION memory_governance_is_secret(p_value TEXT)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
STRICT
SET search_path FROM CURRENT
AS $function$
  SELECT
    p_value ~* $assignment$(api[ _-]?(key|token)|password|passwd|access[ _-]?token|refresh[ _-]?token|auth[ _-]?token|session[ _-]?(id|token)|token|client[ _-]?secret|secret|credentials?|otp|recovery[ _-]?code|cookies?|private[ _-]?key|cvv|cvc|密码|口令|令牌|验证码|恢复码|私钥|密钥|凭证|安全码)\s*(is|=|:|：|是|为)\s*["']?[^\s,，;；]{4,}$assignment$
    OR p_value ~* $token$(sk-[a-z0-9_-]{8,}|(eyJ[a-zA-Z0-9_-]{8,}\.){2}[a-zA-Z0-9_-]{8,}|authorization\s*:\s*bearer\s+\S+|-----begin [a-z ]+private key-----)$token$
$function$;

CREATE FUNCTION memory_governance_classify_sensitivity(p_value TEXT)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
STRICT
SET search_path FROM CURRENT
AS $function$
  SELECT CASE
    WHEN memory_governance_is_secret(p_value) THEN 'secret'
    WHEN p_value ~* $sensitive$(diagnos(is|ed)|disease|cancer|diabetes|salary|income|debt|religion|religious|politic(s|al)|sexual orientation|lawsuit|arrested|home address|exact location|诊断|疾病|癌症|糖尿病|工资|收入|负债|宗教|政治观点|性取向|诉讼|被捕|家庭住址|精确位置|住在[^，。！？[:space:]]{2,})$sensitive$
      THEN 'sensitive'
    ELSE 'normal'
  END
$function$;

UPDATE user_memories
SET sensitivity = 'sensitive', updated_at = clock_timestamp()
WHERE sensitivity = 'normal'
  AND memory_governance_classify_sensitivity(content) IN ('sensitive', 'secret');

CREATE FUNCTION memory_governance_scope_generation(
  p_user_id UUID,
  p_scope_type TEXT,
  p_project_id UUID,
  p_conversation_id UUID
) RETURNS BIGINT
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_generation BIGINT;
BEGIN
  IF p_scope_type = 'global'
    AND p_project_id IS NULL AND p_conversation_id IS NULL
  THEN
    RETURN 1;
  ELSIF p_scope_type = 'project'
    AND p_project_id IS NOT NULL AND p_conversation_id IS NULL
  THEN
    SELECT project.scope_generation INTO v_generation
    FROM projects project
    WHERE project.id = p_project_id AND project.user_id = p_user_id
      AND project.deleted_at IS NULL AND project.lifecycle_status = 'active';
  ELSIF p_scope_type = 'conversation'
    AND p_project_id IS NULL AND p_conversation_id IS NOT NULL
  THEN
    SELECT conversation.memory_scope_generation INTO v_generation
    FROM conversations conversation
    WHERE conversation.id = p_conversation_id
      AND conversation.user_id = p_user_id
      AND conversation.deleted_at IS NULL;
  ELSE
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_SCOPE_STALE';
  END IF;
  IF v_generation IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_SCOPE_STALE';
  END IF;
  RETURN v_generation;
END
$function$;

CREATE FUNCTION memory_governance_append_revision(
  p_memory user_memories,
  p_new_content_hash TEXT,
  p_operation TEXT
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
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
    p_memory.id, p_memory.revision + 1, p_memory.user_id, p_operation,
    p_memory.content_hash, p_new_content_hash, p_memory.content, 'user',
    1, p_memory.memory_type, p_memory.normalized_content,
    p_memory.importance, p_memory.tags, p_memory.source,
    p_memory.source_conversation_id, p_memory.source_message_id,
    p_memory.enabled, p_memory.last_used_at, p_memory.scope_type,
    p_memory.project_id, p_memory.scope_conversation_id,
    p_memory.scope_generation, p_memory.visibility_epoch,
    p_memory.authority_kind, p_memory.extraction_profile_id,
    p_memory.lifecycle_status, p_memory.subject_key, p_memory.fact_key,
    p_memory.confidence, p_memory.observed_at, p_memory.valid_from,
    p_memory.valid_to, p_memory.expires_at,
    p_memory.superseded_by_memory_id, p_memory.sensitivity,
    p_memory.temporal_basis, p_memory.temporal_parser_version
  );
END
$function$;

CREATE FUNCTION memory_governance_project_json(
  p_user_id UUID,
  p_project_id UUID
) RETURNS JSONB
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT jsonb_build_object(
    'id', project.id::TEXT,
    'name', project.name,
    'description', project.description,
    'lifecycleStatus', project.lifecycle_status,
    'revision', project.revision,
    'scopeGeneration', project.scope_generation,
    'conversationCount', (
      SELECT count(*) FROM conversations conversation
      WHERE conversation.user_id = p_user_id
        AND conversation.project_id = project.id
        AND conversation.deleted_at IS NULL
    ),
    'memoryCount', (
      SELECT count(*) FROM user_memories memory
      WHERE memory.user_id = p_user_id AND memory.project_id = project.id
        AND memory.deleted_at IS NULL
    ),
    'createdAt', memory_governance_epoch_millis(project.created_at),
    'updatedAt', memory_governance_epoch_millis(project.updated_at),
    'archivedAt', CASE WHEN project.archived_at IS NULL THEN NULL
      ELSE memory_governance_epoch_millis(project.archived_at) END
  )
  FROM projects project
  WHERE project.id = p_project_id AND project.user_id = p_user_id
    AND project.deleted_at IS NULL
$function$;

CREATE FUNCTION memory_governance_policy_json(
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
    'projectName', COALESCE(project.name, ''),
    'projectStatus', COALESCE(project.lifecycle_status, ''),
    'useMode', conversation.memory_use_mode,
    'learnMode', conversation.memory_learn_mode,
    'effectiveUse', COALESCE(settings.enabled, false) AND CASE conversation.memory_use_mode
      WHEN 'on' THEN true WHEN 'off' THEN false
      ELSE COALESCE(settings.search_enabled, true) END,
    'effectiveLearn', COALESCE(settings.enabled, false)
      AND CASE conversation.memory_learn_mode
        WHEN 'on' THEN true WHEN 'off' THEN false
        ELSE COALESCE(settings.auto_record_enabled, false) END
      AND (project.id IS NULL OR project.lifecycle_status = 'active'),
    'learnForcedOff', project.id IS NOT NULL AND project.lifecycle_status = 'archived',
    'scopeGeneration', conversation.memory_scope_generation,
    'updatedAt', memory_governance_epoch_millis(conversation.updated_at)
  )
  FROM conversations conversation
  LEFT JOIN projects project
    ON project.id = conversation.project_id AND project.user_id = p_user_id
      AND project.deleted_at IS NULL
  LEFT JOIN user_memory_settings settings ON settings.user_id = p_user_id
  WHERE conversation.id = p_conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL
$function$;

CREATE FUNCTION memory_governance_memory_json(
  p_user_id UUID,
  p_memory_id UUID
) RETURNS JSONB
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT jsonb_build_object(
    'id', memory.id::TEXT,
    'type', memory.memory_type,
    'content', memory.content,
    'importance', memory.importance,
    'tags', to_jsonb(memory.tags),
    'source', memory.source,
    'authorityKind', memory.authority_kind,
    'enabled', memory.enabled,
    'revision', memory.revision,
    'scopeType', memory.scope_type,
    'projectId', COALESCE(memory.project_id::TEXT, ''),
    'projectName', COALESCE(project.name, ''),
    'conversationId', COALESCE(memory.scope_conversation_id::TEXT, ''),
    'conversationTitle', COALESCE(conversation.title, ''),
    'lifecycleStatus', memory.lifecycle_status,
    'sensitivity', CASE
      WHEN memory_governance_classify_sensitivity(memory.content) IN ('sensitive', 'secret')
        THEN 'sensitive'
      ELSE memory.sensitivity
    END,
    'recallStatus', CASE
      WHEN NOT memory.enabled THEN 'disabled'
      WHEN memory.lifecycle_status <> 'active' THEN memory.lifecycle_status
      WHEN memory.scope_type = 'global' THEN 'v1_active'
      WHEN memory.scope_type = 'project'
        AND project.lifecycle_status = 'active'
        AND project.scope_generation = memory.scope_generation THEN 'shadow_only'
      WHEN memory.scope_type = 'project'
        AND project.lifecycle_status = 'archived' THEN 'project_archived'
      WHEN memory.scope_type = 'conversation'
        AND conversation.memory_scope_generation = memory.scope_generation THEN 'shadow_only'
      ELSE 'scope_stale'
    END,
    'validFrom', CASE WHEN memory.valid_from IS NULL THEN NULL
      ELSE memory_governance_epoch_millis(memory.valid_from) END,
    'validTo', CASE WHEN memory.valid_to IS NULL THEN NULL
      ELSE memory_governance_epoch_millis(memory.valid_to) END,
    'expiresAt', CASE WHEN memory.expires_at IS NULL THEN NULL
      ELSE memory_governance_epoch_millis(memory.expires_at) END,
    'supersededByMemoryId', COALESCE(memory.superseded_by_memory_id::TEXT, ''),
    'lastUsedAt', CASE WHEN memory.last_used_at IS NULL THEN NULL
      ELSE memory_governance_epoch_millis(memory.last_used_at) END,
    'createdAt', memory_governance_epoch_millis(memory.created_at),
    'updatedAt', memory_governance_epoch_millis(memory.updated_at)
  )
  FROM user_memories memory
  LEFT JOIN projects project
    ON project.id = memory.project_id AND project.user_id = p_user_id
      AND project.deleted_at IS NULL
  LEFT JOIN conversations conversation
    ON conversation.id = memory.scope_conversation_id
      AND conversation.user_id = p_user_id
      AND conversation.deleted_at IS NULL
  WHERE memory.id = p_memory_id AND memory.user_id = p_user_id
    AND memory.deleted_at IS NULL
$function$;

CREATE FUNCTION memory_governance_upsert_global_legacy(
  p_memory_id UUID,
  p_user_id UUID,
  p_memory_type TEXT,
  p_content TEXT,
  p_normalized_content TEXT,
  p_importance SMALLINT,
  p_tags TEXT[],
  p_source_conversation_id UUID,
  p_source_message_id UUID,
  p_enabled BOOLEAN
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory_id UUID;
  v_sensitivity TEXT;
BEGIN
  v_sensitivity := memory_governance_classify_sensitivity(p_content);
  IF v_sensitivity = 'secret' THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_MEMORY_INVALID';
  END IF;
  IF v_sensitivity = 'sensitive' AND NOT EXISTS (
    SELECT 1 FROM user_memory_settings settings
    WHERE settings.user_id = p_user_id AND settings.sensitive_memory_enabled
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_SENSITIVE_DISABLED';
  END IF;
  SELECT legacy.id INTO v_memory_id
  FROM memory_upsert_global_manual(
    p_memory_id, p_user_id, p_memory_type, p_content,
    p_normalized_content, p_importance, p_tags,
    p_source_conversation_id, p_source_message_id, p_enabled
  ) legacy;
  UPDATE user_memories memory
  SET sensitivity = v_sensitivity, updated_at = clock_timestamp()
  WHERE memory.id = v_memory_id AND memory.user_id = p_user_id
    AND memory.sensitivity <> v_sensitivity;
  RETURN memory_governance_memory_json(p_user_id, v_memory_id);
END
$function$;

CREATE FUNCTION memory_governance_update_global_legacy(
  p_memory_id UUID,
  p_user_id UUID,
  p_memory_type TEXT,
  p_content TEXT,
  p_normalized_content TEXT,
  p_importance SMALLINT,
  p_tags TEXT[],
  p_enabled BOOLEAN
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory_id UUID;
  v_sensitivity TEXT;
BEGIN
  v_sensitivity := memory_governance_classify_sensitivity(p_content);
  IF v_sensitivity = 'secret' THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_MEMORY_INVALID';
  END IF;
  IF v_sensitivity = 'sensitive' AND NOT EXISTS (
    SELECT 1 FROM user_memory_settings settings
    WHERE settings.user_id = p_user_id AND settings.sensitive_memory_enabled
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_SENSITIVE_DISABLED';
  END IF;
  SELECT legacy.id INTO v_memory_id
  FROM memory_update_global_manual(
    p_memory_id, p_user_id, p_memory_type, p_content,
    p_normalized_content, p_importance, p_tags, p_enabled
  ) legacy;
  IF v_memory_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_MEMORY_NOT_FOUND';
  END IF;
  UPDATE user_memories memory
  SET sensitivity = v_sensitivity, updated_at = clock_timestamp()
  WHERE memory.id = v_memory_id AND memory.user_id = p_user_id
    AND memory.sensitivity <> v_sensitivity;
  RETURN memory_governance_memory_json(p_user_id, v_memory_id);
END
$function$;

CREATE FUNCTION memory_governance_snapshot(p_user_id UUID)
RETURNS JSONB
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_settings JSONB;
  v_projects JSONB;
  v_conversations JSONB;
  v_memories JSONB;
  v_reviews JSONB;
  v_deletions JSONB;
  v_diagnostics JSONB;
BEGIN
  SELECT jsonb_build_object(
    'enabled', COALESCE(settings.enabled, false),
    'searchEnabled', COALESCE(settings.search_enabled, true),
    'autoRecordEnabled', COALESCE(settings.auto_record_enabled, false),
    'sensitiveMemoryEnabled', COALESCE(settings.sensitive_memory_enabled, false),
    'l2Mode', COALESCE(settings.l2_mode, 'inherit'),
    'l3Mode', COALESCE(settings.l3_mode, 'inherit')
  ) INTO v_settings
  FROM (SELECT 1) seed
  LEFT JOIN user_memory_settings settings ON settings.user_id = p_user_id;

  SELECT COALESCE(jsonb_agg(
    memory_governance_project_json(p_user_id, project.id)
    ORDER BY project.updated_at DESC, project.id
  ), '[]'::JSONB) INTO v_projects
  FROM projects project
  WHERE project.user_id = p_user_id AND project.deleted_at IS NULL;

  SELECT COALESCE(jsonb_agg(
    memory_governance_policy_json(p_user_id, conversation.id)
    ORDER BY conversation.updated_at DESC, conversation.id
  ), '[]'::JSONB) INTO v_conversations
  FROM conversations conversation
  WHERE conversation.user_id = p_user_id AND conversation.deleted_at IS NULL;

  SELECT COALESCE(jsonb_agg(
    memory_governance_memory_json(p_user_id, memory.id)
    ORDER BY memory.updated_at DESC, memory.id
  ), '[]'::JSONB) INTO v_memories
  FROM (
    SELECT candidate.id, candidate.updated_at
    FROM user_memories candidate
    WHERE candidate.user_id = p_user_id AND candidate.deleted_at IS NULL
    ORDER BY candidate.updated_at DESC, candidate.id
    LIMIT 500
  ) memory;

  SELECT COALESCE(jsonb_agg(review.payload ORDER BY review.created_at, review.id), '[]'::JSONB)
  INTO v_reviews
  FROM (
    SELECT suggestion.id, suggestion.created_at, jsonb_build_object(
      'id', suggestion.id::TEXT,
      'type', suggestion.candidate_type,
      'content', suggestion.candidate_content,
      'importance', suggestion.importance,
      'tags', to_jsonb(suggestion.tags),
      'sensitivity', suggestion.sensitivity,
      'proposedAction', suggestion.proposed_action,
      'reasonCode', suggestion.decision_reason_code,
      'scopeType', suggestion.proposed_scope_type,
      'projectId', COALESCE(suggestion.proposed_project_id::TEXT, ''),
      'conversationId', COALESCE(suggestion.proposed_conversation_id::TEXT, ''),
      'targets', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
          'memoryId', target.memory_id::TEXT,
          'revision', target.expected_revision,
          'type', COALESCE(memory.memory_type, ''),
          'content', CASE WHEN current_target.is_current
            THEN memory.content ELSE '' END,
          'scopeType', COALESCE(memory.scope_type, ''),
          'current', current_target.is_current
        ) ORDER BY target.memory_id)
        FROM user_memory_review_targets target
        LEFT JOIN user_memories memory
          ON memory.id = target.memory_id AND memory.user_id = p_user_id
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
            AND memory.revision = target.expected_revision
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
        ) current_target ON true
        WHERE target.suggestion_id = suggestion.id
          AND target.user_id = p_user_id
      ), '[]'::JSONB),
      'evidenceMessageIds', COALESCE((
        SELECT jsonb_agg(evidence.source_message_id::TEXT ORDER BY evidence.source_message_id)
        FROM user_memory_review_evidence evidence
        WHERE evidence.suggestion_id = suggestion.id
          AND evidence.user_id = p_user_id
      ), '[]'::JSONB),
      'expiresAt', memory_governance_epoch_millis(suggestion.review_expires_at),
      'createdAt', memory_governance_epoch_millis(suggestion.created_at)
    ) AS payload
    FROM user_memory_review_suggestions suggestion
    WHERE suggestion.user_id = p_user_id AND suggestion.status = 'pending'
      AND suggestion.review_expires_at > clock_timestamp()
    ORDER BY suggestion.created_at, suggestion.id
    LIMIT 100
  ) review;

  SELECT COALESCE(jsonb_agg(deletion.payload ORDER BY deletion.deleted_at DESC), '[]'::JSONB)
  INTO v_deletions
  FROM (
    SELECT manifest.deleted_at, jsonb_build_object(
      'manifestId', manifest.manifest_id::TEXT,
      'memoryId', manifest.memory_id::TEXT,
      'immediateHidden', true,
      'onlinePurgeStatus', lower(manifest.result_code),
      'backupExpiryStatus', CASE
        WHEN manifest.deleted_at + interval '8 weeks' <= clock_timestamp()
          THEN 'retention_expired'
        ELSE 'retention_pending'
      END,
      'backupExpiresAt', memory_governance_epoch_millis(manifest.deleted_at + interval '8 weeks'),
      'deletedAt', memory_governance_epoch_millis(manifest.deleted_at),
      'purgedAt', CASE WHEN manifest.purged_at IS NULL THEN NULL
        ELSE memory_governance_epoch_millis(manifest.purged_at) END
    ) AS payload
    FROM user_memory_deletion_manifests manifest
    WHERE manifest.user_id = p_user_id
    ORDER BY manifest.deleted_at DESC, manifest.manifest_id DESC
    LIMIT 20
  ) deletion;

  SELECT COALESCE(jsonb_agg(diagnostic.payload ORDER BY diagnostic.created_at DESC), '[]'::JSONB)
  INTO v_diagnostics
  FROM (
    SELECT hybrid.created_at, jsonb_build_object(
      'assistantMessageId', hybrid.assistant_message_id::TEXT,
      'profile', hybrid.retrieval_profile_id,
      'status', hybrid.status,
      'resultCode', hybrid.result_code,
      'fallbackCode', hybrid.fallback_code,
      'baselineCount', hybrid.baseline_count,
      'finalCount', hybrid.final_count,
      'overlapCount', hybrid.overlap_count,
      'estimatedTokens', hybrid.estimated_tokens,
      'durationMillis', hybrid.duration_millis,
      'createdAt', memory_governance_epoch_millis(hybrid.created_at)
    ) AS payload
    FROM message_memory_hybrid_shadow_observations hybrid
    WHERE hybrid.user_id = p_user_id
    UNION ALL
    SELECT lexical.created_at, jsonb_build_object(
      'assistantMessageId', lexical.assistant_message_id::TEXT,
      'profile', lexical.retrieval_profile_id,
      'status', lexical.status,
      'resultCode', lexical.result_code,
      'fallbackCode', 'NONE',
      'baselineCount', lexical.baseline_count,
      'finalCount', lexical.lexical_count,
      'overlapCount', lexical.overlap_count,
      'estimatedTokens', 0,
      'durationMillis', lexical.duration_millis,
      'createdAt', memory_governance_epoch_millis(lexical.created_at)
    ) AS payload
    FROM message_memory_lexical_shadow_observations lexical
    WHERE lexical.user_id = p_user_id
    ORDER BY created_at DESC
    LIMIT 20
  ) diagnostic;

  RETURN jsonb_build_object(
    'settings', v_settings,
    'projects', v_projects,
    'conversations', v_conversations,
    'memories', v_memories,
    'reviews', v_reviews,
    'deletions', v_deletions,
    'diagnostics', v_diagnostics
  );
END
$function$;

CREATE FUNCTION memory_governance_create_project(
  p_user_id UUID,
  p_project_id UUID,
  p_name TEXT,
  p_description TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_user_id IS NULL OR p_project_id IS NULL
    OR length(trim(p_name)) = 0 OR char_length(p_name) > 200
    OR char_length(p_description) > 4000
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_PROJECT_INVALID';
  END IF;
  IF (SELECT count(*) FROM projects project
      WHERE project.user_id = p_user_id AND project.deleted_at IS NULL) >= 200
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_PROJECT_LIMIT';
  END IF;
  INSERT INTO projects(id, user_id, name, description)
  VALUES (p_project_id, p_user_id, p_name, p_description);
  RETURN memory_governance_project_json(p_user_id, p_project_id);
END
$function$;

CREATE FUNCTION memory_governance_update_project(
  p_user_id UUID,
  p_project_id UUID,
  p_expected_revision BIGINT,
  p_name TEXT,
  p_description TEXT,
  p_lifecycle_status TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_project projects%ROWTYPE;
  v_generation BIGINT;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT project.* INTO v_project FROM projects project
  WHERE project.id = p_project_id AND project.user_id = p_user_id
    AND project.deleted_at IS NULL FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_PROJECT_NOT_FOUND';
  END IF;
  IF v_project.revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_REVISION_STALE';
  END IF;
  IF length(trim(p_name)) = 0 OR char_length(p_name) > 200
    OR char_length(p_description) > 4000
    OR p_lifecycle_status NOT IN ('active', 'archived')
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_PROJECT_INVALID';
  END IF;
  v_generation := v_project.scope_generation
    + CASE WHEN v_project.lifecycle_status <> p_lifecycle_status THEN 1 ELSE 0 END;
  UPDATE projects project
  SET name = p_name, description = p_description,
      lifecycle_status = p_lifecycle_status,
      revision = project.revision + 1,
      scope_generation = v_generation,
      archived_at = CASE WHEN p_lifecycle_status = 'archived' THEN v_now ELSE NULL END,
      updated_at = v_now
  WHERE project.id = p_project_id;
  IF v_generation <> v_project.scope_generation THEN
    UPDATE user_memories memory
    SET scope_generation = v_generation, updated_at = v_now
    WHERE memory.user_id = p_user_id AND memory.project_id = p_project_id
      AND memory.deleted_at IS NULL;
  END IF;
  RETURN memory_governance_project_json(p_user_id, p_project_id);
END
$function$;

CREATE FUNCTION memory_governance_get_conversation_policy(
  p_user_id UUID,
  p_conversation_id UUID
) RETURNS JSONB
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_result JSONB;
BEGIN
  v_result := memory_governance_policy_json(p_user_id, p_conversation_id);
  IF v_result IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_CONVERSATION_NOT_FOUND';
  END IF;
  RETURN v_result;
END
$function$;

CREATE FUNCTION memory_governance_update_conversation_policy(
  p_user_id UUID,
  p_conversation_id UUID,
  p_expected_scope_generation BIGINT,
  p_project_id UUID,
  p_use_mode TEXT,
  p_learn_mode TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_conversation conversations%ROWTYPE;
  v_generation BIGINT;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT conversation.* INTO v_conversation FROM conversations conversation
  WHERE conversation.id = p_conversation_id
    AND conversation.user_id = p_user_id
    AND conversation.deleted_at IS NULL FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_CONVERSATION_NOT_FOUND';
  END IF;
  IF v_conversation.memory_scope_generation <> p_expected_scope_generation THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_SCOPE_STALE';
  END IF;
  IF p_use_mode NOT IN ('inherit', 'on', 'off')
    OR p_learn_mode NOT IN ('inherit', 'on', 'off')
    OR (p_project_id IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM projects project
      WHERE project.id = p_project_id AND project.user_id = p_user_id
        AND project.deleted_at IS NULL
        AND (project.lifecycle_status = 'active'
          OR project.id = v_conversation.project_id)
    ))
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_POLICY_INVALID';
  END IF;
  v_generation := v_conversation.memory_scope_generation + 1;
  UPDATE conversations conversation
  SET project_id = p_project_id,
      memory_use_mode = p_use_mode,
      memory_learn_mode = p_learn_mode,
      memory_scope_generation = v_generation,
      updated_at = v_now
  WHERE conversation.id = p_conversation_id;
  UPDATE user_memories memory
  SET scope_generation = v_generation, updated_at = v_now
  WHERE memory.user_id = p_user_id
    AND memory.scope_conversation_id = p_conversation_id
    AND memory.deleted_at IS NULL;
  RETURN memory_governance_policy_json(p_user_id, p_conversation_id);
END
$function$;

CREATE FUNCTION memory_governance_create_memory(
  p_user_id UUID,
  p_memory_id UUID,
  p_memory_type TEXT,
  p_content TEXT,
  p_normalized_content TEXT,
  p_importance SMALLINT,
  p_tags TEXT[],
  p_scope_type TEXT,
  p_project_id UUID,
  p_conversation_id UUID,
  p_sensitivity TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_epoch BIGINT;
  v_generation BIGINT;
  v_hash TEXT;
  v_sensitivity TEXT;
BEGIN
  IF p_memory_type NOT IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    ) OR length(trim(p_content)) = 0 OR char_length(p_content) > 2000
    OR length(trim(p_normalized_content)) = 0
    OR char_length(p_normalized_content) > 2000
    OR p_importance NOT BETWEEN 1 AND 5 OR cardinality(p_tags) > 12
    OR p_sensitivity NOT IN ('normal', 'sensitive')
    OR memory_governance_is_secret(p_content)
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_MEMORY_INVALID';
  END IF;
  v_sensitivity := CASE
    WHEN memory_governance_classify_sensitivity(p_content) = 'sensitive'
      THEN 'sensitive'
    ELSE p_sensitivity
  END;
  IF v_sensitivity = 'sensitive' AND NOT EXISTS (
    SELECT 1 FROM user_memory_settings settings
    WHERE settings.user_id = p_user_id AND settings.sensitive_memory_enabled
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_SENSITIVE_DISABLED';
  END IF;
  v_generation := memory_governance_scope_generation(
    p_user_id, p_scope_type, p_project_id, p_conversation_id
  );
  IF EXISTS (
    SELECT 1 FROM user_memories memory
    WHERE memory.user_id = p_user_id AND memory.deleted_at IS NULL
      AND memory.normalized_content = p_normalized_content
      AND memory.scope_type = p_scope_type
      AND (p_scope_type <> 'project' OR memory.project_id = p_project_id)
      AND (p_scope_type <> 'conversation'
        OR memory.scope_conversation_id = p_conversation_id)
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '23505',
      MESSAGE = 'MEMORY_GOVERNANCE_EXACT_CONFLICT';
  END IF;
  INSERT INTO user_memory_state(user_id) VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_epoch FROM user_memory_state state
  WHERE state.user_id = p_user_id FOR UPDATE;
  v_hash := encode(sha256(convert_to(p_content, 'UTF8')), 'hex');
  INSERT INTO user_memories (
    id, user_id, memory_type, content, normalized_content, importance,
    tags, source, enabled, scope_type, project_id,
    scope_conversation_id, scope_generation, revision, visibility_epoch,
    content_hash, authority_kind, lifecycle_status, confidence,
    observed_at, sensitivity, temporal_basis
  ) VALUES (
    p_memory_id, p_user_id, p_memory_type, p_content,
    p_normalized_content, p_importance, p_tags, 'manual', true,
    p_scope_type, p_project_id, p_conversation_id, v_generation,
    1, v_epoch, v_hash, 'manual', 'active', 1.0,
    clock_timestamp(), v_sensitivity, 'none'
  );
  RETURN memory_governance_memory_json(p_user_id, p_memory_id);
END
$function$;

CREATE FUNCTION memory_governance_update_memory(
  p_user_id UUID,
  p_memory_id UUID,
  p_expected_revision BIGINT,
  p_memory_type TEXT,
  p_content TEXT,
  p_normalized_content TEXT,
  p_importance SMALLINT,
  p_tags TEXT[],
  p_scope_type TEXT,
  p_project_id UUID,
  p_conversation_id UUID,
  p_sensitivity TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory user_memories%ROWTYPE;
  v_epoch BIGINT;
  v_generation BIGINT;
  v_hash TEXT;
  v_payload_changed BOOLEAN;
  v_scope_changed BOOLEAN;
  v_sensitivity TEXT;
BEGIN
  SELECT memory.* INTO v_memory FROM user_memories memory
  WHERE memory.id = p_memory_id AND memory.user_id = p_user_id
    AND memory.deleted_at IS NULL FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_MEMORY_NOT_FOUND';
  END IF;
  IF v_memory.revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_REVISION_STALE';
  END IF;
  IF v_memory.lifecycle_status <> 'active' OR p_memory_type NOT IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    ) OR length(trim(p_content)) = 0 OR char_length(p_content) > 2000
    OR length(trim(p_normalized_content)) = 0
    OR char_length(p_normalized_content) > 2000
    OR p_importance NOT BETWEEN 1 AND 5 OR cardinality(p_tags) > 12
    OR p_sensitivity NOT IN ('normal', 'sensitive')
    OR memory_governance_is_secret(p_content)
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_MEMORY_INVALID';
  END IF;
  v_sensitivity := CASE
    WHEN memory_governance_classify_sensitivity(p_content) = 'sensitive'
      THEN 'sensitive'
    ELSE p_sensitivity
  END;
  IF v_sensitivity = 'sensitive' AND NOT EXISTS (
    SELECT 1 FROM user_memory_settings settings
    WHERE settings.user_id = p_user_id AND settings.sensitive_memory_enabled
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_SENSITIVE_DISABLED';
  END IF;
  v_generation := memory_governance_scope_generation(
    p_user_id, p_scope_type, p_project_id, p_conversation_id
  );
  v_payload_changed :=
    v_memory.memory_type IS DISTINCT FROM p_memory_type
    OR v_memory.content IS DISTINCT FROM p_content
    OR v_memory.normalized_content IS DISTINCT FROM p_normalized_content
    OR v_memory.importance IS DISTINCT FROM p_importance
    OR v_memory.tags IS DISTINCT FROM p_tags
    OR v_memory.sensitivity IS DISTINCT FROM v_sensitivity;
  v_scope_changed :=
    v_memory.scope_type IS DISTINCT FROM p_scope_type
    OR v_memory.project_id IS DISTINCT FROM p_project_id
    OR v_memory.scope_conversation_id IS DISTINCT FROM p_conversation_id;
  IF NOT v_payload_changed AND NOT v_scope_changed THEN
    RETURN memory_governance_memory_json(p_user_id, p_memory_id);
  END IF;
  IF EXISTS (
    SELECT 1 FROM user_memories memory
    WHERE memory.user_id = p_user_id AND memory.deleted_at IS NULL
      AND memory.id <> p_memory_id
      AND memory.normalized_content = p_normalized_content
      AND memory.scope_type = p_scope_type
      AND (p_scope_type <> 'project' OR memory.project_id = p_project_id)
      AND (p_scope_type <> 'conversation'
        OR memory.scope_conversation_id = p_conversation_id)
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '23505',
      MESSAGE = 'MEMORY_GOVERNANCE_EXACT_CONFLICT';
  END IF;
  SELECT state.visibility_epoch INTO v_epoch FROM user_memory_state state
  WHERE state.user_id = p_user_id FOR UPDATE;
  v_hash := encode(sha256(convert_to(p_content, 'UTF8')), 'hex');
  PERFORM memory_governance_append_revision(
    v_memory,
    v_hash,
    CASE WHEN v_scope_changed AND NOT v_payload_changed
      THEN 'move' ELSE 'update' END
  );
  IF v_scope_changed AND NOT v_payload_changed THEN
    UPDATE user_memories memory
    SET scope_type = p_scope_type, project_id = p_project_id,
        scope_conversation_id = p_conversation_id,
        scope_generation = v_generation, revision = memory.revision + 1,
        visibility_epoch = v_epoch, updated_at = clock_timestamp()
    WHERE memory.id = p_memory_id;
    RETURN memory_governance_memory_json(p_user_id, p_memory_id);
  END IF;
  UPDATE user_memories memory
  SET memory_type = p_memory_type, content = p_content,
      normalized_content = p_normalized_content, importance = p_importance,
      tags = p_tags, source = 'manual', enabled = true,
      scope_type = p_scope_type, project_id = p_project_id,
      scope_conversation_id = p_conversation_id,
      scope_generation = v_generation, revision = memory.revision + 1,
      visibility_epoch = v_epoch, content_hash = v_hash,
      authority_kind = 'manual', extraction_profile_id = NULL,
      lifecycle_status = 'active', subject_key = NULL, fact_key = NULL,
      confidence = 1.0, observed_at = clock_timestamp(),
      valid_from = NULL, valid_to = NULL, expires_at = NULL,
      superseded_by_memory_id = NULL, sensitivity = v_sensitivity,
      temporal_basis = 'none', temporal_parser_version = NULL,
      updated_at = clock_timestamp()
  WHERE memory.id = p_memory_id;
  RETURN memory_governance_memory_json(p_user_id, p_memory_id);
END
$function$;

CREATE FUNCTION memory_governance_delete_memory(
  p_user_id UUID,
  p_memory_id UUID,
  p_expected_revision BIGINT,
  p_event_id UUID,
  p_job_id UUID,
  p_tombstone_id UUID,
  p_manifest_id UUID
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_manifest user_memory_deletion_manifests%ROWTYPE;
BEGIN
  IF NOT memory_delete_direct_scoped(
    p_user_id, p_memory_id, p_expected_revision,
    p_event_id, p_job_id, p_tombstone_id, p_manifest_id
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_REVISION_STALE';
  END IF;
  SELECT manifest.* INTO v_manifest
  FROM user_memory_deletion_manifests manifest
  WHERE manifest.manifest_id = p_manifest_id AND manifest.user_id = p_user_id;
  RETURN jsonb_build_object(
    'manifestId', v_manifest.manifest_id::TEXT,
    'memoryId', v_manifest.memory_id::TEXT,
    'immediateHidden', true,
    'onlinePurgeStatus', lower(v_manifest.result_code),
    'backupExpiryStatus', 'retention_pending',
    'backupExpiresAt', memory_governance_epoch_millis(v_manifest.deleted_at + interval '8 weeks'),
    'deletedAt', memory_governance_epoch_millis(v_manifest.deleted_at),
    'purgedAt', NULL
  );
END
$function$;

CREATE FUNCTION memory_governance_memory_detail(
  p_user_id UUID,
  p_memory_id UUID
) RETURNS JSONB
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_memory JSONB;
  v_evidence JSONB;
  v_history JSONB;
  v_usages JSONB;
BEGIN
  v_memory := memory_governance_memory_json(p_user_id, p_memory_id);
  IF v_memory IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_MEMORY_NOT_FOUND';
  END IF;
  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'messageId', evidence.source_message_id::TEXT,
    'conversationId', evidence.source_conversation_id::TEXT,
    'conversationTitle', COALESCE(conversation.title, ''),
    'role', evidence.evidence_role,
    'sourceDeleted', source.id IS NULL,
    'sourceExcerpt', CASE WHEN source.id IS NULL THEN ''
      ELSE left(source.content, 500) END,
    'observedAt', memory_governance_epoch_millis(evidence.observed_at)
  ) ORDER BY evidence.observed_at, evidence.source_message_id), '[]'::JSONB)
  INTO v_evidence
  FROM user_memory_evidence evidence
  LEFT JOIN messages source
    ON source.id = evidence.source_message_id AND source.user_id = p_user_id
      AND source.status = 'completed' AND source.deleted_at IS NULL
      AND EXISTS (
        SELECT 1 FROM conversations source_conversation
        WHERE source_conversation.id = source.conversation_id
          AND source_conversation.user_id = p_user_id
          AND source_conversation.deleted_at IS NULL
      )
  LEFT JOIN conversations conversation
    ON conversation.id = evidence.source_conversation_id
      AND conversation.user_id = p_user_id
      AND conversation.deleted_at IS NULL
  WHERE evidence.memory_id = p_memory_id AND evidence.user_id = p_user_id;

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'revision', revision.revision,
    'operation', revision.operation,
    'priorContent', CASE WHEN revision.purged_at IS NULL
      THEN COALESCE(revision.prior_content_snapshot, '') ELSE '' END,
    'actorType', revision.actor_type,
    'resultCode', COALESCE(revision.result_code, ''),
    'purged', revision.purged_at IS NOT NULL,
    'createdAt', memory_governance_epoch_millis(revision.created_at)
  ) ORDER BY revision.revision DESC), '[]'::JSONB)
  INTO v_history
  FROM user_memory_revisions revision
  WHERE revision.memory_id = p_memory_id AND revision.user_id = p_user_id;

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'assistantMessageId', usage.assistant_message_id::TEXT,
    'memoryRevision', usage.entity_revision,
    'createdAt', memory_governance_epoch_millis(usage.created_at)
  ) ORDER BY usage.created_at DESC, usage.assistant_message_id), '[]'::JSONB)
  INTO v_usages
  FROM message_memory_usages usage
  WHERE usage.user_id = p_user_id AND usage.entity_id = p_memory_id;

  RETURN jsonb_build_object(
    'memory', v_memory, 'evidence', v_evidence,
    'history', v_history, 'usages', v_usages
  );
END
$function$;

CREATE FUNCTION memory_governance_decide_review(
  p_user_id UUID,
  p_suggestion_id UUID,
  p_decision_id UUID,
  p_decision_kind TEXT,
  p_memory_id UUID,
  p_edited_content TEXT,
  p_edited_normalized_content TEXT,
  p_decision_hash TEXT
) RETURNS JSONB
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_suggestion user_memory_review_suggestions%ROWTYPE;
  v_existing user_memory_review_decisions%ROWTYPE;
  v_target RECORD;
  v_target_memory user_memories%ROWTYPE;
  v_content TEXT;
  v_normalized TEXT;
  v_content_hash TEXT;
  v_scope_generation BIGINT;
  v_epoch BIGINT;
  v_source_message_id UUID;
  v_source_conversation_id UUID;
  v_result_code TEXT;
  v_result_revision BIGINT;
  v_sensitivity TEXT;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  SELECT suggestion.* INTO v_suggestion
  FROM user_memory_review_suggestions suggestion
  WHERE suggestion.id = p_suggestion_id AND suggestion.user_id = p_user_id
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_NOT_FOUND';
  END IF;
  SELECT decision.* INTO v_existing
  FROM user_memory_review_decisions decision
  WHERE decision.suggestion_id = p_suggestion_id
    AND decision.user_id = p_user_id;
  IF FOUND THEN
    IF v_existing.decision_hash <> p_decision_hash THEN
      RAISE EXCEPTION USING ERRCODE = '40001',
        MESSAGE = 'MEMORY_GOVERNANCE_REPLAY_CONFLICT';
    END IF;
    RETURN jsonb_build_object(
      'suggestionId', p_suggestion_id::TEXT,
      'decision', v_existing.decision_kind,
      'status', CASE WHEN v_existing.result_memory_id IS NULL THEN 'rejected' ELSE 'accepted' END,
      'resultCode', v_existing.result_code,
      'memoryId', COALESCE(v_existing.result_memory_id::TEXT, ''),
      'memoryRevision', COALESCE(v_existing.result_memory_revision, 0)
    );
  END IF;
  IF v_suggestion.status <> 'pending'
    OR v_suggestion.review_expires_at <= v_now
    OR p_decision_kind NOT IN (
      'keep_current', 'accept_new', 'edit_merge', 'keep_both', 'reject'
    )
    OR p_decision_hash !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_STALE';
  END IF;

  SELECT state.visibility_epoch INTO v_epoch FROM user_memory_state state
  WHERE state.user_id = p_user_id FOR UPDATE;
  IF v_epoch IS DISTINCT FROM v_suggestion.visibility_epoch THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_STALE';
  END IF;
  v_scope_generation := memory_governance_scope_generation(
    p_user_id, v_suggestion.proposed_scope_type,
    v_suggestion.proposed_project_id, v_suggestion.proposed_conversation_id
  );
  IF v_scope_generation <> v_suggestion.scope_generation THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_SCOPE_STALE';
  END IF;
  FOR v_target IN
    SELECT target.memory_id, target.expected_revision
    FROM user_memory_review_targets target
    WHERE target.suggestion_id = p_suggestion_id AND target.user_id = p_user_id
    ORDER BY target.memory_id
  LOOP
    SELECT memory.* INTO v_target_memory FROM user_memories memory
    WHERE memory.id = v_target.memory_id AND memory.user_id = p_user_id
      AND memory.deleted_at IS NULL FOR UPDATE;
    IF NOT FOUND OR v_target_memory.revision <> v_target.expected_revision
      OR v_target_memory.lifecycle_status <> 'active'
    THEN
      RAISE EXCEPTION USING ERRCODE = '40001',
        MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_STALE';
    END IF;
  END LOOP;

  IF p_decision_kind IN ('keep_current', 'reject') THEN
    v_result_code := CASE p_decision_kind
      WHEN 'keep_current' THEN 'USER_KEPT_CURRENT' ELSE 'USER_REJECTED' END;
    UPDATE user_memory_review_suggestions suggestion
    SET candidate_content = NULL, normalized_content = NULL, tags = '{}',
        subject_key = NULL, fact_key = NULL, status = 'rejected',
        disposition = 'rejected', decision_kind = p_decision_kind,
        result_code = v_result_code, decided_at = v_now, purged_at = v_now
    WHERE suggestion.id = p_suggestion_id;
    INSERT INTO user_memory_review_decisions(
      id, suggestion_id, user_id, decision_kind, decision_hash, result_code
    ) VALUES (
      p_decision_id, p_suggestion_id, p_user_id,
      p_decision_kind, p_decision_hash, v_result_code
    );
    UPDATE message_memory_activities activity
    SET action = 'rejected', status = 'completed', reason_code = v_result_code,
        updated_at = v_now
    WHERE activity.user_id = p_user_id
      AND activity.source_kind = 'review_suggestion'
      AND activity.source_id = p_suggestion_id;
    RETURN jsonb_build_object(
      'suggestionId', p_suggestion_id::TEXT, 'decision', p_decision_kind,
      'status', 'rejected', 'resultCode', v_result_code,
      'memoryId', '', 'memoryRevision', 0
    );
  END IF;

  IF p_memory_id IS NULL OR v_suggestion.sensitivity = 'secret' THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_STALE';
  END IF;
  v_content := CASE WHEN p_decision_kind = 'edit_merge'
    THEN p_edited_content ELSE v_suggestion.candidate_content END;
  v_normalized := CASE WHEN p_decision_kind = 'edit_merge'
    THEN p_edited_normalized_content ELSE v_suggestion.normalized_content END;
  IF length(trim(v_content)) = 0 OR char_length(v_content) > 2000
    OR length(trim(v_normalized)) = 0 OR char_length(v_normalized) > 2000
    OR memory_governance_is_secret(v_content)
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_STALE';
  END IF;
  v_sensitivity := CASE
    WHEN memory_governance_classify_sensitivity(v_content) = 'sensitive'
      THEN 'sensitive'
    ELSE v_suggestion.sensitivity
  END;
  IF v_sensitivity = 'sensitive' AND NOT EXISTS (
      SELECT 1 FROM user_memory_settings settings
      WHERE settings.user_id = p_user_id AND settings.sensitive_memory_enabled
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_STALE';
  END IF;
  v_content_hash := encode(sha256(convert_to(v_content, 'UTF8')), 'hex');

  IF EXISTS (
    SELECT 1 FROM user_memories memory
    WHERE memory.user_id = p_user_id AND memory.deleted_at IS NULL
      AND memory.normalized_content = v_normalized
      AND memory.scope_type = v_suggestion.proposed_scope_type
      AND (memory.scope_type <> 'project'
        OR memory.project_id = v_suggestion.proposed_project_id)
      AND (memory.scope_type <> 'conversation'
        OR memory.scope_conversation_id = v_suggestion.proposed_conversation_id)
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '23505',
      MESSAGE = 'MEMORY_GOVERNANCE_EXACT_CONFLICT';
  END IF;

  SELECT evidence.source_message_id, evidence.source_conversation_id
  INTO v_source_message_id, v_source_conversation_id
  FROM user_memory_review_evidence evidence
  JOIN messages source
    ON source.id = evidence.source_message_id AND source.user_id = p_user_id
      AND source.role = 'user' AND source.status = 'completed'
      AND source.deleted_at IS NULL
  WHERE evidence.suggestion_id = p_suggestion_id
    AND evidence.user_id = p_user_id AND evidence.evidence_role = 'user'
  ORDER BY evidence.observed_at, evidence.source_message_id
  LIMIT 1;
  IF v_source_message_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'MEMORY_GOVERNANCE_REVIEW_STALE';
  END IF;

  INSERT INTO user_memories (
    id, user_id, memory_type, content, normalized_content, importance,
    tags, source, source_conversation_id, source_message_id, enabled,
    scope_type, project_id, scope_conversation_id, scope_generation,
    revision, visibility_epoch, content_hash, authority_kind,
    lifecycle_status, subject_key, fact_key, confidence, observed_at,
    valid_from, valid_to, expires_at, sensitivity, temporal_basis,
    temporal_parser_version
  ) VALUES (
    p_memory_id, p_user_id, v_suggestion.candidate_type, v_content,
    v_normalized, v_suggestion.importance, v_suggestion.tags, 'ai',
    v_source_conversation_id, v_source_message_id, true,
    v_suggestion.proposed_scope_type, v_suggestion.proposed_project_id,
    v_suggestion.proposed_conversation_id, v_scope_generation,
    1, v_epoch, v_content_hash, 'confirmed', 'active',
    v_suggestion.subject_key, v_suggestion.fact_key, 1.0,
    v_suggestion.observed_at, v_suggestion.valid_from,
    v_suggestion.valid_to, v_suggestion.fact_expires_at,
    v_sensitivity, v_suggestion.temporal_basis,
    v_suggestion.temporal_parser_version
  );
  INSERT INTO user_memory_evidence(
    memory_id, source_message_id, user_id, source_conversation_id,
    evidence_role, source_content_hash, observed_at
  )
  SELECT p_memory_id, evidence.source_message_id, p_user_id,
    evidence.source_conversation_id, evidence.evidence_role,
    evidence.source_content_hash, evidence.observed_at
  FROM user_memory_review_evidence evidence
  JOIN messages source
    ON source.id = evidence.source_message_id AND source.user_id = p_user_id
      AND source.deleted_at IS NULL
  WHERE evidence.suggestion_id = p_suggestion_id
    AND evidence.user_id = p_user_id;

  IF p_decision_kind IN ('accept_new', 'edit_merge') THEN
    FOR v_target IN
      SELECT target.memory_id FROM user_memory_review_targets target
      WHERE target.suggestion_id = p_suggestion_id
        AND target.user_id = p_user_id ORDER BY target.memory_id
    LOOP
      SELECT memory.* INTO v_target_memory FROM user_memories memory
      WHERE memory.id = v_target.memory_id AND memory.user_id = p_user_id
      FOR UPDATE;
      PERFORM memory_governance_append_revision(
        v_target_memory, v_target_memory.content_hash, 'supersede'
      );
      UPDATE user_memories memory
      SET lifecycle_status = 'superseded', enabled = false,
          superseded_by_memory_id = p_memory_id,
          revision = memory.revision + 1, updated_at = v_now
      WHERE memory.id = v_target.memory_id;
    END LOOP;
  END IF;

  v_result_code := CASE p_decision_kind
    WHEN 'edit_merge' THEN 'USER_EDIT_MERGED'
    WHEN 'keep_both' THEN 'USER_KEPT_BOTH'
    ELSE 'USER_ACCEPTED' END;
  v_result_revision := 1;
  UPDATE user_memory_review_suggestions suggestion
  SET candidate_content = NULL, normalized_content = NULL, tags = '{}',
      subject_key = NULL, fact_key = NULL, status = 'accepted',
      decision_kind = p_decision_kind, result_memory_id = p_memory_id,
      result_code = v_result_code, decided_at = v_now, purged_at = v_now
  WHERE suggestion.id = p_suggestion_id;
  INSERT INTO user_memory_review_decisions(
    id, suggestion_id, user_id, decision_kind, decision_hash,
    result_code, result_memory_id, result_memory_revision
  ) VALUES (
    p_decision_id, p_suggestion_id, p_user_id, p_decision_kind,
    p_decision_hash, v_result_code, p_memory_id, v_result_revision
  );
  UPDATE message_memory_activities activity
  SET subject_type = 'memory', subject_id = p_memory_id,
      subject_revision = v_result_revision, action = 'created',
      status = 'completed', reason_code = v_result_code,
      updated_at = v_now
  WHERE activity.user_id = p_user_id
    AND activity.source_kind = 'review_suggestion'
    AND activity.source_id = p_suggestion_id;
  RETURN jsonb_build_object(
    'suggestionId', p_suggestion_id::TEXT, 'decision', p_decision_kind,
    'status', 'accepted', 'resultCode', v_result_code,
    'memoryId', p_memory_id::TEXT, 'memoryRevision', v_result_revision
  );
END
$function$;

CREATE FUNCTION memory_governance_list_message_activities(
  p_user_id UUID,
  p_assistant_message_id UUID,
  p_limit INTEGER
) RETURNS JSONB
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_result JSONB;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 20 OR NOT EXISTS (
    SELECT 1 FROM messages assistant
    WHERE assistant.id = p_assistant_message_id
      AND assistant.user_id = p_user_id
      AND assistant.role = 'assistant' AND assistant.deleted_at IS NULL
      AND EXISTS (
        SELECT 1 FROM conversations activity_conversation
        WHERE activity_conversation.id = assistant.conversation_id
          AND activity_conversation.user_id = p_user_id
          AND activity_conversation.deleted_at IS NULL
      )
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'MEMORY_GOVERNANCE_ACTIVITY_INVALID';
  END IF;
  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'id', activity.id::TEXT,
    'assistantMessageId', activity.assistant_message_id::TEXT,
    'ordinal', activity.ordinal,
    'subjectType', activity.subject_type,
    'subjectId', activity.subject_id::TEXT,
    'subjectRevision', activity.subject_revision,
    'action', activity.action,
    'status', activity.status,
    'reasonCode', activity.reason_code,
    'undoKind', activity.undo_kind,
    'undoStatus', activity.undo_status,
    'sourceKind', activity.source_kind,
    'scopeType', COALESCE(memory.scope_type, suggestion.proposed_scope_type, ''),
    'memoryType', CASE WHEN current_memory.is_current
      THEN COALESCE(memory.memory_type, '') ELSE '' END,
    'memoryContent', CASE WHEN current_memory.is_current
      THEN COALESCE(memory.content, '') ELSE '' END,
    'memoryRevision', CASE WHEN current_memory.is_current
      THEN memory.revision ELSE NULL END,
    'memoryDeleted', activity.subject_type = 'memory'
      AND NOT current_memory.is_current,
    'createdAt', memory_governance_epoch_millis(activity.created_at),
    'updatedAt', memory_governance_epoch_millis(activity.updated_at)
  ) ORDER BY activity.ordinal), '[]'::JSONB) INTO v_result
  FROM (
    SELECT candidate.*
    FROM message_memory_activities candidate
    WHERE candidate.user_id = p_user_id
      AND candidate.assistant_message_id = p_assistant_message_id
    ORDER BY candidate.ordinal
    LIMIT p_limit
  ) activity
  LEFT JOIN user_memories memory
    ON activity.subject_type = 'memory' AND memory.id = activity.subject_id
      AND memory.user_id = p_user_id
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
  LEFT JOIN user_memory_review_suggestions suggestion
    ON activity.source_kind = 'review_suggestion'
      AND suggestion.id = activity.source_id AND suggestion.user_id = p_user_id
  ;
  RETURN v_result;
END
$function$;

DO $pin_governance_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_governance_epoch_millis(timestamp with time zone)',
    'memory_governance_is_secret(text)',
    'memory_governance_classify_sensitivity(text)',
    'memory_governance_scope_generation(uuid,text,uuid,uuid)',
    'memory_governance_append_revision(user_memories,text,text)',
    'memory_governance_project_json(uuid,uuid)',
    'memory_governance_policy_json(uuid,uuid)',
    'memory_governance_memory_json(uuid,uuid)',
    'memory_governance_upsert_global_legacy(uuid,uuid,text,text,text,smallint,text[],uuid,uuid,boolean)',
    'memory_governance_update_global_legacy(uuid,uuid,text,text,text,smallint,text[],boolean)',
    'memory_governance_snapshot(uuid)',
    'memory_governance_create_project(uuid,uuid,text,text)',
    'memory_governance_update_project(uuid,uuid,bigint,text,text,text)',
    'memory_governance_get_conversation_policy(uuid,uuid)',
    'memory_governance_update_conversation_policy(uuid,uuid,bigint,uuid,text,text)',
    'memory_governance_create_memory(uuid,uuid,text,text,text,smallint,text[],text,uuid,uuid,text)',
    'memory_governance_update_memory(uuid,uuid,bigint,text,text,text,smallint,text[],text,uuid,uuid,text)',
    'memory_governance_delete_memory(uuid,uuid,bigint,uuid,uuid,uuid,uuid)',
    'memory_governance_memory_detail(uuid,uuid)',
    'memory_governance_decide_review(uuid,uuid,uuid,text,uuid,text,text,text)',
    'memory_governance_list_message_activities(uuid,uuid,integer)'
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
$pin_governance_functions$;

ALTER TABLE user_memory_review_decisions OWNER TO memory_runtime_owner;
GRANT INSERT, UPDATE ON projects TO memory_runtime_owner;
REVOKE ALL ON user_memory_review_decisions
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;

REVOKE ALL ON FUNCTION memory_governance_epoch_millis(TIMESTAMPTZ),
  memory_governance_is_secret(TEXT),
  memory_governance_classify_sensitivity(TEXT),
  memory_governance_scope_generation(UUID, TEXT, UUID, UUID),
  memory_governance_append_revision(user_memories, TEXT, TEXT),
  memory_governance_project_json(UUID, UUID),
  memory_governance_policy_json(UUID, UUID),
  memory_governance_memory_json(UUID, UUID),
  memory_governance_upsert_global_legacy(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN),
  memory_governance_update_global_legacy(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN),
  memory_governance_snapshot(UUID),
  memory_governance_create_project(UUID, UUID, TEXT, TEXT),
  memory_governance_update_project(UUID, UUID, BIGINT, TEXT, TEXT, TEXT),
  memory_governance_get_conversation_policy(UUID, UUID),
  memory_governance_update_conversation_policy(UUID, UUID, BIGINT, UUID, TEXT, TEXT),
  memory_governance_create_memory(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], TEXT, UUID, UUID, TEXT),
  memory_governance_update_memory(UUID, UUID, BIGINT, TEXT, TEXT, TEXT, SMALLINT, TEXT[], TEXT, UUID, UUID, TEXT),
  memory_governance_delete_memory(UUID, UUID, BIGINT, UUID, UUID, UUID, UUID),
  memory_governance_memory_detail(UUID, UUID),
  memory_governance_decide_review(UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT, TEXT),
  memory_governance_list_message_activities(UUID, UUID, INTEGER)
FROM PUBLIC;

REVOKE EXECUTE ON FUNCTION
  memory_upsert_global_manual(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN),
  memory_update_global_manual(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN)
FROM go_api_runtime;

GRANT EXECUTE ON FUNCTION
  memory_governance_snapshot(UUID),
  memory_governance_create_project(UUID, UUID, TEXT, TEXT),
  memory_governance_update_project(UUID, UUID, BIGINT, TEXT, TEXT, TEXT),
  memory_governance_get_conversation_policy(UUID, UUID),
  memory_governance_update_conversation_policy(UUID, UUID, BIGINT, UUID, TEXT, TEXT),
  memory_governance_upsert_global_legacy(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN),
  memory_governance_update_global_legacy(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN),
  memory_governance_create_memory(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], TEXT, UUID, UUID, TEXT),
  memory_governance_update_memory(UUID, UUID, BIGINT, TEXT, TEXT, TEXT, SMALLINT, TEXT[], TEXT, UUID, UUID, TEXT),
  memory_governance_delete_memory(UUID, UUID, BIGINT, UUID, UUID, UUID, UUID),
  memory_governance_memory_detail(UUID, UUID),
  memory_governance_decide_review(UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT, TEXT),
  memory_governance_list_message_activities(UUID, UUID, INTEGER)
TO go_api_runtime;

DO $owner_create_revocation$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner', current_schema()
  );
END
$owner_create_revocation$;
