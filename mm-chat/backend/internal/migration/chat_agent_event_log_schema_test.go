package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestChatAgentEventLogMigrationMatchesAppliedManifest(t *testing.T) {
	migrations, err := Load(migrationfiles.FS)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}

	const appliedChecksum = "f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042"
	for _, migration := range migrations {
		if migration.ID() != "096_chat_agent_event_log" {
			continue
		}
		if migration.Checksum != appliedChecksum {
			t.Fatalf("migration 096 checksum = %q, want applied manifest %q", migration.Checksum, appliedChecksum)
		}
		return
	}

	t.Fatal("migration 096 is not embedded")
}

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
		"ON CONFLICT (event_id) DO NOTHING",
		"UPDATE messages",
		"conversation_id = v_turn.conversation_id",
		"FOR UPDATE;",
		"SET next_sequence = next_sequence + 1",
		"error_code = COALESCE(error_code, 'AGENT_RUN_INTERRUPTED')",
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

func TestChatAgentEventLogFunctionRepairIsForwardOnlyAndLeastPrivilege(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("099_chat_agent_event_log_function_repair.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("099_chat_agent_event_log_function_repair.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)

	for _, required := range []string{
		"CREATE OR REPLACE FUNCTION chat_agent_start_turn(",
		"CREATE OR REPLACE FUNCTION chat_agent_append_event(",
		"ON CONFLICT ON CONSTRAINT chat_agent_events_pkey DO NOTHING",
		"UPDATE messages AS message",
		"error_code = COALESCE(message.error_code, 'AGENT_RUN_INTERRUPTED')",
		"message.conversation_id = v_turn.conversation_id",
		"SET search_path TO %I, pg_catalog, pg_temp",
		"FROM PUBLIC, go_api_runtime",
		"TO go_api_runtime",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("099 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE schema_migrations",
		"DELETE FROM schema_migrations",
		"CREATE TABLE chat_agent_",
		"DROP FUNCTION chat_agent_",
		"GRANT INSERT ON chat_agent_",
		"GRANT UPDATE ON chat_agent_",
		"GRANT DELETE ON chat_agent_",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("099 migration contains forbidden contract %q", forbidden)
		}
	}
	if got := strings.Count(up, "SECURITY DEFINER"); got != 2 {
		t.Fatalf("099 SECURITY DEFINER count = %d, want 2", got)
	}
	if !strings.Contains(down, "intentionally keeps the known-good function bodies") ||
		strings.Contains(strings.ToUpper(down), "DROP FUNCTION") {
		t.Fatal("099 down migration must be a forward-only no-op")
	}
}
