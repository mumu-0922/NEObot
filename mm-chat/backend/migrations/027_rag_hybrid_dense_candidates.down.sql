REVOKE EXECUTE ON FUNCTION knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM rag_worker_executor;
DROP FUNCTION IF EXISTS knowledge_fetch_hybrid_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
);
