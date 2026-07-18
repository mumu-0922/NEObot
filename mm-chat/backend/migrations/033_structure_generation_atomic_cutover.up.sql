-- G11.9D.3c: expose the fenced promotion and provide one-step rollback to the
-- verified candidate's exact source generation.

GRANT EXECUTE ON FUNCTION knowledge_promote_index_generation(UUID,BIGINT,TEXT)
  TO go_api_runtime;

CREATE FUNCTION knowledge_rollback_index_generation(
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
      AND NOT EXISTS (
        SELECT 1
        FROM knowledge_parent_chunks parent
        JOIN knowledge_child_chunks child
          ON child.parent_chunk_id=parent.id
         AND child.materialization_id=parent.materialization_id
         AND child.index_generation_id=parent.index_generation_id
         AND child.document_id=parent.document_id
         AND child.document_version_id=parent.document_version_id
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
        WHERE parent.index_generation_id=p_target_generation_id
          AND parent.materialization_id=materialization.id
          AND parent.document_id=document.id
          AND parent.document_version_id=version.id
          AND search.status='ready'
          AND search.embedding_model_id='jina-embeddings-v4'
          AND search.embedding_dimensions=1024
          AND cardinality(search.embedding_vector)=1024
          AND array_position(search.embedding_vector,NULL) IS NULL
          AND search.embedding_vector_sha256 IS NOT NULL
          AND search.ready_at IS NOT NULL
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
GRANT EXECUTE ON FUNCTION knowledge_rollback_index_generation(
  UUID,UUID,BIGINT,TEXT,TEXT
) TO go_api_runtime;
