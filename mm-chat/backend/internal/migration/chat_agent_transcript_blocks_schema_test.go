package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestChatAgentTranscriptBlocksWidenEventAuthorityForwardOnly(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("101_chat_agent_transcript_blocks.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("101_chat_agent_transcript_blocks.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down := string(upBytes), string(downBytes)
	for _, required := range []string{
		"DROP CONSTRAINT chat_agent_events_type_allowed",
		"ADD CONSTRAINT chat_agent_events_type_allowed",
		"'context.injected'",
		"'assistant.chunk'",
		"'assistant.block.completed'",
		`"transcriptVersion":2`,
		"CREATE OR REPLACE FUNCTION chat_agent_append_event(",
		"SET search_path TO %I, pg_catalog, pg_temp",
		"), chat_agent_append_event(",
		") TO go_api_runtime",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("101 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE schema_migrations",
		"DELETE FROM schema_migrations",
		"GRANT INSERT ON chat_agent_",
		"GRANT UPDATE ON chat_agent_",
		"GRANT DELETE ON chat_agent_",
	} {
		if strings.Contains(strings.ToLower(up), strings.ToLower(forbidden)) {
			t.Fatalf("101 migration contains forbidden contract %q", forbidden)
		}
	}
	if !strings.Contains(down, "forward-only") || strings.Contains(strings.ToUpper(down), "DROP CONSTRAINT") {
		t.Fatal("101 down migration must retain the widened immutable event authority")
	}
}
