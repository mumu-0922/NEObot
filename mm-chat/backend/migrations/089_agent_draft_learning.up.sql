-- G20.7 held Draft-only learning foundation. Draft bytes remain quarantined;
-- automated checks cannot admit or install a Skill. Only the narrow control
-- role may invoke an authenticated human promotion transaction.

DO $roles$
DECLARE role_name TEXT;can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create FROM pg_roles WHERE rolname=current_user;
  FOREACH role_name IN ARRAY ARRAY['agent_learning_owner','agent_learning_control'] LOOP
    IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name) THEN
      IF NOT can_create THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_LEARNING_REQUIRED_ROLE_MISSING';END IF;
      EXECUTE format('CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',role_name);
    END IF;
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname=role_name AND
      (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)) THEN
      RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_LEARNING_ROLE_MUST_BE_RESTRICTED';END IF;
  END LOOP;
  IF pg_has_role('agent_learning_control','agent_learning_owner','MEMBER')
     OR pg_has_role('go_api_runtime','agent_learning_control','MEMBER')
     OR pg_has_role('agent_orchestrator_runtime','agent_learning_control','MEMBER')
     OR pg_has_role('agent_runner_control','agent_learning_control','MEMBER')
     OR pg_has_role('agent_effect_control','agent_learning_control','MEMBER')
     OR pg_has_role('agent_delegation_control','agent_learning_control','MEMBER')
     OR pg_has_role('agent_cron_control','agent_learning_control','MEMBER') THEN
    RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='AGENT_LEARNING_FORBIDDEN_ROLE_MEMBERSHIP';END IF;
END
$roles$;

ALTER TABLE skill_package_candidates DROP CONSTRAINT skill_candidate_source_type_check;
ALTER TABLE skill_package_candidates ADD CONSTRAINT skill_candidate_source_type_check
  CHECK(source_type IN ('official','lobehub','git','zip','learning'));

CREATE FUNCTION agent_learning_document_sanitized(p_value JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE item JSONB;entry RECORD;normalized TEXT;
BEGIN
  IF p_value IS NULL OR octet_length(p_value::text)>262144 THEN RETURN false;END IF;
  FOR item IN SELECT value FROM jsonb_path_query(p_value,'$.**') value LOOP
    IF jsonb_typeof(item)<>'object' THEN CONTINUE;END IF;
    FOR entry IN SELECT key FROM jsonb_each(item) LOOP
      normalized:=lower(regexp_replace(entry.key,'[_-]','','g'));
      IF normalized IN ('prompt','systemprompt','inputbody','runoutput','skillbody','workspacecontent','toolarguments','toolresults',
        'stdout','stderr','secretvalue','authorization','apikey','password','accesstoken','refreshtoken','credential')
        OR normalized LIKE '%password%' OR normalized LIKE '%authorization%' THEN RETURN false;END IF;
    END LOOP;
  END LOOP;
  RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_evidence_valid(p_value JSONB,p_base TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE item JSONB;path_value JSONB;has_base BOOLEAN:=false;has_event BOOLEAN:=false;
BEGIN
  IF jsonb_typeof(p_value)<>'array' OR jsonb_array_length(p_value) NOT BETWEEN 2 AND 32
     OR octet_length(p_value::text)>32768 THEN RETURN false;END IF;
  FOR item IN SELECT value FROM jsonb_array_elements(p_value) LOOP
    IF jsonb_typeof(item)<>'object' OR (item-ARRAY['kind','ref','fingerprint','paths'])<>'{}'::jsonb
       OR item->>'kind' NOT IN ('source_package','run_event')
       OR item->>'fingerprint'!~'^sha256:[a-f0-9]{64}$'
       OR jsonb_typeof(item->'paths')<>'array' OR jsonb_array_length(item->'paths') NOT BETWEEN 1 AND 256 THEN RETURN false;END IF;
    FOR path_value IN SELECT value FROM jsonb_array_elements(item->'paths') LOOP
      IF jsonb_typeof(path_value)<>'string' OR octet_length(path_value#>>'{}') NOT BETWEEN 1 AND 512
         OR (path_value#>>'{}')<>btrim(path_value#>>'{}') OR position(chr(92) in (path_value#>>'{}'))>0
         OR (path_value#>>'{}')~'(^/|(^|/)[.][.](/|$))' THEN RETURN false;END IF;
    END LOOP;
    IF item->>'kind'='source_package' THEN
      IF item->>'ref'<>p_base OR item->>'fingerprint'<>p_base THEN RETURN false;END IF;has_base:=true;
    ELSE
      IF item->>'ref'!~'^event_[a-z0-9]{16,64}$' THEN RETURN false;END IF;
      has_event:=true;
    END IF;
  END LOOP;
  RETURN has_base AND has_event;
END
$function$;

CREATE FUNCTION agent_learning_tests_valid(p_value JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE item JSONB;
BEGIN
  IF jsonb_typeof(p_value)<>'array' OR jsonb_array_length(p_value) NOT BETWEEN 1 AND 256
     OR octet_length(p_value::text)>65536 THEN RETURN false;END IF;
  FOR item IN SELECT value FROM jsonb_array_elements(p_value) LOOP
    IF jsonb_typeof(item)<>'object' OR (item-ARRAY['path','size','fingerprint'])<>'{}'::jsonb
       OR item->>'path'!~'^tests/[A-Za-z0-9._/-]+$' OR octet_length(item->>'path')>506
       OR jsonb_typeof(item->'size')<>'number' OR (item->>'size')::bigint NOT BETWEEN 0 AND 16777216
       OR item->>'fingerprint'!~'^sha256:[a-f0-9]{64}$' THEN RETURN false;END IF;
  END LOOP;RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_spec_valid(p_spec JSONB)
RETURNS BOOLEAN LANGUAGE plpgsql IMMUTABLE SET search_path FROM CURRENT AS $function$
DECLARE path_value JSONB;
BEGIN
  IF jsonb_typeof(p_spec)<>'object' OR NOT agent_learning_document_sanitized(p_spec)
     OR (p_spec-ARRAY['schemaVersion','sourceRunId','sourceSnapshotId','sourceSnapshotFingerprint','basePackageFingerprint',
       'proposedPackageFingerprint','runtimeBundleFingerprint','sbomFingerprint','archiveFingerprint','evidenceFingerprint',
       'testFingerprint','name','version','archiveBytes','evidence','tests','changedPaths'])<>'{}'::jsonb
     OR p_spec->>'schemaVersion'<>'neo.skill-draft/v1'
     OR p_spec->>'sourceRunId'!~'^run_[a-z0-9]{16,64}$'
     OR p_spec->>'sourceSnapshotId'!~'^snapshot_[a-z0-9]{16,64}$'
     OR p_spec->>'sourceSnapshotFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'basePackageFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'proposedPackageFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'proposedPackageFingerprint'=p_spec->>'basePackageFingerprint'
     OR p_spec->>'runtimeBundleFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'sbomFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'archiveFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'evidenceFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'testFingerprint'!~'^sha256:[a-f0-9]{64}$'
     OR p_spec->>'name'!~'^[a-z0-9]+(-[a-z0-9]+)*$' OR length(p_spec->>'name')>64
     OR octet_length(p_spec->>'version') NOT BETWEEN 1 AND 128
     OR jsonb_typeof(p_spec->'archiveBytes')<>'number' OR (p_spec->>'archiveBytes')::bigint NOT BETWEEN 1 AND 134217728
     OR NOT agent_learning_evidence_valid(p_spec->'evidence',p_spec->>'basePackageFingerprint')
     OR NOT agent_learning_tests_valid(p_spec->'tests')
     OR jsonb_typeof(p_spec->'changedPaths')<>'array' OR jsonb_array_length(p_spec->'changedPaths') NOT BETWEEN 1 AND 2048 THEN RETURN false;END IF;
  FOR path_value IN SELECT value FROM jsonb_array_elements(p_spec->'changedPaths') LOOP
    IF jsonb_typeof(path_value)<>'string' OR octet_length(path_value#>>'{}') NOT BETWEEN 1 AND 512
       OR (path_value#>>'{}')<>btrim(path_value#>>'{}') OR position(chr(92) in (path_value#>>'{}'))>0
       OR (path_value#>>'{}')~'(^/|(^|/)[.][.](/|$))' THEN RETURN false;END IF;
  END LOOP;RETURN true;
END
$function$;

CREATE FUNCTION agent_learning_metrics_valid(p_value JSONB)
RETURNS BOOLEAN LANGUAGE sql IMMUTABLE SET search_path FROM CURRENT AS $function$
  SELECT jsonb_typeof(p_value)='object' AND (SELECT count(*) FROM jsonb_each(p_value))<=16 AND octet_length(p_value::text)<=4096
    AND NOT EXISTS(SELECT 1 FROM jsonb_each(p_value) entry WHERE entry.key!~'^[a-z][A-Za-z0-9]{0,63}$'
      OR jsonb_typeof(entry.value)<>'number' OR (entry.value#>>'{}')::numeric<0 OR (entry.value#>>'{}')::numeric>9007199254740992)
$function$;

CREATE TABLE agent_learning_drafts(
  id TEXT PRIMARY KEY,user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,source_run_id TEXT NOT NULL,
  source_snapshot_id TEXT NOT NULL,source_snapshot_fingerprint TEXT NOT NULL,base_package_fingerprint TEXT NOT NULL
    REFERENCES skill_package_versions(package_fingerprint) ON DELETE RESTRICT,
  proposed_package_fingerprint TEXT NOT NULL,draft_fingerprint TEXT NOT NULL,spec JSONB NOT NULL,draft_object_key TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'quarantined',revision BIGINT NOT NULL DEFAULT 1,check_attempts INTEGER NOT NULL DEFAULT 0,
  check_generation BIGINT NOT NULL DEFAULT 0,check_owner TEXT,check_expires_at TIMESTAMPTZ,next_check_at TIMESTAMPTZ NOT NULL,
  admission_id UUID,promoted_package_fingerprint TEXT,cleanup_after TIMESTAMPTZ,object_deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_learning_draft_run_fk FOREIGN KEY(source_run_id,user_id) REFERENCES agent_runs(id,user_id) ON DELETE RESTRICT,
  CONSTRAINT agent_learning_draft_snapshot_fk FOREIGN KEY(source_snapshot_id,user_id,source_snapshot_fingerprint)
    REFERENCES agent_run_snapshots(id,user_id,fingerprint) ON DELETE RESTRICT,
  CONSTRAINT agent_learning_draft_id_check CHECK(id~'^draft_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_learning_draft_fingerprints CHECK(draft_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND proposed_package_fingerprint~'^sha256:[a-f0-9]{64}$' AND proposed_package_fingerprint<>base_package_fingerprint),
  CONSTRAINT agent_learning_draft_spec CHECK(agent_learning_spec_valid(spec)
    AND source_run_id=spec->>'sourceRunId' AND source_snapshot_id=spec->>'sourceSnapshotId'
    AND source_snapshot_fingerprint=spec->>'sourceSnapshotFingerprint'
    AND base_package_fingerprint=spec->>'basePackageFingerprint'
    AND proposed_package_fingerprint=spec->>'proposedPackageFingerprint'),
  CONSTRAINT agent_learning_draft_object CHECK(draft_object_key='skill-drafts/sha256/'||substring(spec->>'archiveFingerprint' FROM 8)||'.zip'),
  CONSTRAINT agent_learning_draft_state CHECK(state IN ('quarantined','checking','reviewable','check_failed','rejected','promoted')),
  CONSTRAINT agent_learning_draft_claim CHECK(check_attempts BETWEEN 0 AND 3 AND check_generation>=0 AND
    ((state='checking' AND check_owner IS NOT NULL AND check_expires_at IS NOT NULL) OR
     (state<>'checking' AND check_owner IS NULL AND check_expires_at IS NULL))),
  CONSTRAINT agent_learning_draft_promotion CHECK((state='promoted' AND admission_id IS NOT NULL AND promoted_package_fingerprint=proposed_package_fingerprint)
    OR (state<>'promoted' AND admission_id IS NULL AND promoted_package_fingerprint IS NULL)),
  CONSTRAINT agent_learning_draft_cleanup CHECK((state IN ('rejected','promoted') AND cleanup_after IS NOT NULL)
    OR (state NOT IN ('rejected','promoted') AND cleanup_after IS NULL)),
  CONSTRAINT agent_learning_draft_times CHECK(updated_at>=created_at AND next_check_at>=created_at
    AND (object_deleted_at IS NULL OR object_deleted_at>=created_at)),
  UNIQUE(source_run_id,draft_fingerprint),UNIQUE(id,user_id,draft_fingerprint),UNIQUE(admission_id)
);
CREATE INDEX idx_agent_learning_check_queue ON agent_learning_drafts(next_check_at,id)
  WHERE state IN ('quarantined','checking');

CREATE TABLE agent_learning_check_results(
  id TEXT PRIMARY KEY,draft_id TEXT NOT NULL,user_id UUID NOT NULL,draft_fingerprint TEXT NOT NULL,generation BIGINT NOT NULL,
  kind TEXT NOT NULL,status TEXT NOT NULL,reason_code TEXT NOT NULL,suite_fingerprint TEXT NOT NULL,evidence_fingerprint TEXT NOT NULL,
  duration_millis BIGINT NOT NULL,metrics JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_learning_check_draft_fk FOREIGN KEY(draft_id,user_id,draft_fingerprint)
    REFERENCES agent_learning_drafts(id,user_id,draft_fingerprint) ON DELETE CASCADE,
  CONSTRAINT agent_learning_check_id CHECK(id~'^draft_check_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_learning_check_shape CHECK(generation>=1 AND kind IN ('static','isolation','evaluation')
    AND status IN ('passed','failed') AND reason_code~'^[A-Z][A-Z0-9_]{0,63}$'
    AND suite_fingerprint~'^sha256:[a-f0-9]{64}$' AND evidence_fingerprint~'^sha256:[a-f0-9]{64}$'
    AND duration_millis BETWEEN 0 AND 86400000 AND agent_learning_metrics_valid(metrics)),
  UNIQUE(draft_id,generation,kind)
);

CREATE TABLE agent_learning_decisions(
  id TEXT PRIMARY KEY,draft_id TEXT NOT NULL,user_id UUID NOT NULL,draft_fingerprint TEXT NOT NULL,decision TEXT NOT NULL,
  expected_revision BIGINT NOT NULL,proposed_package_fingerprint TEXT NOT NULL,actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  reason_code TEXT NOT NULL,admission_id UUID,decided_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_learning_decision_draft_fk FOREIGN KEY(draft_id,user_id,draft_fingerprint)
    REFERENCES agent_learning_drafts(id,user_id,draft_fingerprint) ON DELETE CASCADE,
  CONSTRAINT agent_learning_decision_id CHECK(id~'^draft_decision_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_learning_decision_shape CHECK(decision IN ('reject','promote') AND expected_revision>=1
    AND proposed_package_fingerprint~'^sha256:[a-f0-9]{64}$' AND reason_code~'^[A-Z][A-Z0-9_]{0,63}$'
    AND ((decision='promote' AND admission_id IS NOT NULL) OR (decision='reject' AND admission_id IS NULL))),
  UNIQUE(draft_id),UNIQUE(admission_id)
);

CREATE TABLE agent_learning_cleanup_queue(
  draft_id TEXT PRIMARY KEY REFERENCES agent_learning_drafts(id) ON DELETE CASCADE,object_key TEXT NOT NULL,
  object_fingerprint TEXT NOT NULL,state TEXT NOT NULL DEFAULT 'pending',generation BIGINT NOT NULL DEFAULT 0,
  claim_owner TEXT,claim_expires_at TIMESTAMPTZ,attempts INTEGER NOT NULL DEFAULT 0,next_attempt_at TIMESTAMPTZ NOT NULL,
  error_code TEXT NOT NULL DEFAULT 'CLEANUP_PENDING',created_at TIMESTAMPTZ NOT NULL,updated_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_learning_cleanup_object CHECK(object_key='skill-drafts/sha256/'||substring(object_fingerprint FROM 8)||'.zip'
    AND object_fingerprint~'^sha256:[a-f0-9]{64}$'),
  CONSTRAINT agent_learning_cleanup_shape CHECK(state IN ('pending','claimed','failed') AND generation>=0 AND attempts BETWEEN 0 AND 10
    AND error_code~'^[A-Z][A-Z0-9_]{0,63}$' AND ((state='claimed' AND claim_owner IS NOT NULL AND claim_expires_at IS NOT NULL)
      OR (state<>'claimed' AND claim_owner IS NULL AND claim_expires_at IS NULL))),
  CONSTRAINT agent_learning_cleanup_times CHECK(updated_at>=created_at AND next_attempt_at>=created_at)
);
CREATE INDEX idx_agent_learning_cleanup_queue ON agent_learning_cleanup_queue(next_attempt_at,draft_id)
  WHERE state IN ('pending','claimed');

CREATE TABLE agent_learning_audit_events(
  id TEXT PRIMARY KEY,draft_id TEXT,user_id UUID NOT NULL,kind TEXT NOT NULL,actor_type TEXT NOT NULL,actor_id TEXT NOT NULL,
  reason_code TEXT NOT NULL,detail JSONB NOT NULL,occurred_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT agent_learning_audit_id CHECK(id~'^draft_event_[a-z0-9]{16,64}$'),
  CONSTRAINT agent_learning_audit_shape CHECK(kind IN ('draft.created','checks.completed','checks.retry','decision.rejected',
    'decision.promoted','cleanup.completed','cleanup.retry','claim.reclaimed','draft.pruned')
    AND actor_type IN ('run','checker','user','reconciler','cleaner') AND octet_length(actor_id) BETWEEN 1 AND 128
    AND reason_code~'^[A-Z][A-Z0-9_]{0,63}$' AND agent_learning_document_sanitized(detail)
    AND octet_length(detail::text)<=4096)
);

CREATE FUNCTION agent_learning_immutable() RETURNS trigger LANGUAGE plpgsql SET search_path FROM CURRENT AS $function$
BEGIN IF TG_OP='DELETE' AND (pg_trigger_depth()>1 OR current_user='agent_learning_owner') THEN RETURN OLD;END IF;
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='AGENT_LEARNING_IMMUTABLE';END
$function$;
CREATE TRIGGER trg_agent_learning_check_immutable BEFORE UPDATE OR DELETE ON agent_learning_check_results FOR EACH ROW EXECUTE FUNCTION agent_learning_immutable();
CREATE TRIGGER trg_agent_learning_decision_immutable BEFORE UPDATE OR DELETE ON agent_learning_decisions FOR EACH ROW EXECUTE FUNCTION agent_learning_immutable();
CREATE TRIGGER trg_agent_learning_audit_immutable BEFORE UPDATE OR DELETE ON agent_learning_audit_events FOR EACH ROW EXECUTE FUNCTION agent_learning_immutable();

CREATE FUNCTION agent_learning_append_audit(p_id TEXT,p_draft TEXT,p_user UUID,p_kind TEXT,p_actor_type TEXT,p_actor_id TEXT,
  p_reason TEXT,p_detail JSONB,p_at TIMESTAMPTZ) RETURNS VOID LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN INSERT INTO agent_learning_audit_events VALUES(p_id,p_draft,p_user,p_kind,p_actor_type,p_actor_id,p_reason,COALESCE(p_detail,'{}'),p_at);END
$function$;

CREATE FUNCTION agent_learning_source_denial(p_draft TEXT)
RETURNS TEXT LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE draft agent_learning_drafts%ROWTYPE;run agent_runs%ROWTYPE;snapshot agent_run_snapshots%ROWTYPE;scopes TEXT[];
BEGIN
  SELECT * INTO draft FROM agent_learning_drafts WHERE id=p_draft;
  IF draft.id IS NULL THEN RETURN 'DRAFT_NOT_FOUND';END IF;
  SELECT * INTO run FROM agent_runs WHERE id=draft.source_run_id AND user_id=draft.user_id;
  SELECT * INTO snapshot FROM agent_run_snapshots WHERE id=draft.source_snapshot_id AND user_id=draft.user_id
    AND fingerprint=draft.source_snapshot_fingerprint;
  IF run.id IS NULL OR run.state<>'succeeded' OR snapshot.id IS NULL OR snapshot.canonical_snapshot->>'packageFingerprint'<>draft.base_package_fingerprint
     OR COALESCE((snapshot.canonical_snapshot->>'depth')::integer,0)<>0 THEN RETURN 'SOURCE_RUN_INVALID';END IF;
  IF EXISTS(SELECT 1 FROM jsonb_array_elements(draft.spec->'evidence') item WHERE item->>'kind'='run_event'
    AND NOT EXISTS(SELECT 1 FROM agent_run_events event WHERE event.id=item->>'ref'
      AND event.run_id=draft.source_run_id AND event.user_id=draft.user_id)) THEN RETURN 'SOURCE_RUN_INVALID';END IF;
  IF EXISTS(SELECT 1 FROM skill_package_versions package WHERE package.package_fingerprint=draft.proposed_package_fingerprint)
     AND draft.state<>'promoted' THEN RETURN 'PACKAGE_ALREADY_EXISTS';END IF;
  scopes:=array_append(run.scope_keys,'learning:*');
  IF agent_orchestrator_active_kill_mode(scopes) IS NOT NULL THEN RETURN 'KILL_SWITCH_ACTIVE';END IF;
  RETURN NULL;
END
$function$;

CREATE FUNCTION agent_learning_create_draft(p_id TEXT,p_user UUID,p_draft_fingerprint TEXT,p_spec JSONB,p_object_key TEXT,
  p_created_at TIMESTAMPTZ,p_audit_id TEXT)
RETURNS TABLE(draft_id TEXT,state TEXT,revision BIGINT,created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE existing agent_learning_drafts%ROWTYPE;run agent_runs%ROWTYPE;snapshot agent_run_snapshots%ROWTYPE;denial TEXT;
BEGIN
  IF p_id!~'^draft_[a-z0-9]{16,64}$' OR p_draft_fingerprint!~'^sha256:[a-f0-9]{64}$'
     OR NOT agent_learning_spec_valid(p_spec) OR p_object_key<>'skill-drafts/sha256/'||substring(p_spec->>'archiveFingerprint' FROM 8)||'.zip'
     OR p_audit_id!~'^draft_event_[a-z0-9]{16,64}$' THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_DRAFT_INVALID';END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(p_user::text||E'\\x00'||(p_spec->>'sourceRunId')||E'\\x00'||p_draft_fingerprint,0));
  SELECT * INTO existing FROM agent_learning_drafts WHERE source_run_id=p_spec->>'sourceRunId' AND draft_fingerprint=p_draft_fingerprint;
  IF FOUND THEN RETURN QUERY SELECT existing.id,existing.state,existing.revision,false;RETURN;END IF;
  SELECT * INTO run FROM agent_runs WHERE id=p_spec->>'sourceRunId' AND user_id=p_user;
  SELECT * INTO snapshot FROM agent_run_snapshots WHERE id=p_spec->>'sourceSnapshotId' AND user_id=p_user
    AND fingerprint=p_spec->>'sourceSnapshotFingerprint';
  IF run.id IS NULL OR run.state<>'succeeded' OR snapshot.id IS NULL OR snapshot.canonical_snapshot->>'packageFingerprint'<>p_spec->>'basePackageFingerprint'
     OR COALESCE((snapshot.canonical_snapshot->>'depth')::integer,0)<>0 THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SOURCE_RUN_INVALID';END IF;
  IF EXISTS(SELECT 1 FROM jsonb_array_elements(p_spec->'evidence') item WHERE item->>'kind'='run_event'
    AND NOT EXISTS(SELECT 1 FROM agent_run_events event WHERE event.id=item->>'ref'
      AND event.run_id=p_spec->>'sourceRunId' AND event.user_id=p_user)) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SOURCE_RUN_INVALID';END IF;
  IF NOT EXISTS(SELECT 1 FROM skill_package_versions WHERE package_fingerprint=p_spec->>'basePackageFingerprint') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='SOURCE_RUN_INVALID';END IF;
  IF EXISTS(SELECT 1 FROM skill_package_versions WHERE package_fingerprint=p_spec->>'proposedPackageFingerprint') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PACKAGE_ALREADY_EXISTS';END IF;
  IF agent_orchestrator_active_kill_mode(array_append(run.scope_keys,'learning:*')) IS NOT NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='KILL_SWITCH_ACTIVE';END IF;
  INSERT INTO agent_learning_drafts(id,user_id,source_run_id,source_snapshot_id,source_snapshot_fingerprint,base_package_fingerprint,
    proposed_package_fingerprint,draft_fingerprint,spec,draft_object_key,next_check_at,created_at,updated_at)
  VALUES(p_id,p_user,p_spec->>'sourceRunId',p_spec->>'sourceSnapshotId',p_spec->>'sourceSnapshotFingerprint',
    p_spec->>'basePackageFingerprint',p_spec->>'proposedPackageFingerprint',p_draft_fingerprint,p_spec,p_object_key,p_created_at,p_created_at,p_created_at);
  PERFORM agent_learning_append_audit(p_audit_id,p_id,p_user,'draft.created','run',p_spec->>'sourceRunId','DRAFT_QUARANTINED',
    jsonb_build_object('draftFingerprint',p_draft_fingerprint,'packageFingerprint',p_spec->>'proposedPackageFingerprint'),p_created_at);
  RETURN QUERY SELECT p_id,'quarantined'::text,1::bigint,true;
END
$function$;

CREATE FUNCTION agent_learning_claim_checks(p_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER)
RETURNS SETOF agent_learning_drafts LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_owner) NOT BETWEEN 1 AND 128 OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CLAIM_INVALID';END IF;
  RETURN QUERY WITH candidates AS(
    SELECT id FROM agent_learning_drafts WHERE state='quarantined' AND next_check_at<=p_now
      ORDER BY next_check_at,id FOR UPDATE SKIP LOCKED LIMIT p_limit
  ) UPDATE agent_learning_drafts draft SET state='checking',check_generation=draft.check_generation+1,check_owner=p_owner,
    check_expires_at=p_now+make_interval(secs=>p_lease_seconds),updated_at=GREATEST(draft.updated_at,p_now)
    FROM candidates WHERE draft.id=candidates.id RETURNING draft.*;
END
$function$;

CREATE FUNCTION agent_learning_complete_checks(p_draft TEXT,p_owner TEXT,p_generation BIGINT,p_results JSONB,p_audit_id TEXT)
RETURNS agent_learning_drafts LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE draft agent_learning_drafts%ROWTYPE;item JSONB;kinds TEXT[];all_passed BOOLEAN;now_at TIMESTAMPTZ:=clock_timestamp();denial TEXT;
BEGIN
  SELECT * INTO draft FROM agent_learning_drafts WHERE id=p_draft FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='DRAFT_NOT_FOUND';END IF;
  IF draft.state<>'checking' OR draft.check_owner IS DISTINCT FROM p_owner OR draft.check_generation<>p_generation
     OR draft.check_expires_at<=now_at THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';END IF;
  IF jsonb_typeof(p_results)<>'array' OR jsonb_array_length(p_results)<>3 OR p_audit_id!~'^draft_event_[a-z0-9]{16,64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CHECK_INVALID';END IF;
  SELECT array_agg(value->>'kind' ORDER BY value->>'kind') INTO kinds FROM jsonb_array_elements(p_results);
  IF kinds<>ARRAY['evaluation','isolation','static'] THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CHECK_INVALID';END IF;
  FOR item IN SELECT value FROM jsonb_array_elements(p_results) LOOP
    IF jsonb_typeof(item)<>'object' OR (item-ARRAY['id','kind','status','reasonCode','suiteFingerprint','evidenceFingerprint','durationMillis','metrics'])<>'{}'::jsonb
       OR item->>'id'!~'^draft_check_[a-z0-9]{16,64}$' OR item->>'status' NOT IN ('passed','failed')
       OR item->>'reasonCode'!~'^[A-Z][A-Z0-9_]{0,63}$' OR item->>'suiteFingerprint'!~'^sha256:[a-f0-9]{64}$'
       OR item->>'evidenceFingerprint'!~'^sha256:[a-f0-9]{64}$' OR jsonb_typeof(item->'durationMillis')<>'number'
       OR (item->>'durationMillis')::bigint NOT BETWEEN 0 AND 86400000 OR NOT agent_learning_metrics_valid(item->'metrics') THEN
      RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CHECK_INVALID';END IF;
  END LOOP;
  denial:=agent_learning_source_denial(p_draft);IF denial IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE=denial;END IF;
  INSERT INTO agent_learning_check_results(id,draft_id,user_id,draft_fingerprint,generation,kind,status,reason_code,
    suite_fingerprint,evidence_fingerprint,duration_millis,metrics,created_at)
  SELECT value->>'id',draft.id,draft.user_id,draft.draft_fingerprint,p_generation,value->>'kind',value->>'status',value->>'reasonCode',
    value->>'suiteFingerprint',value->>'evidenceFingerprint',(value->>'durationMillis')::bigint,value->'metrics',now_at
    FROM jsonb_array_elements(p_results);
  SELECT bool_and(value->>'status'='passed') INTO all_passed FROM jsonb_array_elements(p_results);
  UPDATE agent_learning_drafts SET state=CASE WHEN all_passed THEN 'reviewable' ELSE 'check_failed' END,revision=revision+1,
    check_owner=NULL,check_expires_at=NULL,updated_at=now_at WHERE id=p_draft RETURNING * INTO draft;
  PERFORM agent_learning_append_audit(p_audit_id,draft.id,draft.user_id,'checks.completed','checker',p_owner,
    CASE WHEN all_passed THEN 'CHECKS_PASSED' ELSE 'CHECKS_FAILED' END,jsonb_build_object('generation',p_generation),now_at);
  RETURN draft;
END
$function$;

CREATE FUNCTION agent_learning_release_check(p_draft TEXT,p_owner TEXT,p_generation BIGINT,p_error TEXT,p_retry_at TIMESTAMPTZ,p_audit_id TEXT)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE draft agent_learning_drafts%ROWTYPE;attempts INTEGER;terminal BOOLEAN;now_at TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT * INTO draft FROM agent_learning_drafts WHERE id=p_draft FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='DRAFT_NOT_FOUND';END IF;
  IF draft.state<>'checking' OR draft.check_owner IS DISTINCT FROM p_owner OR draft.check_generation<>p_generation
     OR draft.check_expires_at<=now_at THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';END IF;
  IF p_error!~'^[A-Z][A-Z0-9_]{0,63}$' OR p_retry_at<=now_at OR p_retry_at>now_at+interval '1 day'
     OR p_audit_id!~'^draft_event_[a-z0-9]{16,64}$' THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RETRY_INVALID';END IF;
  attempts:=draft.check_attempts+1;terminal:=attempts>=3;
  UPDATE agent_learning_drafts SET state=CASE WHEN terminal THEN 'check_failed' ELSE 'quarantined' END,revision=revision+1,
    check_attempts=attempts,check_owner=NULL,check_expires_at=NULL,next_check_at=p_retry_at,updated_at=now_at WHERE id=p_draft;
  PERFORM agent_learning_append_audit(p_audit_id,draft.id,draft.user_id,'checks.retry','checker',p_owner,p_error,
    jsonb_build_object('attempts',attempts,'terminal',terminal),now_at);RETURN terminal;
END
$function$;

CREATE FUNCTION agent_learning_reject(p_draft TEXT,p_actor UUID,p_expected_revision BIGINT,p_draft_fingerprint TEXT,
  p_proposed_fingerprint TEXT,p_decision_id TEXT,p_reason TEXT,p_audit_id TEXT)
RETURNS TABLE(draft_id TEXT,state TEXT,revision BIGINT,created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE draft agent_learning_drafts%ROWTYPE;decision agent_learning_decisions%ROWTYPE;now_at TIMESTAMPTZ:=clock_timestamp();
BEGIN
  SELECT * INTO draft FROM agent_learning_drafts WHERE id=p_draft FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='DRAFT_NOT_FOUND';END IF;
  IF draft.state='rejected' THEN
    SELECT item.* INTO decision FROM agent_learning_decisions item WHERE item.draft_id=p_draft;
    IF decision.decision='reject' AND decision.actor_user_id=p_actor AND decision.expected_revision=p_expected_revision
       AND decision.reason_code=p_reason
       AND decision.draft_fingerprint=p_draft_fingerprint AND decision.proposed_package_fingerprint=p_proposed_fingerprint THEN
      RETURN QUERY SELECT draft.id,draft.state,draft.revision,false;RETURN;END IF;
  END IF;
  IF draft.state='promoted' THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROMOTION_DENIED';END IF;
  IF draft.revision<>p_expected_revision OR draft.draft_fingerprint<>p_draft_fingerprint
     OR draft.proposed_package_fingerprint<>p_proposed_fingerprint THEN RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';END IF;
  IF NOT EXISTS(SELECT 1 FROM users WHERE id=p_actor AND deleted_at IS NULL) OR p_decision_id!~'^draft_decision_[a-z0-9]{16,64}$'
     OR p_reason!~'^[A-Z][A-Z0-9_]{0,63}$' OR p_audit_id!~'^draft_event_[a-z0-9]{16,64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_REVIEW_INVALID';END IF;
  INSERT INTO agent_learning_decisions VALUES(p_decision_id,draft.id,draft.user_id,draft.draft_fingerprint,'reject',
    p_expected_revision,draft.proposed_package_fingerprint,p_actor,p_reason,NULL,now_at);
  UPDATE agent_learning_drafts SET state='rejected',revision=revision+1,cleanup_after=now_at+interval '7 days',
    check_owner=NULL,check_expires_at=NULL,updated_at=now_at WHERE id=p_draft RETURNING * INTO draft;
  INSERT INTO agent_learning_cleanup_queue(draft_id,object_key,object_fingerprint,next_attempt_at,created_at,updated_at)
    VALUES(draft.id,draft.draft_object_key,draft.spec->>'archiveFingerprint',draft.cleanup_after,now_at,now_at);
  PERFORM agent_learning_append_audit(p_audit_id,draft.id,draft.user_id,'decision.rejected','user',p_actor::text,p_reason,'{}',now_at);
  RETURN QUERY SELECT draft.id,draft.state,draft.revision,true;
END
$function$;

CREATE FUNCTION agent_learning_promote(p_draft TEXT,p_actor UUID,p_expected_revision BIGINT,p_draft_fingerprint TEXT,
  p_proposed_fingerprint TEXT,p_candidate UUID,p_decision_id TEXT,p_reason TEXT,p_source_ref TEXT,p_source_object_key TEXT,
  p_package JSONB,p_audit_id TEXT)
RETURNS TABLE(draft_id TEXT,admission_id UUID,package_fingerprint TEXT,created BOOLEAN)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE draft agent_learning_drafts%ROWTYPE;decision agent_learning_decisions%ROWTYPE;now_at TIMESTAMPTZ:=clock_timestamp();denial TEXT;
  runtime_fingerprint TEXT;
BEGIN
  SELECT * INTO draft FROM agent_learning_drafts WHERE id=p_draft FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='DRAFT_NOT_FOUND';END IF;
  IF draft.state='promoted' THEN
    SELECT item.* INTO decision FROM agent_learning_decisions item WHERE item.draft_id=p_draft;
    IF decision.decision='promote' AND decision.actor_user_id=p_actor AND decision.expected_revision=p_expected_revision
       AND decision.reason_code=p_reason
       AND decision.draft_fingerprint=p_draft_fingerprint AND decision.proposed_package_fingerprint=p_proposed_fingerprint THEN
      RETURN QUERY SELECT draft.id,draft.admission_id,draft.promoted_package_fingerprint,false;RETURN;END IF;
  END IF;
  IF draft.state<>'reviewable' THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='PROMOTION_DENIED';END IF;
  IF draft.revision<>p_expected_revision OR draft.draft_fingerprint<>p_draft_fingerprint
     OR draft.proposed_package_fingerprint<>p_proposed_fingerprint THEN RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='REVISION_CONFLICT';END IF;
  IF NOT EXISTS(SELECT 1 FROM users WHERE id=p_actor AND deleted_at IS NULL) OR p_decision_id!~'^draft_decision_[a-z0-9]{16,64}$'
     OR p_reason!~'^[A-Z][A-Z0-9_]{0,63}$' OR p_audit_id!~'^draft_event_[a-z0-9]{16,64}$'
     OR p_source_ref<>'draft:'||draft.id||':'||draft.draft_fingerprint
     OR p_source_object_key<>'skill-quarantine/sha256/'||substring(draft.spec->>'archiveFingerprint' FROM 8)||'.zip'
     OR jsonb_typeof(p_package)<>'object' OR (p_package-ARRAY['runtimeBundleFingerprint','sbomFingerprint','name','version','description',
       'license','compatibility','allowedTools','capabilityRequests','hasRuntime','fileCount','packageBytes','expandedBytes',
       'packageObjectKey','sbomObjectKey'])<>'{}'::jsonb THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_PROMOTION_INVALID';END IF;
  IF p_package->>'name'<>draft.spec->>'name' OR p_package->>'version'<>draft.spec->>'version'
     OR p_package->>'runtimeBundleFingerprint'<>draft.spec->>'runtimeBundleFingerprint'
     OR p_package->>'sbomFingerprint'<>draft.spec->>'sbomFingerprint' OR (p_package->>'hasRuntime')::boolean IS NOT TRUE
     OR p_package->>'packageObjectKey'<>'skill-packages/sha256/'||substring(draft.proposed_package_fingerprint FROM 8)||'.zip'
     OR p_package->>'sbomObjectKey'<>'skill-sboms/sha256/'||substring(p_package->>'sbomFingerprint' FROM 8)||'.cdx.json' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_PROMOTION_INVALID';END IF;
  IF (SELECT count(*) FROM agent_learning_check_results result WHERE result.draft_id=draft.id
      AND result.generation=draft.check_generation AND result.status='passed')<>3 THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='CHECKS_INCOMPLETE';END IF;
  denial:=agent_learning_source_denial(draft.id);IF denial IS NOT NULL THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE=denial;END IF;
  runtime_fingerprint:=NULLIF(p_package->>'runtimeBundleFingerprint','');
  INSERT INTO skill_package_versions(package_fingerprint,runtime_bundle_fingerprint,sbom_fingerprint,name,version,description,license,
    compatibility,allowed_tools,capability_requests,has_runtime,file_count,package_bytes,expanded_bytes,package_object_key,sbom_object_key,created_at)
  VALUES(draft.proposed_package_fingerprint,runtime_fingerprint,p_package->>'sbomFingerprint',p_package->>'name',p_package->>'version',
    p_package->>'description',p_package->>'license',p_package->>'compatibility',p_package->'allowedTools',p_package->'capabilityRequests',
    (p_package->>'hasRuntime')::boolean,(p_package->>'fileCount')::integer,(p_package->>'packageBytes')::bigint,
    (p_package->>'expandedBytes')::bigint,p_package->>'packageObjectKey',p_package->>'sbomObjectKey',now_at);
  INSERT INTO skill_package_candidates(id,source_type,source_ref,source_artifact_sha256,source_object_key,package_fingerprint,status,
    admission_eligible,validation_summary,reviewed_by_user_id,review_reason,revision,created_at,updated_at)
  VALUES(p_candidate,'learning',p_source_ref,draft.spec->>'archiveFingerprint',p_source_object_key,draft.proposed_package_fingerprint,
    'admitted',true,'learning_checks_passed',p_actor,p_reason,2,now_at,now_at);
  INSERT INTO agent_learning_decisions VALUES(p_decision_id,draft.id,draft.user_id,draft.draft_fingerprint,'promote',p_expected_revision,
    draft.proposed_package_fingerprint,p_actor,p_reason,p_candidate,now_at);
  UPDATE agent_learning_drafts SET state='promoted',revision=revision+1,admission_id=p_candidate,
    promoted_package_fingerprint=proposed_package_fingerprint,cleanup_after=now_at+interval '7 days',updated_at=now_at
    WHERE id=draft.id RETURNING * INTO draft;
  INSERT INTO agent_learning_cleanup_queue(draft_id,object_key,object_fingerprint,next_attempt_at,created_at,updated_at)
    VALUES(draft.id,draft.draft_object_key,draft.spec->>'archiveFingerprint',draft.cleanup_after,now_at,now_at);
  PERFORM agent_learning_append_audit(p_audit_id,draft.id,draft.user_id,'decision.promoted','user',p_actor::text,p_reason,
    jsonb_build_object('admissionId',p_candidate,'packageFingerprint',draft.proposed_package_fingerprint),now_at);
  RETURN QUERY SELECT draft.id,p_candidate,draft.proposed_package_fingerprint,true;
END
$function$;

CREATE FUNCTION agent_learning_claim_cleanup(p_owner TEXT,p_now TIMESTAMPTZ,p_lease_seconds INTEGER,p_limit INTEGER)
RETURNS SETOF agent_learning_cleanup_queue LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
BEGIN
  IF octet_length(p_owner) NOT BETWEEN 1 AND 128 OR p_lease_seconds NOT BETWEEN 5 AND 300 OR p_limit NOT BETWEEN 1 AND 1000 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_CLAIM_INVALID';END IF;
  RETURN QUERY WITH candidates AS(SELECT draft_id FROM agent_learning_cleanup_queue WHERE
    state='pending' AND next_attempt_at<=p_now
    ORDER BY next_attempt_at,draft_id FOR UPDATE SKIP LOCKED LIMIT p_limit)
  UPDATE agent_learning_cleanup_queue queue SET state='claimed',generation=queue.generation+1,claim_owner=p_owner,
    claim_expires_at=p_now+make_interval(secs=>p_lease_seconds),updated_at=GREATEST(queue.updated_at,p_now)
    FROM candidates WHERE queue.draft_id=candidates.draft_id RETURNING queue.*;
END
$function$;

CREATE FUNCTION agent_learning_complete_cleanup(p_draft TEXT,p_owner TEXT,p_generation BIGINT,p_object_fingerprint TEXT)
RETURNS VOID LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE queue agent_learning_cleanup_queue%ROWTYPE;draft agent_learning_drafts%ROWTYPE;now_at TIMESTAMPTZ:=clock_timestamp();event_id TEXT;
BEGIN
  SELECT * INTO queue FROM agent_learning_cleanup_queue WHERE draft_id=p_draft FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='DRAFT_NOT_FOUND';END IF;
  IF queue.state<>'claimed' OR queue.claim_owner IS DISTINCT FROM p_owner OR queue.generation<>p_generation
     OR queue.claim_expires_at<=now_at OR queue.object_fingerprint<>p_object_fingerprint THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';END IF;
  UPDATE agent_learning_drafts SET object_deleted_at=now_at,updated_at=now_at WHERE id=p_draft RETURNING * INTO draft;
  DELETE FROM agent_learning_cleanup_queue WHERE draft_id=p_draft;
  event_id:='draft_event_'||substr(md5(p_draft||'|'||p_generation::text||'|cleanup'),1,32);
  PERFORM agent_learning_append_audit(event_id,draft.id,draft.user_id,'cleanup.completed','cleaner',p_owner,'OBJECT_DELETED','{}',now_at);
END
$function$;

CREATE FUNCTION agent_learning_release_cleanup(p_draft TEXT,p_owner TEXT,p_generation BIGINT,p_error TEXT,p_retry_at TIMESTAMPTZ)
RETURNS BOOLEAN LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE queue agent_learning_cleanup_queue%ROWTYPE;draft agent_learning_drafts%ROWTYPE;next_attempts INTEGER;terminal BOOLEAN;
  now_at TIMESTAMPTZ:=clock_timestamp();event_id TEXT;
BEGIN
  SELECT * INTO queue FROM agent_learning_cleanup_queue WHERE draft_id=p_draft FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='DRAFT_NOT_FOUND';END IF;
  IF queue.state<>'claimed' OR queue.claim_owner IS DISTINCT FROM p_owner OR queue.generation<>p_generation
     OR queue.claim_expires_at<=now_at THEN RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='STALE_CLAIM';END IF;
  IF p_error!~'^[A-Z][A-Z0-9_]{0,63}$' OR p_retry_at<=now_at OR p_retry_at>now_at+interval '1 day' THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_RETRY_INVALID';END IF;
  next_attempts:=queue.attempts+1;terminal:=next_attempts>=10;
  UPDATE agent_learning_cleanup_queue SET state=CASE WHEN terminal THEN 'failed' ELSE 'pending' END,attempts=next_attempts,
    claim_owner=NULL,claim_expires_at=NULL,next_attempt_at=p_retry_at,error_code=p_error,updated_at=now_at WHERE draft_id=p_draft;
  SELECT * INTO draft FROM agent_learning_drafts WHERE id=p_draft;
  event_id:='draft_event_'||substr(md5(p_draft||'|'||p_generation::text||'|cleanup-retry'),1,32);
  INSERT INTO agent_learning_audit_events VALUES(event_id,draft.id,draft.user_id,'cleanup.retry','cleaner',p_owner,p_error,
    jsonb_build_object('attempts',next_attempts,'terminal',terminal),now_at) ON CONFLICT(id) DO NOTHING;RETURN terminal;
END
$function$;

CREATE FUNCTION agent_learning_reconcile(p_now TIMESTAMPTZ,p_limit INTEGER)
RETURNS TABLE(checks_reclaimed INTEGER,cleanup_reclaimed INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE check_count INTEGER:=0;cleanup_count INTEGER:=0;target RECORD;next_attempts INTEGER;terminal BOOLEAN;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 THEN RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_LIMIT_INVALID';END IF;
  FOR target IN SELECT id,user_id,check_generation,check_attempts FROM agent_learning_drafts
    WHERE state='checking' AND check_expires_at<=p_now ORDER BY check_expires_at,id FOR UPDATE SKIP LOCKED LIMIT p_limit LOOP
    next_attempts:=target.check_attempts+1;terminal:=next_attempts>=3;
    UPDATE agent_learning_drafts SET state=CASE WHEN terminal THEN 'check_failed' ELSE 'quarantined' END,revision=revision+1,
      check_attempts=next_attempts,check_owner=NULL,check_expires_at=NULL,next_check_at=p_now,updated_at=GREATEST(updated_at,p_now) WHERE id=target.id;
    INSERT INTO agent_learning_audit_events VALUES('draft_event_'||substr(md5(target.id||'|'||target.check_generation::text||'|reclaim'),1,32),
      target.id,target.user_id,'claim.reclaimed','reconciler','reconciler','CHECK_CLAIM_RECLAIMED',
      jsonb_build_object('generation',target.check_generation,'terminal',terminal),p_now) ON CONFLICT(id) DO NOTHING;check_count:=check_count+1;
  END LOOP;
  FOR target IN SELECT queue.draft_id,queue.generation,queue.attempts,draft.user_id FROM agent_learning_cleanup_queue queue
    JOIN agent_learning_drafts draft ON draft.id=queue.draft_id WHERE queue.state='claimed' AND queue.claim_expires_at<=p_now
    ORDER BY queue.claim_expires_at,queue.draft_id FOR UPDATE OF queue SKIP LOCKED LIMIT p_limit LOOP
    next_attempts:=target.attempts+1;terminal:=next_attempts>=10;
    UPDATE agent_learning_cleanup_queue SET state=CASE WHEN terminal THEN 'failed' ELSE 'pending' END,attempts=next_attempts,
      claim_owner=NULL,claim_expires_at=NULL,next_attempt_at=p_now,error_code='CLEANUP_CLAIM_RECLAIMED',updated_at=GREATEST(updated_at,p_now)
      WHERE draft_id=target.draft_id;
    INSERT INTO agent_learning_audit_events VALUES('draft_event_'||substr(md5(target.draft_id||'|'||target.generation::text||'|cleanup-reclaim'),1,32),
      target.draft_id,target.user_id,'claim.reclaimed','reconciler','reconciler','CLEANUP_CLAIM_RECLAIMED',
      jsonb_build_object('generation',target.generation,'terminal',terminal),p_now) ON CONFLICT(id) DO NOTHING;cleanup_count:=cleanup_count+1;
  END LOOP;RETURN QUERY SELECT check_count,cleanup_count;
END
$function$;

CREATE FUNCTION agent_learning_prune(p_cutoff TIMESTAMPTZ,p_limit INTEGER)
RETURNS TABLE(drafts_pruned INTEGER,audits_pruned INTEGER)
LANGUAGE plpgsql SECURITY DEFINER SET search_path FROM CURRENT AS $function$
DECLARE draft_count INTEGER:=0;audit_count INTEGER:=0;
BEGIN
  IF p_limit NOT BETWEEN 1 AND 1000 OR p_cutoff>clock_timestamp() THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='AGENT_LEARNING_PRUNE_INVALID';END IF;
  WITH targets AS(SELECT id FROM agent_learning_drafts WHERE state IN ('rejected','promoted') AND object_deleted_at<p_cutoff
    ORDER BY object_deleted_at,id LIMIT p_limit) DELETE FROM agent_learning_drafts draft USING targets WHERE draft.id=targets.id;
  GET DIAGNOSTICS draft_count=ROW_COUNT;
  WITH targets AS(SELECT id FROM agent_learning_audit_events WHERE occurred_at<p_cutoff ORDER BY occurred_at,id LIMIT p_limit)
    DELETE FROM agent_learning_audit_events event USING targets WHERE event.id=targets.id;GET DIAGNOSTICS audit_count=ROW_COUNT;
  RETURN QUERY SELECT draft_count,audit_count;
END
$function$;

DO $harden$ DECLARE schema_name TEXT:=current_schema();identity TEXT;
BEGIN FOREACH identity IN ARRAY ARRAY[
  'agent_learning_document_sanitized(jsonb)','agent_learning_evidence_valid(jsonb,text)',
  'agent_learning_tests_valid(jsonb)','agent_learning_spec_valid(jsonb)','agent_learning_metrics_valid(jsonb)',
  'agent_learning_immutable()','agent_learning_append_audit(text,text,uuid,text,text,text,text,jsonb,timestamp with time zone)',
  'agent_learning_source_denial(text)','agent_learning_create_draft(text,uuid,text,jsonb,text,timestamp with time zone,text)',
  'agent_learning_claim_checks(text,timestamp with time zone,integer,integer)',
  'agent_learning_complete_checks(text,text,bigint,jsonb,text)',
  'agent_learning_release_check(text,text,bigint,text,timestamp with time zone,text)',
  'agent_learning_reject(text,uuid,bigint,text,text,text,text,text)',
  'agent_learning_promote(text,uuid,bigint,text,text,uuid,text,text,text,text,jsonb,text)',
  'agent_learning_claim_cleanup(text,timestamp with time zone,integer,integer)',
  'agent_learning_complete_cleanup(text,text,bigint,text)',
  'agent_learning_release_cleanup(text,text,bigint,text,timestamp with time zone)',
  'agent_learning_reconcile(timestamp with time zone,integer)','agent_learning_prune(timestamp with time zone,integer)'
  ] LOOP EXECUTE format('ALTER FUNCTION %I.%s SET search_path TO %I, pg_catalog, pg_temp',schema_name,identity,schema_name);END LOOP;
END
$harden$;

ALTER TABLE agent_learning_drafts OWNER TO agent_learning_owner;ALTER TABLE agent_learning_check_results OWNER TO agent_learning_owner;
ALTER TABLE agent_learning_decisions OWNER TO agent_learning_owner;ALTER TABLE agent_learning_cleanup_queue OWNER TO agent_learning_owner;
ALTER TABLE agent_learning_audit_events OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_document_sanitized(JSONB) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_evidence_valid(JSONB,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_tests_valid(JSONB) OWNER TO agent_learning_owner;ALTER FUNCTION agent_learning_spec_valid(JSONB) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_metrics_valid(JSONB) OWNER TO agent_learning_owner;ALTER FUNCTION agent_learning_immutable() OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_append_audit(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TIMESTAMPTZ) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_source_denial(TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_create_draft(TEXT,UUID,TEXT,JSONB,TEXT,TIMESTAMPTZ,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_claim_checks(TEXT,TIMESTAMPTZ,INTEGER,INTEGER) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_complete_checks(TEXT,TEXT,BIGINT,JSONB,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_release_check(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_reject(TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_promote(TEXT,UUID,BIGINT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_claim_cleanup(TEXT,TIMESTAMPTZ,INTEGER,INTEGER) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_complete_cleanup(TEXT,TEXT,BIGINT,TEXT) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_release_cleanup(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ) OWNER TO agent_learning_owner;
ALTER FUNCTION agent_learning_reconcile(TIMESTAMPTZ,INTEGER) OWNER TO agent_learning_owner;ALTER FUNCTION agent_learning_prune(TIMESTAMPTZ,INTEGER) OWNER TO agent_learning_owner;

REVOKE ALL ON agent_learning_drafts,agent_learning_check_results,agent_learning_decisions,agent_learning_cleanup_queue,agent_learning_audit_events
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control,agent_cron_control,agent_learning_control;
GRANT SELECT ON agent_learning_drafts,agent_learning_check_results,agent_learning_decisions,agent_learning_cleanup_queue,agent_learning_audit_events TO agent_learning_control;
REVOKE ALL ON FUNCTION agent_learning_document_sanitized(JSONB),agent_learning_evidence_valid(JSONB,TEXT),
  agent_learning_tests_valid(JSONB),agent_learning_spec_valid(JSONB),agent_learning_metrics_valid(JSONB),agent_learning_immutable(),
  agent_learning_append_audit(TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TIMESTAMPTZ),agent_learning_source_denial(TEXT)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control,agent_cron_control,agent_learning_control;
REVOKE ALL ON FUNCTION agent_learning_create_draft(TEXT,UUID,TEXT,JSONB,TEXT,TIMESTAMPTZ,TEXT),
  agent_learning_claim_checks(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),agent_learning_complete_checks(TEXT,TEXT,BIGINT,JSONB,TEXT),
  agent_learning_release_check(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),agent_learning_reject(TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_learning_promote(TEXT,UUID,BIGINT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT),
  agent_learning_claim_cleanup(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),agent_learning_complete_cleanup(TEXT,TEXT,BIGINT,TEXT),
  agent_learning_release_cleanup(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ),agent_learning_reconcile(TIMESTAMPTZ,INTEGER),
  agent_learning_prune(TIMESTAMPTZ,INTEGER)
  FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_delegation_control,agent_cron_control,agent_learning_control;
GRANT EXECUTE ON FUNCTION agent_learning_create_draft(TEXT,UUID,TEXT,JSONB,TEXT,TIMESTAMPTZ,TEXT),
  agent_learning_claim_checks(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),agent_learning_complete_checks(TEXT,TEXT,BIGINT,JSONB,TEXT),
  agent_learning_release_check(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ,TEXT),agent_learning_reject(TEXT,UUID,BIGINT,TEXT,TEXT,TEXT,TEXT,TEXT),
  agent_learning_promote(TEXT,UUID,BIGINT,TEXT,TEXT,UUID,TEXT,TEXT,TEXT,TEXT,JSONB,TEXT),
  agent_learning_claim_cleanup(TEXT,TIMESTAMPTZ,INTEGER,INTEGER),agent_learning_complete_cleanup(TEXT,TEXT,BIGINT,TEXT),
  agent_learning_release_cleanup(TEXT,TEXT,BIGINT,TEXT,TIMESTAMPTZ),agent_learning_reconcile(TIMESTAMPTZ,INTEGER),
  agent_learning_prune(TIMESTAMPTZ,INTEGER) TO agent_learning_control;

DO $schema_acl$ BEGIN
  EXECUTE format('REVOKE CREATE ON SCHEMA %I FROM agent_learning_owner,agent_learning_control,go_api_runtime',current_schema());
  EXECUTE format('GRANT USAGE ON SCHEMA %I TO agent_learning_owner,agent_learning_control',current_schema());
END $schema_acl$;
GRANT SELECT ON users,skill_package_versions,skill_package_candidates,agent_runs,agent_run_snapshots,agent_run_events,agent_kill_switches TO agent_learning_owner;
GRANT INSERT ON skill_package_versions,skill_package_candidates TO agent_learning_owner;
GRANT EXECUTE ON FUNCTION agent_orchestrator_active_kill_mode(TEXT[]) TO agent_learning_owner;
