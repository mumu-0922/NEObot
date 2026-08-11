package migration

import (
	"strings"
	"testing"
)

func TestMCPToolsFoundationSchemaContract(t *testing.T) {
	up := readPhase15SQL(t, "074_mcp_tools_foundation.up.sql")
	down := readPhase15SQL(t, "074_mcp_tools_foundation.down.sql")
	runtimeGrants := readPhase15SQL(t, "075_mcp_runtime_role_grants.up.sql")
	runtimeGrantRollback := readPhase15SQL(t, "075_mcp_runtime_role_grants.down.sql")

	for _, table := range []string{
		"workspaces",
		"workspace_memberships",
		"mcp_servers",
		"mcp_credentials",
		"mcp_server_grants",
		"mcp_conversation_selections",
		"mcp_conversation_servers",
		"mcp_workspace_selections",
		"mcp_workspace_servers",
		"mcp_oauth_states",
		"mcp_run_snapshots",
		"mcp_tool_calls",
		"mcp_tool_results",
		"mcp_artifact_cleanup_queue",
	} {
		if !strings.Contains(strings.ToLower(up), "create table "+table) {
			t.Fatalf("up migration does not create %s", table)
		}
		if !phase15DropsTable(down, table) {
			t.Fatalf("down migration does not drop %s", table)
		}
	}

	assertPhase15Fragments(t, up,
		"retired Plugin selection must be removed without deleting unrelated metadata",
		"update conversations", "metadata = metadata - 'activeplugins'", "where metadata ? 'activeplugins'")
	assertPhase15Fragments(t, mustPhase15TableBody(t, up, "mcp_tool_calls"),
		"MCP call audit metadata must have an explicit retention deadline",
		"arguments_summary jsonb not null", "retained_until timestamptz not null")
	assertPhase15Fragments(t, up,
		"retention and lifecycle queries must be indexed",
		"idx_mcp_tool_calls_retention", "idx_mcp_tool_calls_conversation")
	assertPhase15Fragments(t, up,
		"account deletion must enqueue MCP artifacts before user-owned rows cascade",
		"mcp_enqueue_account_artifacts", "before delete on users", "mcp_artifact_cleanup_queue")
	assertPhase15Fragments(t, down,
		"rollback must preserve the retired Plugin registry for the stable rollback window",
		"drop table if exists mcp_tool_calls")
	if strings.Contains(strings.ToLower(down), "drop table if exists plugin_registry") {
		t.Fatal("074 down migration must preserve plugin_registry")
	}

	assertPhase15Fragments(t, runtimeGrants,
		"the Go API role must receive explicit MCP table capabilities",
		"grant select", "on table workspaces , workspace_memberships", "to go_api_runtime",
		"mcp_artifact_cleanup_queue")
	assertPhase15Fragments(t, runtimeGrants,
		"the account cleanup trigger must not remain public executable",
		"revoke all on function mcp_enqueue_account_artifacts ( ) from public",
		"grant execute on function mcp_enqueue_account_artifacts ( ) to go_api_runtime")
	assertPhase15Fragments(t, runtimeGrantRollback,
		"the runtime grant migration must revoke MCP table access on rollback",
		"revoke select , insert , update , delete", "from go_api_runtime")
}
