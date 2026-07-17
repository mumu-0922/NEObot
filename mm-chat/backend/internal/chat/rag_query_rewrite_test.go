package chat

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestShouldRewriteRAGQueryOnlyMatchesContextDependentFollowUps(t *testing.T) {
	for _, query := range []string{"这个方向有什么成果？", "Tell me more about it.", "继续展开说"} {
		if !shouldRewriteRAGQuery(query) {
			t.Fatalf("shouldRewriteRAGQuery(%q) = false", query)
		}
	}
	for _, query := range []string{"研究方向是什么？", "What is project ALPHA-42?", "西北工业大学"} {
		if shouldRewriteRAGQuery(query) {
			t.Fatalf("shouldRewriteRAGQuery(%q) = true", query)
		}
	}
}

func TestRewriteRAGQueryUsesAtMostSixPriorMessagesAndPreservesCurrentQuestion(t *testing.T) {
	provider := &ragRewriteProvider{chunks: []string{"研究方向推荐系统有什么成果？"}}
	messages := make([]Message, 0, 9)
	for index := 0; index < 8; index++ {
		role := "user"
		if index%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, Message{
			ID:      strings.Repeat(string(rune('a'+index)), 8),
			Role:    role,
			Content: "history " + string(rune('0'+index)),
		})
	}
	messages = append(messages, Message{ID: testMessageID, Role: "user", Content: "这个方向有什么成果？"})

	rewritten, err := rewriteRAGQuery(
		context.Background(),
		provider,
		ModelRef{ProviderID: "mock", ModelID: "mock-chat"},
		testMessageID,
		"这个方向有什么成果？",
		messages,
	)
	if err != nil || rewritten != "研究方向推荐系统有什么成果？" {
		t.Fatalf("rewrite = %q, err=%v", rewritten, err)
	}
	if strings.Contains(provider.input.Prompt, "history 0") || strings.Contains(provider.input.Prompt, "history 1") {
		t.Fatalf("rewrite prompt exceeded history window: %q", provider.input.Prompt)
	}
	if !strings.Contains(provider.input.Prompt, "history 2") || !strings.Contains(provider.input.Prompt, "这个方向有什么成果？") {
		t.Fatalf("rewrite prompt missing bounded context: %q", provider.input.Prompt)
	}
}

func TestRewriteRAGQueryFailsOpenToOriginalLane(t *testing.T) {
	provider := &ragRewriteProvider{err: errors.New("provider unavailable")}
	rewritten, err := rewriteRAGQuery(
		context.Background(),
		provider,
		ModelRef{ProviderID: "mock", ModelID: "mock-chat"},
		testMessageID,
		"Tell me more about it.",
		[]Message{{ID: "prior", Role: "assistant", Content: "Project ALPHA-42"}},
	)
	if err == nil || rewritten != "" {
		t.Fatalf("rewrite = %q, err=%v", rewritten, err)
	}
}

func TestHandlerAutoRAGRewritesContextualFollowUpAndSearchesBothQueries(t *testing.T) {
	repo := newFakeRepository()
	repo.conversations = append(repo.conversations, fakeConversation(testConversationID, "Contextual", 3))
	repo.messages[testConversationID] = []Message{
		fakeMessage("11111111-1111-4111-8111-111111111118", testConversationID, 0, "user", "文档里的研究方向是什么？"),
		fakeMessage("11111111-1111-4111-8111-111111111119", testConversationID, 1, "assistant", "研究方向是推荐系统。"),
		fakeMessage(testMessageID, testConversationID, 2, "user", "这个方向有什么成果？"),
	}
	provider := &sequenceRAGProvider{outputs: [][]string{
		{"推荐系统研究方向有什么成果？"},
		{"成果包括个性化推荐实验。[K1]"},
	}}
	source := &fakeRAGCandidateSource{refs: []knowledge.EvidenceCandidateReference{validRAGCandidate()}}
	handler := NewHandler(
		NewService(repo),
		WithProvider(provider),
		WithRAGAnswerAssembler(NewRAGAnswerAssembler(
			source,
			&fakeRAGHydrator{evidence: []knowledge.HydratedEvidence{validHydratedEvidence()}},
		)),
		WithRAGAnswerGovernanceGate(&fakeRAGAnswerGovernanceGate{
			authority: RAGAnswerAuthority{Processor: "mock", ModelID: "mock-chat", CollectionCount: 1},
		}),
	)

	rec := performAuthenticatedRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"metadata":{"selectedKnowledgeCollectionIds":["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"]},"idempotencyKey":"stream-key-contextual-rag"}`,
	)

	assertStreamStatus(t, rec, http.StatusOK)
	if len(source.queries) != 2 || source.queries[0].QueryText != "这个方向有什么成果？" || source.queries[1].QueryText != "推荐系统研究方向有什么成果？" {
		t.Fatalf("candidate queries = %#v", source.queries)
	}
	assistant := repo.messages[testConversationID][len(repo.messages[testConversationID])-1]
	knowledgeMetadata, ok := assistant.Metadata["knowledge"].(map[string]any)
	if !ok || knowledgeMetadata["queryRewritten"] != true || assistant.Content != "成果包括个性化推荐实验。[K1]" {
		t.Fatalf("assistant = %#v", assistant)
	}
}

type ragRewriteProvider struct {
	chunks []string
	err    error
	input  ProviderRequest
}

type sequenceRAGProvider struct {
	outputs [][]string
	calls   int
}

func (p *sequenceRAGProvider) StreamChat(
	ctx context.Context,
	_ ProviderRequest,
) (<-chan ProviderEvent, error) {
	if p.calls >= len(p.outputs) {
		return nil, errors.New("unexpected provider call")
	}
	chunks := p.outputs[p.calls]
	p.calls++
	events := make(chan ProviderEvent, len(chunks))
	for _, chunk := range chunks {
		select {
		case <-ctx.Done():
			close(events)
			return events, nil
		case events <- ProviderEvent{Type: ProviderEventDelta, Delta: chunk}:
		}
	}
	close(events)
	return events, nil
}

func (p *ragRewriteProvider) StreamChat(
	ctx context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	p.input = input
	if p.err != nil {
		return nil, p.err
	}
	events := make(chan ProviderEvent, len(p.chunks))
	for _, chunk := range p.chunks {
		select {
		case <-ctx.Done():
			close(events)
			return events, nil
		case events <- ProviderEvent{Type: ProviderEventDelta, Delta: chunk}:
		}
	}
	close(events)
	return events, nil
}
