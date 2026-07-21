package migration

import "testing"

func TestTaskModelSettingsMigration(t *testing.T) {
	up := readPhase15SQL(t, "036_task_model_settings.up.sql")
	down := readPhase15SQL(t, "036_task_model_settings.down.sql")

	assertPhase15Fragments(t, up,
		"036 must persist automation task model references per owner",
		"create table task_model_settings",
		"user_id uuid primary key references users",
		"title_generation text not null default ''",
		"related_questions text not null default ''",
		"context_compression text not null default ''",
		"prompt_optimization text not null default ''",
		"rag_query text not null default ''",
		"memory text not null default ''",
		"char_length ( title_generation ) <= 512",
		"grant select , insert , update , delete on task_model_settings")
	assertPhase15Fragments(t, down,
		"036 rollback must remove only task model settings",
		"revoke select , insert , update , delete on task_model_settings",
		"drop table task_model_settings")
}
