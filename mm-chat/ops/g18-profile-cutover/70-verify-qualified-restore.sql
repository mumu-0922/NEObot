\set ON_ERROR_STOP on

DO $restored_state_contract$
DECLARE
  readiness RECORD;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM schema_migrations
    WHERE version = 37 AND name = 'rag_retrieval_profile_pointer'
  ) OR NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile = 'pg17_bm25_pgvector_v1'
      AND revision = 2
  ) OR to_regprocedure(
    'knowledge_assert_pg17_generation_ready(uuid)'
  ) IS NULL OR to_regprocedure(
    'knowledge_sync_pg17_retrieval_materialization(uuid)'
  ) IS NULL
  THEN
    RAISE EXCEPTION 'qualified restore lost profile or operational schema';
  END IF;

  SELECT * INTO readiness
  FROM knowledge_assert_pg17_retrieval_profile_ready();
  IF readiness.eligible_count <> 4101
    OR readiness.vector_count <> 4101
    OR readiness.bm25_count <> 4101
    OR (SELECT count(*) FROM knowledge_child_vector_shadow_projections)
      <> 4105
    OR (SELECT count(*) FROM knowledge_child_bm25_shadow_projections)
      <> 4105
  THEN
    RAISE EXCEPTION 'qualified restore projection result: %', readiness;
  END IF;
END
$restored_state_contract$;

SET ROLE go_api_runtime;
DO $restored_reader_contract$
DECLARE
  winner UUID;
BEGIN
  SELECT child_chunk_id INTO winner
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'BULK_G18_4096',
    ARRAY[
      (4096 % 97)::REAL / 100::REAL,
      ((4096 * 7) % 101)::REAL / 100::REAL,
      1::REAL
    ] || array_fill(0::REAL, ARRAY[1021]),
    10
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  IF winner <> '18610000-0000-0000-0000-000000004096' THEN
    RAISE EXCEPTION 'qualified restore winner=%', winner;
  END IF;
END
$restored_reader_contract$;
RESET ROLE;

DO $restored_security_contract$
BEGIN
  IF has_function_privilege(
    'go_api_runtime',
    'knowledge_assert_pg17_generation_ready(uuid)',
    'EXECUTE'
  ) OR has_table_privilege(
    'go_api_runtime',
    'knowledge_bm25_shadow_build_sources',
    'SELECT'
  ) OR NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_assert_pg17_generation_ready(uuid)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'qualified restore privilege boundary mismatch';
  END IF;
END
$restored_security_contract$;

SELECT
  pg_database_size(current_database()) AS restored_database_bytes,
  pg_total_relation_size(
    'knowledge_child_vector_shadow_projections'
  ) AS restored_vector_projection_bytes,
  pg_total_relation_size(
    'knowledge_child_bm25_shadow_projections'
  ) AS restored_bm25_projection_bytes;

SELECT 'PASS G18.5B.2c restore rows=4105 active=4101 profile=pg17'
  AS result;
