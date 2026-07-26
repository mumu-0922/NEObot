package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

const (
	maxRAGCitations            = 8
	maxRAGCitationSnippetRunes = 480
	maxRAGCitationSourceBytes  = 512
	maxRAGDisplayCoordinate    = 1_000_000_000
)

type RAGCitationDisplayLocator struct {
	Kind      string `json:"kind"`
	Page      int    `json:"page,omitempty"`
	Slide     int    `json:"slide,omitempty"`
	StartCell string `json:"startCell,omitempty"`
	EndCell   string `json:"endCell,omitempty"`
	StartLine int    `json:"startLine,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
}

type RAGCitation struct {
	ID                string                     `json:"id"`
	Marker            string                     `json:"marker"`
	CollectionID      string                     `json:"collectionId"`
	DocumentID        string                     `json:"documentId"`
	DocumentVersionID string                     `json:"documentVersionId"`
	IndexGenerationID string                     `json:"indexGenerationId"`
	MaterializationID string                     `json:"materializationId"`
	ParentChunkID     string                     `json:"parentChunkId"`
	ChildChunkID      string                     `json:"childChunkId"`
	SourceSpanHash    string                     `json:"sourceSpanHash"`
	ContentHash       string                     `json:"contentHash"`
	SourceName        string                     `json:"sourceName,omitempty"`
	DisplayLocator    *RAGCitationDisplayLocator `json:"displayLocator,omitempty"`
	Locator           json.RawMessage            `json:"locator"`
	Snippet           string                     `json:"snippet"`
	RankScore         float64                    `json:"rankScore"`
}

func mintRAGCitations(evidence []knowledge.HydratedEvidence) ([]RAGCitation, error) {
	if len(evidence) == 0 {
		return nil, ErrRAGInsufficientEvidence
	}
	limit := len(evidence)
	if limit > maxRAGCitations {
		limit = maxRAGCitations
	}
	citations := make([]RAGCitation, 0, limit)
	for index := 0; index < limit; index++ {
		item := evidence[index]
		snippet := normalizeRAGCitationSnippet(item.SourceText)
		if snippet == "" || !json.Valid(item.Locator) {
			return nil, ErrRAGInsufficientEvidence
		}
		citation := RAGCitation{
			ID:                ragCitationID(item),
			Marker:            "[K" + strconv.Itoa(index+1) + "]",
			CollectionID:      item.CollectionID,
			DocumentID:        item.DocumentID,
			DocumentVersionID: item.DocumentVersionID,
			IndexGenerationID: item.IndexGenerationID,
			MaterializationID: item.MaterializationID,
			ParentChunkID:     item.ParentChunkID,
			ChildChunkID:      item.ChildChunkID,
			SourceSpanHash:    strings.ToLower(strings.TrimSpace(item.SourceSpanHash)),
			ContentHash:       strings.ToLower(strings.TrimSpace(item.ContentHash)),
			SourceName:        normalizeRAGCitationSourceName(item.SourceName),
			DisplayLocator:    normalizeRAGCitationDisplayLocator(item.Locator),
			Locator:           append(json.RawMessage(nil), item.Locator...),
			Snippet:           snippet,
			RankScore:         item.RankScore,
		}
		if citation.ID == "" || citation.CollectionID == "" ||
			citation.ChildChunkID == "" || citation.SourceSpanHash == "" ||
			citation.ContentHash == "" {
			return nil, ErrRAGInsufficientEvidence
		}
		citations = append(citations, citation)
	}
	return citations, nil
}

func normalizeRAGCitationSourceName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len([]byte(value)) > maxRAGCitationSourceBytes {
		return ""
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return ""
		}
	}
	return value
}

func normalizeRAGCitationDisplayLocator(
	value json.RawMessage,
) *RAGCitationDisplayLocator {
	type locatorSummaryPrimary struct {
		Kind    string          `json:"kind"`
		Locator json.RawMessage `json:"locator"`
	}
	type locatorSummary struct {
		SchemaVersion string                 `json:"schemaVersion"`
		Primary       *locatorSummaryPrimary `json:"primary"`
	}
	type rawLocator struct {
		Kind      string `json:"kind"`
		Page      *int   `json:"page"`
		Slide     *int   `json:"slide"`
		StartCell string `json:"startCell"`
		EndCell   string `json:"endCell"`
		StartLine *int   `json:"startLine"`
		EndLine   *int   `json:"endLine"`
	}

	var summary locatorSummary
	if err := json.Unmarshal(value, &summary); err != nil ||
		summary.SchemaVersion != "g7.4-locator-summary.v1" ||
		summary.Primary == nil || len(summary.Primary.Locator) == 0 {
		return nil
	}

	var locator rawLocator
	if err := json.Unmarshal(summary.Primary.Locator, &locator); err != nil ||
		locator.Kind == "" || locator.Kind != summary.Primary.Kind {
		return nil
	}

	switch locator.Kind {
	case "page_bbox":
		page, ok := humanRAGDisplayCoordinate(locator.Page)
		if !ok {
			return nil
		}
		return &RAGCitationDisplayLocator{Kind: "page", Page: page}
	case "slide_shape":
		slide, ok := humanRAGDisplayCoordinate(locator.Slide)
		if !ok {
			return nil
		}
		return &RAGCitationDisplayLocator{Kind: "slide", Slide: slide}
	case "sheet_cell":
		startCell := normalizeRAGDisplayCell(locator.StartCell)
		endCell := normalizeRAGDisplayCell(locator.EndCell)
		if startCell == "" || endCell == "" {
			return nil
		}
		return &RAGCitationDisplayLocator{
			Kind:      "cell_range",
			StartCell: startCell,
			EndCell:   endCell,
		}
	case "line_range":
		startLine, startOK := humanRAGDisplayCoordinate(locator.StartLine)
		endLine, endOK := humanRAGDisplayCoordinate(locator.EndLine)
		if !startOK || !endOK || endLine < startLine {
			return nil
		}
		return &RAGCitationDisplayLocator{
			Kind:      "line_range",
			StartLine: startLine,
			EndLine:   endLine,
		}
	default:
		return nil
	}
}

func humanRAGDisplayCoordinate(value *int) (int, bool) {
	if value == nil || *value < 0 || *value >= maxRAGDisplayCoordinate {
		return 0, false
	}
	return *value + 1, true
}

func normalizeRAGDisplayCell(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) < 2 || len(value) > 15 {
		return ""
	}
	letterCount := 0
	for letterCount < len(value) && value[letterCount] >= 'A' && value[letterCount] <= 'Z' {
		letterCount++
	}
	if letterCount < 1 || letterCount > 4 || letterCount == len(value) || value[letterCount] == '0' {
		return ""
	}
	for index := letterCount; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return ""
		}
	}
	return value
}

func ragCitationID(evidence knowledge.HydratedEvidence) string {
	parts := []string{
		evidence.CollectionID,
		evidence.DocumentID,
		evidence.DocumentVersionID,
		evidence.IndexGenerationID,
		evidence.MaterializationID,
		evidence.ParentChunkID,
		evidence.ChildChunkID,
		strings.ToLower(strings.TrimSpace(evidence.SourceSpanHash)),
		strings.ToLower(strings.TrimSpace(evidence.ContentHash)),
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return ""
		}
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "cit_" + hex.EncodeToString(digest[:16])
}

func normalizeRAGCitationSnippet(source string) string {
	fields := strings.Fields(source)
	if len(fields) == 0 {
		return ""
	}
	snippet := strings.Join(fields, " ")
	if utf8.RuneCountInString(snippet) <= maxRAGCitationSnippetRunes {
		return snippet
	}
	runes := []rune(snippet)
	return strings.TrimSpace(string(runes[:maxRAGCitationSnippetRunes-1])) + "…"
}
