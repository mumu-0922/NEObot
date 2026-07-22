\set ON_ERROR_STOP on

DO $rollback_contract$
BEGIN
  IF to_regprocedure(
    'knowledge_assert_pg17_retrieval_profile_ready()'
  ) IS NOT NULL
    OR to_regclass('knowledge_child_bm25_shadow_projections') IS NOT NULL
    OR to_regclass('knowledge_child_vector_shadow_projections') IS NOT NULL
    OR to_regprocedure(
      'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)'
    ) IS NOT NULL
    OR to_regprocedure(
      'knowledge_sync_pg17_retrieval_materialization(uuid)'
    ) IS NOT NULL
    OR to_regprocedure(
      'knowledge_maintain_pg17_retrieval_on_head()'
    ) IS NOT NULL
    OR to_regprocedure(
      'knowledge_assert_pg17_generation_ready(uuid)'
    ) IS NOT NULL
    OR to_regprocedure(
      'knowledge_fence_pg17_generation_cutover()'
    ) IS NOT NULL
  THEN
    RAISE EXCEPTION 'PG17 cutover objects survived candidate rollback';
  END IF;
  IF to_regprocedure(
    'knowledge_fetch_profiled_query_evidence_candidates(uuid[],text,real[],integer)'
  ) IS NULL OR to_regprocedure(
    'knowledge_fetch_hybrid_query_evidence_candidates(uuid[],text,real[],integer)'
  ) IS NULL OR NOT EXISTS (
    SELECT 1 FROM schema_migrations
    WHERE version = 37 AND name = 'rag_retrieval_profile_pointer'
  ) OR NOT EXISTS (
    SELECT 1 FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1 AND active_profile = 'legacy' AND revision = 3
  ) THEN
    RAISE EXCEPTION 'candidate rollback damaged migration 037 or legacy state';
  END IF;
END
$rollback_contract$;

SET ROLE rag_replay_operator;
DO $unavailable_contract$
BEGIN
  BEGIN
    PERFORM * FROM knowledge_set_retrieval_profile(
      'legacy',
      'pg17_bm25_pgvector_v1',
      3,
      'G18.5B.1 post-rollback unavailable proof'
    );
    RAISE EXCEPTION 'rolled-back PG17 profile remained activatable';
  EXCEPTION WHEN SQLSTATE '55000' THEN
    IF SQLERRM <> 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE' THEN
      RAISE;
    END IF;
  END;
END
$unavailable_contract$;
RESET ROLE;

SELECT 'PASS G18.5B.1 rollback retained migration 037 and legacy reader' AS result;
