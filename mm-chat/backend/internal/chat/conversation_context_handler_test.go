package chat

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestHandlerPersistsAndUsesConversationSummary(t *testing.T) {
	repo := newFakeRepository()
	messages := contextBudgetTestMessages(19, "")
	messages[len(messages)-1].MessageID = testMessageID
	repo.conversations = append(
		repo.conversations,
		fakeConversation(testConversationID, "Long context", len(messages)),
	)
	for index, message := range messages {
		repo.messages[testConversationID] = append(
			repo.messages[testConversationID],
			fakeMessage(
				message.MessageID,
				testConversationID,
				index,
				message.Role,
				message.Content,
			),
		)
	}
	provider := &summaryThenAnswerProvider{}
	handler := NewHandler(NewService(repo), WithProvider(provider))
	handler.contextBudgetPolicy = tinyContextBudgetPolicy()

	recorder := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"tiny-model"},"idempotencyKey":"context-summary-handler"}`,
	)

	assertStreamStatus(t, recorder, http.StatusOK)
	if len(provider.inputs) != 2 {
		t.Fatalf("provider calls = %d, want summary plus answer", len(provider.inputs))
	}
	finalInput := provider.inputs[1]
	if len(finalInput.Messages) < 2 || finalInput.Messages[0].Role != "assistant" ||
		!strings.Contains(finalInput.Messages[0].Content, "cobalt-7Q4M") ||
		!strings.Contains(finalInput.SystemPrompt, contextSummaryRuntimeInstruction) {
		t.Fatalf("final summarized provider input = %#v", finalInput)
	}
	persistedSummary, ok := repo.summaries[testConversationID]
	if !ok || persistedSummary.Version != 1 || persistedSummary.SourceMessageCount <= 0 {
		t.Fatalf("persisted summary = %#v", persistedSummary)
	}
	persistedMessages := repo.messages[testConversationID]
	assistant := persistedMessages[len(persistedMessages)-1]
	contextMetadata, ok := assistant.Metadata["context"].(map[string]any)
	if !ok || contextMetadata["mode"] != "summary" || contextMetadata["summaryVersion"] != 1 {
		t.Fatalf("assistant context metadata = %#v", assistant.Metadata["context"])
	}
}

type summaryThenAnswerProvider struct {
	inputs []ProviderRequest
}

func (p *summaryThenAnswerProvider) StreamChat(
	_ context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.inputs = append(p.inputs, input)
	events := make(chan ProviderEvent, 1)
	if input.SystemPrompt == contextSummarySystemInstruction {
		events <- ProviderEvent{
			Type:  ProviderEventDelta,
			Delta: "The user established the one-time code cobalt-7Q4M.",
		}
	} else {
		events <- ProviderEvent{Type: ProviderEventDelta, Delta: "final answer"}
	}
	close(events)
	return events, nil
}
