CREATE INDEX idx_messages_assistant_run_id
  ON messages ((metadata ->> 'runId'))
  WHERE role = 'assistant'
    AND deleted_at IS NULL
    AND metadata ? 'runId';
