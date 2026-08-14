package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentLearningMigrationDefinesQuarantineChecksPromotionAndCleanupAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("089_agent_draft_learning.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("089_agent_draft_learning.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{
		"CREATE TABLE agent_learning_drafts", "CREATE TABLE agent_learning_check_results",
		"CREATE TABLE agent_learning_decisions", "CREATE TABLE agent_learning_cleanup_queue",
		"CREATE TABLE agent_learning_audit_events", "UNIQUE(draft_id,generation,kind)",
		"kind IN ('static','isolation','evaluation')", "CREATE FUNCTION agent_learning_create_draft(",
		"CREATE FUNCTION agent_learning_claim_checks(", "FOR UPDATE SKIP LOCKED",
		"CREATE FUNCTION agent_learning_complete_checks(", "CREATE FUNCTION agent_learning_reject(",
		"CREATE FUNCTION agent_learning_promote(", "CREATE FUNCTION agent_learning_claim_cleanup(",
		"CREATE FUNCTION agent_learning_complete_cleanup(", "CREATE FUNCTION agent_learning_reconcile(",
		"CREATE FUNCTION agent_learning_prune(", "source_type IN ('official','lobehub','git','zip','learning')",
		"SOURCE_RUN_INVALID", "PACKAGE_ALREADY_EXISTS", "KILL_SWITCH_ACTIVE", "STALE_CLAIM",
		"CHECKS_INCOMPLETE", "AGENT_LEARNING_IMMUTABLE", "agent_learning_control",
		"SECURITY DEFINER SET search_path FROM CURRENT", "SET search_path TO %I, pg_catalog, pg_temp",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON agent_learning_", "GRANT UPDATE ON agent_learning_",
		"GRANT DELETE ON agent_learning_", "TO go_api_runtime;", "TO agent_runner_control;",
		"TO agent_effect_control;", "TO agent_delegation_control;", "TO agent_cron_control;",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"AGENT_LEARNING_DOWN_DATA_EXISTS", "source_type='learning'",
		"DROP FUNCTION agent_learning_prune(TIMESTAMPTZ,INTEGER)",
		"DROP FUNCTION agent_learning_promote(", "DROP TABLE agent_learning_audit_events",
		"DROP TABLE agent_learning_drafts", "CHECK(source_type IN ('official','lobehub','git','zip'))",
		"REVOKE ALL ON SCHEMA %I FROM agent_learning_owner,agent_learning_control",
		"DROP ROLE agent_learning_control", "DROP ROLE agent_learning_owner",
	} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
