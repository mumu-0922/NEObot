package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentProductShadowMigrationDefinesBoundedProductAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("090_agent_product_shadow.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("090_agent_product_shadow.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{
		"CREATE TABLE agent_product_run_cancellations",
		"CREATE TABLE agent_artifacts",
		"CREATE TABLE agent_shadow_policies",
		"CREATE TABLE agent_shadow_user_opt_ins",
		"CREATE TABLE agent_shadow_observations",
		"CREATE VIEW agent_product_runs",
		"CREATE VIEW agent_product_approvals",
		"CREATE VIEW agent_product_schedules",
		"CREATE VIEW agent_product_drafts",
		"CREATE FUNCTION agent_product_get_artifact(",
		"CREATE FUNCTION agent_product_cancel_run(",
		"CREATE FUNCTION agent_product_shadow_snapshot(",
		"CREATE FUNCTION agent_product_append_shadow_observation(",
		"mode IN ('synthetic','read_only')",
		"'ISOLATION_UNAVAILABLE'",
		"GENERATION_STALE",
		"FINGERPRINT_DRIFT",
		"BUDGET_EXCEEDED",
		"KILL_SWITCH_ACTIVE",
		"agent_orchestrator_active_kill_mode(ARRAY[",
		"SECURITY DEFINER SET search_path FROM CURRENT",
		"SET search_path TO %I, pg_catalog, pg_temp",
		"GRANT SELECT ON agent_product_runs",
		"GRANT EXECUTE ON FUNCTION agent_orchestrator_active_kill_mode(TEXT[])",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON agent_shadow_",
		"GRANT UPDATE ON agent_shadow_",
		"GRANT DELETE ON agent_shadow_",
		"GRANT SELECT ON agent_run_snapshots TO go_api_runtime",
		"GRANT EXECUTE ON FUNCTION agent_effect_claim_commit",
		"GRANT EXECUTE ON FUNCTION agent_cron_claim_due",
		"GRANT EXECUTE ON FUNCTION agent_learning_claim_checks",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"AGENT_PRODUCT_DOWN_DATA_EXISTS",
		"DROP FUNCTION agent_product_append_shadow_observation",
		"DROP FUNCTION agent_product_get_artifact",
		"DROP VIEW agent_product_runs",
		"DROP TABLE agent_shadow_observations",
		"DROP TABLE agent_product_run_cancellations",
		"DROP ROLE agent_product_owner",
	} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
