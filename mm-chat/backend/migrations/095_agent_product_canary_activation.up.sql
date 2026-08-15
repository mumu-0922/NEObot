-- G21.6 opens one fixed product canary through a durable API-to-worker handoff.
-- The HTTP role can append only an eligible current-user request. A disjoint
-- worker role claims the request and composes existing Orchestrator/Runner
-- authority. Final promotion remains a separate operator-only append.

DO $roles$
DECLARE can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create FROM pg_roles WHERE rolname=current_user;
  IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_product_canary_worker') THEN
    IF NOT can_create THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PRODUCT_CANARY_REQUIRED_ROLE_MISSING';
    END IF;
    CREATE ROLE agent_product_canary_worker NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='agent_product_canary_worker' AND
    (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PRODUCT_CANARY_ROLE_MUST_BE_RESTRICTED';
  END IF;
  IF pg_has_role('agent_product_canary_worker','agent_product_owner','MEMBER')
     OR pg_has_role('agent_product_canary_worker','agent_orchestrator_runtime','MEMBER')
     OR pg_has_role('agent_product_canary_worker','agent_runner_control','MEMBER')
     OR pg_has_role('agent_product_canary_worker','agent_effect_control','MEMBER')
     OR pg_has_role('agent_product_canary_worker','agent_delegation_control','MEMBER')
     OR pg_has_role('agent_product_canary_worker','agent_cron_control','MEMBER')
     OR pg_has_role('agent_product_canary_worker','agent_learning_control','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_PRODUCT_CANARY_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE TABLE agent_product_canary_activations(
  activation_id TEXT PRIMARY KEY,
  release_commit TEXT NOT NULL,
  policy_revision BIGINT NOT NULL REFERENCES agent_shadow_policies(revision) ON DELETE RESTRICT,
  admission_id UUID NOT NULL,
  package_fingerprint TEXT NOT NULL,
  runtime_bundle_fingerprint TEXT NOT NULL,
  plan_fingerprint TEXT NOT NULL,
  control_activation_fingerprint TEXT NOT NULL,
  root_activation_fingerprint TEXT NOT NULL,
  broker_activation_fingerprint TEXT NOT NULL,
  project_activation_fingerprint TEXT NOT NULL,
  child_activation_fingerprint TEXT NOT NULL,
  cron_activation_fingerprint TEXT NOT NULL,
  learning_activation_fingerprint TEXT NOT NULL,
  max_requests INTEGER NOT NULL,
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
  CONSTRAINT agent_product_canary_activation_id CHECK(activation_id~'^activation_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_product_canary_release CHECK(release_commit~'^[a-f0-9]{40}$'
    AND release_commit<>'0000000000000000000000000000000000000000'),
  CONSTRAINT agent_product_canary_fingerprints CHECK(
    package_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND runtime_bundle_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND plan_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND control_activation_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND root_activation_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND broker_activation_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND project_activation_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND child_activation_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND cron_activation_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND learning_activation_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND package_fingerprint<>'sha256:'||repeat('0',64)
    AND runtime_bundle_fingerprint<>'sha256:'||repeat('0',64)
    AND plan_fingerprint<>'sha256:'||repeat('0',64)
    AND control_activation_fingerprint<>'sha256:'||repeat('0',64)
    AND root_activation_fingerprint<>'sha256:'||repeat('0',64)
    AND broker_activation_fingerprint<>'sha256:'||repeat('0',64)
    AND project_activation_fingerprint<>'sha256:'||repeat('0',64)
    AND child_activation_fingerprint<>'sha256:'||repeat('0',64)
    AND cron_activation_fingerprint<>'sha256:'||repeat('0',64)
    AND learning_activation_fingerprint<>'sha256:'||repeat('0',64)
    AND control_activation_fingerprint<>ALL(ARRAY[root_activation_fingerprint,
      broker_activation_fingerprint,project_activation_fingerprint,
      child_activation_fingerprint,cron_activation_fingerprint,learning_activation_fingerprint])
    AND root_activation_fingerprint<>ALL(ARRAY[broker_activation_fingerprint,
      project_activation_fingerprint,child_activation_fingerprint,
      cron_activation_fingerprint,learning_activation_fingerprint])
    AND broker_activation_fingerprint<>ALL(ARRAY[project_activation_fingerprint,
      child_activation_fingerprint,cron_activation_fingerprint,learning_activation_fingerprint])
    AND project_activation_fingerprint<>ALL(ARRAY[child_activation_fingerprint,
      cron_activation_fingerprint,learning_activation_fingerprint])
    AND child_activation_fingerprint<>ALL(ARRAY[cron_activation_fingerprint,
      learning_activation_fingerprint])
    AND cron_activation_fingerprint<>learning_activation_fingerprint),
  CONSTRAINT agent_product_canary_budget CHECK(max_requests BETWEEN 1 AND 20),
  CONSTRAINT agent_product_canary_window CHECK(
    valid_until>valid_from AND valid_until<=valid_from+interval '24 hours'),
  CONSTRAINT agent_product_canary_actor CHECK(
    actor_type='operator' AND octet_length(actor_id) BETWEEN 1 AND 128
    AND reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_product_canary_activation_state CHECK(
    (enabled AND disabled_at IS NULL AND disabled_actor_id IS NULL AND disabled_reason_code IS NULL)
    OR (NOT enabled AND disabled_at IS NOT NULL
      AND octet_length(disabled_actor_id) BETWEEN 1 AND 128
      AND disabled_reason_code~'^[A-Z][A-Z0-9_]{0,63}$')),
  UNIQUE(policy_revision),
  UNIQUE(activation_id,policy_revision)
);

CREATE TABLE agent_product_canary_requests(
  request_id TEXT PRIMARY KEY,
  activation_id TEXT NOT NULL REFERENCES agent_product_canary_activations(activation_id) ON DELETE RESTRICT,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  policy_revision BIGINT NOT NULL,
  opt_generation BIGINT NOT NULL,
  admission_id UUID NOT NULL,
  package_fingerprint TEXT NOT NULL,
  runtime_bundle_fingerprint TEXT NOT NULL,
  plan_fingerprint TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'queued',
  claim_generation BIGINT NOT NULL DEFAULT 0,
  claim_owner TEXT,
  claim_expires_at TIMESTAMPTZ,
  failure_count INTEGER NOT NULL DEFAULT 0,
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  terminal_at TIMESTAMPTZ,
  CONSTRAINT agent_product_canary_request_activation_fk
    FOREIGN KEY(activation_id,policy_revision)
    REFERENCES agent_product_canary_activations(activation_id,policy_revision) ON DELETE RESTRICT,
  CONSTRAINT agent_product_canary_request_id CHECK(request_id~'^product_request_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_product_canary_request_generation CHECK(opt_generation>=1 AND claim_generation>=0),
  CONSTRAINT agent_product_canary_request_fingerprints CHECK(
    package_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND runtime_bundle_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND plan_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND request_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_product_canary_request_state CHECK(state IN ('queued','claimed','completed','failed')),
  CONSTRAINT agent_product_canary_request_failure CHECK(
    failure_count BETWEEN 0 AND 3 AND (error_code IS NULL OR error_code~'^[A-Z][A-Z0-9_]{0,63}$')),
  CONSTRAINT agent_product_canary_request_shape CHECK(
    (state='queued' AND claim_owner IS NULL AND claim_expires_at IS NULL AND terminal_at IS NULL)
    OR (state='claimed' AND claim_generation>=1 AND octet_length(claim_owner) BETWEEN 1 AND 128
      AND claim_expires_at IS NOT NULL AND terminal_at IS NULL)
    OR (state IN ('completed','failed') AND claim_generation>=1
      AND octet_length(claim_owner) BETWEEN 1 AND 128
      AND claim_expires_at IS NOT NULL AND terminal_at IS NOT NULL)),
  UNIQUE(user_id,policy_revision,opt_generation)
);
CREATE INDEX idx_agent_product_canary_request_claim
  ON agent_product_canary_requests(activation_id,state,claim_expires_at,created_at,request_id);

CREATE TABLE agent_product_canary_receipts(
  request_id TEXT PRIMARY KEY REFERENCES agent_product_canary_requests(request_id) ON DELETE RESTRICT,
  activation_id TEXT NOT NULL REFERENCES agent_product_canary_activations(activation_id) ON DELETE RESTRICT,
  user_id UUID NOT NULL,
  run_id TEXT NOT NULL,
  attempt_id TEXT NOT NULL,
  run_snapshot_fingerprint TEXT NOT NULL,
  plan_fingerprint TEXT NOT NULL,
  receipt_fingerprint TEXT NOT NULL,
  outcome TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_product_canary_receipt_run_fk
    FOREIGN KEY(run_id,user_id) REFERENCES agent_runs(id,user_id) ON DELETE RESTRICT,
  CONSTRAINT agent_product_canary_receipt_attempt_fk
    FOREIGN KEY(run_id,user_id,attempt_id) REFERENCES agent_attempts(run_id,user_id,id) ON DELETE RESTRICT,
  CONSTRAINT agent_product_canary_receipt_ids CHECK(
    run_id~'^run_[a-z0-9]{16,64}$' AND attempt_id~'^attempt_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_product_canary_receipt_fingerprints CHECK(
    run_snapshot_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND plan_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND receipt_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_product_canary_receipt_outcome CHECK(outcome='bounded_canary_passed'),
  UNIQUE(run_id),UNIQUE(attempt_id),UNIQUE(receipt_fingerprint)
);

CREATE TABLE agent_product_canary_promotions(
  promotion_id TEXT PRIMARY KEY,
  activation_id TEXT NOT NULL REFERENCES agent_product_canary_activations(activation_id) ON DELETE RESTRICT,
  request_id TEXT NOT NULL REFERENCES agent_product_canary_receipts(request_id) ON DELETE RESTRICT,
  release_commit TEXT NOT NULL,
  closure_fingerprint TEXT NOT NULL,
  receipt_fingerprint TEXT NOT NULL,
  decision TEXT NOT NULL,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_product_canary_promotion_id CHECK(promotion_id~'^promotion_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_product_canary_promotion_release CHECK(release_commit~'^[a-f0-9]{40}$'),
  CONSTRAINT agent_product_canary_promotion_fingerprints CHECK(
    closure_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND receipt_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_product_canary_promotion_decision CHECK(decision='PROMOTION_READY'),
  CONSTRAINT agent_product_canary_promotion_actor CHECK(
    actor_type='operator' AND octet_length(actor_id) BETWEEN 1 AND 128
    AND reason_code~'^[A-Z][A-Z0-9_]{0,63}$'),
  UNIQUE(activation_id),UNIQUE(closure_fingerprint)
);

CREATE FUNCTION agent_product_canary_activation_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_PRODUCT_CANARY_ACTIVATION_IMMUTABLE';
  END IF;
  IF to_jsonb(NEW)-ARRAY['enabled','disabled_at','disabled_actor_id','disabled_reason_code']::text[]
       <>to_jsonb(OLD)-ARRAY['enabled','disabled_at','disabled_actor_id','disabled_reason_code']::text[]
     OR NOT OLD.enabled OR NEW.enabled OR NEW.disabled_at IS NULL
     OR NEW.disabled_actor_id IS NULL OR octet_length(NEW.disabled_actor_id) NOT BETWEEN 1 AND 128
     OR NEW.disabled_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_PRODUCT_CANARY_ACTIVATION_IMMUTABLE';
  END IF;
  RETURN NEW;
END
$function$;
CREATE TRIGGER trg_agent_product_canary_activation_guard BEFORE UPDATE OR DELETE
  ON agent_product_canary_activations FOR EACH ROW EXECUTE FUNCTION agent_product_canary_activation_guard();

CREATE FUNCTION agent_product_canary_request_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_PRODUCT_CANARY_REQUEST_IMMUTABLE';
  END IF;
  IF to_jsonb(NEW)-ARRAY['state','claim_generation','claim_owner','claim_expires_at',
       'failure_count','error_code','updated_at','terminal_at']::text[]
     <>to_jsonb(OLD)-ARRAY['state','claim_generation','claim_owner','claim_expires_at',
       'failure_count','error_code','updated_at','terminal_at']::text[]
     OR NEW.updated_at<OLD.updated_at OR NEW.failure_count<OLD.failure_count
     OR NOT ((OLD.state='queued' AND NEW.state='claimed' AND NEW.claim_generation=OLD.claim_generation+1)
       OR (OLD.state='claimed' AND NEW.state='claimed' AND NEW.claim_generation=OLD.claim_generation+1)
       OR (OLD.state='claimed' AND NEW.state='queued' AND NEW.claim_generation=OLD.claim_generation)
       OR (OLD.state='claimed' AND NEW.state IN ('completed','failed') AND NEW.claim_generation=OLD.claim_generation)) THEN
    RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_PRODUCT_CANARY_REQUEST_IMMUTABLE';
  END IF;
  RETURN NEW;
END
$function$;
CREATE TRIGGER trg_agent_product_canary_request_guard BEFORE UPDATE OR DELETE
  ON agent_product_canary_requests FOR EACH ROW EXECUTE FUNCTION agent_product_canary_request_guard();
CREATE TRIGGER trg_agent_product_canary_receipt_immutable BEFORE UPDATE OR DELETE
  ON agent_product_canary_receipts FOR EACH ROW EXECUTE FUNCTION agent_product_fact_immutable();
CREATE TRIGGER trg_agent_product_canary_promotion_immutable BEFORE UPDATE OR DELETE
  ON agent_product_canary_promotions FOR EACH ROW EXECUTE FUNCTION agent_product_fact_immutable();

CREATE FUNCTION agent_product_canary_provision_activation(
  p_activation_id TEXT,p_release_commit TEXT,p_policy_revision BIGINT,
  p_admission_id UUID,p_package_fingerprint TEXT,p_runtime_fingerprint TEXT,
  p_plan_fingerprint TEXT,p_control_fingerprint TEXT,p_root_fingerprint TEXT,
  p_broker_fingerprint TEXT,p_project_fingerprint TEXT,p_child_fingerprint TEXT,
  p_cron_fingerprint TEXT,p_learning_fingerprint TEXT,p_max_requests INTEGER,
  p_valid_from TIMESTAMPTZ,p_valid_until TIMESTAMPTZ,p_actor_id TEXT,p_reason_code TEXT
) RETURNS agent_product_canary_activations
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE activation agent_product_canary_activations%ROWTYPE;policy agent_shadow_policies%ROWTYPE;
BEGIN
  IF p_activation_id!~'^activation_[a-z0-9]{16,64}$'
     OR p_release_commit!~'^[a-f0-9]{40}$' OR p_release_commit=repeat('0',40)
     OR p_package_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_runtime_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_plan_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_control_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_root_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_broker_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_project_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_child_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_cron_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_learning_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR 'sha256:'||repeat('0',64)=ANY(ARRAY[p_package_fingerprint,p_runtime_fingerprint,
       p_plan_fingerprint,p_control_fingerprint,p_root_fingerprint,p_broker_fingerprint,
       p_project_fingerprint,p_child_fingerprint,p_cron_fingerprint,p_learning_fingerprint])
     OR cardinality(ARRAY(SELECT DISTINCT value FROM unnest(ARRAY[
       p_control_fingerprint,p_root_fingerprint,p_broker_fingerprint,p_project_fingerprint,
       p_child_fingerprint,p_cron_fingerprint,p_learning_fingerprint]) value))<>7
     OR p_max_requests NOT BETWEEN 1 AND 20
     OR p_valid_until<=p_valid_from OR p_valid_until>p_valid_from+interval '24 hours'
     OR octet_length(p_actor_id) NOT BETWEEN 1 AND 128
     OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_ACTIVATION_INVALID';
  END IF;
  SELECT * INTO activation FROM agent_product_canary_activations WHERE activation_id=p_activation_id;
  IF FOUND THEN
    IF activation.release_commit<>p_release_commit OR activation.policy_revision<>p_policy_revision
       OR activation.admission_id<>p_admission_id OR activation.package_fingerprint<>p_package_fingerprint
       OR activation.runtime_bundle_fingerprint<>p_runtime_fingerprint
       OR activation.plan_fingerprint<>p_plan_fingerprint
       OR activation.control_activation_fingerprint<>p_control_fingerprint
       OR activation.root_activation_fingerprint<>p_root_fingerprint
       OR activation.broker_activation_fingerprint<>p_broker_fingerprint
       OR activation.project_activation_fingerprint<>p_project_fingerprint
       OR activation.child_activation_fingerprint<>p_child_fingerprint
       OR activation.cron_activation_fingerprint<>p_cron_fingerprint
       OR activation.learning_activation_fingerprint<>p_learning_fingerprint
       OR activation.max_requests<>p_max_requests OR activation.valid_from<>p_valid_from
       OR activation.valid_until<>p_valid_until OR activation.actor_id<>p_actor_id
       OR activation.reason_code<>p_reason_code THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN activation;
  END IF;
  SELECT * INTO policy FROM agent_shadow_policies WHERE revision=p_policy_revision FOR SHARE;
  IF NOT FOUND OR NOT policy.enabled OR policy.admission_id<>p_admission_id
     OR policy.package_fingerprint<>p_package_fingerprint
     OR policy.runtime_bundle_fingerprint<>p_runtime_fingerprint
     OR policy.mode<>'read_only' OR policy.starts_at>p_valid_from OR policy.expires_at<p_valid_until
     OR NOT EXISTS(SELECT 1 FROM skill_package_candidates candidate
       JOIN skill_package_versions package ON package.package_fingerprint=candidate.package_fingerprint
       WHERE candidate.id=p_admission_id AND candidate.status='admitted'
         AND candidate.package_fingerprint=p_package_fingerprint
         AND package.runtime_bundle_fingerprint=p_runtime_fingerprint AND package.has_runtime) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PRODUCT_CANARY_ACTIVATION_DRIFT';
  END IF;
  INSERT INTO agent_product_canary_activations(
    activation_id,release_commit,policy_revision,admission_id,package_fingerprint,
    runtime_bundle_fingerprint,plan_fingerprint,control_activation_fingerprint,
    root_activation_fingerprint,broker_activation_fingerprint,project_activation_fingerprint,
    child_activation_fingerprint,cron_activation_fingerprint,learning_activation_fingerprint,
    max_requests,valid_from,valid_until,actor_type,actor_id,reason_code)
  VALUES(p_activation_id,p_release_commit,p_policy_revision,p_admission_id,p_package_fingerprint,
    p_runtime_fingerprint,p_plan_fingerprint,p_control_fingerprint,p_root_fingerprint,
    p_broker_fingerprint,p_project_fingerprint,p_child_fingerprint,p_cron_fingerprint,
    p_learning_fingerprint,p_max_requests,p_valid_from,p_valid_until,'operator',p_actor_id,p_reason_code)
  RETURNING * INTO activation;
  RETURN activation;
END
$function$;

CREATE FUNCTION agent_product_canary_disable_activation(
  p_activation_id TEXT,p_actor_id TEXT,p_reason_code TEXT
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE activation agent_product_canary_activations%ROWTYPE;
BEGIN
  IF octet_length(p_actor_id) NOT BETWEEN 1 AND 128 OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_ACTIVATION_INVALID';
  END IF;
  SELECT * INTO activation FROM agent_product_canary_activations WHERE activation_id=p_activation_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PRODUCT_CANARY_ACTIVATION_NOT_FOUND';END IF;
  IF NOT activation.enabled THEN RETURN false;END IF;
  UPDATE agent_product_canary_activations SET enabled=false,disabled_at=clock_timestamp(),
    disabled_actor_id=p_actor_id,disabled_reason_code=p_reason_code WHERE activation_id=p_activation_id;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_product_canary_status(p_user_id UUID)
RETURNS TABLE(activation_id TEXT,plan_fingerprint TEXT,effective BOOLEAN,
  held_reason_code TEXT,remaining_requests INTEGER)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE shadow RECORD;activation agent_product_canary_activations%ROWTYPE;used INTEGER:=0;
BEGIN
  SELECT * INTO shadow FROM agent_product_shadow_snapshot(p_user_id);
  IF shadow.policy_revision=0 OR NOT shadow.eligible THEN
    RETURN QUERY SELECT ''::text,''::text,false,shadow.held_reason_code,GREATEST(0,0);
    RETURN;
  END IF;
  SELECT * INTO activation FROM agent_product_canary_activations item
  WHERE item.policy_revision=shadow.policy_revision AND item.enabled
    AND clock_timestamp()>=item.valid_from AND clock_timestamp()<item.valid_until
    AND item.admission_id=shadow.admission_id
    AND item.package_fingerprint=shadow.package_fingerprint
    AND item.runtime_bundle_fingerprint=shadow.runtime_bundle_fingerprint
  ORDER BY item.created_at DESC LIMIT 1;
  IF NOT FOUND THEN
    RETURN QUERY SELECT ''::text,''::text,false,'ISOLATION_UNAVAILABLE'::text,0;
    RETURN;
  END IF;
  SELECT count(*)::integer INTO used FROM agent_product_canary_requests request
    WHERE request.activation_id=activation.activation_id;
  IF used>=activation.max_requests THEN
    RETURN QUERY SELECT activation.activation_id,activation.plan_fingerprint,false,'BUDGET_EXCEEDED'::text,0;
    RETURN;
  END IF;
  RETURN QUERY SELECT activation.activation_id,activation.plan_fingerprint,true,
    'PRODUCT_CANARY_READY'::text,activation.max_requests-used;
END
$function$;

CREATE FUNCTION agent_product_canary_enqueue(
  p_request_id TEXT,p_user_id UUID,p_expected_policy_revision BIGINT,
  p_expected_generation BIGINT,p_request_fingerprint TEXT
) RETURNS agent_product_canary_requests
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE shadow RECORD;status RECORD;activation agent_product_canary_activations%ROWTYPE;
  existing agent_product_canary_requests%ROWTYPE;created agent_product_canary_requests%ROWTYPE;
  expected_fingerprint TEXT;
BEGIN
  IF p_request_id!~'^product_request_[a-z0-9]{16,64}$'
     OR p_expected_policy_revision<1 OR p_expected_generation<1
     OR p_request_fingerprint!~'^sha256:[a-f0-9]{64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_REQUEST_INVALID';
  END IF;
  expected_fingerprint:='sha256:'||encode(sha256(
    convert_to('neo.agent-product-canary-request/v1','UTF8')||decode('00','hex')||
    convert_to(p_user_id::text,'UTF8')||decode('00','hex')||
    convert_to(p_expected_policy_revision::text,'UTF8')||decode('00','hex')||
    convert_to(p_expected_generation::text,'UTF8')),'hex');
  IF p_request_fingerprint<>expected_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_REQUEST_INVALID';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(
    'agent-product-canary-enqueue:'||p_user_id::text||':'||
    p_expected_policy_revision::text||':'||p_expected_generation::text,0));
  SELECT * INTO existing FROM agent_product_canary_requests request
    WHERE request.user_id=p_user_id AND request.policy_revision=p_expected_policy_revision
      AND request.opt_generation=p_expected_generation;
  IF FOUND THEN
    IF existing.request_fingerprint<>p_request_fingerprint THEN
      RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';
    END IF;
    RETURN existing;
  END IF;
  SELECT * INTO shadow FROM agent_product_shadow_snapshot(p_user_id);
  IF shadow.policy_revision<>p_expected_policy_revision OR shadow.opt_generation<>p_expected_generation THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='GENERATION_STALE';
  END IF;
  SELECT * INTO status FROM agent_product_canary_status(p_user_id);
  IF NOT status.effective THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE=status.held_reason_code;
  END IF;
  SELECT * INTO activation FROM agent_product_canary_activations item
    WHERE item.activation_id=status.activation_id FOR UPDATE;
  IF NOT FOUND OR NOT activation.enabled OR clock_timestamp()<activation.valid_from
     OR clock_timestamp()>=activation.valid_until THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='ISOLATION_UNAVAILABLE';
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(activation.activation_id,0));
  IF (SELECT count(*) FROM agent_product_canary_requests request
      WHERE request.activation_id=activation.activation_id)>=activation.max_requests THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='BUDGET_EXCEEDED';
  END IF;
  IF agent_orchestrator_active_kill_mode(ARRAY[
    'global:*','user:'||p_user_id::text,'admission:'||activation.admission_id::text,
    'skill:'||activation.package_fingerprint]) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  INSERT INTO agent_product_canary_requests(request_id,activation_id,user_id,policy_revision,
    opt_generation,admission_id,package_fingerprint,runtime_bundle_fingerprint,
    plan_fingerprint,request_fingerprint)
  VALUES(p_request_id,activation.activation_id,p_user_id,p_expected_policy_revision,
    p_expected_generation,activation.admission_id,activation.package_fingerprint,
    activation.runtime_bundle_fingerprint,activation.plan_fingerprint,p_request_fingerprint)
  RETURNING * INTO created;
  RETURN created;
END
$function$;

CREATE FUNCTION agent_product_canary_worker_get_activation(p_activation_id TEXT)
RETURNS SETOF agent_product_canary_activations
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT activation.* FROM agent_product_canary_activations activation
  WHERE activation.activation_id=p_activation_id
$function$;

CREATE FUNCTION agent_product_canary_worker_health(p_activation_id TEXT,p_now TIMESTAMPTZ)
RETURNS TABLE(stale_claims INTEGER,pending_terminalizations INTEGER)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT
    count(*) FILTER(WHERE request.state='claimed' AND request.claim_expires_at<=p_now)::integer,
    count(*) FILTER(WHERE request.state='claimed' AND request.claim_expires_at>p_now
      AND receipt.request_id IS NULL AND EXISTS(
        SELECT 1 FROM agent_runs run
        JOIN agent_steps step ON step.run_id=run.id AND step.user_id=run.user_id
          AND step.ordinal=0 AND step.kind='product_canary'
        JOIN agent_attempts attempt ON attempt.run_id=run.id AND attempt.user_id=run.user_id
          AND attempt.step_id=step.id
        WHERE run.user_id=request.user_id
          AND run.idempotency_key='g21.6-product-canary-'||
            substring(request.request_id FROM char_length('product_request_')+1)
          AND run.state IN ('canceled','succeeded')
          AND attempt.state IN ('canceled','succeeded')
      ))::integer
  FROM agent_product_canary_requests request
  LEFT JOIN agent_product_canary_receipts receipt ON receipt.request_id=request.request_id
  WHERE request.activation_id=p_activation_id
$function$;

CREATE FUNCTION agent_product_canary_worker_claim_requests(
  p_activation_id TEXT,p_claim_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER
) RETURNS SETOF agent_product_canary_requests
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_claim_owner) NOT BETWEEN 1 AND 128
     OR p_lease_seconds NOT BETWEEN 10 AND 300 OR p_limit NOT BETWEEN 1 AND 20 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_CLAIM_INVALID';
  END IF;
  RETURN QUERY WITH candidates AS(
    SELECT request.request_id FROM agent_product_canary_activations activation
    JOIN agent_product_canary_requests request ON request.activation_id=activation.activation_id
    WHERE activation.activation_id=p_activation_id AND activation.enabled
      AND p_now>=activation.valid_from AND p_now<activation.valid_until
      AND request.state='queued'
    ORDER BY request.created_at,request.request_id
    FOR UPDATE OF request SKIP LOCKED LIMIT p_limit
  ),claimed AS(
    UPDATE agent_product_canary_requests request SET state='claimed',
      claim_generation=request.claim_generation+1,claim_owner=p_claim_owner,
      claim_expires_at=p_now+make_interval(secs=>p_lease_seconds),error_code=NULL,
      updated_at=GREATEST(request.updated_at,p_now)
    FROM candidates WHERE request.request_id=candidates.request_id RETURNING request.*
  ) SELECT claimed.* FROM claimed ORDER BY claimed.created_at,claimed.request_id;
END
$function$;

CREATE FUNCTION agent_product_canary_worker_complete_request(
  p_activation_id TEXT,p_request_id TEXT,p_claim_owner TEXT,p_claim_generation BIGINT,
  p_run_id TEXT,p_attempt_id TEXT,p_run_snapshot_fingerprint TEXT,
  p_plan_fingerprint TEXT,p_receipt_fingerprint TEXT,p_outcome TEXT
) RETURNS agent_product_canary_receipts
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE request agent_product_canary_requests%ROWTYPE;receipt agent_product_canary_receipts%ROWTYPE;
  run_state TEXT;attempt_state TEXT;expected_receipt TEXT;
BEGIN
  SELECT * INTO receipt FROM agent_product_canary_receipts WHERE request_id=p_request_id;
  IF FOUND THEN
    IF receipt.activation_id<>p_activation_id OR receipt.run_id<>p_run_id
       OR receipt.attempt_id<>p_attempt_id OR receipt.run_snapshot_fingerprint<>p_run_snapshot_fingerprint
       OR receipt.plan_fingerprint<>p_plan_fingerprint OR receipt.receipt_fingerprint<>p_receipt_fingerprint
       OR receipt.outcome<>p_outcome THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN receipt;
  END IF;
  SELECT * INTO request FROM agent_product_canary_requests item
    WHERE item.request_id=p_request_id AND item.activation_id=p_activation_id FOR UPDATE;
  IF NOT FOUND OR request.state<>'claimed' OR request.claim_owner<>p_claim_owner
     OR request.claim_generation<>p_claim_generation OR request.plan_fingerprint<>p_plan_fingerprint
     OR request.claim_expires_at<=clock_timestamp()
     OR p_run_snapshot_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_receipt_fingerprint!~'^sha256:[a-f0-9]{64}$' OR p_outcome<>'bounded_canary_passed' THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='GENERATION_STALE';
  END IF;
  expected_receipt:='sha256:'||encode(sha256(
    convert_to('neo.agent-product-canary-receipt/v1','UTF8')||decode('00','hex')||
    convert_to(p_activation_id,'UTF8')||decode('00','hex')||convert_to(p_request_id,'UTF8')||decode('00','hex')||
    convert_to(p_run_id,'UTF8')||decode('00','hex')||convert_to(p_attempt_id,'UTF8')||decode('00','hex')||
    convert_to(p_run_snapshot_fingerprint,'UTF8')||decode('00','hex')||convert_to(p_plan_fingerprint,'UTF8')),'hex');
  IF p_receipt_fingerprint<>expected_receipt THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PRODUCT_CANARY_RECEIPT_BINDING_INVALID';
  END IF;
  SELECT run.state,attempt.state INTO run_state,attempt_state FROM agent_runs run
    JOIN agent_attempts attempt ON attempt.run_id=run.id AND attempt.user_id=run.user_id
    JOIN agent_steps step ON step.id=attempt.step_id AND step.run_id=run.id AND step.user_id=run.user_id
    WHERE run.id=p_run_id AND run.user_id=request.user_id
      AND run.snapshot_fingerprint=p_run_snapshot_fingerprint
      AND run.idempotency_key='g21.6-product-canary-'||
        substring(p_request_id FROM char_length('product_request_')+1)
      AND attempt.id=p_attempt_id AND step.ordinal=0 AND step.kind='product_canary';
  IF NOT FOUND OR run_state NOT IN ('canceled','succeeded') OR attempt_state NOT IN ('canceled','succeeded') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PRODUCT_CANARY_RUN_NOT_TERMINAL';
  END IF;
  INSERT INTO agent_product_canary_receipts(request_id,activation_id,user_id,run_id,attempt_id,
    run_snapshot_fingerprint,plan_fingerprint,receipt_fingerprint,outcome)
  VALUES(p_request_id,p_activation_id,request.user_id,p_run_id,p_attempt_id,
    p_run_snapshot_fingerprint,p_plan_fingerprint,p_receipt_fingerprint,p_outcome)
  RETURNING * INTO receipt;
  UPDATE agent_product_canary_requests SET state='completed',updated_at=clock_timestamp(),
    terminal_at=clock_timestamp(),error_code=NULL WHERE request_id=p_request_id;
  RETURN receipt;
END
$function$;

CREATE FUNCTION agent_product_canary_worker_release_request(
  p_activation_id TEXT,p_request_id TEXT,p_claim_owner TEXT,p_claim_generation BIGINT,
  p_error_code TEXT,p_retry BOOLEAN
) RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE request agent_product_canary_requests%ROWTYPE;terminal BOOLEAN;
BEGIN
  IF p_error_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_RELEASE_INVALID';
  END IF;
  SELECT * INTO request FROM agent_product_canary_requests item
    WHERE item.request_id=p_request_id AND item.activation_id=p_activation_id FOR UPDATE;
  IF NOT FOUND OR request.state<>'claimed' OR request.claim_owner<>p_claim_owner
     OR request.claim_generation<>p_claim_generation
     OR request.claim_expires_at<=clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='GENERATION_STALE';
  END IF;
  terminal:=NOT p_retry OR request.failure_count>=2;
  UPDATE agent_product_canary_requests SET state=CASE WHEN terminal THEN 'failed' ELSE 'queued' END,
    claim_owner=CASE WHEN terminal THEN claim_owner ELSE NULL END,
    claim_expires_at=CASE WHEN terminal THEN claim_expires_at ELSE NULL END,
    failure_count=failure_count+1,error_code=p_error_code,updated_at=clock_timestamp(),
    terminal_at=CASE WHEN terminal THEN clock_timestamp() ELSE NULL END
  WHERE request_id=p_request_id;
  RETURN terminal;
END
$function$;

CREATE FUNCTION agent_product_canary_worker_reconcile(
  p_activation_id TEXT,p_now TIMESTAMPTZ,p_limit INTEGER
) RETURNS INTEGER LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE changed INTEGER;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_RECONCILE_INVALID';
  END IF;
  WITH candidates AS(
    SELECT request.request_id FROM agent_product_canary_requests request
    WHERE request.activation_id=p_activation_id AND request.state='claimed'
      AND request.claim_expires_at<=p_now
    ORDER BY request.claim_expires_at,request.request_id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) UPDATE agent_product_canary_requests request
    SET state=CASE WHEN request.failure_count>=2 THEN 'failed' ELSE 'queued' END,
      claim_owner=CASE WHEN request.failure_count>=2 THEN request.claim_owner ELSE NULL END,
      claim_expires_at=CASE WHEN request.failure_count>=2 THEN request.claim_expires_at ELSE NULL END,
      error_code='CLAIM_EXPIRED',failure_count=request.failure_count+1,
      updated_at=GREATEST(request.updated_at,p_now),
      terminal_at=CASE WHEN request.failure_count>=2 THEN p_now ELSE NULL END
    FROM candidates WHERE request.request_id=candidates.request_id;
  GET DIAGNOSTICS changed=ROW_COUNT;
  RETURN changed;
END
$function$;

CREATE FUNCTION agent_product_canary_record_promotion(
  p_promotion_id TEXT,p_activation_id TEXT,p_request_id TEXT,p_release_commit TEXT,
  p_closure_fingerprint TEXT,p_receipt_fingerprint TEXT,p_decision TEXT,
  p_actor_id TEXT,p_reason_code TEXT
) RETURNS agent_product_canary_promotions
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE promotion agent_product_canary_promotions%ROWTYPE;activation agent_product_canary_activations%ROWTYPE;
  receipt agent_product_canary_receipts%ROWTYPE;
BEGIN
  IF p_promotion_id!~'^promotion_[a-z0-9]{16,64}$' OR p_release_commit!~'^[a-f0-9]{40}$'
     OR p_closure_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR p_receipt_fingerprint!~'^sha256:[a-f0-9]{64}$' OR p_decision<>'PROMOTION_READY'
     OR octet_length(p_actor_id) NOT BETWEEN 1 AND 128 OR p_reason_code!~'^[A-Z][A-Z0-9_]{0,63}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_PRODUCT_CANARY_PROMOTION_INVALID';
  END IF;
  SELECT * INTO promotion FROM agent_product_canary_promotions WHERE promotion_id=p_promotion_id;
  IF FOUND THEN
    IF promotion.activation_id<>p_activation_id OR promotion.request_id<>p_request_id
       OR promotion.release_commit<>p_release_commit OR promotion.closure_fingerprint<>p_closure_fingerprint
       OR promotion.receipt_fingerprint<>p_receipt_fingerprint OR promotion.actor_id<>p_actor_id
       OR promotion.reason_code<>p_reason_code THEN
      RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REPLAY_DETECTED';
    END IF;
    RETURN promotion;
  END IF;
  SELECT * INTO activation FROM agent_product_canary_activations WHERE activation_id=p_activation_id FOR SHARE;
  SELECT * INTO receipt FROM agent_product_canary_receipts
    WHERE request_id=p_request_id AND activation_id=p_activation_id FOR SHARE;
  IF NOT FOUND OR activation.release_commit<>p_release_commit
     OR receipt.receipt_fingerprint<>p_receipt_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='AGENT_PRODUCT_CANARY_PROMOTION_DRIFT';
  END IF;
  INSERT INTO agent_product_canary_promotions(promotion_id,activation_id,request_id,
    release_commit,closure_fingerprint,receipt_fingerprint,decision,actor_type,actor_id,reason_code)
  VALUES(p_promotion_id,p_activation_id,p_request_id,p_release_commit,p_closure_fingerprint,
    p_receipt_fingerprint,p_decision,'operator',p_actor_id,p_reason_code)
  RETURNING * INTO promotion;
  RETURN promotion;
END
$function$;

DO $harden$
DECLARE schema_name TEXT:=current_schema();identity TEXT;
BEGIN
  FOREACH identity IN ARRAY ARRAY[
    'agent_product_canary_provision_activation(text,text,bigint,uuid,text,text,text,text,text,text,text,text,text,text,integer,timestamp with time zone,timestamp with time zone,text,text)',
    'agent_product_canary_disable_activation(text,text,text)',
    'agent_product_canary_status(uuid)',
    'agent_product_canary_enqueue(text,uuid,bigint,bigint,text)',
    'agent_product_canary_worker_get_activation(text)',
    'agent_product_canary_worker_health(text,timestamp with time zone)',
    'agent_product_canary_worker_claim_requests(text,text,timestamp with time zone,integer,integer)',
    'agent_product_canary_worker_complete_request(text,text,text,bigint,text,text,text,text,text,text)',
    'agent_product_canary_worker_release_request(text,text,text,bigint,text,boolean)',
    'agent_product_canary_worker_reconcile(text,timestamp with time zone,integer)',
    'agent_product_canary_record_promotion(text,text,text,text,text,text,text,text,text)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',schema_name,identity,schema_name);
  END LOOP;
END
$harden$;

ALTER TABLE agent_product_canary_activations OWNER TO agent_product_owner;
ALTER TABLE agent_product_canary_requests OWNER TO agent_product_owner;
ALTER TABLE agent_product_canary_receipts OWNER TO agent_product_owner;
ALTER TABLE agent_product_canary_promotions OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_activation_guard() OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_request_guard() OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_provision_activation(TEXT,TEXT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_disable_activation(TEXT,TEXT,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_status(UUID) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_enqueue(TEXT,UUID,BIGINT,BIGINT,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_worker_get_activation(TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_worker_health(TEXT,TIMESTAMPTZ) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_worker_claim_requests(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_worker_complete_request(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_worker_release_request(TEXT,TEXT,TEXT,BIGINT,TEXT,BOOLEAN) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER) OWNER TO agent_product_owner;
ALTER FUNCTION agent_product_canary_record_promotion(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_product_owner;

REVOKE ALL ON agent_product_canary_activations,agent_product_canary_requests,
  agent_product_canary_receipts,agent_product_canary_promotions
  FROM PUBLIC,go_api_runtime,agent_product_canary_worker,agent_orchestrator_runtime,
  agent_runner_control,agent_effect_control,agent_delegation_control,agent_cron_control,agent_learning_control;
REVOKE ALL ON FUNCTION
  agent_product_canary_provision_activation(TEXT,TEXT,BIGINT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,INTEGER,TIMESTAMPTZ,TIMESTAMPTZ,TEXT,TEXT),
  agent_product_canary_disable_activation(TEXT,TEXT,TEXT),
  agent_product_canary_status(UUID),
  agent_product_canary_enqueue(TEXT,UUID,BIGINT,BIGINT,TEXT),
  agent_product_canary_worker_get_activation(TEXT),
  agent_product_canary_worker_health(TEXT,TIMESTAMPTZ),
  agent_product_canary_worker_claim_requests(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_product_canary_worker_complete_request(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_product_canary_worker_release_request(TEXT,TEXT,TEXT,BIGINT,TEXT,BOOLEAN),
  agent_product_canary_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER),
  agent_product_canary_record_promotion(TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_product_canary_worker;
GRANT EXECUTE ON FUNCTION agent_product_canary_status(UUID),
  agent_product_canary_enqueue(TEXT,UUID,BIGINT,BIGINT,TEXT) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION agent_product_canary_worker_get_activation(TEXT),
  agent_product_canary_worker_health(TEXT,TIMESTAMPTZ),
  agent_product_canary_worker_claim_requests(TEXT,TEXT,TIMESTAMPTZ,INTEGER,INTEGER),
  agent_product_canary_worker_complete_request(TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_product_canary_worker_release_request(TEXT,TEXT,TEXT,BIGINT,TEXT,BOOLEAN),
  agent_product_canary_worker_reconcile(TEXT,TIMESTAMPTZ,INTEGER)
  TO agent_product_canary_worker;

DO $schema_acl$
BEGIN
  EXECUTE format('REVOKE ALL ON SCHEMA %I FROM agent_product_canary_worker',current_schema());
END
$schema_acl$;
