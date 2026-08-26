package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestDirectSkillInstallationsMigrationDefinesPrivateOwnerAuthority(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("107_direct_skill_installations.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("107_direct_skill_installations.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.ToLower(string(upBytes))
	down := strings.ToLower(string(downBytes))
	for _, required := range []string{
		"add column owner_user_id uuid references users(id) on delete cascade",
		"unique nulls not distinct (source_type, source_ref, owner_user_id)",
		"status = 'validated' and reviewed_by_user_id is null",
		"candidate.owner_user_id = new.user_id",
		"skill_installation_authority_required",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("107 up migration missing %q", required)
		}
	}
	for _, required := range []string{
		"direct_skill_installations_down_data_exists",
		"skill_installation_admission_required",
		"drop column owner_user_id",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("107 down migration missing %q", required)
		}
	}
}
