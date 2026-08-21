package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestHostWorkspacesUpgradeExistingRegistryInPlace(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("102_host_workspaces.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("102_host_workspaces.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down := string(upBytes), string(downBytes)
	for _, required := range []string{
		"ALTER TABLE workspaces",
		"ADD COLUMN system_prompt",
		"ADD COLUMN runner_id",
		"workspaces_host_binding_shape",
		"idx_workspaces_owner_runner_directory_active",
		"ADD COLUMN agent_workspace_id",
		"conversations_agent_workspace_owner_fk",
		"GRANT SELECT ON TABLE workspaces TO go_api_runtime",
		"GRANT INSERT (",
		"GRANT UPDATE (",
		") ON TABLE workspaces TO go_api_runtime",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("102 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP TABLE workspaces",
		"DELETE FROM workspaces",
		"UPDATE schema_migrations",
		"ON DELETE CASCADE",
	} {
		if strings.Contains(strings.ToUpper(up), strings.ToUpper(forbidden)) {
			t.Fatalf("102 up migration contains forbidden contract %q", forbidden)
		}
	}
	if !strings.Contains(down, "HOST_WORKSPACE_ROLLBACK_BLOCKED") ||
		!strings.Contains(down, "agent_workspace_id IS NOT NULL") ||
		!strings.Contains(down, ") ON TABLE conversations FROM go_api_runtime") {
		t.Fatal("102 down migration must guard durable Workspace and execution bindings")
	}
}
