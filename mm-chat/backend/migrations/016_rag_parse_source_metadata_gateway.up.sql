-- G7.5.15 parse source metadata gateway function.
--
-- This function lets the Python parse handler resolve source file metadata
-- through a token-fenced SECURITY DEFINER call. It returns only object-storage
-- metadata needed by the default-off source gateway; production dispatch remains
-- gated until later slices add object-byte and parser/projection promotion.

CREATE FUNCTION knowledge_fetch_parse_source_metadata(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_file_id UUID,
  p_materialization_id UUID
) RETURNS TABLE(
  file_id UUID,
  storage_backend TEXT,
  object_key TEXT,
  sha256 TEXT,
  byte_size BIGINT,
  content_type TEXT
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
    OR p_file_id IS NULL
    OR p_materialization_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PARSE_SOURCE_ARGUMENT_INVALID';
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
    AND processing_job.file_id = p_file_id
    AND processing_job.materialization_id = p_materialization_id
    AND processing_job.index_generation_id IS NOT NULL
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  RETURN QUERY
  SELECT
    file_record.id,
    file_record.storage_backend,
    file_record.object_key,
    file_record.sha256,
    file_record.byte_size,
    file_record.mime_type AS content_type
  FROM files file_record
  JOIN knowledge_collections collection
    ON collection.id = job.collection_id
  JOIN knowledge_documents document
    ON document.collection_id = collection.id
   AND document.id = job.document_id
  JOIN knowledge_document_versions version
    ON version.document_id = document.id
   AND version.id = job.document_version_id
   AND version.file_id = file_record.id
  JOIN knowledge_document_materializations materialization
    ON materialization.id = p_materialization_id
   AND materialization.index_generation_id = job.index_generation_id
   AND materialization.collection_id = job.collection_id
   AND materialization.document_id = job.document_id
   AND materialization.document_version_id = job.document_version_id
   AND materialization.file_id = file_record.id
  WHERE file_record.id = p_file_id
    AND file_record.upload_status = 'available'
    AND file_record.deleted_at IS NULL
    AND file_record.byte_size > 0
    AND file_record.sha256 = version.content_hash
    AND materialization.source_content_hash = file_record.sha256
    AND materialization.status = 'staging'
    AND materialization.collection_acl_revision = job.collection_acl_revision
    AND materialization.collection_visibility_epoch = job.collection_visibility_epoch
    AND materialization.collection_processing_revision = job.collection_processing_revision
    AND materialization.document_visibility_epoch = job.document_visibility_epoch
    AND collection.deleted_at IS NULL
    AND collection.acl_revision = job.collection_acl_revision
    AND collection.visibility_epoch = job.collection_visibility_epoch
    AND collection.collection_processing_revision = job.collection_processing_revision
    AND document.deleted_at IS NULL
    AND document.status IN ('processing', 'active')
    AND document.visibility_epoch = job.document_visibility_epoch
    AND version.status IN ('uploaded', 'processing', 'active');
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_SOURCE_METADATA_MISSING';
  END IF;
END
$function$;

ALTER FUNCTION knowledge_fetch_parse_source_metadata(
  UUID, UUID, UUID, UUID, UUID
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_fetch_parse_source_metadata(
  UUID, UUID, UUID, UUID, UUID
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_fetch_parse_source_metadata(
  UUID, UUID, UUID, UUID, UUID
) TO rag_worker_executor;
