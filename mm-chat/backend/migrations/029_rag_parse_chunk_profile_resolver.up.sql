-- G11.9D.2.3b: resolve the leased parse job's generation chunk profile
-- before any parser-provider request is made.

CREATE FUNCTION knowledge_resolve_parse_chunk_profile(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_index_generation_id UUID,
  p_materialization_id UUID
) RETURNS TABLE(chunk_profile_hash TEXT)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_index_generation_id IS NULL
    OR p_materialization_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PARSE_CHUNK_PROFILE_ARGUMENT_INVALID';
  END IF;

  RETURN QUERY
  SELECT profile.chunk_profile_hash
  FROM knowledge_processing_jobs processing_job
  JOIN knowledge_document_materializations materialization
    ON materialization.id=p_materialization_id
   AND materialization.index_generation_id=p_index_generation_id
   AND materialization.collection_id=processing_job.collection_id
   AND materialization.document_id=processing_job.document_id
   AND materialization.document_version_id=processing_job.document_version_id
   AND materialization.file_id=processing_job.file_id
   AND materialization.status='staging'
  JOIN knowledge_index_generations generation
    ON generation.id=p_index_generation_id
   AND generation.id=processing_job.index_generation_id
   AND generation.status IN ('building','verified','active')
  JOIN knowledge_index_profiles profile
    ON profile.id=generation.index_profile_id
  WHERE processing_job.id=p_job_id
    AND processing_job.status='processing'
    AND processing_job.stage='parse'
    AND processing_job.operation IN ('initial','replace','reprocess')
    AND NOT processing_job.legacy_projection_unbound
    AND processing_job.lease_owner=p_worker_id
    AND processing_job.lease_token=p_lease_token
    AND processing_job.lease_expires_at > clock_timestamp()
    AND processing_job.materialization_id=p_materialization_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_CHUNK_PROFILE_MISSING';
  END IF;
END
$function$;

ALTER FUNCTION knowledge_resolve_parse_chunk_profile(
  UUID,UUID,UUID,UUID,UUID
) OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_resolve_parse_chunk_profile(
  UUID,UUID,UUID,UUID,UUID
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_resolve_parse_chunk_profile(
  UUID,UUID,UUID,UUID,UUID
) TO rag_worker_executor;
