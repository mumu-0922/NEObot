DO $rollback_guard$
BEGIN
  IF EXISTS (SELECT 1 FROM memory_outbox)
    OR EXISTS (SELECT 1 FROM memory_jobs)
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'MEMORY_WORKER_ROLLBACK_REQUIRES_EMPTY_QUEUE';
  END IF;
END
$rollback_guard$;

REVOKE EXECUTE ON FUNCTION memory_append_turn_completed_event(
  UUID, UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT
) FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION memory_worker_claim_job(UUID, UUID, INTEGER),
  memory_worker_hydrate_capture(UUID, UUID, UUID),
  memory_worker_apply_capture_candidate(
    UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[]
  ),
  memory_worker_complete_job(UUID, UUID, UUID),
  memory_worker_retry_job(UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN),
  memory_worker_readiness()
FROM memory_worker_runtime;

DROP FUNCTION memory_worker_readiness();
DROP FUNCTION memory_worker_retry_job(
  UUID, UUID, UUID, TEXT, TIMESTAMPTZ, BOOLEAN
);
DROP FUNCTION memory_worker_complete_job(UUID, UUID, UUID);
DROP FUNCTION memory_worker_apply_capture_candidate(
  UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT, TEXT[]
);
DROP FUNCTION memory_worker_hydrate_capture(UUID, UUID, UUID);
DROP FUNCTION memory_worker_claim_job(UUID, UUID, INTEGER);
DROP FUNCTION memory_append_turn_completed_event(
  UUID, UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, TEXT, SMALLINT
);

REVOKE SELECT, INSERT, UPDATE ON memory_outbox, memory_jobs, user_memories
  FROM memory_runtime_owner;
REVOKE SELECT, UPDATE ON conversations FROM memory_runtime_owner;
REVOKE SELECT ON
  users,
  messages,
  projects,
  user_memory_settings,
  task_model_settings,
  provider_configs
  FROM memory_runtime_owner;

DROP TABLE memory_jobs;
DROP TABLE memory_outbox;

DO $schema_privileges$
BEGIN
  EXECUTE format(
    'REVOKE USAGE ON SCHEMA %I FROM memory_runtime_owner, memory_worker_runtime',
    current_schema()
  );
END
$schema_privileges$;
