-- Memory v2 production auto-capture promotion. The existing candidate router
-- remains authoritative: only current SHADOW_ADD proposals may cross into
-- canonical Memory, and the governance acceptance function owns the shared
-- canonical/evidence write path.

ALTER TABLE user_memory_review_suggestions
  DROP CONSTRAINT user_memory_review_suggestions_decision_kind_allowed,
  DROP CONSTRAINT user_memory_review_suggestions_plaintext_shape,
  DROP CONSTRAINT user_memory_review_suggestions_state_shape,
  ADD CONSTRAINT user_memory_review_suggestions_decision_kind_allowed CHECK (
    decision_kind IS NULL OR decision_kind IN (
      'keep_current', 'accept_new', 'edit_merge', 'keep_both', 'reject',
      'auto_accept'
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
        'USER_KEPT_BOTH', 'AUTO_CAPTURED'
      )
    )
  ),
  ADD CONSTRAINT user_memory_review_suggestions_state_shape CHECK (
    (status = 'shadow' AND disposition = 'shadow' AND decided_at IS NULL
      AND decision_kind IS NULL AND result_memory_id IS NULL)
    OR (status = 'pending' AND disposition = 'review' AND decided_at IS NULL
      AND decision_kind IS NULL AND result_memory_id IS NULL)
    OR (status = 'accepted' AND disposition = 'review' AND decided_at IS NOT NULL
      AND decision_kind IN ('accept_new', 'edit_merge', 'keep_both', 'auto_accept')
      AND result_memory_id IS NOT NULL)
    OR (status = 'rejected' AND disposition = 'rejected' AND decided_at IS NOT NULL
      AND (decision_kind IS NULL OR decision_kind IN ('keep_current', 'reject'))
      AND result_memory_id IS NULL)
    OR (status = 'expired' AND disposition IN ('shadow', 'review')
      AND decided_at IS NOT NULL AND decision_kind IS NULL
      AND result_memory_id IS NULL)
  );

ALTER TABLE user_memory_review_decisions
  DROP CONSTRAINT user_memory_review_decisions_kind_allowed,
  DROP CONSTRAINT user_memory_review_decisions_result_shape,
  ADD CONSTRAINT user_memory_review_decisions_kind_allowed CHECK (
    decision_kind IN (
      'keep_current', 'accept_new', 'edit_merge', 'keep_both', 'reject',
      'auto_accept'
    )
  ),
  ADD CONSTRAINT user_memory_review_decisions_result_shape CHECK (
    (decision_kind IN ('keep_current', 'reject')
      AND result_memory_id IS NULL AND result_memory_revision IS NULL)
    OR (decision_kind IN ('accept_new', 'edit_merge', 'keep_both', 'auto_accept')
      AND result_memory_id IS NOT NULL AND result_memory_revision >= 1)
  );

CREATE FUNCTION memory_worker_promote_capture_candidates(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS TABLE (
  promoted_count INTEGER,
  review_count INTEGER,
  rejected_count INTEGER
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
#variable_conflict use_column
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_suggestion user_memory_review_suggestions%ROWTYPE;
  v_scope_generation BIGINT;
  v_memory_id UUID;
  v_decision_id UUID;
  v_decision_hash TEXT;
  v_review_reason TEXT;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  -- Reuse the complete hydrate fence: live lease/outbox, completed source and
  -- assistant, source hash, effective Learn policy, scope/epoch, and Provider
  -- profile are re-authorized after the Provider boundary.
  PERFORM 1 FROM memory_worker_hydrate_capture_v2(
    p_job_id, p_worker_id, p_lease_token
  );

  SELECT job.* INTO v_job
  FROM memory_jobs job
  JOIN memory_outbox outbox
    ON outbox.event_id = job.event_id
    AND outbox.user_id = job.user_id
    AND outbox.status = 'processing'
    AND outbox.lease_owner = p_worker_id
    AND outbox.lease_token = p_lease_token
    AND outbox.lease_expires_at > v_now
    AND outbox.visibility_epoch = job.visibility_epoch
  WHERE job.job_id = p_job_id
    AND job.stage = 'extract'
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM memory_capture_candidate_batches batch
    WHERE batch.capture_job_id = v_job.job_id
      AND batch.event_id = v_job.event_id
      AND batch.user_id = v_job.user_id
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001', MESSAGE = 'MEMORY_CAPTURE_PROPOSAL_MISSING';
  END IF;

  -- An explicit settings rollback stops new canonical writes even when a
  -- Conversation-level Learn override kept the original capture job eligible.
  IF NOT EXISTS (
    SELECT 1 FROM user_memory_settings settings
    WHERE settings.user_id = v_job.user_id
      AND settings.enabled
      AND settings.auto_record_enabled
  ) THEN
    RETURN QUERY SELECT 0, 0, 0;
    RETURN;
  END IF;

  FOR v_suggestion IN
    SELECT suggestion.*
    FROM user_memory_review_suggestions suggestion
    WHERE suggestion.capture_job_id = v_job.job_id
      AND suggestion.event_id = v_job.event_id
      AND suggestion.user_id = v_job.user_id
      AND suggestion.status = 'shadow'
      AND suggestion.disposition = 'shadow'
      AND suggestion.decision_reason_code = 'SHADOW_ADD'
    ORDER BY suggestion.ordinal
    FOR UPDATE
  LOOP
    v_scope_generation := memory_governance_scope_generation(
      v_job.user_id,
      v_suggestion.proposed_scope_type,
      v_suggestion.proposed_project_id,
      v_suggestion.proposed_conversation_id
    );
    IF v_suggestion.visibility_epoch <> v_job.visibility_epoch
      OR v_suggestion.scope_generation <> v_scope_generation
      OR NOT EXISTS (
        SELECT 1 FROM user_memory_state state
        WHERE state.user_id = v_job.user_id
          AND state.visibility_epoch = v_suggestion.visibility_epoch
      )
    THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001', MESSAGE = 'MEMORY_VISIBILITY_EPOCH_DRIFT';
    END IF;

    IF NOT EXISTS (
      SELECT 1
      FROM user_memory_review_evidence evidence
      JOIN messages source
        ON source.id = evidence.source_message_id
        AND source.user_id = evidence.user_id
        AND source.conversation_id = evidence.source_conversation_id
        AND source.role = 'user'
        AND source.status = 'completed'
        AND source.deleted_at IS NULL
        AND encode(sha256(convert_to(source.content, 'UTF8')), 'hex') =
          evidence.source_content_hash
      WHERE evidence.suggestion_id = v_suggestion.id
        AND evidence.user_id = v_job.user_id
        AND evidence.evidence_role = 'user'
        AND evidence.source_message_id = v_job.source_message_id
        AND evidence.source_conversation_id = v_job.source_conversation_id
        AND evidence.source_content_hash = v_job.source_hash
    ) THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001', MESSAGE = 'MEMORY_CAPTURE_SOURCE_DRIFT';
    END IF;

    v_review_reason := NULL;
    IF v_suggestion.proposed_action <> 'ADD'
      OR v_suggestion.sensitivity <> 'normal'
      OR v_suggestion.confirmation_kind NOT IN (
        'explicit_user', 'confirmed_assistant'
      )
      OR v_suggestion.fact_expires_at IS NOT NULL
      OR v_suggestion.valid_to IS NOT NULL
      OR v_suggestion.temporal_basis IN (
        'relative_ambiguous', 'model_inferred'
      )
    THEN
      v_review_reason := 'AUTO_PROMOTION_REVIEW_REQUIRED';
    ELSIF v_suggestion.confirmation_kind = 'confirmed_assistant'
      AND NOT EXISTS (
        SELECT 1
        FROM user_memory_review_evidence evidence
        JOIN messages assistant
          ON assistant.id = evidence.source_message_id
          AND assistant.user_id = evidence.user_id
          AND assistant.conversation_id = evidence.source_conversation_id
          AND assistant.role = 'assistant'
          AND assistant.status = 'completed'
          AND assistant.deleted_at IS NULL
          AND encode(sha256(convert_to(assistant.content, 'UTF8')), 'hex') =
            evidence.source_content_hash
        WHERE evidence.suggestion_id = v_suggestion.id
          AND evidence.user_id = v_job.user_id
          AND evidence.evidence_role = 'assistant_context'
      )
    THEN
      v_review_reason := 'AUTO_PROMOTION_EVIDENCE_STALE';
    ELSIF EXISTS (
      SELECT 1 FROM user_memory_tombstones tombstone
      WHERE tombstone.user_id = v_job.user_id
        AND (
          tombstone.content_hash = v_suggestion.candidate_hash
          OR (
            v_suggestion.fact_key IS NOT NULL
            AND tombstone.fact_key = v_suggestion.fact_key
          )
        )
    ) THEN
      v_review_reason := 'AUTO_PROMOTION_TOMBSTONED';
    ELSIF EXISTS (
      SELECT 1 FROM user_memory_review_targets target
      WHERE target.suggestion_id = v_suggestion.id
        AND target.user_id = v_job.user_id
    ) OR EXISTS (
      SELECT 1 FROM user_memories memory
      WHERE memory.user_id = v_job.user_id
        AND memory.scope_type = v_suggestion.proposed_scope_type
        AND memory.project_id IS NOT DISTINCT FROM v_suggestion.proposed_project_id
        AND memory.scope_conversation_id IS NOT DISTINCT FROM
          v_suggestion.proposed_conversation_id
        AND memory.deleted_at IS NULL
        AND memory.enabled
        AND memory.lifecycle_status = 'active'
        AND (
          memory.normalized_content = v_suggestion.normalized_content
          OR (
            v_suggestion.fact_key IS NOT NULL
            AND memory.fact_key = v_suggestion.fact_key
          )
        )
    ) THEN
      v_review_reason := 'AUTO_PROMOTION_CONFLICT';
    END IF;

    IF v_review_reason IS NOT NULL THEN
      UPDATE user_memory_review_suggestions suggestion
      SET disposition = 'review', status = 'pending',
          decision_reason_code = v_review_reason
      WHERE suggestion.id = v_suggestion.id
        AND suggestion.user_id = v_job.user_id
        AND suggestion.status = 'shadow';
      INSERT INTO message_memory_activities (
        id, assistant_message_id, ordinal, user_id,
        subject_type, subject_id, action, status, reason_code,
        source_kind, source_id, created_at, updated_at
      ) VALUES (
        gen_random_uuid(), v_job.assistant_message_id,
        memory_next_activity_ordinal(v_job.assistant_message_id), v_job.user_id,
        'review_suggestion', v_suggestion.id, 'review_required', 'pending',
        v_review_reason, 'review_suggestion', v_suggestion.id, v_now, v_now
      ) ON CONFLICT (source_kind, source_id) DO NOTHING;
      CONTINUE;
    END IF;

    -- Convert the shadow to the existing pending input shape, then invoke the
    -- reviewed governance path in the same transaction. This deliberately
    -- shares canonical insert, evidence copy, and exact-conflict enforcement.
    UPDATE user_memory_review_suggestions suggestion
    SET disposition = 'review', status = 'pending',
        decision_reason_code = 'AUTO_CAPTURE_ELIGIBLE'
    WHERE suggestion.id = v_suggestion.id
      AND suggestion.user_id = v_job.user_id
      AND suggestion.status = 'shadow';
    INSERT INTO message_memory_activities (
      id, assistant_message_id, ordinal, user_id,
      subject_type, subject_id, action, status, reason_code,
      source_kind, source_id, created_at, updated_at
    ) VALUES (
      gen_random_uuid(), v_job.assistant_message_id,
      memory_next_activity_ordinal(v_job.assistant_message_id), v_job.user_id,
      'review_suggestion', v_suggestion.id, 'review_required', 'pending',
      'AUTO_CAPTURE_ELIGIBLE', 'review_suggestion', v_suggestion.id,
      v_now, v_now
    ) ON CONFLICT (source_kind, source_id) DO NOTHING;

    v_memory_id := gen_random_uuid();
    v_decision_id := gen_random_uuid();
    v_decision_hash := encode(sha256(convert_to(
      'auto-capture-v1:' || v_suggestion.id::TEXT || ':' ||
      v_suggestion.candidate_hash || ':' || v_suggestion.visibility_epoch::TEXT,
      'UTF8'
    )), 'hex');
    PERFORM memory_governance_decide_review(
      v_job.user_id, v_suggestion.id, v_decision_id, 'accept_new',
      v_memory_id, NULL, NULL, v_decision_hash
    );

    UPDATE user_memories memory
    SET authority_kind = 'auto',
        extraction_profile_id = v_suggestion.extraction_profile_id,
        confidence = v_suggestion.confidence
    WHERE memory.id = v_memory_id AND memory.user_id = v_job.user_id;
    UPDATE user_memory_review_suggestions suggestion
    SET decision_kind = 'auto_accept', result_code = 'AUTO_CAPTURED'
    WHERE suggestion.id = v_suggestion.id AND suggestion.user_id = v_job.user_id;
    UPDATE user_memory_review_decisions decision
    SET decision_kind = 'auto_accept', result_code = 'AUTO_CAPTURED'
    WHERE decision.id = v_decision_id
      AND decision.suggestion_id = v_suggestion.id
      AND decision.user_id = v_job.user_id;
    UPDATE message_memory_activities activity
    SET reason_code = 'AUTO_CAPTURED', updated_at = v_now
    WHERE activity.user_id = v_job.user_id
      AND activity.source_kind = 'review_suggestion'
      AND activity.source_id = v_suggestion.id;
  END LOOP;

  RETURN QUERY
  SELECT
    count(*) FILTER (
      WHERE suggestion.status = 'accepted'
        AND suggestion.decision_kind = 'auto_accept'
        AND suggestion.result_code = 'AUTO_CAPTURED'
    )::INTEGER,
    count(*) FILTER (
      WHERE suggestion.status = 'pending'
        AND suggestion.decision_reason_code LIKE 'AUTO_PROMOTION_%'
    )::INTEGER,
    count(*) FILTER (
      WHERE suggestion.status = 'rejected'
        AND suggestion.decision_reason_code LIKE 'AUTO_PROMOTION_%'
    )::INTEGER
  FROM user_memory_review_suggestions suggestion
  WHERE suggestion.capture_job_id = v_job.job_id
    AND suggestion.event_id = v_job.event_id
    AND suggestion.user_id = v_job.user_id;
END
$function$;

DO $harden_auto_capture_function$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.memory_worker_promote_capture_candidates(uuid,uuid,uuid) '
      || 'SET search_path TO %I, pg_catalog, pg_temp',
    schema_name, schema_name
  );
  EXECUTE format(
    'ALTER FUNCTION %I.memory_worker_promote_capture_candidates(uuid,uuid,uuid) '
      || 'OWNER TO memory_runtime_owner',
    schema_name
  );
END
$harden_auto_capture_function$;

REVOKE ALL ON FUNCTION memory_worker_promote_capture_candidates(UUID, UUID, UUID)
  FROM PUBLIC, go_api_runtime;
GRANT EXECUTE ON FUNCTION memory_worker_promote_capture_candidates(UUID, UUID, UUID)
  TO memory_worker_runtime;
