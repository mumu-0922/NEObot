REVOKE EXECUTE ON FUNCTION knowledge_fail_structure_generation(UUID,BIGINT,TEXT,TEXT)
  FROM go_api_runtime;
DROP FUNCTION knowledge_fail_structure_generation(UUID,BIGINT,TEXT,TEXT);

CREATE OR REPLACE FUNCTION knowledge_promote_index_generation(
  p_index_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_manifest_hash TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  previous_generation UUID;
BEGIN
  IF p_manifest_hash IS NULL OR p_manifest_hash !~ '^[0-9a-f]{64}$' THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_PROMOTION_ARGUMENT_INVALID';
  END IF;
  IF to_regclass('knowledge_search_profiles') IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_SEARCH_SCHEMA_NOT_READY';
  END IF;
  SELECT active_index_generation_id INTO previous_generation
  FROM knowledge_corpus_projection_head
  WHERE singleton_id=1 AND head_revision=p_expected_head_revision
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_HEAD_STALE';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM knowledge_index_generations generation
    JOIN knowledge_projection_state state
      ON state.index_generation_id=generation.id
    WHERE generation.id=p_index_generation_id
      AND generation.status='verified'
      AND generation.artifact_manifest_hash=p_manifest_hash
      AND state.readiness='ready' AND state.manifest_hash=p_manifest_hash
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_NOT_READY';
  END IF;

  IF previous_generation IS NOT NULL THEN
    UPDATE knowledge_index_generations
    SET status='retired',retired_at=clock_timestamp()
    WHERE id=previous_generation AND status='active';
    UPDATE knowledge_projection_state
    SET readiness='retired',updated_at=clock_timestamp()
    WHERE index_generation_id=previous_generation;
  END IF;
  UPDATE knowledge_index_generations
  SET status='active',activated_at=clock_timestamp()
  WHERE id=p_index_generation_id AND status='verified';
  UPDATE knowledge_corpus_projection_head
  SET active_index_generation_id=p_index_generation_id,
    corpus_projection_revision=corpus_projection_revision+1,
    head_revision=head_revision+1,updated_at=clock_timestamp()
  WHERE singleton_id=1 AND head_revision=p_expected_head_revision;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_HEAD_STALE';
  END IF;
  RETURN true;
END
$function$;

ALTER FUNCTION knowledge_promote_index_generation(UUID,BIGINT,TEXT)
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_promote_index_generation(UUID,BIGINT,TEXT)
  FROM PUBLIC;
