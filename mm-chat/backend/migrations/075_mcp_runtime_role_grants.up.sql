-- Grant the Go API runtime only the MCP table capabilities exercised by the
-- server-authoritative repository and cleanup worker. PostgreSQL migration
-- ownership remains separate from the login role.

GRANT SELECT
  ON TABLE workspaces, workspace_memberships
  TO go_api_runtime;

GRANT SELECT, INSERT, UPDATE, DELETE
  ON TABLE
    mcp_servers,
    mcp_credentials,
    mcp_server_grants,
    mcp_conversation_selections,
    mcp_conversation_servers,
    mcp_workspace_selections,
    mcp_workspace_servers,
    mcp_oauth_states,
    mcp_run_snapshots,
    mcp_tool_calls,
    mcp_tool_results,
    mcp_artifact_cleanup_queue
  TO go_api_runtime;

REVOKE ALL ON FUNCTION mcp_enqueue_account_artifacts() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION mcp_enqueue_account_artifacts() TO go_api_runtime;
