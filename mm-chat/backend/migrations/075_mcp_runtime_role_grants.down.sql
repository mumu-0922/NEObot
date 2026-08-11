REVOKE EXECUTE ON FUNCTION mcp_enqueue_account_artifacts() FROM go_api_runtime;
GRANT EXECUTE ON FUNCTION mcp_enqueue_account_artifacts() TO PUBLIC;

REVOKE SELECT, INSERT, UPDATE, DELETE
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
  FROM go_api_runtime;

REVOKE SELECT
  ON TABLE workspaces, workspace_memberships
  FROM go_api_runtime;
