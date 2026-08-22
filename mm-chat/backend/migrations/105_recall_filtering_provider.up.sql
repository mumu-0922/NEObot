ALTER TABLE task_model_settings
  DROP CONSTRAINT task_model_settings_refs_bounded;

ALTER TABLE task_model_settings
  ADD COLUMN recall_filtering TEXT NOT NULL DEFAULT '';

ALTER TABLE task_model_settings
  ADD CONSTRAINT task_model_settings_refs_bounded CHECK (
    char_length(title_generation) <= 512 AND
    char_length(related_questions) <= 512 AND
    char_length(context_compression) <= 512 AND
    char_length(prompt_optimization) <= 512 AND
    char_length(rag_query) <= 512 AND
    char_length(memory) <= 512 AND
    char_length(recall_filtering) <= 512
  );
