DO $guard_memory_portability_rollback$
BEGIN
  IF EXISTS (SELECT 1 FROM memory_import_batches)
    OR EXISTS (SELECT 1 FROM memory_deletion_replay_entries)
    OR EXISTS (SELECT 1 FROM user_memories WHERE source = 'import')
    OR EXISTS (SELECT 1 FROM user_memory_revisions WHERE actor_type = 'import')
  THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_PORTABILITY_ROLLBACK_REQUIRES_NO_IMPORT_HISTORY';
  END IF;
END
$guard_memory_portability_rollback$;

DROP FUNCTION memory_portability_rebuild_projections();
DROP FUNCTION memory_portability_replay_deletion(JSONB);
DROP FUNCTION memory_portability_export_deletions();
DROP FUNCTION memory_portability_complete_import(UUID, UUID, INTEGER, INTEGER);
DROP FUNCTION memory_portability_finalize_memory(UUID, UUID, UUID, TEXT, UUID);
DROP FUNCTION memory_portability_add_revision(UUID, UUID, UUID, JSONB, TEXT, UUID, UUID, BIGINT, UUID);
DROP FUNCTION memory_portability_add_memory(UUID, UUID, UUID, JSONB, TEXT, TEXT, UUID, UUID, BIGINT);
DROP FUNCTION memory_portability_create_project(UUID, UUID, UUID, TEXT, TEXT, TEXT);
DROP FUNCTION memory_portability_begin_import(UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, INTEGER, INTEGER, INTEGER);
DROP FUNCTION memory_portability_resolve_memory(UUID, TEXT, TEXT, TEXT, TEXT, UUID, UUID, BIGINT);
DROP FUNCTION memory_portability_completed_import(UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT);
DROP FUNCTION memory_portability_resolve_conversation(UUID, UUID);
DROP FUNCTION memory_portability_resolve_project(UUID, UUID);
DROP FUNCTION memory_portability_export_records(UUID, BOOLEAN);
DROP FUNCTION memory_portability_authority_state(UUID);

DROP TABLE memory_import_batches;
DROP TABLE memory_deletion_replay_entries;

ALTER TABLE user_memories
  DROP CONSTRAINT user_memories_source_allowed,
  ADD CONSTRAINT user_memories_source_allowed
    CHECK (source IN ('manual', 'ai', 'direct_user'));

ALTER TABLE user_memory_revisions
  DROP CONSTRAINT user_memory_revisions_actor_allowed,
  ADD CONSTRAINT user_memory_revisions_actor_allowed
    CHECK (actor_type IN ('user', 'worker', 'operator')),
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
