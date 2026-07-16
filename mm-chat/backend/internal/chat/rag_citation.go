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
)

type RAGCitation struct {
	ID                string          `json:"id"`
	Marker            string          `json:"marker"`
	CollectionID      string          `json:"collectionId"`
	DocumentID        string          `json:"documentId"`
	DocumentVersionID string          `json:"documentVersionId"`
	IndexGenerationID string          `json:"indexGenerationId"`
	MaterializationID string          `json:"materializationId"`
	ParentChunkID     string          `json:"parentChunkId"`
	ChildChunkID      string          `json:"childChunkId"`
	SourceSpanHash    string          `json:"sourceSpanHash"`
	ContentHash       string          `json:"contentHash"`
	Locator           json.RawMessage `json:"locator"`
	Snippet           string          `json:"snippet"`
	RankScore         float64         `json:"rankScore"`
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
			Marker:            "[" + strconv.Itoa(index+1) + "]",
			CollectionID:      item.CollectionID,
			DocumentID:        item.DocumentID,
			DocumentVersionID: item.DocumentVersionID,
			IndexGenerationID: item.IndexGenerationID,
			MaterializationID: item.MaterializationID,
			ParentChunkID:     item.ParentChunkID,
			ChildChunkID:      item.ChildChunkID,
			SourceSpanHash:    strings.ToLower(strings.TrimSpace(item.SourceSpanHash)),
			ContentHash:       strings.ToLower(strings.TrimSpace(item.ContentHash)),
			Locator:           append(json.RawMessage(nil), item.Locator...),
			Snippet:           snippet,
			RankScore:         item.RankScore,
		}
		if citation.ID == "" || citation.CollectionID == "" || citation.ChildChunkID == "" || citation.SourceSpanHash == "" || citation.ContentHash == "" {
			return nil, ErrRAGInsufficientEvidence
		}
		citations = append(citations, citation)
	}
	return citations, nil
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
