-- Server-authoritative MCP Tools foundation. Shared deployment manifests remain
-- file-authoritative; PostgreSQL owns user servers, credentials, grants,
-- conversation selections, OAuth state, and bounded call/result records.

CREATE TABLE workspaces (
  id UUID PRIMARY KEY,
  owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT workspaces_name_not_blank CHECK (length(trim(name)) > 0),
  CONSTRAINT workspaces_timestamps_order CHECK (updated_at >= created_at),
  CONSTRAINT workspaces_deleted_after_created
    CHECK (deleted_at IS NULL OR deleted_at >= created_at)
);

CREATE TABLE workspace_memberships (
  workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  removed_at TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, user_id),
  CONSTRAINT workspace_memberships_role_check
    CHECK (role IN ('admin', 'member')),
  CONSTRAINT workspace_memberships_status_check
    CHECK (status IN ('active', 'removed')),
  CONSTRAINT workspace_memberships_status_timestamp_check CHECK (
    (status = 'active' AND removed_at IS NULL)
    OR (status = 'removed' AND removed_at IS NOT NULL)
  ),
  CONSTRAINT workspace_memberships_timestamps_order
    CHECK (updated_at >= created_at),
  CONSTRAINT workspace_memberships_removed_after_created
    CHECK (removed_at IS NULL OR removed_at >= created_at)
);

ALTER TABLE conversations
  ADD COLUMN workspace_id UUID REFERENCES workspaces(id) ON DELETE SET NULL;

-- Retired plugin selection must not remain an executable or copyable
-- conversation capability after the hard cut.
UPDATE conversations
SET metadata = metadata - 'activePlugins'
WHERE metadata ? 'activePlugins';

CREATE TABLE mcp_servers (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  endpoint_url TEXT NOT NULL,
  transport TEXT NOT NULL DEFAULT 'streamable_http',
  auth_type TEXT NOT NULL DEFAULT 'none',
  auth_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'draft',
  tool_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
  tool_snapshot_hash TEXT,
  validated_at TIMESTAMPTZ,
  last_error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ,
  CONSTRAINT mcp_servers_name_not_blank CHECK (length(trim(name)) > 0),
  CONSTRAINT mcp_servers_endpoint_not_blank CHECK (length(trim(endpoint_url)) > 0),
  CONSTRAINT mcp_servers_transport_check
    CHECK (transport IN ('streamable_http')),
  CONSTRAINT mcp_servers_auth_type_check
    CHECK (auth_type IN ('none', 'header', 'oauth')),
  CONSTRAINT mcp_servers_auth_config_object
    CHECK (jsonb_typeof(auth_config) = 'object'),
  CONSTRAINT mcp_servers_status_check
    CHECK (status IN ('draft', 'ready', 'needs_auth', 'unavailable', 'disabled')),
  CONSTRAINT mcp_servers_tool_snapshot_array
    CHECK (jsonb_typeof(tool_snapshot) = 'array'),
  CONSTRAINT mcp_servers_snapshot_hash_check CHECK (
    tool_snapshot_hash IS NULL OR tool_snapshot_hash ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT mcp_servers_error_code_not_blank
    CHECK (last_error_code IS NULL OR length(trim(last_error_code)) > 0),
  CONSTRAINT mcp_servers_timestamps_order CHECK (updated_at >= created_at),
  CONSTRAINT mcp_servers_deleted_after_created
    CHECK (deleted_at IS NULL OR deleted_at >= created_at)
);

CREATE TABLE mcp_credentials (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  server_source TEXT NOT NULL,
  server_ref TEXT NOT NULL,
  kind TEXT NOT NULL,
  encrypted_secret_ref TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, server_source, server_ref),
  CONSTRAINT mcp_credentials_source_check
    CHECK (server_source IN ('catalog', 'manifest', 'private')),
  CONSTRAINT mcp_credentials_ref_not_blank
    CHECK (length(trim(server_ref)) > 0),
  CONSTRAINT mcp_credentials_kind_check
    CHECK (kind IN ('header', 'oauth')),
  CONSTRAINT mcp_credentials_secret_not_blank
    CHECK (length(trim(encrypted_secret_ref)) > 0),
  CONSTRAINT mcp_credentials_metadata_object
    CHECK (jsonb_typeof(metadata) = 'object'),
  CONSTRAINT mcp_credentials_timestamps_order CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_server_grants (
  id UUID PRIMARY KEY,
  server_source TEXT NOT NULL,
  server_ref TEXT NOT NULL,
  scope_type TEXT NOT NULL,
  scope_id UUID,
  default_enabled BOOLEAN NOT NULL DEFAULT false,
  created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_server_grants_source_check
    CHECK (server_source IN ('catalog', 'manifest', 'private')),
  CONSTRAINT mcp_server_grants_ref_not_blank
    CHECK (length(trim(server_ref)) > 0),
  CONSTRAINT mcp_server_grants_scope_check
    CHECK (scope_type IN ('global', 'team', 'workspace', 'user')),
  CONSTRAINT mcp_server_grants_scope_shape CHECK (
    (scope_type = 'global' AND scope_id IS NULL)
    OR (scope_type <> 'global' AND scope_id IS NOT NULL)
  ),
  CONSTRAINT mcp_server_grants_unique
    UNIQUE NULLS NOT DISTINCT (server_source, server_ref, scope_type, scope_id),
  CONSTRAINT mcp_server_grants_timestamps_order CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_conversation_selections (
  conversation_id UUID PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mode TEXT NOT NULL DEFAULT 'inherit',
  revision BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_conversation_selections_mode_check
    CHECK (mode IN ('inherit', 'custom')),
  CONSTRAINT mcp_conversation_selections_revision_positive CHECK (revision >= 1),
  CONSTRAINT mcp_conversation_selections_timestamps_order
    CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_conversation_servers (
  conversation_id UUID NOT NULL
    REFERENCES mcp_conversation_selections(conversation_id) ON DELETE CASCADE,
  server_source TEXT NOT NULL,
  server_ref TEXT NOT NULL,
  disabled_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (conversation_id, server_source, server_ref),
  CONSTRAINT mcp_conversation_servers_source_check
    CHECK (server_source IN ('catalog', 'manifest', 'private')),
  CONSTRAINT mcp_conversation_servers_ref_not_blank
    CHECK (length(trim(server_ref)) > 0),
  CONSTRAINT mcp_conversation_servers_disabled_array
    CHECK (jsonb_typeof(disabled_tools) = 'array'),
  CONSTRAINT mcp_conversation_servers_timestamps_order CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_workspace_selections (
  workspace_id UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_workspace_selections_revision_positive CHECK (revision >= 1),
  CONSTRAINT mcp_workspace_selections_timestamps_order
    CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_workspace_servers (
  workspace_id UUID NOT NULL
    REFERENCES mcp_workspace_selections(workspace_id) ON DELETE CASCADE,
  server_source TEXT NOT NULL,
  server_ref TEXT NOT NULL,
  disabled_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (workspace_id, server_source, server_ref),
  CONSTRAINT mcp_workspace_servers_source_check
    CHECK (server_source IN ('catalog', 'manifest', 'private')),
  CONSTRAINT mcp_workspace_servers_ref_not_blank
    CHECK (length(trim(server_ref)) > 0),
  CONSTRAINT mcp_workspace_servers_disabled_array
    CHECK (jsonb_typeof(disabled_tools) = 'array'),
  CONSTRAINT mcp_workspace_servers_timestamps_order CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_oauth_states (
  id UUID PRIMARY KEY,
  state_hash TEXT NOT NULL UNIQUE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  server_source TEXT NOT NULL,
  server_ref TEXT NOT NULL,
  encrypted_flow_ref TEXT NOT NULL,
  return_url TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_oauth_states_hash_check CHECK (state_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT mcp_oauth_states_source_check
    CHECK (server_source IN ('catalog', 'manifest', 'private')),
  CONSTRAINT mcp_oauth_states_ref_not_blank
    CHECK (length(trim(server_ref)) > 0),
  CONSTRAINT mcp_oauth_states_flow_not_blank
    CHECK (length(trim(encrypted_flow_ref)) > 0),
  CONSTRAINT mcp_oauth_states_return_not_blank CHECK (length(trim(return_url)) > 0),
  CONSTRAINT mcp_oauth_states_expiry CHECK (expires_at > created_at),
  CONSTRAINT mcp_oauth_states_consumed_after_created
    CHECK (consumed_at IS NULL OR consumed_at >= created_at),
  CONSTRAINT mcp_oauth_states_timestamps_order CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_run_snapshots (
  run_id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
  selection_revision BIGINT NOT NULL,
  snapshot_hash TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  CONSTRAINT mcp_run_snapshots_revision_positive CHECK (selection_revision >= 1),
  CONSTRAINT mcp_run_snapshots_hash_check CHECK (snapshot_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT mcp_run_snapshots_snapshot_object CHECK (jsonb_typeof(snapshot) = 'object'),
  CONSTRAINT mcp_run_snapshots_expiry CHECK (expires_at > created_at)
);

CREATE TABLE mcp_tool_calls (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  message_id UUID REFERENCES messages(id) ON DELETE CASCADE,
  run_id UUID NOT NULL,
  server_source TEXT NOT NULL,
  server_ref TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  tool_alias TEXT NOT NULL,
  classification TEXT NOT NULL DEFAULT 'unknown',
  status TEXT NOT NULL DEFAULT 'queued',
  round_no INTEGER NOT NULL,
  call_no INTEGER NOT NULL,
  arguments_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  result_summary TEXT,
  error_code TEXT,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  duration_ms BIGINT,
  retained_until TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_tool_calls_source_check
    CHECK (server_source IN ('catalog', 'manifest', 'private')),
  CONSTRAINT mcp_tool_calls_ref_not_blank CHECK (length(trim(server_ref)) > 0),
  CONSTRAINT mcp_tool_calls_tool_not_blank CHECK (length(trim(tool_name)) > 0),
  CONSTRAINT mcp_tool_calls_alias_not_blank CHECK (length(trim(tool_alias)) > 0),
  CONSTRAINT mcp_tool_calls_classification_check
    CHECK (classification IN ('read', 'write', 'unknown')),
  CONSTRAINT mcp_tool_calls_status_check CHECK (
    status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'outcome_unknown')
  ),
  CONSTRAINT mcp_tool_calls_round_positive CHECK (round_no >= 1),
  CONSTRAINT mcp_tool_calls_call_positive CHECK (call_no >= 1),
  CONSTRAINT mcp_tool_calls_arguments_object
    CHECK (jsonb_typeof(arguments_summary) = 'object'),
  CONSTRAINT mcp_tool_calls_duration_non_negative
    CHECK (duration_ms IS NULL OR duration_ms >= 0),
  CONSTRAINT mcp_tool_calls_completed_after_started CHECK (
    completed_at IS NULL OR started_at IS NULL OR completed_at >= started_at
  ),
  CONSTRAINT mcp_tool_calls_timestamps_order CHECK (updated_at >= created_at)
);

CREATE TABLE mcp_tool_results (
  call_id UUID PRIMARY KEY REFERENCES mcp_tool_calls(id) ON DELETE CASCADE,
  content JSONB NOT NULL DEFAULT '[]'::jsonb,
  object_keys JSONB NOT NULL DEFAULT '[]'::jsonb,
  byte_size BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_tool_results_content_array CHECK (jsonb_typeof(content) = 'array'),
  CONSTRAINT mcp_tool_results_keys_array CHECK (jsonb_typeof(object_keys) = 'array'),
  CONSTRAINT mcp_tool_results_size_non_negative CHECK (byte_size >= 0),
  CONSTRAINT mcp_tool_results_timestamps_order CHECK (updated_at >= created_at)
);

-- Account rows may be deleted directly by identity administration. Preserve
-- only server-generated object coordinates outside the user FK cascade so the
-- cleanup-only worker can remove MinIO bytes after PostgreSQL completes.
CREATE TABLE mcp_artifact_cleanup_queue (
  object_key TEXT PRIMARY KEY,
  conversation_id UUID NOT NULL,
  call_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_artifact_cleanup_key_not_blank CHECK (length(trim(object_key)) > 0)
);

CREATE FUNCTION mcp_enqueue_account_artifacts()
RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $$
BEGIN
  INSERT INTO public.mcp_artifact_cleanup_queue (
    object_key, conversation_id, call_id
  )
  SELECT object_key.value, call.conversation_id, call.id
  FROM public.mcp_tool_calls call
  JOIN public.mcp_tool_results result ON result.call_id = call.id
  CROSS JOIN LATERAL jsonb_array_elements_text(result.object_keys) AS object_key(value)
  WHERE call.user_id = OLD.id
  ON CONFLICT (object_key) DO NOTHING;
  RETURN OLD;
END;
$$;

CREATE TRIGGER trg_mcp_enqueue_account_artifacts
BEFORE DELETE ON users
FOR EACH ROW
EXECUTE FUNCTION mcp_enqueue_account_artifacts();

CREATE INDEX idx_workspaces_owner_active
  ON workspaces(owner_user_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_workspaces_team_active
  ON workspaces(team_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_workspace_memberships_user_active
  ON workspace_memberships(user_id, workspace_id) WHERE status = 'active';
CREATE INDEX idx_conversations_workspace
  ON conversations(workspace_id) WHERE workspace_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_mcp_servers_user_active
  ON mcp_servers(user_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_mcp_servers_user_endpoint_active
  ON mcp_servers(user_id, endpoint_url) WHERE deleted_at IS NULL;
CREATE INDEX idx_mcp_credentials_server
  ON mcp_credentials(server_source, server_ref, user_id);
CREATE INDEX idx_mcp_grants_scope
  ON mcp_server_grants(scope_type, scope_id, server_source, server_ref);
CREATE INDEX idx_mcp_selections_user ON mcp_conversation_selections(user_id, updated_at DESC);
CREATE INDEX idx_mcp_workspace_servers_ref
  ON mcp_workspace_servers(server_source, server_ref, workspace_id);
CREATE INDEX idx_mcp_oauth_states_expiry
  ON mcp_oauth_states(expires_at) WHERE consumed_at IS NULL;
CREATE INDEX idx_mcp_run_snapshots_conversation
  ON mcp_run_snapshots(conversation_id, created_at DESC);
CREATE INDEX idx_mcp_tool_calls_run
  ON mcp_tool_calls(run_id, call_no);
CREATE INDEX idx_mcp_tool_calls_conversation
  ON mcp_tool_calls(conversation_id, created_at DESC);
CREATE INDEX idx_mcp_tool_calls_user_created
  ON mcp_tool_calls(user_id, created_at DESC);
CREATE INDEX idx_mcp_tool_calls_retention
  ON mcp_tool_calls(retained_until, id);
CREATE INDEX idx_mcp_artifact_cleanup_created
  ON mcp_artifact_cleanup_queue(created_at, object_key);
