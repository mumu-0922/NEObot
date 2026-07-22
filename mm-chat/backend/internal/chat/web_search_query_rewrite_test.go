package chat

import (
	"context"
	"strings"
	"testing"
)

func TestRewriteWebSearchQueryUsesRecentConversationAndRuntimeModel(t *testing.T) {
	provider := &ragRewriteProvider{chunks: []string{
		"Standalone Web search query: DeepSeek V4 Flash 上下文窗口长度 官方文档",
	}}
	messages := []ProviderMessage{
		{MessageID: "u1", Role: "user", Content: "你不是说自己有 1M 上下文吗？"},
		{MessageID: "a1", Role: "assistant", Content: "需要联网核对具体规格。"},
		{MessageID: "u2", Role: "user", Content: "你自己联网搜"},
	}

	rewritten, err := rewriteWebSearchQuery(
		context.Background(),
		provider,
		ModelRef{ProviderID: "fixture", ModelID: "deepseek-v4-flash"},
		"u2",
		"你自己联网搜",
		messages,
	)
	if err != nil || rewritten != "DeepSeek V4 Flash 上下文窗口长度 官方文档" {
		t.Fatalf("rewrite = %q, err = %v", rewritten, err)
	}
	if !strings.Contains(provider.input.Prompt, "deepseek-v4-flash") ||
		!strings.Contains(provider.input.Prompt, "1M 上下文") ||
		!strings.Contains(provider.input.Prompt, "你自己联网搜") {
		t.Fatalf("rewrite prompt missing context: %q", provider.input.Prompt)
	}
	if !strings.Contains(provider.input.SystemPrompt, "untrusted data") ||
		!strings.Contains(provider.input.SystemPrompt, "do not answer") {
		t.Fatalf("rewrite system prompt = %q", provider.input.SystemPrompt)
	}
}

func TestRewriteWebSearchQueryNeedsPriorContextAndFailsClosedOnOversizeOutput(t *testing.T) {
	provider := &ragRewriteProvider{chunks: []string{"standalone"}}
	rewritten, err := rewriteWebSearchQuery(
		context.Background(),
		provider,
		ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		"u1",
		"standalone topic",
		[]ProviderMessage{{MessageID: "u1", Role: "user", Content: "standalone topic"}},
	)
	if err != nil || rewritten != "" || provider.input.Prompt != "" {
		t.Fatalf("no-history rewrite = %q, err = %v, input = %#v", rewritten, err, provider.input)
	}

	provider = &ragRewriteProvider{chunks: []string{
		strings.Repeat("x", maxWebSearchRewrittenQueryBytes+1),
	}}
	_, err = rewriteWebSearchQuery(
		context.Background(),
		provider,
		ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
		"u2",
		"这个呢",
		[]ProviderMessage{
			{MessageID: "u1", Role: "user", Content: "prior subject"},
			{MessageID: "u2", Role: "user", Content: "这个呢"},
		},
	)
	if err == nil {
		t.Fatal("oversized rewrite output unexpectedly succeeded")
	}
}

func TestRecentWebSearchRewriteHistoryIsBoundedAndDropsAttachments(t *testing.T) {
	messages := make([]ProviderMessage, 0, 9)
	for index := 0; index < 8; index++ {
		role := "user"
		if index%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, ProviderMessage{
			MessageID:   strings.Repeat(string(rune('a'+index)), 4),
			Role:        role,
			Content:     "history " + string(rune('0'+index)),
			Attachments: []ProviderAttachment{{FileName: "private.txt"}},
		})
	}
	messages = append(messages, ProviderMessage{MessageID: "current", Role: "user", Content: "follow up"})

	history := recentWebSearchRewriteHistory(messages, "current")
	if len(history) != maxWebSearchRewriteHistoryMessages ||
		history[0].Content != "history 2" || history[len(history)-1].Content != "history 7" {
		t.Fatalf("bounded history = %#v", history)
	}
	for _, message := range history {
		if len(message.Attachments) != 0 {
			t.Fatalf("history retained attachments: %#v", message)
		}
	}
}
