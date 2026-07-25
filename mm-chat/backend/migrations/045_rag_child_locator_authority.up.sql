-- Make the exact matched Child Search projection the Citation locator
-- authority. Parent locators remain source-backed context metadata only.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_locator_summary_is_valid(
  p_summary JSONB
) RETURNS BOOLEAN
LANGUAGE plpgsql
IMMUTABLE
STRICT
PARALLEL SAFE
SET search_path FROM CURRENT
AS $function$
DECLARE
  locator_entry JSONB;
  primary_locator JSONB;
  locator JSONB;
  locator_kind TEXT;
  observed_hashes JSONB := '[]'::JSONB;
  primary_found BOOLEAN := false;
  start_value BIGINT;
  end_value BIGINT;
BEGIN
  IF jsonb_typeof(p_summary) IS DISTINCT FROM 'object'
    OR p_summary->>'schemaVersion' IS DISTINCT FROM
      'g7.4-locator-summary.v1'
    OR jsonb_typeof(p_summary->'primary') IS DISTINCT FROM 'object'
    OR jsonb_typeof(p_summary->'fragments') IS DISTINCT FROM 'array'
    OR jsonb_typeof(p_summary->'locatorAggregateHashes') IS DISTINCT FROM
      'array'
  THEN
    RETURN false;
  END IF;
  IF jsonb_array_length(p_summary->'fragments') < 1
    OR jsonb_array_length(p_summary->'fragments') <>
      jsonb_array_length(p_summary->'locatorAggregateHashes')
  THEN
    RETURN false;
  END IF;

  primary_locator := p_summary->'primary';
  FOR locator_entry IN
    SELECT item.value
    FROM jsonb_array_elements(p_summary->'fragments') item(value)
  LOOP
    IF jsonb_typeof(locator_entry) IS DISTINCT FROM 'object'
      OR COALESCE(locator_entry->>'locatorAggregateHash', '') !~
        '^[0-9a-f]{64}$'
      OR jsonb_typeof(locator_entry->'locator') IS DISTINCT FROM 'object'
    THEN
      RETURN false;
    END IF;

    locator_kind := locator_entry->>'kind';
    locator := locator_entry->'locator';
    IF COALESCE(locator_kind, '') NOT IN (
        'text_offset', 'line_range', 'page_bbox', 'slide_shape',
        'sheet_cell', 'ooxml_part_xpath'
      )
      OR locator->>'kind' IS DISTINCT FROM locator_kind
    THEN
      RETURN false;
    END IF;

    BEGIN
      CASE locator_kind
        WHEN 'text_offset' THEN
          IF COALESCE(locator->>'start', '') !~ '^(0|[1-9][0-9]*)$'
            OR COALESCE(locator->>'end', '') !~ '^(0|[1-9][0-9]*)$'
          THEN
            RETURN false;
          END IF;
          start_value := (locator->>'start')::BIGINT;
          end_value := (locator->>'end')::BIGINT;
          IF end_value <= start_value THEN
            RETURN false;
          END IF;
        WHEN 'line_range' THEN
          IF COALESCE(locator->>'startLine', '') !~ '^(0|[1-9][0-9]*)$'
            OR COALESCE(locator->>'endLine', '') !~ '^(0|[1-9][0-9]*)$'
          THEN
            RETURN false;
          END IF;
          start_value := (locator->>'startLine')::BIGINT;
          end_value := (locator->>'endLine')::BIGINT;
          IF end_value < start_value THEN
            RETURN false;
          END IF;
        WHEN 'page_bbox' THEN
          IF COALESCE(locator->>'page', '') !~ '^(0|[1-9][0-9]*)$'
            OR COALESCE(locator->>'x1', '') !~ '^(0|[1-9][0-9]*)$'
            OR COALESCE(locator->>'y1', '') !~ '^(0|[1-9][0-9]*)$'
            OR COALESCE(locator->>'x2', '') !~ '^(0|[1-9][0-9]*)$'
            OR COALESCE(locator->>'y2', '') !~ '^(0|[1-9][0-9]*)$'
          THEN
            RETURN false;
          END IF;
          IF (locator->>'x2')::BIGINT <= (locator->>'x1')::BIGINT
            OR (locator->>'y2')::BIGINT <= (locator->>'y1')::BIGINT
          THEN
            RETURN false;
          END IF;
        WHEN 'slide_shape' THEN
          IF COALESCE(locator->>'slide', '') !~ '^(0|[1-9][0-9]*)$'
            OR COALESCE(locator->>'shape', '') !~ '^(0|[1-9][0-9]*)$'
          THEN
            RETURN false;
          END IF;
          PERFORM (locator->>'slide')::BIGINT, (locator->>'shape')::BIGINT;
        WHEN 'sheet_cell' THEN
          IF COALESCE(locator->>'sheet', '') !~ '^[0-9a-f]{64}$'
            OR COALESCE(length(trim(locator->>'startCell')), 0)
              NOT BETWEEN 1 AND 128
            OR COALESCE(length(trim(locator->>'endCell')), 0)
              NOT BETWEEN 1 AND 128
            OR locator->>'startCell' IS DISTINCT FROM
              trim(locator->>'startCell')
            OR locator->>'endCell' IS DISTINCT FROM trim(locator->>'endCell')
          THEN
            RETURN false;
          END IF;
        WHEN 'ooxml_part_xpath' THEN
          IF COALESCE(locator->>'part', '') !~ '^[0-9a-f]{64}$'
            OR COALESCE(locator->>'xpath', '') !~ '^[0-9a-f]{64}$'
          THEN
            RETURN false;
          END IF;
        ELSE
          RETURN false;
      END CASE;
    EXCEPTION
      WHEN numeric_value_out_of_range OR invalid_text_representation THEN
        RETURN false;
    END;

    observed_hashes := observed_hashes ||
      jsonb_build_array(locator_entry->'locatorAggregateHash');
    IF locator_entry = primary_locator THEN
      primary_found := true;
    END IF;
  END LOOP;

  RETURN primary_found
    AND observed_hashes = p_summary->'locatorAggregateHashes';
END
$function$;

ALTER FUNCTION knowledge_locator_summary_is_valid(JSONB)
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_locator_summary_is_valid(JSONB) FROM PUBLIC;

CREATE OR REPLACE FUNCTION knowledge_verify_structure_generation(
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
  WHERE child.index_generation_id=p_index_generation_id
    AND parent.chunk_profile_hash=p_expected_chunk_profile_hash
    AND child.chunk_profile_hash=p_expected_chunk_profile_hash
    AND knowledge_locator_summary_is_valid(parent.locator_summary)
    AND knowledge_locator_summary_is_valid(search.locator_summary)
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
REVOKE EXECUTE ON FUNCTION knowledge_verify_structure_generation(UUID,BIGINT,TEXT)
  FROM go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_verify_structure_generation(UUID,BIGINT,TEXT)
  TO rag_replay_operator;

CREATE OR REPLACE FUNCTION knowledge_rollback_index_generation(
  p_active_generation_id UUID,
  p_target_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_active_manifest_hash TEXT,
  p_target_manifest_hash TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_generation knowledge_index_generations%ROWTYPE;
  target_generation knowledge_index_generations%ROWTYPE;
  transition_time TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_active_generation_id IS NULL OR p_target_generation_id IS NULL
    OR p_active_generation_id=p_target_generation_id
    OR p_expected_head_revision IS NULL OR p_expected_head_revision < 1
    OR p_active_manifest_hash IS NULL
    OR p_active_manifest_hash !~ '^[0-9a-f]{64}$'
    OR p_target_manifest_hash IS NULL
    OR p_target_manifest_hash !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_GENERATION_ROLLBACK_ARGUMENT_INVALID';
  END IF;

  PERFORM 1
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id=1
    AND head.head_revision=p_expected_head_revision
    AND head.active_index_generation_id=p_active_generation_id
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_HEAD_STALE';
  END IF;

  SELECT generation.* INTO active_generation
  FROM knowledge_index_generations generation
  JOIN knowledge_projection_state state
    ON state.index_generation_id=generation.id
  WHERE generation.id=p_active_generation_id
    AND generation.status='active'
    AND generation.artifact_manifest_hash=p_active_manifest_hash
    AND state.readiness='ready'
    AND state.manifest_hash=p_active_manifest_hash
  FOR UPDATE OF generation,state;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_ACTIVE_MISMATCH';
  END IF;
  IF (active_generation.build_snapshot->>'schemaVersion') IS DISTINCT FROM
      'g11.9d-structure-rebuild-snapshot.v1'
    OR (active_generation.build_snapshot->>'sourceGenerationId')
      IS DISTINCT FROM p_target_generation_id::TEXT
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_SOURCE_MISMATCH';
  END IF;

  SELECT generation.* INTO target_generation
  FROM knowledge_index_generations generation
  JOIN knowledge_projection_state state
    ON state.index_generation_id=generation.id
  WHERE generation.id=p_target_generation_id
    AND generation.status='retired'
    AND generation.artifact_manifest_hash=p_target_manifest_hash
    AND state.readiness='retired'
    AND state.manifest_hash=p_target_manifest_hash
  FOR UPDATE OF generation,state;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_TARGET_MISMATCH';
  END IF;

  IF NOT EXISTS (
      SELECT 1
      FROM knowledge_documents document
      JOIN knowledge_document_versions version
        ON version.id=document.current_version_id
       AND version.document_id=document.id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
    ) OR EXISTS (
      SELECT 1
      FROM knowledge_documents document
      JOIN knowledge_collections collection
        ON collection.id=document.collection_id
       AND collection.deleted_at IS NULL
      JOIN knowledge_document_versions version
        ON version.id=document.current_version_id
       AND version.document_id=document.id
       AND version.status='active'
      JOIN files file ON file.id=version.file_id
       AND file.upload_status='available' AND file.deleted_at IS NULL
      WHERE document.status='active' AND document.deleted_at IS NULL
        AND NOT EXISTS (
          SELECT 1
          FROM knowledge_document_projection_heads target_head
          JOIN knowledge_document_materializations materialization
            ON materialization.id=target_head.active_materialization_id
           AND materialization.index_generation_id=target_head.index_generation_id
           AND materialization.document_id=target_head.document_id
           AND materialization.status='published'
          WHERE target_head.index_generation_id=p_target_generation_id
            AND target_head.document_id=document.id
            AND materialization.collection_id=collection.id
            AND materialization.document_version_id=version.id
            AND materialization.file_id=version.file_id
            AND materialization.source_content_hash=version.content_hash
        )
    )
  THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_COVERAGE_INVALID';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_documents document
    JOIN knowledge_collections collection
      ON collection.id=document.collection_id
     AND collection.deleted_at IS NULL
    JOIN knowledge_document_versions version
      ON version.id=document.current_version_id
     AND version.document_id=document.id
     AND version.status='active'
    JOIN files file ON file.id=version.file_id
     AND file.upload_status='available' AND file.deleted_at IS NULL
    JOIN knowledge_document_projection_heads target_head
      ON target_head.index_generation_id=p_target_generation_id
     AND target_head.document_id=document.id
    JOIN knowledge_document_materializations materialization
      ON materialization.id=target_head.active_materialization_id
     AND materialization.index_generation_id=target_head.index_generation_id
     AND materialization.document_id=target_head.document_id
     AND materialization.document_version_id=version.id
     AND materialization.status='published'
    WHERE document.status='active' AND document.deleted_at IS NULL
      AND (
        NOT EXISTS (
          SELECT 1
          FROM knowledge_parent_chunks parent
          WHERE parent.index_generation_id=p_target_generation_id
            AND parent.materialization_id=materialization.id
            AND parent.document_id=document.id
            AND parent.document_version_id=version.id
        )
        OR NOT EXISTS (
          SELECT 1
          FROM knowledge_child_chunks child
          WHERE child.index_generation_id=p_target_generation_id
            AND child.materialization_id=materialization.id
            AND child.document_id=document.id
            AND child.document_version_id=version.id
        )
        OR EXISTS (
          SELECT 1
          FROM knowledge_parent_chunks parent
          WHERE parent.index_generation_id=p_target_generation_id
            AND parent.materialization_id=materialization.id
            AND parent.document_id=document.id
            AND parent.document_version_id=version.id
            AND NOT EXISTS (
              SELECT 1
              FROM knowledge_child_chunks child
              WHERE child.parent_chunk_id=parent.id
                AND child.materialization_id=parent.materialization_id
            )
        )
        OR EXISTS (
          SELECT 1
          FROM knowledge_child_chunks child
          JOIN knowledge_parent_chunks parent
            ON parent.id=child.parent_chunk_id
           AND parent.materialization_id=child.materialization_id
           AND parent.index_generation_id=child.index_generation_id
           AND parent.document_id=child.document_id
           AND parent.document_version_id=child.document_version_id
          LEFT JOIN knowledge_child_search_projections search
            ON search.child_chunk_id=child.id
           AND search.parent_chunk_id=parent.id
           AND search.materialization_id=child.materialization_id
           AND search.index_generation_id=child.index_generation_id
           AND search.document_id=child.document_id
           AND search.document_version_id=child.document_version_id
           AND search.source_span_hash=child.source_span_hash
           AND search.chunk_profile_hash=child.chunk_profile_hash
           AND search.content_hash=child.content_hash
          LEFT JOIN knowledge_search_profiles search_profile
            ON search_profile.id=search.search_profile_id
           AND search_profile.index_profile_id=target_generation.index_profile_id
           AND search_profile.provider_profile_id='mineru_jina_postgres_v1'
           AND search_profile.embedding_processor='jina'
           AND search_profile.embedding_model_id='jina-embeddings-v4'
           AND search_profile.embedding_dimensions=1024
          WHERE child.index_generation_id=p_target_generation_id
            AND child.materialization_id=materialization.id
            AND child.document_id=document.id
            AND child.document_version_id=version.id
            AND (
              search.child_chunk_id IS NULL
              OR search_profile.id IS NULL
              OR search.status<>'ready'
              OR search.embedding_model_id<>'jina-embeddings-v4'
              OR search.embedding_dimensions<>1024
              OR cardinality(search.embedding_vector)<>1024
              OR array_position(search.embedding_vector,NULL) IS NOT NULL
              OR search.embedding_vector_sha256 IS NULL
              OR search.ready_at IS NULL
              OR NOT knowledge_locator_summary_is_valid(parent.locator_summary)
              OR NOT knowledge_locator_summary_is_valid(search.locator_summary)
            )
        )
      )
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_PROJECTION_INCOMPLETE';
  END IF;

  UPDATE knowledge_index_generations
  SET status='retired',retired_at=transition_time
  WHERE id=p_active_generation_id AND status='active'
    AND artifact_manifest_hash=p_active_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;
  UPDATE knowledge_projection_state
  SET readiness='retired',updated_at=transition_time
  WHERE index_generation_id=p_active_generation_id
    AND readiness='ready' AND manifest_hash=p_active_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;

  UPDATE knowledge_index_generations
  SET status='active',retired_at=NULL
  WHERE id=p_target_generation_id AND status='retired'
    AND artifact_manifest_hash=p_target_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;
  UPDATE knowledge_projection_state
  SET readiness='ready',updated_at=transition_time
  WHERE index_generation_id=p_target_generation_id
    AND readiness='retired' AND manifest_hash=p_target_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_STATE_STALE';
  END IF;

  UPDATE knowledge_corpus_projection_head
  SET active_index_generation_id=p_target_generation_id,
    corpus_projection_revision=corpus_projection_revision+1,
    head_revision=head_revision+1,updated_at=transition_time
  WHERE singleton_id=1
    AND head_revision=p_expected_head_revision
    AND active_index_generation_id=p_active_generation_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_GENERATION_ROLLBACK_HEAD_STALE';
  END IF;
  RETURN true;
END
$function$;


ALTER FUNCTION knowledge_rollback_index_generation(UUID,UUID,BIGINT,TEXT,TEXT)
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_rollback_index_generation(
  UUID,UUID,BIGINT,TEXT,TEXT
) FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION knowledge_rollback_index_generation(
  UUID,UUID,BIGINT,TEXT,TEXT
) FROM go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_rollback_index_generation(
  UUID,UUID,BIGINT,TEXT,TEXT
) TO rag_replay_operator;

CREATE OR REPLACE FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  p_actor_user_id UUID,
  p_session_id UUID,
  p_conversation_id UUID,
  p_references JSONB
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  source_text TEXT,
  child_token_count INTEGER,
  parent_source_text TEXT,
  parent_token_count INTEGER,
  locator JSONB
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_references IS NULL
    OR jsonb_typeof(p_references) <> 'array'
    OR jsonb_array_length(p_references) NOT BETWEEN 1 AND 16
    OR NOT EXISTS (
      SELECT 1 FROM sessions s
      JOIN conversations c ON c.id = p_conversation_id
      WHERE s.id = p_session_id AND s.user_id = p_actor_user_id
        AND s.revoked_at IS NULL AND s.expires_at > clock_timestamp()
        AND c.user_id = p_actor_user_id AND c.status <> 'deleted'
        AND c.deleted_at IS NULL
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'RAG_HYDRATION_NOT_AUTHORIZED';
  END IF;

  RETURN QUERY
  WITH requested AS (
    SELECT * FROM jsonb_to_recordset(p_references) AS reference(
      collection_id UUID,
      document_id UUID,
      document_version_id UUID,
      index_generation_id UUID,
      materialization_id UUID,
      parent_chunk_id UUID,
      child_chunk_id UUID,
      source_span_hash TEXT,
      content_hash TEXT
    )
  ), authorized AS (
    SELECT
      r.*,
      child.content AS child_source_text,
      child.token_count AS child_token_count,
      parent.content AS parent_source_text,
      parent.token_count AS parent_token_count,
      search.locator_summary AS child_locator_summary
    FROM requested r
    JOIN knowledge_corpus_projection_head corpus
      ON corpus.singleton_id = 1
      AND corpus.active_index_generation_id = r.index_generation_id
    JOIN knowledge_document_projection_heads head
      ON head.index_generation_id = r.index_generation_id
      AND head.document_id = r.document_id
      AND head.active_materialization_id = r.materialization_id
    JOIN knowledge_document_materializations m
      ON m.id = r.materialization_id
      AND m.collection_id = r.collection_id
      AND m.document_id = r.document_id
      AND m.document_version_id = r.document_version_id
      AND m.index_generation_id = r.index_generation_id
      AND m.status = 'published'
    JOIN knowledge_collections collection ON collection.id = r.collection_id
    JOIN knowledge_documents document
      ON document.id = r.document_id
      AND document.collection_id = collection.id
      AND document.current_version_id = r.document_version_id
      AND document.status = 'active' AND document.deleted_at IS NULL
    JOIN knowledge_document_versions version
      ON version.id = r.document_version_id
      AND version.document_id = document.id
      AND version.status = 'active'
      AND version.visibility_epoch = m.document_visibility_epoch
      AND version.content_hash = m.source_content_hash
    JOIN knowledge_index_generations generation
      ON generation.id = r.index_generation_id
      AND generation.status = 'active'
    JOIN knowledge_parent_chunks parent
      ON parent.id = r.parent_chunk_id
      AND parent.materialization_id = r.materialization_id
      AND parent.index_generation_id = r.index_generation_id
      AND parent.document_id = r.document_id
      AND parent.document_version_id = r.document_version_id
    JOIN knowledge_child_chunks child
      ON child.id = r.child_chunk_id
      AND child.parent_chunk_id = parent.id
      AND child.materialization_id = r.materialization_id
      AND child.index_generation_id = r.index_generation_id
      AND child.document_id = r.document_id
      AND child.document_version_id = r.document_version_id
      AND child.source_span_hash = r.source_span_hash
      AND child.content_hash = r.content_hash
    JOIN knowledge_search_profiles search_profile
      ON search_profile.index_profile_id = generation.index_profile_id
      AND search_profile.provider_profile_id = 'mineru_jina_postgres_v1'
      AND search_profile.embedding_processor = 'jina'
      AND search_profile.embedding_model_id = 'jina-embeddings-v4'
      AND search_profile.embedding_dimensions = 1024
    JOIN knowledge_child_search_projections search
      ON search.child_chunk_id = child.id
      AND search.parent_chunk_id = parent.id
      AND search.materialization_id = child.materialization_id
      AND search.index_generation_id = child.index_generation_id
      AND search.collection_id = r.collection_id
      AND search.document_id = child.document_id
      AND search.document_version_id = child.document_version_id
      AND search.search_profile_id = search_profile.id
      AND search.source_span_hash = child.source_span_hash
      AND search.chunk_profile_hash = child.chunk_profile_hash
      AND search.content_hash = child.content_hash
      AND search.status = 'ready'
      AND search.embedding_model_id = 'jina-embeddings-v4'
      AND search.embedding_dimensions = 1024
      AND cardinality(search.embedding_vector) = 1024
      AND array_position(search.embedding_vector, NULL) IS NULL
      AND search.embedding_vector_sha256 IS NOT NULL
      AND search.ready_at IS NOT NULL
      AND knowledge_locator_summary_is_valid(search.locator_summary)
    WHERE collection.deleted_at IS NULL
      AND collection.acl_revision = m.collection_acl_revision
      AND collection.visibility_epoch = m.collection_visibility_epoch
      AND collection.collection_processing_revision = m.collection_processing_revision
      AND document.visibility_epoch = m.document_visibility_epoch
      AND (
        (collection.scope = 'personal' AND collection.owner_user_id = p_actor_user_id)
        OR (
          collection.scope = 'team'
          AND EXISTS (
            SELECT 1 FROM team_memberships membership
            JOIN teams team ON team.id = membership.team_id
            WHERE membership.team_id = collection.team_id
              AND membership.user_id = p_actor_user_id
              AND membership.status = 'active' AND team.deleted_at IS NULL
          )
        )
      )
  )
  SELECT
    authorized.collection_id,
    authorized.document_id,
    authorized.document_version_id,
    authorized.index_generation_id,
    authorized.materialization_id,
    authorized.parent_chunk_id,
    authorized.child_chunk_id,
    authorized.source_span_hash,
    authorized.content_hash,
    authorized.child_source_text,
    authorized.child_token_count,
    authorized.parent_source_text,
    authorized.parent_token_count,
    authorized.child_locator_summary
  FROM authorized
  WHERE octet_length(authorized.child_source_text) <= 65536
    AND octet_length(authorized.parent_source_text) <= 65536;
END
$function$;


ALTER FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) TO go_evidence_hydrator, go_api_runtime;
