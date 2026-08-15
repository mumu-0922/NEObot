package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentChildReapTransportDefinesFunctionOnlyBridge(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("093_agent_child_canary_reap_transport.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("093_agent_child_canary_reap_transport.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)
	for _, required := range []string{
		"CREATE FUNCTION agent_delegation_reap_inventory(",
		"CREATE OR REPLACE FUNCTION agent_delegation_complete_reap(",
		"request.method='launch'", "max(request.expires_at)",
		"request.lease_generation=reap.generation",
		"request.lease_owner=reap.lease_owner",
		"p_succeeded IS NULL", "AGENT_REAP_AUTHORITY_ACTIVE", "AGENT_REAP_SANDBOX_MISMATCH",
		"UPDATE agent_runner_sandboxes SET state='terminal'",
		"GRANT EXECUTE ON FUNCTION agent_delegation_reap_inventory(INTEGER)",
		"TO agent_delegation_control",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("093 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT SELECT ON agent_runner_requests,agent_runner_sandboxes TO agent_delegation_control",
		"GRANT UPDATE ON agent_runner_sandboxes TO agent_delegation_control",
		"GRANT INSERT ON agent_runner_", "GRANT DELETE ON agent_runner_",
		"TO go_api_runtime", "TO agent_runner_control;",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("093 grants forbidden runtime authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"AGENT_CHILD_REAP_TRANSPORT_DOWN_REQUIRES_CLEAN",
		"WHERE sandbox.state NOT IN ('terminal','orphaned')",
		"DROP FUNCTION agent_delegation_reap_inventory(INTEGER)",
		"CREATE OR REPLACE FUNCTION agent_delegation_complete_reap(",
		"REVOKE SELECT,UPDATE ON agent_runner_sandboxes FROM agent_delegation_owner",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("093 down migration missing %q", required)
		}
	}
}
