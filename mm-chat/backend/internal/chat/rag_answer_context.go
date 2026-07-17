package chat

import (
	"encoding/json"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

const autoRAGSystemInstruction = `Relevant Knowledge evidence is included with the user question.
Use it as additional context when it helps answer the question; you may also use general model knowledge.
Every claim supported by Knowledge should cite the matching marker like [K1].
Do not claim or cite a source that you did not use.`

func buildAutoRAGProviderRequest(
	userQuestion string,
	baseSystemPrompt string,
	evidence []knowledge.HydratedEvidence,
	citations []RAGCitation,
) (string, string, error) {
	if len(evidence) == 0 || len(citations) == 0 || len(evidence) < len(citations) {
		return "", "", ErrRAGInsufficientEvidence
	}
	var system strings.Builder
	if trimmed := strings.TrimSpace(baseSystemPrompt); trimmed != "" {
		system.WriteString(trimmed)
		system.WriteString("\n\n")
	}
	system.WriteString(autoRAGSystemInstruction)

	var prompt strings.Builder
	prompt.WriteString("User question:\n")
	prompt.WriteString(strings.TrimSpace(userQuestion))
	prompt.WriteString("\n\nRelevant Knowledge evidence:\n")
	for index, citation := range citations {
		if index >= len(evidence) || strings.TrimSpace(citation.Snippet) == "" || strings.TrimSpace(citation.Marker) == "" {
			return "", "", ErrRAGInsufficientEvidence
		}
		locator := compactCitationLocator(citation.Locator)
		prompt.WriteString(citation.Marker)
		prompt.WriteString(" ")
		prompt.WriteString(citation.Snippet)
		if locator != "" {
			prompt.WriteString("\nLocator: ")
			prompt.WriteString(locator)
		}
		prompt.WriteString("\nSource hash: ")
		prompt.WriteString(citation.SourceSpanHash)
		prompt.WriteString(" / ")
		prompt.WriteString(citation.ContentHash)
		prompt.WriteString("\n\n")
	}
	prompt.WriteString("Answer naturally. Cite Knowledge markers for claims that use the evidence above.")
	return prompt.String(), system.String(), nil
}

func compactCitationLocator(locator json.RawMessage) string {
	if !json.Valid(locator) {
		return ""
	}
	var value any
	if err := json.Unmarshal(locator, &value); err != nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func autoRAGAnswerProviderMetadata(decision autoRAGDecision) map[string]any {
	metadata := map[string]any{
		"mode":          "auto",
		"citationCount": len(decision.Citations),
		"citations":     append([]RAGCitation(nil), decision.Citations...),
	}
	if decision.Authority != nil {
		metadata["answerGovernance"] = *decision.Authority
	}
	return metadata
}
