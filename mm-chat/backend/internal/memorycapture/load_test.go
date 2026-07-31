package memorycapture

import (
	"os"
	"path/filepath"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
)

func TestLoadProtectedRegressionRequiresExactGeneratorBytes(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		generate func() (memoryauthor.RegressionPool, error)
	}{
		{name: "v2", profile: memoryauthor.RegressionProfileID, generate: memoryauthor.GenerateRegression},
		{name: "v3", profile: memoryauthor.RegressionRepairedProfileID, generate: memoryauthor.GenerateRegressionV3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool, err := test.generate()
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(t.TempDir(), "native-regression")
			if err := memoryauthor.PublishRegression(root, pool); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadProtectedRegression(root)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Pool.Manifest.Generator.Profile != test.profile ||
				loaded.FixtureRawSHA256 != pool.Manifest.FixtureRawSHA256 ||
				loaded.CorpusRawSHA256 != pool.Manifest.CorpusRawSHA256 ||
				loaded.AuditRawSHA256 != pool.Manifest.AuditRawSHA256 ||
				len(loaded.ManifestRawSHA256) != 64 {
				t.Fatalf("protected regression = %#v", loaded)
			}

			corpusPath := filepath.Join(root, memoryauthor.RegressionCorpusFile)
			if err := os.WriteFile(
				corpusPath,
				append(append([]byte(nil), pool.CorpusJSON...), '\n'),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadProtectedRegression(root); err == nil {
				t.Fatal("raw byte drift was accepted")
			}
		})
	}
}

func TestLoadProtectedRegressionRejectsCrossSchemaArtifact(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "cross-schema-regression")
	if err := memoryauthor.PublishRegression(root, pool); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, memoryauthor.RegressionCorpusFile),
		pool.FixtureJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProtectedRegression(root); err == nil {
		t.Fatal("fixture schema was accepted as a regression corpus")
	}
}
