-- Guarded rollback for G20.4 Broker effect authority.
DO $guard$
BEGIN
  IF EXISTS(SELECT 1 FROM agent_effect_intents)
     OR EXISTS(SELECT 1 FROM agent_effect_approvals)
     OR EXISTS(SELECT 1 FROM agent_effect_receipts)
     OR EXISTS(SELECT 1 FROM agent_effect_grant_revocations)
     OR EXISTS(SELECT 1 FROM agent_effect_cancellations)
     OR EXISTS(SELECT 1 FROM agent_secret_handles) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_EFFECT_DOWN_DATA_EXISTS';
  END IF;
END
$guard$;

ALTER TABLE agent_runner_requests DROP CONSTRAINT agent_runner_request_method_check;
ALTER TABLE agent_runner_requests ADD CONSTRAINT agent_runner_request_method_check
  CHECK(method IN ('launch','heartbeat','cancel'));

CREATE OR REPLACE FUNCTION agent_orchestrator_validate_runner_authority(
  p_method TEXT,p_user_id UUID,p_run_id TEXT,p_step_id TEXT,p_attempt_id TEXT,
  p_lease_generation BIGINT,p_lease_owner TEXT,p_lease_token_hash TEXT,
  p_snapshot_fingerprint TEXT
) RETURNS BIGINT
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_attempt agent_attempts%ROWTYPE;
  v_run agent_runs%ROWTYPE;
  v_epoch BIGINT;
  v_kill_mode TEXT;
BEGIN
  SELECT attempt.* INTO v_attempt FROM agent_attempts attempt
  WHERE attempt.id=p_attempt_id AND attempt.run_id=p_run_id
    AND attempt.user_id=p_user_id AND attempt.step_id=p_step_id FOR UPDATE;
  IF NOT FOUND OR v_attempt.generation<>p_lease_generation
     OR v_attempt.lease_owner<>p_lease_owner
     OR v_attempt.lease_token_hash<>p_lease_token_hash
     OR v_attempt.state NOT IN ('leased','starting','running','prepared','committing')
     OR v_attempt.lease_expires_at<=v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  SELECT run.* INTO v_run FROM agent_runs run
  WHERE run.id=p_run_id AND run.user_id=p_user_id FOR SHARE;
  IF NOT FOUND OR v_run.snapshot_fingerprint<>p_snapshot_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SNAPSHOT_MISMATCH';
  END IF;
  SELECT state.epoch INTO v_epoch FROM agent_kill_switch_state state WHERE singleton;
  v_kill_mode:=agent_orchestrator_active_kill_mode(v_run.scope_keys);
  IF p_method IN ('launch','heartbeat') AND v_kill_mode<>'' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  RETURN v_epoch;
END
$function$;

REVOKE ALL ON FUNCTION
  agent_effect_prepare(TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,TEXT,INTEGER,INTEGER,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ),
  agent_effect_decide_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT),
  agent_effect_cancel(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_claim_commit(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_complete_commit(TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_revoke_grant(TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_get_intent(UUID,TEXT),agent_effect_list_committing(INTEGER),
  agent_effect_expire(TIMESTAMPTZ,INTEGER)
  ,agent_secret_handle_create(TEXT,TEXT,TEXT,TEXT,TIMESTAMPTZ,JSONB)
  ,agent_secret_handle_consume(TEXT,TEXT,TIMESTAMPTZ)
  ,agent_secret_handle_revoke(TEXT,TIMESTAMPTZ)
  ,agent_secret_handle_revoke_intent(TEXT,TIMESTAMPTZ)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
  agent_effect_control;
REVOKE ALL ON FUNCTION agent_orchestrator_effect_fence(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
  agent_effect_control,agent_effect_owner;

DROP FUNCTION agent_effect_expire(TIMESTAMPTZ,INTEGER);
DROP FUNCTION agent_secret_handle_revoke_intent(TEXT,TIMESTAMPTZ);
DROP FUNCTION agent_secret_handle_revoke(TEXT,TIMESTAMPTZ);
DROP FUNCTION agent_secret_handle_consume(TEXT,TEXT,TIMESTAMPTZ);
DROP FUNCTION agent_secret_handle_create(TEXT,TEXT,TEXT,TEXT,TIMESTAMPTZ,JSONB);
DROP FUNCTION agent_effect_list_committing(INTEGER);
DROP FUNCTION agent_effect_get_intent(UUID,TEXT);
DROP FUNCTION agent_effect_complete_commit(TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_effect_revoke_grant(TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_effect_claim_commit(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_effect_cancel(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT);
DROP FUNCTION agent_effect_decide_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT);
DROP FUNCTION agent_effect_prepare(TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,TEXT,INTEGER,INTEGER,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ);
DROP FUNCTION agent_orchestrator_effect_fence(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT);
DROP TABLE agent_secret_handles;
DROP TABLE agent_effect_cancellations;
DROP TABLE agent_effect_receipts;
DROP TABLE agent_effect_grant_revocations;
DROP TABLE agent_effect_approvals;
DROP TABLE agent_effect_intents;
DROP FUNCTION agent_effect_immutable();

DO $schema_privileges$
BEGIN
  EXECUTE format('REVOKE USAGE ON SCHEMA %I FROM agent_effect_owner, agent_effect_control',current_schema());
END
$schema_privileges$;

DO $roles$
DECLARE role_name TEXT;
BEGIN
  FOREACH role_name IN ARRAY ARRAY['agent_effect_control','agent_effect_owner'] LOOP
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF EXISTS(
        SELECT 1 FROM pg_auth_members membership
        JOIN pg_roles parent_role ON parent_role.oid=membership.roleid
        JOIN pg_roles member_role ON member_role.oid=membership.member
        WHERE parent_role.rolname=role_name OR member_role.rolname=role_name) THEN
        RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_EFFECT_DOWN_ROLE_IN_USE';
      END IF;
      EXECUTE format('DROP ROLE %I',role_name);
    END IF;
  END LOOP;
END
$roles$;
