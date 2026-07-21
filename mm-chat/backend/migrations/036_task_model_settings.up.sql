CREATE TABLE task_model_settings (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  title_generation TEXT NOT NULL DEFAULT '',
  related_questions TEXT NOT NULL DEFAULT '',
  context_compression TEXT NOT NULL DEFAULT '',
  prompt_optimization TEXT NOT NULL DEFAULT '',
  rag_query TEXT NOT NULL DEFAULT '',
  memory TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT task_model_settings_refs_bounded CHECK (
    char_length(title_generation) <= 512 AND
    char_length(related_questions) <= 512 AND
    char_length(context_compression) <= 512 AND
    char_length(prompt_optimization) <= 512 AND
    char_length(rag_query) <= 512 AND
    char_length(memory) <= 512
  ),
  CONSTRAINT task_model_settings_timestamps_order CHECK (
    updated_at >= created_at
  )
);

GRANT SELECT, INSERT, UPDATE, DELETE ON task_model_settings TO go_api_runtime;
