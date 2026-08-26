package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestSkillConversationSelectionsMigrationDefinesOwnerBoundCASAuthority(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("106_skill_conversation_selections.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("106_skill_conversation_selections.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))
	for _, required := range []string{
		"create table skill_conversation_selections",
		"conversation_id uuid primary key",
		"unique (conversation_id, user_id)",
		"foreign key (conversation_id, user_id)",
		"references conversations(id, user_id)",
		"create table skill_conversation_installations",
		"foreign key (conversation_id, user_id)",
		"references skill_conversation_selections(conversation_id, user_id)",
		"foreign key (installation_id, user_id, package_fingerprint)",
		"references skill_installations(id, user_id, package_fingerprint)",
		"grant select, insert, update, delete",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("106 up migration missing %q", required)
		}
	}
	for _, required := range []string{
		"drop table if exists skill_conversation_installations",
		"drop table if exists skill_conversation_selections",
		"drop constraint if exists skill_installations_id_user_fingerprint_unique",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("106 down migration missing %q", required)
		}
	}
}
