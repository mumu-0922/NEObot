\set ON_ERROR_STOP on

DO $concurrent_projection_contract$
DECLARE
  readiness RECORD;
BEGIN
  IF (SELECT count(*) FROM knowledge_document_projection_heads
      WHERE document_id IN (
        '18400000-0000-0000-0000-000000000002'::UUID,
        '18400000-0000-0000-0000-000000000012'::UUID
      )) <> 2
    OR (SELECT count(*) FROM knowledge_child_vector_shadow_projections
        WHERE child_chunk_id IN (
          '18400000-0000-0000-0000-000000000006'::UUID,
          '18400000-0000-0000-0000-000000000016'::UUID
        )) <> 2
    OR (SELECT count(*) FROM knowledge_child_bm25_shadow_projections
        WHERE child_chunk_id IN (
          '18400000-0000-0000-0000-000000000006'::UUID,
          '18400000-0000-0000-0000-000000000016'::UUID
        )) <> 2
  THEN
    RAISE EXCEPTION 'concurrent head publication projection count mismatch';
  END IF;

  SELECT * INTO readiness
  FROM knowledge_assert_pg17_retrieval_profile_ready();
  IF readiness.eligible_count <> 6
    OR readiness.vector_count <> 6
    OR readiness.bm25_count <> 6
  THEN
    RAISE EXCEPTION 'post-publication readiness result: %', readiness;
  END IF;
END
$concurrent_projection_contract$;

SET ROLE go_api_runtime;
DO $published_reader_contract$
DECLARE
  alpha_winner UUID;
  beta_winner UUID;
BEGIN
  SELECT child_chunk_id INTO alpha_winner
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'LIVE_ALPHA',
    ARRAY[0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1020]),
    5
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  SELECT child_chunk_id INTO beta_winner
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'LIVE_BETA',
    ARRAY[0::REAL, 0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1019]),
    5
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  IF alpha_winner <> '18400000-0000-0000-0000-000000000006'
    OR beta_winner <> '18400000-0000-0000-0000-000000000016'
  THEN
    RAISE EXCEPTION 'published winners alpha=% beta=%',
      alpha_winner, beta_winner;
  END IF;
END
$published_reader_contract$;
RESET ROLE;

UPDATE knowledge_document_projection_heads
SET active_materialization_id = active_materialization_id
WHERE document_id = '18400000-0000-0000-0000-000000000012';

SET ROLE rag_replay_operator;
DO $idempotent_manual_sync$
DECLARE
  result RECORD;
BEGIN
  SELECT * INTO result
  FROM knowledge_sync_pg17_retrieval_materialization(
    '18400000-0000-0000-0000-000000000014'
  );
  IF result.eligible_count <> 1
    OR result.vector_inserted_count <> 0
    OR result.bm25_inserted_count <> 0
    OR result.verified_count <> 1
  THEN
    RAISE EXCEPTION 'idempotent materialization sync result: %', result;
  END IF;
END
$idempotent_manual_sync$;
RESET ROLE;

UPDATE knowledge_documents
SET status = 'tombstoned',
    visibility_epoch = visibility_epoch + 1,
    deleted_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE id = '18400000-0000-0000-0000-000000000002';

DO $delete_visibility_contract$
DECLARE
  alpha_count BIGINT;
  beta_count BIGINT;
  readiness RECORD;
BEGIN
  SELECT count(*) INTO alpha_count
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'LIVE_ALPHA',
    ARRAY[0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1020]),
    5
  ) candidate
  WHERE candidate.child_chunk_id =
    '18400000-0000-0000-0000-000000000006';
  SELECT count(*) INTO beta_count
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'LIVE_BETA',
    ARRAY[0::REAL, 0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1019]),
    5
  ) candidate
  WHERE candidate.child_chunk_id =
    '18400000-0000-0000-0000-000000000016';
  IF alpha_count <> 0 OR beta_count <> 1 THEN
    RAISE EXCEPTION 'delete visibility alpha=% beta=%',
      alpha_count, beta_count;
  END IF;
  IF (SELECT count(*) FROM knowledge_child_vector_shadow_projections) <> 6
    OR (SELECT count(*) FROM knowledge_child_bm25_shadow_projections) <> 6
  THEN
    RAISE EXCEPTION 'delete destroyed immutable rollback projections';
  END IF;

  SELECT * INTO readiness
  FROM knowledge_assert_pg17_retrieval_profile_ready();
  IF readiness.eligible_count <> 5
    OR readiness.vector_count <> 5
    OR readiness.bm25_count <> 5
  THEN
    RAISE EXCEPTION 'post-delete readiness result: %', readiness;
  END IF;
END
$delete_visibility_contract$;

DO $maintenance_security_contract$
BEGIN
  IF has_function_privilege(
    'go_api_runtime',
    'knowledge_sync_pg17_retrieval_materialization(uuid)',
    'EXECUTE'
  ) OR has_function_privilege(
    'rag_worker_executor',
    'knowledge_sync_pg17_retrieval_materialization(uuid)',
    'EXECUTE'
  ) OR NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_sync_pg17_retrieval_materialization(uuid)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'materialization sync privileges are not bounded';
  END IF;
  IF EXISTS (
    SELECT 1 FROM pg_proc function
    WHERE function.oid IN (
      'knowledge_sync_pg17_retrieval_materialization(uuid)'::REGPROCEDURE,
      'knowledge_maintain_pg17_retrieval_on_head()'::REGPROCEDURE
    )
      AND NOT EXISTS (
        SELECT 1 FROM unnest(function.proconfig) setting
        WHERE setting LIKE 'search_path=%pg_catalog%pg_temp'
          AND setting NOT LIKE '%$user%'
      )
  ) THEN
    RAISE EXCEPTION 'maintenance SECURITY DEFINER path is not hardened';
  END IF;
END
$maintenance_security_contract$;

SELECT 'PASS G18.5B.2a concurrent publish=2 sync=atomic delete=hidden rows=retained' AS result;
