-- Register an isolated SiliconFlow Pro BGE retrieval vector space without
-- mutating the current Jina Active Generation. Every read, write, rebuild,
-- verification, and cutover seam remains generation/search-profile fenced.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

-- The two admitted retrieval tuples are exact. Matching dimensions do not make
-- Jina and BGE vectors interchangeable.
DO $drop_legacy_search_checks$
DECLARE
  constraint_row RECORD;
BEGIN
  FOR constraint_row IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid = 'knowledge_search_profiles'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) ILIKE '%jina%'
  LOOP
    EXECUTE format(
      'ALTER TABLE knowledge_search_profiles DROP CONSTRAINT %I',
      constraint_row.conname
    );
  END LOOP;
END
$drop_legacy_search_checks$;

ALTER TABLE knowledge_search_profiles
  ADD CONSTRAINT knowledge_search_profiles_supported_tuple_check CHECK (
    (
      provider_profile_id = 'mineru_jina_postgres_v1'
      AND embedding_processor = 'jina'
      AND embedding_model_id = 'jina-embeddings-v4'
      AND rerank_processor = 'jina'
      AND rerank_model_id = 'jina-reranker-v3'
    ) OR (
      provider_profile_id = 'siliconflow_bge_m3_v1'
      AND embedding_processor = 'siliconflow'
      AND embedding_model_id = 'Pro/BAAI/bge-m3'
      AND rerank_processor = 'siliconflow'
      AND rerank_model_id = 'Pro/BAAI/bge-reranker-v2-m3'
    )
  );

DO $drop_legacy_child_search_check$
DECLARE
  constraint_row RECORD;
BEGIN
  FOR constraint_row IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid = 'knowledge_child_search_projections'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) ILIKE '%jina-embeddings-v4%'
  LOOP
    EXECUTE format(
      'ALTER TABLE knowledge_child_search_projections DROP CONSTRAINT %I',
      constraint_row.conname
    );
  END LOOP;
END
$drop_legacy_child_search_check$;

ALTER TABLE knowledge_child_search_projections
  ADD CONSTRAINT knowledge_child_search_supported_model_check CHECK (
    embedding_model_id IN ('jina-embeddings-v4', 'Pro/BAAI/bge-m3')
  );

DO $drop_legacy_vector_shadow_check$
DECLARE
  constraint_row RECORD;
BEGIN
  FOR constraint_row IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid = 'knowledge_child_vector_shadow_projections'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) ILIKE '%jina-embeddings-v4%'
  LOOP
    EXECUTE format(
      'ALTER TABLE knowledge_child_vector_shadow_projections DROP CONSTRAINT %I',
      constraint_row.conname
    );
  END LOOP;
END
$drop_legacy_vector_shadow_check$;

ALTER TABLE knowledge_child_vector_shadow_projections
  ADD CONSTRAINT knowledge_vector_shadow_supported_model_check CHECK (
    embedding_model_id IN ('jina-embeddings-v4', 'Pro/BAAI/bge-m3')
  );

CREATE FUNCTION knowledge_retrieval_profile_id(
  p_provider_profile_id TEXT,
  p_embedding_processor TEXT,
  p_embedding_model_id TEXT,
  p_embedding_dimensions INTEGER,
  p_rerank_processor TEXT,
  p_rerank_model_id TEXT
) RETURNS TEXT
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
SET search_path FROM CURRENT
AS $function$
  SELECT CASE
    WHEN p_provider_profile_id = 'mineru_jina_postgres_v1'
      AND p_embedding_processor = 'jina'
      AND p_embedding_model_id = 'jina-embeddings-v4'
      AND p_embedding_dimensions = 1024
      AND p_rerank_processor = 'jina'
      AND p_rerank_model_id = 'jina-reranker-v3'
      THEN 'jina_v4_v3'
    WHEN p_provider_profile_id = 'siliconflow_bge_m3_v1'
      AND p_embedding_processor = 'siliconflow'
      AND p_embedding_model_id = 'Pro/BAAI/bge-m3'
      AND p_embedding_dimensions = 1024
      AND p_rerank_processor = 'siliconflow'
      AND p_rerank_model_id = 'Pro/BAAI/bge-reranker-v2-m3'
      THEN 'siliconflow_bge_m3_v1'
    ELSE NULL
  END
$function$;

ALTER FUNCTION knowledge_retrieval_profile_id(
  TEXT, TEXT, TEXT, INTEGER, TEXT, TEXT
) OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_retrieval_profile_id(
  TEXT, TEXT, TEXT, INTEGER, TEXT, TEXT
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_retrieval_profile_id(
  TEXT, TEXT, TEXT, INTEGER, TEXT, TEXT
) TO rag_projection_owner, go_api_runtime, rag_worker_executor,
  rag_replay_operator;

INSERT INTO knowledge_structure_chunk_profile_descriptors(
  chunk_profile_hash,
  schema_version,
  profile
) VALUES (
  '36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73',
  'mm-chat.structure-chunk-profile.v3',
  '{
    "bounds": {
      "child": {"hardMaximum": 650, "target": 400, "targetMaximum": 500, "targetMinimum": 300},
      "overlap": {"maximum": 100, "target": 64},
      "parent": {"hardMaximum": 2000, "targetMaximum": 1600, "targetMinimum": 1200}
    },
    "derivedContext": {"citationAuthority": "original_source_span", "countedInOverlap": false, "maximumTokens": 96},
    "nonIndexable": {"policy": "preserve_source_exclude_retrieval", "signals": ["repeated_text", "page_position", "frequency"]},
    "routes": {
      "code": "logical_lines_then_token",
      "formula": "atomic_then_token",
      "json": "subtree_path_then_token",
      "narrative": "semantic_hint_then_sentence_recursive",
      "slide": "slide_shape_then_token",
      "table": "header_row_group_then_token"
    },
    "schemaVersion": "mm-chat.structure-chunk-profile.v3",
    "semantic": {
      "admission": "long_unstructured_narrative_only",
      "failure": "deterministic_sentence_recursive_fallback",
      "hintAuthority": "content_and_embedding_profile_hash_bound",
      "profileHash": "f8de6087c6b28fe89b904549e0ddcbe4b51ebb88aecf8232ab07e6ec0d316165"
    },
    "tokenizer": {
      "artifactSha256": "223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7",
      "name": "cl100k_base",
      "normalization": "none",
      "profileHash": "bdff1b0c1c8195fc2fd0a1818bac2ca66a9332a53a5cdf3d434132dff02724a0",
      "revision": "openai-public-2022-12-14",
      "specialTokenPolicy": "encode_ordinary",
      "vocabularySha256": "d48a1992b71a810f377931afd97b5b28588e412918a3f2d9e445b019f29dc6e4"
    }
  }'::jsonb
);

-- Approved provider metadata is immutable and contains no credential.
INSERT INTO processor_governance_profiles(
  id, processor, endpoint_id, model_api_version, allowed_purposes,
  allowed_data_types, region, retention_policy, deletion_contract,
  training_use, status, governance_revision, manifest_hash, model_id,
  profile_contract_hash
) VALUES
(
  'ffd802ab-0e84-5fbb-a6cd-991e96dbe36e', 'siliconflow',
  'siliconflow-cn-v1', 'v1-2026-07-24',
  ARRAY['passage_embedding','query_embedding']::TEXT[],
  ARRAY['text/plain']::TEXT[], 'CN', 'request-scoped',
  'provider-request-ephemeral', 'disabled', 'approved', 1,
  'a43c4583c9a2d7179b4f4dc547ac7ada3df9e58a0e07aacc07809bdcc645f81d',
  'Pro/BAAI/bge-m3',
  '119b0c64092d385c6f463d2af8605d6039243495b86faa6fc325baef001bacb1'
),
(
  'a317ecb4-dc6a-5541-bc65-81cccca900e6', 'siliconflow',
  'siliconflow-cn-v1', 'v1-2026-07-24', ARRAY['rerank']::TEXT[],
  ARRAY['text/plain']::TEXT[], 'CN', 'request-scoped',
  'provider-request-ephemeral', 'disabled', 'approved', 1,
  '70cb2c1f967acda7cca15c5838465c7f5a1fccec9668c1fa15e7b1900fceb977',
  'Pro/BAAI/bge-reranker-v2-m3',
  '7c042a6ca6e27a308e41c9788e4735a65073b4dd1423904e29a2f6338eebb41b'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO processor_governance_heads(
  processor, endpoint_id, model_id, status, active_profile_id,
  active_governance_revision, head_revision
) VALUES
(
  'siliconflow', 'siliconflow-cn-v1', 'Pro/BAAI/bge-m3', 'active',
  'ffd802ab-0e84-5fbb-a6cd-991e96dbe36e', 1, 1
),
(
  'siliconflow', 'siliconflow-cn-v1',
  'Pro/BAAI/bge-reranker-v2-m3', 'active',
  'a317ecb4-dc6a-5541-bc65-81cccca900e6', 1, 1
)
ON CONFLICT (processor, endpoint_id, model_id) DO NOTHING;

DO $verify_siliconflow_governance$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM processor_governance_profiles
    WHERE id = 'ffd802ab-0e84-5fbb-a6cd-991e96dbe36e'
      AND processor = 'siliconflow'
      AND endpoint_id = 'siliconflow-cn-v1'
      AND model_id = 'Pro/BAAI/bge-m3'
      AND allowed_purposes =
        ARRAY['passage_embedding','query_embedding']::TEXT[]
      AND allowed_data_types = ARRAY['text/plain']::TEXT[]
      AND governance_revision = 1
      AND manifest_hash =
        'a43c4583c9a2d7179b4f4dc547ac7ada3df9e58a0e07aacc07809bdcc645f81d'
      AND profile_contract_hash =
        '119b0c64092d385c6f463d2af8605d6039243495b86faa6fc325baef001bacb1'
  ) OR NOT EXISTS (
    SELECT 1 FROM processor_governance_profiles
    WHERE id = 'a317ecb4-dc6a-5541-bc65-81cccca900e6'
      AND processor = 'siliconflow'
      AND endpoint_id = 'siliconflow-cn-v1'
      AND model_id = 'Pro/BAAI/bge-reranker-v2-m3'
      AND allowed_purposes = ARRAY['rerank']::TEXT[]
      AND allowed_data_types = ARRAY['text/plain']::TEXT[]
      AND governance_revision = 1
      AND manifest_hash =
        '70cb2c1f967acda7cca15c5838465c7f5a1fccec9668c1fa15e7b1900fceb977'
      AND profile_contract_hash =
        '7c042a6ca6e27a308e41c9788e4735a65073b4dd1423904e29a2f6338eebb41b'
  ) OR NOT EXISTS (
    SELECT 1 FROM processor_governance_heads
    WHERE processor = 'siliconflow'
      AND endpoint_id = 'siliconflow-cn-v1'
      AND model_id = 'Pro/BAAI/bge-m3'
      AND active_profile_id = 'ffd802ab-0e84-5fbb-a6cd-991e96dbe36e'
      AND active_governance_revision = 1
      AND head_revision = 1
      AND status = 'active'
  ) OR NOT EXISTS (
    SELECT 1 FROM processor_governance_heads
    WHERE processor = 'siliconflow'
      AND endpoint_id = 'siliconflow-cn-v1'
      AND model_id = 'Pro/BAAI/bge-reranker-v2-m3'
      AND active_profile_id = 'a317ecb4-dc6a-5541-bc65-81cccca900e6'
      AND active_governance_revision = 1
      AND head_revision = 1
      AND status = 'active'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_SILICONFLOW_GOVERNANCE_CONFLICT';
  END IF;
END
$verify_siliconflow_governance$;

-- The operator-approved provider migration derives collection-scoped consent
-- from each current granted Jina purpose. It creates new auditable rows rather
-- than rewriting the original consent authority.
WITH source_consent AS (
  SELECT DISTINCT ON (consent.collection_id)
    consent.*
  FROM processing_consents consent
  WHERE consent.scope = 'collection'
    AND consent.collection_id IS NOT NULL
    AND consent.processor = 'jina'
    AND consent.model_id = 'jina-embeddings-v4'
    AND consent.decision = 'granted'
    AND consent.superseded_at IS NULL
    AND (consent.expires_at IS NULL OR consent.expires_at > clock_timestamp())
    AND 'passage_embedding' = ANY(consent.purposes)
    AND 'text/plain' = ANY(consent.data_types)
  ORDER BY consent.collection_id, consent.consent_revision DESC
)
INSERT INTO processing_consents(
  id, scope, collection_id, user_id, processor, endpoint_id, model_id,
  governance_profile_id, governance_revision, governance_head_revision,
  purposes, data_types, policy_version, decision, consent_revision,
  granted_by_user_id, decided_at, expires_at, superseded_at, created_at,
  updated_at
)
SELECT
  gen_random_uuid(), 'collection', source.collection_id, NULL, 'siliconflow',
  'siliconflow-cn-v1', 'Pro/BAAI/bge-m3',
  'ffd802ab-0e84-5fbb-a6cd-991e96dbe36e', 1, 1,
  ARRAY['passage_embedding']::TEXT[], ARRAY['text/plain']::TEXT[],
  '2026-07-24-siliconflow-pro-bge-approved-v1', 'granted', 1,
  source.granted_by_user_id, CURRENT_TIMESTAMP, source.expires_at, NULL,
  CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM source_consent source
WHERE NOT EXISTS (
  SELECT 1 FROM processing_consents existing
  WHERE existing.scope = 'collection'
    AND existing.collection_id = source.collection_id
    AND existing.processor = 'siliconflow'
    AND existing.endpoint_id = 'siliconflow-cn-v1'
    AND existing.model_id = 'Pro/BAAI/bge-m3'
    AND existing.decision = 'granted'
    AND existing.superseded_at IS NULL
);

WITH source_consent AS (
  SELECT DISTINCT ON (consent.user_id)
    consent.*
  FROM processing_consents consent
  WHERE consent.scope = 'query'
    AND consent.user_id IS NOT NULL
    AND consent.processor = 'jina'
    AND consent.model_id = 'jina-embeddings-v4'
    AND consent.decision = 'granted'
    AND consent.superseded_at IS NULL
    AND (consent.expires_at IS NULL OR consent.expires_at > clock_timestamp())
    AND 'query_embedding' = ANY(consent.purposes)
    AND 'text/plain' = ANY(consent.data_types)
  ORDER BY consent.user_id, consent.consent_revision DESC
)
INSERT INTO processing_consents(
  id, scope, collection_id, user_id, processor, endpoint_id, model_id,
  governance_profile_id, governance_revision, governance_head_revision,
  purposes, data_types, policy_version, decision, consent_revision,
  granted_by_user_id, decided_at, expires_at, superseded_at, created_at,
  updated_at
)
SELECT
  gen_random_uuid(), 'query', NULL, source.user_id, 'siliconflow',
  'siliconflow-cn-v1', 'Pro/BAAI/bge-m3',
  'ffd802ab-0e84-5fbb-a6cd-991e96dbe36e', 1, 1,
  ARRAY['query_embedding']::TEXT[], ARRAY['text/plain']::TEXT[],
  '2026-07-24-siliconflow-pro-bge-approved-v1', 'granted', 1,
  source.granted_by_user_id, CURRENT_TIMESTAMP, source.expires_at, NULL,
  CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM source_consent source
WHERE NOT EXISTS (
  SELECT 1 FROM processing_consents existing
  WHERE existing.scope = 'query'
    AND existing.user_id = source.user_id
    AND existing.processor = 'siliconflow'
    AND existing.endpoint_id = 'siliconflow-cn-v1'
    AND existing.model_id = 'Pro/BAAI/bge-m3'
    AND existing.decision = 'granted'
    AND existing.superseded_at IS NULL
);

WITH source_consent AS (
  SELECT DISTINCT ON (consent.collection_id)
    consent.*
  FROM processing_consents consent
  WHERE consent.scope = 'collection'
    AND consent.collection_id IS NOT NULL
    AND consent.processor = 'jina'
    AND consent.model_id = 'jina-reranker-v3'
    AND consent.decision = 'granted'
    AND consent.superseded_at IS NULL
    AND (consent.expires_at IS NULL OR consent.expires_at > clock_timestamp())
    AND 'rerank' = ANY(consent.purposes)
    AND 'text/plain' = ANY(consent.data_types)
  ORDER BY consent.collection_id, consent.consent_revision DESC
)
INSERT INTO processing_consents(
  id, scope, collection_id, user_id, processor, endpoint_id, model_id,
  governance_profile_id, governance_revision, governance_head_revision,
  purposes, data_types, policy_version, decision, consent_revision,
  granted_by_user_id, decided_at, expires_at, superseded_at, created_at,
  updated_at
)
SELECT
  gen_random_uuid(), 'collection', source.collection_id, NULL, 'siliconflow',
  'siliconflow-cn-v1', 'Pro/BAAI/bge-reranker-v2-m3',
  'a317ecb4-dc6a-5541-bc65-81cccca900e6', 1, 1,
  ARRAY['rerank']::TEXT[], ARRAY['text/plain']::TEXT[],
  '2026-07-24-siliconflow-pro-bge-approved-v1', 'granted', 1,
  source.granted_by_user_id, CURRENT_TIMESTAMP, source.expires_at, NULL,
  CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM source_consent source
WHERE NOT EXISTS (
  SELECT 1 FROM processing_consents existing
  WHERE existing.scope = 'collection'
    AND existing.collection_id = source.collection_id
    AND existing.processor = 'siliconflow'
    AND existing.endpoint_id = 'siliconflow-cn-v1'
    AND existing.model_id = 'Pro/BAAI/bge-reranker-v2-m3'
    AND existing.decision = 'granted'
    AND existing.superseded_at IS NULL
);

WITH source_consent AS (
  SELECT DISTINCT ON (consent.user_id)
    consent.*
  FROM processing_consents consent
  WHERE consent.scope = 'query'
    AND consent.user_id IS NOT NULL
    AND consent.processor = 'jina'
    AND consent.model_id = 'jina-reranker-v3'
    AND consent.decision = 'granted'
    AND consent.superseded_at IS NULL
    AND (consent.expires_at IS NULL OR consent.expires_at > clock_timestamp())
    AND 'rerank' = ANY(consent.purposes)
    AND 'text/plain' = ANY(consent.data_types)
  ORDER BY consent.user_id, consent.consent_revision DESC
)
INSERT INTO processing_consents(
  id, scope, collection_id, user_id, processor, endpoint_id, model_id,
  governance_profile_id, governance_revision, governance_head_revision,
  purposes, data_types, policy_version, decision, consent_revision,
  granted_by_user_id, decided_at, expires_at, superseded_at, created_at,
  updated_at
)
SELECT
  gen_random_uuid(), 'query', NULL, source.user_id, 'siliconflow',
  'siliconflow-cn-v1', 'Pro/BAAI/bge-reranker-v2-m3',
  'a317ecb4-dc6a-5541-bc65-81cccca900e6', 1, 1,
  ARRAY['rerank']::TEXT[], ARRAY['text/plain']::TEXT[],
  '2026-07-24-siliconflow-pro-bge-approved-v1', 'granted', 1,
  source.granted_by_user_id, CURRENT_TIMESTAMP, source.expires_at, NULL,
  CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM source_consent source
WHERE NOT EXISTS (
  SELECT 1 FROM processing_consents existing
  WHERE existing.scope = 'query'
    AND existing.user_id = source.user_id
    AND existing.processor = 'siliconflow'
    AND existing.endpoint_id = 'siliconflow-cn-v1'
    AND existing.model_id = 'Pro/BAAI/bge-reranker-v2-m3'
    AND existing.decision = 'granted'
    AND existing.superseded_at IS NULL
);


CREATE FUNCTION knowledge_resolve_generation_embedding_profile(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_index_generation_id UUID,
  p_materialization_id UUID
) RETURNS TABLE(
  processor TEXT,
  embedding_model_id TEXT,
  embedding_dimensions INTEGER
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_job_id IS NULL OR p_worker_id IS NULL OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_index_generation_id IS NULL OR p_materialization_id IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE = '22023',
      MESSAGE = 'RAG_GENERATION_EMBEDDING_PROFILE_ARGUMENT_INVALID';
  END IF;

  RETURN QUERY
  SELECT
    search_profile.embedding_processor,
    search_profile.embedding_model_id,
    search_profile.embedding_dimensions
  FROM knowledge_processing_jobs processing_job
  JOIN knowledge_document_materializations materialization
    ON materialization.id = p_materialization_id
   AND materialization.index_generation_id = p_index_generation_id
   AND materialization.collection_id = processing_job.collection_id
   AND materialization.document_id = processing_job.document_id
   AND materialization.document_version_id = processing_job.document_version_id
   AND materialization.file_id = processing_job.file_id
   AND materialization.status = 'staging'
  JOIN knowledge_index_generations generation
    ON generation.id = p_index_generation_id
   AND generation.id = processing_job.index_generation_id
   AND generation.status IN ('building', 'verified', 'active')
  JOIN knowledge_index_profiles index_profile
    ON index_profile.id = generation.index_profile_id
  JOIN knowledge_search_profiles search_profile
    ON search_profile.index_profile_id = index_profile.id
   AND search_profile.embedding_processor = index_profile.embedding_processor
   AND search_profile.embedding_model_id = index_profile.embedding_model_id
   AND search_profile.rerank_processor = index_profile.rerank_processor
   AND search_profile.rerank_model_id = index_profile.rerank_model_id
   AND knowledge_retrieval_profile_id(
     search_profile.provider_profile_id,
     search_profile.embedding_processor,
     search_profile.embedding_model_id,
     search_profile.embedding_dimensions,
     search_profile.rerank_processor,
     search_profile.rerank_model_id
   ) IS NOT NULL
  WHERE processing_job.id = p_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'parse'
    AND processing_job.operation IN ('initial', 'replace', 'reprocess')
    AND NOT processing_job.legacy_projection_unbound
    AND processing_job.lease_owner = p_worker_id
    AND processing_job.lease_token = p_lease_token
    AND processing_job.lease_expires_at > clock_timestamp()
    AND processing_job.materialization_id = p_materialization_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001',
      MESSAGE = 'RAG_GENERATION_EMBEDDING_PROFILE_MISSING';
  END IF;
END
$function$;

CREATE FUNCTION knowledge_resolve_generation_retrieval_profile(
  p_index_generation_id UUID
) RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  retrieval_profile_id TEXT,
  provider_id TEXT,
  embedding_model_id TEXT,
  embedding_dimensions INTEGER,
  rerank_model_id TEXT
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT
    generation.id,
    search_profile.id,
    knowledge_retrieval_profile_id(
      search_profile.provider_profile_id,
      search_profile.embedding_processor,
      search_profile.embedding_model_id,
      search_profile.embedding_dimensions,
      search_profile.rerank_processor,
      search_profile.rerank_model_id
    ),
    search_profile.embedding_processor,
    search_profile.embedding_model_id,
    search_profile.embedding_dimensions,
    search_profile.rerank_model_id
  FROM knowledge_index_generations generation
  JOIN knowledge_index_profiles index_profile
    ON index_profile.id = generation.index_profile_id
  JOIN knowledge_search_profiles search_profile
    ON search_profile.index_profile_id = index_profile.id
   AND search_profile.embedding_processor = index_profile.embedding_processor
   AND search_profile.embedding_model_id = index_profile.embedding_model_id
   AND search_profile.rerank_processor = index_profile.rerank_processor
   AND search_profile.rerank_model_id = index_profile.rerank_model_id
  WHERE generation.id = p_index_generation_id
    AND generation.status IN ('building', 'verified', 'active', 'retired')
    AND knowledge_retrieval_profile_id(
      search_profile.provider_profile_id,
      search_profile.embedding_processor,
      search_profile.embedding_model_id,
      search_profile.embedding_dimensions,
      search_profile.rerank_processor,
      search_profile.rerank_model_id
    ) IS NOT NULL
$function$;

CREATE FUNCTION knowledge_resolve_active_retrieval_profile()
RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  retrieval_profile_id TEXT,
  provider_id TEXT,
  embedding_model_id TEXT,
  embedding_dimensions INTEGER,
  rerank_model_id TEXT
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT profile.*
  FROM knowledge_corpus_projection_head head
  JOIN knowledge_retrieval_profile_head retrieval_head
    ON retrieval_head.singleton_id = 1
   AND retrieval_head.active_profile = 'pg17_bm25_pgvector_v1'
  CROSS JOIN LATERAL knowledge_resolve_generation_retrieval_profile(
    head.active_index_generation_id
  ) profile
  JOIN knowledge_index_generations generation
    ON generation.id = profile.index_generation_id
   AND generation.status = 'active'
  WHERE head.singleton_id = 1
$function$;

ALTER FUNCTION knowledge_resolve_generation_embedding_profile(
  UUID, UUID, UUID, UUID, UUID
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_resolve_generation_retrieval_profile(UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_resolve_active_retrieval_profile()
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_resolve_generation_embedding_profile(
  UUID, UUID, UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_resolve_generation_retrieval_profile(UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_resolve_active_retrieval_profile()
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_resolve_generation_embedding_profile(
  UUID, UUID, UUID, UUID, UUID
) TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_resolve_generation_retrieval_profile(UUID)
  TO go_api_runtime, rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_resolve_active_retrieval_profile()
  TO go_api_runtime, rag_replay_operator;

CREATE OR REPLACE FUNCTION knowledge_stage_parse_projection(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID,
  p_artifact_set_id UUID,
  p_source_sha256 TEXT,
  p_chunk_profile_hash TEXT,
  p_blocks JSONB,
  p_parent_chunks JSONB,
  p_child_chunks JSONB,
  p_chunk_block_spans JSONB,
  p_child_search_projections JSONB
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  job knowledge_processing_jobs%ROWTYPE;
  materialization knowledge_document_materializations%ROWTYPE;
  generation knowledge_index_generations%ROWTYPE;
  index_profile knowledge_index_profiles%ROWTYPE;
  search_profile knowledge_search_profiles%ROWTYPE;
  block_count INTEGER;
  parent_count INTEGER;
  child_count INTEGER;
  span_count INTEGER;
  search_count INTEGER;
  staged_count INTEGER;
BEGIN
  IF p_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
    OR p_artifact_set_id IS NULL
    OR p_source_sha256 IS NULL OR p_source_sha256 !~ '^[0-9a-f]{64}$'
    OR p_chunk_profile_hash IS NULL OR p_chunk_profile_hash !~ '^[0-9a-f]{64}$'
    OR p_blocks IS NULL OR jsonb_typeof(p_blocks) <> 'array'
    OR p_parent_chunks IS NULL OR jsonb_typeof(p_parent_chunks) <> 'array'
    OR p_child_chunks IS NULL OR jsonb_typeof(p_child_chunks) <> 'array'
    OR p_chunk_block_spans IS NULL OR jsonb_typeof(p_chunk_block_spans) <> 'array'
    OR p_child_search_projections IS NULL
    OR jsonb_typeof(p_child_search_projections) <> 'array'
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PARSE_PROJECTION_ARGUMENT_INVALID';
  END IF;

  block_count := jsonb_array_length(p_blocks);
  parent_count := jsonb_array_length(p_parent_chunks);
  child_count := jsonb_array_length(p_child_chunks);
  span_count := jsonb_array_length(p_chunk_block_spans);
  search_count := jsonb_array_length(p_child_search_projections);
  IF block_count < 1 OR parent_count < 1 OR child_count < 1
    OR search_count < 1 OR search_count <> child_count
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PARSE_PROJECTION_BATCH_INVALID';
  END IF;

  SELECT processing_job.* INTO job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'parse'
    AND processing_job.operation IN ('initial', 'replace', 'reprocess')
    AND NOT processing_job.legacy_projection_unbound
    AND processing_job.lease_owner = p_worker_id
    AND processing_job.lease_token = p_lease_token
    AND processing_job.lease_expires_at > clock_timestamp()
    AND processing_job.materialization_id = p_materialization_id
    AND processing_job.index_generation_id IS NOT NULL
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  SELECT * INTO materialization
  FROM knowledge_document_materializations
  WHERE id = p_materialization_id
    AND index_generation_id = job.index_generation_id
    AND collection_id = job.collection_id
    AND document_id = job.document_id
    AND document_version_id = job.document_version_id
    AND file_id = job.file_id
    AND source_content_hash = p_source_sha256
    AND collection_acl_revision = job.collection_acl_revision
    AND collection_visibility_epoch = job.collection_visibility_epoch
    AND collection_processing_revision = job.collection_processing_revision
    AND document_visibility_epoch = job.document_visibility_epoch
    AND status = 'staging'
    AND (
      parse_artifact_set_id IS NULL OR parse_artifact_set_id = p_artifact_set_id
    )
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_PROJECTION_MATERIALIZATION_MISMATCH';
  END IF;

  SELECT * INTO generation
  FROM knowledge_index_generations
  WHERE id = job.index_generation_id
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_PROJECTION_GENERATION_MISSING';
  END IF;

  SELECT * INTO index_profile
  FROM knowledge_index_profiles
  WHERE id = generation.index_profile_id
    AND chunk_profile_hash = p_chunk_profile_hash
    AND embedding_role = 'passage'
    AND (
      (embedding_processor = 'jina' AND embedding_model_id = 'jina-embeddings-v4')
      OR (embedding_processor = 'siliconflow' AND embedding_model_id = 'Pro/BAAI/bge-m3')
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_PROJECTION_PROFILE_MISMATCH';
  END IF;

  SELECT profile.* INTO search_profile
  FROM knowledge_search_profiles profile
  WHERE profile.index_profile_id = index_profile.id
    AND profile.embedding_processor = index_profile.embedding_processor
    AND profile.embedding_model_id = index_profile.embedding_model_id
    AND profile.rerank_processor = index_profile.rerank_processor
    AND profile.rerank_model_id = index_profile.rerank_model_id
    AND knowledge_retrieval_profile_id(
      profile.provider_profile_id, profile.embedding_processor,
      profile.embedding_model_id, profile.embedding_dimensions,
      profile.rerank_processor, profile.rerank_model_id
    ) IS NOT NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_SEARCH_PROFILE_MISSING';
  END IF;

  INSERT INTO knowledge_parser_artifact_sets (
    id, document_id, document_version_id, file_id, index_profile_id,
    parser_kind, parser_version, source_content_hash, config_hash,
    manifest_hash, status, quality_report
  ) VALUES (
    p_artifact_set_id, job.document_id, job.document_version_id, job.file_id,
    index_profile.id, job.processor,
    COALESCE(NULLIF(job.model_id, ''), job.processor), p_source_sha256,
    index_profile.parser_manifest_hash, p_chunk_profile_hash, 'staging',
    jsonb_build_object(
      'schemaVersion', 'g7.5-parse-projection-stage.v1',
      'stagedBy', 'knowledge_stage_parse_projection'
    )
  ) ON CONFLICT (id) DO NOTHING;

  PERFORM 1
  FROM knowledge_parser_artifact_sets artifact_set
  WHERE artifact_set.id = p_artifact_set_id
    AND artifact_set.document_id = job.document_id
    AND artifact_set.document_version_id = job.document_version_id
    AND artifact_set.file_id = job.file_id
    AND artifact_set.index_profile_id = index_profile.id
    AND artifact_set.source_content_hash = p_source_sha256
    AND artifact_set.status IN ('staging', 'verified');
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_ARTIFACT_SET_MISMATCH';
  END IF;

  UPDATE knowledge_document_materializations
  SET parse_artifact_set_id = p_artifact_set_id
  WHERE id = materialization.id
    AND (
      parse_artifact_set_id IS NULL OR parse_artifact_set_id = p_artifact_set_id
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_PROJECTION_MATERIALIZATION_MISMATCH';
  END IF;

  INSERT INTO knowledge_blocks (
    id, artifact_set_id, document_id, document_version_id, parent_block_id,
    ordinal, block_type, heading_path, text_content, locator_kind, locator,
    reading_order, provenance, confidence, content_hash, source_span_hash,
    derived, non_indexable, needs_review
  )
  SELECT
    block.id, block.artifact_set_id, block.document_id,
    block.document_version_id, block.parent_block_id, block.ordinal,
    block.block_type, block.heading_path, block.text_content, block.locator_kind,
    block.locator, block.reading_order, block.provenance, block.confidence,
    block.content_hash, block.source_span_hash, block.derived,
    block.non_indexable, block.needs_review
  FROM jsonb_to_recordset(p_blocks) AS block(
    id UUID, artifact_set_id UUID, document_id UUID, document_version_id UUID,
    parent_block_id UUID, ordinal BIGINT, block_type TEXT, heading_path TEXT[],
    text_content TEXT, locator_kind TEXT, locator JSONB, reading_order BIGINT,
    provenance JSONB, confidence NUMERIC, content_hash TEXT,
    source_span_hash TEXT, derived BOOLEAN, non_indexable BOOLEAN,
    needs_review BOOLEAN
  )
  WHERE block.artifact_set_id = p_artifact_set_id
    AND block.document_id = job.document_id
    AND block.document_version_id = job.document_version_id
  ORDER BY block.ordinal, block.id
  ON CONFLICT DO NOTHING;

  SELECT count(*)::INTEGER INTO staged_count
  FROM knowledge_blocks block
  WHERE block.artifact_set_id = p_artifact_set_id;
  IF staged_count <> block_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_BLOCK_PROJECTION_MISMATCH';
  END IF;

  INSERT INTO knowledge_parent_chunks (
    id, materialization_id, index_generation_id, document_id,
    document_version_id, ordinal, chunk_profile_hash, source_span_hash,
    content_hash, content, token_count, heading_path, locator_summary
  )
  SELECT
    parent.id, parent.materialization_id, parent.index_generation_id,
    parent.document_id, parent.document_version_id, parent.ordinal,
    parent.chunk_profile_hash, parent.source_span_hash, parent.content_hash,
    parent.content, parent.token_count, parent.heading_path,
    parent.locator_summary
  FROM jsonb_to_recordset(p_parent_chunks) AS parent(
    id UUID, materialization_id UUID, index_generation_id UUID,
    document_id UUID, document_version_id UUID, ordinal BIGINT,
    chunk_profile_hash TEXT, source_span_hash TEXT, content_hash TEXT,
    content TEXT, token_count INTEGER, heading_path TEXT[],
    locator_summary JSONB
  )
  WHERE parent.materialization_id = p_materialization_id
    AND parent.index_generation_id = job.index_generation_id
    AND parent.document_id = job.document_id
    AND parent.document_version_id = job.document_version_id
    AND parent.chunk_profile_hash = p_chunk_profile_hash
  ON CONFLICT DO NOTHING;

  SELECT count(*)::INTEGER INTO staged_count
  FROM knowledge_parent_chunks parent
  WHERE parent.materialization_id = p_materialization_id;
  IF staged_count <> parent_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_PARENT_CHUNK_PROJECTION_MISMATCH';
  END IF;

  INSERT INTO knowledge_child_chunks (
    id, parent_chunk_id, materialization_id, index_generation_id, document_id,
    document_version_id, ordinal, chunk_profile_hash, source_span_hash,
    content_hash, content, token_count, overlap_before_tokens,
    overlap_after_tokens
  )
  SELECT
    child.id, child.parent_chunk_id, child.materialization_id,
    child.index_generation_id, child.document_id, child.document_version_id,
    child.ordinal, child.chunk_profile_hash, child.source_span_hash,
    child.content_hash, child.content, child.token_count,
    child.overlap_before_tokens, child.overlap_after_tokens
  FROM jsonb_to_recordset(p_child_chunks) AS child(
    id UUID, parent_chunk_id UUID, materialization_id UUID,
    index_generation_id UUID, document_id UUID, document_version_id UUID,
    ordinal BIGINT, chunk_profile_hash TEXT, source_span_hash TEXT,
    content_hash TEXT, content TEXT, token_count INTEGER,
    overlap_before_tokens INTEGER, overlap_after_tokens INTEGER
  )
  WHERE child.materialization_id = p_materialization_id
    AND child.index_generation_id = job.index_generation_id
    AND child.document_id = job.document_id
    AND child.document_version_id = job.document_version_id
    AND child.chunk_profile_hash = p_chunk_profile_hash
  ON CONFLICT DO NOTHING;

  SELECT count(*)::INTEGER INTO staged_count
  FROM knowledge_child_chunks child
  WHERE child.materialization_id = p_materialization_id;
  IF staged_count <> child_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_CHILD_CHUNK_PROJECTION_MISMATCH';
  END IF;

  INSERT INTO knowledge_chunk_block_spans (
    chunk_kind, chunk_id, block_id, span_ordinal, start_offset, end_offset
  )
  SELECT
    span.chunk_kind, span.chunk_id, span.block_id, span.span_ordinal,
    span.start_offset, span.end_offset
  FROM jsonb_to_recordset(p_chunk_block_spans) AS span(
    chunk_kind TEXT, chunk_id UUID, block_id UUID, span_ordinal INTEGER,
    start_offset BIGINT, end_offset BIGINT, fragment_source_span_hash TEXT
  )
  WHERE span.chunk_kind IN ('parent', 'child')
  ON CONFLICT DO NOTHING;

  SELECT count(*)::INTEGER INTO staged_count
  FROM knowledge_chunk_block_spans span
  WHERE (
    span.chunk_kind = 'parent'
    AND EXISTS (
      SELECT 1 FROM knowledge_parent_chunks parent
      WHERE parent.id = span.chunk_id
        AND parent.materialization_id = p_materialization_id
    )
  ) OR (
    span.chunk_kind = 'child'
    AND EXISTS (
      SELECT 1 FROM knowledge_child_chunks child
      WHERE child.id = span.chunk_id
        AND child.materialization_id = p_materialization_id
    )
  );
  IF staged_count <> span_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_CHUNK_SPAN_PROJECTION_MISMATCH';
  END IF;

  INSERT INTO knowledge_child_search_projections (
    child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
    collection_id, document_id, document_version_id, search_profile_id,
    embedding_model_id, embedding_dimensions, lexical_text, exact_terms,
    source_span_hash, chunk_profile_hash, content_hash, locator_summary
  )
  SELECT
    search.child_chunk_id, search.parent_chunk_id, search.materialization_id,
    search.index_generation_id, search.collection_id, search.document_id,
    search.document_version_id, search_profile.id, search.embedding_model_id,
    search.embedding_dimensions, search.lexical_text, search.exact_terms,
    search.source_span_hash, search.chunk_profile_hash, search.content_hash,
    search.locator_summary
  FROM jsonb_to_recordset(p_child_search_projections) AS search(
    child_chunk_id UUID, parent_chunk_id UUID, materialization_id UUID,
    index_generation_id UUID, collection_id UUID, document_id UUID,
    document_version_id UUID, embedding_model_id TEXT,
    embedding_dimensions INTEGER, lexical_text TEXT, exact_terms TEXT[],
    source_span_hash TEXT, chunk_profile_hash TEXT, content_hash TEXT,
    locator_summary JSONB
  )
  WHERE search.materialization_id = p_materialization_id
    AND search.index_generation_id = job.index_generation_id
    AND search.collection_id = job.collection_id
    AND search.document_id = job.document_id
    AND search.document_version_id = job.document_version_id
    AND search.embedding_model_id = search_profile.embedding_model_id
    AND search.embedding_dimensions = search_profile.embedding_dimensions
    AND search.chunk_profile_hash = p_chunk_profile_hash
  ON CONFLICT DO NOTHING;

  SELECT count(*)::INTEGER INTO staged_count
  FROM knowledge_child_search_projections search
  WHERE search.materialization_id = p_materialization_id;
  IF staged_count <> search_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_SEARCH_PROJECTION_MISMATCH';
  END IF;

  RETURN true;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_complete_parse_and_enqueue_embedding(
  p_parse_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID,
  p_embedding_job_id UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  parse_job knowledge_processing_jobs%ROWTYPE;
  materialization knowledge_document_materializations%ROWTYPE;
  generation knowledge_index_generations%ROWTYPE;
  index_profile knowledge_index_profiles%ROWTYPE;
  embedding_profile processor_governance_profiles%ROWTYPE;
  embedding_head processor_governance_heads%ROWTYPE;
  embedding_consent processing_consents%ROWTYPE;
  staged_search_count BIGINT;
BEGIN
  IF p_parse_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
    OR p_embedding_job_id IS NULL
    OR p_embedding_job_id = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_embedding_job_id = p_parse_job_id
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PARSE_COMPLETION_ARGUMENT_INVALID';
  END IF;

  SELECT processing_job.* INTO parse_job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_parse_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'parse'
    AND processing_job.operation IN ('initial', 'replace', 'reprocess')
    AND NOT processing_job.legacy_projection_unbound
    AND processing_job.lease_owner = p_worker_id
    AND processing_job.lease_token = p_lease_token
    AND processing_job.lease_expires_at > clock_timestamp()
    AND processing_job.materialization_id = p_materialization_id
    AND processing_job.index_generation_id IS NOT NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  SELECT * INTO materialization
  FROM knowledge_document_materializations
  WHERE id = p_materialization_id
    AND index_generation_id = parse_job.index_generation_id
    AND collection_id = parse_job.collection_id
    AND document_id = parse_job.document_id
    AND document_version_id = parse_job.document_version_id
    AND file_id = parse_job.file_id
    AND collection_acl_revision = parse_job.collection_acl_revision
    AND collection_visibility_epoch = parse_job.collection_visibility_epoch
    AND collection_processing_revision = parse_job.collection_processing_revision
    AND document_visibility_epoch = parse_job.document_visibility_epoch
    AND status = 'staging'
    AND parse_artifact_set_id IS NOT NULL
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_MATERIALIZATION_MISSING';
  END IF;

  SELECT count(*) INTO staged_search_count
  FROM knowledge_child_search_projections search
  WHERE search.materialization_id = p_materialization_id
    AND search.index_generation_id = parse_job.index_generation_id
    AND search.collection_id = parse_job.collection_id
    AND search.document_id = parse_job.document_id
    AND search.document_version_id = parse_job.document_version_id
    AND search.embedding_model_id IN ('jina-embeddings-v4', 'Pro/BAAI/bge-m3')
    AND search.embedding_dimensions = 1024
    AND search.status = 'staging'
    AND search.embedding_vector IS NULL
    AND search.embedding_vector_sha256 IS NULL;
  IF staged_search_count < 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_SEARCH_STAGING_MISSING';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_processing_jobs existing_job
    WHERE existing_job.stage = 'passage_embedding'
      AND existing_job.status <> 'cancelled'
      AND (
        existing_job.materialization_id = p_materialization_id
        OR existing_job.caused_by_job_id = p_parse_job_id
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_EMBEDDING_JOB_EXISTS';
  END IF;

  SELECT * INTO generation
  FROM knowledge_index_generations
  WHERE id = parse_job.index_generation_id
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_GENERATION_MISSING';
  END IF;

  SELECT * INTO index_profile
  FROM knowledge_index_profiles
  WHERE id = generation.index_profile_id
    AND embedding_role = 'passage'
    AND (
      (embedding_processor = 'jina' AND embedding_model_id = 'jina-embeddings-v4')
      OR (embedding_processor = 'siliconflow' AND embedding_model_id = 'Pro/BAAI/bge-m3')
    );
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_PROFILE_MISMATCH';
  END IF;

  SELECT head.* INTO embedding_head
  FROM processor_governance_heads head
  WHERE head.processor = index_profile.embedding_processor
    AND head.endpoint_id = index_profile.embedding_endpoint_id
    AND head.model_id = index_profile.embedding_model_id
    AND head.status = 'active'
    AND head.active_profile_id IS NOT NULL
    AND head.active_governance_revision IS NOT NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_GOVERNANCE_HEAD_MISSING';
  END IF;

  SELECT profile.* INTO embedding_profile
  FROM processor_governance_profiles profile
  WHERE profile.processor = embedding_head.processor
    AND profile.endpoint_id = embedding_head.endpoint_id
    AND profile.model_id = embedding_head.model_id
    AND profile.id = embedding_head.active_profile_id
    AND profile.governance_revision = embedding_head.active_governance_revision
    AND profile.status = 'approved'
    AND 'passage_embedding' = ANY(profile.allowed_purposes)
    AND 'text/plain' = ANY(profile.allowed_data_types);
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_GOVERNANCE_PROFILE_MISSING';
  END IF;

  SELECT consent.* INTO embedding_consent
  FROM processing_consents consent
  WHERE consent.scope = 'collection'
    AND consent.collection_id = parse_job.collection_id
    AND consent.processor = embedding_profile.processor
    AND consent.endpoint_id = embedding_profile.endpoint_id
    AND consent.model_id = embedding_profile.model_id
    AND consent.governance_profile_id = embedding_profile.id
    AND consent.governance_revision = embedding_profile.governance_revision
    AND consent.governance_head_revision = embedding_head.head_revision
    AND consent.decision = 'granted'
    AND consent.superseded_at IS NULL
    AND (consent.expires_at IS NULL OR consent.expires_at > clock_timestamp())
    AND 'passage_embedding' = ANY(consent.purposes)
    AND 'text/plain' = ANY(consent.data_types)
  ORDER BY consent.consent_revision DESC
  LIMIT 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_CONSENT_MISSING';
  END IF;

  INSERT INTO knowledge_processing_jobs (
    id, collection_id, document_id, document_version_id, file_id, stage,
    operation, processor, endpoint_id, model_id, governance_profile_id,
    governance_revision, governance_head_revision, collection_consent_id,
    collection_consent_revision, collection_acl_revision,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch, requested_by_user_id, caused_by_job_id,
    idempotency_scope, idempotency_key, request_hash, status, attempt_count,
    max_attempts, available_at, index_generation_id, materialization_id,
    legacy_projection_unbound
  ) VALUES (
    p_embedding_job_id, parse_job.collection_id, parse_job.document_id,
    parse_job.document_version_id, parse_job.file_id, 'passage_embedding',
    parse_job.operation, embedding_profile.processor, embedding_profile.endpoint_id,
    embedding_profile.model_id, embedding_profile.id,
    embedding_profile.governance_revision, embedding_head.head_revision,
    embedding_consent.id, embedding_consent.consent_revision,
    parse_job.collection_acl_revision, parse_job.collection_visibility_epoch,
    parse_job.collection_processing_revision, parse_job.document_visibility_epoch,
    parse_job.requested_by_user_id, parse_job.id,
    'rag:passage_embedding:' || p_materialization_id::TEXT,
    p_embedding_job_id::TEXT,
    encode(sha256(convert_to(
      'passage_embedding:' || p_materialization_id::TEXT || ':' || p_parse_job_id::TEXT,
      'UTF8'
    )), 'hex'),
    'pending', 0, parse_job.max_attempts, clock_timestamp(),
    parse_job.index_generation_id, p_materialization_id, false
  );

  UPDATE knowledge_processing_jobs
  SET status = 'succeeded',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = clock_timestamp(),
      error_code = NULL,
      updated_at = clock_timestamp()
  WHERE id = parse_job.id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  RETURN true;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_stage_passage_embedding(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID,
  p_child_chunk_id UUID,
  p_embedding_vector REAL[],
  p_embedding_vector_sha256 TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  job knowledge_processing_jobs%ROWTYPE;
BEGIN
  IF p_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
    OR p_child_chunk_id IS NULL
    OR p_embedding_vector IS NULL
    OR cardinality(p_embedding_vector) <> 1024
    OR array_position(p_embedding_vector, NULL) IS NOT NULL
    OR p_embedding_vector_sha256 IS NULL
    OR p_embedding_vector_sha256 !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PASSAGE_EMBEDDING_ARGUMENT_INVALID';
  END IF;

  SELECT processing_job.* INTO job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'passage_embedding'
    AND processing_job.operation IN ('initial', 'replace', 'reprocess')
    AND NOT processing_job.legacy_projection_unbound
    AND processing_job.lease_owner = p_worker_id
    AND processing_job.lease_token = p_lease_token
    AND processing_job.lease_expires_at > clock_timestamp()
    AND processing_job.materialization_id = p_materialization_id
    AND processing_job.index_generation_id IS NOT NULL
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  UPDATE knowledge_child_search_projections search
  SET embedding_vector = p_embedding_vector,
      embedding_vector_sha256 = p_embedding_vector_sha256,
      status = 'ready',
      ready_at = COALESCE(search.ready_at, clock_timestamp())
  FROM knowledge_child_chunks child
  WHERE search.child_chunk_id = p_child_chunk_id
    AND search.child_chunk_id = child.id
    AND search.materialization_id = child.materialization_id
    AND search.index_generation_id = child.index_generation_id
    AND search.document_id = child.document_id
    AND search.document_version_id = child.document_version_id
    AND search.source_span_hash = child.source_span_hash
    AND search.chunk_profile_hash = child.chunk_profile_hash
    AND search.content_hash = child.content_hash
    AND search.materialization_id = p_materialization_id
    AND search.index_generation_id = job.index_generation_id
    AND search.document_id = job.document_id
    AND search.document_version_id = job.document_version_id
    AND search.embedding_model_id = job.model_id
    AND search.embedding_dimensions = 1024
    AND (
      (job.processor = 'jina' AND job.model_id = 'jina-embeddings-v4')
      OR (job.processor = 'siliconflow' AND job.model_id = 'Pro/BAAI/bge-m3')
    )
    AND search.status IN ('staging', 'ready');
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PASSAGE_EMBEDDING_TARGET_MISSING';
  END IF;

  RETURN true;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_assert_materialization_search_complete(
  p_materialization_id UUID,
  p_expected_child_count BIGINT,
  p_expected_embedding_model_id TEXT,
  p_expected_embedding_dimensions INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  materialization knowledge_document_materializations%ROWTYPE;
  child_count BIGINT;
  ready_count BIGINT;
BEGIN
  IF p_materialization_id IS NULL
    OR p_expected_child_count IS NULL OR p_expected_child_count < 0
    OR p_expected_embedding_model_id NOT IN ('jina-embeddings-v4', 'Pro/BAAI/bge-m3')
    OR p_expected_embedding_dimensions <> 1024
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_SEARCH_COMPLETENESS_ARGUMENT_INVALID';
  END IF;

  SELECT * INTO materialization
  FROM knowledge_document_materializations
  WHERE id = p_materialization_id
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_SEARCH_MATERIALIZATION_MISSING';
  END IF;

  SELECT count(*) INTO child_count
  FROM knowledge_child_chunks child
  WHERE child.materialization_id = materialization.id;
  IF child_count <> p_expected_child_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_SEARCH_CHILD_COUNT_MISMATCH';
  END IF;

  SELECT count(*) INTO ready_count
  FROM knowledge_child_chunks child
  JOIN knowledge_child_search_projections search
    ON search.child_chunk_id = child.id
    AND search.materialization_id = child.materialization_id
    AND search.index_generation_id = child.index_generation_id
    AND search.document_id = child.document_id
    AND search.document_version_id = child.document_version_id
    AND search.source_span_hash = child.source_span_hash
    AND search.chunk_profile_hash = child.chunk_profile_hash
    AND search.content_hash = child.content_hash
  WHERE child.materialization_id = materialization.id
    AND search.status = 'ready'
    AND search.embedding_model_id = p_expected_embedding_model_id
    AND search.embedding_dimensions = p_expected_embedding_dimensions
    AND EXISTS (
      SELECT 1 FROM knowledge_search_profiles profile
      JOIN knowledge_index_generations generation
        ON generation.index_profile_id = profile.index_profile_id
       AND generation.id = search.index_generation_id
      WHERE profile.id = search.search_profile_id
        AND profile.embedding_model_id = search.embedding_model_id
        AND profile.embedding_dimensions = search.embedding_dimensions
        AND knowledge_retrieval_profile_id(
          profile.provider_profile_id, profile.embedding_processor,
          profile.embedding_model_id, profile.embedding_dimensions,
          profile.rerank_processor, profile.rerank_model_id
        ) IS NOT NULL
    )
    AND search.embedding_vector IS NOT NULL
    AND cardinality(search.embedding_vector) = p_expected_embedding_dimensions;

  IF ready_count <> child_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_SEARCH_PROJECTION_INCOMPLETE';
  END IF;

  RETURN true;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_complete_embedding_and_publish(
  p_embedding_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  embedding_job knowledge_processing_jobs%ROWTYPE;
  materialization knowledge_document_materializations%ROWTYPE;
  previous_current_version_id UUID;
  child_count BIGINT;
  corpus_revision BIGINT;
  computed_manifest_hash TEXT;
  computed_result_hash TEXT;
BEGIN
  IF p_embedding_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_ARGUMENT_INVALID';
  END IF;

  SELECT processing_job.* INTO embedding_job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_embedding_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'passage_embedding'
    AND processing_job.operation IN ('initial', 'replace', 'reprocess')
    AND (
      (processing_job.processor = 'jina' AND processing_job.model_id = 'jina-embeddings-v4')
      OR (processing_job.processor = 'siliconflow' AND processing_job.model_id = 'Pro/BAAI/bge-m3')
    )
    AND NOT processing_job.legacy_projection_unbound
    AND processing_job.lease_owner = p_worker_id
    AND processing_job.lease_token = p_lease_token
    AND processing_job.lease_expires_at > clock_timestamp()
    AND processing_job.materialization_id = p_materialization_id
    AND processing_job.index_generation_id IS NOT NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  SELECT * INTO materialization
  FROM knowledge_document_materializations
  WHERE id = p_materialization_id
    AND index_generation_id = embedding_job.index_generation_id
    AND collection_id = embedding_job.collection_id
    AND document_id = embedding_job.document_id
    AND document_version_id = embedding_job.document_version_id
    AND file_id = embedding_job.file_id
    AND collection_acl_revision = embedding_job.collection_acl_revision
    AND collection_visibility_epoch = embedding_job.collection_visibility_epoch
    AND collection_processing_revision = embedding_job.collection_processing_revision
    AND document_visibility_epoch = embedding_job.document_visibility_epoch
    AND status = 'staging'
    AND parse_artifact_set_id IS NOT NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_MATERIALIZATION_MISSING';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_collections collection
    JOIN knowledge_documents document
      ON document.collection_id = collection.id
     AND document.id = materialization.document_id
    JOIN knowledge_document_versions version
      ON version.document_id = document.id
     AND version.id = materialization.document_version_id
     AND version.file_id = materialization.file_id
    WHERE collection.id = materialization.collection_id
      AND collection.deleted_at IS NULL
      AND document.deleted_at IS NULL
      AND document.status IN ('processing', 'active')
      AND version.status IN ('uploaded', 'processing', 'active')
      AND collection.acl_revision = materialization.collection_acl_revision
      AND collection.visibility_epoch = materialization.collection_visibility_epoch
      AND collection.collection_processing_revision =
        materialization.collection_processing_revision
      AND document.visibility_epoch = materialization.document_visibility_epoch
      AND version.visibility_epoch = materialization.document_visibility_epoch
      AND version.content_hash = materialization.source_content_hash
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_AUTHORITY_STALE';
  END IF;

  SELECT count(*) INTO child_count
  FROM knowledge_child_chunks child
  WHERE child.materialization_id = materialization.id;
  IF child_count < 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_CHILDREN_MISSING';
  END IF;

  PERFORM knowledge_assert_materialization_search_complete(
    materialization.id,
    child_count,
    embedding_job.model_id,
    1024
  );

  SELECT
    encode(sha256(convert_to(
      'g7.5p:manifest:' || materialization.id::TEXT || ':' ||
      materialization.index_generation_id::TEXT || ':' ||
      materialization.document_id::TEXT || ':' ||
      materialization.document_version_id::TEXT || ':' ||
      materialization.parse_artifact_set_id::TEXT || ':' ||
      materialization.source_content_hash || ':' ||
      string_agg(
        child.id::TEXT || ':' || child.content_hash || ':' ||
        search.embedding_vector_sha256,
        ',' ORDER BY child.ordinal, child.id
      ),
      'UTF8'
    )), 'hex'),
    encode(sha256(convert_to(
      'g7.5p:result:' || materialization.id::TEXT || ':' ||
      materialization.base_profile_hash || ':' || child_count::TEXT || ':' ||
      string_agg(
        search.child_chunk_id::TEXT || ':' || search.source_span_hash || ':' ||
        search.content_hash || ':' || search.embedding_vector_sha256,
        ',' ORDER BY child.ordinal, child.id
      ),
      'UTF8'
    )), 'hex')
  INTO computed_manifest_hash, computed_result_hash
  FROM knowledge_child_chunks child
  JOIN knowledge_child_search_projections search
    ON search.child_chunk_id = child.id
   AND search.materialization_id = child.materialization_id
   AND search.index_generation_id = child.index_generation_id
   AND search.document_id = child.document_id
   AND search.document_version_id = child.document_version_id
   AND search.source_span_hash = child.source_span_hash
   AND search.chunk_profile_hash = child.chunk_profile_hash
   AND search.content_hash = child.content_hash
  WHERE child.materialization_id = materialization.id
    AND search.status = 'ready'
    AND search.embedding_model_id = embedding_job.model_id
    AND search.embedding_dimensions = 1024
    AND search.embedding_vector IS NOT NULL
    AND search.embedding_vector_sha256 IS NOT NULL;

  IF computed_manifest_hash IS NULL OR computed_result_hash IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_HASH_MISSING';
  END IF;

  SELECT document.current_version_id INTO previous_current_version_id
  FROM knowledge_documents document
  WHERE document.id = materialization.document_id
    AND document.collection_id = materialization.collection_id
  FOR UPDATE;

  UPDATE knowledge_document_versions version
  SET status = 'tombstoned',
      updated_at = clock_timestamp()
  WHERE version.document_id = materialization.document_id
    AND version.id = previous_current_version_id
    AND version.id <> materialization.document_version_id
    AND version.status = 'active';

  UPDATE knowledge_document_versions version
  SET status = 'active',
      error_code = NULL,
      updated_at = clock_timestamp()
  WHERE version.document_id = materialization.document_id
    AND version.id = materialization.document_version_id
    AND version.file_id = materialization.file_id
    AND version.status IN ('uploaded', 'processing', 'active')
    AND version.content_hash = materialization.source_content_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_VERSION_STALE';
  END IF;

  UPDATE knowledge_documents document
  SET status = 'active',
      current_version_id = materialization.document_version_id,
      updated_at = clock_timestamp()
  WHERE document.id = materialization.document_id
    AND document.collection_id = materialization.collection_id
    AND document.status IN ('processing', 'active')
    AND document.deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_DOCUMENT_STALE';
  END IF;

  SELECT corpus_projection_revision INTO corpus_revision
  FROM knowledge_corpus_projection_head
  WHERE singleton_id = 1
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_CORPUS_HEAD_MISSING';
  END IF;

  UPDATE knowledge_document_materializations
  SET status = 'published',
      manifest_hash = computed_manifest_hash,
      result_hash = computed_result_hash,
      verified_at = clock_timestamp(),
      published_at = clock_timestamp()
  WHERE id = materialization.id
    AND status = 'staging';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_MATERIALIZATION_STALE';
  END IF;

  INSERT INTO knowledge_document_projection_heads(
    index_generation_id,
    document_id,
    active_materialization_id,
    document_projection_revision,
    last_corpus_projection_revision,
    updated_at
  ) VALUES (
    materialization.index_generation_id,
    materialization.document_id,
    materialization.id,
    1,
    corpus_revision + 1,
    clock_timestamp()
  ) ON CONFLICT (index_generation_id, document_id) DO UPDATE
    SET active_materialization_id = EXCLUDED.active_materialization_id,
        document_projection_revision =
          knowledge_document_projection_heads.document_projection_revision + 1,
        last_corpus_projection_revision = EXCLUDED.last_corpus_projection_revision,
        updated_at = clock_timestamp();

  UPDATE knowledge_corpus_projection_head
  SET corpus_projection_revision = corpus_projection_revision + 1,
      updated_at = clock_timestamp()
  WHERE singleton_id = 1;

  UPDATE knowledge_processing_jobs
  SET status = 'succeeded',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = clock_timestamp(),
      error_code = NULL,
      updated_at = clock_timestamp()
  WHERE id = embedding_job.id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  RETURN true;
END
$function$;

CREATE OR REPLACE VIEW knowledge_pgvector_shadow_sources
WITH (security_barrier = true)
AS
SELECT
  search.child_chunk_id,
  search.parent_chunk_id,
  search.materialization_id,
  search.index_generation_id,
  search.collection_id,
  search.document_id,
  search.document_version_id,
  search.search_profile_id,
  search.embedding_model_id,
  search.embedding_dimensions,
  search.embedding_vector,
  search.embedding_vector_sha256,
  search.source_span_hash,
  search.chunk_profile_hash,
  search.content_hash,
  materialization.collection_visibility_epoch,
  materialization.collection_processing_revision,
  materialization.document_visibility_epoch
FROM knowledge_child_search_projections search
JOIN knowledge_search_profiles search_profile
  ON search_profile.id = search.search_profile_id
 AND knowledge_retrieval_profile_id(
   search_profile.provider_profile_id, search_profile.embedding_processor,
   search_profile.embedding_model_id, search_profile.embedding_dimensions,
   search_profile.rerank_processor, search_profile.rerank_model_id
 ) IS NOT NULL
JOIN knowledge_index_generations generation
  ON generation.id = search.index_generation_id
 AND generation.index_profile_id = search_profile.index_profile_id
 AND generation.status IN ('building', 'verified', 'active', 'retired')
JOIN knowledge_document_projection_heads document_head
  ON document_head.index_generation_id = search.index_generation_id
 AND document_head.document_id = search.document_id
 AND document_head.active_materialization_id = search.materialization_id
JOIN knowledge_document_materializations materialization
  ON materialization.id = search.materialization_id
 AND materialization.index_generation_id = search.index_generation_id
 AND materialization.collection_id = search.collection_id
 AND materialization.document_id = search.document_id
 AND materialization.document_version_id = search.document_version_id
 AND materialization.status = 'published'
JOIN knowledge_collections collection
  ON collection.id = search.collection_id
 AND collection.deleted_at IS NULL
 AND collection.visibility_epoch = materialization.collection_visibility_epoch
 AND collection.collection_processing_revision =
   materialization.collection_processing_revision
JOIN knowledge_documents document
  ON document.id = search.document_id
 AND document.collection_id = search.collection_id
 AND document.status = 'active'
 AND document.deleted_at IS NULL
 AND document.current_version_id = search.document_version_id
 AND document.visibility_epoch = materialization.document_visibility_epoch
JOIN knowledge_document_versions version
  ON version.id = search.document_version_id
 AND version.document_id = search.document_id
 AND version.status = 'active'
 AND version.content_hash = materialization.source_content_hash
JOIN knowledge_child_chunks child
  ON child.id = search.child_chunk_id
 AND child.parent_chunk_id = search.parent_chunk_id
 AND child.materialization_id = search.materialization_id
 AND child.index_generation_id = search.index_generation_id
 AND child.document_id = search.document_id
 AND child.document_version_id = search.document_version_id
 AND child.source_span_hash = search.source_span_hash
 AND child.chunk_profile_hash = search.chunk_profile_hash
 AND child.content_hash = search.content_hash
WHERE search.status = 'ready'
  AND search.embedding_model_id = search_profile.embedding_model_id
  AND search.embedding_dimensions = search_profile.embedding_dimensions
  AND search.embedding_vector IS NOT NULL
  AND search.embedding_vector_sha256 IS NOT NULL
  AND cardinality(search.embedding_vector) = 1024;

CREATE OR REPLACE VIEW knowledge_bm25_shadow_build_sources
AS
SELECT
  search.child_chunk_id,
  search.parent_chunk_id,
  search.materialization_id,
  search.index_generation_id,
  search.collection_id,
  search.document_id,
  search.document_version_id,
  search.search_profile_id,
  search.lexical_text,
  search.exact_terms,
  search.source_span_hash,
  search.chunk_profile_hash,
  search.content_hash,
  child.ordinal AS child_ordinal,
  materialization.collection_visibility_epoch,
  materialization.collection_processing_revision,
  materialization.document_visibility_epoch
FROM knowledge_child_search_projections search
JOIN knowledge_search_profiles search_profile
  ON search_profile.id = search.search_profile_id
 AND knowledge_retrieval_profile_id(
   search_profile.provider_profile_id, search_profile.embedding_processor,
   search_profile.embedding_model_id, search_profile.embedding_dimensions,
   search_profile.rerank_processor, search_profile.rerank_model_id
 ) IS NOT NULL
JOIN knowledge_index_generations generation
  ON generation.id = search.index_generation_id
 AND generation.index_profile_id = search_profile.index_profile_id
 AND generation.status IN ('building', 'verified', 'active', 'retired')
JOIN knowledge_document_projection_heads document_head
  ON document_head.index_generation_id = search.index_generation_id
 AND document_head.document_id = search.document_id
 AND document_head.active_materialization_id = search.materialization_id
JOIN knowledge_document_materializations materialization
  ON materialization.id = search.materialization_id
 AND materialization.index_generation_id = search.index_generation_id
 AND materialization.collection_id = search.collection_id
 AND materialization.document_id = search.document_id
 AND materialization.document_version_id = search.document_version_id
 AND materialization.status = 'published'
JOIN knowledge_collections collection
  ON collection.id = search.collection_id
 AND collection.deleted_at IS NULL
 AND collection.visibility_epoch = materialization.collection_visibility_epoch
 AND collection.collection_processing_revision =
   materialization.collection_processing_revision
JOIN knowledge_documents document
  ON document.id = search.document_id
 AND document.collection_id = search.collection_id
 AND document.status = 'active'
 AND document.deleted_at IS NULL
 AND document.current_version_id = search.document_version_id
 AND document.visibility_epoch = materialization.document_visibility_epoch
JOIN knowledge_document_versions version
  ON version.id = search.document_version_id
 AND version.document_id = search.document_id
 AND version.status = 'active'
 AND version.content_hash = materialization.source_content_hash
JOIN knowledge_child_chunks child
  ON child.id = search.child_chunk_id
 AND child.parent_chunk_id = search.parent_chunk_id
 AND child.materialization_id = search.materialization_id
 AND child.index_generation_id = search.index_generation_id
 AND child.document_id = search.document_id
 AND child.document_version_id = search.document_version_id
 AND child.source_span_hash = search.source_span_hash
 AND child.chunk_profile_hash = search.chunk_profile_hash
 AND child.content_hash = search.content_hash
WHERE search.status = 'ready'
  AND search.embedding_model_id = search_profile.embedding_model_id
  AND search.embedding_dimensions = search_profile.embedding_dimensions
  AND search.embedding_vector IS NOT NULL
  AND search.embedding_vector_sha256 IS NOT NULL
  AND cardinality(search.embedding_vector) = 1024;

CREATE OR REPLACE VIEW knowledge_bm25_shadow_sources
AS
SELECT source.*
FROM knowledge_bm25_shadow_build_sources source
JOIN knowledge_corpus_projection_head corpus
  ON corpus.active_index_generation_id = source.index_generation_id
JOIN knowledge_index_generations generation
  ON generation.id = source.index_generation_id
 AND generation.status = 'active'
WHERE corpus.singleton_id = 1;


DROP INDEX idx_knowledge_child_vector_shadow_hnsw;
CREATE INDEX idx_knowledge_child_vector_shadow_jina_hnsw
  ON knowledge_child_vector_shadow_projections
  USING hnsw (embedding_vector vector_cosine_ops)
  WITH (m = 16, ef_construction = 64)
  WHERE embedding_model_id = 'jina-embeddings-v4';
CREATE INDEX idx_knowledge_child_vector_shadow_bge_hnsw
  ON knowledge_child_vector_shadow_projections
  USING hnsw (embedding_vector vector_cosine_ops)
  WITH (m = 16, ef_construction = 64)
  WHERE embedding_model_id = 'Pro/BAAI/bge-m3';

CREATE OR REPLACE FUNCTION knowledge_backfill_pgvector_shadow(
  p_index_generation_id UUID,
  p_search_profile_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  inserted_count BIGINT,
  verified_shadow_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  profile_matches INTEGER;
  generation_matches INTEGER;
  v_eligible_count BIGINT;
  v_inserted_count BIGINT;
  v_verified_shadow_count BIGINT;
BEGIN
  IF p_index_generation_id IS NULL OR p_search_profile_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 3);

  SELECT count(*) INTO profile_matches
  FROM knowledge_search_profiles profile
  WHERE profile.id = p_search_profile_id
    AND knowledge_retrieval_profile_id(
      profile.provider_profile_id, profile.embedding_processor,
      profile.embedding_model_id, profile.embedding_dimensions,
      profile.rerank_processor, profile.rerank_model_id
    ) IS NOT NULL;
  SELECT count(*) INTO generation_matches
  FROM knowledge_index_generations generation
  JOIN knowledge_search_profiles profile
    ON profile.id = p_search_profile_id
   AND profile.index_profile_id = generation.index_profile_id
  WHERE generation.id = p_index_generation_id
    AND generation.status IN ('building', 'verified', 'active', 'retired');
  IF profile_matches <> 1 OR generation_matches <> 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_PROFILE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_pgvector_shadow_sources source
    WHERE source.index_generation_id = p_index_generation_id
      AND source.search_profile_id = p_search_profile_id
      AND (
        EXISTS (
          SELECT 1
          FROM unnest(source.embedding_vector) component
          WHERE component::TEXT IN ('NaN', 'Infinity', '-Infinity')
        )
        OR (
          SELECT sqrt(sum(
            component::DOUBLE PRECISION * component::DOUBLE PRECISION
          ))
          FROM unnest(source.embedding_vector) component
        ) <= 0
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_SOURCE_INVALID';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_child_vector_shadow_projections shadow
    JOIN knowledge_pgvector_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE shadow.index_generation_id = p_index_generation_id
      AND shadow.search_profile_id = p_search_profile_id
      AND (
        shadow.index_generation_id <> source.index_generation_id
        OR shadow.search_profile_id <> source.search_profile_id
        OR shadow.embedding_vector_sha256 <>
          source.embedding_vector_sha256
        OR shadow.embedding_vector::REAL[] IS DISTINCT FROM
          source.embedding_vector
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_EXISTING_MISMATCH';
  END IF;

  SELECT count(*) INTO v_eligible_count
  FROM knowledge_pgvector_shadow_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;

  INSERT INTO knowledge_child_vector_shadow_projections (
    child_chunk_id, parent_chunk_id, materialization_id,
    index_generation_id, collection_id, document_id, document_version_id,
    search_profile_id, embedding_model_id, embedding_dimensions,
    embedding_vector, embedding_vector_sha256, embedding_norm,
    source_span_hash, chunk_profile_hash, content_hash,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch
  )
  SELECT
    source.child_chunk_id,
    source.parent_chunk_id,
    source.materialization_id,
    source.index_generation_id,
    source.collection_id,
    source.document_id,
    source.document_version_id,
    source.search_profile_id,
    source.embedding_model_id,
    source.embedding_dimensions,
    source.embedding_vector::VECTOR(1024),
    source.embedding_vector_sha256,
    vector_norm(source.embedding_vector::VECTOR(1024)),
    source.source_span_hash,
    source.chunk_profile_hash,
    source.content_hash,
    source.collection_visibility_epoch,
    source.collection_processing_revision,
    source.document_visibility_epoch
  FROM knowledge_pgvector_shadow_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS v_inserted_count = ROW_COUNT;

  SELECT count(*) INTO v_verified_shadow_count
  FROM knowledge_pgvector_shadow_sources source
  JOIN knowledge_child_vector_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.embedding_model_id = source.embedding_model_id
   AND shadow.embedding_dimensions = source.embedding_dimensions
   AND shadow.embedding_vector_sha256 = source.embedding_vector_sha256
   AND shadow.embedding_vector::REAL[] = source.embedding_vector
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;
  IF v_verified_shadow_count <> v_eligible_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    v_eligible_count,
    v_inserted_count,
    v_verified_shadow_count;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_backfill_bm25_shadow(
  p_index_generation_id UUID,
  p_search_profile_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  inserted_count BIGINT,
  verified_shadow_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  profile_matches INTEGER;
  generation_matches INTEGER;
  v_eligible_count BIGINT;
  v_inserted_count BIGINT;
  v_verified_shadow_count BIGINT;
BEGIN
  IF p_index_generation_id IS NULL OR p_search_profile_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_BM25_SHADOW_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 4);

  SELECT count(*) INTO profile_matches
  FROM knowledge_search_profiles profile
  WHERE profile.id = p_search_profile_id
    AND knowledge_retrieval_profile_id(
      profile.provider_profile_id, profile.embedding_processor,
      profile.embedding_model_id, profile.embedding_dimensions,
      profile.rerank_processor, profile.rerank_model_id
    ) IS NOT NULL;
  SELECT count(*) INTO generation_matches
  FROM knowledge_index_generations generation
  JOIN knowledge_search_profiles profile
    ON profile.id = p_search_profile_id
   AND profile.index_profile_id = generation.index_profile_id
  WHERE generation.id = p_index_generation_id
    AND generation.status IN ('building', 'verified', 'active', 'retired');
  IF profile_matches <> 1 OR generation_matches <> 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_BM25_SHADOW_PROFILE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN knowledge_bm25_shadow_build_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE shadow.index_generation_id = p_index_generation_id
      AND shadow.search_profile_id = p_search_profile_id
      AND (
        shadow.index_generation_id <> source.index_generation_id
        OR shadow.search_profile_id <> source.search_profile_id
        OR shadow.bm25_text IS DISTINCT FROM
          knowledge_build_bm25_shadow_text(
            source.lexical_text,
            source.exact_terms
          )
        OR shadow.exact_terms IS DISTINCT FROM
          knowledge_normalize_bm25_shadow_terms(source.exact_terms)
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_BM25_SHADOW_EXISTING_MISMATCH';
  END IF;

  SELECT count(*) INTO v_eligible_count
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;

  INSERT INTO knowledge_child_bm25_shadow_projections (
    child_chunk_id, parent_chunk_id, materialization_id,
    index_generation_id, collection_id, document_id, document_version_id,
    search_profile_id, bm25_text, exact_terms, source_span_hash,
    chunk_profile_hash, content_hash, child_ordinal,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch
  )
  SELECT
    source.child_chunk_id,
    source.parent_chunk_id,
    source.materialization_id,
    source.index_generation_id,
    source.collection_id,
    source.document_id,
    source.document_version_id,
    source.search_profile_id,
    knowledge_build_bm25_shadow_text(
      source.lexical_text,
      source.exact_terms
    ),
    knowledge_normalize_bm25_shadow_terms(source.exact_terms),
    source.source_span_hash,
    source.chunk_profile_hash,
    source.content_hash,
    source.child_ordinal,
    source.collection_visibility_epoch,
    source.collection_processing_revision,
    source.document_visibility_epoch
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS v_inserted_count = ROW_COUNT;

  SELECT count(*) INTO v_verified_shadow_count
  FROM knowledge_bm25_shadow_build_sources source
  JOIN knowledge_child_bm25_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.bm25_text = knowledge_build_bm25_shadow_text(
     source.lexical_text,
     source.exact_terms
   )
   AND shadow.exact_terms =
     knowledge_normalize_bm25_shadow_terms(source.exact_terms)
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.child_ordinal = source.child_ordinal
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;
  IF v_verified_shadow_count <> v_eligible_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_BM25_SHADOW_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    v_eligible_count,
    v_inserted_count,
    v_verified_shadow_count;
END
$function$;

-- BGE-only Candidate allocation and provider-generic lifecycle fences.
CREATE OR REPLACE FUNCTION knowledge_begin_structure_generation_rebuild(
  p_index_profile_id UUID,
  p_search_profile_id UUID,
  p_generation_id UUID,
  p_chunk_profile_hash TEXT,
  p_base_profile_hash TEXT,
  p_parser_manifest_hash TEXT,
  p_search_profile_hash TEXT,
  p_build_snapshot_hash TEXT,
  p_allocations JSONB
) RETURNS TABLE(
  candidate_generation_id UUID,
  allocated_document_count BIGINT,
  active_generation_id UUID
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_generation knowledge_index_generations%ROWTYPE;
  active_profile knowledge_index_profiles%ROWTYPE;
  allocation JSONB;
  document_row RECORD;
  allocation_count BIGINT;
  expected_count BIGINT;
  outbox_floor BIGINT;
BEGIN
  IF p_index_profile_id IS NULL OR p_search_profile_id IS NULL
    OR p_generation_id IS NULL
    OR p_chunk_profile_hash !~ '^[0-9a-f]{64}$'
    OR p_base_profile_hash !~ '^[0-9a-f]{64}$'
    OR p_parser_manifest_hash !~ '^[0-9a-f]{64}$'
    OR p_search_profile_hash !~ '^[0-9a-f]{64}$'
    OR p_build_snapshot_hash !~ '^[0-9a-f]{64}$'
    OR jsonb_typeof(p_allocations) <> 'array'
    OR jsonb_array_length(p_allocations) < 1
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_STRUCTURE_REBUILD_ARGUMENT_INVALID';
  END IF;

  SELECT generation.* INTO active_generation
  FROM knowledge_corpus_projection_head head
  JOIN knowledge_index_generations generation
    ON generation.id=head.active_index_generation_id
   AND generation.status='active'
  WHERE head.singleton_id=1
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_REBUILD_ACTIVE_GENERATION_MISSING';
  END IF;
  IF EXISTS (
    SELECT 1 FROM knowledge_index_generations
    WHERE status IN ('building','verified')
  ) THEN
    RAISE EXCEPTION USING ERRCODE='55000',
      MESSAGE='RAG_STRUCTURE_REBUILD_CANDIDATE_EXISTS';
  END IF;

  SELECT * INTO active_profile FROM knowledge_index_profiles
  WHERE id=active_generation.index_profile_id;
  SELECT count(*) INTO expected_count
  FROM knowledge_documents document
  JOIN knowledge_document_versions version
    ON version.id=document.current_version_id
   AND version.document_id=document.id
   AND version.status='active'
  JOIN files file ON file.id=version.file_id
   AND file.upload_status='available' AND file.deleted_at IS NULL
  WHERE document.deleted_at IS NULL AND document.status='active';
  SELECT count(*), count(DISTINCT value->>'documentId')
    INTO allocation_count, outbox_floor
  FROM jsonb_array_elements(p_allocations);
  IF allocation_count <> expected_count OR outbox_floor <> expected_count
    OR EXISTS (
      SELECT 1
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.id=document.current_version_id
       AND version.document_id=document.id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.deleted_at IS NULL AND document.status='active'
        AND NOT EXISTS (
          SELECT 1
          FROM jsonb_array_elements(p_allocations) item
          WHERE item->>'documentId'=document.id::text
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_STRUCTURE_REBUILD_ALLOCATION_COVERAGE_INVALID';
  END IF;

  INSERT INTO knowledge_index_profiles(
    id,contract_version,canonical_schema_version,parser_manifest,
    parser_manifest_hash,chunk_manifest,chunk_profile_hash,
    embedding_processor,embedding_endpoint_id,embedding_model_id,
    embedding_api_version,embedding_role,rerank_processor,rerank_endpoint_id,
    rerank_model_id,rerank_api_version,base_profile_hash
  ) VALUES (
    p_index_profile_id,active_profile.contract_version,
    active_profile.canonical_schema_version,
    jsonb_build_object('schemaVersion','g11.9d-structure-parser-manifest.v1',
      'native','structure','mineru','structure'),p_parser_manifest_hash,
    jsonb_build_object('schemaVersion','g11.9d-structure-chunk-manifest.v1',
      'chunkProfileHash',p_chunk_profile_hash),p_chunk_profile_hash,
    'siliconflow','siliconflow-cn-v1','Pro/BAAI/bge-m3',
    'v1-2026-07-24','passage','siliconflow','siliconflow-cn-v1',
    'Pro/BAAI/bge-reranker-v2-m3','v1-2026-07-24',p_base_profile_hash
  );
  INSERT INTO knowledge_search_profiles(
    id,index_profile_id,provider_profile_id,embedding_processor,
    embedding_model_id,embedding_dimensions,rerank_processor,rerank_model_id,
    lexical_config,exact_config,profile_hash
  ) VALUES (
    p_search_profile_id,p_index_profile_id,'siliconflow_bge_m3_v1',
    'siliconflow','Pro/BAAI/bge-m3',1024,'siliconflow',
    'Pro/BAAI/bge-reranker-v2-m3',
    '{"schemaVersion":"mm-chat.lexical-profile.v2","tokenizer":"simple+cjk-bigram"}'::jsonb,
    '{"schemaVersion":"mm-chat.exact-profile.v2","normalization":"lower-trim-deduplicate"}'::jsonb,
    p_search_profile_hash
  );
  INSERT INTO knowledge_index_generations(
    id,index_profile_id,generation_seq,status,build_snapshot,build_snapshot_hash
  ) VALUES (
    p_generation_id,p_index_profile_id,
    (SELECT COALESCE(max(generation_seq),0)+1 FROM knowledge_index_generations),
    'building',jsonb_build_object(
      'schemaVersion','g11.9d-structure-rebuild-snapshot.v1',
      'sourceGenerationId',active_generation.id,
      'documentCount',expected_count),p_build_snapshot_hash
  );
  SELECT COALESCE(max(id),0) INTO outbox_floor FROM knowledge_outbox;
  INSERT INTO knowledge_projection_state(
    index_generation_id,readiness,projection_revision,required_outbox_floor,
    contiguous_applied_outbox_id,document_count
  ) VALUES (p_generation_id,'building',1,outbox_floor,outbox_floor,expected_count);

  FOR allocation IN SELECT value FROM jsonb_array_elements(p_allocations) LOOP
    IF allocation->>'documentId' !~
      '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR allocation->>'materializationId' !~
      '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR allocation->>'jobId' !~
      '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR allocation->>'requestHash' !~ '^[0-9a-f]{64}$'
    THEN
      RAISE EXCEPTION USING ERRCODE='22023',
        MESSAGE='RAG_STRUCTURE_REBUILD_ALLOCATION_INVALID';
    END IF;
    SELECT d.id document_id,d.collection_id,v.id version_id,v.file_id,
      v.content_hash,v.visibility_epoch,c.acl_revision,c.visibility_epoch collection_visibility_epoch,
      c.collection_processing_revision processing_revision,authority.* INTO document_row
    FROM knowledge_documents d
    JOIN knowledge_collections c ON c.id=d.collection_id
    JOIN knowledge_document_versions v ON v.id=d.current_version_id
    JOIN LATERAL (
      SELECT j.processor,j.endpoint_id,j.model_id,j.governance_profile_id,
        j.governance_revision,j.governance_head_revision,j.collection_consent_id,
        j.collection_consent_revision,j.requested_by_user_id
      FROM knowledge_processing_jobs j
      WHERE j.document_id=d.id AND j.stage='parse' AND j.processor IS NOT NULL
      ORDER BY j.created_at DESC LIMIT 1
    ) authority ON true
    WHERE d.id=(allocation->>'documentId')::uuid
      AND d.deleted_at IS NULL AND d.status='active' AND v.status='active';
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='RAG_STRUCTURE_REBUILD_DOCUMENT_INVALID';
    END IF;
    INSERT INTO knowledge_document_materializations(
      id,index_generation_id,collection_id,document_id,document_version_id,
      file_id,materialization_seq,source_content_hash,base_profile_hash,
      collection_acl_revision,collection_visibility_epoch,
      collection_processing_revision,document_visibility_epoch,status
    ) VALUES (
      (allocation->>'materializationId')::uuid,p_generation_id,
      document_row.collection_id,document_row.document_id,document_row.version_id,
      document_row.file_id,1,document_row.content_hash,p_base_profile_hash,
      document_row.acl_revision,document_row.collection_visibility_epoch,
      document_row.processing_revision,document_row.visibility_epoch,'staging'
    );
    INSERT INTO knowledge_processing_jobs(
      id,collection_id,document_id,document_version_id,file_id,stage,operation,
      processor,endpoint_id,model_id,governance_profile_id,governance_revision,
      governance_head_revision,collection_consent_id,collection_consent_revision,
      collection_acl_revision,collection_visibility_epoch,
      collection_processing_revision,document_visibility_epoch,
      requested_by_user_id,idempotency_scope,idempotency_key,request_hash,
      max_attempts,index_generation_id,materialization_id,legacy_projection_unbound
    ) VALUES (
      (allocation->>'jobId')::uuid,document_row.collection_id,
      document_row.document_id,document_row.version_id,document_row.file_id,
      'parse','reprocess',document_row.processor,document_row.endpoint_id,
      document_row.model_id,document_row.governance_profile_id,
      document_row.governance_revision,document_row.governance_head_revision,
      document_row.collection_consent_id,document_row.collection_consent_revision,
      document_row.acl_revision,document_row.collection_visibility_epoch,
      document_row.processing_revision,document_row.visibility_epoch,
      document_row.requested_by_user_id,
      'structure-rebuild:'||p_generation_id::text,
      document_row.document_id::text,
      allocation->>'requestHash',
      3,p_generation_id,(allocation->>'materializationId')::uuid,false
    );
  END LOOP;
  RETURN QUERY SELECT p_generation_id,expected_count,active_generation.id;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_begin_registered_structure_generation_rebuild(
  p_index_profile_id UUID,
  p_search_profile_id UUID,
  p_generation_id UUID,
  p_chunk_profile_hash TEXT,
  p_base_profile_hash TEXT,
  p_parser_manifest_hash TEXT,
  p_search_profile_hash TEXT,
  p_build_snapshot_hash TEXT,
  p_allocations JSONB
) RETURNS TABLE(
  candidate_generation_id UUID,
  allocated_document_count BIGINT,
  active_generation_id UUID
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_structure_chunk_profile_descriptors descriptor
    WHERE descriptor.chunk_profile_hash = p_chunk_profile_hash
      AND descriptor.schema_version = 'mm-chat.structure-chunk-profile.v3'
      AND descriptor.chunk_profile_hash =
        '36845c249aa551d4d86720c38dfef9eb9e36ed49573a7547d2a5381d5f085d73'
  )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_STRUCTURE_OPERATOR_PROFILE_UNREGISTERED';
  END IF;

  RETURN QUERY
  SELECT rebuild.candidate_generation_id,
    rebuild.allocated_document_count,
    rebuild.active_generation_id
  FROM knowledge_begin_structure_generation_rebuild(
    p_index_profile_id,
    p_search_profile_id,
    p_generation_id,
    p_chunk_profile_hash,
    p_base_profile_hash,
    p_parser_manifest_hash,
    p_search_profile_hash,
    p_build_snapshot_hash,
    p_allocations
  ) rebuild;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_assert_pg17_generation_ready(
  p_index_generation_id UUID
) RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  document_count BIGINT,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  selected_search_profile_id UUID;
  expected_document_count BIGINT;
  source_document_count BIGINT;
  expected_count BIGINT;
  paired_source_count BIGINT;
  verified_vector_count BIGINT;
  verified_bm25_count BIGINT;
BEGIN
  IF p_index_generation_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_RETRIEVAL_GENERATION_ARGUMENT_INVALID';
  END IF;

  SELECT profile.id INTO selected_search_profile_id
  FROM knowledge_index_generations generation
  JOIN knowledge_search_profiles profile
    ON profile.index_profile_id = generation.index_profile_id
   AND knowledge_retrieval_profile_id(
     profile.provider_profile_id, profile.embedding_processor,
     profile.embedding_model_id, profile.embedding_dimensions,
     profile.rerank_processor, profile.rerank_model_id
   ) IS NOT NULL
  WHERE generation.id = p_index_generation_id
    AND generation.status IN ('building', 'verified', 'active', 'retired');
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_GENERATION_PROFILE_MISMATCH';
  END IF;

  SELECT count(*) INTO expected_document_count
  FROM knowledge_documents document
  JOIN knowledge_collections collection
    ON collection.id = document.collection_id
   AND collection.deleted_at IS NULL
  JOIN knowledge_document_versions version
    ON version.id = document.current_version_id
   AND version.document_id = document.id
   AND version.status = 'active'
  JOIN files file
    ON file.id = version.file_id
   AND file.upload_status = 'available'
   AND file.deleted_at IS NULL
  WHERE document.status = 'active'
    AND document.deleted_at IS NULL;

  SELECT count(*), count(DISTINCT source.document_id)
  INTO expected_count, source_document_count
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = selected_search_profile_id;

  SELECT count(*) INTO paired_source_count
  FROM knowledge_bm25_shadow_build_sources bm25
  JOIN knowledge_pgvector_shadow_sources vector_source
    ON vector_source.child_chunk_id = bm25.child_chunk_id
   AND vector_source.parent_chunk_id = bm25.parent_chunk_id
   AND vector_source.materialization_id = bm25.materialization_id
   AND vector_source.index_generation_id = bm25.index_generation_id
   AND vector_source.collection_id = bm25.collection_id
   AND vector_source.document_id = bm25.document_id
   AND vector_source.document_version_id = bm25.document_version_id
   AND vector_source.search_profile_id = bm25.search_profile_id
   AND vector_source.source_span_hash = bm25.source_span_hash
   AND vector_source.chunk_profile_hash = bm25.chunk_profile_hash
   AND vector_source.content_hash = bm25.content_hash
   AND vector_source.collection_visibility_epoch =
     bm25.collection_visibility_epoch
   AND vector_source.collection_processing_revision =
     bm25.collection_processing_revision
   AND vector_source.document_visibility_epoch =
     bm25.document_visibility_epoch
  WHERE bm25.index_generation_id = p_index_generation_id
    AND bm25.search_profile_id = selected_search_profile_id;

  SELECT count(*) INTO verified_vector_count
  FROM knowledge_pgvector_shadow_sources source
  JOIN knowledge_child_vector_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.embedding_model_id = source.embedding_model_id
   AND shadow.embedding_dimensions = source.embedding_dimensions
   AND shadow.embedding_vector_sha256 = source.embedding_vector_sha256
   AND shadow.embedding_vector::REAL[] = source.embedding_vector
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = selected_search_profile_id;

  SELECT count(*) INTO verified_bm25_count
  FROM knowledge_bm25_shadow_build_sources source
  JOIN knowledge_child_bm25_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.bm25_text = knowledge_build_bm25_shadow_text(
     source.lexical_text,
     source.exact_terms
   )
   AND shadow.exact_terms =
     knowledge_normalize_bm25_shadow_terms(source.exact_terms)
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.child_ordinal = source.child_ordinal
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = selected_search_profile_id;

  IF expected_document_count < 1
    OR source_document_count <> expected_document_count
    OR expected_count < expected_document_count
    OR paired_source_count <> expected_count
    OR verified_vector_count <> expected_count
    OR verified_bm25_count <> expected_count
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_GENERATION_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    p_index_generation_id,
    selected_search_profile_id,
    expected_document_count,
    expected_count,
    verified_vector_count,
    verified_bm25_count;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_generation_id UUID;
  active_search_profile_id UUID;
  expected_count BIGINT;
  verified_vector_count BIGINT;
  verified_bm25_count BIGINT;
BEGIN
  SELECT generation.id, profile.id
  INTO active_generation_id, active_search_profile_id
  FROM knowledge_corpus_projection_head corpus
  JOIN knowledge_index_generations generation
    ON generation.id = corpus.active_index_generation_id
   AND generation.status = 'active'
  JOIN knowledge_search_profiles profile
    ON profile.index_profile_id = generation.index_profile_id
   AND knowledge_retrieval_profile_id(
     profile.provider_profile_id, profile.embedding_processor,
     profile.embedding_model_id, profile.embedding_dimensions,
     profile.rerank_processor, profile.rerank_model_id
   ) IS NOT NULL
  WHERE corpus.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING';
  END IF;

  SELECT count(*) INTO expected_count
  FROM knowledge_bm25_shadow_sources source
  WHERE source.index_generation_id = active_generation_id
    AND source.search_profile_id = active_search_profile_id;

  SELECT count(*) INTO verified_vector_count
  FROM knowledge_pgvector_shadow_sources source
  JOIN knowledge_child_vector_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.embedding_model_id = source.embedding_model_id
   AND shadow.embedding_dimensions = source.embedding_dimensions
   AND shadow.embedding_vector_sha256 = source.embedding_vector_sha256
   AND shadow.embedding_vector::REAL[] = source.embedding_vector
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = active_generation_id
    AND source.search_profile_id = active_search_profile_id;

  SELECT count(*) INTO verified_bm25_count
  FROM knowledge_bm25_shadow_sources source
  JOIN knowledge_child_bm25_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.bm25_text = knowledge_build_bm25_shadow_text(
     source.lexical_text,
     source.exact_terms
   )
   AND shadow.exact_terms =
     knowledge_normalize_bm25_shadow_terms(source.exact_terms)
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.child_ordinal = source.child_ordinal
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = active_generation_id
    AND source.search_profile_id = active_search_profile_id;

  IF verified_vector_count <> expected_count
    OR verified_bm25_count <> expected_count
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    active_generation_id,
    active_search_profile_id,
    expected_count,
    verified_vector_count,
    verified_bm25_count;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_verify_structure_generation(
  p_index_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_expected_chunk_profile_hash TEXT
) RETURNS TABLE(
  candidate_generation_id UUID,
  artifact_manifest_hash TEXT,
  document_count BIGINT,
  block_count BIGINT,
  parent_count BIGINT,
  child_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  candidate_generation knowledge_index_generations%ROWTYPE;
  candidate_state knowledge_projection_state%ROWTYPE;
  active_generation_id UUID;
  candidate_search_profile_id UUID;
  candidate_embedding_model_id TEXT;
  candidate_embedding_dimensions INTEGER;
  expected_document_count BIGINT;
  candidate_document_count BIGINT;
  candidate_materialization_count BIGINT;
  candidate_block_count BIGINT;
  candidate_parent_count BIGINT;
  candidate_child_count BIGINT;
  ready_child_count BIGINT;
  projection_head_count BIGINT;
  latest_job_count BIGINT;
  latest_succeeded_job_count BIGINT;
  materialization_aggregate TEXT;
  block_aggregate TEXT;
  parent_aggregate TEXT;
  child_aggregate TEXT;
  computed_manifest_hash TEXT;
  verification_time TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_index_generation_id IS NULL
    OR p_expected_head_revision IS NULL OR p_expected_head_revision < 1
    OR p_expected_chunk_profile_hash IS NULL
    OR p_expected_chunk_profile_hash !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_STRUCTURE_VERIFY_ARGUMENT_INVALID';
  END IF;

  SELECT head.active_index_generation_id INTO active_generation_id
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id=1
    AND head.head_revision=p_expected_head_revision
    AND head.active_index_generation_id IS NOT NULL
    AND head.active_index_generation_id<>p_index_generation_id
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_HEAD_STALE';
  END IF;

  SELECT generation.* INTO candidate_generation
  FROM knowledge_index_generations generation
  WHERE generation.id=p_index_generation_id
    AND generation.status IN ('building','verified')
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_CANDIDATE_MISSING';
  END IF;

  SELECT state.* INTO candidate_state
  FROM knowledge_projection_state state
  WHERE state.index_generation_id=p_index_generation_id
  FOR UPDATE;
  IF NOT FOUND
    OR candidate_state.readiness NOT IN ('building','ready')
    OR (candidate_generation.status='building'
      AND candidate_state.readiness<>'building')
    OR (candidate_generation.status='verified'
      AND candidate_state.readiness<>'ready')
    OR candidate_state.contiguous_applied_outbox_id<>
      candidate_state.required_outbox_floor
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_STATE_INVALID';
  END IF;

  SELECT search_profile.id, search_profile.embedding_model_id,
    search_profile.embedding_dimensions
  INTO candidate_search_profile_id, candidate_embedding_model_id,
    candidate_embedding_dimensions
  FROM knowledge_index_profiles index_profile
  JOIN knowledge_search_profiles search_profile
    ON search_profile.index_profile_id=index_profile.id
   AND search_profile.embedding_processor=index_profile.embedding_processor
   AND search_profile.embedding_model_id=index_profile.embedding_model_id
   AND search_profile.rerank_processor=index_profile.rerank_processor
   AND search_profile.rerank_model_id=index_profile.rerank_model_id
   AND knowledge_retrieval_profile_id(
     search_profile.provider_profile_id, search_profile.embedding_processor,
     search_profile.embedding_model_id, search_profile.embedding_dimensions,
     search_profile.rerank_processor, search_profile.rerank_model_id
   ) IS NOT NULL
  WHERE index_profile.id=candidate_generation.index_profile_id
    AND index_profile.chunk_profile_hash=p_expected_chunk_profile_hash
    AND index_profile.embedding_role='passage';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_PROFILE_MISMATCH';
  END IF;

  SELECT count(*) INTO expected_document_count
  FROM knowledge_documents document
  JOIN knowledge_document_versions version
    ON version.document_id=document.id
   AND version.id=document.current_version_id
   AND version.status='active'
  JOIN files file ON file.id=version.file_id
   AND file.upload_status='available' AND file.deleted_at IS NULL
  WHERE document.status='active' AND document.deleted_at IS NULL;

  SELECT count(*),count(DISTINCT materialization.document_id)
    INTO candidate_materialization_count,candidate_document_count
  FROM knowledge_document_materializations materialization
  WHERE materialization.index_generation_id=p_index_generation_id
    AND materialization.status='published'
    AND materialization.parse_artifact_set_id IS NOT NULL
    AND materialization.manifest_hash IS NOT NULL
    AND materialization.result_hash IS NOT NULL
    AND materialization.verified_at IS NOT NULL
    AND materialization.published_at IS NOT NULL;
  IF expected_document_count<1
    OR candidate_materialization_count<>expected_document_count
    OR candidate_document_count<>expected_document_count
    OR candidate_state.document_count<>expected_document_count
    OR EXISTS (
      SELECT document.id,version.id,version.file_id,version.content_hash
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.document_id=document.id
       AND version.id=document.current_version_id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
      EXCEPT
      SELECT materialization.document_id,materialization.document_version_id,
        materialization.file_id,materialization.source_content_hash
      FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND materialization.status='published'
    ) OR EXISTS (
      SELECT materialization.document_id,materialization.document_version_id,
        materialization.file_id,materialization.source_content_hash
      FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND materialization.status='published'
      EXCEPT
      SELECT document.id,version.id,version.file_id,version.content_hash
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.document_id=document.id
       AND version.id=document.current_version_id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_COVERAGE_INVALID';
  END IF;

  WITH latest_job AS (
    SELECT DISTINCT ON (job.materialization_id,job.stage)
      job.materialization_id,job.stage,job.status
    FROM knowledge_processing_jobs job
    WHERE job.index_generation_id=p_index_generation_id
      AND job.stage IN ('parse','passage_embedding')
    ORDER BY job.materialization_id,job.stage,job.created_at DESC,job.id DESC
  )
  SELECT count(*),count(*) FILTER (WHERE status='succeeded')
    INTO latest_job_count,latest_succeeded_job_count
  FROM latest_job;
  IF latest_job_count<>expected_document_count*2
    OR latest_succeeded_job_count<>latest_job_count
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_JOBS_INCOMPLETE';
  END IF;

  SELECT count(*) INTO projection_head_count
  FROM knowledge_document_projection_heads head
  JOIN knowledge_document_materializations materialization
    ON materialization.id=head.active_materialization_id
   AND materialization.index_generation_id=head.index_generation_id
   AND materialization.document_id=head.document_id
   AND materialization.status='published'
  WHERE head.index_generation_id=p_index_generation_id;
  IF projection_head_count<>expected_document_count THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_HEADS_INCOMPLETE';
  END IF;

  SELECT count(*) INTO candidate_block_count
  FROM knowledge_blocks block
  JOIN knowledge_parser_artifact_sets artifact_set
    ON artifact_set.id=block.artifact_set_id
   AND artifact_set.document_id=block.document_id
   AND artifact_set.document_version_id=block.document_version_id
  JOIN knowledge_document_materializations materialization
    ON materialization.parse_artifact_set_id=artifact_set.id
   AND materialization.document_id=artifact_set.document_id
   AND materialization.document_version_id=artifact_set.document_version_id
  WHERE materialization.index_generation_id=p_index_generation_id
    AND artifact_set.index_profile_id=candidate_generation.index_profile_id
    AND artifact_set.status IN ('staging','verified');
  IF candidate_block_count<expected_document_count
    OR EXISTS (
      SELECT 1
      FROM knowledge_document_materializations materialization
      JOIN knowledge_parser_artifact_sets artifact_set
        ON artifact_set.id=materialization.parse_artifact_set_id
      WHERE materialization.index_generation_id=p_index_generation_id
        AND (artifact_set.index_profile_id<>candidate_generation.index_profile_id
          OR artifact_set.status NOT IN ('staging','verified'))
    ) OR EXISTS (
      SELECT 1
      FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND NOT EXISTS (
          SELECT 1 FROM knowledge_blocks block
          WHERE block.artifact_set_id=materialization.parse_artifact_set_id
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_ARTIFACTS_INCOMPLETE';
  END IF;

  SELECT count(*) INTO candidate_parent_count
  FROM knowledge_parent_chunks parent
  WHERE parent.index_generation_id=p_index_generation_id;
  SELECT count(*) INTO candidate_child_count
  FROM knowledge_child_chunks child
  WHERE child.index_generation_id=p_index_generation_id;
  SELECT count(*) INTO ready_child_count
  FROM knowledge_child_chunks child
  JOIN knowledge_parent_chunks parent
    ON parent.id=child.parent_chunk_id
   AND parent.materialization_id=child.materialization_id
   AND parent.index_generation_id=child.index_generation_id
  JOIN knowledge_child_search_projections search
    ON search.child_chunk_id=child.id
   AND search.parent_chunk_id=parent.id
   AND search.materialization_id=child.materialization_id
   AND search.index_generation_id=child.index_generation_id
   AND search.document_id=child.document_id
   AND search.document_version_id=child.document_version_id
   AND search.source_span_hash=child.source_span_hash
   AND search.chunk_profile_hash=child.chunk_profile_hash
   AND search.content_hash=child.content_hash
  WHERE child.index_generation_id=p_index_generation_id
    AND parent.chunk_profile_hash=p_expected_chunk_profile_hash
    AND child.chunk_profile_hash=p_expected_chunk_profile_hash
    AND knowledge_locator_summary_is_valid(parent.locator_summary)
    AND knowledge_locator_summary_is_valid(search.locator_summary)
    AND search.search_profile_id=candidate_search_profile_id
    AND search.status='ready'
    AND search.embedding_model_id=candidate_embedding_model_id
    AND search.embedding_dimensions=candidate_embedding_dimensions
    AND cardinality(search.embedding_vector)=1024
    AND array_position(search.embedding_vector,NULL) IS NULL
    AND search.embedding_vector_sha256 IS NOT NULL
    AND search.ready_at IS NOT NULL;
  IF candidate_parent_count<expected_document_count
    OR candidate_child_count<expected_document_count
    OR ready_child_count<>candidate_child_count
    OR EXISTS (
      SELECT 1 FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND (NOT EXISTS (
          SELECT 1 FROM knowledge_parent_chunks parent
          WHERE parent.materialization_id=materialization.id
        ) OR NOT EXISTS (
          SELECT 1 FROM knowledge_child_chunks child
          WHERE child.materialization_id=materialization.id
        ))
    ) OR EXISTS (
      SELECT 1 FROM knowledge_parent_chunks parent
      WHERE parent.index_generation_id=p_index_generation_id
        AND NOT EXISTS (
          SELECT 1 FROM knowledge_child_chunks child
          WHERE child.parent_chunk_id=parent.id
            AND child.materialization_id=parent.materialization_id
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_PROJECTION_INCOMPLETE';
  END IF;

  IF candidate_generation.status='building' THEN
    UPDATE knowledge_parser_artifact_sets artifact_set
    SET status='verified',verified_at=verification_time
    FROM knowledge_document_materializations materialization
    WHERE materialization.index_generation_id=p_index_generation_id
      AND materialization.parse_artifact_set_id=artifact_set.id
      AND artifact_set.status='staging';
  ELSIF EXISTS (
    SELECT 1
    FROM knowledge_document_materializations materialization
    JOIN knowledge_parser_artifact_sets artifact_set
      ON artifact_set.id=materialization.parse_artifact_set_id
    WHERE materialization.index_generation_id=p_index_generation_id
      AND (artifact_set.status<>'verified' OR artifact_set.verified_at IS NULL)
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_ARTIFACTS_INCOMPLETE';
  END IF;

  SELECT string_agg(row_hash,'' ORDER BY document_id,materialization_id)
    INTO materialization_aggregate
  FROM (
    SELECT materialization.document_id,
      materialization.id materialization_id,
      encode(sha256(convert_to(
        'g11.9d.3a:materialization:v1|' || materialization.id::text || '|' ||
        materialization.document_id::text || '|' ||
        materialization.document_version_id::text || '|' ||
        materialization.file_id::text || '|' ||
        materialization.parse_artifact_set_id::text || '|' ||
        materialization.source_content_hash || '|' ||
        materialization.base_profile_hash || '|' ||
        materialization.manifest_hash || '|' || materialization.result_hash || '|' ||
        artifact_set.manifest_hash || '|' || artifact_set.parser_kind || '|' ||
        artifact_set.parser_version || '|' ||
        encode(sha256(convert_to(artifact_set.quality_report::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_document_materializations materialization
    JOIN knowledge_parser_artifact_sets artifact_set
      ON artifact_set.id=materialization.parse_artifact_set_id
    WHERE materialization.index_generation_id=p_index_generation_id
  ) rows;

  SELECT string_agg(row_hash,'' ORDER BY document_id,ordinal,block_id)
    INTO block_aggregate
  FROM (
    SELECT block.document_id,block.ordinal,block.id block_id,
      encode(sha256(convert_to(
        'g11.9d.3a:block:v1|' || block.id::text || '|' ||
        block.artifact_set_id::text || '|' || block.document_id::text || '|' ||
        block.document_version_id::text || '|' || block.ordinal::text || '|' ||
        block.block_type || '|' || block.content_hash || '|' ||
        block.source_span_hash || '|' || block.locator_kind || '|' ||
        encode(sha256(convert_to(block.locator::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_blocks block
    JOIN knowledge_document_materializations materialization
      ON materialization.parse_artifact_set_id=block.artifact_set_id
    WHERE materialization.index_generation_id=p_index_generation_id
  ) rows;

  SELECT string_agg(row_hash,'' ORDER BY document_id,ordinal,parent_id)
    INTO parent_aggregate
  FROM (
    SELECT parent.document_id,parent.ordinal,parent.id parent_id,
      encode(sha256(convert_to(
        'g11.9d.3a:parent:v1|' || parent.id::text || '|' ||
        parent.materialization_id::text || '|' || parent.document_id::text || '|' ||
        parent.document_version_id::text || '|' || parent.ordinal::text || '|' ||
        parent.chunk_profile_hash || '|' || parent.source_span_hash || '|' ||
        parent.content_hash || '|' ||
        encode(sha256(convert_to(parent.locator_summary::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_parent_chunks parent
    WHERE parent.index_generation_id=p_index_generation_id
  ) rows;

  SELECT string_agg(row_hash,'' ORDER BY document_id,ordinal,child_id)
    INTO child_aggregate
  FROM (
    SELECT child.document_id,child.ordinal,child.id child_id,
      encode(sha256(convert_to(
        'g11.9d.3a:child:v1|' || child.id::text || '|' ||
        child.parent_chunk_id::text || '|' || child.materialization_id::text || '|' ||
        child.document_id::text || '|' || child.document_version_id::text || '|' ||
        child.ordinal::text || '|' || child.chunk_profile_hash || '|' ||
        child.source_span_hash || '|' || child.content_hash || '|' ||
        search.search_profile_id::text || '|' || search.embedding_model_id || '|' ||
        search.embedding_dimensions::text || '|' ||
        search.embedding_vector_sha256 || '|' ||
        encode(sha256(convert_to(search.locator_summary::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_child_chunks child
    JOIN knowledge_child_search_projections search
      ON search.child_chunk_id=child.id
     AND search.materialization_id=child.materialization_id
    WHERE child.index_generation_id=p_index_generation_id
  ) rows;

  IF materialization_aggregate IS NULL OR block_aggregate IS NULL
    OR parent_aggregate IS NULL OR child_aggregate IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_MANIFEST_INPUT_MISSING';
  END IF;
  computed_manifest_hash := encode(sha256(convert_to(
    'g11.9d.3a:structure-generation-manifest:v1' || E'\n' ||
    candidate_generation.id::text || E'\n' ||
    candidate_generation.index_profile_id::text || E'\n' ||
    candidate_generation.build_snapshot_hash || E'\n' ||
    p_expected_chunk_profile_hash || E'\n' ||
    expected_document_count::text || E'\n' || candidate_block_count::text || E'\n' ||
    candidate_parent_count::text || E'\n' || candidate_child_count::text || E'\n' ||
    materialization_aggregate || E'\n' || block_aggregate || E'\n' ||
    parent_aggregate || E'\n' || child_aggregate,
    'UTF8'
  )),'hex');

  IF candidate_generation.status='building' THEN
    UPDATE knowledge_index_generations
    SET status='verified',artifact_manifest_hash=computed_manifest_hash,
      verified_at=verification_time
    WHERE id=p_index_generation_id AND status='building';
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='RAG_STRUCTURE_VERIFY_CANDIDATE_STALE';
    END IF;
    UPDATE knowledge_projection_state
    SET readiness='ready',projection_revision=projection_revision+1,
      manifest_hash=computed_manifest_hash,
      document_count=expected_document_count,
      parent_count=candidate_parent_count,child_count=candidate_child_count,
      verified_at=verification_time,updated_at=verification_time
    WHERE index_generation_id=p_index_generation_id AND readiness='building';
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='RAG_STRUCTURE_VERIFY_STATE_STALE';
    END IF;
  ELSIF candidate_generation.artifact_manifest_hash IS DISTINCT FROM
      computed_manifest_hash
    OR candidate_state.manifest_hash IS DISTINCT FROM computed_manifest_hash
    OR candidate_state.document_count<>expected_document_count
    OR candidate_state.parent_count<>candidate_parent_count
    OR candidate_state.child_count<>candidate_child_count
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_REPLAY_MISMATCH';
  END IF;

  RETURN QUERY SELECT p_index_generation_id,computed_manifest_hash,
    expected_document_count,candidate_block_count,
    candidate_parent_count,candidate_child_count;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_rollback_index_generation(
  p_active_generation_id UUID,
  p_target_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_active_manifest_hash TEXT,
  p_target_manifest_hash TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_generation knowledge_index_generations%ROWTYPE;
  target_generation knowledge_index_generations%ROWTYPE;
  transition_time TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_active_generation_id IS NULL OR p_target_generation_id IS NULL
    OR p_active_generation_id=p_target_generation_id
    OR p_expected_head_revision IS NULL OR p_expected_head_revision < 1
    OR p_active_manifest_hash IS NULL
    OR p_active_manifest_hash !~ '^[0-9a-f]{64}$'
    OR p_target_manifest_hash IS NULL
    OR p_target_manifest_hash !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_GENERATION_ROLLBACK_ARGUMENT_INVALID';
  END IF;

  PERFORM 1
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id=1
    AND head.head_revision=p_expected_head_revision
    AND head.active_index_generation_id=p_active_generation_id
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_HEAD_STALE';
  END IF;

  SELECT generation.* INTO active_generation
  FROM knowledge_index_generations generation
  JOIN knowledge_projection_state state
    ON state.index_generation_id=generation.id
  WHERE generation.id=p_active_generation_id
    AND generation.status='active'
    AND generation.artifact_manifest_hash=p_active_manifest_hash
    AND state.readiness='ready'
    AND state.manifest_hash=p_active_manifest_hash
  FOR UPDATE OF generation,state;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_ACTIVE_MISMATCH';
  END IF;
  IF (active_generation.build_snapshot->>'schemaVersion') IS DISTINCT FROM
      'g11.9d-structure-rebuild-snapshot.v1'
    OR (active_generation.build_snapshot->>'sourceGenerationId')
      IS DISTINCT FROM p_target_generation_id::TEXT
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_SOURCE_MISMATCH';
  END IF;

  SELECT generation.* INTO target_generation
  FROM knowledge_index_generations generation
  JOIN knowledge_projection_state state
    ON state.index_generation_id=generation.id
  WHERE generation.id=p_target_generation_id
    AND generation.status='retired'
    AND generation.artifact_manifest_hash=p_target_manifest_hash
    AND state.readiness='retired'
    AND state.manifest_hash=p_target_manifest_hash
  FOR UPDATE OF generation,state;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_TARGET_MISMATCH';
  END IF;

  IF NOT EXISTS (
      SELECT 1
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.id=document.current_version_id
       AND version.document_id=document.id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
    ) OR EXISTS (
      SELECT 1
      FROM knowledge_documents document
      JOIN knowledge_collections collection
        ON collection.id=document.collection_id
       AND collection.deleted_at IS NULL
      JOIN knowledge_document_versions version
        ON version.id=document.current_version_id
       AND version.document_id=document.id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
        AND NOT EXISTS (
          SELECT 1
          FROM knowledge_document_projection_heads target_head
          JOIN knowledge_document_materializations materialization
            ON materialization.id=target_head.active_materialization_id
           AND materialization.index_generation_id=target_head.index_generation_id
           AND materialization.document_id=target_head.document_id
           AND materialization.status='published'
          WHERE target_head.index_generation_id=p_target_generation_id
            AND target_head.document_id=document.id
            AND materialization.collection_id=collection.id
            AND materialization.document_version_id=version.id
            AND materialization.file_id=version.file_id
            AND materialization.source_content_hash=version.content_hash
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_COVERAGE_INVALID';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_documents document
    JOIN knowledge_collections collection
      ON collection.id=document.collection_id
     AND collection.deleted_at IS NULL
    JOIN knowledge_document_versions version
      ON version.id=document.current_version_id
     AND version.document_id=document.id
     AND version.status='active'
    JOIN files file ON file.id=version.file_id
     AND file.upload_status='available' AND file.deleted_at IS NULL
    JOIN knowledge_document_projection_heads target_head
      ON target_head.index_generation_id=p_target_generation_id
     AND target_head.document_id=document.id
    JOIN knowledge_document_materializations materialization
      ON materialization.id=target_head.active_materialization_id
     AND materialization.index_generation_id=target_head.index_generation_id
     AND materialization.document_id=target_head.document_id
     AND materialization.document_version_id=version.id
     AND materialization.status='published'
    WHERE document.status='active' AND document.deleted_at IS NULL
      AND (
        NOT EXISTS (
          SELECT 1
          FROM knowledge_parent_chunks parent
          WHERE parent.index_generation_id=p_target_generation_id
            AND parent.materialization_id=materialization.id
            AND parent.document_id=document.id
            AND parent.document_version_id=version.id
        )
        OR NOT EXISTS (
          SELECT 1
          FROM knowledge_child_chunks child
          WHERE child.index_generation_id=p_target_generation_id
            AND child.materialization_id=materialization.id
            AND child.document_id=document.id
            AND child.document_version_id=version.id
        )
        OR EXISTS (
          SELECT 1
          FROM knowledge_parent_chunks parent
          WHERE parent.index_generation_id=p_target_generation_id
            AND parent.materialization_id=materialization.id
            AND parent.document_id=document.id
            AND parent.document_version_id=version.id
            AND NOT EXISTS (
              SELECT 1
              FROM knowledge_child_chunks child
              WHERE child.parent_chunk_id=parent.id
                AND child.materialization_id=parent.materialization_id
            )
        )
        OR EXISTS (
          SELECT 1
          FROM knowledge_child_chunks child
          JOIN knowledge_parent_chunks parent
            ON parent.id=child.parent_chunk_id
           AND parent.materialization_id=child.materialization_id
           AND parent.index_generation_id=child.index_generation_id
           AND parent.document_id=child.document_id
           AND parent.document_version_id=child.document_version_id
          LEFT JOIN knowledge_child_search_projections search
            ON search.child_chunk_id=child.id
           AND search.parent_chunk_id=parent.id
           AND search.materialization_id=child.materialization_id
           AND search.index_generation_id=child.index_generation_id
           AND search.document_id=child.document_id
           AND search.document_version_id=child.document_version_id
           AND search.source_span_hash=child.source_span_hash
           AND search.chunk_profile_hash=child.chunk_profile_hash
           AND search.content_hash=child.content_hash
          LEFT JOIN knowledge_search_profiles search_profile
            ON search_profile.id=search.search_profile_id
           AND search_profile.index_profile_id=target_generation.index_profile_id
           AND knowledge_retrieval_profile_id(
             search_profile.provider_profile_id,
             search_profile.embedding_processor,
             search_profile.embedding_model_id,
             search_profile.embedding_dimensions,
             search_profile.rerank_processor,
             search_profile.rerank_model_id
           ) IS NOT NULL
          WHERE child.index_generation_id=p_target_generation_id
            AND child.materialization_id=materialization.id
            AND child.document_id=document.id
            AND child.document_version_id=version.id
            AND (
              search.child_chunk_id IS NULL
              OR search_profile.id IS NULL
              OR search.status<>'ready'
              OR search.embedding_model_id<>search_profile.embedding_model_id
              OR search.embedding_dimensions<>search_profile.embedding_dimensions
              OR cardinality(search.embedding_vector)<>1024
              OR array_position(search.embedding_vector,NULL) IS NOT NULL
              OR search.embedding_vector_sha256 IS NULL
              OR search.ready_at IS NULL
              OR NOT knowledge_locator_summary_is_valid(parent.locator_summary)
              OR NOT knowledge_locator_summary_is_valid(search.locator_summary)
            )
        )
      )
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_PROJECTION_INCOMPLETE';
  END IF;

  UPDATE knowledge_index_generations
  SET status='retired',retired_at=transition_time
  WHERE id=p_active_generation_id AND status='active'
    AND artifact_manifest_hash=p_active_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;
  UPDATE knowledge_projection_state
  SET readiness='retired',updated_at=transition_time
  WHERE index_generation_id=p_active_generation_id
    AND readiness='ready' AND manifest_hash=p_active_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;

  UPDATE knowledge_index_generations
  SET status='active',retired_at=NULL
  WHERE id=p_target_generation_id AND status='retired'
    AND artifact_manifest_hash=p_target_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;
  UPDATE knowledge_projection_state
  SET readiness='ready',updated_at=transition_time
  WHERE index_generation_id=p_target_generation_id
    AND readiness='retired' AND manifest_hash=p_target_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;

  UPDATE knowledge_corpus_projection_head
  SET active_index_generation_id=p_target_generation_id,
    corpus_projection_revision=corpus_projection_revision+1,
    head_revision=head_revision+1,updated_at=transition_time
  WHERE singleton_id=1
    AND head_revision=p_expected_head_revision
    AND active_index_generation_id=p_active_generation_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_HEAD_STALE';
  END IF;
  RETURN true;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_reauthorize_and_hydrate_evidence_v47_base(
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
  content_hash TEXT,
  source_text TEXT,
  child_token_count INTEGER,
  parent_source_text TEXT,
  parent_token_count INTEGER,
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
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'RAG_HYDRATION_NOT_AUTHORIZED';
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
      source_span_hash TEXT,
      content_hash TEXT
    )
  ), authorized AS (
    SELECT
      r.*,
      child.content AS child_source_text,
      child.token_count AS child_token_count,
      parent.content AS parent_source_text,
      parent.token_count AS parent_token_count,
      search.locator_summary AS child_locator_summary
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
      AND version.document_id = document.id
      AND version.status = 'active'
      AND version.visibility_epoch = m.document_visibility_epoch
      AND version.content_hash = m.source_content_hash
    JOIN knowledge_index_generations generation
      ON generation.id = r.index_generation_id
      AND generation.status = 'active'
    JOIN knowledge_parent_chunks parent
      ON parent.id = r.parent_chunk_id
      AND parent.materialization_id = r.materialization_id
      AND parent.index_generation_id = r.index_generation_id
      AND parent.document_id = r.document_id
      AND parent.document_version_id = r.document_version_id
    JOIN knowledge_child_chunks child
      ON child.id = r.child_chunk_id
      AND child.parent_chunk_id = parent.id
      AND child.materialization_id = r.materialization_id
      AND child.index_generation_id = r.index_generation_id
      AND child.document_id = r.document_id
      AND child.document_version_id = r.document_version_id
      AND child.source_span_hash = r.source_span_hash
      AND child.content_hash = r.content_hash
    JOIN knowledge_search_profiles search_profile
      ON search_profile.index_profile_id = generation.index_profile_id
      AND knowledge_retrieval_profile_id(
        search_profile.provider_profile_id, search_profile.embedding_processor,
        search_profile.embedding_model_id, search_profile.embedding_dimensions,
        search_profile.rerank_processor, search_profile.rerank_model_id
      ) IS NOT NULL
    JOIN knowledge_child_search_projections search
      ON search.child_chunk_id = child.id
      AND search.parent_chunk_id = parent.id
      AND search.materialization_id = child.materialization_id
      AND search.index_generation_id = child.index_generation_id
      AND search.collection_id = r.collection_id
      AND search.document_id = child.document_id
      AND search.document_version_id = child.document_version_id
      AND search.search_profile_id = search_profile.id
      AND search.source_span_hash = child.source_span_hash
      AND search.chunk_profile_hash = child.chunk_profile_hash
      AND search.content_hash = child.content_hash
      AND search.status = 'ready'
      AND search.embedding_model_id = search_profile.embedding_model_id
      AND search.embedding_dimensions = search_profile.embedding_dimensions
      AND cardinality(search.embedding_vector) = 1024
      AND array_position(search.embedding_vector, NULL) IS NULL
      AND search.embedding_vector_sha256 IS NOT NULL
      AND search.ready_at IS NOT NULL
      AND knowledge_locator_summary_is_valid(search.locator_summary)
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
  SELECT
    authorized.collection_id,
    authorized.document_id,
    authorized.document_version_id,
    authorized.index_generation_id,
    authorized.materialization_id,
    authorized.parent_chunk_id,
    authorized.child_chunk_id,
    authorized.source_span_hash,
    authorized.content_hash,
    authorized.child_source_text,
    authorized.child_token_count,
    authorized.parent_source_text,
    authorized.parent_token_count,
    authorized.child_locator_summary
  FROM authorized
  WHERE octet_length(authorized.child_source_text) <= 65536
    AND octet_length(authorized.parent_source_text) <= 65536;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_hydrate_generation_evaluation_evidence_v47_base(
  p_index_generation_id UUID,
  p_collection_ids UUID[],
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
  content_hash TEXT,
  source_text TEXT,
  child_token_count INTEGER,
  parent_source_text TEXT,
  parent_token_count INTEGER,
  locator JSONB,
  provenance_valid BOOLEAN,
  cell_lineage_valid BOOLEAN
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_index_generation_id IS NULL
    OR p_collection_ids IS NULL
    OR cardinality(p_collection_ids) NOT BETWEEN 1 AND 32
    OR array_position(p_collection_ids, NULL) IS NOT NULL
    OR p_references IS NULL
    OR jsonb_typeof(p_references) <> 'array'
    OR jsonb_array_length(p_references) NOT BETWEEN 1 AND 16
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_GENERATION_EVALUATION_ARGUMENT_INVALID';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_index_generations generation
    JOIN knowledge_projection_state state
      ON state.index_generation_id = generation.id
     AND state.readiness = 'ready'
    LEFT JOIN knowledge_corpus_projection_head head
      ON head.singleton_id = 1
     AND head.active_index_generation_id = generation.id
    WHERE generation.id = p_index_generation_id
      AND generation.artifact_manifest_hash IS NOT NULL
      AND (
        (generation.status = 'active' AND head.singleton_id = 1)
        OR generation.status = 'verified'
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_GENERATION_EVALUATION_GENERATION_UNAVAILABLE';
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
      source_span_hash TEXT,
      content_hash TEXT
    )
  ), authorized AS (
    SELECT
      reference.*,
      child.content AS child_source_text,
      child.token_count AS child_tokens,
      parent.content AS parent_source_text,
      parent.token_count AS parent_tokens,
      search.locator_summary AS child_locator,
      CASE
        WHEN search.locator_summary->'primary'->>'kind' <>
          'sheet_cell' THEN true
        ELSE EXISTS (
          SELECT 1
          FROM knowledge_chunk_block_spans span
          JOIN knowledge_blocks block ON block.id = span.block_id
          WHERE span.chunk_kind = 'child'
            AND span.chunk_id = child.id
            AND block.artifact_set_id = materialization.parse_artifact_set_id
            AND block.document_id = child.document_id
            AND block.document_version_id = child.document_version_id
            AND block.locator_kind = 'sheet_cell'
            AND block.locator->>'sheet' =
              search.locator_summary->'primary'->'locator'->>'sheet'
        )
      END AS has_cell_lineage
    FROM requested reference
    JOIN knowledge_document_projection_heads head
      ON head.index_generation_id = p_index_generation_id
     AND head.document_id = reference.document_id
     AND head.active_materialization_id = reference.materialization_id
    JOIN knowledge_document_materializations materialization
      ON materialization.id = reference.materialization_id
     AND materialization.index_generation_id = p_index_generation_id
     AND materialization.collection_id = reference.collection_id
     AND materialization.document_id = reference.document_id
     AND materialization.document_version_id = reference.document_version_id
     AND materialization.status = 'published'
    JOIN knowledge_collections collection
      ON collection.id = reference.collection_id
     AND collection.deleted_at IS NULL
     AND collection.visibility_epoch = materialization.collection_visibility_epoch
     AND collection.collection_processing_revision =
       materialization.collection_processing_revision
    JOIN knowledge_documents document
      ON document.id = reference.document_id
     AND document.collection_id = reference.collection_id
     AND document.current_version_id = reference.document_version_id
     AND document.status = 'active'
     AND document.deleted_at IS NULL
     AND document.visibility_epoch = materialization.document_visibility_epoch
    JOIN knowledge_document_versions version
      ON version.id = reference.document_version_id
     AND version.document_id = reference.document_id
     AND version.status = 'active'
     AND version.content_hash = materialization.source_content_hash
    JOIN knowledge_index_generations generation
      ON generation.id = p_index_generation_id
    JOIN knowledge_parent_chunks parent
      ON parent.id = reference.parent_chunk_id
     AND parent.materialization_id = reference.materialization_id
     AND parent.index_generation_id = p_index_generation_id
     AND parent.document_id = reference.document_id
     AND parent.document_version_id = reference.document_version_id
    JOIN knowledge_child_chunks child
      ON child.id = reference.child_chunk_id
     AND child.parent_chunk_id = parent.id
     AND child.materialization_id = reference.materialization_id
     AND child.index_generation_id = p_index_generation_id
     AND child.document_id = reference.document_id
     AND child.document_version_id = reference.document_version_id
     AND child.source_span_hash = reference.source_span_hash
     AND child.content_hash = reference.content_hash
    JOIN knowledge_search_profiles search_profile
      ON search_profile.index_profile_id = generation.index_profile_id
      AND knowledge_retrieval_profile_id(
        search_profile.provider_profile_id, search_profile.embedding_processor,
        search_profile.embedding_model_id, search_profile.embedding_dimensions,
        search_profile.rerank_processor, search_profile.rerank_model_id
      ) IS NOT NULL
    JOIN knowledge_child_search_projections search
      ON search.child_chunk_id = child.id
     AND search.parent_chunk_id = parent.id
     AND search.materialization_id = child.materialization_id
     AND search.index_generation_id = p_index_generation_id
     AND search.collection_id = reference.collection_id
     AND search.document_id = child.document_id
     AND search.document_version_id = child.document_version_id
     AND search.search_profile_id = search_profile.id
     AND search.source_span_hash = child.source_span_hash
     AND search.chunk_profile_hash = child.chunk_profile_hash
     AND search.content_hash = child.content_hash
     AND search.status = 'ready'
     AND search.embedding_vector_sha256 IS NOT NULL
     AND cardinality(search.embedding_vector) = 1024
     AND array_position(search.embedding_vector, NULL) IS NULL
     AND knowledge_locator_summary_is_valid(search.locator_summary)
    WHERE reference.index_generation_id = p_index_generation_id
      AND reference.collection_id = ANY(p_collection_ids)
  )
  SELECT
    authorized.collection_id,
    authorized.document_id,
    authorized.document_version_id,
    authorized.index_generation_id,
    authorized.materialization_id,
    authorized.parent_chunk_id,
    authorized.child_chunk_id,
    authorized.source_span_hash,
    authorized.content_hash,
    authorized.child_source_text,
    authorized.child_tokens,
    authorized.parent_source_text,
    authorized.parent_tokens,
    authorized.child_locator,
    true,
    authorized.has_cell_lineage
  FROM authorized
  WHERE octet_length(authorized.child_source_text) <= 65536
    AND octet_length(authorized.parent_source_text) <= 65536;
END
$function$;
CREATE FUNCTION knowledge_fetch_fenced_hybrid_candidates_core(
  p_index_generation_id UUID,
  p_search_profile_id UUID,
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding REAL[],
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  rank_score REAL
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  normalized_query TEXT;
  query_terms TEXT[];
  bm25_query TEXT;
  query_vector VECTOR(1024);
  query_norm DOUBLE PRECISION;
  oversample_limit INTEGER;
  minimum_dense_similarity CONSTANT DOUBLE PRECISION := 0.48;
  minimum_dense_query_characters CONSTANT INTEGER := 8;
  rrf_constant CONSTANT DOUBLE PRECISION := 60.0;
BEGIN
  normalized_query := btrim(p_query_text);
  IF p_index_generation_id IS NULL
    OR p_search_profile_id IS NULL
    OR p_collection_ids IS NULL
    OR cardinality(p_collection_ids) NOT BETWEEN 1 AND 32
    OR array_position(p_collection_ids, NULL) IS NOT NULL
    OR normalized_query IS NULL
    OR octet_length(normalized_query) NOT BETWEEN 1 AND 2048
    OR p_limit IS NULL
    OR p_limit NOT BETWEEN 1 AND 50
    OR p_query_embedding IS NULL
    OR cardinality(p_query_embedding) <> 1024
    OR array_position(p_query_embedding, NULL) IS NOT NULL
    OR EXISTS (
      SELECT 1
      FROM unnest(p_query_embedding) component
      WHERE component::TEXT IN ('NaN', 'Infinity', '-Infinity')
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_GENERATION_EVALUATION_ARGUMENT_INVALID';
  END IF;

  SELECT sqrt(sum(
    component::DOUBLE PRECISION * component::DOUBLE PRECISION
  )) INTO query_norm
  FROM unnest(p_query_embedding) component;
  IF query_norm IS NULL OR query_norm <= 0
    OR query_norm::TEXT IN ('NaN', 'Infinity', '-Infinity')
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_GENERATION_EVALUATION_ARGUMENT_INVALID';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_index_generations generation
    JOIN knowledge_projection_state state
      ON state.index_generation_id = generation.id
     AND state.readiness = 'ready'
    LEFT JOIN knowledge_corpus_projection_head head
      ON head.singleton_id = 1
     AND head.active_index_generation_id = generation.id
    JOIN knowledge_search_profiles search_profile
      ON search_profile.id = p_search_profile_id
     AND search_profile.index_profile_id = generation.index_profile_id
     AND knowledge_retrieval_profile_id(
       search_profile.provider_profile_id,
       search_profile.embedding_processor,
       search_profile.embedding_model_id,
       search_profile.embedding_dimensions,
       search_profile.rerank_processor,
       search_profile.rerank_model_id
     ) IS NOT NULL
    WHERE generation.id = p_index_generation_id
      AND (
        (generation.status = 'active' AND head.singleton_id = 1)
        OR (
          generation.status = 'verified'
          AND generation.artifact_manifest_hash IS NOT NULL
        )
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_GENERATION_EVALUATION_GENERATION_UNAVAILABLE';
  END IF;

  query_vector := p_query_embedding::VECTOR(1024);
  query_terms := knowledge_bm25_shadow_query_terms(normalized_query);
  bm25_query := knowledge_build_bm25_shadow_text(
    normalized_query,
    query_terms
  );
  oversample_limit := least(p_limit * 8, 400);

  RETURN QUERY
  WITH selected_collection AS (
    SELECT DISTINCT unnest(p_collection_ids) AS id
  ), bm25_probe AS MATERIALIZED (
    SELECT
      shadow.child_chunk_id,
      shadow.bm25_text <@> to_bm25query(
        bm25_query,
        'idx_knowledge_child_bm25_shadow_text'
      ) AS raw_score
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN selected_collection selected ON selected.id = shadow.collection_id
    WHERE shadow.index_generation_id = p_index_generation_id
      AND shadow.search_profile_id = p_search_profile_id
      AND shadow.bm25_text <@> to_bm25query(
        bm25_query,
        'idx_knowledge_child_bm25_shadow_text'
      ) < 0
    ORDER BY raw_score, shadow.child_chunk_id
    LIMIT oversample_limit
  ), exact_probe AS MATERIALIZED (
    SELECT shadow.child_chunk_id, 0::DOUBLE PRECISION AS raw_score
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN selected_collection selected ON selected.id = shadow.collection_id
    WHERE shadow.index_generation_id = p_index_generation_id
      AND shadow.search_profile_id = p_search_profile_id
      AND cardinality(query_terms) > 0
      AND shadow.exact_terms && query_terms
    ORDER BY shadow.child_chunk_id
    LIMIT oversample_limit
  ), lexical_pool AS (
    SELECT combined.child_chunk_id, min(combined.raw_score) AS raw_score
    FROM (
      SELECT probe.child_chunk_id, probe.raw_score FROM bm25_probe probe
      UNION ALL
      SELECT probe.child_chunk_id, probe.raw_score FROM exact_probe probe
    ) combined
    GROUP BY combined.child_chunk_id
  ), bm25_authorized AS (
    SELECT
      source.collection_id,
      source.document_id,
      source.document_version_id,
      source.index_generation_id,
      source.materialization_id,
      source.parent_chunk_id,
      source.child_chunk_id,
      source.source_span_hash,
      source.content_hash,
      pool.raw_score,
      shadow.exact_terms && query_terms AS exact_match,
      position(lower(normalized_query) IN lower(source.lexical_text)) > 0
        AS phrase_match,
      source.child_ordinal
    FROM lexical_pool pool
    JOIN knowledge_child_bm25_shadow_projections shadow
      ON shadow.child_chunk_id = pool.child_chunk_id
     AND shadow.index_generation_id = p_index_generation_id
     AND shadow.search_profile_id = p_search_profile_id
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_build_sources candidate
      WHERE candidate.child_chunk_id = shadow.child_chunk_id
        AND candidate.index_generation_id = p_index_generation_id
        AND candidate.search_profile_id = p_search_profile_id
        AND candidate.search_profile_id = shadow.search_profile_id
        AND candidate.content_hash = shadow.content_hash
      OFFSET 0
    ) source
  ), bm25_ranked_unbounded AS (
    SELECT
      authorized.*,
      row_number() OVER (
        ORDER BY
          authorized.exact_match DESC,
          authorized.phrase_match DESC,
          authorized.raw_score,
          authorized.child_ordinal,
          authorized.child_chunk_id
      )::INTEGER AS lane_rank
    FROM bm25_authorized authorized
  ), bm25_ranked AS (
    SELECT *
    FROM bm25_ranked_unbounded ranked
    WHERE ranked.lane_rank <= p_limit
  ), dense_probe AS MATERIALIZED (
    SELECT
      shadow.child_chunk_id,
      shadow.search_profile_id,
      shadow.content_hash,
      shadow.embedding_vector <=> query_vector AS distance
    FROM knowledge_child_vector_shadow_projections shadow
    WHERE shadow.index_generation_id = p_index_generation_id
      AND shadow.search_profile_id = p_search_profile_id
      AND char_length(normalized_query) >= minimum_dense_query_characters
      AND shadow.collection_id = ANY(p_collection_ids)
      AND shadow.embedding_vector <=> query_vector <=
        1 - minimum_dense_similarity
    ORDER BY shadow.embedding_vector <=> query_vector
    LIMIT oversample_limit
  ), dense_scored AS MATERIALIZED (
    SELECT
      source.collection_id,
      source.document_id,
      source.document_version_id,
      source.index_generation_id,
      source.materialization_id,
      source.parent_chunk_id,
      source.child_chunk_id,
      source.source_span_hash,
      source.content_hash,
      1 - probe.distance AS similarity,
      source.child_ordinal
    FROM dense_probe probe
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_build_sources candidate
      WHERE candidate.child_chunk_id = probe.child_chunk_id
        AND candidate.index_generation_id = p_index_generation_id
        AND candidate.search_profile_id = p_search_profile_id
        AND candidate.search_profile_id = probe.search_profile_id
        AND candidate.content_hash = probe.content_hash
      OFFSET 0
    ) source
    ORDER BY
      probe.distance,
      source.child_ordinal,
      source.child_chunk_id
    LIMIT p_limit
  ), dense_ranked AS (
    SELECT
      scored.*,
      row_number() OVER (
        ORDER BY
          scored.similarity DESC,
          scored.child_ordinal,
          scored.child_chunk_id
      )::INTEGER AS lane_rank
    FROM dense_scored scored
  ), fused_base AS (
    SELECT
      COALESCE(bm25.collection_id, dense.collection_id) AS collection_id,
      COALESCE(bm25.document_id, dense.document_id) AS document_id,
      COALESCE(
        bm25.document_version_id,
        dense.document_version_id
      ) AS document_version_id,
      COALESCE(
        bm25.index_generation_id,
        dense.index_generation_id
      ) AS index_generation_id,
      COALESCE(
        bm25.materialization_id,
        dense.materialization_id
      ) AS materialization_id,
      COALESCE(bm25.parent_chunk_id, dense.parent_chunk_id)
        AS parent_chunk_id,
      COALESCE(bm25.child_chunk_id, dense.child_chunk_id) AS child_chunk_id,
      COALESCE(bm25.source_span_hash, dense.source_span_hash)
        AS source_span_hash,
      COALESCE(bm25.content_hash, dense.content_hash) AS content_hash,
      bm25.lane_rank AS bm25_rank,
      dense.lane_rank AS dense_rank,
      (
        CASE WHEN bm25.lane_rank IS NULL THEN 0
          ELSE 1.0 / (rrf_constant + bm25.lane_rank) END
        + CASE WHEN dense.lane_rank IS NULL THEN 0
          ELSE 1.0 / (rrf_constant + dense.lane_rank) END
      ) AS fused_score
    FROM bm25_ranked bm25
    FULL JOIN dense_ranked dense
      ON dense.child_chunk_id = bm25.child_chunk_id
  ), fused_ranked AS (
    SELECT
      fused.*,
      row_number() OVER (
        ORDER BY
          fused.fused_score DESC,
          least(
            COALESCE(fused.bm25_rank, 2147483647),
            COALESCE(fused.dense_rank, 2147483647)
          ),
          fused.child_chunk_id
      )::INTEGER AS fused_rank
    FROM fused_base fused
  )
  SELECT
    fused.collection_id,
    fused.document_id,
    fused.document_version_id,
    fused.index_generation_id,
    fused.materialization_id,
    fused.parent_chunk_id,
    fused.child_chunk_id,
    fused.source_span_hash,
    fused.content_hash,
    fused.fused_score::REAL
  FROM fused_ranked fused
  ORDER BY fused.fused_rank
  LIMIT p_limit;
END
$function$;
CREATE OR REPLACE FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding VECTOR(1024),
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  bm25_rank INTEGER,
  bm25_score DOUBLE PRECISION,
  dense_rank INTEGER,
  dense_score DOUBLE PRECISION,
  fused_rank INTEGER,
  fused_score DOUBLE PRECISION
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_profile RECORD;
  normalized_query TEXT;
  query_terms TEXT[];
  bm25_query TEXT;
  query_norm DOUBLE PRECISION;
  oversample_limit INTEGER;
  minimum_dense_similarity CONSTANT DOUBLE PRECISION := 0.48;
  minimum_dense_query_characters CONSTANT INTEGER := 8;
  rrf_constant CONSTANT DOUBLE PRECISION := 60.0;
BEGIN
  SELECT * INTO active_profile
  FROM knowledge_resolve_active_retrieval_profile();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING';
  END IF;
  normalized_query := btrim(p_query_text);
  IF p_collection_ids IS NULL
    OR cardinality(p_collection_ids) NOT BETWEEN 1 AND 32
    OR array_position(p_collection_ids, NULL) IS NOT NULL
    OR normalized_query IS NULL
    OR octet_length(normalized_query) NOT BETWEEN 1 AND 2048
    OR p_limit IS NULL
    OR p_limit NOT BETWEEN 1 AND 50
    OR p_query_embedding IS NULL
    OR EXISTS (
      SELECT 1
      FROM unnest(p_query_embedding::REAL[]) component
      WHERE component::TEXT IN ('NaN', 'Infinity', '-Infinity')
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_HYBRID_SHADOW_ARGUMENT_INVALID';
  END IF;

  query_norm := vector_norm(p_query_embedding);
  IF query_norm IS NULL
    OR query_norm <= 0
    OR query_norm::TEXT IN ('NaN', 'Infinity', '-Infinity')
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_HYBRID_SHADOW_ARGUMENT_INVALID';
  END IF;

  query_terms := knowledge_bm25_shadow_query_terms(normalized_query);
  bm25_query := knowledge_build_bm25_shadow_text(
    normalized_query,
    query_terms
  );
  oversample_limit := least(p_limit * 8, 400);

  RETURN QUERY
  WITH selected_collection AS (
    SELECT DISTINCT unnest(p_collection_ids) AS id
  ), bm25_probe AS MATERIALIZED (
    SELECT
      shadow.child_chunk_id,
      shadow.bm25_text <@> to_bm25query(
        bm25_query,
        'idx_knowledge_child_bm25_shadow_text'
      ) AS raw_score
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN selected_collection selected
      ON selected.id = shadow.collection_id
    WHERE shadow.index_generation_id = active_profile.index_generation_id
      AND shadow.search_profile_id = active_profile.search_profile_id
      AND shadow.bm25_text <@> to_bm25query(
      bm25_query,
      'idx_knowledge_child_bm25_shadow_text'
    ) < 0
    ORDER BY raw_score, shadow.child_chunk_id
    LIMIT oversample_limit
  ), exact_probe AS MATERIALIZED (
    SELECT shadow.child_chunk_id, 0::DOUBLE PRECISION AS raw_score
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN selected_collection selected
      ON selected.id = shadow.collection_id
    WHERE shadow.index_generation_id = active_profile.index_generation_id
      AND shadow.search_profile_id = active_profile.search_profile_id
      AND cardinality(query_terms) > 0
      AND shadow.exact_terms && query_terms
    ORDER BY shadow.child_chunk_id
    LIMIT oversample_limit
  ), lexical_pool AS (
    SELECT combined.child_chunk_id, min(combined.raw_score) AS raw_score
    FROM (
      SELECT probe.child_chunk_id, probe.raw_score FROM bm25_probe probe
      UNION ALL
      SELECT probe.child_chunk_id, probe.raw_score FROM exact_probe probe
    ) combined
    GROUP BY combined.child_chunk_id
  ), bm25_authorized AS (
    SELECT
      source.collection_id,
      source.document_id,
      source.document_version_id,
      source.index_generation_id,
      source.materialization_id,
      source.parent_chunk_id,
      source.child_chunk_id,
      source.source_span_hash,
      source.content_hash,
      pool.raw_score,
      shadow.exact_terms && query_terms AS exact_match,
      position(lower(normalized_query) IN lower(source.lexical_text)) > 0
        AS phrase_match,
      source.child_ordinal
    FROM lexical_pool pool
    JOIN knowledge_child_bm25_shadow_projections shadow
      ON shadow.child_chunk_id = pool.child_chunk_id
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_sources candidate
      WHERE candidate.child_chunk_id = shadow.child_chunk_id
        AND candidate.index_generation_id = shadow.index_generation_id
        AND candidate.search_profile_id = shadow.search_profile_id
        AND candidate.content_hash = shadow.content_hash
      OFFSET 0
    ) source
  ), bm25_ranked_unbounded AS (
    SELECT
      authorized.*,
      row_number() OVER (
        ORDER BY
          authorized.exact_match DESC,
          authorized.phrase_match DESC,
          authorized.raw_score,
          authorized.child_ordinal,
          authorized.child_chunk_id
      )::INTEGER AS lane_rank
    FROM bm25_authorized authorized
  ), bm25_ranked AS (
    SELECT *
    FROM bm25_ranked_unbounded ranked
    WHERE ranked.lane_rank <= p_limit
  ), dense_probe AS MATERIALIZED (
    SELECT
      shadow.child_chunk_id,
      shadow.index_generation_id,
      shadow.search_profile_id,
      shadow.content_hash,
      shadow.embedding_vector <=> p_query_embedding AS distance
    FROM knowledge_child_vector_shadow_projections shadow
    WHERE shadow.index_generation_id = active_profile.index_generation_id
      AND shadow.search_profile_id = active_profile.search_profile_id
      AND char_length(normalized_query) >= minimum_dense_query_characters
      AND shadow.collection_id = ANY(p_collection_ids)
      AND shadow.embedding_vector <=> p_query_embedding <=
        1 - minimum_dense_similarity
    ORDER BY shadow.embedding_vector <=> p_query_embedding
    LIMIT oversample_limit
  ), dense_scored AS MATERIALIZED (
    SELECT
      source.collection_id,
      source.document_id,
      source.document_version_id,
      source.index_generation_id,
      source.materialization_id,
      source.parent_chunk_id,
      source.child_chunk_id,
      source.source_span_hash,
      source.content_hash,
      1 - probe.distance AS similarity,
      source.child_ordinal
    FROM dense_probe probe
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_sources candidate
      WHERE candidate.child_chunk_id = probe.child_chunk_id
        AND candidate.index_generation_id = probe.index_generation_id
        AND candidate.search_profile_id = probe.search_profile_id
        AND candidate.content_hash = probe.content_hash
      OFFSET 0
    ) source
    ORDER BY
      probe.distance,
      source.child_ordinal,
      source.child_chunk_id
    LIMIT p_limit
  ), dense_ranked AS (
    SELECT
      scored.*,
      row_number() OVER (
        ORDER BY
          scored.similarity DESC,
          scored.child_ordinal,
          scored.child_chunk_id
      )::INTEGER AS lane_rank
    FROM dense_scored scored
  ), fused_base AS (
    SELECT
      COALESCE(bm25.collection_id, dense.collection_id) AS collection_id,
      COALESCE(bm25.document_id, dense.document_id) AS document_id,
      COALESCE(
        bm25.document_version_id,
        dense.document_version_id
      ) AS document_version_id,
      COALESCE(
        bm25.index_generation_id,
        dense.index_generation_id
      ) AS index_generation_id,
      COALESCE(
        bm25.materialization_id,
        dense.materialization_id
      ) AS materialization_id,
      COALESCE(bm25.parent_chunk_id, dense.parent_chunk_id)
        AS parent_chunk_id,
      COALESCE(bm25.child_chunk_id, dense.child_chunk_id) AS child_chunk_id,
      COALESCE(bm25.source_span_hash, dense.source_span_hash)
        AS source_span_hash,
      COALESCE(bm25.content_hash, dense.content_hash) AS content_hash,
      bm25.lane_rank AS bm25_rank,
      bm25.raw_score AS bm25_score,
      dense.lane_rank AS dense_rank,
      dense.similarity AS dense_score,
      (
        CASE WHEN bm25.lane_rank IS NULL THEN 0
          ELSE 1.0 / (rrf_constant + bm25.lane_rank) END
        + CASE WHEN dense.lane_rank IS NULL THEN 0
          ELSE 1.0 / (rrf_constant + dense.lane_rank) END
      ) AS fused_score
    FROM bm25_ranked bm25
    FULL JOIN dense_ranked dense
      ON dense.child_chunk_id = bm25.child_chunk_id
  ), fused_ranked AS (
    SELECT
      fused.*,
      row_number() OVER (
        ORDER BY
          fused.fused_score DESC,
          least(
            COALESCE(fused.bm25_rank, 2147483647),
            COALESCE(fused.dense_rank, 2147483647)
          ),
          fused.child_chunk_id
      )::INTEGER AS fused_rank
    FROM fused_base fused
  )
  SELECT
    fused.collection_id,
    fused.document_id,
    fused.document_version_id,
    fused.index_generation_id,
    fused.materialization_id,
    fused.parent_chunk_id,
    fused.child_chunk_id,
    fused.source_span_hash,
    fused.content_hash,
    fused.bm25_rank,
    fused.bm25_score,
    fused.dense_rank,
    fused.dense_score,
    fused.fused_rank,
    fused.fused_score
  FROM fused_ranked fused
  ORDER BY fused.fused_rank
  LIMIT p_limit;
END
$function$;
CREATE FUNCTION knowledge_fetch_fenced_query_evidence_candidates(
  p_expected_generation_id UUID,
  p_expected_search_profile_id UUID,
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  rank_score REAL
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  normalized_query TEXT;
  compact_query TEXT;
  query_terms TEXT[];
  query_bigrams TEXT[];
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_corpus_projection_head head
    JOIN knowledge_retrieval_profile_head retrieval_head
      ON retrieval_head.singleton_id = 1
     AND retrieval_head.active_profile = 'pg17_bm25_pgvector_v1'
    JOIN knowledge_index_generations generation
      ON generation.id = head.active_index_generation_id
     AND generation.status = 'active'
    JOIN knowledge_search_profiles profile
      ON profile.id = p_expected_search_profile_id
     AND profile.index_profile_id = generation.index_profile_id
    WHERE head.singleton_id = 1
      AND head.active_index_generation_id = p_expected_generation_id
      AND knowledge_retrieval_profile_id(
        profile.provider_profile_id, profile.embedding_processor,
        profile.embedding_model_id, profile.embedding_dimensions,
        profile.rerank_processor, profile.rerank_model_id
      ) IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_CHANGED';
  END IF;
  normalized_query := trim(p_query_text);
  IF p_collection_ids IS NULL
    OR cardinality(p_collection_ids) NOT BETWEEN 1 AND 32
    OR array_position(p_collection_ids, NULL) IS NOT NULL
    OR normalized_query IS NULL
    OR octet_length(normalized_query) NOT BETWEEN 1 AND 2048
    OR p_limit IS NULL
    OR p_limit NOT BETWEEN 1 AND 50
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_QUERY_CANDIDATE_ARGUMENT_INVALID';
  END IF;

  SELECT array_agg(term ORDER BY term) INTO query_terms
  FROM (
    SELECT DISTINCT lower(term) AS term
    FROM regexp_split_to_table(normalized_query, '[^[:alnum:]_]+'::TEXT) AS term
    WHERE length(trim(term)) > 0
    LIMIT 64
  ) terms;

  compact_query := lower(regexp_replace(
    normalized_query,
    '[[:space:][:punct:]，。！？；：、（）【】《》“”‘’…—·]+',
    '',
    'g'
  ));

  SELECT array_agg(term ORDER BY term) INTO query_bigrams
  FROM (
    SELECT DISTINCT substr(compact_query, ordinal, 2) AS term
    FROM generate_series(1, greatest(char_length(compact_query) - 1, 0))
      AS ordinal
    LIMIT 64
  ) terms;

  RETURN QUERY
  WITH selected_collection AS (
    SELECT DISTINCT unnest(p_collection_ids) AS id
  ), ranked AS (
    SELECT
      search.collection_id,
      search.document_id,
      search.document_version_id,
      search.index_generation_id,
      search.materialization_id,
      search.parent_chunk_id,
      search.child_chunk_id,
      search.source_span_hash,
      search.content_hash,
      (
        ts_rank(search.lexical_tsv, plainto_tsquery('simple', normalized_query))
        + CASE
            WHEN query_terms IS NOT NULL AND search.exact_terms && query_terms
              THEN 0.25
            ELSE 0
          END
        + CASE
            WHEN position(lower(normalized_query) IN lower(child.content)) > 0
              THEN 1.0
            ELSE 0
          END
        + CASE
            WHEN cardinality(query_bigrams) > 0
              THEN 0.5 * bigram_signal.hit_count::REAL /
                cardinality(query_bigrams)::REAL
            ELSE 0
          END
      )::REAL AS rank_score,
      child.ordinal AS child_ordinal
    FROM selected_collection selected
    JOIN knowledge_corpus_projection_head corpus
      ON corpus.singleton_id = 1
     AND corpus.active_index_generation_id = p_expected_generation_id
    JOIN knowledge_child_search_projections search
      ON search.collection_id = selected.id
     AND search.index_generation_id = p_expected_generation_id
     AND search.search_profile_id = p_expected_search_profile_id
    JOIN knowledge_search_profiles search_profile
      ON search_profile.id = search.search_profile_id
    JOIN knowledge_document_projection_heads head
      ON head.index_generation_id = search.index_generation_id
     AND head.document_id = search.document_id
     AND head.active_materialization_id = search.materialization_id
    JOIN knowledge_document_materializations materialization
      ON materialization.id = search.materialization_id
     AND materialization.index_generation_id = search.index_generation_id
     AND materialization.collection_id = search.collection_id
     AND materialization.document_id = search.document_id
     AND materialization.document_version_id = search.document_version_id
     AND materialization.status = 'published'
    JOIN knowledge_collections collection
      ON collection.id = search.collection_id
     AND collection.deleted_at IS NULL
     AND collection.visibility_epoch = materialization.collection_visibility_epoch
     AND collection.collection_processing_revision =
       materialization.collection_processing_revision
    JOIN knowledge_documents document
      ON document.id = search.document_id
     AND document.collection_id = search.collection_id
     AND document.status = 'active'
     AND document.deleted_at IS NULL
     AND document.current_version_id = search.document_version_id
     AND document.visibility_epoch = materialization.document_visibility_epoch
    JOIN knowledge_document_versions version
      ON version.id = search.document_version_id
     AND version.document_id = search.document_id
     AND version.status = 'active'
     AND version.content_hash = materialization.source_content_hash
    JOIN knowledge_child_chunks child
      ON child.id = search.child_chunk_id
     AND child.parent_chunk_id = search.parent_chunk_id
     AND child.materialization_id = search.materialization_id
     AND child.index_generation_id = search.index_generation_id
     AND child.document_id = search.document_id
     AND child.document_version_id = search.document_version_id
     AND child.source_span_hash = search.source_span_hash
     AND child.chunk_profile_hash = search.chunk_profile_hash
     AND child.content_hash = search.content_hash
    CROSS JOIN LATERAL (
      SELECT count(*)::INTEGER AS hit_count
      FROM unnest(coalesce(query_bigrams, ARRAY[]::TEXT[])) AS bigram(term)
      WHERE position(bigram.term IN lower(child.content)) > 0
    ) bigram_signal
    WHERE search.status = 'ready'
      AND search.embedding_model_id = search_profile.embedding_model_id
      AND search.embedding_dimensions = search_profile.embedding_dimensions
      AND search.embedding_vector IS NOT NULL
      AND search.embedding_vector_sha256 IS NOT NULL
      AND (
        search.lexical_tsv @@ plainto_tsquery('simple', normalized_query)
        OR (query_terms IS NOT NULL AND search.exact_terms && query_terms)
        OR position(lower(normalized_query) IN lower(child.content)) > 0
        OR (
          cardinality(query_bigrams) > 0
          AND bigram_signal.hit_count >= least(2, cardinality(query_bigrams))
        )
      )
  )
  SELECT
    ranked.collection_id,
    ranked.document_id,
    ranked.document_version_id,
    ranked.index_generation_id,
    ranked.materialization_id,
    ranked.parent_chunk_id,
    ranked.child_chunk_id,
    ranked.source_span_hash,
    ranked.content_hash,
    ranked.rank_score
  FROM ranked
  ORDER BY ranked.rank_score DESC, ranked.child_ordinal, ranked.child_chunk_id
  LIMIT p_limit;
END
$function$;

ALTER FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) RENAME TO knowledge_fetch_query_evidence_candidates_v48_legacy;

CREATE OR REPLACE FUNCTION knowledge_fetch_query_evidence_candidates(
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  rank_score REAL
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_profile RECORD;
  selected_retrieval_profile TEXT;
BEGIN
  SELECT head.active_profile INTO selected_retrieval_profile
  FROM knowledge_retrieval_profile_head head
  WHERE head.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;
  IF selected_retrieval_profile = 'legacy' THEN
    RETURN QUERY SELECT candidate.*
    FROM knowledge_fetch_query_evidence_candidates_v48_legacy(
      p_collection_ids, p_query_text, p_limit
    ) candidate;
    RETURN;
  END IF;
  IF selected_retrieval_profile <> 'pg17_bm25_pgvector_v1' THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
  END IF;
  SELECT * INTO active_profile
  FROM knowledge_resolve_active_retrieval_profile();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING';
  END IF;
  RETURN QUERY SELECT candidate.*
  FROM knowledge_fetch_fenced_query_evidence_candidates(
    active_profile.index_generation_id, active_profile.search_profile_id,
    p_collection_ids, p_query_text, p_limit
  ) candidate;
END
$function$;

CREATE FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  p_expected_generation_id UUID,
  p_expected_search_profile_id UUID,
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding REAL[],
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  rank_score REAL
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_corpus_projection_head head
    JOIN knowledge_retrieval_profile_head retrieval_head
      ON retrieval_head.singleton_id = 1
     AND retrieval_head.active_profile = 'pg17_bm25_pgvector_v1'
    JOIN knowledge_index_generations generation
      ON generation.id = head.active_index_generation_id
     AND generation.status = 'active'
    JOIN knowledge_search_profiles profile
      ON profile.id = p_expected_search_profile_id
     AND profile.index_profile_id = generation.index_profile_id
    WHERE head.singleton_id = 1
      AND head.active_index_generation_id = p_expected_generation_id
      AND knowledge_retrieval_profile_id(
        profile.provider_profile_id, profile.embedding_processor,
        profile.embedding_model_id, profile.embedding_dimensions,
        profile.rerank_processor, profile.rerank_model_id
      ) IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING ERRCODE = '40001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_CHANGED';
  END IF;

  RETURN QUERY
  WITH base AS MATERIALIZED (
    SELECT candidate.*,
      row_number() OVER (
        ORDER BY candidate.rank_score DESC, candidate.child_chunk_id
      )::INTEGER AS base_rank
    FROM knowledge_fetch_fenced_hybrid_candidates_core(
      p_expected_generation_id, p_expected_search_profile_id,
      p_collection_ids, p_query_text, p_query_embedding, p_limit
    ) candidate
  ), source_name AS MATERIALIZED (
    SELECT candidate.*
    FROM knowledge_fetch_source_name_evidence_candidates(
      p_expected_generation_id, p_collection_ids, p_query_text, p_limit
    ) candidate
  ), fused AS (
    SELECT
      COALESCE(base.collection_id, source_name.collection_id) AS collection_id,
      COALESCE(base.document_id, source_name.document_id) AS document_id,
      COALESCE(base.document_version_id, source_name.document_version_id)
        AS document_version_id,
      COALESCE(base.index_generation_id, source_name.index_generation_id)
        AS index_generation_id,
      COALESCE(base.materialization_id, source_name.materialization_id)
        AS materialization_id,
      COALESCE(base.parent_chunk_id, source_name.parent_chunk_id)
        AS parent_chunk_id,
      COALESCE(base.child_chunk_id, source_name.child_chunk_id)
        AS child_chunk_id,
      COALESCE(base.source_span_hash, source_name.source_span_hash)
        AS source_span_hash,
      COALESCE(base.content_hash, source_name.content_hash) AS content_hash,
      base.base_rank,
      source_name.source_rank,
      COALESCE(base.rank_score::DOUBLE PRECISION, 0) +
        CASE WHEN source_name.source_rank IS NULL THEN 0
          ELSE 2.0 / (60.0 + source_name.source_rank) END AS fused_score
    FROM base
    FULL JOIN source_name
      ON source_name.child_chunk_id = base.child_chunk_id
  )
  SELECT fused.collection_id, fused.document_id,
    fused.document_version_id, fused.index_generation_id,
    fused.materialization_id, fused.parent_chunk_id, fused.child_chunk_id,
    fused.source_span_hash, fused.content_hash, fused.fused_score::REAL
  FROM fused
  ORDER BY fused.fused_score DESC,
    least(COALESCE(fused.source_rank, 2147483647),
      COALESCE(fused.base_rank, 2147483647)),
    fused.child_chunk_id
  LIMIT p_limit;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding REAL[],
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  rank_score REAL
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_profile RECORD;
  selected_retrieval_profile TEXT;
BEGIN
  SELECT head.active_profile INTO selected_retrieval_profile
  FROM knowledge_retrieval_profile_head head
  WHERE head.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;
  IF selected_retrieval_profile = 'legacy' THEN
    RETURN QUERY SELECT candidate.*
    FROM knowledge_fetch_profiled_query_evidence_candidates_v47_base(
      p_collection_ids, p_query_text, p_query_embedding, p_limit
    ) candidate;
    RETURN;
  END IF;
  IF selected_retrieval_profile <> 'pg17_bm25_pgvector_v1' THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
  END IF;
  SELECT * INTO active_profile
  FROM knowledge_resolve_active_retrieval_profile();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING';
  END IF;
  RETURN QUERY SELECT candidate.*
  FROM knowledge_fetch_fenced_profiled_query_evidence_candidates(
    active_profile.index_generation_id, active_profile.search_profile_id,
    p_collection_ids, p_query_text, p_query_embedding, p_limit
  ) candidate;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_fetch_generation_evaluation_candidates_v47_base(
  p_index_generation_id UUID,
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding REAL[],
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  rank_score REAL
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  generation_profile RECORD;
BEGIN
  SELECT * INTO generation_profile
  FROM knowledge_resolve_generation_retrieval_profile(p_index_generation_id);
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = '55000',
      MESSAGE = 'RAG_GENERATION_EVALUATION_GENERATION_UNAVAILABLE';
  END IF;
  RETURN QUERY SELECT candidate.*
  FROM knowledge_fetch_fenced_hybrid_candidates_core(
    p_index_generation_id, generation_profile.search_profile_id,
    p_collection_ids, p_query_text, p_query_embedding, p_limit
  ) candidate;
END
$function$;

ALTER FUNCTION knowledge_begin_structure_generation_rebuild(
  UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_begin_registered_structure_generation_rebuild(
  UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_verify_structure_generation(UUID, BIGINT, TEXT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_rollback_index_generation(
  UUID, UUID, BIGINT, TEXT, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_reauthorize_and_hydrate_evidence_v47_base(
  UUID, UUID, UUID, JSONB
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_hydrate_generation_evaluation_evidence_v47_base(
  UUID, UUID[], JSONB
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_fenced_hybrid_candidates_core(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_fenced_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_query_evidence_candidates_v48_legacy(
  UUID[], TEXT, INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_generation_evaluation_candidates_v47_base(
  UUID, UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_fetch_fenced_hybrid_candidates_core(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor;
REVOKE ALL ON FUNCTION knowledge_fetch_fenced_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_query_evidence_candidates_v48_legacy(
  UUID[], TEXT, INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates_v47_base(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_reauthorize_and_hydrate_evidence_v47_base(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC, go_api_runtime, go_evidence_hydrator;
REVOKE ALL ON FUNCTION knowledge_hydrate_generation_evaluation_evidence_v47_base(
  UUID, UUID[], JSONB
) FROM PUBLIC, rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_fenced_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, INTEGER
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) TO rag_worker_executor, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_assert_pg17_generation_ready(UUID),
  knowledge_assert_pg17_retrieval_profile_ready(),
  knowledge_verify_structure_generation(UUID, BIGINT, TEXT),
  knowledge_rollback_index_generation(UUID, UUID, BIGINT, TEXT, TEXT),
  knowledge_begin_registered_structure_generation_rebuild(
    UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
  ) TO rag_replay_operator;
