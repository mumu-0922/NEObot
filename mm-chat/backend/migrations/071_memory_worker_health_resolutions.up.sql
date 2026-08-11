-- Make Memory health actionable without deleting immutable capture evidence.
-- Scheduled governance maintenance is not capture indexing work, while an
-- explicitly reviewed historical extract dead letter may be acknowledged
-- through a narrow, user-bound, content-free capability.

DO $memory_worker_health_resolution_prerequisite$
BEGIN
  IF to_regprocedure('memory_user_health(uuid)') IS NULL
    OR to_regclass('memory_jobs') IS NULL
    OR to_regclass('user_memory_search_projections') IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_WORKER_HEALTH_RESOLUTION_REQUIRES_070';
  END IF;
END
$memory_worker_health_resolution_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

ALTER TABLE memory_jobs
  ADD CONSTRAINT memory_jobs_job_id_user_unique UNIQUE (job_id, user_id);

CREATE TABLE memory_job_health_resolutions (
  job_id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  error_code TEXT NOT NULL,
  resolution_code TEXT NOT NULL,
  resolved_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  resolved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_job_health_resolution_job_owner_fk
    FOREIGN KEY (job_id, user_id)
    REFERENCES memory_jobs(job_id, user_id) ON DELETE RESTRICT,
  CONSTRAINT memory_job_health_resolution_actor_owner
    CHECK (resolved_by_user_id = user_id),
  CONSTRAINT memory_job_health_resolution_error_code
    CHECK (error_code ~ '^[A-Z0-9_]{1,64}$'),
  CONSTRAINT memory_job_health_resolution_code_allowed CHECK (
    resolution_code IN (
      'source_no_longer_current',
      'historical_failure_accepted'
    )
  )
);

CREATE FUNCTION memory_job_health_resolution_append_only_guard()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_APPEND_ONLY';
END
$function$;

CREATE TRIGGER memory_job_health_resolutions_append_only
BEFORE UPDATE OR DELETE ON memory_job_health_resolutions
FOR EACH ROW EXECUTE FUNCTION memory_job_health_resolution_append_only_guard();

CREATE FUNCTION memory_acknowledge_job_health(
  p_user_id UUID,
  p_job_id UUID,
  p_expected_error_code TEXT,
  p_resolution_code TEXT
)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_job memory_jobs%ROWTYPE;
  v_existing memory_job_health_resolutions%ROWTYPE;
BEGIN
  IF p_user_id IS NULL
    OR p_job_id IS NULL
    OR p_expected_error_code IS NULL
    OR p_resolution_code IS NULL
    OR p_expected_error_code !~ '^[A-Z0-9_]{1,64}$'
    OR p_resolution_code NOT IN (
      'source_no_longer_current',
      'historical_failure_accepted'
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_INVALID';
  END IF;

  SELECT job.* INTO v_job
  FROM memory_jobs job
  WHERE job.job_id = p_job_id
    AND job.user_id = p_user_id
  FOR SHARE;

  IF NOT FOUND
    OR v_job.stage <> 'extract'
    OR v_job.status <> 'dead_letter'
    OR v_job.error_code IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_NOT_ELIGIBLE';
  END IF;

  IF v_job.error_code <> p_expected_error_code THEN
    RAISE EXCEPTION USING
      ERRCODE = '40001',
      MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_DRIFT';
  END IF;

  IF p_resolution_code = 'source_no_longer_current' THEN
    IF v_job.error_code <> 'SOURCE_DRIFT'
      OR NOT EXISTS (
        SELECT 1
        FROM conversations conversation
        WHERE conversation.id = v_job.source_conversation_id
          AND conversation.user_id = v_job.user_id
          AND (
            conversation.deleted_at IS NOT NULL
            OR conversation.status <> 'active'
          )
      )
    THEN
      RAISE EXCEPTION USING
        ERRCODE = '55000',
        MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_SOURCE_CURRENT';
    END IF;
  ELSIF v_job.completed_at > clock_timestamp() - interval '24 hours' THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_NOT_HISTORICAL';
  END IF;

  INSERT INTO memory_job_health_resolutions (
    job_id, user_id, error_code, resolution_code, resolved_by_user_id
  ) VALUES (
    v_job.job_id, v_job.user_id, v_job.error_code,
    p_resolution_code, p_user_id
  )
  ON CONFLICT (job_id) DO NOTHING;

  IF FOUND THEN
    RETURN true;
  END IF;

  SELECT resolution.* INTO v_existing
  FROM memory_job_health_resolutions resolution
  WHERE resolution.job_id = p_job_id;

  IF v_existing.user_id <> p_user_id
    OR v_existing.error_code <> p_expected_error_code
    OR v_existing.resolution_code <> p_resolution_code
    OR v_existing.resolved_by_user_id <> p_user_id
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '40001',
      MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_CONFLICT';
  END IF;

  RETURN false;
END
$function$;

CREATE OR REPLACE FUNCTION memory_user_health(p_user_id UUID)
RETURNS TABLE (
  worker_available BOOLEAN,
  embedding_worker_available BOOLEAN,
  capture_pending_count BIGINT,
  capture_processing_count BIGINT,
  capture_dead_letter_count BIGINT,
  projection_ready_count BIGINT,
  projection_pending_count BIGINT,
  projection_failed_count BIGINT
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  WITH live_workers AS MATERIALIZED (
    SELECT
      count(*) > 0 AS worker_available,
      count(*) FILTER (WHERE heartbeat.embedding_enabled) > 0
        AS embedding_worker_available
    FROM memory_worker_heartbeats heartbeat
    WHERE heartbeat.expires_at > now()
  ), capture AS MATERIALIZED (
    SELECT
      count(*) FILTER (WHERE job.status = 'pending') AS pending_count,
      count(*) FILTER (WHERE job.status = 'processing') AS processing_count,
      count(*) FILTER (WHERE job.status = 'dead_letter') AS dead_letter_count
    FROM memory_jobs job
    WHERE job.user_id = p_user_id
      AND job.stage = 'extract'
      AND NOT EXISTS (
        SELECT 1
        FROM memory_job_health_resolutions resolution
        WHERE resolution.job_id = job.job_id
          AND resolution.user_id = job.user_id
      )
  ), projections AS MATERIALIZED (
    SELECT coalesce(projection.embedding_status, 'pending') AS embedding_status
    FROM user_memories memory
    JOIN user_memory_state state
      ON state.user_id = memory.user_id
     AND state.visibility_epoch = memory.visibility_epoch
    JOIN user_memory_settings settings
      ON settings.user_id = memory.user_id
     AND settings.enabled
     AND settings.search_enabled
     AND (
       memory.sensitivity = 'normal'
       OR settings.sensitive_memory_enabled
     )
    LEFT JOIN user_memory_search_projections projection
      ON projection.memory_id = memory.id
     AND projection.user_id = memory.user_id
     AND projection.memory_revision = memory.revision
     AND projection.content_hash = memory.content_hash
     AND projection.visibility_epoch = memory.visibility_epoch
     AND projection.scope_type = memory.scope_type
     AND projection.scope_generation = memory.scope_generation
     AND projection.projection_generation = state.active_projection_generation
    LEFT JOIN projects scoped_project
      ON memory.scope_type = 'project'
     AND scoped_project.id = memory.project_id
     AND scoped_project.user_id = memory.user_id
     AND scoped_project.lifecycle_status = 'active'
     AND scoped_project.scope_generation = memory.scope_generation
    LEFT JOIN conversations scoped_conversation
      ON memory.scope_type = 'conversation'
     AND scoped_conversation.id = memory.scope_conversation_id
     AND scoped_conversation.user_id = memory.user_id
     AND scoped_conversation.deleted_at IS NULL
     AND scoped_conversation.memory_scope_generation = memory.scope_generation
    WHERE memory.user_id = p_user_id
      AND memory.deleted_at IS NULL
      AND memory.enabled
      AND memory.lifecycle_status = 'active'
      AND (memory.valid_from IS NULL OR memory.valid_from <= now())
      AND (memory.valid_to IS NULL OR now() < memory.valid_to)
      AND (memory.expires_at IS NULL OR now() < memory.expires_at)
      AND (
        memory.scope_type = 'global'
        OR (
          memory.scope_type = 'project'
          AND scoped_project.id IS NOT NULL
        )
        OR (
          memory.scope_type = 'conversation'
          AND scoped_conversation.id IS NOT NULL
        )
      )
  ), projection_counts AS MATERIALIZED (
    SELECT
      count(*) FILTER (WHERE embedding_status = 'ready') AS ready_count,
      count(*) FILTER (WHERE embedding_status = 'pending') AS pending_count,
      count(*) FILTER (WHERE embedding_status = 'failed') AS failed_count
    FROM projections
  )
  SELECT
    live_workers.worker_available,
    live_workers.embedding_worker_available,
    capture.pending_count,
    capture.processing_count,
    capture.dead_letter_count,
    projection_counts.ready_count,
    projection_counts.pending_count,
    projection_counts.failed_count
  FROM live_workers
  CROSS JOIN capture
  CROSS JOIN projection_counts
$function$;

DO $harden_memory_worker_health_resolution_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_job_health_resolution_append_only_guard()',
    'memory_acknowledge_job_health(uuid,uuid,text,text)',
    'memory_user_health(uuid)'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,
      function_identity,
      schema_name
    );
    EXECUTE format(
      'ALTER FUNCTION %I.%s OWNER TO memory_runtime_owner',
      schema_name,
      function_identity
    );
  END LOOP;
END
$harden_memory_worker_health_resolution_functions$;

ALTER TABLE memory_job_health_resolutions OWNER TO memory_runtime_owner;

REVOKE ALL ON memory_job_health_resolutions
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_job_health_resolution_append_only_guard()
  FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_acknowledge_job_health(UUID, UUID, TEXT, TEXT)
  FROM PUBLIC, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_user_health(UUID)
  FROM PUBLIC, memory_worker_runtime;

GRANT EXECUTE ON FUNCTION memory_acknowledge_job_health(UUID, UUID, TEXT, TEXT),
  memory_user_health(UUID)
TO go_api_runtime;

DO $memory_worker_health_resolution_schema_privileges$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'GRANT USAGE ON SCHEMA %I TO memory_runtime_owner, go_api_runtime',
    schema_name
  );
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM memory_runtime_owner, go_api_runtime, memory_worker_runtime',
    schema_name
  );
END
$memory_worker_health_resolution_schema_privileges$;
