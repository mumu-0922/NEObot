DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM memory_user_actions)
    OR EXISTS (SELECT 1 FROM memory_user_action_targets)
    OR EXISTS (SELECT 1 FROM message_memory_activities)
    OR EXISTS (SELECT 1 FROM message_memory_usages)
    OR EXISTS (
      SELECT 1 FROM user_memory_revisions
      WHERE prior_snapshot_schema_major IS NOT NULL
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_ACTION_ROLLBACK_REQUIRES_EMPTY_HISTORY';
  END IF;
  IF EXISTS (
    SELECT 1 FROM user_memories WHERE source = 'direct_user'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_ACTION_ROLLBACK_REQUIRES_V1_SOURCE';
  END IF;
END
$guard$;

REVOKE EXECUTE ON FUNCTION memory_hydrate_direct_user_action(
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
) FROM go_api_runtime;

DROP TRIGGER memory_jobs_dead_letter_activity ON memory_jobs;
DROP TRIGGER user_memory_review_suggestions_activity
  ON user_memory_review_suggestions;

DROP FUNCTION memory_dead_letter_activity_trigger();
DROP FUNCTION memory_review_activity_trigger();
DROP FUNCTION memory_undo_activity(
  UUID, UUID, BIGINT, UUID, UUID, UUID, UUID
);
DROP FUNCTION memory_list_message_usages(UUID, UUID);
DROP FUNCTION memory_list_activities(UUID, UUID, INTEGER);
DROP FUNCTION memory_record_message_usages(UUID, UUID, UUID, JSONB);
DROP FUNCTION memory_apply_direct_user_action(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID,
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, TEXT, TEXT,
  TEXT, SMALLINT, TEXT[], TEXT, TEXT, DOUBLE PRECISION, JSONB, TEXT, TEXT
);
DROP FUNCTION memory_delete_direct_scoped(
  UUID, UUID, BIGINT, UUID, UUID, UUID, UUID
);
DROP FUNCTION memory_hydrate_direct_user_action(UUID, UUID, UUID, UUID);
DROP FUNCTION memory_next_activity_ordinal(UUID);

DROP TABLE message_memory_usages;
DROP TABLE message_memory_activities;
DROP TABLE memory_user_action_targets;
DROP TABLE memory_user_actions;

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

  PERFORM 1
  FROM user_memory_state state
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
  SET result_code = 'ONLINE_PURGED', purged_at = v_now
  WHERE manifest.event_id = v_job.event_id
    AND manifest.user_id = v_job.user_id
    AND manifest.memory_id = v_memory.id
    AND manifest.tombstone_id = v_job.target_tombstone_id
    AND manifest.result_code = 'PENDING';
  RETURN true;
END
$function$;

DROP TRIGGER user_memory_revisions_append_only ON user_memory_revisions;
DROP FUNCTION user_memory_revision_append_only_guard();

ALTER TABLE user_memory_revisions
  DROP CONSTRAINT user_memory_revisions_full_snapshot_shape,
  DROP CONSTRAINT user_memory_revisions_snapshot_schema_supported,
  DROP COLUMN prior_snapshot_schema_major,
  DROP COLUMN prior_memory_type,
  DROP COLUMN prior_normalized_content,
  DROP COLUMN prior_importance,
  DROP COLUMN prior_tags,
  DROP COLUMN prior_source,
  DROP COLUMN prior_source_conversation_id,
  DROP COLUMN prior_source_message_id,
  DROP COLUMN prior_enabled,
  DROP COLUMN prior_last_used_at,
  DROP COLUMN prior_scope_type,
  DROP COLUMN prior_project_id,
  DROP COLUMN prior_scope_conversation_id,
  DROP COLUMN prior_scope_generation,
  DROP COLUMN prior_visibility_epoch,
  DROP COLUMN prior_authority_kind,
  DROP COLUMN prior_extraction_profile_id,
  DROP COLUMN prior_lifecycle_status,
  DROP COLUMN prior_subject_key,
  DROP COLUMN prior_fact_key,
  DROP COLUMN prior_confidence,
  DROP COLUMN prior_observed_at,
  DROP COLUMN prior_valid_from,
  DROP COLUMN prior_valid_to,
  DROP COLUMN prior_expires_at,
  DROP COLUMN prior_superseded_by_memory_id,
  DROP COLUMN prior_sensitivity,
  DROP COLUMN prior_temporal_basis,
  DROP COLUMN prior_temporal_parser_version;

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
    ERRCODE = '55000', MESSAGE = 'MEMORY_REVISION_APPEND_ONLY';
END
$function$;

CREATE TRIGGER user_memory_revisions_append_only
BEFORE UPDATE OR DELETE ON user_memory_revisions
FOR EACH ROW EXECUTE FUNCTION user_memory_revision_append_only_guard();

ALTER TABLE user_memories
  DROP CONSTRAINT user_memories_source_allowed,
  ADD CONSTRAINT user_memories_source_allowed
    CHECK (source IN ('manual', 'ai'));

DO $harden_functions$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.user_memory_revision_append_only_guard() SET search_path TO %I, pg_catalog, pg_temp',
    schema_name, schema_name
  );
  EXECUTE format(
    'ALTER FUNCTION %I.memory_worker_purge_memory(uuid,uuid,uuid) SET search_path TO %I, pg_catalog, pg_temp',
    schema_name, schema_name
  );
END
$harden_functions$;
