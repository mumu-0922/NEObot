-- G21.2 bounded Artifact publication authority. The canary Broker may
-- authorize and attach one exact object, but receives no direct product-table
-- DML. The host Runner and Sandbox receive no PostgreSQL or object credential.

DO $roles$
DECLARE
  can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create
  FROM pg_roles WHERE rolname=current_user;
  IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_artifact_control') THEN
    IF NOT can_create THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_ARTIFACT_REQUIRED_ROLE_MISSING';
    END IF;
    CREATE ROLE agent_artifact_control
      NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_artifact_control' AND
    (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_ARTIFACT_ROLE_MUST_BE_RESTRICTED';
  END IF;
  IF pg_has_role('agent_artifact_control','agent_product_owner','MEMBER')
     OR pg_has_role('agent_artifact_control','agent_effect_owner','MEMBER')
     OR pg_has_role('agent_artifact_control','agent_orchestrator_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_artifact_control','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_artifact_control','MEMBER')
     OR pg_has_role('agent_runner_control','agent_artifact_control','MEMBER')
     OR pg_has_role('agent_effect_control','agent_artifact_control','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_ARTIFACT_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

-- Lock and validate the exact committing effect without giving the product
-- owner UPDATE or DELETE privileges on the effect ledger.
CREATE FUNCTION agent_effect_artifact_fence(
  p_intent_id TEXT,p_user_id UUID,p_run_id TEXT,p_attempt_id TEXT,
  p_generation BIGINT,p_snapshot_fingerprint TEXT,p_grant_fingerprint TEXT,
  p_registry_fingerprint TEXT,p_resource TEXT
) RETURNS agent_effect_intents
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_intent agent_effect_intents%ROWTYPE;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents
  WHERE id=p_intent_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND OR v_intent.state<>'committing'
     OR v_intent.run_id<>p_run_id OR v_intent.attempt_id<>p_attempt_id
     OR v_intent.lease_generation<>p_generation
     OR v_intent.snapshot_fingerprint<>p_snapshot_fingerprint
     OR v_intent.grant_fingerprint<>p_grant_fingerprint
     OR v_intent.registry_fingerprint<>p_registry_fingerprint
     OR v_intent.tool_identity<>'artifact.publish'
     OR v_intent.capability<>'artifact.publish'
     OR v_intent.action<>'publish' OR v_intent.resource<>p_resource THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='ARTIFACT_DENIED';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_grant_revocations revocation
      WHERE revocation.grant_id=v_intent.grant_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='GRANT_DENIED';
  END IF;
  RETURN v_intent;
END
$function$;

-- Lock the durable Run/Step/Attempt and Kill-Switch epoch without advancing
-- the state machine. Attachment is valid only while Commit still owns the
-- current live generation.
CREATE FUNCTION agent_orchestrator_artifact_fence(
  p_user_id UUID,p_run_id TEXT,p_step_id TEXT,p_attempt_id TEXT,
  p_generation BIGINT,p_lease_owner TEXT,p_lease_token_hash TEXT,
  p_snapshot_fingerprint TEXT,p_kill_switch_epoch BIGINT,
  p_grant_fingerprint TEXT,p_registry_fingerprint TEXT,
  p_media_type TEXT,p_size_bytes BIGINT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_run agent_runs%ROWTYPE;
  v_step agent_steps%ROWTYPE;
  v_attempt agent_attempts%ROWTYPE;
  v_epoch BIGINT;
  v_snapshot JSONB;
  v_max_bytes_text TEXT;
  v_max_bytes BIGINT;
BEGIN
  SELECT * INTO v_run FROM agent_runs
  WHERE id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_step FROM agent_steps
  WHERE id=p_step_id AND run_id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_attempt FROM agent_attempts
  WHERE id=p_attempt_id AND run_id=p_run_id AND step_id=p_step_id
    AND user_id=p_user_id FOR UPDATE;
  SELECT epoch INTO v_epoch FROM agent_kill_switch_state WHERE singleton FOR SHARE;
  IF v_run.id IS NULL OR v_step.id IS NULL OR v_attempt.id IS NULL
     OR v_run.state<>'running' OR v_step.state<>'running'
     OR v_attempt.state<>'committing'
     OR v_run.snapshot_fingerprint<>p_snapshot_fingerprint
     OR v_attempt.generation<>p_generation OR v_step.current_generation<>p_generation
     OR v_attempt.lease_owner<>p_lease_owner
     OR v_attempt.lease_token_hash<>p_lease_token_hash
     OR v_attempt.lease_expires_at<=v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  IF v_epoch<>p_kill_switch_epoch
     OR agent_orchestrator_active_kill_mode(v_run.scope_keys) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  SELECT snapshot.canonical_snapshot INTO v_snapshot
  FROM agent_run_snapshots snapshot
  WHERE snapshot.id=v_run.snapshot_id AND snapshot.user_id=p_user_id
    AND snapshot.fingerprint=p_snapshot_fingerprint;
  v_max_bytes_text:=v_snapshot#>>'{artifactPolicy,maxBytes}';
  IF v_snapshot->>'schemaVersion'<>'neo.agent-broker-canary-snapshot/v1'
     OR v_snapshot->>'grantFingerprint'<>p_grant_fingerprint
     OR v_snapshot->>'registryFingerprint'<>p_registry_fingerprint
     OR v_max_bytes_text IS NULL OR v_max_bytes_text!~'^(0|[1-9][0-9]{0,7})$'
     OR jsonb_typeof(v_snapshot#>'{artifactPolicy,allowedMediaTypes}')<>'array' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='ARTIFACT_DENIED';
  END IF;
  v_max_bytes:=v_max_bytes_text::BIGINT;
  IF v_max_bytes>33554432 OR p_size_bytes>v_max_bytes
     OR NOT EXISTS(
       SELECT 1 FROM jsonb_array_elements_text(
         v_snapshot#>'{artifactPolicy,allowedMediaTypes}') media(value)
       WHERE media.value=p_media_type
     ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='ARTIFACT_DENIED';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_product_validate_artifact_authority(
  p_intent_id TEXT,p_user_id UUID,p_run_id TEXT,p_attempt_id TEXT,
  p_generation BIGINT,p_snapshot_fingerprint TEXT,p_grant_fingerprint TEXT,
  p_registry_fingerprint TEXT,p_name TEXT,p_media_type TEXT,p_size_bytes BIGINT,
  p_fingerprint TEXT,p_object_key TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_intent agent_effect_intents%ROWTYPE;
BEGIN
  IF p_name IS NULL OR octet_length(p_name) NOT BETWEEN 1 AND 128
     OR p_name<>btrim(p_name) OR p_name~E'[\\x00\\r\\n/\\\\]'
     OR p_media_type IS NULL
     OR p_media_type!~'^[a-z0-9][a-z0-9.+-]{0,63}/[a-z0-9][a-z0-9.+-]{0,63}$'
     OR p_size_bytes NOT BETWEEN 0 AND 33554432
     OR p_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_object_key<>'agent-artifacts/'||p_run_id||'/'||p_attempt_id||'/'||
       p_generation::text||'/'||substring(p_fingerprint FROM 8) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='ARTIFACT_DENIED';
  END IF;
  SELECT * INTO v_intent FROM agent_effect_artifact_fence(
    p_intent_id,p_user_id,p_run_id,p_attempt_id,p_generation,
    p_snapshot_fingerprint,p_grant_fingerprint,p_registry_fingerprint,p_name);
  PERFORM agent_orchestrator_artifact_fence(
    p_user_id,p_run_id,v_intent.step_id,p_attempt_id,p_generation,
    v_intent.lease_owner,v_intent.lease_token_hash,p_snapshot_fingerprint,
    v_intent.kill_switch_epoch,p_grant_fingerprint,p_registry_fingerprint,
    p_media_type,p_size_bytes);
  RETURN true;
END
$function$;

CREATE FUNCTION agent_artifact_authorize(
  p_intent_id TEXT,p_user_id UUID,p_run_id TEXT,p_attempt_id TEXT,
  p_generation BIGINT,p_snapshot_fingerprint TEXT,p_grant_fingerprint TEXT,
  p_registry_fingerprint TEXT,p_name TEXT,p_media_type TEXT,p_size_bytes BIGINT,
  p_fingerprint TEXT,p_object_key TEXT
) RETURNS BOOLEAN
LANGUAGE sql VOLATILE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT agent_product_validate_artifact_authority(
    p_intent_id,p_user_id,p_run_id,p_attempt_id,p_generation,
    p_snapshot_fingerprint,p_grant_fingerprint,p_registry_fingerprint,
    p_name,p_media_type,p_size_bytes,p_fingerprint,p_object_key)
$function$;

CREATE FUNCTION agent_artifact_attach(
  p_artifact_id TEXT,p_intent_id TEXT,p_user_id UUID,p_run_id TEXT,
  p_attempt_id TEXT,p_generation BIGINT,p_snapshot_fingerprint TEXT,
  p_grant_fingerprint TEXT,p_registry_fingerprint TEXT,p_name TEXT,
  p_media_type TEXT,p_size_bytes BIGINT,p_fingerprint TEXT,p_object_key TEXT
) RETURNS SETOF agent_artifacts
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_existing agent_artifacts%ROWTYPE;
BEGIN
  IF p_artifact_id!~'^artifact_[a-z0-9]{16,64}$' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='ARTIFACT_DENIED';
  END IF;
  PERFORM agent_product_validate_artifact_authority(
    p_intent_id,p_user_id,p_run_id,p_attempt_id,p_generation,
    p_snapshot_fingerprint,p_grant_fingerprint,p_registry_fingerprint,
    p_name,p_media_type,p_size_bytes,p_fingerprint,p_object_key);
  SELECT * INTO v_existing FROM agent_artifacts
  WHERE id=p_artifact_id OR
    (run_id=p_run_id AND attempt_id=p_attempt_id
      AND generation=p_generation AND name=p_name)
  FOR UPDATE;
  IF FOUND THEN
    IF v_existing.id<>p_artifact_id OR v_existing.user_id<>p_user_id
       OR v_existing.run_id<>p_run_id OR v_existing.attempt_id<>p_attempt_id
       OR v_existing.generation<>p_generation OR v_existing.name<>p_name
       OR v_existing.media_type<>p_media_type OR v_existing.size_bytes<>p_size_bytes
       OR v_existing.fingerprint<>p_fingerprint OR v_existing.object_key<>p_object_key THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN NEXT v_existing;RETURN;
  END IF;
  INSERT INTO agent_artifacts(
    id,run_id,user_id,attempt_id,generation,name,media_type,size_bytes,
    fingerprint,object_key
  ) VALUES(
    p_artifact_id,p_run_id,p_user_id,p_attempt_id,p_generation,p_name,
    p_media_type,p_size_bytes,p_fingerprint,p_object_key
  ) RETURNING * INTO v_existing;
  RETURN NEXT v_existing;
END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema();identity TEXT;
BEGIN
  FOREACH identity IN ARRAY ARRAY[
    'agent_effect_artifact_fence(text,uuid,text,text,bigint,text,text,text,text)',
    'agent_orchestrator_artifact_fence(uuid,text,text,text,bigint,text,text,text,bigint,text,text,text,bigint)',
    'agent_product_validate_artifact_authority(text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)',
    'agent_artifact_authorize(text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)',
    'agent_artifact_attach(text,text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,identity,schema_name);
  END LOOP;
END
$harden$;

ALTER FUNCTION agent_effect_artifact_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT)
  OWNER TO agent_effect_owner;
ALTER FUNCTION agent_orchestrator_artifact_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT)
  OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_product_validate_artifact_authority(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT)
  OWNER TO agent_product_owner;
ALTER FUNCTION agent_artifact_authorize(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT)
  OWNER TO agent_product_owner;
ALTER FUNCTION agent_artifact_attach(TEXT,TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT)
  OWNER TO agent_product_owner;

REVOKE ALL ON FUNCTION
  agent_effect_artifact_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT),
  agent_orchestrator_artifact_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT),
  agent_product_validate_artifact_authority(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT),
  agent_artifact_authorize(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT),
  agent_artifact_attach(TEXT,TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
    agent_effect_control,agent_artifact_control;
GRANT EXECUTE ON FUNCTION
  agent_effect_artifact_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT)
  TO agent_product_owner;
GRANT EXECUTE ON FUNCTION
  agent_orchestrator_artifact_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT)
  TO agent_product_owner;
GRANT EXECUTE ON FUNCTION
  agent_product_validate_artifact_authority(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT)
  TO agent_product_owner;
GRANT EXECUTE ON FUNCTION
  agent_artifact_authorize(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT),
  agent_artifact_attach(TEXT,TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT)
  TO agent_artifact_control;

REVOKE ALL ON agent_artifacts FROM agent_artifact_control;
DO $schema_acl$ BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_artifact_control',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_artifact_control',current_schema());
END
$schema_acl$;
