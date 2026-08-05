-- Add a durable, content-free Memory Worker heartbeat and an authenticated
-- per-user health summary. This changes observability only: it does not grant
-- reader promotion, Provider authority, or runtime table CRUD.

DO $memory_worker_health_prerequisite$
BEGIN
  IF to_regprocedure('memory_worker_readiness()') IS NULL
    OR to_regclass('user_memory_search_projections') IS NULL
    OR to_regclass('user_memory_embedding_jobs') IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_WORKER_HEALTH_REQUIRES_MEMORY_HYBRID_FOUNDATION';
  END IF;
END
$memory_worker_health_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE TABLE memory_worker_heartbeats (
  worker_id UUID PRIMARY KEY,
  embedding_enabled BOOLEAN NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  heartbeat_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT memory_worker_heartbeat_time_order CHECK (
    heartbeat_at >= started_at AND expires_at > heartbeat_at
  )
);

CREATE INDEX idx_memory_worker_heartbeats_expiry
  ON memory_worker_heartbeats(expires_at, worker_id);

CREATE FUNCTION memory_worker_heartbeat(
  p_worker_id UUID,
  p_ttl_seconds INTEGER,
  p_embedding_enabled BOOLEAN
)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_worker_id IS NULL
    OR p_ttl_seconds IS NULL
    OR p_embedding_enabled IS NULL
    OR p_ttl_seconds < 5
    OR p_ttl_seconds > 120
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023', MESSAGE = 'MEMORY_WORKER_HEARTBEAT_INVALID';
  END IF;

  DELETE FROM memory_worker_heartbeats heartbeat
  WHERE heartbeat.expires_at < v_now - interval '1 day';

  INSERT INTO memory_worker_heartbeats(
    worker_id, embedding_enabled, started_at, heartbeat_at, expires_at
  ) VALUES (
    p_worker_id, p_embedding_enabled, v_now, v_now,
    v_now + make_interval(secs => p_ttl_seconds)
  )
  ON CONFLICT (worker_id) DO UPDATE SET
    embedding_enabled = EXCLUDED.embedding_enabled,
    heartbeat_at = EXCLUDED.heartbeat_at,
    expires_at = EXCLUDED.expires_at;

  RETURN true;
END
$function$;

CREATE FUNCTION memory_worker_retire(p_worker_id UUID)
RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  DELETE FROM memory_worker_heartbeats heartbeat
  WHERE heartbeat.worker_id = p_worker_id;
  RETURN FOUND;
END
$function$;

CREATE OR REPLACE FUNCTION memory_worker_readiness()
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
    EXISTS (
      SELECT 1
      FROM memory_worker_heartbeats heartbeat
      WHERE heartbeat.expires_at > now()
    ),
    count(*) FILTER (WHERE job.status = 'pending'),
    count(*) FILTER (WHERE job.status = 'processing'),
    count(*) FILTER (WHERE job.status = 'dead_letter'),
    min(job.created_at) FILTER (WHERE job.status = 'pending')
  FROM memory_jobs job
$function$;

CREATE FUNCTION memory_user_health(p_user_id UUID)
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

DO $harden_memory_worker_health_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'memory_worker_heartbeat(uuid,integer,boolean)',
    'memory_worker_retire(uuid)',
    'memory_worker_readiness()',
    'memory_user_health(uuid)'
  ] LOOP
    EXECUTE format(
      'ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,
      function_identity,
      schema_name
    );
  END LOOP;
END
$harden_memory_worker_health_functions$;

ALTER TABLE memory_worker_heartbeats OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_heartbeat(UUID, INTEGER, BOOLEAN)
  OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_retire(UUID) OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_worker_readiness() OWNER TO memory_runtime_owner;
ALTER FUNCTION memory_user_health(UUID) OWNER TO memory_runtime_owner;

REVOKE ALL ON memory_worker_heartbeats FROM PUBLIC, go_api_runtime, memory_worker_runtime;
REVOKE ALL ON FUNCTION memory_worker_heartbeat(UUID, INTEGER, BOOLEAN)
  FROM PUBLIC, go_api_runtime;
REVOKE ALL ON FUNCTION memory_worker_retire(UUID)
  FROM PUBLIC, go_api_runtime;
REVOKE ALL ON FUNCTION memory_user_health(UUID)
  FROM PUBLIC, memory_worker_runtime;

GRANT EXECUTE ON FUNCTION memory_worker_heartbeat(UUID, INTEGER, BOOLEAN),
  memory_worker_retire(UUID), memory_worker_readiness()
TO memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_user_health(UUID) TO go_api_runtime;
