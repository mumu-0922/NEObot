-- Read-only, generation-bound retrieval for the operator promotion evaluator.
--
-- Production chat remains on the current-authority reader. These functions let
-- the evaluator compare one explicit Active/verified Candidate pair without
-- moving the corpus head or granting raw projection-table access.

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
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_build_sources candidate
      WHERE candidate.child_chunk_id = shadow.child_chunk_id
        AND candidate.index_generation_id = p_index_generation_id
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

CREATE FUNCTION knowledge_hydrate_generation_evaluation_evidence(
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
     AND search_profile.provider_profile_id = 'mineru_jina_postgres_v1'
     AND search_profile.embedding_processor = 'jina'
     AND search_profile.embedding_model_id = 'jina-embeddings-v4'
     AND search_profile.embedding_dimensions = 1024
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

ALTER FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) TO rag_replay_operator;
