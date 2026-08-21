-- Do not erase an explicit non-default permission choice during rollback.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM conversations
    WHERE agent_permission_mode <> 'workspace-write'
  ) THEN
    RAISE EXCEPTION 'cannot remove durable Agent permission selections';
  END IF;
END
$$;

REVOKE UPDATE (agent_permission_mode) ON TABLE conversations FROM go_api_runtime;

ALTER TABLE conversations
  DROP CONSTRAINT IF EXISTS conversations_agent_permission_mode_allowed,
  DROP COLUMN IF EXISTS agent_permission_mode;
