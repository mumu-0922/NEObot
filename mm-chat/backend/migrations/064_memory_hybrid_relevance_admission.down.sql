REVOKE ALL ON FUNCTION memory_authorize_hybrid_rerank(
  UUID, UUID, UUID, TEXT, REAL[]
) FROM go_api_runtime;
DROP FUNCTION memory_authorize_hybrid_rerank(
  UUID, UUID, UUID, TEXT, REAL[]
);
