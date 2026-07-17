package chat

import (
	"errors"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func TestMintRAGCitationsBindsHashesLocatorAndSnippet(t *testing.T) {
	evidence := validHydratedEvidence()
	evidence.SourceText = " alpha\n\n evidence   source with   spaces "

	citations, err := mintRAGCitations([]knowledge.HydratedEvidence{evidence})
	if err != nil {
		t.Fatalf("mintRAGCitations() error = %v", err)
	}
	if len(citations) != 1 {
		t.Fatalf("citations = %d, want 1", len(citations))
	}
	citation := citations[0]
	if !strings.HasPrefix(citation.ID, "cit_") || len(citation.ID) != len("cit_")+32 {
		t.Fatalf("citation id = %q", citation.ID)
	}
	if citation.Marker != "[K1]" {
		t.Fatalf("marker = %q, want [K1]", citation.Marker)
	}
	if citation.SourceSpanHash != evidence.SourceSpanHash || citation.ContentHash != evidence.ContentHash {
		t.Fatalf("citation hashes = %s/%s", citation.SourceSpanHash, citation.ContentHash)
	}
	if string(citation.Locator) != `{"page":1}` {
		t.Fatalf("locator = %s", citation.Locator)
	}
	if citation.Snippet != "alpha evidence source with spaces" {
		t.Fatalf("snippet = %q", citation.Snippet)
	}
}

func TestMintRAGCitationsRejectsUnsafeEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*knowledge.HydratedEvidence)
	}{
		{name: "empty source", mutate: func(e *knowledge.HydratedEvidence) { e.SourceText = " " }},
		{name: "invalid locator", mutate: func(e *knowledge.HydratedEvidence) { e.Locator = []byte(`{`) }},
		{name: "missing child", mutate: func(e *knowledge.HydratedEvidence) { e.ChildChunkID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := validHydratedEvidence()
			tt.mutate(&evidence)

			_, err := mintRAGCitations([]knowledge.HydratedEvidence{evidence})
			if !errors.Is(err, ErrRAGInsufficientEvidence) {
				t.Fatalf("error = %v, want ErrRAGInsufficientEvidence", err)
			}
		})
	}
}

func TestNormalizeRAGCitationSnippetTruncates(t *testing.T) {
	long := strings.Repeat("界", maxRAGCitationSnippetRunes+20)
	snippet := normalizeRAGCitationSnippet(long)
	if !strings.HasSuffix(snippet, "…") {
		t.Fatalf("snippet does not end with ellipsis: %q", snippet)
	}
	if got := len([]rune(snippet)); got != maxRAGCitationSnippetRunes {
		t.Fatalf("snippet runes = %d, want %d", got, maxRAGCitationSnippetRunes)
	}
}
