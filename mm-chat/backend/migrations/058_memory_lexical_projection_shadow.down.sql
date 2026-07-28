-- PR7 rollback removes only rebuildable lexical projection. Shadow comparison
-- evidence is guarded and the active reader must remain v1/NULL.

DO $memory_lexical_rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1 FROM user_memory_state
    WHERE active_retrieval_profile_id IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_LEXICAL_ROLLBACK_REQUIRES_V1_READER';
  END IF;
  IF EXISTS (SELECT 1 FROM message_memory_lexical_shadow_observations) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_LEXICAL_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS';
  END IF;
END
$memory_lexical_rollback_guard$;

REVOKE EXECUTE ON FUNCTION memory_compare_lexical_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, INTEGER
) FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION knowledge_normalize_bm25_shadow_terms(TEXT[])
  FROM memory_runtime_owner;
REVOKE EXECUTE ON FUNCTION knowledge_bm25_shadow_query_terms(TEXT)
  FROM memory_runtime_owner;
REVOKE EXECUTE ON FUNCTION knowledge_build_bm25_shadow_text(TEXT, TEXT[])
  FROM memory_runtime_owner;
DROP FUNCTION memory_compare_lexical_shadow(
  UUID, UUID, UUID, UUID, TEXT, TEXT, JSONB, INTEGER
);

DROP TRIGGER conversations_lexical_projection_update ON conversations;
DROP FUNCTION memory_refresh_conversation_lexical_projections();
DROP TRIGGER projects_lexical_projection_update ON projects;
DROP FUNCTION memory_refresh_project_lexical_projections();
DROP TRIGGER user_memory_state_lexical_projection_update ON user_memory_state;
DROP FUNCTION memory_refresh_user_lexical_projections();
DROP TRIGGER user_memories_lexical_projection_update ON user_memories;
DROP TRIGGER user_memories_lexical_projection_insert ON user_memories;
DROP FUNCTION memory_maintain_lexical_projection();
DROP FUNCTION memory_refresh_lexical_projection(UUID);

DROP TABLE message_memory_lexical_shadow_results;
DROP TABLE message_memory_lexical_shadow_observations;
DROP TABLE user_memory_search_projections;
