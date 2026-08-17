package localskills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadWorkspaceArtifactSupportsBinaryAndEnforcesWorkspaceBoundary(t *testing.T) {
	workspace := t.TempDir()
	executor, err := NewExecutor(Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh", ApprovalMode: ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 2 * time.Second, MaxOutput: 4096,
		MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "result"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte{0x00, 0x01, 0xfe, 0xff, 'P', 'N', 'G'}
	if err := os.WriteFile(filepath.Join(workspace, "result", "chart.bin"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	snapshot, err := executor.ReadWorkspaceArtifact(
		context.Background(), "result/chart.bin", int64(len(body)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Path != "result/chart.bin" || string(snapshot.Body) != string(body) ||
		snapshot.Version == "" {
		t.Fatalf("snapshot=%#v", snapshot)
	}

	if _, err := executor.ReadWorkspaceArtifact(
		context.Background(), "result/chart.bin", int64(len(body)-1),
	); !errors.Is(err, ErrWorkspaceFileTooLarge) {
		t.Fatalf("limit error=%v", err)
	}
	if _, err := executor.ReadWorkspaceArtifact(
		context.Background(), "../chart.bin", 1024,
	); !errors.Is(err, ErrWorkspaceInvalidInput) {
		t.Fatalf("traversal error=%v", err)
	}
	if _, err := executor.ReadWorkspaceArtifact(
		context.Background(), "result", 1024,
	); !errors.Is(err, ErrWorkspaceInvalidPath) {
		t.Fatalf("directory error=%v", err)
	}
	if err := os.Symlink("chart.bin", filepath.Join(workspace, "result", "alias.bin")); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.ReadWorkspaceArtifact(
		context.Background(), "result/alias.bin", 1024,
	); !errors.Is(err, ErrWorkspaceInvalidPath) {
		t.Fatalf("symlink error=%v", err)
	}
}
