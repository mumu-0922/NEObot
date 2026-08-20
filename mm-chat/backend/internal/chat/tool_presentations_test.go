package chat

import (
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

func TestBackgroundTerminalUsesJobPresentation(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: t.TempDir(), WorkspaceRoot: t.TempDir(),
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 2 * time.Second,
		MaxOutput: 64 << 10, MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	runtime := newLocalSkillToolRuntime(executor, nil)
	presentation := runtime.localProcessPresentation(ProviderToolCall{
		Name: localTerminalToolName,
		Arguments: `{"command":"printf ok","skill":null,"workingDir":null,` +
			`"timeoutSeconds":1,"runInBackground":true}`,
	})
	if presentation == nil || presentation.Card != "job" ||
		presentation.Operation != "start" || !presentation.Background ||
		presentation.Command != "printf ok" {
		t.Fatalf("background presentation=%#v", presentation)
	}
}

func TestProcessPresentationTerminalOutputIsBoundedAndRedacted(t *testing.T) {
	trace := newProcessTrace("message-1")
	step := trace.add(ProcessStep{
		ID: "terminal-1", Kind: ProcessStepKindTool,
		Status: ProcessStepStatusCompleted, LabelKey: "process.tool",
		Detail: map[string]any{
			"toolName": localTerminalToolName, "mode": "local_direct",
		},
		Presentation: &ProcessStepPresentation{
			Version: 1, Card: "terminal", Command: "printf ok",
			Transcript: []ProcessTranscriptEntry{
				{Sequence: 1, Stream: "stdout", Content: "token=fixture-secret\nok"},
				{Sequence: 2, Stream: "stderr", Content: "Bearer abcdefghijklmnop"},
			},
		},
	})
	if step.Presentation == nil || len(step.Presentation.Transcript) != 2 {
		t.Fatalf("presentation=%#v", step.Presentation)
	}
	encoded := step.Presentation.Transcript[0].Content + step.Presentation.Transcript[1].Content
	if strings.Contains(encoded, "fixture-secret") || strings.Contains(encoded, "abcdefghijklmnop") ||
		!strings.Contains(encoded, "[REDACTED]") {
		t.Fatalf("transcript was not redacted: %q", encoded)
	}
}

func TestProcessPresentationTerminalOutputKeepsHeadAndTail(t *testing.T) {
	content := "HEAD" + strings.Repeat("x", maxProcessPresentationTextBytes) + "TAIL"
	entries, truncated := sanitizeProcessTranscript([]ProcessTranscriptEntry{
		{Sequence: 1, Stream: "stdout", Content: content},
	}, false)
	if !truncated || len(entries) != 2 || !strings.HasPrefix(entries[0].Content, "HEAD") ||
		!strings.HasSuffix(entries[1].Content, "TAIL") {
		t.Fatalf("entries=%d truncated=%v head=%q tail=%q", len(entries), truncated,
			entries[0].Content[:min(len(entries[0].Content), 8)],
			entries[len(entries)-1].Content[max(len(entries[len(entries)-1].Content)-8, 0):])
	}
	if len(entries[0].Content)+len(entries[1].Content) > maxProcessPresentationTextBytes {
		t.Fatalf("bounded bytes=%d", len(entries[0].Content)+len(entries[1].Content))
	}
}

func TestLocalAndMCPPresentersProjectOnlySafeFields(t *testing.T) {
	runtime := &localSkillToolRuntime{}
	file := runtime.localProcessPresentation(ProviderToolCall{
		Name: localFileEditToolName,
		Arguments: `{"path":"src/example.go","oldText":"old","newText":"new",` +
			`"replaceAll":false,"expectedVersion":"sha256:fixture"}`,
	})
	if file == nil || file.Card != "file" || file.Path != "src/example.go" ||
		!strings.Contains(file.Diff, "-old") || !strings.Contains(file.Diff, "+new") {
		t.Fatalf("file presentation=%#v", file)
	}

	terminal := completeLocalProcessPresentation(&ProcessStepPresentation{
		Version: 1, Card: "terminal", Command: "printf ok",
	}, localSkillSuccessResult(ProviderToolCall{ID: "call-1", Name: localTerminalToolName}, map[string]any{
		"exitCode": 0, "stdout": "ok", "stderr": "warn", "truncated": false,
	}))
	if terminal == nil || terminal.ExitCode == nil || *terminal.ExitCode != 0 ||
		len(terminal.Transcript) != 2 {
		t.Fatalf("terminal presentation=%#v", terminal)
	}

	browser := mcpProcessPresentation("Playwright", "browser_navigate")
	if browser.Card != "browser" || browser.Title != "Playwright" ||
		strings.Contains(browser.Summary, "http") {
		t.Fatalf("browser fallback=%#v", browser)
	}
	mcp := mcpProcessPresentation("Context7", "query-docs")
	if mcp.Card != "mcp" || mcp.Operation != "query-docs" {
		t.Fatalf("mcp fallback=%#v", mcp)
	}
}

func TestSearchPresentationDoesNotPersistExactQuery(t *testing.T) {
	presentation := searchProcessPresentation(searchWebToolName, "private exact query", "tavily", 3)
	if presentation.Query != "" || presentation.Provider != "tavily" || presentation.Count != 3 {
		t.Fatalf("search presentation=%#v", presentation)
	}
}

func TestTerminalResultPresentationKeepsExistingResultFacts(t *testing.T) {
	exitCode := 7
	presentation := terminalResultPresentation(&ProcessStepPresentation{
		Version: 1, Card: "terminal", Command: "exit 7",
	}, &localskills.Result{ExitCode: exitCode, Stdout: "done", Stderr: "warning"})
	if presentation == nil || presentation.ExitCode == nil || *presentation.ExitCode != exitCode {
		t.Fatalf("presentation=%#v", presentation)
	}
}

func TestBackgroundJobPresentationPersistsBoundedLifecycleMetadata(t *testing.T) {
	job := completeLocalProcessPresentation(&ProcessStepPresentation{
		Version: 1, Card: "job", Operation: "start", Command: "printf ok",
		CWD: "$NEO_CHAT_WORKSPACE", Background: true,
	}, localSkillSuccessResult(ProviderToolCall{ID: "call-1", Name: localTerminalToolName}, map[string]any{
		"result": map[string]any{
			"jobId": "job_0123456789abcdef0123456789abcdef", "status": "completed",
			"startedAt": "2026-08-20T10:00:00Z", "completedAt": "2026-08-20T10:00:01Z",
			"durationMillis": 1000, "exitCode": 0,
			"stdout": "token=fixture-secret\nok", "stderr": "",
		},
	}))
	if job == nil || job.JobID != "job_0123456789abcdef0123456789abcdef" ||
		job.JobStatus != "completed" || job.JobDurationMS != 1000 ||
		job.ExitCode == nil || *job.ExitCode != 0 || len(job.Transcript) != 1 {
		t.Fatalf("job presentation=%#v", job)
	}
	trace := newProcessTrace("message-job")
	step := trace.add(ProcessStep{
		ID: "message-job:tool:1", Kind: ProcessStepKindTool,
		Status: ProcessStepStatusCompleted, LabelKey: "process.tool",
		Detail: map[string]any{
			"toolName": localTerminalToolName, "mode": "local_direct",
			"durability": "process_local",
		},
		Presentation: job,
	})
	if step.Presentation == nil || step.Presentation.Card != "job" ||
		strings.Contains(step.Presentation.Transcript[0].Content, "fixture-secret") ||
		!strings.Contains(step.Presentation.Transcript[0].Content, "[REDACTED]") {
		t.Fatalf("sanitized job presentation=%#v", step.Presentation)
	}
}

func TestTerminalLiveOutputHoldsBackUnsafeTail(t *testing.T) {
	stable := terminalStableTranscript([]ProcessTranscriptEntry{
		{Sequence: 1, Stream: "stdout", Content: strings.Repeat("x", 128)},
	}, processReasoningStreamHoldbackBytes)
	if len(stable) != 1 || len(stable[0].Content) != 64 {
		t.Fatalf("stable=%#v", stable)
	}
}
