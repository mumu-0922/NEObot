package chat

import (
	"strings"
	"testing"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestPrepareWebSearchResultMintsStableBoundedCitations(t *testing.T) {
	longContent := strings.Repeat("界", maxWebSourceSnippetBytes)
	result, citations := prepareWebSearchResult(websearch.Result{
		Sources: []websearch.Source{{
			Title: "Fixture", URL: "https://example.com/source#fragment", Content: longContent,
		}},
	})

	if len(result.Sources) != 1 || len(citations) != 1 {
		t.Fatalf("result/citations = %#v / %#v", result, citations)
	}
	if len(result.Sources[0].Content) > maxWebSourceSnippetBytes ||
		!utf8.ValidString(result.Sources[0].Content) {
		t.Fatalf("bounded source content is invalid: bytes=%d", len(result.Sources[0].Content))
	}
	if citations[0].Marker != "[W1]" || citations[0].URL != "https://example.com/source" ||
		!strings.HasPrefix(citations[0].ID, "web_") {
		t.Fatalf("citation = %#v", citations[0])
	}
}

func TestBuildWebSearchProviderRequestAndArtifacts(t *testing.T) {
	result := websearch.Result{
		Sources: []websearch.Source{{
			Title: "Fixture", URL: "https://example.com/source", Content: "fresh evidence",
		}},
		Images: []websearch.Image{{URL: "https://example.com/image.png", Description: "fixture"}},
	}
	prompt, system := buildWebSearchProviderRequest("question", "base", result)
	if !strings.Contains(prompt, "[W1] Fixture") || !strings.Contains(prompt, "fresh evidence") ||
		!strings.Contains(system, "matching marker such as [W1]") {
		t.Fatalf("prompt/system = %q / %q", prompt, system)
	}

	blocks := webSearchOutputBlocks("message-id", result)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %#v", blocks)
	}
	block, ok := blocks[0].(map[string]any)
	if !ok || block["id"] != "message-id-web-sources" || block["type"] != "search" {
		t.Fatalf("block = %#v", blocks[0])
	}
	metadata := withWebSearchMessageMetadata(
		map[string]any{"runId": "run"},
		&websearch.ActiveExecution{
			Mode: websearch.ExecutionExternal, External: &fakeWebSearchProvider{},
		},
		result,
	)
	web, ok := metadata["web"].(map[string]any)
	if !ok || web["sourceCount"] != 1 || web["imageCount"] != 1 || web["provider"] != "tavily" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestBuildWebSearchProviderRequestAdaptsToTokenBudget(t *testing.T) {
	result := websearch.Result{Sources: []websearch.Source{{
		Title: "Large fixture",
		URL:   "https://example.com/large",
		Content: strings.Repeat(
			"large multilingual evidence 内容 ",
			2000,
		),
	}}}
	context := buildWebSearchProviderRequestWithBudget(
		"question",
		"base",
		result,
		512,
	)
	if len(context.Result.Sources) != 1 ||
		context.Result.Sources[0].Content == result.Sources[0].Content ||
		context.EstimatedTokens <= 0 || context.EstimatedTokens > 512 {
		t.Fatalf("budgeted Web context = %#v", context)
	}
}

func TestMissingBuiltInWebCitationDeltaAddsOnlyMissingMarkers(t *testing.T) {
	result := websearch.Result{Sources: []websearch.Source{
		{Title: "A", URL: "https://example.com/a", Content: "a"},
		{Title: "B", URL: "https://example.com/b", Content: "b"},
	}}
	if got := missingBuiltInWebCitationDelta("answer [W1]", result); got != "\n\nSources: [W2]" {
		t.Fatalf("delta = %q", got)
	}
}

func TestUsedWebSearchProjectionKeepsOriginalMarkerWithoutRenumbering(t *testing.T) {
	result := websearch.Result{Sources: []websearch.Source{
		{Title: "Unused", URL: "https://example.test/unused", Content: "unused"},
		{Title: "Used", URL: "https://example.test/used", Content: "used"},
	}}
	bounded, citations := usedWebSearchProjection("answer [W2]", result)
	if len(bounded.Sources) != 1 || bounded.Sources[0].Title != "Used" ||
		len(citations) != 1 || citations[0].Marker != "[W2]" {
		t.Fatalf("projection = %#v / %#v", bounded, citations)
	}
	blocks := usedWebSearchOutputBlocks("message-id", "answer [W2]", result)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %#v", blocks)
	}
	block := blocks[0].(map[string]any)
	source := block["sources"].([]any)[0].(map[string]any)
	metadata := source["metadata"].(map[string]any)
	if metadata["marker"] != "[W2]" {
		t.Fatalf("source metadata = %#v", metadata)
	}
}
