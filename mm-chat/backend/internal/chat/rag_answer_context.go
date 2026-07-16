package chat

import (
	"encoding/json"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

const strictRAGSystemInstruction = `Strict Knowledge mode is enabled.
Use only the verified Knowledge evidence in the user message.
Every factual claim grounded in Knowledge must cite the matching marker like [1].
If the evidence is insufficient, answer exactly: `

func buildStrictRAGProviderRequest(
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
	system.WriteString(strictRAGSystemInstruction)
	system.WriteString(ragRefusalText())

	var prompt strings.Builder
	prompt.WriteString("User question:\n")
	prompt.WriteString(strings.TrimSpace(userQuestion))
	prompt.WriteString("\n\nVerified Knowledge evidence:\n")
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
	prompt.WriteString("Answer using only the verified evidence above. Include citation markers in the answer.")
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

func strictRAGAnswerCitesEvidence(answer string, citations []RAGCitation) bool {
	answer = strings.TrimSpace(answer)
	if answer == "" || len(citations) == 0 {
		return false
	}
	for _, citation := range citations {
		marker := strings.TrimSpace(citation.Marker)
		if marker != "" && strings.Contains(answer, marker) {
			return true
		}
	}
	return false
}

func strictRAGAnswerProviderMetadata(decision strictRAGDecision) map[string]any {
	metadata := map[string]any{
		"mode":          "strict",
		"citationCount": len(decision.Citations),
	}
	if decision.Authority != nil {
		metadata["answerGovernance"] = *decision.Authority
	}
	return metadata
}
