package migration

import "testing"

func TestUserMemoriesMigration(t *testing.T) {
	up := readPhase15SQL(t, "035_user_memories.up.sql")
	down := readPhase15SQL(t, "035_user_memories.down.sql")

	assertPhase15Fragments(t, up,
		"035 must keep durable memory separate from conversation summaries",
		"create table user_memory_settings",
		"user_id uuid primary key references users",
		"enabled boolean not null default false",
		"auto_record_enabled boolean not null default false",
		"create table user_memories",
		"user_id uuid not null references users",
		"source_conversation_id uuid references conversations",
		"source_message_id uuid references messages",
		"deleted_at timestamptz",
		"create unique index idx_user_memories_active_content",
		"where deleted_at is null",
		"grant select , insert , update , delete on user_memory_settings",
		"grant select , insert , update , delete on user_memories")
	assertPhase15Fragments(t, down,
		"035 rollback must remove memory rows and settings only",
		"revoke select , insert , update , delete on user_memories",
		"revoke select , insert , update , delete on user_memory_settings",
		"drop table user_memories",
		"drop table user_memory_settings")
}
