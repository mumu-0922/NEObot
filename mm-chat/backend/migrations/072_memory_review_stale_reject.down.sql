-- Restore the migration-060 target fence for every Review decision.

DO $memory_review_stale_reject_prerequisite$
BEGIN
  IF to_regprocedure(
    'memory_governance_decide_review(uuid,uuid,uuid,text,uuid,text,text,text)'
  ) IS NULL
    OR to_regclass('user_memory_review_suggestions') IS NULL
    OR to_regclass('user_memory_review_decisions') IS NULL
    OR to_regclass('user_memory_review_targets') IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_REVIEW_STALE_REJECT_ROLLBACK_REQUIRES_060';
  END IF;
END
$memory_review_stale_reject_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE OR REPLACE FUNCTION memory_governance_decide_review(
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

DO $pin_memory_review_stale_reject$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.memory_governance_decide_review(uuid,uuid,uuid,text,uuid,text,text,text) SET search_path TO %I, pg_catalog, pg_temp',
    schema_name,
    schema_name
  );
  EXECUTE format(
    'ALTER FUNCTION %I.memory_governance_decide_review(uuid,uuid,uuid,text,uuid,text,text,text) OWNER TO memory_runtime_owner',
    schema_name
  );
END
$pin_memory_review_stale_reject$;

REVOKE ALL ON FUNCTION memory_governance_decide_review(
  UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT, TEXT
) FROM PUBLIC, memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_governance_decide_review(
  UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT, TEXT
) TO go_api_runtime;
