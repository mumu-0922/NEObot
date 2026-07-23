package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/websearch"
)

type plannerFailureProvider struct {
	plannerOutput string
	plannerErr    error
	answer        string
	calls         int
}

func (provider *plannerFailureProvider) StreamChat(
	ctx context.Context,
	_ ProviderRequest,
) (<-chan ProviderEvent, error) {
	provider.calls++
	if provider.calls == 1 && provider.plannerErr != nil {
		return nil, provider.plannerErr
	}
	output := provider.answer
	if provider.calls == 1 {
		output = provider.plannerOutput
	}
	events := make(chan ProviderEvent, 1)
	select {
	case <-ctx.Done():
		close(events)
		return events, nil
	case events <- ProviderEvent{Type: ProviderEventDelta, Delta: output}:
	}
	close(events)
	return events, nil
}

func TestCompatibilityKnowledgeLoopStreamsLiveHitBeforeOrdinaryAnswer(t *testing.T) {
	provider := &capturingSequenceProvider{outputs: [][]string{
		{`{"route":"knowledge","knowledgeQuery":"selected fixture","webQuery":""}`},
		{"compatibility answer [K1]"},
	}}
	runtime := fixtureKnowledgeToolRuntime()
	runtime.OriginalQueryText = "selected fixture"
	events := startCompatibilityKnowledgeLoop(
		context.Background(),
		compatibilityKnowledgeLoopInput{
			Provider: provider,
			Request: ProviderRequest{
				Prompt:             "selected fixture",
				UserMessageID:      testMessageID,
				ConversationID:     testConversationID,
				AssistantMessageID: "assistant-1",
				ModelRef:           ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
				Messages: []ProviderMessage{{
					MessageID: testMessageID, Role: "user", Content: "selected fixture",
				}},
			},
			Runtime: runtime,
		},
	)
	var content strings.Builder
	var statuses []string
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.ToolExecution != nil {
			statuses = append(statuses, event.ToolExecution.Status)
		}
	}
	if content.String() != "compatibility answer [K1]" ||
		len(statuses) != 2 || statuses[0] != ProcessStepStatusRunning ||
		statuses[1] != ProcessStepStatusCompleted {
		t.Fatalf("content/statuses = %q / %#v", content.String(), statuses)
	}
	if len(provider.inputs) != 2 ||
		!strings.Contains(provider.inputs[1].Prompt, "[K1]") ||
		!strings.Contains(provider.inputs[1].Messages[0].Content, "[K1]") {
		t.Fatalf("compatibility provider input = %#v", provider.inputs)
	}
}

func TestCompatibilityKnowledgeLoopContinuesIntoExternalSearchPlanner(t *testing.T) {
	provider := &capturingSequenceProvider{outputs: [][]string{
		{`{"route":"both","knowledgeQuery":"selected fixture","webQuery":"public fixture"}`},
		{"mixed compatibility answer [K1] [W1]"},
	}}
	webProvider := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
		Title: "Public", URL: "https://example.test/public", Content: "public evidence",
	}}}}
	runtime := fixtureKnowledgeToolRuntime()
	runtime.OriginalQueryText = "compare selected and public fixture"
	external := externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt:   runtime.OriginalQueryText,
			ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		},
		PlannerMessages: []ProviderMessage{{Role: "user", Content: runtime.OriginalQueryText}},
		SearchService:   websearch.NewService(&fakeWebSearchResolver{}),
		Execution: websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: webProvider,
		},
		MaxResults: 5,
	}
	events := startCompatibilityKnowledgeLoop(
		context.Background(),
		compatibilityKnowledgeLoopInput{
			Provider: provider,
			Request: ProviderRequest{
				Prompt: runtime.OriginalQueryText,
				ModelRef: ModelRef{
					ProviderID: "fixture", ModelID: "fixture-model",
				},
				Messages: []ProviderMessage{{Role: "user", Content: runtime.OriginalQueryText}},
			},
			Runtime:        runtime,
			ExternalSearch: &external,
		},
	)
	var content strings.Builder
	var names []string
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.ToolExecution != nil &&
			event.ToolExecution.Status == ProcessStepStatusCompleted {
			names = append(names, event.ToolExecution.Name)
		}
	}
	if content.String() != "mixed compatibility answer [K1] [W1]" ||
		len(names) != 2 || names[0] != searchKnowledgeToolName ||
		names[1] != searchWebToolName || webProvider.calls != 1 {
		t.Fatalf("content/names/Web = %q / %#v / %d", content.String(), names, webProvider.calls)
	}
	if len(provider.inputs) != 2 ||
		!strings.Contains(provider.inputs[1].Prompt, "[K1]") ||
		!strings.Contains(provider.inputs[1].Prompt, "[W1]") ||
		!strings.Contains(provider.inputs[1].SystemPrompt, "state the conflict") {
		t.Fatalf("compatibility provider inputs = %#v", provider.inputs)
	}
}

func TestCompatibilityPlannerFailuresUseOnlyDeterministicStrongSignals(t *testing.T) {
	t.Run("invalid JSON with strong catalog match uses Knowledge", func(t *testing.T) {
		source := &fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}}
		runtime := fixtureKnowledgeToolRuntime()
		runtime.Assembler = NewRAGAnswerAssembler(
			source,
			&fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}},
		)
		runtime.OriginalQueryText = "selected template"
		runtime.StrongCatalogMatch = true
		provider := &plannerFailureProvider{
			plannerOutput: "not JSON",
			answer:        "Knowledge fallback [K1]",
		}
		content, failedPlans := collectCompatibilityFallback(
			t,
			startCompatibilityKnowledgeLoop(context.Background(), compatibilityKnowledgeLoopInput{
				Provider: provider,
				Request: ProviderRequest{
					Prompt:   runtime.OriginalQueryText,
					ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
				},
				Runtime: runtime,
			}),
		)
		if content != "Knowledge fallback [K1]" || failedPlans != 1 || source.calls != 1 {
			t.Fatalf("content/planner/candidates = %q / %d / %d", content, failedPlans, source.calls)
		}
	})

	t.Run("planner timeout with forced Search uses Web", func(t *testing.T) {
		runtime := fixtureKnowledgeToolRuntime()
		runtime.OriginalQueryText = "latest public fixture"
		provider := &plannerFailureProvider{
			plannerErr: context.DeadlineExceeded,
			answer:     "Web fallback [W1]",
		}
		webProvider := &fakeWebSearchProvider{result: websearch.Result{Sources: []websearch.Source{{
			Title: "Public", URL: "https://example.test/public", Content: "public evidence",
		}}}}
		external := externalWebToolLoopInput{
			Provider:      provider,
			SearchService: websearch.NewService(&fakeWebSearchResolver{}),
			Execution: websearch.ActiveExecution{
				Mode: websearch.ExecutionExternal, External: webProvider,
			},
			MaxResults: 5,
		}
		content, failedPlans := collectCompatibilityFallback(
			t,
			startCompatibilityKnowledgeLoop(context.Background(), compatibilityKnowledgeLoopInput{
				Provider: provider,
				Request: ProviderRequest{
					Prompt:   runtime.OriginalQueryText,
					ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
				},
				Runtime:        runtime,
				ExternalSearch: &external,
				ForceSearch:    true,
			}),
		)
		if content != "Web fallback [W1]" || failedPlans != 1 || webProvider.calls != 1 {
			t.Fatalf("content/planner/Web = %q / %d / %d", content, failedPlans, webProvider.calls)
		}
	})

	t.Run("provider failure without a strong signal answers Direct", func(t *testing.T) {
		source := &fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}}
		runtime := fixtureKnowledgeToolRuntime()
		runtime.Assembler = NewRAGAnswerAssembler(source, &fakeRAGHydrator{})
		runtime.OriginalQueryText = "write a birthday greeting"
		provider := &plannerFailureProvider{
			plannerErr: errors.New("planner provider unavailable"),
			answer:     "Direct fallback",
		}
		content, failedPlans := collectCompatibilityFallback(
			t,
			startCompatibilityKnowledgeLoop(context.Background(), compatibilityKnowledgeLoopInput{
				Provider: provider,
				Request: ProviderRequest{
					Prompt:   runtime.OriginalQueryText,
					ModelRef: ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
				},
				Runtime: runtime,
			}),
		)
		if content != "Direct fallback" || failedPlans != 1 || source.calls != 0 {
			t.Fatalf("content/planner/candidates = %q / %d / %d", content, failedPlans, source.calls)
		}
	})
}

func collectCompatibilityFallback(
	t *testing.T,
	events <-chan ProviderEvent,
) (string, int) {
	t.Helper()
	var content strings.Builder
	failedPlans := 0
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			content.WriteString(event.Delta)
		}
		if event.ToolExecution != nil &&
			event.ToolExecution.Name == "retrieval_router" &&
			event.ToolExecution.Status == ProcessStepStatusFailed &&
			event.ToolExecution.FailureCategory == "planner_failed" {
			failedPlans++
		}
	}
	return content.String(), failedPlans
}
