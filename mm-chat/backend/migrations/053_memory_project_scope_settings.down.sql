DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM projects) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_V2_ROLLBACK_REQUIRES_EMPTY_PROJECTS';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM user_memories
    WHERE scope_type <> 'global'
       OR project_id IS NOT NULL
       OR scope_conversation_id IS NOT NULL
       OR scope_generation <> 1
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_V2_ROLLBACK_REQUIRES_GLOBAL_MEMORY_ONLY';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM conversations
    WHERE project_id IS NOT NULL
       OR memory_scope_generation <> 1
       OR memory_use_mode <> 'inherit'
       OR memory_learn_mode <> 'inherit'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_V2_ROLLBACK_REQUIRES_INHERITED_CONVERSATION_POLICY';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM user_memory_settings
    WHERE sensitive_memory_enabled
       OR l2_mode <> 'inherit'
       OR l3_mode <> 'inherit'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'MEMORY_V2_ROLLBACK_REQUIRES_DEFAULT_MEMORY_SETTINGS';
  END IF;
END
$$;

DROP INDEX IF EXISTS idx_user_memories_active_conversation_content;
DROP INDEX IF EXISTS idx_user_memories_active_project_content;
DROP INDEX IF EXISTS idx_user_memories_active_global_content;

CREATE UNIQUE INDEX idx_user_memories_active_content
  ON user_memories(user_id, normalized_content)
  WHERE deleted_at IS NULL;

ALTER TABLE user_memories
  DROP CONSTRAINT IF EXISTS user_memories_scope_conversation_owner_fk,
  DROP CONSTRAINT IF EXISTS user_memories_project_owner_fk,
  DROP CONSTRAINT IF EXISTS user_memories_scope_generation_positive,
  DROP CONSTRAINT IF EXISTS user_memories_scope_shape_check,
  DROP CONSTRAINT IF EXISTS user_memories_scope_type_allowed,
  DROP COLUMN IF EXISTS scope_generation,
  DROP COLUMN IF EXISTS scope_conversation_id,
  DROP COLUMN IF EXISTS project_id,
  DROP COLUMN IF EXISTS scope_type;

ALTER TABLE user_memory_settings
  DROP CONSTRAINT IF EXISTS user_memory_settings_l3_mode_allowed,
  DROP CONSTRAINT IF EXISTS user_memory_settings_l2_mode_allowed,
  DROP COLUMN IF EXISTS l3_mode,
  DROP COLUMN IF EXISTS l2_mode,
  DROP COLUMN IF EXISTS sensitive_memory_enabled;

DROP INDEX IF EXISTS idx_conversations_project_updated;

ALTER TABLE conversations
  DROP CONSTRAINT IF EXISTS conversations_project_owner_fk,
  DROP CONSTRAINT IF EXISTS conversations_memory_learn_mode_allowed,
  DROP CONSTRAINT IF EXISTS conversations_memory_use_mode_allowed,
  DROP CONSTRAINT IF EXISTS conversations_memory_scope_generation_positive,
  DROP COLUMN IF EXISTS memory_learn_mode,
  DROP COLUMN IF EXISTS memory_use_mode,
  DROP COLUMN IF EXISTS memory_scope_generation,
  DROP COLUMN IF EXISTS project_id,
  DROP CONSTRAINT IF EXISTS conversations_id_user_unique;

DROP INDEX IF EXISTS idx_projects_user_lifecycle_updated;
DROP INDEX IF EXISTS idx_projects_user_active_updated;
DROP TABLE projects;
