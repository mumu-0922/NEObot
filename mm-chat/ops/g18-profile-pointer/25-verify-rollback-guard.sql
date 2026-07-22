\set ON_ERROR_STOP on

DO $rollback_guard_contract$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM schema_migrations
    WHERE version = 37
      AND name = 'rag_retrieval_profile_pointer'
      AND length(checksum) = 64
  ) OR to_regprocedure(
    'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)'
  ) IS NULL OR to_regprocedure(
    'knowledge_set_retrieval_profile(text,text,bigint,text)'
  ) IS NULL THEN
    RAISE EXCEPTION 'failed rollback did not preserve migration 037 atomically';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE active_profile = 'pg17_bm25_pgvector_v1'
      AND revision = 1
  ) THEN
    RAISE EXCEPTION 'rollback guard changed the active profile';
  END IF;
END
$rollback_guard_contract$;

SELECT 'PASS G18.5A non-legacy rollback rejected atomically' AS result;
