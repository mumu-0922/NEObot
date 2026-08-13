-- G20.3 Agent Runner control-plane authority. PostgreSQL persists exact
-- authority-ticket/replay claims and expected Sandbox lifecycle. The host-side
-- neo-runnerd process never receives a database credential.

DO $roles$
DECLARE
  role_name TEXT;
  can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create
  FROM pg_roles WHERE rolname = current_user;
  FOREACH role_name IN ARRAY ARRAY['agent_runner_owner','agent_runner_control'] LOOP
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF NOT can_create THEN
        RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='AGENT_RUNNER_REQUIRED_ROLE_MISSING';
      END IF;
      EXECUTE format(
        'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',
        role_name
      );
    END IF;
    IF EXISTS (
      SELECT 1 FROM pg_roles WHERE rolname=role_name
        AND (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole
             OR rolreplication OR rolbypassrls)
    ) THEN
      RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='AGENT_RUNNER_ROLE_MUST_BE_RESTRICTED';
    END IF;
  END LOOP;
  IF pg_has_role('agent_runner_control','agent_runner_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_runner_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_runner_control','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_runner_owner','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_runner_control','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501', MESSAGE='AGENT_RUNNER_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE FUNCTION agent_runner_response_sanitized(p_response JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE
  v_item JSONB;
  v_entry RECORD;
  v_key TEXT;
BEGIN
  IF jsonb_typeof(p_response)<>'object' OR octet_length(p_response::TEXT)>262144 THEN
    RETURN false;
  END IF;
  FOR v_item IN SELECT value FROM jsonb_path_query(p_response,'$.**') value LOOP
    IF jsonb_typeof(v_item)<>'object' THEN CONTINUE; END IF;
    FOR v_entry IN SELECT key FROM jsonb_each(v_item) LOOP
      v_key:=lower(regexp_replace(v_entry.key,'[_-]','','g'));
      IF v_key IN (
        'leasetoken','stdout','stderr','content','workspacecontent','skillbody',
        'toolarguments','toolresults','secret','secretvalue','authorization',
        'apikey','password','accesstoken','refreshtoken','credential','hostpath',
        'sourcepath','certificate','privatekey'
      ) OR v_key LIKE '%password%' OR v_key LIKE '%authorization%'
        OR v_key LIKE '%privatekey%' THEN
        RETURN false;
      END IF;
    END LOOP;
  END LOOP;
  RETURN true;
END
$function$;

CREATE TABLE agent_runner_requests (
  caller_identity TEXT NOT NULL,
  runner_id TEXT NOT NULL,
  request_id TEXT NOT NULL,
  nonce TEXT NOT NULL,
  method TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  user_id UUID NOT NULL,
  run_id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  attempt_id TEXT NOT NULL,
  lease_generation BIGINT NOT NULL,
  lease_owner TEXT NOT NULL,
  lease_token_hash TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  kill_switch_epoch BIGINT NOT NULL,
  issued_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'claimed',
  response_fingerprint TEXT,
  response_body JSONB,
  completed_at TIMESTAMPTZ,
  PRIMARY KEY(caller_identity,request_id,nonce),
  CONSTRAINT agent_runner_request_attempt_fk FOREIGN KEY(
    run_id,user_id,attempt_id
  ) REFERENCES agent_attempts(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_runner_request_step_fk FOREIGN KEY(
    run_id,user_id,step_id
  ) REFERENCES agent_steps(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_runner_request_caller_check CHECK(
    caller_identity ~ '^[A-Za-z][A-Za-z0-9_.:@/-]{0,127}$'
  ),
  CONSTRAINT agent_runner_request_runner_check CHECK(
    runner_id ~ '^[A-Za-z][A-Za-z0-9_.:@/-]{0,127}$' AND runner_id=lease_owner
  ),
  CONSTRAINT agent_runner_request_id_check CHECK(request_id ~ '^rpc_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_runner_request_nonce_check CHECK(nonce ~ '^[A-Za-z0-9_-]{32,128}$'),
  CONSTRAINT agent_runner_request_method_check CHECK(method IN ('launch','heartbeat','cancel')),
  CONSTRAINT agent_runner_request_fingerprint_check CHECK(
    request_fingerprint ~ '^sha256:[0-9a-f]{64}$'
  ),
  CONSTRAINT agent_runner_request_generation_check CHECK(lease_generation>=1),
  CONSTRAINT agent_runner_request_owner_check CHECK(
    octet_length(lease_owner) BETWEEN 1 AND 128 AND lease_owner=trim(lease_owner)
  ),
  CONSTRAINT agent_runner_request_token_hash_check CHECK(lease_token_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT agent_runner_request_snapshot_check CHECK(snapshot_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT agent_runner_request_epoch_check CHECK(kill_switch_epoch>=0),
  CONSTRAINT agent_runner_request_expiry_check CHECK(
    expires_at>issued_at AND expires_at<=issued_at+interval '15 seconds'
  ),
  CONSTRAINT agent_runner_request_status_check CHECK(status IN ('claimed','completed')),
  CONSTRAINT agent_runner_request_response_check CHECK(
    (status='claimed' AND response_fingerprint IS NULL AND response_body IS NULL AND completed_at IS NULL)
    OR (status='completed' AND response_fingerprint ~ '^sha256:[0-9a-f]{64}$'
      AND response_body IS NOT NULL AND completed_at IS NOT NULL
      AND agent_runner_response_sanitized(response_body))
  )
);

CREATE INDEX idx_agent_runner_requests_expiry
  ON agent_runner_requests(expires_at,request_id);
CREATE INDEX idx_agent_runner_requests_attempt
  ON agent_runner_requests(attempt_id,lease_generation,issued_at);

CREATE TABLE agent_runner_sandboxes (
  sandbox_id TEXT PRIMARY KEY,
  user_id UUID NOT NULL,
  run_id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  attempt_id TEXT NOT NULL,
  lease_generation BIGINT NOT NULL,
  runner_id TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  spec_fingerprint TEXT NOT NULL,
  probe_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'expected',
  observed_terminal TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  terminal_at TIMESTAMPTZ,
  CONSTRAINT agent_runner_sandbox_attempt_fk FOREIGN KEY(
    run_id,user_id,attempt_id
  ) REFERENCES agent_attempts(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_runner_sandbox_step_fk FOREIGN KEY(
    run_id,user_id,step_id
  ) REFERENCES agent_steps(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_runner_sandbox_attempt_unique UNIQUE(attempt_id),
  CONSTRAINT agent_runner_sandbox_identity_unique UNIQUE(
    sandbox_id,attempt_id,lease_generation,snapshot_fingerprint,spec_fingerprint
  ),
  CONSTRAINT agent_runner_sandbox_id_check CHECK(sandbox_id ~ '^sandbox_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_runner_sandbox_generation_check CHECK(lease_generation>=1),
  CONSTRAINT agent_runner_sandbox_runner_check CHECK(runner_id ~ '^[A-Za-z][A-Za-z0-9_.:@/-]{0,127}$'),
  CONSTRAINT agent_runner_sandbox_snapshot_check CHECK(snapshot_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT agent_runner_sandbox_spec_check CHECK(spec_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT agent_runner_sandbox_probe_check CHECK(probe_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT agent_runner_sandbox_state_check CHECK(
    state IN ('expected','starting','running','stopping','terminal','orphaned')
  ),
  CONSTRAINT agent_runner_sandbox_terminal_check CHECK(
    (state IN ('terminal','orphaned') AND terminal_at IS NOT NULL
      AND observed_terminal IN ('succeeded','failed','canceled','killed','outcome_unknown','orphaned'))
    OR (state NOT IN ('terminal','orphaned') AND terminal_at IS NULL AND observed_terminal IS NULL)
  ),
  CONSTRAINT agent_runner_sandbox_time_check CHECK(
    updated_at>=created_at AND (terminal_at IS NULL OR terminal_at>=created_at)
  )
);

CREATE INDEX idx_agent_runner_sandboxes_reconcile
  ON agent_runner_sandboxes(runner_id,state,updated_at,sandbox_id)
  WHERE state NOT IN ('terminal','orphaned');
CREATE INDEX idx_agent_runner_sandboxes_retention
  ON agent_runner_sandboxes(terminal_at,sandbox_id)
  WHERE terminal_at IS NOT NULL;

-- The Runner control owner may validate exact G20.2 authority only through
-- this lock-holding seam. It receives no SELECT/UPDATE privilege on
-- Orchestrator tables, and the host daemon receives no database role at all.
CREATE FUNCTION agent_orchestrator_validate_runner_authority(
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

CREATE FUNCTION agent_runner_issue_authority(
  p_caller_identity TEXT,p_request_id TEXT,p_nonce TEXT,p_method TEXT,
  p_request_fingerprint TEXT,p_runner_id TEXT,p_user_id UUID,p_run_id TEXT,p_step_id TEXT,
  p_attempt_id TEXT,p_lease_generation BIGINT,p_lease_owner TEXT,
  p_lease_token_hash TEXT,p_snapshot_fingerprint TEXT,p_ttl_seconds INTEGER
) RETURNS TABLE(
  runner_id TEXT,caller_identity TEXT,request_id TEXT,method TEXT,run_id TEXT,step_id TEXT,
  attempt_id TEXT,lease_generation BIGINT,lease_owner TEXT,lease_token_hash TEXT,
  snapshot_fingerprint TEXT,kill_switch_epoch BIGINT,issued_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,replayed BOOLEAN,response_body JSONB
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_request agent_runner_requests%ROWTYPE;
  v_epoch BIGINT;
BEGIN
  IF p_ttl_seconds NOT BETWEEN 1 AND 15 OR p_runner_id<>p_lease_owner THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_RUNNER_TTL_INVALID';
  END IF;
  SELECT request.* INTO v_request FROM agent_runner_requests request
  WHERE request.caller_identity=p_caller_identity
    AND request.request_id=p_request_id
    AND request.nonce=p_nonce
  FOR UPDATE;
  IF FOUND THEN
    IF v_request.method<>p_method OR v_request.request_fingerprint<>p_request_fingerprint
       OR v_request.runner_id<>p_runner_id
       OR v_request.user_id<>p_user_id OR v_request.run_id<>p_run_id
       OR v_request.step_id<>p_step_id OR v_request.attempt_id<>p_attempt_id
       OR v_request.lease_generation<>p_lease_generation
       OR v_request.lease_owner<>p_lease_owner
       OR v_request.lease_token_hash<>p_lease_token_hash
       OR v_request.snapshot_fingerprint<>p_snapshot_fingerprint THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    IF v_request.status='claimed' THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN QUERY SELECT v_request.runner_id,v_request.caller_identity,v_request.request_id,v_request.method,
      v_request.run_id,v_request.step_id,v_request.attempt_id,v_request.lease_generation,
      v_request.lease_owner,v_request.lease_token_hash,v_request.snapshot_fingerprint,
      v_request.kill_switch_epoch,v_request.issued_at,v_request.expires_at,true,
      v_request.response_body;
    RETURN;
  END IF;

  v_epoch:=agent_orchestrator_validate_runner_authority(
    p_method,p_user_id,p_run_id,p_step_id,p_attempt_id,p_lease_generation,
    p_lease_owner,p_lease_token_hash,p_snapshot_fingerprint
  );
  INSERT INTO agent_runner_requests(
    caller_identity,runner_id,request_id,nonce,method,request_fingerprint,user_id,run_id,
    step_id,attempt_id,lease_generation,lease_owner,lease_token_hash,
    snapshot_fingerprint,kill_switch_epoch,issued_at,expires_at
  ) VALUES (
    p_caller_identity,p_runner_id,p_request_id,p_nonce,p_method,p_request_fingerprint,p_user_id,
    p_run_id,p_step_id,p_attempt_id,p_lease_generation,p_lease_owner,
    p_lease_token_hash,p_snapshot_fingerprint,v_epoch,v_now,
    v_now+make_interval(secs=>p_ttl_seconds)
  ) RETURNING * INTO v_request;
  RETURN QUERY SELECT v_request.runner_id,v_request.caller_identity,v_request.request_id,v_request.method,
    v_request.run_id,v_request.step_id,v_request.attempt_id,v_request.lease_generation,
    v_request.lease_owner,v_request.lease_token_hash,v_request.snapshot_fingerprint,
    v_request.kill_switch_epoch,v_request.issued_at,v_request.expires_at,false,NULL::JSONB;
END
$function$;

CREATE FUNCTION agent_runner_complete_request(
  p_caller_identity TEXT,p_request_id TEXT,p_nonce TEXT,
  p_request_fingerprint TEXT,p_response_fingerprint TEXT,p_response_body JSONB
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_request agent_runner_requests%ROWTYPE;
BEGIN
  SELECT request.* INTO v_request FROM agent_runner_requests request
  WHERE request.caller_identity=p_caller_identity AND request.request_id=p_request_id
    AND request.nonce=p_nonce
  FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_RUNNER_REQUEST_NOT_FOUND'; END IF;
  IF v_request.request_fingerprint<>p_request_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
  END IF;
  IF v_request.status='completed' THEN
    IF v_request.response_fingerprint<>p_response_fingerprint OR v_request.response_body<>p_response_body THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN true;
  END IF;
  IF p_response_fingerprint !~ '^sha256:[0-9a-f]{64}$'
     OR NOT agent_runner_response_sanitized(p_response_body) THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_RUNNER_RESPONSE_INVALID';
  END IF;
  UPDATE agent_runner_requests SET status='completed',response_fingerprint=p_response_fingerprint,
    response_body=p_response_body,completed_at=clock_timestamp()
  WHERE caller_identity=p_caller_identity AND request_id=p_request_id AND nonce=p_nonce;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_runner_expect_sandbox(
  p_sandbox_id TEXT,p_caller_identity TEXT,p_request_id TEXT,p_nonce TEXT,
  p_user_id UUID,p_run_id TEXT,p_step_id TEXT,p_attempt_id TEXT,
  p_lease_generation BIGINT,p_runner_id TEXT,p_snapshot_fingerprint TEXT,
  p_spec_fingerprint TEXT,p_probe_fingerprint TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  PERFORM 1 FROM agent_runner_requests request
  WHERE request.caller_identity=p_caller_identity
    AND request.request_id=p_request_id AND request.nonce=p_nonce
    AND request.method='launch' AND request.user_id=p_user_id
    AND request.run_id=p_run_id AND request.step_id=p_step_id
    AND request.attempt_id=p_attempt_id
    AND request.lease_generation=p_lease_generation
    AND request.runner_id=p_runner_id
    AND request.snapshot_fingerprint=p_snapshot_fingerprint
    AND request.expires_at>clock_timestamp() FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE'; END IF;
  INSERT INTO agent_runner_sandboxes(
    sandbox_id,user_id,run_id,step_id,attempt_id,lease_generation,runner_id,
    snapshot_fingerprint,spec_fingerprint,probe_fingerprint
  ) VALUES (
    p_sandbox_id,p_user_id,p_run_id,p_step_id,p_attempt_id,p_lease_generation,
    p_runner_id,p_snapshot_fingerprint,p_spec_fingerprint,p_probe_fingerprint
  ) ON CONFLICT(attempt_id) DO NOTHING;
  IF NOT EXISTS(
    SELECT 1 FROM agent_runner_sandboxes WHERE sandbox_id=p_sandbox_id
      AND attempt_id=p_attempt_id AND lease_generation=p_lease_generation
      AND runner_id=p_runner_id AND snapshot_fingerprint=p_snapshot_fingerprint
      AND spec_fingerprint=p_spec_fingerprint AND probe_fingerprint=p_probe_fingerprint
  ) THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SNAPSHOT_MISMATCH'; END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_runner_update_sandbox(
  p_sandbox_id TEXT,p_attempt_id TEXT,p_lease_generation BIGINT,
  p_expected_state TEXT,p_to_state TEXT,p_observed_terminal TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_current TEXT;
BEGIN
  SELECT sandbox.state INTO v_current FROM agent_runner_sandboxes sandbox
  WHERE sandbox.sandbox_id=p_sandbox_id AND sandbox.attempt_id=p_attempt_id
    AND sandbox.lease_generation=p_lease_generation FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_RUNNER_SANDBOX_NOT_FOUND'; END IF;
  IF v_current<>p_expected_state OR NOT (
    (p_expected_state='expected' AND p_to_state='starting')
    OR (p_expected_state='starting' AND p_to_state IN ('running','terminal','orphaned'))
    OR (p_expected_state='running' AND p_to_state IN ('stopping','terminal','orphaned'))
    OR (p_expected_state='stopping' AND p_to_state IN ('terminal','orphaned'))
  ) THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION'; END IF;
  IF (p_to_state IN ('terminal','orphaned'))<>(p_observed_terminal IS NOT NULL) THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_RUNNER_TERMINAL_INVALID';
  END IF;
  UPDATE agent_runner_sandboxes SET state=p_to_state,
    observed_terminal=p_observed_terminal,updated_at=clock_timestamp(),
    terminal_at=CASE WHEN p_to_state IN ('terminal','orphaned') THEN clock_timestamp() ELSE NULL END
  WHERE sandbox_id=p_sandbox_id;
  RETURN true;
END
$function$;

-- Owner-only projection used by the credential-free Runner control recovery
-- function. It exposes bounded state/fence metadata, never lease credentials.
CREATE FUNCTION agent_orchestrator_runner_recovery_attempts(p_limit INTEGER)
RETURNS TABLE(
  run_id TEXT,user_id UUID,step_id TEXT,attempt_id TEXT,lease_generation BIGINT,
  attempt_state TEXT,lease_expires_at TIMESTAMPTZ,lease_expired BOOLEAN
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT attempt.run_id,attempt.user_id,attempt.step_id,attempt.id,
    attempt.generation,attempt.state,attempt.lease_expires_at,
    attempt.lease_expires_at<=clock_timestamp()
  FROM agent_attempts attempt
  WHERE attempt.state IN ('leased','starting','running','prepared','committing')
  ORDER BY attempt.updated_at,attempt.id LIMIT p_limit
$function$;

CREATE FUNCTION agent_runner_recovery_sandboxes(p_limit INTEGER)
RETURNS TABLE(
  sandbox_id TEXT,user_id UUID,run_id TEXT,step_id TEXT,attempt_id TEXT,
  lease_generation BIGINT,runner_id TEXT,snapshot_fingerprint TEXT,
  spec_fingerprint TEXT,probe_fingerprint TEXT,sandbox_state TEXT,
  attempt_state TEXT,lease_expires_at TIMESTAMPTZ,lease_expired BOOLEAN
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT sandbox.sandbox_id,sandbox.user_id,sandbox.run_id,sandbox.step_id,
    sandbox.attempt_id,sandbox.lease_generation,sandbox.runner_id,
    sandbox.snapshot_fingerprint,sandbox.spec_fingerprint,sandbox.probe_fingerprint,
    sandbox.state,attempt.attempt_state,attempt.lease_expires_at,
    attempt.lease_expired
  FROM agent_runner_sandboxes sandbox
  JOIN agent_orchestrator_runner_recovery_attempts(p_limit) attempt
    ON attempt.attempt_id=sandbox.attempt_id
    AND attempt.run_id=sandbox.run_id AND attempt.user_id=sandbox.user_id
  WHERE sandbox.state NOT IN ('terminal','orphaned')
  ORDER BY sandbox.updated_at,sandbox.sandbox_id LIMIT p_limit
$function$;

CREATE FUNCTION agent_runner_prune(p_cutoff TIMESTAMPTZ,p_limit INTEGER)
RETURNS TABLE(requests_deleted INTEGER,sandboxes_deleted INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_requests INTEGER;
  v_sandboxes INTEGER;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 OR p_cutoff>clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_RUNNER_RETENTION_INVALID';
  END IF;
  WITH targets AS (
    SELECT caller_identity,request_id,nonce FROM agent_runner_requests
    WHERE status='completed' AND expires_at<p_cutoff
    ORDER BY expires_at,request_id LIMIT p_limit FOR UPDATE SKIP LOCKED
  ), deleted AS (
    DELETE FROM agent_runner_requests request USING targets
    WHERE request.caller_identity=targets.caller_identity
      AND request.request_id=targets.request_id AND request.nonce=targets.nonce
    RETURNING 1
  ) SELECT count(*) INTO v_requests FROM deleted;
  WITH targets AS (
    SELECT sandbox_id FROM agent_runner_sandboxes
    WHERE terminal_at IS NOT NULL AND terminal_at<p_cutoff
    ORDER BY terminal_at,sandbox_id LIMIT p_limit FOR UPDATE SKIP LOCKED
  ), deleted AS (
    DELETE FROM agent_runner_sandboxes sandbox USING targets
    WHERE sandbox.sandbox_id=targets.sandbox_id RETURNING 1
  ) SELECT count(*) INTO v_sandboxes FROM deleted;
  RETURN QUERY SELECT v_requests,v_sandboxes;
END
$function$;

DO $harden_functions$
DECLARE
  schema_name TEXT:=current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'agent_orchestrator_validate_runner_authority(text,uuid,text,text,text,bigint,text,text,text)',
    'agent_orchestrator_runner_recovery_attempts(integer)',
    'agent_runner_issue_authority(text,text,text,text,text,text,uuid,text,text,text,bigint,text,text,text,integer)',
    'agent_runner_complete_request(text,text,text,text,text,jsonb)',
    'agent_runner_expect_sandbox(text,text,text,text,uuid,text,text,text,bigint,text,text,text,text)',
    'agent_runner_update_sandbox(text,text,bigint,text,text,text)',
    'agent_runner_recovery_sandboxes(integer)',
    'agent_runner_prune(timestamp with time zone,integer)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,function_identity,schema_name);
  END LOOP;
END
$harden_functions$;

ALTER TABLE agent_runner_requests OWNER TO agent_runner_owner;
ALTER TABLE agent_runner_sandboxes OWNER TO agent_runner_owner;
ALTER FUNCTION agent_runner_response_sanitized(JSONB) OWNER TO agent_runner_owner;
ALTER FUNCTION agent_orchestrator_validate_runner_authority(TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT)
  OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_runner_recovery_attempts(INTEGER)
  OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_runner_issue_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,INTEGER) OWNER TO agent_runner_owner;
ALTER FUNCTION agent_runner_complete_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB) OWNER TO agent_runner_owner;
ALTER FUNCTION agent_runner_expect_sandbox(TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_runner_owner;
ALTER FUNCTION agent_runner_update_sandbox(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT) OWNER TO agent_runner_owner;
ALTER FUNCTION agent_runner_recovery_sandboxes(INTEGER) OWNER TO agent_runner_owner;
ALTER FUNCTION agent_runner_prune(TIMESTAMPTZ,INTEGER) OWNER TO agent_runner_owner;

REVOKE ALL ON agent_runner_requests,agent_runner_sandboxes
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control;
REVOKE ALL ON FUNCTION agent_runner_response_sanitized(JSONB)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control;
REVOKE ALL ON FUNCTION agent_orchestrator_validate_runner_authority(TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control;
REVOKE ALL ON FUNCTION agent_orchestrator_runner_recovery_attempts(INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control;
REVOKE ALL ON FUNCTION agent_runner_issue_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,INTEGER),
  agent_runner_complete_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),
  agent_runner_expect_sandbox(TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT),
  agent_runner_update_sandbox(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT),
  agent_runner_recovery_sandboxes(INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime;
REVOKE ALL ON FUNCTION agent_runner_prune(TIMESTAMPTZ,INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control;

GRANT EXECUTE ON FUNCTION agent_runner_issue_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,INTEGER),
  agent_runner_complete_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),
  agent_runner_expect_sandbox(TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT),
  agent_runner_update_sandbox(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT),
  agent_runner_recovery_sandboxes(INTEGER)
  TO agent_runner_control;
GRANT EXECUTE ON FUNCTION agent_orchestrator_validate_runner_authority(TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT)
  TO agent_runner_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_runner_recovery_attempts(INTEGER)
  TO agent_runner_owner;

DO $schema_privileges$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM agent_runner_owner, agent_runner_control, go_api_runtime, agent_orchestrator_runtime',
    current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_runner_owner, agent_runner_control',current_schema());
END
$schema_privileges$;
