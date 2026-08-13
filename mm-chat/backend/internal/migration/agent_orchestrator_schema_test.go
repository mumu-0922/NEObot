package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentOrchestratorMigrationDefinesDurableAuthorityAndFences(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("084_agent_orchestrator_foundation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("084_agent_orchestrator_foundation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, contract := range []string{
		"CREATE TABLE agent_run_snapshots",
		"CREATE TABLE agent_runs",
		"CREATE TABLE agent_steps",
		"CREATE TABLE agent_attempts",
		"CREATE TABLE agent_run_events",
		"CREATE TABLE agent_kill_switches",
		"CREATE UNIQUE INDEX idx_agent_attempt_one_live_per_step",
		"UNIQUE (run_id, sequence)",
		"CREATE TRIGGER trg_agent_snapshot_immutable",
		"CREATE TRIGGER trg_agent_event_immutable",
		"CREATE FUNCTION agent_orchestrator_enqueue_run(",
		"CREATE FUNCTION agent_orchestrator_transition(",
		"CREATE FUNCTION agent_orchestrator_acquire_step(",
		"CREATE FUNCTION agent_orchestrator_heartbeat_attempt(",
		"CREATE FUNCTION agent_orchestrator_rebuild_projection(",
		"CREATE FUNCTION agent_orchestrator_append_kill_switch(",
		"CREATE FUNCTION agent_orchestrator_prune_terminal_runs(",
		"REVOKE ALL ON agent_run_snapshots,agent_runs,agent_steps,agent_attempts",
		"FROM PUBLIC,go_api_runtime,agent_orchestrator_runtime",
		"GRANT SELECT ON agent_run_snapshots,agent_runs,agent_steps,agent_attempts",
	} {
		if !strings.Contains(text, contract) {
			t.Fatalf("migration missing %q", contract)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON agent_",
		"GRANT UPDATE ON agent_",
		"GRANT DELETE ON agent_",
		"TO go_api_runtime",
		"runner_url",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	downText := string(down)
	for _, contract := range []string{
		"AGENT_ORCHESTRATOR_DOWN_DATA_EXISTS",
		"DROP TABLE agent_kill_switches",
		"DROP TABLE agent_run_events",
		"DROP TABLE agent_attempts",
		"DROP TABLE agent_steps",
		"DROP TABLE agent_runs",
		"DROP TABLE agent_run_snapshots",
	} {
		if !strings.Contains(downText, contract) {
			t.Fatalf("down migration missing %q", contract)
		}
	}
}
