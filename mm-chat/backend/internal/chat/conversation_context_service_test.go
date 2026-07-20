package chat

import (
	"context"
	"strings"
	"testing"
)

func TestServiceValidatesConversationContextSummary(t *testing.T) {
	service := NewService(newFakeRepository())
	valid := UpsertConversationContextSummaryInput{
		SourceFirstMessageID:   contextBudgetTestUUID(1),
		SourceLastMessageID:    contextBudgetTestUUID(2),
		SourceMessageCount:     2,
		SourceDigest:           strings.Repeat("a", 64),
		Summary:                "bounded summary",
		EstimatedSourceTokens:  10,
		EstimatedSummaryTokens: 3,
	}
	tests := []struct {
		name   string
		mutate func(*UpsertConversationContextSummaryInput)
	}{
		{name: "boundary", mutate: func(input *UpsertConversationContextSummaryInput) {
			input.SourceLastMessageID = "not-a-uuid"
		}},
		{name: "count", mutate: func(input *UpsertConversationContextSummaryInput) {
			input.SourceMessageCount = 0
		}},
		{name: "digest", mutate: func(input *UpsertConversationContextSummaryInput) {
			input.SourceDigest = "not-a-digest"
		}},
		{name: "summary", mutate: func(input *UpsertConversationContextSummaryInput) {
			input.Summary = " "
		}},
		{name: "tokens", mutate: func(input *UpsertConversationContextSummaryInput) {
			input.EstimatedSourceTokens = -1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			if _, err := service.UpsertConversationContextSummary(
				context.Background(), testConversationID, input,
			); err == nil {
				t.Fatal("invalid context summary was accepted")
			}
		})
	}
}
