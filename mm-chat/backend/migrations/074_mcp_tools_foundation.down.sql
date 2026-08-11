DROP INDEX IF EXISTS idx_mcp_artifact_cleanup_created;
DROP INDEX IF EXISTS idx_mcp_tool_calls_retention;
DROP INDEX IF EXISTS idx_mcp_tool_calls_user_created;
DROP INDEX IF EXISTS idx_mcp_tool_calls_conversation;
DROP INDEX IF EXISTS idx_mcp_tool_calls_run;
DROP INDEX IF EXISTS idx_mcp_oauth_states_expiry;
DROP INDEX IF EXISTS idx_mcp_run_snapshots_conversation;
DROP INDEX IF EXISTS idx_mcp_workspace_servers_ref;
DROP INDEX IF EXISTS idx_mcp_selections_user;
DROP INDEX IF EXISTS idx_mcp_grants_scope;
DROP INDEX IF EXISTS idx_mcp_credentials_server;
DROP INDEX IF EXISTS idx_mcp_servers_user_endpoint_active;
DROP INDEX IF EXISTS idx_mcp_servers_user_active;
DROP INDEX IF EXISTS idx_conversations_workspace;
DROP INDEX IF EXISTS idx_workspace_memberships_user_active;
DROP INDEX IF EXISTS idx_workspaces_team_active;
DROP INDEX IF EXISTS idx_workspaces_owner_active;

DROP TRIGGER IF EXISTS trg_mcp_enqueue_account_artifacts ON users;
DROP FUNCTION IF EXISTS mcp_enqueue_account_artifacts();

DROP TABLE IF EXISTS mcp_artifact_cleanup_queue;
DROP TABLE IF EXISTS mcp_tool_results;
DROP TABLE IF EXISTS mcp_tool_calls;
DROP TABLE IF EXISTS mcp_run_snapshots;
DROP TABLE IF EXISTS mcp_oauth_states;
DROP TABLE IF EXISTS mcp_workspace_servers;
DROP TABLE IF EXISTS mcp_workspace_selections;
DROP TABLE IF EXISTS mcp_conversation_servers;
DROP TABLE IF EXISTS mcp_conversation_selections;
DROP TABLE IF EXISTS mcp_server_grants;
DROP TABLE IF EXISTS mcp_credentials;
DROP TABLE IF EXISTS mcp_servers;

ALTER TABLE conversations DROP COLUMN IF EXISTS workspace_id;
DROP TABLE IF EXISTS workspace_memberships;
DROP TABLE IF EXISTS workspaces;
