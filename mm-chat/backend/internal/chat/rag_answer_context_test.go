package chat

import (
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestBuildAutoRAGProviderRequestCarriesRelevantCitationContext(t *testing.T) {
	evidence := validHydratedEvidence()
	citation := RAGCitation{
		ID:             "cit_1",
		Marker:         "[K1]",
		SourceSpanHash: evidence.SourceSpanHash,
		ContentHash:    evidence.ContentHash,
		Locator:        evidence.Locator,
		Snippet:        "alpha evidence source",
	}

	context, err := buildAutoRAGProviderRequest(
		"What does alpha say?",
		"Be concise.",
		[]knowledge.HydratedEvidence{evidence},
		[]RAGCitation{citation},
		4096,
	)

	if err != nil {
		t.Fatalf("buildAutoRAGProviderRequest() error = %v", err)
	}
	for _, want := range []string{
		"User question:",
		"What does alpha say?",
		"Relevant Knowledge evidence:",
		"[K1] Matched Child evidence:",
		`Source file metadata (not Citation evidence): "alpha-source.md"`,
		"alpha evidence source",
		"Expanded Parent context",
		evidence.ParentSourceText,
		`Locator: {"page":1}`,
		evidence.SourceSpanHash,
		evidence.ContentHash,
	} {
		if !strings.Contains(context.Prompt, want) {
			t.Fatalf("prompt missing %q; prompt=%s", want, context.Prompt)
		}
	}
	if !strings.Contains(context.SystemPrompt, "additional context") ||
		!strings.Contains(context.SystemPrompt, "general model knowledge") ||
		!strings.Contains(context.SystemPrompt, "Be concise.") {
		t.Fatalf("system prompt = %q", context.SystemPrompt)
	}
	if len(context.Evidence) != 1 || len(context.Citations) != 1 ||
		context.EstimatedTokens <= 0 || context.EstimatedTokens > 4096 {
		t.Fatalf("context projection = %#v", context)
	}
}

func TestBuildAutoRAGProviderRequestRejectsMissingCitationContext(t *testing.T) {
	_, err := buildAutoRAGProviderRequest(
		"question",
		"",
		[]knowledge.HydratedEvidence{validHydratedEvidence()},
		nil,
		4096,
	)
	if err != ErrRAGInsufficientEvidence {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}

func TestProjectKnowledgeEvidenceKeepsChildrenFirstAndDeduplicatesParents(t *testing.T) {
	first := validHydratedEvidence()
	first.RankScore = 1
	second := first
	second.ChildChunkID = "11111111-1111-4111-8111-111111111112"
	second.SourceText = "second matched child"
	second.ContentHash = strings.Repeat("d", 64)
	second.RankScore = 0.9
	third := first
	third.ParentChunkID = "22222222-2222-4222-8222-222222222223"
	third.ChildChunkID = "11111111-1111-4111-8111-111111111113"
	third.SourceText = "third matched child"
	third.ParentSourceText = "a different parent context"
	third.ContentHash = strings.Repeat("e", 64)
	third.RankScore = 0.4
	evidence := []knowledge.HydratedEvidence{first, second, third}
	citations, err := mintRAGCitations(evidence)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := projectKnowledgeEvidence(evidence, citations, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(projection.Blocks))
	}
	expanded := 0
	for _, block := range projection.Blocks {
		if block.ParentSourceText != "" {
			expanded++
		}
	}
	if expanded != 1 || projection.Blocks[0].ParentSourceText == "" ||
		projection.Blocks[1].ParentSourceText != "" ||
		projection.Blocks[2].ParentSourceText != "" {
		t.Fatalf("parent expansion = %#v", projection.Blocks)
	}
}

func TestProjectKnowledgeEvidenceAdaptsChildCountToBudget(t *testing.T) {
	first := validHydratedEvidence()
	first.ParentSourceText = first.SourceText
	second := first
	second.ChildChunkID = "11111111-1111-4111-8111-111111111114"
	second.ContentHash = strings.Repeat("f", 64)
	second.SourceText = strings.Repeat("second child context ", 200)
	second.ParentSourceText = second.SourceText
	citations, err := mintRAGCitations([]knowledge.HydratedEvidence{first, second})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := projectKnowledgeEvidence(
		[]knowledge.HydratedEvidence{first, second},
		citations,
		512,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Evidence) != 1 ||
		projection.Evidence[0].ChildChunkID != first.ChildChunkID {
		t.Fatalf("budgeted evidence = %#v", projection.Evidence)
	}
}
