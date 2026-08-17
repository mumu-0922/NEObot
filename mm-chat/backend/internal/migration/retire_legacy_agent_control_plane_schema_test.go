package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestRetireLegacyAgentControlPlaneIsExplicitFailClosedAndIrreversible(t *testing.T) {
	t.Helper()
	upBytes, err := migrationfiles.FS.ReadFile("098_retire_legacy_agent_control_plane.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("098_retire_legacy_agent_control_plane.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, required := range []string{
		"LEGACY_AGENT_CONTROL_PLANE_DATA_EXISTS",
		"LEGACY_AGENT_CONTROL_PLANE_PARTIAL_STATE",
		"LEGACY_AGENT_CONTROL_PLANE_SINGLETON_DRIFT",
		"IN ACCESS EXCLUSIVE MODE",
		"FOREACH object_name IN ARRAY legacy_fact_tables",
		"agent_kill_switch_state must contain only its bootstrap singleton",
		"agent_shadow_boot_state must contain only its bootstrap singleton",
		"public.agent_product_runs",
		"DROP TRIGGER IF EXISTS trg_agent_snapshot_immutable ON public.agent_run_snapshots",
		"ALTER TABLE %I.%I DROP CONSTRAINT %I",
		"DROP TABLE IF EXISTS public.agent_artifacts",
		"REASSIGN OWNED BY %I TO CURRENT_USER",
		"DROP OWNED BY %I",
		"DROP ROLE %I",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("098 up migration missing %q", required)
		}
	}

	legacyTables := []string{
		"agent_artifacts", "agent_attempts", "agent_cron_approval_revocations",
		"agent_cron_approvals", "agent_cron_audit_events", "agent_cron_revisions",
		"agent_cron_templates", "agent_cron_triggers", "agent_cron_worker_targets",
		"agent_delegation_authorities", "agent_delegation_lineage", "agent_delegation_reaps",
		"agent_delegation_settlements", "agent_effect_approvals", "agent_effect_cancellations",
		"agent_effect_grant_revocations", "agent_effect_intents", "agent_effect_receipts",
		"agent_kill_switch_state", "agent_kill_switches", "agent_learning_audit_events",
		"agent_learning_check_results", "agent_learning_cleanup_queue", "agent_learning_decisions",
		"agent_learning_drafts", "agent_learning_runner_attempts", "agent_learning_runner_requests",
		"agent_learning_runner_results", "agent_learning_worker_targets", "agent_product_canary_activations",
		"agent_product_canary_promotions", "agent_product_canary_receipts", "agent_product_canary_requests",
		"agent_product_run_cancellations", "agent_project_canary_resources", "agent_project_mutation_cleanups",
		"agent_project_mutation_receipts", "agent_run_events", "agent_run_snapshots",
		"agent_runner_requests", "agent_runner_sandboxes", "agent_runs", "agent_secret_handles",
		"agent_shadow_boot_state", "agent_shadow_observations", "agent_shadow_policies",
		"agent_shadow_user_opt_ins", "agent_steps",
	}
	for _, table := range legacyTables {
		if !strings.Contains(up, "'"+table+"'") || !strings.Contains(up, "public."+table) {
			t.Fatalf("098 up migration does not explicitly whitelist %q", table)
		}
	}

	factStart := strings.Index(up, "legacy_fact_tables CONSTANT")
	factEnd := strings.Index(up[factStart:], "legacy_views CONSTANT")
	if factStart < 0 || factEnd < 0 {
		t.Fatal("098 up migration fact-table whitelist is missing")
	}
	facts := up[factStart : factStart+factEnd]
	for _, singleton := range []string{"agent_kill_switch_state", "agent_shadow_boot_state"} {
		if strings.Contains(facts, "'"+singleton+"'") {
			t.Fatalf("098 treats bootstrap singleton %q as an empty fact table", singleton)
		}
	}

	for _, forbidden := range []string{
		"chat_agent_",
		"DROP TABLE agent_",
		"DROP VIEW agent_",
		"DROP FUNCTION agent_",
		" CASCADE",
		"LIKE 'agent\\_%'",
	} {
		if strings.Contains(up, forbidden) {
			t.Fatalf("098 up migration contains forbidden broad cleanup %q", forbidden)
		}
	}
	if !strings.Contains(down, "Irreversible by design") || strings.Contains(down, "CREATE ") {
		t.Fatal("098 down migration must remain an explicit irreversible no-op")
	}
}
