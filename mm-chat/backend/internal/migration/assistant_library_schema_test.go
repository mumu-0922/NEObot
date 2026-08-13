package migration

import (
	"strings"
	"testing"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestAssistantLibraryMigrationDefinesAuthorityAndRevisionFences(t *testing.T) {
	up, err := migrationfiles.FS.ReadFile("082_assistant_library.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := migrationfiles.FS.ReadFile("082_assistant_library.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, contract := range []string{
		"CREATE TABLE assistant_library_entries",
		"user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE",
		"revision BIGINT NOT NULL DEFAULT 1",
		"required_tools JSONB NOT NULL DEFAULT '[]'::jsonb",
		"CREATE UNIQUE INDEX idx_assistant_library_user_store_unique",
		"WHERE source = 'lobehub'",
		"CREATE TABLE assistant_market_admissions",
		"CHECK (status IN ('admitted', 'rejected'))",
		"content_fingerprint ~ '^[0-9a-f]{64}$'",
		"TO go_api_runtime",
	} {
		if !strings.Contains(text, contract) {
			t.Fatalf("migration missing %q", contract)
		}
	}
	if strings.Contains(text, "UNIQUE NULLS NOT DISTINCT (user_id, source, source_identifier)") {
		t.Fatal("migration would limit each user to one custom Assistant")
	}
	if !strings.Contains(string(down), "DROP TABLE IF EXISTS assistant_library_entries") {
		t.Fatal("down migration does not remove assistant library")
	}
}
