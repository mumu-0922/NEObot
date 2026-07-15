-- G7.5.9 purge projection gateway functions.
--
-- These functions are the first real Postgres projection adapter surface behind
-- the Python purge handler seam. They remain default-off until a later slice
-- adds readiness/registry promotion.

CREATE FUNCTION knowledge_mark_purge_invisible(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_collection_id UUID,
  p_document_id UUID,
  p_document_version_id UUID,
  p_collection_visibility_epoch BIGINT,
  p_document_visibility_epoch BIGINT
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  collection_visibility_epoch BIGINT,
  document_visibility_epoch BIGINT,
  query_visible BOOLEAN
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
    OR p_collection_id IS NULL
    OR p_document_id IS NULL
    OR p_document_version_id IS NULL
    OR p_collection_visibility_epoch IS NULL
    OR p_collection_visibility_epoch < 1
    OR p_document_visibility_epoch IS NULL
    OR p_document_visibility_epoch < 1
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PURGE_INVISIBILITY_ARGUMENT_INVALID';
  END IF;

  SELECT * INTO job
  FROM knowledge_processing_jobs
  WHERE id = p_job_id
    AND status = 'processing'
    AND stage = 'purge'
    AND operation = 'purge'
    AND NOT legacy_projection_unbound
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp()
    AND collection_id = p_collection_id
    AND document_id = p_document_id
    AND document_version_id = p_document_version_id
    AND collection_visibility_epoch = p_collection_visibility_epoch
    AND document_visibility_epoch = p_document_visibility_epoch
    AND index_generation_id IS NOT NULL
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  RETURN QUERY
  SELECT
    p_collection_id,
    p_document_id,
    p_document_version_id,
    p_collection_visibility_epoch,
    p_document_visibility_epoch,
    EXISTS (
      SELECT 1
      FROM knowledge_collections collection
      JOIN knowledge_documents document
        ON document.collection_id = collection.id
       AND document.id = p_document_id
      JOIN knowledge_document_versions version
        ON version.document_id = document.id
       AND version.id = p_document_version_id
      JOIN knowledge_document_projection_heads head
        ON head.index_generation_id = job.index_generation_id
       AND head.document_id = document.id
       AND head.active_materialization_id IS NOT NULL
      WHERE collection.id = p_collection_id
        AND collection.deleted_at IS NULL
        AND collection.visibility_epoch = p_collection_visibility_epoch
        AND document.deleted_at IS NULL
        AND document.status = 'active'
        AND document.current_version_id = p_document_version_id
        AND document.visibility_epoch = p_document_visibility_epoch
        AND version.status = 'active'
    );
END
$function$;

CREATE FUNCTION knowledge_purge_search_projection(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_collection_id UUID,
  p_document_id UUID,
  p_document_version_id UUID,
  p_index_generation_id UUID,
  p_materialization_id UUID
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  purged_child_search_rows INTEGER,
  remaining_ready_child_search_rows INTEGER
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  purged_count INTEGER;
BEGIN
  IF p_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_collection_id IS NULL
    OR p_document_id IS NULL
    OR p_document_version_id IS NULL
    OR p_index_generation_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PURGE_PROJECTION_ARGUMENT_INVALID';
  END IF;

  PERFORM 1
  FROM knowledge_processing_jobs job
  WHERE job.id = p_job_id
    AND job.status = 'processing'
    AND job.stage = 'purge'
    AND job.operation = 'purge'
    AND NOT job.legacy_projection_unbound
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > clock_timestamp()
    AND job.collection_id = p_collection_id
    AND job.document_id = p_document_id
    AND job.document_version_id = p_document_version_id
    AND job.index_generation_id = p_index_generation_id
    AND (
      job.materialization_id IS NOT DISTINCT FROM p_materialization_id
      OR job.materialization_id IS NULL
    )
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  UPDATE knowledge_child_search_projections search
  SET status = 'purged',
      purged_at = COALESCE(search.purged_at, clock_timestamp())
  WHERE search.collection_id = p_collection_id
    AND search.document_id = p_document_id
    AND search.document_version_id = p_document_version_id
    AND search.index_generation_id = p_index_generation_id
    AND (
      p_materialization_id IS NULL
      OR search.materialization_id = p_materialization_id
    )
    AND search.status <> 'purged';
  GET DIAGNOSTICS purged_count = ROW_COUNT;

  RETURN QUERY
  SELECT
    p_collection_id,
    p_document_id,
    p_document_version_id,
    p_index_generation_id,
    p_materialization_id,
    purged_count,
    count(*)::INTEGER
  FROM knowledge_child_search_projections search
  WHERE search.collection_id = p_collection_id
    AND search.document_id = p_document_id
    AND search.document_version_id = p_document_version_id
    AND search.index_generation_id = p_index_generation_id
    AND (
      p_materialization_id IS NULL
      OR search.materialization_id = p_materialization_id
    )
    AND search.status = 'ready';
END
$function$;

CREATE FUNCTION knowledge_assert_purge_complete(
  p_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_collection_id UUID,
  p_document_id UUID,
  p_document_version_id UUID,
  p_index_generation_id UUID,
  p_materialization_id UUID,
  p_expected_purged_child_search_rows INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  remaining_ready_count INTEGER;
BEGIN
  IF p_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_collection_id IS NULL
    OR p_document_id IS NULL
    OR p_document_version_id IS NULL
    OR p_index_generation_id IS NULL
    OR p_expected_purged_child_search_rows IS NULL
    OR p_expected_purged_child_search_rows < 0
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PURGE_COMPLETENESS_ARGUMENT_INVALID';
  END IF;

  PERFORM 1
  FROM knowledge_processing_jobs job
  WHERE job.id = p_job_id
    AND job.status = 'processing'
    AND job.stage = 'purge'
    AND job.operation = 'purge'
    AND NOT job.legacy_projection_unbound
    AND job.lease_owner = p_worker_id
    AND job.lease_token = p_lease_token
    AND job.lease_expires_at > clock_timestamp()
    AND job.collection_id = p_collection_id
    AND job.document_id = p_document_id
    AND job.document_version_id = p_document_version_id
    AND job.index_generation_id = p_index_generation_id
    AND (
      job.materialization_id IS NOT DISTINCT FROM p_materialization_id
      OR job.materialization_id IS NULL
    )
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  SELECT count(*)::INTEGER INTO remaining_ready_count
  FROM knowledge_child_search_projections search
  WHERE search.collection_id = p_collection_id
    AND search.document_id = p_document_id
    AND search.document_version_id = p_document_version_id
    AND search.index_generation_id = p_index_generation_id
    AND (
      p_materialization_id IS NULL
      OR search.materialization_id = p_materialization_id
    )
    AND search.status = 'ready';

  IF remaining_ready_count <> 0 THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PURGE_PROJECTION_INCOMPLETE';
  END IF;

  RETURN true;
END
$function$;

ALTER FUNCTION knowledge_mark_purge_invisible(
  UUID, UUID, UUID, UUID, UUID, UUID, BIGINT, BIGINT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_purge_search_projection(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_assert_purge_complete(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID, INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_mark_purge_invisible(
  UUID, UUID, UUID, UUID, UUID, UUID, BIGINT, BIGINT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_purge_search_projection(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_assert_purge_complete(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID, INTEGER
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_mark_purge_invisible(
  UUID, UUID, UUID, UUID, UUID, UUID, BIGINT, BIGINT
) TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_purge_search_projection(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID
) TO rag_worker_executor;
GRANT EXECUTE ON FUNCTION knowledge_assert_purge_complete(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID, INTEGER
) TO rag_worker_executor;
