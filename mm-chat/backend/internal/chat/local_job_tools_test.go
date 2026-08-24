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

func TestBackgroundTerminalRequiresCompletedJobOutputForVerification(t *testing.T) {
	registry := &chatToolRegistry{
		ordered: make([]chatToolRegistration, 0, 2),
		byName:  map[string]chatToolRegistration{}, colliding: map[string]struct{}{},
	}
	registry.register(chatToolRegistration{
		Name: localTerminalToolName, Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskExecute, ProjectForModel: identityChatToolResult,
	})
	registry.register(chatToolRegistration{
		Name: localJobOutputToolName, Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskRead, ProjectForModel: identityChatToolResult,
	})
	policy := newChatCompletionPolicy()
	policy.observe(registry, []ProviderToolCall{{ID: "start", Name: localTerminalToolName}},
		[]ProviderToolResult{{CallID: "start", Name: localTerminalToolName,
			Content: `{"result":{"jobId":"job-1","status":"running"},"durability":"process_local"}`}})
	if _, err := policy.verify("start", "started"); err == nil {
		t.Fatal("background start became completion evidence")
	}
	policy.observe(registry, []ProviderToolCall{{ID: "running", Name: localJobOutputToolName}},
		[]ProviderToolResult{{CallID: "running", Name: localJobOutputToolName,
			Content: `{"result":{"jobId":"job-1","status":"running"}}`}})
	if _, err := policy.verify("running", "still running"); err == nil {
		t.Fatal("running output became completion evidence")
	}
	policy.observe(registry, []ProviderToolCall{{ID: "foreground", Name: localTerminalToolName}},
		[]ProviderToolResult{{CallID: "foreground", Name: localTerminalToolName,
			Content: `{"exitCode":0}`}})
	if _, err := policy.verify("foreground", "foreground command completed"); err == nil ||
		err.Error() != "verification_evidence_invalid" {
		t.Fatalf("foreground command verified a background Job: %v", err)
	}
	policy.observe(registry, []ProviderToolCall{{ID: "done", Name: localJobOutputToolName}},
		[]ProviderToolResult{{CallID: "done", Name: localJobOutputToolName,
			Content: `{"result":{"jobId":"job-1","status":"completed"}}`}})
	if _, err := policy.verify("done", "job completed successfully"); err != nil {
		t.Fatalf("completed output evidence error=%v", err)
	}
}

func TestBackgroundTerminalTracksEachPendingJobByExactID(t *testing.T) {
	registry := &chatToolRegistry{
		ordered: make([]chatToolRegistration, 0, 2),
		byName:  map[string]chatToolRegistration{}, colliding: map[string]struct{}{},
	}
	registry.register(chatToolRegistration{
		Name: localTerminalToolName, Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskExecute, ProjectForModel: identityChatToolResult,
	})
	registry.register(chatToolRegistration{
		Name: localJobOutputToolName, Backend: chatToolBackendLocalSkill,
		RiskClass: chatToolRiskRead, ProjectForModel: identityChatToolResult,
	})
	policy := newChatCompletionPolicy()
	for _, jobID := range []string{"job-1", "job-2"} {
		policy.observe(registry,
			[]ProviderToolCall{{ID: "start-" + jobID, Name: localTerminalToolName}},
			[]ProviderToolResult{{CallID: "start-" + jobID, Name: localTerminalToolName,
				Content: `{"result":{"jobId":"` + jobID + `","status":"running"}}`}},
		)
	}
	policy.observe(registry, []ProviderToolCall{{ID: "wrong", Name: localJobOutputToolName}},
		[]ProviderToolResult{{CallID: "wrong", Name: localJobOutputToolName,
			Content: `{"result":{"jobId":"job-3","status":"completed"}}`}})
	if _, err := policy.verify("wrong", "unrelated Job completed"); err == nil ||
		err.Error() != "verification_evidence_invalid" {
		t.Fatalf("unrelated Job evidence error=%v", err)
	}
	for index, jobID := range []string{"job-1", "job-2"} {
		callID := "done-" + jobID
		policy.observe(registry, []ProviderToolCall{{ID: callID, Name: localJobOutputToolName}},
			[]ProviderToolResult{{CallID: callID, Name: localJobOutputToolName,
				Content: `{"result":{"jobId":"` + jobID + `","status":"completed"}}`}})
		if _, err := policy.verify(callID, jobID+" completed"); err != nil {
			t.Fatalf("%s evidence error=%v", jobID, err)
		}
		if got := policy.requiresVerification(); got != (index == 0) {
			t.Fatalf("after %s requiresVerification=%v", jobID, got)
		}
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
