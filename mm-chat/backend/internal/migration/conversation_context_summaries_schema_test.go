package migration

import "testing"

func TestConversationContextSummariesMigration(t *testing.T) {
	up := readPhase15SQL(t, "034_conversation_context_summaries.up.sql")
	down := readPhase15SQL(t, "034_conversation_context_summaries.down.sql")

	assertPhase15Fragments(t, up,
		"034 must persist versioned conversation summary boundaries",
		"create table conversation_context_summaries",
		"conversation_id uuid primary key references conversations",
		"source_first_message_id uuid not null references messages",
		"source_last_message_id uuid not null references messages",
		"source_digest ~ '^[0-9a-f]{64}$'",
		"octet_length ( summary ) <= 65536",
		"grant select , insert , update , delete on conversation_context_summaries")
	assertPhase15Fragments(t, down,
		"034 rollback must remove only the summary projection",
		"revoke select , insert , update , delete on conversation_context_summaries",
		"drop table conversation_context_summaries")
}
