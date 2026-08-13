DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM agent_runs)
     OR EXISTS (SELECT 1 FROM agent_run_snapshots)
     OR EXISTS (SELECT 1 FROM agent_run_events)
     OR EXISTS (SELECT 1 FROM agent_kill_switches) THEN
    RAISE EXCEPTION USING ERRCODE='55000',
      MESSAGE='AGENT_ORCHESTRATOR_DOWN_DATA_EXISTS';
  END IF;
END
$guard$;

REVOKE ALL ON FUNCTION agent_orchestrator_enqueue_run(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]
),agent_orchestrator_transition(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
),agent_orchestrator_acquire_step(
  UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT[],INTEGER,TEXT,TEXT,TEXT
),agent_orchestrator_heartbeat_attempt(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,INTEGER,TEXT,TEXT,TEXT
),agent_orchestrator_observe_terminal_conflict(
  TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
),agent_orchestrator_recovery_runs(INTEGER),
  agent_orchestrator_resolve_kill_switch(UUID,TEXT)
  FROM agent_orchestrator_runtime;
REVOKE ALL ON agent_run_snapshots,agent_runs,agent_steps,agent_attempts,
  agent_run_events,agent_kill_switch_state,agent_kill_switches
  FROM agent_orchestrator_runtime;

DROP FUNCTION agent_orchestrator_prune_terminal_runs(TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_orchestrator_resolve_kill_switch(UUID,TEXT);
DROP FUNCTION agent_orchestrator_append_kill_switch(
  TEXT,TEXT,TEXT,TEXT,BOOLEAN,BIGINT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_orchestrator_rebuild_projection(UUID,TEXT);
DROP FUNCTION agent_orchestrator_recovery_runs(INTEGER);
DROP FUNCTION agent_orchestrator_observe_terminal_conflict(
  TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB);
DROP FUNCTION agent_orchestrator_heartbeat_attempt(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,INTEGER,TEXT,TEXT,TEXT);
DROP FUNCTION agent_orchestrator_acquire_step(
  UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT[],INTEGER,TEXT,TEXT,TEXT);
DROP FUNCTION agent_orchestrator_transition(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB);
DROP FUNCTION agent_orchestrator_transition_allowed(TEXT,TEXT,TEXT);
DROP FUNCTION agent_orchestrator_enqueue_run(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]);
DROP FUNCTION agent_orchestrator_active_kill_mode(TEXT[]);
DROP FUNCTION agent_orchestrator_append_event(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,
  TIMESTAMPTZ,JSONB,TIMESTAMPTZ);
DROP TRIGGER trg_agent_event_immutable ON agent_run_events;
DROP FUNCTION agent_orchestrator_event_immutable();
DROP TRIGGER trg_agent_snapshot_immutable ON agent_run_snapshots;
DROP FUNCTION agent_orchestrator_snapshot_immutable();

DROP TABLE agent_kill_switches;
DROP TABLE agent_kill_switch_state;
DROP TABLE agent_run_events;
DROP FUNCTION agent_orchestrator_detail_sanitized(JSONB);
DROP TABLE agent_attempts;
DROP TABLE agent_steps;
DROP TABLE agent_runs;
DROP TABLE agent_run_snapshots;
DROP FUNCTION agent_orchestrator_snapshot_sanitized(JSONB);

DO $schema_privileges$
BEGIN
  EXECUTE format(
    'REVOKE USAGE ON SCHEMA %I FROM agent_orchestrator_owner, agent_orchestrator_runtime',
    current_schema());
END
$schema_privileges$;
