DO $guard_governance_rollback$
BEGIN
  IF EXISTS (SELECT 1 FROM user_memory_review_decisions) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_GOVERNANCE_ROLLBACK_REQUIRES_NO_DECISIONS';
  END IF;
  IF EXISTS (
    SELECT 1 FROM user_memory_review_suggestions
    WHERE status = 'accepted' OR decision_kind IS NOT NULL
      OR result_memory_id IS NOT NULL
      OR result_code IN (
        'USER_KEPT_CURRENT', 'USER_REJECTED', 'USER_ACCEPTED',
        'USER_EDIT_MERGED', 'USER_KEPT_BOTH'
      )
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_GOVERNANCE_ROLLBACK_REQUIRES_LEGACY_REVIEWS';
  END IF;
  IF EXISTS (
    SELECT 1 FROM user_memory_revisions WHERE operation = 'move'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_GOVERNANCE_ROLLBACK_REQUIRES_NO_MOVE_REVISIONS';
  END IF;
END
$guard_governance_rollback$;

DROP FUNCTION memory_governance_list_message_activities(UUID, UUID, INTEGER);
DROP FUNCTION memory_governance_decide_review(UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT, TEXT);
DROP FUNCTION memory_governance_memory_detail(UUID, UUID);
DROP FUNCTION memory_governance_delete_memory(UUID, UUID, BIGINT, UUID, UUID, UUID, UUID);
DROP FUNCTION memory_governance_update_memory(UUID, UUID, BIGINT, TEXT, TEXT, TEXT, SMALLINT, TEXT[], TEXT, UUID, UUID, TEXT);
DROP FUNCTION memory_governance_create_memory(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], TEXT, UUID, UUID, TEXT);
DROP FUNCTION memory_governance_update_conversation_policy(UUID, UUID, BIGINT, UUID, TEXT, TEXT);
DROP FUNCTION memory_governance_get_conversation_policy(UUID, UUID);
DROP FUNCTION memory_governance_update_project(UUID, UUID, BIGINT, TEXT, TEXT, TEXT);
DROP FUNCTION memory_governance_create_project(UUID, UUID, TEXT, TEXT);
DROP FUNCTION memory_governance_snapshot(UUID);
DROP FUNCTION memory_governance_update_global_legacy(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN);
DROP FUNCTION memory_governance_upsert_global_legacy(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN);
DROP FUNCTION memory_governance_memory_json(UUID, UUID);
DROP FUNCTION memory_governance_policy_json(UUID, UUID);
DROP FUNCTION memory_governance_project_json(UUID, UUID);
DROP FUNCTION memory_governance_append_revision(user_memories, TEXT, TEXT);
DROP FUNCTION memory_governance_scope_generation(UUID, TEXT, UUID, UUID);
DROP FUNCTION memory_governance_classify_sensitivity(TEXT);
DROP FUNCTION memory_governance_is_secret(TEXT);
DROP FUNCTION memory_governance_epoch_millis(TIMESTAMPTZ);

DROP TABLE user_memory_review_decisions;

REVOKE INSERT, UPDATE ON projects FROM memory_runtime_owner;

GRANT EXECUTE ON FUNCTION
  memory_upsert_global_manual(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], UUID, UUID, BOOLEAN),
  memory_update_global_manual(UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[], BOOLEAN)
TO go_api_runtime;

ALTER TABLE user_memory_revisions
  DROP CONSTRAINT user_memory_revisions_operation_allowed,
  ADD CONSTRAINT user_memory_revisions_operation_allowed
    CHECK (operation IN (
      'update', 'merge', 'supersede', 'delete', 'restore'
    ));

ALTER TABLE user_memory_review_suggestions
  DROP CONSTRAINT user_memory_review_suggestions_status_allowed,
  DROP CONSTRAINT user_memory_review_suggestions_plaintext_shape,
  DROP CONSTRAINT user_memory_review_suggestions_state_shape,
  DROP CONSTRAINT user_memory_review_suggestions_result_memory_owner_fk,
  DROP CONSTRAINT user_memory_review_suggestions_decision_kind_allowed,
  DROP COLUMN result_memory_id,
  DROP COLUMN decision_kind,
  ADD CONSTRAINT user_memory_review_suggestions_status_allowed
    CHECK (status IN ('shadow', 'pending', 'rejected', 'expired')),
  ADD CONSTRAINT user_memory_review_suggestions_plaintext_shape CHECK (
    (
      status IN ('shadow', 'pending')
      AND candidate_content IS NOT NULL
      AND length(trim(candidate_content)) > 0
      AND char_length(candidate_content) <= 2000
      AND normalized_content IS NOT NULL
      AND length(trim(normalized_content)) > 0
      AND char_length(normalized_content) <= 2000
      AND purged_at IS NULL AND result_code IS NULL
    )
    OR (
      status IN ('rejected', 'expired')
      AND candidate_content IS NULL AND normalized_content IS NULL
      AND tags = '{}'::TEXT[] AND subject_key IS NULL AND fact_key IS NULL
      AND purged_at IS NOT NULL
      AND result_code IN (
        'SECRET_REJECTED', 'SENSITIVE_DISABLED', 'MODEL_REJECTED',
        'TOMBSTONED', 'PLAINTEXT_EXPIRED'
      )
    )
  ),
  ADD CONSTRAINT user_memory_review_suggestions_state_shape CHECK (
    (status = 'shadow' AND disposition = 'shadow' AND decided_at IS NULL)
    OR (status = 'pending' AND disposition = 'review' AND decided_at IS NULL)
    OR (status = 'rejected' AND disposition = 'rejected' AND decided_at IS NOT NULL)
    OR (status = 'expired' AND disposition IN ('shadow', 'review')
      AND decided_at IS NOT NULL)
  );
