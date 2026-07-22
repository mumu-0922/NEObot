DO $rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile <> 'legacy'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY';
  END IF;
END
$rollback_guard$;

REVOKE EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM rag_worker_executor, go_api_runtime;
REVOKE EXECUTE ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) FROM rag_replay_operator;

DROP FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
);
DROP FUNCTION knowledge_set_retrieval_profile(TEXT, TEXT, BIGINT, TEXT);
DROP TABLE knowledge_retrieval_profile_transitions;
DROP TABLE knowledge_retrieval_profile_head;
