package chat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/skillsupply"
	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestChatToolRegistryOrdersAuthorityAndCarriesExecutionPolicy(t *testing.T) {
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(t.TempDir(), "skills"),
		WorkspaceRoot: t.TempDir(), ShellPath: "/bin/sh",
		ApprovalMode: localskills.ApprovalSmart, CallTimeout: 2 * time.Second,
		RunTimeout: 5 * time.Second, MaxOutput: 4096, MaxCalls: 4,
		MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	localRuntime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Description: "fixture",
	}})
	memoryRuntime := &memoryToolRuntime{
		Searcher:           &memoryToolTestSearcher{},
		ConversationID:     "11111111-1111-4111-8111-111111111111",
		AssistantMessageID: "22222222-2222-4222-8222-222222222222",
		Query:              "what did I save?", forceFirstCall: true,
	}
	registry := newChatToolRegistry(externalWebToolLoopInput{
		Memory: memoryRuntime,
		Knowledge: &knowledgeToolRuntime{
			SelectedCollectionIDs: []string{"33333333-3333-4333-8333-333333333333"},
		},
		LocalSkills: localRuntime,
	})
	definitions := registry.definitions(1)
	want := []string{
		"search_memory", "search_knowledge", "skill", "file_read", "file_write",
		"file_edit", "file_search", "job_list", "job_output", "job_kill", "terminal",
	}
	if len(definitions) != len(want) {
		t.Fatalf("definitions=%#v", definitions)
	}
	for index, name := range want {
		if definitions[index].Function.Name != name {
			t.Fatalf("definition[%d]=%q want %q", index, definitions[index].Function.Name, name)
		}
	}
	if secondStep := registry.definitions(2); len(secondStep) != 10 ||
		secondStep[0].Function.Name != searchKnowledgeToolName {
		t.Fatalf("second-step definitions=%#v", secondStep)
	}
	terminal, ok := registry.lookup(localTerminalToolName)
	if !ok || terminal.RiskClass != chatToolRiskExecute ||
		terminal.ApprovalRule != chatToolApprovalLocalPolicy ||
		terminal.Timeout != 2*time.Second || terminal.MaxOutputBytes != 4096 ||
		terminal.ProjectForModel == nil {
		t.Fatalf("terminal registration=%#v", terminal)
	}
	skill, ok := registry.lookup(localSkillToolName)
	if !ok || skill.RiskClass != chatToolRiskRead ||
		skill.ApprovalRule != chatToolApprovalNone {
		t.Fatalf("skill registration=%#v", skill)
	}
	if _, ok := registry.lookup(legacySkillViewToolName); !ok {
		t.Fatal("legacy Skill continuation registration is missing")
	}
	fileWrite, ok := registry.lookup(localFileWriteToolName)
	if !ok || fileWrite.RiskClass != chatToolRiskWrite ||
		!fileWrite.MutationResultNeedsFollowup {
		t.Fatalf("file_write registration=%#v", fileWrite)
	}
	for _, name := range []string{
		localFileReadToolName, localFileSearchToolName,
		localJobListToolName, localJobOutputToolName, legacySkillViewToolName,
	} {
		registration, ok := registry.lookup(name)
		if !ok || !registration.AllowParallel {
			t.Fatalf("safe read %q registration=%#v / %v", name, registration, ok)
		}
	}
	for _, name := range []string{
		localSkillToolName, legacySkillsListToolName, localFileWriteToolName,
		localFileEditToolName, localJobKillToolName, localTerminalToolName,
	} {
		registration, ok := registry.lookup(name)
		if !ok || registration.AllowParallel {
			t.Fatalf("barrier Tool %q registration=%#v / %v", name, registration, ok)
		}
	}
	for _, forbidden := range []string{"subagent", "delegate_task"} {
		if _, ok := registry.lookup(forbidden); ok {
			t.Fatalf("default registry exposed forbidden Tool %q", forbidden)
		}
	}
}

func TestChatToolRegistryFailsClosedOnNameCollision(t *testing.T) {
	registry := &chatToolRegistry{
		byName: make(map[string]chatToolRegistration), colliding: make(map[string]struct{}),
	}
	definition := searchWebToolDefinition()
	registration := retrievalToolRegistration(
		definition, chatToolBackendWeb, chatToolRiskExternal, false,
	)
	if !registry.register(registration) || registry.register(registration) {
		t.Fatal("collision registration result is incorrect")
	}
	if _, ok := registry.lookup(searchWebToolName); ok || len(registry.definitions(1)) != 0 {
		t.Fatal("colliding Tool remained executable or model-visible")
	}
}

func TestRequiredLocalSkillRegistryExposesOnlyExactSkillLoader(t *testing.T) {
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
		Name: "fixture-skill", Description: "fixture",
	}})
	if _, err := runtime.prepareUserPrompt("use fixture-skill", "use fixture-skill"); err != nil {
		t.Fatal(err)
	}
	registry := newRequiredLocalSkillRegistry(runtime)
	definitions := registry.definitions(0)
	if len(definitions) != 1 || definitions[0].Function.Name != localSkillToolName {
		t.Fatalf("required definitions=%#v", definitions)
	}
	if _, ok := registry.lookup(localTerminalToolName); ok {
		t.Fatal("terminal leaked into required Skill registry")
	}
}

func TestChatToolRegistryContinuesAcrossSkillTerminalAndWebBackends(t *testing.T) {
	runtimeRoot := t.TempDir()
	skillRoot := filepath.Join(runtimeRoot, "fixture")
	if err := os.Mkdir(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	skillBody := []byte("---\nname: fixture-skill\ndescription: fixture\n---\nVerify with Tools.\n")
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), skillBody, 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: runtimeRoot, WorkspaceRoot: t.TempDir(),
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 4096,
		MaxCalls: 8, MaxRounds: 8, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	localRuntime := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "fixture-skill", Version: "1.0.0", Description: "fixture",
		RootPath: skillRoot, Files: []string{"SKILL.md"},
		PackageFingerprint: testRuntimeSkillFingerprint(map[string][]byte{
			"SKILL.md": skillBody,
		}),
	}})
	prompt, err := localRuntime.prepareUserPrompt(
		"use fixture-skill and verify online", "use fixture-skill and verify online",
	)
	if err != nil {
		t.Fatal(err)
	}
	search := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Registry", URL: "https://example.test/registry", Content: "verified",
	}}}}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "skill", Name: localSkillToolName, Arguments: `{"name":"fixture-skill"}`,
		}}},
		{
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "web", Name: searchWebToolName, Arguments: `{"query":"registry fixture"}`,
			}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "terminal", Name: localTerminalToolName,
				Arguments: `{"command":"printf registry-ok","skill":null,"workingDir":null,"timeoutSeconds":1}`,
			}},
		},
		{{Type: ProviderEventDelta, Delta: "registry complete"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: prompt, ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		}}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		MaxResults: 5, LocalSkills: localRuntime,
	})
	var answer strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			answer.WriteString(event.Delta)
		}
	}
	if answer.String() != "registry complete" || search.calls != 1 || len(provider.inputs) != 3 {
		t.Fatalf("answer/search/steps=%q/%d/%d", answer.String(), search.calls, len(provider.inputs))
	}
	results := provider.inputs[2].Continuation[1].Results
	if len(results) != 2 || results[0].CallID != "web" || results[1].CallID != "terminal" ||
		!strings.Contains(results[0].Content, "[W1]") ||
		!strings.Contains(results[1].Content, "registry-ok") {
		t.Fatalf("cross-backend results=%#v", results)
	}
}

func TestChatToolRegistryReturnsUnknownAndBadArgumentsToSameModel(t *testing.T) {
	search := &fakeWebSearchProvider{}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "unknown", Name: "missing_tool", Arguments: `{}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "bad-arguments", Name: searchWebToolName, Arguments: `{"query":`,
		}}},
		{{Type: ProviderEventDelta, Delta: "reported both failures"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "test failures", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		}}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
	})
	var answer strings.Builder
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			answer.WriteString(event.Delta)
		}
	}
	if answer.String() != "reported both failures" || search.calls != 0 ||
		len(provider.inputs) != 3 {
		t.Fatalf("answer/search/steps=%q/%d/%d", answer.String(), search.calls, len(provider.inputs))
	}
	first := provider.inputs[2].Continuation[0].Results[0]
	second := provider.inputs[2].Continuation[1].Results[0]
	if !first.IsError || !strings.Contains(first.Content, "unknown_tool") ||
		!second.IsError || !strings.Contains(second.Content, "invalid_arguments") {
		t.Fatalf("structured failures=%#v/%#v", first, second)
	}
}

func TestChatToolRegistryGlobalCallBudgetDeniesCallsWithoutSideEffects(t *testing.T) {
	calls := make([]ProviderEvent, 0, maxChatAgentToolCalls+1)
	for index := 0; index <= maxChatAgentToolCalls; index++ {
		calls = append(calls, ProviderEvent{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID:   "unknown-" + string(rune('a'+index%26)),
				Name: "missing_tool", Arguments: `{}`,
			},
		})
	}
	search := &fakeWebSearchProvider{}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		calls,
		{{Type: ProviderEventDelta, Delta: "budget reported"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "budget", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		}}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
	})
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	results := provider.inputs[1].Continuation[0].Results
	if len(results) != maxChatAgentToolCalls+1 || search.calls != 0 ||
		!strings.Contains(results[len(results)-1].Content, "turn_call_budget_exhausted") {
		t.Fatalf("result count/search/last=%d/%d/%#v", len(results), search.calls, results[len(results)-1])
	}
}
