package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/skillsupply"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestLocalSkillToolLoopLoadsSkillRunsTerminalAndContinuesSameModel(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	skillRoot := filepath.Join(runtimeRoot, "fixture")
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	skillBody := []byte("---\nname: fixture-skill\ndescription: fixture\n---\nUse terminal to finish the fixture.\n")
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), skillBody, 0o600); err != nil {
		t.Fatal(err)
	}
	skill := skillsupply.RuntimeSkill{
		Name: "fixture-skill", Version: "1.0.0", Description: "Runs a fixture",
		RootPath: skillRoot, Files: []string{"SKILL.md"},
		PackageFingerprint: testRuntimeSkillFingerprint(map[string][]byte{
			"SKILL.md": skillBody,
		}),
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: runtimeRoot, WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 3 * time.Second, RunTimeout: 10 * time.Second,
		MaxOutput: 64 << 10, MaxCalls: 8, MaxRounds: 8, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{skill})
	prompt, err := runtime.prepareUserPrompt("run fixture", "Use fixture-skill to run fixture")
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "skill-call", Name: localSkillToolName,
			Arguments: `{"name":"fixture-skill"}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "terminal-call", Name: localTerminalToolName,
			Arguments: `{"command":"test -f \"$NEO_CHAT_ACTIVE_SKILL_ROOT/SKILL.md\"; printf executed > result.txt; printf terminal-ok","skill":"fixture-skill","workingDir":null,"timeoutSeconds":2}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "fixture complete"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:       prompt,
			ModelRef:     ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
			SystemPrompt: appendLocalSkillSystemInstruction("base", runtime),
		},
		LocalSkills: runtime,
	})
	var content strings.Builder
	executions := make([]ProviderToolExecutionEvent, 0, 4)
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.ToolExecution != nil {
			executions = append(executions, *event.ToolExecution)
		}
	}
	if content.String() != "fixture complete" || len(provider.inputs) != 3 ||
		len(provider.inputs[2].Continuation) != 2 {
		t.Fatalf("content=%q inputs=%#v", content.String(), provider.inputs)
	}
	first := provider.inputs[0]
	if first.ToolChoice != ProviderToolChoiceRequired || len(first.Tools) != 1 ||
		first.Tools[0].Function.Name != localSkillToolName || first.UseReasoning ||
		!first.DisableThinking {
		t.Fatalf("required Skill prelude=%#v", first)
	}
	terminalResult := provider.inputs[2].Continuation[1].Results[0]
	if terminalResult.IsError ||
		!strings.Contains(terminalResult.Content, `"stdout":"terminal-ok"`) ||
		strings.Contains(terminalResult.Content, "evidenceToolCallId") {
		t.Fatalf("terminal result=%#v", terminalResult)
	}
	resultFile, err := os.ReadFile(filepath.Join(workspace, "result.txt"))
	if err != nil || string(resultFile) != "executed" {
		t.Fatalf("workspace result=%q error=%v", resultFile, err)
	}
	if len(executions) != 4 {
		t.Fatalf("executions=%#v", executions)
	}
	runningTerminal := executions[2].Presentation
	completedTerminal := executions[3].Presentation
	if runningTerminal == nil || completedTerminal == nil ||
		runningTerminal.Card != "terminal" ||
		!strings.Contains(runningTerminal.Command, "printf executed") ||
		runningTerminal.CWD != "$NEO_CHAT_WORKSPACE" ||
		runningTerminal.ExitCode != nil || completedTerminal.ExitCode == nil ||
		*completedTerminal.ExitCode != 0 {
		t.Fatalf("terminal presentations=%#v / %#v", runningTerminal, completedTerminal)
	}
	encodedExecutions, err := json.Marshal(executions)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedExecutions), "printf executed") ||
		strings.Contains(string(encodedExecutions), "terminal-ok") ||
		!strings.Contains(string(encodedExecutions), `"mode":"local_direct"`) ||
		!strings.Contains(string(encodedExecutions), `"timeoutSeconds":2`) {
		t.Fatalf("unsafe or incomplete process events=%s", encodedExecutions)
	}
	if strings.Contains(provider.inputs[0].SystemPrompt, "Use terminal to finish") ||
		!strings.Contains(provider.inputs[0].SystemPrompt, "fixture-skill") ||
		!strings.Contains(provider.inputs[0].SystemPrompt, "python3") ||
		!strings.Contains(provider.inputs[0].SystemPrompt, "at most 3 seconds") ||
		!strings.Contains(provider.inputs[0].SystemPrompt, "runInBackground=true") {
		t.Fatalf("progressive prompt=%q", provider.inputs[0].SystemPrompt)
	}
}

func TestLocalSkillRuntimeDoesNotMaskTypedProviderStartupFailure(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "runtime"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{nil, nil},
		syncErrors: map[int]error{
			0: newProviderFailure(ProviderFailureUpstreamFailed, "private first detail"),
			1: newProviderFailure(ProviderFailureUpstreamFailed, "private second detail"),
		},
	}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "ordinary Agent task",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		LocalSkills: runtime,
	})
	var failure error
	for event := range events {
		if event.Error != nil {
			failure = event.Error
		}
	}
	var localFailure *localSkillRunFailure
	category, typed := ProviderFailureCategoryOf(failure)
	if len(provider.inputs) != 2 || errors.As(failure, &localFailure) || !typed ||
		category != ProviderFailureUpstreamFailed {
		t.Fatalf("inputs=%d local=%#v category=%q/%t failure=%v",
			len(provider.inputs), localFailure, category, typed, failure)
	}
}

func TestLocalTerminalStreamsTransientPresentationAndPersistsFinalSnapshot(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: runtimeRoot, WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 3 * time.Second, RunTimeout: 10 * time.Second,
		MaxOutput: 64 << 10, MaxCalls: 8, MaxRounds: 8, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	events := make(chan ProviderEvent, 16)
	result, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "terminal-live-call", Name: localTerminalToolName,
		Arguments: `{"command":"head -c 1024 /dev/zero | tr '\\0' x","skill":null,` +
			`"workingDir":null,"timeoutSeconds":2,"runInBackground":false}`,
	}, 1, 1)
	if err != nil || result.IsError {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	close(events)
	var transient *ProviderToolExecutionEvent
	var completed *ProviderToolExecutionEvent
	for event := range events {
		if event.ToolExecution == nil {
			continue
		}
		if event.ToolExecution.Transient {
			copy := *event.ToolExecution
			transient = &copy
		}
		if event.ToolExecution.Status == ProcessStepStatusCompleted {
			copy := *event.ToolExecution
			completed = &copy
		}
	}
	if transient == nil || transient.Presentation == nil ||
		len(transient.Presentation.Transcript) == 0 {
		t.Fatalf("transient=%#v", transient)
	}
	if completed == nil || completed.Transient || completed.Presentation == nil ||
		len(completed.Presentation.Transcript) != 1 ||
		len(completed.Presentation.Transcript[0].Content) != 1024 {
		t.Fatalf("completed=%#v", completed)
	}
}

func TestLocalTerminalNonzeroExitIsFailedButKeepsBoundedResult(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: t.TempDir(), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 64 << 10, MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	events := make(chan ProviderEvent, 8)
	result, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "terminal-nonzero", Name: localTerminalToolName,
		Arguments: `{"command":"printf missing-python >&2; exit 127","skill":null,` +
			`"workingDir":null,"timeoutSeconds":1,"runInBackground":false}`,
	}, 1, 1)
	if err != nil || !result.IsError ||
		!strings.Contains(result.Content, `"error":"nonzero_exit"`) ||
		!strings.Contains(result.Content, `"exitCode":127`) ||
		!strings.Contains(result.Content, `"stderr":"missing-python"`) ||
		strings.Contains(result.Content, "evidenceToolCallId") {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	close(events)
	var terminal *ProviderToolExecutionEvent
	for event := range events {
		if event.ToolExecution != nil && event.ToolExecution.CallStatus == "failed" {
			copy := *event.ToolExecution
			terminal = &copy
		}
	}
	if terminal == nil || terminal.Status != ProcessStepStatusFailed ||
		terminal.FailureCategory != "nonzero_exit" || terminal.Presentation == nil ||
		terminal.Presentation.ExitCode == nil || *terminal.Presentation.ExitCode != 127 ||
		len(terminal.Presentation.Transcript) != 1 ||
		terminal.Presentation.Transcript[0].Stream != "stderr" ||
		terminal.Presentation.Transcript[0].Content != "missing-python" {
		t.Fatalf("terminal=%#v", terminal)
	}
}

func TestLocalTerminalTimeoutIsFailedButKeepsResult(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: t.TempDir(), WorkspaceRoot: t.TempDir(),
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 64 << 10, MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	events := make(chan ProviderEvent, 8)
	result, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "terminal-timeout", Name: localTerminalToolName,
		Arguments: `{"command":"sleep 30","skill":null,"workingDir":null,` +
			`"timeoutSeconds":1,"runInBackground":false}`,
	}, 1, 1)
	if err != nil || !result.IsError ||
		!strings.Contains(result.Content, `"error":"timeout"`) ||
		!strings.Contains(result.Content, `"exitCode":124`) ||
		!strings.Contains(result.Content, `"timedOut":true`) {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	close(events)
	var terminal *ProviderToolExecutionEvent
	for event := range events {
		if event.ToolExecution != nil && event.ToolExecution.CallStatus == "failed" {
			copy := *event.ToolExecution
			terminal = &copy
		}
	}
	if terminal == nil || terminal.FailureCategory != "timeout" ||
		terminal.Presentation == nil || !terminal.Presentation.TimedOut ||
		terminal.Presentation.ExitCode == nil || *terminal.Presentation.ExitCode != 124 {
		t.Fatalf("terminal=%#v", terminal)
	}
}

func TestLocalSkillPreludePreservesMemoryAsFirstTaskRound(t *testing.T) {
	runtimeRoot := t.TempDir()
	skillRoot := filepath.Join(runtimeRoot, "fixture")
	if err := os.Mkdir(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("---\nname: fixture-skill\ndescription: fixture\n---\nUse saved Memory when asked.\n")
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: runtimeRoot, WorkspaceRoot: t.TempDir(),
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 4096,
		MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	localRuntime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Description: "Uses saved personal Memory",
		RootPath: skillRoot, Files: []string{"SKILL.md"},
		PackageFingerprint: testRuntimeSkillFingerprint(map[string][]byte{
			"SKILL.md": body,
		}),
	}})
	prompt, err := localRuntime.prepareUserPrompt(
		"fixture-skill：我喜欢喝什么？", "fixture-skill：我喜欢喝什么？",
	)
	if err != nil {
		t.Fatal(err)
	}
	searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
		Memories: []usermemory.Memory{{
			ID: "11111111-1111-4111-8111-111111111111", Revision: 1,
			ScopeType: "global", Type: "preference", Content: "用户喜欢喝茶。",
		}},
	}}
	memoryRuntime := &memoryToolRuntime{
		Searcher: searcher, ConversationID: "22222222-2222-4222-8222-222222222222",
		AssistantMessageID: "33333333-3333-4333-8333-333333333333",
		Query:              "我喜欢喝什么？", forceFirstCall: true,
	}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "skill", Name: localSkillToolName, Arguments: `{"name":"fixture-skill"}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "memory", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "你喜欢喝茶。"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: prompt, ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		Memory: memoryRuntime, LocalSkills: localRuntime,
	})
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	if searcher.calls != 1 || len(provider.inputs) != 3 ||
		provider.inputs[0].ToolChoice != ProviderToolChoiceRequired ||
		provider.inputs[0].Tools[0].Function.Name != localSkillToolName ||
		provider.inputs[1].ToolChoice != ProviderToolChoiceRequired ||
		provider.inputs[1].Tools[0].Function.Name != usermemory.HybridMemoryToolName {
		t.Fatalf("Skill/Memory task rounds=%#v searchCalls=%d", provider.inputs, searcher.calls)
	}
}

func TestLocalSkillToolDefinitionsAreOpenAIStrictCompatible(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture", Files: []string{"SKILL.md"},
	}})
	definitions := runtime.definitions()
	wantNames := []string{
		localSkillToolName, localFileReadToolName, localFileWriteToolName,
		localFileEditToolName, localFileSearchToolName, localJobListToolName,
		localJobOutputToolName, localJobKillToolName, localTerminalToolName,
	}
	if len(definitions) != len(wantNames) {
		t.Fatalf("definitions=%#v", definitions)
	}
	for index, name := range wantNames {
		if definitions[index].Function.Name != name {
			t.Fatalf("definition[%d]=%q want %q", index, definitions[index].Function.Name, name)
		}
	}
	for _, definition := range definitions {
		if !definition.Function.Strict {
			t.Fatalf("%s is not strict", definition.Function.Name)
		}
		properties, ok := definition.Function.Parameters["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s properties=%#v", definition.Function.Name, definition.Function.Parameters["properties"])
		}
		required, ok := definition.Function.Parameters["required"].([]string)
		if !ok || len(required) != len(properties) {
			t.Fatalf("%s required=%#v properties=%#v", definition.Function.Name, required, properties)
		}
		requiredSet := make(map[string]struct{}, len(required))
		for _, name := range required {
			requiredSet[name] = struct{}{}
		}
		for name := range properties {
			if _, exists := requiredSet[name]; !exists {
				t.Fatalf("%s property %q is not required", definition.Function.Name, name)
			}
		}
	}
	for _, field := range []struct {
		tool string
		name string
	}{
		{localTerminalToolName, "skill"},
		{localTerminalToolName, "workingDir"},
		{localTerminalToolName, "timeoutSeconds"},
	} {
		definition := definitions[0]
		for _, candidate := range definitions {
			if candidate.Function.Name == field.tool {
				definition = candidate
				break
			}
		}
		properties := definition.Function.Parameters["properties"].(map[string]any)
		schema := properties[field.name].(map[string]any)
		types, ok := schema["type"].([]string)
		if !ok || !containsLocalSkillSchemaType(types, "null") {
			t.Fatalf("%s.%s is not nullable: %#v", field.tool, field.name, schema)
		}
	}
	terminal := definitions[len(definitions)-1]
	timeoutSchema := terminal.Function.Parameters["properties"].(map[string]any)["timeoutSeconds"].(map[string]any)
	if maximum, ok := timeoutSchema["maximum"].(int); !ok || maximum != 1 {
		t.Fatalf("terminal timeout schema=%#v", timeoutSchema)
	}
}

func containsLocalSkillSchemaType(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestLocalSkillToolRejectsTraversalAndDestructiveTerminal(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 4096, MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture", RootPath: filepath.Join(workspace, ".skills", "fixture"),
		Files: []string{"SKILL.md"},
	}})
	for _, call := range []ProviderToolCall{
		{ID: "view", Name: legacySkillViewToolName, Arguments: `{"name":"missing","path":"../SKILL.md"}`},
		{ID: "terminal", Name: localTerminalToolName, Arguments: `{"command":"rm -rf ./build"}`},
	} {
		events := make(chan ProviderEvent, 4)
		result, err := runtime.execute(context.Background(), events, call, 1, 1)
		if err != nil || !result.IsError {
			t.Fatalf("call=%#v result=%#v error=%v", call, result, err)
		}
		if call.Name == localTerminalToolName && !strings.Contains(result.Content, "approval_required") {
			t.Fatalf("destructive result=%s", result.Content)
		}
	}
}

func TestLocalWorkspaceToolsStayEnabledWithoutInstalledSkills(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	prompt := runtime.promptInstruction()
	definitions := runtime.definitions()
	if !runtime.enabled() || runtime.skillsAvailable() || len(definitions) != 8 ||
		definitions[0].Function.Name != localFileReadToolName ||
		definitions[7].Function.Name != localTerminalToolName ||
		!strings.Contains(prompt, `"replacement":true`) ||
		!strings.Contains(prompt, `"tombstone":true`) ||
		!strings.Contains(prompt, `"skills":[]`) {
		t.Fatalf("empty catalog workspace runtime=%#v definitions=%#v", runtime, definitions)
	}
}

func TestLocalWorkspaceFileToolsRejectStaleWriteAndReadBack(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, "skills"), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 64 << 10,
		MaxCalls: 8, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	events := make(chan ProviderEvent, 16)
	write, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "write", Name: localFileWriteToolName,
		Arguments: `{"path":"fixture.txt","content":"first","expectedVersion":"absent"}`,
	}, 1, 1)
	if err != nil || write.IsError || !strings.Contains(write.Content, `"version":"sha256:`) {
		t.Fatalf("write=%#v error=%v", write, err)
	}
	if strings.Contains(write.Content, "evidenceToolCallId") {
		t.Fatalf("write result leaked retired completion evidence: %s", write.Content)
	}
	stale, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "stale", Name: localFileWriteToolName,
		Arguments: `{"path":"fixture.txt","content":"second","expectedVersion":"absent"}`,
	}, 2, 2)
	if err != nil || !stale.IsError || !strings.Contains(stale.Content, "version_conflict") {
		t.Fatalf("stale=%#v error=%v", stale, err)
	}
	read, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "read", Name: localFileReadToolName,
		Arguments: `{"path":"fixture.txt","offset":null,"limit":null}`,
	}, 3, 3)
	if err != nil || read.IsError || !strings.Contains(read.Content, `"content":"first"`) {
		t.Fatalf("read=%#v error=%v", read, err)
	}
}

func TestBoundWorkspaceWriteCreatesFileReferenceAndHidesPublishFallback(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, "skills"), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 64 << 10,
		MaxCalls: 8, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	runtime.bindArtifactPublisher(&fakeWorkspaceArtifactPublisher{}, 1<<20)
	runtime.bindWorkspace("0198ca9a-81c6-7c8d-9444-b16da02de9b4")
	if runtime.handles(localPublishFileToolName) ||
		strings.Contains(runtime.promptInstruction(), publishFileSystemInstruction) {
		t.Fatal("bound Workspace exposed publish_file compatibility fallback")
	}
	for _, definition := range runtime.definitions() {
		if definition.Function.Name == localPublishFileToolName {
			t.Fatal("bound Workspace advertised publish_file")
		}
	}
	write, err := runtime.execute(context.Background(), make(chan ProviderEvent, 4), ProviderToolCall{
		ID: "write-bound", Name: localFileWriteToolName,
		Arguments: `{"path":"reports/result.txt","content":"ready","expectedVersion":"absent"}`,
	}, 1, 1)
	if err != nil || write.IsError {
		t.Fatalf("write=%#v error=%v", write, err)
	}
	blocks := runtime.workspaceFileOutputBlocks("message-1")
	if len(blocks) != 1 {
		t.Fatalf("workspace blocks=%#v", blocks)
	}
	block, ok := blocks[0].(map[string]any)
	if !ok || block["type"] != "workspace_file" ||
		block["path"] != "reports/result.txt" ||
		block["workspaceId"] != "0198ca9a-81c6-7c8d-9444-b16da02de9b4" {
		t.Fatalf("workspace block=%#v", blocks[0])
	}
}

func TestLocalSkillCallBudgetReturnsFailureThenFinalContinuationWithoutTools(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 1,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture", Files: []string{"SKILL.md"},
	}})
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "first", Name: legacySkillsListToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "over-budget", Name: legacySkillsListToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "bounded final answer"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "list twice", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		LocalSkills: runtime,
	})
	var content strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
	}
	if content.String() != "bounded final answer" || len(provider.inputs) != 3 ||
		len(provider.inputs[2].Tools) != 0 || len(provider.inputs[2].Continuation) != 2 {
		t.Fatalf("content=%q inputs=%#v", content.String(), provider.inputs)
	}
	budgetResult := provider.inputs[2].Continuation[1].Results[0]
	if !budgetResult.IsError || !strings.Contains(budgetResult.Content, "budget_exhausted") {
		t.Fatalf("budget result=%#v", budgetResult)
	}
}

func TestCompletionDrivenAgentIgnoresAbsoluteLocalBudgetsAndBlocksNoProgress(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 1,
		MaxRounds: 1, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture", Files: []string{"SKILL.md"},
	}})
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "first", Name: legacySkillsListToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "second", Name: legacySkillsListToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "third", Name: legacySkillsListToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "blocked status"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "repeat", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		LocalSkills: runtime, CompletionDriven: true,
	})
	var content strings.Builder
	var outcome *ProviderAgentOutcomeEvent
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.AgentOutcome != nil {
			value := *event.AgentOutcome
			outcome = &value
		}
	}
	if content.String() != "blocked status" || len(provider.inputs) != 4 ||
		len(provider.inputs[3].Tools) != 0 || runtime.calls != 3 {
		t.Fatalf("content=%q calls=%d inputs=%#v", content.String(), runtime.calls, provider.inputs)
	}
	if outcome == nil || outcome.Outcome != chatAgentOutcomeBlocked ||
		outcome.Reason != chatAgentBlockRepeatedToolOutcome {
		t.Fatalf("outcome=%#v", outcome)
	}
	for _, exchange := range provider.inputs[3].Continuation {
		for _, result := range exchange.Results {
			if result.IsError && strings.Contains(result.Content, "budget_exhausted") {
				t.Fatalf("completion-driven run hit absolute budget: %#v", result)
			}
		}
	}
}

func TestLocalSkillTerminalRejectsPackageDriftBeforeProcessStart(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	skillRoot := filepath.Join(runtimeRoot, "fixture")
	if err := os.Mkdir(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("canonical")
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	skill := skillsupply.RuntimeSkill{
		Name: "fixture", RootPath: skillRoot, Files: []string{"SKILL.md"},
		PackageFingerprint: testRuntimeSkillFingerprint(map[string][]byte{
			"SKILL.md": body,
		}),
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: runtimeRoot, WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 4096,
		MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "injected"), []byte("drift"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{skill})
	result, err := runtime.execute(context.Background(), make(chan ProviderEvent, 4), ProviderToolCall{
		ID: "terminal", Name: localTerminalToolName,
		Arguments: `{"command":"printf ran > marker","skill":"fixture"}`,
	}, 1, 1)
	if err != nil || !result.IsError || !strings.Contains(result.Content, "package_drift") {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "marker")); !os.IsNotExist(err) {
		t.Fatalf("drifted package command started: %v", err)
	}
}

func TestLocalSkillRoundBudgetUsesFinalContinuationWithoutTools(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 1, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture", Files: []string{"SKILL.md"},
	}})
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "first", Name: legacySkillsListToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "round-bounded answer"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "list", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		LocalSkills: runtime,
	})
	var content strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
	}
	if content.String() != "round-bounded answer" || len(provider.inputs) != 2 ||
		len(provider.inputs[1].Tools) != 0 || len(provider.inputs[1].Continuation) != 1 {
		t.Fatalf("content=%q inputs=%#v", content.String(), provider.inputs)
	}
}

func testRuntimeSkillFingerprint(files map[string][]byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("neo.skill-package/v1\x00"))
	for _, name := range []string{"SKILL.md"} {
		body := files[name]
		sum := sha256.Sum256(body)
		_, _ = fmt.Fprintf(
			digest,
			"%d:%s\x00%d\x00%s\n",
			len(name), name, len(body), hex.EncodeToString(sum[:]),
		)
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}
