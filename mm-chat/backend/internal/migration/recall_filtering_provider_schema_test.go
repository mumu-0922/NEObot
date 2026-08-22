package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestRecallFilteringProviderMigrationIsAdditiveAndReversible(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("105_recall_filtering_provider.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("105_recall_filtering_provider.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))

	for _, required := range []string{
		"alter table task_model_settings",
		"add column recall_filtering text not null default ''",
		"char_length(recall_filtering) <= 512",
		"add constraint task_model_settings_refs_bounded",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("105 up migration missing %q", required)
		}
	}
	for _, required := range []string{
		"drop column recall_filtering",
		"add constraint task_model_settings_refs_bounded",
		"char_length(memory) <= 512",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("105 down migration missing %q", required)
		}
	}
	if strings.Contains(down, "drop table task_model_settings") {
		t.Fatal("105 down migration must preserve existing task model settings")
	}
}
