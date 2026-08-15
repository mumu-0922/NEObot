-- G21.4 closes the durable Child-reap transport gap without granting a
-- controller direct Runner table access. The delegation function owner reads
-- the exact terminal Child Attempt and its optional Runner projection, waits
-- out signed launch authority, and atomically closes both projections.

GRANT SELECT ON agent_runner_requests,agent_runner_sandboxes TO agent_delegation_owner;
GRANT UPDATE ON agent_runner_sandboxes TO agent_delegation_owner;

CREATE FUNCTION agent_delegation_reap_inventory(p_limit INTEGER)
RETURNS TABLE(
  reap_id TEXT,parent_run_id TEXT,child_run_id TEXT,user_id UUID,step_id TEXT,
  attempt_id TEXT,generation BIGINT,lease_owner TEXT,mode TEXT,reap_state TEXT,
  retry_count INTEGER,attempt_state TEXT,authority_snapshot_fingerprint TEXT,
  sandbox_id TEXT,sandbox_lease_generation BIGINT,sandbox_runner_id TEXT,
  sandbox_snapshot_fingerprint TEXT,spec_fingerprint TEXT,
  probe_fingerprint TEXT,sandbox_state TEXT,
  launch_authority_expires_at TIMESTAMPTZ
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_REAP_INVENTORY_INVALID';
  END IF;
  RETURN QUERY
  SELECT reap.reap_id,reap.parent_run_id,reap.child_run_id,authority.user_id,
    attempt.step_id,reap.attempt_id,reap.generation,reap.lease_owner,reap.mode,
    reap.state,reap.retry_count,attempt.state,authority.snapshot_fingerprint,
    sandbox.sandbox_id,sandbox.lease_generation,sandbox.runner_id,
    sandbox.snapshot_fingerprint,sandbox.spec_fingerprint,
    sandbox.probe_fingerprint,sandbox.state,launch.expires_at
  FROM agent_delegation_reaps reap
  JOIN agent_delegation_authorities authority
    ON authority.run_id=reap.child_run_id AND authority.depth=1
  JOIN agent_attempts attempt
    ON attempt.run_id=reap.child_run_id AND attempt.user_id=authority.user_id
    AND attempt.id=reap.attempt_id
  LEFT JOIN agent_runner_sandboxes sandbox
    ON sandbox.run_id=reap.child_run_id AND sandbox.user_id=authority.user_id
    AND sandbox.attempt_id=reap.attempt_id
  LEFT JOIN LATERAL (
    SELECT max(request.expires_at) AS expires_at
    FROM agent_runner_requests request
    WHERE request.method='launch' AND request.run_id=reap.child_run_id
      AND request.user_id=authority.user_id AND request.step_id=attempt.step_id
      AND request.attempt_id=reap.attempt_id
      AND request.lease_generation=reap.generation
      AND request.lease_owner=reap.lease_owner
  ) launch ON true
  WHERE reap.state IN ('pending','failed')
  ORDER BY reap.created_at,reap.reap_id
  LIMIT p_limit;
END
$function$;

CREATE OR REPLACE FUNCTION agent_delegation_complete_reap(
  p_reap_id TEXT,p_succeeded BOOLEAN,p_error_code TEXT
)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_reap agent_delegation_reaps%ROWTYPE;
  v_user_id UUID;
  v_step_id TEXT;
  v_sandbox agent_runner_sandboxes%ROWTYPE;
  v_launch_expires_at TIMESTAMPTZ;
  v_terminal TEXT;
BEGIN
  IF p_succeeded IS NULL
     OR p_reap_id !~ '^reap_[a-z0-9]{16,64}$'
     OR (p_succeeded AND COALESCE(p_error_code,'')<>'')
     OR (NOT p_succeeded AND COALESCE(p_error_code,'') !~ '^[A-Z][A-Z0-9_]{0,63}$') THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_REAP_COMPLETION_INVALID';
  END IF;

  SELECT reap.* INTO v_reap FROM agent_delegation_reaps reap
  WHERE reap.reap_id=p_reap_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_REAP_NOT_FOUND';
  END IF;
  IF v_reap.state='reaped' THEN
    IF NOT p_succeeded THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN false;
  END IF;

  IF NOT p_succeeded THEN
    UPDATE agent_delegation_reaps SET state='failed',retry_count=retry_count+1,
      error_code=p_error_code,completed_at=NULL WHERE reap_id=p_reap_id;
    RETURN true;
  END IF;

  SELECT authority.user_id,attempt.step_id INTO v_user_id,v_step_id
  FROM agent_delegation_authorities authority
  JOIN agent_attempts attempt
    ON attempt.run_id=authority.run_id AND attempt.user_id=authority.user_id
  WHERE authority.run_id=v_reap.child_run_id AND authority.depth=1
    AND attempt.id=v_reap.attempt_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_REAP_TARGET_MISSING';
  END IF;

  SELECT max(request.expires_at) INTO v_launch_expires_at
  FROM agent_runner_requests request
  WHERE request.method='launch' AND request.run_id=v_reap.child_run_id
    AND request.user_id=v_user_id AND request.step_id=v_step_id
    AND request.attempt_id=v_reap.attempt_id
    AND request.lease_generation=v_reap.generation
    AND request.lease_owner=v_reap.lease_owner;
  IF v_launch_expires_at>clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_REAP_AUTHORITY_ACTIVE';
  END IF;

  SELECT sandbox.* INTO v_sandbox FROM agent_runner_sandboxes sandbox
  WHERE sandbox.attempt_id=v_reap.attempt_id FOR UPDATE;
  IF FOUND THEN
    IF v_sandbox.run_id<>v_reap.child_run_id OR v_sandbox.user_id<>v_user_id
       OR v_sandbox.step_id<>v_step_id
       OR v_sandbox.lease_generation<>v_reap.generation
       OR v_sandbox.runner_id<>v_reap.lease_owner THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_REAP_SANDBOX_MISMATCH';
    END IF;
    IF v_sandbox.state NOT IN ('terminal','orphaned') THEN
      v_terminal:=CASE v_reap.mode WHEN 'kill' THEN 'killed' ELSE 'canceled' END;
      UPDATE agent_runner_sandboxes SET state='terminal',
        observed_terminal=v_terminal,updated_at=clock_timestamp(),
        terminal_at=clock_timestamp()
      WHERE sandbox_id=v_sandbox.sandbox_id;
    END IF;
  END IF;

  UPDATE agent_delegation_reaps SET state='reaped',retry_count=retry_count+1,
    error_code=NULL,completed_at=clock_timestamp() WHERE reap_id=p_reap_id;
  RETURN true;
END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.agent_delegation_reap_inventory(integer) SET search_path TO %I, pg_catalog, pg_temp',
    schema_name,schema_name);
  EXECUTE format(
    'ALTER FUNCTION %I.agent_delegation_complete_reap(text,boolean,text) SET search_path TO %I, pg_catalog, pg_temp',
    schema_name,schema_name);
END
$harden$;

ALTER FUNCTION agent_delegation_reap_inventory(INTEGER) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT) OWNER TO agent_delegation_owner;

REVOKE ALL ON FUNCTION agent_delegation_reap_inventory(INTEGER),
  agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
    agent_effect_control,agent_delegation_control;
GRANT EXECUTE ON FUNCTION agent_delegation_reap_inventory(INTEGER),
  agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  TO agent_delegation_control;
