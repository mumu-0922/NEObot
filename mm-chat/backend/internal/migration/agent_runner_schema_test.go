package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentRunnerMigrationDefinesCredentialFreeControlAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("085_agent_runner_foundation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("085_agent_runner_foundation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, contract := range []string{
		"CREATE TABLE agent_runner_requests", "CREATE TABLE agent_runner_sandboxes",
		"runner_id TEXT NOT NULL", "runner_id=lease_owner", "v_request.runner_id<>p_runner_id",
		"CREATE FUNCTION agent_orchestrator_validate_runner_authority(",
		"CREATE FUNCTION agent_runner_issue_authority(", "CREATE FUNCTION agent_runner_complete_request(",
		"CREATE FUNCTION agent_runner_expect_sandbox(", "CREATE FUNCTION agent_runner_update_sandbox(",
		"CREATE FUNCTION agent_runner_recovery_sandboxes(", "CREATE FUNCTION agent_runner_prune(",
		"agent_runner_response_sanitized", "agent_runner_control", "FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime",
	} {
		if !strings.Contains(text, contract) {
			t.Fatalf("migration missing %q", contract)
		}
	}
	for _, forbidden := range []string{
		"agent_runner_runtime", "GRANT SELECT ON agent_runner_", "GRANT INSERT ON agent_runner_",
		"GRANT UPDATE ON agent_runner_", "GRANT DELETE ON agent_runner_", "TO go_api_runtime",
		"TO agent_orchestrator_runtime",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, contract := range []string{
		"AGENT_RUNNER_DOWN_DATA_EXISTS", "DROP TABLE agent_runner_sandboxes",
		"DROP TABLE agent_runner_requests", "DROP ROLE %I",
	} {
		if !strings.Contains(string(down), contract) {
			t.Fatalf("down migration missing %q", contract)
		}
	}
}
