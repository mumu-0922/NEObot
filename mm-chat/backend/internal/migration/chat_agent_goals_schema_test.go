package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestChatAgentGoalsDefineCASRoundsLeastPrivilegeAndGuardedRollback(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("097_chat_agent_goals.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("097_chat_agent_goals.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, required := range []string{
		"CREATE TABLE chat_agent_goals (",
		"CHECK (phase IN ('active', 'paused', 'blocked', 'complete'))",
		"rounds_started >= 0 AND max_goal_rounds >= 3",
		"CREATE FUNCTION chat_agent_create_goal(",
		"CREATE FUNCTION chat_agent_change_goal(",
		"CREATE FUNCTION chat_agent_cancel_goal(",
		"CREATE FUNCTION chat_agent_start_goal_round(",
		"v_goal.revision <> p_expected_revision",
		"WHERE chat_agent_goals.phase = 'complete'",
		"p_round <> v_goal.rounds_started + 1",
		"'goal.changed'",
		"'goal.round.started'",
		"PERFORM 1 FROM chat_agent_append_event(",
		"SET search_path TO %I, pg_catalog, pg_temp",
		"REVOKE ALL ON chat_agent_goals FROM PUBLIC, go_api_runtime",
		"GRANT SELECT ON chat_agent_goals TO go_api_runtime",
		"GRANT EXECUTE ON FUNCTION chat_agent_create_goal(",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("097 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON chat_agent_goals",
		"GRANT UPDATE ON chat_agent_goals",
		"GRANT DELETE ON chat_agent_goals",
		"agent_run_events",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("097 migration contains forbidden contract %q", forbidden)
		}
	}
	for _, required := range []string{
		"CHAT_AGENT_GOALS_DOWN_DATA_EXISTS",
		"DROP FUNCTION chat_agent_start_goal_round(",
		"DROP FUNCTION chat_agent_cancel_goal(",
		"DROP FUNCTION chat_agent_change_goal(",
		"DROP FUNCTION chat_agent_create_goal(",
		"DROP TABLE chat_agent_goals",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("097 down migration missing %q", required)
		}
	}
}
