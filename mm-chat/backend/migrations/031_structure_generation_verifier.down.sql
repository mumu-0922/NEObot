REVOKE EXECUTE ON FUNCTION knowledge_verify_structure_generation(UUID,BIGINT,TEXT)
  FROM go_api_runtime;
DROP FUNCTION knowledge_verify_structure_generation(UUID,BIGINT,TEXT);
REVOKE UPDATE(status, verified_at)
  ON knowledge_parser_artifact_sets
FROM rag_projection_owner;
