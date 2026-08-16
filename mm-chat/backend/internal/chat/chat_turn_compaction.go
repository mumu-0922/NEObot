package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	toolResultPruneThresholdBytes = 64 << 10
	toolResultPruneHeadBytes      = 24 << 10
	toolResultPruneTailBytes      = 24 << 10

	turnCompactionThresholdBytes  = 256 << 10
	turnCompactionKeepExchanges   = 4
	turnCompactionCheckpointBytes = 16 << 10

	overflowToolResultThresholdBytes = 16 << 10
	overflowToolResultHeadBytes      = 6 << 10
	overflowToolResultTailBytes      = 6 << 10
	overflowCompactionKeepExchanges  = 2
)

type ProviderContextReplacementEvent struct {
	Reason            string `json:"reason"`
	BeforeBytes       int    `json:"beforeBytes"`
	AfterBytes        int    `json:"afterBytes"`
	ResultsPruned     int    `json:"resultsPruned"`
	ExchangesReplaced int    `json:"exchangesReplaced"`
}

func appendCompactedChatAgentContinuation(
	ctx context.Context,
	events chan<- ProviderEvent,
	exchanges []ProviderToolExchange,
	exchange ProviderToolExchange,
) ([]ProviderToolExchange, bool) {
	exchanges = append(exchanges, exchange)
	compacted, replacement := compactChatAgentContinuation(exchanges, false)
	if replacement == nil {
		return exchanges, true
	}
	if !sendProviderEvent(ctx, events, ProviderEvent{
		Type: ProviderEventContextReplaced, ContextReplacement: replacement,
	}) {
		return nil, false
	}
	return compacted, true
}

func prependProviderEvent(
	ctx context.Context,
	first ProviderEvent,
	rest <-chan ProviderEvent,
) <-chan ProviderEvent {
	events := make(chan ProviderEvent)
	go func() {
		defer close(events)
		if !sendProviderEvent(ctx, events, first) {
			return
		}
		for event := range rest {
			if !sendProviderEvent(ctx, events, event) {
				return
			}
		}
	}()
	return events
}

func compactChatAgentContinuation(
	exchanges []ProviderToolExchange,
	aggressive bool,
) ([]ProviderToolExchange, *ProviderContextReplacementEvent) {
	before := providerContinuationSize(exchanges)
	if before == 0 {
		return exchanges, nil
	}
	threshold := toolResultPruneThresholdBytes
	head := toolResultPruneHeadBytes
	tail := toolResultPruneTailBytes
	keep := turnCompactionKeepExchanges
	reason := "tool_result_pruning"
	if aggressive {
		threshold = overflowToolResultThresholdBytes
		head = overflowToolResultHeadBytes
		tail = overflowToolResultTailBytes
		keep = overflowCompactionKeepExchanges
		reason = "provider_context_overflow"
	}
	compacted, pruned := pruneProviderToolResults(exchanges, threshold, head, tail)
	replaced := 0
	if (aggressive || providerContinuationSize(compacted) > turnCompactionThresholdBytes) &&
		len(compacted) > keep {
		prefixCount := len(compacted) - keep
		checkpoint := buildProviderContinuationCheckpoint(compacted[:prefixCount])
		compacted = append(
			[]ProviderToolExchange{{Checkpoint: checkpoint}},
			compacted[prefixCount:]...,
		)
		replaced = prefixCount
		reason = "turn_summary_compaction"
		if aggressive {
			reason = "provider_context_overflow"
		}
	}
	after := providerContinuationSize(compacted)
	if after >= before || (pruned == 0 && replaced == 0) {
		return exchanges, nil
	}
	return compacted, &ProviderContextReplacementEvent{
		Reason: reason, BeforeBytes: before, AfterBytes: after,
		ResultsPruned: pruned, ExchangesReplaced: replaced,
	}
}

func pruneProviderToolResults(
	exchanges []ProviderToolExchange,
	threshold int,
	headBytes int,
	tailBytes int,
) ([]ProviderToolExchange, int) {
	result := append([]ProviderToolExchange(nil), exchanges...)
	pruned := 0
	for exchangeIndex := range result {
		results := append([]ProviderToolResult(nil), result[exchangeIndex].Results...)
		for resultIndex := range results {
			content := results[resultIndex].Content
			if len(content) <= threshold {
				continue
			}
			head := utf8Prefix(content, headBytes)
			tail := utf8Suffix(content, tailBytes)
			omitted := len(content) - len(head) - len(tail)
			if omitted <= 0 {
				continue
			}
			results[resultIndex].Content = head + fmt.Sprintf(
				"\n...[tool result pruned: %d bytes omitted]...\n", omitted,
			) + tail
			pruned++
		}
		result[exchangeIndex].Results = results
	}
	return result, pruned
}

func buildProviderContinuationCheckpoint(exchanges []ProviderToolExchange) string {
	var builder strings.Builder
	builder.WriteString("<agent_context_checkpoint>\n")
	for _, exchange := range exchanges {
		if checkpoint := strings.TrimSpace(exchange.Checkpoint); checkpoint != "" {
			checkpoint = strings.TrimPrefix(checkpoint, "<agent_context_checkpoint>\n")
			checkpoint = strings.TrimSuffix(checkpoint, "\n</agent_context_checkpoint>")
			appendBoundedCheckpointText(&builder, checkpoint+"\n")
			continue
		}
		if len(exchange.Calls) == 0 {
			if strings.TrimSpace(exchange.FollowupPrompt) != "" {
				appendBoundedCheckpointText(&builder, "- prior Agent continuation completed\n")
			}
			continue
		}
		resultsByID := make(map[string]ProviderToolResult, len(exchange.Results))
		for _, result := range exchange.Results {
			resultsByID[strings.TrimSpace(result.CallID)] = result
		}
		for _, call := range exchange.Calls {
			callID := strings.TrimSpace(call.ID)
			name := normalizedToolName(call.Name)
			result, found := resultsByID[callID]
			status := "missing"
			resultBytes := 0
			if found {
				status = "succeeded"
				if result.IsError {
					status = "failed"
				}
				resultBytes = len(result.Content)
			}
			appendBoundedCheckpointText(&builder, fmt.Sprintf(
				"- tool=%s callId=%s status=%s resultBytes=%d\n",
				truncateChatAgentUTF8(name, 128), truncateChatAgentUTF8(callID, 256),
				status, resultBytes,
			))
		}
	}
	closing := "</agent_context_checkpoint>"
	body := builder.String()
	if len(body)+len(closing) > turnCompactionCheckpointBytes {
		body = utf8Prefix(body, turnCompactionCheckpointBytes-len(closing))
	}
	return body + closing
}

func appendBoundedCheckpointText(builder *strings.Builder, value string) {
	remaining := turnCompactionCheckpointBytes - builder.Len()
	if remaining <= 0 {
		return
	}
	builder.WriteString(utf8Prefix(value, remaining))
}

func providerContinuationSize(exchanges []ProviderToolExchange) int {
	total := 0
	for _, exchange := range exchanges {
		total += len(exchange.AssistantContent) + len(exchange.AssistantReasoning) +
			len(exchange.FollowupPrompt) + len(exchange.Checkpoint)
		for _, call := range exchange.Calls {
			total += len(call.ID) + len(call.Name) + len(call.Arguments) +
				len(call.FailureCategory)
		}
		for _, result := range exchange.Results {
			total += len(result.CallID) + len(result.Name) + len(result.Content) + 1
		}
		if exchange.ProviderState != nil {
			if encoded, err := json.Marshal(exchange.ProviderState); err == nil {
				total += len(encoded)
			}
		}
	}
	return total
}

func utf8Prefix(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func utf8Suffix(value string, maximum int) string {
	if maximum <= 0 {
		return ""
	}
	if len(value) <= maximum {
		return value
	}
	value = value[len(value)-maximum:]
	for !utf8.ValidString(value) {
		value = value[1:]
	}
	return value
}
