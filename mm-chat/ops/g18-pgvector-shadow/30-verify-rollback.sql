\set ON_ERROR_STOP on

DO $rollback_contract$
BEGIN
  IF to_regclass('knowledge_child_vector_shadow_projections') IS NOT NULL
    OR to_regclass('knowledge_pgvector_shadow_sources') IS NOT NULL
    OR to_regprocedure(
      'knowledge_backfill_pgvector_shadow(uuid,uuid)'
    ) IS NOT NULL
    OR to_regprocedure('knowledge_validate_pgvector_shadow_insert()') IS NOT NULL
  THEN
    RAISE EXCEPTION 'G18.3 shadow objects survived rollback';
  END IF;
  IF to_regprocedure(
    'knowledge_fetch_hybrid_query_evidence_candidates(uuid[],text,real[],integer)'
  ) IS NULL THEN
    RAISE EXCEPTION 'legacy production reader was removed by shadow rollback';
  END IF;
  IF (
    SELECT count(*)
    FROM knowledge_child_search_projections
    WHERE embedding_vector IS NOT NULL
  ) <> 6 THEN
    RAISE EXCEPTION 'legacy REAL[] source changed during shadow rollback';
  END IF;
END
$rollback_contract$;

SELECT 'PASS G18.3 rollback retained legacy REAL[] reader/data' AS result;
