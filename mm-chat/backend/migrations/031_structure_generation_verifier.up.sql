-- G11.9D.3a: verify one complete structure candidate without promoting it.

GRANT UPDATE(status, verified_at)
  ON knowledge_parser_artifact_sets
TO rag_projection_owner;

CREATE FUNCTION knowledge_verify_structure_generation(
  p_index_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_expected_chunk_profile_hash TEXT
) RETURNS TABLE(
  candidate_generation_id UUID,
  artifact_manifest_hash TEXT,
  document_count BIGINT,
  block_count BIGINT,
  parent_count BIGINT,
  child_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  candidate_generation knowledge_index_generations%ROWTYPE;
  candidate_state knowledge_projection_state%ROWTYPE;
  active_generation_id UUID;
  candidate_search_profile_id UUID;
  expected_document_count BIGINT;
  candidate_document_count BIGINT;
  candidate_materialization_count BIGINT;
  candidate_block_count BIGINT;
  candidate_parent_count BIGINT;
  candidate_child_count BIGINT;
  ready_child_count BIGINT;
  projection_head_count BIGINT;
  latest_job_count BIGINT;
  latest_succeeded_job_count BIGINT;
  materialization_aggregate TEXT;
  block_aggregate TEXT;
  parent_aggregate TEXT;
  child_aggregate TEXT;
  computed_manifest_hash TEXT;
  verification_time TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_index_generation_id IS NULL
    OR p_expected_head_revision IS NULL OR p_expected_head_revision < 1
    OR p_expected_chunk_profile_hash IS NULL
    OR p_expected_chunk_profile_hash !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_STRUCTURE_VERIFY_ARGUMENT_INVALID';
  END IF;

  SELECT head.active_index_generation_id INTO active_generation_id
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id=1
    AND head.head_revision=p_expected_head_revision
    AND head.active_index_generation_id IS NOT NULL
    AND head.active_index_generation_id<>p_index_generation_id
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_HEAD_STALE';
  END IF;

  SELECT generation.* INTO candidate_generation
  FROM knowledge_index_generations generation
  WHERE generation.id=p_index_generation_id
    AND generation.status IN ('building','verified')
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_CANDIDATE_MISSING';
  END IF;

  SELECT state.* INTO candidate_state
  FROM knowledge_projection_state state
  WHERE state.index_generation_id=p_index_generation_id
  FOR UPDATE;
  IF NOT FOUND
    OR candidate_state.readiness NOT IN ('building','ready')
    OR (candidate_generation.status='building'
      AND candidate_state.readiness<>'building')
    OR (candidate_generation.status='verified'
      AND candidate_state.readiness<>'ready')
    OR candidate_state.contiguous_applied_outbox_id<>
      candidate_state.required_outbox_floor
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_STATE_INVALID';
  END IF;

  SELECT search_profile.id INTO candidate_search_profile_id
  FROM knowledge_index_profiles index_profile
  JOIN knowledge_search_profiles search_profile
    ON search_profile.index_profile_id=index_profile.id
   AND search_profile.provider_profile_id='mineru_jina_postgres_v1'
   AND search_profile.embedding_processor='jina'
   AND search_profile.embedding_model_id='jina-embeddings-v4'
   AND search_profile.embedding_dimensions=1024
  WHERE index_profile.id=candidate_generation.index_profile_id
    AND index_profile.chunk_profile_hash=p_expected_chunk_profile_hash
    AND index_profile.embedding_processor='jina'
    AND index_profile.embedding_model_id='jina-embeddings-v4'
    AND index_profile.embedding_role='passage';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_PROFILE_MISMATCH';
  END IF;

  SELECT count(*) INTO expected_document_count
  FROM knowledge_documents document
  JOIN knowledge_document_versions version
    ON version.document_id=document.id
   AND version.id=document.current_version_id
   AND version.status='active'
  JOIN files file ON file.id=version.file_id
   AND file.upload_status='available' AND file.deleted_at IS NULL
  WHERE document.status='active' AND document.deleted_at IS NULL;

  SELECT count(*),count(DISTINCT materialization.document_id)
    INTO candidate_materialization_count,candidate_document_count
  FROM knowledge_document_materializations materialization
  WHERE materialization.index_generation_id=p_index_generation_id
    AND materialization.status='published'
    AND materialization.parse_artifact_set_id IS NOT NULL
    AND materialization.manifest_hash IS NOT NULL
    AND materialization.result_hash IS NOT NULL
    AND materialization.verified_at IS NOT NULL
    AND materialization.published_at IS NOT NULL;
  IF expected_document_count<1
    OR candidate_materialization_count<>expected_document_count
    OR candidate_document_count<>expected_document_count
    OR candidate_state.document_count<>expected_document_count
    OR EXISTS (
      SELECT document.id,version.id,version.file_id,version.content_hash
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.document_id=document.id
       AND version.id=document.current_version_id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
      EXCEPT
      SELECT materialization.document_id,materialization.document_version_id,
        materialization.file_id,materialization.source_content_hash
      FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND materialization.status='published'
    ) OR EXISTS (
      SELECT materialization.document_id,materialization.document_version_id,
        materialization.file_id,materialization.source_content_hash
      FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND materialization.status='published'
      EXCEPT
      SELECT document.id,version.id,version.file_id,version.content_hash
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.document_id=document.id
       AND version.id=document.current_version_id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_COVERAGE_INVALID';
  END IF;

  WITH latest_job AS (
    SELECT DISTINCT ON (job.materialization_id,job.stage)
      job.materialization_id,job.stage,job.status
    FROM knowledge_processing_jobs job
    WHERE job.index_generation_id=p_index_generation_id
      AND job.stage IN ('parse','passage_embedding')
    ORDER BY job.materialization_id,job.stage,job.created_at DESC,job.id DESC
  )
  SELECT count(*),count(*) FILTER (WHERE status='succeeded')
    INTO latest_job_count,latest_succeeded_job_count
  FROM latest_job;
  IF latest_job_count<>expected_document_count*2
    OR latest_succeeded_job_count<>latest_job_count
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_JOBS_INCOMPLETE';
  END IF;

  SELECT count(*) INTO projection_head_count
  FROM knowledge_document_projection_heads head
  JOIN knowledge_document_materializations materialization
    ON materialization.id=head.active_materialization_id
   AND materialization.index_generation_id=head.index_generation_id
   AND materialization.document_id=head.document_id
   AND materialization.status='published'
  WHERE head.index_generation_id=p_index_generation_id;
  IF projection_head_count<>expected_document_count THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_HEADS_INCOMPLETE';
  END IF;

  SELECT count(*) INTO candidate_block_count
  FROM knowledge_blocks block
  JOIN knowledge_parser_artifact_sets artifact_set
    ON artifact_set.id=block.artifact_set_id
   AND artifact_set.document_id=block.document_id
   AND artifact_set.document_version_id=block.document_version_id
  JOIN knowledge_document_materializations materialization
    ON materialization.parse_artifact_set_id=artifact_set.id
   AND materialization.document_id=artifact_set.document_id
   AND materialization.document_version_id=artifact_set.document_version_id
  WHERE materialization.index_generation_id=p_index_generation_id
    AND artifact_set.index_profile_id=candidate_generation.index_profile_id
    AND artifact_set.status IN ('staging','verified');
  IF candidate_block_count<expected_document_count
    OR EXISTS (
      SELECT 1
      FROM knowledge_document_materializations materialization
      JOIN knowledge_parser_artifact_sets artifact_set
        ON artifact_set.id=materialization.parse_artifact_set_id
      WHERE materialization.index_generation_id=p_index_generation_id
        AND (artifact_set.index_profile_id<>candidate_generation.index_profile_id
          OR artifact_set.status NOT IN ('staging','verified'))
    ) OR EXISTS (
      SELECT 1
      FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND NOT EXISTS (
          SELECT 1 FROM knowledge_blocks block
          WHERE block.artifact_set_id=materialization.parse_artifact_set_id
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_ARTIFACTS_INCOMPLETE';
  END IF;

  SELECT count(*) INTO candidate_parent_count
  FROM knowledge_parent_chunks parent
  WHERE parent.index_generation_id=p_index_generation_id;
  SELECT count(*) INTO candidate_child_count
  FROM knowledge_child_chunks child
  WHERE child.index_generation_id=p_index_generation_id;
  SELECT count(*) INTO ready_child_count
  FROM knowledge_child_chunks child
  JOIN knowledge_parent_chunks parent
    ON parent.id=child.parent_chunk_id
   AND parent.materialization_id=child.materialization_id
   AND parent.index_generation_id=child.index_generation_id
  JOIN knowledge_child_search_projections search
    ON search.child_chunk_id=child.id
   AND search.parent_chunk_id=parent.id
   AND search.materialization_id=child.materialization_id
   AND search.index_generation_id=child.index_generation_id
   AND search.document_id=child.document_id
   AND search.document_version_id=child.document_version_id
   AND search.source_span_hash=child.source_span_hash
   AND search.chunk_profile_hash=child.chunk_profile_hash
   AND search.content_hash=child.content_hash
   AND search.locator_summary=parent.locator_summary
  WHERE child.index_generation_id=p_index_generation_id
    AND parent.chunk_profile_hash=p_expected_chunk_profile_hash
    AND child.chunk_profile_hash=p_expected_chunk_profile_hash
    AND parent.locator_summary->>'schemaVersion'=
      'g7.4-locator-summary.v1'
    AND jsonb_typeof(parent.locator_summary->'primary')='object'
    AND parent.locator_summary->'primary'->>'kind' IN (
      'text_offset','line_range','page_bbox','slide_shape','sheet_cell',
      'ooxml_part_xpath'
    )
    AND jsonb_typeof(parent.locator_summary->'primary'->'locator')='object'
    AND search.search_profile_id=candidate_search_profile_id
    AND search.status='ready'
    AND search.embedding_model_id='jina-embeddings-v4'
    AND search.embedding_dimensions=1024
    AND cardinality(search.embedding_vector)=1024
    AND array_position(search.embedding_vector,NULL) IS NULL
    AND search.embedding_vector_sha256 IS NOT NULL
    AND search.ready_at IS NOT NULL;
  IF candidate_parent_count<expected_document_count
    OR candidate_child_count<expected_document_count
    OR ready_child_count<>candidate_child_count
    OR EXISTS (
      SELECT 1 FROM knowledge_document_materializations materialization
      WHERE materialization.index_generation_id=p_index_generation_id
        AND (NOT EXISTS (
          SELECT 1 FROM knowledge_parent_chunks parent
          WHERE parent.materialization_id=materialization.id
        ) OR NOT EXISTS (
          SELECT 1 FROM knowledge_child_chunks child
          WHERE child.materialization_id=materialization.id
        ))
    ) OR EXISTS (
      SELECT 1 FROM knowledge_parent_chunks parent
      WHERE parent.index_generation_id=p_index_generation_id
        AND NOT EXISTS (
          SELECT 1 FROM knowledge_child_chunks child
          WHERE child.parent_chunk_id=parent.id
            AND child.materialization_id=parent.materialization_id
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_PROJECTION_INCOMPLETE';
  END IF;

  IF candidate_generation.status='building' THEN
    UPDATE knowledge_parser_artifact_sets artifact_set
    SET status='verified',verified_at=verification_time
    FROM knowledge_document_materializations materialization
    WHERE materialization.index_generation_id=p_index_generation_id
      AND materialization.parse_artifact_set_id=artifact_set.id
      AND artifact_set.status='staging';
  ELSIF EXISTS (
    SELECT 1
    FROM knowledge_document_materializations materialization
    JOIN knowledge_parser_artifact_sets artifact_set
      ON artifact_set.id=materialization.parse_artifact_set_id
    WHERE materialization.index_generation_id=p_index_generation_id
      AND (artifact_set.status<>'verified' OR artifact_set.verified_at IS NULL)
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_ARTIFACTS_INCOMPLETE';
  END IF;

  SELECT string_agg(row_hash,'' ORDER BY document_id,materialization_id)
    INTO materialization_aggregate
  FROM (
    SELECT materialization.document_id,
      materialization.id materialization_id,
      encode(sha256(convert_to(
        'g11.9d.3a:materialization:v1|' || materialization.id::text || '|' ||
        materialization.document_id::text || '|' ||
        materialization.document_version_id::text || '|' ||
        materialization.file_id::text || '|' ||
        materialization.parse_artifact_set_id::text || '|' ||
        materialization.source_content_hash || '|' ||
        materialization.base_profile_hash || '|' ||
        materialization.manifest_hash || '|' || materialization.result_hash || '|' ||
        artifact_set.manifest_hash || '|' || artifact_set.parser_kind || '|' ||
        artifact_set.parser_version || '|' ||
        encode(sha256(convert_to(artifact_set.quality_report::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_document_materializations materialization
    JOIN knowledge_parser_artifact_sets artifact_set
      ON artifact_set.id=materialization.parse_artifact_set_id
    WHERE materialization.index_generation_id=p_index_generation_id
  ) rows;

  SELECT string_agg(row_hash,'' ORDER BY document_id,ordinal,block_id)
    INTO block_aggregate
  FROM (
    SELECT block.document_id,block.ordinal,block.id block_id,
      encode(sha256(convert_to(
        'g11.9d.3a:block:v1|' || block.id::text || '|' ||
        block.artifact_set_id::text || '|' || block.document_id::text || '|' ||
        block.document_version_id::text || '|' || block.ordinal::text || '|' ||
        block.block_type || '|' || block.content_hash || '|' ||
        block.source_span_hash || '|' || block.locator_kind || '|' ||
        encode(sha256(convert_to(block.locator::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_blocks block
    JOIN knowledge_document_materializations materialization
      ON materialization.parse_artifact_set_id=block.artifact_set_id
    WHERE materialization.index_generation_id=p_index_generation_id
  ) rows;

  SELECT string_agg(row_hash,'' ORDER BY document_id,ordinal,parent_id)
    INTO parent_aggregate
  FROM (
    SELECT parent.document_id,parent.ordinal,parent.id parent_id,
      encode(sha256(convert_to(
        'g11.9d.3a:parent:v1|' || parent.id::text || '|' ||
        parent.materialization_id::text || '|' || parent.document_id::text || '|' ||
        parent.document_version_id::text || '|' || parent.ordinal::text || '|' ||
        parent.chunk_profile_hash || '|' || parent.source_span_hash || '|' ||
        parent.content_hash || '|' ||
        encode(sha256(convert_to(parent.locator_summary::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_parent_chunks parent
    WHERE parent.index_generation_id=p_index_generation_id
  ) rows;

  SELECT string_agg(row_hash,'' ORDER BY document_id,ordinal,child_id)
    INTO child_aggregate
  FROM (
    SELECT child.document_id,child.ordinal,child.id child_id,
      encode(sha256(convert_to(
        'g11.9d.3a:child:v1|' || child.id::text || '|' ||
        child.parent_chunk_id::text || '|' || child.materialization_id::text || '|' ||
        child.document_id::text || '|' || child.document_version_id::text || '|' ||
        child.ordinal::text || '|' || child.chunk_profile_hash || '|' ||
        child.source_span_hash || '|' || child.content_hash || '|' ||
        search.search_profile_id::text || '|' || search.embedding_model_id || '|' ||
        search.embedding_dimensions::text || '|' ||
        search.embedding_vector_sha256 || '|' ||
        encode(sha256(convert_to(search.locator_summary::text,'UTF8')),'hex'),
        'UTF8'
      )),'hex') row_hash
    FROM knowledge_child_chunks child
    JOIN knowledge_child_search_projections search
      ON search.child_chunk_id=child.id
     AND search.materialization_id=child.materialization_id
    WHERE child.index_generation_id=p_index_generation_id
  ) rows;

  IF materialization_aggregate IS NULL OR block_aggregate IS NULL
    OR parent_aggregate IS NULL OR child_aggregate IS NULL
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_MANIFEST_INPUT_MISSING';
  END IF;
  computed_manifest_hash := encode(sha256(convert_to(
    'g11.9d.3a:structure-generation-manifest:v1' || E'\n' ||
    candidate_generation.id::text || E'\n' ||
    candidate_generation.index_profile_id::text || E'\n' ||
    candidate_generation.build_snapshot_hash || E'\n' ||
    p_expected_chunk_profile_hash || E'\n' ||
    expected_document_count::text || E'\n' || candidate_block_count::text || E'\n' ||
    candidate_parent_count::text || E'\n' || candidate_child_count::text || E'\n' ||
    materialization_aggregate || E'\n' || block_aggregate || E'\n' ||
    parent_aggregate || E'\n' || child_aggregate,
    'UTF8'
  )),'hex');

  IF candidate_generation.status='building' THEN
    UPDATE knowledge_index_generations
    SET status='verified',artifact_manifest_hash=computed_manifest_hash,
      verified_at=verification_time
    WHERE id=p_index_generation_id AND status='building';
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='RAG_STRUCTURE_VERIFY_CANDIDATE_STALE';
    END IF;
    UPDATE knowledge_projection_state
    SET readiness='ready',projection_revision=projection_revision+1,
      manifest_hash=computed_manifest_hash,
      document_count=expected_document_count,
      parent_count=candidate_parent_count,child_count=candidate_child_count,
      verified_at=verification_time,updated_at=verification_time
    WHERE index_generation_id=p_index_generation_id AND readiness='building';
    IF NOT FOUND THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='RAG_STRUCTURE_VERIFY_STATE_STALE';
    END IF;
  ELSIF candidate_generation.artifact_manifest_hash IS DISTINCT FROM
      computed_manifest_hash
    OR candidate_state.manifest_hash IS DISTINCT FROM computed_manifest_hash
    OR candidate_state.document_count<>expected_document_count
    OR candidate_state.parent_count<>candidate_parent_count
    OR candidate_state.child_count<>candidate_child_count
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_VERIFY_REPLAY_MISMATCH';
  END IF;

  RETURN QUERY SELECT p_index_generation_id,computed_manifest_hash,
    expected_document_count,candidate_block_count,
    candidate_parent_count,candidate_child_count;
END
$function$;

ALTER FUNCTION knowledge_verify_structure_generation(UUID,BIGINT,TEXT)
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_verify_structure_generation(UUID,BIGINT,TEXT)
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_verify_structure_generation(UUID,BIGINT,TEXT)
  TO go_api_runtime;
