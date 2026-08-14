DO $guard$ BEGIN
  IF EXISTS(SELECT 1 FROM agent_delegation_authorities) OR EXISTS(SELECT 1 FROM agent_delegation_lineage)
     OR EXISTS(SELECT 1 FROM agent_delegation_settlements) OR EXISTS(SELECT 1 FROM agent_delegation_reaps) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_DELEGATION_DOWN_DATA_EXISTS';END IF;
END $guard$;

REVOKE SELECT,UPDATE ON agent_runs,agent_steps,agent_attempts FROM agent_delegation_owner;
REVOKE SELECT ON agent_run_snapshots,agent_kill_switch_state,agent_kill_switches FROM agent_delegation_owner;
REVOKE EXECUTE ON FUNCTION agent_orchestrator_enqueue_run(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]),
  agent_orchestrator_active_kill_mode(TEXT[]),
  agent_orchestrator_append_event(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,JSONB,TIMESTAMPTZ)
  FROM agent_delegation_owner;

REVOKE ALL ON FUNCTION agent_delegation_register_root(TEXT,UUID,JSONB,JSONB,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB,TIMESTAMPTZ),
  agent_delegation_enqueue_child(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[],TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB),
  agent_delegation_admit_launch(UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),agent_delegation_settle(TEXT,UUID,TEXT,JSONB,TEXT),
  agent_delegation_cascade(UUID,TEXT,TEXT,TEXT,TEXT,TEXT),agent_delegation_reconcile(INTEGER),agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control;
DROP FUNCTION agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT);
DROP FUNCTION agent_delegation_reconcile(INTEGER);
DROP FUNCTION agent_delegation_cascade(UUID,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_delegation_settle(TEXT,UUID,TEXT,JSONB,TEXT);
DROP FUNCTION agent_delegation_admit_launch(UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB);
DROP FUNCTION agent_delegation_enqueue_child(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[],TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB);
DROP FUNCTION agent_delegation_register_root(TEXT,UUID,JSONB,JSONB,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB,TIMESTAMPTZ);
DROP TRIGGER trg_agent_delegation_settlement_immutable ON agent_delegation_settlements;
DROP TABLE agent_delegation_reaps;
DROP TABLE agent_delegation_settlements;
DROP TABLE agent_delegation_lineage;
DROP TABLE agent_delegation_authorities;
DROP FUNCTION agent_delegation_immutable();
DROP FUNCTION agent_delegation_json_subset(JSONB,JSONB);
DROP FUNCTION agent_delegation_registry_subset(JSONB,JSONB);
DROP FUNCTION agent_delegation_registry_valid(JSONB,INTEGER);
DROP FUNCTION agent_delegation_budget_valid(JSONB);
DO $schema_privileges$ BEGIN EXECUTE format('REVOKE USAGE ON SCHEMA %I FROM agent_delegation_owner,agent_delegation_control',current_schema());END $schema_privileges$;
DO $roles$ DECLARE role_name TEXT;BEGIN
  FOREACH role_name IN ARRAY ARRAY['agent_delegation_control','agent_delegation_owner'] LOOP
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF EXISTS(SELECT 1 FROM pg_auth_members membership JOIN pg_roles parent_role ON parent_role.oid=membership.roleid
        JOIN pg_roles member_role ON member_role.oid=membership.member WHERE parent_role.rolname=role_name OR member_role.rolname=role_name) THEN
        RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_DELEGATION_DOWN_ROLE_IN_USE';END IF;
      EXECUTE format('DROP ROLE %I',role_name);END IF;
  END LOOP;
END $roles$;
