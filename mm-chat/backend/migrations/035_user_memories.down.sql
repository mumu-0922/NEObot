REVOKE SELECT, INSERT, UPDATE, DELETE ON user_memories FROM go_api_runtime;
REVOKE SELECT, INSERT, UPDATE, DELETE ON user_memory_settings FROM go_api_runtime;

DROP TABLE user_memories;
DROP TABLE user_memory_settings;
