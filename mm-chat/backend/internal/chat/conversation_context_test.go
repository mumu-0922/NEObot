package chat

import "testing"

func TestBuildProviderConversationMessagesRestoresLegacyLinearHistory(t *testing.T) {
	messages := []Message{
		{ID: "u1", Role: "user", Content: "first question"},
		{ID: "a1", Role: "assistant", Content: "first answer"},
		{ID: "u2", Role: "user", Content: "follow up"},
	}

	got := buildProviderConversationMessages(messages, "u2", "follow up", nil)
	assertProviderMessages(t, got, []ProviderMessage{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "follow up"},
	})
}

func TestBuildProviderConversationMessagesUsesCurrentSiblingBranch(t *testing.T) {
	messages := []Message{
		{ID: "u1", Role: "user", Content: "question", Metadata: map[string]any{"treeParentMessageId": nil}},
		{ID: "a-old", Role: "assistant", Content: "old answer", ParentMessageID: "u1"},
		{ID: "a-new", Role: "assistant", Content: "new answer", ParentMessageID: "u1"},
		{
			ID: "u2", Role: "user", Content: "continue", ParentMessageID: "a-old",
			Metadata: map[string]any{"treeParentMessageId": "a-new"},
		},
	}

	got := buildProviderConversationMessages(messages, "u2", "continue", nil)
	assertProviderMessages(t, got, []ProviderMessage{
		{Role: "user", Content: "question"},
		{Role: "assistant", Content: "new answer"},
		{Role: "user", Content: "continue"},
	})
}

func TestBuildProviderConversationMessagesHonorsExplicitRoot(t *testing.T) {
	messages := []Message{
		{ID: "u1", Role: "user", Content: "old root"},
		{ID: "a1", Role: "assistant", Content: "old answer"},
		{ID: "u2", Role: "user", Content: "new root", Metadata: map[string]any{"treeParentMessageId": nil}},
	}

	got := buildProviderConversationMessages(messages, "u2", "new root", nil)
	assertProviderMessages(t, got, []ProviderMessage{{Role: "user", Content: "new root"}})
}

func TestBuildProviderConversationMessagesReplacesOnlyCurrentPrompt(t *testing.T) {
	attachments := []ProviderAttachment{{FileID: "image-1", Data: []byte("png")}}
	messages := []Message{
		{ID: "u1", Role: "user", Content: "original history"},
		{ID: "a1", Role: "assistant", Content: "answer"},
		{ID: "u2", Role: "user", Content: "raw current"},
	}

	got := buildProviderConversationMessages(messages, "u2", "grounded current [K1]", attachments)
	assertProviderMessages(t, got, []ProviderMessage{
		{Role: "user", Content: "original history"},
		{Role: "assistant", Content: "answer"},
		{Role: "user", Content: "grounded current [K1]", Attachments: attachments},
	})
}

func TestBuildProviderConversationMessagesDoesNotCarryTurnScopedMarkersForward(t *testing.T) {
	messages := []Message{
		{ID: "u1", Role: "user", Content: "knowledge question"},
		{ID: "a1", Role: "assistant", Content: "grounded answer [K1] and web [W1]"},
		{ID: "u2", Role: "user", Content: "unrelated follow up"},
	}

	got := buildProviderConversationMessages(messages, "u2", "unrelated follow up", nil)
	assertProviderMessages(t, got, []ProviderMessage{
		{Role: "user", Content: "knowledge question"},
		{Role: "assistant", Content: "grounded answer and web"},
		{Role: "user", Content: "unrelated follow up"},
	})
}

func assertProviderMessages(t *testing.T, got []ProviderMessage, want []ProviderMessage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("provider messages len = %d, want %d; got=%#v", len(got), len(want), got)
	}
	for index := range want {
		if got[index].Role != want[index].Role || got[index].Content != want[index].Content {
			t.Fatalf("provider message[%d] = %#v, want %#v", index, got[index], want[index])
		}
		if len(got[index].Attachments) != len(want[index].Attachments) {
			t.Fatalf("provider message[%d] attachments = %#v, want %#v", index, got[index].Attachments, want[index].Attachments)
		}
	}
}
