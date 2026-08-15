package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentProductCanaryActivationDefinesDurableLeastPrivilegeHandoff(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("095_agent_product_canary_activation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("095_agent_product_canary_activation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down := string(upBytes), string(downBytes)
	for _, required := range []string{
		"CREATE ROLE agent_product_canary_worker NOLOGIN NOSUPERUSER",
		"CREATE TABLE agent_product_canary_activations(",
		"CREATE TABLE agent_product_canary_requests(",
		"CREATE TABLE agent_product_canary_receipts(",
		"CREATE TABLE agent_product_canary_promotions(",
		"CREATE FUNCTION agent_product_canary_status(",
		"CREATE FUNCTION agent_product_canary_enqueue(",
		"CREATE FUNCTION agent_product_canary_worker_health(",
		"CREATE FUNCTION agent_product_canary_worker_claim_requests(",
		"FOR UPDATE OF request SKIP LOCKED",
		"CREATE FUNCTION agent_product_canary_worker_complete_request(",
		"AGENT_PRODUCT_CANARY_RECEIPT_BINDING_INVALID",
		"run.idempotency_key='g21.6-product-canary-'",
		"CREATE FUNCTION agent_product_canary_worker_release_request(",
		"CREATE FUNCTION agent_product_canary_worker_reconcile(",
		"CASE WHEN request.failure_count>=2 THEN 'failed' ELSE 'queued' END",
		"terminal_at=CASE WHEN request.failure_count>=2 THEN p_now ELSE NULL END",
		"CREATE FUNCTION agent_product_canary_record_promotion(",
		"p_decision<>'PROMOTION_READY'",
		"agent_orchestrator_active_kill_mode(ARRAY[",
		"GRANT EXECUTE ON FUNCTION agent_product_canary_status(UUID)",
		"TO agent_product_canary_worker",
		"SET search_path TO %I, pg_catalog, pg_temp",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("095 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT SELECT ON agent_product_canary_",
		"GRANT INSERT ON agent_product_canary_",
		"GRANT UPDATE ON agent_product_canary_",
		"GRANT EXECUTE ON FUNCTION agent_product_canary_provision_activation",
		"GRANT EXECUTE ON FUNCTION agent_product_canary_record_promotion",
		"agent_effect_commit",
		"agent_delegation_enqueue",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("095 grants forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"AGENT_PRODUCT_CANARY_DOWN_REQUIRES_EMPTY",
		"DROP FUNCTION agent_product_canary_record_promotion(",
		"DROP FUNCTION agent_product_canary_enqueue(",
		"DROP FUNCTION agent_product_canary_worker_health(",
		"DROP TABLE agent_product_canary_promotions",
		"DROP TABLE agent_product_canary_receipts",
		"DROP TABLE agent_product_canary_requests",
		"DROP TABLE agent_product_canary_activations",
		"DROP ROLE agent_product_canary_worker",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("095 down migration missing %q", required)
		}
	}
}
