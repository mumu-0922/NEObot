package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentCronMigrationDefinesRevisionClaimAndTriggerAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("088_agent_cron_foundation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("088_agent_cron_foundation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{
		"CREATE TABLE agent_cron_templates", "CREATE TABLE agent_cron_revisions",
		"CREATE TABLE agent_cron_approvals", "CREATE TABLE agent_cron_approval_revocations",
		"CREATE TABLE agent_cron_triggers", "CREATE TABLE agent_cron_audit_events",
		"CREATE FUNCTION agent_cron_audit_sanitized(",
		"(p_spec->'owner')-ARRAY['userId','projectId','assistantId']",
		"capability-ARRAY['capability','actions','resources','approval','maxCalls']",
		"secret-ARRAY['slot','brokerRef','actions','ttlSeconds']",
		"CREATE FUNCTION agent_cron_claim_due(", "FOR UPDATE SKIP LOCKED",
		"CREATE FUNCTION agent_cron_advance_cursor(", "CREATE FUNCTION agent_cron_claim_triggers(",
		"CREATE FUNCTION agent_cron_enqueue_trigger(", "agent_orchestrator_enqueue_run(",
		"CREATE FUNCTION agent_cron_reconcile(", "CREATE FUNCTION agent_cron_prune(",
		"OWNER_REVOKED", "SKILL_REVOKED", "GRANT_REVOKED", "APPROVAL_REVOKED",
		"SECRET_REVOKED", "KILL_SWITCH_ACTIVE", "STALE_TEMPLATE", "STALE_CLAIM",
		"agent_cron_control", "SECURITY DEFINER SET search_path FROM CURRENT",
		"SET search_path TO %I, pg_catalog, pg_temp",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON agent_cron_", "GRANT UPDATE ON agent_cron_", "GRANT DELETE ON agent_cron_",
		"TO go_api_runtime;", "TO agent_runner_control;", "TO agent_effect_control;",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"AGENT_CRON_DOWN_DATA_EXISTS", "DROP FUNCTION agent_cron_prune(TIMESTAMPTZ,INTEGER)",
		"DROP FUNCTION agent_cron_enqueue_trigger(", "DROP TABLE agent_cron_audit_events",
		"DROP TABLE agent_cron_templates", "REVOKE SELECT ON users,skill_installations",
		"REVOKE USAGE ON SCHEMA %I FROM agent_cron_owner,agent_cron_control", "DROP ROLE %I",
	} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
