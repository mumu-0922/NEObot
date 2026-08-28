package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestRetireBuiltinPlaywrightBrowserIsExactAndPreservesHistory(t *testing.T) {
	t.Parallel()
	upBytes, err := migrationfiles.FS.ReadFile("108_retire_builtin_playwright_browser.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("108_retire_builtin_playwright_browser.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, required := range []string{
		"mcp_conversation_servers",
		"mcp_conversation_selections",
		"mcp_workspace_servers",
		"mcp_workspace_selections",
		"mcp_credentials",
		"mcp_oauth_states",
		"mcp_server_grants",
		"server_source = 'manifest'",
		"server_ref = 'playwright-browser-0.0.79'",
		"revision = selection.revision + 1",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("108 up migration missing %q", required)
		}
	}
	for _, historical := range []string{"mcp_run_snapshots", "mcp_tool_calls", "mcp_tool_results"} {
		if strings.Contains(up, "DELETE FROM "+historical) {
			t.Fatalf("108 must preserve historical table %q", historical)
		}
	}
	if !strings.Contains(down, "Irreversible by design") || strings.Contains(down, "INSERT INTO") {
		t.Fatal("108 down migration must remain an explicit irreversible no-op")
	}
}
