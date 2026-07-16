package knowledge

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func knowledgeMigrationFSThrough(t *testing.T, maxVersion int64) fstest.MapFS {
	t.Helper()
	loaded, err := migration.Load(migrationfiles.FS)
	if err != nil {
		t.Fatal(err)
	}
	result := fstest.MapFS{}
	for _, migration := range loaded {
		if migration.Version > maxVersion {
			continue
		}
		for _, path := range []string{migration.UpPath, migration.DownPath} {
			data, err := fs.ReadFile(migrationfiles.FS, path)
			if err != nil {
				t.Fatal(err)
			}
			result[path] = &fstest.MapFile{Data: data}
		}
	}
	return result
}
