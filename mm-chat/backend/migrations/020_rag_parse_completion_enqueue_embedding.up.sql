-- G7.5N parse stage finalizer.
--
-- After Python has staged parser projection rows, this token-fenced finalizer
-- atomically commits the parse job and creates exactly one pending
-- passage_embedding job. The generic worker finish path must not run after this
-- function succeeds.

CREATE FUNCTION knowledge_complete_parse_and_enqueue_embedding(
  p_parse_job_id UUID,
  p_worker_id UUID,
  p_lease_token UUID,
  p_materialization_id UUID,
  p_embedding_job_id UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  parse_job knowledge_processing_jobs%ROWTYPE;
  materialization knowledge_document_materializations%ROWTYPE;
  generation knowledge_index_generations%ROWTYPE;
  index_profile knowledge_index_profiles%ROWTYPE;
  embedding_profile processor_governance_profiles%ROWTYPE;
  embedding_head processor_governance_heads%ROWTYPE;
  embedding_consent processing_consents%ROWTYPE;
  staged_search_count BIGINT;
BEGIN
  IF p_parse_job_id IS NULL
    OR p_worker_id IS NULL
    OR p_lease_token IS NULL
    OR p_lease_token = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_materialization_id IS NULL
    OR p_embedding_job_id IS NULL
    OR p_embedding_job_id = '00000000-0000-0000-0000-000000000000'::UUID
    OR p_embedding_job_id = p_parse_job_id
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PARSE_COMPLETION_ARGUMENT_INVALID';
  END IF;

  SELECT processing_job.* INTO parse_job
  FROM knowledge_processing_jobs processing_job
  WHERE processing_job.id = p_parse_job_id
    AND processing_job.status = 'processing'
    AND processing_job.stage = 'parse'
    AND processing_job.operation IN ('initial', 'replace', 'reprocess')
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
    AND index_generation_id = parse_job.index_generation_id
    AND collection_id = parse_job.collection_id
    AND document_id = parse_job.document_id
    AND document_version_id = parse_job.document_version_id
    AND file_id = parse_job.file_id
    AND collection_acl_revision = parse_job.collection_acl_revision
    AND collection_visibility_epoch = parse_job.collection_visibility_epoch
    AND collection_processing_revision = parse_job.collection_processing_revision
    AND document_visibility_epoch = parse_job.document_visibility_epoch
    AND status = 'staging'
    AND parse_artifact_set_id IS NOT NULL
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_MATERIALIZATION_MISSING';
  END IF;

  SELECT count(*) INTO staged_search_count
  FROM knowledge_child_search_projections search
  WHERE search.materialization_id = p_materialization_id
    AND search.index_generation_id = parse_job.index_generation_id
    AND search.collection_id = parse_job.collection_id
    AND search.document_id = parse_job.document_id
    AND search.document_version_id = parse_job.document_version_id
    AND search.embedding_model_id = 'jina-embeddings-v4'
    AND search.embedding_dimensions = 1024
    AND search.status = 'staging'
    AND search.embedding_vector IS NULL
    AND search.embedding_vector_sha256 IS NULL;
  IF staged_search_count < 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_SEARCH_STAGING_MISSING';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_processing_jobs existing_job
    WHERE existing_job.stage = 'passage_embedding'
      AND existing_job.status <> 'cancelled'
      AND (
        existing_job.materialization_id = p_materialization_id
        OR existing_job.caused_by_job_id = p_parse_job_id
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_EMBEDDING_JOB_EXISTS';
  END IF;

  SELECT * INTO generation
  FROM knowledge_index_generations
  WHERE id = parse_job.index_generation_id
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_GENERATION_MISSING';
  END IF;

  SELECT * INTO index_profile
  FROM knowledge_index_profiles
  WHERE id = generation.index_profile_id
    AND embedding_processor = 'jina'
    AND embedding_model_id = 'jina-embeddings-v4'
    AND embedding_role = 'passage';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_PROFILE_MISMATCH';
  END IF;

  SELECT head.* INTO embedding_head
  FROM processor_governance_heads head
  WHERE head.processor = index_profile.embedding_processor
    AND head.endpoint_id = index_profile.embedding_endpoint_id
    AND head.model_id = index_profile.embedding_model_id
    AND head.status = 'active'
    AND head.active_profile_id IS NOT NULL
    AND head.active_governance_revision IS NOT NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_GOVERNANCE_HEAD_MISSING';
  END IF;

  SELECT profile.* INTO embedding_profile
  FROM processor_governance_profiles profile
  WHERE profile.processor = embedding_head.processor
    AND profile.endpoint_id = embedding_head.endpoint_id
    AND profile.model_id = embedding_head.model_id
    AND profile.id = embedding_head.active_profile_id
    AND profile.governance_revision = embedding_head.active_governance_revision
    AND profile.status = 'approved'
    AND 'passage_embedding' = ANY(profile.allowed_purposes)
    AND 'text/plain' = ANY(profile.allowed_data_types);
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_GOVERNANCE_PROFILE_MISSING';
  END IF;

  SELECT consent.* INTO embedding_consent
  FROM processing_consents consent
  WHERE consent.scope = 'collection'
    AND consent.collection_id = parse_job.collection_id
    AND consent.processor = embedding_profile.processor
    AND consent.endpoint_id = embedding_profile.endpoint_id
    AND consent.model_id = embedding_profile.model_id
    AND consent.governance_profile_id = embedding_profile.id
    AND consent.governance_revision = embedding_profile.governance_revision
    AND consent.governance_head_revision = embedding_head.head_revision
    AND consent.decision = 'granted'
    AND consent.superseded_at IS NULL
    AND (consent.expires_at IS NULL OR consent.expires_at > clock_timestamp())
    AND 'passage_embedding' = ANY(consent.purposes)
    AND 'text/plain' = ANY(consent.data_types)
  ORDER BY consent.consent_revision DESC
  LIMIT 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PARSE_COMPLETION_CONSENT_MISSING';
  END IF;

  INSERT INTO knowledge_processing_jobs (
    id, collection_id, document_id, document_version_id, file_id, stage,
    operation, processor, endpoint_id, model_id, governance_profile_id,
    governance_revision, governance_head_revision, collection_consent_id,
    collection_consent_revision, collection_acl_revision,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch, requested_by_user_id, caused_by_job_id,
    idempotency_scope, idempotency_key, request_hash, status, attempt_count,
    max_attempts, available_at, index_generation_id, materialization_id,
    legacy_projection_unbound
  ) VALUES (
    p_embedding_job_id, parse_job.collection_id, parse_job.document_id,
    parse_job.document_version_id, parse_job.file_id, 'passage_embedding',
    parse_job.operation, embedding_profile.processor, embedding_profile.endpoint_id,
    embedding_profile.model_id, embedding_profile.id,
    embedding_profile.governance_revision, embedding_head.head_revision,
    embedding_consent.id, embedding_consent.consent_revision,
    parse_job.collection_acl_revision, parse_job.collection_visibility_epoch,
    parse_job.collection_processing_revision, parse_job.document_visibility_epoch,
    parse_job.requested_by_user_id, parse_job.id,
    'rag:passage_embedding:' || p_materialization_id::TEXT,
    p_embedding_job_id::TEXT,
    encode(sha256(convert_to(
      'passage_embedding:' || p_materialization_id::TEXT || ':' || p_parse_job_id::TEXT,
      'UTF8'
    )), 'hex'),
    'pending', 0, parse_job.max_attempts, clock_timestamp(),
    parse_job.index_generation_id, p_materialization_id, false
  );

  UPDATE knowledge_processing_jobs
  SET status = 'succeeded',
      lease_owner = NULL,
      lease_token = NULL,
      lease_expires_at = NULL,
      completed_at = clock_timestamp(),
      error_code = NULL,
      updated_at = clock_timestamp()
  WHERE id = parse_job.id
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

ALTER FUNCTION knowledge_complete_parse_and_enqueue_embedding(
  UUID, UUID, UUID, UUID, UUID
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_complete_parse_and_enqueue_embedding(
  UUID, UUID, UUID, UUID, UUID
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_complete_parse_and_enqueue_embedding(
  UUID, UUID, UUID, UUID, UUID
) TO rag_worker_executor;

GRANT SELECT ON
  processor_governance_profiles,
  processor_governance_heads,
  processing_consents
TO rag_projection_owner;
