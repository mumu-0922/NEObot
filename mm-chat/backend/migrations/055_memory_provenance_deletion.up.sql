-- Memory v2 PR4 canonical provenance and deletion correctness. Canonical
-- plaintext remains in user_memories; supporting tables retain references,
-- hashes, and bounded prior snapshots only.

CREATE TABLE user_memory_state (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  visibility_epoch BIGINT NOT NULL DEFAULT 1,
  active_projection_generation BIGINT NOT NULL DEFAULT 1,
  active_retrieval_profile_id TEXT,
  active_l2_generation BIGINT NOT NULL DEFAULT 1,
  active_l3_generation BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_state_visibility_epoch_positive
    CHECK (visibility_epoch >= 1),
  CONSTRAINT user_memory_state_projection_generation_positive
    CHECK (active_projection_generation >= 1),
  CONSTRAINT user_memory_state_l2_generation_positive
    CHECK (active_l2_generation >= 1),
  CONSTRAINT user_memory_state_l3_generation_positive
    CHECK (active_l3_generation >= 1),
  CONSTRAINT user_memory_state_retrieval_profile_bounded CHECK (
    active_retrieval_profile_id IS NULL
    OR (
      octet_length(active_retrieval_profile_id) BETWEEN 1 AND 256
      AND active_retrieval_profile_id = trim(active_retrieval_profile_id)
    )
  ),
  CONSTRAINT user_memory_state_timestamps_order CHECK (updated_at >= created_at)
);

INSERT INTO user_memory_state(user_id)
SELECT id FROM users
ON CONFLICT (user_id) DO NOTHING;

ALTER TABLE messages
  ADD CONSTRAINT messages_id_user_unique UNIQUE (id, user_id);

ALTER TABLE user_memories
  ADD CONSTRAINT user_memories_id_user_unique UNIQUE (id, user_id),
  ADD COLUMN revision BIGINT,
  ADD COLUMN visibility_epoch BIGINT,
  ADD COLUMN content_hash TEXT,
  ADD COLUMN authority_kind TEXT,
  ADD COLUMN extraction_profile_id TEXT;

UPDATE user_memories memory
SET
  revision = 1,
  visibility_epoch = state.visibility_epoch,
  content_hash = encode(sha256(convert_to(memory.content, 'UTF8')), 'hex'),
  authority_kind = CASE memory.source
    WHEN 'manual' THEN 'manual'
    ELSE 'auto'
  END
FROM user_memory_state state
WHERE state.user_id = memory.user_id;

ALTER TABLE user_memories
  ALTER COLUMN revision SET DEFAULT 1,
  ALTER COLUMN revision SET NOT NULL,
  ALTER COLUMN visibility_epoch SET DEFAULT 1,
  ALTER COLUMN visibility_epoch SET NOT NULL,
  ALTER COLUMN content_hash SET NOT NULL,
  ALTER COLUMN authority_kind SET NOT NULL,
  ADD CONSTRAINT user_memories_revision_positive CHECK (revision >= 1),
  ADD CONSTRAINT user_memories_visibility_epoch_positive
    CHECK (visibility_epoch >= 1),
  ADD CONSTRAINT user_memories_content_hash_check
    CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT user_memories_authority_kind_allowed CHECK (
    authority_kind IN ('manual', 'direct_user', 'confirmed', 'import', 'auto')
  ),
  ADD CONSTRAINT user_memories_extraction_profile_check CHECK (
    extraction_profile_id IS NULL
    OR extraction_profile_id ~ '^[0-9a-f]{64}$'
  );

ALTER TABLE user_memories
  DROP CONSTRAINT user_memories_content_bounded,
  DROP CONSTRAINT user_memories_normalized_content_bounded,
  ADD CONSTRAINT user_memories_content_bounded CHECK (
    (
      length(trim(content)) > 0
      AND char_length(content) <= 2000
      AND length(trim(normalized_content)) > 0
      AND char_length(normalized_content) <= 2000
    )
    OR (
      deleted_at IS NOT NULL
      AND content = ''
      AND normalized_content = ''
    )
  );

CREATE TABLE user_memory_evidence (
  memory_id UUID NOT NULL,
  source_message_id UUID NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_conversation_id UUID NOT NULL,
  evidence_role TEXT NOT NULL,
  source_content_hash TEXT NOT NULL,
  observed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (memory_id, source_message_id),
  CONSTRAINT user_memory_evidence_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_evidence_message_owner_fk
    FOREIGN KEY (source_message_id, user_id)
    REFERENCES messages(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_evidence_conversation_owner_fk
    FOREIGN KEY (source_conversation_id, user_id)
    REFERENCES conversations(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_evidence_role_allowed
    CHECK (evidence_role IN ('user', 'assistant_context')),
  CONSTRAINT user_memory_evidence_source_hash_check
    CHECK (source_content_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT user_memory_evidence_observed_order
    CHECK (observed_at <= created_at)
);

CREATE INDEX idx_user_memory_evidence_user_source
  ON user_memory_evidence(user_id, source_conversation_id, source_message_id);

INSERT INTO user_memory_evidence (
  memory_id,
  source_message_id,
  user_id,
  source_conversation_id,
  evidence_role,
  source_content_hash,
  observed_at,
  created_at
)
SELECT
  memory.id,
  source.id,
  memory.user_id,
  source.conversation_id,
  'user',
  encode(sha256(convert_to(source.content, 'UTF8')), 'hex'),
  COALESCE(source.completed_at, source.created_at),
  GREATEST(memory.created_at, COALESCE(source.completed_at, source.created_at))
FROM user_memories memory
JOIN messages source
  ON source.id = memory.source_message_id
  AND source.user_id = memory.user_id
  AND source.conversation_id = memory.source_conversation_id
  AND source.role = 'user'
  AND source.status = 'completed'
  AND source.deleted_at IS NULL
ON CONFLICT (memory_id, source_message_id) DO NOTHING;

-- An old AI row without surviving user authority must not remain recallable.
UPDATE user_memories memory
SET enabled = false,
    updated_at = GREATEST(memory.updated_at, clock_timestamp())
WHERE memory.source = 'ai'
  AND memory.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM user_memory_evidence evidence
    WHERE evidence.memory_id = memory.id
      AND evidence.evidence_role = 'user'
  );

CREATE TABLE user_memory_revisions (
  memory_id UUID NOT NULL,
  revision BIGINT NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  operation TEXT NOT NULL,
  old_content_hash TEXT NOT NULL,
  new_content_hash TEXT NOT NULL,
  prior_content_snapshot TEXT,
  actor_type TEXT NOT NULL,
  job_id UUID,
  result_code TEXT,
  purged_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (memory_id, revision),
  CONSTRAINT user_memory_revisions_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_revisions_revision_positive CHECK (revision >= 2),
  CONSTRAINT user_memory_revisions_operation_allowed
    CHECK (operation IN ('update', 'merge', 'supersede', 'delete', 'restore')),
  CONSTRAINT user_memory_revisions_old_hash_check
    CHECK (old_content_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT user_memory_revisions_new_hash_check
    CHECK (new_content_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT user_memory_revisions_snapshot_bounded CHECK (
    prior_content_snapshot IS NULL
    OR char_length(prior_content_snapshot) <= 2000
  ),
  CONSTRAINT user_memory_revisions_actor_allowed
    CHECK (actor_type IN ('user', 'worker', 'operator')),
  CONSTRAINT user_memory_revisions_result_code_sanitized CHECK (
    result_code IS NULL OR result_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  CONSTRAINT user_memory_revisions_purge_shape CHECK (
    (purged_at IS NULL AND result_code IS NULL)
    OR (
      purged_at IS NOT NULL
      AND result_code = 'ONLINE_PURGED'
      AND prior_content_snapshot IS NULL
      AND purged_at >= created_at
    )
  )
);

CREATE INDEX idx_user_memory_revisions_user_created
  ON user_memory_revisions(user_id, created_at DESC, memory_id, revision DESC);

CREATE FUNCTION user_memory_revision_append_only_guard()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF TG_OP = 'DELETE' THEN
    -- Account erase/hard parent removal owns the only allowed delete path.
    -- Direct history deletion still sees its canonical parent and is denied.
    IF NOT EXISTS (
      SELECT 1
      FROM user_memories memory
      WHERE memory.id = OLD.memory_id
        AND memory.user_id = OLD.user_id
    ) THEN
      RETURN OLD;
    END IF;
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_REVISION_APPEND_ONLY';
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
      NEW.job_id, NEW.created_at
    ) IS NOT DISTINCT FROM ROW(
      OLD.memory_id, OLD.revision, OLD.user_id, OLD.operation,
      OLD.old_content_hash, OLD.new_content_hash, OLD.actor_type,
      OLD.job_id, OLD.created_at
    )
  THEN
    RETURN NEW;
  END IF;

  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'MEMORY_REVISION_APPEND_ONLY';
END
$function$;

CREATE TRIGGER user_memory_revisions_append_only
BEFORE UPDATE OR DELETE ON user_memory_revisions
FOR EACH ROW EXECUTE FUNCTION user_memory_revision_append_only_guard();

CREATE TABLE user_memory_tombstones (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  memory_id UUID NOT NULL,
  content_hash TEXT NOT NULL,
  fact_key TEXT,
  source_conversation_id UUID,
  source_message_id UUID,
  source_content_hash TEXT,
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_tombstones_id_user_unique UNIQUE (id, user_id),
  CONSTRAINT user_memory_tombstones_memory_unique UNIQUE (memory_id),
  CONSTRAINT user_memory_tombstones_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_tombstones_content_hash_check
    CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT user_memory_tombstones_source_hash_check
    CHECK (
      source_content_hash IS NULL
      OR source_content_hash ~ '^[0-9a-f]{64}$'
    ),
  CONSTRAINT user_memory_tombstones_fact_key_bounded CHECK (
    fact_key IS NULL OR octet_length(fact_key) BETWEEN 1 AND 512
  ),
  CONSTRAINT user_memory_tombstones_source_shape CHECK (
    (source_message_id IS NULL AND source_content_hash IS NULL)
    OR (source_message_id IS NOT NULL AND source_content_hash IS NOT NULL)
  ),
  CONSTRAINT user_memory_tombstones_reason_allowed CHECK (
    reason IN ('user_delete', 'conversation_delete', 'project_delete', 'account_erase')
  )
);

CREATE INDEX idx_user_memory_tombstones_source
  ON user_memory_tombstones(user_id, source_message_id, source_content_hash)
  WHERE source_message_id IS NOT NULL;

CREATE TABLE user_memory_deletion_manifests (
  manifest_id UUID PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  memory_id UUID NOT NULL UNIQUE,
  tombstone_id UUID NOT NULL UNIQUE,
  content_hash TEXT NOT NULL,
  scope_generation BIGINT NOT NULL,
  visibility_epoch BIGINT NOT NULL,
  deleted_at TIMESTAMPTZ NOT NULL,
  result_code TEXT NOT NULL DEFAULT 'PENDING',
  purged_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_deletion_manifests_content_hash_check
    CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT user_memory_deletion_manifests_scope_generation_positive
    CHECK (scope_generation >= 1),
  CONSTRAINT user_memory_deletion_manifests_visibility_epoch_positive
    CHECK (visibility_epoch >= 1),
  CONSTRAINT user_memory_deletion_manifests_result_allowed
    CHECK (result_code IN ('PENDING', 'ONLINE_PURGED')),
  CONSTRAINT user_memory_deletion_manifests_result_shape CHECK (
    (result_code = 'PENDING' AND purged_at IS NULL)
    OR (
      result_code = 'ONLINE_PURGED'
      AND purged_at IS NOT NULL
      AND purged_at >= deleted_at
    )
  ),
  CONSTRAINT user_memory_deletion_manifests_created_order
    CHECK (created_at >= deleted_at)
);

CREATE INDEX idx_user_memory_deletion_manifests_created
  ON user_memory_deletion_manifests(created_at, manifest_id);

-- Extend the durable queue with an ID-only memory.deleted event and a
-- provider-free purge shape. Existing extract rows remain unchanged.
ALTER TABLE memory_outbox
  DROP CONSTRAINT memory_outbox_event_type_allowed,
  DROP CONSTRAINT memory_outbox_payload_id_only,
  DROP CONSTRAINT memory_outbox_attempts_bounded,
  ADD CONSTRAINT memory_outbox_event_type_allowed
    CHECK (event_type IN ('turn.completed', 'memory.deleted')),
  ADD CONSTRAINT memory_outbox_attempts_bounded CHECK (
    (
      (
        event_type = 'turn.completed'
        AND max_attempts BETWEEN 1 AND 32
      )
      OR (
        event_type = 'memory.deleted'
        AND max_attempts = 128
      )
    )
    AND attempt_count BETWEEN 0 AND max_attempts
  ),
  ADD CONSTRAINT memory_outbox_payload_id_only CHECK (
    (
      event_type = 'turn.completed'
      AND payload ? 'schemaMajor'
      AND payload ? 'conversationId'
      AND payload ? 'userMessageId'
      AND payload ? 'assistantMessageId'
      AND payload ? 'sourceHash'
      AND payload ? 'memoryScopeGeneration'
      AND payload ? 'visibilityEpoch'
      AND payload ? 'providerProfile'
      AND (payload - ARRAY[
        'schemaMajor', 'conversationId', 'userMessageId',
        'assistantMessageId', 'sourceHash', 'projectId',
        'memoryScopeGeneration', 'projectScopeGeneration',
        'visibilityEpoch', 'providerProfile'
      ]::TEXT[]) = '{}'::JSONB
      AND jsonb_typeof(payload->'providerProfile') = 'object'
      AND (payload->'providerProfile') ? 'providerSource'
      AND (payload->'providerProfile') ? 'providerId'
      AND (payload->'providerProfile') ? 'modelId'
      AND (payload->'providerProfile') ? 'profileHash'
      AND ((payload->'providerProfile') - ARRAY[
        'providerSource', 'providerId', 'modelId',
        'profileHash', 'providerConfigId'
      ]::TEXT[]) = '{}'::JSONB
    )
    OR (
      event_type = 'memory.deleted'
      AND payload ? 'schemaMajor'
      AND payload ? 'memoryId'
      AND payload ? 'tombstoneId'
      AND payload ? 'contentHash'
      AND payload ? 'scopeGeneration'
      AND payload ? 'visibilityEpoch'
      AND payload ? 'deletedAt'
      AND (payload - ARRAY[
        'schemaMajor', 'memoryId', 'tombstoneId', 'contentHash',
        'scopeGeneration', 'visibilityEpoch', 'deletedAt'
      ]::TEXT[]) = '{}'::JSONB
    )
  );

ALTER TABLE memory_jobs
  DROP CONSTRAINT memory_jobs_attempts_bounded,
  ADD COLUMN target_memory_id UUID,
  ADD COLUMN target_tombstone_id UUID,
  ALTER COLUMN source_conversation_id DROP NOT NULL,
  ALTER COLUMN source_message_id DROP NOT NULL,
  ALTER COLUMN assistant_message_id DROP NOT NULL,
  ALTER COLUMN source_hash DROP NOT NULL,
  ALTER COLUMN provider_source DROP NOT NULL,
  ALTER COLUMN provider_id DROP NOT NULL,
  ALTER COLUMN model_id DROP NOT NULL,
  ALTER COLUMN processing_profile DROP NOT NULL,
  ADD CONSTRAINT memory_jobs_target_memory_owner_fk
    FOREIGN KEY (target_memory_id, user_id)
    REFERENCES user_memories(id, user_id)
    ON DELETE CASCADE,
  ADD CONSTRAINT memory_jobs_target_tombstone_owner_fk
    FOREIGN KEY (target_tombstone_id, user_id)
    REFERENCES user_memory_tombstones(id, user_id)
    ON DELETE CASCADE,
  ADD CONSTRAINT memory_jobs_attempts_bounded CHECK (
    (
      (
        stage = 'purge'
        AND max_attempts = 128
      )
      OR (
        stage <> 'purge'
        AND max_attempts BETWEEN 1 AND 32
      )
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

CREATE INDEX idx_memory_jobs_purge_target
  ON memory_jobs(target_memory_id, job_id)
  WHERE stage = 'purge';

CREATE FUNCTION memory_upsert_global_manual(
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
  v_existing user_memories%ROWTYPE;
  v_epoch BIGINT;
  v_content_hash TEXT;
  v_importance SMALLINT;
  v_changed BOOLEAN;
BEGIN
  IF p_memory_id IS NULL OR p_user_id IS NULL
    OR p_memory_type NOT IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    )
    OR length(trim(p_content)) = 0
    OR char_length(p_content) > 2000
    OR length(trim(p_normalized_content)) = 0
    OR char_length(p_normalized_content) > 2000
    OR p_importance NOT BETWEEN 1 AND 5
    OR cardinality(p_tags) > 12
    OR NOT p_enabled
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_MANUAL_CANDIDATE_INVALID';
  END IF;

  IF (p_source_conversation_id IS NULL) <> (p_source_message_id IS NULL) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_MANUAL_SOURCE_INVALID';
  END IF;
  IF p_source_message_id IS NOT NULL AND NOT EXISTS (
    SELECT 1
    FROM messages source
    JOIN conversations conversation
      ON conversation.id = source.conversation_id
      AND conversation.user_id = p_user_id
      AND conversation.deleted_at IS NULL
    WHERE source.id = p_source_message_id
      AND source.conversation_id = p_source_conversation_id
      AND source.user_id = p_user_id
      AND source.role = 'user'
      AND source.status = 'completed'
      AND source.deleted_at IS NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_MANUAL_SOURCE_INVALID';
  END IF;

  INSERT INTO user_memory_state(user_id)
  VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_epoch
  FROM user_memory_state state
  WHERE state.user_id = p_user_id
  FOR UPDATE;

  v_content_hash := encode(sha256(convert_to(p_content, 'UTF8')), 'hex');
  SELECT memory.* INTO v_existing
  FROM user_memories memory
  WHERE memory.user_id = p_user_id
    AND memory.scope_type = 'global'
    AND memory.normalized_content = p_normalized_content
    AND memory.deleted_at IS NULL
  FOR UPDATE;

  IF NOT FOUND THEN
    INSERT INTO user_memories (
      id, user_id, memory_type, content, normalized_content, importance,
      tags, source, source_conversation_id, source_message_id, enabled,
      scope_type, revision, visibility_epoch, content_hash,
      authority_kind, extraction_profile_id
    ) VALUES (
      p_memory_id, p_user_id, p_memory_type, p_content,
      p_normalized_content, p_importance, p_tags, 'manual',
      p_source_conversation_id, p_source_message_id, true, 'global',
      1, v_epoch, v_content_hash, 'manual', NULL
    );
  ELSE
    v_importance := GREATEST(v_existing.importance, p_importance);
    v_changed := ROW(
      v_existing.memory_type,
      v_existing.content,
      v_existing.importance,
      v_existing.tags,
      v_existing.source,
      v_existing.source_conversation_id,
      v_existing.source_message_id,
      v_existing.enabled,
      v_existing.visibility_epoch,
      v_existing.content_hash,
      v_existing.authority_kind,
      v_existing.extraction_profile_id
    ) IS DISTINCT FROM ROW(
      p_memory_type,
      p_content,
      v_importance,
      p_tags,
      'manual'::TEXT,
      p_source_conversation_id,
      p_source_message_id,
      true,
      v_epoch,
      v_content_hash,
      'manual'::TEXT,
      NULL::TEXT
    );
    IF v_changed THEN
      INSERT INTO user_memory_revisions (
        memory_id, revision, user_id, operation, old_content_hash,
        new_content_hash, prior_content_snapshot, actor_type
      ) VALUES (
        v_existing.id, v_existing.revision + 1, p_user_id, 'update',
        v_existing.content_hash, v_content_hash, v_existing.content, 'user'
      );
      UPDATE user_memories memory
      SET memory_type = p_memory_type,
          content = p_content,
          importance = v_importance,
          tags = p_tags,
          source = 'manual',
          source_conversation_id = p_source_conversation_id,
          source_message_id = p_source_message_id,
          enabled = true,
          revision = memory.revision + 1,
          visibility_epoch = v_epoch,
          content_hash = v_content_hash,
          authority_kind = 'manual',
          extraction_profile_id = NULL,
          updated_at = clock_timestamp()
      WHERE memory.id = v_existing.id;
    END IF;
  END IF;

  RETURN QUERY
  SELECT
    memory.id,
    memory.user_id,
    memory.memory_type,
    memory.content,
    memory.normalized_content,
    memory.importance,
    to_json(memory.tags)::TEXT,
    memory.source,
    memory.source_conversation_id,
    memory.source_message_id,
    memory.enabled,
    memory.last_used_at,
    memory.created_at,
    memory.updated_at,
    memory.deleted_at
  FROM user_memories memory
  WHERE memory.user_id = p_user_id
    AND memory.scope_type = 'global'
    AND memory.normalized_content = p_normalized_content
    AND memory.deleted_at IS NULL;
END
$function$;

CREATE FUNCTION memory_update_global_manual(
  p_memory_id UUID,
  p_user_id UUID,
  p_memory_type TEXT,
  p_content TEXT,
  p_normalized_content TEXT,
  p_importance SMALLINT,
  p_tags TEXT[],
  p_enabled BOOLEAN
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
  v_existing user_memories%ROWTYPE;
  v_epoch BIGINT;
  v_content_hash TEXT;
  v_changed BOOLEAN;
BEGIN
  IF p_memory_id IS NULL OR p_user_id IS NULL
    OR p_memory_type NOT IN (
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
      MESSAGE = 'MEMORY_MANUAL_CANDIDATE_INVALID';
  END IF;

  INSERT INTO user_memory_state(user_id)
  VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_epoch
  FROM user_memory_state state
  WHERE state.user_id = p_user_id
  FOR UPDATE;

  SELECT memory.* INTO v_existing
  FROM user_memories memory
  WHERE memory.id = p_memory_id
    AND memory.user_id = p_user_id
    AND memory.scope_type = 'global'
    AND memory.deleted_at IS NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RETURN;
  END IF;

  v_content_hash := encode(sha256(convert_to(p_content, 'UTF8')), 'hex');
  v_changed := ROW(
    v_existing.memory_type,
    v_existing.content,
    v_existing.normalized_content,
    v_existing.importance,
    v_existing.tags,
    v_existing.enabled,
    v_existing.visibility_epoch,
    v_existing.content_hash,
    v_existing.authority_kind,
    v_existing.extraction_profile_id
  ) IS DISTINCT FROM ROW(
    p_memory_type,
    p_content,
    p_normalized_content,
    p_importance,
    p_tags,
    p_enabled,
    v_epoch,
    v_content_hash,
    'manual'::TEXT,
    NULL::TEXT
  );

  IF v_changed THEN
    INSERT INTO user_memory_revisions (
      memory_id, revision, user_id, operation, old_content_hash,
      new_content_hash, prior_content_snapshot, actor_type
    ) VALUES (
      v_existing.id, v_existing.revision + 1, p_user_id, 'update',
      v_existing.content_hash, v_content_hash, v_existing.content, 'user'
    );
    UPDATE user_memories memory
    SET memory_type = p_memory_type,
        content = p_content,
        normalized_content = p_normalized_content,
        importance = p_importance,
        tags = p_tags,
        enabled = p_enabled,
        revision = memory.revision + 1,
        visibility_epoch = v_epoch,
        content_hash = v_content_hash,
        authority_kind = 'manual',
        extraction_profile_id = NULL,
        updated_at = clock_timestamp()
    WHERE memory.id = v_existing.id;
  END IF;

  RETURN QUERY
  SELECT
    memory.id,
    memory.user_id,
    memory.memory_type,
    memory.content,
    memory.normalized_content,
    memory.importance,
    to_json(memory.tags)::TEXT,
    memory.source,
    memory.source_conversation_id,
    memory.source_message_id,
    memory.enabled,
    memory.last_used_at,
    memory.created_at,
    memory.updated_at,
    memory.deleted_at
  FROM user_memories memory
  WHERE memory.id = p_memory_id
    AND memory.user_id = p_user_id
    AND memory.scope_type = 'global'
    AND memory.deleted_at IS NULL;
END
$function$;

CREATE FUNCTION memory_delete_global(
  p_user_id UUID,
  p_memory_id UUID,
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
  IF p_user_id IS NULL OR p_memory_id IS NULL OR p_event_id IS NULL
    OR p_job_id IS NULL OR p_tombstone_id IS NULL OR p_manifest_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_DELETE_INPUT_INVALID';
  END IF;

  INSERT INTO user_memory_state(user_id)
  VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_epoch
  FROM user_memory_state state
  WHERE state.user_id = p_user_id
  FOR UPDATE;

  SELECT memory.* INTO v_memory
  FROM user_memories memory
  WHERE memory.id = p_memory_id
    AND memory.user_id = p_user_id
    AND memory.scope_type = 'global'
    AND memory.deleted_at IS NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RETURN false;
  END IF;

  SELECT evidence.source_content_hash INTO v_source_hash
  FROM user_memory_evidence evidence
  WHERE evidence.memory_id = v_memory.id
    AND evidence.source_message_id = v_memory.source_message_id
    AND evidence.evidence_role = 'user'
  LIMIT 1;

  INSERT INTO user_memory_revisions (
    memory_id, revision, user_id, operation, old_content_hash,
    new_content_hash, prior_content_snapshot, actor_type
  ) VALUES (
    v_memory.id, v_memory.revision + 1, p_user_id, 'delete',
    v_memory.content_hash, v_memory.content_hash, v_memory.content, 'user'
  );

  UPDATE user_memories memory
  SET enabled = false,
      deleted_at = v_now,
      revision = memory.revision + 1,
      updated_at = v_now
  WHERE memory.id = v_memory.id;

  INSERT INTO user_memory_tombstones (
    id, user_id, memory_id, content_hash,
    source_conversation_id, source_message_id, source_content_hash,
    reason, created_at
  ) VALUES (
    p_tombstone_id, p_user_id, v_memory.id, v_memory.content_hash,
    v_memory.source_conversation_id,
    CASE WHEN v_source_hash IS NULL THEN NULL ELSE v_memory.source_message_id END,
    v_source_hash,
    'user_delete', v_now
  );

  INSERT INTO user_memory_deletion_manifests (
    manifest_id, event_id, user_id, memory_id, tombstone_id,
    content_hash, scope_generation, visibility_epoch,
    deleted_at, created_at
  ) VALUES (
    p_manifest_id, p_event_id, p_user_id, v_memory.id, p_tombstone_id,
    v_memory.content_hash, v_memory.scope_generation, v_epoch,
    v_now, v_now
  );

  INSERT INTO memory_outbox (
    event_id, user_id, event_schema_major, event_type,
    aggregate_id, visibility_epoch, payload, created_at,
    updated_at, available_at, max_attempts
  ) VALUES (
    p_event_id, p_user_id, 2, 'memory.deleted',
    v_memory.id, v_epoch,
    jsonb_build_object(
      'schemaMajor', 2,
      'memoryId', v_memory.id::TEXT,
      'tombstoneId', p_tombstone_id::TEXT,
      'contentHash', v_memory.content_hash,
      'scopeGeneration', v_memory.scope_generation,
      'visibilityEpoch', v_epoch,
      'deletedAt', v_now
    ),
    v_now, v_now, v_now, 128
  );

  INSERT INTO memory_jobs (
    job_id, user_id, event_id, stage, idempotency_key,
    scope_generation, visibility_epoch, target_memory_id,
    target_tombstone_id, created_at, updated_at, available_at,
    max_attempts
  ) VALUES (
    p_job_id, p_user_id, p_event_id, 'purge',
    'memory:purge:v2:' || v_memory.id::TEXT,
    v_memory.scope_generation, v_epoch, v_memory.id,
    p_tombstone_id, v_now, v_now, v_now, 128
  );

  RETURN true;
END
$function$;

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
  v_visibility_epoch BIGINT;
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

  INSERT INTO user_memory_state(user_id)
  VALUES (p_user_id)
  ON CONFLICT (user_id) DO NOTHING;
  SELECT state.visibility_epoch INTO v_visibility_epoch
  FROM user_memory_state state
  WHERE state.user_id = p_user_id
  FOR UPDATE;

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
    'visibilityEpoch', v_visibility_epoch,
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
    v_visibility_epoch,
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
    v_visibility_epoch
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

  IF v_job.stage <> 'extract' THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_STAGE_INVALID';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM user_memory_state state
    WHERE state.user_id = v_job.user_id
      AND state.visibility_epoch = v_job.visibility_epoch
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_VISIBILITY_EPOCH_DRIFT';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM user_memory_tombstones tombstone
    WHERE tombstone.user_id = v_job.user_id
      AND tombstone.source_message_id = v_job.source_message_id
      AND tombstone.source_content_hash = v_job.source_hash
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_SOURCE_TOMBSTONED';
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
  v_existing user_memories%ROWTYPE;
  v_epoch BIGINT;
  v_content_hash TEXT;
  v_importance SMALLINT;
  v_changed BOOLEAN;
BEGIN
  PERFORM 1
  FROM memory_worker_hydrate_capture(p_job_id, p_worker_id, p_lease_token);

  SELECT job.* INTO v_job
  FROM memory_jobs job
  WHERE job.job_id = p_job_id
    AND job.stage = 'extract'
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  SELECT state.visibility_epoch INTO v_epoch
  FROM user_memory_state state
  WHERE state.user_id = v_job.user_id
  FOR UPDATE;
  IF NOT FOUND OR v_epoch <> v_job.visibility_epoch THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_VISIBILITY_EPOCH_DRIFT';
  END IF;

  IF p_memory_id IS NULL
    OR p_memory_type NOT IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    )
    OR length(trim(p_content)) = 0
    OR char_length(p_content) > 2000
    OR length(trim(p_normalized_content)) = 0
    OR char_length(p_normalized_content) > 2000
    OR p_importance NOT BETWEEN 1 AND 5
    OR p_tags IS NULL
    OR cardinality(p_tags) > 12
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_CANDIDATE_INVALID';
  END IF;

  v_content_hash := encode(sha256(convert_to(p_content, 'UTF8')), 'hex');
  IF EXISTS (
    SELECT 1
    FROM user_memory_tombstones tombstone
    WHERE tombstone.user_id = v_job.user_id
      AND tombstone.content_hash = v_content_hash
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_CANDIDATE_TOMBSTONED';
  END IF;

  SELECT memory.* INTO v_existing
  FROM user_memories memory
  WHERE memory.user_id = v_job.user_id
    AND memory.scope_type = 'global'
    AND memory.normalized_content = p_normalized_content
    AND memory.deleted_at IS NULL
  FOR UPDATE;

  -- Exact automatic candidates cannot rewrite explicit user authority. PR5
  -- will add semantic conflict suggestions; PR4 remains fail-closed NOOP.
  IF FOUND AND v_existing.authority_kind <> 'auto' THEN
    RETURN QUERY
    SELECT
      memory.id, memory.user_id, memory.memory_type, memory.content,
      memory.normalized_content, memory.importance,
      to_json(memory.tags)::TEXT, memory.source,
      memory.source_conversation_id, memory.source_message_id,
      memory.enabled, memory.last_used_at, memory.created_at,
      memory.updated_at, memory.deleted_at
    FROM user_memories memory
    WHERE memory.id = v_existing.id;
    RETURN;
  END IF;

  IF NOT FOUND THEN
    INSERT INTO user_memories (
      id, user_id, memory_type, content, normalized_content, importance,
      tags, source, source_conversation_id, source_message_id, enabled,
      scope_type, revision, visibility_epoch, content_hash,
      authority_kind, extraction_profile_id
    ) VALUES (
      p_memory_id, v_job.user_id, p_memory_type, p_content,
      p_normalized_content, p_importance, p_tags, 'ai',
      v_job.source_conversation_id, v_job.source_message_id, true,
      'global', 1, v_epoch, v_content_hash, 'auto',
      v_job.processing_profile
    )
    RETURNING * INTO v_existing;
  ELSE
    v_importance := GREATEST(v_existing.importance, p_importance);
    v_changed := ROW(
      v_existing.memory_type,
      v_existing.content,
      v_existing.importance,
      v_existing.tags,
      v_existing.source,
      v_existing.source_conversation_id,
      v_existing.source_message_id,
      v_existing.enabled,
      v_existing.visibility_epoch,
      v_existing.content_hash,
      v_existing.authority_kind,
      v_existing.extraction_profile_id
    ) IS DISTINCT FROM ROW(
      p_memory_type,
      p_content,
      v_importance,
      p_tags,
      'ai'::TEXT,
      v_job.source_conversation_id,
      v_job.source_message_id,
      true,
      v_epoch,
      v_content_hash,
      'auto'::TEXT,
      v_job.processing_profile
    );
    IF v_changed THEN
      INSERT INTO user_memory_revisions (
        memory_id, revision, user_id, operation, old_content_hash,
        new_content_hash, prior_content_snapshot, actor_type, job_id
      ) VALUES (
        v_existing.id, v_existing.revision + 1, v_job.user_id,
        'update', v_existing.content_hash, v_content_hash,
        v_existing.content, 'worker', v_job.job_id
      );
      UPDATE user_memories memory
      SET memory_type = p_memory_type,
          content = p_content,
          importance = v_importance,
          tags = p_tags,
          source = 'ai',
          source_conversation_id = v_job.source_conversation_id,
          source_message_id = v_job.source_message_id,
          enabled = true,
          revision = memory.revision + 1,
          visibility_epoch = v_epoch,
          content_hash = v_content_hash,
          authority_kind = 'auto',
          extraction_profile_id = v_job.processing_profile,
          updated_at = clock_timestamp()
      WHERE memory.id = v_existing.id
      RETURNING memory.* INTO v_existing;
    END IF;
  END IF;

  INSERT INTO user_memory_evidence (
    memory_id, source_message_id, user_id, source_conversation_id,
    evidence_role, source_content_hash, observed_at
  )
  SELECT
    v_existing.id, source.id, v_job.user_id, source.conversation_id,
    'user', v_job.source_hash, COALESCE(source.completed_at, source.created_at)
  FROM messages source
  WHERE source.id = v_job.source_message_id
    AND source.conversation_id = v_job.source_conversation_id
    AND source.user_id = v_job.user_id
    AND source.role = 'user'
    AND source.status = 'completed'
    AND source.deleted_at IS NULL
  ON CONFLICT (memory_id, source_message_id) DO NOTHING;
  IF NOT FOUND AND NOT EXISTS (
    SELECT 1
    FROM user_memory_evidence evidence
    WHERE evidence.memory_id = v_existing.id
      AND evidence.source_message_id = v_job.source_message_id
      AND evidence.user_id = v_job.user_id
      AND evidence.source_content_hash = v_job.source_hash
      AND evidence.evidence_role = 'user'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_CAPTURE_SOURCE_DRIFT';
  END IF;

  RETURN QUERY
  SELECT
    memory.id, memory.user_id, memory.memory_type, memory.content,
    memory.normalized_content, memory.importance,
    to_json(memory.tags)::TEXT, memory.source,
    memory.source_conversation_id, memory.source_message_id,
    memory.enabled, memory.last_used_at, memory.created_at,
    memory.updated_at, memory.deleted_at
  FROM user_memories memory
  WHERE memory.id = v_existing.id;
END
$function$;

CREATE FUNCTION memory_worker_purge_memory(
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
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  PERFORM 1
  FROM user_memory_state state
  WHERE state.user_id = v_job.user_id
    AND state.visibility_epoch = v_job.visibility_epoch
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_VISIBILITY_EPOCH_DRIFT';
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
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_PURGE_TARGET_DRIFT';
  END IF;

  DELETE FROM user_memory_evidence evidence
  WHERE evidence.memory_id = v_memory.id
    AND evidence.user_id = v_job.user_id;

  UPDATE user_memory_revisions revision
  SET prior_content_snapshot = NULL,
      result_code = 'ONLINE_PURGED',
      purged_at = v_now
  WHERE revision.memory_id = v_memory.id
    AND revision.user_id = v_job.user_id
    AND revision.prior_content_snapshot IS NOT NULL
    AND revision.purged_at IS NULL;

  UPDATE user_memories memory
  SET content = '',
      normalized_content = '',
      tags = '{}',
      source_conversation_id = NULL,
      source_message_id = NULL,
      extraction_profile_id = NULL,
      updated_at = GREATEST(memory.updated_at, v_now)
  WHERE memory.id = v_memory.id
    AND (
      memory.content <> ''
      OR memory.normalized_content <> ''
      OR cardinality(memory.tags) > 0
      OR memory.source_conversation_id IS NOT NULL
      OR memory.source_message_id IS NOT NULL
      OR memory.extraction_profile_id IS NOT NULL
    );

  UPDATE user_memory_deletion_manifests manifest
  SET result_code = 'ONLINE_PURGED',
      purged_at = v_now
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
    'memory_upsert_global_manual(uuid,uuid,text,text,text,smallint,text[],uuid,uuid,boolean)',
    'memory_update_global_manual(uuid,uuid,text,text,text,smallint,text[],boolean)',
    'memory_delete_global(uuid,uuid,uuid,uuid,uuid,uuid)',
    'memory_append_turn_completed_event(uuid,uuid,uuid,uuid,uuid,uuid,text,text,text,smallint)',
    'memory_worker_hydrate_capture(uuid,uuid,uuid)',
    'memory_worker_apply_capture_candidate(uuid,uuid,uuid,uuid,text,text,text,smallint,text[])',
    'memory_worker_purge_memory(uuid,uuid,uuid)'
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

GRANT SELECT, INSERT, UPDATE ON user_memory_state
  TO memory_runtime_owner;
GRANT SELECT, INSERT, DELETE ON user_memory_evidence
  TO memory_runtime_owner;
GRANT SELECT, INSERT, UPDATE ON user_memory_revisions
  TO memory_runtime_owner;
GRANT SELECT, INSERT ON user_memory_tombstones
  TO memory_runtime_owner;
GRANT SELECT, INSERT, UPDATE ON user_memory_deletion_manifests
  TO memory_runtime_owner;

ALTER FUNCTION memory_upsert_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_update_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_delete_global(UUID, UUID, UUID, UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_purge_memory(UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;

REVOKE ALL ON
  user_memory_state,
  user_memory_evidence,
  user_memory_revisions,
  user_memory_tombstones,
  user_memory_deletion_manifests
FROM PUBLIC, go_api_runtime, memory_worker_runtime;

REVOKE DELETE ON user_memories FROM go_api_runtime;

REVOKE ALL ON FUNCTION user_memory_revision_append_only_guard() FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_upsert_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_update_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_delete_global(
  UUID, UUID, UUID, UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_purge_memory(UUID, UUID, UUID) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION memory_upsert_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN
), memory_update_global_manual(
  UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN
), memory_delete_global(
  UUID, UUID, UUID, UUID, UUID, UUID
) TO go_api_runtime;

GRANT EXECUTE ON FUNCTION memory_worker_purge_memory(UUID, UUID, UUID)
TO memory_worker_runtime;

DO $owner_create_revocation$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner',
    current_schema()
  );
END
$owner_create_revocation$;
