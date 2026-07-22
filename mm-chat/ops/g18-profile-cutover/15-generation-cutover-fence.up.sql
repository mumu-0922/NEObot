\set ON_ERROR_STOP on

DO $generation_fence_prerequisite$
BEGIN
  IF to_regclass('knowledge_bm25_shadow_build_sources') IS NULL
    OR to_regclass('knowledge_pgvector_shadow_sources') IS NULL
    OR to_regprocedure(
      'knowledge_sync_pg17_retrieval_materialization(uuid)'
    ) IS NULL
  THEN
    RAISE EXCEPTION
      'G18 generation fence requires PG17 projection maintenance';
  END IF;
END
$generation_fence_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_assert_pg17_generation_ready(
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
   AND profile.provider_profile_id = 'mineru_jina_postgres_v1'
   AND profile.embedding_processor = 'jina'
   AND profile.embedding_model_id = 'jina-embeddings-v4'
   AND profile.embedding_dimensions = 1024
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

CREATE FUNCTION knowledge_fence_pg17_generation_cutover()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  selected_profile TEXT;
BEGIN
  IF OLD.active_index_generation_id IS NOT DISTINCT FROM
      NEW.active_index_generation_id
  THEN
    RETURN NEW;
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 3);
  PERFORM pg_advisory_xact_lock(1296912978, 4);

  SELECT profile.active_profile INTO selected_profile
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;
  IF selected_profile = 'legacy' THEN
    RETURN NEW;
  END IF;
  IF selected_profile <> 'pg17_bm25_pgvector_v1' THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
  END IF;

  PERFORM * FROM knowledge_assert_pg17_generation_ready(
    NEW.active_index_generation_id
  );
  RETURN NEW;
END
$function$;

CREATE TRIGGER knowledge_corpus_head_pg17_retrieval_fence
BEFORE UPDATE OF active_index_generation_id
ON knowledge_corpus_projection_head
FOR EACH ROW EXECUTE FUNCTION knowledge_fence_pg17_generation_cutover();

ALTER FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fence_pg17_generation_cutover()
  OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fence_pg17_generation_cutover()
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  TO rag_replay_operator;

SELECT 'PASS G18.5B.2b generation cutover fence' AS result;
