-- G20.5 held Child Agent delegation authority. This migration adds no public
-- route, startup worker, Runner credential or production execution path.

DO $roles$
DECLARE role_name TEXT; can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create FROM pg_roles WHERE rolname=current_user;
  FOREACH role_name IN ARRAY ARRAY['agent_delegation_owner','agent_delegation_control'] LOOP
    IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF NOT can_create THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_DELEGATION_REQUIRED_ROLE_MISSING';END IF;
      EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',role_name);
    END IF;
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name AND
      (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_DELEGATION_ROLE_MUST_BE_RESTRICTED';
    END IF;
  END LOOP;
END
$roles$;

CREATE FUNCTION agent_delegation_budget_valid(p_budget JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
BEGIN
  RETURN COALESCE(jsonb_typeof(p_budget)='object'
    AND (p_budget-ARRAY['maxWallSeconds','maxModelTokens','maxToolCalls','maxArtifactBytes']::text[])='{}'::jsonb
    AND p_budget ?& ARRAY['maxWallSeconds','maxModelTokens','maxToolCalls','maxArtifactBytes']
    AND jsonb_typeof(p_budget->'maxWallSeconds')='number'
    AND jsonb_typeof(p_budget->'maxModelTokens')='number'
    AND jsonb_typeof(p_budget->'maxToolCalls')='number'
    AND jsonb_typeof(p_budget->'maxArtifactBytes')='number'
    AND (p_budget->>'maxWallSeconds')::BIGINT BETWEEN 0 AND 86400
    AND (p_budget->>'maxModelTokens')::BIGINT BETWEEN 0 AND 10000000
    AND (p_budget->>'maxToolCalls')::BIGINT BETWEEN 0 AND 10000
    AND (p_budget->>'maxArtifactBytes')::BIGINT BETWEEN 0 AND 10737418240,false);
EXCEPTION WHEN OTHERS THEN RETURN false;
END
$function$;

CREATE FUNCTION agent_delegation_json_subset(p_child JSONB,p_parent JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE child_cap JSONB; parent_cap JSONB; child_secret JSONB; parent_secret JSONB;
  child_rule JSONB; parent_rule JSONB; child_resource TEXT; parent_resource TEXT; matched BOOLEAN;
BEGIN
  IF jsonb_typeof(p_child) IS DISTINCT FROM 'object' OR jsonb_typeof(p_parent) IS DISTINCT FROM 'object' THEN RETURN false;END IF;
  IF p_child->>'schemaVersion' IS DISTINCT FROM 'neo.capability-grant/v1'
     OR p_parent->>'schemaVersion' IS DISTINCT FROM 'neo.capability-grant/v1'
     OR p_child->>'issuedAt' IS DISTINCT FROM p_parent->>'issuedAt'
     OR p_child->'subject' IS DISTINCT FROM p_parent->'subject'
     OR p_child->>'packageFingerprint' IS DISTINCT FROM p_parent->>'packageFingerprint'
     OR p_child->>'runtimeBundleFingerprint' IS DISTINCT FROM p_parent->>'runtimeBundleFingerprint'
     OR NOT agent_delegation_budget_valid(p_child->'budget')
     OR NOT agent_delegation_budget_valid(p_parent->'budget')
     OR (p_child#>>'{budget,maxWallSeconds}')::BIGINT>(p_parent#>>'{budget,maxWallSeconds}')::BIGINT
     OR (p_child#>>'{budget,maxModelTokens}')::BIGINT>(p_parent#>>'{budget,maxModelTokens}')::BIGINT
     OR (p_child#>>'{budget,maxToolCalls}')::BIGINT>(p_parent#>>'{budget,maxToolCalls}')::BIGINT
     OR (p_child#>>'{budget,maxArtifactBytes}')::BIGINT>(p_parent#>>'{budget,maxArtifactBytes}')::BIGINT
     OR (p_child->>'expiresAt')::TIMESTAMPTZ>(p_parent->>'expiresAt')::TIMESTAMPTZ THEN RETURN false;END IF;

  IF p_child->'run' IS DISTINCT FROM jsonb_build_object('runId',p_child#>>'{run,runId}',
      'parentRunId',p_child#>>'{run,parentRunId}','depth',1)
     OR (p_parent#>>'{run,depth}')::INTEGER<>0
     OR p_child#>>'{run,parentRunId}'<>p_parent#>>'{run,runId}' THEN RETURN false;END IF;

  IF (CASE p_child#>>'{egress,mode}' WHEN 'none' THEN 0 WHEN 'allowlist' THEN 1 WHEN 'brokered' THEN 2 ELSE 99 END)
     > (CASE p_parent#>>'{egress,mode}' WHEN 'none' THEN 0 WHEN 'allowlist' THEN 1 WHEN 'brokered' THEN 2 ELSE -1 END) THEN RETURN false;END IF;
  FOR child_rule IN SELECT value FROM jsonb_array_elements(p_child#>'{egress,rules}') LOOP
    matched:=false;
    FOR parent_rule IN SELECT value FROM jsonb_array_elements(p_parent#>'{egress,rules}') LOOP
      IF child_rule->>'scheme'=parent_rule->>'scheme' AND child_rule->>'host'=parent_rule->>'host'
         AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(child_rule->'ports') p
           WHERE NOT (parent_rule->'ports' @> jsonb_build_array(p.value))) THEN matched:=true;EXIT;END IF;
    END LOOP;
    IF NOT matched THEN RETURN false;END IF;
  END LOOP;

  FOR child_secret IN SELECT value FROM jsonb_array_elements(p_child->'secrets') LOOP
    SELECT value INTO parent_secret FROM jsonb_array_elements(p_parent->'secrets')
      WHERE value->>'slot'=child_secret->>'slot';
    IF parent_secret IS NULL OR child_secret->>'brokerRef'<>parent_secret->>'brokerRef'
       OR (child_secret->>'ttlSeconds')::INTEGER>(parent_secret->>'ttlSeconds')::INTEGER
       OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(child_secret->'actions') action
         WHERE NOT (parent_secret->'actions' ? action.value)) THEN RETURN false;END IF;
  END LOOP;

  FOR child_cap IN SELECT value FROM jsonb_array_elements(p_child->'capabilities') LOOP
    IF child_cap->>'capability' IN ('delegate_task','cron_manage','grant_manage','secret_manage','runtime_manage') THEN RETURN false;END IF;
    SELECT value INTO parent_cap FROM jsonb_array_elements(p_parent->'capabilities')
      WHERE value->>'capability'=child_cap->>'capability';
    IF parent_cap IS NULL OR (child_cap->>'maxCalls')::INTEGER>(parent_cap->>'maxCalls')::INTEGER
       OR (CASE child_cap->>'approval' WHEN 'denied' THEN 0 WHEN 'per_commit' THEN 1 WHEN 'once' THEN 2 WHEN 'automatic' THEN 3 ELSE 99 END)
          > (CASE parent_cap->>'approval' WHEN 'denied' THEN 0 WHEN 'per_commit' THEN 1 WHEN 'once' THEN 2 WHEN 'automatic' THEN 3 ELSE -1 END)
       OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(child_cap->'actions') action
          WHERE NOT (parent_cap->'actions' ? action.value)) THEN RETURN false;END IF;
    IF child_cap#>>'{resources,kind}' NOT IN ('exact','prefix') THEN RETURN false;END IF;
    FOR child_resource IN SELECT value FROM jsonb_array_elements_text(child_cap#>'{resources,values}') LOOP
      matched:=false;
      FOR parent_resource IN SELECT value FROM jsonb_array_elements_text(parent_cap#>'{resources,values}') LOOP
        IF (parent_cap#>>'{resources,kind}')='prefix' AND starts_with(child_resource,parent_resource)
           OR (parent_cap#>>'{resources,kind}')='exact' AND (child_cap#>>'{resources,kind}')='exact' AND child_resource=parent_resource THEN
          matched:=true;EXIT;
        END IF;
      END LOOP;
      IF NOT matched THEN RETURN false;END IF;
    END LOOP;
  END LOOP;
  RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END
$function$;

CREATE FUNCTION agent_delegation_registry_valid(p_registry JSONB,p_depth INTEGER)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
BEGIN
  RETURN COALESCE(p_depth IN (0,1)
    AND jsonb_typeof(p_registry)='object'
    AND p_registry->>'schemaVersion'='neo.tool-registry/v1'
    AND jsonb_typeof(p_registry->'subject')='object'
    AND jsonb_typeof(p_registry->'egress')='object'
    AND jsonb_typeof(p_registry->'secrets')='array'
    AND agent_delegation_budget_valid(p_registry->'budget')
    AND jsonb_typeof(p_registry->'tools')='array'
    AND jsonb_array_length(p_registry->'tools')<=128
    AND (p_registry->>'depth')::INTEGER=p_depth
    AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(p_registry->'tools') tool
      WHERE jsonb_typeof(tool) IS DISTINCT FROM 'object'
         OR COALESCE(tool->>'identity','')!~'^[a-z][a-z0-9]*([._-][a-z0-9]+)*$'
         OR COALESCE(tool->>'capability','')!~'^[a-z][a-z0-9]*([._-][a-z0-9]+)*$'
         OR jsonb_typeof(tool->'actions') IS DISTINCT FROM 'array' OR jsonb_array_length(tool->'actions') NOT BETWEEN 1 AND 32
         OR jsonb_typeof(tool->'resources') IS DISTINCT FROM 'object' OR COALESCE(tool#>>'{resources,kind}','') NOT IN ('exact','prefix')
         OR jsonb_typeof(tool#>'{resources,values}') IS DISTINCT FROM 'array' OR jsonb_array_length(tool#>'{resources,values}') NOT BETWEEN 1 AND 128
         OR COALESCE(tool->>'approval','') NOT IN ('automatic','once','per_commit')
         OR COALESCE((tool->>'maxCalls')::INTEGER,0) NOT BETWEEN 1 AND 10000
         OR COALESCE(tool->>'classification','') NOT IN ('read','mutable','unknown')
         OR jsonb_typeof(tool->'idempotent') IS DISTINCT FROM 'boolean'
         OR ((tool->>'idempotent')::BOOLEAN AND tool->>'classification'<>'read'))
    AND (SELECT count(*)=count(DISTINCT tool->>'identity') FROM jsonb_array_elements(p_registry->'tools') tool)
    AND (p_depth=0 OR NOT EXISTS(
      SELECT 1 FROM jsonb_array_elements(p_registry->'tools') tool
      WHERE tool->>'identity' IN ('delegate_task','cron_manage','grant_manage','secret_manage','runtime_manage')
         OR tool->>'capability' IN ('delegate_task','cron_manage','grant_manage','secret_manage','runtime_manage')
    )),false);
EXCEPTION WHEN OTHERS THEN RETURN false;
END
$function$;

CREATE FUNCTION agent_delegation_registry_subset(p_child JSONB,p_parent JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE child_tool JSONB;parent_tool JSONB;child_resource TEXT;parent_resource TEXT;matched BOOLEAN;
BEGIN
  IF NOT agent_delegation_registry_valid(p_child,1) OR NOT agent_delegation_registry_valid(p_parent,0) THEN RETURN false;END IF;
  FOR child_tool IN SELECT value FROM jsonb_array_elements(p_child->'tools') LOOP
    SELECT value INTO parent_tool FROM jsonb_array_elements(p_parent->'tools')
      WHERE value->>'identity'=child_tool->>'identity';
    IF parent_tool IS NULL OR child_tool->>'capability' IS DISTINCT FROM parent_tool->>'capability'
       OR child_tool->>'classification' IS DISTINCT FROM parent_tool->>'classification'
       OR (child_tool->>'idempotent')::BOOLEAN IS DISTINCT FROM (parent_tool->>'idempotent')::BOOLEAN
       OR (child_tool->>'maxCalls')::INTEGER>(parent_tool->>'maxCalls')::INTEGER
       OR (CASE child_tool->>'approval' WHEN 'per_commit' THEN 1 WHEN 'once' THEN 2 WHEN 'automatic' THEN 3 ELSE 99 END)
          > (CASE parent_tool->>'approval' WHEN 'per_commit' THEN 1 WHEN 'once' THEN 2 WHEN 'automatic' THEN 3 ELSE -1 END)
       OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(child_tool->'actions') action
          WHERE NOT (parent_tool->'actions' ? action.value)) THEN RETURN false;END IF;
    FOR child_resource IN SELECT value FROM jsonb_array_elements_text(child_tool#>'{resources,values}') LOOP
      matched:=false;
      FOR parent_resource IN SELECT value FROM jsonb_array_elements_text(parent_tool#>'{resources,values}') LOOP
        IF (parent_tool#>>'{resources,kind}')='prefix' AND starts_with(child_resource,parent_resource)
           OR (parent_tool#>>'{resources,kind}')='exact' AND (child_tool#>>'{resources,kind}')='exact' AND child_resource=parent_resource THEN
          matched:=true;EXIT;
        END IF;
      END LOOP;
      IF NOT matched THEN RETURN false;END IF;
    END LOOP;
  END LOOP;
  RETURN true;
EXCEPTION WHEN OTHERS THEN RETURN false;
END
$function$;

CREATE TABLE agent_delegation_authorities(
  run_id TEXT PRIMARY KEY REFERENCES agent_runs(id) ON DELETE CASCADE,
  root_run_id TEXT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
  parent_run_id TEXT REFERENCES agent_runs(id) ON DELETE CASCADE,
  depth SMALLINT NOT NULL,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  subject JSONB NOT NULL, model JSONB NOT NULL,
  package_fingerprint TEXT NOT NULL,runtime_bundle_fingerprint TEXT NOT NULL,
  grant_id TEXT NOT NULL,grant_fingerprint TEXT NOT NULL,grant_json JSONB NOT NULL,
  registry_fingerprint TEXT NOT NULL,registry_json JSONB NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,budget JSONB NOT NULL,
  reserved_budget JSONB NOT NULL DEFAULT '{"maxWallSeconds":0,"maxModelTokens":0,"maxToolCalls":0,"maxArtifactBytes":0}'::jsonb,
  consumed_budget JSONB NOT NULL DEFAULT '{"maxWallSeconds":0,"maxModelTokens":0,"maxToolCalls":0,"maxArtifactBytes":0}'::jsonb,
  state TEXT NOT NULL DEFAULT 'active',created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_delegation_run_owner_fk FOREIGN KEY(run_id,user_id) REFERENCES agent_runs(id,user_id) ON DELETE CASCADE,
  CONSTRAINT agent_delegation_snapshot_fk FOREIGN KEY(snapshot_id,user_id,snapshot_fingerprint)
    REFERENCES agent_run_snapshots(id,user_id,fingerprint) ON DELETE CASCADE,
  CONSTRAINT agent_delegation_depth_shape CHECK(
    (depth=0 AND parent_run_id IS NULL AND root_run_id=run_id) OR
    (depth=1 AND parent_run_id IS NOT NULL AND parent_run_id<>run_id AND root_run_id=parent_run_id)),
  CONSTRAINT agent_delegation_subject_check CHECK(jsonb_typeof(subject)='object'),
  CONSTRAINT agent_delegation_model_check CHECK(jsonb_typeof(model)='object' AND octet_length(model::text)<=1024),
  CONSTRAINT agent_delegation_fingerprints CHECK(snapshot_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND package_fingerprint~'^sha256:[a-f0-9]{64}$' AND runtime_bundle_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND grant_fingerprint~'^sha256:[a-f0-9]{64}$' AND registry_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_delegation_grant_check CHECK(grant_id~'^grant_[a-z0-9]{16,64}$' AND jsonb_typeof(grant_json)='object'),
  CONSTRAINT agent_delegation_registry_check CHECK(agent_delegation_registry_valid(registry_json,depth)),
  CONSTRAINT agent_delegation_budget_check CHECK(agent_delegation_budget_valid(budget)
    AND agent_delegation_budget_valid(reserved_budget) AND agent_delegation_budget_valid(consumed_budget)),
  CONSTRAINT agent_delegation_state_check CHECK(state IN ('active','settled','canceled','killed','outcome_unknown')),
  UNIQUE(run_id,user_id,depth),UNIQUE(run_id,user_id,parent_run_id,depth)
);
CREATE INDEX idx_agent_delegation_parent ON agent_delegation_authorities(parent_run_id,state,run_id) WHERE depth=1;

CREATE TABLE agent_delegation_lineage(
  child_run_id TEXT PRIMARY KEY REFERENCES agent_delegation_authorities(run_id) ON DELETE CASCADE,
  parent_run_id TEXT NOT NULL REFERENCES agent_delegation_authorities(run_id) ON DELETE CASCADE,
  user_id UUID NOT NULL,parent_step_id TEXT NOT NULL,parent_attempt_id TEXT NOT NULL,parent_generation BIGINT NOT NULL,
  parent_lease_owner TEXT NOT NULL,parent_lease_token_hash TEXT NOT NULL,idempotency_key TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,reservation JSONB NOT NULL,settled_usage JSONB,
  state TEXT NOT NULL DEFAULT 'reserved',created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),settled_at TIMESTAMPTZ,
  CONSTRAINT agent_delegation_parent_attempt_fk FOREIGN KEY(parent_run_id,user_id,parent_attempt_id)
    REFERENCES agent_attempts(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_delegation_parent_step_fk FOREIGN KEY(parent_run_id,user_id,parent_step_id)
    REFERENCES agent_steps(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_delegation_lineage_generation CHECK(parent_generation>=1),
  CONSTRAINT agent_delegation_lineage_token CHECK(parent_lease_token_hash~'^[a-f0-9]{64}$'),
  CONSTRAINT agent_delegation_lineage_request CHECK(request_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_delegation_lineage_budget CHECK(agent_delegation_budget_valid(reservation)
    AND (settled_usage IS NULL OR agent_delegation_budget_valid(settled_usage))),
  CONSTRAINT agent_delegation_lineage_state CHECK(state IN ('reserved','settled','canceled','killed','outcome_unknown')),
  CONSTRAINT agent_delegation_lineage_settlement CHECK((state='reserved' AND settled_usage IS NULL AND settled_at IS NULL)
    OR (state<>'reserved' AND settled_usage IS NOT NULL AND settled_at IS NOT NULL)),
  UNIQUE(user_id,parent_run_id,idempotency_key)
);

CREATE TABLE agent_delegation_settlements(
  settlement_id TEXT PRIMARY KEY,child_run_id TEXT NOT NULL UNIQUE REFERENCES agent_delegation_lineage(child_run_id) ON DELETE CASCADE,
  outcome TEXT NOT NULL,usage JSONB NOT NULL,settled_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_delegation_settlement_id CHECK(settlement_id~'^settlement_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_delegation_settlement_outcome CHECK(outcome IN ('succeeded','failed','canceled','killed','outcome_unknown')),
  CONSTRAINT agent_delegation_settlement_budget CHECK(agent_delegation_budget_valid(usage))
);

CREATE TABLE agent_delegation_reaps(
  reap_id TEXT PRIMARY KEY,parent_run_id TEXT NOT NULL,child_run_id TEXT NOT NULL,attempt_id TEXT NOT NULL,
  generation BIGINT NOT NULL,lease_owner TEXT NOT NULL,mode TEXT NOT NULL,state TEXT NOT NULL DEFAULT 'pending',
  retry_count INTEGER NOT NULL DEFAULT 0,error_code TEXT,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),completed_at TIMESTAMPTZ,
  CONSTRAINT agent_delegation_reap_id CHECK(reap_id~'^reap_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_delegation_reap_attempt_fk FOREIGN KEY(child_run_id,attempt_id) REFERENCES agent_attempts(run_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_delegation_reap_mode CHECK(mode IN ('cancel','kill')),
  CONSTRAINT agent_delegation_reap_state CHECK(state IN ('pending','failed','reaped')),
  CONSTRAINT agent_delegation_reap_error CHECK(error_code IS NULL OR error_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_delegation_reap_shape CHECK((state='reaped' AND completed_at IS NOT NULL AND error_code IS NULL)
    OR (state<>'reaped' AND completed_at IS NULL)),UNIQUE(child_run_id,attempt_id,generation,mode)
);
CREATE INDEX idx_agent_delegation_reaps_pending ON agent_delegation_reaps(state,created_at,reap_id) WHERE state IN ('pending','failed');

CREATE FUNCTION agent_delegation_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN IF TG_OP='DELETE' AND pg_trigger_depth()>1 THEN RETURN OLD;END IF;
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_DELEGATION_FACT_IMMUTABLE';END
$function$;
CREATE TRIGGER trg_agent_delegation_settlement_immutable BEFORE UPDATE OR DELETE ON agent_delegation_settlements
FOR EACH ROW EXECUTE FUNCTION agent_delegation_immutable();

CREATE FUNCTION agent_delegation_register_root(
  p_run_id TEXT,p_user_id UUID,p_subject JSONB,p_model JSONB,p_package_fingerprint TEXT,p_runtime_fingerprint TEXT,
  p_grant_id TEXT,p_grant_fingerprint TEXT,p_grant JSONB,p_registry_fingerprint TEXT,p_registry JSONB,p_expires_at TIMESTAMPTZ
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_run agent_runs%ROWTYPE;v_snapshot agent_run_snapshots%ROWTYPE;v_existing agent_delegation_authorities%ROWTYPE;
BEGIN
  SELECT * INTO v_run FROM agent_runs WHERE id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_snapshot FROM agent_run_snapshots WHERE id=v_run.snapshot_id;
  IF NOT FOUND OR v_run.state NOT IN ('admitted','queued','running') OR p_grant#>>'{run,runId}'<>p_run_id
     OR (p_grant#>>'{run,depth}')::INTEGER<>0 OR p_grant#>'{run}'?'parentRunId'
     OR p_grant->>'schemaVersion'<>'neo.capability-grant/v1' OR p_grant->>'grantId'<>p_grant_id
     OR p_registry->>'runId'<>p_run_id OR (p_registry->>'depth')::INTEGER<>0
     OR NOT agent_delegation_registry_valid(p_registry,0)
     OR p_grant->'subject'<>p_subject OR p_registry->'subject'<>p_subject
     OR p_subject->>'userId' IS DISTINCT FROM p_user_id::TEXT
     OR p_grant->>'packageFingerprint'<>p_package_fingerprint OR p_grant->>'runtimeBundleFingerprint'<>p_runtime_fingerprint
     OR p_registry->>'packageFingerprint'<>p_package_fingerprint OR p_registry->>'runtimeBundleFingerprint'<>p_runtime_fingerprint
     OR p_registry->>'grantId'<>p_grant_id OR p_registry->>'registryFingerprint'<>p_registry_fingerprint
     OR p_registry->'egress'<>p_grant->'egress' OR p_registry->'secrets'<>p_grant->'secrets'
     OR p_registry->'budget'<>p_grant->'budget'
     OR (p_registry->>'expiresAt')::TIMESTAMPTZ<>(p_grant->>'expiresAt')::TIMESTAMPTZ
     OR abs(extract(epoch FROM (p_expires_at-(p_grant->>'expiresAt')::TIMESTAMPTZ)))>0.000001
     OR p_expires_at<=clock_timestamp() OR NOT agent_delegation_budget_valid(p_grant->'budget') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PARENT_INVALID';END IF;
  SELECT * INTO v_existing FROM agent_delegation_authorities WHERE run_id=p_run_id;
  IF FOUND THEN
    IF v_existing.user_id<>p_user_id OR v_existing.subject<>p_subject OR v_existing.model<>p_model
       OR v_existing.grant_fingerprint<>p_grant_fingerprint OR v_existing.registry_fingerprint<>p_registry_fingerprint THEN
      RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='IDEMPOTENCY_CONFLICT';END IF;
    RETURN false;
  END IF;
  INSERT INTO agent_delegation_authorities(run_id,root_run_id,parent_run_id,depth,user_id,snapshot_id,snapshot_fingerprint,
    subject,model,package_fingerprint,runtime_bundle_fingerprint,grant_id,grant_fingerprint,grant_json,
    registry_fingerprint,registry_json,expires_at,budget)
  VALUES(p_run_id,p_run_id,NULL,0,p_user_id,v_run.snapshot_id,v_run.snapshot_fingerprint,p_subject,p_model,
    p_package_fingerprint,p_runtime_fingerprint,p_grant_id,p_grant_fingerprint,p_grant,p_registry_fingerprint,p_registry,p_expires_at,p_grant->'budget');
  RETURN true;
END
$function$;

CREATE FUNCTION agent_delegation_enqueue_child(
  p_child_run_id TEXT,p_child_snapshot_id TEXT,p_user_id UUID,p_idempotency_key TEXT,p_child_snapshot_fingerprint TEXT,
  p_request_fingerprint TEXT,p_child_snapshot JSONB,p_steps JSONB,p_scope_keys TEXT[],p_event_ids TEXT[],
  p_parent_run_id TEXT,p_parent_step_id TEXT,p_parent_attempt_id TEXT,p_parent_generation BIGINT,p_parent_lease_owner TEXT,
  p_parent_lease_token_hash TEXT,p_child_grant_fingerprint TEXT,p_child_grant JSONB,p_child_registry_fingerprint TEXT,p_child_registry JSONB
) RETURNS TABLE(run_id TEXT,created BOOLEAN) LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_parent agent_delegation_authorities%ROWTYPE;v_existing agent_delegation_lineage%ROWTYPE;v_attempt agent_attempts%ROWTYPE;
  v_child_depth INTEGER;v_child_budget JSONB;v_now TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT authority.* INTO v_parent FROM agent_delegation_authorities authority WHERE authority.run_id=p_parent_run_id AND authority.user_id=p_user_id FOR UPDATE;
  SELECT lineage.* INTO v_existing FROM agent_delegation_lineage lineage WHERE lineage.user_id=p_user_id AND lineage.parent_run_id=p_parent_run_id
    AND lineage.idempotency_key=p_idempotency_key FOR UPDATE;
  IF FOUND THEN
    IF v_existing.request_fingerprint<>p_request_fingerprint THEN RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='IDEMPOTENCY_CONFLICT';END IF;
    RETURN QUERY SELECT v_existing.child_run_id,false;RETURN;
  END IF;
  IF v_parent.depth<>0 OR v_parent.parent_run_id IS NOT NULL OR v_parent.state<>'active' OR v_parent.expires_at<=v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PARENT_INVALID';END IF;
  SELECT attempt.* INTO v_attempt FROM agent_attempts attempt WHERE attempt.id=p_parent_attempt_id AND attempt.run_id=p_parent_run_id AND attempt.user_id=p_user_id
    AND attempt.step_id=p_parent_step_id FOR UPDATE;
  IF NOT FOUND OR v_attempt.generation<>p_parent_generation OR v_attempt.lease_owner<>p_parent_lease_owner
     OR v_attempt.lease_token_hash<>p_parent_lease_token_hash OR v_attempt.state NOT IN ('starting','running','prepared','committing')
     OR v_attempt.lease_expires_at<=v_now THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PARENT_STALE';END IF;
  IF agent_orchestrator_active_kill_mode((SELECT scope_keys FROM agent_runs WHERE id=p_parent_run_id)) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';END IF;
  v_child_depth:=(p_child_grant#>>'{run,depth}')::INTEGER;v_child_budget:=p_child_grant->'budget';
  IF v_child_depth<>1 OR p_child_grant#>>'{run,parentRunId}'<>p_parent_run_id OR p_child_grant#>>'{run,runId}'<>p_child_run_id
     OR p_child_registry->>'runId'<>p_child_run_id OR (p_child_registry->>'depth')::INTEGER<>1
     OR NOT agent_delegation_registry_valid(p_child_registry,1)
     OR p_child_registry->>'registryFingerprint'<>p_child_registry_fingerprint
     OR p_child_registry->'subject'<>v_parent.subject
     OR p_child_registry->>'packageFingerprint'<>v_parent.package_fingerprint
     OR p_child_registry->>'runtimeBundleFingerprint'<>v_parent.runtime_bundle_fingerprint
     OR p_child_registry->>'grantId'<>p_child_grant->>'grantId'
     OR p_child_registry->'egress'<>p_child_grant->'egress'
     OR p_child_registry->'secrets'<>p_child_grant->'secrets'
     OR p_child_registry->'budget'<>p_child_grant->'budget'
     OR (p_child_registry->>'expiresAt')::TIMESTAMPTZ<>(p_child_grant->>'expiresAt')::TIMESTAMPTZ
     OR NOT agent_delegation_registry_subset(p_child_registry,v_parent.registry_json)
     OR p_child_snapshot->>'schemaVersion'<>'neo.agent-snapshot/v1'
     OR p_child_snapshot->>'rootRunId'<>p_parent_run_id
     OR p_child_snapshot->>'parentRunId'<>p_parent_run_id
     OR (p_child_snapshot->>'depth')::INTEGER<>1
     OR p_child_snapshot->'model'<>v_parent.model
     OR p_child_snapshot->>'packageFingerprint'<>v_parent.package_fingerprint
     OR p_child_snapshot->>'runtimeBundleFingerprint'<>v_parent.runtime_bundle_fingerprint
     OR p_child_snapshot->>'grantFingerprint'<>p_child_grant_fingerprint
     OR p_child_snapshot->>'registryFingerprint'<>p_child_registry_fingerprint
     OR p_child_snapshot->'budget'<>v_child_budget
     OR NOT agent_delegation_json_subset(p_child_grant,v_parent.grant_json) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SUBSET_VIOLATION';END IF;
  IF (v_parent.reserved_budget->>'maxWallSeconds')::BIGINT+(v_parent.consumed_budget->>'maxWallSeconds')::BIGINT+(v_child_budget->>'maxWallSeconds')::BIGINT>(v_parent.budget->>'maxWallSeconds')::BIGINT
     OR (v_parent.reserved_budget->>'maxModelTokens')::BIGINT+(v_parent.consumed_budget->>'maxModelTokens')::BIGINT+(v_child_budget->>'maxModelTokens')::BIGINT>(v_parent.budget->>'maxModelTokens')::BIGINT
     OR (v_parent.reserved_budget->>'maxToolCalls')::BIGINT+(v_parent.consumed_budget->>'maxToolCalls')::BIGINT+(v_child_budget->>'maxToolCalls')::BIGINT>(v_parent.budget->>'maxToolCalls')::BIGINT
     OR (v_parent.reserved_budget->>'maxArtifactBytes')::BIGINT+(v_parent.consumed_budget->>'maxArtifactBytes')::BIGINT+(v_child_budget->>'maxArtifactBytes')::BIGINT>(v_parent.budget->>'maxArtifactBytes')::BIGINT THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='BUDGET_EXCEEDED';END IF;
  PERFORM * FROM agent_orchestrator_enqueue_run(p_child_run_id,p_child_snapshot_id,p_user_id,p_idempotency_key,
    p_child_snapshot_fingerprint,p_request_fingerprint,p_child_snapshot,p_steps,p_scope_keys,p_event_ids);
  INSERT INTO agent_delegation_authorities(run_id,root_run_id,parent_run_id,depth,user_id,snapshot_id,snapshot_fingerprint,
    subject,model,package_fingerprint,runtime_bundle_fingerprint,grant_id,grant_fingerprint,grant_json,
    registry_fingerprint,registry_json,expires_at,budget)
  VALUES(p_child_run_id,p_parent_run_id,p_parent_run_id,1,p_user_id,p_child_snapshot_id,p_child_snapshot_fingerprint,
    v_parent.subject,v_parent.model,v_parent.package_fingerprint,v_parent.runtime_bundle_fingerprint,
    p_child_grant->>'grantId',p_child_grant_fingerprint,p_child_grant,p_child_registry_fingerprint,p_child_registry,
    (p_child_grant->>'expiresAt')::TIMESTAMPTZ,v_child_budget);
  INSERT INTO agent_delegation_lineage(child_run_id,parent_run_id,user_id,parent_step_id,parent_attempt_id,parent_generation,
    parent_lease_owner,parent_lease_token_hash,idempotency_key,request_fingerprint,reservation)
  VALUES(p_child_run_id,p_parent_run_id,p_user_id,p_parent_step_id,p_parent_attempt_id,p_parent_generation,
    p_parent_lease_owner,p_parent_lease_token_hash,p_idempotency_key,p_request_fingerprint,v_child_budget);
  UPDATE agent_delegation_authorities SET reserved_budget=jsonb_build_object(
    'maxWallSeconds',(reserved_budget->>'maxWallSeconds')::BIGINT+(v_child_budget->>'maxWallSeconds')::BIGINT,
    'maxModelTokens',(reserved_budget->>'maxModelTokens')::BIGINT+(v_child_budget->>'maxModelTokens')::BIGINT,
    'maxToolCalls',(reserved_budget->>'maxToolCalls')::BIGINT+(v_child_budget->>'maxToolCalls')::BIGINT,
    'maxArtifactBytes',(reserved_budget->>'maxArtifactBytes')::BIGINT+(v_child_budget->>'maxArtifactBytes')::BIGINT)
  WHERE agent_delegation_authorities.run_id=p_parent_run_id;
  RETURN QUERY SELECT p_child_run_id,true;
END
$function$;

CREATE FUNCTION agent_delegation_admit_launch(p_user_id UUID,p_run_id TEXT,p_attempt_id TEXT,p_generation BIGINT,
  p_lease_owner TEXT,p_lease_token_hash TEXT,p_snapshot_fingerprint TEXT,p_grant_fingerprint TEXT,p_registry_fingerprint TEXT,p_tools JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_child agent_delegation_authorities%ROWTYPE;v_lineage agent_delegation_lineage%ROWTYPE;v_parent agent_delegation_authorities%ROWTYPE;
  v_child_attempt agent_attempts%ROWTYPE;v_parent_attempt agent_attempts%ROWTYPE;v_now TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT * INTO v_child FROM agent_delegation_authorities WHERE run_id=p_run_id AND user_id=p_user_id FOR SHARE;
  SELECT * INTO v_lineage FROM agent_delegation_lineage WHERE child_run_id=p_run_id FOR SHARE;
  SELECT * INTO v_parent FROM agent_delegation_authorities WHERE run_id=v_lineage.parent_run_id FOR SHARE;
  SELECT * INTO v_child_attempt FROM agent_attempts WHERE id=p_attempt_id AND run_id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_parent_attempt FROM agent_attempts WHERE id=v_lineage.parent_attempt_id AND run_id=v_lineage.parent_run_id FOR SHARE;
  IF v_child.depth<>1 OR v_child.parent_run_id<>v_parent.run_id OR v_parent.depth<>0 OR v_child.state<>'active' OR v_parent.state<>'active'
     OR v_child.expires_at<=v_now OR v_parent.expires_at<=v_now
     OR v_child.snapshot_fingerprint<>p_snapshot_fingerprint OR v_child.grant_fingerprint<>p_grant_fingerprint
     OR v_child.registry_fingerprint<>p_registry_fingerprint OR p_tools<>(SELECT coalesce(jsonb_agg(tool->>'identity' ORDER BY tool->>'identity'),'[]'::jsonb) FROM jsonb_array_elements(v_child.registry_json->'tools') tool)
     OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(p_tools) tool WHERE tool.value IN ('delegate_task','cron_manage','grant_manage','secret_manage','runtime_manage')) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SNAPSHOT_MISMATCH';END IF;
  IF v_child_attempt.generation<>p_generation OR v_child_attempt.lease_owner<>p_lease_owner OR v_child_attempt.lease_token_hash<>p_lease_token_hash
     OR v_child_attempt.state NOT IN ('leased','starting') OR v_child_attempt.lease_expires_at<=v_now THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';END IF;
  IF v_parent_attempt.generation<>v_lineage.parent_generation OR v_parent_attempt.lease_owner<>v_lineage.parent_lease_owner
     OR v_parent_attempt.lease_token_hash<>v_lineage.parent_lease_token_hash OR v_parent_attempt.state NOT IN ('starting','running','prepared','committing')
     OR v_parent_attempt.lease_expires_at<=v_now THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PARENT_STALE';END IF;
  IF agent_orchestrator_active_kill_mode((SELECT scope_keys FROM agent_runs WHERE id=p_run_id)) IS NOT NULL
     OR agent_orchestrator_active_kill_mode((SELECT scope_keys FROM agent_runs WHERE id=v_parent.run_id)) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_delegation_settle(p_settlement_id TEXT,p_user_id UUID,p_child_run_id TEXT,p_usage JSONB,p_outcome TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_lineage agent_delegation_lineage%ROWTYPE;v_existing agent_delegation_settlements%ROWTYPE;v_run_state TEXT;
BEGIN
  SELECT * INTO v_lineage FROM agent_delegation_lineage WHERE child_run_id=p_child_run_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_DELEGATION_NOT_FOUND';END IF;
  SELECT * INTO v_existing FROM agent_delegation_settlements WHERE child_run_id=p_child_run_id;
  IF FOUND THEN IF v_existing.usage<>p_usage OR v_existing.outcome<>p_outcome THEN RAISE EXCEPTION USING ERRCODE='23505',MESSAGE='IDEMPOTENCY_CONFLICT';END IF;RETURN false;END IF;
  SELECT state INTO v_run_state FROM agent_runs WHERE id=p_child_run_id AND user_id=p_user_id FOR UPDATE;
  IF v_run_state IS NULL OR v_run_state<>p_outcome
     OR v_run_state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SETTLEMENT_INVALID';END IF;
  IF NOT agent_delegation_budget_valid(p_usage) OR (p_usage->>'maxWallSeconds')::BIGINT>(v_lineage.reservation->>'maxWallSeconds')::BIGINT
     OR (p_usage->>'maxModelTokens')::BIGINT>(v_lineage.reservation->>'maxModelTokens')::BIGINT
     OR (p_usage->>'maxToolCalls')::BIGINT>(v_lineage.reservation->>'maxToolCalls')::BIGINT
     OR (p_usage->>'maxArtifactBytes')::BIGINT>(v_lineage.reservation->>'maxArtifactBytes')::BIGINT THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='BUDGET_EXCEEDED';END IF;
  INSERT INTO agent_delegation_settlements VALUES(p_settlement_id,p_child_run_id,p_outcome,p_usage,clock_timestamp());
  UPDATE agent_delegation_lineage SET state=CASE p_outcome WHEN 'canceled' THEN 'canceled' WHEN 'killed' THEN 'killed' WHEN 'outcome_unknown' THEN 'outcome_unknown' ELSE 'settled' END,
    settled_usage=p_usage,settled_at=clock_timestamp() WHERE child_run_id=p_child_run_id;
  UPDATE agent_delegation_authorities SET state=CASE p_outcome WHEN 'canceled' THEN 'canceled' WHEN 'killed' THEN 'killed' WHEN 'outcome_unknown' THEN 'outcome_unknown' ELSE 'settled' END WHERE run_id=p_child_run_id;
  UPDATE agent_delegation_authorities SET reserved_budget=jsonb_build_object(
    'maxWallSeconds',(reserved_budget->>'maxWallSeconds')::BIGINT-(v_lineage.reservation->>'maxWallSeconds')::BIGINT,
    'maxModelTokens',(reserved_budget->>'maxModelTokens')::BIGINT-(v_lineage.reservation->>'maxModelTokens')::BIGINT,
    'maxToolCalls',(reserved_budget->>'maxToolCalls')::BIGINT-(v_lineage.reservation->>'maxToolCalls')::BIGINT,
    'maxArtifactBytes',(reserved_budget->>'maxArtifactBytes')::BIGINT-(v_lineage.reservation->>'maxArtifactBytes')::BIGINT),
    consumed_budget=jsonb_build_object(
    'maxWallSeconds',(consumed_budget->>'maxWallSeconds')::BIGINT+(p_usage->>'maxWallSeconds')::BIGINT,
    'maxModelTokens',(consumed_budget->>'maxModelTokens')::BIGINT+(p_usage->>'maxModelTokens')::BIGINT,
    'maxToolCalls',(consumed_budget->>'maxToolCalls')::BIGINT+(p_usage->>'maxToolCalls')::BIGINT,
    'maxArtifactBytes',(consumed_budget->>'maxArtifactBytes')::BIGINT+(p_usage->>'maxArtifactBytes')::BIGINT)
  WHERE run_id=v_lineage.parent_run_id;RETURN true;
END
$function$;

CREATE FUNCTION agent_delegation_cascade(p_user_id UUID,p_parent_run_id TEXT,p_mode TEXT,p_actor_type TEXT,p_actor_id TEXT,p_reason_code TEXT)
RETURNS SETOF agent_delegation_reaps LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE child RECORD;attempt agent_attempts%ROWTYPE;child_step RECORD;reap_id TEXT;
  now_at TIMESTAMPTZ:=clock_timestamp();terminal TEXT;child_run_state TEXT;
BEGIN
  IF p_mode NOT IN ('cancel','kill') THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CASCADE_INVALID';END IF;
  terminal:=CASE p_mode WHEN 'kill' THEN 'killed' ELSE 'canceled' END;
  PERFORM 1 FROM agent_delegation_authorities WHERE run_id=p_parent_run_id AND user_id=p_user_id AND depth=0 FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PARENT_INVALID';END IF;
  FOR child IN SELECT authority.run_id FROM agent_delegation_authorities authority WHERE authority.parent_run_id=p_parent_run_id AND authority.state='active' ORDER BY authority.run_id FOR UPDATE LOOP
    SELECT state INTO child_run_state FROM agent_runs WHERE id=child.run_id AND user_id=p_user_id FOR UPDATE;
    FOR attempt IN SELECT attempt_row.* FROM agent_attempts attempt_row
      WHERE attempt_row.run_id=child.run_id AND attempt_row.user_id=p_user_id
        AND attempt_row.state IN ('leased','starting','running','prepared','committing')
      ORDER BY attempt_row.step_id,attempt_row.generation FOR UPDATE LOOP
      reap_id:='reap_'||substr(md5(concat_ws('|',p_parent_run_id,child.run_id,attempt.id,attempt.generation::TEXT,p_mode)),1,32);
      UPDATE agent_attempts SET state=terminal,terminal_at=now_at,updated_at=now_at,lease_expires_at=LEAST(lease_expires_at,now_at) WHERE id=attempt.id;
      PERFORM agent_orchestrator_append_event('event_'||substr(md5(reap_id||'|attempt'),1,32),child.run_id,p_user_id,attempt.step_id,attempt.id,
        'attempt.transition','attempt',attempt.state,terminal,p_actor_type,p_actor_id,p_reason_code,attempt.generation,NULL,'{}',now_at);
      INSERT INTO agent_delegation_reaps(reap_id,parent_run_id,child_run_id,attempt_id,generation,lease_owner,mode)
      VALUES(reap_id,p_parent_run_id,child.run_id,attempt.id,attempt.generation,attempt.lease_owner,p_mode)
      ON CONFLICT(child_run_id,attempt_id,generation,mode) DO NOTHING;
    END LOOP;
    FOR child_step IN SELECT step.id,step.state FROM agent_steps step
      WHERE step.run_id=child.run_id AND step.user_id=p_user_id
        AND step.state NOT IN ('succeeded','failed','skipped','canceled','killed','outcome_unknown')
      ORDER BY step.ordinal FOR UPDATE LOOP
      UPDATE agent_steps SET state=terminal,terminal_at=now_at,updated_at=now_at WHERE id=child_step.id;
      PERFORM agent_orchestrator_append_event(
        'event_'||substr(md5(concat_ws('|',p_parent_run_id,child.run_id,child_step.id,p_mode,'step')),1,32),
        child.run_id,p_user_id,child_step.id,NULL,'step.transition','step',child_step.state,terminal,
        p_actor_type,p_actor_id,p_reason_code,NULL,NULL,'{}',now_at);
    END LOOP;
    IF child_run_state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown') THEN
      UPDATE agent_runs SET state=terminal,terminal_at=now_at,updated_at=now_at WHERE id=child.run_id;
      PERFORM agent_orchestrator_append_event(
        'event_'||substr(md5(concat_ws('|',p_parent_run_id,child.run_id,p_mode,'run')),1,32),
        child.run_id,p_user_id,NULL,NULL,'run.transition','run',child_run_state,terminal,
        p_actor_type,p_actor_id,p_reason_code,NULL,NULL,'{}',now_at);
    END IF;
    UPDATE agent_delegation_authorities SET state=terminal WHERE run_id=child.run_id;
  END LOOP;
  RETURN QUERY SELECT reap.* FROM agent_delegation_reaps reap WHERE reap.parent_run_id=p_parent_run_id AND reap.mode=p_mode AND reap.state IN ('pending','failed') ORDER BY reap.created_at,reap.reap_id;
END
$function$;

CREATE FUNCTION agent_delegation_reconcile(p_limit INTEGER)
RETURNS INTEGER LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE parent RECORD;recovered INTEGER:=0;now_at TIMESTAMPTZ:=clock_timestamp();
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_RECONCILE_INVALID';END IF;
  FOR parent IN
    WITH roots AS (
      SELECT authority.run_id,authority.user_id,authority.state AS authority_state,
        authority.expires_at,run.state AS run_state,run.scope_keys,
        agent_orchestrator_active_kill_mode(run.scope_keys) AS kill_mode,
        EXISTS(SELECT 1 FROM agent_delegation_authorities child
          JOIN agent_delegation_lineage lineage ON lineage.child_run_id=child.run_id
          JOIN agent_attempts parent_attempt ON parent_attempt.id=lineage.parent_attempt_id
          WHERE child.parent_run_id=authority.run_id AND child.state='active'
            AND (parent_attempt.generation<>lineage.parent_generation
              OR parent_attempt.lease_owner<>lineage.parent_lease_owner
              OR parent_attempt.lease_token_hash<>lineage.parent_lease_token_hash
              OR parent_attempt.state NOT IN ('starting','running','prepared','committing')
              OR parent_attempt.lease_expires_at<=now_at)) AS stale_attempt
      FROM agent_delegation_authorities authority
      JOIN agent_runs run ON run.id=authority.run_id AND run.user_id=authority.user_id
      WHERE authority.depth=0
        AND EXISTS(SELECT 1 FROM agent_delegation_authorities child
          WHERE child.parent_run_id=authority.run_id AND child.state='active')
    )
    SELECT roots.user_id,roots.run_id,
      CASE WHEN roots.kill_mode='kill' OR roots.run_state IN ('failed','killed','outcome_unknown')
          OR roots.authority_state IN ('killed','outcome_unknown') OR roots.expires_at<=now_at OR roots.stale_attempt
        THEN 'kill' ELSE 'cancel' END AS mode
    FROM roots
    WHERE roots.authority_state<>'active' OR roots.expires_at<=now_at OR roots.stale_attempt
      OR roots.kill_mode IS NOT NULL
      OR roots.run_state IN ('succeeded','failed','canceled','killed','outcome_unknown')
    ORDER BY roots.run_id LIMIT p_limit
  LOOP
    PERFORM * FROM agent_delegation_cascade(parent.user_id,parent.run_id,parent.mode,
      'orchestrator','delegation-reconcile','PARENT_AUTHORITY_STALE');
    recovered:=recovered+1;
  END LOOP;
  RETURN recovered;
END
$function$;

CREATE FUNCTION agent_delegation_complete_reap(p_reap_id TEXT,p_succeeded BOOLEAN,p_error_code TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN UPDATE agent_delegation_reaps SET state=CASE WHEN p_succeeded THEN 'reaped' ELSE 'failed' END,
  retry_count=retry_count+1,error_code=CASE WHEN p_succeeded THEN NULL ELSE p_error_code END,
  completed_at=CASE WHEN p_succeeded THEN clock_timestamp() ELSE NULL END WHERE reap_id=p_reap_id AND state<>'reaped';RETURN FOUND;END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema();function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'agent_delegation_budget_valid(jsonb)',
    'agent_delegation_json_subset(jsonb,jsonb)',
    'agent_delegation_registry_valid(jsonb,integer)',
    'agent_delegation_registry_subset(jsonb,jsonb)',
    'agent_delegation_immutable()',
    'agent_delegation_register_root(text,uuid,jsonb,jsonb,text,text,text,text,jsonb,text,jsonb,timestamp with time zone)',
    'agent_delegation_enqueue_child(text,text,uuid,text,text,text,jsonb,jsonb,text[],text[],text,text,text,bigint,text,text,text,jsonb,text,jsonb)',
    'agent_delegation_admit_launch(uuid,text,text,bigint,text,text,text,text,text,jsonb)',
    'agent_delegation_settle(text,uuid,text,jsonb,text)',
    'agent_delegation_cascade(uuid,text,text,text,text,text)',
    'agent_delegation_reconcile(integer)',
    'agent_delegation_complete_reap(text,boolean,text)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,function_identity,schema_name);
  END LOOP;
END
$harden$;

ALTER TABLE agent_delegation_authorities OWNER TO agent_delegation_owner;
ALTER TABLE agent_delegation_lineage OWNER TO agent_delegation_owner;
ALTER TABLE agent_delegation_settlements OWNER TO agent_delegation_owner;
ALTER TABLE agent_delegation_reaps OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_budget_valid(JSONB) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_json_subset(JSONB,JSONB) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_registry_valid(JSONB,INTEGER) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_registry_subset(JSONB,JSONB) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_immutable() OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_register_root(TEXT,UUID,JSONB,JSONB,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB,TIMESTAMPTZ) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_enqueue_child(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[],TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_admit_launch(UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_settle(TEXT,UUID,TEXT,JSONB,TEXT) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_cascade(UUID,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_reconcile(INTEGER) OWNER TO agent_delegation_owner;
ALTER FUNCTION agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT) OWNER TO agent_delegation_owner;

REVOKE ALL ON agent_delegation_authorities,agent_delegation_lineage,agent_delegation_settlements,agent_delegation_reaps FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control;
GRANT SELECT ON agent_delegation_authorities,agent_delegation_lineage,agent_delegation_settlements,agent_delegation_reaps TO agent_delegation_control;
REVOKE ALL ON FUNCTION agent_delegation_register_root(TEXT,UUID,JSONB,JSONB,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB,TIMESTAMPTZ),
  agent_delegation_enqueue_child(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[],TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB),
  agent_delegation_admit_launch(UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),agent_delegation_settle(TEXT,UUID,TEXT,JSONB,TEXT),
  agent_delegation_cascade(UUID,TEXT,TEXT,TEXT,TEXT,TEXT),agent_delegation_reconcile(INTEGER),agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control;
GRANT EXECUTE ON FUNCTION agent_delegation_register_root(TEXT,UUID,JSONB,JSONB,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB,TIMESTAMPTZ),
  agent_delegation_enqueue_child(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[],TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,JSONB,TEXT,JSONB),
  agent_delegation_admit_launch(UUID,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),agent_delegation_settle(TEXT,UUID,TEXT,JSONB,TEXT),
  agent_delegation_cascade(UUID,TEXT,TEXT,TEXT,TEXT,TEXT),agent_delegation_reconcile(INTEGER),agent_delegation_complete_reap(TEXT,BOOLEAN,TEXT)
  TO agent_delegation_control;

DO $schema_privileges$ BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_delegation_owner,agent_delegation_control,go_api_runtime',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_delegation_owner,agent_delegation_control',current_schema());
END $schema_privileges$;
GRANT SELECT,UPDATE ON agent_runs,agent_steps,agent_attempts TO agent_delegation_owner;
GRANT SELECT ON agent_run_snapshots,agent_kill_switch_state,agent_kill_switches TO agent_delegation_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_enqueue_run(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]),
  agent_orchestrator_active_kill_mode(TEXT[]),
  agent_orchestrator_append_event(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,JSONB,TIMESTAMPTZ)
  TO agent_delegation_owner;
