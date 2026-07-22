package chat

import (
	"context"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestCompatibilityKnowledgeLoopStreamsLiveHitBeforeOrdinaryAnswer(t *testing.T) {
	provider := &titleProvider{chunks: []string{"compatibility answer [K1]"}}
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
	if !strings.Contains(provider.input.Prompt, "[K1]") ||
		!strings.Contains(provider.input.Messages[0].Content, "[K1]") {
		t.Fatalf("compatibility provider input = %#v", provider.input)
	}
}

func TestCompatibilityKnowledgeLoopContinuesIntoExternalSearchPlanner(t *testing.T) {
	provider := &capturingSequenceProvider{outputs: [][]string{
		{`{"shouldSearch":true,"query":"public fixture"}`},
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
