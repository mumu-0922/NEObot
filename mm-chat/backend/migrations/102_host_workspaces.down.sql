DO $guard$
BEGIN
  IF EXISTS (
    SELECT 1 FROM workspaces
    WHERE system_prompt <> '' OR files <> '[]'::jsonb OR color IS NOT NULL
      OR enable_search IS NOT NULL OR enable_reasoning IS NOT NULL
      OR runner_id IS NOT NULL OR legacy_imported_at IS NOT NULL
  ) OR EXISTS (
    SELECT 1 FROM conversations WHERE agent_workspace_id IS NOT NULL
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'HOST_WORKSPACE_ROLLBACK_BLOCKED';
  END IF;
END
$guard$;

REVOKE INSERT (
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
) ON TABLE workspaces FROM go_api_runtime;
REVOKE UPDATE (
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
) ON TABLE workspaces FROM go_api_runtime;
REVOKE UPDATE (
  workspace_id,
  agent_workspace_id,
  agent_workspace_runner_id,
  agent_workspace_canonical_path,
  agent_workspace_fingerprint,
  agent_workspace_bound_at,
  updated_at
) ON TABLE conversations FROM go_api_runtime;

DROP INDEX IF EXISTS idx_conversations_agent_workspace;
ALTER TABLE conversations
  DROP CONSTRAINT IF EXISTS conversations_agent_workspace_shape,
  DROP CONSTRAINT IF EXISTS conversations_agent_workspace_owner_fk,
  DROP COLUMN IF EXISTS agent_workspace_bound_at,
  DROP COLUMN IF EXISTS agent_workspace_fingerprint,
  DROP COLUMN IF EXISTS agent_workspace_canonical_path,
  DROP COLUMN IF EXISTS agent_workspace_runner_id,
  DROP COLUMN IF EXISTS agent_workspace_id;

DROP INDEX IF EXISTS idx_workspaces_owner_runner_directory_active;
ALTER TABLE workspaces
  DROP CONSTRAINT IF EXISTS workspaces_host_binding_shape,
  DROP CONSTRAINT IF EXISTS workspaces_directory_fingerprint_shape,
  DROP CONSTRAINT IF EXISTS workspaces_path_kind_allowed,
  DROP CONSTRAINT IF EXISTS workspaces_runner_id_shape,
  DROP CONSTRAINT IF EXISTS workspaces_revision_positive,
  DROP CONSTRAINT IF EXISTS workspaces_color_bounded,
  DROP CONSTRAINT IF EXISTS workspaces_files_array_bounded,
  DROP CONSTRAINT IF EXISTS workspaces_system_prompt_bounded,
  DROP CONSTRAINT IF EXISTS workspaces_name_bounded,
  DROP CONSTRAINT IF EXISTS workspaces_id_owner_unique,
  DROP COLUMN IF EXISTS legacy_imported_at,
  DROP COLUMN IF EXISTS bound_at,
  DROP COLUMN IF EXISTS directory_fingerprint,
  DROP COLUMN IF EXISTS path_kind,
  DROP COLUMN IF EXISTS display_path,
  DROP COLUMN IF EXISTS canonical_path,
  DROP COLUMN IF EXISTS runner_id,
  DROP COLUMN IF EXISTS revision,
  DROP COLUMN IF EXISTS enable_reasoning,
  DROP COLUMN IF EXISTS enable_search,
  DROP COLUMN IF EXISTS color,
  DROP COLUMN IF EXISTS files,
  DROP COLUMN IF EXISTS system_prompt;
