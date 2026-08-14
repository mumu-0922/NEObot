-- G20.8 authenticated Agent Center projections and held Shadow authority.
-- The API receives bounded views and exact functions, never Runtime worker or
-- direct table DML authority.

DO $roles$
DECLARE can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create FROM pg_roles WHERE rolname=current_user;
  IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_product_owner') THEN
    IF NOT can_create THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PRODUCT_REQUIRED_ROLE_MISSING';
    END IF;
    CREATE ROLE agent_product_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_product_owner' AND
    (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PRODUCT_ROLE_MUST_BE_RESTRICTED';
  END IF;
  IF pg_has_role('go_api_runtime','agent_product_owner','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_product_owner','MEMBER')
     OR pg_has_role('agent_runner_control','agent_product_owner','MEMBER')
     OR pg_has_role('agent_effect_control','agent_product_owner','MEMBER')
     OR pg_has_role('agent_delegation_control','agent_product_owner','MEMBER')
     OR pg_has_role('agent_cron_control','agent_product_owner','MEMBER')
     OR pg_has_role('agent_learning_control','agent_product_owner','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PRODUCT_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE TABLE agent_product_run_cancellations(
  cancellation_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  mode TEXT NOT NULL,
  state TEXT NOT NULL,
  expected_run_state TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  completed_at TIMESTAMPTZ,
  CONSTRAINT agent_product_cancellation_run_fk FOREIGN KEY(run_id,user_id)
    REFERENCES agent_runs(id,user_id) ON DELETE CASCADE,
  CONSTRAINT agent_product_cancellation_id_check
    CHECK(cancellation_id~'^cancellation_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_product_cancellation_mode_check CHECK(mode IN ('cancel','kill')),
  CONSTRAINT agent_product_cancellation_state_check CHECK(state IN ('requested','completed')),
  CONSTRAINT agent_product_cancellation_run_state_check
    CHECK(expected_run_state IN ('pending','admitted','queued','running')),
  CONSTRAINT agent_product_cancellation_fingerprint_check
    CHECK(snapshot_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_product_cancellation_reason_check
    CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_product_cancellation_shape CHECK(
    (state='requested' AND completed_at IS NULL) OR
    (state='completed' AND completed_at IS NOT NULL)),
  UNIQUE(run_id)
);

CREATE TABLE agent_artifacts(
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  attempt_id TEXT NOT NULL,
  generation BIGINT NOT NULL,
  name TEXT NOT NULL,
  media_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  fingerprint TEXT NOT NULL,
  object_key TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_artifact_run_fk FOREIGN KEY(run_id,user_id)
    REFERENCES agent_runs(id,user_id) ON DELETE CASCADE,
  CONSTRAINT agent_artifact_attempt_fk FOREIGN KEY(run_id,user_id,attempt_id)
    REFERENCES agent_attempts(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_artifact_id_check CHECK(id~'^artifact_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_artifact_generation_check CHECK(generation>=1),
  CONSTRAINT agent_artifact_name_check CHECK(octet_length(name) BETWEEN 1 AND 255 AND name=btrim(name)
    AND name!~E'[\\x00\\r\\n/\\\\]'),
  CONSTRAINT agent_artifact_media_check CHECK(media_type~'^[a-z0-9][a-z0-9.+-]{0,63}/[a-z0-9][a-z0-9.+-]{0,63}$'),
  CONSTRAINT agent_artifact_size_check CHECK(size_bytes BETWEEN 0 AND 1073741824),
  CONSTRAINT agent_artifact_fingerprint_check CHECK(fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_artifact_object_check CHECK(object_key='agent-artifacts/'||run_id||'/'||attempt_id||'/'||generation::text||'/'||substring(fingerprint FROM 8)),
  UNIQUE(run_id,attempt_id,generation,name)
);

CREATE TABLE agent_shadow_policies(
  revision BIGINT PRIMARY KEY,
  enabled BOOLEAN NOT NULL,
  mode TEXT NOT NULL,
  admission_id UUID,
  package_fingerprint TEXT NOT NULL DEFAULT '',
  runtime_bundle_fingerprint TEXT NOT NULL DEFAULT '',
  cohort_basis_points INTEGER NOT NULL,
  max_observations INTEGER NOT NULL,
  max_errors INTEGER NOT NULL,
  starts_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  reason_code TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_shadow_policy_revision_check CHECK(revision>=1),
  CONSTRAINT agent_shadow_policy_mode_check CHECK(mode IN ('synthetic','read_only')),
  CONSTRAINT agent_shadow_policy_cohort_check CHECK(cohort_basis_points BETWEEN 0 AND 10000),
  CONSTRAINT agent_shadow_policy_budget_check CHECK(max_observations BETWEEN 1 AND 100000 AND max_errors BETWEEN 0 AND max_observations),
  CONSTRAINT agent_shadow_policy_reason_check CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_shadow_policy_shape CHECK(
    (enabled AND admission_id IS NOT NULL
      AND package_fingerprint~'^sha256:[a-f0-9]{64}$'
      AND runtime_bundle_fingerprint~'^sha256:[a-f0-9]{64}$'
      AND starts_at IS NOT NULL AND expires_at>starts_at)
    OR (NOT enabled AND admission_id IS NULL AND package_fingerprint=''
      AND runtime_bundle_fingerprint='' AND starts_at IS NULL AND expires_at IS NULL))
);

CREATE TABLE agent_shadow_user_opt_ins(
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  generation BIGINT NOT NULL,
  policy_revision BIGINT NOT NULL REFERENCES agent_shadow_policies(revision) ON DELETE RESTRICT,
  opted_in BOOLEAN NOT NULL,
  reason_code TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY(user_id,generation),
  CONSTRAINT agent_shadow_opt_generation_check CHECK(generation>=1),
  CONSTRAINT agent_shadow_opt_reason_check CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$')
);

CREATE TABLE agent_shadow_boot_state(
  singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),
  epoch BIGINT NOT NULL DEFAULT 0 CHECK(epoch>=0),
  boot_id TEXT NOT NULL DEFAULT 'boot_unregistered0000',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_shadow_boot_id_check CHECK(boot_id~'^boot_[a-z0-9]{16,64}$')
);
INSERT INTO agent_shadow_boot_state(singleton) VALUES(true);

CREATE TABLE agent_shadow_observations(
  observation_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  boot_epoch BIGINT NOT NULL,
  policy_revision BIGINT NOT NULL REFERENCES agent_shadow_policies(revision) ON DELETE RESTRICT,
  user_id UUID NOT NULL,
  generation BIGINT NOT NULL,
  admission_id UUID NOT NULL,
  package_fingerprint TEXT NOT NULL,
  runtime_bundle_fingerprint TEXT NOT NULL,
  mode TEXT NOT NULL,
  outcome TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  latency_bucket TEXT NOT NULL,
  run_count INTEGER NOT NULL,
  step_count INTEGER NOT NULL,
  attempt_count INTEGER NOT NULL,
  error_count INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_shadow_observation_opt_fk FOREIGN KEY(user_id,generation)
    REFERENCES agent_shadow_user_opt_ins(user_id,generation) ON DELETE RESTRICT,
  CONSTRAINT agent_shadow_observation_mode_check CHECK(mode IN ('synthetic','read_only')),
  CONSTRAINT agent_shadow_observation_outcome_check CHECK(outcome IN ('succeeded','failed','held')),
  CONSTRAINT agent_shadow_observation_reason_check CHECK(reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_shadow_observation_latency_check CHECK(latency_bucket IN ('lt_100ms','lt_500ms','lt_2s','gte_2s')),
  CONSTRAINT agent_shadow_observation_counts_check CHECK(run_count BETWEEN 0 AND 100000
    AND step_count BETWEEN 0 AND 100000 AND attempt_count BETWEEN 0 AND 100000
    AND error_count BETWEEN 0 AND 100000),
  CONSTRAINT agent_shadow_observation_fingerprints_check
    CHECK(package_fingerprint~'^sha256:[a-f0-9]{64}$'
      AND runtime_bundle_fingerprint~'^sha256:[a-f0-9]{64}$')
);
CREATE INDEX idx_agent_shadow_observation_budget
  ON agent_shadow_observations(policy_revision,user_id,generation,observation_id);

CREATE FUNCTION agent_product_fact_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  IF TG_OP='DELETE' AND pg_trigger_depth()>1 THEN RETURN OLD;END IF;
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_PRODUCT_FACT_IMMUTABLE';
END
$function$;
CREATE TRIGGER trg_agent_product_cancellation_immutable
  BEFORE DELETE ON agent_product_run_cancellations FOR EACH ROW EXECUTE FUNCTION agent_product_fact_immutable();
CREATE TRIGGER trg_agent_artifact_immutable
  BEFORE UPDATE OR DELETE ON agent_artifacts FOR EACH ROW EXECUTE FUNCTION agent_product_fact_immutable();
CREATE TRIGGER trg_agent_shadow_policy_immutable
  BEFORE UPDATE OR DELETE ON agent_shadow_policies FOR EACH ROW EXECUTE FUNCTION agent_product_fact_immutable();
CREATE TRIGGER trg_agent_shadow_opt_immutable
  BEFORE UPDATE OR DELETE ON agent_shadow_user_opt_ins FOR EACH ROW EXECUTE FUNCTION agent_product_fact_immutable();
CREATE TRIGGER trg_agent_shadow_observation_immutable
  BEFORE UPDATE OR DELETE ON agent_shadow_observations FOR EACH ROW EXECUTE FUNCTION agent_product_fact_immutable();

CREATE VIEW agent_product_runs AS
SELECT run.id,run.user_id,run.state,run.snapshot_fingerprint,run.request_fingerprint,
  run.created_at,run.updated_at,run.terminal_at,
  cancellation.state AS cancellation_state,cancellation.mode AS cancellation_mode,
  cancellation.reason_code AS cancellation_reason
FROM agent_runs run
LEFT JOIN agent_product_run_cancellations cancellation ON cancellation.run_id=run.id;

CREATE VIEW agent_product_steps AS
SELECT id,run_id,user_id,ordinal,kind,state,current_generation,created_at,updated_at,terminal_at
FROM agent_steps;

CREATE VIEW agent_product_attempts AS
SELECT id,run_id,user_id,step_id,generation,state,lease_expires_at,created_at,updated_at,terminal_at
FROM agent_attempts;

CREATE VIEW agent_product_events AS
SELECT id,run_id,user_id,step_id,attempt_id,sequence,occurred_at,kind,entity,from_state,to_state,
  actor_type,reason_code,generation,lease_expires_at
FROM agent_run_events;

CREATE VIEW agent_product_approvals AS
SELECT id AS intent_id,user_id,run_id,intent_fingerprint,tool_identity,capability,action,
  arguments_fingerprint,approval_class,approval_revision,state,created_at,expires_at,
  approved_at,terminal_at,error_code
FROM agent_effect_intents;

CREATE VIEW agent_product_children AS
SELECT run_id,user_id,parent_run_id,depth,state,package_fingerprint,runtime_bundle_fingerprint,
  grant_fingerprint,registry_fingerprint,expires_at,created_at
FROM agent_delegation_authorities;

CREATE VIEW agent_product_artifacts AS
SELECT id,run_id,user_id,attempt_id,generation,name,media_type,size_bytes,fingerprint,created_at
FROM agent_artifacts;

CREATE VIEW agent_product_schedules AS
SELECT template.id,template.user_id,template.state,template.current_revision,
  template.current_revision_fingerprint AS revision_fingerprint,template.next_trigger_at,
  revision.spec#>>'{schedule,expression}' AS schedule_expression,
  revision.spec#>>'{schedule,timezone}' AS timezone,
  revision.spec#>>'{schedule,calculator}' AS calculator,
  revision.automation_class,revision.approval_id,
  revision.spec#>>'{skill,packageFingerprint}' AS package_fingerprint,
  revision.spec#>>'{skill,runtimeBundleFingerprint}' AS runtime_bundle_fingerprint,
  revision.spec#>>'{policies,missed}' AS missed_policy,
  revision.spec#>>'{policies,overlap}' AS overlap_policy,
  template.created_at,template.updated_at
FROM agent_cron_templates template
JOIN agent_cron_revisions revision ON revision.template_id=template.id
  AND revision.revision=template.current_revision;

CREATE VIEW agent_product_drafts AS
SELECT draft.id,draft.user_id,draft.state,draft.revision,draft.draft_fingerprint,
  draft.base_package_fingerprint,draft.proposed_package_fingerprint,
  draft.spec->>'runtimeBundleFingerprint' AS runtime_bundle_fingerprint,
  draft.spec->>'name' AS name,draft.spec->>'version' AS version,
  draft.check_generation,draft.check_attempts,draft.admission_id,
  draft.created_at,draft.updated_at,draft.object_deleted_at
FROM agent_learning_drafts draft;

CREATE VIEW agent_product_draft_checks AS
SELECT id,draft_id,user_id,generation,kind,status,reason_code,suite_fingerprint,
  evidence_fingerprint,duration_millis,metrics,created_at
FROM agent_learning_check_results;

CREATE FUNCTION agent_product_get_artifact(
  p_user_id UUID,p_run_id TEXT,p_artifact_id TEXT
) RETURNS TABLE(
  id TEXT,attempt_id TEXT,generation BIGINT,name TEXT,media_type TEXT,
  size_bytes BIGINT,fingerprint TEXT,created_at TIMESTAMPTZ,object_key TEXT
) LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT artifact.id,artifact.attempt_id,artifact.generation,artifact.name,
    artifact.media_type,artifact.size_bytes,artifact.fingerprint,
    artifact.created_at,artifact.object_key
  FROM agent_artifacts artifact
  WHERE artifact.user_id=p_user_id AND artifact.run_id=p_run_id
    AND artifact.id=p_artifact_id
$function$;

CREATE FUNCTION agent_product_cancel_run(
  p_cancellation_id TEXT,p_user_id UUID,p_run_id TEXT,p_expected_state TEXT,
  p_snapshot_fingerprint TEXT,p_mode TEXT,p_reason_code TEXT
) RETURNS agent_product_run_cancellations
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  run agent_runs%ROWTYPE;
  existing agent_product_run_cancellations%ROWTYPE;
  item RECORD;
  now_at TIMESTAMPTZ:=clock_timestamp();
  switch_id TEXT:='switch_'||md5(p_cancellation_id);
BEGIN
  SELECT * INTO existing FROM agent_product_run_cancellations
    WHERE cancellation_id=p_cancellation_id;
  IF FOUND THEN
    IF existing.run_id<>p_run_id OR existing.user_id<>p_user_id
       OR existing.expected_run_state<>p_expected_state
       OR existing.snapshot_fingerprint<>p_snapshot_fingerprint
       OR existing.mode<>p_mode OR existing.reason_code<>p_reason_code THEN
      RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';
    END IF;
    RETURN existing;
  END IF;
  SELECT * INTO run FROM agent_runs
    WHERE id=p_run_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PRODUCT_RUN_NOT_FOUND';
  END IF;
  IF run.state<>p_expected_state OR run.snapshot_fingerprint<>p_snapshot_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';
  END IF;
  IF p_mode NOT IN ('cancel','kill') OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$'
     OR p_cancellation_id!~'^cancellation_[a-z0-9]{16,64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANCEL_INVALID';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_product_run_cancellations WHERE run_id=p_run_id) THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';
  END IF;
  IF run.state IN ('pending','admitted','queued') THEN
    IF EXISTS(SELECT 1 FROM agent_attempts WHERE run_id=p_run_id)
       OR EXISTS(SELECT 1 FROM agent_effect_intents WHERE run_id=p_run_id)
       OR EXISTS(SELECT 1 FROM agent_delegation_authorities WHERE parent_run_id=p_run_id) THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='RUN_CANCEL_BLOCKED';
    END IF;
    INSERT INTO agent_product_run_cancellations(cancellation_id,run_id,user_id,mode,state,
      expected_run_state,snapshot_fingerprint,reason_code,created_at)
    VALUES(p_cancellation_id,p_run_id,p_user_id,p_mode,'requested',p_expected_state,
      p_snapshot_fingerprint,p_reason_code,now_at);
    FOR item IN SELECT * FROM agent_steps WHERE run_id=p_run_id ORDER BY ordinal FOR UPDATE LOOP
      IF item.state IN ('pending','ready') THEN
        UPDATE agent_steps SET state=CASE WHEN p_mode='kill' THEN 'killed' ELSE 'canceled' END,
          updated_at=now_at,terminal_at=now_at WHERE id=item.id;
        PERFORM agent_orchestrator_append_event(
          'event_'||md5(p_cancellation_id||'|step|'||item.id),p_run_id,p_user_id,
          item.id,NULL,'step.transition','step',item.state,
          CASE WHEN p_mode='kill' THEN 'killed' ELSE 'canceled' END,
          'user',p_user_id::text,p_reason_code,NULL,NULL,'{}',now_at);
      END IF;
    END LOOP;
    UPDATE agent_runs SET state=CASE WHEN p_mode='kill' THEN 'killed' ELSE 'canceled' END,
      updated_at=now_at,terminal_at=now_at WHERE id=p_run_id;
    PERFORM agent_orchestrator_append_event(
      'event_'||md5(p_cancellation_id||'|run'),p_run_id,p_user_id,NULL,NULL,'run.transition','run',
      p_expected_state,CASE WHEN p_mode='kill' THEN 'killed' ELSE 'canceled' END,
      'user',p_user_id::text,p_reason_code,NULL,NULL,'{}',now_at);
    UPDATE agent_product_run_cancellations SET state='completed',completed_at=now_at
      WHERE cancellation_id=p_cancellation_id RETURNING * INTO existing;
    RETURN existing;
  END IF;
  IF run.state<>'running' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='RUN_CANCEL_BLOCKED';
  END IF;
  INSERT INTO agent_product_run_cancellations(cancellation_id,run_id,user_id,mode,state,
    expected_run_state,snapshot_fingerprint,reason_code,created_at)
  VALUES(p_cancellation_id,p_run_id,p_user_id,p_mode,'requested',p_expected_state,
    p_snapshot_fingerprint,p_reason_code,now_at) RETURNING * INTO existing;
  PERFORM agent_orchestrator_append_kill_switch(
    switch_id,'run',p_run_id,p_mode,true,0,'user',p_user_id::text,p_reason_code);
  RETURN existing;
END
$function$;

CREATE FUNCTION agent_product_register_shadow_boot(p_boot_id TEXT)
RETURNS BIGINT LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE next_epoch BIGINT;
BEGIN
  IF p_boot_id!~'^boot_[a-z0-9]{16,64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_SHADOW_BOOT_INVALID';
  END IF;
  UPDATE agent_shadow_boot_state SET epoch=epoch+1,boot_id=p_boot_id,updated_at=clock_timestamp()
    WHERE singleton RETURNING epoch INTO next_epoch;
  RETURN next_epoch;
END
$function$;

CREATE FUNCTION agent_product_update_shadow_policy(
  p_actor UUID,p_expected_revision BIGINT,p_enabled BOOLEAN,p_mode TEXT,
  p_admission_id UUID,p_package_fingerprint TEXT,p_runtime_fingerprint TEXT,
  p_cohort_basis_points INTEGER,p_max_observations INTEGER,p_max_errors INTEGER,
  p_starts_at TIMESTAMPTZ,p_expires_at TIMESTAMPTZ,p_reason_code TEXT
) RETURNS agent_shadow_policies
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE current_revision BIGINT;result agent_shadow_policies%ROWTYPE;
BEGIN
  SELECT COALESCE(max(revision),0) INTO current_revision FROM agent_shadow_policies;
  IF current_revision<>p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';
  END IF;
  IF p_mode NOT IN ('synthetic','read_only') OR p_cohort_basis_points NOT BETWEEN 0 AND 10000
     OR p_max_observations NOT BETWEEN 1 AND 100000 OR p_max_errors NOT BETWEEN 0 AND p_max_observations
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$'
     OR NOT EXISTS(SELECT 1 FROM users WHERE id=p_actor AND deleted_at IS NULL) THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_SHADOW_POLICY_INVALID';
  END IF;
  IF p_enabled THEN
    IF p_starts_at IS NULL OR p_expires_at<=p_starts_at OR p_expires_at<=clock_timestamp()
       OR NOT EXISTS(
         SELECT 1 FROM skill_package_candidates candidate
         JOIN skill_package_versions package ON package.package_fingerprint=candidate.package_fingerprint
         WHERE candidate.id=p_admission_id AND candidate.status='admitted'
           AND candidate.package_fingerprint=p_package_fingerprint
           AND package.runtime_bundle_fingerprint=p_runtime_fingerprint
           AND package.has_runtime
       ) THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='FINGERPRINT_DRIFT';
    END IF;
  ELSE
    p_admission_id:=NULL;p_package_fingerprint:='';p_runtime_fingerprint:='';
    p_starts_at:=NULL;p_expires_at:=NULL;
  END IF;
  INSERT INTO agent_shadow_policies(revision,enabled,mode,admission_id,package_fingerprint,
    runtime_bundle_fingerprint,cohort_basis_points,max_observations,max_errors,starts_at,
    expires_at,actor_user_id,reason_code,updated_at)
  VALUES(current_revision+1,p_enabled,p_mode,p_admission_id,p_package_fingerprint,
    p_runtime_fingerprint,p_cohort_basis_points,p_max_observations,p_max_errors,
    p_starts_at,p_expires_at,p_actor,p_reason_code,clock_timestamp()) RETURNING * INTO result;
  RETURN result;
END
$function$;

CREATE FUNCTION agent_product_set_shadow_opt_in(
  p_user_id UUID,p_expected_generation BIGINT,p_policy_revision BIGINT,
  p_opted_in BOOLEAN,p_reason_code TEXT
) RETURNS agent_shadow_user_opt_ins
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE current_generation BIGINT;current_policy BIGINT;result agent_shadow_user_opt_ins%ROWTYPE;
BEGIN
  SELECT COALESCE(max(generation),0) INTO current_generation
    FROM agent_shadow_user_opt_ins WHERE user_id=p_user_id;
  SELECT COALESCE(max(revision),0) INTO current_policy FROM agent_shadow_policies;
  IF current_generation<>p_expected_generation THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='GENERATION_STALE';
  END IF;
  IF p_policy_revision<>current_policy OR p_policy_revision<1 THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';
  END IF;
  IF p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$'
     OR NOT EXISTS(SELECT 1 FROM users WHERE id=p_user_id AND deleted_at IS NULL) THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_SHADOW_OPT_INVALID';
  END IF;
  INSERT INTO agent_shadow_user_opt_ins(user_id,generation,policy_revision,opted_in,reason_code,updated_at)
  VALUES(p_user_id,current_generation+1,p_policy_revision,p_opted_in,p_reason_code,clock_timestamp())
  RETURNING * INTO result;
  RETURN result;
END
$function$;

CREATE FUNCTION agent_product_shadow_snapshot(p_user_id UUID)
RETURNS TABLE(
  policy_revision BIGINT,enabled BOOLEAN,mode TEXT,admission_id UUID,
  package_fingerprint TEXT,runtime_bundle_fingerprint TEXT,cohort_basis_points INTEGER,
  max_observations INTEGER,max_errors INTEGER,starts_at TIMESTAMPTZ,expires_at TIMESTAMPTZ,
  policy_updated_at TIMESTAMPTZ,opted_in BOOLEAN,opt_generation BIGINT,
  opt_policy_revision BIGINT,opt_updated_at TIMESTAMPTZ,cohort_selected BOOLEAN,
  eligible BOOLEAN,effective BOOLEAN,held_reason_code TEXT,observation_count INTEGER,error_count INTEGER
) LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE policy agent_shadow_policies%ROWTYPE;opt agent_shadow_user_opt_ins%ROWTYPE;
  observations INTEGER:=0;errors INTEGER:=0;selected BOOLEAN:=false;is_eligible BOOLEAN:=false;held TEXT;
BEGIN
  SELECT * INTO policy FROM agent_shadow_policies ORDER BY revision DESC LIMIT 1;
  IF NOT FOUND THEN
    RETURN QUERY SELECT 0::bigint,false,'synthetic'::text,NULL::uuid,''::text,''::text,0,1,0,
      NULL::timestamptz,NULL::timestamptz,to_timestamp(0),false,0::bigint,0::bigint,to_timestamp(0),
      false,false,false,'SHADOW_DISABLED'::text,0,0;
    RETURN;
  END IF;
  SELECT * INTO opt FROM agent_shadow_user_opt_ins WHERE user_id=p_user_id ORDER BY generation DESC LIMIT 1;
  IF opt.user_id IS NOT NULL THEN
    SELECT count(*)::integer,COALESCE(sum(observation.error_count),0)::integer INTO observations,errors
    FROM agent_shadow_observations observation
    WHERE observation.policy_revision=policy.revision AND observation.user_id=p_user_id
      AND observation.generation=opt.generation;
  END IF;
  selected:=((hashtextextended(p_user_id::text||E'\\x00'||policy.revision::text||E'\\x00'||
    policy.package_fingerprint||E'\\x00'||policy.runtime_bundle_fingerprint,0) & 9223372036854775807)
    % 10000)<policy.cohort_basis_points;
  held:=CASE
    WHEN NOT policy.enabled THEN 'SHADOW_DISABLED'
    WHEN opt.user_id IS NULL OR NOT opt.opted_in THEN 'USER_OPT_OUT'
    WHEN opt.policy_revision<>policy.revision THEN 'POLICY_STALE'
    WHEN clock_timestamp()<policy.starts_at THEN 'POLICY_NOT_STARTED'
    WHEN clock_timestamp()>=policy.expires_at THEN 'POLICY_EXPIRED'
    WHEN NOT selected THEN 'COHORT_NOT_SELECTED'
    WHEN observations>=policy.max_observations OR errors>policy.max_errors THEN 'BUDGET_EXCEEDED'
    WHEN agent_orchestrator_active_kill_mode(ARRAY[
      'global:*','user:'||p_user_id::text,'admission:'||policy.admission_id::text,
      'skill:'||policy.package_fingerprint
    ]) IS NOT NULL THEN 'KILL_SWITCH_ACTIVE'
    ELSE 'ISOLATION_UNAVAILABLE' END;
  is_eligible:=held='ISOLATION_UNAVAILABLE';
  RETURN QUERY SELECT policy.revision,policy.enabled,policy.mode,policy.admission_id,
    policy.package_fingerprint,policy.runtime_bundle_fingerprint,policy.cohort_basis_points,
    policy.max_observations,policy.max_errors,policy.starts_at,policy.expires_at,policy.updated_at,
    COALESCE(opt.opted_in,false),COALESCE(opt.generation,0),COALESCE(opt.policy_revision,0),
    COALESCE(opt.updated_at,to_timestamp(0)),selected,is_eligible,false,held,observations,errors;
END
$function$;

CREATE FUNCTION agent_product_append_shadow_observation(
  p_boot_id TEXT,p_user_id UUID,p_policy_revision BIGINT,p_generation BIGINT,
  p_admission_id UUID,p_package_fingerprint TEXT,p_runtime_fingerprint TEXT,p_mode TEXT,
  p_outcome TEXT,p_reason_code TEXT,p_latency_bucket TEXT,p_run_count INTEGER,
  p_step_count INTEGER,p_attempt_count INTEGER,p_error_count INTEGER
) RETURNS TABLE(
  observation_id TEXT,policy_revision BIGINT,generation BIGINT,admission_id UUID,
  package_fingerprint TEXT,runtime_bundle_fingerprint TEXT,mode TEXT,outcome TEXT,
  reason_code TEXT,latency_bucket TEXT,run_count INTEGER,step_count INTEGER,
  attempt_count INTEGER,error_count INTEGER,created_at TIMESTAMPTZ
) LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE policy agent_shadow_policies%ROWTYPE;opt agent_shadow_user_opt_ins%ROWTYPE;
  boot agent_shadow_boot_state%ROWTYPE;count_now INTEGER;errors_now INTEGER;inserted agent_shadow_observations%ROWTYPE;
BEGIN
  SELECT * INTO boot FROM agent_shadow_boot_state WHERE singleton FOR SHARE;
  SELECT * INTO policy FROM agent_shadow_policies WHERE revision=p_policy_revision FOR SHARE;
  SELECT opt_in.* INTO opt FROM agent_shadow_user_opt_ins opt_in
    WHERE opt_in.user_id=p_user_id AND opt_in.generation=p_generation FOR SHARE;
  IF boot.boot_id<>p_boot_id OR policy.revision IS NULL OR opt.user_id IS NULL
     OR opt.policy_revision<>policy.revision OR NOT policy.enabled OR NOT opt.opted_in
     OR policy.admission_id<>p_admission_id OR policy.package_fingerprint<>p_package_fingerprint
     OR policy.runtime_bundle_fingerprint<>p_runtime_fingerprint OR policy.mode<>p_mode
     OR clock_timestamp()<policy.starts_at OR clock_timestamp()>=policy.expires_at THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='GENERATION_STALE';
  END IF;
  IF p_outcome NOT IN ('succeeded','failed','held') OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$'
     OR p_latency_bucket NOT IN ('lt_100ms','lt_500ms','lt_2s','gte_2s')
     OR p_run_count NOT BETWEEN 0 AND 100000 OR p_step_count NOT BETWEEN 0 AND 100000
     OR p_attempt_count NOT BETWEEN 0 AND 100000 OR p_error_count NOT BETWEEN 0 AND 100000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_SHADOW_OBSERVATION_INVALID';
  END IF;
  IF agent_orchestrator_active_kill_mode(ARRAY[
    'global:*','user:'||p_user_id::text,'admission:'||policy.admission_id::text,
    'skill:'||policy.package_fingerprint
  ]) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_user_id::text||E'\\x00'||p_policy_revision::text||E'\\x00'||p_generation::text,0));
  SELECT count(*)::integer,COALESCE(sum(observation.error_count),0)::integer INTO count_now,errors_now
    FROM agent_shadow_observations observation WHERE observation.policy_revision=p_policy_revision
      AND observation.user_id=p_user_id AND observation.generation=p_generation;
  IF count_now>=policy.max_observations OR errors_now+p_error_count>policy.max_errors THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='BUDGET_EXCEEDED';
  END IF;
  INSERT INTO agent_shadow_observations(boot_epoch,policy_revision,user_id,generation,admission_id,
    package_fingerprint,runtime_bundle_fingerprint,mode,outcome,reason_code,latency_bucket,
    run_count,step_count,attempt_count,error_count,created_at)
  VALUES(boot.epoch,p_policy_revision,p_user_id,p_generation,p_admission_id,p_package_fingerprint,
    p_runtime_fingerprint,p_mode,p_outcome,p_reason_code,p_latency_bucket,p_run_count,p_step_count,
    p_attempt_count,p_error_count,clock_timestamp()) RETURNING * INTO inserted;
  RETURN QUERY SELECT 'shadow_observation_'||lpad(inserted.observation_id::text,16,'0'),
    inserted.policy_revision,inserted.generation,inserted.admission_id,inserted.package_fingerprint,
    inserted.runtime_bundle_fingerprint,inserted.mode,inserted.outcome,inserted.reason_code,
    inserted.latency_bucket,inserted.run_count,inserted.step_count,inserted.attempt_count,
    inserted.error_count,inserted.created_at;
END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema();identity TEXT;
BEGIN
  FOREACH identity IN ARRAY ARRAY[
    'agent_product_get_artifact(uuid,text,text)',
    'agent_product_cancel_run(text,uuid,text,text,text,text,text)',
    'agent_product_register_shadow_boot(text)',
    'agent_product_update_shadow_policy(uuid,bigint,boolean,text,uuid,text,text,integer,integer,integer,timestamp with time zone,timestamp with time zone,text)',
    'agent_product_set_shadow_opt_in(uuid,bigint,bigint,boolean,text)',
    'agent_product_shadow_snapshot(uuid)',
    'agent_product_append_shadow_observation(text,uuid,bigint,bigint,uuid,text,text,text,text,text,text,integer,integer,integer,integer)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',schema_name,identity,schema_name);
  END LOOP;
END
$harden$;

ALTER TABLE agent_product_run_cancellations OWNER TO agent_product_owner;
ALTER TABLE agent_artifacts OWNER TO agent_product_owner;
ALTER TABLE agent_shadow_policies OWNER TO agent_product_owner;
ALTER TABLE agent_shadow_user_opt_ins OWNER TO agent_product_owner;
ALTER TABLE agent_shadow_boot_state OWNER TO agent_product_owner;
ALTER TABLE agent_shadow_observations OWNER TO agent_product_owner;
ALTER VIEW agent_product_runs OWNER TO agent_product_owner;
ALTER VIEW agent_product_steps OWNER TO agent_product_owner;
ALTER VIEW agent_product_attempts OWNER TO agent_product_owner;
ALTER VIEW agent_product_events OWNER TO agent_product_owner;
ALTER VIEW agent_product_approvals OWNER TO agent_product_owner;
ALTER VIEW agent_product_children OWNER TO agent_product_owner;
ALTER VIEW agent_product_artifacts OWNER TO agent_product_owner;
ALTER VIEW agent_product_schedules OWNER TO agent_product_owner;
ALTER VIEW agent_product_drafts OWNER TO agent_product_owner;
ALTER VIEW agent_product_draft_checks OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_fact_immutable() OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_get_artifact(UUID,TEXT,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_cancel_run(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_register_shadow_boot(TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_update_shadow_policy(UUID,BIGINT,BOOLEAN,TEXT,UUID,TEXT,TEXT,INTEGER,INTEGER,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_set_shadow_opt_in(UUID,BIGINT,BIGINT,BOOLEAN,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_shadow_snapshot(UUID) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_append_shadow_observation(TEXT,UUID,BIGINT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,INTEGER,INTEGER,INTEGER) OWNER TO agent_product_owner;

REVOKE ALL ON agent_product_run_cancellations,agent_artifacts,agent_shadow_policies,
  agent_shadow_user_opt_ins,agent_shadow_boot_state,agent_shadow_observations
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
  agent_effect_control,agent_delegation_control,agent_cron_control,agent_learning_control;
REVOKE ALL ON agent_product_runs,agent_product_steps,agent_product_attempts,agent_product_events,
  agent_product_approvals,agent_product_children,agent_product_artifacts,agent_product_schedules,
  agent_product_drafts,agent_product_draft_checks FROM PUBLIC,go_api_runtime;
GRANT SELECT ON agent_product_runs,agent_product_steps,agent_product_attempts,agent_product_events,
  agent_product_approvals,agent_product_children,agent_product_artifacts,agent_product_schedules,
  agent_product_drafts,agent_product_draft_checks TO go_api_runtime;

REVOKE ALL ON FUNCTION agent_product_fact_immutable(),
  agent_product_get_artifact(UUID,TEXT,TEXT),
  agent_product_cancel_run(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_product_register_shadow_boot(TEXT),
  agent_product_update_shadow_policy(UUID,BIGINT,BOOLEAN,TEXT,UUID,TEXT,TEXT,INTEGER,INTEGER,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT),
  agent_product_set_shadow_opt_in(UUID,BIGINT,BIGINT,BOOLEAN,TEXT),
  agent_product_shadow_snapshot(UUID),
  agent_product_append_shadow_observation(TEXT,UUID,BIGINT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,INTEGER,INTEGER,INTEGER)
  FROM PUBLIC,go_api_runtime;
GRANT EXECUTE ON FUNCTION
  agent_product_get_artifact(UUID,TEXT,TEXT),
  agent_product_cancel_run(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_product_register_shadow_boot(TEXT),
  agent_product_update_shadow_policy(UUID,BIGINT,BOOLEAN,TEXT,UUID,TEXT,TEXT,INTEGER,INTEGER,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT),
  agent_product_set_shadow_opt_in(UUID,BIGINT,BIGINT,BOOLEAN,TEXT),
  agent_product_shadow_snapshot(UUID),
  agent_product_append_shadow_observation(TEXT,UUID,BIGINT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,INTEGER,INTEGER,INTEGER)
  TO go_api_runtime;

-- Existing owning services remain the mutation seam. G20.8 gives the API only
-- user approval/cancel, Cron user revision/lifecycle and human Draft review.
GRANT EXECUTE ON FUNCTION agent_effect_decide_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT),
  agent_effect_cancel(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT) TO go_api_runtime;
GRANT SELECT ON agent_cron_templates,agent_cron_revisions TO go_api_runtime;
GRANT EXECUTE ON FUNCTION agent_cron_create_revision(TEXT,UUID,BIGINT,TEXT,JSONB,TIMESTAMPTZ,TEXT,TEXT,TEXT,TEXT),
  agent_cron_set_lifecycle(TEXT,UUID,BIGINT,TEXT,TIMESTAMPTZ,INTEGER,TEXT,TEXT,TEXT,TEXT),
  agent_cron_revoke_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT) TO go_api_runtime;
GRANT SELECT ON agent_learning_drafts,agent_learning_check_results,agent_learning_decisions TO go_api_runtime;
GRANT EXECUTE ON FUNCTION agent_learning_reject(TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_learning_promote(TEXT,UUID,BIGINT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT) TO go_api_runtime;

GRANT SELECT ON users,agent_runs,agent_steps,agent_attempts,agent_run_events,
  agent_effect_intents,agent_delegation_authorities,agent_cron_templates,agent_cron_revisions,
  agent_learning_drafts,agent_learning_check_results,skill_package_candidates,skill_package_versions
  TO agent_product_owner;
GRANT UPDATE ON agent_runs,agent_steps TO agent_product_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_active_kill_mode(TEXT[])
  TO agent_product_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_append_event(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,JSONB,TIMESTAMPTZ),
  agent_orchestrator_append_kill_switch(TEXT,TEXT,TEXT,TEXT,BOOLEAN,BIGINT,TEXT,TEXT,TEXT)
  TO agent_product_owner;

DO $schema_acl$ BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_product_owner,go_api_runtime',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_product_owner',current_schema());
END
$schema_acl$;
