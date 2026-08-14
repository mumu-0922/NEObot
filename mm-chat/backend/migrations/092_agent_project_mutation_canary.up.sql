-- G21.3 one-action synthetic Project mutation authority. This migration does
-- not create a user Project store: only an operator-provisioned canary resource
-- can be changed through exact function-only authority.

DO $roles$
DECLARE can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create FROM pg_roles WHERE rolname=current_user;
  IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_project_mutation_owner') THEN
    IF NOT can_create THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PROJECT_MUTATION_REQUIRED_OWNER_MISSING';
    END IF;
    CREATE ROLE agent_project_mutation_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_project_mutation_control') THEN
    IF NOT can_create THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PROJECT_MUTATION_REQUIRED_ROLE_MISSING';
    END IF;
    CREATE ROLE agent_project_mutation_control NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname IN ('agent_project_mutation_owner','agent_project_mutation_control')
    AND (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PROJECT_MUTATION_ROLE_MUST_BE_RESTRICTED';
  END IF;
  IF pg_has_role('agent_project_mutation_control','agent_project_mutation_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_project_mutation_control','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_project_mutation_control','MEMBER')
     OR pg_has_role('agent_runner_control','agent_project_mutation_control','MEMBER')
     OR pg_has_role('agent_effect_control','agent_project_mutation_control','MEMBER')
     OR pg_has_role('agent_artifact_control','agent_project_mutation_control','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PROJECT_MUTATION_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE TABLE agent_project_canary_resources (
  resource_id TEXT PRIMARY KEY CHECK(resource_id ~ '^project-canary/[a-z0-9][a-z0-9._-]{0,63}$'),
  user_id UUID NOT NULL,
  project_id TEXT NOT NULL CHECK(project_id ~ '^project_[A-Za-z0-9_-]{8,128}$'),
  relative_path TEXT NOT NULL CHECK(relative_path ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'),
  baseline_content BYTEA NOT NULL,
  baseline_content_fingerprint TEXT NOT NULL CHECK(baseline_content_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  baseline_revision TEXT NOT NULL CHECK(baseline_revision ~ '^sha256:[a-f0-9]{64}$'),
  current_content BYTEA NOT NULL,
  current_content_fingerprint TEXT NOT NULL CHECK(current_content_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  current_revision TEXT NOT NULL CHECK(current_revision ~ '^sha256:[a-f0-9]{64}$'),
  max_bytes BIGINT NOT NULL CHECK(max_bytes BETWEEN 1 AND 4096),
  state TEXT NOT NULL DEFAULT 'clean' CHECK(state IN ('clean','mutated')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  UNIQUE(user_id,project_id,relative_path)
);

CREATE TABLE agent_project_mutation_receipts (
  idempotency_key TEXT PRIMARY KEY CHECK(idempotency_key ~ '^commit_[A-Za-z0-9_-]{24,153}$'),
  intent_id TEXT NOT NULL UNIQUE REFERENCES agent_effect_intents(id) ON DELETE RESTRICT,
  intent_fingerprint TEXT NOT NULL CHECK(intent_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  resource_id TEXT NOT NULL REFERENCES agent_project_canary_resources(resource_id) ON DELETE RESTRICT,
  user_id UUID NOT NULL,
  run_id TEXT NOT NULL,
  attempt_id TEXT NOT NULL,
  generation BIGINT NOT NULL CHECK(generation>=1),
  base_revision TEXT NOT NULL CHECK(base_revision ~ '^sha256:[a-f0-9]{64}$'),
  mutation_fingerprint TEXT NOT NULL CHECK(mutation_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  content_fingerprint TEXT NOT NULL CHECK(content_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  committed_revision TEXT NOT NULL CHECK(committed_revision ~ '^sha256:[a-f0-9]{64}$'),
  receipt_fingerprint TEXT NOT NULL UNIQUE CHECK(receipt_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  committed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  cleaned_at TIMESTAMPTZ
);

CREATE TABLE agent_project_mutation_cleanups (
  receipt_fingerprint TEXT PRIMARY KEY REFERENCES agent_project_mutation_receipts(receipt_fingerprint) ON DELETE RESTRICT,
  resource_id TEXT NOT NULL REFERENCES agent_project_canary_resources(resource_id) ON DELETE RESTRICT,
  restored_revision TEXT NOT NULL CHECK(restored_revision ~ '^sha256:[a-f0-9]{64}$'),
  cleaned_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE FUNCTION agent_project_mutation_fact_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PROJECT_MUTATION_FACT_IMMUTABLE';
END
$function$;
CREATE TRIGGER agent_project_mutation_receipts_immutable
BEFORE UPDATE OR DELETE ON agent_project_mutation_receipts
FOR EACH ROW EXECUTE FUNCTION agent_project_mutation_fact_immutable();
CREATE TRIGGER agent_project_mutation_cleanups_immutable
BEFORE UPDATE OR DELETE ON agent_project_mutation_cleanups
FOR EACH ROW EXECUTE FUNCTION agent_project_mutation_fact_immutable();

CREATE FUNCTION agent_effect_project_mutation_fence(
  p_intent_id TEXT,p_user_id UUID,p_run_id TEXT,p_attempt_id TEXT,
  p_generation BIGINT,p_snapshot_fingerprint TEXT,p_grant_fingerprint TEXT,
  p_registry_fingerprint TEXT,p_idempotency_key TEXT,p_resource TEXT,
  p_base_revision TEXT
) RETURNS agent_effect_intents
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_intent agent_effect_intents%ROWTYPE;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents
  WHERE id=p_intent_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND OR v_intent.state<>'committing'
     OR v_intent.run_id<>p_run_id OR v_intent.attempt_id<>p_attempt_id
     OR v_intent.lease_generation<>p_generation
     OR v_intent.snapshot_fingerprint<>p_snapshot_fingerprint
     OR v_intent.grant_fingerprint<>p_grant_fingerprint
     OR v_intent.registry_fingerprint<>p_registry_fingerprint
     OR v_intent.idempotency_key<>p_idempotency_key
     OR v_intent.base_revision<>p_base_revision
     OR v_intent.tool_identity<>'project.patch'
     OR v_intent.capability<>'project.write'
     OR v_intent.action<>'apply_patch' OR v_intent.resource<>p_resource
     OR v_intent.approval_class<>'per_commit' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM agent_effect_approvals approval
      WHERE approval.intent_id=v_intent.id AND approval.intent_fingerprint=v_intent.intent_fingerprint
        AND approval.decision='approved') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='APPROVAL_REQUIRED';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_grant_revocations revocation
      WHERE revocation.grant_id=v_intent.grant_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='GRANT_DENIED';
  END IF;
  RETURN v_intent;
END
$function$;

CREATE FUNCTION agent_orchestrator_project_mutation_fence(
  p_user_id UUID,p_run_id TEXT,p_step_id TEXT,p_attempt_id TEXT,
  p_generation BIGINT,p_lease_owner TEXT,p_lease_token_hash TEXT,
  p_snapshot_fingerprint TEXT,p_kill_switch_epoch BIGINT,
  p_grant_fingerprint TEXT,p_registry_fingerprint TEXT,p_resource TEXT,
  p_path TEXT,p_size_bytes BIGINT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();v_run agent_runs%ROWTYPE;
  v_step agent_steps%ROWTYPE;v_attempt agent_attempts%ROWTYPE;v_epoch BIGINT;
  v_snapshot JSONB;v_max_bytes_text TEXT;v_max_bytes BIGINT;
BEGIN
  SELECT * INTO v_run FROM agent_runs WHERE id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_step FROM agent_steps WHERE id=p_step_id AND run_id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_attempt FROM agent_attempts WHERE id=p_attempt_id AND run_id=p_run_id
    AND step_id=p_step_id AND user_id=p_user_id FOR UPDATE;
  SELECT epoch INTO v_epoch FROM agent_kill_switch_state WHERE singleton FOR SHARE;
  IF v_run.id IS NULL OR v_step.id IS NULL OR v_attempt.id IS NULL
     OR v_run.state<>'running' OR v_step.state<>'running' OR v_attempt.state<>'committing'
     OR v_run.snapshot_fingerprint<>p_snapshot_fingerprint
     OR v_attempt.generation<>p_generation OR v_step.current_generation<>p_generation
     OR v_attempt.lease_owner<>p_lease_owner OR v_attempt.lease_token_hash<>p_lease_token_hash
     OR v_attempt.lease_expires_at<=v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  IF v_epoch<>p_kill_switch_epoch OR agent_orchestrator_active_kill_mode(v_run.scope_keys) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  SELECT snapshot.canonical_snapshot INTO v_snapshot FROM agent_run_snapshots snapshot
  WHERE snapshot.id=v_run.snapshot_id AND snapshot.user_id=p_user_id
    AND snapshot.fingerprint=p_snapshot_fingerprint;
  v_max_bytes_text:=v_snapshot#>>'{projectPolicy,maxBytes}';
  IF v_snapshot->>'schemaVersion'<>'neo.agent-project-canary-snapshot/v1'
     OR v_snapshot->>'grantFingerprint'<>p_grant_fingerprint
     OR v_snapshot->>'registryFingerprint'<>p_registry_fingerprint
     OR v_snapshot#>>'{projectPolicy,resource}'<>p_resource
     OR v_snapshot#>>'{projectPolicy,path}'<>p_path
     OR v_max_bytes_text IS NULL OR v_max_bytes_text!~'^[1-9][0-9]{0,3}$' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  v_max_bytes:=v_max_bytes_text::BIGINT;
  IF v_max_bytes>4096 OR p_size_bytes<1 OR p_size_bytes>v_max_bytes THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_effect_project_cleanup_fence(
  p_intent_id TEXT,p_user_id UUID,p_receipt_fingerprint TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_intent agent_effect_intents%ROWTYPE;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents
  WHERE id=p_intent_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND OR v_intent.state<>'committed'
     OR v_intent.receipt_fingerprint<>p_receipt_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_project_canary_provision(
  p_resource TEXT,p_user_id UUID,p_project_id TEXT,p_path TEXT,
  p_baseline_content BYTEA,p_max_bytes BIGINT
) RETURNS SETOF agent_project_canary_resources
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_content_fingerprint TEXT;v_revision TEXT;v_existing agent_project_canary_resources%ROWTYPE;
BEGIN
  IF p_resource!~'^project-canary/[a-z0-9][a-z0-9._-]{0,63}$'
     OR p_project_id!~'^project_[A-Za-z0-9_-]{8,128}$'
     OR p_path!~'^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'
     OR p_max_bytes NOT BETWEEN 1 AND 4096
     OR octet_length(p_baseline_content)<1 OR octet_length(p_baseline_content)>p_max_bytes THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  PERFORM convert_from(p_baseline_content,'UTF8');
  v_content_fingerprint:='sha256:'||encode(sha256(convert_to('neo-project-file-v1','UTF8')||decode('00','hex')||p_baseline_content),'hex');
  v_revision:='sha256:'||encode(sha256(convert_to('neo-project-canary-revision-v1','UTF8')||decode('00','hex')||convert_to(v_content_fingerprint,'UTF8')),'hex');
  SELECT * INTO v_existing FROM agent_project_canary_resources WHERE resource_id=p_resource FOR UPDATE;
  IF FOUND THEN
    IF v_existing.user_id<>p_user_id OR v_existing.project_id<>p_project_id
       OR v_existing.relative_path<>p_path OR v_existing.baseline_content<>p_baseline_content
       OR v_existing.max_bytes<>p_max_bytes THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN NEXT v_existing;RETURN;
  END IF;
  INSERT INTO agent_project_canary_resources(resource_id,user_id,project_id,relative_path,
    baseline_content,baseline_content_fingerprint,baseline_revision,current_content,
    current_content_fingerprint,current_revision,max_bytes)
  VALUES(p_resource,p_user_id,p_project_id,p_path,p_baseline_content,v_content_fingerprint,
    v_revision,p_baseline_content,v_content_fingerprint,v_revision,p_max_bytes)
  RETURNING * INTO v_existing;
  RETURN NEXT v_existing;
END
$function$;

CREATE FUNCTION agent_project_mutation_commit(
  p_intent_id TEXT,p_user_id UUID,p_run_id TEXT,p_attempt_id TEXT,
  p_generation BIGINT,p_snapshot_fingerprint TEXT,p_grant_fingerprint TEXT,
  p_registry_fingerprint TEXT,p_idempotency_key TEXT,p_resource TEXT,
  p_base_revision TEXT,p_path TEXT,p_content BYTEA,p_content_fingerprint TEXT,
  p_mutation_fingerprint TEXT
) RETURNS SETOF agent_project_mutation_receipts
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_intent agent_effect_intents%ROWTYPE;v_resource agent_project_canary_resources%ROWTYPE;
  v_existing agent_project_mutation_receipts%ROWTYPE;v_content_fingerprint TEXT;
  v_mutation_fingerprint TEXT;v_revision TEXT;v_receipt TEXT;
BEGIN
  IF p_path!~'^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$' OR octet_length(p_content)<1
     OR octet_length(p_content)>4096 THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  PERFORM convert_from(p_content,'UTF8');
  v_content_fingerprint:='sha256:'||encode(sha256(convert_to('neo-project-file-v1','UTF8')||decode('00','hex')||p_content),'hex');
  v_mutation_fingerprint:='sha256:'||encode(sha256(convert_to('neo-project-canary-mutation-v1','UTF8')||decode('00','hex')||
    convert_to(p_base_revision||E'\n'||p_resource||E'\n'||p_path||E'\n'||v_content_fingerprint,'UTF8')),'hex');
  IF p_content_fingerprint<>v_content_fingerprint OR p_mutation_fingerprint<>v_mutation_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  SELECT * INTO v_intent FROM agent_effect_project_mutation_fence(p_intent_id,p_user_id,
    p_run_id,p_attempt_id,p_generation,p_snapshot_fingerprint,p_grant_fingerprint,
    p_registry_fingerprint,p_idempotency_key,p_resource,p_base_revision);
  PERFORM agent_orchestrator_project_mutation_fence(p_user_id,p_run_id,v_intent.step_id,
    p_attempt_id,p_generation,v_intent.lease_owner,v_intent.lease_token_hash,
    p_snapshot_fingerprint,v_intent.kill_switch_epoch,p_grant_fingerprint,
    p_registry_fingerprint,p_resource,p_path,octet_length(p_content));
  SELECT * INTO v_existing FROM agent_project_mutation_receipts
    WHERE idempotency_key=p_idempotency_key OR intent_id=p_intent_id FOR UPDATE;
  IF FOUND THEN
    IF v_existing.intent_id<>p_intent_id OR v_existing.intent_fingerprint<>v_intent.intent_fingerprint
       OR v_existing.resource_id<>p_resource OR v_existing.user_id<>p_user_id
       OR v_existing.run_id<>p_run_id OR v_existing.attempt_id<>p_attempt_id
       OR v_existing.generation<>p_generation OR v_existing.base_revision<>p_base_revision
       OR v_existing.mutation_fingerprint<>p_mutation_fingerprint
       OR v_existing.content_fingerprint<>p_content_fingerprint THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN NEXT v_existing;RETURN;
  END IF;
  SELECT * INTO v_resource FROM agent_project_canary_resources WHERE resource_id=p_resource FOR UPDATE;
  IF NOT FOUND OR v_resource.user_id<>p_user_id OR v_resource.project_id<>v_intent.project_id
     OR v_resource.relative_path<>p_path OR octet_length(p_content)>v_resource.max_bytes THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  IF v_resource.state<>'clean' OR v_resource.current_revision<>p_base_revision THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_CONFLICT';
  END IF;
  v_revision:='sha256:'||encode(sha256(convert_to('neo-project-canary-commit-v1','UTF8')||decode('00','hex')||
    convert_to(p_base_revision||E'\n'||p_mutation_fingerprint,'UTF8')),'hex');
  v_receipt:='sha256:'||encode(sha256(convert_to('neo-project-canary-receipt-v1','UTF8')||decode('00','hex')||
    convert_to(p_idempotency_key||E'\n'||p_resource||E'\n'||v_revision||E'\n'||p_mutation_fingerprint,'UTF8')),'hex');
  UPDATE agent_project_canary_resources SET current_content=p_content,
    current_content_fingerprint=p_content_fingerprint,current_revision=v_revision,
    state='mutated',updated_at=clock_timestamp() WHERE resource_id=p_resource;
  INSERT INTO agent_project_mutation_receipts(idempotency_key,intent_id,intent_fingerprint,
    resource_id,user_id,run_id,attempt_id,generation,base_revision,mutation_fingerprint,
    content_fingerprint,committed_revision,receipt_fingerprint)
  VALUES(p_idempotency_key,p_intent_id,v_intent.intent_fingerprint,p_resource,p_user_id,
    p_run_id,p_attempt_id,p_generation,p_base_revision,p_mutation_fingerprint,
    p_content_fingerprint,v_revision,v_receipt) RETURNING * INTO v_existing;
  RETURN NEXT v_existing;
END
$function$;

CREATE FUNCTION agent_project_mutation_status(
  p_intent_id TEXT,p_user_id UUID,p_idempotency_key TEXT,p_resource TEXT,
  p_base_revision TEXT,p_mutation_fingerprint TEXT
) RETURNS TABLE(outcome TEXT,receipt_fingerprint TEXT,current_revision TEXT)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_receipt agent_project_mutation_receipts%ROWTYPE;v_resource agent_project_canary_resources%ROWTYPE;
BEGIN
  SELECT * INTO v_receipt FROM agent_project_mutation_receipts
    WHERE idempotency_key=p_idempotency_key OR intent_id=p_intent_id;
  IF FOUND THEN
    IF v_receipt.intent_id<>p_intent_id OR v_receipt.user_id<>p_user_id
       OR v_receipt.resource_id<>p_resource OR v_receipt.base_revision<>p_base_revision
       OR v_receipt.mutation_fingerprint<>p_mutation_fingerprint THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN QUERY SELECT 'committed'::TEXT,v_receipt.receipt_fingerprint,v_receipt.committed_revision;RETURN;
  END IF;
  SELECT * INTO v_resource FROM agent_project_canary_resources WHERE resource_id=p_resource;
  IF NOT FOUND OR v_resource.user_id<>p_user_id THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  IF v_resource.state='clean' AND v_resource.current_revision=p_base_revision THEN
    RETURN QUERY SELECT 'rejected'::TEXT,''::TEXT,v_resource.current_revision;RETURN;
  END IF;
  RETURN QUERY SELECT 'outcome_unknown'::TEXT,''::TEXT,v_resource.current_revision;
END
$function$;

CREATE FUNCTION agent_project_mutation_cleanup(
  p_intent_id TEXT,p_user_id UUID,p_idempotency_key TEXT,p_resource TEXT,
  p_receipt_fingerprint TEXT
) RETURNS SETOF agent_project_mutation_cleanups
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_receipt agent_project_mutation_receipts%ROWTYPE;v_resource agent_project_canary_resources%ROWTYPE;
  v_cleanup agent_project_mutation_cleanups%ROWTYPE;
BEGIN
  SELECT * INTO v_receipt FROM agent_project_mutation_receipts
    WHERE idempotency_key=p_idempotency_key AND intent_id=p_intent_id FOR UPDATE;
  IF v_receipt.idempotency_key IS NULL OR v_receipt.user_id<>p_user_id
     OR v_receipt.resource_id<>p_resource OR v_receipt.receipt_fingerprint<>p_receipt_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_MUTATION_DENIED';
  END IF;
  PERFORM agent_effect_project_cleanup_fence(p_intent_id,p_user_id,p_receipt_fingerprint);
  SELECT * INTO v_cleanup FROM agent_project_mutation_cleanups
    WHERE receipt_fingerprint=p_receipt_fingerprint;
  IF FOUND THEN RETURN NEXT v_cleanup;RETURN;END IF;
  SELECT * INTO v_resource FROM agent_project_canary_resources WHERE resource_id=p_resource FOR UPDATE;
  IF NOT FOUND OR v_resource.state<>'mutated'
     OR v_resource.current_revision<>v_receipt.committed_revision
     OR v_resource.current_content_fingerprint<>v_receipt.content_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROJECT_CONFLICT';
  END IF;
  UPDATE agent_project_canary_resources SET current_content=baseline_content,
    current_content_fingerprint=baseline_content_fingerprint,current_revision=baseline_revision,
    state='clean',updated_at=clock_timestamp() WHERE resource_id=p_resource;
  INSERT INTO agent_project_mutation_cleanups(receipt_fingerprint,resource_id,restored_revision)
    VALUES(p_receipt_fingerprint,p_resource,v_resource.baseline_revision) RETURNING * INTO v_cleanup;
  UPDATE agent_project_mutation_receipts SET cleaned_at=clock_timestamp()
    WHERE receipt_fingerprint=p_receipt_fingerprint;
  RETURN NEXT v_cleanup;
END
$function$;

-- The cleanup timestamp is the only mutable receipt field and is written only
-- by the cleanup function. Replace the blanket trigger with a narrow guard.
DROP TRIGGER agent_project_mutation_receipts_immutable ON agent_project_mutation_receipts;
CREATE FUNCTION agent_project_mutation_receipt_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  IF TG_OP='DELETE' OR NEW.idempotency_key<>OLD.idempotency_key
     OR NEW.intent_id<>OLD.intent_id OR NEW.intent_fingerprint<>OLD.intent_fingerprint
     OR NEW.resource_id<>OLD.resource_id OR NEW.user_id<>OLD.user_id
     OR NEW.run_id<>OLD.run_id OR NEW.attempt_id<>OLD.attempt_id
     OR NEW.generation<>OLD.generation OR NEW.base_revision<>OLD.base_revision
     OR NEW.mutation_fingerprint<>OLD.mutation_fingerprint
     OR NEW.content_fingerprint<>OLD.content_fingerprint
     OR NEW.committed_revision<>OLD.committed_revision
     OR NEW.receipt_fingerprint<>OLD.receipt_fingerprint OR NEW.committed_at<>OLD.committed_at
     OR OLD.cleaned_at IS NOT NULL OR NEW.cleaned_at IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PROJECT_MUTATION_FACT_IMMUTABLE';
  END IF;
  RETURN NEW;
END
$function$;
CREATE TRIGGER agent_project_mutation_receipts_immutable
BEFORE UPDATE OR DELETE ON agent_project_mutation_receipts
FOR EACH ROW EXECUTE FUNCTION agent_project_mutation_receipt_guard();

DO $harden$
DECLARE schema_name TEXT:=current_schema();identity TEXT;
BEGIN
  FOREACH identity IN ARRAY ARRAY[
    'agent_project_mutation_fact_immutable()','agent_project_mutation_receipt_guard()',
    'agent_effect_project_mutation_fence(text,uuid,text,text,bigint,text,text,text,text,text,text)',
    'agent_effect_project_cleanup_fence(text,uuid,text)',
    'agent_orchestrator_project_mutation_fence(uuid,text,text,text,bigint,text,text,text,bigint,text,text,text,text,bigint)',
    'agent_project_canary_provision(text,uuid,text,text,bytea,bigint)',
    'agent_project_mutation_commit(text,uuid,text,text,bigint,text,text,text,text,text,text,text,bytea,text,text)',
    'agent_project_mutation_status(text,uuid,text,text,text,text)',
    'agent_project_mutation_cleanup(text,uuid,text,text,text)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',schema_name,identity,schema_name);
  END LOOP;
END
$harden$;

ALTER TABLE agent_project_canary_resources OWNER TO agent_project_mutation_owner;
ALTER TABLE agent_project_mutation_receipts OWNER TO agent_project_mutation_owner;
ALTER TABLE agent_project_mutation_cleanups OWNER TO agent_project_mutation_owner;
ALTER FUNCTION agent_project_mutation_fact_immutable() OWNER TO agent_project_mutation_owner;
ALTER FUNCTION agent_project_mutation_receipt_guard() OWNER TO agent_project_mutation_owner;
ALTER FUNCTION agent_effect_project_mutation_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_project_cleanup_fence(TEXT,UUID,TEXT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_orchestrator_project_mutation_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_project_canary_provision(TEXT,UUID,TEXT,TEXT,BYTEA,BIGINT) OWNER TO agent_project_mutation_owner;
ALTER FUNCTION agent_project_mutation_commit(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BYTEA,TEXT,TEXT) OWNER TO agent_project_mutation_owner;
ALTER FUNCTION agent_project_mutation_status(TEXT,UUID,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_project_mutation_owner;
ALTER FUNCTION agent_project_mutation_cleanup(TEXT,UUID,TEXT,TEXT,TEXT) OWNER TO agent_project_mutation_owner;

REVOKE ALL ON agent_project_canary_resources,agent_project_mutation_receipts,
  agent_project_mutation_cleanups FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,
  agent_runner_control,agent_effect_control,agent_artifact_control,agent_project_mutation_control;
GRANT SELECT ON agent_project_canary_resources,agent_project_mutation_receipts,
  agent_project_mutation_cleanups TO agent_project_mutation_control;

REVOKE ALL ON FUNCTION agent_effect_project_mutation_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_project_cleanup_fence(TEXT,UUID,TEXT),
  agent_orchestrator_project_mutation_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT),
  agent_project_canary_provision(TEXT,UUID,TEXT,TEXT,BYTEA,BIGINT),
  agent_project_mutation_commit(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BYTEA,TEXT,TEXT),
  agent_project_mutation_status(TEXT,UUID,TEXT,TEXT,TEXT,TEXT),
  agent_project_mutation_cleanup(TEXT,UUID,TEXT,TEXT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
    agent_effect_control,agent_artifact_control,agent_project_mutation_control;
GRANT EXECUTE ON FUNCTION agent_effect_project_mutation_fence(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT)
  TO agent_project_mutation_owner;
GRANT EXECUTE ON FUNCTION agent_effect_project_cleanup_fence(TEXT,UUID,TEXT)
  TO agent_project_mutation_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_project_mutation_fence(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT)
  TO agent_project_mutation_owner;
GRANT EXECUTE ON FUNCTION agent_project_mutation_commit(TEXT,UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BYTEA,TEXT,TEXT),
  agent_project_mutation_status(TEXT,UUID,TEXT,TEXT,TEXT,TEXT),
  agent_project_mutation_cleanup(TEXT,UUID,TEXT,TEXT,TEXT)
  TO agent_project_mutation_control;

DO $schema_acl$ BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_project_mutation_owner,agent_project_mutation_control',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_project_mutation_owner,agent_project_mutation_control',current_schema());
END
$schema_acl$;
