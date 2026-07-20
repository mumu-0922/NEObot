package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestContextBudgetPolicyUsesLongestModelPrefix(t *testing.T) {
	policy := contextBudgetPolicy{
		DefaultWindowTokens: 10_000,
		ModelWindowTokens: map[string]int{
			"gpt":     20_000,
			"gpt-5":   30_000,
			"gpt-5.6": 40_000,
		},
		ReservedOutputTokens: 1_000,
	}
	window, budget := policy.inputBudgetTokens("GPT-5.6-SOL")
	if window != 40_000 || budget != 39_000 {
		t.Fatalf("window/budget = %d/%d", window, budget)
	}
}

func TestEstimateProviderTextTokensIsConservativeForMultilingualText(t *testing.T) {
	if got := estimateProviderTextTokens("abcdefgh"); got != 2 {
		t.Fatalf("ASCII estimate = %d, want 2", got)
	}
	if got := estimateProviderTextTokens("中文测试"); got != 8 {
		t.Fatalf("CJK estimate = %d, want 8", got)
	}
}

func TestPrepareConversationContextCreatesAndRollsSummary(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))
	handler.contextBudgetPolicy = tinyContextBudgetPolicy()
	provider := &contextSummaryTestProvider{summaries: []string{
		"The one-time code is cobalt-7Q4M.",
		"The one-time code is cobalt-7Q4M and the user chose option B.",
	}}
	messages := contextBudgetTestMessages(19, "")

	first := handler.prepareConversationContext(
		context.Background(),
		testConversationID,
		ModelRef{ProviderID: "SERVER_DEFAULT", ModelID: "tiny-model"},
		provider,
		"be accurate",
		messages,
	)
	if first.Mode != "summary" || !first.UsesSummary || first.SummaryVersion != 1 {
		t.Fatalf("first preparation = %#v", first)
	}
	if first.SummarizedMessageCount <= 0 || first.SummarizedMessageCount >= len(messages) {
		t.Fatalf("first summarized count = %d", first.SummarizedMessageCount)
	}
	if len(first.Messages) < 2 || first.Messages[0].Role != "assistant" ||
		!strings.Contains(first.Messages[0].Content, "cobalt-7Q4M") {
		t.Fatalf("first prepared messages = %#v", first.Messages)
	}
	if first.EstimatedInputTokens > first.InputBudgetTokens {
		t.Fatalf("first estimate/budget = %d/%d", first.EstimatedInputTokens, first.InputBudgetTokens)
	}
	restartedHandler := NewHandler(NewService(repo))
	restartedHandler.contextBudgetPolicy = tinyContextBudgetPolicy()
	restartedProvider := &contextSummaryTestProvider{}
	reused := restartedHandler.prepareConversationContext(
		context.Background(),
		testConversationID,
		ModelRef{ProviderID: "SERVER_DEFAULT", ModelID: "tiny-model"},
		restartedProvider,
		"be accurate",
		messages,
	)
	if reused.Mode != "summary" || reused.SummaryVersion != 1 ||
		len(restartedProvider.inputs) != 0 {
		t.Fatalf("restarted summary reuse = %#v, calls=%d", reused, len(restartedProvider.inputs))
	}

	additional := contextBudgetTestMessages(18, "next-")
	for index := range additional {
		additional[index].MessageID = contextBudgetTestUUID(100 + index)
	}
	messages = append(messages, additional...)
	second := handler.prepareConversationContext(
		context.Background(),
		testConversationID,
		ModelRef{ProviderID: "SERVER_DEFAULT", ModelID: "tiny-model"},
		provider,
		"be accurate",
		messages,
	)
	if second.Mode != "summary" || second.SummaryVersion != 2 ||
		second.SummarizedMessageCount <= first.SummarizedMessageCount {
		t.Fatalf("second preparation = %#v", second)
	}
	if len(provider.inputs) != 2 ||
		!strings.Contains(provider.inputs[1].Prompt, "cobalt-7Q4M") {
		t.Fatalf("rolling summary prompts = %#v", provider.inputs)
	}
	persisted := repo.summaries[testConversationID]
	if persisted.Version != 2 || persisted.SourceMessageCount != second.SummarizedMessageCount {
		t.Fatalf("persisted rolling summary = %#v", persisted)
	}
}

func TestPrepareConversationContextInvalidatesEditedPrefix(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))
	handler.contextBudgetPolicy = contextBudgetPolicy{
		DefaultWindowTokens: 100_000, ModelWindowTokens: map[string]int{},
		ReservedOutputTokens: 1_000, TriggerPercent: 80, TargetPercent: 50,
	}
	messages := contextBudgetTestMessages(7, "")
	repo.summaries[testConversationID] = contextBudgetTestSummary(messages[:4], "old summary", 3)
	messages[0].Content = "edited content"

	prepared := handler.prepareConversationContext(
		context.Background(), testConversationID,
		ModelRef{ProviderID: "mock", ModelID: "large"},
		&contextSummaryTestProvider{}, "", messages,
	)
	if prepared.Mode != "full" || prepared.DegradationReason != "summary_invalidated" || prepared.UsesSummary {
		t.Fatalf("invalidated preparation = %#v", prepared)
	}
}

func TestPrepareConversationContextFallsBackToRecentTail(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))
	handler.contextBudgetPolicy = tinyContextBudgetPolicy()
	provider := &contextSummaryTestProvider{err: errors.New("summary unavailable")}
	messages := contextBudgetTestMessages(19, "")

	prepared := handler.prepareConversationContext(
		context.Background(), testConversationID,
		ModelRef{ProviderID: "mock", ModelID: "tiny-model"},
		provider, "", messages,
	)
	if prepared.Mode != "tail_fallback" || prepared.DegradationReason != "summary_generation_failed" {
		t.Fatalf("fallback preparation = %#v", prepared)
	}
	if len(prepared.Messages) >= len(messages) ||
		prepared.Messages[len(prepared.Messages)-1].MessageID != messages[len(messages)-1].MessageID {
		t.Fatalf("fallback tail = %#v", prepared.Messages)
	}
	if len(repo.summaries) != 0 {
		t.Fatalf("failed summary persisted: %#v", repo.summaries)
	}
}

func TestPrepareConversationContextDoesNotClaimOversizeStoredSummaryInFallback(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo))
	handler.contextBudgetPolicy = tinyContextBudgetPolicy()
	messages := contextBudgetTestMessages(19, "")
	repo.summaries[testConversationID] = contextBudgetTestSummary(
		messages[:4],
		strings.Repeat("oversized-summary ", 500),
		3,
	)

	prepared := handler.prepareConversationContext(
		context.Background(), testConversationID,
		ModelRef{ProviderID: "mock", ModelID: "tiny-model"},
		&contextSummaryTestProvider{err: errors.New("summary unavailable")},
		"base instruction", messages,
	)
	if prepared.Mode != "tail_fallback" || prepared.UsesSummary ||
		prepared.SummaryVersion != 0 ||
		strings.Contains(prepared.SystemPrompt, contextSummaryRuntimeInstruction) {
		t.Fatalf("oversize stored-summary fallback = %#v", prepared)
	}
}

func TestConversationContextSummaryDigestRejectsSiblingBranch(t *testing.T) {
	messages := contextBudgetTestMessages(7, "")
	summary := contextBudgetTestSummary(messages[:4], "summary", 2)
	if !conversationContextSummaryMatches(summary, messages) {
		t.Fatal("matching summary was rejected")
	}
	messages[3].MessageID = contextBudgetTestUUID(99)
	if conversationContextSummaryMatches(summary, messages) {
		t.Fatal("sibling branch reused a summary from another prefix")
	}
}

func TestGenerateConversationSummaryRejectsNilEventStream(t *testing.T) {
	if _, err := generateConversationSummary(
		context.Background(), nilEventContextProvider{}, ModelRef{ModelID: "fixture"}, "prompt",
	); err == nil {
		t.Fatal("nil Provider event stream was accepted")
	}
}

func tinyContextBudgetPolicy() contextBudgetPolicy {
	return contextBudgetPolicy{
		DefaultWindowTokens:   6_000,
		ModelWindowTokens:     map[string]int{},
		ReservedOutputTokens:  1_000,
		SafetyBufferTokens:    0,
		TriggerPercent:        80,
		TargetPercent:         50,
		MinimumRecentMessages: 5,
	}
}

func contextBudgetTestMessages(count int, prefix string) []ProviderMessage {
	messages := make([]ProviderMessage, 0, count)
	for index := 0; index < count; index++ {
		role := "user"
		if index%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, ProviderMessage{
			MessageID: contextBudgetTestUUID(index + 1),
			Role:      role,
			Content: fmt.Sprintf(
				"%smessage-%02d %s",
				prefix,
				index,
				strings.Repeat(string(rune('a'+index%20)), 900),
			),
		})
	}
	return messages
}

func contextBudgetTestUUID(index int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", index)
}

func contextBudgetTestSummary(
	messages []ProviderMessage,
	summary string,
	version int,
) ConversationContextSummary {
	return ConversationContextSummary{
		ConversationID:         testConversationID,
		Version:                version,
		SourceFirstMessageID:   messages[0].MessageID,
		SourceLastMessageID:    messages[len(messages)-1].MessageID,
		SourceMessageCount:     len(messages),
		SourceDigest:           conversationContextDigest(messages),
		Summary:                summary,
		EstimatedSourceTokens:  estimateProviderMessagesTokens(messages),
		EstimatedSummaryTokens: estimateProviderTextTokens(summary),
	}
}

type contextSummaryTestProvider struct {
	inputs    []ProviderRequest
	summaries []string
	err       error
}

type nilEventContextProvider struct{}

func (nilEventContextProvider) StreamChat(
	context.Context,
	ProviderRequest,
) (<-chan ProviderEvent, error) {
	return nil, nil
}

func (p *contextSummaryTestProvider) StreamChat(
	_ context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.inputs = append(p.inputs, input)
	if p.err != nil {
		return nil, p.err
	}
	events := make(chan ProviderEvent, 1)
	index := len(p.inputs) - 1
	if index < len(p.summaries) {
		events <- ProviderEvent{Type: ProviderEventDelta, Delta: p.summaries[index]}
	}
	close(events)
	return events, nil
}
