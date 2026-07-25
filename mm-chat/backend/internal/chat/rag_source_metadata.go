package chat

import (
	"strconv"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

func formatRAGRerankDocument(evidence knowledge.HydratedEvidence) string {
	content := strings.TrimSpace(evidence.SourceText)
	label := renderRAGSourceMetadata(evidence.SourceName)
	if label == "" {
		return content
	}
	return label + "\nMatched Child source:\n" + content
}

func renderRAGSourceMetadata(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len([]byte(value)) > 512 ||
		strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return "Source file metadata (not Citation evidence): " + strconv.Quote(value)
}
