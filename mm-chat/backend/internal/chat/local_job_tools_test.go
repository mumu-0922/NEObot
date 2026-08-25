package chat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

func TestLocalBackgroundJobToolsStartWaitNotifyAndEnforceScope(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, "skills"), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 64 << 10,
		MaxCalls: 16, MaxRounds: 8, MaxConcurrent: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	runtime.bindJobScope("user-1", "conversation-1")
	events := make(chan ProviderEvent, 16)
	started, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "start", Name: localTerminalToolName,
		Arguments: `{"command":"sleep 0.05; printf job-done","skill":null,"workingDir":null,"timeoutSeconds":null,"runInBackground":true}`,
	}, 1, 1)
	if err != nil || started.IsError {
		t.Fatalf("started=%#v error=%v", started, err)
	}
	var startPayload struct {
		Result localskills.JobSnapshot `json:"result"`
	}
	if err := json.Unmarshal([]byte(started.Content), &startPayload); err != nil ||
		startPayload.Result.ID == "" || startPayload.Result.Status != localskills.JobStatusRunning {
		t.Fatalf("start payload=%#v error=%v content=%s", startPayload, err, started.Content)
	}
	completed, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "output", Name: localJobOutputToolName,
		Arguments: `{"jobId":"` + startPayload.Result.ID + `","wait":true,"timeoutSeconds":1}`,
	}, 2, 2)
	if err != nil || completed.IsError || !strings.Contains(completed.Content, `"status":"completed"`) ||
		!strings.Contains(completed.Content, `"stdout":"job-done"`) {
		t.Fatalf("completed=%#v error=%v", completed, err)
	}
	notice := runtime.consumeJobCompletionPrompt()
	if !strings.Contains(notice, startPayload.Result.ID) ||
		!strings.Contains(notice, `"durability":"process_local"`) ||
		strings.Contains(notice, "job-done") {
		t.Fatalf("notice=%q", notice)
	}
	forged := newLocalSkillToolRuntime(executor, nil)
	forged.bindJobScope("user-2", "conversation-1")
	denied, err := forged.execute(context.Background(), make(chan ProviderEvent, 4), ProviderToolCall{
		ID: "forged", Name: localJobOutputToolName,
		Arguments: `{"jobId":"` + startPayload.Result.ID + `","wait":false,"timeoutSeconds":null}`,
	}, 1, 1)
	if err != nil || !denied.IsError || !strings.Contains(denied.Content, "job_not_found") {
		t.Fatalf("denied=%#v error=%v", denied, err)
	}

	seenProcessLocal := false
	close(events)
	for event := range events {
		if event.ToolExecution != nil && event.ToolExecution.Durability == "process_local" {
			seenProcessLocal = true
		}
	}
	if !seenProcessLocal {
		t.Fatal("process-local durability was not emitted")
	}
}

func TestBackgroundBashRegistersDeclaredWorkspaceFileOnCompletion(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, "skills"), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 64 << 10,
		MaxCalls: 8, MaxRounds: 8, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	runtime.bindJobScope("user-1", "conversation-1")
	runtime.bindWorkspace("workspace-1")
	started, err := runtime.execute(context.Background(), make(chan ProviderEvent, 8), ProviderToolCall{
		ID: "start-file", Name: localTerminalToolName,
		Arguments: `{"command":"printf generated > report.txt","skill":null,"workingDir":null,"timeoutSeconds":null,"runInBackground":true,"outputFiles":["report.txt"]}`,
	}, 1, 1)
	if err != nil || started.IsError {
		t.Fatalf("started=%#v error=%v", started, err)
	}
	var startPayload struct {
		Result localskills.JobSnapshot `json:"result"`
	}
	if err := json.Unmarshal([]byte(started.Content), &startPayload); err != nil {
		t.Fatal(err)
	}
	completed, err := runtime.execute(context.Background(), make(chan ProviderEvent, 8), ProviderToolCall{
		ID: "output-file", Name: localJobOutputToolName,
		Arguments: `{"jobId":"` + startPayload.Result.ID + `","wait":true,"timeoutSeconds":1}`,
	}, 2, 2)
	if err != nil || completed.IsError ||
		!strings.Contains(completed.Content, `"workspaceFiles"`) ||
		!strings.Contains(completed.Content, `"path":"report.txt"`) {
		t.Fatalf("completed=%#v error=%v", completed, err)
	}
	blocks := runtime.workspaceFileOutputBlocks("message-1")
	if len(blocks) != 1 {
		t.Fatalf("workspace output blocks=%#v", blocks)
	}
}

func TestBackgroundJobOutputUsesRuntimeBoundInsteadOfMetadataBound(t *testing.T) {
	result := localSkillSuccessResult(ProviderToolCall{
		ID: "large-job", Name: localJobOutputToolName,
	}, map[string]any{
		"result": map[string]any{
			"status": "completed", "stdout": strings.Repeat("x", maxLocalSkillToolResultMetadata),
		},
	})
	if result.IsError || strings.Contains(result.Content, "result_too_large") ||
		len(result.Content) <= maxLocalSkillToolResultMetadata {
		t.Fatalf("large bounded Job output was rejected: error=%t bytes=%d", result.IsError, len(result.Content))
	}
}
