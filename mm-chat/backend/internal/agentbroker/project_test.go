package agentbroker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestProjectPatchRejectsUnsafePathsCollisionsAndBounds(t *testing.T) {
	for _, entries := range [][]ProjectPatchEntry{
		{{Path: "/etc/passwd", Operation: PatchWrite, Content: []byte("x")}},
		{{Path: "../secret", Operation: PatchWrite, Content: []byte("x")}},
		{{Path: "a\\b", Operation: PatchWrite, Content: []byte("x")}},
		{{Path: "a\x00b", Operation: PatchWrite, Content: []byte("x")}},
		{{Path: "Readme.md", Operation: PatchWrite, Content: []byte("x")}, {Path: "README.md", Operation: PatchDelete}},
		{{Path: "café.txt", Operation: PatchWrite, Content: []byte("x")}, {Path: "cafe\u0301.txt", Operation: PatchDelete}},
		{{Path: "link", Operation: "symlink", Content: []byte("target")}},
		{{Path: "large", Operation: PatchWrite, Content: []byte(strings.Repeat("x", int(maxPatchBytes+1)))}},
	} {
		if _, err := CanonicalProjectPatch("rev_01234567", entries); err == nil {
			t.Fatalf("unsafe patch accepted: %#v", entries)
		}
	}
}

func TestProjectCASFakeCommitsOnceAndNeverRebases(t *testing.T) {
	executor, _ := NewDeterministicCASFake("rev_01234567")
	patch, _ := CanonicalProjectPatch("rev_01234567", []ProjectPatchEntry{{Path: "src/a.txt", Operation: PatchWrite, Content: []byte("one")}})
	revision, err := executor.CommitPatch(context.Background(), "commit_0123456789abcdefghijkl", patch)
	if err != nil || revision == patch.BaseRevision {
		t.Fatalf("CommitPatch = %q, %v", revision, err)
	}
	replay, err := executor.CommitPatch(context.Background(), "commit_0123456789abcdefghijkl", patch)
	if err != nil || replay != revision {
		t.Fatalf("replay = %q, %v", replay, err)
	}
	conflict, _ := CanonicalProjectPatch("rev_01234567", []ProjectPatchEntry{{Path: "src/b.txt", Operation: PatchWrite, Content: []byte("two")}})
	if _, err := executor.CommitPatch(context.Background(), "commit_0123456789abcdefghijklm", conflict); !errors.Is(err, ErrProjectConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}
