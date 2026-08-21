-- Persist the effective Host Agent permission preset independently from
-- browser state. New and historical conversations start at the recommended
-- workspace-write preset; the Host capability boundary still decides whether
-- a preset can be selected or executed on the live machine.

ALTER TABLE conversations
  ADD COLUMN agent_permission_mode TEXT NOT NULL DEFAULT 'workspace-write',
  ADD CONSTRAINT conversations_agent_permission_mode_allowed CHECK (
    agent_permission_mode IN ('read-only', 'workspace-write', 'danger-full-access')
  ) NOT VALID;

ALTER TABLE conversations
  VALIDATE CONSTRAINT conversations_agent_permission_mode_allowed;

GRANT SELECT ON TABLE conversations TO go_api_runtime;
GRANT UPDATE (agent_permission_mode, updated_at) ON TABLE conversations TO go_api_runtime;
