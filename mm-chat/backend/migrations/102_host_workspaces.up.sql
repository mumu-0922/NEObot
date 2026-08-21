-- Upgrade the existing MCP-visible workspace registry in place so the same
-- durable identity can preserve browser Workspace settings and later bind one
-- canonical Host directory. Existing rows remain valid and unbound.

ALTER TABLE workspaces
  ADD COLUMN system_prompt TEXT NOT NULL DEFAULT '',
  ADD COLUMN files JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN color TEXT,
  ADD COLUMN enable_search BOOLEAN,
  ADD COLUMN enable_reasoning BOOLEAN,
  ADD COLUMN revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN runner_id TEXT,
  ADD COLUMN canonical_path TEXT,
  ADD COLUMN display_path TEXT,
  ADD COLUMN path_kind TEXT,
  ADD COLUMN directory_fingerprint TEXT,
  ADD COLUMN bound_at TIMESTAMPTZ,
  ADD COLUMN legacy_imported_at TIMESTAMPTZ,
  ADD CONSTRAINT workspaces_id_owner_unique UNIQUE (id, owner_user_id),
  ADD CONSTRAINT workspaces_name_bounded CHECK (
    length(btrim(name)) > 0 AND char_length(name) <= 200
  ) NOT VALID,
  ADD CONSTRAINT workspaces_system_prompt_bounded CHECK (
    octet_length(system_prompt) <= 262144
  ) NOT VALID,
  ADD CONSTRAINT workspaces_files_array_bounded CHECK (
    jsonb_typeof(files) = 'array' AND octet_length(files::text) <= 1048576
  ) NOT VALID,
  ADD CONSTRAINT workspaces_color_bounded CHECK (
    color IS NULL OR (length(btrim(color)) > 0 AND char_length(color) <= 64)
  ) NOT VALID,
  ADD CONSTRAINT workspaces_revision_positive CHECK (revision >= 1) NOT VALID,
  ADD CONSTRAINT workspaces_runner_id_shape CHECK (
    runner_id IS NULL OR runner_id ~ '^[a-z][a-z0-9-]{2,63}$'
  ) NOT VALID,
  ADD CONSTRAINT workspaces_path_kind_allowed CHECK (
    path_kind IS NULL OR path_kind IN ('wsl', 'windows-mounted')
  ) NOT VALID,
  ADD CONSTRAINT workspaces_directory_fingerprint_shape CHECK (
    directory_fingerprint IS NULL OR directory_fingerprint ~ '^sha256:[0-9a-f]{64}$'
  ) NOT VALID,
  ADD CONSTRAINT workspaces_host_binding_shape CHECK (
    (
      runner_id IS NULL AND canonical_path IS NULL AND display_path IS NULL
      AND path_kind IS NULL AND directory_fingerprint IS NULL AND bound_at IS NULL
    ) OR (
      runner_id IS NOT NULL AND canonical_path IS NOT NULL AND display_path IS NOT NULL
      AND path_kind IS NOT NULL AND directory_fingerprint IS NOT NULL AND bound_at IS NOT NULL
      AND canonical_path LIKE '/%' AND length(canonical_path) <= 4096
      AND length(display_path) BETWEEN 1 AND 4096
    )
  ) NOT VALID;

CREATE UNIQUE INDEX idx_workspaces_owner_runner_directory_active
  ON workspaces(owner_user_id, runner_id, directory_fingerprint)
  WHERE deleted_at IS NULL AND runner_id IS NOT NULL;

ALTER TABLE conversations
  ADD COLUMN agent_workspace_id UUID,
  ADD COLUMN agent_workspace_runner_id TEXT,
  ADD COLUMN agent_workspace_canonical_path TEXT,
  ADD COLUMN agent_workspace_fingerprint TEXT,
  ADD COLUMN agent_workspace_bound_at TIMESTAMPTZ,
  ADD CONSTRAINT conversations_agent_workspace_owner_fk
    FOREIGN KEY (agent_workspace_id, user_id)
    REFERENCES workspaces(id, owner_user_id)
    ON DELETE RESTRICT
    NOT VALID,
  ADD CONSTRAINT conversations_agent_workspace_shape CHECK (
    (
      agent_workspace_id IS NULL AND agent_workspace_runner_id IS NULL
      AND agent_workspace_canonical_path IS NULL AND agent_workspace_fingerprint IS NULL
      AND agent_workspace_bound_at IS NULL
    ) OR (
      agent_workspace_id IS NOT NULL AND agent_workspace_runner_id IS NOT NULL
      AND agent_workspace_canonical_path IS NOT NULL AND agent_workspace_fingerprint IS NOT NULL
      AND agent_workspace_bound_at IS NOT NULL
      AND agent_workspace_runner_id ~ '^[a-z][a-z0-9-]{2,63}$'
      AND agent_workspace_canonical_path LIKE '/%'
      AND length(agent_workspace_canonical_path) <= 4096
      AND agent_workspace_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    )
  ) NOT VALID;

CREATE INDEX idx_conversations_agent_workspace
  ON conversations(user_id, agent_workspace_id, updated_at DESC)
  WHERE agent_workspace_id IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE workspaces
  VALIDATE CONSTRAINT workspaces_name_bounded,
  VALIDATE CONSTRAINT workspaces_system_prompt_bounded,
  VALIDATE CONSTRAINT workspaces_files_array_bounded,
  VALIDATE CONSTRAINT workspaces_color_bounded,
  VALIDATE CONSTRAINT workspaces_revision_positive,
  VALIDATE CONSTRAINT workspaces_runner_id_shape,
  VALIDATE CONSTRAINT workspaces_path_kind_allowed,
  VALIDATE CONSTRAINT workspaces_directory_fingerprint_shape,
  VALIDATE CONSTRAINT workspaces_host_binding_shape;

ALTER TABLE conversations
  VALIDATE CONSTRAINT conversations_agent_workspace_owner_fk,
  VALIDATE CONSTRAINT conversations_agent_workspace_shape;

GRANT SELECT ON TABLE workspaces TO go_api_runtime;
GRANT INSERT (
  id,
  owner_user_id,
  name,
  system_prompt,
  files,
  color,
  enable_search,
  enable_reasoning,
  legacy_imported_at,
  created_at,
  updated_at
) ON TABLE workspaces TO go_api_runtime;
GRANT UPDATE (
  name,
  system_prompt,
  files,
  color,
  enable_search,
  enable_reasoning,
  revision,
  runner_id,
  canonical_path,
  display_path,
  path_kind,
  directory_fingerprint,
  bound_at,
  legacy_imported_at,
  updated_at,
  deleted_at
) ON TABLE workspaces TO go_api_runtime;
GRANT SELECT, UPDATE (
  workspace_id,
  agent_workspace_id,
  agent_workspace_runner_id,
  agent_workspace_canonical_path,
  agent_workspace_fingerprint,
  agent_workspace_bound_at,
  updated_at
) ON TABLE conversations TO go_api_runtime;
REVOKE DELETE ON TABLE workspaces FROM go_api_runtime;
