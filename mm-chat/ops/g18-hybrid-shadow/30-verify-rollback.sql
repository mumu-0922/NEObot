\set ON_ERROR_STOP on

DO $rollback_contract$
BEGIN
  IF to_regclass('knowledge_child_bm25_shadow_projections') IS NOT NULL
    OR to_regclass('knowledge_bm25_shadow_sources') IS NOT NULL
    OR to_regclass('knowledge_bm25_shadow_build_sources') IS NOT NULL
    OR to_regprocedure(
      'knowledge_backfill_bm25_shadow(uuid,uuid)'
    ) IS NOT NULL
    OR to_regprocedure(
      'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)'
    ) IS NOT NULL
    OR to_regprocedure('knowledge_validate_bm25_shadow_insert()') IS NOT NULL
    OR to_regprocedure(
      'knowledge_build_bm25_shadow_text(text,text[])'
    ) IS NOT NULL
  THEN
    RAISE EXCEPTION 'G18.4 shadow objects survived rollback';
  END IF;
  IF to_regprocedure(
    'knowledge_fetch_hybrid_query_evidence_candidates(uuid[],text,real[],integer)'
  ) IS NULL THEN
    RAISE EXCEPTION 'legacy production reader was removed by G18.4 rollback';
  END IF;
  IF (
    SELECT count(*)
    FROM knowledge_child_search_projections
    WHERE embedding_vector IS NOT NULL
  ) <> 4 THEN
    RAISE EXCEPTION 'legacy REAL[] source changed during G18.4 rollback';
  END IF;
END
$rollback_contract$;

SELECT 'PASS G18.4 rollback retained legacy REAL[] reader/data' AS result;

\if :{?expect_vector_shadow}
\else
  \set expect_vector_shadow true
\endif

\if :expect_vector_shadow
DO $vector_retention_contract$
BEGIN
  IF to_regclass('knowledge_child_vector_shadow_projections') IS NULL
    OR to_regprocedure(
      'knowledge_backfill_pgvector_shadow(uuid,uuid)'
    ) IS NULL
  THEN
    RAISE EXCEPTION 'G18.4 rollback removed G18.3 vector shadow objects';
  END IF;
END
$vector_retention_contract$;

SELECT 'PASS G18.4 rollback retained G18.3 vector shadow' AS result;
\else
DO $final_vector_rollback_contract$
BEGIN
  IF to_regclass('knowledge_child_vector_shadow_projections') IS NOT NULL
    OR to_regprocedure(
      'knowledge_backfill_pgvector_shadow(uuid,uuid)'
    ) IS NOT NULL
  THEN
    RAISE EXCEPTION 'G18.3 vector shadow survived final rollback';
  END IF;
END
$final_vector_rollback_contract$;

SELECT 'PASS G18.4 final rollback retained legacy REAL[] reader/data' AS result;
\endif
