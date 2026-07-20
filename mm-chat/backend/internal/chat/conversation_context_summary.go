package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func conversationContextSummaryMatches(
	summary ConversationContextSummary,
	messages []ProviderMessage,
) bool {
	count := summary.SourceMessageCount
	if count <= 0 || count >= len(messages) || len(summary.SourceDigest) != 64 {
		return false
	}
	prefix := messages[:count]
	return prefix[0].MessageID == summary.SourceFirstMessageID &&
		prefix[len(prefix)-1].MessageID == summary.SourceLastMessageID &&
		conversationContextDigest(prefix) == summary.SourceDigest
}

func conversationContextDigest(messages []ProviderMessage) string {
	digest := sha256.New()
	for _, message := range messages {
		for _, value := range []string{message.MessageID, message.Role, message.Content} {
			_, _ = digest.Write([]byte(fmt.Sprintf("%d:", len(value))))
			_, _ = digest.Write([]byte(value))
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func buildConversationSummaryPrompt(
	previousSummary string,
	messages []ProviderMessage,
	inputBudget int,
) (string, error) {
	type summaryMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	payload := struct {
		Version         string           `json:"version"`
		PreviousSummary string           `json:"previousSummary,omitempty"`
		NewMessages     []summaryMessage `json:"newMessages"`
	}{
		Version:         contextSummaryVersion,
		PreviousSummary: previousSummary,
		NewMessages:     make([]summaryMessage, 0, len(messages)),
	}
	for _, message := range messages {
		payload.NewMessages = append(payload.NewMessages, summaryMessage{
			Role: message.Role, Content: message.Content,
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", errContextSummaryUnavailable
	}
	prompt := "Create one cumulative summary from this JSON conversation data:\n" + string(encoded)
	if estimateProviderTextTokens(prompt)+estimateProviderTextTokens(contextSummarySystemInstruction) > inputBudget {
		return "", errContextSummaryUnavailable
	}
	return prompt, nil
}

func generateConversationSummary(
	ctx context.Context,
	provider Provider,
	modelRef ModelRef,
	prompt string,
) (string, error) {
	events, err := provider.StreamChat(ctx, ProviderRequest{
		Prompt:       prompt,
		SystemPrompt: contextSummarySystemInstruction,
		ModelRef:     modelRef,
	})
	if err != nil {
		return "", errContextSummaryUnavailable
	}
	if events == nil {
		return "", errContextSummaryUnavailable
	}
	var builder strings.Builder
	providerFailed := false
	overflow := false
	for event := range events {
		if event.Error != nil {
			providerFailed = true
			continue
		}
		if event.Type == ProviderEventDelta {
			remaining := contextSummaryMaxBytes - builder.Len()
			if remaining <= 0 || len(event.Delta) > remaining {
				overflow = true
				continue
			}
			builder.WriteString(event.Delta)
		}
	}
	summary := strings.TrimSpace(builder.String())
	if providerFailed || overflow || summary == "" {
		return "", errContextSummaryUnavailable
	}
	return summary, nil
}

func injectConversationContextSummary(
	summary string,
	tail []ProviderMessage,
) []ProviderMessage {
	prepared := make([]ProviderMessage, 0, len(tail)+1)
	prepared = append(prepared, ProviderMessage{
		Role: "assistant",
		Content: "Earlier conversation summary (server-generated from prior turns):\n" +
			strings.TrimSpace(summary),
	})
	prepared = append(prepared, tail...)
	return prepared
}

func appendContextSummaryRuntimeInstruction(systemPrompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if strings.Contains(systemPrompt, contextSummaryRuntimeInstruction) {
		return systemPrompt
	}
	if systemPrompt == "" {
		return contextSummaryRuntimeInstruction
	}
	return systemPrompt + "\n\n" + contextSummaryRuntimeInstruction
}

func stripContextSummaryRuntimeInstruction(systemPrompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == contextSummaryRuntimeInstruction {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(
		systemPrompt,
		"\n\n"+contextSummaryRuntimeInstruction,
	))
}

func withConversationContextMetadata(
	base map[string]any,
	prepared conversationContextPreparation,
) map[string]any {
	metadata := cloneJSONObject(base)
	contextMetadata := map[string]any{
		"mode":                 prepared.Mode,
		"contextWindowTokens":  prepared.ContextWindowTokens,
		"inputBudgetTokens":    prepared.InputBudgetTokens,
		"estimatedInputTokens": prepared.EstimatedInputTokens,
	}
	if prepared.SummaryVersion > 0 {
		contextMetadata["summaryVersion"] = prepared.SummaryVersion
		contextMetadata["summarizedMessageCount"] = prepared.SummarizedMessageCount
	}
	if prepared.DegradationReason != "" {
		contextMetadata["degradationReason"] = prepared.DegradationReason
	}
	metadata["context"] = contextMetadata
	return metadata
}

func estimateProviderInputTokens(systemPrompt string, messages []ProviderMessage) int {
	return estimateProviderTextTokens(systemPrompt) + estimateProviderMessagesTokens(messages)
}

func estimateProviderMessagesTokens(messages []ProviderMessage) int {
	total := 0
	for _, message := range messages {
		total += estimateProviderTextTokens(message.Role) +
			estimateProviderTextTokens(message.Content) + 6
		total += len(message.Attachments) * 1_024
	}
	return total
}

func estimateProviderTextTokens(value string) int {
	if value == "" {
		return 0
	}
	ascii := 0
	nonASCII := 0
	for _, runeValue := range value {
		if runeValue <= 0x7f {
			ascii++
		} else {
			nonASCII++
		}
	}
	return (ascii+3)/4 + nonASCII*2
}
