-- G11.9D.2.3a: atomically allocate a non-active mixed-format rebuild.

GRANT SELECT, INSERT ON knowledge_index_profiles TO rag_projection_owner;

CREATE FUNCTION knowledge_begin_structure_generation_rebuild(
  p_index_profile_id UUID,
  p_search_profile_id UUID,
  p_generation_id UUID,
  p_chunk_profile_hash TEXT,
  p_base_profile_hash TEXT,
  p_parser_manifest_hash TEXT,
  p_search_profile_hash TEXT,
  p_build_snapshot_hash TEXT,
  p_allocations JSONB
) RETURNS TABLE(
  candidate_generation_id UUID,
  allocated_document_count BIGINT,
  active_generation_id UUID
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_generation knowledge_index_generations%ROWTYPE;
  active_profile knowledge_index_profiles%ROWTYPE;
  active_search knowledge_search_profiles%ROWTYPE;
  allocation JSONB;
  document_row RECORD;
  allocation_count BIGINT;
  expected_count BIGINT;
  outbox_floor BIGINT;
BEGIN
  IF p_index_profile_id IS NULL OR p_search_profile_id IS NULL
    OR p_generation_id IS NULL
    OR p_chunk_profile_hash !~ '^[0-9a-f]{64}$'
    OR p_base_profile_hash !~ '^[0-9a-f]{64}$'
    OR p_parser_manifest_hash !~ '^[0-9a-f]{64}$'
    OR p_search_profile_hash !~ '^[0-9a-f]{64}$'
    OR p_build_snapshot_hash !~ '^[0-9a-f]{64}$'
    OR jsonb_typeof(p_allocations) <> 'array'
    OR jsonb_array_length(p_allocations) < 1
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_STRUCTURE_REBUILD_ARGUMENT_INVALID';
  END IF;

  SELECT generation.* INTO active_generation
  FROM knowledge_corpus_projection_head head
  JOIN knowledge_index_generations generation
    ON generation.id=head.active_index_generation_id
   AND generation.status='active'
  WHERE head.singleton_id=1
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_REBUILD_ACTIVE_GENERATION_MISSING';
  END IF;
  IF EXISTS (
    SELECT 1 FROM knowledge_index_generations
    WHERE status IN ('building','verified')
  ) THEN
    RAISE EXCEPTION USING ERRCODE='55000',
      MESSAGE='RAG_STRUCTURE_REBUILD_CANDIDATE_EXISTS';
  END IF;

  SELECT * INTO active_profile FROM knowledge_index_profiles
  WHERE id=active_generation.index_profile_id;
  SELECT * INTO active_search FROM knowledge_search_profiles
  WHERE index_profile_id=active_profile.id
    AND provider_profile_id='mineru_jina_postgres_v1';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_REBUILD_ACTIVE_PROFILE_MISSING';
  END IF;

  SELECT count(*) INTO expected_count
  FROM knowledge_documents document
  JOIN knowledge_document_versions version
    ON version.id=document.current_version_id
   AND version.document_id=document.id
   AND version.status='active'
  JOIN files file ON file.id=version.file_id
   AND file.upload_status='available' AND file.deleted_at IS NULL
  WHERE document.deleted_at IS NULL AND document.status='active';
  SELECT count(*), count(DISTINCT value->>'documentId')
    INTO allocation_count, outbox_floor
  FROM jsonb_array_elements(p_allocations);
  IF allocation_count <> expected_count OR outbox_floor <> expected_count
    OR EXISTS (
      SELECT 1
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.id=document.current_version_id
       AND version.document_id=document.id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.deleted_at IS NULL AND document.status='active'
        AND NOT EXISTS (
          SELECT 1
          FROM jsonb_array_elements(p_allocations) item
          WHERE item->>'documentId'=document.id::text
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_STRUCTURE_REBUILD_ALLOCATION_COVERAGE_INVALID';
  END IF;

  INSERT INTO knowledge_index_profiles(
    id,contract_version,canonical_schema_version,parser_manifest,
    parser_manifest_hash,chunk_manifest,chunk_profile_hash,
    embedding_processor,embedding_endpoint_id,embedding_model_id,
    embedding_api_version,embedding_role,rerank_processor,rerank_endpoint_id,
    rerank_model_id,rerank_api_version,base_profile_hash
  ) VALUES (
    p_index_profile_id,active_profile.contract_version,
    active_profile.canonical_schema_version,
    jsonb_build_object('schemaVersion','g11.9d-structure-parser-manifest.v1',
      'native','structure','mineru','structure'),p_parser_manifest_hash,
    jsonb_build_object('schemaVersion','g11.9d-structure-chunk-manifest.v1',
      'chunkProfileHash',p_chunk_profile_hash),p_chunk_profile_hash,
    active_profile.embedding_processor,active_profile.embedding_endpoint_id,
    active_profile.embedding_model_id,active_profile.embedding_api_version,
    active_profile.embedding_role,active_profile.rerank_processor,
    active_profile.rerank_endpoint_id,active_profile.rerank_model_id,
    active_profile.rerank_api_version,p_base_profile_hash
  );
  INSERT INTO knowledge_search_profiles(
    id,index_profile_id,provider_profile_id,embedding_processor,
    embedding_model_id,embedding_dimensions,rerank_processor,rerank_model_id,
    lexical_config,exact_config,profile_hash
  ) VALUES (
    p_search_profile_id,p_index_profile_id,active_search.provider_profile_id,
    active_search.embedding_processor,active_search.embedding_model_id,
    active_search.embedding_dimensions,active_search.rerank_processor,
    active_search.rerank_model_id,active_search.lexical_config,
    active_search.exact_config,p_search_profile_hash
  );
  INSERT INTO knowledge_index_generations(
    id,index_profile_id,generation_seq,status,build_snapshot,build_snapshot_hash
  ) VALUES (
    p_generation_id,p_index_profile_id,
    (SELECT COALESCE(max(generation_seq),0)+1 FROM knowledge_index_generations),
    'building',jsonb_build_object(
      'schemaVersion','g11.9d-structure-rebuild-snapshot.v1',
      'sourceGenerationId',active_generation.id,
      'documentCount',expected_count),p_build_snapshot_hash
  );
  SELECT COALESCE(max(id),0) INTO outbox_floor FROM knowledge_outbox;
  INSERT INTO knowledge_projection_state(
    index_generation_id,readiness,projection_revision,required_outbox_floor,
    contiguous_applied_outbox_id,document_count
  ) VALUES (p_generation_id,'building',1,outbox_floor,outbox_floor,expected_count);

  FOR allocation IN SELECT value FROM jsonb_array_elements(p_allocations) LOOP
    IF allocation->>'documentId' !~
      '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR allocation->>'materializationId' !~
      '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR allocation->>'jobId' !~
      '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
      OR allocation->>'requestHash' !~ '^[0-9a-f]{64}$'
    THEN
      RAISE EXCEPTION USING ERRCODE='22023',
        MESSAGE='RAG_STRUCTURE_REBUILD_ALLOCATION_INVALID';
    END IF;
    SELECT d.id document_id,d.collection_id,v.id version_id,v.file_id,
      v.content_hash,v.visibility_epoch,c.acl_revision,c.visibility_epoch collection_visibility_epoch,
      c.collection_processing_revision processing_revision,authority.* INTO document_row
    FROM knowledge_documents d
    JOIN knowledge_collections c ON c.id=d.collection_id
    JOIN knowledge_document_versions v ON v.id=d.current_version_id
    JOIN LATERAL (
      SELECT j.processor,j.endpoint_id,j.model_id,j.governance_profile_id,
        j.governance_revision,j.governance_head_revision,j.collection_consent_id,
        j.collection_consent_revision,j.requested_by_user_id
      FROM knowledge_processing_jobs j
      WHERE j.document_id=d.id AND j.stage='parse' AND j.processor IS NOT NULL
      ORDER BY j.created_at DESC LIMIT 1
    ) authority ON true
    WHERE d.id=(allocation->>'documentId')::uuid
      AND d.deleted_at IS NULL AND d.status='active' AND v.status='active';
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='RAG_STRUCTURE_REBUILD_DOCUMENT_INVALID';
    END IF;
    INSERT INTO knowledge_document_materializations(
      id,index_generation_id,collection_id,document_id,document_version_id,
      file_id,materialization_seq,source_content_hash,base_profile_hash,
      collection_acl_revision,collection_visibility_epoch,
      collection_processing_revision,document_visibility_epoch,status
    ) VALUES (
      (allocation->>'materializationId')::uuid,p_generation_id,
      document_row.collection_id,document_row.document_id,document_row.version_id,
      document_row.file_id,1,document_row.content_hash,p_base_profile_hash,
      document_row.acl_revision,document_row.collection_visibility_epoch,
      document_row.processing_revision,document_row.visibility_epoch,'staging'
    );
    INSERT INTO knowledge_processing_jobs(
      id,collection_id,document_id,document_version_id,file_id,stage,operation,
      processor,endpoint_id,model_id,governance_profile_id,governance_revision,
      governance_head_revision,collection_consent_id,collection_consent_revision,
      collection_acl_revision,collection_visibility_epoch,
      collection_processing_revision,document_visibility_epoch,
      requested_by_user_id,idempotency_scope,idempotency_key,request_hash,
      max_attempts,index_generation_id,materialization_id,legacy_projection_unbound
    ) VALUES (
      (allocation->>'jobId')::uuid,document_row.collection_id,
      document_row.document_id,document_row.version_id,document_row.file_id,
      'parse','reprocess',document_row.processor,document_row.endpoint_id,
      document_row.model_id,document_row.governance_profile_id,
      document_row.governance_revision,document_row.governance_head_revision,
      document_row.collection_consent_id,document_row.collection_consent_revision,
      document_row.acl_revision,document_row.collection_visibility_epoch,
      document_row.processing_revision,document_row.visibility_epoch,
      document_row.requested_by_user_id,
      'structure-rebuild:'||p_generation_id::text,
      document_row.document_id::text,
      allocation->>'requestHash',
      3,p_generation_id,(allocation->>'materializationId')::uuid,false
    );
  END LOOP;
  RETURN QUERY SELECT p_generation_id,expected_count,active_generation.id;
END
$function$;

ALTER FUNCTION knowledge_begin_structure_generation_rebuild(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
) OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_begin_structure_generation_rebuild(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_begin_structure_generation_rebuild(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
) TO go_api_runtime;
