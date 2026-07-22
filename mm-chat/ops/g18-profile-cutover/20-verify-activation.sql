\set ON_ERROR_STOP on

SET ROLE rag_replay_operator;
DO $partial_backfill_rejection$
BEGIN
  BEGIN
    PERFORM * FROM knowledge_set_retrieval_profile(
      'legacy',
      'pg17_bm25_pgvector_v1',
      1,
      'G18.5B.1 partial-backfill rejection'
    );
    RAISE EXCEPTION 'PG17 profile activated before BM25 backfill';
  EXCEPTION WHEN SQLSTATE '55000' THEN
    IF SQLERRM <> 'RAG_RETRIEVAL_PROFILE_BACKFILL_INCOMPLETE' THEN
      RAISE;
    END IF;
  END;
END
$partial_backfill_rejection$;
RESET ROLE;

DO $partial_backfill_state$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1 AND active_profile = 'legacy' AND revision = 1
  ) OR (SELECT count(*) FROM knowledge_retrieval_profile_transitions) <> 0 THEN
    RAISE EXCEPTION 'failed partial-backfill activation changed pointer state';
  END IF;
END
$partial_backfill_state$;

SET ROLE rag_replay_operator;
DO $backfill_and_activation$
DECLARE
  vector_result RECORD;
  bm25_result RECORD;
  readiness RECORD;
  transition_result RECORD;
BEGIN
  SELECT * INTO vector_result
  FROM knowledge_backfill_pgvector_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF vector_result.eligible_count <> 4
    OR vector_result.inserted_count <> 0
    OR vector_result.verified_shadow_count <> 4
  THEN
    RAISE EXCEPTION 'unexpected vector backfill result: %', vector_result;
  END IF;
  SELECT * INTO bm25_result
  FROM knowledge_backfill_bm25_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF bm25_result.eligible_count <> 4
    OR bm25_result.inserted_count <> 4
    OR bm25_result.verified_shadow_count <> 4
  THEN
    RAISE EXCEPTION 'unexpected BM25 backfill result: %', bm25_result;
  END IF;

  SELECT * INTO readiness
  FROM knowledge_assert_pg17_retrieval_profile_ready();
  IF readiness.index_generation_id <>
      '18180000-0000-0000-0000-000000000007'
    OR readiness.search_profile_id <>
      '18180000-0000-0000-0000-000000000011'
    OR readiness.eligible_count <> 4
    OR readiness.vector_count <> 4
    OR readiness.bm25_count <> 4
  THEN
    RAISE EXCEPTION 'unexpected readiness result: %', readiness;
  END IF;

  SELECT * INTO transition_result
  FROM knowledge_set_retrieval_profile(
    'legacy',
    'pg17_bm25_pgvector_v1',
    1,
    'G18.5B.1 verified disposable activation'
  );
  IF transition_result.active_profile <> 'pg17_bm25_pgvector_v1'
    OR transition_result.revision <> 2
  THEN
    RAISE EXCEPTION 'unexpected activation result: %', transition_result;
  END IF;
END
$backfill_and_activation$;
RESET ROLE;

SET ROLE go_api_runtime;
DO $go_runtime_reader$
DECLARE
  winner UUID;
  winner_score REAL;
  negative_count BIGINT;
BEGIN
  SELECT child_chunk_id, rank_score INTO winner, winner_score
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'ERR_CONN_RESET /api/v1/jobs',
    ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]),
    5
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  IF winner <> '18180000-0000-0000-0000-000000000013'
    OR winner_score IS NULL OR winner_score <= 0
  THEN
    RAISE EXCEPTION 'PG17 profiled winner=% score=%', winner, winner_score;
  END IF;

  SELECT count(*) INTO negative_count
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    '杭州明天会下雨吗？',
    ARRAY[0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1021]),
    5
  );
  IF negative_count <> 0 THEN
    RAISE EXCEPTION 'PG17 profiled negative returned % candidates',
      negative_count;
  END IF;

  BEGIN
    PERFORM * FROM knowledge_fetch_profiled_query_evidence_candidates(
      ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
      'zero vector rejection',
      array_fill(0::REAL, ARRAY[1024]),
      5
    );
    RAISE EXCEPTION 'profiled reader accepted a zero query vector';
  EXCEPTION WHEN SQLSTATE '22023' THEN
    IF SQLERRM <> 'RAG_HYBRID_QUERY_EMBEDDING_INVALID' THEN
      RAISE;
    END IF;
  END;

  BEGIN
    PERFORM * FROM knowledge_set_retrieval_profile(
      'pg17_bm25_pgvector_v1', 'legacy', 2, 'forbidden runtime mutation'
    );
    RAISE EXCEPTION 'Go runtime mutated retrieval profile';
  EXCEPTION WHEN SQLSTATE '42501' THEN
    NULL;
  END;
END
$go_runtime_reader$;
RESET ROLE;

SET ROLE rag_worker_executor;
DO $worker_runtime_reader$
DECLARE
  winner UUID;
BEGIN
  SELECT child_chunk_id INTO winner
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '服务暂时中断该怎么办',
    ARRAY[0.8::REAL, 0.6::REAL] || array_fill(0::REAL, ARRAY[1022]),
    5
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  IF winner <> '18300000-0000-0000-0000-000000000001' THEN
    RAISE EXCEPTION 'worker semantic winner = %', winner;
  END IF;

  BEGIN
    PERFORM * FROM knowledge_fetch_hybrid_shadow_diagnostics(
      ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
      'forbidden diagnostics',
      (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
      5
    );
    RAISE EXCEPTION 'worker executed private diagnostics';
  EXCEPTION WHEN SQLSTATE '42501' THEN
    NULL;
  END;
END
$worker_runtime_reader$;
RESET ROLE;

DO $activation_security_contract$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile = 'pg17_bm25_pgvector_v1'
      AND revision = 2
  ) OR NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_transitions
    WHERE from_profile = 'legacy'
      AND to_profile = 'pg17_bm25_pgvector_v1'
      AND revision = 2
  ) OR (SELECT count(*) FROM knowledge_retrieval_profile_transitions) <> 1
  THEN
    RAISE EXCEPTION 'activation pointer/history contract failed';
  END IF;

  IF has_function_privilege(
    'go_api_runtime',
    'knowledge_assert_pg17_retrieval_profile_ready()',
    'EXECUTE'
  ) OR has_function_privilege(
    'rag_worker_executor',
    'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)',
    'EXECUTE'
  ) OR NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_assert_pg17_retrieval_profile_ready()',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'profile activation privileges are not bounded';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    WHERE function.oid IN (
      'knowledge_assert_pg17_retrieval_profile_ready()'::REGPROCEDURE,
      'knowledge_set_retrieval_profile(text,text,bigint,text)'::REGPROCEDURE,
      'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)'::REGPROCEDURE
    )
      AND NOT EXISTS (
        SELECT 1 FROM unnest(function.proconfig) setting
        WHERE setting LIKE 'search_path=%pg_catalog%pg_temp'
          AND setting NOT LIKE '%$user%'
      )
  ) THEN
    RAISE EXCEPTION 'profile cutover SECURITY DEFINER path is not hardened';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    CROSS JOIN LATERAL unnest(function.proargnames) argument(name)
    WHERE function.oid =
      'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)'::REGPROCEDURE
      AND argument.name IN (
        'lexical_text', 'bm25_text', 'source_text', 'exact_terms',
        'bm25_rank', 'bm25_score', 'dense_rank', 'dense_score'
      )
  ) THEN
    RAISE EXCEPTION 'profiled reader exposes private text or diagnostics';
  END IF;
END
$activation_security_contract$;

SELECT 'PASS G18.5B.1 backfill=complete activation=pg17 roles=bounded negatives=0' AS result;
