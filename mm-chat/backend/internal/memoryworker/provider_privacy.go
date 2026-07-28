package memoryworker

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	sensitivityNormal = iota
	sensitivitySensitive
	sensitivitySecret
)

var (
	uuidRE = regexp.MustCompile(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-` +
			`[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`,
	)
)

type providerMessage struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	ObservedAt time.Time `json:"observedAt"`
}

type decisionMemory struct {
	ID             string `json:"id"`
	Revision       int64  `json:"revision"`
	Type           string `json:"type"`
	Content        string `json:"content"`
	AuthorityKind  string `json:"authorityKind"`
	ScopeType      string `json:"scopeType"`
	ProjectID      string `json:"projectId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	FactKey        string `json:"factKey,omitempty"`
}

func prepareProviderMessages(job Job, capture Capture) ([]providerMessage, bool) {
	prepared := make([]providerMessage, 0, len(capture.Messages))
	remaining := memoryExtractionInputChars
	sourceVisible := false
	for index := len(capture.Messages) - 1; index >= 0 && remaining > 0; index-- {
		message := capture.Messages[index]
		content := redactProviderText(message.Content, capture.SensitiveMemoryEnabled)
		content = truncateRunes(strings.TrimSpace(content), remaining)
		if content == "" {
			continue
		}
		remaining -= utf8.RuneCountInString(content)
		if message.ID == job.SourceMessageID && message.Role == "user" {
			sourceVisible = true
		}
		prepared = append(prepared, providerMessage{
			ID: message.ID, Role: message.Role, Content: content,
			ObservedAt: message.ObservedAt.UTC(),
		})
	}
	for left, right := 0, len(prepared)-1; left < right; left, right = left+1, right-1 {
		prepared[left], prepared[right] = prepared[right], prepared[left]
	}
	return prepared, sourceVisible
}

func prepareDecisionMemories(capture Capture) []decisionMemory {
	result := make([]decisionMemory, 0, len(capture.CurrentMemories))
	for _, memory := range capture.CurrentMemories {
		content := redactProviderText(memory.Content, capture.SensitiveMemoryEnabled)
		if strings.TrimSpace(content) == "" {
			continue
		}
		result = append(result, decisionMemory{
			ID: memory.ID, Revision: memory.Revision, Type: memory.Type,
			Content: content, AuthorityKind: memory.AuthorityKind,
			ScopeType: memory.ScopeType, ProjectID: memory.ProjectID,
			ConversationID: memory.ConversationID, FactKey: memory.FactKey,
		})
	}
	return result
}

func redactProviderText(value string, allowSensitive bool) string {
	return usermemory.RedactMemoryProviderText(value, allowSensitive)
}

func classifySensitivity(value string) int {
	switch usermemory.ClassifyMemorySensitivity(value) {
	case usermemory.SensitivitySecret:
		return sensitivitySecret
	case usermemory.SensitivitySensitive:
		return sensitivitySensitive
	default:
		return sensitivityNormal
	}
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
