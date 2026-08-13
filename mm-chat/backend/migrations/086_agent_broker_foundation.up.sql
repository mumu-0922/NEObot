-- G20.4 Agent Broker durable effect authority. The Backend control plane owns
-- Prepare/approval/Commit and sanitized receipts. neo-runnerd remains
-- credential-free and has no PostgreSQL role.

DO $roles$
DECLARE
  role_name TEXT;
  can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create
  FROM pg_roles WHERE rolname=current_user;
  FOREACH role_name IN ARRAY ARRAY['agent_effect_owner','agent_effect_control'] LOOP
    IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF NOT can_create THEN
        RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_EFFECT_REQUIRED_ROLE_MISSING';
      END IF;
      EXECUTE format(
        'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',
        role_name);
    END IF;
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name AND
      (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_EFFECT_ROLE_MUST_BE_RESTRICTED';
    END IF;
  END LOOP;
  IF pg_has_role('agent_effect_control','agent_effect_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_effect_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_effect_control','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_effect_owner','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_effect_control','MEMBER')
     OR pg_has_role('agent_runner_control','agent_effect_owner','MEMBER')
     OR pg_has_role('agent_runner_control','agent_effect_control','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_EFFECT_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

ALTER TABLE agent_runner_requests DROP CONSTRAINT agent_runner_request_method_check;
ALTER TABLE agent_runner_requests ADD CONSTRAINT agent_runner_request_method_check
  CHECK(method IN ('launch','heartbeat','cancel','prepare','commit'));

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
  IF p_method NOT IN ('launch','heartbeat','cancel','prepare','commit') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
  END IF;
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
  IF p_method IN ('launch','heartbeat','prepare','commit') AND v_kill_mode<>'' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  RETURN v_epoch;
END
$function$;

CREATE TABLE agent_effect_intents (
  id TEXT PRIMARY KEY,
  request_id TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  intent_fingerprint TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  project_id TEXT NOT NULL,
  assistant_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  attempt_id TEXT NOT NULL,
  lease_generation BIGINT NOT NULL,
  lease_owner TEXT NOT NULL,
  lease_token_hash TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  grant_id TEXT NOT NULL,
  grant_fingerprint TEXT NOT NULL,
  registry_fingerprint TEXT NOT NULL,
  tool_identity TEXT NOT NULL,
  capability TEXT NOT NULL,
  action TEXT NOT NULL,
  resource TEXT NOT NULL,
  arguments_fingerprint TEXT NOT NULL,
  canonical_arguments JSONB NOT NULL,
  base_revision TEXT NOT NULL DEFAULT '',
  approval_class TEXT NOT NULL,
  capability_max_calls INTEGER NOT NULL,
  grant_max_tool_calls INTEGER NOT NULL,
  approval_revision BIGINT NOT NULL DEFAULT 1,
  state TEXT NOT NULL,
  kill_switch_epoch BIGINT NOT NULL,
  receipt_fingerprint TEXT,
  executor_status_digest TEXT,
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  approved_at TIMESTAMPTZ,
  committing_at TIMESTAMPTZ,
  terminal_at TIMESTAMPTZ,
  CONSTRAINT agent_effect_request_unique UNIQUE(user_id,request_id),
  CONSTRAINT agent_effect_attempt_fk FOREIGN KEY(run_id,user_id,attempt_id)
    REFERENCES agent_attempts(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_effect_step_fk FOREIGN KEY(run_id,user_id,step_id)
    REFERENCES agent_steps(run_id,user_id,id) ON DELETE CASCADE,
  CONSTRAINT agent_effect_id_check CHECK(id ~ '^intent_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_effect_request_id_check CHECK(request_id ~ '^request_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_effect_request_fingerprint_check CHECK(request_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_intent_fingerprint_check CHECK(intent_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_key_check CHECK(idempotency_key ~ '^commit_[A-Za-z0-9_-]{24,128}$'),
  CONSTRAINT agent_effect_subject_check CHECK(
    project_id ~ '^project_[a-z0-9]{8,64}$' AND assistant_id ~ '^assistant_[a-z0-9]{8,64}$'),
  CONSTRAINT agent_effect_generation_check CHECK(lease_generation>=1),
  CONSTRAINT agent_effect_owner_check CHECK(octet_length(lease_owner) BETWEEN 1 AND 128),
  CONSTRAINT agent_effect_token_check CHECK(lease_token_hash ~ '^[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_snapshot_check CHECK(snapshot_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_grant_check CHECK(
    grant_id ~ '^grant_[a-z0-9]{16,64}$' AND grant_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_registry_check CHECK(registry_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_identity_check CHECK(
    tool_identity ~ '^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$'
    AND capability ~ '^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$'
    AND action ~ '^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$'),
  CONSTRAINT agent_effect_resource_check CHECK(
    octet_length(resource) BETWEEN 1 AND 256 AND resource !~ E'[\\x00\\r\\n]'),
  CONSTRAINT agent_effect_arguments_check CHECK(
    arguments_fingerprint ~ '^sha256:[a-f0-9]{64}$'
    AND jsonb_typeof(canonical_arguments) IS NOT NULL
    AND octet_length(canonical_arguments::text)<=262144),
  CONSTRAINT agent_effect_base_revision_check CHECK(
    base_revision='' OR base_revision ~ '^(rev_[A-Za-z0-9_-]{8,128}|sha256:[a-f0-9]{64})$'),
  CONSTRAINT agent_effect_approval_check CHECK(
    approval_class IN ('automatic','once','per_commit') AND approval_revision>=1),
  CONSTRAINT agent_effect_budget_check CHECK(
    capability_max_calls BETWEEN 1 AND 10000 AND grant_max_tool_calls BETWEEN 1 AND 10000),
  CONSTRAINT agent_effect_state_check CHECK(state IN (
    'prepared','awaiting_approval','approved','committing','committed','failed',
    'rejected','canceled','expired','outcome_unknown')),
  CONSTRAINT agent_effect_epoch_check CHECK(kill_switch_epoch>=0),
  CONSTRAINT agent_effect_receipt_check CHECK(
    receipt_fingerprint IS NULL OR receipt_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_status_digest_check CHECK(
    executor_status_digest IS NULL OR executor_status_digest ~ '^[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_error_check CHECK(
    error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_effect_time_check CHECK(
    expires_at>created_at AND expires_at<=created_at+interval '1 hour'
    AND (approved_at IS NULL OR approved_at>=created_at)
    AND (committing_at IS NULL OR committing_at>=created_at)
    AND (terminal_at IS NULL OR terminal_at>=created_at)),
  CONSTRAINT agent_effect_state_shape CHECK(
    (state IN ('prepared','awaiting_approval') AND approved_at IS NULL
      AND committing_at IS NULL AND terminal_at IS NULL
      AND receipt_fingerprint IS NULL AND error_code IS NULL)
    OR (state='approved' AND approved_at IS NOT NULL AND committing_at IS NULL
      AND terminal_at IS NULL AND receipt_fingerprint IS NULL AND error_code IS NULL)
    OR (state='committing' AND approved_at IS NOT NULL AND committing_at IS NOT NULL
      AND terminal_at IS NULL AND receipt_fingerprint IS NULL AND error_code IS NULL)
    OR (state='committed' AND approved_at IS NOT NULL AND committing_at IS NOT NULL
      AND terminal_at IS NOT NULL AND receipt_fingerprint IS NOT NULL AND error_code IS NULL)
    OR (state IN ('failed','rejected','canceled','expired','outcome_unknown')
      AND terminal_at IS NOT NULL AND error_code IS NOT NULL
      AND receipt_fingerprint IS NULL))
);

CREATE INDEX idx_agent_effect_intents_expiry
  ON agent_effect_intents(expires_at,id)
  WHERE state IN ('prepared','awaiting_approval','approved');
CREATE INDEX idx_agent_effect_intents_reconcile
  ON agent_effect_intents(committing_at,id) WHERE state='committing';
CREATE INDEX idx_agent_effect_intents_retention
  ON agent_effect_intents(terminal_at,id) WHERE terminal_at IS NOT NULL;

CREATE TABLE agent_effect_approvals (
  approval_id TEXT PRIMARY KEY,
  intent_id TEXT NOT NULL REFERENCES agent_effect_intents(id) ON DELETE CASCADE,
  intent_fingerprint TEXT NOT NULL,
  revision BIGINT NOT NULL,
  decision TEXT NOT NULL,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  decided_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_effect_approval_intent_revision UNIQUE(intent_id,revision),
  CONSTRAINT agent_effect_approval_id_check CHECK(approval_id ~ '^approval_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_effect_approval_fingerprint_check CHECK(intent_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_approval_revision_check CHECK(revision>=1),
  CONSTRAINT agent_effect_approval_decision_check CHECK(decision IN ('approved','denied')),
  CONSTRAINT agent_effect_approval_actor_check CHECK(
    actor_type IN ('user','operator','system') AND octet_length(actor_id) BETWEEN 1 AND 128),
  CONSTRAINT agent_effect_approval_reason_check CHECK(reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$')
);

CREATE TABLE agent_effect_receipts (
  idempotency_key TEXT PRIMARY KEY,
  intent_id TEXT NOT NULL UNIQUE REFERENCES agent_effect_intents(id) ON DELETE CASCADE,
  intent_fingerprint TEXT NOT NULL,
  outcome TEXT NOT NULL,
  receipt_fingerprint TEXT,
  executor_status_digest TEXT,
  error_code TEXT,
  completed_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_effect_receipt_key_check CHECK(idempotency_key ~ '^commit_[A-Za-z0-9_-]{24,128}$'),
  CONSTRAINT agent_effect_receipt_intent_check CHECK(intent_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_receipt_outcome_check CHECK(outcome IN ('committed','failed','outcome_unknown')),
  CONSTRAINT agent_effect_receipt_fingerprint_check CHECK(
    (outcome='committed' AND receipt_fingerprint ~ '^sha256:[a-f0-9]{64}$' AND error_code IS NULL)
    OR (outcome IN ('failed','outcome_unknown') AND receipt_fingerprint IS NULL
      AND error_code ~ '^[A-Z][A-Z0-9_]{0,63}$')),
  CONSTRAINT agent_effect_receipt_status_check CHECK(
    executor_status_digest IS NULL OR executor_status_digest ~ '^[a-f0-9]{64}$')
);

CREATE TABLE agent_effect_grant_revocations (
  grant_id TEXT PRIMARY KEY,
  grant_fingerprint TEXT NOT NULL,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  revoked_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_effect_grant_revocation_id_check
    CHECK(grant_id ~ '^grant_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_effect_grant_revocation_fingerprint_check
    CHECK(grant_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_grant_revocation_actor_check CHECK(
    actor_type IN ('operator','system')
    AND octet_length(actor_id) BETWEEN 1 AND 128),
  CONSTRAINT agent_effect_grant_revocation_reason_check
    CHECK(reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$')
);

CREATE TABLE agent_effect_cancellations (
  cancellation_id TEXT PRIMARY KEY,
  intent_id TEXT NOT NULL UNIQUE REFERENCES agent_effect_intents(id) ON DELETE CASCADE,
  intent_fingerprint TEXT NOT NULL,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  canceled_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_effect_cancellation_id_check
    CHECK(cancellation_id ~ '^cancellation_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_effect_cancellation_fingerprint_check
    CHECK(intent_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_effect_cancellation_actor_check CHECK(
    actor_type IN ('user','operator')
    AND octet_length(actor_id) BETWEEN 1 AND 128),
  CONSTRAINT agent_effect_cancellation_reason_check
    CHECK(reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$')
);

CREATE TABLE agent_secret_handles (
  handle_digest TEXT PRIMARY KEY,
  intent_id TEXT NOT NULL REFERENCES agent_effect_intents(id) ON DELETE CASCADE,
  secret_ref TEXT NOT NULL,
  binding_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_secret_handle_digest_check CHECK(handle_digest ~ '^[a-f0-9]{64}$'),
  CONSTRAINT agent_secret_ref_check CHECK(secret_ref ~ '^secret_ref_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_secret_binding_check CHECK(binding_fingerprint ~ '^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_secret_state_check CHECK(state IN ('active','used','revoked','expired')),
  CONSTRAINT agent_secret_time_check CHECK(
    expires_at>created_at AND expires_at<=created_at+interval '1 hour'
    AND (used_at IS NULL OR used_at>=created_at)
    AND (revoked_at IS NULL OR revoked_at>=created_at)),
  CONSTRAINT agent_secret_state_shape CHECK(
    (state='active' AND used_at IS NULL AND revoked_at IS NULL)
    OR (state='used' AND used_at IS NOT NULL)
    OR (state IN ('revoked','expired') AND revoked_at IS NOT NULL))
);

CREATE INDEX idx_agent_secret_handles_expiry
  ON agent_secret_handles(expires_at,handle_digest) WHERE state='active';

CREATE FUNCTION agent_effect_immutable()
RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  IF TG_OP='DELETE' AND pg_trigger_depth()>1 THEN RETURN OLD;END IF;
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_EFFECT_IMMUTABLE';
END
$function$;

CREATE TRIGGER trg_agent_effect_approval_immutable
BEFORE UPDATE OR DELETE ON agent_effect_approvals
FOR EACH ROW EXECUTE FUNCTION agent_effect_immutable();
CREATE TRIGGER trg_agent_effect_receipt_immutable
BEFORE UPDATE OR DELETE ON agent_effect_receipts
FOR EACH ROW EXECUTE FUNCTION agent_effect_immutable();
CREATE TRIGGER trg_agent_effect_grant_revocation_immutable
BEFORE UPDATE OR DELETE ON agent_effect_grant_revocations
FOR EACH ROW EXECUTE FUNCTION agent_effect_immutable();
CREATE TRIGGER trg_agent_effect_cancellation_immutable
BEFORE UPDATE OR DELETE ON agent_effect_cancellations
FOR EACH ROW EXECUTE FUNCTION agent_effect_immutable();

CREATE FUNCTION agent_effect_revoke_grant(
  p_grant_id TEXT,p_grant_fingerprint TEXT,p_actor_type TEXT,
  p_actor_id TEXT,p_reason_code TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_existing agent_effect_grant_revocations%ROWTYPE;
BEGIN
  SELECT * INTO v_existing FROM agent_effect_grant_revocations
  WHERE grant_id=p_grant_id FOR UPDATE;
  IF FOUND THEN
    IF v_existing.grant_fingerprint<>p_grant_fingerprint
       OR v_existing.actor_type<>p_actor_type OR v_existing.actor_id<>p_actor_id
       OR v_existing.reason_code<>p_reason_code THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    UPDATE agent_secret_handles SET state='revoked',revoked_at=clock_timestamp()
    WHERE intent_id IN (
      SELECT id FROM agent_effect_intents WHERE grant_id=p_grant_id
    ) AND state='active';
    RETURN false;
  END IF;
  INSERT INTO agent_effect_grant_revocations(
    grant_id,grant_fingerprint,actor_type,actor_id,reason_code,revoked_at
  ) VALUES(p_grant_id,p_grant_fingerprint,p_actor_type,p_actor_id,p_reason_code,
    clock_timestamp());
  UPDATE agent_secret_handles SET state='revoked',revoked_at=clock_timestamp()
  WHERE intent_id IN (
    SELECT id FROM agent_effect_intents WHERE grant_id=p_grant_id
  ) AND state='active';
  RETURN true;
END
$function$;

-- This narrow helper is owned by the Orchestrator authority. The effect owner
-- can validate/advance only the exact Attempt bound to one effect intent; it
-- receives no direct Orchestrator table privilege.
CREATE FUNCTION agent_orchestrator_effect_fence(
  p_operation TEXT,p_intent_id TEXT,p_user_id UUID,p_run_id TEXT,p_step_id TEXT,
  p_attempt_id TEXT,p_generation BIGINT,p_lease_owner TEXT,p_lease_token_hash TEXT,
  p_snapshot_fingerprint TEXT,p_expected_epoch BIGINT,p_terminal_state TEXT DEFAULT NULL
) RETURNS BIGINT
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_run agent_runs%ROWTYPE;
  v_step agent_steps%ROWTYPE;
  v_attempt agent_attempts%ROWTYPE;
  v_epoch BIGINT;
  v_mode TEXT;
  v_from TEXT;
  v_to TEXT;
  v_event_id TEXT;
  v_other_step RECORD;
BEGIN
  IF p_operation NOT IN ('prepare','commit','secret','cancel','complete') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
  END IF;
  SELECT run.* INTO v_run FROM agent_runs run
  WHERE run.id=p_run_id AND run.user_id=p_user_id FOR UPDATE;
  SELECT step.* INTO v_step FROM agent_steps step
  WHERE step.id=p_step_id AND step.run_id=p_run_id AND step.user_id=p_user_id FOR UPDATE;
  SELECT attempt.* INTO v_attempt FROM agent_attempts attempt
  WHERE attempt.id=p_attempt_id AND attempt.run_id=p_run_id
    AND attempt.step_id=p_step_id AND attempt.user_id=p_user_id FOR UPDATE;
  IF v_run.id IS NULL OR v_step.id IS NULL OR v_attempt.id IS NULL
     OR v_run.snapshot_fingerprint<>p_snapshot_fingerprint
     OR v_attempt.generation<>p_generation OR v_step.current_generation<>p_generation
     OR v_attempt.lease_owner<>p_lease_owner
     OR (p_operation IN ('prepare','commit','secret') AND v_attempt.lease_token_hash<>p_lease_token_hash)
     OR (p_operation IN ('prepare','commit','secret') AND v_attempt.lease_expires_at<=v_now) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  SELECT state.epoch INTO v_epoch FROM agent_kill_switch_state state WHERE singleton;
  v_mode:=agent_orchestrator_active_kill_mode(v_run.scope_keys);
  IF p_operation IN ('prepare','commit','secret') AND (v_mode IS NOT NULL OR v_epoch<>p_expected_epoch) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  IF p_operation='secret' THEN
    IF v_attempt.state<>'committing' THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
    END IF;
    RETURN v_epoch;
  END IF;
  IF p_operation='prepare' THEN
    v_from:='running';v_to:='prepared';
    v_event_id:='event_'||md5(p_intent_id||E'\\x00prepare');
  ELSIF p_operation='commit' THEN
    v_from:='prepared';v_to:='committing';
    v_event_id:='event_'||md5(p_intent_id||E'\\x00commit');
  ELSIF p_operation='cancel' THEN
    v_from:='prepared';v_to:='canceled';
    v_event_id:='event_'||md5(p_intent_id||E'\\x00cancel');
  ELSE
    v_from:='committing';v_to:=p_terminal_state;
    v_event_id:='event_'||md5(p_intent_id||E'\\x00complete-attempt');
    IF v_to NOT IN ('succeeded','failed','outcome_unknown') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
    END IF;
    IF v_to='outcome_unknown' AND (v_run.state<>'running' OR v_step.state<>'running'
       OR EXISTS(SELECT 1 FROM agent_steps other_step
         WHERE other_step.run_id=p_run_id AND other_step.id<>p_step_id
           AND other_step.state NOT IN (
             'pending','ready','succeeded','failed','skipped','canceled','killed','outcome_unknown'))) THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
    END IF;
  END IF;
  IF v_attempt.state<>v_from THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
  END IF;
  UPDATE agent_attempts SET state=v_to,updated_at=v_now,
    terminal_at=CASE WHEN p_operation IN ('cancel','complete') THEN v_now ELSE NULL END
  WHERE id=p_attempt_id;
  PERFORM agent_orchestrator_append_event(
    v_event_id,p_run_id,p_user_id,p_step_id,p_attempt_id,'attempt.transition',
    'attempt',v_from,v_to,'orchestrator','agent-effect-control',
    CASE p_operation WHEN 'prepare' THEN 'EFFECT_PREPARED'
      WHEN 'commit' THEN 'EFFECT_COMMITTING'
      WHEN 'cancel' THEN 'EFFECT_CANCELED' ELSE 'EFFECT_COMPLETED' END,
    p_generation,v_attempt.lease_expires_at,
    '{}'::jsonb,v_now);
  IF p_operation='complete' AND v_to='outcome_unknown' THEN
    UPDATE agent_steps SET state='outcome_unknown',updated_at=v_now,terminal_at=v_now
    WHERE id=p_step_id;
    PERFORM agent_orchestrator_append_event(
      'event_'||md5(p_intent_id||E'\\x00complete-step'),p_run_id,p_user_id,
      p_step_id,NULL,'step.transition','step',v_step.state,'outcome_unknown',
      'orchestrator','agent-effect-control','EFFECT_OUTCOME_UNKNOWN',
      NULL,NULL,'{}'::jsonb,v_now);
    FOR v_other_step IN
      SELECT id,state FROM agent_steps
      WHERE run_id=p_run_id AND id<>p_step_id AND state IN ('pending','ready')
      ORDER BY ordinal FOR UPDATE
    LOOP
      UPDATE agent_steps SET state='canceled',updated_at=v_now,terminal_at=v_now
      WHERE id=v_other_step.id;
      PERFORM agent_orchestrator_append_event(
        'event_'||md5(p_intent_id||E'\\x00cancel-step\\x00'||v_other_step.id),
        p_run_id,p_user_id,v_other_step.id,NULL,'step.transition','step',
        v_other_step.state,'canceled','orchestrator','agent-effect-control',
        'EFFECT_OUTCOME_UNKNOWN',NULL,NULL,'{}'::jsonb,v_now);
    END LOOP;
    UPDATE agent_runs SET state='outcome_unknown',updated_at=v_now,terminal_at=v_now
    WHERE id=p_run_id;
    PERFORM agent_orchestrator_append_event(
      'event_'||md5(p_intent_id||E'\\x00complete-run'),p_run_id,p_user_id,
      NULL,NULL,'run.transition','run',v_run.state,'outcome_unknown',
      'orchestrator','agent-effect-control','EFFECT_OUTCOME_UNKNOWN',
      NULL,NULL,'{}'::jsonb,v_now);
  END IF;
  RETURN v_epoch;
END
$function$;

CREATE FUNCTION agent_effect_prepare(
  p_id TEXT,p_request_id TEXT,p_request_fingerprint TEXT,p_intent_fingerprint TEXT,
  p_idempotency_key TEXT,p_user_id UUID,p_project_id TEXT,p_assistant_id TEXT,
  p_run_id TEXT,p_step_id TEXT,p_attempt_id TEXT,p_generation BIGINT,
  p_lease_owner TEXT,p_lease_token_hash TEXT,p_snapshot_fingerprint TEXT,
  p_grant_id TEXT,p_grant_fingerprint TEXT,p_registry_fingerprint TEXT,
  p_tool_identity TEXT,p_capability TEXT,p_action TEXT,p_resource TEXT,
  p_arguments_fingerprint TEXT,p_canonical_arguments JSONB,p_base_revision TEXT,
  p_approval_class TEXT,p_capability_max_calls INTEGER,p_grant_max_tool_calls INTEGER,
  p_kill_switch_epoch BIGINT,p_created_at TIMESTAMPTZ,p_expires_at TIMESTAMPTZ
) RETURNS SETOF agent_effect_intents
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_existing agent_effect_intents%ROWTYPE;
  v_approval_id TEXT;
BEGIN
  IF p_created_at<v_now-interval '30 seconds' OR p_created_at>v_now+interval '30 seconds'
     OR p_expires_at<=v_now OR p_expires_at>p_created_at+interval '1 hour'
     OR p_capability_max_calls NOT BETWEEN 1 AND 10000
     OR p_grant_max_tool_calls NOT BETWEEN 1 AND 10000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_EFFECT_TIME_OR_BUDGET_INVALID';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_grant_revocations revocation
      WHERE revocation.grant_id=p_grant_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='GRANT_DENIED';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_run_id,86420));
  SELECT * INTO v_existing FROM agent_effect_intents
  WHERE user_id=p_user_id AND request_id=p_request_id FOR UPDATE;
  IF FOUND THEN
    IF v_existing.request_fingerprint<>p_request_fingerprint THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN NEXT v_existing;RETURN;
  END IF;
  IF (SELECT count(*) FROM agent_effect_intents existing
      WHERE existing.run_id=p_run_id AND existing.capability=p_capability
        AND existing.state IN ('awaiting_approval','approved','committing','committed','outcome_unknown'))>=p_capability_max_calls
     OR (SELECT count(*) FROM agent_effect_intents existing
         WHERE existing.run_id=p_run_id
           AND existing.state IN ('awaiting_approval','approved','committing','committed','outcome_unknown'))>=p_grant_max_tool_calls THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='BUDGET_EXHAUSTED';
  END IF;
  PERFORM agent_orchestrator_effect_fence(
    'prepare',p_id,p_user_id,p_run_id,p_step_id,p_attempt_id,p_generation,
    p_lease_owner,p_lease_token_hash,p_snapshot_fingerprint,p_kill_switch_epoch,NULL);
  INSERT INTO agent_effect_intents(
    id,request_id,request_fingerprint,intent_fingerprint,idempotency_key,
    user_id,project_id,assistant_id,run_id,step_id,attempt_id,lease_generation,
    lease_owner,lease_token_hash,snapshot_fingerprint,grant_id,grant_fingerprint,
    registry_fingerprint,tool_identity,capability,action,resource,
    arguments_fingerprint,canonical_arguments,base_revision,approval_class,
    capability_max_calls,grant_max_tool_calls,state,kill_switch_epoch,
    created_at,expires_at,approved_at
  ) VALUES (
    p_id,p_request_id,p_request_fingerprint,p_intent_fingerprint,p_idempotency_key,
    p_user_id,p_project_id,p_assistant_id,p_run_id,p_step_id,p_attempt_id,p_generation,
    p_lease_owner,p_lease_token_hash,p_snapshot_fingerprint,p_grant_id,p_grant_fingerprint,
    p_registry_fingerprint,p_tool_identity,p_capability,p_action,p_resource,
    p_arguments_fingerprint,p_canonical_arguments,COALESCE(p_base_revision,''),p_approval_class,
    p_capability_max_calls,p_grant_max_tool_calls,
    CASE WHEN p_approval_class='automatic' THEN 'approved' ELSE 'awaiting_approval' END,
    p_kill_switch_epoch,p_created_at,p_expires_at,
    CASE WHEN p_approval_class='automatic' THEN p_created_at ELSE NULL END
  ) RETURNING * INTO v_existing;
  IF p_approval_class='automatic' THEN
    v_approval_id:='approval_'||substring(p_intent_fingerprint from 8 for 32);
    INSERT INTO agent_effect_approvals(
      approval_id,intent_id,intent_fingerprint,revision,decision,
      actor_type,actor_id,reason_code,decided_at
    ) VALUES(v_approval_id,p_id,p_intent_fingerprint,1,'approved',
      'system','agent-effect-control','AUTOMATIC_POLICY',p_created_at);
  END IF;
  RETURN NEXT v_existing;
END
$function$;

CREATE FUNCTION agent_effect_decide_approval(
  p_approval_id TEXT,p_user_id UUID,p_intent_id TEXT,p_intent_fingerprint TEXT,
  p_decision TEXT,p_actor_type TEXT,p_actor_id TEXT,p_reason_code TEXT,
  p_expected_revision BIGINT
) RETURNS SETOF agent_effect_intents
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_intent agent_effect_intents%ROWTYPE;
  v_existing agent_effect_approvals%ROWTYPE;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents
  WHERE id=p_intent_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND OR v_intent.intent_fingerprint<>p_intent_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='APPROVAL_DENIED';
  END IF;
  SELECT * INTO v_existing FROM agent_effect_approvals WHERE approval_id=p_approval_id;
  IF FOUND THEN
    IF v_existing.intent_id<>p_intent_id OR v_existing.intent_fingerprint<>p_intent_fingerprint
       OR v_existing.decision<>p_decision OR v_existing.actor_type<>p_actor_type
       OR v_existing.actor_id<>p_actor_id OR v_existing.reason_code<>p_reason_code
       OR v_existing.revision<>p_expected_revision THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN NEXT v_intent;RETURN;
  END IF;
  IF v_intent.state<>'awaiting_approval' OR v_intent.approval_class='automatic'
     OR v_intent.expires_at<=v_now OR v_intent.approval_revision<>p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='APPROVAL_DENIED';
  END IF;
  IF p_actor_type='user' AND p_actor_id IS DISTINCT FROM p_user_id::TEXT THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='APPROVAL_DENIED';
  END IF;
  INSERT INTO agent_effect_approvals(
    approval_id,intent_id,intent_fingerprint,revision,decision,
    actor_type,actor_id,reason_code,decided_at
  ) VALUES(p_approval_id,p_intent_id,p_intent_fingerprint,p_expected_revision,
    p_decision,p_actor_type,p_actor_id,p_reason_code,v_now);
  UPDATE agent_effect_intents SET
    state=CASE WHEN p_decision='approved' THEN 'approved' ELSE 'rejected' END,
    approved_at=CASE WHEN p_decision='approved' THEN v_now ELSE NULL END,
    terminal_at=CASE WHEN p_decision='denied' THEN v_now ELSE NULL END,
    error_code=CASE WHEN p_decision='denied' THEN 'APPROVAL_DENIED' ELSE NULL END,
    approval_revision=approval_revision+1
  WHERE id=p_intent_id RETURNING * INTO v_intent;
  RETURN NEXT v_intent;
END
$function$;

CREATE FUNCTION agent_effect_cancel(
  p_cancellation_id TEXT,p_user_id UUID,p_intent_id TEXT,p_intent_fingerprint TEXT,
  p_actor_type TEXT,p_actor_id TEXT,p_reason_code TEXT
) RETURNS SETOF agent_effect_intents
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_intent agent_effect_intents%ROWTYPE;
  v_existing agent_effect_cancellations%ROWTYPE;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents
  WHERE id=p_intent_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND OR v_intent.intent_fingerprint<>p_intent_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_EFFECT_NOT_FOUND';
  END IF;
  SELECT * INTO v_existing FROM agent_effect_cancellations
  WHERE cancellation_id=p_cancellation_id;
  IF FOUND THEN
    IF v_existing.intent_id<>p_intent_id
       OR v_existing.intent_fingerprint<>p_intent_fingerprint
       OR v_existing.actor_type<>p_actor_type OR v_existing.actor_id<>p_actor_id
       OR v_existing.reason_code<>p_reason_code THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    IF v_intent.state<>'canceled' THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
    END IF;
    RETURN NEXT v_intent;RETURN;
  END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_cancellations cancellation
      WHERE cancellation.intent_id=p_intent_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
  END IF;
  IF v_intent.expires_at<=v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INTENT_EXPIRED';
  END IF;
  IF v_intent.state NOT IN ('awaiting_approval','approved')
     OR p_actor_type NOT IN ('user','operator')
     OR (p_actor_type='user' AND p_actor_id IS DISTINCT FROM p_user_id::TEXT) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
  END IF;
  PERFORM agent_orchestrator_effect_fence(
    'cancel',p_intent_id,v_intent.user_id,v_intent.run_id,v_intent.step_id,
    v_intent.attempt_id,v_intent.lease_generation,v_intent.lease_owner,
    v_intent.lease_token_hash,v_intent.snapshot_fingerprint,
    v_intent.kill_switch_epoch,NULL);
  UPDATE agent_effect_intents SET state='canceled',terminal_at=v_now,
    error_code='INTENT_CANCELED' WHERE id=p_intent_id RETURNING * INTO v_intent;
  INSERT INTO agent_effect_cancellations(
    cancellation_id,intent_id,intent_fingerprint,actor_type,actor_id,reason_code,canceled_at
  ) VALUES(p_cancellation_id,p_intent_id,p_intent_fingerprint,p_actor_type,
    p_actor_id,p_reason_code,v_now);
  UPDATE agent_secret_handles SET state='revoked',revoked_at=v_now
  WHERE intent_id=p_intent_id AND state='active';
  RETURN NEXT v_intent;
END
$function$;

CREATE FUNCTION agent_effect_claim_commit(
  p_user_id UUID,p_run_id TEXT,p_step_id TEXT,p_attempt_id TEXT,p_generation BIGINT,
  p_lease_owner TEXT,p_lease_token_hash TEXT,p_snapshot_fingerprint TEXT,
  p_grant_fingerprint TEXT,p_registry_fingerprint TEXT,p_kill_switch_epoch BIGINT,
  p_intent_id TEXT,p_intent_fingerprint TEXT,p_approval_id TEXT,p_idempotency_key TEXT
) RETURNS TABLE(intent agent_effect_intents,replayed BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_intent agent_effect_intents%ROWTYPE;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents
  WHERE id=p_intent_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_EFFECT_NOT_FOUND';END IF;
  IF v_intent.intent_fingerprint<>p_intent_fingerprint
     OR v_intent.idempotency_key<>p_idempotency_key THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
  END IF;
  IF v_intent.run_id<>p_run_id OR v_intent.step_id<>p_step_id
     OR v_intent.attempt_id<>p_attempt_id OR v_intent.lease_generation<>p_generation
     OR v_intent.lease_owner<>p_lease_owner OR v_intent.lease_token_hash<>p_lease_token_hash
     OR v_intent.snapshot_fingerprint<>p_snapshot_fingerprint
     OR v_intent.grant_fingerprint<>p_grant_fingerprint
     OR v_intent.registry_fingerprint<>p_registry_fingerprint
     OR v_intent.kill_switch_epoch<>p_kill_switch_epoch THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='LEASE_STALE';
  END IF;
  IF v_intent.state IN ('committed','failed','outcome_unknown')
     AND v_intent.approval_class<>'automatic' AND NOT EXISTS(
    SELECT 1 FROM agent_effect_approvals approval
    WHERE approval.approval_id=p_approval_id AND approval.intent_id=p_intent_id
      AND approval.intent_fingerprint=p_intent_fingerprint AND approval.decision='approved') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='APPROVAL_REQUIRED';
  END IF;
  IF v_intent.state IN ('committed','failed','rejected','canceled','expired','outcome_unknown') THEN
    RETURN QUERY SELECT v_intent,true;RETURN;
  END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_grant_revocations revocation
      WHERE revocation.grant_id=v_intent.grant_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='GRANT_DENIED';
  END IF;
  IF v_intent.state='committing' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
  END IF;
  IF v_intent.expires_at<=v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INTENT_EXPIRED';
  END IF;
  IF v_intent.state<>'approved' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='APPROVAL_REQUIRED';
  END IF;
  IF v_intent.approval_class<>'automatic' AND NOT EXISTS(
    SELECT 1 FROM agent_effect_approvals approval
    WHERE approval.approval_id=p_approval_id AND approval.intent_id=p_intent_id
      AND approval.intent_fingerprint=p_intent_fingerprint AND approval.decision='approved') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='APPROVAL_REQUIRED';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_run_id,86420));
  IF (SELECT count(*) FROM agent_effect_intents other_intent
      WHERE other_intent.run_id=v_intent.run_id
        AND other_intent.capability=v_intent.capability
        AND other_intent.state IN ('committing','committed'))>=v_intent.capability_max_calls
     OR (SELECT count(*) FROM agent_effect_intents other_intent
         WHERE other_intent.run_id=v_intent.run_id
           AND other_intent.state IN ('committing','committed'))>=v_intent.grant_max_tool_calls THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='BUDGET_EXHAUSTED';
  END IF;
  PERFORM agent_orchestrator_effect_fence(
    'commit',p_intent_id,p_user_id,p_run_id,p_step_id,p_attempt_id,p_generation,
    p_lease_owner,p_lease_token_hash,p_snapshot_fingerprint,p_kill_switch_epoch,NULL);
  UPDATE agent_effect_intents SET state='committing',committing_at=v_now
  WHERE id=p_intent_id RETURNING * INTO v_intent;
  RETURN QUERY SELECT v_intent,false;
END
$function$;

CREATE FUNCTION agent_effect_complete_commit(
  p_intent_id TEXT,p_state TEXT,p_receipt_fingerprint TEXT,
  p_executor_status_digest TEXT,p_error_code TEXT
) RETURNS SETOF agent_effect_intents
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_intent agent_effect_intents%ROWTYPE;
  v_attempt_terminal TEXT;
  v_outcome TEXT;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents WHERE id=p_intent_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_EFFECT_NOT_FOUND';END IF;
  IF v_intent.state IN ('committed','failed','outcome_unknown') THEN
    IF v_intent.state<>p_state OR COALESCE(v_intent.receipt_fingerprint,'')<>COALESCE(p_receipt_fingerprint,'')
       OR COALESCE(v_intent.executor_status_digest,'')<>COALESCE(p_executor_status_digest,'')
       OR COALESCE(v_intent.error_code,'')<>COALESCE(p_error_code,'') THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN NEXT v_intent;RETURN;
  END IF;
  IF v_intent.state<>'committing' OR p_state NOT IN ('committed','failed','outcome_unknown') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='INVALID_TRANSITION';
  END IF;
  v_attempt_terminal:=CASE p_state WHEN 'committed' THEN 'succeeded'
    WHEN 'outcome_unknown' THEN 'outcome_unknown' ELSE 'failed' END;
  PERFORM agent_orchestrator_effect_fence(
    'complete',p_intent_id,v_intent.user_id,v_intent.run_id,v_intent.step_id,
    v_intent.attempt_id,v_intent.lease_generation,v_intent.lease_owner,
    v_intent.lease_token_hash,v_intent.snapshot_fingerprint,
    v_intent.kill_switch_epoch,v_attempt_terminal);
  UPDATE agent_effect_intents SET state=p_state,receipt_fingerprint=NULLIF(p_receipt_fingerprint,''),
    executor_status_digest=NULLIF(p_executor_status_digest,''),error_code=NULLIF(p_error_code,''),
    terminal_at=v_now WHERE id=p_intent_id RETURNING * INTO v_intent;
  v_outcome:=CASE p_state WHEN 'committed' THEN 'committed'
    WHEN 'outcome_unknown' THEN 'outcome_unknown' ELSE 'failed' END;
  INSERT INTO agent_effect_receipts(
    idempotency_key,intent_id,intent_fingerprint,outcome,receipt_fingerprint,
    executor_status_digest,error_code,completed_at
  ) VALUES(v_intent.idempotency_key,v_intent.id,v_intent.intent_fingerprint,v_outcome,
    NULLIF(p_receipt_fingerprint,''),NULLIF(p_executor_status_digest,''),
    NULLIF(p_error_code,''),v_now);
  UPDATE agent_secret_handles SET state='revoked',revoked_at=v_now
  WHERE intent_id=p_intent_id AND state='active';
  RETURN NEXT v_intent;
END
$function$;

CREATE FUNCTION agent_effect_get_intent(p_user_id UUID,p_intent_id TEXT)
RETURNS SETOF agent_effect_intents
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT * FROM agent_effect_intents WHERE id=p_intent_id AND user_id=p_user_id
$function$;

CREATE FUNCTION agent_effect_list_committing(p_limit INTEGER)
RETURNS SETOF agent_effect_intents
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT * FROM agent_effect_intents WHERE state='committing'
  ORDER BY committing_at,id LIMIT LEAST(GREATEST(p_limit,1),1000)
$function$;

CREATE FUNCTION agent_effect_expire(p_cutoff TIMESTAMPTZ,p_limit INTEGER)
RETURNS INTEGER
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_count INTEGER;
BEGIN
  WITH targets AS (
    SELECT id FROM agent_effect_intents
    WHERE state IN ('prepared','awaiting_approval','approved') AND expires_at<=p_cutoff
    ORDER BY expires_at,id LIMIT LEAST(GREATEST(p_limit,1),1000) FOR UPDATE SKIP LOCKED
  ), changed AS (
    UPDATE agent_effect_intents intent SET state='expired',terminal_at=clock_timestamp(),
      error_code='INTENT_EXPIRED' FROM targets WHERE intent.id=targets.id RETURNING intent.id
  ) SELECT count(*) INTO v_count FROM changed;
  UPDATE agent_secret_handles SET state='expired',revoked_at=clock_timestamp()
  WHERE state='active' AND expires_at<=p_cutoff;
  UPDATE agent_secret_handles handle SET state='revoked',revoked_at=clock_timestamp()
  FROM agent_effect_intents intent,agent_attempts attempt,agent_runs run
  WHERE handle.state='active' AND handle.intent_id=intent.id
    AND attempt.id=intent.attempt_id AND attempt.run_id=intent.run_id
    AND attempt.user_id=intent.user_id AND run.id=intent.run_id
    AND run.user_id=intent.user_id AND (
      intent.state<>'committing' OR attempt.state<>'committing'
      OR attempt.generation<>intent.lease_generation
      OR attempt.lease_expires_at<=p_cutoff
      OR (SELECT epoch FROM agent_kill_switch_state WHERE singleton)<>intent.kill_switch_epoch
      OR agent_orchestrator_active_kill_mode(run.scope_keys) IS NOT NULL);
  RETURN v_count;
END
$function$;

CREATE FUNCTION agent_secret_handle_create(
  p_handle_digest TEXT,p_intent_id TEXT,p_secret_ref TEXT,
  p_binding_fingerprint TEXT,p_expires_at TIMESTAMPTZ,p_binding JSONB
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_now TIMESTAMPTZ:=clock_timestamp();
  v_intent agent_effect_intents%ROWTYPE;
BEGIN
  SELECT * INTO v_intent FROM agent_effect_intents WHERE id=p_intent_id FOR UPDATE;
  IF p_expires_at<=v_now OR p_expires_at>v_now+interval '1 hour'
     OR NOT FOUND OR v_intent.state<>'committing' OR v_intent.expires_at<=v_now
     OR p_expires_at>v_intent.expires_at OR jsonb_typeof(p_binding)<>'object'
     OR NOT (p_binding ?& ARRAY['subject','runId','stepId','attemptId','generation',
          'capability','action','destinationFingerprint','intentFingerprint'])
     OR (p_binding-ARRAY['subject','runId','stepId','attemptId','generation','capability',
          'action','destinationFingerprint','intentFingerprint']::TEXT[])<>'{}'::jsonb
     OR jsonb_typeof(p_binding->'subject')<>'object'
     OR NOT ((p_binding->'subject') ?& ARRAY['userId','projectId','assistantId'])
     OR ((p_binding->'subject')-ARRAY['userId','projectId','assistantId']::TEXT[])<>'{}'::jsonb
     OR p_binding#>>'{subject,userId}' IS DISTINCT FROM v_intent.user_id::TEXT
     OR p_binding#>>'{subject,projectId}' IS DISTINCT FROM v_intent.project_id
     OR p_binding#>>'{subject,assistantId}' IS DISTINCT FROM v_intent.assistant_id
     OR p_binding->>'runId' IS DISTINCT FROM v_intent.run_id
     OR p_binding->>'stepId' IS DISTINCT FROM v_intent.step_id
     OR p_binding->>'attemptId' IS DISTINCT FROM v_intent.attempt_id
     OR (CASE WHEN p_binding->>'generation' ~ '^[1-9][0-9]*$'
       THEN (p_binding->>'generation')::BIGINT<>v_intent.lease_generation ELSE true END)
     OR p_binding->>'capability' IS DISTINCT FROM v_intent.capability
     OR p_binding->>'action' IS DISTINCT FROM v_intent.action
     OR p_binding->>'intentFingerprint' IS DISTINCT FROM v_intent.intent_fingerprint
     OR COALESCE(p_binding->>'destinationFingerprint' !~ '^sha256:[a-f0-9]{64}$',true) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SECRET_DENIED';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_grant_revocations revocation
      WHERE revocation.grant_id=v_intent.grant_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='GRANT_DENIED';
  END IF;
  PERFORM agent_orchestrator_effect_fence(
    'secret',p_intent_id,v_intent.user_id,v_intent.run_id,v_intent.step_id,
    v_intent.attempt_id,v_intent.lease_generation,v_intent.lease_owner,
    v_intent.lease_token_hash,v_intent.snapshot_fingerprint,
    v_intent.kill_switch_epoch,NULL);
  INSERT INTO agent_secret_handles(
    handle_digest,intent_id,secret_ref,binding_fingerprint,state,expires_at,created_at
  ) VALUES(p_handle_digest,p_intent_id,p_secret_ref,p_binding_fingerprint,'active',p_expires_at,v_now);
  RETURN true;
END
$function$;

CREATE FUNCTION agent_secret_handle_consume(
  p_handle_digest TEXT,p_binding_fingerprint TEXT,p_used_at TIMESTAMPTZ
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_intent agent_effect_intents%ROWTYPE;
  v_now TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT intent.* INTO v_intent FROM agent_secret_handles handle
  JOIN agent_effect_intents intent ON intent.id=handle.intent_id
  WHERE handle.handle_digest=p_handle_digest
    AND handle.binding_fingerprint=p_binding_fingerprint
    AND handle.state='active' AND handle.expires_at>v_now
  FOR UPDATE OF handle,intent;
  IF p_used_at<v_now-interval '30 seconds' OR p_used_at>v_now+interval '30 seconds'
     OR NOT FOUND OR v_intent.state<>'committing' OR v_intent.expires_at<=v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SECRET_DENIED';
  END IF;
  IF EXISTS(SELECT 1 FROM agent_effect_grant_revocations revocation
      WHERE revocation.grant_id=v_intent.grant_id) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='GRANT_DENIED';
  END IF;
  PERFORM agent_orchestrator_effect_fence(
    'secret',v_intent.id,v_intent.user_id,v_intent.run_id,v_intent.step_id,
    v_intent.attempt_id,v_intent.lease_generation,v_intent.lease_owner,
    v_intent.lease_token_hash,v_intent.snapshot_fingerprint,
    v_intent.kill_switch_epoch,NULL);
  UPDATE agent_secret_handles SET state='used',used_at=v_now
  WHERE handle_digest=p_handle_digest AND binding_fingerprint=p_binding_fingerprint
    AND state='active' AND expires_at>v_now;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SECRET_DENIED';END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_secret_handle_revoke(
  p_handle_digest TEXT,p_revoked_at TIMESTAMPTZ
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  UPDATE agent_secret_handles SET state='revoked',revoked_at=p_revoked_at
  WHERE handle_digest=p_handle_digest AND state='active';
  RETURN FOUND;
END
$function$;

CREATE FUNCTION agent_secret_handle_revoke_intent(p_intent_id TEXT,p_revoked_at TIMESTAMPTZ)
RETURNS INTEGER
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE v_count INTEGER;
BEGIN
  WITH changed AS (
    UPDATE agent_secret_handles SET state='revoked',revoked_at=p_revoked_at
    WHERE intent_id=p_intent_id AND state='active' RETURNING handle_digest
  ) SELECT count(*) INTO v_count FROM changed;
  RETURN v_count;
END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema(); function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'agent_orchestrator_effect_fence(text,text,uuid,text,text,text,bigint,text,text,text,bigint,text)',
    'agent_effect_prepare(text,text,text,text,text,uuid,text,text,text,text,text,bigint,text,text,text,text,text,text,text,text,text,text,text,jsonb,text,text,integer,integer,bigint,timestamp with time zone,timestamp with time zone)',
    'agent_effect_decide_approval(text,uuid,text,text,text,text,text,text,bigint)',
	'agent_effect_cancel(text,uuid,text,text,text,text,text)',
    'agent_effect_claim_commit(uuid,text,text,text,bigint,text,text,text,text,text,bigint,text,text,text,text)',
    'agent_effect_complete_commit(text,text,text,text,text)',
    'agent_effect_revoke_grant(text,text,text,text,text)',
    'agent_effect_get_intent(uuid,text)','agent_effect_list_committing(integer)',
    'agent_effect_expire(timestamp with time zone,integer)'
	,'agent_secret_handle_create(text,text,text,text,timestamp with time zone,jsonb)'
	,'agent_secret_handle_consume(text,text,timestamp with time zone)'
	,'agent_secret_handle_revoke(text,timestamp with time zone)'
	,'agent_secret_handle_revoke_intent(text,timestamp with time zone)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,function_identity,schema_name);
  END LOOP;
END
$harden$;

ALTER TABLE agent_effect_intents OWNER TO agent_effect_owner;
ALTER TABLE agent_effect_approvals OWNER TO agent_effect_owner;
ALTER TABLE agent_effect_receipts OWNER TO agent_effect_owner;
ALTER TABLE agent_effect_grant_revocations OWNER TO agent_effect_owner;
ALTER TABLE agent_effect_cancellations OWNER TO agent_effect_owner;
ALTER TABLE agent_secret_handles OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_immutable() OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_prepare(TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,TEXT,INTEGER,INTEGER,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_decide_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_cancel(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_claim_commit(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_complete_commit(TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_revoke_grant(TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_get_intent(UUID,TEXT) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_list_committing(INTEGER) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_effect_expire(TIMESTAMPTZ,INTEGER) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_secret_handle_create(TEXT,TEXT,TEXT,TEXT,TIMESTAMPTZ,JSONB) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_secret_handle_consume(TEXT,TEXT,TIMESTAMPTZ) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_secret_handle_revoke(TEXT,TIMESTAMPTZ) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_secret_handle_revoke_intent(TEXT,TIMESTAMPTZ) OWNER TO agent_effect_owner;
ALTER FUNCTION agent_orchestrator_effect_fence(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT) OWNER TO agent_orchestrator_owner;

REVOKE ALL ON agent_effect_intents,agent_effect_approvals,agent_effect_receipts,
  agent_effect_grant_revocations,agent_effect_cancellations,agent_secret_handles FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,
  agent_runner_control,agent_effect_control;
GRANT SELECT ON agent_effect_intents,agent_effect_approvals,agent_effect_receipts,
  agent_effect_grant_revocations,agent_effect_cancellations,agent_secret_handles TO agent_effect_control;

REVOKE ALL ON FUNCTION agent_effect_immutable(),
  agent_orchestrator_effect_fence(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,
  agent_effect_control;
GRANT EXECUTE ON FUNCTION agent_orchestrator_effect_fence(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,BIGINT,TEXT)
  TO agent_effect_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_append_event(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TIMESTAMPTZ,JSONB,TIMESTAMPTZ),
  agent_orchestrator_active_kill_mode(TEXT[]) TO agent_orchestrator_owner;

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
GRANT EXECUTE ON FUNCTION
  agent_effect_prepare(TEXT,TEXT,TEXT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT,TEXT,INTEGER,INTEGER,BIGINT,TIMESTAMPTZ,TIMESTAMPTZ),
  agent_effect_decide_approval(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT),
	  agent_effect_cancel(TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_claim_commit(UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_complete_commit(TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_revoke_grant(TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_effect_get_intent(UUID,TEXT),agent_effect_list_committing(INTEGER),
  agent_effect_expire(TIMESTAMPTZ,INTEGER) TO agent_effect_control;
GRANT EXECUTE ON FUNCTION
  agent_secret_handle_create(TEXT,TEXT,TEXT,TEXT,TIMESTAMPTZ,JSONB),
  agent_secret_handle_consume(TEXT,TEXT,TIMESTAMPTZ),
  agent_secret_handle_revoke(TEXT,TIMESTAMPTZ),
  agent_secret_handle_revoke_intent(TEXT,TIMESTAMPTZ) TO agent_effect_control;

DO $schema_privileges$
BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_effect_owner, agent_effect_control, go_api_runtime',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_effect_owner, agent_effect_control',current_schema());
END
$schema_privileges$;
