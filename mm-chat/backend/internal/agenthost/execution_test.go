package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

func TestHostExecutionRoundTripUsesPinnedWorkspace(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("host-project"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := newTestResolver(t)
	descriptor, err := resolver.ResolveWorkspace(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	manager := newTestExecutionManager(t, resolver)
	handler, err := NewHandler(HandlerConfig{
		RunnerID: "wsl-test-runner", Version: "test", Token: testToken,
		Resolver: resolver, Execution: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-test-runner")
	capabilities, err := client.Capabilities(context.Background())
	if err != nil || !capabilities.Features.Execution || len(capabilities.Features.PermissionModes) != 0 {
		t.Fatalf("execution capabilities = %+v, %v", capabilities.Features, err)
	}

	workspace := ExecutionWorkspace{
		CanonicalPath: descriptor.CanonicalPath, DirectoryFingerprint: descriptor.DirectoryFingerprint,
	}
	var terminal localskills.Result
	executeHostTool(t, client, ToolExecuteRequest{
		Workspace: workspace, Tool: ToolTerminal,
		Arguments: mustExecutionJSON(t, TerminalToolArguments{
			Command:        "test \"$PWD\" = " + shellSingleQuote(project) + " && printf host-terminal",
			TimeoutSeconds: 2,
		}),
	}, &terminal)
	if terminal.ExitCode != 0 || terminal.Stdout != "host-terminal" {
		t.Fatalf("terminal result = %+v", terminal)
	}

	var read localskills.FileReadResult
	executeHostTool(t, client, ToolExecuteRequest{
		Workspace: workspace, Tool: ToolFileRead,
		Arguments: mustExecutionJSON(t, localskills.FileReadRequest{Path: "README.md"}),
	}, &read)
	if read.Content != "host-project" || !strings.HasPrefix(read.Version, "sha256:") {
		t.Fatalf("file read = %+v", read)
	}

	var write localskills.FileWriteResult
	largeContent := strings.Repeat("x", 20<<10)
	executeHostTool(t, client, ToolExecuteRequest{
		Workspace: workspace, Tool: ToolFileWrite,
		Arguments: mustExecutionJSON(t, localskills.FileWriteRequest{
			Path: "created.txt", Content: largeContent, ExpectedVersion: localskills.WorkspaceVersionAbsent,
		}),
	}, &write)
	created, err := os.ReadFile(filepath.Join(project, "created.txt"))
	if err != nil || string(created) != largeContent || write.Path != "created.txt" {
		t.Fatalf("host write = %+v, %q, %v", write, created, err)
	}

	scope := ExecutionScope{UserID: "user-1", ConversationID: "conversation-1"}
	var started localskills.JobSnapshot
	executeHostTool(t, client, ToolExecuteRequest{
		Workspace: workspace, Scope: scope, Tool: ToolJobStart,
		Arguments: mustExecutionJSON(t, TerminalToolArguments{
			Command: "printf host-job", TimeoutSeconds: 2,
		}),
	}, &started)
	if started.ID == "" {
		t.Fatalf("job start = %+v", started)
	}
	var completed localskills.JobSnapshot
	executeHostTool(t, client, ToolExecuteRequest{
		Workspace: workspace, Scope: scope, Tool: ToolJobOutput,
		Arguments: mustExecutionJSON(t, struct {
			JobID       string        `json:"jobId"`
			Wait        bool          `json:"wait"`
			WaitTimeout time.Duration `json:"waitTimeout"`
		}{started.ID, true, time.Second}),
	}, &completed)
	if completed.Status != localskills.JobStatusCompleted || completed.Stdout != "host-job" {
		t.Fatalf("job output = %+v", completed)
	}
}

func TestExecutionManagerRejectsSymlinkedSkillAuthority(t *testing.T) {
	resolver := newTestResolver(t)
	skillsRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(skillsRoot, "escaped")); err != nil {
		t.Fatal(err)
	}
	manager, err := NewExecutionManager(resolver, ExecutionConfig{
		SkillsRoot: skillsRoot, ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 1 << 20, MaxConcurrent: 2,
	})
	if err != nil {
		t.Fatalf("NewExecutionManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if root, ok := manager.activeSkillRoot("escaped"); ok || root != "" {
		t.Fatalf("symlinked active Skill root accepted: %q", root)
	}

	symlinkRoot := filepath.Join(t.TempDir(), "skills-link")
	if err := os.Symlink(skillsRoot, symlinkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := NewExecutionManager(resolver, ExecutionConfig{
		SkillsRoot: symlinkRoot, ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 1 << 20, MaxConcurrent: 2,
	}); !errors.Is(err, ErrWorkspacePathInvalid) {
		t.Fatalf("symlinked Skills root error = %v", err)
	}
}

func TestExecutionValidationRejectsMalformedFingerprintAndJobScope(t *testing.T) {
	for _, fingerprint := range []string{
		"", "sha256:" + strings.Repeat("A", 64), "sha256:" + strings.Repeat("z", 64),
	} {
		if validExecutionFingerprint(fingerprint) {
			t.Fatalf("invalid fingerprint accepted: %q", fingerprint)
		}
	}
	if !validExecutionFingerprint("sha256:" + strings.Repeat("a", 64)) {
		t.Fatal("valid execution fingerprint rejected")
	}
	for _, scope := range []ExecutionScope{
		{},
		{UserID: "user", ConversationID: "conversation\nprivate"},
		{UserID: strings.Repeat("u", 129), ConversationID: "conversation"},
	} {
		if validExecutionScope(scope) {
			t.Fatalf("invalid execution scope accepted: %+v", scope)
		}
	}
	if !validExecutionScope(ExecutionScope{UserID: "user", ConversationID: "conversation"}) {
		t.Fatal("valid execution scope rejected")
	}
}

func TestHostExecutionRejectsWorkspaceDriftAndCancelsProcess(t *testing.T) {
	project := t.TempDir()
	resolver := newTestResolver(t)
	descriptor, err := resolver.ResolveWorkspace(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	manager := newTestExecutionManager(t, resolver)
	handler, err := NewHandler(HandlerConfig{
		RunnerID: "wsl-test-runner", Version: "test", Token: testToken,
		Resolver: resolver, Execution: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-test-runner")
	request := ToolExecuteRequest{
		Workspace: ExecutionWorkspace{
			CanonicalPath:        descriptor.CanonicalPath,
			DirectoryFingerprint: "sha256:" + strings.Repeat("0", 64),
		},
		Tool:      ToolTerminal,
		Arguments: mustExecutionJSON(t, TerminalToolArguments{Command: "printf forbidden"}),
	}
	var result localskills.Result
	if err := client.ExecuteTool(
		context.Background(), request, &result,
	); !errors.Is(err, localskills.ErrWorkspaceInvalidPath) {
		t.Fatalf("workspace drift error = %v", err)
	}

	request.Workspace.DirectoryFingerprint = descriptor.DirectoryFingerprint
	request.Arguments = mustExecutionJSON(t, TerminalToolArguments{Command: "sleep 30"})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := client.ExecuteTool(ctx, request, &result); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation error = %v", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("cancelled Host process did not return promptly")
	}
}

func newTestExecutionManager(t *testing.T, resolver WorkspaceResolver) *ExecutionManager {
	t.Helper()
	manager, err := NewExecutionManager(resolver, ExecutionConfig{
		SkillsRoot: t.TempDir(), ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 1 << 20, MaxConcurrent: 2,
	})
	if err != nil {
		t.Fatalf("NewExecutionManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

func executeHostTool(t *testing.T, client *Client, request ToolExecuteRequest, output any) {
	t.Helper()
	if err := client.ExecuteTool(context.Background(), request, output); err != nil {
		t.Fatalf("ExecuteTool(%s) error = %v", request.Tool, err)
	}
}

func mustExecutionJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
