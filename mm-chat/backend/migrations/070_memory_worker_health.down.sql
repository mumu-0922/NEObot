DO $memory_worker_health_rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM memory_worker_heartbeats heartbeat
    WHERE heartbeat.expires_at > clock_timestamp()
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_HEALTH_ROLLBACK_REQUIRES_STOPPED_WORKERS';
  END IF;
END
$memory_worker_health_rollback_guard$;

REVOKE ALL ON FUNCTION memory_user_health(UUID) FROM go_api_runtime;
REVOKE ALL ON FUNCTION memory_worker_heartbeat(UUID, INTEGER, BOOLEAN),
  memory_worker_retire(UUID)
FROM memory_worker_runtime;

DROP FUNCTION memory_user_health(UUID);
DROP FUNCTION memory_worker_retire(UUID);
DROP FUNCTION memory_worker_heartbeat(UUID, INTEGER, BOOLEAN);

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
    true,
    count(*) FILTER (WHERE status = 'pending'),
    count(*) FILTER (WHERE status = 'processing'),
    count(*) FILTER (WHERE status = 'dead_letter'),
    min(created_at) FILTER (WHERE status = 'pending')
  FROM memory_jobs
$function$;

DO $restore_memory_worker_readiness_search_path$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.memory_worker_readiness() SET search_path TO %I, pg_catalog, pg_temp',
    schema_name,
    schema_name
  );
END
$restore_memory_worker_readiness_search_path$;

ALTER FUNCTION memory_worker_readiness() OWNER TO memory_runtime_owner;
REVOKE ALL ON FUNCTION memory_worker_readiness() FROM PUBLIC, go_api_runtime;
GRANT EXECUTE ON FUNCTION memory_worker_readiness() TO memory_worker_runtime;

DROP TABLE memory_worker_heartbeats;
