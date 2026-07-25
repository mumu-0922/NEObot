-- Add current-authority source-name routing without changing immutable Chunk
-- source text. The filename is metadata-only: it may select and rerank a
-- document, but never becomes Citation or quoted source authority.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_source_name_key(p_value TEXT)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
STRICT
PARALLEL SAFE
SET search_path FROM CURRENT
AS $function$
  SELECT lower(regexp_replace(btrim(p_value), '[^[:alnum:]]+', '', 'g'))
$function$;

CREATE FUNCTION knowledge_fetch_source_name_evidence_candidates(
  p_index_generation_id UUID,
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
  source_rank INTEGER
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  normalized_query TEXT;
  normalized_query_key TEXT;
  query_terms TEXT[];
BEGIN
  normalized_query := btrim(p_query_text);
  normalized_query_key := knowledge_source_name_key(normalized_query);
  IF p_index_generation_id IS NULL
    OR p_collection_ids IS NULL
    OR cardinality(p_collection_ids) NOT BETWEEN 1 AND 32
    OR array_position(p_collection_ids, NULL) IS NOT NULL
    OR normalized_query IS NULL
    OR octet_length(normalized_query) NOT BETWEEN 1 AND 2048
    OR length(normalized_query_key) < 6
    OR p_limit IS NULL
    OR p_limit NOT BETWEEN 1 AND 50
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_SOURCE_NAME_QUERY_ARGUMENT_INVALID';
  END IF;

  query_terms := knowledge_bm25_shadow_query_terms(normalized_query);

  RETURN QUERY
  WITH matched_documents AS MATERIALIZED (
    SELECT
      materialization.id AS materialization_id,
      materialization.index_generation_id,
      materialization.collection_id,
      materialization.document_id,
      materialization.document_version_id
    FROM knowledge_document_materializations materialization
    JOIN files file_record
      ON file_record.id = materialization.file_id
     AND file_record.upload_status = 'available'
     AND file_record.deleted_at IS NULL
    WHERE materialization.index_generation_id = p_index_generation_id
      AND materialization.collection_id = ANY(p_collection_ids)
      AND materialization.status = 'published'
      AND octet_length(file_record.original_filename) BETWEEN 1 AND 512
      AND length(knowledge_source_name_key(regexp_replace(
        file_record.original_filename,
        '\.[^.]{1,16}$',
        ''
      ))) BETWEEN 6 AND 256
      AND position(knowledge_source_name_key(regexp_replace(
        file_record.original_filename,
        '\.[^.]{1,16}$',
        ''
      )) IN normalized_query_key) > 0
  ), matched AS MATERIALIZED (
    SELECT source.*
    FROM matched_documents document
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_build_sources candidate
      WHERE candidate.materialization_id = document.materialization_id
        AND candidate.index_generation_id = document.index_generation_id
        AND candidate.collection_id = document.collection_id
        AND candidate.document_id = document.document_id
        AND candidate.document_version_id = document.document_version_id
      OFFSET 0
    ) source
  ), scored AS (
    SELECT
      matched.*,
      position(lower(normalized_query) IN lower(matched.lexical_text)) > 0
        AS phrase_match,
      (
        SELECT count(*)::INTEGER
        FROM unnest(matched.exact_terms) term
        WHERE term = ANY(query_terms)
      ) AS exact_overlap
    FROM matched
  ), ranked AS (
    SELECT
      scored.*,
      row_number() OVER (
        ORDER BY
          scored.phrase_match DESC,
          scored.exact_overlap DESC,
          scored.child_ordinal,
          scored.child_chunk_id
      )::INTEGER AS lane_rank
    FROM scored
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
    ranked.lane_rank
  FROM ranked
  ORDER BY ranked.lane_rank
  LIMIT p_limit;
END
$function$;

ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) RENAME TO knowledge_fetch_profiled_query_evidence_candidates_v47_base;

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
  selected_profile TEXT;
  active_generation_id UUID;
  source_name_weight CONSTANT DOUBLE PRECISION := 2.0;
  rrf_constant CONSTANT DOUBLE PRECISION := 60.0;
BEGIN
  SELECT profile.active_profile INTO selected_profile
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1;

  IF selected_profile IS DISTINCT FROM 'pg17_bm25_pgvector_v1' THEN
    RETURN QUERY
    SELECT candidate.*
    FROM knowledge_fetch_profiled_query_evidence_candidates_v47_base(
      p_collection_ids,
      p_query_text,
      p_query_embedding,
      p_limit
    ) candidate;
    RETURN;
  END IF;

  SELECT head.active_index_generation_id INTO active_generation_id
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id = 1;
  IF active_generation_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING';
  END IF;

  RETURN QUERY
  WITH base AS MATERIALIZED (
    SELECT
      candidate.*,
      row_number() OVER (
        ORDER BY candidate.rank_score DESC, candidate.child_chunk_id
      )::INTEGER AS base_rank
    FROM knowledge_fetch_profiled_query_evidence_candidates_v47_base(
      p_collection_ids,
      p_query_text,
      p_query_embedding,
      p_limit
    ) candidate
  ), source_name AS MATERIALIZED (
    SELECT candidate.*
    FROM knowledge_fetch_source_name_evidence_candidates(
      active_generation_id,
      p_collection_ids,
      p_query_text,
      p_limit
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
          ELSE source_name_weight /
            (rrf_constant + source_name.source_rank) END AS fused_score
    FROM base
    FULL JOIN source_name
      ON source_name.child_chunk_id = base.child_chunk_id
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
  FROM fused
  ORDER BY
    fused.fused_score DESC,
    least(
      COALESCE(fused.source_rank, 2147483647),
      COALESCE(fused.base_rank, 2147483647)
    ),
    fused.child_chunk_id
  LIMIT p_limit;
END
$function$;

ALTER FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) RENAME TO knowledge_fetch_generation_evaluation_candidates_v47_base;

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
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  WITH base AS MATERIALIZED (
    SELECT
      candidate.*,
      row_number() OVER (
        ORDER BY candidate.rank_score DESC, candidate.child_chunk_id
      )::INTEGER AS base_rank
    FROM knowledge_fetch_generation_evaluation_candidates_v47_base(
      p_index_generation_id,
      p_collection_ids,
      p_query_text,
      p_query_embedding,
      p_limit
    ) candidate
  ), source_name AS MATERIALIZED (
    SELECT candidate.*
    FROM knowledge_fetch_source_name_evidence_candidates(
      p_index_generation_id,
      p_collection_ids,
      p_query_text,
      p_limit
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
  FROM fused
  ORDER BY
    fused.fused_score DESC,
    least(
      COALESCE(fused.source_rank, 2147483647),
      COALESCE(fused.base_rank, 2147483647)
    ),
    fused.child_chunk_id
  LIMIT p_limit
$function$;

ALTER FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) RENAME TO knowledge_reauthorize_and_hydrate_evidence_v47_base;

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
  content_hash TEXT,
  source_name TEXT,
  source_text TEXT,
  child_token_count INTEGER,
  parent_source_text TEXT,
  parent_token_count INTEGER,
  locator JSONB
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT
    evidence.collection_id,
    evidence.document_id,
    evidence.document_version_id,
    evidence.index_generation_id,
    evidence.materialization_id,
    evidence.parent_chunk_id,
    evidence.child_chunk_id,
    evidence.source_span_hash,
    evidence.content_hash,
    file_record.original_filename,
    evidence.source_text,
    evidence.child_token_count,
    evidence.parent_source_text,
    evidence.parent_token_count,
    evidence.locator
  FROM knowledge_reauthorize_and_hydrate_evidence_v47_base(
    p_actor_user_id,
    p_session_id,
    p_conversation_id,
    p_references
  ) evidence
  JOIN knowledge_document_materializations materialization
    ON materialization.id = evidence.materialization_id
   AND materialization.index_generation_id = evidence.index_generation_id
   AND materialization.collection_id = evidence.collection_id
   AND materialization.document_id = evidence.document_id
   AND materialization.document_version_id = evidence.document_version_id
  JOIN files file_record
    ON file_record.id = materialization.file_id
   AND file_record.upload_status = 'available'
   AND file_record.deleted_at IS NULL
  WHERE octet_length(file_record.original_filename) BETWEEN 1 AND 512
$function$;

ALTER FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) RENAME TO knowledge_hydrate_generation_evaluation_evidence_v47_base;

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
  source_name TEXT,
  source_text TEXT,
  child_token_count INTEGER,
  parent_source_text TEXT,
  parent_token_count INTEGER,
  locator JSONB,
  provenance_valid BOOLEAN,
  cell_lineage_valid BOOLEAN
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT
    evidence.collection_id,
    evidence.document_id,
    evidence.document_version_id,
    evidence.index_generation_id,
    evidence.materialization_id,
    evidence.parent_chunk_id,
    evidence.child_chunk_id,
    evidence.source_span_hash,
    evidence.content_hash,
    file_record.original_filename,
    evidence.source_text,
    evidence.child_token_count,
    evidence.parent_source_text,
    evidence.parent_token_count,
    evidence.locator,
    evidence.provenance_valid,
    evidence.cell_lineage_valid
  FROM knowledge_hydrate_generation_evaluation_evidence_v47_base(
    p_index_generation_id,
    p_collection_ids,
    p_references
  ) evidence
  JOIN knowledge_document_materializations materialization
    ON materialization.id = evidence.materialization_id
   AND materialization.index_generation_id = evidence.index_generation_id
   AND materialization.collection_id = evidence.collection_id
   AND materialization.document_id = evidence.document_id
   AND materialization.document_version_id = evidence.document_version_id
  JOIN files file_record
    ON file_record.id = materialization.file_id
   AND file_record.upload_status = 'available'
   AND file_record.deleted_at IS NULL
  WHERE octet_length(file_record.original_filename) BETWEEN 1 AND 512
$function$;

ALTER FUNCTION knowledge_source_name_key(TEXT) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_source_name_evidence_candidates(
  UUID, UUID[], TEXT, INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_source_name_key(TEXT) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_source_name_evidence_candidates(
  UUID, UUID[], TEXT, INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates_v47_base(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates_v47_base(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, go_api_runtime, rag_worker_executor, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_reauthorize_and_hydrate_evidence_v47_base(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC, go_api_runtime, go_evidence_hydrator;
REVOKE ALL ON FUNCTION knowledge_hydrate_generation_evaluation_evidence_v47_base(
  UUID, UUID[], JSONB
) FROM PUBLIC, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) TO go_evidence_hydrator, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) TO rag_replay_operator;
