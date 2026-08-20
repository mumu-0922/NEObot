package chat

import (
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/localskills"
)

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
