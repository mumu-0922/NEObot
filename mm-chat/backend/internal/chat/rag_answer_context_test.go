package chat

import (
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestBuildStrictRAGProviderRequestCarriesOnlyVerifiedCitationContext(t *testing.T) {
	evidence := validHydratedEvidence()
	citation := RAGCitation{
		ID:             "cit_1",
		Marker:         "[1]",
		SourceSpanHash: evidence.SourceSpanHash,
		ContentHash:    evidence.ContentHash,
		Locator:        evidence.Locator,
		Snippet:        "alpha evidence source",
	}

	prompt, systemPrompt, err := buildStrictRAGProviderRequest(
		"What does alpha say?",
		"Be concise.",
		[]knowledge.HydratedEvidence{evidence},
		[]RAGCitation{citation},
	)

	if err != nil {
		t.Fatalf("buildStrictRAGProviderRequest() error = %v", err)
	}
	for _, want := range []string{
		"User question:",
		"What does alpha say?",
		"Verified Knowledge evidence:",
		"[1] alpha evidence source",
		`Locator: {"page":1}`,
		evidence.SourceSpanHash,
		evidence.ContentHash,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q; prompt=%s", want, prompt)
		}
	}
	if !strings.Contains(systemPrompt, "Strict Knowledge mode") || !strings.Contains(systemPrompt, "Be concise.") || !strings.Contains(systemPrompt, ragRefusalText()) {
		t.Fatalf("system prompt = %q", systemPrompt)
	}
}

func TestBuildStrictRAGProviderRequestRejectsMissingCitationContext(t *testing.T) {
	_, _, err := buildStrictRAGProviderRequest("question", "", []knowledge.HydratedEvidence{validHydratedEvidence()}, nil)
	if err != ErrRAGInsufficientEvidence {
		t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
	}
}

func TestStrictRAGAnswerCitesEvidence(t *testing.T) {
	citations := []RAGCitation{{Marker: "[1]"}, {Marker: "[2]"}}
	if !strictRAGAnswerCitesEvidence("Answer [2]", citations) {
		t.Fatal("answer with marker was rejected")
	}
	if strictRAGAnswerCitesEvidence("Answer without marker", citations) {
		t.Fatal("answer without marker was accepted")
	}
}
