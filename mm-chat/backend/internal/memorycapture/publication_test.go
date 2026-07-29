package memorycapture

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishArtifactsExclusiveCreatesPrivateCompleteBundle(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "artifacts")
	digests, err := PublishArtifactsExclusive(directory, []Artifact{
		{Name: "run-manifest.json", Body: []byte("manifest\n")},
		{Name: "baseline.json", Body: []byte("baseline\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(digests) != 2 || len(digests["run-manifest.json"]) != 64 {
		t.Fatalf("artifact digests = %#v", digests)
	}
	for _, name := range []string{"baseline.json", "run-manifest.json"} {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("artifact %q mode = %v/%v", name, info, err)
		}
	}
	if _, err := PublishArtifactsExclusive(directory, []Artifact{{
		Name: "baseline.json", Body: []byte("replacement\n"),
	}}); !errors.Is(err, ErrCaptureStateConflict) {
		t.Fatalf("replacement error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(directory, "baseline.json"))
	if err != nil || string(body) != "baseline\n" {
		t.Fatalf("existing artifact changed = %q/%v", body, err)
	}
}

func TestPublishArtifactsExclusivePreflightLeavesNoPartialBundle(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "candidate.json"), []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := PublishArtifactsExclusive(directory, []Artifact{
		{Name: "baseline.json", Body: []byte("baseline\n")},
		{Name: "candidate.json", Body: []byte("candidate\n")},
		{Name: "run-manifest.json", Body: []byte("manifest\n")},
	})
	if !errors.Is(err, ErrCaptureStateConflict) {
		t.Fatalf("PublishArtifactsExclusive() error = %v", err)
	}
	for _, name := range []string{"baseline.json", "run-manifest.json"} {
		if _, statErr := os.Stat(filepath.Join(directory, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("partial artifact %q exists: %v", name, statErr)
		}
	}
}

func TestPublishArtifactsExclusiveRejectsUnsafePathOrPermissions(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishArtifactsExclusive(directory, []Artifact{{
		Name: "../escape", Body: []byte("bad"),
	}}); err == nil {
		t.Fatal("unsafe artifact name was accepted")
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishArtifactsExclusive(directory, []Artifact{{
		Name: "safe.json", Body: []byte("bad"),
	}}); err == nil {
		t.Fatal("non-private output directory was accepted")
	}
}
