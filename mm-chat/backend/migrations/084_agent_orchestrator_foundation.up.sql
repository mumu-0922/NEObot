-- G20.2 durable Agent Orchestrator foundation. PostgreSQL is the only Run,
-- Step, Attempt, event, lease and Kill Switch authority. This migration adds
-- no HTTP route, Runner, Sandbox, Tool grant or side-effect executor.

DO $roles$
DECLARE
  role_name TEXT;
  can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create
  FROM pg_roles WHERE rolname = current_user;
  FOREACH role_name IN ARRAY ARRAY[
    'agent_orchestrator_owner',
    'agent_orchestrator_runtime'
  ] LOOP
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
      IF NOT can_create THEN
        RAISE EXCEPTION USING ERRCODE = '42501',
          MESSAGE = 'AGENT_ORCHESTRATOR_REQUIRED_ROLE_MISSING';
      END IF;
      EXECUTE format(
        'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',
        role_name
      );
    END IF;
    IF EXISTS (
      SELECT 1 FROM pg_roles WHERE rolname = role_name
        AND (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole
             OR rolreplication OR rolbypassrls)
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '42501',
        MESSAGE = 'AGENT_ORCHESTRATOR_ROLE_MUST_BE_RESTRICTED';
    END IF;
  END LOOP;
  IF pg_has_role('agent_orchestrator_runtime', 'agent_orchestrator_owner', 'MEMBER')
     OR pg_has_role('go_api_runtime', 'agent_orchestrator_owner', 'MEMBER')
     OR pg_has_role('go_api_runtime', 'agent_orchestrator_runtime', 'MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE = '42501',
      MESSAGE = 'AGENT_ORCHESTRATOR_FORBIDDEN_ROLE_MEMBERSHIP';
  END IF;
END
$roles$;

CREATE FUNCTION agent_orchestrator_snapshot_sanitized(p_snapshot JSONB)
RETURNS BOOLEAN
LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE
  v_item JSONB;
  v_entry RECORD;
  v_key TEXT;
BEGIN
  IF jsonb_typeof(p_snapshot) <> 'object' THEN RETURN false; END IF;
  FOR v_item IN SELECT value FROM jsonb_path_query(p_snapshot,'$.**') value LOOP
    IF jsonb_typeof(v_item) <> 'object' THEN CONTINUE; END IF;
    FOR v_entry IN SELECT key FROM jsonb_each(v_item) LOOP
      v_key := lower(regexp_replace(v_entry.key,'[_-]','','g'));
      IF v_key IN (
        'prompt','systemprompt','skillbody','workspacecontent','toolarguments',
        'toolresults','stdout','stderr','secretvalue','leasetoken',
        'authorization','apikey','password','accesstoken','refreshtoken',
        'credential'
      ) OR v_key LIKE '%password%' OR v_key LIKE '%authorization%' THEN
        RETURN false;
      END IF;
    END LOOP;
  END LOOP;
  RETURN true;
END
$function$;

CREATE TABLE agent_run_snapshots (
  id TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  fingerprint TEXT NOT NULL,
  canonical_snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  CONSTRAINT agent_snapshot_id_check
    CHECK (id ~ '^snapshot_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_snapshot_fingerprint_check
    CHECK (fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT agent_snapshot_canonical_object
    CHECK (agent_orchestrator_snapshot_sanitized(canonical_snapshot)
      AND octet_length(canonical_snapshot::TEXT) <= 65536),
  CONSTRAINT agent_snapshot_owner_fingerprint_unique
    UNIQUE (id, user_id, fingerprint)
);

CREATE TABLE agent_runs (
  id TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL,
  snapshot_fingerprint TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending',
  next_sequence BIGINT NOT NULL DEFAULT 1,
  scope_keys TEXT[] NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  terminal_at TIMESTAMPTZ,
  CONSTRAINT agent_run_id_check CHECK (id ~ '^run_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_run_snapshot_fk FOREIGN KEY (
    snapshot_id, user_id, snapshot_fingerprint
  ) REFERENCES agent_run_snapshots(id, user_id, fingerprint) ON DELETE CASCADE,
  CONSTRAINT agent_run_snapshot_unique UNIQUE (snapshot_id),
  CONSTRAINT agent_run_owner_idempotency_unique UNIQUE (user_id, idempotency_key),
  CONSTRAINT agent_run_owner_unique UNIQUE (id, user_id),
  CONSTRAINT agent_run_request_fingerprint_check
    CHECK (request_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
  CONSTRAINT agent_run_idempotency_bounded CHECK (
    octet_length(idempotency_key) BETWEEN 1 AND 256
    AND idempotency_key = trim(idempotency_key)
  ),
  CONSTRAINT agent_run_state_check CHECK (state IN (
    'pending', 'admitted', 'queued', 'running', 'succeeded', 'failed',
    'canceled', 'killed', 'outcome_unknown'
  )),
  CONSTRAINT agent_run_sequence_positive CHECK (next_sequence >= 1),
  CONSTRAINT agent_run_scope_keys_check CHECK (
    cardinality(scope_keys) BETWEEN 4 AND 32
    AND array_position(scope_keys, 'global:*') IS NOT NULL
    AND array_position(scope_keys, 'scheduler:*') IS NOT NULL
    AND array_position(scope_keys, 'user:' || user_id::TEXT) IS NOT NULL
    AND array_position(scope_keys, 'run:' || id) IS NOT NULL
  ),
  CONSTRAINT agent_run_state_shape CHECK (
    (state IN ('succeeded','failed','canceled','killed','outcome_unknown')
      AND terminal_at IS NOT NULL)
    OR (state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown')
      AND terminal_at IS NULL)
  ),
  CONSTRAINT agent_run_timestamps_order CHECK (
    updated_at >= created_at AND (terminal_at IS NULL OR terminal_at >= created_at)
  )
);

CREATE INDEX idx_agent_runs_recovery
  ON agent_runs(state, updated_at, id)
  WHERE state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown');
CREATE INDEX idx_agent_runs_retention
  ON agent_runs(terminal_at, id)
  WHERE terminal_at IS NOT NULL;

CREATE TABLE agent_steps (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  ordinal INTEGER NOT NULL,
  kind TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'pending',
  current_generation BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  terminal_at TIMESTAMPTZ,
  CONSTRAINT agent_step_id_check CHECK (id ~ '^step_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_step_run_owner_fk FOREIGN KEY (run_id, user_id)
    REFERENCES agent_runs(id, user_id) ON DELETE CASCADE,
  CONSTRAINT agent_step_run_ordinal_unique UNIQUE (run_id, ordinal),
  CONSTRAINT agent_step_run_id_unique UNIQUE (run_id, id),
  CONSTRAINT agent_step_run_owner_id_unique UNIQUE (run_id, user_id, id),
  CONSTRAINT agent_step_ordinal_nonnegative CHECK (ordinal >= 0),
  CONSTRAINT agent_step_kind_bounded CHECK (
    octet_length(kind) BETWEEN 1 AND 64 AND kind = trim(kind)
  ),
  CONSTRAINT agent_step_state_check CHECK (state IN (
    'pending', 'ready', 'running', 'succeeded', 'failed', 'skipped',
    'canceled', 'killed', 'outcome_unknown'
  )),
  CONSTRAINT agent_step_generation_nonnegative CHECK (current_generation >= 0),
  CONSTRAINT agent_step_state_shape CHECK (
    (state IN ('succeeded','failed','skipped','canceled','killed','outcome_unknown')
      AND terminal_at IS NOT NULL)
    OR (state NOT IN ('succeeded','failed','skipped','canceled','killed','outcome_unknown')
      AND terminal_at IS NULL)
  ),
  CONSTRAINT agent_step_timestamps_order CHECK (
    updated_at >= created_at AND (terminal_at IS NULL OR terminal_at >= created_at)
  )
);

CREATE TABLE agent_attempts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  step_id TEXT NOT NULL,
  generation BIGINT NOT NULL,
  state TEXT NOT NULL DEFAULT 'leased',
  lease_owner TEXT NOT NULL,
  lease_token_hash TEXT NOT NULL,
  lease_expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  terminal_at TIMESTAMPTZ,
  CONSTRAINT agent_attempt_id_check CHECK (id ~ '^attempt_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_attempt_step_owner_fk FOREIGN KEY (run_id, user_id, step_id)
    REFERENCES agent_steps(run_id, user_id, id) ON DELETE CASCADE,
  CONSTRAINT agent_attempt_run_id_unique UNIQUE (run_id, id),
  CONSTRAINT agent_attempt_run_owner_id_unique UNIQUE (run_id, user_id, id),
  CONSTRAINT agent_attempt_step_generation_unique UNIQUE (step_id, generation),
  CONSTRAINT agent_attempt_generation_positive CHECK (generation >= 1),
  CONSTRAINT agent_attempt_state_check CHECK (state IN (
    'leased', 'starting', 'running', 'prepared', 'committing', 'succeeded',
    'failed', 'canceled', 'killed', 'outcome_unknown', 'lease_expired'
  )),
  CONSTRAINT agent_attempt_lease_owner_bounded CHECK (
    octet_length(lease_owner) BETWEEN 1 AND 128 AND lease_owner = trim(lease_owner)
  ),
  CONSTRAINT agent_attempt_token_hash_check CHECK (lease_token_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT agent_attempt_state_shape CHECK (
    (state IN ('succeeded','failed','canceled','killed','outcome_unknown','lease_expired')
      AND terminal_at IS NOT NULL)
    OR (state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown','lease_expired')
      AND terminal_at IS NULL)
  ),
  CONSTRAINT agent_attempt_timestamps_order CHECK (
    updated_at >= created_at
    AND lease_expires_at >= created_at
    AND (terminal_at IS NULL OR terminal_at >= created_at)
  )
);

CREATE UNIQUE INDEX idx_agent_attempt_one_live_per_step
  ON agent_attempts(step_id)
  WHERE state IN ('leased','starting','running','prepared','committing');
CREATE INDEX idx_agent_attempt_expired
  ON agent_attempts(lease_expires_at, id)
  WHERE state IN ('leased','starting','running','prepared','committing');

CREATE FUNCTION agent_orchestrator_detail_sanitized(p_detail JSONB)
RETURNS BOOLEAN
LANGUAGE sql IMMUTABLE SET search_path FROM CURRENT AS $function$
  SELECT jsonb_typeof(p_detail) = 'object'
    AND octet_length(p_detail::TEXT) <= 4096
    AND (p_detail - ARRAY[
      'observationCode','classification','count','durationMs','byteCount',
      'fingerprint','outcome','conflictState'
    ]::TEXT[]) = '{}'::jsonb
    AND NOT EXISTS (
      SELECT 1 FROM jsonb_each(p_detail) entry
      WHERE jsonb_typeof(entry.value) NOT IN ('null','boolean','number','string')
        OR (jsonb_typeof(entry.value) = 'number' AND (entry.value #>> '{}')::NUMERIC < 0)
        OR (jsonb_typeof(entry.value) = 'string' AND entry.key = 'fingerprint'
          AND (entry.value #>> '{}') !~ '^sha256:[0-9a-f]{64}$')
        OR (jsonb_typeof(entry.value) = 'string' AND entry.key <> 'fingerprint'
          AND (entry.value #>> '{}') !~ '^[A-Za-z][A-Za-z0-9_.:-]{0,127}$')
    )
$function$;

CREATE TABLE agent_run_events (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  step_id TEXT,
  attempt_id TEXT,
  sequence BIGINT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  kind TEXT NOT NULL,
  entity TEXT NOT NULL,
  from_state TEXT,
  to_state TEXT NOT NULL,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  generation BIGINT,
  lease_expires_at TIMESTAMPTZ,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  CONSTRAINT agent_event_id_check CHECK (id ~ '^event_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_event_run_owner_fk FOREIGN KEY (run_id, user_id)
    REFERENCES agent_runs(id, user_id) ON DELETE CASCADE,
  CONSTRAINT agent_event_step_fk FOREIGN KEY (run_id, step_id)
    REFERENCES agent_steps(run_id, id) ON DELETE CASCADE,
  CONSTRAINT agent_event_attempt_fk FOREIGN KEY (run_id, attempt_id)
    REFERENCES agent_attempts(run_id, id) ON DELETE CASCADE,
  CONSTRAINT agent_event_run_sequence_unique UNIQUE (run_id, sequence),
  CONSTRAINT agent_event_sequence_positive CHECK (sequence >= 1),
  CONSTRAINT agent_event_kind_check CHECK (kind IN (
    'run.transition', 'step.transition', 'attempt.transition',
    'lease.acquired', 'lease.heartbeat', 'lease.expired'
  )),
  CONSTRAINT agent_event_entity_check CHECK (entity IN ('run','step','attempt','fact')),
  CONSTRAINT agent_event_actor_type_check CHECK (
    actor_type IN ('user','orchestrator','runner','scheduler','operator')
  ),
  CONSTRAINT agent_event_actor_id_bounded CHECK (
    octet_length(actor_id) BETWEEN 1 AND 128 AND actor_id = trim(actor_id)
  ),
  CONSTRAINT agent_event_reason_check CHECK (reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$'),
  CONSTRAINT agent_event_generation_positive CHECK (generation IS NULL OR generation >= 1),
  CONSTRAINT agent_event_detail_sanitized CHECK (
    agent_orchestrator_detail_sanitized(detail)
  ),
  CONSTRAINT agent_event_shape CHECK (
    (entity = 'run' AND step_id IS NULL AND attempt_id IS NULL)
    OR (entity = 'step' AND step_id IS NOT NULL AND attempt_id IS NULL)
    OR (entity = 'attempt' AND step_id IS NOT NULL AND attempt_id IS NOT NULL
      AND generation IS NOT NULL)
    OR entity = 'fact'
  ),
  CONSTRAINT agent_event_lease_shape CHECK (
    (kind LIKE 'lease.%' AND step_id IS NOT NULL AND attempt_id IS NOT NULL
      AND generation IS NOT NULL AND lease_expires_at IS NOT NULL)
    OR kind NOT LIKE 'lease.%'
  )
);

CREATE INDEX idx_agent_events_run_order ON agent_run_events(run_id, sequence);

CREATE TABLE agent_kill_switch_state (
  singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
  epoch BIGINT NOT NULL DEFAULT 0 CHECK (epoch >= 0)
);
INSERT INTO agent_kill_switch_state(singleton, epoch) VALUES (true, 0);

CREATE TABLE agent_kill_switches (
  switch_id TEXT NOT NULL,
  revision BIGINT NOT NULL,
  epoch BIGINT NOT NULL,
  scope_type TEXT NOT NULL,
  scope_value TEXT NOT NULL,
  scope_key TEXT NOT NULL,
  mode TEXT NOT NULL,
  active BOOLEAN NOT NULL,
  actor_type TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (switch_id, revision),
  UNIQUE (epoch),
  CONSTRAINT agent_kill_id_check CHECK (switch_id ~ '^switch_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_kill_revision_positive CHECK (revision >= 1 AND epoch >= 1),
  CONSTRAINT agent_kill_scope_type_check CHECK (scope_type IN (
    'global','scheduler','runner','source','admission','skill','tool',
    'capability','action','egress','secret','project','user','run'
  )),
  CONSTRAINT agent_kill_scope_value_bounded CHECK (
    octet_length(scope_value) BETWEEN 1 AND 512 AND scope_value = trim(scope_value)
  ),
  CONSTRAINT agent_kill_scope_key_exact CHECK (scope_key = scope_type || ':' || scope_value),
  CONSTRAINT agent_kill_global_exact CHECK (scope_type <> 'global' OR scope_value = '*'),
  CONSTRAINT agent_kill_mode_check CHECK (mode IN ('deny_new','cancel','kill')),
  CONSTRAINT agent_kill_actor_type_check CHECK (
    actor_type IN ('user','orchestrator','runner','scheduler','operator')
  ),
  CONSTRAINT agent_kill_actor_bounded CHECK (
    octet_length(actor_id) BETWEEN 1 AND 128 AND actor_id = trim(actor_id)
  ),
  CONSTRAINT agent_kill_reason_check CHECK (reason_code ~ '^[A-Z][A-Z0-9_]{0,63}$')
);

CREATE INDEX idx_agent_kill_scope_epoch
  ON agent_kill_switches(scope_key, epoch DESC);

CREATE FUNCTION agent_orchestrator_snapshot_immutable()
RETURNS trigger LANGUAGE plpgsql AS $function$
BEGIN
  RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'AGENT_SNAPSHOT_IMMUTABLE';
END
$function$;

CREATE TRIGGER trg_agent_snapshot_immutable
BEFORE UPDATE ON agent_run_snapshots
FOR EACH ROW EXECUTE FUNCTION agent_orchestrator_snapshot_immutable();

CREATE FUNCTION agent_orchestrator_event_immutable()
RETURNS trigger LANGUAGE plpgsql AS $function$
BEGIN
  -- Parent/account retention deletes enter through an owning row and may
  -- cascade the ledger. A direct event delete remains depth 1 and is denied.
  IF TG_OP = 'DELETE' AND pg_trigger_depth() > 1 THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION USING ERRCODE = '55000', MESSAGE = 'AGENT_EVENT_IMMUTABLE';
END
$function$;

CREATE TRIGGER trg_agent_event_immutable
BEFORE UPDATE OR DELETE ON agent_run_events
FOR EACH ROW EXECUTE FUNCTION agent_orchestrator_event_immutable();

CREATE FUNCTION agent_orchestrator_append_event(
  p_event_id TEXT, p_run_id TEXT, p_user_id UUID, p_step_id TEXT,
  p_attempt_id TEXT, p_kind TEXT, p_entity TEXT, p_from_state TEXT,
  p_to_state TEXT, p_actor_type TEXT, p_actor_id TEXT, p_reason_code TEXT,
  p_generation BIGINT, p_lease_expires_at TIMESTAMPTZ, p_detail JSONB,
  p_occurred_at TIMESTAMPTZ DEFAULT clock_timestamp()
) RETURNS BIGINT
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_sequence BIGINT;
BEGIN
  UPDATE agent_runs SET next_sequence = next_sequence + 1,
    updated_at = GREATEST(updated_at, p_occurred_at)
  WHERE id = p_run_id AND user_id = p_user_id
  RETURNING next_sequence - 1 INTO v_sequence;
  IF v_sequence IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'AGENT_RUN_NOT_FOUND';
  END IF;
  INSERT INTO agent_run_events(
    id, run_id, user_id, step_id, attempt_id, sequence, occurred_at,
    kind, entity, from_state, to_state, actor_type, actor_id, reason_code,
    generation, lease_expires_at, detail
  ) VALUES (
    p_event_id, p_run_id, p_user_id, p_step_id, p_attempt_id, v_sequence,
    p_occurred_at, p_kind, p_entity, p_from_state, p_to_state, p_actor_type,
    p_actor_id, p_reason_code, p_generation, p_lease_expires_at,
    COALESCE(p_detail, '{}'::jsonb)
  );
  RETURN v_sequence;
END
$function$;

CREATE FUNCTION agent_orchestrator_active_kill_mode(p_scope_keys TEXT[])
RETURNS TEXT
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  WITH latest AS (
    SELECT DISTINCT ON (switch_id) switch_id, scope_key, mode, active
    FROM agent_kill_switches ORDER BY switch_id, revision DESC
  )
  SELECT CASE max(CASE mode WHEN 'kill' THEN 3 WHEN 'cancel' THEN 2 ELSE 1 END)
    WHEN 3 THEN 'kill' WHEN 2 THEN 'cancel' WHEN 1 THEN 'deny_new' ELSE NULL END
  FROM latest WHERE active AND scope_key = ANY(p_scope_keys)
$function$;

CREATE FUNCTION agent_orchestrator_enqueue_run(
  p_run_id TEXT, p_snapshot_id TEXT, p_user_id UUID, p_idempotency_key TEXT,
  p_snapshot_fingerprint TEXT, p_request_fingerprint TEXT,
  p_canonical_snapshot JSONB, p_steps JSONB, p_scope_keys TEXT[],
  p_event_ids TEXT[]
) RETURNS TABLE(run_id TEXT, created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_existing agent_runs%ROWTYPE;
  v_step JSONB;
  v_index INTEGER := 0;
  v_event_index INTEGER := 1;
  v_now TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF jsonb_typeof(p_canonical_snapshot) <> 'object'
     OR jsonb_typeof(p_steps) <> 'array'
     OR jsonb_array_length(p_steps) NOT BETWEEN 1 AND 256
     OR cardinality(p_event_ids) <> 3 + (2 * jsonb_array_length(p_steps))
     OR agent_orchestrator_active_kill_mode(p_scope_keys) IS NOT NULL THEN
    IF agent_orchestrator_active_kill_mode(p_scope_keys) IS NOT NULL THEN
      RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'KILL_SWITCH_ACTIVE';
    END IF;
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'AGENT_ENQUEUE_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(hashtextextended(p_user_id::TEXT || E'\\x00' || p_idempotency_key, 0));
  SELECT * INTO v_existing FROM agent_runs
  WHERE user_id = p_user_id AND idempotency_key = p_idempotency_key FOR UPDATE;
  IF FOUND THEN
    IF v_existing.request_fingerprint <> p_request_fingerprint
       OR v_existing.snapshot_fingerprint <> p_snapshot_fingerprint
       OR (SELECT snapshot.canonical_snapshot FROM agent_run_snapshots snapshot
           WHERE snapshot.id=v_existing.snapshot_id) <> p_canonical_snapshot
       OR array_remove(v_existing.scope_keys,'run:'||v_existing.id)
          <> array_remove(p_scope_keys,'run:'||p_run_id)
       OR (SELECT jsonb_agg(step.kind ORDER BY step.ordinal)
           FROM agent_steps step WHERE step.run_id=v_existing.id)
          <> (SELECT jsonb_agg(item.value->>'kind' ORDER BY item.ordinality)
              FROM jsonb_array_elements(p_steps) WITH ORDINALITY item(value,ordinality)) THEN
      RAISE EXCEPTION USING ERRCODE = '23505', MESSAGE = 'IDEMPOTENCY_CONFLICT';
    END IF;
    RETURN QUERY SELECT v_existing.id, false;
    RETURN;
  END IF;

  INSERT INTO agent_run_snapshots(id,user_id,fingerprint,canonical_snapshot,created_at)
  VALUES (p_snapshot_id,p_user_id,p_snapshot_fingerprint,p_canonical_snapshot,v_now);
  INSERT INTO agent_runs(
    id,user_id,snapshot_id,snapshot_fingerprint,idempotency_key,
    request_fingerprint,state,next_sequence,scope_keys,created_at,updated_at
  ) VALUES (
    p_run_id,p_user_id,p_snapshot_id,p_snapshot_fingerprint,p_idempotency_key,
    p_request_fingerprint,'pending',1,p_scope_keys,v_now,v_now
  );
  PERFORM agent_orchestrator_append_event(
    p_event_ids[v_event_index],p_run_id,p_user_id,NULL,NULL,'run.transition',
    'run',NULL,'pending','orchestrator','enqueue','RUN_CREATED',NULL,NULL,'{}',v_now
  );
  v_event_index := v_event_index + 1;

  FOR v_step IN SELECT value FROM jsonb_array_elements(p_steps) LOOP
    IF (v_step - ARRAY['stepId','kind']::TEXT[]) <> '{}'::jsonb
       OR NOT (v_step ? 'stepId' AND v_step ? 'kind') THEN
      RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'AGENT_STEP_PLAN_INVALID';
    END IF;
    INSERT INTO agent_steps(id,run_id,user_id,ordinal,kind,state,created_at,updated_at)
    VALUES (v_step->>'stepId',p_run_id,p_user_id,v_index,v_step->>'kind','pending',v_now,v_now);
    PERFORM agent_orchestrator_append_event(
      p_event_ids[v_event_index],p_run_id,p_user_id,v_step->>'stepId',NULL,
      'step.transition','step',NULL,'pending','orchestrator','enqueue',
      'STEP_CREATED',NULL,NULL,'{}',v_now
    );
    v_event_index := v_event_index + 1;
    v_index := v_index + 1;
  END LOOP;

  UPDATE agent_runs SET state='admitted',updated_at=v_now WHERE id=p_run_id;
  PERFORM agent_orchestrator_append_event(
    p_event_ids[v_event_index],p_run_id,p_user_id,NULL,NULL,'run.transition',
    'run','pending','admitted','orchestrator','enqueue','RUN_ADMITTED',NULL,NULL,'{}',v_now
  );
  v_event_index := v_event_index + 1;
  UPDATE agent_runs SET state='queued',updated_at=v_now WHERE id=p_run_id;
  PERFORM agent_orchestrator_append_event(
    p_event_ids[v_event_index],p_run_id,p_user_id,NULL,NULL,'run.transition',
    'run','admitted','queued','orchestrator','enqueue','RUN_QUEUED',NULL,NULL,'{}',v_now
  );
  v_event_index := v_event_index + 1;

  FOR v_step IN SELECT value FROM jsonb_array_elements(p_steps) LOOP
    UPDATE agent_steps SET state='ready',updated_at=v_now WHERE id=v_step->>'stepId';
    PERFORM agent_orchestrator_append_event(
      p_event_ids[v_event_index],p_run_id,p_user_id,v_step->>'stepId',NULL,
      'step.transition','step','pending','ready','orchestrator','enqueue',
      'STEP_READY',NULL,NULL,'{}',v_now
    );
    v_event_index := v_event_index + 1;
  END LOOP;
  RETURN QUERY SELECT p_run_id, true;
END
$function$;

CREATE FUNCTION agent_orchestrator_transition_allowed(
  p_entity TEXT, p_from TEXT, p_to TEXT
) RETURNS BOOLEAN LANGUAGE sql IMMUTABLE AS $function$
  SELECT CASE p_entity
    WHEN 'run' THEN (p_from,p_to) IN (
      ('pending','admitted'),('admitted','queued'),('queued','running'),
      ('pending','canceled'),('pending','killed'),('pending','failed'),
      ('admitted','canceled'),('admitted','killed'),('admitted','failed'),
      ('queued','canceled'),('queued','killed'),('queued','failed'),
      ('running','succeeded'),('running','failed'),('running','canceled'),
      ('running','killed'),('running','outcome_unknown'))
    WHEN 'step' THEN (p_from,p_to) IN (
      ('pending','ready'),('ready','running'),
      ('pending','skipped'),('pending','canceled'),('pending','killed'),('pending','failed'),
      ('ready','skipped'),('ready','canceled'),('ready','killed'),('ready','failed'),
      ('running','succeeded'),('running','failed'),('running','canceled'),
      ('running','killed'),('running','outcome_unknown'))
    WHEN 'attempt' THEN (p_from,p_to) IN (
      ('leased','starting'),('starting','running'),('running','prepared'),
      ('prepared','committing'),
      ('leased','failed'),('leased','canceled'),('leased','killed'),('leased','lease_expired'),
      ('starting','failed'),('starting','canceled'),('starting','killed'),('starting','lease_expired'),
      ('running','failed'),('running','canceled'),('running','killed'),('running','lease_expired'),
      ('prepared','failed'),('prepared','canceled'),('prepared','killed'),('prepared','lease_expired'),
      ('committing','succeeded'),('committing','failed'),('committing','killed'),
      ('committing','outcome_unknown'))
    ELSE false END
$function$;

CREATE FUNCTION agent_orchestrator_transition(
  p_event_id TEXT, p_user_id UUID, p_run_id TEXT, p_step_id TEXT,
  p_attempt_id TEXT, p_generation BIGINT, p_lease_owner TEXT,
  p_lease_token_hash TEXT, p_entity TEXT, p_expected TEXT, p_to TEXT,
  p_actor_type TEXT, p_actor_id TEXT, p_reason_code TEXT, p_detail JSONB
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_run agent_runs%ROWTYPE;
  v_step agent_steps%ROWTYPE;
  v_attempt agent_attempts%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_terminal BOOLEAN;
  v_mode TEXT;
  v_parent_kind TEXT;
BEGIN
  IF NOT agent_orchestrator_transition_allowed(p_entity,p_expected,p_to) THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
  END IF;
  SELECT run.* INTO v_run FROM agent_runs run
  WHERE run.id=p_run_id AND run.user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='AGENT_RUN_NOT_FOUND'; END IF;

  IF p_entity = 'run' THEN
    IF v_run.state <> p_expected THEN
      RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
    END IF;
    v_terminal := p_to IN ('succeeded','failed','canceled','killed','outcome_unknown');
    IF v_terminal AND EXISTS (
      SELECT 1 FROM agent_steps step WHERE step.run_id=p_run_id
        AND step.state NOT IN ('succeeded','failed','skipped','canceled','killed','outcome_unknown')
    ) THEN
      RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
    END IF;
    IF p_to='succeeded' AND EXISTS (
      SELECT 1 FROM agent_steps step WHERE step.run_id=p_run_id
        AND step.state NOT IN ('succeeded','skipped')
    ) THEN
      RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
    END IF;
    UPDATE agent_runs SET state=p_to,updated_at=v_now,
      terminal_at=CASE WHEN v_terminal THEN v_now ELSE NULL END WHERE id=p_run_id;
    PERFORM agent_orchestrator_append_event(
      p_event_id,p_run_id,p_user_id,NULL,NULL,'run.transition','run',p_expected,p_to,
      p_actor_type,p_actor_id,p_reason_code,NULL,NULL,p_detail,v_now);
    RETURN true;
  END IF;

  SELECT step.* INTO v_step FROM agent_steps step
  WHERE step.id=p_step_id AND step.run_id=p_run_id AND step.user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='AGENT_STEP_NOT_FOUND'; END IF;
  IF p_entity = 'step' THEN
    IF v_step.state <> p_expected THEN
      RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
    END IF;
    IF p_expected = 'running' THEN
  SELECT attempt.* INTO v_attempt FROM agent_attempts attempt
      WHERE attempt.id=p_attempt_id AND attempt.run_id=p_run_id
        AND attempt.user_id=p_user_id AND attempt.step_id=p_step_id FOR UPDATE;
      IF NOT FOUND OR v_attempt.generation<>p_generation
         OR v_step.current_generation<>p_generation
         OR v_attempt.lease_owner<>p_lease_owner
         OR v_attempt.lease_token_hash<>p_lease_token_hash
         OR v_attempt.state<>p_to THEN
        RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='LEASE_STALE';
      END IF;
      SELECT step.kind INTO v_parent_kind FROM agent_steps step
      WHERE step.run_id=p_run_id AND step.ordinal=v_step.ordinal-1;
      IF v_parent_kind IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM agent_steps parent
        WHERE parent.run_id=p_run_id AND parent.ordinal=v_step.ordinal-1
          AND parent.state IN ('succeeded','skipped')
      ) THEN
        RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
      END IF;
    END IF;
    v_terminal := p_to IN ('succeeded','failed','skipped','canceled','killed','outcome_unknown');
    UPDATE agent_steps SET state=p_to,updated_at=v_now,
      terminal_at=CASE WHEN v_terminal THEN v_now ELSE NULL END WHERE id=p_step_id;
    PERFORM agent_orchestrator_append_event(
      p_event_id,p_run_id,p_user_id,p_step_id,NULL,'step.transition','step',
      p_expected,p_to,p_actor_type,p_actor_id,p_reason_code,NULL,NULL,p_detail,v_now);
    RETURN true;
  END IF;

  SELECT * INTO v_attempt FROM agent_attempts
  WHERE id=p_attempt_id AND run_id=p_run_id AND user_id=p_user_id
    AND step_id=p_step_id FOR UPDATE;
  IF NOT FOUND OR v_attempt.generation <> p_generation
     OR v_step.current_generation <> p_generation
     OR v_attempt.lease_owner <> p_lease_owner
     OR v_attempt.lease_token_hash <> p_lease_token_hash
     OR v_attempt.lease_expires_at <= v_now THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='LEASE_STALE';
  END IF;
  IF v_run.state<>'running' OR v_step.state<>'running' THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
  END IF;
  IF v_attempt.state <> p_expected THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
  END IF;
  v_mode := agent_orchestrator_active_kill_mode(v_run.scope_keys);
  IF v_mode IN ('cancel','kill')
     AND p_to NOT IN ('failed','canceled','killed','outcome_unknown') THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  v_terminal := p_to IN ('succeeded','failed','canceled','killed','outcome_unknown','lease_expired');
  UPDATE agent_attempts SET state=p_to,updated_at=v_now,
    terminal_at=CASE WHEN v_terminal THEN v_now ELSE NULL END WHERE id=p_attempt_id;
  PERFORM agent_orchestrator_append_event(
    p_event_id,p_run_id,p_user_id,p_step_id,p_attempt_id,'attempt.transition',
    'attempt',p_expected,p_to,p_actor_type,p_actor_id,p_reason_code,p_generation,
    v_attempt.lease_expires_at,p_detail,v_now);
  RETURN true;
END
$function$;

CREATE FUNCTION agent_orchestrator_acquire_step(
  p_user_id UUID, p_run_id TEXT, p_step_id TEXT, p_attempt_id TEXT,
  p_lease_owner TEXT, p_lease_token_hash TEXT, p_event_ids TEXT[],
  p_ttl_seconds INTEGER, p_actor_type TEXT, p_actor_id TEXT, p_reason_code TEXT
) RETURNS TABLE(
  attempt_id TEXT, run_id TEXT, step_id TEXT, generation BIGINT, state TEXT,
  lease_owner TEXT, lease_expires_at TIMESTAMPTZ, created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_run agent_runs%ROWTYPE;
  v_step agent_steps%ROWTYPE;
  v_prior agent_attempts%ROWTYPE;
  v_generation BIGINT;
  v_expires TIMESTAMPTZ;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_index INTEGER := 1;
BEGIN
  IF p_ttl_seconds NOT BETWEEN 5 AND 3600 OR cardinality(p_event_ids) <> 4 THEN
    RAISE EXCEPTION USING ERRCODE='22023', MESSAGE='AGENT_LEASE_INVALID';
  END IF;
  SELECT run.* INTO v_run FROM agent_runs run
  WHERE run.id=p_run_id AND run.user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='AGENT_RUN_NOT_FOUND'; END IF;
  SELECT step.* INTO v_step FROM agent_steps step
  WHERE step.id=p_step_id AND step.run_id=p_run_id AND step.user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='AGENT_STEP_NOT_FOUND'; END IF;
  IF v_run.state NOT IN ('queued','running') OR v_step.state NOT IN ('ready','running') THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
  END IF;
  IF v_run.state='queued' AND EXISTS (
    SELECT 1 FROM agent_steps earlier WHERE earlier.run_id=p_run_id
      AND earlier.ordinal<v_step.ordinal
      AND earlier.state NOT IN ('succeeded','skipped')
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
  END IF;
  IF v_step.state='ready' AND v_step.ordinal>0 AND NOT EXISTS (
    SELECT 1 FROM agent_steps parent
    WHERE parent.run_id=p_run_id AND parent.ordinal=v_step.ordinal-1
      AND parent.state IN ('succeeded','skipped')
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
  END IF;
  IF agent_orchestrator_active_kill_mode(v_run.scope_keys) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  SELECT attempt.* INTO v_prior FROM agent_attempts attempt
  WHERE attempt.step_id=p_step_id AND attempt.generation=v_step.current_generation
  FOR UPDATE;
  IF FOUND THEN
    IF v_prior.state IN ('leased','starting','running','prepared','committing') THEN
      IF v_prior.lease_expires_at > v_now THEN
        RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='LEASE_STALE';
      END IF;
      UPDATE agent_attempts attempt SET state='lease_expired',terminal_at=v_now,updated_at=v_now
      WHERE attempt.id=v_prior.id;
      PERFORM agent_orchestrator_append_event(
        p_event_ids[v_index],p_run_id,p_user_id,p_step_id,v_prior.id,
        'lease.expired','attempt',v_prior.state,'lease_expired',p_actor_type,
        p_actor_id,'LEASE_EXPIRED',v_prior.generation,v_prior.lease_expires_at,'{}',v_now);
      v_index := v_index + 1;
    ELSIF v_prior.state <> 'lease_expired' THEN
      RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
    END IF;
  END IF;
  IF v_run.state='queued' THEN
    UPDATE agent_runs run SET state='running',updated_at=v_now WHERE run.id=p_run_id;
    PERFORM agent_orchestrator_append_event(
      p_event_ids[v_index],p_run_id,p_user_id,NULL,NULL,'run.transition','run',
      'queued','running',p_actor_type,p_actor_id,'RUN_STARTED',NULL,NULL,'{}',v_now);
    v_index := v_index + 1;
  END IF;
  IF v_step.state='ready' THEN
    UPDATE agent_steps step SET state='running',updated_at=v_now WHERE step.id=p_step_id;
    PERFORM agent_orchestrator_append_event(
      p_event_ids[v_index],p_run_id,p_user_id,p_step_id,NULL,'step.transition','step',
      'ready','running',p_actor_type,p_actor_id,'STEP_STARTED',NULL,NULL,'{}',v_now);
    v_index := v_index + 1;
  END IF;
  v_generation := v_step.current_generation + 1;
  v_expires := v_now + make_interval(secs => p_ttl_seconds);
  UPDATE agent_steps step SET current_generation=v_generation,updated_at=v_now WHERE step.id=p_step_id;
  INSERT INTO agent_attempts(
    id,run_id,user_id,step_id,generation,state,lease_owner,lease_token_hash,
    lease_expires_at,created_at,updated_at
  ) VALUES (
    p_attempt_id,p_run_id,p_user_id,p_step_id,v_generation,'leased',p_lease_owner,
    p_lease_token_hash,v_expires,v_now,v_now
  );
  PERFORM agent_orchestrator_append_event(
    p_event_ids[v_index],p_run_id,p_user_id,p_step_id,p_attempt_id,
    'lease.acquired','attempt',NULL,'leased',p_actor_type,p_actor_id,p_reason_code,
    v_generation,v_expires,'{}',v_now);
  RETURN QUERY SELECT p_attempt_id,p_run_id,p_step_id,v_generation,'leased'::TEXT,
    p_lease_owner,v_expires,v_now,v_now;
END
$function$;

CREATE FUNCTION agent_orchestrator_heartbeat_attempt(
  p_event_id TEXT, p_user_id UUID, p_run_id TEXT, p_step_id TEXT,
  p_attempt_id TEXT, p_generation BIGINT, p_lease_owner TEXT,
  p_lease_token_hash TEXT, p_ttl_seconds INTEGER, p_actor_type TEXT,
  p_actor_id TEXT, p_reason_code TEXT
) RETURNS TIMESTAMPTZ
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_run agent_runs%ROWTYPE;
  v_step agent_steps%ROWTYPE;
  v_attempt agent_attempts%ROWTYPE;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_expires TIMESTAMPTZ;
BEGIN
  IF p_ttl_seconds NOT BETWEEN 5 AND 3600 THEN
    RAISE EXCEPTION USING ERRCODE='22023', MESSAGE='AGENT_LEASE_INVALID';
  END IF;
  SELECT * INTO v_run FROM agent_runs WHERE id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_step FROM agent_steps
    WHERE id=p_step_id AND run_id=p_run_id AND user_id=p_user_id FOR UPDATE;
  SELECT * INTO v_attempt FROM agent_attempts
    WHERE id=p_attempt_id AND run_id=p_run_id AND user_id=p_user_id AND step_id=p_step_id FOR UPDATE;
  IF v_attempt.id IS NULL OR v_step.id IS NULL OR v_run.id IS NULL
     OR v_attempt.generation<>p_generation OR v_step.current_generation<>p_generation
     OR v_attempt.lease_owner<>p_lease_owner OR v_attempt.lease_token_hash<>p_lease_token_hash
     OR v_attempt.lease_expires_at<=v_now
     OR v_attempt.state NOT IN ('leased','starting','running','prepared','committing') THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='LEASE_STALE';
  END IF;
  IF agent_orchestrator_active_kill_mode(v_run.scope_keys) IN ('cancel','kill') THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='KILL_SWITCH_ACTIVE';
  END IF;
  v_expires := v_now + make_interval(secs => p_ttl_seconds);
  UPDATE agent_attempts SET lease_expires_at=v_expires,updated_at=v_now WHERE id=p_attempt_id;
  PERFORM agent_orchestrator_append_event(
    p_event_id,p_run_id,p_user_id,p_step_id,p_attempt_id,'lease.heartbeat','fact',
    v_attempt.state,v_attempt.state,p_actor_type,p_actor_id,p_reason_code,
    p_generation,v_expires,'{}',v_now);
  RETURN v_expires;
END
$function$;

CREATE FUNCTION agent_orchestrator_observe_terminal_conflict(
  p_event_id TEXT, p_user_id UUID, p_run_id TEXT, p_step_id TEXT,
  p_attempt_id TEXT, p_observed_state TEXT, p_actor_type TEXT,
  p_actor_id TEXT, p_reason_code TEXT, p_detail JSONB
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_current TEXT;
  v_generation BIGINT;
  v_expires TIMESTAMPTZ;
BEGIN
  IF p_attempt_id IS NOT NULL THEN
    SELECT state,generation,lease_expires_at INTO v_current,v_generation,v_expires
    FROM agent_attempts WHERE id=p_attempt_id AND run_id=p_run_id AND user_id=p_user_id;
  ELSIF p_step_id IS NOT NULL THEN
    SELECT state INTO v_current FROM agent_steps
    WHERE id=p_step_id AND run_id=p_run_id AND user_id=p_user_id;
  ELSE
    SELECT state INTO v_current FROM agent_runs WHERE id=p_run_id AND user_id=p_user_id;
  END IF;
  IF v_current NOT IN ('succeeded','failed','skipped','canceled','killed','outcome_unknown','lease_expired')
     OR p_observed_state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown') THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='INVALID_TRANSITION';
  END IF;
  PERFORM agent_orchestrator_append_event(
    p_event_id,p_run_id,p_user_id,p_step_id,p_attempt_id,'run.transition','fact',
    v_current,v_current,p_actor_type,p_actor_id,p_reason_code,v_generation,v_expires,
    COALESCE(p_detail,'{}') || jsonb_build_object('conflictState',p_observed_state));
  RETURN true;
END
$function$;

CREATE FUNCTION agent_orchestrator_recovery_runs(p_limit INTEGER)
RETURNS TABLE(
  run_id TEXT, user_id UUID, run_state TEXT, snapshot_id TEXT,
  attempt_id TEXT, step_id TEXT, generation BIGINT, attempt_state TEXT,
  lease_expires_at TIMESTAMPTZ
)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
  SELECT run.id,run.user_id,run.state,run.snapshot_id,
    attempt.id,attempt.step_id,attempt.generation,attempt.state,attempt.lease_expires_at
  FROM agent_runs run
  LEFT JOIN agent_steps step ON step.run_id=run.id
  LEFT JOIN agent_attempts attempt ON attempt.step_id=step.id
    AND attempt.generation=step.current_generation
  WHERE run.state NOT IN ('succeeded','failed','canceled','killed','outcome_unknown')
  ORDER BY run.updated_at,run.id,step.ordinal
  LIMIT p_limit
$function$;

CREATE FUNCTION agent_orchestrator_rebuild_projection(p_user_id UUID, p_run_id TEXT)
RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_run_state TEXT;
  v_run_terminal TIMESTAMPTZ;
BEGIN
  PERFORM 1 FROM agent_runs WHERE id=p_run_id AND user_id=p_user_id FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='AGENT_RUN_NOT_FOUND'; END IF;
  SELECT to_state,
    CASE WHEN to_state IN ('succeeded','failed','canceled','killed','outcome_unknown')
      THEN occurred_at ELSE NULL END
  INTO v_run_state,v_run_terminal
  FROM agent_run_events WHERE run_id=p_run_id AND entity='run'
  ORDER BY sequence DESC LIMIT 1;
  IF v_run_state IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='AGENT_EVENT_REBUILD_INCOMPLETE';
  END IF;
  UPDATE agent_runs SET state=v_run_state,terminal_at=v_run_terminal,
    next_sequence=(SELECT COALESCE(max(sequence),0)+1 FROM agent_run_events WHERE run_id=p_run_id),
    updated_at=(SELECT max(occurred_at) FROM agent_run_events WHERE run_id=p_run_id)
  WHERE id=p_run_id;

  UPDATE agent_steps step SET
    state=event.to_state,
    current_generation=COALESCE((
      SELECT max(generation) FROM agent_run_events
      WHERE run_id=p_run_id AND step_id=step.id AND generation IS NOT NULL
    ),0),
    terminal_at=CASE WHEN event.to_state IN (
      'succeeded','failed','skipped','canceled','killed','outcome_unknown'
    ) THEN event.occurred_at ELSE NULL END,
    updated_at=event.occurred_at
  FROM (
    SELECT DISTINCT ON (step_id) step_id,to_state,occurred_at
    FROM agent_run_events
    WHERE run_id=p_run_id AND entity='step'
    ORDER BY step_id,sequence DESC
  ) event
  WHERE step.run_id=p_run_id AND event.step_id=step.id;

  UPDATE agent_attempts attempt SET
    state=event.to_state,
    lease_expires_at=COALESCE((
      SELECT lease_expires_at FROM agent_run_events
      WHERE run_id=p_run_id AND attempt_id=attempt.id AND lease_expires_at IS NOT NULL
      ORDER BY sequence DESC LIMIT 1
    ),attempt.lease_expires_at),
    terminal_at=CASE WHEN event.to_state IN (
      'succeeded','failed','canceled','killed','outcome_unknown','lease_expired'
    ) THEN event.occurred_at ELSE NULL END,
    updated_at=event.occurred_at
  FROM (
    SELECT DISTINCT ON (attempt_id) attempt_id,to_state,occurred_at
    FROM agent_run_events
    WHERE run_id=p_run_id AND entity='attempt'
    ORDER BY attempt_id,sequence DESC
  ) event
  WHERE attempt.run_id=p_run_id AND event.attempt_id=attempt.id;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_orchestrator_append_kill_switch(
  p_switch_id TEXT, p_scope_type TEXT, p_scope_value TEXT, p_mode TEXT,
  p_active BOOLEAN, p_expected_revision BIGINT, p_actor_type TEXT,
  p_actor_id TEXT, p_reason_code TEXT
) RETURNS TABLE(
  switch_id TEXT, scope_type TEXT, scope_value TEXT, mode TEXT, active BOOLEAN,
  revision BIGINT, epoch BIGINT, actor_type TEXT, actor_id TEXT,
  reason_code TEXT, created_at TIMESTAMPTZ
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_revision BIGINT;
  v_epoch BIGINT;
  v_now TIMESTAMPTZ := clock_timestamp();
  v_prior_scope_type TEXT;
  v_prior_scope_value TEXT;
BEGIN
  PERFORM 1 FROM agent_kill_switch_state WHERE singleton FOR UPDATE;
  SELECT COALESCE(max(k.revision),0) INTO v_revision
  FROM agent_kill_switches k WHERE k.switch_id=p_switch_id;
  IF v_revision <> p_expected_revision THEN
    RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='REVISION_CONFLICT';
  END IF;
  IF v_revision > 0 THEN
    SELECT k.scope_type,k.scope_value INTO v_prior_scope_type,v_prior_scope_value
    FROM agent_kill_switches k
    WHERE k.switch_id=p_switch_id ORDER BY k.revision DESC LIMIT 1;
    IF v_prior_scope_type<>p_scope_type OR v_prior_scope_value<>p_scope_value THEN
      RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='REVISION_CONFLICT';
    END IF;
  END IF;
  UPDATE agent_kill_switch_state SET epoch=agent_kill_switch_state.epoch+1
  WHERE singleton RETURNING agent_kill_switch_state.epoch INTO v_epoch;
  v_revision := v_revision + 1;
  INSERT INTO agent_kill_switches(
    switch_id,revision,epoch,scope_type,scope_value,scope_key,mode,active,
    actor_type,actor_id,reason_code,created_at
  ) VALUES (
    p_switch_id,v_revision,v_epoch,p_scope_type,p_scope_value,
    p_scope_type||':'||p_scope_value,p_mode,p_active,p_actor_type,p_actor_id,
    p_reason_code,v_now
  );
  RETURN QUERY SELECT p_switch_id,p_scope_type,p_scope_value,p_mode,p_active,
    v_revision,v_epoch,p_actor_type,p_actor_id,p_reason_code,v_now;
END
$function$;

CREATE FUNCTION agent_orchestrator_resolve_kill_switch(p_user_id UUID, p_run_id TEXT)
RETURNS TABLE(active BOOLEAN, mode TEXT, epoch BIGINT, switch_ids TEXT[])
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_scopes TEXT[];
  v_epoch BIGINT;
BEGIN
  SELECT scope_keys INTO v_scopes FROM agent_runs
  WHERE id=p_run_id AND user_id=p_user_id;
  IF v_scopes IS NULL THEN RAISE EXCEPTION USING ERRCODE='P0001', MESSAGE='AGENT_RUN_NOT_FOUND'; END IF;
  SELECT state.epoch INTO v_epoch FROM agent_kill_switch_state state WHERE singleton;
  RETURN QUERY
  WITH latest AS (
    SELECT DISTINCT ON (entry.switch_id) entry.*
    FROM agent_kill_switches entry ORDER BY entry.switch_id,entry.revision DESC
  ), applicable AS (
    SELECT * FROM latest WHERE latest.active AND latest.scope_key=ANY(v_scopes)
  )
  SELECT count(*)>0,
    CASE max(CASE applicable.mode WHEN 'kill' THEN 3 WHEN 'cancel' THEN 2 ELSE 1 END)
      WHEN 3 THEN 'kill' WHEN 2 THEN 'cancel' WHEN 1 THEN 'deny_new' ELSE '' END,
    v_epoch,COALESCE(array_agg(applicable.switch_id ORDER BY applicable.switch_id)
      FILTER (WHERE applicable.switch_id IS NOT NULL),'{}'::TEXT[])
  FROM applicable;
END
$function$;

CREATE FUNCTION agent_orchestrator_prune_terminal_runs(
  p_cutoff TIMESTAMPTZ, p_limit INTEGER
) RETURNS INTEGER
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE
  v_deleted INTEGER;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 OR p_cutoff > clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='22023', MESSAGE='AGENT_RETENTION_INVALID';
  END IF;
  WITH targets AS (
    SELECT id,snapshot_id FROM agent_runs
    WHERE terminal_at IS NOT NULL AND terminal_at < p_cutoff
    ORDER BY terminal_at,id LIMIT p_limit FOR UPDATE SKIP LOCKED
  ), deleted_runs AS (
    DELETE FROM agent_runs run USING targets
    WHERE run.id=targets.id RETURNING run.snapshot_id
  ), deleted_snapshots AS (
    DELETE FROM agent_run_snapshots snapshot USING deleted_runs
    WHERE snapshot.id=deleted_runs.snapshot_id RETURNING snapshot.id
  ) SELECT count(*) INTO v_deleted FROM deleted_snapshots;
  RETURN v_deleted;
END
$function$;

DO $harden_functions$
DECLARE
  schema_name TEXT := current_schema();
  function_identity TEXT;
BEGIN
  FOREACH function_identity IN ARRAY ARRAY[
    'agent_orchestrator_append_event(text,text,uuid,text,text,text,text,text,text,text,text,text,bigint,timestamp with time zone,jsonb,timestamp with time zone)',
    'agent_orchestrator_active_kill_mode(text[])',
    'agent_orchestrator_enqueue_run(text,text,uuid,text,text,text,jsonb,jsonb,text[],text[])',
    'agent_orchestrator_transition_allowed(text,text,text)',
    'agent_orchestrator_transition(text,uuid,text,text,text,bigint,text,text,text,text,text,text,text,text,jsonb)',
    'agent_orchestrator_acquire_step(uuid,text,text,text,text,text,text[],integer,text,text,text)',
    'agent_orchestrator_heartbeat_attempt(text,uuid,text,text,text,bigint,text,text,integer,text,text,text)',
    'agent_orchestrator_observe_terminal_conflict(text,uuid,text,text,text,text,text,text,text,jsonb)',
    'agent_orchestrator_recovery_runs(integer)',
    'agent_orchestrator_rebuild_projection(uuid,text)',
    'agent_orchestrator_append_kill_switch(text,text,text,text,boolean,bigint,text,text,text)',
    'agent_orchestrator_resolve_kill_switch(uuid,text)',
    'agent_orchestrator_prune_terminal_runs(timestamp with time zone,integer)'
  ] LOOP
    EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',
      schema_name,function_identity,schema_name);
  END LOOP;
END
$harden_functions$;

ALTER TABLE agent_run_snapshots OWNER TO agent_orchestrator_owner;
ALTER TABLE agent_runs OWNER TO agent_orchestrator_owner;
ALTER TABLE agent_steps OWNER TO agent_orchestrator_owner;
ALTER TABLE agent_attempts OWNER TO agent_orchestrator_owner;
ALTER TABLE agent_run_events OWNER TO agent_orchestrator_owner;
ALTER TABLE agent_kill_switch_state OWNER TO agent_orchestrator_owner;
ALTER TABLE agent_kill_switches OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_snapshot_immutable() OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_event_immutable() OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_snapshot_sanitized(JSONB) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_detail_sanitized(JSONB) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_append_event(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,
  TIMESTAMPTZ,JSONB,TIMESTAMPTZ
) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_active_kill_mode(TEXT[]) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_enqueue_run(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]
) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_transition_allowed(TEXT,TEXT,TEXT) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_transition(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_acquire_step(
  UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT[],INTEGER,TEXT,TEXT,TEXT
) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_heartbeat_attempt(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,INTEGER,TEXT,TEXT,TEXT
) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_observe_terminal_conflict(
  TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_recovery_runs(INTEGER) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_rebuild_projection(UUID,TEXT) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_append_kill_switch(
  TEXT,TEXT,TEXT,TEXT,BOOLEAN,BIGINT,TEXT,TEXT,TEXT
) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_resolve_kill_switch(UUID,TEXT) OWNER TO agent_orchestrator_owner;
ALTER FUNCTION agent_orchestrator_prune_terminal_runs(TIMESTAMPTZ,INTEGER)
  OWNER TO agent_orchestrator_owner;

REVOKE ALL ON agent_run_snapshots,agent_runs,agent_steps,agent_attempts,
  agent_run_events,agent_kill_switch_state,agent_kill_switches
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime;
GRANT SELECT ON agent_run_snapshots,agent_runs,agent_steps,agent_attempts,
  agent_run_events,agent_kill_switch_state,agent_kill_switches
  TO agent_orchestrator_runtime;

REVOKE ALL ON FUNCTION agent_orchestrator_append_event(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,BIGINT,
  TIMESTAMPTZ,JSONB,TIMESTAMPTZ
) FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime;
REVOKE ALL ON FUNCTION agent_orchestrator_snapshot_immutable(),
  agent_orchestrator_event_immutable(),agent_orchestrator_snapshot_sanitized(JSONB),
  agent_orchestrator_detail_sanitized(JSONB)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime;
REVOKE ALL ON FUNCTION agent_orchestrator_active_kill_mode(TEXT[])
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime;
REVOKE ALL ON FUNCTION agent_orchestrator_transition_allowed(TEXT,TEXT,TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime;
REVOKE ALL ON FUNCTION agent_orchestrator_rebuild_projection(UUID,TEXT),
  agent_orchestrator_append_kill_switch(TEXT,TEXT,TEXT,TEXT,BOOLEAN,BIGINT,TEXT,TEXT,TEXT),
  agent_orchestrator_prune_terminal_runs(TIMESTAMPTZ,INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime;

REVOKE ALL ON FUNCTION agent_orchestrator_enqueue_run(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]
),agent_orchestrator_transition(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
),agent_orchestrator_acquire_step(
  UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT[],INTEGER,TEXT,TEXT,TEXT
),agent_orchestrator_heartbeat_attempt(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,INTEGER,TEXT,TEXT,TEXT
),agent_orchestrator_observe_terminal_conflict(
  TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
),agent_orchestrator_recovery_runs(INTEGER),
  agent_orchestrator_resolve_kill_switch(UUID,TEXT)
  FROM PUBLIC,go_api_runtime;

GRANT EXECUTE ON FUNCTION agent_orchestrator_enqueue_run(
  TEXT,TEXT,UUID,TEXT,TEXT,TEXT,JSONB,JSONB,TEXT[],TEXT[]
),agent_orchestrator_transition(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
),agent_orchestrator_acquire_step(
  UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT[],INTEGER,TEXT,TEXT,TEXT
),agent_orchestrator_heartbeat_attempt(
  TEXT,UUID,TEXT,TEXT,TEXT,BIGINT,TEXT,TEXT,INTEGER,TEXT,TEXT,TEXT
),agent_orchestrator_observe_terminal_conflict(
  TEXT,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
),agent_orchestrator_recovery_runs(INTEGER),
  agent_orchestrator_resolve_kill_switch(UUID,TEXT)
  TO agent_orchestrator_runtime;

DO $schema_privileges$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM agent_orchestrator_owner, agent_orchestrator_runtime, go_api_runtime',
    current_schema());
  EXECUTE format(
    'GRANT USAGE ON SCHEMA %I TO agent_orchestrator_owner, agent_orchestrator_runtime',
    current_schema());
END
$schema_privileges$;
