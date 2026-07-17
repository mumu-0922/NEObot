-- G11.8 rollback: restore the G7.6A lexical/exact-term candidate function.
--
-- Python query code may rank only references from collections explicitly selected
-- by Go for the current chat. This function does not grant access by itself and
-- intentionally returns citation references, not document body text. Go must
-- reauthorize/hydrate returned references before answer generation.

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
  normalized_query TEXT;
  query_terms TEXT[];
BEGIN
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
      )::REAL AS rank_score,
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
    WHERE search.status = 'ready'
      AND search.embedding_model_id = 'jina-embeddings-v4'
      AND search.embedding_dimensions = 1024
      AND search.embedding_vector IS NOT NULL
      AND search.embedding_vector_sha256 IS NOT NULL
      AND (
        search.lexical_tsv @@ plainto_tsquery('simple', normalized_query)
        OR (query_terms IS NOT NULL AND search.exact_terms && query_terms)
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

ALTER FUNCTION knowledge_fetch_query_evidence_candidates(UUID[], TEXT, INTEGER)
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) TO go_api_runtime;
