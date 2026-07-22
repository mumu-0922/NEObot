\set ON_ERROR_STOP on

DO $extension_contract$
BEGIN
  IF current_setting('server_version_num')::integer / 10000 <> 17 THEN
    RAISE EXCEPTION 'G18 profile cutover requires PostgreSQL 17';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'pg_textsearch')
      <> '1.3.1' THEN
    RAISE EXCEPTION 'G18 profile cutover requires pg_textsearch 1.3.1';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'vector')
      <> '0.8.5' THEN
    RAISE EXCEPTION 'G18 profile cutover requires pgvector 0.8.5';
  END IF;
  IF to_regclass('knowledge_child_vector_shadow_projections') IS NULL
    OR to_regclass('knowledge_child_bm25_shadow_projections') IS NULL
    OR to_regprocedure(
      'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)'
    ) IS NULL
  THEN
    RAISE EXCEPTION 'G18 profile cutover requires reviewed shadow schema';
  END IF;
END
$extension_contract$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
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
   AND profile.provider_profile_id = 'mineru_jina_postgres_v1'
   AND profile.embedding_processor = 'jina'
   AND profile.embedding_model_id = 'jina-embeddings-v4'
   AND profile.embedding_dimensions = 1024
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

  PERFORM pg_advisory_xact_lock(1296912978, 3);
  PERFORM pg_advisory_xact_lock(1296912978, 4);
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
    PERFORM * FROM knowledge_assert_pg17_retrieval_profile_ready();
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
  query_norm DOUBLE PRECISION;
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

  IF selected_profile = 'pg17_bm25_pgvector_v1' THEN
    IF p_query_embedding IS NULL
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
        MESSAGE = 'RAG_HYBRID_QUERY_EMBEDDING_INVALID';
    END IF;
    SELECT sqrt(sum(
      component::DOUBLE PRECISION * component::DOUBLE PRECISION
    )) INTO query_norm
    FROM unnest(p_query_embedding) component;
    IF query_norm IS NULL OR query_norm <= 0 THEN
      RAISE EXCEPTION USING
        ERRCODE = '22023',
        MESSAGE = 'RAG_HYBRID_QUERY_EMBEDDING_INVALID';
    END IF;

    RETURN QUERY
    SELECT
      candidate.collection_id,
      candidate.document_id,
      candidate.document_version_id,
      candidate.index_generation_id,
      candidate.materialization_id,
      candidate.parent_chunk_id,
      candidate.child_chunk_id,
      candidate.source_span_hash,
      candidate.content_hash,
      candidate.fused_score::REAL
    FROM knowledge_fetch_hybrid_shadow_diagnostics(
      p_collection_ids,
      p_query_text,
      p_query_embedding::VECTOR(1024),
      p_limit
    ) candidate
    ORDER BY candidate.fused_rank;
    RETURN;
  END IF;

  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
END
$function$;

ALTER FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;

SELECT 'PASS G18.5B.1 profile router candidate' AS result;
