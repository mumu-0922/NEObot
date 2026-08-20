package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestChatAgentApprovalsAreCASBoundedAndLeastPrivilege(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("100_chat_agent_approvals.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("100_chat_agent_approvals.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down := string(upBytes), string(downBytes)
	for _, required := range []string{
		"CREATE TABLE chat_agent_approvals (",
		"CREATE TABLE chat_agent_conversation_tool_grants (",
		"UNIQUE (turn_id, execution_id)",
		"expires_at <= created_at + interval '5 minutes'",
		"CREATE FUNCTION chat_agent_create_approval(",
		"CREATE FUNCTION chat_agent_decide_approval(",
		"CREATE FUNCTION chat_agent_recover_approvals(",
		"IF v_approval.status <> 'pending' THEN",
		"CHAT_AGENT_APPROVAL_STALE_REVISION",
		"decision = 'restart_denied'",
		"SET search_path TO %I, pg_catalog, pg_temp",
		"REVOKE ALL ON chat_agent_approvals, chat_agent_conversation_tool_grants",
		"GRANT EXECUTE ON FUNCTION chat_agent_create_approval(",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("100 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"raw_arguments",
		"raw_result",
		"GRANT INSERT ON chat_agent_",
		"GRANT UPDATE ON chat_agent_",
		"GRANT DELETE ON chat_agent_",
		"UPDATE schema_migrations",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("100 migration contains forbidden contract %q", forbidden)
		}
	}
	if got := strings.Count(up, "SECURITY DEFINER"); got != 3 {
		t.Fatalf("100 SECURITY DEFINER count = %d, want 3", got)
	}
	for _, required := range []string{
		"CHAT_AGENT_APPROVALS_DOWN_DATA_EXISTS",
		"DROP FUNCTION chat_agent_recover_approvals",
		"DROP TABLE chat_agent_approvals",
		"DROP TABLE chat_agent_conversation_tool_grants",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("100 down migration missing %q", required)
		}
	}
}
