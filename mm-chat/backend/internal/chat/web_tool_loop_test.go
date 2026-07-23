package chat

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestExternalWebToolLoopRunsNativeSearchAndContinuesSameModel(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "call-1", Name: searchWebToolName,
				Arguments: `{"query":"latest fixture"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "answer [W1]"}},
	}}
	search := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://example.test/fixture", Content: "fresh",
	}}}}

	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "latest fixture",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		}}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		MaxResults:  5,
		ForceSearch: true,
	})

	var content strings.Builder
	var executions []ProviderToolExecutionEvent
	var searchEvents int
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		switch event.Type {
		case ProviderEventDelta:
			content.WriteString(event.Delta)
		case ProviderEventToolExecution:
			executions = append(executions, *event.ToolExecution)
		case ProviderEventSearch:
			searchEvents++
		}
	}
	if content.String() != "answer [W1]" || search.calls != 1 || searchEvents != 1 {
		t.Fatalf("content/search/events = %q / %d / %d", content.String(), search.calls, searchEvents)
	}
	if len(executions) != 2 || executions[0].Status != ProcessStepStatusRunning ||
		executions[1].Status != ProcessStepStatusCompleted {
		t.Fatalf("executions = %#v", executions)
	}
	if len(provider.inputs) != 2 || provider.inputs[0].ToolChoice != ProviderToolChoiceRequired ||
		provider.inputs[1].ToolChoice != ProviderToolChoiceAuto {
		t.Fatalf("provider inputs = %#v", provider.inputs)
	}
	continuation := provider.inputs[1].Continuation
	if len(continuation) != 1 || len(continuation[0].Results) != 1 ||
		continuation[0].Results[0].CallID != "call-1" ||
		!strings.Contains(continuation[0].Results[0].Content, "[W1]") {
		t.Fatalf("continuation = %#v", continuation)
	}
}

func TestExternalWebToolLoopNativeAutoSkipsSearchWithoutToolCall(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{{
		{Type: ProviderEventDelta, Delta: "ordinary writing answer"},
	}}}
	search := &fakeWebSearchProvider{}
	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "rewrite this sentence",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
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
	if content.String() != "ordinary writing answer" || search.calls != 0 || len(provider.inputs) != 1 ||
		provider.inputs[0].ToolChoice != ProviderToolChoiceAuto {
		t.Fatalf("content/search/inputs = %q / %d / %#v", content.String(), search.calls, provider.inputs)
	}
}

func TestExternalWebToolLoopForcedNativeNoCallUsesSameModelCompatibility(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{{
			{Type: ProviderEventDelta, Delta: "discarded unsupported answer"},
		}},
		chatRounds: [][]ProviderEvent{
			{{Type: ProviderEventDelta, Delta: `{"shouldSearch":true,"query":"standalone query"}`}},
			{{Type: ProviderEventDelta, Delta: "compatibility answer [W1]"}},
		},
	}
	search := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://example.test/fixture", Content: "fresh",
	}}}}
	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "search this",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		PlannerMessages: []ProviderMessage{{Role: "user", Content: "search this"}},
		SearchService:   websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		ForceSearch: true,
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
	if content.String() != "compatibility answer [W1]" || search.calls != 1 ||
		search.request.Query != "standalone query" || len(provider.chatInputs) != 2 {
		t.Fatalf("content/search/provider = %q / %#v / %#v", content.String(), search.request, provider.chatInputs)
	}
}

func TestExternalWebToolLoopSearchFailureContinuesWithoutWebResult(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "call-1", Name: searchWebToolName,
				Arguments: `{"query":"latest fixture"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "truthful fallback"}},
	}}
	search := &fakeWebSearchProvider{err: errors.New("fixture failure")}
	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "latest fixture",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		ForceSearch: true,
	})
	var content strings.Builder
	var failed *ProviderToolExecutionEvent
	var searchEvents int
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.Type == ProviderEventSearch {
			searchEvents++
		}
		if event.ToolExecution != nil && event.ToolExecution.Status == ProcessStepStatusFailed {
			copy := *event.ToolExecution
			failed = &copy
		}
	}
	if content.String() != "truthful fallback" || failed == nil || searchEvents != 0 {
		t.Fatalf("content/failed/search events = %q / %#v / %d", content.String(), failed, searchEvents)
	}
	result := provider.inputs[1].Continuation[0].Results[0].Content
	if !strings.Contains(result, `"ok":false`) || !strings.Contains(result, "do not use [W] markers") {
		t.Fatalf("failure tool result = %q", result)
	}
	if !provider.inputs[1].Continuation[0].Results[0].IsError {
		t.Fatal("failed Tool Result was not marked as an error")
	}
}

func TestExternalWebToolLoopRecoversFailedNativeContinuationWithEvidence(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{
			{
				{Type: ProviderEventUsage, Usage: &TokenUsage{
					PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5,
				}},
				{
					Type: ProviderEventToolCallCompleted,
					ToolCall: &ProviderToolCall{
						ID: "call-1", Name: searchWebToolName,
						Arguments: `{"query":"latest fixture"}`,
					},
				},
			},
			{{Error: errors.New("continuation stream failed")}},
		},
		chatRounds: [][]ProviderEvent{{
			{Type: ProviderEventDelta, Delta: "recovered answer [W1]"},
			{Type: ProviderEventUsage, Usage: &TokenUsage{
				PromptTokens: 5, CompletionTokens: 7, TotalTokens: 12,
			}},
		}},
	}
	search := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://example.test/fixture", Content: "fresh",
	}}}}
	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "latest fixture",
			Messages: []ProviderMessage{{
				Role: "user", Content: "latest fixture",
			}},
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		ForceSearch: true,
	})
	var content strings.Builder
	var usages []TokenUsage
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.Type == ProviderEventUsage && event.Usage != nil {
			usages = append(usages, *event.Usage)
		}
	}
	if content.String() != "recovered answer [W1]" || search.calls != 1 ||
		len(provider.inputs) != 2 || len(provider.chatInputs) != 1 ||
		len(usages) != 2 || usages[1].PromptTokens != 7 ||
		usages[1].CompletionTokens != 10 || usages[1].TotalTokens != 17 {
		t.Fatalf(
			"content/search/tool/chat/usage = %q / %d / %d / %d / %#v",
			content.String(), search.calls, len(provider.inputs), len(provider.chatInputs), usages,
		)
	}
	fallback := provider.chatInputs[0]
	if !strings.Contains(fallback.Prompt, "Relevant Web evidence") ||
		!strings.Contains(fallback.Prompt, "[W1]") ||
		len(fallback.Messages) != 1 || fallback.Messages[0].Content != fallback.Prompt {
		t.Fatalf("fallback request = %#v", fallback)
	}
}

func TestExternalWebToolLoopRecoversSynchronousContinuationFailureWithEvidence(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "call-1", Name: searchWebToolName,
				Arguments: `{"query":"latest fixture"}`,
			},
		}}},
		syncErrors: map[int]error{1: errors.New("continuation start failed")},
		chatRounds: [][]ProviderEvent{{
			{Type: ProviderEventDelta, Delta: "recovered answer [W1]"},
		}},
	}
	search := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://example.test/fixture", Content: "fresh",
	}}}}
	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "latest fixture",
			Messages: []ProviderMessage{{
				Role: "user", Content: "latest fixture",
			}},
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		ForceSearch: true,
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
	if content.String() != "recovered answer [W1]" || search.calls != 1 ||
		len(provider.inputs) != 2 || len(provider.chatInputs) != 1 {
		t.Fatalf(
			"content/search/tool/chat = %q / %d / %d / %d",
			content.String(), search.calls, len(provider.inputs), len(provider.chatInputs),
		)
	}
}

func TestExternalWebToolLoopDoesNotRecoverAfterAnswerContent(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "call-1", Name: searchWebToolName,
				Arguments: `{"query":"latest fixture"}`,
			},
		}},
		{
			{Type: ProviderEventDelta, Delta: "partial answer"},
			{Error: errors.New("continuation stream failed")},
		},
	}}
	search := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://example.test/fixture", Content: "fresh",
	}}}}
	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "latest fixture",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		ForceSearch: true,
	})
	var content strings.Builder
	var gotError bool
	for event := range events {
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.Error != nil {
			gotError = true
		}
	}
	if content.String() != "partial answer" || !gotError || len(provider.chatInputs) != 0 {
		t.Fatalf(
			"content/error/chat = %q / %v / %d",
			content.String(), gotError, len(provider.chatInputs),
		)
	}
}

func TestExternalWebToolLoopAggregatesUsageAcrossNativeRounds(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{
			{Type: ProviderEventUsage, Usage: &TokenUsage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{ID: "call-1", Name: searchWebToolName, Arguments: `{"query":"first"}`}},
		},
		{
			{Type: ProviderEventUsage, Usage: &TokenUsage{PromptTokens: 5, CompletionTokens: 7, TotalTokens: 12}},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{ID: "call-2", Name: searchWebToolName, Arguments: `{"query":"second"}`}},
		},
		{
			{Type: ProviderEventDelta, Delta: "answer [W1]"},
			{Type: ProviderEventUsage, Usage: &TokenUsage{PromptTokens: 11, CompletionTokens: 13, TotalTokens: 24}},
		},
	}}
	search := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://example.test/fixture", Content: "fresh",
	}}}}
	events := startExternalWebToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "latest", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		ForceSearch: true,
	})
	var usages []TokenUsage
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventUsage && event.Usage != nil {
			usages = append(usages, *event.Usage)
		}
	}
	if len(usages) != 3 || usages[0].TotalTokens != 5 || usages[1].TotalTokens != 17 ||
		usages[2].PromptTokens != 18 || usages[2].CompletionTokens != 23 ||
		usages[2].TotalTokens != 41 {
		t.Fatalf("aggregated usage = %#v", usages)
	}
	if len(provider.inputs) != 3 || len(provider.inputs[2].Continuation) != 2 || search.calls != 2 {
		t.Fatalf("rounds/continuation/search = %d / %d / %d", len(provider.inputs), len(provider.inputs[2].Continuation), search.calls)
	}
	continuation := provider.inputs[2].Continuation
	firstResult := continuation[0].Results[0].Content
	secondResult := continuation[1].Results[0].Content
	if !strings.Contains(firstResult, `"marker":"[W1]"`) ||
		!strings.Contains(secondResult, `"sources":[]`) ||
		strings.Count(firstResult+secondResult, "https://example.test/fixture") != 1 {
		t.Fatalf("incremental Tool Results = %q / %q", firstResult, secondResult)
	}
}

func TestExternalWebToolLoopCancelsInFlightSearch(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{{{
		Type: ProviderEventToolCallCompleted,
		ToolCall: &ProviderToolCall{
			ID: "call-cancel", Name: searchWebToolName,
			Arguments: `{"query":"blocking fixture"}`,
		},
	}}}}
	search := &blockingWebSearchProvider{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := startExternalWebToolLoop(ctx, externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "blocking fixture",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
		ForceSearch: true,
	})
	var executions []ProviderToolExecutionEvent
	for event := range events {
		if event.ToolExecution != nil {
			executions = append(executions, *event.ToolExecution)
		}
		if event.ToolExecution != nil &&
			event.ToolExecution.Status == ProcessStepStatusRunning {
			cancel()
		}
	}
	select {
	case <-search.started:
	default:
		t.Fatal("Search did not start")
	}
	select {
	case <-search.cancelled:
	default:
		t.Fatal("Search context was not cancelled")
	}
	if len(executions) != 2 ||
		executions[0].Status != ProcessStepStatusRunning ||
		executions[1].Status != ProcessStepStatusCancelled ||
		executions[1].FailureCategory != "" {
		t.Fatalf("cancelled executions = %#v", executions)
	}
}

func TestCompatibilityExternalWebSearchCancellationEmitsCancelled(t *testing.T) {
	provider := &blockingCompatibilityPlannerProvider{started: make(chan struct{})}
	search := &fakeWebSearchProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	events := startExternalWebToolLoop(ctx, externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "blocking planner",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: search,
		},
	})
	<-provider.started
	cancel()

	var executions []ProviderToolExecutionEvent
	for event := range events {
		if event.ToolExecution != nil {
			executions = append(executions, *event.ToolExecution)
		}
	}
	if len(executions) != 1 ||
		executions[0].ExecutionID != "compatibility-plan" ||
		executions[0].Status != ProcessStepStatusCancelled ||
		executions[0].FailureCategory != "" || search.calls != 0 {
		t.Fatalf("cancelled planner executions/search = %#v / %d", executions, search.calls)
	}
}

func TestValidateSearchWebToolCallRejectsMalformedAndUnknownCalls(t *testing.T) {
	tests := []struct {
		name string
		call ProviderToolCall
		want string
	}{
		{name: "malformed JSON", call: ProviderToolCall{Name: searchWebToolName, Arguments: `{"query":`}, want: "invalid_arguments"},
		{name: "missing query", call: ProviderToolCall{Name: searchWebToolName, Arguments: `{}`}, want: "invalid_arguments"},
		{name: "unknown tool", call: ProviderToolCall{Name: "delete_everything", Arguments: `{}`}, want: "unknown_tool"},
		{name: "provider rejection", call: ProviderToolCall{Name: searchWebToolName, FailureCategory: "arguments_too_large"}, want: "arguments_too_large"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, got := validateSearchWebToolCall(test.call)
			if got != test.want {
				t.Fatalf("failure = %q, want %q", got, test.want)
			}
		})
	}
}

func TestWebSearchSuccessToolResultReturnsOnlyNewStableMarkers(t *testing.T) {
	first := websearch.Source{
		Title: "First", URL: "https://example.test/first", Content: "one",
	}
	second := websearch.Source{
		Title: "Second", URL: "https://example.test/second", Content: "two",
	}
	result := webSearchSuccessToolResult(
		websearch.Result{Sources: []websearch.Source{first}},
		websearch.Result{Sources: []websearch.Source{first, second}},
	)
	if strings.Contains(result, first.URL) || !strings.Contains(result, second.URL) ||
		!strings.Contains(result, `"marker":"[W2]"`) {
		t.Fatalf("incremental result = %q", result)
	}
}

type scriptedToolRoundProvider struct {
	rounds     [][]ProviderEvent
	chatRounds [][]ProviderEvent
	syncErrors map[int]error
	inputs     []ProviderRoundRequest
	chatInputs []ProviderRequest
}

type blockingWebSearchProvider struct {
	started       chan struct{}
	cancelled     chan struct{}
	startedOnce   sync.Once
	cancelledOnce sync.Once
}

type blockingCompatibilityPlannerProvider struct {
	started     chan struct{}
	startedOnce sync.Once
}

func (p *blockingCompatibilityPlannerProvider) StreamChat(
	ctx context.Context,
	_ ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.startedOnce.Do(func() { close(p.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func (p *blockingWebSearchProvider) ID() websearch.ProviderID {
	return websearch.ProviderTavily
}

func (p *blockingWebSearchProvider) Search(
	ctx context.Context,
	_ websearch.Request,
) (websearch.Result, error) {
	p.startedOnce.Do(func() { close(p.started) })
	<-ctx.Done()
	p.cancelledOnce.Do(func() { close(p.cancelled) })
	return websearch.Result{}, ctx.Err()
}

func (p *scriptedToolRoundProvider) StreamToolRound(
	ctx context.Context,
	input ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	p.inputs = append(p.inputs, input)
	index := len(p.inputs) - 1
	if err := p.syncErrors[index]; err != nil {
		return nil, err
	}
	if index >= len(p.rounds) {
		return nil, errors.New("unexpected tool round")
	}
	return providerEventFixtureChannel(ctx, p.rounds[index]), nil
}

func (p *scriptedToolRoundProvider) StreamChat(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.chatInputs = append(p.chatInputs, input)
	index := len(p.chatInputs) - 1
	if index >= len(p.chatRounds) {
		return nil, errors.New("unexpected compatibility round")
	}
	return providerEventFixtureChannel(ctx, p.chatRounds[index]), nil
}

func providerEventFixtureChannel(
	ctx context.Context,
	fixture []ProviderEvent,
) <-chan ProviderEvent {
	events := make(chan ProviderEvent, len(fixture))
	for _, event := range fixture {
		select {
		case <-ctx.Done():
			close(events)
			return events
		case events <- event:
		}
	}
	close(events)
	return events
}
