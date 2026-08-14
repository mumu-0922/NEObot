package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentProjectMutationMigrationDefinesFunctionOnlyCASAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("092_agent_project_mutation_canary.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("092_agent_project_mutation_canary.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{
		"CREATE ROLE agent_project_mutation_owner", "CREATE ROLE agent_project_mutation_control",
		"CREATE TABLE agent_project_canary_resources", "CREATE TABLE agent_project_mutation_receipts",
		"CREATE TABLE agent_project_mutation_cleanups", "CREATE FUNCTION agent_effect_project_mutation_fence(",
		"CREATE FUNCTION agent_effect_project_cleanup_fence(",
		"CREATE FUNCTION agent_orchestrator_project_mutation_fence(",
		"CREATE FUNCTION agent_project_canary_provision(", "CREATE FUNCTION agent_project_mutation_commit(",
		"CREATE FUNCTION agent_project_mutation_status(", "CREATE FUNCTION agent_project_mutation_cleanup(",
		"v_intent.approval_class<>'per_commit'", "agent_effect_approvals",
		"agent_effect_grant_revocations", "agent_orchestrator_active_kill_mode(v_run.scope_keys) IS NOT NULL",
		"neo.agent-project-canary-snapshot/v1", "PROJECT_CONFLICT", "outcome_unknown",
		"SECURITY DEFINER SET search_path FROM CURRENT", "SET search_path TO %I, pg_catalog, pg_temp",
		"TO agent_project_mutation_control", "REVOKE ALL ON agent_project_canary_resources",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON agent_project_canary_resources", "GRANT UPDATE ON agent_project_canary_resources",
		"GRANT DELETE ON agent_project_canary_resources",
		"GRANT agent_project_mutation_owner TO agent_project_mutation_control",
		"TO go_api_runtime", "TO agent_runner_control", "TO agent_orchestrator_runtime",
		"GRANT EXECUTE ON FUNCTION agent_project_canary_provision", // operator-only provision
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"AGENT_PROJECT_MUTATION_DOWN_REQUIRES_EMPTY", "DROP FUNCTION agent_project_mutation_cleanup",
		"DROP FUNCTION agent_project_mutation_status", "DROP FUNCTION agent_project_mutation_commit",
		"DROP TABLE agent_project_mutation_receipts", "DROP ROLE agent_project_mutation_control",
		"DROP ROLE agent_project_mutation_owner",
	} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
