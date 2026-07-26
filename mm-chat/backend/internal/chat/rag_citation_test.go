package chat

import (
	"errors"
	"reflect"
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
	if citation.SourceName != evidence.SourceName {
		t.Fatalf("source name = %q, want %q", citation.SourceName, evidence.SourceName)
	}
	if citation.DisplayLocator != nil {
		t.Fatalf("legacy locator unexpectedly projected for display = %#v", citation.DisplayLocator)
	}
	if string(citation.Locator) != `{"page":1}` {
		t.Fatalf("locator = %s", citation.Locator)
	}
	if citation.Snippet != "alpha evidence source with spaces" {
		t.Fatalf("snippet = %q", citation.Snippet)
	}
}

func TestNormalizeRAGCitationDisplayLocator(t *testing.T) {
	tests := []struct {
		name    string
		locator string
		want    *RAGCitationDisplayLocator
	}{
		{
			name:    "page",
			locator: locatorSummaryFixture("page_bbox", `{"kind":"page_bbox","page":2,"x1":1,"y1":2,"x2":3,"y2":4}`),
			want:    &RAGCitationDisplayLocator{Kind: "page", Page: 3},
		},
		{
			name:    "slide",
			locator: locatorSummaryFixture("slide_shape", `{"kind":"slide_shape","slide":5,"shape":7}`),
			want:    &RAGCitationDisplayLocator{Kind: "slide", Slide: 6},
		},
		{
			name: "cell range strips opaque sheet",
			locator: locatorSummaryFixture(
				"sheet_cell",
				`{"kind":"sheet_cell","sheet":"`+
					strings.Repeat("a", 64)+`","startCell":"a3","endCell":"C12"}`,
			),
			want: &RAGCitationDisplayLocator{
				Kind: "cell_range", StartCell: "A3", EndCell: "C12",
			},
		},
		{
			name:    "line range",
			locator: locatorSummaryFixture("line_range", `{"kind":"line_range","startLine":17,"endLine":34}`),
			want: &RAGCitationDisplayLocator{
				Kind: "line_range", StartLine: 18, EndLine: 35,
			},
		},
		{
			name: "opaque OOXML locator",
			locator: locatorSummaryFixture(
				"ooxml_part_xpath",
				`{"kind":"ooxml_part_xpath","part":"`+
					strings.Repeat("a", 64)+`","xpath":"opaque"}`,
			),
		},
		{
			name:    "kind mismatch",
			locator: locatorSummaryFixture("page_bbox", `{"kind":"line_range","startLine":0,"endLine":1}`),
		},
		{
			name:    "invalid cell",
			locator: locatorSummaryFixture("sheet_cell", `{"kind":"sheet_cell","startCell":"../A1","endCell":"B2"}`),
		},
		{
			name:    "backwards line range",
			locator: locatorSummaryFixture("line_range", `{"kind":"line_range","startLine":5,"endLine":2}`),
		},
		{
			name:    "unknown schema",
			locator: `{"schemaVersion":"future","primary":{"kind":"page_bbox","locator":{"kind":"page_bbox","page":0}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRAGCitationDisplayLocator([]byte(tt.locator))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("display locator = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMintRAGCitationsOmitsUnsafeDisplaySourceName(t *testing.T) {
	evidence := validHydratedEvidence()
	evidence.SourceName = "unsafe\nname.pdf"

	citations, err := mintRAGCitations([]knowledge.HydratedEvidence{evidence})
	if err != nil {
		t.Fatalf("mintRAGCitations() error = %v", err)
	}
	if citations[0].SourceName != "" {
		t.Fatalf("unsafe source name persisted = %q", citations[0].SourceName)
	}
}

func locatorSummaryFixture(kind string, locator string) string {
	return `{"schemaVersion":"g7.4-locator-summary.v1","primary":{"kind":"` +
		kind + `","locator":` + locator + `},"fragments":[]}`
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
