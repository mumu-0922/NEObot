DO $guard_auto_capture_rollback$
BEGIN
  IF EXISTS (
    SELECT 1 FROM user_memory_review_decisions
    WHERE decision_kind = 'auto_accept' OR result_code = 'AUTO_CAPTURED'
  ) OR EXISTS (
    SELECT 1 FROM user_memory_review_suggestions
    WHERE decision_kind = 'auto_accept' OR result_code = 'AUTO_CAPTURED'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'MEMORY_AUTO_CAPTURE_ROLLBACK_REQUIRES_NO_PROMOTIONS';
  END IF;
END
$guard_auto_capture_rollback$;

REVOKE ALL ON FUNCTION memory_worker_promote_capture_candidates(UUID, UUID, UUID)
  FROM memory_worker_runtime;
DROP FUNCTION memory_worker_promote_capture_candidates(UUID, UUID, UUID);

ALTER TABLE user_memory_review_decisions
  DROP CONSTRAINT user_memory_review_decisions_kind_allowed,
  DROP CONSTRAINT user_memory_review_decisions_result_shape,
  ADD CONSTRAINT user_memory_review_decisions_kind_allowed CHECK (
    decision_kind IN (
      'keep_current', 'accept_new', 'edit_merge', 'keep_both', 'reject'
    )
  ),
  ADD CONSTRAINT user_memory_review_decisions_result_shape CHECK (
    (decision_kind IN ('keep_current', 'reject')
      AND result_memory_id IS NULL AND result_memory_revision IS NULL)
    OR (decision_kind IN ('accept_new', 'edit_merge', 'keep_both')
      AND result_memory_id IS NOT NULL AND result_memory_revision >= 1)
  );

ALTER TABLE user_memory_review_suggestions
  DROP CONSTRAINT user_memory_review_suggestions_decision_kind_allowed,
  DROP CONSTRAINT user_memory_review_suggestions_plaintext_shape,
  DROP CONSTRAINT user_memory_review_suggestions_state_shape,
  ADD CONSTRAINT user_memory_review_suggestions_decision_kind_allowed CHECK (
    decision_kind IS NULL OR decision_kind IN (
      'keep_current', 'accept_new', 'edit_merge', 'keep_both', 'reject'
    )
  ),
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
