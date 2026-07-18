REVOKE EXECUTE ON FUNCTION knowledge_rollback_index_generation(
  UUID,UUID,BIGINT,TEXT,TEXT
) FROM go_api_runtime;
DROP FUNCTION knowledge_rollback_index_generation(UUID,UUID,BIGINT,TEXT,TEXT);

REVOKE EXECUTE ON FUNCTION knowledge_promote_index_generation(UUID,BIGINT,TEXT)
  FROM go_api_runtime;
