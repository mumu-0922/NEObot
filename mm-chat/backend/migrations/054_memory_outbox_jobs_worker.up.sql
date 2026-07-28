-- Memory v2 PR3 durable capture queue. Redis may wake consumers, but these
-- PostgreSQL rows and lease-fenced functions remain the only job authority.

DO $roles$
DECLARE
  role_name TEXT;
  can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create
  FROM pg_roles WHERE rolname = current_user;

  FOREACH role_name IN ARRAY ARRAY[
    'memory_runtime_owner',
    'memory_worker_runtime'
  ] LOOP
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
      IF NOT can_create THEN
        RAISE EXCEPTION USING
          ERRCODE = '42501',
          MESSAGE = 'MEMORY_REQUIRED_ROLE_MISSING';
      END IF;
      EXECUTE format(
        'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',
        role_name
      );
    END IF;

    IF EXISTS (
      SELECT 1
      FROM pg_roles
      WHERE rolname = role_name
        AND (
          rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole
          OR rolreplication OR rolbypassrls
        )
    ) THEN
      RAISE EXCEPTION USING
        ERRCODE = '42501',
        MESSAGE = 'MEMORY_REQUIRED_ROLE_MUST_BE_RESTRICTED';
    END IF;
  END LOOP;

  IF pg_has_role('go_api_runtime', 'memory_runtime_owner', 'MEMBER')
    OR pg_has_role('go_api_runtime', 'memory_worker_runtime', 'MEMBER')
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'MEMORY_GO_API_RUNTIME_FORBIDDEN_MEMBERSHIP';
  END IF;

  IF pg_has_role('memory_worker_runtime', 'memory_runtime_owner', 'MEMBER') THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'MEMORY_WORKER_RUNTIME_FORBIDDEN_OWNER_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE TABLE memory_outbox (
  event_id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_schema_major SMALLINT NOT NULL,
  event_type TEXT NOT NULL,
  aggregate_id UUID NOT NULL,
  visibility_epoch BIGINT NOT NULL DEFAULT 1,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_owner UUID,
  lease_token UUID,
  lease_expires_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT,
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_outbox_user_event_aggregate_unique
    UNIQUE (user_id, event_type, aggregate_id),
  CONSTRAINT memory_outbox_schema_major_supported
    CHECK (event_schema_major IN (1, 2)),
  CONSTRAINT memory_outbox_event_type_allowed
    CHECK (event_type = 'turn.completed'),
  CONSTRAINT memory_outbox_visibility_epoch_positive
    CHECK (visibility_epoch >= 1),
  CONSTRAINT memory_outbox_payload_object
    CHECK (jsonb_typeof(payload) = 'object'),
  CONSTRAINT memory_outbox_payload_id_only CHECK (
    payload ? 'schemaMajor'
    AND payload ? 'conversationId'
    AND payload ? 'userMessageId'
    AND payload ? 'assistantMessageId'
    AND payload ? 'sourceHash'
    AND payload ? 'memoryScopeGeneration'
    AND payload ? 'visibilityEpoch'
    AND payload ? 'providerProfile'
    AND (payload - ARRAY[
      'schemaMajor',
      'conversationId',
      'userMessageId',
      'assistantMessageId',
      'sourceHash',
      'projectId',
      'memoryScopeGeneration',
      'projectScopeGeneration',
      'visibilityEpoch',
      'providerProfile'
    ]::TEXT[]) = '{}'::JSONB
    AND jsonb_typeof(payload->'providerProfile') = 'object'
    AND (payload->'providerProfile') ? 'providerSource'
    AND (payload->'providerProfile') ? 'providerId'
    AND (payload->'providerProfile') ? 'modelId'
    AND (payload->'providerProfile') ? 'profileHash'
    AND ((payload->'providerProfile') - ARRAY[
      'providerSource',
      'providerId',
      'modelId',
      'profileHash',
      'providerConfigId'
    ]::TEXT[]) = '{}'::JSONB
  ),
  CONSTRAINT memory_outbox_status_allowed
    CHECK (status IN ('pending', 'processing', 'completed', 'dead_letter')),
  CONSTRAINT memory_outbox_attempts_bounded CHECK (
    max_attempts BETWEEN 1 AND 32
    AND attempt_count BETWEEN 0 AND max_attempts
  ),
  CONSTRAINT memory_outbox_error_code_sanitized CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  CONSTRAINT memory_outbox_last_error_sanitized CHECK (
    last_error IS NULL OR last_error ~ '^[A-Z0-9_]{1,64}$'
  ),
  CONSTRAINT memory_outbox_state_shape CHECK (
    (
      status = 'pending'
      AND lease_owner IS NULL
      AND lease_token IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NULL
      AND error_code IS NULL
      AND attempt_count < max_attempts
    )
    OR (
      status = 'processing'
      AND lease_owner IS NOT NULL
      AND lease_token IS NOT NULL
      AND lease_expires_at IS NOT NULL
      AND completed_at IS NULL
      AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'completed'
      AND lease_owner IS NULL
      AND lease_token IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NOT NULL
      AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'dead_letter'
      AND lease_owner IS NULL
      AND lease_token IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NOT NULL
      AND error_code IS NOT NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
  ),
  CONSTRAINT memory_outbox_available_after_created
    CHECK (available_at >= created_at),
  CONSTRAINT memory_outbox_lease_after_created
    CHECK (lease_expires_at IS NULL OR lease_expires_at >= created_at),
  CONSTRAINT memory_outbox_completed_after_created
    CHECK (completed_at IS NULL OR completed_at >= created_at),
  CONSTRAINT memory_outbox_timestamps_order
    CHECK (updated_at >= created_at)
);

CREATE INDEX idx_memory_outbox_pending
  ON memory_outbox(available_at, created_at, event_id)
  WHERE status = 'pending';
CREATE INDEX idx_memory_outbox_expired_lease
  ON memory_outbox(lease_expires_at, event_id)
  WHERE status = 'processing';
CREATE INDEX idx_memory_outbox_user_created
  ON memory_outbox(user_id, created_at DESC, event_id);

CREATE TABLE memory_jobs (
  job_id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_id UUID NOT NULL REFERENCES memory_outbox(event_id) ON DELETE CASCADE,
  stage TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  source_conversation_id UUID NOT NULL,
  source_message_id UUID NOT NULL,
  assistant_message_id UUID NOT NULL,
  source_hash TEXT NOT NULL,
  provider_source TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  provider_record_id UUID,
  provider_config_updated_at TIMESTAMPTZ,
  model_id TEXT NOT NULL,
  processing_profile TEXT NOT NULL,
  scope_generation BIGINT NOT NULL,
  project_scope_generation BIGINT,
  visibility_epoch BIGINT NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'pending',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_owner UUID,
  lease_token UUID,
  lease_expires_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_jobs_event_stage_unique UNIQUE (event_id, stage),
  CONSTRAINT memory_jobs_stage_allowed CHECK (
    stage IN (
      'extract', 'resolve', 'embed', 'l2_refresh', 'l3_refresh',
      'purge', 'rebuild'
    )
  ),
  CONSTRAINT memory_jobs_idempotency_key_bounded CHECK (
    octet_length(idempotency_key) BETWEEN 1 AND 256
    AND idempotency_key = trim(idempotency_key)
  ),
  CONSTRAINT memory_jobs_source_hash_check
    CHECK (source_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_jobs_provider_source_allowed
    CHECK (provider_source IN ('server-default', 'server-stored', 'request', 'legacy')),
  CONSTRAINT memory_jobs_provider_id_bounded CHECK (
    octet_length(provider_id) BETWEEN 1 AND 512
    AND provider_id = trim(provider_id)
  ),
  CONSTRAINT memory_jobs_provider_config_shape CHECK (
    (provider_record_id IS NULL AND provider_config_updated_at IS NULL)
    OR (provider_record_id IS NOT NULL AND provider_config_updated_at IS NOT NULL)
  ),
  CONSTRAINT memory_jobs_model_id_bounded CHECK (
    octet_length(model_id) BETWEEN 1 AND 512
    AND model_id = trim(model_id)
  ),
  CONSTRAINT memory_jobs_processing_profile_check
    CHECK (processing_profile ~ '^[0-9a-f]{64}$'),
  CONSTRAINT memory_jobs_scope_generation_positive
    CHECK (scope_generation >= 1),
  CONSTRAINT memory_jobs_project_scope_generation_positive
    CHECK (project_scope_generation IS NULL OR project_scope_generation >= 1),
  CONSTRAINT memory_jobs_visibility_epoch_positive
    CHECK (visibility_epoch >= 1),
  CONSTRAINT memory_jobs_status_allowed
    CHECK (status IN ('pending', 'processing', 'completed', 'dead_letter')),
  CONSTRAINT memory_jobs_attempts_bounded CHECK (
    max_attempts BETWEEN 1 AND 32
    AND attempt_count BETWEEN 0 AND max_attempts
  ),
  CONSTRAINT memory_jobs_error_code_sanitized CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  CONSTRAINT memory_jobs_state_shape CHECK (
    (
      status = 'pending'
      AND lease_owner IS NULL
      AND lease_token IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NULL
      AND error_code IS NULL
      AND attempt_count < max_attempts
    )
    OR (
      status = 'processing'
      AND lease_owner IS NOT NULL
      AND lease_token IS NOT NULL
      AND lease_expires_at IS NOT NULL
      AND completed_at IS NULL
      AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'completed'
      AND lease_owner IS NULL
      AND lease_token IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NOT NULL
      AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'dead_letter'
      AND lease_owner IS NULL
      AND lease_token IS NULL
      AND lease_expires_at IS NULL
      AND completed_at IS NOT NULL
      AND error_code IS NOT NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
  ),
  CONSTRAINT memory_jobs_available_after_created
    CHECK (available_at >= created_at),
  CONSTRAINT memory_jobs_lease_after_created
    CHECK (lease_expires_at IS NULL OR lease_expires_at >= created_at),
  CONSTRAINT memory_jobs_completed_after_created
    CHECK (completed_at IS NULL OR completed_at >= created_at),
  CONSTRAINT memory_jobs_timestamps_order
    CHECK (updated_at >= created_at)
);

CREATE INDEX idx_memory_jobs_claim
  ON memory_jobs(available_at, created_at, job_id)
  WHERE status = 'pending';
CREATE INDEX idx_memory_jobs_expired_lease
  ON memory_jobs(lease_expires_at, job_id)
  WHERE status = 'processing';
CREATE INDEX idx_memory_jobs_user_created
  ON memory_jobs(user_id, created_at DESC, job_id);

CREATE FUNCTION memory_append_turn_completed_event(
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
    'visibilityEpoch', 1,
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
    1,
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
    1
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

CREATE FUNCTION memory_worker_claim_job(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER
) RETURNS TABLE (
  job_id UUID,
  user_id UUID,
  event_id UUID,
  event_schema_major SMALLINT,
  stage TEXT,
  attempt_count INTEGER,
  max_attempts INTEGER,
  source_conversation_id UUID,
  source_message_id UUID,
  assistant_message_id UUID,
  source_hash TEXT,
  provider_source TEXT,
  provider_id TEXT,
  provider_record_id UUID,
  model_id TEXT,
  processing_profile TEXT,
  scope_generation BIGINT,
  project_scope_generation BIGINT,
  visibility_epoch BIGINT,
  lease_expires_at TIMESTAMPTZ
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job_id UUID;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_lease_expires_at TIMESTAMPTZ;
BEGIN
  IF p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_seconds < 5 OR p_lease_seconds > 900
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_JOB_LEASE_INVALID';
  END IF;
  v_lease_expires_at := v_now + make_interval(secs => p_lease_seconds);

  -- An expired final-attempt lease cannot be reclaimed without violating the
  -- bounded attempt counter. Terminalize it before selecting the next claim
  -- so a crash on the last attempt never leaves a permanent processing row.
  UPDATE memory_outbox outbox
  SET status = 'dead_letter',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = v_now,
      error_code = 'LEASE_EXPIRED',
      last_error = 'LEASE_EXPIRED',
      updated_at = v_now
  FROM memory_jobs job
  WHERE job.event_id = outbox.event_id
    AND job.status = 'processing'
    AND job.lease_expires_at <= v_now
    AND job.attempt_count >= job.max_attempts
    AND outbox.status = 'processing'
    AND outbox.lease_owner = job.lease_owner
    AND outbox.lease_token = job.lease_token;

  UPDATE memory_jobs job
  SET status = 'dead_letter',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = v_now,
      error_code = 'LEASE_EXPIRED',
      updated_at = v_now
  WHERE job.status = 'processing'
    AND job.lease_expires_at <= v_now
    AND job.attempt_count >= job.max_attempts;

  SELECT candidate.job_id INTO v_job_id
  FROM memory_jobs candidate
  WHERE (
      candidate.status = 'pending'
      AND candidate.available_at <= v_now
      AND candidate.attempt_count < candidate.max_attempts
    ) OR (
      candidate.status = 'processing'
      AND candidate.lease_expires_at <= v_now
      AND candidate.attempt_count < candidate.max_attempts
    )
  ORDER BY
    CASE WHEN candidate.status = 'processing' THEN candidate.lease_expires_at ELSE candidate.available_at END,
    candidate.created_at,
    candidate.job_id
  FOR UPDATE SKIP LOCKED
  LIMIT 1;

  IF v_job_id IS NULL THEN
    RETURN;
  END IF;

  UPDATE memory_jobs job
  SET status = 'processing',
      attempt_count = job.attempt_count + 1,
      lease_owner = p_worker_id,
      lease_token = p_lease_token,
      lease_expires_at = v_lease_expires_at,
      completed_at = NULL,
      error_code = NULL,
      updated_at = v_now
  WHERE job.job_id = v_job_id;

  UPDATE memory_outbox outbox
  SET status = 'processing',
      attempt_count = job.attempt_count,
      lease_owner = job.lease_owner,
      lease_token = job.lease_token,
      lease_expires_at = job.lease_expires_at,
      completed_at = NULL,
      error_code = NULL,
      last_error = NULL,
      updated_at = v_now
  FROM memory_jobs job
  WHERE job.job_id = v_job_id AND outbox.event_id = job.event_id;

  RETURN QUERY
  SELECT
    job.job_id,
    job.user_id,
    job.event_id,
    outbox.event_schema_major,
    job.stage,
    job.attempt_count,
    job.max_attempts,
    job.source_conversation_id,
    job.source_message_id,
    job.assistant_message_id,
    job.source_hash,
    job.provider_source,
    job.provider_id,
    job.provider_record_id,
    job.model_id,
    job.processing_profile,
    job.scope_generation,
    job.project_scope_generation,
    job.visibility_epoch,
    job.lease_expires_at
  FROM memory_jobs job
  JOIN memory_outbox outbox ON outbox.event_id = job.event_id
  WHERE job.job_id = v_job_id;
END
$function$;

CREATE FUNCTION memory_worker_hydrate_capture(
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

CREATE FUNCTION memory_worker_apply_capture_candidate(
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
BEGIN
  PERFORM 1
  FROM memory_worker_hydrate_capture(p_job_id, p_worker_id, p_lease_token);

  SELECT * INTO v_job
  FROM memory_jobs
  WHERE job_id = p_job_id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  IF p_memory_type NOT IN (
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
      MESSAGE = 'MEMORY_CANDIDATE_INVALID';
  END IF;

  RETURN QUERY
  INSERT INTO user_memories (
    id,
    user_id,
    memory_type,
    content,
    normalized_content,
    importance,
    tags,
    source,
    source_conversation_id,
    source_message_id,
    enabled,
    scope_type
  ) VALUES (
    p_memory_id,
    v_job.user_id,
    p_memory_type,
    p_content,
    p_normalized_content,
    p_importance,
    p_tags,
    'ai',
    v_job.source_conversation_id,
    v_job.source_message_id,
    true,
    'global'
  )
  ON CONFLICT (user_id, normalized_content)
  WHERE deleted_at IS NULL AND scope_type = 'global' DO UPDATE SET
    memory_type = EXCLUDED.memory_type,
    content = EXCLUDED.content,
    importance = GREATEST(user_memories.importance, EXCLUDED.importance),
    tags = EXCLUDED.tags,
    source = EXCLUDED.source,
    source_conversation_id = EXCLUDED.source_conversation_id,
    source_message_id = EXCLUDED.source_message_id,
    enabled = true,
    updated_at = now()
  RETURNING
    user_memories.id,
    user_memories.user_id,
    user_memories.memory_type,
    user_memories.content,
    user_memories.normalized_content,
    user_memories.importance,
    to_json(user_memories.tags)::TEXT,
    user_memories.source,
    user_memories.source_conversation_id,
    user_memories.source_message_id,
    user_memories.enabled,
    user_memories.last_used_at,
    user_memories.created_at,
    user_memories.updated_at,
    user_memories.deleted_at;
END
$function$;

CREATE FUNCTION memory_worker_complete_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID
) RETURNS VOID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_event_id UUID;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  UPDATE memory_jobs
  SET status = 'completed',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = v_now,
      error_code = NULL,
      updated_at = v_now
  WHERE job_id = p_job_id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > v_now
  RETURNING event_id INTO v_event_id;
  IF v_event_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  UPDATE memory_outbox
  SET status = 'completed',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = v_now,
      error_code = NULL,
      last_error = NULL,
      updated_at = v_now
  WHERE event_id = v_event_id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_OUTBOX_LEASE_LOST';
  END IF;
END
$function$;

CREATE FUNCTION memory_worker_retry_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_error_code TEXT,
  p_available_at TIMESTAMPTZ,
  p_terminal BOOLEAN
) RETURNS TEXT
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_status TEXT;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_available_at TIMESTAMPTZ := GREATEST(p_available_at, clock_timestamp());
BEGIN
  IF p_error_code !~ '^[A-Z0-9_]{1,64}$' THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_JOB_ERROR_CODE_INVALID';
  END IF;

  SELECT * INTO v_job
  FROM memory_jobs
  WHERE job_id = p_job_id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > v_now
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_JOB_LEASE_LOST';
  END IF;

  IF p_terminal OR v_job.attempt_count >= v_job.max_attempts THEN
    v_status := 'dead_letter';
  ELSE
    v_status := 'pending';
  END IF;

  UPDATE memory_jobs
  SET status = v_status,
      available_at = CASE WHEN v_status = 'pending' THEN v_available_at ELSE available_at END,
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = CASE WHEN v_status = 'dead_letter' THEN v_now ELSE NULL END,
      error_code = CASE WHEN v_status = 'dead_letter' THEN p_error_code ELSE NULL END,
      updated_at = v_now
  WHERE job_id = v_job.job_id;

  UPDATE memory_outbox
  SET status = v_status,
      available_at = CASE WHEN v_status = 'pending' THEN v_available_at ELSE available_at END,
      attempt_count = v_job.attempt_count,
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = CASE WHEN v_status = 'dead_letter' THEN v_now ELSE NULL END,
      error_code = CASE WHEN v_status = 'dead_letter' THEN p_error_code ELSE NULL END,
      last_error = p_error_code,
      updated_at = v_now
  WHERE event_id = v_job.event_id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_OUTBOX_LEASE_LOST';
  END IF;

  RETURN v_status;
END
$function$;

CREATE FUNCTION memory_worker_readiness()
RETURNS TABLE (
  consumer_ready BOOLEAN,
  pending_count BIGINT,
  processing_count BIGINT,
  dead_letter_count BIGINT,
  oldest_pending_at TIMESTAMPTZ
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT
    true,
    count(*) FILTER (WHERE status = 'pending'),
    count(*) FILTER (WHERE status = 'processing'),
    count(*) FILTER (WHERE status = 'dead_letter'),
    min(created_at) FILTER (WHERE status = 'pending')
  FROM memory_jobs
$function$;

-- Pin every SECURITY DEFINER lookup to the application schema used by this
-- migration, while retaining SET search_path FROM CURRENT in the definitions
-- so isolated-schema migration replay remains valid.
DO $harden_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_append_turn_completed_event(uuid,uuid,uuid,uuid,uuid,uuid,text,text,text,smallint)',
    'memory_worker_claim_job(uuid,uuid,integer)',
    'memory_worker_hydrate_capture(uuid,uuid,uuid)',
    'memory_worker_apply_capture_candidate(uuid,uuid,uuid,uuid,text,text,text,smallint,text[])',
    'memory_worker_complete_job(uuid,uuid,uuid)',
    'memory_worker_retry_job(uuid,uuid,uuid,text,timestamp with time zone,boolean)',
    'memory_worker_readiness()'
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

DO $schema_privileges$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner, memory_worker_runtime, go_api_runtime',
    schema_name
  );
  EXECUTE format(
    'GRANT USAGE ON SCHEMA %I TO memory_runtime_owner, memory_worker_runtime, go_api_runtime',
    schema_name
  );
END
$schema_privileges$;

GRANT SELECT, INSERT, UPDATE ON memory_outbox, memory_jobs, user_memories
  TO memory_runtime_owner;
GRANT SELECT, UPDATE ON conversations TO memory_runtime_owner;
GRANT SELECT ON
  users,
  messages,
  projects,
  user_memory_settings,
  task_model_settings,
  provider_configs
  TO memory_runtime_owner;

ALTER FUNCTION memory_append_turn_completed_event(
  UUID, UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_claim_job(UUID, UUID, INTEGER)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_hydrate_capture(UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_apply_capture_candidate(
  UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[]
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_complete_job(UUID, UUID, UUID)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_retry_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_readiness()
  OWNER TO memory_runtime_owner;

REVOKE ALL ON memory_outbox, memory_jobs FROM PUBLIC;
REVOKE ALL ON memory_outbox, memory_jobs FROM go_api_runtime, memory_worker_runtime;

REVOKE ALL ON FUNCTION memory_append_turn_completed_event(
  UUID, UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_claim_job(UUID, UUID, INTEGER) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_hydrate_capture(UUID, UUID, UUID) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_apply_capture_candidate(
  UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[]
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_complete_job(UUID, UUID, UUID) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_retry_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
) FROM PUBLIC;
REVOKE ALL ON FUNCTION memory_worker_readiness() FROM PUBLIC;

GRANT EXECUTE ON FUNCTION memory_append_turn_completed_event(
  UUID, UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION memory_worker_claim_job(UUID, UUID, INTEGER),
  memory_worker_hydrate_capture(UUID, UUID, UUID),
  memory_worker_apply_capture_candidate(
    UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[]
  ),
  memory_worker_complete_job(UUID, UUID, UUID),
  memory_worker_retry_job(UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN),
  memory_worker_readiness()
TO memory_worker_runtime;

DO $owner_create_revocation$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner',
    current_schema()
  );
END
$owner_create_revocation$;
