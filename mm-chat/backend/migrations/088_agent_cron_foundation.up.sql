-- G20.6 held Cron scheduling foundation. PostgreSQL is the only durable
-- template revision, approval, cursor, claim, occurrence, Run-link and audit
-- authority. This migration adds no HTTP route, startup worker or production
-- Scheduler/Runner wiring.

DO $roles$
DECLARE role_name TEXT;can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create FROM pg_roles WHERE rolname=current_user;
  FOREACH role_name IN ARRAY ARRAY['agent_cron_owner','agent_cron_control'] LOOP
    IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF NOT can_create THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_CRON_REQUIRED_ROLE_MISSING';END IF;
      EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',role_name);
    END IF;
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name AND
      (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_CRON_ROLE_MUST_BE_RESTRICTED';
    END IF;
  END LOOP;
  IF pg_has_role('agent_cron_control','agent_cron_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_cron_control','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_cron_control','MEMBER')
     OR pg_has_role('agent_runner_control','agent_cron_control','MEMBER')
     OR pg_has_role('agent_effect_control','agent_cron_control','MEMBER')
     OR pg_has_role('agent_delegation_control','agent_cron_control','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_CRON_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE FUNCTION agent_cron_document_sanitized(p_value JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE item JSONB;entry RECORD;normalized TEXT;
BEGIN
  IF jsonb_typeof(p_value)<>'object' THEN RETURN false;END IF;
  FOR item IN SELECT value FROM jsonb_path_query(p_value,'$.**') value LOOP
    IF jsonb_typeof(item)<>'object' THEN CONTINUE;END IF;
    FOR entry IN SELECT key FROM jsonb_each(item) LOOP
      normalized:=lower(regexp_replace(entry.key,'[_-]','','g'));
      IF normalized IN ('prompt','systemprompt','skillbody','workspacecontent','toolarguments','toolresults',
        'stdout','stderr','secretvalue','leasetoken','authorization','apikey','password','accesstoken',
        'refreshtoken','credential') OR normalized LIKE '%password%' OR normalized LIKE '%authorization%' THEN
        RETURN false;
      END IF;
    END LOOP;
  END LOOP;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_cron_spec_valid(p_spec JSONB,p_user_id UUID)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE capability JSONB;secret JSONB;step JSONB;egress_rule JSONB;scope_value TEXT;expected_scopes TEXT[];actual_scopes TEXT[];
  issued_at TIMESTAMPTZ;expires_at TIMESTAMPTZ;
BEGIN
  IF NOT agent_cron_document_sanitized(p_spec) OR octet_length(p_spec::text)>262144
     OR p_spec-ARRAY['schemaVersion','owner','input','schedule','model','budget','skill','grant','egress','secrets','automation','policies','steps','scopeKeys']<>'{}'::jsonb
     OR (p_spec->'owner')-ARRAY['userId','projectId','assistantId']<>'{}'::jsonb
     OR (p_spec->'input')-ARRAY['ref','fingerprint']<>'{}'::jsonb
     OR (p_spec->'schedule')-ARRAY['expression','timezone','calculator']<>'{}'::jsonb
     OR (p_spec->'model')-ARRAY['provider','modelId']<>'{}'::jsonb
     OR (p_spec->'budget')-ARRAY['maxWallSeconds','maxModelTokens','maxToolCalls','maxArtifactBytes']<>'{}'::jsonb
     OR (p_spec->'skill')-ARRAY['installationId','admissionId','packageFingerprint','runtimeBundleFingerprint']<>'{}'::jsonb
     OR (p_spec->'grant')-ARRAY['grantId','grantFingerprint','registryFingerprint','issuedAt','expiresAt','capabilities']<>'{}'::jsonb
     OR (p_spec->'egress')-ARRAY['mode','rules']<>'{}'::jsonb
     OR (p_spec->'automation')-ARRAY['class','approvalId']<>'{}'::jsonb
     OR (p_spec->'policies')-ARRAY['missed','catchupWindowSeconds','maxCatchupRuns','overlap','maxAttempts','retryBackoffSeconds']<>'{}'::jsonb
     OR p_spec->>'schemaVersion'<>'neo.cron-template/v1'
     OR p_spec#>>'{owner,userId}' IS DISTINCT FROM p_user_id::text
     OR p_spec#>>'{owner,projectId}'!~'^project_[a-z0-9]{8,64}$'
     OR p_spec#>>'{owner,assistantId}'!~'^assistant_[a-z0-9]{8,64}$'
     OR p_spec#>>'{input,ref}'!~'^input_ref_[a-z0-9]{16,64}$'
     OR p_spec#>>'{input,fingerprint}'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec#>>'{schedule,expression}' IS NULL OR octet_length(p_spec#>>'{schedule,expression}') NOT BETWEEN 9 AND 256
     OR cardinality(regexp_split_to_array(p_spec#>>'{schedule,expression}','[[:space:]]+'))<>5
     OR p_spec#>>'{schedule,expression}' LIKE 'TZ=%' OR p_spec#>>'{schedule,expression}' LIKE 'CRON_TZ=%'
     OR p_spec#>>'{schedule,calculator}'<>'robfig-cron/v3.0.1+go-tzdata'
     OR p_spec#>>'{schedule,timezone}' IS NULL OR octet_length(p_spec#>>'{schedule,timezone}') NOT BETWEEN 1 AND 128
     OR p_spec#>>'{model,provider}'!~'^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$'
     OR octet_length(p_spec#>>'{model,modelId}') NOT BETWEEN 1 AND 128
     OR p_spec#>>'{skill,installationId}'!~'^[0-9a-f-]{36}$'
     OR p_spec#>>'{skill,admissionId}'!~'^[0-9a-f-]{36}$'
     OR p_spec#>>'{skill,packageFingerprint}'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec#>>'{skill,runtimeBundleFingerprint}'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec#>>'{grant,grantId}'!~'^grant_[a-z0-9]{16,64}$'
     OR p_spec#>>'{grant,grantFingerprint}'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec#>>'{grant,registryFingerprint}'!~'^sha256:[a-f0-9]{64}$'
     OR jsonb_typeof(p_spec#>'{grant,capabilities}')<>'array'
     OR jsonb_array_length(p_spec#>'{grant,capabilities}') NOT BETWEEN 1 AND 128
     OR jsonb_typeof(p_spec->'secrets')<>'array' OR jsonb_array_length(p_spec->'secrets')>25
     OR jsonb_typeof(p_spec->'steps')<>'array' OR jsonb_array_length(p_spec->'steps') NOT BETWEEN 1 AND 32
     OR jsonb_typeof(p_spec->'scopeKeys')<>'array'
     OR p_spec#>>'{automation,approvalId}'!~'^approval_[a-z0-9]{16,64}$'
     OR p_spec#>>'{automation,class}' NOT IN ('automation_read_only','automation_brokered_effect')
     OR p_spec#>>'{policies,missed}' NOT IN ('skip','fire_once','catch_up')
     OR p_spec#>>'{policies,overlap}' NOT IN ('skip','buffer_one','allow')
     OR (p_spec#>>'{policies,catchupWindowSeconds}')::integer NOT BETWEEN 10 AND 86400
     OR (p_spec#>>'{policies,maxCatchupRuns}')::integer NOT BETWEEN 1 AND 100
     OR (p_spec#>>'{policies,maxAttempts}')::integer NOT BETWEEN 1 AND 10
     OR (p_spec#>>'{policies,retryBackoffSeconds}')::integer NOT BETWEEN 1 AND 3600
     OR (p_spec#>>'{budget,maxWallSeconds}')::bigint NOT BETWEEN 0 AND 86400
     OR (p_spec#>>'{budget,maxModelTokens}')::bigint NOT BETWEEN 0 AND 10000000
     OR (p_spec#>>'{budget,maxToolCalls}')::bigint NOT BETWEEN 0 AND 10000
     OR (p_spec#>>'{budget,maxArtifactBytes}')::bigint NOT BETWEEN 0 AND 10737418240
     OR p_spec#>>'{egress,mode}' NOT IN ('none','allowlist','brokered')
     OR jsonb_typeof(p_spec#>'{egress,rules}')<>'array'
     OR jsonb_array_length(p_spec#>'{egress,rules}')>64 THEN RETURN false;END IF;
  issued_at:=(p_spec#>>'{grant,issuedAt}')::timestamptz;expires_at:=(p_spec#>>'{grant,expiresAt}')::timestamptz;
  IF issued_at>=expires_at THEN RETURN false;END IF;
  FOR capability IN SELECT value FROM jsonb_array_elements(p_spec#>'{grant,capabilities}') LOOP
    IF capability-ARRAY['capability','actions','resources','approval','maxCalls']<>'{}'::jsonb
       OR (capability->'resources')-ARRAY['kind','values']<>'{}'::jsonb
       OR capability->>'capability'!~'^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$'
       OR capability->>'approval' NOT IN ('automatic','once','per_commit','denied')
       OR (capability->>'maxCalls')::integer NOT BETWEEN 0 AND 10000
       OR jsonb_typeof(capability->'actions')<>'array' OR jsonb_array_length(capability->'actions') NOT BETWEEN 1 AND 32
       OR capability#>>'{resources,kind}' NOT IN ('exact','prefix')
       OR jsonb_typeof(capability#>'{resources,values}')<>'array'
       OR jsonb_array_length(capability#>'{resources,values}') NOT BETWEEN 1 AND 128
       OR (p_spec#>>'{automation,class}'='automation_read_only' AND capability->>'approval'<>'automatic') THEN RETURN false;END IF;
  END LOOP;
  FOR secret IN SELECT value FROM jsonb_array_elements(p_spec->'secrets') LOOP
    IF secret-ARRAY['slot','brokerRef','actions','ttlSeconds']<>'{}'::jsonb
       OR secret->>'slot'!~'^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$'
       OR secret->>'brokerRef'!~'^secret_ref_[a-z0-9]{16,64}$'
       OR (secret->>'ttlSeconds')::integer NOT BETWEEN 1 AND 3600
       OR jsonb_typeof(secret->'actions')<>'array' OR jsonb_array_length(secret->'actions') NOT BETWEEN 1 AND 32 THEN RETURN false;END IF;
  END LOOP;
  FOR egress_rule IN SELECT value FROM jsonb_array_elements(p_spec#>'{egress,rules}') LOOP
    IF egress_rule-ARRAY['scheme','host','ports']<>'{}'::jsonb
       OR egress_rule->>'scheme' NOT IN ('https','wss')
       OR octet_length(egress_rule->>'host') NOT BETWEEN 1 AND 253
       OR jsonb_typeof(egress_rule->'ports')<>'array' OR jsonb_array_length(egress_rule->'ports') NOT BETWEEN 1 AND 8
       OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(egress_rule->'ports') port WHERE port::integer NOT BETWEEN 1 AND 65535) THEN RETURN false;END IF;
  END LOOP;
  FOR step IN SELECT value FROM jsonb_array_elements(p_spec->'steps') LOOP
    IF step-'kind'<>'{}'::jsonb OR step->>'kind'!~'^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$' OR octet_length(step->>'kind')>64 THEN RETURN false;END IF;
  END LOOP;
  SELECT array_agg(value ORDER BY value) INTO actual_scopes FROM jsonb_array_elements_text(p_spec->'scopeKeys');
  expected_scopes:=ARRAY['global:*','scheduler:*','user:'||p_user_id::text,
    'project:'||(p_spec#>>'{owner,projectId}'),'skill:'||(p_spec#>>'{skill,packageFingerprint}'),
    'admission:'||(p_spec#>>'{skill,admissionId}')];
  FOR secret IN SELECT value FROM jsonb_array_elements(p_spec->'secrets') LOOP expected_scopes:=array_append(expected_scopes,'secret:'||(secret->>'brokerRef'));END LOOP;
  SELECT array_agg(value ORDER BY value) INTO expected_scopes FROM unnest(expected_scopes) value;
  IF actual_scopes IS DISTINCT FROM expected_scopes OR cardinality(actual_scopes)<>cardinality(ARRAY(SELECT DISTINCT unnest(actual_scopes)))
     OR cardinality(actual_scopes)+1>32 THEN RETURN false;END IF;
  RETURN true;
EXCEPTION WHEN others THEN RETURN false;
END
$function$;

CREATE FUNCTION agent_cron_audit_sanitized(p_detail JSONB)
RETURNS BOOLEAN LANGUAGE sql IMMUTABLE SET search_path FROM CURRENT AS $function$
  SELECT jsonb_typeof(p_detail)='object' AND octet_length(p_detail::text)<=4096
    AND p_detail-ARRAY['revision','fingerprint','fromState','toState','missedCount','countTruncated',
      'runId','created','retryCount','claimGeneration']::text[]='{}'::jsonb
    AND NOT EXISTS(SELECT 1 FROM jsonb_each(p_detail) entry
      WHERE jsonb_typeof(entry.value) NOT IN ('null','boolean','number','string')
        OR (jsonb_typeof(entry.value)='number' AND (entry.value#>>'{}')::numeric<0)
        OR (entry.key='fingerprint' AND (entry.value#>>'{}')!~'^sha256:[a-f0-9]{64}$')
        OR (jsonb_typeof(entry.value)='string' AND entry.key<>'fingerprint'
          AND (entry.value#>>'{}')!~'^[A-Za-z][A-Za-z0-9_.:-]{0,127}$'))
$function$;

CREATE TABLE agent_cron_templates(
  id TEXT PRIMARY KEY,user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  current_revision BIGINT NOT NULL,current_revision_fingerprint TEXT NOT NULL,state TEXT NOT NULL,
  next_trigger_at TIMESTAMPTZ,cursor_claim_generation BIGINT NOT NULL DEFAULT 0,
  cursor_claim_owner TEXT,cursor_claim_expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  deleted_at TIMESTAMPTZ,UNIQUE(id,user_id),
  CONSTRAINT agent_cron_template_id_check CHECK(id~'^cron_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_cron_template_revision_check CHECK(current_revision>=1 AND current_revision_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_cron_template_state_check CHECK(state IN ('active','paused','deleted')),
  CONSTRAINT agent_cron_template_claim_check CHECK(cursor_claim_generation>=0 AND
    ((cursor_claim_owner IS NULL AND cursor_claim_expires_at IS NULL) OR
     (octet_length(cursor_claim_owner) BETWEEN 1 AND 128 AND cursor_claim_expires_at IS NOT NULL))),
  CONSTRAINT agent_cron_template_shape CHECK((state='active' AND next_trigger_at IS NOT NULL AND deleted_at IS NULL)
    OR (state='paused' AND next_trigger_at IS NOT NULL AND deleted_at IS NULL)
    OR (state='deleted' AND next_trigger_at IS NULL AND deleted_at IS NOT NULL)),
  CONSTRAINT agent_cron_template_times CHECK(updated_at>=created_at AND (deleted_at IS NULL OR deleted_at>=created_at))
);
CREATE INDEX idx_agent_cron_due ON agent_cron_templates(next_trigger_at,id) WHERE state='active';

CREATE TABLE agent_cron_revisions(
  template_id TEXT NOT NULL,user_id UUID NOT NULL,revision BIGINT NOT NULL,revision_fingerprint TEXT NOT NULL,
  spec JSONB NOT NULL,approval_id TEXT NOT NULL,automation_class TEXT NOT NULL,expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(template_id,revision),
  UNIQUE(template_id,revision,revision_fingerprint),UNIQUE(approval_id),
  CONSTRAINT agent_cron_revision_template_fk FOREIGN KEY(template_id,user_id) REFERENCES agent_cron_templates(id,user_id) ON DELETE CASCADE,
  CONSTRAINT agent_cron_revision_number CHECK(revision>=1),
  CONSTRAINT agent_cron_revision_fingerprint CHECK(revision_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_cron_revision_spec CHECK(agent_cron_spec_valid(spec,user_id)),
  CONSTRAINT agent_cron_revision_binding CHECK(approval_id=spec#>>'{automation,approvalId}'
    AND automation_class=spec#>>'{automation,class}' AND expires_at=(spec#>>'{grant,expiresAt}')::timestamptz)
);

CREATE TABLE agent_cron_approvals(
  approval_id TEXT PRIMARY KEY,template_id TEXT NOT NULL,revision BIGINT NOT NULL,revision_fingerprint TEXT NOT NULL,
  automation_class TEXT NOT NULL,actor_type TEXT NOT NULL,actor_id TEXT NOT NULL,reason_code TEXT NOT NULL,
  approved_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_cron_approval_revision_fk FOREIGN KEY(template_id,revision,revision_fingerprint)
    REFERENCES agent_cron_revisions(template_id,revision,revision_fingerprint) ON DELETE CASCADE,
  CONSTRAINT agent_cron_approval_id_check CHECK(approval_id~'^approval_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_cron_approval_class CHECK(automation_class IN ('automation_read_only','automation_brokered_effect')),
  CONSTRAINT agent_cron_approval_actor CHECK(actor_type IN ('user','operator') AND octet_length(actor_id) BETWEEN 1 AND 128),
  CONSTRAINT agent_cron_approval_reason CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$')
);

CREATE TABLE agent_cron_approval_revocations(
  revocation_id TEXT PRIMARY KEY,approval_id TEXT NOT NULL UNIQUE REFERENCES agent_cron_approvals(approval_id) ON DELETE CASCADE,
  actor_type TEXT NOT NULL,actor_id TEXT NOT NULL,reason_code TEXT NOT NULL,revoked_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_cron_revocation_id_check CHECK(revocation_id~'^cron_revocation_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_cron_revocation_actor CHECK(actor_type IN ('user','operator') AND octet_length(actor_id) BETWEEN 1 AND 128),
  CONSTRAINT agent_cron_revocation_reason CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$')
);

CREATE TABLE agent_cron_triggers(
  id TEXT PRIMARY KEY,template_id TEXT NOT NULL,user_id UUID NOT NULL,revision BIGINT NOT NULL,revision_fingerprint TEXT NOT NULL,
  scheduled_for TIMESTAMPTZ NOT NULL,occurrence_fingerprint TEXT NOT NULL,state TEXT NOT NULL,reason_code TEXT NOT NULL,
  run_id TEXT REFERENCES agent_runs(id) ON DELETE SET NULL,retry_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),claim_generation BIGINT NOT NULL DEFAULT 0,
  claim_owner TEXT,claim_expires_at TIMESTAMPTZ,created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),terminal_at TIMESTAMPTZ,
  UNIQUE(template_id,revision,scheduled_for),UNIQUE(template_id,revision,occurrence_fingerprint),
  CONSTRAINT agent_cron_trigger_revision_fk FOREIGN KEY(template_id,revision,revision_fingerprint)
    REFERENCES agent_cron_revisions(template_id,revision,revision_fingerprint) ON DELETE CASCADE,
  CONSTRAINT agent_cron_trigger_id_check CHECK(id~'^cron_trigger_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_cron_trigger_occurrence CHECK(occurrence_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_cron_trigger_state CHECK(state IN ('pending','claimed','enqueued','skipped','failed')),
  CONSTRAINT agent_cron_trigger_reason CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_cron_trigger_claim CHECK(claim_generation>=0 AND retry_count>=0 AND
    ((state='claimed' AND octet_length(claim_owner) BETWEEN 1 AND 128 AND claim_expires_at IS NOT NULL)
     OR (state<>'claimed' AND claim_owner IS NULL AND claim_expires_at IS NULL))),
  CONSTRAINT agent_cron_trigger_shape CHECK((state='enqueued' AND run_id IS NOT NULL AND terminal_at IS NOT NULL)
    OR (state IN ('skipped','failed') AND run_id IS NULL AND terminal_at IS NOT NULL)
    OR (state IN ('pending','claimed') AND run_id IS NULL AND terminal_at IS NULL)),
  CONSTRAINT agent_cron_trigger_times CHECK(updated_at>=created_at AND (terminal_at IS NULL OR terminal_at>=created_at))
);
CREATE INDEX idx_agent_cron_trigger_claim ON agent_cron_triggers(next_attempt_at,scheduled_for,id)
  WHERE state IN ('pending','claimed');

CREATE TABLE agent_cron_audit_events(
  event_id TEXT PRIMARY KEY,template_id TEXT NOT NULL,user_id UUID NOT NULL,revision BIGINT NOT NULL,
  trigger_id TEXT REFERENCES agent_cron_triggers(id) ON DELETE SET NULL,kind TEXT NOT NULL,actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,reason_code TEXT NOT NULL,detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_cron_audit_revision_fk FOREIGN KEY(template_id,revision) REFERENCES agent_cron_revisions(template_id,revision) ON DELETE CASCADE,
  CONSTRAINT agent_cron_audit_id_check CHECK(event_id~'^cron_event_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_cron_audit_kind CHECK(kind IN ('revision.approved','approval.revoked','lifecycle.changed','cursor.advanced',
    'trigger.materialized','trigger.claimed','trigger.enqueued','trigger.skipped','trigger.retry','trigger.failed','claim.reclaimed')),
  CONSTRAINT agent_cron_audit_actor CHECK(actor_type IN ('user','operator','scheduler') AND octet_length(actor_id) BETWEEN 1 AND 128),
  CONSTRAINT agent_cron_audit_reason CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_cron_audit_detail CHECK(agent_cron_audit_sanitized(detail))
);
CREATE INDEX idx_agent_cron_audit_history ON agent_cron_audit_events(template_id,occurred_at,event_id);

CREATE FUNCTION agent_cron_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN IF TG_OP='DELETE' AND (pg_trigger_depth()>1 OR current_user='agent_cron_owner') THEN RETURN OLD;END IF;
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_CRON_IMMUTABLE';END
$function$;
CREATE TRIGGER trg_agent_cron_revision_immutable BEFORE UPDATE OR DELETE ON agent_cron_revisions FOR EACH ROW EXECUTE FUNCTION agent_cron_immutable();
CREATE TRIGGER trg_agent_cron_approval_immutable BEFORE UPDATE OR DELETE ON agent_cron_approvals FOR EACH ROW EXECUTE FUNCTION agent_cron_immutable();
CREATE TRIGGER trg_agent_cron_revocation_immutable BEFORE UPDATE OR DELETE ON agent_cron_approval_revocations FOR EACH ROW EXECUTE FUNCTION agent_cron_immutable();
CREATE TRIGGER trg_agent_cron_audit_immutable BEFORE UPDATE OR DELETE ON agent_cron_audit_events FOR EACH ROW EXECUTE FUNCTION agent_cron_immutable();

CREATE FUNCTION agent_cron_append_audit(p_event_id TEXT,p_template_id TEXT,p_user_id UUID,p_revision BIGINT,
  p_trigger_id TEXT,p_kind TEXT,p_actor_type TEXT,p_actor_id TEXT,p_reason_code TEXT,p_detail JSONB,p_occurred_at TIMESTAMPTZ)
RETURNS VOID LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN INSERT INTO agent_cron_audit_events(event_id,template_id,user_id,revision,trigger_id,kind,actor_type,actor_id,reason_code,detail,occurred_at)
  VALUES(p_event_id,p_template_id,p_user_id,p_revision,p_trigger_id,p_kind,p_actor_type,p_actor_id,p_reason_code,COALESCE(p_detail,'{}'::jsonb),p_occurred_at);
END
$function$;

CREATE FUNCTION agent_cron_denial_reason(p_template_id TEXT,p_revision BIGINT)
RETURNS TEXT LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE template agent_cron_templates%ROWTYPE;revision agent_cron_revisions%ROWTYPE;scopes TEXT[];
BEGIN
  SELECT * INTO template FROM agent_cron_templates WHERE id=p_template_id;
  SELECT * INTO revision FROM agent_cron_revisions WHERE template_id=p_template_id AND agent_cron_revisions.revision=p_revision;
  IF template.id IS NULL OR revision.template_id IS NULL OR template.state<>'active' OR template.current_revision<>p_revision
     OR template.current_revision_fingerprint<>revision.revision_fingerprint THEN RETURN 'STALE_TEMPLATE';END IF;
  IF NOT EXISTS(SELECT 1 FROM users WHERE id=template.user_id AND deleted_at IS NULL) THEN RETURN 'OWNER_REVOKED';END IF;
  IF NOT EXISTS(SELECT 1 FROM skill_installations installation
      JOIN skill_package_candidates candidate ON candidate.id=installation.admission_id AND candidate.package_fingerprint=installation.package_fingerprint
      JOIN skill_package_versions package ON package.package_fingerprint=installation.package_fingerprint
      WHERE installation.id=(revision.spec#>>'{skill,installationId}')::uuid AND installation.user_id=template.user_id
        AND installation.admission_id=(revision.spec#>>'{skill,admissionId}')::uuid
        AND installation.package_fingerprint=revision.spec#>>'{skill,packageFingerprint}' AND candidate.status='admitted'
        AND package.runtime_bundle_fingerprint=revision.spec#>>'{skill,runtimeBundleFingerprint}') THEN RETURN 'SKILL_REVOKED';END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_grant_revocations revocation WHERE revocation.grant_id=revision.spec#>>'{grant,grantId}') THEN RETURN 'GRANT_REVOKED';END IF;
  IF EXISTS(SELECT 1 FROM agent_cron_approval_revocations revocation WHERE revocation.approval_id=revision.approval_id) THEN RETURN 'APPROVAL_REVOKED';END IF;
  IF revision.expires_at<=clock_timestamp() THEN RETURN 'TEMPLATE_EXPIRED';END IF;
  SELECT array_agg(value) INTO scopes FROM jsonb_array_elements_text(revision.spec->'scopeKeys');
  IF EXISTS(WITH latest AS(SELECT DISTINCT ON(entry.switch_id) entry.scope_key,entry.active FROM agent_kill_switches entry ORDER BY entry.switch_id,entry.revision DESC)
    SELECT 1 FROM latest WHERE active AND scope_key LIKE 'secret:%' AND scope_key=ANY(scopes)) THEN RETURN 'SECRET_REVOKED';END IF;
  IF agent_orchestrator_active_kill_mode(scopes) IS NOT NULL THEN RETURN 'KILL_SWITCH_ACTIVE';END IF;
  RETURN NULL;
END
$function$;

CREATE FUNCTION agent_cron_create_revision(p_template_id TEXT,p_user_id UUID,p_expected_revision BIGINT,
  p_revision_fingerprint TEXT,p_spec JSONB,p_next_trigger_at TIMESTAMPTZ,p_actor_type TEXT,p_actor_id TEXT,
  p_reason_code TEXT,p_audit_event_id TEXT)
RETURNS TABLE(template_id TEXT,current_revision BIGINT,state TEXT,next_trigger_at TIMESTAMPTZ,created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE template agent_cron_templates%ROWTYPE;new_revision BIGINT;now_at TIMESTAMPTZ:=clock_timestamp();denial TEXT;
BEGIN
  IF p_expected_revision<0 OR p_revision_fingerprint!~'^sha256:[a-f0-9]{64}$' OR NOT agent_cron_spec_valid(p_spec,p_user_id)
     OR p_next_trigger_at<=now_at OR p_actor_type NOT IN ('user','operator') OR octet_length(p_actor_id) NOT BETWEEN 1 AND 128
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' OR p_audit_event_id!~'^cron_event_[a-z0-9]{16,64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_REVISION_INVALID';END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_user_id::text||E'\\x00'||p_template_id,0));
  SELECT * INTO template FROM agent_cron_templates WHERE id=p_template_id FOR UPDATE;
  IF FOUND AND template.user_id<>p_user_id THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_CRON_NOT_FOUND';END IF;
  IF FOUND AND template.current_revision=p_expected_revision+1 AND template.current_revision_fingerprint=p_revision_fingerprint THEN
    RETURN QUERY SELECT template.id,template.current_revision,template.state,template.next_trigger_at,false;RETURN;
  END IF;
  IF (NOT FOUND AND p_expected_revision<>0) OR (FOUND AND (template.state='deleted' OR template.current_revision<>p_expected_revision)) THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';END IF;
  IF NOT EXISTS(SELECT 1 FROM users WHERE id=p_user_id AND deleted_at IS NULL) THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='OWNER_REVOKED';END IF;
  new_revision:=p_expected_revision+1;
  IF NOT FOUND THEN
    INSERT INTO agent_cron_templates(id,user_id,current_revision,current_revision_fingerprint,state,next_trigger_at,created_at,updated_at)
    VALUES(p_template_id,p_user_id,new_revision,p_revision_fingerprint,'active',p_next_trigger_at,now_at,now_at);
  END IF;
  INSERT INTO agent_cron_revisions(template_id,user_id,revision,revision_fingerprint,spec,approval_id,automation_class,expires_at,created_at)
  VALUES(p_template_id,p_user_id,new_revision,p_revision_fingerprint,p_spec,p_spec#>>'{automation,approvalId}',
    p_spec#>>'{automation,class}',(p_spec#>>'{grant,expiresAt}')::timestamptz,now_at);
  INSERT INTO agent_cron_approvals(approval_id,template_id,revision,revision_fingerprint,automation_class,actor_type,actor_id,reason_code,approved_at)
  VALUES(p_spec#>>'{automation,approvalId}',p_template_id,new_revision,p_revision_fingerprint,p_spec#>>'{automation,class}',p_actor_type,p_actor_id,p_reason_code,now_at);
  IF p_expected_revision>0 THEN UPDATE agent_cron_templates SET current_revision=new_revision,current_revision_fingerprint=p_revision_fingerprint,
    next_trigger_at=p_next_trigger_at,cursor_claim_owner=NULL,cursor_claim_expires_at=NULL,updated_at=now_at WHERE id=p_template_id;END IF;
  denial:=agent_cron_denial_reason(p_template_id,new_revision);
  IF denial IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE=denial;END IF;
  PERFORM agent_cron_append_audit(p_audit_event_id,p_template_id,p_user_id,new_revision,NULL,'revision.approved',p_actor_type,p_actor_id,
    'REVISION_APPROVED',jsonb_build_object('revision',new_revision,'fingerprint',p_revision_fingerprint),now_at);
  SELECT * INTO template FROM agent_cron_templates WHERE id=p_template_id;
  RETURN QUERY SELECT template.id,template.current_revision,template.state,template.next_trigger_at,true;
END
$function$;

CREATE FUNCTION agent_cron_set_lifecycle(p_template_id TEXT,p_user_id UUID,p_expected_revision BIGINT,p_to_state TEXT,
  p_next_trigger_at TIMESTAMPTZ,p_missed_count INTEGER,p_actor_type TEXT,p_actor_id TEXT,p_reason_code TEXT,p_audit_event_id TEXT)
RETURNS agent_cron_templates LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE template agent_cron_templates%ROWTYPE;from_state TEXT;now_at TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT * INTO template FROM agent_cron_templates WHERE id=p_template_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_NOT_FOUND';END IF;
  IF template.current_revision<>p_expected_revision THEN RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';END IF;
  IF p_to_state NOT IN ('active','paused','deleted') OR p_actor_type NOT IN ('user','operator','scheduler')
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' OR p_missed_count<0 THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_LIFECYCLE_INVALID';END IF;
  IF template.state=p_to_state THEN RETURN template;END IF;
  IF NOT ((template.state='active' AND p_to_state IN ('paused','deleted')) OR (template.state='paused' AND p_to_state IN ('active','deleted'))) THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_LIFECYCLE_INVALID';END IF;
  IF p_to_state='active' AND (p_next_trigger_at IS NULL OR p_next_trigger_at<=now_at) THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_RESUME_CURSOR_INVALID';END IF;
  from_state:=template.state;
  UPDATE agent_cron_templates SET state=p_to_state,next_trigger_at=CASE WHEN p_to_state='deleted' THEN NULL WHEN p_to_state='active' THEN p_next_trigger_at ELSE next_trigger_at END,
    cursor_claim_owner=NULL,cursor_claim_expires_at=NULL,deleted_at=CASE WHEN p_to_state='deleted' THEN now_at ELSE NULL END,updated_at=now_at WHERE id=p_template_id RETURNING * INTO template;
  PERFORM agent_cron_append_audit(p_audit_event_id,p_template_id,p_user_id,p_expected_revision,NULL,'lifecycle.changed',p_actor_type,p_actor_id,p_reason_code,
    jsonb_build_object('fromState',from_state,'toState',p_to_state,'missedCount',p_missed_count),now_at);
  RETURN template;
END
$function$;

CREATE FUNCTION agent_cron_revoke_approval(p_revocation_id TEXT,p_user_id UUID,p_approval_id TEXT,p_actor_type TEXT,
  p_actor_id TEXT,p_reason_code TEXT,p_audit_event_id TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE approval agent_cron_approvals%ROWTYPE;now_at TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT item.* INTO approval FROM agent_cron_approvals item JOIN agent_cron_templates template ON template.id=item.template_id
    WHERE item.approval_id=p_approval_id AND template.user_id=p_user_id FOR SHARE OF item;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_NOT_FOUND';END IF;
  IF EXISTS(SELECT 1 FROM agent_cron_approval_revocations WHERE approval_id=p_approval_id) THEN RETURN false;END IF;
  INSERT INTO agent_cron_approval_revocations VALUES(p_revocation_id,p_approval_id,p_actor_type,p_actor_id,p_reason_code,now_at);
  PERFORM agent_cron_append_audit(p_audit_event_id,approval.template_id,p_user_id,approval.revision,NULL,'approval.revoked',p_actor_type,p_actor_id,p_reason_code,'{}',now_at);
  RETURN true;
END
$function$;

CREATE FUNCTION agent_cron_claim_due(p_claim_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER)
RETURNS TABLE(template_id TEXT,user_id UUID,revision BIGINT,revision_fingerprint TEXT,next_trigger_at TIMESTAMPTZ,
  claim_generation BIGINT,claim_owner TEXT,claim_expires_at TIMESTAMPTZ,spec JSONB)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_claim_owner) NOT BETWEEN 1 AND 128 OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_CLAIM_INVALID';END IF;
  RETURN QUERY WITH candidates AS(
    SELECT template.id FROM agent_cron_templates template WHERE template.state='active' AND template.next_trigger_at<=p_now
      AND (template.cursor_claim_owner IS NULL OR template.cursor_claim_expires_at<=p_now)
      ORDER BY template.next_trigger_at,template.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ),claimed AS(
    UPDATE agent_cron_templates template SET cursor_claim_generation=template.cursor_claim_generation+1,
      cursor_claim_owner=p_claim_owner,cursor_claim_expires_at=p_now+make_interval(secs=>p_lease_seconds),updated_at=GREATEST(template.updated_at,p_now)
    FROM candidates WHERE template.id=candidates.id RETURNING template.*
  ) SELECT claimed.id,claimed.user_id,claimed.current_revision,claimed.current_revision_fingerprint,claimed.next_trigger_at,
      claimed.cursor_claim_generation,claimed.cursor_claim_owner,claimed.cursor_claim_expires_at,revision.spec
    FROM claimed JOIN agent_cron_revisions revision ON revision.template_id=claimed.id AND revision.revision=claimed.current_revision
    ORDER BY claimed.next_trigger_at,claimed.id;
END
$function$;

CREATE FUNCTION agent_cron_advance_cursor(p_template_id TEXT,p_revision BIGINT,p_revision_fingerprint TEXT,p_claim_owner TEXT,
  p_claim_generation BIGINT,p_expected_cursor TIMESTAMPTZ,p_observed_at TIMESTAMPTZ,p_next_trigger_at TIMESTAMPTZ,p_decisions JSONB)
RETURNS SETOF agent_cron_triggers LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE template agent_cron_templates%ROWTYPE;revision agent_cron_revisions%ROWTYPE;decision JSONB;decision_state TEXT;decision_reason TEXT;
  scheduled TIMESTAMPTZ;now_at TIMESTAMPTZ:=clock_timestamp();overlap_policy TEXT;has_buffer BOOLEAN;
BEGIN
  SELECT * INTO template FROM agent_cron_templates WHERE id=p_template_id FOR UPDATE;
  SELECT * INTO revision FROM agent_cron_revisions WHERE template_id=p_template_id AND agent_cron_revisions.revision=p_revision;
  IF template.id IS NULL OR revision.template_id IS NULL OR template.state<>'active' OR template.current_revision<>p_revision
     OR template.current_revision_fingerprint<>p_revision_fingerprint OR revision.revision_fingerprint<>p_revision_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_TEMPLATE';END IF;
  IF template.cursor_claim_owner IS DISTINCT FROM p_claim_owner OR template.cursor_claim_generation<>p_claim_generation
     OR template.cursor_claim_expires_at<=now_at OR template.next_trigger_at<>p_expected_cursor THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';END IF;
  IF jsonb_typeof(p_decisions)<>'array' OR jsonb_array_length(p_decisions)>102 OR p_next_trigger_at<=p_observed_at OR p_expected_cursor>p_observed_at THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_ADVANCE_INVALID';END IF;
  overlap_policy:=revision.spec#>>'{policies,overlap}';
  SELECT EXISTS(SELECT 1 FROM agent_cron_triggers trigger WHERE trigger.template_id=p_template_id AND trigger.state IN ('pending','claimed')) INTO has_buffer;
  FOR decision IN SELECT value FROM jsonb_array_elements(p_decisions) LOOP
    scheduled:=(decision->>'scheduledFor')::timestamptz;decision_state:=decision->>'state';decision_reason:=decision->>'reasonCode';
    IF decision->>'triggerId'!~'^cron_trigger_[a-z0-9]{16,64}$' OR decision->>'auditEventId'!~'^cron_event_[a-z0-9]{16,64}$'
       OR decision->>'occurrenceFingerprint'!~'^sha256:[a-f0-9]{64}$' OR decision_state NOT IN ('pending','skipped')
       OR decision_reason!~'^[A-Z][A-Z0-9_]{0,63}$' OR scheduled<p_expected_cursor OR scheduled>p_observed_at
       OR (decision->>'missedCount')::integer<0 THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_ADVANCE_INVALID';END IF;
    IF decision_state='pending' AND overlap_policy='buffer_one' AND has_buffer THEN decision_state:='skipped';decision_reason:='OVERLAP_BUFFER_FULL';END IF;
    INSERT INTO agent_cron_triggers(id,template_id,user_id,revision,revision_fingerprint,scheduled_for,occurrence_fingerprint,state,reason_code,
      next_attempt_at,created_at,updated_at,terminal_at)
    VALUES(decision->>'triggerId',p_template_id,template.user_id,p_revision,p_revision_fingerprint,scheduled,decision->>'occurrenceFingerprint',decision_state,
      decision_reason,p_observed_at,p_observed_at,p_observed_at,CASE WHEN decision_state='skipped' THEN p_observed_at ELSE NULL END);
    IF decision_state='pending' THEN has_buffer:=true;END IF;
    PERFORM agent_cron_append_audit(decision->>'auditEventId',p_template_id,template.user_id,p_revision,decision->>'triggerId',
      CASE WHEN decision_state='pending' THEN 'trigger.materialized' ELSE 'trigger.skipped' END,'scheduler',p_claim_owner,decision_reason,
      jsonb_build_object('missedCount',(decision->>'missedCount')::integer,'countTruncated',(decision->>'countTruncated')::boolean),p_observed_at);
  END LOOP;
  UPDATE agent_cron_templates SET next_trigger_at=p_next_trigger_at,cursor_claim_owner=NULL,cursor_claim_expires_at=NULL,updated_at=GREATEST(updated_at,p_observed_at)
    WHERE id=p_template_id;
  RETURN QUERY SELECT trigger.* FROM agent_cron_triggers trigger WHERE trigger.template_id=p_template_id AND trigger.revision=p_revision
    AND trigger.created_at=p_observed_at ORDER BY trigger.scheduled_for,trigger.id;
END
$function$;

CREATE FUNCTION agent_cron_claim_triggers(p_claim_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER)
RETURNS TABLE(trigger_id TEXT,template_id TEXT,user_id UUID,revision BIGINT,revision_fingerprint TEXT,scheduled_for TIMESTAMPTZ,
  occurrence_fingerprint TEXT,state TEXT,reason_code TEXT,run_id TEXT,retry_count INTEGER,next_attempt_at TIMESTAMPTZ,
  claim_generation BIGINT,claim_owner TEXT,claim_expires_at TIMESTAMPTZ,spec JSONB)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_claim_owner) NOT BETWEEN 1 AND 128 OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_CLAIM_INVALID';END IF;
  RETURN QUERY WITH candidates AS(
    SELECT trigger.id FROM agent_cron_triggers trigger WHERE
      ((trigger.state='pending' AND trigger.next_attempt_at<=p_now) OR (trigger.state='claimed' AND trigger.claim_expires_at<=p_now))
      AND NOT EXISTS(SELECT 1 FROM agent_cron_triggers older WHERE older.template_id=trigger.template_id
        AND older.id<>trigger.id AND older.state IN ('pending','claimed')
        AND (older.scheduled_for,older.id)<(trigger.scheduled_for,trigger.id))
      ORDER BY trigger.next_attempt_at,trigger.scheduled_for,trigger.id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ),claimed AS(
    UPDATE agent_cron_triggers trigger SET state='claimed',claim_generation=trigger.claim_generation+1,claim_owner=p_claim_owner,
      claim_expires_at=p_now+make_interval(secs=>p_lease_seconds),updated_at=GREATEST(trigger.updated_at,p_now)
    FROM candidates WHERE trigger.id=candidates.id RETURNING trigger.*
  ) SELECT claimed.id,claimed.template_id,claimed.user_id,claimed.revision,claimed.revision_fingerprint,claimed.scheduled_for,
      claimed.occurrence_fingerprint,claimed.state,claimed.reason_code,COALESCE(claimed.run_id,''),claimed.retry_count,claimed.next_attempt_at,
      claimed.claim_generation,claimed.claim_owner,claimed.claim_expires_at,revision.spec
    FROM claimed JOIN agent_cron_revisions revision ON revision.template_id=claimed.template_id AND revision.revision=claimed.revision
    ORDER BY claimed.next_attempt_at,claimed.scheduled_for,claimed.id;
END
$function$;

CREATE FUNCTION agent_cron_enqueue_trigger(p_trigger_id TEXT,p_claim_owner TEXT,p_claim_generation BIGINT,p_run_id TEXT,p_snapshot_id TEXT,
  p_snapshot_fingerprint TEXT,p_request_fingerprint TEXT,p_canonical_snapshot JSONB,p_steps JSONB,p_event_ids TEXT[],p_audit_event_id TEXT)
RETURNS TABLE(trigger_id TEXT,state TEXT,reason_code TEXT,run_id TEXT,created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE trigger agent_cron_triggers%ROWTYPE;revision agent_cron_revisions%ROWTYPE;template agent_cron_templates%ROWTYPE;
  denial TEXT;overlap_policy TEXT;run_result RECORD;scopes TEXT[];now_at TIMESTAMPTZ:=clock_timestamp();expected_steps JSONB;
BEGIN
  SELECT * INTO trigger FROM agent_cron_triggers WHERE id=p_trigger_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_NOT_FOUND';END IF;
  IF trigger.state='enqueued' THEN RETURN QUERY SELECT trigger.id,trigger.state,trigger.reason_code,trigger.run_id,false;RETURN;END IF;
  IF trigger.state<>'claimed' OR trigger.claim_owner IS DISTINCT FROM p_claim_owner OR trigger.claim_generation<>p_claim_generation
     OR trigger.claim_expires_at<=now_at THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';END IF;
  SELECT * INTO revision FROM agent_cron_revisions WHERE template_id=trigger.template_id AND agent_cron_revisions.revision=trigger.revision;
  SELECT * INTO template FROM agent_cron_templates WHERE id=trigger.template_id FOR SHARE;
  denial:=agent_cron_denial_reason(trigger.template_id,trigger.revision);
  IF denial IS NOT NULL THEN
    UPDATE agent_cron_triggers SET state='skipped',reason_code=denial,claim_owner=NULL,claim_expires_at=NULL,terminal_at=now_at,updated_at=now_at WHERE id=trigger.id RETURNING * INTO trigger;
    PERFORM agent_cron_append_audit(p_audit_event_id,trigger.template_id,trigger.user_id,trigger.revision,trigger.id,'trigger.skipped','scheduler',p_claim_owner,denial,'{}',now_at);
    RETURN QUERY SELECT trigger.id,trigger.state,trigger.reason_code,COALESCE(trigger.run_id,''),false;RETURN;
  END IF;
  SELECT jsonb_agg(value->>'kind' ORDER BY ordinality) INTO expected_steps FROM jsonb_array_elements(revision.spec->'steps') WITH ORDINALITY item(value,ordinality);
  IF jsonb_typeof(p_steps)<>'array' OR (SELECT jsonb_agg(value->>'kind' ORDER BY ordinality) FROM jsonb_array_elements(p_steps) WITH ORDINALITY item(value,ordinality))<>expected_steps
     OR NOT agent_orchestrator_snapshot_sanitized(p_canonical_snapshot) OR p_snapshot_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_request_fingerprint!~'^sha256:[a-f0-9]{64}$' THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_ENQUEUE_INVALID';END IF;
  overlap_policy:=revision.spec#>>'{policies,overlap}';
  IF EXISTS(SELECT 1 FROM agent_cron_triggers other JOIN agent_runs run ON run.id=other.run_id
      WHERE other.template_id=trigger.template_id AND other.id<>trigger.id AND other.state='enqueued'
        AND run.state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown')) THEN
    IF overlap_policy='skip' THEN
      UPDATE agent_cron_triggers SET state='skipped',reason_code='OVERLAP_SKIPPED',claim_owner=NULL,claim_expires_at=NULL,terminal_at=now_at,updated_at=now_at WHERE id=trigger.id RETURNING * INTO trigger;
      PERFORM agent_cron_append_audit(p_audit_event_id,trigger.template_id,trigger.user_id,trigger.revision,trigger.id,'trigger.skipped','scheduler',p_claim_owner,'OVERLAP_SKIPPED','{}',now_at);
      RETURN QUERY SELECT trigger.id,trigger.state,trigger.reason_code,'',false;RETURN;
    ELSIF overlap_policy='buffer_one' THEN
      UPDATE agent_cron_triggers SET state='pending',reason_code='OVERLAP_BUFFERED',claim_owner=NULL,claim_expires_at=NULL,
        next_attempt_at=now_at+make_interval(secs=>(revision.spec#>>'{policies,retryBackoffSeconds}')::integer),updated_at=now_at WHERE id=trigger.id RETURNING * INTO trigger;
      PERFORM agent_cron_append_audit(p_audit_event_id,trigger.template_id,trigger.user_id,trigger.revision,trigger.id,'trigger.retry','scheduler',p_claim_owner,'OVERLAP_BUFFERED','{}',now_at);
      RETURN QUERY SELECT trigger.id,trigger.state,trigger.reason_code,'',false;RETURN;
    END IF;
  END IF;
  SELECT array_agg(value) INTO scopes FROM jsonb_array_elements_text(revision.spec->'scopeKeys');scopes:=array_append(scopes,'run:'||p_run_id);
  SELECT * INTO run_result FROM agent_orchestrator_enqueue_run(p_run_id,p_snapshot_id,trigger.user_id,
    'cron:'||trigger.template_id||':'||trigger.revision::text||':'||extract(epoch FROM trigger.scheduled_for)::text,
    p_snapshot_fingerprint,p_request_fingerprint,p_canonical_snapshot,p_steps,scopes,p_event_ids);
  UPDATE agent_cron_triggers SET state='enqueued',reason_code='RUN_ENQUEUED',run_id=run_result.run_id,claim_owner=NULL,claim_expires_at=NULL,
    terminal_at=now_at,updated_at=now_at WHERE id=trigger.id RETURNING * INTO trigger;
  PERFORM agent_cron_append_audit(p_audit_event_id,trigger.template_id,trigger.user_id,trigger.revision,trigger.id,'trigger.enqueued','scheduler',p_claim_owner,
    'RUN_ENQUEUED',jsonb_build_object('runId',run_result.run_id,'created',run_result.created),now_at);
  RETURN QUERY SELECT trigger.id,trigger.state,trigger.reason_code,trigger.run_id,run_result.created;
END
$function$;

CREATE FUNCTION agent_cron_release_trigger(p_trigger_id TEXT,p_claim_owner TEXT,p_claim_generation BIGINT,p_error_code TEXT,
  p_retry_at TIMESTAMPTZ,p_audit_event_id TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE trigger agent_cron_triggers%ROWTYPE;revision agent_cron_revisions%ROWTYPE;new_retry INTEGER;terminal BOOLEAN;now_at TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT * INTO trigger FROM agent_cron_triggers WHERE id=p_trigger_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_NOT_FOUND';END IF;
  IF trigger.state<>'claimed' OR trigger.claim_owner IS DISTINCT FROM p_claim_owner OR trigger.claim_generation<>p_claim_generation
     OR trigger.claim_expires_at<=now_at THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';END IF;
  SELECT * INTO revision FROM agent_cron_revisions WHERE template_id=trigger.template_id AND agent_cron_revisions.revision=trigger.revision;
  IF p_error_code!~'^[A-Z][A-Z0-9_]{0,63}$' OR p_retry_at<=now_at OR p_retry_at>now_at+interval '1 day' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_RETRY_INVALID';END IF;
  new_retry:=trigger.retry_count+1;terminal:=new_retry>=(revision.spec#>>'{policies,maxAttempts}')::integer;
  UPDATE agent_cron_triggers SET state=CASE WHEN terminal THEN 'failed' ELSE 'pending' END,reason_code=p_error_code,retry_count=new_retry,
    next_attempt_at=p_retry_at,claim_owner=NULL,claim_expires_at=NULL,terminal_at=CASE WHEN terminal THEN now_at ELSE NULL END,updated_at=now_at
    WHERE id=p_trigger_id RETURNING * INTO trigger;
  PERFORM agent_cron_append_audit(p_audit_event_id,trigger.template_id,trigger.user_id,trigger.revision,trigger.id,
    CASE WHEN terminal THEN 'trigger.failed' ELSE 'trigger.retry' END,'scheduler',p_claim_owner,p_error_code,jsonb_build_object('retryCount',new_retry),now_at);
  RETURN terminal;
END
$function$;

CREATE FUNCTION agent_cron_reconcile(p_now TIMESTAMPTZ,p_limit INTEGER)
RETURNS TABLE(cursor_claims_reclaimed INTEGER,trigger_claims_reclaimed INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE cursor_count INTEGER:=0;trigger_count INTEGER:=0;target RECORD;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_LIMIT_INVALID';END IF;
  WITH targets AS(SELECT id FROM agent_cron_templates WHERE cursor_claim_expires_at<=p_now ORDER BY cursor_claim_expires_at,id FOR UPDATE SKIP LOCKED LIMIT p_limit)
    UPDATE agent_cron_templates template SET cursor_claim_owner=NULL,cursor_claim_expires_at=NULL,updated_at=GREATEST(updated_at,p_now)
    FROM targets WHERE template.id=targets.id;GET DIAGNOSTICS cursor_count=ROW_COUNT;
  FOR target IN SELECT trigger.id,trigger.template_id,trigger.user_id,trigger.revision,trigger.claim_generation
    FROM agent_cron_triggers trigger WHERE trigger.state='claimed' AND trigger.claim_expires_at<=p_now
    ORDER BY trigger.claim_expires_at,trigger.id FOR UPDATE SKIP LOCKED LIMIT p_limit LOOP
    UPDATE agent_cron_triggers SET state='pending',reason_code='CLAIM_RECLAIMED',claim_owner=NULL,claim_expires_at=NULL,next_attempt_at=p_now,updated_at=GREATEST(updated_at,p_now) WHERE id=target.id;
    INSERT INTO agent_cron_audit_events(event_id,template_id,user_id,revision,trigger_id,kind,actor_type,actor_id,reason_code,detail,occurred_at)
    VALUES('cron_event_'||substr(md5(target.id||'|'||target.claim_generation::text||'|reclaim'),1,32),target.template_id,target.user_id,target.revision,target.id,
      'claim.reclaimed','scheduler','reconciler','CLAIM_RECLAIMED',jsonb_build_object('claimGeneration',target.claim_generation),p_now) ON CONFLICT(event_id) DO NOTHING;
    trigger_count:=trigger_count+1;
  END LOOP;
  RETURN QUERY SELECT cursor_count,trigger_count;
END
$function$;

CREATE FUNCTION agent_cron_prune(p_cutoff TIMESTAMPTZ,p_limit INTEGER)
RETURNS TABLE(triggers_pruned INTEGER,audits_pruned INTEGER,templates_pruned INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE trigger_count INTEGER:=0;audit_count INTEGER:=0;template_count INTEGER:=0;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 OR p_cutoff>clock_timestamp() THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_PRUNE_INVALID';END IF;
  WITH targets AS(SELECT trigger.id FROM agent_cron_triggers trigger LEFT JOIN agent_runs run ON run.id=trigger.run_id
    WHERE trigger.terminal_at<p_cutoff AND (trigger.state IN ('skipped','failed') OR
      (trigger.state='enqueued' AND (run.id IS NULL OR run.state IN ('succeeded','failed','canceled','killed','outcome_unknown'))))
    ORDER BY trigger.terminal_at,trigger.id LIMIT p_limit)
    DELETE FROM agent_cron_triggers trigger USING targets WHERE trigger.id=targets.id;GET DIAGNOSTICS trigger_count=ROW_COUNT;
  WITH targets AS(SELECT event_id FROM agent_cron_audit_events WHERE occurred_at<p_cutoff ORDER BY occurred_at,event_id LIMIT p_limit)
    DELETE FROM agent_cron_audit_events event USING targets WHERE event.event_id=targets.event_id;GET DIAGNOSTICS audit_count=ROW_COUNT;
  WITH targets AS(SELECT template.id FROM agent_cron_templates template WHERE template.state='deleted' AND template.deleted_at<p_cutoff
      AND NOT EXISTS(SELECT 1 FROM agent_cron_triggers trigger WHERE trigger.template_id=template.id)
      AND NOT EXISTS(SELECT 1 FROM agent_cron_audit_events event WHERE event.template_id=template.id)
    ORDER BY template.deleted_at,template.id LIMIT p_limit)
    DELETE FROM agent_cron_templates template USING targets WHERE template.id=targets.id;GET DIAGNOSTICS template_count=ROW_COUNT;
  RETURN QUERY SELECT trigger_count,audit_count,template_count;
END
$function$;

DO $harden$ DECLARE schema_name TEXT:=current_schema();identity TEXT;
BEGIN FOREACH identity IN ARRAY ARRAY[
  'agent_cron_document_sanitized(jsonb)','agent_cron_spec_valid(jsonb,uuid)','agent_cron_audit_sanitized(jsonb)','agent_cron_immutable()',
  'agent_cron_append_audit(text,text,uuid,bigint,text,text,text,text,text,jsonb,timestamp with time zone)',
  'agent_cron_denial_reason(text,bigint)','agent_cron_create_revision(text,uuid,bigint,text,jsonb,timestamp with time zone,text,text,text,text)',
  'agent_cron_set_lifecycle(text,uuid,bigint,text,timestamp with time zone,integer,text,text,text,text)',
  'agent_cron_revoke_approval(text,uuid,text,text,text,text,text)',
  'agent_cron_claim_due(text,timestamp with time zone,integer,integer)',
  'agent_cron_advance_cursor(text,bigint,text,text,bigint,timestamp with time zone,timestamp with time zone,timestamp with time zone,jsonb)',
  'agent_cron_claim_triggers(text,timestamp with time zone,integer,integer)',
  'agent_cron_enqueue_trigger(text,text,bigint,text,text,text,text,jsonb,jsonb,text[],text)',
  'agent_cron_release_trigger(text,text,bigint,text,timestamp with time zone,text)',
  'agent_cron_reconcile(timestamp with time zone,integer)','agent_cron_prune(timestamp with time zone,integer)'
  ] LOOP EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',schema_name,identity,schema_name);END LOOP;
END
$harden$;

ALTER TABLE agent_cron_templates OWNER TO agent_cron_owner;ALTER TABLE agent_cron_revisions OWNER TO agent_cron_owner;
ALTER TABLE agent_cron_approvals OWNER TO agent_cron_owner;ALTER TABLE agent_cron_approval_revocations OWNER TO agent_cron_owner;
ALTER TABLE agent_cron_triggers OWNER TO agent_cron_owner;ALTER TABLE agent_cron_audit_events OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_document_sanitized(JSONB) OWNER TO agent_cron_owner;ALTER FUNCTION agent_cron_spec_valid(JSONB,UUID) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_audit_sanitized(JSONB) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_immutable() OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_append_audit(TEXT,TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,TIMESTAMPTZ) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_denial_reason(TEXT,BIGINT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_create_revision(TEXT,UUID,BIGINT,TEXT,JSONB,TIMESTAMPTZ,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_set_lifecycle(TEXT,UUID,BIGINT,TEXT,TIMESTAMPTZ,INTEGER,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_revoke_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_claim_due(TEXT,TIMESTAMPTZ,INTEGER,INTEGER) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_advance_cursor(TEXT,BIGINT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ,JSONB) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_claim_triggers(TEXT,TIMESTAMPTZ,INTEGER,INTEGER) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_enqueue_trigger(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_release_trigger(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_reconcile(TIMESTAMPTZ,INTEGER) OWNER TO agent_cron_owner;ALTER FUNCTION agent_cron_prune(TIMESTAMPTZ,INTEGER) OWNER TO agent_cron_owner;

REVOKE ALL ON agent_cron_templates,agent_cron_revisions,agent_cron_approvals,agent_cron_approval_revocations,agent_cron_triggers,agent_cron_audit_events
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control,agent_cron_control;
GRANT SELECT ON agent_cron_templates,agent_cron_revisions,agent_cron_approvals,agent_cron_approval_revocations,agent_cron_triggers,agent_cron_audit_events TO agent_cron_control;
REVOKE ALL ON FUNCTION agent_cron_document_sanitized(JSONB),agent_cron_spec_valid(JSONB,UUID),agent_cron_audit_sanitized(JSONB),agent_cron_immutable(),
  agent_cron_append_audit(TEXT,TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,TIMESTAMPTZ),agent_cron_denial_reason(TEXT,BIGINT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control,agent_cron_control;
REVOKE ALL ON FUNCTION agent_cron_create_revision(TEXT,UUID,BIGINT,TEXT,JSONB,TIMESTAMPTZ,TEXT,TEXT,TEXT,TEXT),
  agent_cron_set_lifecycle(TEXT,UUID,BIGINT,TEXT,TIMESTAMPTZ,INTEGER,TEXT,TEXT,TEXT,TEXT),
  agent_cron_revoke_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT),agent_cron_claim_due(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_advance_cursor(TEXT,BIGINT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ,JSONB),
  agent_cron_claim_triggers(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_enqueue_trigger(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT),
  agent_cron_release_trigger(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),agent_cron_reconcile(TIMESTAMPTZ,INTEGER),agent_cron_prune(TIMESTAMPTZ,INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control,agent_cron_control;
GRANT EXECUTE ON FUNCTION agent_cron_create_revision(TEXT,UUID,BIGINT,TEXT,JSONB,TIMESTAMPTZ,TEXT,TEXT,TEXT,TEXT),
  agent_cron_set_lifecycle(TEXT,UUID,BIGINT,TEXT,TIMESTAMPTZ,INTEGER,TEXT,TEXT,TEXT,TEXT),
  agent_cron_revoke_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT),agent_cron_claim_due(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_advance_cursor(TEXT,BIGINT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ,JSONB),
  agent_cron_claim_triggers(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_enqueue_trigger(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT),
  agent_cron_release_trigger(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),agent_cron_reconcile(TIMESTAMPTZ,INTEGER),agent_cron_prune(TIMESTAMPTZ,INTEGER)
  TO agent_cron_control;

DO $schema_privileges$ BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_cron_owner,agent_cron_control,go_api_runtime',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_cron_owner,agent_cron_control',current_schema());
END $schema_privileges$;
GRANT SELECT ON users,skill_installations,skill_package_candidates,skill_package_versions,agent_effect_grant_revocations,
  agent_kill_switches,agent_runs TO agent_cron_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_enqueue_run(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]),
  agent_orchestrator_active_kill_mode(TEXT[]),agent_orchestrator_snapshot_sanitized(JSONB)
  TO agent_cron_owner;
