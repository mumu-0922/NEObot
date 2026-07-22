package chat

import (
	"context"
	"strings"
	"sync"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestRetrievalToolLoopRunsKnowledgeThenWebWithIsolatedMarkers(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "knowledge-1", Name: searchKnowledgeToolName,
				Arguments: `{"query":"internal fixture"}`,
			},
		}},
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "web-1", Name: searchWebToolName,
				Arguments: `{"query":"public fixture"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "mixed answer [K1] [W1]"}},
	}}
	webProvider := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Public", URL: "https://example.test/public", Content: "current",
	}}}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   "compare the selected source with current public information",
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: webProvider,
		},
		Knowledge: fixtureKnowledgeToolRuntime(),
	})

	var content strings.Builder
	var completed []ProviderToolExecutionEvent
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.ToolExecution != nil &&
			event.ToolExecution.Status == ProcessStepStatusCompleted {
			completed = append(completed, *event.ToolExecution)
		}
	}
	if content.String() != "mixed answer [K1] [W1]" || len(completed) != 2 {
		t.Fatalf("content/completed = %q / %#v", content.String(), completed)
	}
	if completed[0].Name != searchKnowledgeToolName ||
		len(completed[0].CitationMarkers) != 1 ||
		completed[0].CitationMarkers[0] != "[K1]" ||
		completed[0].Knowledge == nil || !completed[0].Knowledge.ReadyForAnswer() {
		t.Fatalf("Knowledge execution = %#v", completed[0])
	}
	if completed[1].Name != searchWebToolName ||
		len(completed[1].CitationMarkers) != 1 ||
		completed[1].CitationMarkers[0] != "[W1]" {
		t.Fatalf("Web execution = %#v", completed[1])
	}
	if len(provider.inputs) != 3 || len(provider.inputs[0].Tools) != 2 ||
		provider.inputs[0].Tools[0].Function.Name != searchWebToolName ||
		provider.inputs[0].Tools[1].Function.Name != searchKnowledgeToolName {
		t.Fatalf("registered tools = %#v", provider.inputs)
	}
	if !strings.Contains(
		provider.inputs[1].Continuation[0].Results[0].Content,
		`"marker":"[K1]"`,
	) || !strings.Contains(
		provider.inputs[2].Continuation[1].Results[0].Content,
		`"marker":"[W1]"`,
	) {
		t.Fatalf("continuation = %#v", provider.inputs[2].Continuation)
	}
}

func TestRetrievalToolLoopRunsWebThenKnowledge(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "web-1", Name: searchWebToolName,
				Arguments: `{"query":"public fixture"}`,
			},
		}},
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "knowledge-1", Name: searchKnowledgeToolName,
				Arguments: `{"query":"internal fixture"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "answer [W1] [K1]"}},
	}}
	webProvider := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Public", URL: "https://example.test/public", Content: "current",
	}}}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider:      provider,
		Request:       ProviderRequest{ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture"}},
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: webProvider,
		},
		Knowledge: fixtureKnowledgeToolRuntime(),
	})
	var names []string
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.ToolExecution != nil &&
			event.ToolExecution.Status == ProcessStepStatusCompleted {
			names = append(names, event.ToolExecution.Name)
		}
	}
	if len(names) != 2 || names[0] != searchWebToolName ||
		names[1] != searchKnowledgeToolName {
		t.Fatalf("execution order = %#v", names)
	}
}

func TestRetrievalToolLoopKnowledgeMissIsSuccessfulAndContinues(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "knowledge-miss", Name: searchKnowledgeToolName,
				Arguments: `{"query":"unrelated fixture"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "ordinary answer"}},
	}}
	runtime := fixtureKnowledgeToolRuntime()
	runtime.Assembler = NewRAGAnswerAssembler(
		&fakeRAGCandidateSource{},
		&fakeRAGHydrator{},
	)
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "unrelated", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture"},
		},
		Knowledge: runtime,
	})
	var completed *ProviderToolExecutionEvent
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.ToolExecution != nil &&
			event.ToolExecution.Status == ProcessStepStatusCompleted {
			copy := *event.ToolExecution
			completed = &copy
		}
	}
	if completed == nil || completed.Knowledge == nil ||
		completed.Knowledge.Outcome != "no_evidence" ||
		len(completed.CitationMarkers) != 0 {
		t.Fatalf("miss execution = %#v", completed)
	}
	result := provider.inputs[1].Continuation[0].Results[0]
	if result.IsError || !strings.Contains(result.Content, `"sources":[]`) ||
		!strings.Contains(result.Content, `"ok":true`) {
		t.Fatalf("miss result = %#v", result)
	}
}

func TestRetrievalToolLoopRepeatedKnowledgeCallReusesMarker(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "knowledge-1", Name: searchKnowledgeToolName,
				Arguments: `{"query":"first"}`,
			},
		}},
		{{
			Type: ProviderEventToolCallCompleted,
			ToolCall: &ProviderToolCall{
				ID: "knowledge-2", Name: searchKnowledgeToolName,
				Arguments: `{"query":"second"}`,
			},
		}},
		{{Type: ProviderEventDelta, Delta: "answer [K1]"}},
	}}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider:  provider,
		Request:   ProviderRequest{ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture"}},
		Knowledge: fixtureKnowledgeToolRuntime(),
	})
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
	}
	if len(provider.inputs) != 3 || len(provider.inputs[2].Continuation) != 2 {
		t.Fatalf("rounds = %#v", provider.inputs)
	}
	for _, exchange := range provider.inputs[2].Continuation {
		if len(exchange.Results) != 1 ||
			!strings.Contains(exchange.Results[0].Content, `"marker":"[K1]"`) ||
			strings.Contains(exchange.Results[0].Content, "[K2]") {
			t.Fatalf("unstable marker exchange = %#v", exchange)
		}
	}
}

func TestRetrievalToolDefinitionsOmitKnowledgeWithoutSelection(t *testing.T) {
	webProvider := &fakeWebSearchProvider{}
	tools := retrievalToolDefinitions(externalWebToolLoopInput{
		SearchService: websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: webProvider,
		},
	})
	if len(tools) != 1 || tools[0].Function.Name != searchWebToolName {
		t.Fatalf("tools = %#v", tools)
	}
	if tools := retrievalToolDefinitions(externalWebToolLoopInput{}); len(tools) != 0 {
		t.Fatalf("empty tools = %#v", tools)
	}
}

func TestRetrievalToolLoopCancelsInFlightKnowledgeRetrieval(t *testing.T) {
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{{{
		Type: ProviderEventToolCallCompleted,
		ToolCall: &ProviderToolCall{
			ID: "knowledge-cancel", Name: searchKnowledgeToolName,
			Arguments: `{"query":"blocking fixture"}`,
		},
	}}}}
	candidates := &blockingRAGCandidateSource{
		started: make(chan struct{}), cancelled: make(chan struct{}),
	}
	runtime := fixtureKnowledgeToolRuntime()
	runtime.Assembler = NewRAGAnswerAssembler(candidates, &fakeRAGHydrator{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := startRetrievalToolLoop(ctx, externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture"},
		},
		Knowledge: runtime,
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
	case <-candidates.started:
	default:
		t.Fatal("Knowledge retrieval did not start")
	}
	select {
	case <-candidates.cancelled:
	default:
		t.Fatal("Knowledge retrieval context was not cancelled")
	}
	if len(executions) != 2 ||
		executions[0].Status != ProcessStepStatusRunning ||
		executions[1].Status != ProcessStepStatusCancelled ||
		executions[1].FailureCategory != "" {
		t.Fatalf("cancelled Knowledge executions = %#v", executions)
	}
}

func fixtureKnowledgeToolRuntime() *knowledgeToolRuntime {
	return &knowledgeToolRuntime{
		Assembler: NewRAGAnswerAssembler(
			&fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}},
			&fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}},
		),
		AnswerGate: &fakeRAGAnswerGovernanceGate{authority: RAGAnswerAuthority{
			Processor: "fixture", ModelID: "fixture-model", CollectionCount: 1,
		}},
		ActorUserID:           "user-1",
		SessionID:             "session-1",
		ConversationID:        testConversationID,
		SelectedCollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		GovernanceModelRef: ModelRef{
			ProviderID: "fixture", ModelID: "fixture-model",
		},
	}
}

type blockingRAGCandidateSource struct {
	started       chan struct{}
	cancelled     chan struct{}
	startedOnce   sync.Once
	cancelledOnce sync.Once
}

func (source *blockingRAGCandidateSource) FetchEvidenceCandidates(
	ctx context.Context,
	_ RAGCandidateQuery,
) ([]knowledge.EvidenceCandidateReference, error) {
	source.startedOnce.Do(func() { close(source.started) })
	<-ctx.Done()
	source.cancelledOnce.Do(func() { close(source.cancelled) })
	return nil, ctx.Err()
}
