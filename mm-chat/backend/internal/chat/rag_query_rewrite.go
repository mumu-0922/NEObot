package chat

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

const (
	maxRAGRewriteHistoryMessages = 6
	maxRAGRewriteMessageBytes    = 1200
	maxRAGRewrittenQueryBytes    = 2048
)

var ragContextualChineseMarkers = []string{
	"这个", "那个", "它", "它们", "他们", "上述", "前面", "刚才", "其中",
	"继续", "再详细", "展开说", "这件事", "该内容", "该文档",
}

var ragContextualEnglishMarkers = map[string]struct{}{
	"it": {}, "its": {}, "that": {}, "this": {}, "they": {}, "them": {},
	"their": {}, "those": {}, "these": {}, "above": {}, "previous": {},
	"former": {}, "continue": {},
}

func shouldRewriteRAGQuery(query string) bool {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > maxRAGRewrittenQueryBytes {
		return false
	}
	lower := strings.ToLower(query)
	for _, marker := range ragContextualChineseMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, token := range strings.FieldsFunc(lower, func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsNumber(value)
	}) {
		if _, ok := ragContextualEnglishMarkers[token]; ok {
			return true
		}
	}
	return false
}

func rewriteRAGQuery(
	ctx context.Context,
	provider Provider,
	modelRef ModelRef,
	currentMessageID string,
	query string,
	messages []Message,
) (string, error) {
	if provider == nil || !shouldRewriteRAGQuery(query) {
		return "", nil
	}
	history := recentRAGRewriteHistory(messages, currentMessageID)
	if len(history) == 0 {
		return "", nil
	}

	var prompt strings.Builder
	prompt.WriteString("Conversation context:\n")
	for _, message := range history {
		prompt.WriteString(message.Role)
		prompt.WriteString(": ")
		prompt.WriteString(message.Content)
		prompt.WriteByte('\n')
	}
	prompt.WriteString("\nFollow-up question:\n")
	prompt.WriteString(strings.TrimSpace(query))
	prompt.WriteString("\n\nStandalone retrieval query:")

	events, err := provider.StreamChat(ctx, ProviderRequest{
		Prompt:       prompt.String(),
		SystemPrompt: "Rewrite only context-dependent follow-up questions into one standalone retrieval query. Preserve exact names, identifiers, numbers, and language. Return only the rewritten query without explanation.",
		ModelRef:     modelRef,
	})
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for event := range events {
		if event.Error != nil {
			return "", event.Error
		}
		if event.Type != ProviderEventDelta || event.Delta == "" {
			continue
		}
		if output.Len()+len(event.Delta) > maxRAGRewrittenQueryBytes {
			return "", errors.New("rag query rewrite exceeds limit")
		}
		output.WriteString(event.Delta)
	}
	rewritten := normalizeRAGRewrittenQuery(output.String())
	if rewritten == "" || strings.EqualFold(rewritten, strings.TrimSpace(query)) {
		return "", nil
	}
	return rewritten, nil
}

func recentRAGRewriteHistory(messages []Message, currentMessageID string) []Message {
	history := make([]Message, 0, maxRAGRewriteHistoryMessages)
	for index := len(messages) - 1; index >= 0 && len(history) < maxRAGRewriteHistoryMessages; index-- {
		message := messages[index]
		if message.ID == currentMessageID || (message.Role != "user" && message.Role != "assistant") {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		content = truncateRAGRewriteText(content, maxRAGRewriteMessageBytes)
		message.Content = content
		history = append(history, message)
	}
	for left, right := 0, len(history)-1; left < right; left, right = left+1, right-1 {
		history[left], history[right] = history[right], history[left]
	}
	return history
}

func truncateRAGRewriteText(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	end := 0
	for index := range value {
		if index > maxBytes {
			break
		}
		end = index
	}
	if end == 0 {
		return ""
	}
	return strings.TrimSpace(value[:end])
}

func normalizeRAGRewrittenQuery(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```text")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"Standalone retrieval query:", "Standalone query:", "Query:"} {
		if strings.HasPrefix(strings.ToLower(value), strings.ToLower(prefix)) {
			value = strings.TrimSpace(value[len(prefix):])
			break
		}
	}
	value = strings.Trim(value, "\"'` \t\r\n")
	if newline := strings.IndexByte(value, '\n'); newline >= 0 {
		value = strings.TrimSpace(value[:newline])
	}
	if value == "" || len(value) > maxRAGRewrittenQueryBytes {
		return ""
	}
	return value
}
