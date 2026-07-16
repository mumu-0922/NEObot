-- G7.5.11 passage-embedding projection gateway functions.
--
-- These functions let the Python passage_embedding handler fetch child text
-- candidates and stage Jina 1024 passage embeddings through token-fenced
-- SECURITY DEFINER calls. Production dispatch remains default-off.

CREATE FUNCTION knowledge_fetch_passage_embedding_candidates(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID
) RETURNS TABLE(
  child_chunk_id UUID,
  content TEXT,
  content_hash TEXT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  job knowledge_processing_jobs%ROWTYPE;
BEGIN
  IF p_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PASSAGE_CANDIDATE_ARGUMENT_INVALID';
  END IF;

  SELECT processing_job.* INTO job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'passage_embedding'
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

  RETURN QUERY
  SELECT
    search.child_chunk_id,
    search.lexical_text,
    search.content_hash
  FROM knowledge_child_chunks child
  JOIN knowledge_child_search_projections search
    ON search.child_chunk_id = child.id
   AND search.materialization_id = child.materialization_id
   AND search.index_generation_id = child.index_generation_id
   AND search.document_id = child.document_id
   AND search.document_version_id = child.document_version_id
   AND search.source_span_hash = child.source_span_hash
   AND search.chunk_profile_hash = child.chunk_profile_hash
   AND search.content_hash = child.content_hash
  WHERE child.materialization_id = p_materialization_id
    AND child.index_generation_id = job.index_generation_id
    AND child.document_id = job.document_id
    AND child.document_version_id = job.document_version_id
    AND search.status IN ('staging', 'ready')
  ORDER BY child.ordinal, child.id;
END
$function$;

CREATE FUNCTION knowledge_stage_passage_embedding(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID,
  p_child_chunk_id UUID,
  p_embedding_vector REAL[],
  p_embedding_vector_sha256 TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  job knowledge_processing_jobs%ROWTYPE;
BEGIN
  IF p_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
    OR p_child_chunk_id IS NULL
    OR p_embedding_vector IS NULL
    OR cardinality(p_embedding_vector) <> 1024
    OR array_position(p_embedding_vector, NULL) IS NOT NULL
    OR p_embedding_vector_sha256 IS NULL
    OR p_embedding_vector_sha256 !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PASSAGE_EMBEDDING_ARGUMENT_INVALID';
  END IF;

  SELECT processing_job.* INTO job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'passage_embedding'
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

  UPDATE knowledge_child_search_projections search
  SET embedding_model_id = 'jina-embeddings-v4',
      embedding_dimensions = 1024,
      embedding_vector = p_embedding_vector,
      embedding_vector_sha256 = p_embedding_vector_sha256,
      status = 'ready',
      ready_at = COALESCE(search.ready_at, clock_timestamp())
  FROM knowledge_child_chunks child
  WHERE search.child_chunk_id = p_child_chunk_id
    AND search.child_chunk_id = child.id
    AND search.materialization_id = child.materialization_id
    AND search.index_generation_id = child.index_generation_id
    AND search.document_id = child.document_id
    AND search.document_version_id = child.document_version_id
    AND search.source_span_hash = child.source_span_hash
    AND search.chunk_profile_hash = child.chunk_profile_hash
    AND search.content_hash = child.content_hash
    AND search.materialization_id = p_materialization_id
    AND search.index_generation_id = job.index_generation_id
    AND search.document_id = job.document_id
    AND search.document_version_id = job.document_version_id
    AND search.status IN ('staging', 'ready');
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PASSAGE_EMBEDDING_TARGET_MISSING';
  END IF;

  RETURN true;
END
$function$;

ALTER FUNCTION knowledge_fetch_passage_embedding_candidates(
  UUID, UUID, UUID, UUID
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_stage_passage_embedding(
  UUID, UUID, UUID, UUID, UUID, REAL[], TEXT
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_fetch_passage_embedding_candidates(
  UUID, UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_stage_passage_embedding(
  UUID, UUID, UUID, UUID, UUID, REAL[], TEXT
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_fetch_passage_embedding_candidates(
  UUID, UUID, UUID, UUID
) TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_stage_passage_embedding(
  UUID, UUID, UUID, UUID, UUID, REAL[], TEXT
) TO rag_worker_executor;
