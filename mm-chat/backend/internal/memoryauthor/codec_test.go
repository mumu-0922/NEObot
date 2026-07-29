package memoryauthor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictArtifactDecodingAndHashBinding(t *testing.T) {
	pool, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "duplicate",
			body: bytes.Replace(pool.FixtureJSON, []byte(`"schemaVersion":`), []byte(`"schemaVersion":"duplicate","schemaVersion":`), 1),
			want: "duplicate JSON object key",
		},
		{
			name: "unknown",
			body: bytes.Replace(pool.FixtureJSON, []byte(`"id":`), []byte(`"apiUrl":"http://forbidden.invalid","id":`), 1),
			want: "unknown field",
		},
		{name: "trailing", body: append(append([]byte(nil), pool.FixtureJSON...), []byte(`{}`)...), want: "trailing"},
		{
			name: "hash drift",
			body: bytes.Replace(pool.FixtureJSON, []byte("合成项目"), []byte("漂移项目"), 1),
			want: "content hash does not match",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeFixtureManifest(bytes.NewReader(test.body))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeFixtureManifest() error = %v, want %q", err, test.want)
			}
		})
	}
	oversized := bytes.Repeat([]byte(" "), maximumArtifactBytes+1)
	if _, err := DecodeCandidateManifest(bytes.NewReader(oversized)); err == nil ||
		!strings.Contains(err.Error(), "size") {
		t.Fatalf("oversized candidate manifest error = %v", err)
	}
}

func TestPublishLoadAndFormalPathPolicy(t *testing.T) {
	pool, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "v1")
	if err := PublishPool(root, pool); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPool(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.FixtureJSON, pool.FixtureJSON) || !bytes.Equal(loaded.GoldenJSON, pool.GoldenJSON) {
		t.Fatal("published pool bytes changed")
	}
	if err := PublishPool(root, pool); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second PublishPool() error = %v", err)
	}
	for _, relative := range []string{CandidateFixtureFile, CandidateGoldenFile, CandidateManifestFile} {
		info, err := os.Stat(filepath.Join(root, relative))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode/error = %v, %v", relative, info.Mode().Perm(), err)
		}
	}

	repository := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(repository, "mm-chat", "data", "memory-benchmark", "v1")
	if _, err := ValidateFormalRoot(allowed, repository); err != nil {
		t.Fatalf("allowed root rejected: %v", err)
	}
	for _, forbidden := range []string{
		filepath.Join(repository, "mm-chat", "backend", "fixtures"),
		filepath.Join(repository, "mm-chat", "secrets", "benchmark"),
		filepath.Join(repository, "mm-chat", "backup", "benchmark"),
	} {
		if _, err := ValidateFormalRoot(forbidden, repository); err == nil {
			t.Fatalf("forbidden root accepted: %s", forbidden)
		}
	}
	otherRepository := t.TempDir()
	if err := os.Mkdir(filepath.Join(otherRepository, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateFormalRoot(filepath.Join(otherRepository, "data", "v1"), repository); err == nil {
		t.Fatal("root inside another Git repository was accepted")
	}
	realParent := t.TempDir()
	symlinkParent := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(realParent, symlinkParent); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateFormalRoot(filepath.Join(symlinkParent, "v1"), repository); err == nil ||
		!strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink root error = %v", err)
	}
}

func TestLoadPoolRejectsTamperAndLoosePermissions(t *testing.T) {
	root := newTestPoolRoot(t)
	path := filepath.Join(root, CandidateManifestFile)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPool(root); err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("loose permission error = %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	body = bytes.Replace(body, []byte(ProfileSeed), []byte("2026072902"), 1)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPool(root); err == nil {
		t.Fatal("tampered candidate manifest was accepted")
	}
}
