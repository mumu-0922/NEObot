package chat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/hostworkspace"
	"neo-chat/mm-chat/backend/internal/localskills"
)

type fakeHostWorkspaceExecutionService struct {
	status       hostworkspace.HostStatus
	binding      hostworkspace.ExecutionBinding
	lockErr      error
	executeErr   error
	lockCalls    int
	executeCalls int
	lastRequest  agenthost.ToolExecuteRequest
	result       localskills.Result
}

func (service *fakeHostWorkspaceExecutionService) HostStatus(context.Context) hostworkspace.HostStatus {
	return service.status
}

func (service *fakeHostWorkspaceExecutionService) LockConversationExecutionWorkspace(
	context.Context,
	string,
	string,
) (hostworkspace.ExecutionBinding, error) {
	service.lockCalls++
	return service.binding, service.lockErr
}

func (service *fakeHostWorkspaceExecutionService) ExecuteTool(
	_ context.Context,
	request agenthost.ToolExecuteRequest,
	output any,
) error {
	service.executeCalls++
	service.lastRequest = request
	if service.executeErr != nil {
		return service.executeErr
	}
	result, ok := output.(*localskills.Result)
	if !ok {
		return errors.New("unexpected result type")
	}
	*result = service.result
	return nil
}

func TestLocalToolExecutorForConversationKeepsLegacyLocalAndRoutesBoundHost(t *testing.T) {
	localExecutor := newHostRoutingLocalExecutor(t)
	binding := hostworkspace.ExecutionBinding{
		ConversationID: "conversation-1",
		WorkspaceID:    "workspace-1",
		RunnerID:       "wsl-test-runner",
		CanonicalPath:  "/mnt/d/project",
		DirectoryFingerprint: "sha256:" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	host := &fakeHostWorkspaceExecutionService{
		status: hostworkspace.HostStatus{
			Status: "ready", RunnerID: binding.RunnerID,
			Features: agenthost.HostFeatures{Execution: true},
		},
		binding: binding,
		result:  localskills.Result{ExitCode: 0, Stdout: "host-result"},
	}
	handler := &Handler{localSkillExecutor: localExecutor, hostWorkspaceService: host}

	legacy, err := handler.localToolExecutorForConversation(
		context.Background(), "conversation-legacy", "",
	)
	if err != nil || legacy != localExecutor || host.lockCalls != 0 {
		t.Fatalf("legacy executor = %T, err = %v, lock calls = %d", legacy, err, host.lockCalls)
	}

	bound, err := handler.localToolExecutorForConversation(
		context.Background(), binding.ConversationID, binding.WorkspaceID,
	)
	if err != nil {
		t.Fatalf("bound executor error = %v", err)
	}
	if _, ok := bound.(*hostWorkspaceExecutor); !ok || host.lockCalls != 1 {
		t.Fatalf("bound executor = %T, lock calls = %d", bound, host.lockCalls)
	}
	result, err := bound.Execute(context.Background(), localskills.Request{Command: "pwd"})
	if err != nil || result.Stdout != "host-result" || host.executeCalls != 1 ||
		host.lastRequest.Workspace.CanonicalPath != binding.CanonicalPath ||
		host.lastRequest.Workspace.DirectoryFingerprint != binding.DirectoryFingerprint {
		t.Fatalf("Host execution = %+v, %v, request = %+v", result, err, host.lastRequest)
	}
}

func TestBoundHostExecutionFailsClosedWithoutLocalWorkspaceMutation(t *testing.T) {
	localWorkspace := t.TempDir()
	localExecutor := newHostRoutingLocalExecutorAt(t, localWorkspace)
	binding := hostworkspace.ExecutionBinding{
		ConversationID: "conversation-1", WorkspaceID: "workspace-1",
		RunnerID: "wsl-test-runner", CanonicalPath: "/mnt/d/project",
		DirectoryFingerprint: "sha256:" +
			"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	host := &fakeHostWorkspaceExecutionService{
		status: hostworkspace.HostStatus{
			Status: "ready", RunnerID: binding.RunnerID,
			Features: agenthost.HostFeatures{Execution: true},
		},
		binding: binding, executeErr: agenthost.ErrHostUnavailable,
	}
	handler := &Handler{localSkillExecutor: localExecutor, hostWorkspaceService: host}
	executor, err := handler.localToolExecutorForConversation(
		context.Background(), binding.ConversationID, binding.WorkspaceID,
	)
	if err != nil {
		t.Fatalf("route bound executor: %v", err)
	}
	_, err = executor.Execute(context.Background(), localskills.Request{
		Command: "printf fallback > must-not-exist.txt",
	})
	if !errors.Is(err, agenthost.ErrHostUnavailable) {
		t.Fatalf("Host loss error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(localWorkspace, "must-not-exist.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("bound execution fell back to local workspace: %v", statErr)
	}
}

func TestLocalToolExecutorForConversationRejectsUnavailableOrChangedRunner(t *testing.T) {
	localExecutor := newHostRoutingLocalExecutor(t)
	binding := hostworkspace.ExecutionBinding{
		ConversationID: "conversation-1", WorkspaceID: "workspace-1",
		RunnerID: "wsl-old-runner", CanonicalPath: "/mnt/d/project",
		DirectoryFingerprint: "sha256:" +
			"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}
	tests := []struct {
		name    string
		service *fakeHostWorkspaceExecutionService
		wantErr error
	}{
		{
			name: "execution capability unavailable",
			service: &fakeHostWorkspaceExecutionService{status: hostworkspace.HostStatus{
				Status: "ready", RunnerID: binding.RunnerID,
			}},
			wantErr: errHostExecutionUnavailable,
		},
		{
			name: "runner identity changed",
			service: &fakeHostWorkspaceExecutionService{
				status: hostworkspace.HostStatus{
					Status: "ready", RunnerID: "wsl-new-runner",
					Features: agenthost.HostFeatures{Execution: true},
				},
				binding: binding,
			},
			wantErr: errHostWorkspaceBinding,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := &Handler{
				localSkillExecutor: localExecutor, hostWorkspaceService: test.service,
			}
			executor, err := handler.localToolExecutorForConversation(
				context.Background(), binding.ConversationID, binding.WorkspaceID,
			)
			if executor != nil || !errors.Is(err, test.wantErr) {
				t.Fatalf("executor = %T, error = %v", executor, err)
			}
		})
	}
}

func TestHostWorkspaceExecutorMapsSkillRootAndStablePresentation(t *testing.T) {
	runtimeRoot := t.TempDir()
	activeRoot := filepath.Join(runtimeRoot, "skill-a")
	if err := os.Mkdir(activeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	host := &fakeHostWorkspaceExecutionService{
		result: localskills.Result{ExitCode: 0, Stdout: "done"},
	}
	executor := newHostWorkspaceExecutor(host, hostworkspace.ExecutionBinding{
		RunnerID: "wsl-test-runner", CanonicalPath: "/mnt/d/private/project",
		DirectoryFingerprint: "sha256:" +
			"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
	}, localskills.Config{Enabled: true, RuntimeRoot: runtimeRoot})
	if config := executor.Config(); config.RuntimeMode != localskills.RuntimeHostWorkspace ||
		config.WorkspaceRoot != "" || config.WorkspaceHostRoot != "" {
		t.Fatalf("Host executor leaked local Workspace config: %+v", config)
	}
	result, err := executor.Execute(context.Background(), localskills.Request{
		Command:         "printf '%s' /mnt/d/private/project",
		WorkingDir:      "src",
		SkillsRoot:      runtimeRoot,
		ActiveSkillRoot: activeRoot,
	})
	if err != nil || result.Stdout != "done" || host.lastRequest.ActiveSkillRoot != "skill-a" {
		t.Fatalf("execution = %+v, %v, request = %+v", result, err, host.lastRequest)
	}
	command, cwd, ok := executor.TerminalPresentation(localskills.Request{
		Command:         "cat " + activeRoot + " /mnt/d/private/project/README.md",
		WorkingDir:      "src",
		SkillsRoot:      runtimeRoot,
		ActiveSkillRoot: activeRoot,
	}, false)
	if !ok || cwd != "$NEO_CHAT_WORKSPACE/src" ||
		command != "cat $NEO_CHAT_ACTIVE_SKILL_ROOT $NEO_CHAT_WORKSPACE/README.md" {
		t.Fatalf("presentation = %q, %q, %v", command, cwd, ok)
	}
}

func TestHostWorkspaceRuntimeEmitsHostModeWithDurableTerminalPresentation(t *testing.T) {
	host := &fakeHostWorkspaceExecutionService{
		result: localskills.Result{ExitCode: 0, Stdout: "host"},
	}
	executor := newHostWorkspaceExecutor(host, hostworkspace.ExecutionBinding{
		RunnerID: "wsl-test-runner", CanonicalPath: "/mnt/d/project",
		DirectoryFingerprint: "sha256:" +
			"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	}, localskills.Config{
		Enabled: true, RuntimeRoot: t.TempDir(),
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 1 << 20, MaxCalls: 8, MaxRounds: 4,
	})
	runtime := newLocalSkillToolRuntime(executor, nil)
	events := make(chan ProviderEvent, 4)
	result, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "call-1", Name: localTerminalToolName,
		Arguments: `{"command":"pwd","skill":null,"workingDir":null,"timeoutSeconds":null,"runInBackground":false}`,
	}, 1, 1)
	if err != nil || result.Content == "" {
		t.Fatalf("execute Host runtime = %+v, %v", result, err)
	}
	close(events)
	count := 0
	for event := range events {
		if event.ToolExecution == nil {
			continue
		}
		count++
		execution := event.ToolExecution
		if execution.Mode != localskills.RuntimeHostWorkspace || execution.Presentation == nil ||
			execution.Presentation.Card != "terminal" {
			t.Fatalf("Host execution event = %+v", execution)
		}
		if sanitized := sanitizeProcessStepPresentation(ProcessStepKindTool, map[string]any{
			"toolName": localTerminalToolName, "mode": execution.Mode,
		}, execution.Presentation); sanitized == nil {
			t.Fatal("Host terminal presentation was dropped by durable sanitizer")
		}
	}
	if count != 2 {
		t.Fatalf("Host execution event count = %d", count)
	}
	definitions := runtime.definitions()
	description := definitions[len(definitions)-1].Function.Description
	if description == "" || !strings.Contains(description, "Host Workspace") ||
		strings.Contains(description, "Backend user") {
		t.Fatalf("Host terminal description = %q", description)
	}
}

func newHostRoutingLocalExecutor(t *testing.T) *localskills.Executor {
	t.Helper()
	return newHostRoutingLocalExecutorAt(t, t.TempDir())
}

func newHostRoutingLocalExecutorAt(t *testing.T, workspace string) *localskills.Executor {
	t.Helper()
	runtimeRoot := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: runtimeRoot, WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 1 << 20, MaxCalls: 16, MaxRounds: 8, MaxConcurrent: 2,
	})
	if err != nil {
		t.Fatalf("NewExecutor() error = %v", err)
	}
	t.Cleanup(func() { _ = executor.Close() })
	return executor
}
