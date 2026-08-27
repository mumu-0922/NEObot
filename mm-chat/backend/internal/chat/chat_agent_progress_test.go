package chat

import (
	"strings"
	"testing"
)

func TestChatAgentProgressTrackerBlocksRepeatedToolOutcome(t *testing.T) {
	tracker := newChatAgentProgressTracker()
	calls := []ProviderToolCall{{Name: "file_read", Arguments: `{ "path": "a.txt" }`}}
	results := []ProviderToolResult{{Name: "file_read", Content: `{"content":"same"}`}}
	for round := 1; round < maxRepeatedChatAgentToolOutcomes; round++ {
		if reason, blocked := tracker.observe(calls, results); blocked || reason != "" {
			t.Fatalf("round %d blocked=%v reason=%q", round, blocked, reason)
		}
	}
	reason, blocked := tracker.observe(
		[]ProviderToolCall{{Name: "file_read", Arguments: `{"path":"a.txt"}`}},
		results,
	)
	if !blocked || reason != chatAgentBlockRepeatedToolOutcome {
		t.Fatalf("blocked=%v reason=%q", blocked, reason)
	}
}

func TestChatAgentProgressTrackerResetsConsecutiveErrorsAfterProgress(t *testing.T) {
	tracker := newChatAgentProgressTracker()
	for round := 0; round < maxConsecutiveChatAgentErrorRounds-1; round++ {
		call := []ProviderToolCall{{Name: "terminal", Arguments: `{"command":"attempt-` + string(rune('a'+round)) + `"}`}}
		result := []ProviderToolResult{{Name: "terminal", Content: "failed", IsError: true}}
		if _, blocked := tracker.observe(call, result); blocked {
			t.Fatalf("error round %d blocked early", round+1)
		}
	}
	if _, blocked := tracker.observe(
		[]ProviderToolCall{{Name: "terminal", Arguments: `{"command":"recover"}`}},
		[]ProviderToolResult{{Name: "terminal", Content: "ok"}},
	); blocked {
		t.Fatal("successful progress stayed blocked")
	}
	if tracker.consecutiveErrorRound != 0 {
		t.Fatalf("consecutive errors=%d", tracker.consecutiveErrorRound)
	}
}

func TestChatAgentProgressTrackerBlocksConsecutiveDistinctErrors(t *testing.T) {
	tracker := newChatAgentProgressTracker()
	for round := 0; round < maxConsecutiveChatAgentErrorRounds; round++ {
		calls := []ProviderToolCall{{
			Name: "terminal", Arguments: `{"command":"attempt-` + string(rune('a'+round)) + `"}`,
		}}
		results := []ProviderToolResult{{
			Name: "terminal", Content: "failure-" + string(rune('a'+round)), IsError: true,
		}}
		reason, blocked := tracker.observe(calls, results)
		if round < maxConsecutiveChatAgentErrorRounds-1 && blocked {
			t.Fatalf("round %d blocked early with %q", round+1, reason)
		}
		if round == maxConsecutiveChatAgentErrorRounds-1 &&
			(!blocked || reason != chatAgentBlockConsecutiveToolErrors) {
			t.Fatalf("final blocked=%v reason=%q", blocked, reason)
		}
	}
}

func TestChatAgentBlockedInstructionForbidsFalseSuccess(t *testing.T) {
	request := withChatAgentBlockedInstruction(
		ProviderRequest{SystemPrompt: "base"},
		chatAgentBlockConsecutiveToolErrors,
	)
	if !strings.Contains(request.SystemPrompt, "not completed successfully") ||
		!strings.Contains(request.SystemPrompt, chatAgentBlockConsecutiveToolErrors) ||
		!strings.Contains(request.SystemPrompt, "Do not claim success") {
		t.Fatalf("blocked instruction=%q", request.SystemPrompt)
	}
}

func TestAppendChatAgentNarrationSystemInstructionIsScopedAndIdempotent(t *testing.T) {
	base := "Base system instruction."
	withNarration := appendChatAgentNarrationSystemInstruction(base)
	if !strings.HasPrefix(withNarration, base+"\n\n") ||
		!strings.Contains(withNarration, "user-visible progress updates") ||
		!strings.Contains(withNarration, "never expose chain-of-thought") {
		t.Fatalf("narration instruction=%q", withNarration)
	}
	idempotent := appendChatAgentNarrationSystemInstruction(withNarration)
	if idempotent != withNarration ||
		strings.Count(idempotent, chatAgentNarrationSystemInstruction) != 1 {
		t.Fatalf("narration instruction duplicated=%q", idempotent)
	}
}
