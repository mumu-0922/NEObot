CREATE TABLE projects (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  lifecycle_status TEXT NOT NULL DEFAULT 'active',
  revision BIGINT NOT NULL DEFAULT 1,
  scope_generation BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  CONSTRAINT projects_id_user_unique UNIQUE (id, user_id),
  CONSTRAINT projects_name_bounded CHECK (
    length(trim(name)) > 0 AND char_length(name) <= 200
  ),
  CONSTRAINT projects_description_bounded CHECK (char_length(description) <= 4000),
  CONSTRAINT projects_lifecycle_status_allowed CHECK (
    lifecycle_status IN ('active', 'archived')
  ),
  CONSTRAINT projects_revision_positive CHECK (revision >= 1),
  CONSTRAINT projects_scope_generation_positive CHECK (scope_generation >= 1),
  CONSTRAINT projects_timestamps_order CHECK (updated_at >= created_at),
  CONSTRAINT projects_archived_at_order CHECK (
    archived_at IS NULL OR archived_at >= created_at
  ),
  CONSTRAINT projects_deleted_at_order CHECK (
    deleted_at IS NULL OR deleted_at >= created_at
  )
);

CREATE INDEX idx_projects_user_active_updated
  ON projects(user_id, updated_at DESC, id)
  WHERE deleted_at IS NULL AND lifecycle_status = 'active';

CREATE INDEX idx_projects_user_lifecycle_updated
  ON projects(user_id, lifecycle_status, updated_at DESC, id)
  WHERE deleted_at IS NULL;

ALTER TABLE conversations
  ADD COLUMN project_id UUID,
  ADD COLUMN memory_scope_generation BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN memory_use_mode TEXT NOT NULL DEFAULT 'inherit',
  ADD COLUMN memory_learn_mode TEXT NOT NULL DEFAULT 'inherit',
  ADD CONSTRAINT conversations_id_user_unique UNIQUE (id, user_id),
  ADD CONSTRAINT conversations_memory_scope_generation_positive
    CHECK (memory_scope_generation >= 1) NOT VALID,
  ADD CONSTRAINT conversations_memory_use_mode_allowed
    CHECK (memory_use_mode IN ('inherit', 'on', 'off')) NOT VALID,
  ADD CONSTRAINT conversations_memory_learn_mode_allowed
    CHECK (memory_learn_mode IN ('inherit', 'on', 'off')) NOT VALID,
  ADD CONSTRAINT conversations_project_owner_fk
    FOREIGN KEY (project_id, user_id)
    REFERENCES projects(id, user_id)
    ON DELETE RESTRICT
    NOT VALID;

CREATE INDEX idx_conversations_project_updated
  ON conversations(user_id, project_id, updated_at DESC, id)
  WHERE project_id IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE user_memory_settings
  ADD COLUMN sensitive_memory_enabled BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN l2_mode TEXT NOT NULL DEFAULT 'inherit',
  ADD COLUMN l3_mode TEXT NOT NULL DEFAULT 'inherit',
  ADD CONSTRAINT user_memory_settings_l2_mode_allowed
    CHECK (l2_mode IN ('inherit', 'on', 'off')) NOT VALID,
  ADD CONSTRAINT user_memory_settings_l3_mode_allowed
    CHECK (l3_mode IN ('inherit', 'on', 'off')) NOT VALID;

ALTER TABLE user_memories
  ADD COLUMN scope_type TEXT,
  ADD COLUMN project_id UUID,
  ADD COLUMN scope_conversation_id UUID,
  ADD COLUMN scope_generation BIGINT;

UPDATE user_memories
SET
  scope_type = 'global',
  project_id = NULL,
  scope_conversation_id = NULL,
  scope_generation = 1
WHERE scope_type IS NULL OR scope_generation IS NULL;

ALTER TABLE user_memories
  ALTER COLUMN scope_type SET DEFAULT 'global',
  ALTER COLUMN scope_type SET NOT NULL,
  ALTER COLUMN scope_generation SET DEFAULT 1,
  ALTER COLUMN scope_generation SET NOT NULL,
  ADD CONSTRAINT user_memories_scope_type_allowed
    CHECK (scope_type IN ('global', 'project', 'conversation')) NOT VALID,
  ADD CONSTRAINT user_memories_scope_shape_check CHECK (
    (
      scope_type = 'global'
      AND project_id IS NULL
      AND scope_conversation_id IS NULL
    )
    OR (
      scope_type = 'project'
      AND project_id IS NOT NULL
      AND scope_conversation_id IS NULL
    )
    OR (
      scope_type = 'conversation'
      AND project_id IS NULL
      AND scope_conversation_id IS NOT NULL
    )
  ) NOT VALID,
  ADD CONSTRAINT user_memories_scope_generation_positive
    CHECK (scope_generation >= 1) NOT VALID,
  ADD CONSTRAINT user_memories_project_owner_fk
    FOREIGN KEY (project_id, user_id)
    REFERENCES projects(id, user_id)
    ON DELETE RESTRICT
    NOT VALID,
  ADD CONSTRAINT user_memories_scope_conversation_owner_fk
    FOREIGN KEY (scope_conversation_id, user_id)
    REFERENCES conversations(id, user_id)
    ON DELETE RESTRICT
    NOT VALID;

DROP INDEX idx_user_memories_active_content;

CREATE UNIQUE INDEX idx_user_memories_active_global_content
  ON user_memories(user_id, normalized_content)
  WHERE deleted_at IS NULL AND scope_type = 'global';

CREATE UNIQUE INDEX idx_user_memories_active_project_content
  ON user_memories(user_id, project_id, normalized_content)
  WHERE deleted_at IS NULL AND scope_type = 'project';

CREATE UNIQUE INDEX idx_user_memories_active_conversation_content
  ON user_memories(user_id, scope_conversation_id, normalized_content)
  WHERE deleted_at IS NULL AND scope_type = 'conversation';

ALTER TABLE conversations
  VALIDATE CONSTRAINT conversations_memory_scope_generation_positive,
  VALIDATE CONSTRAINT conversations_memory_use_mode_allowed,
  VALIDATE CONSTRAINT conversations_memory_learn_mode_allowed,
  VALIDATE CONSTRAINT conversations_project_owner_fk;

ALTER TABLE user_memory_settings
  VALIDATE CONSTRAINT user_memory_settings_l2_mode_allowed,
  VALIDATE CONSTRAINT user_memory_settings_l3_mode_allowed;

ALTER TABLE user_memories
  VALIDATE CONSTRAINT user_memories_scope_type_allowed,
  VALIDATE CONSTRAINT user_memories_scope_shape_check,
  VALIDATE CONSTRAINT user_memories_scope_generation_positive,
  VALIDATE CONSTRAINT user_memories_project_owner_fk,
  VALIDATE CONSTRAINT user_memories_scope_conversation_owner_fk;
