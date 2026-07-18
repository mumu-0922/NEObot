-- G11.9D.3b: revalidate the verified candidate inside promotion and provide
-- an auditable fail rollback that never mutates the active generation.

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
  candidate_chunk_profile_hash TEXT;
  verified_manifest_hash TEXT;
  transition_time TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_index_generation_id IS NULL
    OR p_expected_head_revision IS NULL OR p_expected_head_revision < 1
    OR p_manifest_hash IS NULL OR p_manifest_hash !~ '^[0-9a-f]{64}$'
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_PROMOTION_ARGUMENT_INVALID';
  END IF;
  IF to_regclass('knowledge_search_profiles') IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_SEARCH_SCHEMA_NOT_READY';
  END IF;

  SELECT head.active_index_generation_id INTO previous_generation
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id=1
    AND head.head_revision=p_expected_head_revision
    AND head.active_index_generation_id IS NOT NULL
    AND head.active_index_generation_id<>p_index_generation_id
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_HEAD_STALE';
  END IF;

  SELECT profile.chunk_profile_hash INTO candidate_chunk_profile_hash
  FROM knowledge_index_generations generation
  JOIN knowledge_index_profiles profile ON profile.id=generation.index_profile_id
  JOIN knowledge_projection_state state
    ON state.index_generation_id=generation.id
  WHERE generation.id=p_index_generation_id
    AND generation.status='verified'
    AND generation.artifact_manifest_hash=p_manifest_hash
    AND state.readiness='ready'
    AND state.manifest_hash=p_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_NOT_READY';
  END IF;

  SELECT verification.artifact_manifest_hash INTO verified_manifest_hash
  FROM knowledge_verify_structure_generation(
    p_index_generation_id,
    p_expected_head_revision,
    candidate_chunk_profile_hash
  ) verification;
  IF verified_manifest_hash IS DISTINCT FROM p_manifest_hash THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_FENCE_MISMATCH';
  END IF;

  UPDATE knowledge_index_generations
  SET status='retired',retired_at=transition_time
  WHERE id=previous_generation AND status='active';
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_HEAD_STALE';
  END IF;
  UPDATE knowledge_projection_state
  SET readiness='retired',updated_at=transition_time
  WHERE index_generation_id=previous_generation;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_NOT_READY';
  END IF;

  UPDATE knowledge_index_generations
  SET status='active',activated_at=transition_time
  WHERE id=p_index_generation_id
    AND status='verified'
    AND artifact_manifest_hash=p_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_PROMOTION_NOT_READY';
  END IF;
  UPDATE knowledge_corpus_projection_head
  SET active_index_generation_id=p_index_generation_id,
    corpus_projection_revision=corpus_projection_revision+1,
    head_revision=head_revision+1,updated_at=transition_time
  WHERE singleton_id=1
    AND head_revision=p_expected_head_revision
    AND active_index_generation_id=previous_generation;
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
REVOKE EXECUTE ON FUNCTION knowledge_promote_index_generation(UUID,BIGINT,TEXT)
  FROM go_api_runtime;

CREATE FUNCTION knowledge_fail_structure_generation(
  p_index_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_expected_manifest_hash TEXT,
  p_failure_code TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  candidate_generation knowledge_index_generations%ROWTYPE;
  candidate_state knowledge_projection_state%ROWTYPE;
  failure_time TIMESTAMPTZ := clock_timestamp();
BEGIN
  IF p_index_generation_id IS NULL
    OR p_expected_head_revision IS NULL OR p_expected_head_revision < 1
    OR p_expected_manifest_hash IS NULL
    OR p_expected_manifest_hash !~ '^[0-9a-f]{64}$'
    OR p_failure_code IS NULL OR p_failure_code !~ '^[A-Z0-9_]{1,64}$'
  THEN
    RAISE EXCEPTION USING ERRCODE='22023',
      MESSAGE='RAG_STRUCTURE_FAIL_ARGUMENT_INVALID';
  END IF;

  PERFORM 1
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id=1
    AND head.head_revision=p_expected_head_revision
    AND head.active_index_generation_id IS NOT NULL
    AND head.active_index_generation_id<>p_index_generation_id
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_FAIL_HEAD_STALE';
  END IF;

  SELECT generation.* INTO candidate_generation
  FROM knowledge_index_generations generation
  WHERE generation.id=p_index_generation_id
    AND generation.status IN ('verified','failed')
    AND generation.artifact_manifest_hash=p_expected_manifest_hash
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_FAIL_CANDIDATE_MISSING';
  END IF;
  SELECT state.* INTO candidate_state
  FROM knowledge_projection_state state
  WHERE state.index_generation_id=p_index_generation_id
    AND state.manifest_hash=p_expected_manifest_hash
    AND state.readiness IN ('ready','failed')
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_FAIL_STATE_MISMATCH';
  END IF;

  IF candidate_generation.status='failed' THEN
    IF candidate_generation.failure_code<>p_failure_code
      OR candidate_state.readiness<>'failed'
    THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='RAG_STRUCTURE_FAIL_REPLAY_MISMATCH';
    END IF;
    RETURN true;
  END IF;
  IF candidate_state.readiness<>'ready' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_FAIL_STATE_MISMATCH';
  END IF;

  UPDATE knowledge_index_generations
  SET status='failed',failure_code=p_failure_code,failed_at=failure_time
  WHERE id=p_index_generation_id
    AND status='verified'
    AND artifact_manifest_hash=p_expected_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_FAIL_CANDIDATE_STALE';
  END IF;
  UPDATE knowledge_projection_state
  SET readiness='failed',updated_at=failure_time
  WHERE index_generation_id=p_index_generation_id
    AND readiness='ready'
    AND manifest_hash=p_expected_manifest_hash;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='RAG_STRUCTURE_FAIL_STATE_STALE';
  END IF;
  RETURN true;
END
$function$;

ALTER FUNCTION knowledge_fail_structure_generation(UUID,BIGINT,TEXT,TEXT)
  OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_fail_structure_generation(UUID,BIGINT,TEXT,TEXT)
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_fail_structure_generation(UUID,BIGINT,TEXT,TEXT)
  TO go_api_runtime;
