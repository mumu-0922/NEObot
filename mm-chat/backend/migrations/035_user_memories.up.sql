CREATE TABLE user_memory_settings (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  enabled BOOLEAN NOT NULL DEFAULT false,
  search_enabled BOOLEAN NOT NULL DEFAULT true,
  auto_record_enabled BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_memory_settings_timestamps_order CHECK (updated_at >= created_at)
);

CREATE TABLE user_memories (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  memory_type TEXT NOT NULL,
  content TEXT NOT NULL,
  normalized_content TEXT NOT NULL,
  importance SMALLINT NOT NULL DEFAULT 3,
  tags TEXT[] NOT NULL DEFAULT '{}',
  source TEXT NOT NULL,
  source_conversation_id UUID REFERENCES conversations(id) ON DELETE SET NULL,
  source_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT user_memories_type_allowed CHECK (
    memory_type IN ('fact', 'preference', 'instruction', 'project', 'warning', 'decision', 'context')
  ),
  CONSTRAINT user_memories_source_allowed CHECK (source IN ('manual', 'ai')),
  CONSTRAINT user_memories_content_bounded CHECK (
    length(trim(content)) > 0 AND char_length(content) <= 2000
  ),
  CONSTRAINT user_memories_normalized_content_bounded CHECK (
    length(trim(normalized_content)) > 0 AND char_length(normalized_content) <= 2000
  ),
  CONSTRAINT user_memories_importance_range CHECK (importance BETWEEN 1 AND 5),
  CONSTRAINT user_memories_tag_count_bounded CHECK (cardinality(tags) <= 12),
  CONSTRAINT user_memories_timestamps_order CHECK (updated_at >= created_at),
  CONSTRAINT user_memories_deleted_at_order CHECK (
    deleted_at IS NULL OR deleted_at >= created_at
  )
);

CREATE UNIQUE INDEX idx_user_memories_active_content
  ON user_memories(user_id, normalized_content)
  WHERE deleted_at IS NULL;

CREATE INDEX idx_user_memories_active_user_updated
  ON user_memories(user_id, updated_at DESC)
  WHERE deleted_at IS NULL AND enabled;

GRANT SELECT, INSERT, UPDATE, DELETE ON user_memory_settings TO go_api_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON user_memories TO go_api_runtime;
