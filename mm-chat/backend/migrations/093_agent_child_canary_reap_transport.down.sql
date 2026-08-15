DO $guard$
BEGIN
  IF EXISTS(
    SELECT 1 FROM agent_delegation_reaps WHERE state IN ('pending','failed')
  ) OR EXISTS(
    SELECT 1 FROM agent_runner_sandboxes sandbox
    JOIN agent_delegation_authorities authority
      ON authority.run_id=sandbox.run_id AND authority.user_id=sandbox.user_id
      AND authority.depth=1
    WHERE sandbox.state NOT IN ('terminal','orphaned')
  ) THEN
    RAISE EXCEPTION USING ERRCODE='55000',
      MESSAGE='AGENT_CHILD_REAP_TRANSPORT_DOWN_REQUIRES_CLEAN';
  END IF;
END
$guard$;

REVOKE ALL ON FUNCTION agent_delegation_reap_inventory(INTEGER),
  agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
    agent_effect_control,agent_delegation_control;
DROP FUNCTION agent_delegation_reap_inventory(INTEGER);

CREATE OR REPLACE FUNCTION agent_delegation_complete_reap(
  p_reap_id TEXT,p_succeeded BOOLEAN,p_error_code TEXT
)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  UPDATE agent_delegation_reaps
  SET state=CASE WHEN p_succeeded THEN 'reaped' ELSE 'failed' END,
    retry_count=retry_count+1,
    error_code=CASE WHEN p_succeeded THEN NULL ELSE p_error_code END,
    completed_at=CASE WHEN p_succeeded THEN clock_timestamp() ELSE NULL END
  WHERE reap_id=p_reap_id AND state<>'reaped';
  RETURN FOUND;
END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema();
BEGIN
  EXECUTE format(
    'ALTER FUNCTION %I.agent_delegation_complete_reap(text,boolean,text) SET search_path TO %I, pg_catalog, pg_temp',
    schema_name,schema_name);
END
$harden$;
ALTER FUNCTION agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT) OWNER TO agent_delegation_owner;
REVOKE ALL ON FUNCTION agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
    agent_effect_control,agent_delegation_control;
GRANT EXECUTE ON FUNCTION agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  TO agent_delegation_control;

REVOKE SELECT,UPDATE ON agent_runner_sandboxes FROM agent_delegation_owner;
REVOKE SELECT ON agent_runner_requests FROM agent_delegation_owner;
