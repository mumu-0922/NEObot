package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestSkillSupplyChainMigrationDefinesImmutableAuthorityAndFences(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("083_skill_supply_chain.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("083_skill_supply_chain.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, contract := range []string{
		"CREATE TABLE skill_package_versions",
		"package_fingerprint TEXT PRIMARY KEY",
		"runtime_bundle_fingerprint TEXT",
		"allowed_tools JSONB NOT NULL DEFAULT '[]'::jsonb",
		"capability_requests JSONB NOT NULL DEFAULT '[]'::jsonb",
		"package_bytes BETWEEN 1 AND 134217728",
		"CREATE TABLE skill_package_candidates",
		"UNIQUE (source_type, source_ref)",
		"UNIQUE (id, package_fingerprint)",
		"status <> 'admitted' OR admission_eligible",
		"reviewed_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL",
		"CREATE TABLE skill_installations",
		"user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"UNIQUE (user_id, skill_name)",
		"FOREIGN KEY (admission_id, package_fingerprint)",
		"CREATE FUNCTION enforce_skill_installation_admission()",
		"CREATE TRIGGER trg_skill_installation_admission",
		"GRANT SELECT, INSERT ON TABLE skill_package_versions TO go_api_runtime",
		"GRANT SELECT, INSERT ON TABLE skill_package_candidates TO go_api_runtime",
		"GRANT UPDATE (status, reviewed_by_user_id, review_reason, revision, updated_at)",
		"GRANT SELECT, INSERT, DELETE ON TABLE skill_installations TO go_api_runtime",
	} {
		if !strings.Contains(text, contract) {
			t.Fatalf("migration missing %q", contract)
		}
	}
	downText := string(down)
	for _, contract := range []string{
		"SKILL_SUPPLY_CHAIN_DOWN_DATA_EXISTS",
		"DROP TABLE IF EXISTS skill_installations",
		"DROP TABLE IF EXISTS skill_package_candidates",
		"DROP TABLE IF EXISTS skill_package_versions",
	} {
		if !strings.Contains(downText, contract) {
			t.Fatalf("down migration missing %q", contract)
		}
	}
}
