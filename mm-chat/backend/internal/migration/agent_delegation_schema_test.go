package migration

import (
	migrationfiles "neo-chat/mm-chat/backend/migrations"
	"strings"
	"testing"
)

func TestAgentDelegationMigrationDefinesDepthBudgetAndCascadeAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("087_agent_child_delegation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("087_agent_child_delegation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{"CREATE TABLE agent_delegation_authorities", "CREATE TABLE agent_delegation_lineage", "CREATE TABLE agent_delegation_settlements", "CREATE TABLE agent_delegation_reaps", "CREATE FUNCTION agent_delegation_registry_subset(", "CREATE FUNCTION agent_delegation_enqueue_child(", "CREATE FUNCTION agent_delegation_admit_launch(", "CREATE FUNCTION agent_delegation_cascade(", "CREATE FUNCTION agent_delegation_reconcile(", "starts_with(child_resource,parent_resource)", "SUBSET_VIOLATION", "BUDGET_EXCEEDED", "PARENT_STALE", "SETTLEMENT_INVALID", "PARENT_AUTHORITY_STALE", "agent_delegation_control", "SECURITY DEFINER SET search_path FROM CURRENT", "SET search_path TO %I, pg_catalog, pg_temp"} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{"TO go_api_runtime", "TO agent_runner_control", "GRANT INSERT ON agent_delegation_", "GRANT UPDATE ON agent_delegation_", "GRANT DELETE ON agent_delegation_"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{"AGENT_DELEGATION_DOWN_DATA_EXISTS", "DROP FUNCTION agent_delegation_reconcile(INTEGER)", "DROP FUNCTION agent_delegation_registry_subset(JSONB,JSONB)", "DROP TABLE agent_delegation_reaps", "DROP TABLE agent_delegation_authorities", "DROP ROLE %I"} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
