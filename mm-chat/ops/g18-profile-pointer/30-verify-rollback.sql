\set ON_ERROR_STOP on

DO $rollback_contract$
BEGIN
  IF to_regclass('knowledge_retrieval_profile_head') IS NOT NULL
    OR to_regclass('knowledge_retrieval_profile_transitions') IS NOT NULL
    OR to_regprocedure(
      'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)'
    ) IS NOT NULL
    OR to_regprocedure(
      'knowledge_set_retrieval_profile(text,text,bigint,text)'
    ) IS NOT NULL
  THEN
    RAISE EXCEPTION 'migration 037 objects survived rollback';
  END IF;
  IF to_regprocedure(
    'knowledge_fetch_hybrid_query_evidence_candidates(uuid[],text,real[],integer)'
  ) IS NULL THEN
    RAISE EXCEPTION 'migration 037 rollback removed the legacy reader';
  END IF;
  IF EXISTS (SELECT 1 FROM schema_migrations WHERE version = 37) THEN
    RAISE EXCEPTION 'migration 037 record survived rollback';
  END IF;
END
$rollback_contract$;

SELECT 'PASS G18.5A rollback retained legacy reader' AS result;
