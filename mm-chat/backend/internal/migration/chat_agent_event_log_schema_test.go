package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestChatAgentEventLogDefinesAppendOnlyDurableProjection(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("096_chat_agent_event_log.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("096_chat_agent_event_log.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, required := range []string{
		"CREATE TABLE chat_agent_turns (",
		"CREATE TABLE chat_agent_events (",
		"CONSTRAINT chat_agent_events_turn_sequence_unique UNIQUE (turn_id, sequence)",
		"CONSTRAINT chat_agent_events_turn_event_unique UNIQUE (turn_id, event_id)",
		"'turn.started', 'turn.ended'",
		"'goal.changed', 'goal.round.started'",
		"'context.replaced'",
		"CREATE TRIGGER trg_chat_agent_event_immutable",
		"Parent/user/conversation/message hard deletion owns the only removal",
		"CREATE FUNCTION chat_agent_start_turn(",
		"CREATE FUNCTION chat_agent_append_event(",
		"ON CONFLICT ON CONSTRAINT chat_agent_events_pkey DO NOTHING",
		"UPDATE messages AS message",
		"message.conversation_id = v_turn.conversation_id",
		"FOR UPDATE;",
		"SET next_sequence = next_sequence + 1",
		"error_code = COALESCE(message.error_code, 'AGENT_RUN_INTERRUPTED')",
		"REVOKE ALL ON chat_agent_turns, chat_agent_events FROM PUBLIC, go_api_runtime",
		"GRANT SELECT ON chat_agent_turns, chat_agent_events TO go_api_runtime",
		"GRANT EXECUTE ON FUNCTION chat_agent_start_turn(",
		"SET search_path TO %I, pg_catalog, pg_temp",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("096 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"GRANT INSERT ON chat_agent_",
		"GRANT UPDATE ON chat_agent_",
		"GRANT DELETE ON chat_agent_",
		"MAX(sequence)",
		"INSERT INTO agent_run_events",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("096 migration contains forbidden contract %q", forbidden)
		}
	}
	for _, required := range []string{
		"CHAT_AGENT_EVENT_LOG_DOWN_DATA_EXISTS",
		"DROP FUNCTION chat_agent_append_event(",
		"DROP TRIGGER trg_chat_agent_event_immutable ON chat_agent_events",
		"DROP TABLE chat_agent_events",
		"DROP TABLE chat_agent_turns",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("096 down migration missing %q", required)
		}
	}
}
