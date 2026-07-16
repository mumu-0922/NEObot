-- Revert G7.8 Go API evidence-candidate execution grant.

REVOKE EXECUTE ON FUNCTION knowledge_fetch_query_evidence_candidates(
  UUID[], TEXT, INTEGER
) FROM go_api_runtime;
