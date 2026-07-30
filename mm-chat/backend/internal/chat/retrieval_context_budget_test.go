package chat

import (
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestRetrievalContextBudgetSharesSelectedModelCapacity(t *testing.T) {
	request := ProviderRequest{
		Prompt:       "question",
		SystemPrompt: "base",
		Messages: []ProviderMessage{{
			Role: "user", Content: "question",
		}},
		ModelRef: ModelRef{ModelID: "unknown-32k-model"},
	}
	budget := newRetrievalContextBudget(request, true, true, false)
	if budget.totalTokens <= 0 ||
		budget.knowledgeTokens+budget.webTokens != budget.totalTokens ||
		budget.knowledgeTokens != budget.totalTokens*retrievalKnowledgeSharePercent/100 {
		t.Fatalf("shared budget = %#v", budget)
	}
	knowledgeRemaining := budget.remaining(retrievalEvidenceKnowledge)
	webRemaining := budget.remaining(retrievalEvidenceWeb)
	budget.consume(retrievalEvidenceKnowledge, knowledgeRemaining+100)
	if budget.remaining(retrievalEvidenceKnowledge) != 0 ||
		budget.remaining(retrievalEvidenceWeb) != webRemaining {
		t.Fatalf("lane consumption escaped its share = %#v", budget)
	}
}

func TestKnowledgeAndWebContextsStayInsideSharedBudget(t *testing.T) {
	request := ProviderRequest{
		Prompt:       "Compare the private plan with current public guidance.",
		SystemPrompt: "Be precise.",
		Messages: []ProviderMessage{{
			Role: "user", Content: "Compare the private plan with current public guidance.",
		}},
		ModelRef: ModelRef{ModelID: "unknown-32k-model"},
	}
	budget := newRetrievalContextBudget(request, true, true, false)
	evidence := validHydratedEvidence()
	evidence.SourceText = strings.Repeat("private child evidence ", 120)
	evidence.ParentSourceText = strings.Repeat("private parent context ", 220)
	evidence.ChildTokenCount = 360
	evidence.ParentTokenCount = 660
	citations, err := mintRAGCitations([]knowledge.HydratedEvidence{evidence})
	if err != nil {
		t.Fatal(err)
	}
	knowledgeContext, err := buildAutoRAGProviderRequest(
		request.Prompt,
		request.SystemPrompt,
		[]knowledge.HydratedEvidence{evidence},
		citations,
		budget.remaining(retrievalEvidenceKnowledge),
	)
	if err != nil {
		t.Fatal(err)
	}
	budget.consume(
		retrievalEvidenceKnowledge,
		knowledgeContext.EstimatedTokens,
	)
	webContext := buildWebSearchProviderRequestWithBudget(
		knowledgeContext.Prompt,
		knowledgeContext.SystemPrompt,
		websearch.Result{Sources: []websearch.Source{{
			Title: "Public source", URL: "https://example.com/public",
			Content: strings.Repeat("public web evidence ", 5000),
		}}},
		budget.remaining(retrievalEvidenceWeb),
	)
	budget.consume(retrievalEvidenceWeb, webContext.EstimatedTokens)
	if len(webContext.Result.Sources) != 1 ||
		knowledgeContext.EstimatedTokens > budget.knowledgeTokens ||
		webContext.EstimatedTokens > budget.webTokens ||
		budget.knowledgeConsumed+budget.webConsumed > budget.totalTokens {
		t.Fatalf(
			"context/budget = knowledge=%d web=%d budget=%#v result=%#v",
			knowledgeContext.EstimatedTokens,
			webContext.EstimatedTokens,
			budget,
			webContext.Result,
		)
	}
}
