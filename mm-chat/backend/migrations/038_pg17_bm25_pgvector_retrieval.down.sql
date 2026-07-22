-- Roll application traffic and the durable retrieval pointer back to
-- `legacy` before removing the PostgreSQL 17 retrieval objects. The extensions
-- remain installed because they are server capabilities, not rollback data.

-- Frozen from `mm-chat/ops/g18-profile-cutover/15-generation-cutover-fence.down.sql`.
DO $rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile <> 'legacy'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY';
  END IF;
END
$rollback_guard$;

DROP TRIGGER knowledge_corpus_head_pg17_retrieval_fence
  ON knowledge_corpus_projection_head;
REVOKE EXECUTE ON FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  FROM rag_replay_operator;
DROP FUNCTION knowledge_fence_pg17_generation_cutover();
DROP FUNCTION knowledge_assert_pg17_generation_ready(UUID);

SELECT 'PASS G18.5B.2b generation cutover fence rollback' AS result;

-- Frozen from `mm-chat/ops/g18-profile-cutover/10-active-projection-maintenance.down.sql`.
DO $rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile <> 'legacy'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY';
  END IF;
END
$rollback_guard$;

DROP TRIGGER knowledge_document_projection_head_pg17_retrieval
  ON knowledge_document_projection_heads;
REVOKE EXECUTE ON FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  FROM rag_replay_operator;
DROP FUNCTION knowledge_maintain_pg17_retrieval_on_head();
DROP FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID);

SELECT 'PASS G18.5B.2a active projection maintenance rollback' AS result;

-- Frozen from `mm-chat/ops/g18-profile-cutover/00-profile-router.down.sql`.
DO $rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile <> 'legacy'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY';
  END IF;
END
$rollback_guard$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE OR REPLACE FUNCTION knowledge_set_retrieval_profile(
  p_expected_profile TEXT,
  p_target_profile TEXT,
  p_expected_revision BIGINT,
  p_reason TEXT
) RETURNS TABLE(
  active_profile TEXT,
  revision BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  head knowledge_retrieval_profile_head%ROWTYPE;
  normalized_reason TEXT;
BEGIN
  normalized_reason := btrim(p_reason);
  IF p_expected_profile IS NULL
    OR p_expected_profile NOT IN ('legacy', 'pg17_bm25_pgvector_v1')
    OR p_target_profile IS NULL
    OR p_target_profile NOT IN ('legacy', 'pg17_bm25_pgvector_v1')
    OR p_expected_revision IS NULL
    OR p_expected_revision < 1
    OR normalized_reason IS NULL
    OR octet_length(normalized_reason) NOT BETWEEN 1 AND 512
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 5);
  SELECT profile.* INTO head
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;
  IF head.active_profile <> p_expected_profile
    OR head.revision <> p_expected_revision
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '40001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_CONFLICT';
  END IF;

  IF p_target_profile = 'pg17_bm25_pgvector_v1' THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
  END IF;
  IF p_target_profile = head.active_profile THEN
    RETURN QUERY SELECT head.active_profile, head.revision;
    RETURN;
  END IF;

  UPDATE knowledge_retrieval_profile_head profile
  SET active_profile = p_target_profile,
      revision = head.revision + 1,
      updated_at = clock_timestamp()
  WHERE profile.singleton_id = 1
  RETURNING profile.* INTO head;
  INSERT INTO knowledge_retrieval_profile_transitions (
    from_profile, to_profile, revision, reason
  ) VALUES (
    p_expected_profile,
    head.active_profile,
    head.revision,
    normalized_reason
  );

  RETURN QUERY SELECT head.active_profile, head.revision;
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
  selected_profile TEXT;
BEGIN
  SELECT profile.active_profile INTO selected_profile
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;

  IF selected_profile = 'legacy' THEN
    RETURN QUERY
    SELECT candidate.*
    FROM knowledge_fetch_hybrid_query_evidence_candidates(
      p_collection_ids,
      p_query_text,
      p_query_embedding,
      p_limit
    ) candidate;
    RETURN;
  END IF;

  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
END
$function$;

ALTER FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;

REVOKE EXECUTE ON FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  FROM rag_replay_operator;
DROP FUNCTION knowledge_assert_pg17_retrieval_profile_ready();

SELECT 'PASS G18.5B.1 profile router candidate rollback' AS result;

-- Frozen from `mm-chat/ops/g18-hybrid-shadow/00-shadow-schema.down.sql`.
SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

DROP FUNCTION IF EXISTS knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
);
DROP FUNCTION IF EXISTS knowledge_backfill_bm25_shadow(UUID, UUID);
DROP TABLE IF EXISTS knowledge_child_bm25_shadow_projections;
DROP FUNCTION IF EXISTS knowledge_validate_bm25_shadow_insert();
DROP VIEW IF EXISTS knowledge_bm25_shadow_sources;
DROP VIEW IF EXISTS knowledge_bm25_shadow_build_sources;
DROP FUNCTION IF EXISTS knowledge_build_bm25_shadow_text(TEXT, TEXT[]);
DROP FUNCTION IF EXISTS knowledge_bm25_shadow_query_terms(TEXT);
DROP FUNCTION IF EXISTS knowledge_normalize_bm25_shadow_terms(TEXT[]);

SELECT 'PASS G18.4 BM25 hybrid shadow rollback' AS result;

-- Frozen from `mm-chat/ops/g18-pgvector-shadow/00-shadow-schema.down.sql`.
SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

DROP FUNCTION IF EXISTS knowledge_backfill_pgvector_shadow(UUID, UUID);
DROP TABLE IF EXISTS knowledge_child_vector_shadow_projections;
DROP FUNCTION IF EXISTS knowledge_validate_pgvector_shadow_insert();
DROP VIEW IF EXISTS knowledge_pgvector_shadow_sources;

SELECT 'PASS G18.3 pgvector shadow rollback' AS result;
