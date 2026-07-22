\set ON_ERROR_STOP on

DO $head_contract$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile = 'legacy'
      AND revision = 1
  ) THEN
    RAISE EXCEPTION 'retrieval profile does not default to legacy revision 1';
  END IF;
  IF (SELECT count(*) FROM knowledge_retrieval_profile_transitions) <> 0 THEN
    RAISE EXCEPTION 'fresh retrieval profile has transition history';
  END IF;
END
$head_contract$;

CREATE TEMP TABLE g18_legacy_candidates AS
SELECT *
FROM knowledge_fetch_hybrid_query_evidence_candidates(
  ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
  'synthetic semantic query',
  array_fill(0.001::REAL, ARRAY[1024]),
  5
);

CREATE TEMP TABLE g18_profiled_candidates AS
SELECT *
FROM knowledge_fetch_profiled_query_evidence_candidates(
  ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
  'synthetic semantic query',
  array_fill(0.001::REAL, ARRAY[1024]),
  5
);

DO $parity_contract$
BEGIN
  IF (SELECT count(*) FROM g18_legacy_candidates) <> 1
    OR EXISTS (
      (SELECT * FROM g18_legacy_candidates EXCEPT ALL
       SELECT * FROM g18_profiled_candidates)
      UNION ALL
      (SELECT * FROM g18_profiled_candidates EXCEPT ALL
       SELECT * FROM g18_legacy_candidates)
    )
  THEN
    RAISE EXCEPTION 'legacy/profiled reader parity failed';
  END IF;
END
$parity_contract$;

SET ROLE go_api_runtime;
DO $go_runtime_contract$
DECLARE
  candidate_count BIGINT;
BEGIN
  SELECT count(*) INTO candidate_count
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'synthetic semantic query',
    array_fill(0.001::REAL, ARRAY[1024]),
    5
  );
  IF candidate_count <> 1 THEN
    RAISE EXCEPTION 'Go runtime profiled candidate count = %', candidate_count;
  END IF;
END
$go_runtime_contract$;
RESET ROLE;

SET ROLE rag_worker_executor;
DO $worker_runtime_contract$
DECLARE
  candidate_count BIGINT;
BEGIN
  SELECT count(*) INTO candidate_count
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'synthetic semantic query',
    array_fill(0.001::REAL, ARRAY[1024]),
    5
  );
  IF candidate_count <> 1 THEN
    RAISE EXCEPTION 'worker profiled candidate count = %', candidate_count;
  END IF;
END
$worker_runtime_contract$;
RESET ROLE;

SET ROLE rag_replay_operator;
DO $operator_contract$
DECLARE
  result RECORD;
BEGIN
  SELECT * INTO result
  FROM knowledge_set_retrieval_profile(
    'legacy',
    'legacy',
    1,
    'G18.5A idempotent legacy proof'
  );
  IF result.active_profile <> 'legacy' OR result.revision <> 1 THEN
    RAISE EXCEPTION 'idempotent pointer result = %', result;
  END IF;

  BEGIN
    PERFORM * FROM knowledge_set_retrieval_profile(
      'legacy',
      'pg17_bm25_pgvector_v1',
      1,
      'G18.5A unavailable target proof'
    );
    RAISE EXCEPTION 'unavailable PG17 profile was activated';
  EXCEPTION WHEN SQLSTATE '55000' THEN
    IF SQLERRM <> 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE' THEN
      RAISE;
    END IF;
  END;

  BEGIN
    PERFORM * FROM knowledge_set_retrieval_profile(
      'legacy',
      'legacy',
      999,
      'G18.5A compare-and-swap conflict proof'
    );
    RAISE EXCEPTION 'stale profile revision was accepted';
  EXCEPTION WHEN SQLSTATE '40001' THEN
    IF SQLERRM <> 'RAG_RETRIEVAL_PROFILE_CONFLICT' THEN
      RAISE;
    END IF;
  END;
END
$operator_contract$;
RESET ROLE;

DO $security_contract$
BEGIN
  IF has_table_privilege(
    'go_api_runtime',
    'knowledge_retrieval_profile_head',
    'SELECT'
  ) OR has_table_privilege(
    'rag_worker_executor',
    'knowledge_retrieval_profile_transitions',
    'SELECT'
  ) OR has_function_privilege(
    'go_api_runtime',
    'knowledge_set_retrieval_profile(text,text,bigint,text)',
    'EXECUTE'
  ) OR has_function_privilege(
    'rag_worker_executor',
    'knowledge_set_retrieval_profile(text,text,bigint,text)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'production runtime received profile mutation privileges';
  END IF;
  IF NOT has_function_privilege(
    'go_api_runtime',
    'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)',
    'EXECUTE'
  ) OR NOT has_function_privilege(
    'rag_worker_executor',
    'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)',
    'EXECUTE'
  ) OR NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_set_retrieval_profile(text,text,bigint,text)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'required profiled retrieval privileges are missing';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    WHERE function.oid IN (
      'knowledge_set_retrieval_profile(text,text,bigint,text)'::REGPROCEDURE,
      'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)'::REGPROCEDURE
    )
      AND NOT EXISTS (
        SELECT 1
        FROM unnest(function.proconfig) setting
        WHERE setting LIKE 'search_path=%pg_catalog%pg_temp'
          AND setting NOT LIKE '%$user%'
      )
  ) THEN
    RAISE EXCEPTION 'profile SECURITY DEFINER search_path is not hardened';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE active_profile = 'legacy' AND revision = 1
  ) OR (SELECT count(*) FROM knowledge_retrieval_profile_transitions) <> 0 THEN
    RAISE EXCEPTION 'failed pointer operations changed durable state';
  END IF;
END
$security_contract$;

DROP TABLE g18_profiled_candidates;
DROP TABLE g18_legacy_candidates;

SELECT 'PASS G18.5A profile=legacy parity=exact roles=bounded pg17=fail-closed' AS result;
