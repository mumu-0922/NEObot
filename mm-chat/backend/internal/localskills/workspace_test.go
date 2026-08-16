package localskills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceFileReadReturnsCompleteVersionAndUTF8SafeWindow(t *testing.T) {
	workspace := t.TempDir()
	body := "甲乙丙丁\nsecond line\n"
	if err := os.WriteFile(filepath.Join(workspace, "fixture.txt"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	result, err := executor.ReadWorkspaceFile(context.Background(), FileReadRequest{
		Path: "fixture.txt", Offset: 1, Limit: 7,
	})
	if err != nil || result.Path != "fixture.txt" || result.Content != "乙丙" ||
		result.Offset != 3 || result.NextOffset != 9 || !result.Truncated ||
		result.Version != workspaceVersion([]byte(body)) || result.Size != len(body) {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestWorkspaceFileWriteRequiresVersionAndRejectsExternalDrift(t *testing.T) {
	workspace := t.TempDir()
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	created, err := executor.WriteWorkspaceFile(context.Background(), FileWriteRequest{
		Path: "nested/fixture.txt", Content: "first", ExpectedVersion: WorkspaceVersionAbsent,
	})
	if err != nil || created.Version != workspaceVersion([]byte("first")) || created.Size != 5 {
		t.Fatalf("created=%#v error=%v", created, err)
	}
	if _, err := executor.WriteWorkspaceFile(context.Background(), FileWriteRequest{
		Path: "nested/fixture.txt", Content: "blind", ExpectedVersion: WorkspaceVersionAbsent,
	}); !errors.Is(err, ErrWorkspaceVersionConflict) {
		t.Fatalf("blind overwrite error=%v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "nested", "fixture.txt"), []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.WriteWorkspaceFile(context.Background(), FileWriteRequest{
		Path: "nested/fixture.txt", Content: "stale", ExpectedVersion: created.Version,
	}); !errors.Is(err, ErrWorkspaceVersionConflict) {
		t.Fatalf("stale overwrite error=%v", err)
	}
	body, err := os.ReadFile(filepath.Join(workspace, "nested", "fixture.txt"))
	if err != nil || string(body) != "external" {
		t.Fatalf("body=%q error=%v", body, err)
	}
}

func TestWorkspaceFileEditRequiresUniqueTargetAndCurrentVersion(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "fixture.txt")
	if err := os.WriteFile(path, []byte("old old"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	version := workspaceVersion([]byte("old old"))
	if _, err := executor.EditWorkspaceFile(context.Background(), FileEditRequest{
		Path: "fixture.txt", OldText: "old", NewText: "new", ExpectedVersion: version,
	}); !errors.Is(err, ErrWorkspaceEditConflict) {
		t.Fatalf("ambiguous edit error=%v", err)
	}
	edited, err := executor.EditWorkspaceFile(context.Background(), FileEditRequest{
		Path: "fixture.txt", OldText: "old", NewText: "new", ReplaceAll: true,
		ExpectedVersion: version,
	})
	if err != nil || edited.Version != workspaceVersion([]byte("new new")) {
		t.Fatalf("edited=%#v error=%v", edited, err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "new new" {
		t.Fatalf("body=%q error=%v", body, err)
	}
}

func TestWorkspaceToolsRejectTraversalAbsoluteAndSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	for _, name := range []string{"../secret.txt", filepath.Join(outside, "secret.txt"), "escape/secret.txt"} {
		if _, err := executor.ReadWorkspaceFile(context.Background(), FileReadRequest{Path: name}); err == nil {
			t.Fatalf("read %q escaped workspace", name)
		}
	}
	if _, err := executor.WriteWorkspaceFile(context.Background(), FileWriteRequest{
		Path: "escape/new.txt", Content: "forbidden", ExpectedVersion: WorkspaceVersionAbsent,
	}); err == nil {
		t.Fatal("write escaped workspace")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside file exists: %v", err)
	}
}

func TestWorkspaceToolsRejectInRootSymlinkAliases(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "real"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "real", "file.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(workspace, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real/file.txt", filepath.Join(workspace, "file-alias.txt")); err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	for _, name := range []string{"alias/file.txt", "file-alias.txt"} {
		if _, err := executor.ReadWorkspaceFile(
			context.Background(), FileReadRequest{Path: name},
		); !errors.Is(err, ErrWorkspaceInvalidPath) {
			t.Fatalf("read %q error=%v", name, err)
		}
	}
	if _, err := executor.WriteWorkspaceFile(context.Background(), FileWriteRequest{
		Path: "alias/new.txt", Content: "forbidden", ExpectedVersion: WorkspaceVersionAbsent,
	}); !errors.Is(err, ErrWorkspaceInvalidPath) {
		t.Fatalf("write alias error=%v", err)
	}
	if _, err := executor.SearchWorkspaceFiles(context.Background(), FileSearchRequest{
		Path: "alias", Query: "inside",
	}); !errors.Is(err, ErrWorkspaceInvalidPath) {
		t.Fatalf("search alias error=%v", err)
	}
}

func TestWorkspaceSearchIsBoundedAndSkipsGeneratedAndSymlinkTrees(t *testing.T) {
	workspace := t.TempDir()
	for name, body := range map[string]string{
		"a.go":                   "package fixture\n// needle first\n",
		"nested/b.go":            "// needle second\n",
		"nested/other.txt":       "no match\n",
		"node_modules/hidden.js": "needle hidden\n",
	} {
		fullPath := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.go"), []byte("needle outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "linked")); err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	result, err := executor.SearchWorkspaceFiles(context.Background(), FileSearchRequest{
		Query: "needle", Glob: "*.go", MaxResults: 10,
	})
	if err != nil || result.Truncated || len(result.Matches) != 2 ||
		result.Matches[0].Path != "a.go" || result.Matches[0].Line != 2 ||
		result.Matches[1].Path != "nested/b.go" ||
		strings.Contains(result.Matches[0].Preview+result.Matches[1].Preview, "outside") {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	limited, err := executor.SearchWorkspaceFiles(context.Background(), FileSearchRequest{
		Query: "needle", Glob: "*.go", MaxResults: 1,
	})
	if err != nil || len(limited.Matches) != 1 || !limited.Truncated {
		t.Fatalf("limited=%#v error=%v", limited, err)
	}
}

func TestWorkspaceSearchBoundsEnumerationBeforeGlobFiltering(t *testing.T) {
	workspace := t.TempDir()
	for index := 0; index < MaxWorkspaceSearchFiles; index++ {
		name := filepath.Join(workspace, fmt.Sprintf("a-%04d.txt", index))
		if err := os.WriteFile(name, []byte("not selected\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "z.go"), []byte("needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := newTestExecutor(t, workspace, ApprovalSmart, 64<<10, 3*time.Second)
	result, err := executor.SearchWorkspaceFiles(context.Background(), FileSearchRequest{
		Query: "needle", Glob: "*.go", MaxResults: 10,
	})
	if err != nil || !result.Truncated || len(result.Matches) != 0 || result.FilesScanned != 0 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
