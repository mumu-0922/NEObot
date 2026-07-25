-- Permanently retire every executable Jina RAG seam while preserving its
-- Generation, projection, profile, and audit rows as read-only history.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

-- Provider credentials are not historical retrieval evidence. Remove the
-- encrypted payload and its connection attestation, then hide the retired
-- provider record from ordinary repository reads.
UPDATE provider_configs
SET encrypted_secret_ref = NULL,
  config = (config - 'connectionTestSHA256' - 'connectionTestedAt') ||
    '{"enabled":false}'::jsonb,
  deleted_at = COALESCE(deleted_at, clock_timestamp()),
  updated_at = clock_timestamp()
WHERE upper(trim(provider_id)) = 'RAG:JINA'
   OR (
     lower(trim(config->>'kind')) = 'rag'
     AND lower(trim(config->>'ragProvider')) = 'jina'
   );

DO $verify_jina_provider_secret_purge$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM provider_configs
    WHERE (
        upper(trim(provider_id)) = 'RAG:JINA'
        OR (
          lower(trim(config->>'kind')) = 'rag'
          AND lower(trim(config->>'ragProvider')) = 'jina'
        )
      )
      AND (
        encrypted_secret_ref IS NOT NULL
        OR COALESCE((config->>'enabled')::boolean, false)
        OR config ? 'connectionTestSHA256'
        OR config ? 'connectionTestedAt'
        OR deleted_at IS NULL
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_JINA_PROVIDER_SECRET_PURGE_FAILED';
  END IF;
END
$verify_jina_provider_secret_purge$;

-- The Web Reader is not retrieval evidence. Remove its durable executable
-- registry entry; application-level denylisting remains authoritative for
-- stale restores and databases that have not applied this migration yet.
DELETE FROM plugin_registry
WHERE lower(trim(plugin_id)) = 'jina-web-reader';

DO $verify_jina_plugin_registry_purge$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM plugin_registry
    WHERE lower(trim(plugin_id)) = 'jina-web-reader'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_JINA_PLUGIN_REGISTRY_PURGE_FAILED';
  END IF;
END
$verify_jina_plugin_registry_purge$;

-- The current Jina Active row may remain active during the temporary
-- BM25-only window. Any later transition into Active, including rollback,
-- must resolve to the exact admitted SiliconFlow BGE tuple.
CREATE FUNCTION knowledge_enforce_bge_active_generation_transition()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NEW.status = 'active'
    AND (
      TG_OP = 'INSERT'
      OR OLD.status IS DISTINCT FROM 'active'
      OR OLD.index_profile_id IS DISTINCT FROM NEW.index_profile_id
    )
    AND NOT EXISTS (
      SELECT 1
      FROM knowledge_index_profiles index_profile
      JOIN knowledge_search_profiles search_profile
        ON search_profile.index_profile_id = index_profile.id
      WHERE index_profile.id = NEW.index_profile_id
        AND index_profile.embedding_processor = 'siliconflow'
        AND index_profile.embedding_model_id = 'Pro/BAAI/bge-m3'
        AND index_profile.rerank_processor = 'siliconflow'
        AND index_profile.rerank_model_id = 'Pro/BAAI/bge-reranker-v2-m3'
        AND search_profile.provider_profile_id = 'siliconflow_bge_m3_v1'
        AND search_profile.embedding_processor = 'siliconflow'
        AND search_profile.embedding_model_id = 'Pro/BAAI/bge-m3'
        AND search_profile.embedding_dimensions = 1024
        AND search_profile.rerank_processor = 'siliconflow'
        AND search_profile.rerank_model_id = 'Pro/BAAI/bge-reranker-v2-m3'
        AND knowledge_retrieval_profile_id(
          search_profile.provider_profile_id,
          search_profile.embedding_processor,
          search_profile.embedding_model_id,
          search_profile.embedding_dimensions,
          search_profile.rerank_processor,
          search_profile.rerank_model_id
        ) = 'siliconflow_bge_m3_v1'
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETIRED_RETRIEVAL_GENERATION_ACTIVATION_FORBIDDEN';
  END IF;
  RETURN NEW;
END
$function$;

ALTER FUNCTION knowledge_enforce_bge_active_generation_transition()
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_enforce_bge_active_generation_transition()
  FROM PUBLIC;

CREATE TRIGGER knowledge_index_generation_bge_active_insert_fence
BEFORE INSERT ON knowledge_index_generations
FOR EACH ROW
EXECUTE FUNCTION knowledge_enforce_bge_active_generation_transition();

CREATE TRIGGER knowledge_index_generation_bge_active_update_fence
BEFORE UPDATE OF status, index_profile_id ON knowledge_index_generations
FOR EACH ROW
EXECUTE FUNCTION knowledge_enforce_bge_active_generation_transition();

-- Preserve the historical Active binding only as lexical identity. Every
-- vector-consuming reader must prove the exact BGE tuple before it can reach
-- the pre-retirement implementation.
ALTER FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) RENAME TO knowledge_fetch_fenced_profiled_candidates_v49_base;

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
DECLARE
  executable_search_profile_id UUID;
BEGIN
  SELECT profile.id INTO executable_search_profile_id
    FROM knowledge_index_generations generation
    JOIN knowledge_search_profiles profile
      ON profile.index_profile_id = generation.index_profile_id
    WHERE generation.id = p_expected_generation_id
      AND profile.provider_profile_id = 'siliconflow_bge_m3_v1'
      AND profile.embedding_processor = 'siliconflow'
      AND profile.embedding_model_id = 'Pro/BAAI/bge-m3'
      AND profile.embedding_dimensions = 1024
      AND profile.rerank_processor = 'siliconflow'
      AND profile.rerank_model_id = 'Pro/BAAI/bge-reranker-v2-m3'
      AND knowledge_retrieval_profile_id(
        profile.provider_profile_id, profile.embedding_processor,
        profile.embedding_model_id, profile.embedding_dimensions,
        profile.rerank_processor, profile.rerank_model_id
      ) = 'siliconflow_bge_m3_v1';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETIRED_RETRIEVAL_PROFILE_NON_EXECUTABLE';
  END IF;
  IF executable_search_profile_id IS DISTINCT FROM p_expected_search_profile_id
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '40001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_CHANGED';
  END IF;

  RETURN QUERY
  SELECT candidate.*
  FROM knowledge_fetch_fenced_profiled_candidates_v49_base(
    p_expected_generation_id, p_expected_search_profile_id,
    p_collection_ids, p_query_text, p_query_embedding, p_limit
  ) candidate;
END
$function$;

-- The stable compatibility signature remains callable, but a legacy/Jina
-- Active binding is forced through its lexical reader and never consumes the
-- caller-supplied vector.
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) RENAME TO knowledge_fetch_profiled_query_evidence_candidates_v49_base;

CREATE FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
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
BEGIN
  SELECT * INTO active_profile
  FROM knowledge_resolve_active_retrieval_profile();

  IF FOUND
    AND active_profile.retrieval_profile_id = 'siliconflow_bge_m3_v1'
    AND active_profile.provider_id = 'siliconflow'
    AND active_profile.embedding_model_id = 'Pro/BAAI/bge-m3'
    AND active_profile.embedding_dimensions = 1024
    AND active_profile.rerank_model_id = 'Pro/BAAI/bge-reranker-v2-m3'
  THEN
    RETURN QUERY
    SELECT candidate.*
    FROM knowledge_fetch_profiled_query_evidence_candidates_v49_base(
      p_collection_ids, p_query_text, p_query_embedding, p_limit
    ) candidate;
    RETURN;
  END IF;

  RETURN QUERY
  SELECT candidate.*
  FROM knowledge_fetch_query_evidence_candidates(
    p_collection_ids, p_query_text, p_limit
  ) candidate;
END
$function$;

-- Candidate evaluation is BGE-only. The historical evaluator implementation
-- remains hash/audit history but cannot read a Jina Dense projection.
ALTER FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) RENAME TO knowledge_fetch_generation_evaluation_candidates_v49_base;

CREATE FUNCTION knowledge_fetch_generation_evaluation_candidates(
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
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_index_generations generation
    JOIN knowledge_search_profiles profile
      ON profile.index_profile_id = generation.index_profile_id
    WHERE generation.id = p_index_generation_id
      AND profile.provider_profile_id = 'siliconflow_bge_m3_v1'
      AND profile.embedding_processor = 'siliconflow'
      AND profile.embedding_model_id = 'Pro/BAAI/bge-m3'
      AND profile.embedding_dimensions = 1024
      AND profile.rerank_processor = 'siliconflow'
      AND profile.rerank_model_id = 'Pro/BAAI/bge-reranker-v2-m3'
      AND knowledge_retrieval_profile_id(
        profile.provider_profile_id, profile.embedding_processor,
        profile.embedding_model_id, profile.embedding_dimensions,
        profile.rerank_processor, profile.rerank_model_id
      ) = 'siliconflow_bge_m3_v1'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETIRED_RETRIEVAL_PROFILE_NON_EXECUTABLE';
  END IF;

  RETURN QUERY
  SELECT candidate.*
  FROM knowledge_fetch_generation_evaluation_candidates_v49_base(
    p_index_generation_id, p_collection_ids, p_query_text,
    p_query_embedding, p_limit
  ) candidate;
END
$function$;

-- Operator diagnostics are also vector-consuming execution and therefore may
-- run only against the current BGE Generation/Profile.
ALTER FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) RENAME TO knowledge_fetch_hybrid_shadow_diagnostics_v49_base;

CREATE FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
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
BEGIN
  SELECT * INTO active_profile
  FROM knowledge_resolve_active_retrieval_profile();
  IF NOT FOUND
    OR active_profile.retrieval_profile_id <> 'siliconflow_bge_m3_v1'
    OR active_profile.provider_id <> 'siliconflow'
    OR active_profile.embedding_model_id <> 'Pro/BAAI/bge-m3'
    OR active_profile.embedding_dimensions <> 1024
    OR active_profile.rerank_model_id <> 'Pro/BAAI/bge-reranker-v2-m3'
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETIRED_RETRIEVAL_PROFILE_NON_EXECUTABLE';
  END IF;

  RETURN QUERY
  SELECT diagnostic.*
  FROM knowledge_fetch_hybrid_shadow_diagnostics_v49_base(
    p_collection_ids, p_query_text, p_query_embedding, p_limit
  ) diagnostic;
END
$function$;

ALTER FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION
  knowledge_fetch_fenced_profiled_candidates_v49_base(
    UUID, UUID, UUID[], TEXT, REAL[], INTEGER
  ) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates_v49_base(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates_v49_base(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics_v49_base(
  UUID[], TEXT, VECTOR, INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;

REVOKE ALL ON FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor;

GRANT EXECUTE ON FUNCTION knowledge_fetch_fenced_profiled_query_evidence_candidates(
  UUID, UUID, UUID[], TEXT, REAL[], INTEGER
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) TO rag_replay_operator;
