package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAgentCronLearningActivationDefinesExactFunctionOnlyWorkers(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("094_agent_cron_learning_activation.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("094_agent_cron_learning_activation.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, required := range []string{
		"CREATE ROLE %I NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS",
		"'agent_cron_worker','agent_learning_worker'",
		"CREATE TABLE agent_cron_worker_targets(",
		"CREATE TABLE agent_learning_worker_targets(",
		"AGENT_WORKER_TARGET_IMMUTABLE",
		"CREATE FUNCTION agent_cron_worker_provision_target(",
		"CREATE FUNCTION agent_learning_worker_provision_target(",
		"CREATE FUNCTION agent_cron_worker_claim_due(",
		"JOIN agent_cron_templates template ON template.id=target.template_id",
		"FOR UPDATE OF template SKIP LOCKED",
		"CREATE FUNCTION agent_cron_worker_claim_triggers(",
		"FOR UPDATE OF trigger SKIP LOCKED",
		"CREATE FUNCTION agent_cron_worker_reconcile(",
		"CREATE FUNCTION agent_cron_worker_prune(",
		"CREATE TABLE agent_learning_runner_attempts(",
		"CREATE TABLE agent_learning_runner_results(",
		"CREATE TABLE agent_learning_runner_requests(",
		"method IN ('launch','result','cancel')",
		"caller_identity='spiffe://neo-chat/agent-runtime-draft-learning'",
		"CREATE FUNCTION agent_learning_worker_claim_checks(",
		"CREATE FUNCTION agent_learning_worker_begin_runner_check(",
		"CREATE FUNCTION agent_learning_worker_issue_runner_authority(",
		"CREATE FUNCTION agent_learning_worker_record_runner_result(",
		"CREATE FUNCTION agent_learning_worker_complete_runner_cleanup(",
		"CREATE FUNCTION agent_learning_worker_complete_checks(",
		"attempt.state='completed' AND attempt.cleanup_state='completed'",
		"CREATE FUNCTION agent_learning_worker_claim_cleanup(",
		"CREATE FUNCTION agent_learning_worker_reconcile(",
		"CREATE FUNCTION agent_learning_worker_prune_runner(",
		"GRANT EXECUTE ON FUNCTION\n  agent_cron_worker_get_target(TEXT)",
		"GRANT EXECUTE ON FUNCTION\n  agent_learning_worker_get_target(TEXT)",
		"REVOKE ALL ON agent_cron_worker_targets,agent_learning_worker_targets",
		"SET search_path TO %I, pg_catalog, pg_temp",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("094 up migration missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"GRANT SELECT ON agent_cron_worker_targets",
		"GRANT SELECT ON agent_learning_worker_targets",
		"GRANT INSERT ON agent_cron_worker_targets",
		"GRANT UPDATE ON agent_learning_worker_targets",
		"agent_learning_promote(TEXT,UUID",
		"agent_learning_reject(TEXT,UUID",
		"agent_learning_create_draft(TEXT,UUID",
		"agent_cron_claim_due(p_claim_owner",
		"agent_learning_claim_checks(p_owner",
		"GRANT EXECUTE ON FUNCTION agent_cron_worker_provision_target",
		"GRANT EXECUTE ON FUNCTION agent_learning_worker_provision_target",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("094 grants or uses forbidden worker authority %q", forbidden)
		}
	}

	for _, required := range []string{
		"AGENT_WORKER_DOWN_ACTIVE_TARGET",
		"AGENT_WORKER_DOWN_LIVE_CLAIM",
		"AGENT_WORKER_DOWN_UNRESOLVED_RUNNER_ATTEMPT",
		"AGENT_WORKER_DOWN_CLEANUP_PENDING",
		"AGENT_WORKER_DOWN_LOGIN_MEMBERSHIP",
		"AGENT_WORKER_DOWN_RETAINED_ACTIVATION_FACTS",
		"DROP FUNCTION agent_learning_worker_issue_runner_authority(",
		"DROP FUNCTION agent_cron_worker_claim_due(",
		"DROP TABLE agent_learning_runner_requests",
		"DROP TABLE agent_learning_runner_results",
		"DROP TABLE agent_learning_runner_attempts",
		"DROP TABLE agent_learning_worker_targets",
		"DROP TABLE agent_cron_worker_targets",
		"DROP ROLE agent_learning_worker",
		"DROP ROLE agent_cron_worker",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("094 down migration missing %q", required)
		}
	}
}
