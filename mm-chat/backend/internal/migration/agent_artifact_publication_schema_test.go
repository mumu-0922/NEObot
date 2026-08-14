package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentArtifactPublicationMigrationDefinesFunctionOnlyAuthority(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("091_agent_artifact_publication.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("091_agent_artifact_publication.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, required := range []string{
		"CREATE ROLE agent_artifact_control", "CREATE FUNCTION agent_effect_artifact_fence(",
		"CREATE FUNCTION agent_orchestrator_artifact_fence(",
		"CREATE FUNCTION agent_product_validate_artifact_authority(",
		"CREATE FUNCTION agent_artifact_authorize(", "CREATE FUNCTION agent_artifact_attach(",
		"v_intent.state<>'committing'", "v_attempt.state<>'committing'",
		"v_attempt.lease_expires_at<=v_now", "agent_orchestrator_active_kill_mode(v_run.scope_keys) IS NOT NULL",
		"agent_effect_grant_revocations", "artifactPolicy,maxBytes", "allowedMediaTypes",
		"REPLAY_DETECTED", "SECURITY DEFINER SET search_path FROM CURRENT",
		"SET search_path TO %I, pg_catalog, pg_temp", "TO agent_artifact_control",
		"REVOKE ALL ON agent_artifacts FROM agent_artifact_control",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON agent_artifacts", "GRANT UPDATE ON agent_artifacts",
		"GRANT DELETE ON agent_artifacts", "GRANT agent_product_owner TO agent_artifact_control",
		"TO go_api_runtime", "TO agent_runner_control", "TO agent_orchestrator_runtime",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("migration contains forbidden authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"DROP FUNCTION agent_artifact_attach", "DROP FUNCTION agent_artifact_authorize",
		"DROP FUNCTION agent_effect_artifact_fence", "DROP FUNCTION agent_orchestrator_artifact_fence",
		"DROP ROLE agent_artifact_control",
	} {
		if !strings.Contains(string(down), required) {
			t.Fatalf("down migration missing %q", required)
		}
	}
}
