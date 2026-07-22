package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	maxWebSourceSnippetBytes = 8 << 10
	maxWebContextBytes       = 64 << 10
)

const webSearchSystemInstruction = `Relevant Web evidence is included with the user request.
Use it only when it helps answer the request. Cite every Web-supported claim with the matching marker such as [W1].
Do not invent a marker or claim that an unused source supports the answer.`

type WebCitation struct {
	ID      string `json:"id"`
	Marker  string `json:"marker"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

func prepareWebSearchResult(input websearch.Result) (websearch.Result, []WebCitation) {
	normalized := websearch.NormalizeResult(input, websearch.MaxResults)
	bounded := websearch.Result{
		Sources: make([]websearch.Source, 0, len(normalized.Sources)),
		Images:  append(make([]websearch.Image, 0, len(normalized.Images)), normalized.Images...),
	}
	citations := make([]WebCitation, 0, len(normalized.Sources))
	remaining := maxWebContextBytes
	for _, source := range normalized.Sources {
		if remaining <= 0 {
			break
		}
		limit := min(maxWebSourceSnippetBytes, remaining)
		source.Content = truncateWebUTF8(source.Content, limit)
		if strings.TrimSpace(source.Content) == "" {
			continue
		}
		remaining -= len(source.Content)
		bounded.Sources = append(bounded.Sources, source)
		index := len(bounded.Sources)
		citations = append(citations, WebCitation{
			ID:      webCitationID(source.URL),
			Marker:  "[W" + strconv.Itoa(index) + "]",
			Title:   source.Title,
			URL:     source.URL,
			Snippet: source.Content,
		})
	}
	return bounded, citations
}

func mergeWebSearchResults(current websearch.Result, incoming websearch.Result) websearch.Result {
	merged := websearch.Result{
		Sources: append(append([]websearch.Source(nil), current.Sources...), incoming.Sources...),
		Images:  append(append([]websearch.Image(nil), current.Images...), incoming.Images...),
	}
	bounded, _ := prepareWebSearchResult(merged)
	return bounded
}

func buildWebSearchProviderRequest(
	basePrompt string,
	baseSystemPrompt string,
	result websearch.Result,
) (string, string) {
	bounded, citations := prepareWebSearchResult(result)
	if len(citations) == 0 {
		return basePrompt, baseSystemPrompt
	}

	var system strings.Builder
	if trimmed := strings.TrimSpace(baseSystemPrompt); trimmed != "" {
		system.WriteString(trimmed)
		system.WriteString("\n\n")
	}
	system.WriteString(webSearchSystemInstruction)

	var prompt strings.Builder
	prompt.WriteString(strings.TrimSpace(basePrompt))
	prompt.WriteString("\n\nRelevant Web evidence:\n")
	for index, citation := range citations {
		prompt.WriteString(citation.Marker)
		prompt.WriteString(" ")
		prompt.WriteString(citation.Title)
		prompt.WriteString("\nURL: ")
		prompt.WriteString(citation.URL)
		prompt.WriteString("\n")
		prompt.WriteString(bounded.Sources[index].Content)
		prompt.WriteString("\n\n")
	}
	prompt.WriteString("Answer naturally and cite Web markers for claims that use the evidence above.")
	return prompt.String(), system.String()
}

func webSearchOutputBlocks(messageID string, result websearch.Result) []any {
	bounded, citations := prepareWebSearchResult(result)
	return webSearchOutputBlocksFromProjection(messageID, bounded, citations)
}

func usedWebSearchOutputBlocks(
	messageID string,
	content string,
	result websearch.Result,
) []any {
	bounded, citations := usedWebSearchProjection(content, result)
	return webSearchOutputBlocksFromProjection(messageID, bounded, citations)
}

func webSearchOutputBlocksFromProjection(
	messageID string,
	bounded websearch.Result,
	citations []WebCitation,
) []any {
	if len(citations) == 0 && len(bounded.Images) == 0 {
		return nil
	}
	sources := make([]any, 0, len(citations))
	for _, citation := range citations {
		sources = append(sources, map[string]any{
			"title":   citation.Title,
			"url":     citation.URL,
			"content": citation.Snippet,
			"metadata": map[string]any{
				"citationId": citation.ID,
				"marker":     citation.Marker,
			},
		})
	}
	images := make([]any, 0, len(bounded.Images))
	for _, image := range bounded.Images {
		images = append(images, map[string]any{
			"url":         image.URL,
			"description": image.Description,
		})
	}
	return []any{map[string]any{
		"id":          messageID + "-web-sources",
		"type":        "search",
		"isSearching": false,
		"sources":     sources,
		"images":      images,
	}}
}

func withWebSearchMessageMetadata(
	base map[string]any,
	execution *websearch.ActiveExecution,
	result websearch.Result,
) map[string]any {
	bounded, citations := prepareWebSearchResult(result)
	return withWebSearchMetadataProjection(base, execution, bounded, citations)
}

func withUsedWebSearchMessageMetadata(
	base map[string]any,
	execution *websearch.ActiveExecution,
	content string,
	result websearch.Result,
) map[string]any {
	bounded, citations := usedWebSearchProjection(content, result)
	return withWebSearchMetadataProjection(base, execution, bounded, citations)
}

func withWebSearchMetadataProjection(
	base map[string]any,
	execution *websearch.ActiveExecution,
	bounded websearch.Result,
	citations []WebCitation,
) map[string]any {
	metadata := ensureObject(base)
	if execution == nil {
		return metadata
	}
	provider := string(execution.ModelBuiltIn)
	if execution.Mode == websearch.ExecutionExternal && execution.External != nil {
		provider = string(execution.External.ID())
	}
	metadata["web"] = map[string]any{
		"enabled":       true,
		"mode":          execution.Mode,
		"provider":      provider,
		"sourceCount":   len(bounded.Sources),
		"imageCount":    len(bounded.Images),
		"citationCount": len(citations),
		"citations":     citations,
	}
	return metadata
}

// usedWebSearchProjection keeps the marker minted for each source in the
// original current-turn result. Filtering [W1] out must not silently rename a
// surviving [W2] to [W1]. Images have no answer marker, so they remain live
// retrieval artifacts and are not projected into durable Citations.
func usedWebSearchProjection(
	content string,
	result websearch.Result,
) (websearch.Result, []WebCitation) {
	bounded, citations := prepareWebSearchResult(result)
	used := websearch.Result{Sources: []websearch.Source{}, Images: []websearch.Image{}}
	usedCitations := make([]WebCitation, 0, len(citations))
	for index, citation := range citations {
		if !strings.Contains(content, citation.Marker) {
			continue
		}
		used.Sources = append(used.Sources, bounded.Sources[index])
		usedCitations = append(usedCitations, citation)
	}
	return used, usedCitations
}

func missingBuiltInWebCitationDelta(content string, result websearch.Result) string {
	_, citations := prepareWebSearchResult(result)
	missing := make([]string, 0, len(citations))
	for _, citation := range citations {
		if !strings.Contains(content, citation.Marker) {
			missing = append(missing, citation.Marker)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "\n\nSources: " + strings.Join(missing, " ")
}

func webCitationID(sourceURL string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(sourceURL)))
	return "web_" + hex.EncodeToString(digest[:16])
}

func truncateWebUTF8(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for value != "" && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}
