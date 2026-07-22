\set ON_ERROR_STOP on

DO $restart_state_contract$
DECLARE
  readiness RECORD;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile = 'pg17_bm25_pgvector_v1'
      AND revision = 2
  ) OR (SELECT count(*) FROM knowledge_retrieval_profile_transitions) <> 1
  THEN
    RAISE EXCEPTION 'restart did not retain PG17 profile revision 2';
  END IF;

  SELECT * INTO readiness
  FROM knowledge_assert_pg17_retrieval_profile_ready();
  IF readiness.eligible_count <> 5
    OR readiness.vector_count <> 5
    OR readiness.bm25_count <> 5
  THEN
    RAISE EXCEPTION 'restart readiness result: %', readiness;
  END IF;
END
$restart_state_contract$;

SET ROLE go_api_runtime;
DO $restart_reader_contract$
DECLARE
  winner UUID;
BEGIN
  SELECT child_chunk_id INTO winner
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'ERR_CONN_RESET',
    ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]),
    5
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  IF winner <> '18180000-0000-0000-0000-000000000013' THEN
    RAISE EXCEPTION 'restart profiled winner = %', winner;
  END IF;
END
$restart_reader_contract$;
RESET ROLE;

SELECT 'PASS G18.5B.1 restart retained pg17 profile and reader' AS result;
