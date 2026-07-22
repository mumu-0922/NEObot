\set ON_ERROR_STOP on

SET ROLE rag_replay_operator;
DO $switch_back_contract$
DECLARE
  result RECORD;
BEGIN
  SELECT * INTO result
  FROM knowledge_set_retrieval_profile(
    'pg17_bm25_pgvector_v1',
    'legacy',
    2,
    'G18.5B.1 verified disposable rollback'
  );
  IF result.active_profile <> 'legacy' OR result.revision <> 3 THEN
    RAISE EXCEPTION 'unexpected legacy rollback result: %', result;
  END IF;
END
$switch_back_contract$;
RESET ROLE;

CREATE TEMP TABLE g18_direct_legacy AS
SELECT *
FROM knowledge_fetch_hybrid_query_evidence_candidates(
  ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
  'ERR_CONN_RESET',
  ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]),
  5
);

CREATE TEMP TABLE g18_profiled_legacy AS
SELECT *
FROM knowledge_fetch_profiled_query_evidence_candidates(
  ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
  'ERR_CONN_RESET',
  ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]),
  5
);

DO $legacy_parity_contract$
BEGIN
  IF EXISTS (
    (SELECT * FROM g18_direct_legacy EXCEPT ALL
     SELECT * FROM g18_profiled_legacy)
    UNION ALL
    (SELECT * FROM g18_profiled_legacy EXCEPT ALL
     SELECT * FROM g18_direct_legacy)
  ) OR NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1 AND active_profile = 'legacy' AND revision = 3
  ) OR NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_transitions
    WHERE from_profile = 'pg17_bm25_pgvector_v1'
      AND to_profile = 'legacy'
      AND revision = 3
  ) OR (SELECT count(*) FROM knowledge_retrieval_profile_transitions) <> 2
  THEN
    RAISE EXCEPTION 'controlled legacy rollback parity/history failed';
  END IF;
END
$legacy_parity_contract$;

DROP TABLE g18_profiled_legacy;
DROP TABLE g18_direct_legacy;

SELECT 'PASS G18.5B.1 controlled rollback restored exact legacy reader' AS result;
