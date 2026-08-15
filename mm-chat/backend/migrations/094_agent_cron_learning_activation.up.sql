-- G21.5 activates one operator-bound Cron Template and one operator-bound
-- quarantined Draft without opening the global Scheduler/Learning cohorts.
-- Worker roles receive function-only authority; provisioning and human Promote
-- remain outside both worker roles.

DO $roles$
DECLARE role_name TEXT; can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create FROM pg_roles WHERE rolname=current_user;
  FOREACH role_name IN ARRAY ARRAY['agent_cron_worker','agent_learning_worker'] LOOP
    IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF NOT can_create THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_WORKER_REQUIRED_ROLE_MISSING';
      END IF;
      EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',role_name);
    END IF;
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name AND
      (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_WORKER_ROLE_MUST_BE_RESTRICTED';
    END IF;
  END LOOP;
  IF pg_has_role('agent_cron_worker','agent_cron_owner','MEMBER')
     OR pg_has_role('agent_cron_worker','agent_cron_control','MEMBER')
     OR pg_has_role('agent_learning_worker','agent_learning_owner','MEMBER')
     OR pg_has_role('agent_learning_worker','agent_learning_control','MEMBER')
     OR pg_has_role('agent_cron_worker','agent_learning_worker','MEMBER')
     OR pg_has_role('agent_learning_worker','agent_cron_worker','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_WORKER_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE TABLE agent_cron_worker_targets(
  activation_id TEXT PRIMARY KEY,
  template_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  revision BIGINT NOT NULL,
  revision_fingerprint TEXT NOT NULL,
  plan_fingerprint TEXT NOT NULL,
  valid_from TIMESTAMPTZ NOT NULL,
  valid_until TIMESTAMPTZ NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  disabled_at TIMESTAMPTZ,
  disabled_actor_id TEXT,
  disabled_reason_code TEXT,
  CONSTRAINT agent_cron_worker_target_revision_fk
    FOREIGN KEY(template_id,revision,revision_fingerprint)
    REFERENCES agent_cron_revisions(template_id,revision,revision_fingerprint) ON DELETE RESTRICT,
  CONSTRAINT agent_cron_worker_target_id CHECK(activation_id~'^activation_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_cron_worker_target_fingerprints CHECK(
    revision_fingerprint~'^sha256:[a-f0-9]{64}$' AND plan_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_cron_worker_target_actor CHECK(
    actor_type='operator' AND octet_length(actor_id) BETWEEN 1 AND 128
    AND reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_cron_worker_target_window CHECK(valid_until>valid_from AND valid_until<=valid_from+interval '7 days'),
  CONSTRAINT agent_cron_worker_target_state CHECK(
    (enabled AND disabled_at IS NULL AND disabled_actor_id IS NULL AND disabled_reason_code IS NULL)
    OR (NOT enabled AND disabled_at IS NOT NULL
      AND octet_length(disabled_actor_id) BETWEEN 1 AND 128
      AND disabled_reason_code~'^[A-Z][A-Z0-9_]{0,63}$')),
  UNIQUE(template_id,revision,revision_fingerprint)
);

CREATE TABLE agent_learning_worker_targets(
  activation_id TEXT PRIMARY KEY,
  draft_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  draft_fingerprint TEXT NOT NULL,
  proposed_package_fingerprint TEXT NOT NULL,
  runtime_bundle_fingerprint TEXT NOT NULL,
  archive_fingerprint TEXT NOT NULL,
  runner_snapshot_fingerprint TEXT NOT NULL,
  workspace_snapshot_id TEXT NOT NULL,
  workspace_fingerprint TEXT NOT NULL,
  isolation_suite_fingerprint TEXT NOT NULL,
  evaluation_suite_fingerprint TEXT NOT NULL,
  plan_fingerprint TEXT NOT NULL,
  valid_from TIMESTAMPTZ NOT NULL,
  valid_until TIMESTAMPTZ NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  disabled_at TIMESTAMPTZ,
  disabled_actor_id TEXT,
  disabled_reason_code TEXT,
  CONSTRAINT agent_learning_worker_target_draft_fk
    FOREIGN KEY(draft_id,user_id,draft_fingerprint)
    REFERENCES agent_learning_drafts(id,user_id,draft_fingerprint) ON DELETE RESTRICT,
  CONSTRAINT agent_learning_worker_target_id CHECK(activation_id~'^activation_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_learning_worker_target_ids CHECK(workspace_snapshot_id~'^workspace_snapshot_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_learning_worker_target_fingerprints CHECK(
    draft_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND proposed_package_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND runtime_bundle_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND archive_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND runner_snapshot_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND workspace_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND isolation_suite_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND evaluation_suite_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND plan_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_learning_worker_target_actor CHECK(
    actor_type='operator' AND octet_length(actor_id) BETWEEN 1 AND 128
    AND reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_learning_worker_target_window CHECK(valid_until>valid_from AND valid_until<=valid_from+interval '7 days'),
  CONSTRAINT agent_learning_worker_target_state CHECK(
    (enabled AND disabled_at IS NULL AND disabled_actor_id IS NULL AND disabled_reason_code IS NULL)
    OR (NOT enabled AND disabled_at IS NOT NULL
      AND octet_length(disabled_actor_id) BETWEEN 1 AND 128
      AND disabled_reason_code~'^[A-Z][A-Z0-9_]{0,63}$')),
  UNIQUE(draft_id,draft_fingerprint),
  UNIQUE(activation_id,draft_id,user_id,draft_fingerprint)
);

CREATE FUNCTION agent_worker_target_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_TARGET_IMMUTABLE';
  END IF;
  IF to_jsonb(NEW)-ARRAY['enabled','disabled_at','disabled_actor_id','disabled_reason_code']::text[]
       <>to_jsonb(OLD)-ARRAY['enabled','disabled_at','disabled_actor_id','disabled_reason_code']::text[]
     OR NOT OLD.enabled OR NEW.enabled OR NEW.disabled_at IS NULL
     OR NEW.disabled_actor_id IS NULL OR octet_length(NEW.disabled_actor_id) NOT BETWEEN 1 AND 128
     OR NEW.disabled_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_WORKER_TARGET_IMMUTABLE';
  END IF;
  RETURN NEW;
END
$function$;

CREATE TRIGGER trg_agent_cron_worker_target_guard
BEFORE UPDATE OR DELETE ON agent_cron_worker_targets
FOR EACH ROW EXECUTE FUNCTION agent_worker_target_guard();
CREATE TRIGGER trg_agent_learning_worker_target_guard
BEFORE UPDATE OR DELETE ON agent_learning_worker_targets
FOR EACH ROW EXECUTE FUNCTION agent_worker_target_guard();

CREATE FUNCTION agent_cron_worker_provision_target(
  p_activation_id TEXT,p_template_id TEXT,p_user_id UUID,p_revision BIGINT,
  p_revision_fingerprint TEXT,p_plan_fingerprint TEXT,p_valid_from TIMESTAMPTZ,
  p_valid_until TIMESTAMPTZ,p_actor_id TEXT,p_reason_code TEXT
) RETURNS agent_cron_worker_targets
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE target agent_cron_worker_targets%ROWTYPE; template agent_cron_templates%ROWTYPE;
BEGIN
  IF p_activation_id!~'^activation_[a-z0-9]{16,64}$'
     OR p_revision<1 OR p_revision_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_plan_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_valid_until<=p_valid_from OR p_valid_until>p_valid_from+interval '7 days'
     OR p_actor_id IS NULL OR octet_length(p_actor_id) NOT BETWEEN 1 AND 128
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_WORKER_TARGET_INVALID';
  END IF;
  SELECT item.* INTO target FROM agent_cron_worker_targets item
  WHERE item.activation_id=p_activation_id;
  IF FOUND THEN
    IF target.template_id<>p_template_id OR target.user_id<>p_user_id
       OR target.revision<>p_revision OR target.revision_fingerprint<>p_revision_fingerprint
       OR target.plan_fingerprint<>p_plan_fingerprint OR target.valid_from<>p_valid_from
       OR target.valid_until<>p_valid_until OR target.actor_id<>p_actor_id
       OR target.reason_code<>p_reason_code THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN target;
  END IF;
  SELECT item.* INTO template FROM agent_cron_templates item WHERE item.id=p_template_id FOR SHARE;
  IF NOT FOUND OR template.user_id<>p_user_id OR template.state<>'active'
     OR template.current_revision<>p_revision
     OR template.current_revision_fingerprint<>p_revision_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_WORKER_TARGET_DRIFT';
  END IF;
  INSERT INTO agent_cron_worker_targets(
    activation_id,template_id,user_id,revision,revision_fingerprint,plan_fingerprint,
    valid_from,valid_until,actor_type,actor_id,reason_code)
  VALUES(p_activation_id,p_template_id,p_user_id,p_revision,p_revision_fingerprint,
    p_plan_fingerprint,p_valid_from,p_valid_until,'operator',p_actor_id,p_reason_code)
  RETURNING * INTO target;
  RETURN target;
END
$function$;

CREATE FUNCTION agent_cron_worker_disable_target(p_activation_id TEXT,p_actor_id TEXT,p_reason_code TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE target agent_cron_worker_targets%ROWTYPE;
BEGIN
  IF p_actor_id IS NULL OR octet_length(p_actor_id) NOT BETWEEN 1 AND 128
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_WORKER_TARGET_INVALID';
  END IF;
  SELECT item.* INTO target FROM agent_cron_worker_targets item
  WHERE item.activation_id=p_activation_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_WORKER_TARGET_NOT_FOUND'; END IF;
  IF NOT target.enabled THEN RETURN false; END IF;
  UPDATE agent_cron_worker_targets SET enabled=false,disabled_at=clock_timestamp(),
    disabled_actor_id=p_actor_id,disabled_reason_code=p_reason_code
  WHERE activation_id=p_activation_id;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_worker_provision_target(
  p_activation_id TEXT,p_draft_id TEXT,p_user_id UUID,p_draft_fingerprint TEXT,
  p_proposed_package_fingerprint TEXT,p_runtime_bundle_fingerprint TEXT,
  p_archive_fingerprint TEXT,p_runner_snapshot_fingerprint TEXT,
  p_workspace_snapshot_id TEXT,p_workspace_fingerprint TEXT,
  p_isolation_suite_fingerprint TEXT,p_evaluation_suite_fingerprint TEXT,
  p_plan_fingerprint TEXT,p_valid_from TIMESTAMPTZ,p_valid_until TIMESTAMPTZ,
  p_actor_id TEXT,p_reason_code TEXT
) RETURNS agent_learning_worker_targets
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE target agent_learning_worker_targets%ROWTYPE; draft agent_learning_drafts%ROWTYPE;
BEGIN
  IF p_activation_id!~'^activation_[a-z0-9]{16,64}$'
     OR p_draft_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_proposed_package_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_runtime_bundle_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_archive_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_runner_snapshot_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_workspace_snapshot_id!~'^workspace_snapshot_[a-z0-9]{16,64}$'
     OR p_workspace_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_isolation_suite_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_evaluation_suite_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_plan_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_valid_until<=p_valid_from OR p_valid_until>p_valid_from+interval '7 days'
     OR p_actor_id IS NULL OR octet_length(p_actor_id) NOT BETWEEN 1 AND 128
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_WORKER_TARGET_INVALID';
  END IF;
  SELECT item.* INTO target FROM agent_learning_worker_targets item
  WHERE item.activation_id=p_activation_id;
  IF FOUND THEN
    IF target.draft_id<>p_draft_id OR target.user_id<>p_user_id
       OR target.draft_fingerprint<>p_draft_fingerprint
       OR target.proposed_package_fingerprint<>p_proposed_package_fingerprint
       OR target.runtime_bundle_fingerprint<>p_runtime_bundle_fingerprint
       OR target.archive_fingerprint<>p_archive_fingerprint
       OR target.runner_snapshot_fingerprint<>p_runner_snapshot_fingerprint
       OR target.workspace_snapshot_id<>p_workspace_snapshot_id
       OR target.workspace_fingerprint<>p_workspace_fingerprint
       OR target.isolation_suite_fingerprint<>p_isolation_suite_fingerprint
       OR target.evaluation_suite_fingerprint<>p_evaluation_suite_fingerprint
       OR target.plan_fingerprint<>p_plan_fingerprint OR target.valid_from<>p_valid_from
       OR target.valid_until<>p_valid_until OR target.actor_id<>p_actor_id
       OR target.reason_code<>p_reason_code THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN target;
  END IF;
  SELECT item.* INTO draft FROM agent_learning_drafts item WHERE item.id=p_draft_id FOR SHARE;
  IF NOT FOUND OR draft.user_id<>p_user_id OR draft.state<>'quarantined'
     OR draft.draft_fingerprint<>p_draft_fingerprint
     OR draft.proposed_package_fingerprint<>p_proposed_package_fingerprint
     OR draft.spec->>'runtimeBundleFingerprint'<>p_runtime_bundle_fingerprint
     OR draft.spec->>'archiveFingerprint'<>p_archive_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_WORKER_TARGET_DRIFT';
  END IF;
  INSERT INTO agent_learning_worker_targets(
    activation_id,draft_id,user_id,draft_fingerprint,proposed_package_fingerprint,
    runtime_bundle_fingerprint,archive_fingerprint,runner_snapshot_fingerprint,
    workspace_snapshot_id,workspace_fingerprint,isolation_suite_fingerprint,
    evaluation_suite_fingerprint,plan_fingerprint,valid_from,valid_until,
    actor_type,actor_id,reason_code)
  VALUES(p_activation_id,p_draft_id,p_user_id,p_draft_fingerprint,
    p_proposed_package_fingerprint,p_runtime_bundle_fingerprint,p_archive_fingerprint,
    p_runner_snapshot_fingerprint,p_workspace_snapshot_id,p_workspace_fingerprint,
    p_isolation_suite_fingerprint,p_evaluation_suite_fingerprint,p_plan_fingerprint,
    p_valid_from,p_valid_until,'operator',p_actor_id,p_reason_code)
  RETURNING * INTO target;
  RETURN target;
END
$function$;

CREATE FUNCTION agent_learning_worker_disable_target(p_activation_id TEXT,p_actor_id TEXT,p_reason_code TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE target agent_learning_worker_targets%ROWTYPE;
BEGIN
  IF p_actor_id IS NULL OR octet_length(p_actor_id) NOT BETWEEN 1 AND 128
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_WORKER_TARGET_INVALID';
  END IF;
  SELECT item.* INTO target FROM agent_learning_worker_targets item
  WHERE item.activation_id=p_activation_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_WORKER_TARGET_NOT_FOUND'; END IF;
  IF NOT target.enabled THEN RETURN false; END IF;
  UPDATE agent_learning_worker_targets SET enabled=false,disabled_at=clock_timestamp(),
    disabled_actor_id=p_actor_id,disabled_reason_code=p_reason_code
  WHERE activation_id=p_activation_id;
  RETURN true;
END
$function$;

-- Worker-readable target projections expose only content-free bindings.
CREATE FUNCTION agent_cron_worker_get_target(p_activation_id TEXT)
RETURNS SETOF agent_cron_worker_targets
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT target.* FROM agent_cron_worker_targets target
  WHERE target.activation_id=p_activation_id
$function$;

CREATE FUNCTION agent_learning_worker_get_target(p_activation_id TEXT)
RETURNS SETOF agent_learning_worker_targets
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT target.* FROM agent_learning_worker_targets target
  WHERE target.activation_id=p_activation_id
$function$;

-- Cron claims put activation membership in the same locking statement. No
-- globally claimed row is filtered or released later in Go.
CREATE FUNCTION agent_cron_worker_claim_due(
  p_activation_id TEXT,p_claim_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER
) RETURNS TABLE(template_id TEXT,user_id UUID,revision BIGINT,revision_fingerprint TEXT,
  next_trigger_at TIMESTAMPTZ,claim_generation BIGINT,claim_owner TEXT,
  claim_expires_at TIMESTAMPTZ,spec JSONB)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_claim_owner) NOT BETWEEN 1 AND 128
     OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_CLAIM_INVALID';
  END IF;
  RETURN QUERY WITH candidates AS(
    SELECT template.id FROM agent_cron_worker_targets target
    JOIN agent_cron_templates template ON template.id=target.template_id
      AND template.user_id=target.user_id
      AND template.current_revision=target.revision
      AND template.current_revision_fingerprint=target.revision_fingerprint
    WHERE target.activation_id=p_activation_id AND target.enabled
      AND p_now>=target.valid_from AND p_now<target.valid_until
      AND template.state='active' AND template.next_trigger_at<=p_now
      AND (template.cursor_claim_owner IS NULL OR template.cursor_claim_expires_at<=p_now)
    ORDER BY template.next_trigger_at,template.id FOR UPDATE OF template SKIP LOCKED LIMIT p_limit
  ),claimed AS(
    UPDATE agent_cron_templates template
    SET cursor_claim_generation=template.cursor_claim_generation+1,
      cursor_claim_owner=p_claim_owner,
      cursor_claim_expires_at=p_now+make_interval(secs=>p_lease_seconds),
      updated_at=GREATEST(template.updated_at,p_now)
    FROM candidates WHERE template.id=candidates.id RETURNING template.*
  ) SELECT claimed.id,claimed.user_id,claimed.current_revision,
      claimed.current_revision_fingerprint,claimed.next_trigger_at,
      claimed.cursor_claim_generation,claimed.cursor_claim_owner,
      claimed.cursor_claim_expires_at,revision.spec
    FROM claimed JOIN agent_cron_revisions revision
      ON revision.template_id=claimed.id AND revision.revision=claimed.current_revision
    ORDER BY claimed.next_trigger_at,claimed.id;
END
$function$;

CREATE FUNCTION agent_cron_worker_advance_cursor(
  p_activation_id TEXT,p_template_id TEXT,p_revision BIGINT,p_revision_fingerprint TEXT,
  p_claim_owner TEXT,p_claim_generation BIGINT,p_expected_cursor TIMESTAMPTZ,
  p_observed_at TIMESTAMPTZ,p_next_trigger_at TIMESTAMPTZ,p_decisions JSONB
) RETURNS SETOF agent_cron_triggers
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM agent_cron_worker_targets target
    WHERE target.activation_id=p_activation_id AND target.enabled
      AND p_observed_at>=target.valid_from AND p_observed_at<target.valid_until
      AND target.template_id=p_template_id AND target.revision=p_revision
      AND target.revision_fingerprint=p_revision_fingerprint) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_WORKER_TARGET_DRIFT';
  END IF;
  RETURN QUERY SELECT * FROM agent_cron_advance_cursor(
    p_template_id,p_revision,p_revision_fingerprint,p_claim_owner,p_claim_generation,
    p_expected_cursor,p_observed_at,p_next_trigger_at,p_decisions);
END
$function$;

CREATE FUNCTION agent_cron_worker_claim_triggers(
  p_activation_id TEXT,p_claim_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER
) RETURNS TABLE(trigger_id TEXT,template_id TEXT,user_id UUID,revision BIGINT,
  revision_fingerprint TEXT,scheduled_for TIMESTAMPTZ,occurrence_fingerprint TEXT,
  state TEXT,reason_code TEXT,run_id TEXT,retry_count INTEGER,next_attempt_at TIMESTAMPTZ,
  claim_generation BIGINT,claim_owner TEXT,claim_expires_at TIMESTAMPTZ,spec JSONB)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_claim_owner) NOT BETWEEN 1 AND 128
     OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_CLAIM_INVALID';
  END IF;
  RETURN QUERY WITH candidates AS(
    SELECT trigger.id FROM agent_cron_worker_targets target
    JOIN agent_cron_triggers trigger ON trigger.template_id=target.template_id
      AND trigger.user_id=target.user_id AND trigger.revision=target.revision
      AND trigger.revision_fingerprint=target.revision_fingerprint
    WHERE target.activation_id=p_activation_id AND target.enabled
      AND p_now>=target.valid_from AND p_now<target.valid_until
      AND ((trigger.state='pending' AND trigger.next_attempt_at<=p_now)
        OR (trigger.state='claimed' AND trigger.claim_expires_at<=p_now))
      AND NOT EXISTS(SELECT 1 FROM agent_cron_triggers older
        WHERE older.template_id=trigger.template_id AND older.id<>trigger.id
          AND older.state IN ('pending','claimed')
          AND (older.scheduled_for,older.id)<(trigger.scheduled_for,trigger.id))
    ORDER BY trigger.next_attempt_at,trigger.scheduled_for,trigger.id
    FOR UPDATE OF trigger SKIP LOCKED LIMIT p_limit
  ),claimed AS(
    UPDATE agent_cron_triggers trigger SET state='claimed',
      claim_generation=trigger.claim_generation+1,claim_owner=p_claim_owner,
      claim_expires_at=p_now+make_interval(secs=>p_lease_seconds),
      updated_at=GREATEST(trigger.updated_at,p_now)
    FROM candidates WHERE trigger.id=candidates.id RETURNING trigger.*
  ) SELECT claimed.id,claimed.template_id,claimed.user_id,claimed.revision,
      claimed.revision_fingerprint,claimed.scheduled_for,claimed.occurrence_fingerprint,
      claimed.state,claimed.reason_code,COALESCE(claimed.run_id,''),claimed.retry_count,
      claimed.next_attempt_at,claimed.claim_generation,claimed.claim_owner,
      claimed.claim_expires_at,revision.spec
    FROM claimed JOIN agent_cron_revisions revision
      ON revision.template_id=claimed.template_id AND revision.revision=claimed.revision
    ORDER BY claimed.next_attempt_at,claimed.scheduled_for,claimed.id;
END
$function$;

CREATE FUNCTION agent_cron_worker_enqueue_trigger(
  p_activation_id TEXT,p_trigger_id TEXT,p_claim_owner TEXT,p_claim_generation BIGINT,
  p_run_id TEXT,p_snapshot_id TEXT,p_snapshot_fingerprint TEXT,p_request_fingerprint TEXT,
  p_canonical_snapshot JSONB,p_steps JSONB,p_event_ids TEXT[],p_audit_event_id TEXT
) RETURNS TABLE(trigger_id TEXT,state TEXT,reason_code TEXT,run_id TEXT,created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM agent_cron_worker_targets target
    JOIN agent_cron_triggers trigger ON trigger.id=p_trigger_id
      AND trigger.template_id=target.template_id AND trigger.user_id=target.user_id
      AND trigger.revision=target.revision
      AND trigger.revision_fingerprint=target.revision_fingerprint
    WHERE target.activation_id=p_activation_id AND target.enabled
      AND clock_timestamp()>=target.valid_from AND clock_timestamp()<target.valid_until) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_WORKER_TARGET_DRIFT';
  END IF;
  RETURN QUERY SELECT * FROM agent_cron_enqueue_trigger(
    p_trigger_id,p_claim_owner,p_claim_generation,p_run_id,p_snapshot_id,
    p_snapshot_fingerprint,p_request_fingerprint,p_canonical_snapshot,p_steps,
    p_event_ids,p_audit_event_id);
END
$function$;

CREATE FUNCTION agent_cron_worker_release_trigger(
  p_activation_id TEXT,p_trigger_id TEXT,p_claim_owner TEXT,p_claim_generation BIGINT,
  p_error_code TEXT,p_retry_at TIMESTAMPTZ,p_audit_event_id TEXT
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM agent_cron_worker_targets target
    JOIN agent_cron_triggers trigger ON trigger.id=p_trigger_id
      AND trigger.template_id=target.template_id AND trigger.user_id=target.user_id
      AND trigger.revision=target.revision
      AND trigger.revision_fingerprint=target.revision_fingerprint
    WHERE target.activation_id=p_activation_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_CRON_WORKER_TARGET_DRIFT';
  END IF;
  RETURN agent_cron_release_trigger(p_trigger_id,p_claim_owner,p_claim_generation,
    p_error_code,p_retry_at,p_audit_event_id);
END
$function$;

CREATE FUNCTION agent_cron_worker_reconcile(
  p_activation_id TEXT,p_now TIMESTAMPTZ,p_limit INTEGER
) RETURNS TABLE(cursor_claims_reclaimed INTEGER,trigger_claims_reclaimed INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE cursor_count INTEGER:=0; trigger_count INTEGER:=0; item RECORD;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_LIMIT_INVALID';
  END IF;
  WITH targets AS(
    SELECT template.id FROM agent_cron_worker_targets target
    JOIN agent_cron_templates template ON template.id=target.template_id
      AND template.user_id=target.user_id AND template.current_revision=target.revision
      AND template.current_revision_fingerprint=target.revision_fingerprint
    WHERE target.activation_id=p_activation_id AND template.cursor_claim_expires_at<=p_now
    ORDER BY template.cursor_claim_expires_at,template.id
    FOR UPDATE OF template SKIP LOCKED LIMIT p_limit
  ) UPDATE agent_cron_templates template SET cursor_claim_owner=NULL,
    cursor_claim_expires_at=NULL,updated_at=GREATEST(template.updated_at,p_now)
    FROM targets WHERE template.id=targets.id;
  GET DIAGNOSTICS cursor_count=ROW_COUNT;
  FOR item IN SELECT trigger.id,trigger.template_id,trigger.user_id,trigger.revision,
      trigger.claim_generation
    FROM agent_cron_worker_targets target
    JOIN agent_cron_triggers trigger ON trigger.template_id=target.template_id
      AND trigger.user_id=target.user_id AND trigger.revision=target.revision
      AND trigger.revision_fingerprint=target.revision_fingerprint
    WHERE target.activation_id=p_activation_id AND trigger.state='claimed'
      AND trigger.claim_expires_at<=p_now
    ORDER BY trigger.claim_expires_at,trigger.id
    FOR UPDATE OF trigger SKIP LOCKED LIMIT p_limit LOOP
    UPDATE agent_cron_triggers SET state='pending',reason_code='CLAIM_RECLAIMED',
      claim_owner=NULL,claim_expires_at=NULL,next_attempt_at=p_now,
      updated_at=GREATEST(updated_at,p_now) WHERE id=item.id;
    INSERT INTO agent_cron_audit_events(event_id,template_id,user_id,revision,
      trigger_id,kind,actor_type,actor_id,reason_code,detail,occurred_at)
    VALUES('cron_event_'||substr(md5(item.id||'|'||item.claim_generation::text||'|activation-reclaim'),1,32),
      item.template_id,item.user_id,item.revision,item.id,'claim.reclaimed','scheduler',
      'reconciler','CLAIM_RECLAIMED',jsonb_build_object('claimGeneration',item.claim_generation),p_now)
    ON CONFLICT(event_id) DO NOTHING;
    trigger_count:=trigger_count+1;
  END LOOP;
  RETURN QUERY SELECT cursor_count,trigger_count;
END
$function$;

CREATE FUNCTION agent_cron_worker_prune(
  p_activation_id TEXT,p_cutoff TIMESTAMPTZ,p_limit INTEGER
) RETURNS TABLE(triggers_pruned INTEGER,audits_pruned INTEGER,templates_pruned INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE trigger_count INTEGER:=0; audit_count INTEGER:=0;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 OR p_cutoff>clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_CRON_PRUNE_INVALID';
  END IF;
  WITH targets AS(SELECT trigger.id FROM agent_cron_worker_targets target
    JOIN agent_cron_triggers trigger ON trigger.template_id=target.template_id
      AND trigger.user_id=target.user_id AND trigger.revision=target.revision
      AND trigger.revision_fingerprint=target.revision_fingerprint
    LEFT JOIN agent_runs run ON run.id=trigger.run_id
    WHERE target.activation_id=p_activation_id AND trigger.terminal_at<p_cutoff
      AND (trigger.state IN ('skipped','failed') OR (trigger.state='enqueued'
        AND (run.id IS NULL OR run.state IN ('succeeded','failed','canceled','killed','outcome_unknown'))))
    ORDER BY trigger.terminal_at,trigger.id LIMIT p_limit)
  DELETE FROM agent_cron_triggers trigger USING targets WHERE trigger.id=targets.id;
  GET DIAGNOSTICS trigger_count=ROW_COUNT;
  WITH targets AS(SELECT event.event_id FROM agent_cron_worker_targets target
    JOIN agent_cron_audit_events event ON event.template_id=target.template_id
      AND event.user_id=target.user_id AND event.revision=target.revision
    WHERE target.activation_id=p_activation_id AND event.occurred_at<p_cutoff
    ORDER BY event.occurred_at,event.event_id LIMIT p_limit)
  DELETE FROM agent_cron_audit_events event USING targets WHERE event.event_id=targets.event_id;
  GET DIAGNOSTICS audit_count=ROW_COUNT;
  RETURN QUERY SELECT trigger_count,audit_count,0;
END
$function$;

-- Draft-only Runner transport. These attempts are deliberately not foreign
-- keys to agent_runs/agent_attempts and can never reach Broker authority.
CREATE FUNCTION agent_learning_runner_result_valid(p_value JSONB)
RETURNS BOOLEAN LANGUAGE sql IMMUTABLE SET search_path FROM CURRENT AS $function$
  SELECT jsonb_typeof(p_value)='object' AND octet_length(p_value::text)<=16384
    AND p_value-ARRAY['schemaVersion','activationId','draftId','draftFingerprint',
      'checkGeneration','kind','proposedPackageFingerprint','runtimeBundleFingerprint',
      'archiveFingerprint','workspaceSnapshotId','workspaceFingerprint','suiteFingerprint',
      'status','reasonCode','durationMillis','metrics']::text[]='{}'::jsonb
    AND p_value->>'schemaVersion'='neo.agent-draft-runner-result/v1'
    AND p_value->>'activationId'~'^activation_[a-z0-9]{16,64}$'
    AND p_value->>'draftId'~'^draft_[a-z0-9]{16,64}$'
    AND p_value->>'draftFingerprint'~'^sha256:[a-f0-9]{64}$'
    AND jsonb_typeof(p_value->'checkGeneration')='number'
    AND (p_value->>'checkGeneration')::bigint>=1
    AND p_value->>'kind' IN ('isolation','evaluation')
    AND p_value->>'proposedPackageFingerprint'~'^sha256:[a-f0-9]{64}$'
    AND p_value->>'runtimeBundleFingerprint'~'^sha256:[a-f0-9]{64}$'
    AND p_value->>'archiveFingerprint'~'^sha256:[a-f0-9]{64}$'
    AND p_value->>'workspaceSnapshotId'~'^workspace_snapshot_[a-z0-9]{16,64}$'
    AND p_value->>'workspaceFingerprint'~'^sha256:[a-f0-9]{64}$'
    AND p_value->>'suiteFingerprint'~'^sha256:[a-f0-9]{64}$'
    AND p_value->>'status' IN ('passed','failed')
    AND p_value->>'reasonCode'~'^[A-Z][A-Z0-9_]{0,63}$'
    AND jsonb_typeof(p_value->'durationMillis')='number'
    AND (p_value->>'durationMillis')::bigint BETWEEN 0 AND 86400000
    AND agent_learning_metrics_valid(p_value->'metrics')
$function$;

CREATE TABLE agent_learning_runner_attempts(
  attempt_id TEXT PRIMARY KEY,
  activation_id TEXT NOT NULL REFERENCES agent_learning_worker_targets(activation_id) ON DELETE RESTRICT,
  draft_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  draft_fingerprint TEXT NOT NULL,
  check_generation BIGINT NOT NULL,
  kind TEXT NOT NULL,
  transport_run_id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  lease_generation BIGINT NOT NULL,
  lease_owner TEXT NOT NULL,
  lease_token_hash TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'leased',
  sandbox_id TEXT,
  spec_fingerprint TEXT,
  probe_fingerprint TEXT,
  cleanup_state TEXT NOT NULL DEFAULT 'none',
  cleanup_attempts INTEGER NOT NULL DEFAULT 0,
  error_code TEXT,
  lease_expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  terminal_at TIMESTAMPTZ,
  CONSTRAINT agent_learning_runner_attempt_target_fk
    FOREIGN KEY(activation_id,draft_id,user_id,draft_fingerprint)
    REFERENCES agent_learning_worker_targets(activation_id,draft_id,user_id,draft_fingerprint) ON DELETE RESTRICT,
  CONSTRAINT agent_learning_runner_attempt_ids CHECK(
    attempt_id~'^attempt_[a-z0-9]{16,64}$' AND transport_run_id~'^run_[a-z0-9]{16,64}$'
    AND step_id~'^step_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_learning_runner_attempt_shape CHECK(
    check_generation>=1 AND kind IN ('isolation','evaluation')
    AND lease_generation>=1 AND octet_length(lease_owner) BETWEEN 1 AND 256
    AND lease_token_hash~'^[a-f0-9]{64}$'
    AND snapshot_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND state IN ('leased','launched','result_ready','cancel_pending','completed','failed')
    AND cleanup_state IN ('none','pending','failed','completed')
    AND cleanup_attempts BETWEEN 0 AND 10
    AND (error_code IS NULL OR error_code~'^[A-Z][A-Z0-9_]{0,63}$')
    AND ((state='leased' AND sandbox_id IS NULL AND spec_fingerprint IS NULL AND probe_fingerprint IS NULL)
      OR (state='failed' AND sandbox_id IS NULL AND spec_fingerprint IS NULL AND probe_fingerprint IS NULL)
      OR (state<>'leased' AND sandbox_id~'^sandbox_[a-z0-9]{16,64}$'
        AND spec_fingerprint~'^sha256:[a-f0-9]{64}$'
        AND probe_fingerprint~'^sha256:[a-f0-9]{64}$'))
    AND ((state IN ('completed','failed') AND terminal_at IS NOT NULL)
      OR (state NOT IN ('completed','failed') AND terminal_at IS NULL))
    AND ((cleanup_state='none' AND state IN ('leased','launched'))
      OR (cleanup_state='pending' AND state IN ('result_ready','cancel_pending'))
      OR (cleanup_state='failed' AND state='cancel_pending')
      OR (cleanup_state='completed' AND state IN ('completed','failed')))),
  CONSTRAINT agent_learning_runner_attempt_times CHECK(updated_at>=created_at
    AND lease_expires_at>created_at AND (terminal_at IS NULL OR terminal_at>=created_at)),
  UNIQUE(activation_id,draft_id,check_generation,kind),
  UNIQUE(transport_run_id,step_id,attempt_id,lease_generation)
);

CREATE TABLE agent_learning_runner_results(
  attempt_id TEXT PRIMARY KEY REFERENCES agent_learning_runner_attempts(attempt_id) ON DELETE CASCADE,
  activation_id TEXT NOT NULL,
  draft_id TEXT NOT NULL,
  draft_fingerprint TEXT NOT NULL,
  check_generation BIGINT NOT NULL,
  kind TEXT NOT NULL,
  artifact_name TEXT NOT NULL,
  artifact_bytes BIGINT NOT NULL,
  artifact_fingerprint TEXT NOT NULL,
  result JSONB NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_learning_runner_result_binding_fk
    FOREIGN KEY(activation_id,draft_id,check_generation,kind)
    REFERENCES agent_learning_runner_attempts(activation_id,draft_id,check_generation,kind) ON DELETE CASCADE,
  CONSTRAINT agent_learning_runner_result_shape CHECK(
    artifact_name='draft-check-result.json' AND artifact_bytes BETWEEN 2 AND 65536
    AND artifact_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND agent_learning_runner_result_valid(result)
    AND activation_id=result->>'activationId' AND draft_id=result->>'draftId'
    AND draft_fingerprint=result->>'draftFingerprint'
    AND check_generation=(result->>'checkGeneration')::bigint AND kind=result->>'kind'),
  UNIQUE(activation_id,draft_id,check_generation,kind)
);

CREATE TABLE agent_learning_runner_requests(
  caller_identity TEXT NOT NULL,
  request_id TEXT NOT NULL,
  nonce TEXT NOT NULL,
  activation_id TEXT NOT NULL,
  runner_id TEXT NOT NULL,
  method TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  attempt_id TEXT NOT NULL REFERENCES agent_learning_runner_attempts(attempt_id) ON DELETE CASCADE,
  transport_run_id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  lease_generation BIGINT NOT NULL,
  lease_owner TEXT NOT NULL,
  lease_token_hash TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'claimed',
  response_fingerprint TEXT,
  response_body JSONB,
  issued_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  completed_at TIMESTAMPTZ,
  PRIMARY KEY(caller_identity,request_id,nonce),
  CONSTRAINT agent_learning_runner_request_shape CHECK(
    caller_identity='spiffe://neo-chat/agent-runtime-draft-learning'
    AND octet_length(runner_id) BETWEEN 1 AND 256 AND runner_id=lease_owner
    AND request_id~'^rpc_[a-z0-9]{16,64}$' AND nonce~'^[A-Za-z0-9_-]{16,128}$'
    AND method IN ('launch','result','cancel')
    AND request_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND lease_generation>=1 AND lease_token_hash~'^[a-f0-9]{64}$'
    AND snapshot_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND status IN ('claimed','completed') AND expires_at>issued_at
    AND ((status='claimed' AND response_fingerprint IS NULL AND response_body IS NULL AND completed_at IS NULL)
      OR (status='completed' AND response_fingerprint~'^sha256:[a-f0-9]{64}$'
        AND response_body IS NOT NULL AND completed_at IS NOT NULL)))
);
CREATE INDEX idx_agent_learning_runner_request_attempt
  ON agent_learning_runner_requests(attempt_id,issued_at,request_id);

CREATE FUNCTION agent_learning_runner_result_immutable() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  IF TG_OP='DELETE' AND (pg_trigger_depth()>1 OR current_user='agent_learning_owner') THEN RETURN OLD; END IF;
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_LEARNING_RUNNER_RESULT_IMMUTABLE';
END
$function$;
CREATE TRIGGER trg_agent_learning_runner_result_immutable
BEFORE UPDATE OR DELETE ON agent_learning_runner_results
FOR EACH ROW EXECUTE FUNCTION agent_learning_runner_result_immutable();

-- The learning worker receives a complete projection through functions and no
-- direct SELECT on Draft/package tables.
CREATE FUNCTION agent_learning_worker_get_draft(p_activation_id TEXT)
RETURNS TABLE(
  id TEXT,user_id TEXT,draft_fingerprint TEXT,spec JSONB,state TEXT,revision BIGINT,
  check_attempts INTEGER,check_generation BIGINT,check_owner TEXT,
  check_expires_at TIMESTAMPTZ,admission_id TEXT,promoted_package_fingerprint TEXT,
  draft_object_key TEXT,base_package_object_key TEXT,created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,object_deleted_at TIMESTAMPTZ
) LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT draft.id,draft.user_id::text,draft.draft_fingerprint,draft.spec,draft.state,
    draft.revision,draft.check_attempts,draft.check_generation,
    COALESCE(draft.check_owner,''),draft.check_expires_at,
    COALESCE(draft.admission_id::text,''),COALESCE(draft.promoted_package_fingerprint,''),
    draft.draft_object_key,base.package_object_key,draft.created_at,draft.updated_at,
    draft.object_deleted_at
  FROM agent_learning_worker_targets target
  JOIN agent_learning_drafts draft ON draft.id=target.draft_id
    AND draft.user_id=target.user_id AND draft.draft_fingerprint=target.draft_fingerprint
  JOIN skill_package_versions base ON base.package_fingerprint=draft.base_package_fingerprint
  WHERE target.activation_id=p_activation_id
$function$;

CREATE FUNCTION agent_learning_worker_claim_checks(
  p_activation_id TEXT,p_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER
) RETURNS TABLE(
  id TEXT,user_id TEXT,draft_fingerprint TEXT,spec JSONB,state TEXT,revision BIGINT,
  check_attempts INTEGER,check_generation BIGINT,check_owner TEXT,
  check_expires_at TIMESTAMPTZ,admission_id TEXT,promoted_package_fingerprint TEXT,
  draft_object_key TEXT,base_package_object_key TEXT,created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,object_deleted_at TIMESTAMPTZ
) LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_owner) NOT BETWEEN 1 AND 128
     OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CLAIM_INVALID';
  END IF;
  RETURN QUERY WITH candidates AS(
    SELECT draft.id FROM agent_learning_worker_targets target
    JOIN agent_learning_drafts draft ON draft.id=target.draft_id
      AND draft.user_id=target.user_id AND draft.draft_fingerprint=target.draft_fingerprint
      AND draft.proposed_package_fingerprint=target.proposed_package_fingerprint
    WHERE target.activation_id=p_activation_id AND target.enabled
      AND p_now>=target.valid_from AND p_now<target.valid_until
      AND draft.state='quarantined' AND draft.next_check_at<=p_now
    ORDER BY draft.next_check_at,draft.id FOR UPDATE OF draft SKIP LOCKED LIMIT p_limit
  ),claimed AS(
    UPDATE agent_learning_drafts draft SET state='checking',
      check_generation=draft.check_generation+1,check_owner=p_owner,
      check_expires_at=p_now+make_interval(secs=>p_lease_seconds),
      updated_at=GREATEST(draft.updated_at,p_now)
    FROM candidates WHERE draft.id=candidates.id RETURNING draft.*
  ) SELECT claimed.id,claimed.user_id::text,claimed.draft_fingerprint,claimed.spec,
      claimed.state,claimed.revision,claimed.check_attempts,claimed.check_generation,
      COALESCE(claimed.check_owner,''),claimed.check_expires_at,
      COALESCE(claimed.admission_id::text,''),COALESCE(claimed.promoted_package_fingerprint,''),
      claimed.draft_object_key,base.package_object_key,claimed.created_at,claimed.updated_at,
      claimed.object_deleted_at
    FROM claimed JOIN skill_package_versions base
      ON base.package_fingerprint=claimed.base_package_fingerprint
    ORDER BY claimed.next_check_at,claimed.id;
END
$function$;

CREATE FUNCTION agent_learning_worker_begin_runner_check(
  p_activation_id TEXT,p_draft_id TEXT,p_claim_owner TEXT,p_check_generation BIGINT,
  p_kind TEXT,p_attempt_id TEXT,p_transport_run_id TEXT,p_step_id TEXT,
  p_lease_generation BIGINT,p_lease_owner TEXT,p_lease_token_hash TEXT,
  p_snapshot_fingerprint TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER
) RETURNS TABLE(attempt_id TEXT,state TEXT,created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE target agent_learning_worker_targets%ROWTYPE; draft agent_learning_drafts%ROWTYPE;
  existing agent_learning_runner_attempts%ROWTYPE;
BEGIN
  IF p_kind NOT IN ('isolation','evaluation')
     OR p_attempt_id!~'^attempt_[a-z0-9]{16,64}$'
     OR p_transport_run_id!~'^run_[a-z0-9]{16,64}$'
     OR p_step_id!~'^step_[a-z0-9]{16,64}$' OR p_lease_generation<1
     OR octet_length(p_lease_owner) NOT BETWEEN 1 AND 256
     OR p_lease_token_hash!~'^[a-f0-9]{64}$'
     OR p_snapshot_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_lease_seconds NOT BETWEEN 5 AND 300 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RUNNER_ATTEMPT_INVALID';
  END IF;
  SELECT item.* INTO target FROM agent_learning_worker_targets item
  WHERE item.activation_id=p_activation_id AND item.enabled
    AND p_now>=item.valid_from AND p_now<item.valid_until;
  IF NOT FOUND OR target.draft_id<>p_draft_id
     OR target.runner_snapshot_fingerprint<>p_snapshot_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_WORKER_TARGET_DRIFT';
  END IF;
  SELECT item.* INTO draft FROM agent_learning_drafts item WHERE item.id=p_draft_id FOR SHARE;
  IF NOT FOUND OR draft.state<>'checking' OR draft.check_owner IS DISTINCT FROM p_claim_owner
     OR draft.check_generation<>p_check_generation OR draft.check_expires_at<=p_now
     OR draft.user_id<>target.user_id OR draft.draft_fingerprint<>target.draft_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';
  END IF;
  SELECT item.* INTO existing FROM agent_learning_runner_attempts item
  WHERE item.activation_id=p_activation_id AND item.draft_id=p_draft_id
    AND item.check_generation=p_check_generation AND item.kind=p_kind FOR UPDATE;
  IF FOUND THEN
    IF existing.attempt_id<>p_attempt_id OR existing.transport_run_id<>p_transport_run_id
       OR existing.step_id<>p_step_id OR existing.lease_generation<>p_lease_generation
       OR existing.lease_owner<>p_lease_owner OR existing.lease_token_hash<>p_lease_token_hash
       OR existing.snapshot_fingerprint<>p_snapshot_fingerprint THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN QUERY SELECT existing.attempt_id,existing.state,false;
    RETURN;
  END IF;
  INSERT INTO agent_learning_runner_attempts(
    attempt_id,activation_id,draft_id,user_id,draft_fingerprint,check_generation,
    kind,transport_run_id,step_id,lease_generation,lease_owner,lease_token_hash,
    snapshot_fingerprint,lease_expires_at,created_at,updated_at)
  VALUES(p_attempt_id,p_activation_id,p_draft_id,target.user_id,target.draft_fingerprint,
    p_check_generation,p_kind,p_transport_run_id,p_step_id,p_lease_generation,
    p_lease_owner,p_lease_token_hash,p_snapshot_fingerprint,
    p_now+make_interval(secs=>p_lease_seconds),p_now,p_now)
  RETURNING agent_learning_runner_attempts.attempt_id,
    agent_learning_runner_attempts.state INTO p_attempt_id,p_kind;
  RETURN QUERY SELECT p_attempt_id,p_kind,true;
END
$function$;

CREATE FUNCTION agent_learning_worker_issue_runner_authority(
  p_activation_id TEXT,p_caller_identity TEXT,p_runner_id TEXT,p_request_id TEXT,
  p_nonce TEXT,p_method TEXT,p_request_fingerprint TEXT,p_attempt_id TEXT,
  p_lease_token_hash TEXT,p_ttl_seconds INTEGER
) RETURNS TABLE(
  runner_id TEXT,caller_identity TEXT,request_id TEXT,method TEXT,run_id TEXT,step_id TEXT,
  attempt_id TEXT,lease_generation BIGINT,lease_owner TEXT,lease_token_hash TEXT,
  snapshot_fingerprint TEXT,kill_switch_epoch BIGINT,issued_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,replayed BOOLEAN,response_body JSONB
) LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE attempt agent_learning_runner_attempts%ROWTYPE;
  request agent_learning_runner_requests%ROWTYPE; target agent_learning_worker_targets%ROWTYPE;
  now_at TIMESTAMPTZ:=clock_timestamp(); denial TEXT;
BEGIN
  IF p_caller_identity<>'spiffe://neo-chat/agent-runtime-draft-learning'
     OR p_runner_id IS NULL OR octet_length(p_runner_id) NOT BETWEEN 1 AND 256
     OR p_request_id!~'^rpc_[a-z0-9]{16,64}$' OR p_nonce!~'^[A-Za-z0-9_-]{16,128}$'
     OR p_method NOT IN ('launch','result','cancel')
     OR p_request_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_lease_token_hash!~'^[a-f0-9]{64}$' OR p_ttl_seconds NOT BETWEEN 1 AND 15 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RUNNER_AUTHORITY_INVALID';
  END IF;
  SELECT item.* INTO request FROM agent_learning_runner_requests item
  WHERE item.caller_identity=p_caller_identity AND item.request_id=p_request_id
    AND item.nonce=p_nonce FOR UPDATE;
  IF FOUND THEN
    IF request.activation_id<>p_activation_id OR request.runner_id<>p_runner_id
       OR request.method<>p_method OR request.request_fingerprint<>p_request_fingerprint
       OR request.attempt_id<>p_attempt_id OR request.lease_token_hash<>p_lease_token_hash THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    IF request.status='claimed' THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN QUERY SELECT request.runner_id,request.caller_identity,request.request_id,
      request.method,request.transport_run_id,request.step_id,request.attempt_id,
      request.lease_generation,request.lease_owner,request.lease_token_hash,
      request.snapshot_fingerprint,0::bigint,request.issued_at,request.expires_at,
      true,request.response_body;
    RETURN;
  END IF;
  SELECT item.* INTO attempt FROM agent_learning_runner_attempts item
  WHERE item.attempt_id=p_attempt_id FOR UPDATE;
  SELECT item.* INTO target FROM agent_learning_worker_targets item
  WHERE item.activation_id=p_activation_id;
  IF attempt.attempt_id IS NULL OR target.activation_id IS NULL
     OR attempt.activation_id<>p_activation_id OR attempt.lease_owner<>p_runner_id
     OR attempt.lease_token_hash<>p_lease_token_hash OR attempt.lease_expires_at<=now_at
     OR (p_method='launch' AND attempt.state<>'leased')
     OR (p_method='result' AND attempt.state NOT IN ('launched','result_ready'))
     OR (p_method='cancel' AND attempt.state NOT IN ('leased','launched','result_ready','cancel_pending'))
     OR (p_method<>'cancel' AND (NOT target.enabled OR now_at<target.valid_from OR now_at>=target.valid_until)) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  IF p_method<>'cancel' THEN
    denial:=agent_learning_source_denial(attempt.draft_id);
    IF denial IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE=denial; END IF;
  END IF;
  INSERT INTO agent_learning_runner_requests(
    caller_identity,request_id,nonce,activation_id,runner_id,method,
    request_fingerprint,attempt_id,transport_run_id,step_id,lease_generation,
    lease_owner,lease_token_hash,snapshot_fingerprint,issued_at,expires_at)
  VALUES(p_caller_identity,p_request_id,p_nonce,p_activation_id,p_runner_id,p_method,
    p_request_fingerprint,p_attempt_id,attempt.transport_run_id,attempt.step_id,
    attempt.lease_generation,attempt.lease_owner,attempt.lease_token_hash,
    attempt.snapshot_fingerprint,now_at,now_at+make_interval(secs=>p_ttl_seconds))
  RETURNING * INTO request;
  RETURN QUERY SELECT request.runner_id,request.caller_identity,request.request_id,
    request.method,request.transport_run_id,request.step_id,request.attempt_id,
    request.lease_generation,request.lease_owner,request.lease_token_hash,
    request.snapshot_fingerprint,0::bigint,request.issued_at,request.expires_at,
    false,NULL::jsonb;
END
$function$;

CREATE FUNCTION agent_learning_worker_complete_runner_request(
  p_caller_identity TEXT,p_request_id TEXT,p_nonce TEXT,p_request_fingerprint TEXT,
  p_response_fingerprint TEXT,p_response_body JSONB
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE request agent_learning_runner_requests%ROWTYPE;
BEGIN
  SELECT item.* INTO request FROM agent_learning_runner_requests item
  WHERE item.caller_identity=p_caller_identity AND item.request_id=p_request_id
    AND item.nonce=p_nonce FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_RUNNER_REQUEST_NOT_FOUND';
  END IF;
  IF request.request_fingerprint<>p_request_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
  END IF;
  IF request.status='completed' THEN
    IF request.response_fingerprint<>p_response_fingerprint
       OR request.response_body<>p_response_body THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN true;
  END IF;
  IF p_response_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR jsonb_typeof(p_response_body)<>'object'
     OR octet_length(p_response_body::text)>131072
     OR NOT agent_learning_document_sanitized(p_response_body) THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_RUNNER_RESPONSE_INVALID';
  END IF;
  UPDATE agent_learning_runner_requests SET status='completed',
    response_fingerprint=p_response_fingerprint,response_body=p_response_body,
    completed_at=clock_timestamp()
  WHERE caller_identity=p_caller_identity AND request_id=p_request_id AND nonce=p_nonce;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_worker_record_runner_launch(
  p_activation_id TEXT,p_attempt_id TEXT,p_lease_generation BIGINT,
  p_sandbox_id TEXT,p_spec_fingerprint TEXT,p_probe_fingerprint TEXT
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE attempt agent_learning_runner_attempts%ROWTYPE;
BEGIN
  SELECT item.* INTO attempt FROM agent_learning_runner_attempts item
  WHERE item.attempt_id=p_attempt_id FOR UPDATE;
  IF NOT FOUND OR attempt.activation_id<>p_activation_id
     OR attempt.lease_generation<>p_lease_generation THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  IF attempt.state<>'leased' THEN
    IF attempt.sandbox_id=p_sandbox_id AND attempt.spec_fingerprint=p_spec_fingerprint
       AND attempt.probe_fingerprint=p_probe_fingerprint THEN RETURN false; END IF;
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
  END IF;
  IF p_sandbox_id!~'^sandbox_[a-z0-9]{16,64}$'
     OR p_spec_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_probe_fingerprint!~'^sha256:[a-f0-9]{64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RUNNER_LAUNCH_INVALID';
  END IF;
  UPDATE agent_learning_runner_attempts SET state='launched',sandbox_id=p_sandbox_id,
    spec_fingerprint=p_spec_fingerprint,probe_fingerprint=p_probe_fingerprint,
    updated_at=clock_timestamp() WHERE attempt_id=p_attempt_id;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_worker_record_runner_result(
  p_activation_id TEXT,p_attempt_id TEXT,p_lease_generation BIGINT,
  p_artifact_name TEXT,p_artifact_bytes BIGINT,p_artifact_fingerprint TEXT,p_result JSONB
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE attempt agent_learning_runner_attempts%ROWTYPE;
  target agent_learning_worker_targets%ROWTYPE; existing agent_learning_runner_results%ROWTYPE;
  expected_suite TEXT;
BEGIN
  SELECT item.* INTO attempt FROM agent_learning_runner_attempts item
  WHERE item.attempt_id=p_attempt_id FOR UPDATE;
  SELECT item.* INTO target FROM agent_learning_worker_targets item
  WHERE item.activation_id=p_activation_id;
  IF attempt.attempt_id IS NULL OR target.activation_id IS NULL
     OR attempt.activation_id<>p_activation_id OR attempt.lease_generation<>p_lease_generation
     OR attempt.state NOT IN ('launched','result_ready') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  SELECT item.* INTO existing FROM agent_learning_runner_results item
  WHERE item.attempt_id=p_attempt_id;
  IF FOUND THEN
    IF existing.artifact_name<>p_artifact_name OR existing.artifact_bytes<>p_artifact_bytes
       OR existing.artifact_fingerprint<>p_artifact_fingerprint OR existing.result<>p_result THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN false;
  END IF;
  expected_suite:=CASE attempt.kind WHEN 'isolation' THEN target.isolation_suite_fingerprint
    ELSE target.evaluation_suite_fingerprint END;
  IF p_artifact_name<>'draft-check-result.json' OR p_artifact_bytes NOT BETWEEN 2 AND 65536
     OR p_artifact_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR NOT agent_learning_runner_result_valid(p_result)
     OR p_result->>'activationId'<>target.activation_id
     OR p_result->>'draftId'<>target.draft_id
     OR p_result->>'draftFingerprint'<>target.draft_fingerprint
     OR (p_result->>'checkGeneration')::bigint<>attempt.check_generation
     OR p_result->>'kind'<>attempt.kind
     OR p_result->>'proposedPackageFingerprint'<>target.proposed_package_fingerprint
     OR p_result->>'runtimeBundleFingerprint'<>target.runtime_bundle_fingerprint
     OR p_result->>'archiveFingerprint'<>target.archive_fingerprint
     OR p_result->>'workspaceSnapshotId'<>target.workspace_snapshot_id
     OR p_result->>'workspaceFingerprint'<>target.workspace_fingerprint
     OR p_result->>'suiteFingerprint'<>expected_suite THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RUNNER_RESULT_INVALID';
  END IF;
  INSERT INTO agent_learning_runner_results(
    attempt_id,activation_id,draft_id,draft_fingerprint,check_generation,kind,
    artifact_name,artifact_bytes,artifact_fingerprint,result)
  VALUES(p_attempt_id,target.activation_id,target.draft_id,target.draft_fingerprint,
    attempt.check_generation,attempt.kind,p_artifact_name,p_artifact_bytes,
    p_artifact_fingerprint,p_result);
  UPDATE agent_learning_runner_attempts SET state='result_ready',cleanup_state='pending',
    updated_at=clock_timestamp() WHERE attempt_id=p_attempt_id;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_worker_mark_runner_cancel_pending(
  p_activation_id TEXT,p_attempt_id TEXT,p_lease_generation BIGINT,p_error_code TEXT
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE attempt agent_learning_runner_attempts%ROWTYPE;
BEGIN
  SELECT item.* INTO attempt FROM agent_learning_runner_attempts item
  WHERE item.attempt_id=p_attempt_id FOR UPDATE;
  IF NOT FOUND OR attempt.activation_id<>p_activation_id
     OR attempt.lease_generation<>p_lease_generation THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  IF p_error_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RUNNER_CLEANUP_INVALID';
  END IF;
  IF attempt.state IN ('completed','failed') THEN RETURN false; END IF;
  IF attempt.state='leased' THEN
    -- No Sandbox was accepted, so terminal failure has no host residue.
    UPDATE agent_learning_runner_attempts SET state='failed',cleanup_state='completed',
      error_code=p_error_code,updated_at=clock_timestamp(),terminal_at=clock_timestamp()
    WHERE attempt_id=p_attempt_id;
  ELSE
    UPDATE agent_learning_runner_attempts SET state='cancel_pending',cleanup_state='pending',
      error_code=p_error_code,updated_at=clock_timestamp() WHERE attempt_id=p_attempt_id;
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_worker_complete_runner_cleanup(
  p_activation_id TEXT,p_attempt_id TEXT,p_lease_generation BIGINT,
  p_succeeded BOOLEAN,p_error_code TEXT
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE attempt agent_learning_runner_attempts%ROWTYPE; has_result BOOLEAN;
BEGIN
  SELECT item.* INTO attempt FROM agent_learning_runner_attempts item
  WHERE item.attempt_id=p_attempt_id FOR UPDATE;
  IF NOT FOUND OR attempt.activation_id<>p_activation_id
     OR attempt.lease_generation<>p_lease_generation THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  IF p_succeeded IS NULL OR (p_succeeded AND COALESCE(p_error_code,'')<>'')
     OR (NOT p_succeeded AND COALESCE(p_error_code,'')!~'^[A-Z][A-Z0-9_]{0,63}$') THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RUNNER_CLEANUP_INVALID';
  END IF;
  IF attempt.state IN ('completed','failed') THEN
    IF p_succeeded AND attempt.cleanup_state='completed' THEN RETURN false; END IF;
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
  END IF;
  IF attempt.state NOT IN ('result_ready','cancel_pending') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_RUNNER_CLEANUP_REQUIRED';
  END IF;
  IF NOT p_succeeded THEN
    UPDATE agent_learning_runner_attempts SET state='cancel_pending',cleanup_state='failed',
      cleanup_attempts=cleanup_attempts+1,error_code=p_error_code,
      updated_at=clock_timestamp() WHERE attempt_id=p_attempt_id;
    RETURN true;
  END IF;
  SELECT EXISTS(SELECT 1 FROM agent_learning_runner_results result
    WHERE result.attempt_id=p_attempt_id) INTO has_result;
  UPDATE agent_learning_runner_attempts SET state=CASE WHEN has_result THEN 'completed' ELSE 'failed' END,
    cleanup_state='completed',cleanup_attempts=cleanup_attempts+1,
    updated_at=clock_timestamp(),terminal_at=clock_timestamp()
  WHERE attempt_id=p_attempt_id;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_worker_runner_inventory(p_activation_id TEXT,p_limit INTEGER)
RETURNS TABLE(
  attempt_id TEXT,draft_id TEXT,user_id UUID,check_generation BIGINT,kind TEXT,
  transport_run_id TEXT,step_id TEXT,lease_generation BIGINT,lease_owner TEXT,
  snapshot_fingerprint TEXT,state TEXT,sandbox_id TEXT,spec_fingerprint TEXT,
  probe_fingerprint TEXT,cleanup_state TEXT,cleanup_attempts INTEGER,error_code TEXT,
  lease_expires_at TIMESTAMPTZ,authority_expires_at TIMESTAMPTZ
) LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_LIMIT_INVALID';
  END IF;
  RETURN QUERY SELECT attempt.attempt_id,attempt.draft_id,attempt.user_id,
    attempt.check_generation,attempt.kind,attempt.transport_run_id,attempt.step_id,
    attempt.lease_generation,attempt.lease_owner,attempt.snapshot_fingerprint,
    attempt.state,attempt.sandbox_id,attempt.spec_fingerprint,attempt.probe_fingerprint,
    attempt.cleanup_state,attempt.cleanup_attempts,attempt.error_code,
    attempt.lease_expires_at,max(request.expires_at)
  FROM agent_learning_runner_attempts attempt
  LEFT JOIN agent_learning_runner_requests request ON request.attempt_id=attempt.attempt_id
  WHERE attempt.activation_id=p_activation_id AND attempt.state NOT IN ('completed','failed')
  GROUP BY attempt.attempt_id
  ORDER BY attempt.created_at,attempt.attempt_id LIMIT p_limit;
END
$function$;

CREATE FUNCTION agent_learning_worker_reconcile_runner(
  p_activation_id TEXT,p_now TIMESTAMPTZ,p_limit INTEGER
) RETURNS INTEGER LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE reconciled INTEGER:=0;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_LIMIT_INVALID';
  END IF;
  WITH targets AS(
    SELECT attempt.attempt_id FROM agent_learning_runner_attempts attempt
    LEFT JOIN agent_learning_drafts draft ON draft.id=attempt.draft_id
    WHERE attempt.activation_id=p_activation_id
      AND attempt.state IN ('launched','result_ready','cancel_pending')
      AND (attempt.lease_expires_at<=p_now OR draft.state<>'checking'
        OR draft.check_generation<>attempt.check_generation
        OR draft.check_owner IS NULL OR draft.check_expires_at<=p_now)
    ORDER BY attempt.lease_expires_at,attempt.attempt_id
    FOR UPDATE OF attempt SKIP LOCKED LIMIT p_limit
  ) UPDATE agent_learning_runner_attempts attempt SET state='cancel_pending',
    cleanup_state=CASE WHEN attempt.cleanup_state='failed' THEN 'failed' ELSE 'pending' END,
    error_code=COALESCE(attempt.error_code,'RUNNER_RECONCILE_REQUIRED'),
    updated_at=GREATEST(attempt.updated_at,p_now)
    FROM targets WHERE attempt.attempt_id=targets.attempt_id;
  GET DIAGNOSTICS reconciled=ROW_COUNT;
  RETURN reconciled;
END
$function$;

CREATE FUNCTION agent_learning_worker_complete_checks(
  p_activation_id TEXT,p_draft TEXT,p_owner TEXT,p_generation BIGINT,
  p_results JSONB,p_audit_id TEXT
) RETURNS agent_learning_drafts
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE target agent_learning_worker_targets%ROWTYPE; item JSONB;
  stored agent_learning_runner_results%ROWTYPE;
BEGIN
  SELECT value.* INTO target FROM agent_learning_worker_targets value
  WHERE value.activation_id=p_activation_id AND value.draft_id=p_draft;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_WORKER_TARGET_DRIFT';
  END IF;
  IF jsonb_typeof(p_results)<>'array' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CHECK_INVALID';
  END IF;
  FOR item IN SELECT value FROM jsonb_array_elements(p_results)
    WHERE value->>'kind' IN ('isolation','evaluation') LOOP
    IF item->>'status'='failed' AND item->>'reasonCode' IN ('BLOCKED_BY_STATIC','BLOCKED_BY_ISOLATION') THEN
      IF EXISTS(SELECT 1 FROM agent_learning_runner_results result
        WHERE result.activation_id=p_activation_id AND result.draft_id=p_draft
          AND result.check_generation=p_generation AND result.kind=item->>'kind') THEN
        RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='CHECKS_INCOMPLETE';
      END IF;
      CONTINUE;
    END IF;
    SELECT result.* INTO stored FROM agent_learning_runner_results result
    JOIN agent_learning_runner_attempts attempt ON attempt.attempt_id=result.attempt_id
    WHERE result.activation_id=p_activation_id AND result.draft_id=p_draft
      AND result.check_generation=p_generation AND result.kind=item->>'kind'
      AND attempt.state='completed' AND attempt.cleanup_state='completed';
    IF NOT FOUND OR item->>'status'<>stored.result->>'status'
       OR item->>'reasonCode'<>stored.result->>'reasonCode'
       OR item->>'suiteFingerprint'<>stored.result->>'suiteFingerprint'
       OR item->>'evidenceFingerprint'<>stored.artifact_fingerprint
       OR (item->>'durationMillis')::bigint<>(stored.result->>'durationMillis')::bigint
       OR item->'metrics'<>stored.result->'metrics' THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='CHECKS_INCOMPLETE';
    END IF;
  END LOOP;
  RETURN agent_learning_complete_checks(p_draft,p_owner,p_generation,p_results,p_audit_id);
END
$function$;

CREATE FUNCTION agent_learning_worker_release_check(
  p_activation_id TEXT,p_draft TEXT,p_owner TEXT,p_generation BIGINT,
  p_error TEXT,p_retry_at TIMESTAMPTZ,p_audit_id TEXT
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM agent_learning_worker_targets target
    WHERE target.activation_id=p_activation_id AND target.draft_id=p_draft) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_WORKER_TARGET_DRIFT';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_learning_runner_attempts attempt
    WHERE attempt.activation_id=p_activation_id AND attempt.draft_id=p_draft
      AND attempt.check_generation=p_generation AND attempt.state NOT IN ('completed','failed')) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_RUNNER_CLEANUP_REQUIRED';
  END IF;
  RETURN agent_learning_release_check(p_draft,p_owner,p_generation,p_error,p_retry_at,p_audit_id);
END
$function$;

CREATE FUNCTION agent_learning_worker_claim_cleanup(
  p_activation_id TEXT,p_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER
) RETURNS SETOF agent_learning_cleanup_queue
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_owner) NOT BETWEEN 1 AND 128
     OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CLAIM_INVALID';
  END IF;
  RETURN QUERY WITH candidates AS(
    SELECT queue.draft_id FROM agent_learning_worker_targets target
    JOIN agent_learning_cleanup_queue queue ON queue.draft_id=target.draft_id
    WHERE target.activation_id=p_activation_id AND queue.state='pending'
      AND queue.next_attempt_at<=p_now
    ORDER BY queue.next_attempt_at,queue.draft_id
    FOR UPDATE OF queue SKIP LOCKED LIMIT p_limit
  ) UPDATE agent_learning_cleanup_queue queue SET state='claimed',
    generation=queue.generation+1,claim_owner=p_owner,
    claim_expires_at=p_now+make_interval(secs=>p_lease_seconds),
    updated_at=GREATEST(queue.updated_at,p_now)
    FROM candidates WHERE queue.draft_id=candidates.draft_id RETURNING queue.*;
END
$function$;

CREATE FUNCTION agent_learning_worker_complete_cleanup(
  p_activation_id TEXT,p_draft TEXT,p_owner TEXT,p_generation BIGINT,p_object_fingerprint TEXT
) RETURNS VOID LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM agent_learning_worker_targets target
    WHERE target.activation_id=p_activation_id AND target.draft_id=p_draft
      AND target.archive_fingerprint=p_object_fingerprint) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_WORKER_TARGET_DRIFT';
  END IF;
  PERFORM agent_learning_complete_cleanup(p_draft,p_owner,p_generation,p_object_fingerprint);
END
$function$;

CREATE FUNCTION agent_learning_worker_release_cleanup(
  p_activation_id TEXT,p_draft TEXT,p_owner TEXT,p_generation BIGINT,
  p_error TEXT,p_retry_at TIMESTAMPTZ
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM agent_learning_worker_targets target
    WHERE target.activation_id=p_activation_id AND target.draft_id=p_draft) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_LEARNING_WORKER_TARGET_DRIFT';
  END IF;
  RETURN agent_learning_release_cleanup(p_draft,p_owner,p_generation,p_error,p_retry_at);
END
$function$;

CREATE FUNCTION agent_learning_worker_reconcile(
  p_activation_id TEXT,p_now TIMESTAMPTZ,p_limit INTEGER
) RETURNS TABLE(checks_reclaimed INTEGER,cleanup_reclaimed INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE check_count INTEGER:=0; cleanup_count INTEGER:=0; item RECORD;
  next_attempts INTEGER; terminal BOOLEAN;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_LIMIT_INVALID';
  END IF;
  FOR item IN SELECT draft.id,draft.user_id,draft.check_generation,draft.check_attempts
    FROM agent_learning_worker_targets target
    JOIN agent_learning_drafts draft ON draft.id=target.draft_id
      AND draft.user_id=target.user_id AND draft.draft_fingerprint=target.draft_fingerprint
    WHERE target.activation_id=p_activation_id AND draft.state='checking'
      AND draft.check_expires_at<=p_now
      AND NOT EXISTS(SELECT 1 FROM agent_learning_runner_attempts attempt
        WHERE attempt.activation_id=p_activation_id AND attempt.draft_id=draft.id
          AND attempt.check_generation=draft.check_generation
          AND attempt.state NOT IN ('completed','failed'))
    ORDER BY draft.check_expires_at,draft.id
    FOR UPDATE OF draft SKIP LOCKED LIMIT p_limit LOOP
    next_attempts:=item.check_attempts+1; terminal:=next_attempts>=3;
    UPDATE agent_learning_drafts SET state=CASE WHEN terminal THEN 'check_failed' ELSE 'quarantined' END,
      revision=revision+1,check_attempts=next_attempts,check_owner=NULL,
      check_expires_at=NULL,next_check_at=p_now,updated_at=GREATEST(updated_at,p_now)
    WHERE id=item.id;
    INSERT INTO agent_learning_audit_events VALUES(
      'draft_event_'||substr(md5(item.id||'|'||item.check_generation::text||'|activation-reclaim'),1,32),
      item.id,item.user_id,'claim.reclaimed','reconciler','reconciler',
      'CHECK_CLAIM_RECLAIMED',jsonb_build_object('generation',item.check_generation,'terminal',terminal),p_now)
    ON CONFLICT(id) DO NOTHING;
    check_count:=check_count+1;
  END LOOP;
  FOR item IN SELECT queue.draft_id,queue.generation,queue.attempts,draft.user_id
    FROM agent_learning_worker_targets target
    JOIN agent_learning_cleanup_queue queue ON queue.draft_id=target.draft_id
    JOIN agent_learning_drafts draft ON draft.id=queue.draft_id
    WHERE target.activation_id=p_activation_id AND queue.state='claimed'
      AND queue.claim_expires_at<=p_now
    ORDER BY queue.claim_expires_at,queue.draft_id
    FOR UPDATE OF queue SKIP LOCKED LIMIT p_limit LOOP
    next_attempts:=item.attempts+1; terminal:=next_attempts>=10;
    UPDATE agent_learning_cleanup_queue SET state=CASE WHEN terminal THEN 'failed' ELSE 'pending' END,
      attempts=next_attempts,claim_owner=NULL,claim_expires_at=NULL,next_attempt_at=p_now,
      error_code='CLEANUP_CLAIM_RECLAIMED',updated_at=GREATEST(updated_at,p_now)
    WHERE draft_id=item.draft_id;
    INSERT INTO agent_learning_audit_events VALUES(
      'draft_event_'||substr(md5(item.draft_id||'|'||item.generation::text||'|activation-cleanup-reclaim'),1,32),
      item.draft_id,item.user_id,'claim.reclaimed','reconciler','reconciler',
      'CLEANUP_CLAIM_RECLAIMED',jsonb_build_object('generation',item.generation,'terminal',terminal),p_now)
    ON CONFLICT(id) DO NOTHING;
    cleanup_count:=cleanup_count+1;
  END LOOP;
  RETURN QUERY SELECT check_count,cleanup_count;
END
$function$;

CREATE FUNCTION agent_learning_worker_prune_runner(
  p_activation_id TEXT,p_cutoff TIMESTAMPTZ,p_limit INTEGER
) RETURNS TABLE(requests_pruned INTEGER,results_pruned INTEGER,attempts_pruned INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE request_count INTEGER:=0; result_count INTEGER:=0; attempt_count INTEGER:=0;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 OR p_cutoff>clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_PRUNE_INVALID';
  END IF;
  WITH targets AS(SELECT request.caller_identity,request.request_id,request.nonce
    FROM agent_learning_runner_requests request
    WHERE request.activation_id=p_activation_id AND request.status='completed'
      AND request.completed_at<p_cutoff
    ORDER BY request.completed_at,request.request_id LIMIT p_limit)
  DELETE FROM agent_learning_runner_requests request USING targets
  WHERE request.caller_identity=targets.caller_identity
    AND request.request_id=targets.request_id AND request.nonce=targets.nonce;
  GET DIAGNOSTICS request_count=ROW_COUNT;
  WITH targets AS(SELECT result.attempt_id FROM agent_learning_runner_results result
    JOIN agent_learning_runner_attempts attempt ON attempt.attempt_id=result.attempt_id
    JOIN agent_learning_check_results receipt ON receipt.draft_id=result.draft_id
      AND receipt.generation=result.check_generation AND receipt.kind=result.kind
      AND receipt.evidence_fingerprint=result.artifact_fingerprint
    WHERE result.activation_id=p_activation_id AND attempt.state='completed'
      AND attempt.terminal_at<p_cutoff
    ORDER BY attempt.terminal_at,result.attempt_id LIMIT p_limit)
  DELETE FROM agent_learning_runner_results result USING targets
  WHERE result.attempt_id=targets.attempt_id;
  GET DIAGNOSTICS result_count=ROW_COUNT;
  WITH targets AS(SELECT attempt.attempt_id FROM agent_learning_runner_attempts attempt
    WHERE attempt.activation_id=p_activation_id AND attempt.state IN ('completed','failed')
      AND attempt.cleanup_state='completed' AND attempt.terminal_at<p_cutoff
      AND NOT EXISTS(SELECT 1 FROM agent_learning_runner_results result
        WHERE result.attempt_id=attempt.attempt_id)
      AND NOT EXISTS(SELECT 1 FROM agent_learning_runner_requests request
        WHERE request.attempt_id=attempt.attempt_id)
    ORDER BY attempt.terminal_at,attempt.attempt_id LIMIT p_limit)
  DELETE FROM agent_learning_runner_attempts attempt USING targets
  WHERE attempt.attempt_id=targets.attempt_id;
  GET DIAGNOSTICS attempt_count=ROW_COUNT;
  RETURN QUERY SELECT request_count,result_count,attempt_count;
END
$function$;

CREATE FUNCTION agent_learning_worker_prune(
  p_activation_id TEXT,p_cutoff TIMESTAMPTZ,p_limit INTEGER
) RETURNS TABLE(drafts_pruned INTEGER,audits_pruned INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE audit_count INTEGER:=0;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 OR p_cutoff>clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_PRUNE_INVALID';
  END IF;
  WITH targets AS(SELECT event.id FROM agent_learning_worker_targets target
    JOIN agent_learning_audit_events event ON event.draft_id=target.draft_id
      AND event.user_id=target.user_id
    WHERE target.activation_id=p_activation_id AND event.occurred_at<p_cutoff
    ORDER BY event.occurred_at,event.id LIMIT p_limit)
  DELETE FROM agent_learning_audit_events event USING targets WHERE event.id=targets.id;
  GET DIAGNOSTICS audit_count=ROW_COUNT;
  -- Retained activation facts deliberately fence Draft deletion.
  RETURN QUERY SELECT 0,audit_count;
END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema(); identity TEXT;
BEGIN
  FOREACH identity IN ARRAY ARRAY[
    'agent_worker_target_guard()',
    'agent_cron_worker_provision_target(text,text,uuid,bigint,text,text,timestamp with time zone,timestamp with time zone,text,text)',
    'agent_cron_worker_disable_target(text,text,text)',
    'agent_learning_worker_provision_target(text,text,uuid,text,text,text,text,text,text,text,text,text,text,timestamp with time zone,timestamp with time zone,text,text)',
    'agent_learning_worker_disable_target(text,text,text)',
    'agent_cron_worker_get_target(text)','agent_learning_worker_get_target(text)',
    'agent_cron_worker_claim_due(text,text,timestamp with time zone,integer,integer)',
    'agent_cron_worker_advance_cursor(text,text,bigint,text,text,bigint,timestamp with time zone,timestamp with time zone,timestamp with time zone,jsonb)',
    'agent_cron_worker_claim_triggers(text,text,timestamp with time zone,integer,integer)',
    'agent_cron_worker_enqueue_trigger(text,text,text,bigint,text,text,text,text,jsonb,jsonb,text[],text)',
    'agent_cron_worker_release_trigger(text,text,text,bigint,text,timestamp with time zone,text)',
    'agent_cron_worker_reconcile(text,timestamp with time zone,integer)',
    'agent_cron_worker_prune(text,timestamp with time zone,integer)',
    'agent_learning_runner_result_valid(jsonb)','agent_learning_runner_result_immutable()',
    'agent_learning_worker_get_draft(text)',
    'agent_learning_worker_claim_checks(text,text,timestamp with time zone,integer,integer)',
    'agent_learning_worker_begin_runner_check(text,text,text,bigint,text,text,text,text,bigint,text,text,text,timestamp with time zone,integer)',
    'agent_learning_worker_issue_runner_authority(text,text,text,text,text,text,text,text,text,integer)',
    'agent_learning_worker_complete_runner_request(text,text,text,text,text,jsonb)',
    'agent_learning_worker_record_runner_launch(text,text,bigint,text,text,text)',
    'agent_learning_worker_record_runner_result(text,text,bigint,text,bigint,text,jsonb)',
    'agent_learning_worker_mark_runner_cancel_pending(text,text,bigint,text)',
    'agent_learning_worker_complete_runner_cleanup(text,text,bigint,boolean,text)',
    'agent_learning_worker_runner_inventory(text,integer)',
    'agent_learning_worker_reconcile_runner(text,timestamp with time zone,integer)',
    'agent_learning_worker_complete_checks(text,text,text,bigint,jsonb,text)',
    'agent_learning_worker_release_check(text,text,text,bigint,text,timestamp with time zone,text)',
    'agent_learning_worker_claim_cleanup(text,text,timestamp with time zone,integer,integer)',
    'agent_learning_worker_complete_cleanup(text,text,text,bigint,text)',
    'agent_learning_worker_release_cleanup(text,text,text,bigint,text,timestamp with time zone)',
    'agent_learning_worker_reconcile(text,timestamp with time zone,integer)',
    'agent_learning_worker_prune_runner(text,timestamp with time zone,integer)',
    'agent_learning_worker_prune(text,timestamp with time zone,integer)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',schema_name,identity,schema_name);
  END LOOP;
END
$harden$;

ALTER TABLE agent_cron_worker_targets OWNER TO agent_cron_owner;
ALTER TABLE agent_learning_worker_targets OWNER TO agent_learning_owner;
ALTER TABLE agent_learning_runner_attempts OWNER TO agent_learning_owner;
ALTER TABLE agent_learning_runner_results OWNER TO agent_learning_owner;
ALTER TABLE agent_learning_runner_requests OWNER TO agent_learning_owner;
ALTER FUNCTION agent_worker_target_guard() OWNER TO agent_learning_owner;
ALTER FUNCTION agent_cron_worker_provision_target(TEXT,TEXT,UUID,BIGINT,TEXT,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TEXT,TEXT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_cron_worker_disable_target(TEXT,TEXT,TEXT) OWNER TO agent_cron_owner;
ALTER FUNCTION agent_learning_worker_provision_target(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TEXT,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_worker_disable_target(TEXT,TEXT,TEXT) OWNER TO agent_learning_owner;

DO $owners$
DECLARE identity TEXT;
BEGIN
  FOREACH identity IN ARRAY ARRAY[
    'agent_cron_worker_get_target(text)',
    'agent_cron_worker_claim_due(text,text,timestamp with time zone,integer,integer)',
    'agent_cron_worker_advance_cursor(text,text,bigint,text,text,bigint,timestamp with time zone,timestamp with time zone,timestamp with time zone,jsonb)',
    'agent_cron_worker_claim_triggers(text,text,timestamp with time zone,integer,integer)',
    'agent_cron_worker_enqueue_trigger(text,text,text,bigint,text,text,text,text,jsonb,jsonb,text[],text)',
    'agent_cron_worker_release_trigger(text,text,text,bigint,text,timestamp with time zone,text)',
    'agent_cron_worker_reconcile(text,timestamp with time zone,integer)',
    'agent_cron_worker_prune(text,timestamp with time zone,integer)'
  ] LOOP EXECUTE 'ALTER FUNCTION '||identity||' OWNER TO agent_cron_owner'; END LOOP;
  FOREACH identity IN ARRAY ARRAY[
    'agent_learning_worker_get_target(text)','agent_learning_runner_result_valid(jsonb)',
    'agent_learning_runner_result_immutable()','agent_learning_worker_get_draft(text)',
    'agent_learning_worker_claim_checks(text,text,timestamp with time zone,integer,integer)',
    'agent_learning_worker_begin_runner_check(text,text,text,bigint,text,text,text,text,bigint,text,text,text,timestamp with time zone,integer)',
    'agent_learning_worker_issue_runner_authority(text,text,text,text,text,text,text,text,text,integer)',
    'agent_learning_worker_complete_runner_request(text,text,text,text,text,jsonb)',
    'agent_learning_worker_record_runner_launch(text,text,bigint,text,text,text)',
    'agent_learning_worker_record_runner_result(text,text,bigint,text,bigint,text,jsonb)',
    'agent_learning_worker_mark_runner_cancel_pending(text,text,bigint,text)',
    'agent_learning_worker_complete_runner_cleanup(text,text,bigint,boolean,text)',
    'agent_learning_worker_runner_inventory(text,integer)',
    'agent_learning_worker_reconcile_runner(text,timestamp with time zone,integer)',
    'agent_learning_worker_complete_checks(text,text,text,bigint,jsonb,text)',
    'agent_learning_worker_release_check(text,text,text,bigint,text,timestamp with time zone,text)',
    'agent_learning_worker_claim_cleanup(text,text,timestamp with time zone,integer,integer)',
    'agent_learning_worker_complete_cleanup(text,text,text,bigint,text)',
    'agent_learning_worker_release_cleanup(text,text,text,bigint,text,timestamp with time zone)',
    'agent_learning_worker_reconcile(text,timestamp with time zone,integer)',
    'agent_learning_worker_prune_runner(text,timestamp with time zone,integer)',
    'agent_learning_worker_prune(text,timestamp with time zone,integer)'
  ] LOOP EXECUTE 'ALTER FUNCTION '||identity||' OWNER TO agent_learning_owner'; END LOOP;
END
$owners$;

REVOKE ALL ON agent_cron_worker_targets,agent_learning_worker_targets,
  agent_learning_runner_attempts,agent_learning_runner_results,agent_learning_runner_requests
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
    agent_effect_control,agent_delegation_control,agent_cron_control,
    agent_learning_control,agent_cron_worker,agent_learning_worker;

DO $function_acl$
DECLARE schema_name TEXT:=current_schema(); identity TEXT;
BEGIN
  FOR identity IN
    SELECT format('%I.%I(%s)',namespace.nspname,procedure.proname,
      pg_get_function_identity_arguments(procedure.oid))
    FROM pg_proc procedure
    JOIN pg_namespace namespace ON namespace.oid=procedure.pronamespace
    WHERE namespace.nspname=schema_name
      AND (procedure.proname LIKE 'agent_cron_worker_%'
        OR procedure.proname LIKE 'agent_learning_worker_%'
        OR procedure.proname IN ('agent_worker_target_guard',
          'agent_learning_runner_result_valid','agent_learning_runner_result_immutable'))
  LOOP
    EXECUTE 'REVOKE ALL ON FUNCTION '||identity||
      ' FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,'||
      'agent_effect_control,agent_delegation_control,agent_cron_control,'||
      'agent_learning_control,agent_cron_worker,agent_learning_worker';
  END LOOP;
END
$function_acl$;

GRANT EXECUTE ON FUNCTION agent_worker_target_guard() TO agent_cron_owner,agent_learning_owner;

GRANT EXECUTE ON FUNCTION
  agent_cron_worker_get_target(TEXT),
  agent_cron_worker_claim_due(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_worker_advance_cursor(TEXT,TEXT,BIGINT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ,JSONB),
  agent_cron_worker_claim_triggers(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_cron_worker_enqueue_trigger(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT),
  agent_cron_worker_release_trigger(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),
  agent_cron_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER),
  agent_cron_worker_prune(TEXT,TIMESTAMPTZ,INTEGER)
  TO agent_cron_worker;

GRANT EXECUTE ON FUNCTION
  agent_learning_worker_get_target(TEXT),agent_learning_worker_get_draft(TEXT),
  agent_learning_worker_claim_checks(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_learning_worker_begin_runner_check(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_issue_runner_authority(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER),
  agent_learning_worker_complete_runner_request(TEXT,TEXT,TEXT,TEXT,TEXT,JSONB),
  agent_learning_worker_record_runner_launch(TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT),
  agent_learning_worker_record_runner_result(TEXT,TEXT,BIGINT,TEXT,BIGINT,TEXT,JSONB),
  agent_learning_worker_mark_runner_cancel_pending(TEXT,TEXT,BIGINT,TEXT),
  agent_learning_worker_complete_runner_cleanup(TEXT,TEXT,BIGINT,BOOLEAN,TEXT),
  agent_learning_worker_runner_inventory(TEXT,INTEGER),
  agent_learning_worker_reconcile_runner(TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_complete_checks(TEXT,TEXT,TEXT,BIGINT,JSONB,TEXT),
  agent_learning_worker_release_check(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),
  agent_learning_worker_claim_cleanup(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_learning_worker_complete_cleanup(TEXT,TEXT,TEXT,BIGINT,TEXT),
  agent_learning_worker_release_cleanup(TEXT,TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ),
  agent_learning_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_prune_runner(TEXT,TIMESTAMPTZ,INTEGER),
  agent_learning_worker_prune(TEXT,TIMESTAMPTZ,INTEGER)
  TO agent_learning_worker;

DO $schema_acl$
BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_cron_worker,agent_learning_worker',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_cron_worker,agent_learning_worker',current_schema());
END
$schema_acl$;
