package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestChatAgentPermissionModesAreDurableAndRollbackGuarded(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("103_chat_agent_permission_modes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("103_chat_agent_permission_modes.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down := string(upBytes), string(downBytes)
	for _, required := range []string{
		"ADD COLUMN agent_permission_mode TEXT NOT NULL DEFAULT 'workspace-write'",
		"'read-only', 'workspace-write', 'danger-full-access'",
		"VALIDATE CONSTRAINT conversations_agent_permission_mode_allowed",
		"GRANT UPDATE (agent_permission_mode, updated_at)",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("103 up migration missing %q", required)
		}
	}
	if !strings.Contains(down, "agent_permission_mode <> 'workspace-write'") ||
		!strings.Contains(down, "cannot remove durable Agent permission selections") {
		t.Fatal("103 down migration must preserve explicit non-default choices")
	}
}
