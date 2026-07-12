-- Phase 15.2B extension-independent durable projection consistency layer.

DO $roles$
DECLARE
  role_name TEXT;
  can_create BOOLEAN;
BEGIN
  SELECT rolsuper OR rolcreaterole INTO can_create
  FROM pg_roles WHERE rolname = current_user;
  FOREACH role_name IN ARRAY ARRAY[
    'rag_projection_owner',
    'rag_worker_executor',
    'rag_replay_operator',
    'rag_api_reader',
    'go_evidence_hydrator',
    'go_api_runtime'
  ] LOOP
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
      IF NOT can_create THEN
        RAISE EXCEPTION USING
          ERRCODE = '42501',
          MESSAGE = 'RAG_REQUIRED_ROLE_MISSING';
      END IF;
      EXECUTE format(
        'CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS',
        role_name
      );
    END IF;
    IF EXISTS (
      SELECT 1
      FROM pg_roles
      WHERE rolname = role_name
        AND (
          rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole
          OR rolreplication OR rolbypassrls
        )
    ) THEN
      RAISE EXCEPTION USING
        ERRCODE = '42501',
        MESSAGE = 'RAG_REQUIRED_ROLE_MUST_BE_RESTRICTED';
    END IF;
    IF EXISTS (
      SELECT 1
      FROM pg_auth_members membership
      JOIN pg_roles member_role ON member_role.oid = membership.member
      WHERE member_role.rolname = role_name
        AND role_name <> 'go_api_runtime'
    ) THEN
      RAISE EXCEPTION USING
        ERRCODE = '42501',
        MESSAGE = 'RAG_REQUIRED_ROLE_MUST_NOT_INHERIT_MEMBERSHIP';
    END IF;
  END LOOP;

  IF pg_has_role('go_api_runtime', 'rag_projection_owner', 'MEMBER')
    OR pg_has_role('go_api_runtime', 'rag_worker_executor', 'MEMBER')
    OR pg_has_role('go_api_runtime', 'rag_replay_operator', 'MEMBER')
    OR EXISTS (
      SELECT 1
      FROM pg_auth_members membership
      WHERE membership.member = 'go_api_runtime'::REGROLE
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'RAG_GO_API_RUNTIME_FORBIDDEN_MEMBERSHIP';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM pg_namespace namespace
    WHERE namespace.nspname = current_schema()
      AND namespace.nspowner = 'go_api_runtime'::REGROLE
  ) OR EXISTS (
    SELECT 1
    FROM pg_class relation
    WHERE relation.relnamespace = current_schema()::REGNAMESPACE
      AND relation.relowner = 'go_api_runtime'::REGROLE
  ) OR EXISTS (
    SELECT 1
    FROM pg_proc function
    WHERE function.pronamespace = current_schema()::REGNAMESPACE
      AND function.proowner = 'go_api_runtime'::REGROLE
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'RAG_GO_API_RUNTIME_MUST_NOT_OWN_SCHEMA_OBJECTS';
  END IF;
END
$roles$;

LOCK TABLE processor_governance_profiles,
  processor_governance_heads,
  processing_consents,
  knowledge_processing_jobs,
  knowledge_outbox IN ACCESS EXCLUSIVE MODE;

DO $outbox_preflight$
BEGIN
  IF EXISTS (SELECT 1 FROM knowledge_outbox WHERE status = 'processing') THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_OUTBOX_PROCESSING_BASELINE_UNSAFE';
  END IF;
  IF EXISTS (SELECT 1 FROM knowledge_processing_jobs WHERE status = 'processing') THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_JOB_PROCESSING_BASELINE_UNSAFE';
  END IF;
END
$outbox_preflight$;

CREATE TEMP TABLE phase15_profile_mapping (
  profile_id UUID PRIMARY KEY,
  model_id TEXT NOT NULL CHECK (
    length(trim(model_id)) > 0 AND model_id = trim(model_id)
  ),
  profile_contract_hash TEXT NOT NULL CHECK (
    profile_contract_hash ~ '^[0-9a-f]{64}$'
  )
) ON COMMIT DROP;

CREATE TEMP TABLE phase15_head_mapping (
  processor TEXT NOT NULL CHECK (
    length(trim(processor)) > 0 AND processor = trim(processor)
  ),
  endpoint_id TEXT NOT NULL CHECK (
    length(trim(endpoint_id)) > 0 AND endpoint_id = trim(endpoint_id)
  ),
  model_id TEXT NOT NULL CHECK (
    length(trim(model_id)) > 0 AND model_id = trim(model_id)
  ),
  PRIMARY KEY (processor, endpoint_id)
) ON COMMIT DROP;

DO $mapping_shape$
DECLARE
  mapping JSONB;
BEGIN
  BEGIN
    mapping := current_setting('mm_chat.phase15_governance_map', true)::JSONB;
  EXCEPTION WHEN OTHERS THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_GOVERNANCE_MAPPING_INVALID';
  END;
  IF mapping IS NULL
    OR jsonb_typeof(mapping) <> 'object'
    OR NOT (mapping ? 'profiles')
    OR NOT (mapping ? 'heads')
    OR (SELECT count(*) FROM jsonb_object_keys(mapping)) <> 2
    OR jsonb_typeof(mapping->'profiles') <> 'array'
    OR jsonb_typeof(mapping->'heads') <> 'array'
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_GOVERNANCE_MAPPING_INVALID';
  END IF;
END
$mapping_shape$;

INSERT INTO phase15_profile_mapping (
  profile_id, model_id, profile_contract_hash
)
SELECT "profileId", "modelId", "profileContractHash"
FROM jsonb_to_recordset(
  current_setting('mm_chat.phase15_governance_map')::JSONB->'profiles'
) AS item(
  "profileId" UUID,
  "modelId" TEXT,
  "profileContractHash" TEXT
);

INSERT INTO phase15_head_mapping (processor, endpoint_id, model_id)
SELECT processor, "endpointId", "modelId"
FROM jsonb_to_recordset(
  current_setting('mm_chat.phase15_governance_map')::JSONB->'heads'
) AS item(processor TEXT, "endpointId" TEXT, "modelId" TEXT);

DO $mapping_coverage$
BEGIN
  IF EXISTS (
    SELECT id FROM processor_governance_profiles
    EXCEPT SELECT profile_id FROM phase15_profile_mapping
  ) OR EXISTS (
    SELECT profile_id FROM phase15_profile_mapping
    EXCEPT SELECT id FROM processor_governance_profiles
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_GOVERNANCE_PROFILE_MAPPING_COVERAGE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT processor, endpoint_id FROM processor_governance_heads
    EXCEPT SELECT processor, endpoint_id FROM phase15_head_mapping
  ) OR EXISTS (
    SELECT processor, endpoint_id FROM phase15_head_mapping
    EXCEPT SELECT processor, endpoint_id FROM processor_governance_heads
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_GOVERNANCE_HEAD_MAPPING_COVERAGE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM processor_governance_heads h
    JOIN phase15_head_mapping hm
      ON hm.processor = h.processor AND hm.endpoint_id = h.endpoint_id
    JOIN phase15_profile_mapping pm ON pm.profile_id = h.active_profile_id
    WHERE h.active_profile_id IS NOT NULL AND hm.model_id <> pm.model_id
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_GOVERNANCE_HEAD_PROFILE_MODEL_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM processor_governance_profiles profile
    JOIN phase15_profile_mapping mapping ON mapping.profile_id = profile.id
    WHERE mapping.profile_contract_hash <> encode(
      sha256(convert_to(
        '{"contractVersion":1,"processor":' || to_json(profile.processor)::TEXT ||
        ',"endpointId":' || to_json(profile.endpoint_id)::TEXT ||
        ',"modelId":' || replace(replace(replace(
          to_json(mapping.model_id)::TEXT,
          '&', '\u0026'
        ), '<', '\u003c'), '>', '\u003e') ||
        ',"modelApiVersion":' || to_json(profile.model_api_version)::TEXT ||
        ',"allowedPurposes":' || array_to_json(profile.allowed_purposes)::TEXT ||
        ',"allowedDataTypes":' || replace(replace(replace(
          array_to_json(profile.allowed_data_types)::TEXT,
          '&', '\u0026'
        ), '<', '\u003c'), '>', '\u003e') ||
        ',"region":' || to_json(profile.region)::TEXT ||
        ',"retentionPolicy":' || to_json(profile.retention_policy)::TEXT ||
        ',"deletionContract":' || to_json(profile.deletion_contract)::TEXT ||
        ',"trainingUse":' || to_json(profile.training_use)::TEXT ||
        ',"governanceRevision":' || profile.governance_revision::TEXT ||
        ',"manifestHash":' || to_json(profile.manifest_hash)::TEXT || '}',
        'UTF8'
      )),
      'hex'
    )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_GOVERNANCE_PROFILE_HASH_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM processing_consents consent
    JOIN phase15_profile_mapping profile_mapping
      ON profile_mapping.profile_id = consent.governance_profile_id
    JOIN phase15_head_mapping head_mapping
      ON head_mapping.processor = consent.processor
      AND head_mapping.endpoint_id = consent.endpoint_id
    WHERE profile_mapping.model_id <> head_mapping.model_id
  ) OR EXISTS (
    SELECT 1
    FROM knowledge_processing_jobs job
    JOIN phase15_profile_mapping profile_mapping
      ON profile_mapping.profile_id = job.governance_profile_id
    JOIN phase15_head_mapping head_mapping
      ON head_mapping.processor = job.processor
      AND head_mapping.endpoint_id = job.endpoint_id
    WHERE profile_mapping.model_id <> head_mapping.model_id
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_GOVERNANCE_AUTHORITY_MODEL_MISMATCH';
  END IF;
END
$mapping_coverage$;

CREATE TABLE phase15_rag_projection_migration_baseline (
  entity_kind TEXT NOT NULL,
  entity_key TEXT NOT NULL,
  PRIMARY KEY (entity_kind, entity_key),
  CHECK (entity_kind IN ('profile', 'head', 'consent', 'job')),
  CHECK (length(entity_key) > 0)
);

INSERT INTO phase15_rag_projection_migration_baseline
SELECT 'profile', id::TEXT FROM processor_governance_profiles
UNION ALL
SELECT 'head', processor || chr(31) || endpoint_id || chr(31) || hm.model_id
FROM processor_governance_heads h
JOIN phase15_head_mapping hm USING (processor, endpoint_id)
UNION ALL
SELECT 'consent', id::TEXT FROM processing_consents
UNION ALL
SELECT 'job', id::TEXT FROM knowledge_processing_jobs;

ALTER TABLE processor_governance_profiles
  ADD COLUMN model_id TEXT,
  ADD COLUMN profile_contract_hash TEXT;

ALTER TABLE processor_governance_profiles
  DISABLE TRIGGER processor_governance_profiles_immutable;

UPDATE processor_governance_profiles p
SET model_id = m.model_id,
    profile_contract_hash = m.profile_contract_hash
FROM phase15_profile_mapping m
WHERE m.profile_id = p.id;

ALTER TABLE processor_governance_profiles
  ENABLE TRIGGER processor_governance_profiles_immutable;

DO $trigger_restored$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_trigger
    WHERE tgrelid = 'processor_governance_profiles'::regclass
      AND tgname = 'processor_governance_profiles_immutable'
      AND tgenabled <> 'D'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_GOVERNANCE_IMMUTABLE_TRIGGER_NOT_RESTORED';
  END IF;
END
$trigger_restored$;

ALTER TABLE processor_governance_heads ADD COLUMN model_id TEXT;
UPDATE processor_governance_heads h
SET model_id = m.model_id
FROM phase15_head_mapping m
WHERE m.processor = h.processor AND m.endpoint_id = h.endpoint_id;

ALTER TABLE processing_consents ADD COLUMN model_id TEXT;
UPDATE processing_consents c
SET model_id = m.model_id
FROM phase15_profile_mapping m
WHERE m.profile_id = c.governance_profile_id;

ALTER TABLE knowledge_processing_jobs
  ADD COLUMN lease_token UUID,
  ADD COLUMN index_generation_id UUID,
  ADD COLUMN materialization_id UUID,
  ADD COLUMN model_id TEXT,
  ADD COLUMN legacy_projection_unbound BOOLEAN NOT NULL DEFAULT false;

UPDATE knowledge_processing_jobs j
SET model_id = m.model_id,
    legacy_projection_unbound = true
FROM phase15_profile_mapping m
WHERE m.profile_id = j.governance_profile_id;
UPDATE knowledge_processing_jobs
SET legacy_projection_unbound = true
WHERE governance_profile_id IS NULL;

ALTER TABLE processing_consents
  DROP CONSTRAINT processing_consents_governance_profile_fk,
  DROP CONSTRAINT processing_consents_governance_head_fk;
ALTER TABLE knowledge_processing_jobs
  DROP CONSTRAINT knowledge_processing_jobs_collection_consent_fk,
  DROP CONSTRAINT knowledge_processing_jobs_governance_profile_fk,
  DROP CONSTRAINT knowledge_processing_jobs_governance_head_fk,
  DROP CONSTRAINT knowledge_processing_jobs_authority_shape_check,
  DROP CONSTRAINT knowledge_processing_jobs_state_shape_check;
ALTER TABLE processing_consents
  DROP CONSTRAINT processing_consents_collection_job_binding_unique;
ALTER TABLE processor_governance_heads
  DROP CONSTRAINT processor_governance_heads_active_profile_fk,
  DROP CONSTRAINT processor_governance_heads_pkey;
ALTER TABLE processor_governance_profiles
  DROP CONSTRAINT processor_governance_profiles_processor_revision_unique,
  DROP CONSTRAINT processor_governance_profiles_binding_unique,
  DROP CONSTRAINT processor_governance_profiles_active_binding_unique;

ALTER TABLE processor_governance_profiles
  ALTER COLUMN model_id SET NOT NULL,
  ALTER COLUMN profile_contract_hash SET NOT NULL,
  ADD CONSTRAINT processor_governance_profiles_model_not_blank
    CHECK (length(trim(model_id)) > 0 AND model_id = trim(model_id)),
  ADD CONSTRAINT processor_governance_profiles_contract_hash_check
    CHECK (profile_contract_hash ~ '^[0-9a-f]{64}$'),
  ADD CONSTRAINT processor_governance_profiles_model_revision_unique
    UNIQUE (processor, endpoint_id, model_id, governance_revision),
  ADD CONSTRAINT processor_governance_profiles_model_binding_unique
    UNIQUE (processor, endpoint_id, model_id, id, governance_revision),
  ADD CONSTRAINT processor_governance_profiles_model_active_binding_unique
    UNIQUE (processor, endpoint_id, model_id, id, governance_revision, status),
  ADD CONSTRAINT processor_governance_profiles_id_model_hash_unique
    UNIQUE (id, model_id, profile_contract_hash);

ALTER TABLE processor_governance_heads
  ALTER COLUMN model_id SET NOT NULL,
  ADD CONSTRAINT processor_governance_heads_model_not_blank
    CHECK (length(trim(model_id)) > 0 AND model_id = trim(model_id)),
  ADD CONSTRAINT processor_governance_heads_pkey
    PRIMARY KEY (processor, endpoint_id, model_id),
  ADD CONSTRAINT processor_governance_heads_active_profile_model_fk
    FOREIGN KEY (
      processor, endpoint_id, model_id, active_profile_id,
      active_governance_revision, active_profile_status
    ) REFERENCES processor_governance_profiles(
      processor, endpoint_id, model_id, id, governance_revision, status
    ) ON DELETE RESTRICT;

ALTER TABLE processing_consents
  ALTER COLUMN model_id SET NOT NULL,
  ADD CONSTRAINT processing_consents_model_not_blank
    CHECK (length(trim(model_id)) > 0 AND model_id = trim(model_id)),
  ADD CONSTRAINT processing_consents_governance_profile_model_fk
    FOREIGN KEY (
      processor, endpoint_id, model_id, governance_profile_id,
      governance_revision
    ) REFERENCES processor_governance_profiles(
      processor, endpoint_id, model_id, id, governance_revision
    ) ON DELETE RESTRICT,
  ADD CONSTRAINT processing_consents_governance_head_model_fk
    FOREIGN KEY (processor, endpoint_id, model_id)
    REFERENCES processor_governance_heads(processor, endpoint_id, model_id)
    ON DELETE RESTRICT,
  ADD CONSTRAINT processing_consents_collection_job_model_binding_unique UNIQUE (
    collection_id, id, processor, endpoint_id, model_id,
    governance_profile_id, governance_revision,
    governance_head_revision, consent_revision
  );

CREATE UNIQUE INDEX idx_processing_consents_current_collection_endpoint_model
  ON processing_consents(collection_id, processor, endpoint_id, model_id)
  WHERE scope = 'collection' AND superseded_at IS NULL;
CREATE UNIQUE INDEX idx_processing_consents_current_query_endpoint_model
  ON processing_consents(user_id, processor, endpoint_id, model_id)
  WHERE scope = 'query' AND superseded_at IS NULL;
CREATE UNIQUE INDEX idx_processing_consents_collection_endpoint_model_revision
  ON processing_consents(
    collection_id, processor, endpoint_id, model_id, consent_revision
  ) WHERE scope = 'collection';
CREATE UNIQUE INDEX idx_processing_consents_query_endpoint_model_revision
  ON processing_consents(
    user_id, processor, endpoint_id, model_id, consent_revision
  ) WHERE scope = 'query';
DROP INDEX idx_processing_consents_current_collection_processor;
DROP INDEX idx_processing_consents_current_query_processor;
DROP INDEX idx_processing_consents_collection_revision;
DROP INDEX idx_processing_consents_query_revision;

ALTER TABLE knowledge_outbox
  DROP CONSTRAINT knowledge_outbox_status_timestamps_check,
  ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 8,
  ADD COLUMN lock_owner UUID,
  ADD COLUMN lock_token UUID,
  ADD COLUMN lock_expires_at TIMESTAMPTZ,
  ADD COLUMN error_code TEXT,
  ADD COLUMN failed_at TIMESTAMPTZ,
  ADD CONSTRAINT knowledge_outbox_id_event_unique UNIQUE (id, event_id),
  ADD CONSTRAINT knowledge_outbox_attempt_bound_check CHECK (
    max_attempts BETWEEN 1 AND 32
    AND attempt_count BETWEEN 0 AND max_attempts
  ),
  ADD CONSTRAINT knowledge_outbox_error_code_check CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  ADD CONSTRAINT knowledge_outbox_lease_shape_check CHECK (
    (
      status = 'processing'
      AND locked_at IS NOT NULL
      AND lock_owner IS NOT NULL
      AND lock_token IS NOT NULL
      AND lock_expires_at > locked_at
      AND published_at IS NULL
      AND failed_at IS NULL
      AND error_code IS NULL
    ) OR (
      status = 'pending'
      AND locked_at IS NULL
      AND lock_owner IS NULL
      AND lock_token IS NULL
      AND lock_expires_at IS NULL
      AND published_at IS NULL
      AND failed_at IS NULL
      AND error_code IS NULL
      AND attempt_count < max_attempts
    ) OR (
      status = 'published'
      AND locked_at IS NULL
      AND lock_owner IS NULL
      AND lock_token IS NULL
      AND lock_expires_at IS NULL
      AND published_at IS NOT NULL
      AND failed_at IS NULL
      AND error_code IS NULL
    ) OR (
      status = 'failed'
      AND locked_at IS NULL
      AND lock_owner IS NULL
      AND lock_token IS NULL
      AND lock_expires_at IS NULL
      AND published_at IS NULL
      AND failed_at IS NOT NULL
      AND error_code IS NOT NULL
    )
  );

ALTER TABLE knowledge_processing_jobs
  ADD CONSTRAINT knowledge_processing_jobs_model_not_blank CHECK (
    model_id IS NULL OR (length(trim(model_id)) > 0 AND model_id = trim(model_id))
  ),
  ADD CONSTRAINT knowledge_processing_jobs_governance_profile_model_fk
    FOREIGN KEY (
      processor, endpoint_id, model_id, governance_profile_id,
      governance_revision
    ) REFERENCES processor_governance_profiles(
      processor, endpoint_id, model_id, id, governance_revision
    ) ON DELETE RESTRICT,
  ADD CONSTRAINT knowledge_processing_jobs_governance_head_model_fk
    FOREIGN KEY (processor, endpoint_id, model_id)
    REFERENCES processor_governance_heads(processor, endpoint_id, model_id)
    ON DELETE RESTRICT,
  ADD CONSTRAINT knowledge_processing_jobs_collection_consent_model_fk
    FOREIGN KEY (
      collection_id, collection_consent_id, processor, endpoint_id, model_id,
      governance_profile_id, governance_revision,
      governance_head_revision, collection_consent_revision
    ) REFERENCES processing_consents(
      collection_id, id, processor, endpoint_id, model_id,
      governance_profile_id, governance_revision,
      governance_head_revision, consent_revision
    ) ON DELETE RESTRICT,
  ADD CONSTRAINT knowledge_processing_jobs_authority_shape_check CHECK (
    (
      stage IN ('parse', 'passage_embedding')
      AND operation IN ('initial', 'replace', 'reprocess')
      AND processor IS NOT NULL AND endpoint_id IS NOT NULL
      AND model_id IS NOT NULL AND governance_profile_id IS NOT NULL
      AND governance_revision IS NOT NULL
      AND governance_head_revision IS NOT NULL
      AND collection_consent_id IS NOT NULL
      AND collection_consent_revision IS NOT NULL
    ) OR (
      stage = 'purge' AND operation = 'purge'
      AND processor IS NULL AND endpoint_id IS NULL AND model_id IS NULL
      AND governance_profile_id IS NULL AND governance_revision IS NULL
      AND governance_head_revision IS NULL
      AND collection_consent_id IS NULL
      AND collection_consent_revision IS NULL
    )
  ),
  ADD CONSTRAINT knowledge_processing_jobs_projection_binding_check CHECK (
    legacy_projection_unbound
    OR (
      index_generation_id IS NOT NULL
      AND (
        (stage IN ('parse', 'passage_embedding') AND materialization_id IS NOT NULL)
        OR stage = 'purge'
      )
    )
  ),
  ADD CONSTRAINT knowledge_processing_jobs_lease_token_shape_check CHECK (
    (
      status = 'processing'
      AND lease_owner IS NOT NULL AND lease_token IS NOT NULL
      AND lease_expires_at IS NOT NULL AND completed_at IS NULL
      AND error_code IS NULL AND attempt_count BETWEEN 1 AND max_attempts
    ) OR (
      status = 'pending'
      AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NULL
      AND error_code IS NULL AND attempt_count < max_attempts
    ) OR (
      status = 'succeeded'
      AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NULL AND attempt_count BETWEEN 1 AND max_attempts
    ) OR (
      status = 'failed'
      AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NOT NULL AND attempt_count BETWEEN 1 AND max_attempts
    ) OR (
      status = 'cancelled'
      AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
    )
  );

CREATE FUNCTION knowledge_reject_immutable_projection_mutation()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'RAG_IMMUTABLE_PROJECTION_ROW';
END
$function$;

CREATE TABLE knowledge_index_profiles (
  id UUID PRIMARY KEY,
  contract_version SMALLINT NOT NULL CHECK (contract_version >= 1),
  canonical_schema_version TEXT NOT NULL CHECK (
    length(trim(canonical_schema_version)) > 0
  ),
  parser_manifest JSONB NOT NULL CHECK (jsonb_typeof(parser_manifest) = 'object'),
  parser_manifest_hash TEXT NOT NULL CHECK (
    parser_manifest_hash ~ '^[0-9a-f]{64}$'
  ),
  chunk_manifest JSONB NOT NULL CHECK (jsonb_typeof(chunk_manifest) = 'object'),
  chunk_profile_hash TEXT NOT NULL CHECK (
    chunk_profile_hash ~ '^[0-9a-f]{64}$'
  ),
  embedding_processor TEXT NOT NULL CHECK (length(trim(embedding_processor)) > 0),
  embedding_endpoint_id TEXT NOT NULL CHECK (length(trim(embedding_endpoint_id)) > 0),
  embedding_model_id TEXT NOT NULL CHECK (length(trim(embedding_model_id)) > 0),
  embedding_api_version TEXT NOT NULL CHECK (length(trim(embedding_api_version)) > 0),
  embedding_role TEXT NOT NULL CHECK (length(trim(embedding_role)) > 0),
  rerank_processor TEXT NOT NULL CHECK (length(trim(rerank_processor)) > 0),
  rerank_endpoint_id TEXT NOT NULL CHECK (length(trim(rerank_endpoint_id)) > 0),
  rerank_model_id TEXT NOT NULL CHECK (length(trim(rerank_model_id)) > 0),
  rerank_api_version TEXT NOT NULL CHECK (length(trim(rerank_api_version)) > 0),
  base_profile_hash TEXT NOT NULL UNIQUE CHECK (
    base_profile_hash ~ '^[0-9a-f]{64}$'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER knowledge_index_profiles_immutable
BEFORE UPDATE OR DELETE ON knowledge_index_profiles
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE TABLE knowledge_index_generations (
  id UUID PRIMARY KEY,
  index_profile_id UUID NOT NULL
    REFERENCES knowledge_index_profiles(id) ON DELETE RESTRICT,
  generation_seq BIGINT NOT NULL UNIQUE CHECK (generation_seq >= 1),
  status TEXT NOT NULL CHECK (
    status IN ('building', 'verified', 'active', 'retired', 'failed')
  ),
  build_snapshot JSONB NOT NULL CHECK (jsonb_typeof(build_snapshot) = 'object'),
  build_snapshot_hash TEXT NOT NULL CHECK (
    build_snapshot_hash ~ '^[0-9a-f]{64}$'
  ),
  artifact_manifest_hash TEXT CHECK (
    artifact_manifest_hash IS NULL OR artifact_manifest_hash ~ '^[0-9a-f]{64}$'
  ),
  failure_code TEXT CHECK (
    failure_code IS NULL OR failure_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  verified_at TIMESTAMPTZ,
  activated_at TIMESTAMPTZ,
  retired_at TIMESTAMPTZ,
  failed_at TIMESTAMPTZ,
  UNIQUE (id, status),
  CONSTRAINT knowledge_index_generations_state_shape CHECK (
    (status = 'building' AND verified_at IS NULL AND activated_at IS NULL
      AND retired_at IS NULL AND failed_at IS NULL AND failure_code IS NULL)
    OR (status = 'verified' AND verified_at IS NOT NULL AND activated_at IS NULL
      AND retired_at IS NULL AND failed_at IS NULL AND failure_code IS NULL)
    OR (status = 'active' AND verified_at IS NOT NULL AND activated_at IS NOT NULL
      AND retired_at IS NULL AND failed_at IS NULL AND failure_code IS NULL)
    OR (status = 'retired' AND verified_at IS NOT NULL AND activated_at IS NOT NULL
      AND retired_at IS NOT NULL AND failed_at IS NULL AND failure_code IS NULL)
    OR (status = 'failed' AND activated_at IS NULL AND retired_at IS NULL
      AND failed_at IS NOT NULL AND failure_code IS NOT NULL)
  )
);
CREATE UNIQUE INDEX idx_knowledge_index_generations_one_active
  ON knowledge_index_generations((1)) WHERE status = 'active';
CREATE UNIQUE INDEX idx_knowledge_index_generations_one_candidate
  ON knowledge_index_generations((1)) WHERE status IN ('building', 'verified');

CREATE TABLE knowledge_corpus_projection_head (
  singleton_id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (singleton_id = 1),
  active_index_generation_id UUID
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  active_index_generation_status TEXT GENERATED ALWAYS AS (
    CASE WHEN active_index_generation_id IS NOT NULL THEN 'active' ELSE NULL END
  ) STORED,
  corpus_projection_revision BIGINT NOT NULL DEFAULT 1 CHECK (
    corpus_projection_revision >= 1
  ),
  head_revision BIGINT NOT NULL DEFAULT 1 CHECK (head_revision >= 1),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (active_index_generation_id, active_index_generation_status)
    REFERENCES knowledge_index_generations(id, status) ON DELETE RESTRICT
    DEFERRABLE INITIALLY DEFERRED
);
INSERT INTO knowledge_corpus_projection_head(singleton_id) VALUES (1);

CREATE TABLE knowledge_projection_state (
  index_generation_id UUID PRIMARY KEY
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  readiness TEXT NOT NULL CHECK (
    readiness IN ('building', 'catching_up', 'ready', 'degraded', 'retired', 'failed')
  ),
  projection_revision BIGINT NOT NULL CHECK (projection_revision >= 1),
  required_outbox_floor BIGINT NOT NULL CHECK (required_outbox_floor >= 0),
  contiguous_applied_outbox_id BIGINT NOT NULL CHECK (
    contiguous_applied_outbox_id >= 0
    AND contiguous_applied_outbox_id <= required_outbox_floor
  ),
  manifest_hash TEXT CHECK (
    manifest_hash IS NULL OR manifest_hash ~ '^[0-9a-f]{64}$'
  ),
  document_count BIGINT NOT NULL DEFAULT 0 CHECK (document_count >= 0),
  parent_count BIGINT NOT NULL DEFAULT 0 CHECK (parent_count >= 0),
  child_count BIGINT NOT NULL DEFAULT 0 CHECK (child_count >= 0),
  verified_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((readiness = 'ready' AND verified_at IS NOT NULL) OR readiness <> 'ready')
);

CREATE TABLE knowledge_document_materializations (
  id UUID PRIMARY KEY,
  index_generation_id UUID NOT NULL
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  collection_id UUID NOT NULL,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  file_id UUID NOT NULL REFERENCES files(id) ON DELETE RESTRICT,
  materialization_seq BIGINT NOT NULL CHECK (materialization_seq >= 1),
  parse_artifact_set_id UUID,
  source_content_hash TEXT NOT NULL CHECK (
    source_content_hash ~ '^[0-9a-f]{64}$'
  ),
  base_profile_hash TEXT NOT NULL CHECK (base_profile_hash ~ '^[0-9a-f]{64}$'),
  collection_acl_revision BIGINT NOT NULL CHECK (collection_acl_revision >= 1),
  collection_visibility_epoch BIGINT NOT NULL CHECK (collection_visibility_epoch >= 1),
  collection_processing_revision BIGINT NOT NULL CHECK (
    collection_processing_revision >= 1
  ),
  document_visibility_epoch BIGINT NOT NULL CHECK (document_visibility_epoch >= 1),
  status TEXT NOT NULL CHECK (
    status IN ('staging', 'verified', 'published', 'retired', 'failed', 'purging', 'purged')
  ),
  manifest_hash TEXT CHECK (
    manifest_hash IS NULL OR manifest_hash ~ '^[0-9a-f]{64}$'
  ),
  result_hash TEXT CHECK (
    result_hash IS NULL OR result_hash ~ '^[0-9a-f]{64}$'
  ),
  failure_code TEXT CHECK (
    failure_code IS NULL OR failure_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  verified_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  retired_at TIMESTAMPTZ,
  purged_at TIMESTAMPTZ,
  UNIQUE (index_generation_id, document_id, materialization_seq),
  UNIQUE (index_generation_id, document_id, id),
  UNIQUE (index_generation_id, document_id, id, status),
  UNIQUE (id, index_generation_id, document_id, document_version_id),
  FOREIGN KEY (collection_id, document_id)
    REFERENCES knowledge_documents(collection_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (document_id, document_version_id, file_id)
    REFERENCES knowledge_document_versions(document_id, id, file_id)
    ON DELETE RESTRICT,
  CONSTRAINT knowledge_document_materializations_state_shape CHECK (
    (status = 'staging' AND verified_at IS NULL AND published_at IS NULL
      AND retired_at IS NULL AND purged_at IS NULL AND failure_code IS NULL)
    OR (status = 'verified' AND verified_at IS NOT NULL AND published_at IS NULL
      AND retired_at IS NULL AND purged_at IS NULL AND failure_code IS NULL)
    OR (status = 'published' AND verified_at IS NOT NULL AND published_at IS NOT NULL
      AND retired_at IS NULL AND purged_at IS NULL AND failure_code IS NULL)
    OR (status = 'retired' AND retired_at IS NOT NULL AND purged_at IS NULL)
    OR (status = 'failed' AND failure_code IS NOT NULL)
    OR (status = 'purging' AND retired_at IS NOT NULL AND purged_at IS NULL)
    OR (status = 'purged' AND retired_at IS NOT NULL AND purged_at IS NOT NULL)
  )
);

CREATE TABLE knowledge_document_projection_heads (
  index_generation_id UUID NOT NULL,
  document_id UUID NOT NULL,
  active_materialization_id UUID,
  active_materialization_status TEXT GENERATED ALWAYS AS (
    CASE WHEN active_materialization_id IS NOT NULL THEN 'published' ELSE NULL END
  ) STORED,
  document_projection_revision BIGINT NOT NULL DEFAULT 1 CHECK (
    document_projection_revision >= 1
  ),
  last_corpus_projection_revision BIGINT NOT NULL CHECK (
    last_corpus_projection_revision >= 1
  ),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (index_generation_id, document_id),
  FOREIGN KEY (index_generation_id)
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  FOREIGN KEY (
    index_generation_id, document_id, active_materialization_id,
    active_materialization_status
  )
    REFERENCES knowledge_document_materializations(
      index_generation_id, document_id, id, status
    ) ON DELETE RESTRICT
    DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE knowledge_parser_artifact_sets (
  id UUID PRIMARY KEY,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  file_id UUID NOT NULL,
  index_profile_id UUID NOT NULL
    REFERENCES knowledge_index_profiles(id) ON DELETE RESTRICT,
  parser_kind TEXT NOT NULL CHECK (length(trim(parser_kind)) > 0),
  parser_version TEXT NOT NULL CHECK (length(trim(parser_version)) > 0),
  source_content_hash TEXT NOT NULL CHECK (
    source_content_hash ~ '^[0-9a-f]{64}$'
  ),
  config_hash TEXT NOT NULL CHECK (config_hash ~ '^[0-9a-f]{64}$'),
  manifest_hash TEXT NOT NULL CHECK (manifest_hash ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK (
    status IN ('staging', 'verified', 'quarantined', 'purging', 'purged')
  ),
  quality_report JSONB NOT NULL CHECK (jsonb_typeof(quality_report) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  verified_at TIMESTAMPTZ,
  purged_at TIMESTAMPTZ,
  UNIQUE (id, document_id, document_version_id),
  FOREIGN KEY (document_id, document_version_id, file_id)
    REFERENCES knowledge_document_versions(document_id, id, file_id)
    ON DELETE RESTRICT,
  CHECK (
    (status = 'staging' AND verified_at IS NULL AND purged_at IS NULL)
    OR (status = 'verified' AND verified_at IS NOT NULL AND purged_at IS NULL)
    OR (status = 'quarantined' AND purged_at IS NULL)
    OR (status = 'purging' AND purged_at IS NULL)
    OR (status = 'purged' AND purged_at IS NOT NULL)
  )
);

ALTER TABLE knowledge_document_materializations
  ADD CONSTRAINT knowledge_document_materializations_artifact_set_fk
  FOREIGN KEY (
    parse_artifact_set_id, document_id, document_version_id
  ) REFERENCES knowledge_parser_artifact_sets(
    id, document_id, document_version_id
  ) ON DELETE RESTRICT;

CREATE TABLE knowledge_parser_artifacts (
  id UUID PRIMARY KEY,
  artifact_set_id UUID NOT NULL
    REFERENCES knowledge_parser_artifact_sets(id) ON DELETE RESTRICT,
  artifact_kind TEXT NOT NULL CHECK (
    artifact_kind IN ('parser_native', 'canonical_ir', 'quality_report', 'page_asset')
  ),
  object_key TEXT NOT NULL UNIQUE CHECK (length(trim(object_key)) > 0),
  content_type TEXT NOT NULL CHECK (length(trim(content_type)) > 0),
  byte_size BIGINT NOT NULL CHECK (byte_size >= 0),
  sha256 TEXT NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  page_or_part_ref JSONB CHECK (
    page_or_part_ref IS NULL OR jsonb_typeof(page_or_part_ref) = 'object'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (artifact_set_id, artifact_kind, object_key)
);
CREATE TRIGGER knowledge_parser_artifacts_immutable
BEFORE UPDATE OR DELETE ON knowledge_parser_artifacts
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE TABLE knowledge_blocks (
  id UUID PRIMARY KEY,
  artifact_set_id UUID NOT NULL
    REFERENCES knowledge_parser_artifact_sets(id) ON DELETE RESTRICT,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  parent_block_id UUID,
  ordinal BIGINT NOT NULL CHECK (ordinal >= 0),
  block_type TEXT NOT NULL CHECK (length(trim(block_type)) > 0),
  heading_path TEXT[] NOT NULL CHECK (
    array_ndims(heading_path) = 1 AND array_position(heading_path, NULL) IS NULL
  ),
  text_content TEXT,
  markdown_content TEXT,
  html_content TEXT,
  latex_content TEXT,
  code_content TEXT,
  table_data JSONB CHECK (table_data IS NULL OR jsonb_typeof(table_data) = 'object'),
  locator_kind TEXT NOT NULL CHECK (locator_kind IN (
    'text_offset', 'line_range', 'page_bbox', 'slide_shape', 'sheet_cell',
    'ooxml_part_xpath'
  )),
  locator JSONB NOT NULL CHECK (jsonb_typeof(locator) = 'object'),
  reading_order BIGINT NOT NULL CHECK (reading_order >= 0),
  provenance JSONB NOT NULL CHECK (jsonb_typeof(provenance) = 'object'),
  confidence NUMERIC CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  source_span_hash TEXT NOT NULL CHECK (source_span_hash ~ '^[0-9a-f]{64}$'),
  derived BOOLEAN NOT NULL DEFAULT false,
  non_indexable BOOLEAN NOT NULL DEFAULT false,
  needs_review BOOLEAN NOT NULL DEFAULT false,
  UNIQUE (artifact_set_id, ordinal),
  UNIQUE (artifact_set_id, source_span_hash),
  UNIQUE (artifact_set_id, id),
  FOREIGN KEY (artifact_set_id, document_id, document_version_id)
    REFERENCES knowledge_parser_artifact_sets(
      id, document_id, document_version_id
    ) ON DELETE RESTRICT,
  FOREIGN KEY (artifact_set_id, parent_block_id)
    REFERENCES knowledge_blocks(artifact_set_id, id) ON DELETE RESTRICT,
  CHECK (
    text_content IS NOT NULL OR markdown_content IS NOT NULL
    OR html_content IS NOT NULL OR latex_content IS NOT NULL
    OR code_content IS NOT NULL OR table_data IS NOT NULL
  ),
  CONSTRAINT knowledge_blocks_locator_shape CHECK (
    (locator->>'kind') = locator_kind
    AND (
      (locator_kind = 'text_offset'
        AND locator ?& ARRAY['kind', 'start', 'end']
        AND jsonb_typeof(locator->'start') = 'number'
        AND jsonb_typeof(locator->'end') = 'number'
        AND (locator->>'start')::BIGINT >= 0
        AND (locator->>'end')::BIGINT >= (locator->>'start')::BIGINT)
      OR (locator_kind = 'line_range'
        AND locator ?& ARRAY['kind', 'startLine', 'endLine']
        AND jsonb_typeof(locator->'startLine') = 'number'
        AND jsonb_typeof(locator->'endLine') = 'number'
        AND (locator->>'startLine')::BIGINT >= 0
        AND (locator->>'endLine')::BIGINT >= (locator->>'startLine')::BIGINT)
      OR (locator_kind = 'page_bbox'
        AND locator ?& ARRAY['kind', 'page', 'x1', 'y1', 'x2', 'y2']
        AND jsonb_typeof(locator->'page') = 'number'
        AND jsonb_typeof(locator->'x1') = 'number'
        AND jsonb_typeof(locator->'y1') = 'number'
        AND jsonb_typeof(locator->'x2') = 'number'
        AND jsonb_typeof(locator->'y2') = 'number'
        AND (locator->>'page')::BIGINT >= 0
        AND (locator->>'x1')::NUMERIC >= 0
        AND (locator->>'y1')::NUMERIC >= 0
        AND (locator->>'x2')::NUMERIC >= (locator->>'x1')::NUMERIC
        AND (locator->>'y2')::NUMERIC >= (locator->>'y1')::NUMERIC)
      OR (locator_kind = 'slide_shape'
        AND locator ?& ARRAY['kind', 'slide', 'shape']
        AND jsonb_typeof(locator->'slide') = 'number'
        AND jsonb_typeof(locator->'shape') = 'number'
        AND (locator->>'slide')::BIGINT >= 0
        AND (locator->>'shape')::BIGINT >= 0)
      OR (locator_kind = 'sheet_cell'
        AND locator ?& ARRAY['kind', 'sheet', 'startCell', 'endCell']
        AND COALESCE(length(trim(locator->>'sheet')), 0) > 0
        AND COALESCE(length(trim(locator->>'startCell')), 0) > 0
        AND COALESCE(length(trim(locator->>'endCell')), 0) > 0)
      OR (locator_kind = 'ooxml_part_xpath'
        AND locator ?& ARRAY['kind', 'part', 'xpath']
        AND COALESCE(length(trim(locator->>'part')), 0) > 0
        AND COALESCE(length(trim(locator->>'xpath')), 0) > 0)
    )
  )
);
CREATE TRIGGER knowledge_blocks_immutable
BEFORE UPDATE OR DELETE ON knowledge_blocks
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE TABLE knowledge_parent_chunks (
  id UUID PRIMARY KEY,
  materialization_id UUID NOT NULL,
  index_generation_id UUID NOT NULL,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  ordinal BIGINT NOT NULL CHECK (ordinal >= 0),
  chunk_profile_hash TEXT NOT NULL CHECK (chunk_profile_hash ~ '^[0-9a-f]{64}$'),
  source_span_hash TEXT NOT NULL CHECK (source_span_hash ~ '^[0-9a-f]{64}$'),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  content TEXT NOT NULL CHECK (length(content) > 0),
  token_count INTEGER NOT NULL CHECK (token_count > 0),
  heading_path TEXT[] NOT NULL CHECK (
    array_ndims(heading_path) = 1 AND array_position(heading_path, NULL) IS NULL
  ),
  locator_summary JSONB NOT NULL CHECK (jsonb_typeof(locator_summary) = 'object'),
  UNIQUE (materialization_id, ordinal),
  UNIQUE (
    index_generation_id, document_version_id, source_span_hash,
    chunk_profile_hash
  ),
  UNIQUE (
    id, materialization_id, index_generation_id, document_id,
    document_version_id
  ),
  FOREIGN KEY (
    materialization_id, index_generation_id, document_id, document_version_id
  ) REFERENCES knowledge_document_materializations(
    id, index_generation_id, document_id, document_version_id
  ) ON DELETE RESTRICT
);
CREATE TRIGGER knowledge_parent_chunks_immutable
BEFORE UPDATE OR DELETE ON knowledge_parent_chunks
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE TABLE knowledge_child_chunks (
  id UUID PRIMARY KEY,
  parent_chunk_id UUID NOT NULL,
  materialization_id UUID NOT NULL,
  index_generation_id UUID NOT NULL,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  ordinal BIGINT NOT NULL CHECK (ordinal >= 0),
  chunk_profile_hash TEXT NOT NULL CHECK (chunk_profile_hash ~ '^[0-9a-f]{64}$'),
  source_span_hash TEXT NOT NULL CHECK (source_span_hash ~ '^[0-9a-f]{64}$'),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  content TEXT NOT NULL CHECK (length(content) > 0),
  token_count INTEGER NOT NULL CHECK (token_count > 0),
  overlap_before_tokens INTEGER NOT NULL DEFAULT 0 CHECK (overlap_before_tokens >= 0),
  overlap_after_tokens INTEGER NOT NULL DEFAULT 0 CHECK (overlap_after_tokens >= 0),
  UNIQUE (materialization_id, ordinal),
  UNIQUE (
    index_generation_id, document_version_id, source_span_hash,
    chunk_profile_hash
  ),
  UNIQUE (id, materialization_id),
  FOREIGN KEY (
    parent_chunk_id, materialization_id, index_generation_id,
    document_id, document_version_id
  ) REFERENCES knowledge_parent_chunks(
    id, materialization_id, index_generation_id, document_id,
    document_version_id
  ) ON DELETE RESTRICT
);
CREATE TRIGGER knowledge_child_chunks_immutable
BEFORE UPDATE OR DELETE ON knowledge_child_chunks
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE TABLE knowledge_chunk_block_spans (
  chunk_kind TEXT NOT NULL CHECK (chunk_kind IN ('parent', 'child')),
  chunk_id UUID NOT NULL,
  block_id UUID NOT NULL REFERENCES knowledge_blocks(id) ON DELETE RESTRICT,
  span_ordinal INTEGER NOT NULL CHECK (span_ordinal >= 0),
  start_offset BIGINT NOT NULL CHECK (start_offset >= 0),
  end_offset BIGINT NOT NULL CHECK (end_offset > start_offset),
  PRIMARY KEY (chunk_kind, chunk_id, block_id, span_ordinal)
);

CREATE FUNCTION knowledge_validate_chunk_block_span()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NEW.chunk_kind = 'parent' AND EXISTS (
    SELECT 1
    FROM knowledge_parent_chunks chunk
    JOIN knowledge_document_materializations materialization
      ON materialization.id = chunk.materialization_id
    JOIN knowledge_blocks block
      ON block.id = NEW.block_id
      AND block.artifact_set_id = materialization.parse_artifact_set_id
    WHERE chunk.id = NEW.chunk_id
  ) THEN
    RETURN NEW;
  END IF;
  IF NEW.chunk_kind = 'child' AND EXISTS (
    SELECT 1
    FROM knowledge_child_chunks chunk
    JOIN knowledge_document_materializations materialization
      ON materialization.id = chunk.materialization_id
    JOIN knowledge_blocks block
      ON block.id = NEW.block_id
      AND block.artifact_set_id = materialization.parse_artifact_set_id
    WHERE chunk.id = NEW.chunk_id
  ) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION USING
    ERRCODE = '23503',
    MESSAGE = 'RAG_CHUNK_BLOCK_PROVENANCE_MISMATCH';
END
$function$;
CREATE TRIGGER knowledge_chunk_block_spans_validate
BEFORE INSERT OR UPDATE ON knowledge_chunk_block_spans
FOR EACH ROW EXECUTE FUNCTION knowledge_validate_chunk_block_span();

ALTER TABLE knowledge_processing_jobs
  ADD CONSTRAINT knowledge_processing_jobs_generation_fk
    FOREIGN KEY (index_generation_id)
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  ADD CONSTRAINT knowledge_processing_jobs_materialization_fk
    FOREIGN KEY (
      materialization_id, index_generation_id, document_id, document_version_id
    ) REFERENCES knowledge_document_materializations(
      id, index_generation_id, document_id, document_version_id
    ) ON DELETE RESTRICT;

CREATE TABLE knowledge_outbox_applied_events (
  consumer_name TEXT NOT NULL CHECK (
    length(trim(consumer_name)) > 0 AND consumer_name = trim(consumer_name)
  ),
  event_id UUID NOT NULL,
  scope_kind TEXT NOT NULL CHECK (scope_kind IN ('global', 'generation')),
  index_generation_id UUID
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  generation_scope_id UUID GENERATED ALWAYS AS (
    COALESCE(
      index_generation_id,
      '00000000-0000-0000-0000-000000000000'::UUID
    )
  ) STORED,
  result_hash TEXT NOT NULL CHECK (result_hash ~ '^[0-9a-f]{64}$'),
  outbox_id BIGINT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer_name, event_id, generation_scope_id),
  FOREIGN KEY (outbox_id, event_id)
    REFERENCES knowledge_outbox(id, event_id) ON DELETE RESTRICT,
  CHECK (
    (scope_kind = 'global' AND index_generation_id IS NULL)
    OR (
      scope_kind = 'generation'
      AND index_generation_id IS NOT NULL
      AND index_generation_id <> '00000000-0000-0000-0000-000000000000'::UUID
    )
  )
);
CREATE INDEX idx_knowledge_outbox_applied_events_cursor
  ON knowledge_outbox_applied_events(consumer_name, outbox_id);

CREATE TABLE knowledge_outbox_replays (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  event_id UUID NOT NULL REFERENCES knowledge_outbox(event_id) ON DELETE RESTRICT,
  expected_error_code TEXT NOT NULL CHECK (
    expected_error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  operator_id UUID NOT NULL,
  reason TEXT NOT NULL CHECK (
    octet_length(reason) BETWEEN 1 AND 1024 AND length(trim(reason)) > 0
  ),
  replayed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE knowledge_processing_job_replays (
  failed_job_id UUID NOT NULL
    REFERENCES knowledge_processing_jobs(id) ON DELETE RESTRICT,
  successor_job_id UUID NOT NULL UNIQUE
    REFERENCES knowledge_processing_jobs(id) ON DELETE RESTRICT,
  expected_error_code TEXT NOT NULL CHECK (
    expected_error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  operator_id UUID NOT NULL,
  reason TEXT NOT NULL CHECK (
    octet_length(reason) BETWEEN 1 AND 1024 AND length(trim(reason)) > 0
  ),
  replayed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (failed_job_id, successor_job_id)
);

CREATE TABLE knowledge_collection_purges (
  id UUID PRIMARY KEY,
  collection_id UUID NOT NULL
    REFERENCES knowledge_collections(id) ON DELETE RESTRICT,
  collection_visibility_epoch BIGINT NOT NULL CHECK (
    collection_visibility_epoch >= 1
  ),
  source_event_id UUID NOT NULL UNIQUE
    REFERENCES knowledge_outbox(event_id) ON DELETE RESTRICT,
  cursor_document_id UUID,
  cursor_document_version_id UUID,
  cursor_index_generation_id UUID,
  cursor_materialization_id UUID,
  enumeration_complete BOOLEAN NOT NULL DEFAULT false,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (
    status IN ('pending', 'processing', 'succeeded', 'failed')
  ),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8,
  lease_owner UUID,
  lease_token UUID,
  lease_expires_at TIMESTAMPTZ,
  remaining_count_snapshot BIGINT NOT NULL DEFAULT 0 CHECK (
    remaining_count_snapshot >= 0
  ),
  error_code TEXT CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (collection_id, collection_visibility_epoch),
  UNIQUE (id, collection_id, collection_visibility_epoch),
  CHECK (
    attempt_count BETWEEN 0 AND max_attempts AND max_attempts BETWEEN 1 AND 32
  ),
  CHECK (
    (
      cursor_document_id IS NULL
      AND cursor_document_version_id IS NULL
      AND cursor_index_generation_id IS NULL
      AND cursor_materialization_id IS NULL
    ) OR (
      cursor_document_id IS NOT NULL
      AND cursor_document_version_id IS NOT NULL
      AND cursor_index_generation_id IS NOT NULL
      AND cursor_materialization_id IS NOT NULL
    )
  ),
  CHECK (
    (status = 'pending' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NULL
      AND error_code IS NULL AND attempt_count < max_attempts)
    OR (status = 'processing' AND attempt_count >= 1 AND lease_owner IS NOT NULL
      AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL
      AND completed_at IS NULL AND error_code IS NULL)
    OR (status = 'succeeded' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NULL AND enumeration_complete)
    OR (status = 'failed' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NOT NULL)
  )
);
CREATE INDEX idx_knowledge_collection_purges_claim
  ON knowledge_collection_purges(status, lease_expires_at, created_at);

CREATE TABLE knowledge_collection_purge_items (
  id UUID PRIMARY KEY,
  purge_id UUID NOT NULL,
  collection_id UUID NOT NULL,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  collection_visibility_epoch BIGINT NOT NULL CHECK (
    collection_visibility_epoch >= 1
  ),
  index_generation_id UUID NOT NULL
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  materialization_id UUID,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (
    status IN ('pending', 'processing', 'succeeded', 'failed')
  ),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8,
  lease_owner UUID,
  lease_token UUID,
  lease_expires_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_code TEXT CHECK (
    error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (
    collection_id, collection_visibility_epoch, document_version_id,
    index_generation_id
  ),
  FOREIGN KEY (collection_id, document_id)
    REFERENCES knowledge_documents(collection_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (document_id, document_version_id)
    REFERENCES knowledge_document_versions(document_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (purge_id, collection_id, collection_visibility_epoch)
    REFERENCES knowledge_collection_purges(
      id, collection_id, collection_visibility_epoch
    ) ON DELETE RESTRICT,
  FOREIGN KEY (index_generation_id, document_id, materialization_id)
    REFERENCES knowledge_document_materializations(
      index_generation_id, document_id, id
    ) ON DELETE RESTRICT,
  CHECK (
    attempt_count BETWEEN 0 AND max_attempts AND max_attempts BETWEEN 1 AND 32
  ),
  CHECK (
    (status = 'pending' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NULL
      AND error_code IS NULL AND attempt_count < max_attempts)
    OR (status = 'processing' AND attempt_count >= 1 AND lease_owner IS NOT NULL
      AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL
      AND completed_at IS NULL AND error_code IS NULL)
    OR (status = 'succeeded' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NULL)
    OR (status = 'failed' AND lease_owner IS NULL AND lease_token IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NOT NULL)
  )
);
CREATE INDEX idx_knowledge_collection_purge_items_claim
  ON knowledge_collection_purge_items(status, lease_expires_at, created_at);

CREATE FUNCTION knowledge_claim_collection_purge(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER
) RETURNS SETOF knowledge_collection_purges
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_lease_seconds IS NULL OR p_lease_seconds NOT BETWEEN 1 AND 3600
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PURGE_ROOT_CLAIM_ARGUMENT_INVALID';
  END IF;

  UPDATE knowledge_collection_purges
  SET status = 'failed', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = clock_timestamp(),
      error_code = 'MAX_ATTEMPTS_EXCEEDED', updated_at = clock_timestamp()
  WHERE status = 'processing'
    AND lease_expires_at <= clock_timestamp()
    AND attempt_count >= max_attempts;

  RETURN QUERY
  WITH candidate AS (
    SELECT id
    FROM knowledge_collection_purges
    WHERE NOT enumeration_complete
      AND attempt_count < max_attempts
      AND (
        status = 'pending'
        OR (status = 'processing' AND lease_expires_at <= clock_timestamp())
      )
    ORDER BY created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
  )
  UPDATE knowledge_collection_purges purge
  SET status = 'processing', attempt_count = purge.attempt_count + 1,
      lease_owner = p_worker_id, lease_token = p_lease_token,
      lease_expires_at = clock_timestamp()
        + make_interval(secs => p_lease_seconds),
      completed_at = NULL, error_code = NULL, updated_at = clock_timestamp()
  FROM candidate
  WHERE purge.id = candidate.id
  RETURNING purge.*;
END
$function$;

CREATE FUNCTION knowledge_enumerate_collection_purge(
  p_purge_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER,
  p_batch_size INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  root knowledge_collection_purges%ROWTYPE;
  candidate knowledge_document_materializations%ROWTYPE;
  last_document_id UUID;
  last_document_version_id UUID;
  last_generation_id UUID;
  last_materialization_id UUID;
  processed INTEGER := 0;
  has_more BOOLEAN;
BEGIN
  IF p_lease_seconds IS NULL OR p_lease_seconds NOT BETWEEN 1 AND 3600
    OR p_batch_size IS NULL OR p_batch_size NOT BETWEEN 1 AND 1000
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PURGE_ENUMERATION_ARGUMENT_INVALID';
  END IF;

  SELECT * INTO root
  FROM knowledge_collection_purges
  WHERE id = p_purge_id AND status = 'processing'
    AND NOT enumeration_complete
    AND lease_owner = p_worker_id AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp()
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_STALE_PURGE_ROOT_LEASE';
  END IF;

  FOR candidate IN
    SELECT materialization.*
    FROM (
      SELECT DISTINCT ON (
        candidate_materialization.document_id,
        candidate_materialization.document_version_id,
        candidate_materialization.index_generation_id
      ) candidate_materialization.*
      FROM knowledge_document_materializations candidate_materialization
      LEFT JOIN knowledge_document_projection_heads projection_head
        ON projection_head.index_generation_id =
          candidate_materialization.index_generation_id
        AND projection_head.document_id = candidate_materialization.document_id
      WHERE candidate_materialization.collection_id = root.collection_id
        AND candidate_materialization.status <> 'purged'
      ORDER BY candidate_materialization.document_id,
        candidate_materialization.document_version_id,
        candidate_materialization.index_generation_id,
        (
          projection_head.active_materialization_id =
            candidate_materialization.id
        ) DESC NULLS LAST,
        candidate_materialization.materialization_seq DESC,
        candidate_materialization.id DESC
    ) materialization
    WHERE (
        root.cursor_document_id IS NULL
        OR (
          materialization.document_id,
          materialization.document_version_id,
          materialization.index_generation_id,
          materialization.id
        ) > (
          root.cursor_document_id,
          root.cursor_document_version_id,
          root.cursor_index_generation_id,
          root.cursor_materialization_id
        )
      )
    ORDER BY materialization.document_id,
      materialization.document_version_id,
      materialization.index_generation_id,
      materialization.id
    LIMIT p_batch_size
  LOOP
    INSERT INTO knowledge_collection_purge_items(
      id, purge_id, collection_id, document_id, document_version_id,
      collection_visibility_epoch, index_generation_id, materialization_id,
      status, attempt_count, max_attempts, created_at, updated_at
    ) VALUES (
      candidate.id, root.id, root.collection_id, candidate.document_id,
      candidate.document_version_id, root.collection_visibility_epoch,
      candidate.index_generation_id, candidate.id, 'pending', 0, 8,
      clock_timestamp(), clock_timestamp()
    ) ON CONFLICT (
      collection_id, collection_visibility_epoch, document_version_id,
      index_generation_id
    ) DO NOTHING;
    last_document_id := candidate.document_id;
    last_document_version_id := candidate.document_version_id;
    last_generation_id := candidate.index_generation_id;
    last_materialization_id := candidate.id;
    processed := processed + 1;
  END LOOP;

  IF processed = 0 THEN
    has_more := false;
  ELSE
    SELECT EXISTS (
      SELECT 1
      FROM (
        SELECT DISTINCT ON (
          candidate_materialization.document_id,
          candidate_materialization.document_version_id,
          candidate_materialization.index_generation_id
        ) candidate_materialization.*
        FROM knowledge_document_materializations candidate_materialization
        LEFT JOIN knowledge_document_projection_heads projection_head
          ON projection_head.index_generation_id =
            candidate_materialization.index_generation_id
          AND projection_head.document_id = candidate_materialization.document_id
        WHERE candidate_materialization.collection_id = root.collection_id
          AND candidate_materialization.status <> 'purged'
        ORDER BY candidate_materialization.document_id,
          candidate_materialization.document_version_id,
          candidate_materialization.index_generation_id,
          (
            projection_head.active_materialization_id =
              candidate_materialization.id
          ) DESC NULLS LAST,
          candidate_materialization.materialization_seq DESC,
          candidate_materialization.id DESC
      ) materialization
      WHERE (
          materialization.document_id,
          materialization.document_version_id,
          materialization.index_generation_id,
          materialization.id
        ) > (
          last_document_id,
          last_document_version_id,
          last_generation_id,
          last_materialization_id
        )
    ) INTO has_more;
  END IF;

  UPDATE knowledge_collection_purges purge
  SET cursor_document_id = COALESCE(
        last_document_id, purge.cursor_document_id
      ),
      cursor_document_version_id = COALESCE(
        last_document_version_id, purge.cursor_document_version_id
      ),
      cursor_index_generation_id = COALESCE(
        last_generation_id, purge.cursor_index_generation_id
      ),
      cursor_materialization_id = COALESCE(
        last_materialization_id, purge.cursor_materialization_id
      ),
      enumeration_complete = NOT has_more,
      status = CASE WHEN has_more THEN 'processing' ELSE 'pending' END,
      attempt_count = CASE WHEN has_more THEN purge.attempt_count ELSE 0 END,
      lease_owner = CASE WHEN has_more THEN purge.lease_owner ELSE NULL END,
      lease_token = CASE WHEN has_more THEN purge.lease_token ELSE NULL END,
      lease_expires_at = CASE
        WHEN has_more THEN clock_timestamp()
          + make_interval(secs => p_lease_seconds)
        ELSE NULL
      END,
      remaining_count_snapshot = (
        SELECT count(*)
        FROM knowledge_collection_purge_items item
        WHERE item.purge_id = purge.id AND item.status <> 'succeeded'
      ),
      updated_at = clock_timestamp()
  WHERE purge.id = root.id;
  RETURN NOT has_more;
END
$function$;

CREATE FUNCTION knowledge_claim_outbox(
  p_consumer TEXT,
  p_worker_id UUID,
  p_lock_token UUID,
  p_lease_seconds INTEGER
) RETURNS SETOF knowledge_outbox
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_consumer IS NULL OR length(trim(p_consumer)) = 0
    OR p_worker_id IS NULL OR p_lock_token IS NULL
    OR p_lock_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_lease_seconds IS NULL OR p_lease_seconds NOT BETWEEN 1 AND 3600
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_CLAIM_ARGUMENT_INVALID';
  END IF;

  UPDATE knowledge_outbox
  SET status = 'failed', locked_at = NULL, lock_owner = NULL,
      lock_token = NULL, lock_expires_at = NULL,
      error_code = 'MAX_ATTEMPTS_EXCEEDED', failed_at = clock_timestamp(),
      last_error = 'MAX_ATTEMPTS_EXCEEDED', updated_at = clock_timestamp()
  WHERE status = 'processing'
    AND lock_expires_at <= clock_timestamp()
    AND attempt_count >= max_attempts;

  RETURN QUERY
  WITH candidate AS (
    SELECT id
    FROM knowledge_outbox
    WHERE attempt_count < max_attempts
      AND (
        (status = 'pending' AND available_at <= clock_timestamp())
        OR (status = 'processing' AND lock_expires_at <= clock_timestamp())
      )
    ORDER BY available_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
  )
  UPDATE knowledge_outbox o
  SET status = 'processing',
      attempt_count = o.attempt_count + 1,
      locked_at = clock_timestamp(),
      lock_owner = p_worker_id,
      lock_token = p_lock_token,
      lock_expires_at = clock_timestamp() + make_interval(secs => p_lease_seconds),
      published_at = NULL, failed_at = NULL, error_code = NULL,
      last_error = NULL, updated_at = clock_timestamp()
  FROM candidate
  WHERE o.id = candidate.id
  RETURNING o.*;
END
$function$;

CREATE FUNCTION knowledge_apply_and_ack_outbox(
  p_consumer TEXT,
  p_event_id UUID,
  p_worker_id UUID,
  p_lock_token UUID,
  p_scope_kind TEXT,
  p_index_generation_id UUID,
  p_action TEXT,
  p_result_hash TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  claimed knowledge_outbox%ROWTYPE;
  scope_id UUID;
  existing_hash TEXT;
  existing_outbox_id BIGINT;
BEGIN
  IF p_consumer IS NULL OR length(trim(p_consumer)) = 0
    OR p_action IS NULL OR length(trim(p_action)) = 0
    OR p_result_hash IS NULL OR p_result_hash !~ '^[0-9a-f]{64}$'
    OR NOT (
      (p_scope_kind = 'global' AND p_index_generation_id IS NULL)
      OR (
        p_scope_kind = 'generation'
        AND p_index_generation_id IS NOT NULL
        AND p_index_generation_id <>
          '00000000-0000-0000-0000-000000000000'::UUID
      )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_APPLY_ARGUMENT_INVALID';
  END IF;

  SELECT * INTO claimed
  FROM knowledge_outbox
  WHERE event_id = p_event_id
    AND status = 'processing'
    AND lock_owner = p_worker_id
    AND lock_token = p_lock_token
    AND lock_expires_at > clock_timestamp()
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_OUTBOX_LEASE';
  END IF;

  IF p_action = 'collection_purge' THEN
    IF p_scope_kind <> 'global'
      OR claimed.event_type <> 'knowledge.collection.tombstoned'
      OR COALESCE(claimed.payload->>'collectionId', '')
        !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
      OR COALESCE(claimed.payload->>'visibilityEpoch', '') !~ '^[1-9][0-9]*$'
      OR NOT EXISTS (
        SELECT 1 FROM knowledge_collections collection
        WHERE collection.id = (claimed.payload->>'collectionId')::UUID
          AND collection.visibility_epoch =
            (claimed.payload->>'visibilityEpoch')::BIGINT
          AND collection.deleted_at IS NOT NULL
      )
    THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'RAG_COLLECTION_PURGE_EVENT_STALE';
    END IF;
    INSERT INTO knowledge_collection_purges(
      id, collection_id, collection_visibility_epoch, source_event_id,
      enumeration_complete, status, attempt_count, max_attempts,
      remaining_count_snapshot, created_at, updated_at
    ) VALUES (
      claimed.event_id, (claimed.payload->>'collectionId')::UUID,
      (claimed.payload->>'visibilityEpoch')::BIGINT, claimed.event_id,
      false, 'pending', 0, 8, 0, clock_timestamp(), clock_timestamp()
    ) ON CONFLICT (source_event_id) DO NOTHING;
    IF NOT EXISTS (
      SELECT 1 FROM knowledge_collection_purges purge
      WHERE purge.source_event_id = claimed.event_id
        AND purge.collection_id = (claimed.payload->>'collectionId')::UUID
        AND purge.collection_visibility_epoch =
          (claimed.payload->>'visibilityEpoch')::BIGINT
    ) THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'RAG_COLLECTION_PURGE_REPLAY_CONFLICT';
    END IF;
  ELSIF p_action NOT IN ('dispatch', 'noop', 'generation_reconstruct') THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_APPLY_ACTION_INVALID';
  END IF;

  scope_id := COALESCE(
    p_index_generation_id,
    '00000000-0000-0000-0000-000000000000'::UUID
  );
  INSERT INTO knowledge_outbox_applied_events(
    consumer_name, event_id, scope_kind, index_generation_id,
    result_hash, outbox_id, applied_at
  ) VALUES (
    p_consumer, p_event_id, p_scope_kind, p_index_generation_id,
    p_result_hash, claimed.id, clock_timestamp()
  ) ON CONFLICT (consumer_name, event_id, generation_scope_id) DO NOTHING;

  SELECT result_hash, outbox_id INTO existing_hash, existing_outbox_id
  FROM knowledge_outbox_applied_events
  WHERE consumer_name = p_consumer
    AND event_id = p_event_id
    AND generation_scope_id = scope_id;
  IF existing_hash IS DISTINCT FROM p_result_hash
    OR existing_outbox_id IS DISTINCT FROM claimed.id
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_APPLIED_LEDGER_CONFLICT';
  END IF;

  UPDATE knowledge_outbox
  SET status = 'published', published_at = clock_timestamp(),
      locked_at = NULL, lock_owner = NULL, lock_token = NULL,
      lock_expires_at = NULL, error_code = NULL, failed_at = NULL,
      last_error = NULL, updated_at = clock_timestamp()
  WHERE id = claimed.id
    AND status = 'processing'
    AND lock_owner = p_worker_id
    AND lock_token = p_lock_token
    AND lock_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_OUTBOX_LEASE';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_retry_outbox(
  p_event_id UUID,
  p_worker_id UUID,
  p_lock_token UUID,
  p_error_code TEXT,
  p_retry_after_seconds INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  attempts INTEGER;
  maximum INTEGER;
BEGIN
  IF p_error_code IS NULL OR p_error_code !~ '^[A-Z0-9_]{1,64}$'
    OR p_retry_after_seconds IS NULL
    OR p_retry_after_seconds NOT BETWEEN 0 AND 86400
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_RETRY_ARGUMENT_INVALID';
  END IF;
  SELECT attempt_count, max_attempts INTO attempts, maximum
  FROM knowledge_outbox
  WHERE event_id = p_event_id AND status = 'processing'
    AND lock_owner = p_worker_id AND lock_token = p_lock_token
    AND lock_expires_at > clock_timestamp()
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_OUTBOX_LEASE';
  END IF;

  IF attempts >= maximum THEN
    UPDATE knowledge_outbox
    SET status = 'failed', locked_at = NULL, lock_owner = NULL,
        lock_token = NULL, lock_expires_at = NULL,
        error_code = 'MAX_ATTEMPTS_EXCEEDED', failed_at = clock_timestamp(),
        last_error = 'MAX_ATTEMPTS_EXCEEDED', updated_at = clock_timestamp()
    WHERE event_id = p_event_id;
  ELSE
    UPDATE knowledge_outbox
    SET status = 'pending', available_at = clock_timestamp()
          + make_interval(secs => p_retry_after_seconds),
        locked_at = NULL, lock_owner = NULL, lock_token = NULL,
        lock_expires_at = NULL, error_code = NULL, failed_at = NULL,
        last_error = p_error_code, updated_at = clock_timestamp()
    WHERE event_id = p_event_id;
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_fail_outbox(
  p_event_id UUID,
  p_worker_id UUID,
  p_lock_token UUID,
  p_error_code TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_error_code IS NULL OR p_error_code !~ '^[A-Z0-9_]{1,64}$' THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_FAIL_ARGUMENT_INVALID';
  END IF;
  UPDATE knowledge_outbox
  SET status = 'failed', locked_at = NULL, lock_owner = NULL,
      lock_token = NULL, lock_expires_at = NULL,
      error_code = p_error_code, failed_at = clock_timestamp(),
      last_error = p_error_code, updated_at = clock_timestamp()
  WHERE event_id = p_event_id AND status = 'processing'
    AND lock_owner = p_worker_id AND lock_token = p_lock_token
    AND lock_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_OUTBOX_LEASE';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_claim_processing_job(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER,
  p_allowed_stages TEXT[]
) RETURNS SETOF knowledge_processing_jobs
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_lease_seconds IS NULL OR p_lease_seconds NOT BETWEEN 1 AND 3600
    OR cardinality(p_allowed_stages) < 1
    OR p_allowed_stages IS NULL
    OR array_position(p_allowed_stages, NULL) IS NOT NULL
    OR NOT (p_allowed_stages <@ ARRAY['parse', 'passage_embedding', 'purge']::TEXT[])
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_JOB_CLAIM_ARGUMENT_INVALID';
  END IF;

  UPDATE knowledge_processing_jobs
  SET status = 'failed', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = clock_timestamp(),
      error_code = 'MAX_ATTEMPTS_EXCEEDED', updated_at = clock_timestamp()
  WHERE status = 'processing'
    AND lease_expires_at <= clock_timestamp()
    AND attempt_count >= max_attempts;

  RETURN QUERY
  WITH candidate AS (
    SELECT id
    FROM knowledge_processing_jobs
    WHERE NOT legacy_projection_unbound
      AND stage = ANY(p_allowed_stages)
      AND attempt_count < max_attempts
      AND (
        (status = 'pending' AND available_at <= clock_timestamp())
        OR (status = 'processing' AND lease_expires_at <= clock_timestamp())
      )
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
  )
  UPDATE knowledge_processing_jobs j
  SET status = 'processing', attempt_count = j.attempt_count + 1,
      lease_owner = p_worker_id, lease_token = p_lease_token,
      lease_expires_at = clock_timestamp() + make_interval(secs => p_lease_seconds),
      completed_at = NULL, error_code = NULL, updated_at = clock_timestamp()
  FROM candidate
  WHERE j.id = candidate.id
  RETURNING j.*;
END
$function$;

CREATE FUNCTION knowledge_heartbeat_processing_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_lease_seconds IS NULL OR p_lease_seconds NOT BETWEEN 1 AND 3600 THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_HEARTBEAT_ARGUMENT_INVALID';
  END IF;
  UPDATE knowledge_processing_jobs
  SET lease_expires_at = clock_timestamp() + make_interval(secs => p_lease_seconds),
      updated_at = clock_timestamp()
  WHERE id = p_job_id AND status = 'processing'
    AND lease_owner = p_worker_id AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_finish_processing_job(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_outcome TEXT,
  p_error_code TEXT,
  p_retry_after_seconds INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  attempts INTEGER;
  maximum INTEGER;
BEGIN
  IF p_outcome IS NULL
    OR p_outcome NOT IN ('succeeded', 'retry', 'failed', 'cancelled')
    OR p_retry_after_seconds IS NULL
    OR p_retry_after_seconds NOT BETWEEN 0 AND 86400
    OR (
      p_outcome IN ('retry', 'failed')
      AND (p_error_code IS NULL OR p_error_code !~ '^[A-Z0-9_]{1,64}$')
    )
    OR (p_outcome IN ('succeeded', 'cancelled') AND p_error_code IS NOT NULL)
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_JOB_FINISH_ARGUMENT_INVALID';
  END IF;

  SELECT attempt_count, max_attempts INTO attempts, maximum
  FROM knowledge_processing_jobs
  WHERE id = p_job_id AND status = 'processing'
    AND lease_owner = p_worker_id AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp()
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  IF p_outcome = 'retry' AND attempts < maximum THEN
    UPDATE knowledge_processing_jobs
    SET status = 'pending', available_at = clock_timestamp()
          + make_interval(secs => p_retry_after_seconds),
        lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
        completed_at = NULL, error_code = NULL, updated_at = clock_timestamp()
    WHERE id = p_job_id;
  ELSIF p_outcome = 'retry' THEN
    UPDATE knowledge_processing_jobs
    SET status = 'failed', lease_owner = NULL, lease_token = NULL,
        lease_expires_at = NULL, completed_at = clock_timestamp(),
        error_code = 'MAX_ATTEMPTS_EXCEEDED', updated_at = clock_timestamp()
    WHERE id = p_job_id;
  ELSE
    UPDATE knowledge_processing_jobs
    SET status = p_outcome, lease_owner = NULL, lease_token = NULL,
        lease_expires_at = NULL, completed_at = clock_timestamp(),
        error_code = CASE WHEN p_outcome = 'failed' THEN p_error_code ELSE NULL END,
        updated_at = clock_timestamp()
    WHERE id = p_job_id;
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_replay_outbox(
  p_event_id UUID,
  p_expected_error_code TEXT,
  p_operator_id UUID,
  p_reason TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_expected_error_code IS NULL
    OR p_expected_error_code !~ '^[A-Z0-9_]{1,64}$'
    OR p_operator_id IS NULL
    OR p_reason IS NULL OR octet_length(p_reason) NOT BETWEEN 1 AND 1024
    OR length(trim(p_reason)) = 0
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_REPLAY_ARGUMENT_INVALID';
  END IF;

  UPDATE knowledge_outbox
  SET status = 'pending', attempt_count = 0, available_at = clock_timestamp(),
      locked_at = NULL, lock_owner = NULL, lock_token = NULL,
      lock_expires_at = NULL, published_at = NULL, error_code = NULL,
      failed_at = NULL, last_error = NULL, updated_at = clock_timestamp()
  WHERE event_id = p_event_id AND status = 'failed'
    AND error_code = p_expected_error_code;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_REPLAY_PRECONDITION_FAILED';
  END IF;

  INSERT INTO knowledge_outbox_replays(
    event_id, expected_error_code, operator_id, reason, replayed_at
  ) VALUES (
    p_event_id, p_expected_error_code, p_operator_id, trim(p_reason),
    clock_timestamp()
  );
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_replay_processing_job(
  p_job_id UUID,
  p_expected_error_code TEXT,
  p_successor_job_id UUID,
  p_operator_id UUID,
  p_reason TEXT
) RETURNS UUID
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  failed_job knowledge_processing_jobs%ROWTYPE;
BEGIN
  IF p_expected_error_code IS NULL
    OR p_expected_error_code !~ '^[A-Z0-9_]{1,64}$'
    OR p_successor_job_id IS NULL OR p_operator_id IS NULL
    OR p_successor_job_id = p_job_id
    OR p_reason IS NULL OR octet_length(p_reason) NOT BETWEEN 1 AND 1024
    OR length(trim(p_reason)) = 0
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_REPLAY_ARGUMENT_INVALID';
  END IF;

  SELECT * INTO failed_job
  FROM knowledge_processing_jobs
  WHERE id = p_job_id AND status = 'failed'
    AND error_code = p_expected_error_code
    AND NOT legacy_projection_unbound
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_REPLAY_PRECONDITION_FAILED';
  END IF;

  INSERT INTO knowledge_processing_jobs(
    id, collection_id, document_id, document_version_id, file_id,
    stage, operation, processor, endpoint_id, governance_profile_id,
    governance_revision, governance_head_revision, collection_consent_id,
    collection_consent_revision, collection_acl_revision,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch, requested_by_user_id, caused_by_job_id,
    idempotency_scope, idempotency_key, request_hash, status,
    attempt_count, max_attempts, available_at, lease_owner,
    lease_expires_at, completed_at, error_code, created_at, updated_at,
    lease_token, index_generation_id, materialization_id, model_id,
    legacy_projection_unbound
  ) VALUES (
    p_successor_job_id, failed_job.collection_id, failed_job.document_id,
    failed_job.document_version_id, failed_job.file_id,
    failed_job.stage, failed_job.operation, failed_job.processor,
    failed_job.endpoint_id, failed_job.governance_profile_id,
    failed_job.governance_revision, failed_job.governance_head_revision,
    failed_job.collection_consent_id, failed_job.collection_consent_revision,
    failed_job.collection_acl_revision, failed_job.collection_visibility_epoch,
    failed_job.collection_processing_revision,
    failed_job.document_visibility_epoch, failed_job.requested_by_user_id,
    failed_job.id, 'replay:' || failed_job.id::TEXT,
    p_successor_job_id::TEXT, failed_job.request_hash, 'pending', 0,
    failed_job.max_attempts, clock_timestamp(), NULL, NULL, NULL, NULL,
    clock_timestamp(), clock_timestamp(), NULL,
    failed_job.index_generation_id, failed_job.materialization_id,
    failed_job.model_id, failed_job.legacy_projection_unbound
  );

  INSERT INTO knowledge_processing_job_replays(
    failed_job_id, successor_job_id, expected_error_code,
    operator_id, reason, replayed_at
  ) VALUES (
    p_job_id, p_successor_job_id, p_expected_error_code,
    p_operator_id, trim(p_reason), clock_timestamp()
  );
  RETURN p_successor_job_id;
END
$function$;

CREATE FUNCTION knowledge_claim_collection_purge_item(
  p_worker_id UUID,
  p_lease_token UUID,
  p_lease_seconds INTEGER
) RETURNS SETOF knowledge_collection_purge_items
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_lease_seconds IS NULL OR p_lease_seconds NOT BETWEEN 1 AND 3600
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_PURGE_CLAIM_ARGUMENT_INVALID';
  END IF;
  UPDATE knowledge_collection_purge_items
  SET status = 'failed', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = clock_timestamp(),
      error_code = 'MAX_ATTEMPTS_EXCEEDED', updated_at = clock_timestamp()
  WHERE status = 'processing'
    AND lease_expires_at <= clock_timestamp()
    AND attempt_count >= max_attempts;
  RETURN QUERY
  WITH candidate AS (
    SELECT id FROM knowledge_collection_purge_items
    WHERE attempt_count < max_attempts
      AND (status = 'pending' OR (
        status = 'processing' AND lease_expires_at <= clock_timestamp()
      ))
    ORDER BY created_at, id
    FOR UPDATE SKIP LOCKED LIMIT 1
  )
  UPDATE knowledge_collection_purge_items i
  SET status = 'processing', attempt_count = i.attempt_count + 1,
      lease_owner = p_worker_id, lease_token = p_lease_token,
      lease_expires_at = clock_timestamp() + make_interval(secs => p_lease_seconds),
      completed_at = NULL, error_code = NULL, updated_at = clock_timestamp()
  FROM candidate WHERE i.id = candidate.id
  RETURNING i.*;
END
$function$;

CREATE FUNCTION knowledge_finish_collection_purge_item(
  p_item_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_succeeded BOOLEAN,
  p_error_code TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_succeeded IS NULL
    OR (p_succeeded AND p_error_code IS NOT NULL)
    OR (
      NOT p_succeeded
      AND (
        p_error_code IS NULL
        OR p_error_code !~ '^[A-Z0-9_]{1,64}$'
      )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_PURGE_FINISH_ARGUMENT_INVALID';
  END IF;
  UPDATE knowledge_collection_purge_items
  SET status = CASE WHEN p_succeeded THEN 'succeeded' ELSE 'failed' END,
      lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
      completed_at = clock_timestamp(), error_code = p_error_code,
      updated_at = clock_timestamp()
  WHERE id = p_item_id AND status = 'processing'
    AND lease_owner = p_worker_id AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_PURGE_LEASE';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_complete_collection_purge(
  p_purge_id UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  root knowledge_collection_purges%ROWTYPE;
BEGIN
  SELECT * INTO root FROM knowledge_collection_purges
  WHERE id = p_purge_id FOR UPDATE;
  IF NOT FOUND OR root.status <> 'pending' OR NOT root.enumeration_complete OR EXISTS (
    SELECT 1 FROM knowledge_collection_purge_items
    WHERE purge_id = p_purge_id AND status <> 'succeeded'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_PURGE_INCOMPLETE';
  END IF;
  UPDATE knowledge_collection_purges
  SET status = 'succeeded', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, remaining_count_snapshot = 0,
      completed_at = clock_timestamp(), error_code = NULL,
      updated_at = clock_timestamp()
  WHERE id = p_purge_id;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_publish_materialization(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID,
  p_expected_collection_acl_revision BIGINT,
  p_expected_document_visibility_epoch BIGINT,
  p_result_hash TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  job knowledge_processing_jobs%ROWTYPE;
  materialization knowledge_document_materializations%ROWTYPE;
  corpus_revision BIGINT;
BEGIN
  IF p_result_hash IS NULL OR p_result_hash !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_PUBLISH_ARGUMENT_INVALID';
  END IF;
  SELECT * INTO job FROM knowledge_processing_jobs
  WHERE id = p_job_id AND status = 'processing'
    AND lease_owner = p_worker_id AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp()
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;
  SELECT * INTO materialization FROM knowledge_document_materializations
  WHERE id = p_materialization_id AND status = 'verified'
    AND id = job.materialization_id
    AND index_generation_id = job.index_generation_id
    AND document_id = job.document_id
    AND document_version_id = job.document_version_id
    AND collection_acl_revision = p_expected_collection_acl_revision
    AND document_visibility_epoch = p_expected_document_visibility_epoch
    AND result_hash = p_result_hash
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_PUBLISH_FENCE_MISMATCH';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM knowledge_collections c
    JOIN knowledge_documents d ON d.collection_id = c.id
    JOIN knowledge_document_versions v ON v.document_id = d.id
    WHERE c.id = materialization.collection_id AND d.id = materialization.document_id
      AND v.id = materialization.document_version_id
      AND c.deleted_at IS NULL AND d.deleted_at IS NULL
      AND c.acl_revision = materialization.collection_acl_revision
      AND c.visibility_epoch = materialization.collection_visibility_epoch
      AND c.collection_processing_revision = materialization.collection_processing_revision
      AND d.visibility_epoch = materialization.document_visibility_epoch
      AND v.content_hash = materialization.source_content_hash
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_PUBLISH_AUTHORITY_STALE';
  END IF;

  SELECT corpus_projection_revision INTO corpus_revision
  FROM knowledge_corpus_projection_head WHERE singleton_id = 1 FOR UPDATE;
  INSERT INTO knowledge_document_projection_heads(
    index_generation_id, document_id, active_materialization_id,
    document_projection_revision, last_corpus_projection_revision, updated_at
  ) VALUES (
    materialization.index_generation_id, materialization.document_id,
    materialization.id, 1, corpus_revision + 1, clock_timestamp()
  ) ON CONFLICT (index_generation_id, document_id) DO UPDATE
    SET active_materialization_id = EXCLUDED.active_materialization_id,
        document_projection_revision =
          knowledge_document_projection_heads.document_projection_revision + 1,
        last_corpus_projection_revision = EXCLUDED.last_corpus_projection_revision,
        updated_at = clock_timestamp();
  UPDATE knowledge_document_materializations
  SET status = 'published', published_at = clock_timestamp()
  WHERE id = materialization.id;
  UPDATE knowledge_corpus_projection_head
  SET corpus_projection_revision = corpus_projection_revision + 1,
      updated_at = clock_timestamp() WHERE singleton_id = 1;
  UPDATE knowledge_processing_jobs
  SET status = 'succeeded', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = clock_timestamp(),
      error_code = NULL, updated_at = clock_timestamp()
  WHERE id = job.id;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_purge_materialization(
  p_purge_item_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_expected_visibility_epoch BIGINT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  item knowledge_collection_purge_items%ROWTYPE;
BEGIN
  SELECT * INTO item FROM knowledge_collection_purge_items
  WHERE id = p_purge_item_id AND status = 'processing'
    AND lease_owner = p_worker_id AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp()
    AND collection_visibility_epoch = p_expected_visibility_epoch
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_PURGE_LEASE';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM knowledge_collections
    WHERE id = item.collection_id
      AND visibility_epoch >= p_expected_visibility_epoch
      AND deleted_at IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_PURGE_TOMBSTONE_STALE';
  END IF;

  UPDATE knowledge_document_projection_heads
  SET active_materialization_id = NULL,
      document_projection_revision = document_projection_revision + 1,
      updated_at = clock_timestamp()
  WHERE index_generation_id = item.index_generation_id
    AND document_id = item.document_id
    AND active_materialization_id = item.materialization_id;
  UPDATE knowledge_document_materializations
  SET status = 'purged',
      retired_at = COALESCE(retired_at, clock_timestamp()),
      purged_at = clock_timestamp()
  WHERE id = item.materialization_id
    AND status IN ('published', 'retired', 'purging');
  UPDATE knowledge_collection_purge_items
  SET status = 'succeeded', lease_owner = NULL, lease_token = NULL,
      lease_expires_at = NULL, completed_at = clock_timestamp(),
      error_code = NULL, updated_at = clock_timestamp()
  WHERE id = item.id;
  UPDATE knowledge_corpus_projection_head
  SET corpus_projection_revision = corpus_projection_revision + 1,
      updated_at = clock_timestamp() WHERE singleton_id = 1;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_promote_index_generation(
  p_index_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_manifest_hash TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  previous_generation UUID;
BEGIN
  IF p_manifest_hash IS NULL OR p_manifest_hash !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'RAG_PROMOTION_ARGUMENT_INVALID';
  END IF;
  IF to_regclass('knowledge_search_profiles') IS NULL THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_SEARCH_SCHEMA_NOT_READY';
  END IF;
  SELECT active_index_generation_id INTO previous_generation
  FROM knowledge_corpus_projection_head
  WHERE singleton_id = 1 AND head_revision = p_expected_head_revision
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_PROMOTION_HEAD_STALE';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM knowledge_index_generations g
    JOIN knowledge_projection_state s ON s.index_generation_id = g.id
    WHERE g.id = p_index_generation_id AND g.status = 'verified'
      AND g.artifact_manifest_hash = p_manifest_hash
      AND s.readiness = 'ready' AND s.manifest_hash = p_manifest_hash
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_PROMOTION_NOT_READY';
  END IF;

  IF previous_generation IS NOT NULL THEN
    UPDATE knowledge_index_generations
    SET status = 'retired', retired_at = clock_timestamp()
    WHERE id = previous_generation AND status = 'active';
    UPDATE knowledge_projection_state
    SET readiness = 'retired', updated_at = clock_timestamp()
    WHERE index_generation_id = previous_generation;
  END IF;
  UPDATE knowledge_index_generations
  SET status = 'active', activated_at = clock_timestamp()
  WHERE id = p_index_generation_id AND status = 'verified';
  UPDATE knowledge_corpus_projection_head
  SET active_index_generation_id = p_index_generation_id,
      corpus_projection_revision = corpus_projection_revision + 1,
      head_revision = head_revision + 1,
      updated_at = clock_timestamp()
  WHERE singleton_id = 1 AND head_revision = p_expected_head_revision;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_PROMOTION_HEAD_STALE';
  END IF;
  RETURN true;
END
$function$;

CREATE FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  p_actor_user_id UUID,
  p_session_id UUID,
  p_conversation_id UUID,
  p_references JSONB
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  source_text TEXT,
  locator JSONB
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_references IS NULL
    OR jsonb_typeof(p_references) <> 'array'
    OR jsonb_array_length(p_references) NOT BETWEEN 1 AND 16
    OR NOT EXISTS (
      SELECT 1 FROM sessions s
      JOIN conversations c ON c.id = p_conversation_id
      WHERE s.id = p_session_id AND s.user_id = p_actor_user_id
        AND s.revoked_at IS NULL AND s.expires_at > clock_timestamp()
        AND c.user_id = p_actor_user_id AND c.status <> 'deleted'
        AND c.deleted_at IS NULL
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = '42501', MESSAGE = 'RAG_HYDRATION_NOT_AUTHORIZED';
  END IF;

  RETURN QUERY
  WITH requested AS (
    SELECT * FROM jsonb_to_recordset(p_references) AS reference(
      collection_id UUID,
      document_id UUID,
      document_version_id UUID,
      index_generation_id UUID,
      materialization_id UUID,
      parent_chunk_id UUID,
      child_chunk_id UUID,
      source_span_hash TEXT
    )
  ), authorized AS (
    SELECT r.*, child.content, parent.locator_summary
    FROM requested r
    JOIN knowledge_corpus_projection_head corpus
      ON corpus.singleton_id = 1
      AND corpus.active_index_generation_id = r.index_generation_id
    JOIN knowledge_document_projection_heads head
      ON head.index_generation_id = r.index_generation_id
      AND head.document_id = r.document_id
      AND head.active_materialization_id = r.materialization_id
    JOIN knowledge_document_materializations m
      ON m.id = r.materialization_id
      AND m.collection_id = r.collection_id
      AND m.document_id = r.document_id
      AND m.document_version_id = r.document_version_id
      AND m.index_generation_id = r.index_generation_id
      AND m.status = 'published'
    JOIN knowledge_collections collection ON collection.id = r.collection_id
    JOIN knowledge_documents document
      ON document.id = r.document_id
      AND document.collection_id = collection.id
      AND document.current_version_id = r.document_version_id
      AND document.status = 'active' AND document.deleted_at IS NULL
    JOIN knowledge_document_versions version
      ON version.id = r.document_version_id
      AND version.document_id = document.id AND version.status = 'active'
    JOIN knowledge_parent_chunks parent
      ON parent.id = r.parent_chunk_id
      AND parent.materialization_id = r.materialization_id
    JOIN knowledge_child_chunks child
      ON child.id = r.child_chunk_id AND child.parent_chunk_id = parent.id
      AND child.materialization_id = r.materialization_id
      AND child.source_span_hash = r.source_span_hash
    WHERE collection.deleted_at IS NULL
      AND collection.acl_revision = m.collection_acl_revision
      AND collection.visibility_epoch = m.collection_visibility_epoch
      AND collection.collection_processing_revision = m.collection_processing_revision
      AND document.visibility_epoch = m.document_visibility_epoch
      AND (
        (collection.scope = 'personal' AND collection.owner_user_id = p_actor_user_id)
        OR (
          collection.scope = 'team'
          AND EXISTS (
            SELECT 1 FROM team_memberships membership
            JOIN teams team ON team.id = membership.team_id
            WHERE membership.team_id = collection.team_id
              AND membership.user_id = p_actor_user_id
              AND membership.status = 'active' AND team.deleted_at IS NULL
          )
        )
      )
  )
  SELECT authorized.collection_id, authorized.document_id,
    authorized.document_version_id, authorized.index_generation_id,
    authorized.materialization_id, authorized.parent_chunk_id,
    authorized.child_chunk_id, authorized.source_span_hash,
    authorized.content, authorized.locator_summary
  FROM authorized
  WHERE octet_length(authorized.content) <= 65536;
END
$function$;

CREATE FUNCTION knowledge_rag_worker_readiness()
RETURNS TABLE(
  consumer_ready BOOLEAN,
  projection_ready BOOLEAN,
  active_index_generation_id UUID,
  detail JSONB
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  WITH required_function(signature) AS (
    VALUES
      ('knowledge_claim_outbox(text,uuid,uuid,integer)'),
      ('knowledge_apply_and_ack_outbox(text,uuid,uuid,uuid,text,uuid,text,text)'),
      ('knowledge_retry_outbox(uuid,uuid,uuid,text,integer)'),
      ('knowledge_fail_outbox(uuid,uuid,uuid,text)'),
      ('knowledge_claim_processing_job(uuid,uuid,integer,text[])'),
      ('knowledge_heartbeat_processing_job(uuid,uuid,uuid,integer)'),
      ('knowledge_finish_processing_job(uuid,uuid,uuid,text,text,integer)'),
      ('knowledge_claim_collection_purge(uuid,uuid,integer)'),
      ('knowledge_enumerate_collection_purge(uuid,uuid,uuid,integer,integer)'),
      ('knowledge_claim_collection_purge_item(uuid,uuid,integer)'),
      ('knowledge_finish_collection_purge_item(uuid,uuid,uuid,boolean,text)'),
      ('knowledge_complete_collection_purge(uuid)')
  ), worker_capability AS (
    SELECT COALESCE(bool_and(
      to_regprocedure(signature) IS NOT NULL
      AND has_function_privilege(
        session_user,
        to_regprocedure(signature),
        'EXECUTE'
      )
    ), false) AS ready
    FROM required_function
  )
  SELECT
    worker_capability.ready,
    COALESCE(state.readiness = 'ready', false),
    head.active_index_generation_id,
    jsonb_build_object(
      'consumer', CASE
        WHEN worker_capability.ready THEN 'ready'
        ELSE 'not_ready'
      END,
      'projection', COALESCE(state.readiness, 'not_ready'),
      'headRevision', head.head_revision,
      'corpusProjectionRevision', head.corpus_projection_revision
    )
  FROM knowledge_corpus_projection_head head
  CROSS JOIN worker_capability
  LEFT JOIN knowledge_projection_state state
    ON state.index_generation_id = head.active_index_generation_id
  WHERE head.singleton_id = 1
$function$;

DO $privileges$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_namespace namespace,
      aclexplode(COALESCE(
        namespace.nspacl,
        acldefault('n', namespace.nspowner)
      )) privilege
    WHERE namespace.nspname = schema_name
      AND privilege.grantee = 0
      AND privilege.privilege_type = 'CREATE'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'RAG_PROJECTION_SCHEMA_PUBLIC_CREATE';
  END IF;
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM rag_worker_executor, rag_replay_operator, rag_api_reader, go_evidence_hydrator, go_api_runtime',
    schema_name
  );
  IF has_schema_privilege('go_api_runtime', schema_name, 'CREATE') THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'RAG_GO_API_RUNTIME_SCHEMA_CREATE_FORBIDDEN';
  END IF;
  EXECUTE format(
    'GRANT USAGE ON SCHEMA %I TO rag_projection_owner, rag_worker_executor, rag_replay_operator, rag_api_reader, go_evidence_hydrator, go_api_runtime',
    schema_name
  );
  EXECUTE format(
    'GRANT CREATE ON SCHEMA %I TO rag_projection_owner',
    schema_name
  );
END
$privileges$;

GRANT SELECT, INSERT, UPDATE ON
  knowledge_outbox,
  knowledge_outbox_applied_events,
  knowledge_outbox_replays,
  knowledge_processing_jobs,
  knowledge_processing_job_replays,
  knowledge_collection_purges,
  knowledge_collection_purge_items,
  knowledge_document_materializations,
  knowledge_document_projection_heads,
  knowledge_corpus_projection_head,
  knowledge_index_generations,
  knowledge_projection_state
TO rag_projection_owner;
GRANT SELECT ON
  sessions, conversations, team_memberships, teams,
  knowledge_collections, knowledge_documents, knowledge_document_versions,
  knowledge_blocks, knowledge_parent_chunks, knowledge_child_chunks
TO rag_projection_owner;
GRANT USAGE, SELECT ON SEQUENCE knowledge_outbox_replays_id_seq
TO rag_projection_owner;

-- The Go API and its admin commands share one capability role. Keep this an
-- explicit list of the authoritative 001-009 relations so later projection
-- tables never become writable through a broad default privilege.
GRANT SELECT, INSERT, UPDATE, DELETE ON
  users,
  sessions,
  conversations,
  provider_configs,
  files,
  messages,
  message_attachments,
  audit_logs,
  import_batches,
  user_credentials,
  credential_recovery_tokens,
  teams,
  team_memberships,
  team_invites,
  knowledge_collections,
  knowledge_documents,
  knowledge_document_versions,
  user_query_consent_state,
  processor_governance_profiles,
  processor_governance_heads,
  processing_consents,
  knowledge_outbox,
  identity_mail_outbox,
  knowledge_processing_jobs
TO go_api_runtime;
GRANT USAGE, SELECT ON SEQUENCE knowledge_outbox_id_seq
TO go_api_runtime;

ALTER FUNCTION knowledge_claim_outbox(TEXT, UUID, UUID, INTEGER)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_reject_immutable_projection_mutation()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_validate_chunk_block_span()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_apply_and_ack_outbox(
  TEXT, UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_retry_outbox(UUID, UUID, UUID, TEXT, INTEGER)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fail_outbox(UUID, UUID, UUID, TEXT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_claim_processing_job(UUID, UUID, INTEGER, TEXT[])
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_heartbeat_processing_job(UUID, UUID, UUID, INTEGER)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_finish_processing_job(
  UUID, UUID, UUID, TEXT, TEXT, INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_replay_outbox(UUID, TEXT, UUID, TEXT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_replay_processing_job(UUID, TEXT, UUID, UUID, TEXT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_claim_collection_purge(UUID, UUID, INTEGER)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_enumerate_collection_purge(
  UUID, UUID, UUID, INTEGER, INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_claim_collection_purge_item(UUID, UUID, INTEGER)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_finish_collection_purge_item(
  UUID, UUID, UUID, BOOLEAN, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_complete_collection_purge(UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_publish_materialization(
  UUID, UUID, UUID, UUID, BIGINT, BIGINT, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_purge_materialization(UUID, UUID, UUID, BIGINT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_promote_index_generation(UUID, BIGINT, TEXT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_rag_worker_readiness()
  OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_claim_outbox(TEXT, UUID, UUID, INTEGER) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_reject_immutable_projection_mutation() FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_validate_chunk_block_span() FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_apply_and_ack_outbox(
  TEXT, UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_retry_outbox(UUID, UUID, UUID, TEXT, INTEGER) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fail_outbox(UUID, UUID, UUID, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_claim_processing_job(UUID, UUID, INTEGER, TEXT[]) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_heartbeat_processing_job(UUID, UUID, UUID, INTEGER) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_finish_processing_job(
  UUID, UUID, UUID, TEXT, TEXT, INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_replay_outbox(UUID, TEXT, UUID, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_replay_processing_job(UUID, TEXT, UUID, UUID, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_claim_collection_purge(UUID, UUID, INTEGER) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_enumerate_collection_purge(
  UUID, UUID, UUID, INTEGER, INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_claim_collection_purge_item(UUID, UUID, INTEGER) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_finish_collection_purge_item(
  UUID, UUID, UUID, BOOLEAN, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_complete_collection_purge(UUID) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_publish_materialization(
  UUID, UUID, UUID, UUID, BIGINT, BIGINT, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_purge_materialization(UUID, UUID, UUID, BIGINT) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_promote_index_generation(UUID, BIGINT, TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_rag_worker_readiness() FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_claim_outbox(TEXT, UUID, UUID, INTEGER),
  knowledge_apply_and_ack_outbox(TEXT, UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT),
  knowledge_retry_outbox(UUID, UUID, UUID, TEXT, INTEGER),
  knowledge_fail_outbox(UUID, UUID, UUID, TEXT),
  knowledge_claim_processing_job(UUID, UUID, INTEGER, TEXT[]),
  knowledge_heartbeat_processing_job(UUID, UUID, UUID, INTEGER),
  knowledge_finish_processing_job(UUID, UUID, UUID, TEXT, TEXT, INTEGER),
  knowledge_claim_collection_purge(UUID, UUID, INTEGER),
  knowledge_enumerate_collection_purge(UUID, UUID, UUID, INTEGER, INTEGER),
  knowledge_claim_collection_purge_item(UUID, UUID, INTEGER),
  knowledge_finish_collection_purge_item(UUID, UUID, UUID, BOOLEAN, TEXT),
  knowledge_complete_collection_purge(UUID),
  knowledge_publish_materialization(UUID, UUID, UUID, UUID, BIGINT, BIGINT, TEXT),
  knowledge_purge_materialization(UUID, UUID, UUID, BIGINT),
  knowledge_rag_worker_readiness()
TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_replay_outbox(UUID, TEXT, UUID, TEXT),
  knowledge_replay_processing_job(UUID, TEXT, UUID, UUID, TEXT)
TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_rag_worker_readiness()
TO rag_api_reader, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) TO go_evidence_hydrator, go_api_runtime;

DO $owner_create_revocation$
BEGIN
  EXECUTE format(
    'REVOKE CREATE ON SCHEMA %I FROM rag_projection_owner',
    current_schema()
  );
END
$owner_create_revocation$;
