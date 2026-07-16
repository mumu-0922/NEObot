-- G7.5P passage-embedding finalizer.
--
-- After Python has staged all Jina passage embeddings, this token-fenced
-- finalizer atomically verifies the searchable projection, publishes the
-- materialization, activates the document version, advances the projection head,
-- and commits the embedding job. The generic worker finish path must not run
-- after this function succeeds.

CREATE FUNCTION knowledge_complete_embedding_and_publish(
  p_embedding_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  embedding_job knowledge_processing_jobs%ROWTYPE;
  materialization knowledge_document_materializations%ROWTYPE;
  previous_current_version_id UUID;
  child_count BIGINT;
  corpus_revision BIGINT;
  computed_manifest_hash TEXT;
  computed_result_hash TEXT;
BEGIN
  IF p_embedding_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_ARGUMENT_INVALID';
  END IF;

  SELECT processing_job.* INTO embedding_job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_embedding_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'passage_embedding'
    AND processing_job.operation IN ('initial', 'replace', 'reprocess')
    AND processing_job.processor = 'jina'
    AND processing_job.model_id = 'jina-embeddings-v4'
    AND NOT processing_job.legacy_projection_unbound
    AND processing_job.lease_owner = p_worker_id
    AND processing_job.lease_token = p_lease_token
    AND processing_job.lease_expires_at > clock_timestamp()
    AND processing_job.materialization_id = p_materialization_id
    AND processing_job.index_generation_id IS NOT NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  SELECT * INTO materialization
  FROM knowledge_document_materializations
  WHERE id = p_materialization_id
    AND index_generation_id = embedding_job.index_generation_id
    AND collection_id = embedding_job.collection_id
    AND document_id = embedding_job.document_id
    AND document_version_id = embedding_job.document_version_id
    AND file_id = embedding_job.file_id
    AND collection_acl_revision = embedding_job.collection_acl_revision
    AND collection_visibility_epoch = embedding_job.collection_visibility_epoch
    AND collection_processing_revision = embedding_job.collection_processing_revision
    AND document_visibility_epoch = embedding_job.document_visibility_epoch
    AND status = 'staging'
    AND parse_artifact_set_id IS NOT NULL
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_MATERIALIZATION_MISSING';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_collections collection
    JOIN knowledge_documents document
      ON document.collection_id = collection.id
     AND document.id = materialization.document_id
    JOIN knowledge_document_versions version
      ON version.document_id = document.id
     AND version.id = materialization.document_version_id
     AND version.file_id = materialization.file_id
    WHERE collection.id = materialization.collection_id
      AND collection.deleted_at IS NULL
      AND document.deleted_at IS NULL
      AND document.status IN ('processing', 'active')
      AND version.status IN ('uploaded', 'processing', 'active')
      AND collection.acl_revision = materialization.collection_acl_revision
      AND collection.visibility_epoch = materialization.collection_visibility_epoch
      AND collection.collection_processing_revision =
        materialization.collection_processing_revision
      AND document.visibility_epoch = materialization.document_visibility_epoch
      AND version.visibility_epoch = materialization.document_visibility_epoch
      AND version.content_hash = materialization.source_content_hash
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_AUTHORITY_STALE';
  END IF;

  SELECT count(*) INTO child_count
  FROM knowledge_child_chunks child
  WHERE child.materialization_id = materialization.id;
  IF child_count < 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_CHILDREN_MISSING';
  END IF;

  PERFORM knowledge_assert_materialization_search_complete(
    materialization.id,
    child_count,
    'jina-embeddings-v4',
    1024
  );

  SELECT
    encode(sha256(convert_to(
      'g7.5p:manifest:' || materialization.id::TEXT || ':' ||
      materialization.index_generation_id::TEXT || ':' ||
      materialization.document_id::TEXT || ':' ||
      materialization.document_version_id::TEXT || ':' ||
      materialization.parse_artifact_set_id::TEXT || ':' ||
      materialization.source_content_hash || ':' ||
      string_agg(
        child.id::TEXT || ':' || child.content_hash || ':' ||
        search.embedding_vector_sha256,
        ',' ORDER BY child.ordinal, child.id
      ),
      'UTF8'
    )), 'hex'),
    encode(sha256(convert_to(
      'g7.5p:result:' || materialization.id::TEXT || ':' ||
      materialization.base_profile_hash || ':' || child_count::TEXT || ':' ||
      string_agg(
        search.child_chunk_id::TEXT || ':' || search.source_span_hash || ':' ||
        search.content_hash || ':' || search.embedding_vector_sha256,
        ',' ORDER BY child.ordinal, child.id
      ),
      'UTF8'
    )), 'hex')
  INTO computed_manifest_hash, computed_result_hash
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
  WHERE child.materialization_id = materialization.id
    AND search.status = 'ready'
    AND search.embedding_model_id = 'jina-embeddings-v4'
    AND search.embedding_dimensions = 1024
    AND search.embedding_vector IS NOT NULL
    AND search.embedding_vector_sha256 IS NOT NULL;

  IF computed_manifest_hash IS NULL OR computed_result_hash IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_HASH_MISSING';
  END IF;

  SELECT document.current_version_id INTO previous_current_version_id
  FROM knowledge_documents document
  WHERE document.id = materialization.document_id
    AND document.collection_id = materialization.collection_id
  FOR UPDATE;

  UPDATE knowledge_document_versions version
  SET status = 'tombstoned',
      updated_at = clock_timestamp()
  WHERE version.document_id = materialization.document_id
    AND version.id = previous_current_version_id
    AND version.id <> materialization.document_version_id
    AND version.status = 'active';

  UPDATE knowledge_document_versions version
  SET status = 'active',
      error_code = NULL,
      updated_at = clock_timestamp()
  WHERE version.document_id = materialization.document_id
    AND version.id = materialization.document_version_id
    AND version.file_id = materialization.file_id
    AND version.status IN ('uploaded', 'processing', 'active')
    AND version.content_hash = materialization.source_content_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_VERSION_STALE';
  END IF;

  UPDATE knowledge_documents document
  SET status = 'active',
      current_version_id = materialization.document_version_id,
      updated_at = clock_timestamp()
  WHERE document.id = materialization.document_id
    AND document.collection_id = materialization.collection_id
    AND document.status IN ('processing', 'active')
    AND document.deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_DOCUMENT_STALE';
  END IF;

  SELECT corpus_projection_revision INTO corpus_revision
  FROM knowledge_corpus_projection_head
  WHERE singleton_id = 1
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_CORPUS_HEAD_MISSING';
  END IF;

  UPDATE knowledge_document_materializations
  SET status = 'published',
      manifest_hash = computed_manifest_hash,
      result_hash = computed_result_hash,
      verified_at = clock_timestamp(),
      published_at = clock_timestamp()
  WHERE id = materialization.id
    AND status = 'staging';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_EMBEDDING_COMPLETION_MATERIALIZATION_STALE';
  END IF;

  INSERT INTO knowledge_document_projection_heads(
    index_generation_id,
    document_id,
    active_materialization_id,
    document_projection_revision,
    last_corpus_projection_revision,
    updated_at
  ) VALUES (
    materialization.index_generation_id,
    materialization.document_id,
    materialization.id,
    1,
    corpus_revision + 1,
    clock_timestamp()
  ) ON CONFLICT (index_generation_id, document_id) DO UPDATE
    SET active_materialization_id = EXCLUDED.active_materialization_id,
        document_projection_revision =
          knowledge_document_projection_heads.document_projection_revision + 1,
        last_corpus_projection_revision = EXCLUDED.last_corpus_projection_revision,
        updated_at = clock_timestamp();

  UPDATE knowledge_corpus_projection_head
  SET corpus_projection_revision = corpus_projection_revision + 1,
      updated_at = clock_timestamp()
  WHERE singleton_id = 1;

  UPDATE knowledge_processing_jobs
  SET status = 'succeeded',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = clock_timestamp(),
      error_code = NULL,
      updated_at = clock_timestamp()
  WHERE id = embedding_job.id
    AND status = 'processing'
    AND lease_owner = p_worker_id
    AND lease_token = p_lease_token
    AND lease_expires_at > clock_timestamp();
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_STALE_JOB_LEASE';
  END IF;

  RETURN true;
END
$function$;

ALTER FUNCTION knowledge_complete_embedding_and_publish(UUID, UUID, UUID, UUID)
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_complete_embedding_and_publish(
  UUID, UUID, UUID, UUID
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_complete_embedding_and_publish(
  UUID, UUID, UUID, UUID
) TO rag_worker_executor;
GRANT UPDATE(status, current_version_id, updated_at)
  ON knowledge_documents
TO rag_projection_owner;
GRANT UPDATE(status, error_code, updated_at)
  ON knowledge_document_versions
TO rag_projection_owner;

CREATE OR REPLACE FUNCTION knowledge_rag_worker_readiness()
RETURNS TABLE(
  consumer_ready BOOLEAN,
  projection_ready BOOLEAN,
  active_index_generation_id UUID,
  detail JSONB
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  WITH required_function(signature) AS (
    VALUES
      ('knowledge_claim_outbox(text,uuid,uuid,integer)'),
      ('knowledge_apply_and_ack_outbox(text,uuid,uuid,uuid,text,uuid,text,text)'),
      ('knowledge_retry_outbox(uuid,uuid,uuid,text,integer)'),
      ('knowledge_fail_outbox(uuid,uuid,uuid,text)'),
      ('knowledge_claim_processing_job(uuid,uuid,integer,text[])'),
      ('knowledge_heartbeat_processing_job(uuid,uuid,uuid,integer)'),
      ('knowledge_finish_processing_job(uuid,uuid,uuid,text,text,integer)'),
      ('knowledge_claim_collection_purge(uuid,uuid,integer)'),
      ('knowledge_enumerate_collection_purge(uuid,uuid,uuid,integer,integer)'),
      ('knowledge_claim_collection_purge_item(uuid,uuid,integer)'),
      ('knowledge_finish_collection_purge_item(uuid,uuid,uuid,boolean,text)'),
      ('knowledge_complete_collection_purge(uuid)'),
      ('knowledge_assert_materialization_search_complete(uuid,bigint,text,integer)'),
      ('knowledge_complete_embedding_and_publish(uuid,uuid,uuid,uuid)')
  ), worker_capability AS (
    SELECT COALESCE(bool_and(
      to_regprocedure(signature) IS NOT NULL
      AND has_function_privilege(
        session_user,
        to_regprocedure(signature),
        'EXECUTE'
      )
    ), false) AS ready
    FROM required_function
  )
  SELECT
    worker_capability.ready,
    COALESCE(state.readiness = 'ready', false),
    head.active_index_generation_id,
    jsonb_build_object(
      'consumer', CASE
        WHEN worker_capability.ready THEN 'ready'
        ELSE 'not_ready'
      END,
      'projection', COALESCE(state.readiness, 'not_ready'),
      'headRevision', head.head_revision,
      'corpusProjectionRevision', head.corpus_projection_revision,
      'searchCompletenessGate', CASE
        WHEN to_regprocedure(
          'knowledge_assert_materialization_search_complete(uuid,bigint,text,integer)'
        ) IS NOT NULL THEN 'ready'
        ELSE 'not_ready'
      END,
      'embeddingPublishGate', CASE
        WHEN to_regprocedure(
          'knowledge_complete_embedding_and_publish(uuid,uuid,uuid,uuid)'
        ) IS NOT NULL THEN 'ready'
        ELSE 'not_ready'
      END
    )
  FROM knowledge_corpus_projection_head head
  CROSS JOIN worker_capability
  LEFT JOIN knowledge_projection_state state
    ON state.index_generation_id = head.active_index_generation_id
  WHERE head.singleton_id = 1
$function$;

ALTER FUNCTION knowledge_rag_worker_readiness()
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_rag_worker_readiness() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_rag_worker_readiness()
TO rag_worker_executor, rag_api_reader, go_api_runtime;
