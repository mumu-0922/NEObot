package chat

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/resourceorchestrator"
	"neo-chat/mm-chat/backend/internal/skillsupply"
)

type resourceToolServiceProbe struct {
	searchResult resourceorchestrator.SearchResult
	installInput resourceorchestrator.InstallRequest
	installCalls int
	searchCalls  int
}

func (probe *resourceToolServiceProbe) Search(
	context.Context,
	string,
	string,
	string,
) (resourceorchestrator.SearchResult, error) {
	probe.searchCalls++
	return probe.searchResult, nil
}

func (probe *resourceToolServiceProbe) InstallExplicit(
	_ context.Context,
	input resourceorchestrator.InstallRequest,
) (resourceorchestrator.InstallResult, error) {
	probe.installCalls++
	probe.installInput = input
	return resourceorchestrator.InstallResult{
		Kind: input.Kind, ID: "installation-id", Name: "office-xlsx",
		Revision: input.ExactRevision, Status: "installed", RefreshRequired: true,
		MutationAuditID: "00000000-0000-4000-8000-000000000099",
	}, nil
}

func TestResourceToolRequiresSearchAndExactRevisionBeforeExplicitInstall(t *testing.T) {
	probe := &resourceToolServiceProbe{searchResult: resourceorchestrator.SearchResult{
		Kind: resourceorchestrator.KindSkill, Query: "excel",
		Items: []resourceorchestrator.SearchItem{{
			Kind: resourceorchestrator.KindSkill, ID: "candidate-id",
			Name: "office-xlsx", Version: "1.0.0", ExactRevision: "sha256:exact",
			Status: "admitted", PermissionScopes: []string{"write"},
		}},
	}}
	runtime := newResourceToolRuntime(
		probe, "user-id", "conversation-id", "帮我安装 Excel Skill",
	)
	events := make(chan ProviderEvent, 16)
	search, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "search", Name: resourceSearchToolName,
		Arguments: `{"kind":"skill","query":"excel","capability":"xlsx"}`,
	}, 1, 1)
	if err != nil || search.IsError || !strings.Contains(search.Content, "candidate-id") {
		t.Fatalf("search=%#v error=%v", search, err)
	}
	installed, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "install", Name: resourceRequestInstallToolName,
		Arguments: `{"kind":"skill","id":"candidate-id","version":"1.0.0",` +
			`"exactRevision":"sha256:exact","reason":"needed for xlsx"}`,
	}, 1, 2)
	if err != nil || installed.IsError || probe.installCalls != 1 ||
		probe.installInput.ExactRevision != "sha256:exact" ||
		probe.installInput.EntryPoint != "agent_tool" ||
		!strings.Contains(installed.Content, "server_refreshes_snapshot") {
		t.Fatalf("installed=%#v calls=%d input=%#v error=%v", installed, probe.installCalls, probe.installInput, err)
	}
}

func TestResourceInstallRefreshesProjectionBeforeNextProviderRound(t *testing.T) {
	probe := &resourceToolServiceProbe{searchResult: resourceorchestrator.SearchResult{
		Kind: resourceorchestrator.KindSkill, Query: "excel",
		Items: []resourceorchestrator.SearchItem{{
			Kind: resourceorchestrator.KindSkill, ID: "candidate-id",
			Name: "office-xlsx", Version: "1.0.0", ExactRevision: "sha256:exact",
			Status: "admitted",
		}},
	}}
	resourceRuntime := newResourceToolRuntime(
		probe, "user-id", "conversation-id", "安装 Excel Skill",
	)
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
	refreshedLocal := newLocalSkillToolRuntime(executor, []skillsupply.RuntimeSkill{{
		Name: "office-xlsx", Version: "1.0.0", Description: "Create Excel workbooks",
		PackageFingerprint: "sha256:exact",
	}})
	var refreshed *agentRuntimeResourceSnapshot
	resourceRuntime.bindRefresh(func(context.Context) (*agentRuntimeResourceSnapshot, error) {
		refreshed = newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{
			LocalSkills: refreshedLocal, Resource: resourceRuntime,
		})
		return refreshed, nil
	})
	initial := newAgentRuntimeResourceSnapshot(agentRuntimeResourceInput{
		Resource: resourceRuntime,
	})
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "search", Name: resourceSearchToolName,
				Arguments: `{"kind":"skill","query":"excel","capability":"xlsx"}`,
			}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "install", Name: resourceRequestInstallToolName,
				Arguments: `{"kind":"skill","id":"candidate-id","version":"1.0.0",` +
					`"exactRevision":"sha256:exact","reason":"needed for xlsx"}`,
			}},
		},
		{{Type: ProviderEventDelta, Delta: "ready"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "安装 Excel Skill", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		Resource: resourceRuntime, Resources: initial,
	})
	var content strings.Builder
	var executions []ProviderToolExecutionEvent
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
	if content.String() != "ready" || len(provider.inputs) != 2 {
		t.Fatalf("content=%q provider inputs=%d", content.String(), len(provider.inputs))
	}
	var skillExposed bool
	for _, definition := range provider.inputs[1].Tools {
		if definition.Function.Name == localSkillToolName {
			skillExposed = true
		}
	}
	if !skillExposed {
		t.Fatalf("second-round tools=%#v", provider.inputs[1].Tools)
	}
	continuation := provider.inputs[1].Continuation
	if refreshed == nil || refreshed.runRevision == initial.runRevision || len(continuation) != 1 ||
		!strings.Contains(continuation[0].FollowupPrompt, "resource_snapshot_refresh") ||
		!strings.Contains(continuation[0].FollowupPrompt, initial.runRevision) ||
		!strings.Contains(continuation[0].FollowupPrompt, refreshed.runRevision) {
		t.Fatalf("continuation=%#v", continuation)
	}
	var refreshTraced bool
	for _, execution := range executions {
		if execution.Name != resourceRequestInstallToolName || execution.Presentation == nil {
			continue
		}
		var previous, current, auditID string
		for _, item := range execution.Presentation.Items {
			switch item.Label {
			case "previous snapshot":
				previous = item.Detail
			case "current snapshot":
				current = item.Detail
			case "mutation audit":
				auditID = item.Detail
			}
		}
		refreshTraced = previous == initial.runRevision && current == refreshed.runRevision &&
			auditID == "00000000-0000-4000-8000-000000000099"
	}
	if !refreshTraced {
		t.Fatalf("refresh executions=%#v", executions)
	}
}

func TestResourceRefreshFailureStopsContinuation(t *testing.T) {
	probe := &resourceToolServiceProbe{searchResult: resourceorchestrator.SearchResult{
		Kind: resourceorchestrator.KindSkill, Query: "excel",
		Items: []resourceorchestrator.SearchItem{{
			Kind: resourceorchestrator.KindSkill, ID: "candidate-id",
			Name: "office-xlsx", Version: "1.0.0", ExactRevision: "sha256:exact",
			Status: "admitted",
		}},
	}}
	runtime := newResourceToolRuntime(probe, "user-id", "conversation-id", "安装 Excel Skill")
	runtime.bindRefresh(func(context.Context) (*agentRuntimeResourceSnapshot, error) {
		return nil, errors.New("refresh unavailable")
	})
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{{
		{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "search", Name: resourceSearchToolName,
			Arguments: `{"kind":"skill","query":"excel","capability":"xlsx"}`,
		}},
		{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "install", Name: resourceRequestInstallToolName,
			Arguments: `{"kind":"skill","id":"candidate-id","version":"1.0.0",` +
				`"exactRevision":"sha256:exact","reason":"needed for xlsx"}`,
		}},
	}}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "安装 Excel Skill", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		Resource: runtime,
	})
	var failure *chatAgentRunFailure
	for event := range events {
		if event.Error != nil {
			if typed, ok := event.Error.(*chatAgentRunFailure); ok {
				failure = typed
			}
		}
	}
	if failure == nil || failure.code != "RESOURCE_REFRESH_FAILED" || len(provider.inputs) != 1 {
		t.Fatalf("failure=%#v provider inputs=%d", failure, len(provider.inputs))
	}
}

func TestResourceToolRejectsUnsearchedAndAgentInitiatedInstall(t *testing.T) {
	probe := &resourceToolServiceProbe{searchResult: resourceorchestrator.SearchResult{
		Kind: resourceorchestrator.KindSkill, Query: "excel",
		Items: []resourceorchestrator.SearchItem{{
			Kind: resourceorchestrator.KindSkill, ID: "candidate-id",
			Name: "office-xlsx", Version: "1.0.0", ExactRevision: "sha256:exact",
			Status: "admitted",
		}},
	}}
	runtime := newResourceToolRuntime(
		probe, "user-id", "conversation-id", "生成一个 Excel 文件",
	)
	events := make(chan ProviderEvent, 16)
	unsearched, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "unsearched", Name: resourceRequestInstallToolName,
		Arguments: `{"kind":"skill","id":"candidate-id","version":"1.0.0",` +
			`"exactRevision":"sha256:exact","reason":"needed for xlsx"}`,
	}, 1, 1)
	if err != nil || !unsearched.IsError || probe.installCalls != 0 ||
		!strings.Contains(unsearched.Content, "candidate_not_searched_or_changed") {
		t.Fatalf("unsearched=%#v calls=%d error=%v", unsearched, probe.installCalls, err)
	}

	runtime = newResourceToolRuntime(probe, "user-id", "conversation-id", "生成一个 Excel 文件")
	_, _ = runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "search", Name: resourceSearchToolName,
		Arguments: `{"kind":"skill","query":"excel","capability":null}`,
	}, 1, 1)
	denied, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "install", Name: resourceRequestInstallToolName,
		Arguments: `{"kind":"skill","id":"candidate-id","version":"1.0.0",` +
			`"exactRevision":"sha256:exact","reason":"needed for xlsx"}`,
	}, 1, 2)
	if err != nil || !denied.IsError || probe.installCalls != 0 ||
		!strings.Contains(denied.Content, "approval_unavailable") {
		t.Fatalf("denied=%#v calls=%d error=%v", denied, probe.installCalls, err)
	}
}

func TestResourceInstallToolRejectsAndDoesNotEchoSecretArguments(t *testing.T) {
	probe := &resourceToolServiceProbe{}
	runtime := newResourceToolRuntime(
		probe, "user-id", "conversation-id", "安装 DeepWiki MCP",
	)
	events := make(chan ProviderEvent, 4)
	result, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "install", Name: resourceRequestInstallToolName,
		Arguments: `{"kind":"mcp","id":"deepwiki","version":"1.0.0",` +
			`"exactRevision":"exact","reason":"needed","secret":"TOP_SECRET_VALUE"}`,
	}, 1, 1)
	if err != nil || !result.IsError || probe.installCalls != 0 ||
		!strings.Contains(result.Content, "arguments_invalid") ||
		strings.Contains(result.Content, "TOP_SECRET_VALUE") {
		t.Fatalf("result=%#v calls=%d error=%v", result, probe.installCalls, err)
	}
}

func TestResourceSearchDeduplicatesAndCapsUniqueDiscoveryRounds(t *testing.T) {
	probe := &resourceToolServiceProbe{searchResult: resourceorchestrator.SearchResult{
		Kind: resourceorchestrator.KindSkill, Query: "excel", Items: []resourceorchestrator.SearchItem{},
	}}
	runtime := newResourceToolRuntime(probe, "user-id", "conversation-id", "find a skill")
	events := make(chan ProviderEvent, 16)
	for index, query := range []string{"excel", " Excel ", "pdf", "csv"} {
		result, err := runtime.execute(context.Background(), events, ProviderToolCall{
			ID: "search", Name: resourceSearchToolName,
			Arguments: `{"kind":"skill","query":"` + query + `","capability":null}`,
		}, 1, index+1)
		if err != nil {
			t.Fatal(err)
		}
		if index < 3 && result.IsError {
			t.Fatalf("search %d unexpectedly failed: %#v", index, result)
		}
		if index == 3 && (!result.IsError || !strings.Contains(result.Content, "discovery_budget_exhausted")) {
			t.Fatalf("budget result=%#v", result)
		}
	}
	if probe.searchCalls != 2 {
		t.Fatalf("search calls=%d want=2", probe.searchCalls)
	}
}

func TestResourceSearchRejectsKindsUnavailableToCurrentRuntime(t *testing.T) {
	probe := &resourceToolServiceProbe{}
	runtime := newResourceToolRuntime(probe, "user-id", "conversation-id", "find a skill")
	runtime.bindRuntimeAvailability(false, true)
	events := make(chan ProviderEvent, 4)
	result, err := runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "search", Name: resourceSearchToolName,
		Arguments: `{"kind":"skill","query":"excel","capability":"xlsx"}`,
	}, 1, 1)
	if err != nil || !result.IsError || probe.searchCalls != 0 ||
		!strings.Contains(result.Content, "resource_kind_unavailable") {
		t.Fatalf("result=%#v calls=%d error=%v", result, probe.searchCalls, err)
	}
}

func TestResourceToolSchemaOnlyAdvertisesRuntimeAvailableKinds(t *testing.T) {
	runtime := newResourceToolRuntime(&resourceToolServiceProbe{}, "user-id", "conversation-id", "find MCP")
	runtime.bindRuntimeAvailability(false, true)
	registry := newChatToolRegistry(externalWebToolLoopInput{Resource: runtime})
	registration, ok := registry.lookup(resourceSearchToolName)
	if !ok || registration.Definition == nil {
		t.Fatal("resource search definition is unavailable")
	}
	properties, _ := registration.Definition.Function.Parameters["properties"].(map[string]any)
	kind, _ := properties["kind"].(map[string]any)
	values, _ := kind["enum"].([]string)
	if len(values) != 1 || values[0] != resourceorchestrator.KindMCP {
		t.Fatalf("kind enum=%#v", values)
	}
}

func TestAgentInitiatedResourceInstallWaitsForDurableApproval(t *testing.T) {
	probe := &resourceToolServiceProbe{searchResult: resourceorchestrator.SearchResult{
		Kind: resourceorchestrator.KindSkill, Query: "excel",
		Items: []resourceorchestrator.SearchItem{{
			Kind: resourceorchestrator.KindSkill, ID: "candidate-id",
			Name: "office-xlsx", Version: "1.0.0", ExactRevision: "sha256:exact",
			Status: "admitted",
		}},
	}}
	runtime := newResourceToolRuntime(
		probe, "user-id", "conversation-id", "生成一个 Excel 文件",
	)
	repository := newApprovalTestRepository()
	service := NewService(repository)
	waiters := newChatAgentApprovalWaiters()
	runtime.bindApprovalRuntime(newChatToolApprovalRuntime(service, waiters, testTurnID))
	events := make(chan ProviderEvent, 16)
	_, _ = runtime.execute(context.Background(), events, ProviderToolCall{
		ID: "search", Name: resourceSearchToolName,
		Arguments: `{"kind":"skill","query":"excel","capability":"xlsx"}`,
	}, 1, 1)
	resultChannel := make(chan ProviderToolResult, 1)
	errorChannel := make(chan error, 1)
	go func() {
		result, executeErr := runtime.execute(context.Background(), events, ProviderToolCall{
			ID: "install", Name: resourceRequestInstallToolName,
			Arguments: `{"kind":"skill","id":"candidate-id","version":"1.0.0",` +
				`"exactRevision":"sha256:exact","reason":"needed for xlsx"}`,
		}, 1, 2)
		resultChannel <- result
		errorChannel <- executeErr
	}()

	var approval ChatAgentApproval
	select {
	case approval = <-repository.created:
	case <-time.After(time.Second):
		t.Fatal("resource approval request was not persisted")
	}
	decision, err := service.DecideChatAgentApproval(context.Background(), DecideChatAgentApprovalInput{
		ApprovalID: approval.ID, ExpectedRevision: approval.Revision,
		Decision: ChatAgentApprovalAllowOnce, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	waiters.resolve(decision)
	result := <-resultChannel
	if executeErr := <-errorChannel; executeErr != nil || result.IsError || probe.installCalls != 1 {
		t.Fatalf("result=%#v calls=%d error=%v", result, probe.installCalls, executeErr)
	}
	close(events)
	var awaitingApproval bool
	for event := range events {
		if event.ToolExecution != nil &&
			event.ToolExecution.Status == ProcessStepStatusAwaitingApproval &&
			event.ToolExecution.Presentation != nil &&
			event.ToolExecution.Presentation.Approval != nil &&
			event.ToolExecution.Presentation.Approval.ID == approval.ID {
			awaitingApproval = true
		}
	}
	if !awaitingApproval {
		t.Fatal("resource approval presentation was not emitted")
	}
}

func TestResourceInstallIntentIsDirectHumanOnly(t *testing.T) {
	for _, value := range []string{
		"帮我安装 Excel Skill", "安装 DeepWiki MCP", "装一下 DeepWiki MCP", "/skill install candidate-id",
		"install office-xlsx skill",
	} {
		if !hasExplicitResourceInstallIntent(value) {
			t.Fatalf("intent not detected: %q", value)
		}
	}
	if hasExplicitResourceInstallIntent("生成一个 Excel 文件") {
		t.Fatal("ordinary task was treated as explicit install intent")
	}
	for _, value := range []string{
		"不要安装任何东西", "don't install DeepWiki", "Should I install this Skill?",
	} {
		if hasExplicitResourceInstallIntent(value) {
			t.Fatalf("negative or advisory intent was treated as explicit install: %q", value)
		}
	}
}
