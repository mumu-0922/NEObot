-- G7.5.19 parse projection gateway function.
--
-- This function lets the Python parse handler stage Canonical IR / chunk
-- manifest projection rows through one token-fenced SECURITY DEFINER call.
-- Production dispatch remains default-off until later handler promotion gates.

CREATE FUNCTION knowledge_stage_parse_projection(
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
  search_profile_id UUID;
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
    AND embedding_model_id = 'jina-embeddings-v4'
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_PROJECTION_PROFILE_MISMATCH';
  END IF;

  SELECT profile.id INTO search_profile_id
  FROM knowledge_search_profiles profile
  WHERE profile.index_profile_id = index_profile.id
    AND profile.provider_profile_id = 'mineru_jina_postgres_v1'
    AND profile.embedding_model_id = 'jina-embeddings-v4'
    AND profile.embedding_dimensions = 1024
  FOR SHARE;
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
    search.document_version_id, search_profile_id, search.embedding_model_id,
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
    AND search.embedding_model_id = 'jina-embeddings-v4'
    AND search.embedding_dimensions = 1024
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

ALTER FUNCTION knowledge_stage_parse_projection(
  UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, JSONB
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_stage_parse_projection(
  UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, JSONB
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_stage_parse_projection(
  UUID, UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, JSONB
) TO rag_worker_executor;

GRANT SELECT ON knowledge_index_profiles TO rag_projection_owner;
GRANT SELECT, INSERT ON
  knowledge_parser_artifact_sets,
  knowledge_blocks,
  knowledge_parent_chunks,
  knowledge_child_chunks,
  knowledge_chunk_block_spans
TO rag_projection_owner;
