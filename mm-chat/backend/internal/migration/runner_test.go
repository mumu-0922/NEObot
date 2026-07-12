package migration

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestParsePhase15GovernanceMappingCanonicalizesAndRejectsUnknownFields(t *testing.T) {
	mapping, err := ParsePhase15GovernanceMapping([]byte(`{
  "profiles": [{
    "profileId": "00000000-0000-0000-0000-000000000002",
    "modelId": "jina-embeddings-v4",
    "profileContractHash": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }, {
    "profileId": "00000000-0000-0000-0000-000000000001",
    "modelId": "jina-embeddings-v3",
    "profileContractHash": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  }],
  "heads": []
}`))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := canonicalPhase15GovernanceMapping(mapping)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Phase15GovernanceMapping
	if err := json.Unmarshal([]byte(canonical), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Profiles[0].ProfileID != "00000000-0000-0000-0000-000000000001" {
		t.Fatalf("profiles were not canonicalized: %s", canonical)
	}

	_, err = ParsePhase15GovernanceMapping([]byte(
		`{"profiles":[],"heads":[],"credentials":"forbidden"}`,
	))
	if err == nil {
		t.Fatal("unknown field was accepted")
	}
}

func TestCanonicalPhase15GovernanceMappingNormalizesZeroValueToArrays(t *testing.T) {
	canonical, err := canonicalPhase15GovernanceMapping(Phase15GovernanceMapping{})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"profiles":[],"heads":[]}`
	if canonical != want {
		t.Fatalf("canonical zero-value mapping = %s, want %s", canonical, want)
	}
}

func TestCanonicalPhase15GovernanceMappingStillRejectsConstructedDuplicates(t *testing.T) {
	profile := Phase15GovernanceProfileMapping{
		ProfileID:           "00000000-0000-0000-0000-000000000001",
		ModelID:             "jina-embeddings-v4",
		ProfileContractHash: strings.Repeat("a", 64),
	}
	_, err := canonicalPhase15GovernanceMapping(Phase15GovernanceMapping{
		Profiles: []Phase15GovernanceProfileMapping{profile, profile},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate profileId") {
		t.Fatalf("constructed duplicate error = %v", err)
	}
}

func TestParsePhase15GovernanceMappingRejectsMalformedAndDuplicateValues(t *testing.T) {
	sentinel := "MAPPING-CONTENT-MUST-NOT-BE-LOGGED"
	tests := []string{
		`{"profiles":null,"heads":[]}`,
		`{"profiles":[],"heads":null}`,
		`{"profiles":[],"heads":[]}{}`,
		`{"profiles":[],"profiles":[],"heads":[]}`,
		`{"profiles":[{"profileId":"00000000-0000-0000-0000-000000000001","profileId":"00000000-0000-0000-0000-000000000002","modelId":"m","profileContractHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"heads":[]}`,
		`{"profiles":[{"profileId":"NOT-A-UUID","modelId":"m","profileContractHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"heads":[]}`,
		`{"profiles":[{"profileId":"00000000-0000-0000-0000-000000000001","modelId":" m ","profileContractHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}],"heads":[]}`,
		`{"profiles":[],"heads":[{"processor":"jina","endpointId":"hosted","modelId":"m"},{"processor":"jina","endpointId":"hosted","modelId":"m2"}]}`,
	}
	for _, input := range tests {
		if _, err := ParsePhase15GovernanceMapping([]byte(input)); err == nil {
			t.Errorf("accepted invalid mapping: %s", input)
		}
	}
	_, err := ParsePhase15GovernanceMapping([]byte(
		`{"profiles":[],"heads":[],"secret":"` + sentinel + `"}`,
	))
	if err == nil {
		t.Fatal("mapping with unknown secret field was accepted")
	}
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("mapping parse error leaked file content: %v", err)
	}
}

func TestLoadOrdersMigrationsByNumericVersion(t *testing.T) {
	files := fstest.MapFS{
		"010_add_widgets.up.sql":      {Data: []byte("SELECT 10;")},
		"010_add_widgets.down.sql":    {Data: []byte("SELECT -10;")},
		"002_create_users.up.sql":     {Data: []byte("SELECT 2;")},
		"002_create_users.down.sql":   {Data: []byte("SELECT -2;")},
		"001_initial_schema.up.sql":   {Data: []byte("SELECT 1;")},
		"001_initial_schema.down.sql": {Data: []byte("SELECT -1;")},
	}

	migrations, err := Load(files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got := []int64{migrations[0].Version, migrations[1].Version, migrations[2].Version}
	want := []int64{1, 2, 10}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("versions = %v, want %v", got, want)
		}
	}
	if migrations[0].ID() != "001_initial_schema" {
		t.Fatalf("first migration ID = %q", migrations[0].ID())
	}
}

func TestLoadChecksumCoversMigrationIdentityAndBothDirections(t *testing.T) {
	files := fstest.MapFS{
		"001_initial_schema.up.sql":   {Data: []byte("SELECT 1;")},
		"001_initial_schema.down.sql": {Data: []byte("SELECT -1;")},
	}
	loaded, err := Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded[0].Checksum) != 64 {
		t.Fatalf("checksum length = %d, want 64", len(loaded[0].Checksum))
	}
	original := loaded[0].Checksum

	files["001_initial_schema.down.sql"] = &fstest.MapFile{Data: []byte("SELECT -2;")}
	loaded, err = Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Checksum == original {
		t.Fatal("checksum did not change when down migration changed")
	}
}

func TestLoadRejectsInvalidFilename(t *testing.T) {
	files := fstest.MapFS{
		"001_initial_schema.up.sql":   {Data: []byte("SELECT 1;")},
		"001_initial_schema.down.sql": {Data: []byte("SELECT -1;")},
		"bad.sql":                     {Data: []byte("SELECT 0;")},
	}

	if _, err := Load(files); err == nil {
		t.Fatal("Load() error = nil, want invalid filename error")
	}
}

func TestLoadRequiresUpAndDownPair(t *testing.T) {
	files := fstest.MapFS{
		"001_initial_schema.up.sql": {Data: []byte("SELECT 1;")},
	}

	if _, err := Load(files); err == nil {
		t.Fatal("Load() error = nil, want missing down migration error")
	}
}

func TestEmbeddedMigrationsAvoidTransactionControl(t *testing.T) {
	migrations, err := Load(testEmbeddedMigrations())
	if err != nil {
		t.Fatalf("Load() embedded migrations error = %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("Load() embedded migrations = 0, want at least one")
	}

	for _, migration := range migrations {
		for _, filePath := range []string{migration.UpPath, migration.DownPath} {
			contents, err := fs.ReadFile(testEmbeddedMigrations(), filePath)
			if err != nil {
				t.Fatalf("read embedded migration %s: %v", filePath, err)
			}
			upper := strings.ToUpper(string(contents))
			for _, forbidden := range []string{"BEGIN;", "COMMIT;", "ROLLBACK;"} {
				if strings.Contains(upper, forbidden) {
					t.Fatalf("embedded migration %s contains %s; runner owns transactions", filePath, forbidden)
				}
			}
		}
	}
}

func TestPhase15SafeSearchPathPlacesPGTempLast(t *testing.T) {
	if !strings.Contains(
		phase15SafeSearchPathSQL,
		"quote_ident(current_schema()) || ', pg_catalog, pg_temp'",
	) {
		t.Fatalf("unsafe Phase 15 search_path hardening: %s", phase15SafeSearchPathSQL)
	}
}

func testEmbeddedMigrations() fs.FS {
	return migrationfiles.FS
}
