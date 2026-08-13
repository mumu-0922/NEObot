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
	legacyTavilyRepair := readPhase15SQL(t, "078_mcp_legacy_tavily_runner_repair.up.sql")
	legacyTavilyRollback := readPhase15SQL(t, "078_mcp_legacy_tavily_runner_repair.down.sql")
	tavilyCredentialRevalidation := readPhase15SQL(t, "079_mcp_tavily_credential_revalidation.up.sql")
	tavilyCredentialRevalidationRollback := readPhase15SQL(t, "079_mcp_tavily_credential_revalidation.down.sql")
	legacyDeepWikiIcon := readPhase15SQL(t, "080_mcp_legacy_deepwiki_icon.up.sql")
	legacyDeepWikiIconRollback := readPhase15SQL(t, "080_mcp_legacy_deepwiki_icon.down.sql")
	legacyContext7Rebind := readPhase15SQL(t, "081_mcp_legacy_context7_artifact_rebind.up.sql")
	legacyContext7RebindRollback := readPhase15SQL(t, "081_mcp_legacy_context7_artifact_rebind.down.sql")

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
	assertPhase15Fragments(t, legacyTavilyRepair,
		"the narrowly identified anonymous Tavily failure must become a recoverable reviewed Runner draft",
		"runner://marketplace-tavily-ai-tavily-mcp-0.2.19", "transport = 'stdio'",
		"auth_type = 'env'", "status = 'needs_auth'", "last_error_code = 'credential_required'",
		"deploymenthash", "70d1cd77529cffedb9624f4b3f62383e4aa43ba2bfba451c84d4b13e4a4e7d98",
		"not exists", "mcp_credentials")
	assertPhase15Fragments(t, legacyTavilyRollback,
		"rollback must refuse to strand a repaired Runner environment credential",
		"refusing to roll back repaired tavily runner servers while credentials exist",
		"legacyinstallrepair", "https://mcp.tavily.com/mcp", "auth_type = 'none'")
	assertPhase15Fragments(t, tavilyCredentialRevalidation,
		"previous tools/list success must not remain credential authority",
		"runner://marketplace-tavily-ai-tavily-mcp-0.2.19", "status = 'needs_auth'",
		"credential_revalidation_required", "server.status = 'ready'")
	assertPhase15Fragments(t, tavilyCredentialRevalidationRollback,
		"rollback must not restore a false ready state", "select 1")
	assertPhase15Fragments(t, legacyDeepWikiIcon,
		"the exact legacy DeepWiki endpoint receives only bounded display metadata",
		"https://mcp.deepwiki.com/mcp", "https://deepwiki.com/favicon.ico",
		"legacyiconrepair", "coalesce", "metadata , icon")
	assertPhase15Fragments(t, legacyDeepWikiIconRollback,
		"rollback removes only the icon written by the exact repair marker",
		"https://mcp.deepwiki.com/mcp", "https://deepwiki.com/favicon.ico",
		"legacyiconrepair", "= '080'", "- 'icon'")
	assertPhase15Fragments(t, legacyContext7Rebind,
		"the exact legacy Context7 install is rebound to the current reviewed artifact hash",
		"runner://marketplace-upstash-context7-2.2.0", "auth_type = 'none'",
		"runnerartifactid", "upstash-context7", "version", "2.2.0",
		"20a578fff586f03151f2f2f6aa97ea331d3985f8e93ce068f0f59e984cc4964d",
		"22b235834a14b617480cc92dd0f6f6c7587cb399880c666773135971767bc6e2",
		"legacyartifactrepair", "081")
	assertPhase15Fragments(t, legacyContext7RebindRollback,
		"rollback restores only the Context7 hash written by the exact repair marker",
		"runner://marketplace-upstash-context7-2.2.0",
		"22b235834a14b617480cc92dd0f6f6c7587cb399880c666773135971767bc6e2",
		"20a578fff586f03151f2f2f6aa97ea331d3985f8e93ce068f0f59e984cc4964d",
		"legacyartifactrepair", "= '081'", "#- '{metadata , legacyartifactrepair}'")
}
