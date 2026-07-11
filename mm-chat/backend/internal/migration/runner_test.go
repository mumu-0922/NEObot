package migration

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

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

func testEmbeddedMigrations() fs.FS {
	return migrationfiles.FS
}
