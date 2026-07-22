\set ON_ERROR_STOP on

DO $maintenance_prerequisite$
BEGIN
  IF to_regprocedure(
    'knowledge_assert_pg17_retrieval_profile_ready()'
  ) IS NULL OR to_regclass(
    'knowledge_child_vector_shadow_projections'
  ) IS NULL OR to_regclass(
    'knowledge_child_bm25_shadow_projections'
  ) IS NULL OR to_regclass(
    'knowledge_bm25_shadow_build_sources'
  ) IS NULL THEN
    RAISE EXCEPTION 'G18 active maintenance requires the PG17 profile candidate';
  END IF;
END
$maintenance_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_sync_pg17_retrieval_materialization(
  p_materialization_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  vector_inserted_count BIGINT,
  bm25_inserted_count BIGINT,
  verified_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  expected_count BIGINT;
  vector_source_count BIGINT;
  inserted_vector_count BIGINT;
  inserted_bm25_count BIGINT;
  verified_vector_count BIGINT;
  verified_bm25_count BIGINT;
BEGIN
  IF p_materialization_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_RETRIEVAL_MATERIALIZATION_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 3);
  PERFORM pg_advisory_xact_lock(1296912978, 4);

  SELECT count(*) INTO expected_count
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.materialization_id = p_materialization_id;
  SELECT count(*) INTO vector_source_count
  FROM knowledge_pgvector_shadow_sources source
  WHERE source.materialization_id = p_materialization_id;
  IF expected_count < 1 OR vector_source_count <> expected_count THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_MATERIALIZATION_SOURCE_INCOMPLETE';
  END IF;

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
  WHERE source.materialization_id = p_materialization_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS inserted_vector_count = ROW_COUNT;

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
  WHERE source.materialization_id = p_materialization_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS inserted_bm25_count = ROW_COUNT;

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
  WHERE source.materialization_id = p_materialization_id;

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
  WHERE source.materialization_id = p_materialization_id;

  IF verified_vector_count <> expected_count
    OR verified_bm25_count <> expected_count
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_MATERIALIZATION_SYNC_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    expected_count,
    inserted_vector_count,
    inserted_bm25_count,
    least(verified_vector_count, verified_bm25_count);
END
$function$;

CREATE FUNCTION knowledge_maintain_pg17_retrieval_on_head()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  selected_profile TEXT;
BEGIN
  IF TG_OP = 'UPDATE'
    AND OLD.active_materialization_id = NEW.active_materialization_id
  THEN
    RETURN NEW;
  END IF;

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

  PERFORM * FROM knowledge_sync_pg17_retrieval_materialization(
    NEW.active_materialization_id
  );
  RETURN NEW;
END
$function$;

CREATE TRIGGER knowledge_document_projection_head_pg17_retrieval
AFTER INSERT OR UPDATE OF active_materialization_id
ON knowledge_document_projection_heads
FOR EACH ROW EXECUTE FUNCTION knowledge_maintain_pg17_retrieval_on_head();

ALTER FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_maintain_pg17_retrieval_on_head()
  OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_maintain_pg17_retrieval_on_head()
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  TO rag_replay_operator;

SELECT 'PASS G18.5B.2a active projection maintenance' AS result;
