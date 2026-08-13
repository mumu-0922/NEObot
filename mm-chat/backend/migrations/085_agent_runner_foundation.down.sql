-- Guarded rollback for G20.3 Runner control authority.

DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_runner_requests)
     OR EXISTS(SELECT 1 FROM agent_runner_sandboxes) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_RUNNER_DOWN_DATA_EXISTS';
  END IF;
END
$guard$;

REVOKE ALL ON FUNCTION agent_runner_issue_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,INTEGER),
  agent_runner_complete_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),
  agent_runner_expect_sandbox(TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT),
  agent_runner_update_sandbox(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT),
  agent_runner_recovery_sandboxes(INTEGER),agent_runner_prune(TIMESTAMPTZ,INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control;
REVOKE EXECUTE ON FUNCTION agent_orchestrator_validate_runner_authority(TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_runner_owner;
REVOKE EXECUTE ON FUNCTION agent_orchestrator_runner_recovery_attempts(INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_runner_owner;

DROP FUNCTION agent_runner_prune(TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_runner_recovery_sandboxes(INTEGER);
DROP FUNCTION agent_runner_update_sandbox(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_runner_expect_sandbox(TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_runner_complete_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB);
DROP FUNCTION agent_runner_issue_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,INTEGER);
DROP FUNCTION agent_orchestrator_validate_runner_authority(TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_orchestrator_runner_recovery_attempts(INTEGER);
DROP TABLE agent_runner_sandboxes;
DROP TABLE agent_runner_requests;
DROP FUNCTION agent_runner_response_sanitized(JSONB);

DO $schema_privileges$
BEGIN
  EXECUTE format(
    'REVOKE USAGE ON SCHEMA %I FROM agent_runner_owner, agent_runner_control',
    current_schema());
END
$schema_privileges$;

DO $roles$
DECLARE role_name TEXT;
BEGIN
  FOREACH role_name IN ARRAY ARRAY['agent_runner_control','agent_runner_owner'] LOOP
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF EXISTS(
        SELECT 1 FROM pg_auth_members membership
        JOIN pg_roles parent_role ON parent_role.oid=membership.roleid
        JOIN pg_roles member_role ON member_role.oid=membership.member
        WHERE parent_role.rolname=role_name OR member_role.rolname=role_name
      ) THEN
        RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_RUNNER_DOWN_ROLE_IN_USE';
      END IF;
      EXECUTE format('DROP ROLE %I',role_name);
    END IF;
  END LOOP;
END
$roles$;
