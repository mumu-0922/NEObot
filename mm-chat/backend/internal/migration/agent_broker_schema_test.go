package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentBrokerMigrationDefinesDurableLeastPrivilegeAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("086_agent_broker_foundation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("086_agent_broker_foundation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{
		"CREATE TABLE agent_effect_intents", "CREATE TABLE agent_effect_approvals",
		"CREATE TABLE agent_effect_receipts", "CREATE TABLE agent_secret_handles",
		"CREATE TABLE agent_effect_grant_revocations", "CREATE FUNCTION agent_effect_revoke_grant(",
		"CREATE TABLE agent_effect_cancellations", "CREATE FUNCTION agent_effect_cancel(",
		"capability_max_calls", "grant_max_tool_calls", "BUDGET_EXHAUSTED",
		"CREATE FUNCTION agent_effect_prepare(", "CREATE FUNCTION agent_effect_claim_commit(",
		"CREATE FUNCTION agent_effect_complete_commit(", "agent_effect_control",
		"CREATE FUNCTION agent_secret_handle_create(", "CREATE FUNCTION agent_secret_handle_revoke(",
		"p_binding JSONB", "p_operation='secret'", "state='revoked'",
		"SECURITY DEFINER SET search_path FROM CURRENT", "AGENT_EFFECT_IMMUTABLE",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"TO go_api_runtime", "TO agent_runner_control", "TO agent_orchestrator_runtime",
		"GRANT INSERT ON agent_effect_", "GRANT UPDATE ON agent_effect_", "GRANT DELETE ON agent_effect_",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{"AGENT_EFFECT_DOWN_DATA_EXISTS", "DROP TABLE agent_secret_handles",
		"DROP TABLE agent_effect_grant_revocations",
		"DROP TABLE agent_effect_cancellations",
		"DROP TABLE agent_effect_intents", "DROP ROLE %I"} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
