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

	prompt, systemPrompt, err := buildAutoRAGProviderRequest(
		"What does alpha say?",
		"Be concise.",
		[]knowledge.HydratedEvidence{evidence},
		[]RAGCitation{citation},
	)

	if err != nil {
		t.Fatalf("buildAutoRAGProviderRequest() error = %v", err)
	}
	for _, want := range []string{
		"User question:",
		"What does alpha say?",
		"Relevant Knowledge evidence:",
		"[K1] alpha evidence source",
		`Locator: {"page":1}`,
		evidence.SourceSpanHash,
		evidence.ContentHash,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q; prompt=%s", want, prompt)
		}
	}
	if !strings.Contains(systemPrompt, "additional context") ||
		!strings.Contains(systemPrompt, "general model knowledge") ||
		!strings.Contains(systemPrompt, "Be concise.") {
		t.Fatalf("system prompt = %q", systemPrompt)
	}
}

func TestBuildAutoRAGProviderRequestRejectsMissingCitationContext(t *testing.T) {
	_, _, err := buildAutoRAGProviderRequest("question", "", []knowledge.HydratedEvidence{validHydratedEvidence()}, nil)
	if err != ErrRAGInsufficientEvidence {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}
