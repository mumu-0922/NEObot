-- Memory v2 PR5 candidate and Review shadow. Automatic extraction may only
-- persist bounded proposals here; canonical user_memories remain unchanged.

ALTER TABLE user_memories
  ADD COLUMN lifecycle_status TEXT NOT NULL DEFAULT 'active',
  ADD COLUMN subject_key TEXT,
  ADD COLUMN fact_key TEXT,
  ADD COLUMN confidence DOUBLE PRECISION,
  ADD COLUMN observed_at TIMESTAMPTZ,
  ADD COLUMN valid_from TIMESTAMPTZ,
  ADD COLUMN valid_to TIMESTAMPTZ,
  ADD COLUMN expires_at TIMESTAMPTZ,
  ADD COLUMN superseded_by_memory_id UUID,
  ADD COLUMN sensitivity TEXT NOT NULL DEFAULT 'normal',
  ADD COLUMN temporal_basis TEXT NOT NULL DEFAULT 'none',
  ADD COLUMN temporal_parser_version TEXT;

UPDATE user_memories
SET observed_at = created_at,
    confidence = CASE
      WHEN authority_kind IN ('manual', 'direct_user', 'confirmed', 'import')
        THEN 1.0
      ELSE NULL
    END;

ALTER TABLE user_memories
  ALTER COLUMN observed_at SET DEFAULT now(),
  ALTER COLUMN observed_at SET NOT NULL,
  ADD CONSTRAINT user_memories_lifecycle_status_allowed CHECK (
    lifecycle_status IN ('active', 'superseded', 'expired', 'rejected')
  ),
  ADD CONSTRAINT user_memories_subject_key_bounded CHECK (
    subject_key IS NULL OR (
      octet_length(subject_key) BETWEEN 1 AND 256
      AND subject_key = trim(subject_key)
    )
  ),
  ADD CONSTRAINT user_memories_fact_key_bounded CHECK (
    fact_key IS NULL OR (
      octet_length(fact_key) BETWEEN 1 AND 256
      AND fact_key = trim(fact_key)
    )
  ),
  ADD CONSTRAINT user_memories_confidence_range CHECK (
    confidence IS NULL OR confidence BETWEEN 0.0 AND 1.0
  ),
  ADD CONSTRAINT user_memories_validity_order CHECK (
    valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from
  ),
  ADD CONSTRAINT user_memories_expiry_order CHECK (
    expires_at IS NULL OR valid_from IS NULL OR expires_at >= valid_from
  ),
  ADD CONSTRAINT user_memories_superseded_owner_fk
    FOREIGN KEY (superseded_by_memory_id, user_id)
    REFERENCES user_memories(id, user_id)
    ON DELETE RESTRICT,
  ADD CONSTRAINT user_memories_supersede_shape CHECK (
    (lifecycle_status = 'superseded' AND superseded_by_memory_id IS NOT NULL)
    OR (lifecycle_status <> 'superseded' AND superseded_by_memory_id IS NULL)
  ),
  ADD CONSTRAINT user_memories_sensitivity_allowed CHECK (
    sensitivity IN ('normal', 'sensitive')
  ),
  ADD CONSTRAINT user_memories_temporal_basis_allowed CHECK (
    temporal_basis IN (
      'none', 'source_timestamp', 'explicit_absolute',
      'relative_ambiguous', 'model_inferred'
    )
  ),
  ADD CONSTRAINT user_memories_temporal_parser_bounded CHECK (
    temporal_parser_version IS NULL OR (
      octet_length(temporal_parser_version) BETWEEN 1 AND 128
      AND temporal_parser_version = trim(temporal_parser_version)
    )
  );

CREATE INDEX idx_user_memories_current_fact
  ON user_memories(user_id, scope_type, fact_key, updated_at DESC, id)
  WHERE deleted_at IS NULL
    AND enabled
    AND lifecycle_status = 'active'
    AND fact_key IS NOT NULL;

ALTER TABLE memory_jobs
  ADD CONSTRAINT memory_jobs_job_event_user_unique
    UNIQUE (job_id, event_id, user_id);

CREATE TABLE memory_capture_candidate_batches (
  capture_job_id UUID PRIMARY KEY,
  event_id UUID NOT NULL REFERENCES memory_outbox(event_id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  candidate_schema_major SMALLINT NOT NULL,
  extraction_profile_id TEXT NOT NULL,
  decision_profile_id TEXT NOT NULL,
  proposal_hash TEXT NOT NULL,
  proposal_count SMALLINT NOT NULL,
  review_expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_capture_candidate_batches_event_unique UNIQUE (event_id),
  CONSTRAINT memory_capture_candidate_batches_job_event_user_unique
    UNIQUE (capture_job_id, event_id, user_id),
  CONSTRAINT memory_capture_candidate_batches_job_owner_fk
    FOREIGN KEY (capture_job_id, event_id, user_id)
    REFERENCES memory_jobs(job_id, event_id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT memory_capture_candidate_batches_schema_supported
    CHECK (candidate_schema_major = 1),
  CONSTRAINT memory_capture_candidate_batches_extraction_profile_check
    CHECK (extraction_profile_id ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_capture_candidate_batches_decision_profile_check
    CHECK (decision_profile_id ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_capture_candidate_batches_proposal_hash_check
    CHECK (proposal_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_capture_candidate_batches_count_bounded
    CHECK (proposal_count BETWEEN 0 AND 5),
  CONSTRAINT memory_capture_candidate_batches_expiry_order CHECK (
    review_expires_at = created_at + interval '30 days'
  )
);

CREATE TABLE user_memory_review_suggestions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  capture_job_id UUID NOT NULL,
  event_id UUID NOT NULL,
  ordinal SMALLINT NOT NULL,
  candidate_type TEXT NOT NULL,
  candidate_content TEXT,
  normalized_content TEXT,
  candidate_hash TEXT NOT NULL,
  importance SMALLINT NOT NULL,
  tags TEXT[] NOT NULL DEFAULT '{}',
  subject_key TEXT,
  fact_key TEXT,
  sensitivity TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL,
  confidence_band TEXT NOT NULL,
  proposed_scope_type TEXT NOT NULL,
  proposed_project_id UUID,
  proposed_conversation_id UUID,
  scope_generation BIGINT NOT NULL,
  scope_confidence DOUBLE PRECISION NOT NULL,
  confirmation_kind TEXT NOT NULL,
  temporal_basis TEXT NOT NULL,
  temporal_parser_version TEXT,
  observed_at TIMESTAMPTZ NOT NULL,
  valid_from TIMESTAMPTZ,
  valid_to TIMESTAMPTZ,
  fact_expires_at TIMESTAMPTZ,
  proposed_action TEXT NOT NULL,
  disposition TEXT NOT NULL,
  decision_reason_code TEXT NOT NULL,
  status TEXT NOT NULL,
  visibility_epoch BIGINT NOT NULL,
  extraction_profile_id TEXT NOT NULL,
  decision_profile_id TEXT NOT NULL,
  review_expires_at TIMESTAMPTZ NOT NULL,
  decided_at TIMESTAMPTZ,
  result_code TEXT,
  purged_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_review_suggestions_id_user_unique UNIQUE (id, user_id),
  CONSTRAINT user_memory_review_suggestions_batch_ordinal_unique
    UNIQUE (capture_job_id, ordinal),
  CONSTRAINT user_memory_review_suggestions_batch_fk
    FOREIGN KEY (capture_job_id, event_id, user_id)
    REFERENCES memory_capture_candidate_batches(capture_job_id, event_id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_review_suggestions_project_owner_fk
    FOREIGN KEY (proposed_project_id, user_id)
    REFERENCES projects(id, user_id)
    ON DELETE RESTRICT,
  CONSTRAINT user_memory_review_suggestions_conversation_owner_fk
    FOREIGN KEY (proposed_conversation_id, user_id)
    REFERENCES conversations(id, user_id)
    ON DELETE RESTRICT,
  CONSTRAINT user_memory_review_suggestions_type_allowed CHECK (
    candidate_type IN (
      'fact', 'preference', 'instruction', 'project',
      'warning', 'decision', 'context'
    )
  ),
  CONSTRAINT user_memory_review_suggestions_candidate_hash_check
    CHECK (candidate_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT user_memory_review_suggestions_importance_range
    CHECK (importance BETWEEN 1 AND 5),
  CONSTRAINT user_memory_review_suggestions_tag_count_bounded
    CHECK (cardinality(tags) <= 12),
  CONSTRAINT user_memory_review_suggestions_keys_bounded CHECK (
    (subject_key IS NULL OR (
      octet_length(subject_key) BETWEEN 1 AND 256
      AND subject_key = trim(subject_key)
    ))
    AND (fact_key IS NULL OR (
      octet_length(fact_key) BETWEEN 1 AND 256
      AND fact_key = trim(fact_key)
    ))
  ),
  CONSTRAINT user_memory_review_suggestions_sensitivity_allowed
    CHECK (sensitivity IN ('normal', 'sensitive', 'secret')),
  CONSTRAINT user_memory_review_suggestions_confidence_range
    CHECK (confidence BETWEEN 0.0 AND 1.0),
  CONSTRAINT user_memory_review_suggestions_confidence_band_allowed
    CHECK (confidence_band IN ('low', 'medium', 'high')),
  CONSTRAINT user_memory_review_suggestions_scope_allowed
    CHECK (proposed_scope_type IN ('global', 'project', 'conversation')),
  CONSTRAINT user_memory_review_suggestions_scope_shape CHECK (
    (proposed_scope_type = 'global'
      AND proposed_project_id IS NULL
      AND proposed_conversation_id IS NULL)
    OR (proposed_scope_type = 'project'
      AND proposed_project_id IS NOT NULL
      AND proposed_conversation_id IS NULL)
    OR (proposed_scope_type = 'conversation'
      AND proposed_project_id IS NULL
      AND proposed_conversation_id IS NOT NULL)
  ),
  CONSTRAINT user_memory_review_suggestions_scope_generation_positive
    CHECK (scope_generation >= 1),
  CONSTRAINT user_memory_review_suggestions_scope_confidence_range
    CHECK (scope_confidence BETWEEN 0.0 AND 1.0),
  CONSTRAINT user_memory_review_suggestions_confirmation_allowed CHECK (
    confirmation_kind IN ('explicit_user', 'confirmed_assistant')
  ),
  CONSTRAINT user_memory_review_suggestions_temporal_basis_allowed CHECK (
    temporal_basis IN (
      'none', 'source_timestamp', 'explicit_absolute',
      'relative_ambiguous', 'model_inferred'
    )
  ),
  CONSTRAINT user_memory_review_suggestions_temporal_parser_bounded CHECK (
    temporal_parser_version IS NULL OR (
      octet_length(temporal_parser_version) BETWEEN 1 AND 128
      AND temporal_parser_version = trim(temporal_parser_version)
    )
  ),
  CONSTRAINT user_memory_review_suggestions_validity_order CHECK (
    valid_to IS NULL OR valid_from IS NULL OR valid_to >= valid_from
  ),
  CONSTRAINT user_memory_review_suggestions_fact_expiry_order CHECK (
    fact_expires_at IS NULL OR valid_from IS NULL OR fact_expires_at >= valid_from
  ),
  CONSTRAINT user_memory_review_suggestions_action_allowed CHECK (
    proposed_action IN ('ADD', 'NOOP', 'MERGE', 'SUPERSEDE', 'REJECT')
  ),
  CONSTRAINT user_memory_review_suggestions_disposition_allowed
    CHECK (disposition IN ('shadow', 'review', 'rejected')),
  CONSTRAINT user_memory_review_suggestions_reason_sanitized CHECK (
    decision_reason_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  CONSTRAINT user_memory_review_suggestions_status_allowed
    CHECK (status IN ('shadow', 'pending', 'rejected', 'expired')),
  CONSTRAINT user_memory_review_suggestions_visibility_epoch_positive
    CHECK (visibility_epoch >= 1),
  CONSTRAINT user_memory_review_suggestions_profile_checks CHECK (
    extraction_profile_id ~ '^[0-9a-f]{64}$'
    AND decision_profile_id ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT user_memory_review_suggestions_review_expiry_order CHECK (
    review_expires_at = created_at + interval '30 days'
  ),
  CONSTRAINT user_memory_review_suggestions_plaintext_shape CHECK (
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
    )
    OR (
      status IN ('rejected', 'expired')
      AND candidate_content IS NULL
      AND normalized_content IS NULL
      AND tags = '{}'::TEXT[]
      AND subject_key IS NULL
      AND fact_key IS NULL
      AND purged_at IS NOT NULL
      AND result_code IN (
        'SECRET_REJECTED', 'SENSITIVE_DISABLED', 'MODEL_REJECTED',
        'TOMBSTONED', 'PLAINTEXT_EXPIRED'
      )
    )
  ),
  CONSTRAINT user_memory_review_suggestions_state_shape CHECK (
    (status = 'shadow' AND disposition = 'shadow' AND decided_at IS NULL)
    OR (status = 'pending' AND disposition = 'review' AND decided_at IS NULL)
    OR (status = 'rejected' AND disposition = 'rejected' AND decided_at IS NOT NULL)
    OR (status = 'expired' AND disposition IN ('shadow', 'review') AND decided_at IS NOT NULL)
  )
);

CREATE INDEX idx_user_memory_review_suggestions_user_status_expiry
  ON user_memory_review_suggestions(user_id, status, review_expires_at, id);
CREATE INDEX idx_user_memory_review_suggestions_user_scope_fact
  ON user_memory_review_suggestions(
    user_id, proposed_scope_type, proposed_project_id,
    proposed_conversation_id, fact_key, created_at DESC
  ) WHERE status IN ('shadow', 'pending') AND fact_key IS NOT NULL;

CREATE TABLE user_memory_review_targets (
  suggestion_id UUID NOT NULL,
  memory_id UUID NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expected_revision BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (suggestion_id, memory_id),
  CONSTRAINT user_memory_review_targets_suggestion_owner_fk
    FOREIGN KEY (suggestion_id, user_id)
    REFERENCES user_memory_review_suggestions(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_review_targets_memory_owner_fk
    FOREIGN KEY (memory_id, user_id)
    REFERENCES user_memories(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_review_targets_revision_positive
    CHECK (expected_revision >= 1)
);

CREATE TABLE user_memory_review_evidence (
  suggestion_id UUID NOT NULL,
  source_message_id UUID NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_conversation_id UUID NOT NULL,
  evidence_role TEXT NOT NULL,
  source_content_hash TEXT NOT NULL,
  observed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (suggestion_id, source_message_id),
  CONSTRAINT user_memory_review_evidence_suggestion_owner_fk
    FOREIGN KEY (suggestion_id, user_id)
    REFERENCES user_memory_review_suggestions(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_review_evidence_message_owner_fk
    FOREIGN KEY (source_message_id, user_id)
    REFERENCES messages(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_review_evidence_conversation_owner_fk
    FOREIGN KEY (source_conversation_id, user_id)
    REFERENCES conversations(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT user_memory_review_evidence_role_allowed
    CHECK (evidence_role IN ('user', 'assistant_context')),
  CONSTRAINT user_memory_review_evidence_source_hash_check
    CHECK (source_content_hash ~ '^[0-9a-f]{64}$')
);

ALTER TABLE memory_outbox
  DROP CONSTRAINT memory_outbox_attempts_bounded,
  ADD CONSTRAINT memory_outbox_attempts_bounded CHECK (
    (
      (event_type = 'turn.completed' AND max_attempts BETWEEN 1 AND 128)
      OR (event_type = 'memory.deleted' AND max_attempts = 128)
    )
    AND attempt_count BETWEEN 0 AND max_attempts
  );

ALTER TABLE memory_jobs
  DROP CONSTRAINT memory_jobs_stage_allowed,
  DROP CONSTRAINT memory_jobs_attempts_bounded,
  DROP CONSTRAINT memory_jobs_stage_shape_check,
  ADD CONSTRAINT memory_jobs_stage_allowed CHECK (
    stage IN (
      'extract', 'resolve', 'embed', 'l2_refresh', 'l3_refresh',
      'purge', 'review_expire', 'rebuild'
    )
  ),
  ADD CONSTRAINT memory_jobs_attempts_bounded CHECK (
    (
      (stage IN ('purge', 'review_expire') AND max_attempts = 128)
      OR (stage NOT IN ('purge', 'review_expire') AND max_attempts BETWEEN 1 AND 32)
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
      stage = 'review_expire'
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
      AND target_memory_id IS NULL
      AND target_tombstone_id IS NULL
    )
    OR (
      stage NOT IN ('extract', 'purge', 'review_expire')
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

CREATE INDEX idx_memory_jobs_review_expire
  ON memory_jobs(available_at, event_id, job_id)
  WHERE stage = 'review_expire' AND status = 'pending';

CREATE FUNCTION memory_worker_hydrate_capture_v2(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS TABLE (
  user_id UUID,
  context_messages JSONB,
  current_memories JSONB,
  sensitive_memory_enabled BOOLEAN,
  project_id UUID,
  provider_record_id UUID,
  provider_id TEXT,
  provider_label TEXT,
  encrypted_secret_ref TEXT,
  provider_config JSONB,
  model_id TEXT,
  processing_profile TEXT,
  proposal_committed BOOLEAN
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
#variable_conflict use_column
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_legacy RECORD;
  v_conversation conversations%ROWTYPE;
  v_context JSONB;
  v_memories JSONB;
  v_sensitive BOOLEAN;
BEGIN
  SELECT * INTO v_legacy
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
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = v_job.source_conversation_id
    AND conversation.user_id = v_job.user_id
    AND conversation.deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_CAPTURE_SOURCE_DRIFT';
  END IF;

  SELECT settings.sensitive_memory_enabled INTO v_sensitive
  FROM user_memory_settings settings
  WHERE settings.user_id = v_job.user_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_CAPTURE_SOURCE_DRIFT';
  END IF;

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'id', recent.id::TEXT,
    'role', recent.role,
    'content', left(recent.content, 4000),
    'sequenceNo', recent.sequence_no,
    'observedAt', COALESCE(recent.completed_at, recent.created_at)
  ) ORDER BY recent.sequence_no), '[]'::JSONB)
  INTO v_context
  FROM (
    SELECT message.*
    FROM messages message
    WHERE message.conversation_id = v_job.source_conversation_id
      AND message.user_id = v_job.user_id
      AND message.role IN ('user', 'assistant')
      AND message.status = 'completed'
      AND message.deleted_at IS NULL
      AND message.sequence_no <= (
        SELECT sequence_no FROM messages WHERE id = v_job.assistant_message_id
      )
    ORDER BY message.sequence_no DESC
    LIMIT 8
  ) recent;

  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'id', candidate.id::TEXT,
    'revision', candidate.revision,
    'type', candidate.memory_type,
    'content', candidate.content,
    'authorityKind', candidate.authority_kind,
    'scopeType', candidate.scope_type,
    'projectId', candidate.project_id::TEXT,
    'conversationId', candidate.scope_conversation_id::TEXT,
    'factKey', candidate.fact_key,
    'sensitivity', candidate.sensitivity
  ) ORDER BY candidate.scope_rank, candidate.importance DESC,
             candidate.updated_at DESC, candidate.id), '[]'::JSONB)
  INTO v_memories
  FROM (
    SELECT memory.*,
      CASE memory.scope_type
        WHEN 'conversation' THEN 1
        WHEN 'project' THEN 2
        ELSE 3
      END AS scope_rank
    FROM user_memories memory
    WHERE memory.user_id = v_job.user_id
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND memory.visibility_epoch = v_job.visibility_epoch
      AND (
        memory.scope_type = 'global'
        OR (memory.scope_type = 'project'
          AND memory.project_id = v_conversation.project_id)
        OR (memory.scope_type = 'conversation'
          AND memory.scope_conversation_id = v_conversation.id)
      )
    ORDER BY scope_rank, memory.importance DESC,
             memory.updated_at DESC, memory.id
    LIMIT 10
  ) candidate;

  RETURN QUERY SELECT
    v_legacy.user_id,
    v_context,
    v_memories,
    v_sensitive,
    v_conversation.project_id,
    v_legacy.provider_record_id,
    v_legacy.provider_id,
    v_legacy.provider_label,
    v_legacy.encrypted_secret_ref,
    v_legacy.provider_config,
    v_legacy.model_id,
    v_legacy.processing_profile,
    EXISTS (
      SELECT 1 FROM memory_capture_candidate_batches batch
      WHERE batch.capture_job_id = v_job.job_id
        AND batch.user_id = v_job.user_id
        AND batch.event_id = v_job.event_id
    );
END
$function$;

CREATE FUNCTION memory_worker_propose_capture_candidates(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_expiry_job_id UUID,
  p_candidate_schema_major SMALLINT,
  p_extraction_profile_id TEXT,
  p_decision_profile_id TEXT,
  p_candidates JSONB
) RETURNS TABLE (
  proposal_count SMALLINT,
  shadow_count SMALLINT,
  review_count SMALLINT,
  rejected_count SMALLINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
#variable_conflict use_column
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_conversation conversations%ROWTYPE;
  v_batch memory_capture_candidate_batches%ROWTYPE;
  v_candidate JSONB;
  v_index INTEGER := 0;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_proposal_hash TEXT;
  v_count INTEGER;
  v_id UUID;
  v_content TEXT;
  v_normalized TEXT;
  v_hash TEXT;
  v_type TEXT;
  v_importance SMALLINT;
  v_tags TEXT[];
  v_subject_key TEXT;
  v_fact_key TEXT;
  v_sensitivity TEXT;
  v_confidence DOUBLE PRECISION;
  v_confidence_band TEXT;
  v_scope_type TEXT;
  v_project_id UUID;
  v_conversation_id UUID;
  v_scope_generation BIGINT;
  v_scope_confidence DOUBLE PRECISION;
  v_confirmation TEXT;
  v_temporal_basis TEXT;
  v_temporal_parser TEXT;
  v_observed_at TIMESTAMPTZ;
  v_valid_from TIMESTAMPTZ;
  v_valid_to TIMESTAMPTZ;
  v_fact_expires_at TIMESTAMPTZ;
  v_action TEXT;
  v_authority_ids UUID[];
  v_context_ids UUID[];
  v_target_ids UUID[];
  v_exact user_memories%ROWTYPE;
  v_exact_found BOOLEAN;
  v_related_count INTEGER;
  v_manual_related BOOLEAN;
  v_disposition TEXT;
  v_status TEXT;
  v_reason TEXT;
  v_result TEXT;
  v_decided_at TIMESTAMPTZ;
  v_purged_at TIMESTAMPTZ;
BEGIN
  PERFORM 1 FROM memory_worker_hydrate_capture_v2(
    p_job_id, p_worker_id, p_lease_token
  );

  SELECT job.* INTO v_job
  FROM memory_jobs job
  WHERE job.job_id = p_job_id
    AND job.stage = 'extract'
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  SELECT conversation.* INTO v_conversation
  FROM conversations conversation
  WHERE conversation.id = v_job.source_conversation_id
    AND conversation.user_id = v_job.user_id
    AND conversation.deleted_at IS NULL
    AND conversation.memory_scope_generation = v_job.scope_generation;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_CAPTURE_SOURCE_DRIFT';
  END IF;

  IF p_expiry_job_id IS NULL
    OR p_candidate_schema_major <> 1
    OR p_extraction_profile_id !~ '^[0-9a-f]{64}$'
    OR p_decision_profile_id !~ '^[0-9a-f]{64}$'
    OR jsonb_typeof(p_candidates) <> 'array'
    OR octet_length(p_candidates::TEXT) > 65536
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_BATCH_INVALID';
  END IF;

  v_count := jsonb_array_length(p_candidates);
  IF v_count > 5 THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_BATCH_INVALID';
  END IF;
  v_proposal_hash := encode(sha256(convert_to(p_candidates::TEXT, 'UTF8')), 'hex');

  SELECT batch.* INTO v_batch
  FROM memory_capture_candidate_batches batch
  WHERE batch.capture_job_id = p_job_id
  FOR UPDATE;
  IF FOUND THEN
    IF v_batch.user_id <> v_job.user_id
      OR v_batch.event_id <> v_job.event_id
      OR v_batch.candidate_schema_major <> p_candidate_schema_major
      OR v_batch.extraction_profile_id <> p_extraction_profile_id
      OR v_batch.decision_profile_id <> p_decision_profile_id
      OR v_batch.proposal_hash <> v_proposal_hash
      OR v_batch.proposal_count <> v_count
    THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001', MESSAGE = 'MEMORY_CAPTURE_PROPOSAL_CONFLICT';
    END IF;
    RETURN QUERY SELECT
      v_batch.proposal_count,
      count(*) FILTER (WHERE disposition = 'shadow')::SMALLINT,
      count(*) FILTER (WHERE disposition = 'review')::SMALLINT,
      count(*) FILTER (WHERE disposition = 'rejected')::SMALLINT
    FROM user_memory_review_suggestions suggestion
    WHERE suggestion.capture_job_id = p_job_id;
    RETURN;
  END IF;

  INSERT INTO memory_capture_candidate_batches (
    capture_job_id, event_id, user_id, candidate_schema_major,
    extraction_profile_id, decision_profile_id, proposal_hash,
    proposal_count, review_expires_at, created_at
  ) VALUES (
    p_job_id, v_job.event_id, v_job.user_id, p_candidate_schema_major,
    p_extraction_profile_id, p_decision_profile_id, v_proposal_hash,
    v_count, v_now + interval '30 days', v_now
  );

  FOR v_candidate IN SELECT value FROM jsonb_array_elements(p_candidates)
  LOOP
    v_index := v_index + 1;
    IF jsonb_typeof(v_candidate) <> 'object'
      OR NOT (v_candidate ?& ARRAY[
        'id', 'type', 'content', 'normalizedContent', 'candidateHash',
        'importance', 'tags', 'subjectKey', 'factKey', 'sensitivity',
        'confidence', 'confidenceBand', 'authorityUserMessageIds',
        'contextMessageIds', 'confirmationKind', 'proposedScopeType',
        'proposedProjectId', 'proposedConversationId', 'scopeConfidence',
        'temporalBasis', 'temporalParserVersion', 'observedAt',
        'validFrom', 'validTo', 'factExpiresAt', 'proposedAction',
        'targetMemoryIds'
      ]::TEXT[])
      OR (v_candidate - ARRAY[
        'id', 'type', 'content', 'normalizedContent', 'candidateHash',
        'importance', 'tags', 'subjectKey', 'factKey', 'sensitivity',
        'confidence', 'confidenceBand', 'authorityUserMessageIds',
        'contextMessageIds', 'confirmationKind', 'proposedScopeType',
        'proposedProjectId', 'proposedConversationId', 'scopeConfidence',
        'temporalBasis', 'temporalParserVersion', 'observedAt',
        'validFrom', 'validTo', 'factExpiresAt', 'proposedAction',
        'targetMemoryIds'
      ]::TEXT[]) <> '{}'::JSONB
    THEN
      RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_INVALID';
    END IF;

    BEGIN
      v_id := (v_candidate->>'id')::UUID;
      v_type := v_candidate->>'type';
      v_content := v_candidate->>'content';
      v_normalized := v_candidate->>'normalizedContent';
      v_hash := v_candidate->>'candidateHash';
      v_importance := (v_candidate->>'importance')::SMALLINT;
      SELECT COALESCE(array_agg(value), '{}'::TEXT[]) INTO v_tags
      FROM jsonb_array_elements_text(v_candidate->'tags');
      v_subject_key := v_candidate->>'subjectKey';
      v_fact_key := v_candidate->>'factKey';
      v_sensitivity := v_candidate->>'sensitivity';
      v_confidence := (v_candidate->>'confidence')::DOUBLE PRECISION;
      v_confidence_band := v_candidate->>'confidenceBand';
      v_scope_type := v_candidate->>'proposedScopeType';
      v_project_id := NULLIF(v_candidate->>'proposedProjectId', '')::UUID;
      v_conversation_id := NULLIF(v_candidate->>'proposedConversationId', '')::UUID;
      v_scope_confidence := (v_candidate->>'scopeConfidence')::DOUBLE PRECISION;
      v_confirmation := v_candidate->>'confirmationKind';
      v_temporal_basis := v_candidate->>'temporalBasis';
      v_temporal_parser := v_candidate->>'temporalParserVersion';
      v_observed_at := (v_candidate->>'observedAt')::TIMESTAMPTZ;
      v_valid_from := NULLIF(v_candidate->>'validFrom', '')::TIMESTAMPTZ;
      v_valid_to := NULLIF(v_candidate->>'validTo', '')::TIMESTAMPTZ;
      v_fact_expires_at := NULLIF(v_candidate->>'factExpiresAt', '')::TIMESTAMPTZ;
      v_action := v_candidate->>'proposedAction';
      SELECT COALESCE(array_agg(value::UUID), '{}'::UUID[]) INTO v_authority_ids
      FROM jsonb_array_elements_text(v_candidate->'authorityUserMessageIds');
      SELECT COALESCE(array_agg(value::UUID), '{}'::UUID[]) INTO v_context_ids
      FROM jsonb_array_elements_text(v_candidate->'contextMessageIds');
      SELECT COALESCE(array_agg(value::UUID), '{}'::UUID[]) INTO v_target_ids
      FROM jsonb_array_elements_text(v_candidate->'targetMemoryIds');
    EXCEPTION WHEN OTHERS THEN
      RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_INVALID';
    END;

    IF v_type NOT IN (
        'fact', 'preference', 'instruction', 'project',
        'warning', 'decision', 'context'
      )
      OR v_hash !~ '^[0-9a-f]{64}$'
      OR v_importance NOT BETWEEN 1 AND 5
      OR cardinality(v_tags) > 12
      OR EXISTS (
        SELECT 1 FROM unnest(v_tags) tag
        WHERE length(trim(tag)) = 0 OR char_length(tag) > 40
      )
      OR v_sensitivity NOT IN ('normal', 'sensitive', 'secret')
      OR v_confidence NOT BETWEEN 0.0 AND 1.0
      OR v_confidence_band NOT IN ('low', 'medium', 'high')
      OR NOT (
        (v_confidence < 0.50 AND v_confidence_band = 'low')
        OR (v_confidence >= 0.50 AND v_confidence < 0.80
          AND v_confidence_band = 'medium')
        OR (v_confidence >= 0.80 AND v_confidence_band = 'high')
      )
      OR v_scope_confidence NOT BETWEEN 0.0 AND 1.0
      OR v_confirmation NOT IN ('explicit_user', 'confirmed_assistant')
      OR v_temporal_basis NOT IN (
        'none', 'source_timestamp', 'explicit_absolute',
        'relative_ambiguous', 'model_inferred'
      )
      OR NOT (
        (v_temporal_basis = 'none'
          AND v_temporal_parser IS NULL
          AND v_valid_from IS NULL AND v_valid_to IS NULL
          AND v_fact_expires_at IS NULL)
        OR (v_temporal_basis = 'source_timestamp'
          AND v_temporal_parser = 'source-message-v1'
          AND v_valid_from IS NULL AND v_valid_to IS NULL
          AND v_fact_expires_at IS NULL)
        OR (v_temporal_basis = 'explicit_absolute'
          AND v_temporal_parser = 'rfc3339-v1'
          AND (v_valid_from IS NOT NULL OR v_valid_to IS NOT NULL
            OR v_fact_expires_at IS NOT NULL))
        OR (v_temporal_basis IN ('relative_ambiguous', 'model_inferred')
          AND v_temporal_parser = 'unresolved-v1'
          AND v_valid_from IS NULL AND v_valid_to IS NULL
          AND v_fact_expires_at IS NULL)
      )
      OR v_action NOT IN ('ADD', 'NOOP', 'MERGE', 'SUPERSEDE', 'REJECT')
      OR cardinality(v_authority_ids) < 1
      OR cardinality(v_authority_ids) > 8
      OR cardinality(v_context_ids) > 8
      OR cardinality(v_target_ids) > 5
      OR NOT (v_job.source_message_id = ANY(v_authority_ids))
      OR (v_subject_key IS NOT NULL AND (
        octet_length(v_subject_key) NOT BETWEEN 1 AND 256
        OR v_subject_key <> trim(v_subject_key)
      ))
      OR (v_fact_key IS NOT NULL AND (
        octet_length(v_fact_key) NOT BETWEEN 1 AND 256
        OR v_fact_key <> trim(v_fact_key)
      ))
      OR (v_valid_to IS NOT NULL AND v_valid_from IS NOT NULL
        AND v_valid_to < v_valid_from)
      OR (v_fact_expires_at IS NOT NULL AND v_valid_from IS NOT NULL
        AND v_fact_expires_at < v_valid_from)
      OR NOT EXISTS (
        SELECT 1 FROM messages source
        WHERE source.id = v_job.source_message_id
          AND source.user_id = v_job.user_id
          AND source.conversation_id = v_job.source_conversation_id
          AND v_observed_at = COALESCE(source.completed_at, source.created_at)
      )
    THEN
      RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_INVALID';
    END IF;

    IF v_sensitivity = 'secret' THEN
      IF v_content IS NOT NULL OR v_normalized IS NOT NULL
        OR cardinality(v_tags) <> 0
        OR v_subject_key IS NOT NULL OR v_fact_key IS NOT NULL
      THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_SECRET_PLAINTEXT_FORBIDDEN';
      END IF;
    ELSE
      IF v_content IS NULL OR length(trim(v_content)) = 0
        OR char_length(v_content) > 2000
        OR v_normalized IS NULL OR length(trim(v_normalized)) = 0
        OR char_length(v_normalized) > 2000
        OR encode(sha256(convert_to(v_content, 'UTF8')), 'hex') <> v_hash
      THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_INVALID';
      END IF;
    END IF;

    IF v_scope_type = 'global' THEN
      IF v_project_id IS NOT NULL OR v_conversation_id IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_SCOPE_INVALID';
      END IF;
      v_scope_generation := 1;
    ELSIF v_scope_type = 'project' THEN
      IF v_project_id IS NULL OR v_project_id IS DISTINCT FROM v_conversation.project_id
        OR v_conversation_id IS NOT NULL
        OR NOT EXISTS (
          SELECT 1 FROM projects project
          WHERE project.id = v_project_id
            AND project.user_id = v_job.user_id
            AND project.lifecycle_status = 'active'
            AND project.deleted_at IS NULL
            AND project.scope_generation = v_job.project_scope_generation
        )
      THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_SCOPE_INVALID';
      END IF;
      v_scope_generation := v_job.project_scope_generation;
    ELSIF v_scope_type = 'conversation' THEN
      IF v_conversation_id IS NULL
        OR v_conversation_id <> v_job.source_conversation_id
        OR v_project_id IS NOT NULL
      THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_SCOPE_INVALID';
      END IF;
      v_scope_generation := v_job.scope_generation;
    ELSE
      RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'MEMORY_CANDIDATE_SCOPE_INVALID';
    END IF;

    IF (SELECT count(*) FROM messages source
        WHERE source.id = ANY(v_authority_ids)
          AND source.user_id = v_job.user_id
          AND source.conversation_id = v_job.source_conversation_id
          AND source.role = 'user'
          AND source.status = 'completed'
          AND source.deleted_at IS NULL)
        <> cardinality(v_authority_ids)
      OR (SELECT count(*) FROM messages context
          WHERE context.id = ANY(v_context_ids)
            AND context.user_id = v_job.user_id
            AND context.conversation_id = v_job.source_conversation_id
            AND context.role = 'assistant'
            AND context.status = 'completed'
            AND context.deleted_at IS NULL)
        <> cardinality(v_context_ids)
      OR (v_confirmation = 'confirmed_assistant' AND cardinality(v_context_ids) = 0)
    THEN
      RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_CANDIDATE_EVIDENCE_INVALID';
    END IF;

    IF cardinality(v_target_ids) > 0 AND (
      SELECT count(DISTINCT memory.id)
      FROM user_memories memory
      WHERE memory.id = ANY(v_target_ids)
        AND memory.user_id = v_job.user_id
        AND memory.scope_type = v_scope_type
        AND memory.project_id IS NOT DISTINCT FROM v_project_id
        AND memory.scope_conversation_id IS NOT DISTINCT FROM v_conversation_id
        AND memory.deleted_at IS NULL
        AND memory.enabled
        AND memory.lifecycle_status = 'active'
    ) <> cardinality(v_target_ids) THEN
      RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_CANDIDATE_TARGET_INVALID';
    END IF;

    v_disposition := 'shadow';
    v_status := 'shadow';
    v_reason := 'SHADOW_ADD';
    v_result := NULL;
    v_decided_at := NULL;
    v_purged_at := NULL;

    IF v_sensitivity = 'secret' THEN
      v_disposition := 'rejected'; v_status := 'rejected';
      v_reason := 'SECRET_REJECTED'; v_result := 'SECRET_REJECTED';
      v_decided_at := v_now; v_purged_at := v_now; v_action := 'REJECT';
    ELSIF v_sensitivity = 'sensitive' AND NOT EXISTS (
      SELECT 1 FROM user_memory_settings settings
      WHERE settings.user_id = v_job.user_id
        AND settings.sensitive_memory_enabled
    ) THEN
      v_content := NULL; v_normalized := NULL; v_tags := '{}';
      v_subject_key := NULL; v_fact_key := NULL;
      v_disposition := 'rejected'; v_status := 'rejected';
      v_reason := 'SENSITIVE_DISABLED'; v_result := 'SENSITIVE_DISABLED';
      v_decided_at := v_now; v_purged_at := v_now; v_action := 'REJECT';
    ELSIF EXISTS (
      SELECT 1 FROM user_memory_tombstones tombstone
      WHERE tombstone.user_id = v_job.user_id
        AND (tombstone.content_hash = v_hash
          OR (v_fact_key IS NOT NULL AND tombstone.fact_key = v_fact_key))
    ) THEN
      v_content := NULL; v_normalized := NULL; v_tags := '{}';
      v_subject_key := NULL; v_fact_key := NULL;
      v_disposition := 'rejected'; v_status := 'rejected';
      v_reason := 'TOMBSTONED'; v_result := 'TOMBSTONED';
      v_decided_at := v_now; v_purged_at := v_now; v_action := 'REJECT';
    ELSE
      SELECT memory.* INTO v_exact
      FROM user_memories memory
      WHERE memory.user_id = v_job.user_id
        AND memory.scope_type = v_scope_type
        AND memory.project_id IS NOT DISTINCT FROM v_project_id
        AND memory.scope_conversation_id IS NOT DISTINCT FROM v_conversation_id
        AND memory.normalized_content = v_normalized
        AND memory.deleted_at IS NULL
        AND memory.enabled
        AND memory.lifecycle_status = 'active'
      FOR UPDATE;

      v_exact_found := FOUND;

      IF v_exact_found THEN
        v_action := 'NOOP';
        v_reason := 'EXACT_NOOP';
        v_target_ids := array_append(v_target_ids, v_exact.id);
      END IF;

      SELECT count(*), COALESCE(bool_or(
        memory.authority_kind IN ('manual', 'direct_user', 'confirmed', 'import')
      ), false)
      INTO v_related_count, v_manual_related
      FROM user_memories memory
      WHERE memory.user_id = v_job.user_id
        AND memory.scope_type = v_scope_type
        AND memory.project_id IS NOT DISTINCT FROM v_project_id
        AND memory.scope_conversation_id IS NOT DISTINCT FROM v_conversation_id
        AND memory.deleted_at IS NULL
        AND memory.enabled
        AND memory.lifecycle_status = 'active'
        AND (
          memory.id = ANY(v_target_ids)
          OR (v_fact_key IS NOT NULL AND memory.fact_key = v_fact_key)
        );

      IF NOT v_exact_found AND v_action = 'REJECT' THEN
        v_content := NULL; v_normalized := NULL; v_tags := '{}';
        v_subject_key := NULL; v_fact_key := NULL;
        v_disposition := 'rejected'; v_status := 'rejected';
        v_reason := 'MODEL_REJECTED'; v_result := 'MODEL_REJECTED';
        v_decided_at := v_now; v_purged_at := v_now;
      ELSIF v_related_count = 0 AND (
        v_confidence < 0.80 OR v_scope_confidence < 0.80
        OR v_temporal_basis IN ('relative_ambiguous', 'model_inferred')
        OR v_action IN ('MERGE', 'SUPERSEDE', 'NOOP')
      ) THEN
        v_disposition := 'review'; v_status := 'pending';
        v_reason := CASE
          WHEN v_confidence < 0.80 THEN 'LOW_CONFIDENCE'
          WHEN v_scope_confidence < 0.80 THEN 'SCOPE_UNCERTAIN'
          WHEN v_temporal_basis IN ('relative_ambiguous', 'model_inferred')
            THEN 'TEMPORAL_UNCERTAIN'
          ELSE 'DECISION_TARGET_MISSING'
        END;
      ELSIF v_exact_found THEN
        NULL;
      ELSIF v_related_count = 0 THEN
        NULL;
      ELSIF v_related_count > 5 THEN
        v_disposition := 'review'; v_status := 'pending';
        v_reason := 'RELATED_SET_OVERFLOW';
      ELSIF v_manual_related THEN
        v_disposition := 'review'; v_status := 'pending';
        v_reason := 'MANUAL_CONFLICT';
      ELSIF v_action = 'NOOP' THEN
        v_reason := 'SEMANTIC_NOOP';
      ELSE
        v_disposition := 'review'; v_status := 'pending';
        v_reason := CASE v_action
          WHEN 'MERGE' THEN 'MERGE_REVIEW'
          WHEN 'SUPERSEDE' THEN 'SUPERSEDE_REVIEW'
          ELSE 'FACT_CONFLICT'
        END;
      END IF;
    END IF;

    INSERT INTO user_memory_review_suggestions (
      id, user_id, capture_job_id, event_id, ordinal,
      candidate_type, candidate_content, normalized_content, candidate_hash,
      importance, tags, subject_key, fact_key, sensitivity, confidence,
      confidence_band, proposed_scope_type, proposed_project_id,
      proposed_conversation_id, scope_generation, scope_confidence,
      confirmation_kind, temporal_basis, temporal_parser_version,
      observed_at, valid_from, valid_to, fact_expires_at, proposed_action,
      disposition, decision_reason_code, status, visibility_epoch,
      extraction_profile_id, decision_profile_id, review_expires_at,
      decided_at, result_code, purged_at, created_at
    ) VALUES (
      v_id, v_job.user_id, v_job.job_id, v_job.event_id, v_index,
      v_type, v_content, v_normalized, v_hash, v_importance, v_tags,
      v_subject_key, v_fact_key, v_sensitivity, v_confidence,
      v_confidence_band, v_scope_type, v_project_id, v_conversation_id,
      v_scope_generation, v_scope_confidence, v_confirmation,
      v_temporal_basis, v_temporal_parser, v_observed_at,
      v_valid_from, v_valid_to, v_fact_expires_at, v_action,
      v_disposition, v_reason, v_status, v_job.visibility_epoch,
      p_extraction_profile_id, p_decision_profile_id,
      v_now + interval '30 days', v_decided_at, v_result, v_purged_at, v_now
    );

    INSERT INTO user_memory_review_evidence (
      suggestion_id, source_message_id, user_id, source_conversation_id,
      evidence_role, source_content_hash, observed_at
    )
    SELECT
      v_id, source.id, v_job.user_id, source.conversation_id,
      'user', encode(sha256(convert_to(source.content, 'UTF8')), 'hex'),
      COALESCE(source.completed_at, source.created_at)
    FROM messages source
    WHERE source.id = ANY(v_authority_ids);

    INSERT INTO user_memory_review_evidence (
      suggestion_id, source_message_id, user_id, source_conversation_id,
      evidence_role, source_content_hash, observed_at
    )
    SELECT
      v_id, context.id, v_job.user_id, context.conversation_id,
      'assistant_context', encode(sha256(convert_to(context.content, 'UTF8')), 'hex'),
      COALESCE(context.completed_at, context.created_at)
    FROM messages context
    WHERE context.id = ANY(v_context_ids)
    ON CONFLICT (suggestion_id, source_message_id) DO NOTHING;

    INSERT INTO user_memory_review_targets (
      suggestion_id, memory_id, user_id, expected_revision
    )
    SELECT v_id, memory.id, v_job.user_id, memory.revision
    FROM user_memories memory
    WHERE memory.user_id = v_job.user_id
      AND memory.scope_type = v_scope_type
      AND memory.project_id IS NOT DISTINCT FROM v_project_id
      AND memory.scope_conversation_id IS NOT DISTINCT FROM v_conversation_id
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND (
        memory.id = ANY(v_target_ids)
        OR (v_fact_key IS NOT NULL AND memory.fact_key = v_fact_key)
      )
    ORDER BY
      CASE
        WHEN v_exact_found AND memory.id = v_exact.id THEN 0
        WHEN memory.authority_kind IN (
          'manual', 'direct_user', 'confirmed', 'import'
        ) THEN 1
        ELSE 2
      END,
      memory.updated_at DESC,
      memory.id
    LIMIT 5;
  END LOOP;

  IF v_count > 0 THEN
    UPDATE memory_outbox outbox
    SET max_attempts = 128,
        updated_at = GREATEST(outbox.updated_at, v_now)
    WHERE outbox.event_id = v_job.event_id;

    INSERT INTO memory_jobs (
      job_id, user_id, event_id, stage, idempotency_key,
      scope_generation, visibility_epoch, max_attempts,
      available_at, created_at, updated_at
    ) VALUES (
      p_expiry_job_id, v_job.user_id, v_job.event_id, 'review_expire',
      'memory:review-expire:v1:' || v_job.event_id::TEXT,
      1, v_job.visibility_epoch, 128,
      v_now + interval '30 days', v_now, v_now
    )
    ON CONFLICT (event_id, stage) DO NOTHING;

    IF NOT EXISTS (
      SELECT 1 FROM memory_jobs expiry
      WHERE expiry.event_id = v_job.event_id
        AND expiry.stage = 'review_expire'
        AND expiry.user_id = v_job.user_id
    ) THEN
      RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_REVIEW_EXPIRY_JOB_CONFLICT';
    END IF;
  END IF;

  RETURN QUERY SELECT
    v_count::SMALLINT,
    count(*) FILTER (WHERE disposition = 'shadow')::SMALLINT,
    count(*) FILTER (WHERE disposition = 'review')::SMALLINT,
    count(*) FILTER (WHERE disposition = 'rejected')::SMALLINT
  FROM user_memory_review_suggestions suggestion
  WHERE suggestion.capture_job_id = p_job_id;
END
$function$;

CREATE FUNCTION memory_worker_expire_capture_reviews(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS INTEGER
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_capture_job_id UUID;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_count INTEGER;
BEGIN
  SELECT job.* INTO v_job
  FROM memory_jobs job
  WHERE job.job_id = p_job_id
    AND job.stage = 'review_expire'
    AND job.status = 'processing'
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > v_now;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM memory_outbox outbox
    WHERE outbox.event_id = v_job.event_id
      AND outbox.user_id = v_job.user_id
      AND outbox.status = 'processing'
      AND outbox.lease_owner = p_worker_id
      AND outbox.lease_token = p_lease_token
      AND outbox.lease_expires_at > v_now
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_OUTBOX_LEASE_LOST';
  END IF;

  SELECT batch.capture_job_id INTO v_capture_job_id
  FROM memory_capture_candidate_batches batch
  WHERE batch.event_id = v_job.event_id
    AND batch.user_id = v_job.user_id
    AND batch.review_expires_at <= v_now
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'MEMORY_REVIEW_EXPIRY_TARGET_DRIFT';
  END IF;

  UPDATE user_memory_review_suggestions suggestion
  SET candidate_content = NULL,
      normalized_content = NULL,
      tags = '{}',
      subject_key = NULL,
      fact_key = NULL,
      status = 'expired',
      decided_at = v_now,
      result_code = 'PLAINTEXT_EXPIRED',
      purged_at = v_now
  WHERE suggestion.capture_job_id = v_capture_job_id
    AND suggestion.user_id = v_job.user_id
    AND suggestion.status IN ('shadow', 'pending')
    AND suggestion.review_expires_at <= v_now;
  GET DIAGNOSTICS v_count = ROW_COUNT;
  RETURN v_count;
END
$function$;

DO $harden_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_worker_hydrate_capture_v2(uuid,uuid,uuid)',
    'memory_worker_propose_capture_candidates(uuid,uuid,uuid,uuid,smallint,text,text,jsonb)',
    'memory_worker_expire_capture_reviews(uuid,uuid,uuid)'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name, function_identity, schema_name
    );
  END LOOP;
END
$harden_functions$;

GRANT SELECT, INSERT, UPDATE ON memory_capture_candidate_batches TO memory_runtime_owner;
GRANT SELECT, INSERT, UPDATE ON user_memory_review_suggestions TO memory_runtime_owner;
GRANT SELECT, INSERT ON user_memory_review_targets TO memory_runtime_owner;
GRANT SELECT, INSERT ON user_memory_review_evidence TO memory_runtime_owner;

ALTER FUNCTION memory_worker_hydrate_capture_v2(UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_propose_capture_candidates(
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, JSONB
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_expire_capture_reviews(UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;

REVOKE ALL ON
  memory_capture_candidate_batches,
  user_memory_review_suggestions,
  user_memory_review_targets,
  user_memory_review_evidence
FROM PUBLIC, go_api_runtime, memory_worker_runtime;

-- The PR4 function remains for a guarded rollback, but no PR5/N-1 worker may
-- retain authority to auto-apply extracted candidates into canonical rows.
REVOKE EXECUTE ON FUNCTION memory_worker_hydrate_capture(UUID, UUID, UUID)
  FROM memory_worker_runtime;
REVOKE EXECUTE ON FUNCTION memory_worker_apply_capture_candidate(
  UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[]
) FROM memory_worker_runtime;

REVOKE ALL ON FUNCTION memory_worker_hydrate_capture_v2(UUID, UUID, UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_propose_capture_candidates(
  UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, JSONB
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_expire_capture_reviews(UUID, UUID, UUID)
  FROM PUBLIC;

GRANT EXECUTE ON FUNCTION memory_worker_hydrate_capture_v2(UUID, UUID, UUID),
  memory_worker_propose_capture_candidates(
    UUID, UUID, UUID, UUID, SMALLINT, TEXT, TEXT, JSONB
  ),
  memory_worker_expire_capture_reviews(UUID, UUID, UUID)
TO memory_worker_runtime;

DO $owner_create_revocation$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner',
    current_schema()
  );
END
$owner_create_revocation$;
