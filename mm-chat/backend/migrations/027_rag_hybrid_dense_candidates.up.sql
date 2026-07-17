-- G11.9C.2 selected-collection hybrid retrieval.
--
-- Query vectors are produced by the private Python/Jina retrieval.query
-- boundary. This function returns reference-only candidates and keeps the
-- existing publication, generation, visibility, and deletion fences. Lexical
-- and Dense ranks are fused with deterministic RRF before Go reauthorizes and
-- hydrates any source text.

CREATE FUNCTION knowledge_fetch_hybrid_query_evidence_candidates(
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
  query_norm DOUBLE PRECISION;
  -- Calibrated against the live selected corpus: 0.48 preserves the
  -- no-lexical-overlap semantic probe while rejecting weather/cooking noise.
  minimum_dense_similarity CONSTANT DOUBLE PRECISION := 0.48;
  minimum_dense_query_characters CONSTANT INTEGER := 8;
  rrf_constant CONSTANT DOUBLE PRECISION := 60.0;
BEGIN
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

  SELECT sqrt(sum(component::DOUBLE PRECISION * component::DOUBLE PRECISION))
  INTO query_norm
  FROM unnest(p_query_embedding) component;
  IF query_norm IS NULL OR query_norm <= 0 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_HYBRID_QUERY_EMBEDDING_INVALID';
  END IF;

  RETURN QUERY
  WITH lexical_candidates AS (
    SELECT
      candidate.*,
      row_number() OVER (
        ORDER BY candidate.rank_score DESC, candidate.child_chunk_id
      ) AS lane_rank
    FROM knowledge_fetch_query_evidence_candidates(
      p_collection_ids,
      p_query_text,
      p_limit
    ) candidate
  ), selected_collection AS (
    SELECT DISTINCT unnest(p_collection_ids) AS id
  ), dense_scored AS (
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
        vector_score.dot_product /
        (vector_score.document_norm * query_norm)
      ) AS cosine_similarity,
      child.ordinal AS child_ordinal
    FROM selected_collection selected
    JOIN knowledge_corpus_projection_head corpus
      ON corpus.singleton_id = 1
    JOIN knowledge_child_search_projections search
      ON search.collection_id = selected.id
     AND search.index_generation_id = corpus.active_index_generation_id
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
      SELECT
        sum(
          document_component::DOUBLE PRECISION *
          query_component::DOUBLE PRECISION
        ) AS dot_product,
        sqrt(sum(
          document_component::DOUBLE PRECISION *
          document_component::DOUBLE PRECISION
        )) AS document_norm
      FROM unnest(search.embedding_vector) WITH ORDINALITY
        document_vector(document_component, ordinal)
      JOIN unnest(p_query_embedding) WITH ORDINALITY
        query_vector(query_component, ordinal)
        USING (ordinal)
    ) vector_score
    WHERE search.status = 'ready'
      AND search.embedding_model_id = 'jina-embeddings-v4'
      AND search.embedding_dimensions = 1024
      AND search.embedding_vector IS NOT NULL
      AND search.embedding_vector_sha256 IS NOT NULL
      AND cardinality(search.embedding_vector) = 1024
      AND vector_score.document_norm > 0
  ), dense_candidates AS (
    SELECT
      dense_scored.*,
      row_number() OVER (
        ORDER BY
          dense_scored.cosine_similarity DESC,
          dense_scored.child_ordinal,
          dense_scored.child_chunk_id
      ) AS lane_rank
    FROM dense_scored
    WHERE char_length(btrim(p_query_text)) >= minimum_dense_query_characters
      AND dense_scored.cosine_similarity >= minimum_dense_similarity
    ORDER BY
      dense_scored.cosine_similarity DESC,
      dense_scored.child_ordinal,
      dense_scored.child_chunk_id
    LIMIT p_limit
  ), lanes AS (
    SELECT
      lexical.collection_id,
      lexical.document_id,
      lexical.document_version_id,
      lexical.index_generation_id,
      lexical.materialization_id,
      lexical.parent_chunk_id,
      lexical.child_chunk_id,
      lexical.source_span_hash,
      lexical.content_hash,
      lexical.lane_rank
    FROM lexical_candidates lexical
    UNION ALL
    SELECT
      dense.collection_id,
      dense.document_id,
      dense.document_version_id,
      dense.index_generation_id,
      dense.materialization_id,
      dense.parent_chunk_id,
      dense.child_chunk_id,
      dense.source_span_hash,
      dense.content_hash,
      dense.lane_rank
    FROM dense_candidates dense
  ), fused AS (
    SELECT
      lanes.collection_id,
      lanes.document_id,
      lanes.document_version_id,
      lanes.index_generation_id,
      lanes.materialization_id,
      lanes.parent_chunk_id,
      lanes.child_chunk_id,
      lanes.source_span_hash,
      lanes.content_hash,
      sum(1.0 / (rrf_constant + lanes.lane_rank))::REAL AS rank_score
    FROM lanes
    GROUP BY
      lanes.collection_id,
      lanes.document_id,
      lanes.document_version_id,
      lanes.index_generation_id,
      lanes.materialization_id,
      lanes.parent_chunk_id,
      lanes.child_chunk_id,
      lanes.source_span_hash,
      lanes.content_hash
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
    fused.rank_score
  FROM fused
  ORDER BY fused.rank_score DESC, fused.child_chunk_id
  LIMIT p_limit;
END
$function$;

ALTER FUNCTION knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO go_api_runtime;
