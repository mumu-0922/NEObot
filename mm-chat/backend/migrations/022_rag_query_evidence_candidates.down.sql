-- Revert G7.6A selected-collection evidence candidate fetch.

REVOKE EXECUTE ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) FROM rag_worker_executor;
DROP FUNCTION IF EXISTS knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
);
