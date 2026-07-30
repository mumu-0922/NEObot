package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/usermemory"
	"neo-chat/mm-chat/backend/internal/websearch"
)

const memoryToolLoopTestContent = "Keep project answers concise"

func TestMemoryToolLoopNoCallStreamsFirstRoundWithoutSearch(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{{
		{Type: ProviderEventReasoningDelta, ReasoningDelta: "ordinary reasoning"},
		{Type: ProviderEventDelta, Delta: "ordinary answer"},
	}}}
	searcher := &memoryToolTestSearcher{}

	content, reasoning, events := collectMemoryToolLoopEvents(t, provider, searcher)
	if content != "ordinary answer" || reasoning != "ordinary reasoning" ||
		searcher.calls != 0 || len(provider.inputs) != 1 || len(provider.chatInputs) != 0 {
		t.Fatalf(
			"content/reasoning/search/tool/chat = %q / %q / %d / %d / %d",
			content, reasoning, searcher.calls, len(provider.inputs), len(provider.chatInputs),
		)
	}
	assertOnlyFirstRoundOffersMemoryTool(t, provider.inputs)
	if len(events) != 0 {
		t.Fatalf("unexpected Tool executions = %#v", events)
	}
}

func TestMemoryToolLoopExactCallSearchesOnceAndContinuesSameModel(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{
			{Type: ProviderEventDelta, Delta: "discarded pre-Tool prose"},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			}},
		},
		{{Type: ProviderEventDelta, Delta: "answer using saved preference"}},
	}}
	searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
		Memories: []usermemory.Memory{{
			ID: "11111111-1111-4111-8111-111111111111", Revision: 3,
			ScopeType: "global", Type: "preference", Content: memoryToolLoopTestContent,
		}},
	}}

	content, reasoning, events := collectMemoryToolLoopEvents(t, provider, searcher)
	if content != "answer using saved preference" || reasoning != "" ||
		searcher.calls != 1 || len(provider.inputs) != 2 || len(provider.chatInputs) != 0 {
		t.Fatalf(
			"content/reasoning/search/tool/chat = %q / %q / %d / %d / %d",
			content, reasoning, searcher.calls, len(provider.inputs), len(provider.chatInputs),
		)
	}
	if searcher.input.ContractVersion != usermemory.HybridMemoryToolContractVersion ||
		searcher.input.ContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
		searcher.input.Query != "current request" {
		t.Fatalf("Memory search authority = %#v", searcher.input)
	}
	assertOnlyFirstRoundOffersMemoryTool(t, provider.inputs)
	if provider.inputs[0].ModelRef != provider.inputs[1].ModelRef {
		t.Fatalf("continuation changed model: %#v", provider.inputs)
	}
	continuation := provider.inputs[1].Continuation
	if len(continuation) != 1 || len(continuation[0].Calls) != 1 ||
		len(continuation[0].Results) != 1 || continuation[0].Results[0].IsError {
		t.Fatalf("continuation = %#v", continuation)
	}
	result := continuation[0].Results[0].Content
	if !strings.Contains(result, memoryToolLoopTestContent) ||
		!strings.Contains(result, `"type":"preference"`) ||
		strings.Contains(strings.ToLower(result), "score") {
		t.Fatalf("Memory Tool result = %q", result)
	}
	if len(events) != 2 || events[0].Status != ProcessStepStatusRunning ||
		events[1].Status != ProcessStepStatusCompleted {
		t.Fatalf("Tool executions = %#v", events)
	}
}

func TestMemoryToolLoopCoexistsWithValidWebCall(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "web-call-1", Name: searchWebToolName,
				Arguments: `{"query":"latest fixture"}`,
			}},
		},
		{{Type: ProviderEventDelta, Delta: "answer using Memory and Web [W1]"}},
	}}
	searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
		Memories: []usermemory.Memory{{
			ID: "11111111-1111-4111-8111-111111111111", Revision: 3,
			ScopeType: "global", Type: "preference", Content: memoryToolLoopTestContent,
		}},
	}}
	web := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://example.test/fixture", Content: "fresh",
	}}}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "current request",
			Messages: []ProviderMessage{{Role: "user", Content: "current request"}},
			ModelRef: ModelRef{ProviderID: "configured", ModelID: "selected-model"},
		},
		Memory: &memoryToolRuntime{
			Searcher: searcher, ConversationID: "22222222-2222-4222-8222-222222222222",
			AssistantMessageID: "33333333-3333-4333-8333-333333333333",
			Query:              "current request",
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: web,
		},
		MaxResults: 5,
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
	if content.String() != "answer using Memory and Web [W1]" ||
		searcher.calls != 1 || web.calls != 1 || len(provider.inputs) != 2 {
		t.Fatalf(
			"content/memory/web/rounds = %q / %d / %d / %d",
			content.String(), searcher.calls, web.calls, len(provider.inputs),
		)
	}
	assertOnlyFirstRoundOffersMemoryTool(t, provider.inputs)
	results := provider.inputs[1].Continuation[0].Results
	if len(results) != 2 || results[0].IsError || results[1].IsError ||
		!strings.Contains(results[0].Content, memoryToolLoopTestContent) ||
		!strings.Contains(results[1].Content, "https://example.test/fixture") {
		t.Fatalf("coexisting Tool results = %#v", results)
	}
}

func TestMemoryToolLoopInvalidCallsNeverReleaseMemory(t *testing.T) {
	tests := []struct {
		name  string
		calls []ProviderToolCall
	}{
		{name: "duplicate", calls: []ProviderToolCall{
			{ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`},
			{ID: "memory-call-2", Name: usermemory.HybridMemoryToolName, Arguments: `{}`},
		}},
		{name: "unknown", calls: []ProviderToolCall{
			{ID: "unknown-call", Name: "read_server_files", Arguments: `{}`},
		}},
		{name: "non-exact name", calls: []ProviderToolCall{
			{ID: "memory-call-1", Name: " search_memory ", Arguments: `{}`},
		}},
		{name: "unknown alongside memory", calls: []ProviderToolCall{
			{ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`},
			{ID: "unknown-call", Name: "read_server_files", Arguments: `{}`},
		}},
		{name: "malformed arguments", calls: []ProviderToolCall{
			{ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{"query":"forbidden"}`},
		}},
		{name: "null arguments", calls: []ProviderToolCall{
			{ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `null`},
		}},
		{name: "empty call ID", calls: []ProviderToolCall{
			{ID: " ", Name: usermemory.HybridMemoryToolName, Arguments: `{}`},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstRound := make([]ProviderEvent, 0, len(test.calls))
			for index := range test.calls {
				call := test.calls[index]
				firstRound = append(firstRound, ProviderEvent{
					Type: ProviderEventToolCallCompleted, ToolCall: &call,
				})
			}
			provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
				firstRound,
				{{Type: ProviderEventDelta, Delta: "ordinary fail-closed answer"}},
			}}
			searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
				Memories: []usermemory.Memory{{
					ID:   "11111111-1111-4111-8111-111111111111",
					Type: "fact", Content: memoryToolLoopTestContent,
				}},
			}}

			content, _, _ := collectMemoryToolLoopEvents(t, provider, searcher)
			if content != "ordinary fail-closed answer" || searcher.calls != 0 ||
				len(provider.inputs) != 2 {
				t.Fatalf(
					"content/search/rounds = %q / %d / %d",
					content, searcher.calls, len(provider.inputs),
				)
			}
			assertOnlyFirstRoundOffersMemoryTool(t, provider.inputs)
		})
	}
}

func TestMemoryToolLoopRetrievalFailureOrEmptyResultContinuesNormally(t *testing.T) {
	tests := []struct {
		name        string
		result      usermemory.HybridMemoryToolSearchResult
		wantIsError bool
	}{
		{
			name: "retrieval failure",
			result: usermemory.HybridMemoryToolSearchResult{
				Memories: []usermemory.Memory{}, FailureCategory: "authority_stale",
			},
			wantIsError: true,
		},
		{
			name:   "empty result",
			result: usermemory.HybridMemoryToolSearchResult{Memories: []usermemory.Memory{}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := memoryToolCallThenAnswerProvider("ordinary answer without Memory")
			searcher := &memoryToolTestSearcher{result: test.result}
			content, _, _ := collectMemoryToolLoopEvents(t, provider, searcher)
			if content != "ordinary answer without Memory" || searcher.calls != 1 ||
				len(provider.inputs) != 2 {
				t.Fatalf(
					"content/search/rounds = %q / %d / %d",
					content, searcher.calls, len(provider.inputs),
				)
			}
			result := provider.inputs[1].Continuation[0].Results[0]
			if result.IsError != test.wantIsError ||
				strings.Contains(result.Content, memoryToolLoopTestContent) {
				t.Fatalf("fail-closed Tool result = %#v", result)
			}
		})
	}
}

func TestMemoryToolLoopRejectsLaterRoundMemoryCall(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "memory-call-2", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventDelta, Delta: "answer after rejecting a second Memory call"}},
	}}
	searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
		Memories: []usermemory.Memory{{
			ID:   "11111111-1111-4111-8111-111111111111",
			Type: "preference", Content: memoryToolLoopTestContent,
		}},
	}}

	content, _, _ := collectMemoryToolLoopEvents(t, provider, searcher)
	if content != "answer after rejecting a second Memory call" ||
		searcher.calls != 1 || len(provider.inputs) != 3 {
		t.Fatalf(
			"content/search/rounds = %q / %d / %d",
			content, searcher.calls, len(provider.inputs),
		)
	}
	assertOnlyFirstRoundOffersMemoryTool(t, provider.inputs)
	secondExchange := provider.inputs[2].Continuation[1]
	if len(secondExchange.Results) != 1 || !secondExchange.Results[0].IsError ||
		strings.Contains(secondExchange.Results[0].Content, memoryToolLoopTestContent) {
		t.Fatalf("later-round Memory result = %#v", secondExchange.Results)
	}
}

func TestMemoryToolLoopFirstRoundStreamFailureAfterCallRecoversNormally(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{{
			{Type: ProviderEventDelta, Delta: "discarded first-round draft"},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			}},
			{Error: errors.New("fixture first-round stream failure")},
		}},
		chatRounds: [][]ProviderEvent{{
			{Type: ProviderEventDelta, Delta: "recovered ordinary answer"},
		}},
	}
	searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
		Memories: []usermemory.Memory{{
			ID:   "11111111-1111-4111-8111-111111111111",
			Type: "preference", Content: memoryToolLoopTestContent,
		}},
	}}

	content, _, _ := collectMemoryToolLoopEvents(t, provider, searcher)
	if content != "recovered ordinary answer" || searcher.calls != 0 ||
		len(provider.inputs) != 1 || len(provider.chatInputs) != 1 {
		t.Fatalf(
			"content/search/tool/chat = %q / %d / %d / %d",
			content, searcher.calls, len(provider.inputs), len(provider.chatInputs),
		)
	}
	if providerRequestContains(provider.chatInputs[0], memoryToolLoopTestContent) {
		t.Fatalf("first-round recovery leaked Memory body = %#v", provider.chatInputs[0])
	}
}

func TestMemoryToolLoopContinuationFailureRecoversWithoutMemoryBody(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{
			{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			}}},
			{{Error: context.DeadlineExceeded}},
		},
		chatRounds: [][]ProviderEvent{{
			{Type: ProviderEventDelta, Delta: "recovered ordinary answer"},
		}},
	}
	searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
		Memories: []usermemory.Memory{{
			ID:   "11111111-1111-4111-8111-111111111111",
			Type: "preference", Content: memoryToolLoopTestContent,
		}},
	}}

	content, _, _ := collectMemoryToolLoopEvents(t, provider, searcher)
	if content != "recovered ordinary answer" || searcher.calls != 1 ||
		len(provider.inputs) != 2 || len(provider.chatInputs) != 1 {
		t.Fatalf(
			"content/search/tool/chat = %q / %d / %d / %d",
			content, searcher.calls, len(provider.inputs), len(provider.chatInputs),
		)
	}
	recovery := provider.chatInputs[0]
	if providerRequestContains(recovery, memoryToolLoopTestContent) {
		t.Fatalf("recovery leaked Memory body = %#v", recovery)
	}
}

func TestMemoryToolLoopEmptyResultContinuationFailureStillRecovers(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{
			{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			}}},
			{{Error: errors.New("fixture continuation failure")}},
		},
		chatRounds: [][]ProviderEvent{{
			{Type: ProviderEventDelta, Delta: "recovered without Memory"},
		}},
	}
	searcher := &memoryToolTestSearcher{result: usermemory.HybridMemoryToolSearchResult{
		Memories: []usermemory.Memory{},
	}}

	content, _, _ := collectMemoryToolLoopEvents(t, provider, searcher)
	if content != "recovered without Memory" || searcher.calls != 1 ||
		len(provider.chatInputs) != 1 {
		t.Fatalf(
			"content/search/recovery = %q / %d / %d",
			content, searcher.calls, len(provider.chatInputs),
		)
	}
}

func TestSearchMemoryToolCanonicalContractHash(t *testing.T) {
	digest, err := SearchMemoryToolContractSHA256()
	if err != nil {
		t.Fatal(err)
	}
	tool := SearchMemoryToolDefinition()
	if digest != usermemory.HybridMemoryToolContractSHA256 ||
		tool.Function.Name != usermemory.HybridMemoryToolName ||
		tool.Function.Parameters["additionalProperties"] != false {
		t.Fatalf("Tool contract/hash = %#v / %q", tool, digest)
	}
}

func TestMemoryToolRuntimePreservesExplicitDirectActionPath(t *testing.T) {
	handler := &Handler{
		memoryToolLoopEnabled: true,
		userMemoryService:     &usermemory.Service{},
	}
	runtime := handler.newMemoryToolRuntime(
		context.Background(),
		true,
		chatSearchModeOff,
		"记住我以后要简洁回答",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	)
	if runtime != nil {
		t.Fatalf("explicit direct action was routed through read Tool: %#v", runtime)
	}
}

type memoryToolTestSearcher struct {
	result usermemory.HybridMemoryToolSearchResult
	input  usermemory.HybridMemoryToolSearchInput
	calls  int
}

func (searcher *memoryToolTestSearcher) SearchRelevantAfterMemoryToolCall(
	_ context.Context,
	input usermemory.HybridMemoryToolSearchInput,
) usermemory.HybridMemoryToolSearchResult {
	searcher.calls++
	searcher.input = input
	return searcher.result
}

func collectMemoryToolLoopEvents(
	t *testing.T,
	provider *scriptedToolRoundProvider,
	searcher *memoryToolTestSearcher,
) (string, string, []ProviderToolExecutionEvent) {
	t.Helper()
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "current request",
			Messages: []ProviderMessage{{Role: "user", Content: "current request"}},
			ModelRef: ModelRef{ProviderID: "configured", ModelID: "selected-model"},
		},
		Memory: &memoryToolRuntime{
			Searcher: searcher, ConversationID: "22222222-2222-4222-8222-222222222222",
			AssistantMessageID: "33333333-3333-4333-8333-333333333333",
			Query:              "current request",
		},
	})
	var content strings.Builder
	var reasoning strings.Builder
	executions := make([]ProviderToolExecutionEvent, 0)
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		switch event.Type {
		case ProviderEventDelta:
			content.WriteString(event.Delta)
		case ProviderEventReasoningDelta:
			reasoning.WriteString(event.ReasoningDelta)
		case ProviderEventToolExecution:
			if event.ToolExecution != nil {
				executions = append(executions, *event.ToolExecution)
			}
		}
	}
	return content.String(), reasoning.String(), executions
}

func memoryToolCallThenAnswerProvider(answer string) *scriptedToolRoundProvider {
	return &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
			ID: "memory-call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
		}}},
		{{Type: ProviderEventDelta, Delta: answer}},
	}}
}

func assertOnlyFirstRoundOffersMemoryTool(
	t *testing.T,
	inputs []ProviderRoundRequest,
) {
	t.Helper()
	for round, input := range inputs {
		found := false
		for _, tool := range input.Tools {
			if tool.Function.Name == usermemory.HybridMemoryToolName {
				found = true
			}
		}
		if found != (round == 0) {
			t.Fatalf("round %d Memory Tool availability = %v; inputs=%#v", round+1, found, inputs)
		}
	}
}

func providerRequestContains(request ProviderRequest, value string) bool {
	if strings.Contains(request.Prompt, value) || strings.Contains(request.SystemPrompt, value) {
		return true
	}
	for _, message := range request.Messages {
		if strings.Contains(message.Content, value) {
			return true
		}
	}
	return false
}

var _ memoryToolSearcher = (*memoryToolTestSearcher)(nil)
