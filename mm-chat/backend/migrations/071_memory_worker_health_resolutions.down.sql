DO $memory_worker_health_resolution_rollback_guard$
BEGIN
  LOCK TABLE memory_job_health_resolutions IN ACCESS EXCLUSIVE MODE;
  IF EXISTS (SELECT 1 FROM memory_job_health_resolutions) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_JOB_HEALTH_RESOLUTION_ROLLBACK_REQUIRES_EMPTY';
  END IF;
END
$memory_worker_health_resolution_rollback_guard$;

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

DO $restore_memory_worker_health_search_path$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.memory_user_health(uuid) SET search_path TO %I, pg_catalog, pg_temp',
    schema_name,
    schema_name
  );
END
$restore_memory_worker_health_search_path$;

ALTER FUNCTION memory_user_health(UUID) OWNER TO memory_runtime_owner;
REVOKE ALL ON FUNCTION memory_user_health(UUID)
  FROM PUBLIC, memory_worker_runtime;
GRANT EXECUTE ON FUNCTION memory_user_health(UUID) TO go_api_runtime;

REVOKE ALL ON FUNCTION memory_acknowledge_job_health(UUID, UUID, TEXT, TEXT)
  FROM go_api_runtime;
DROP FUNCTION memory_acknowledge_job_health(UUID, UUID, TEXT, TEXT);
DROP TRIGGER memory_job_health_resolutions_append_only
  ON memory_job_health_resolutions;
DROP FUNCTION memory_job_health_resolution_append_only_guard();
DROP TABLE memory_job_health_resolutions;

ALTER TABLE memory_jobs
  DROP CONSTRAINT memory_jobs_job_id_user_unique;
