package chat

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

func TestToolResultPruningPreservesPairIdentityEndsAndUTF8(t *testing.T) {
	content := "BEGIN-" + strings.Repeat("界", 30_000) + "-END"
	exchanges := []ProviderToolExchange{{
		Calls: []ProviderToolCall{{ID: "call-1", Name: "terminal", Arguments: `{}`}},
		Results: []ProviderToolResult{{
			CallID: "call-1", Name: "terminal", Content: content, IsError: true,
		}},
	}}
	compacted, replacement := compactChatAgentContinuation(exchanges, false)
	if replacement == nil || replacement.ResultsPruned != 1 ||
		replacement.ExchangesReplaced != 0 || replacement.AfterBytes >= replacement.BeforeBytes {
		t.Fatalf("replacement=%#v", replacement)
	}
	result := compacted[0].Results[0]
	if result.CallID != "call-1" || result.Name != "terminal" || !result.IsError ||
		!strings.HasPrefix(result.Content, "BEGIN-") || !strings.HasSuffix(result.Content, "-END") ||
		!strings.Contains(result.Content, "tool result pruned") ||
		!strings.Contains(result.Content, "bytes omitted") {
		t.Fatalf("pruned result=%#v", result)
	}
	if !strings.HasPrefix(exchanges[0].Results[0].Content, "BEGIN-") ||
		len(exchanges[0].Results[0].Content) != len(content) {
		t.Fatal("input continuation was mutated")
	}
}

func TestTurnCompactionReplacesOnlyCompleteOldExchangesAndKeepsRecentProviderState(t *testing.T) {
	exchanges := make([]ProviderToolExchange, 0, 7)
	for index := 0; index < 7; index++ {
		callID := "call-" + string(rune('a'+index))
		exchanges = append(exchanges, ProviderToolExchange{
			AssistantContent: strings.Repeat(string(rune('A'+index)), 48<<10),
			Calls:            []ProviderToolCall{{ID: callID, Name: "file_read", Arguments: `{}`}},
			Results: []ProviderToolResult{{
				CallID: callID, Name: "file_read", Content: `{"ok":true}`,
			}},
			ProviderState: map[string]any{"signature": callID},
		})
	}
	recent := append([]ProviderToolExchange(nil), exchanges[len(exchanges)-turnCompactionKeepExchanges:]...)
	compacted, replacement := compactChatAgentContinuation(exchanges, false)
	if replacement == nil || replacement.ExchangesReplaced != 3 || len(compacted) != 5 ||
		compacted[0].Checkpoint == "" || strings.Contains(compacted[0].Checkpoint, strings.Repeat("A", 100)) {
		t.Fatalf("compacted=%#v replacement=%#v", compacted, replacement)
	}
	if !strings.Contains(compacted[0].Checkpoint, "callId=call-a") ||
		!strings.Contains(compacted[0].Checkpoint, "callId=call-c") {
		t.Fatalf("checkpoint=%q", compacted[0].Checkpoint)
	}
	if !reflect.DeepEqual(compacted[1:], recent) {
		t.Fatal("recent exchanges or Provider state changed")
	}
	mergedInput := append([]ProviderToolExchange{{Checkpoint: compacted[0].Checkpoint}}, exchanges...)
	merged, _ := compactChatAgentContinuation(mergedInput, true)
	if len(merged) > 0 && strings.Count(merged[0].Checkpoint, "<agent_context_checkpoint>") > 1 {
		t.Fatalf("checkpoint nested instead of merged: %q", merged[0].Checkpoint)
	}
}

func TestProviderContinuationSerializersRenderSyntheticCheckpoint(t *testing.T) {
	exchange := ProviderToolExchange{Checkpoint: "<agent_context_checkpoint>safe</agent_context_checkpoint>"}
	openAI := appendOpenAICompatibleContinuation(nil, []ProviderToolExchange{exchange})
	if len(openAI) != 1 || openAI[0].Role != "user" || openAI[0].Content != exchange.Checkpoint {
		t.Fatalf("OpenAI checkpoint=%#v", openAI)
	}
	anthropic, err := appendAnthropicContinuation(nil, []ProviderToolExchange{exchange})
	if err != nil || len(anthropic) != 1 || anthropic[0].Role != "user" ||
		anthropic[0].Content != exchange.Checkpoint {
		t.Fatalf("Anthropic checkpoint=%#v error=%v", anthropic, err)
	}
}

func TestToolLoopRetriesContextOverflowOnceOnlyAfterContinuationShrinks(t *testing.T) {
	runtime := newCompactionTestRuntime(t)
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{
			{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "large-output", Name: localTerminalToolName,
				Arguments: `{"command":"head -c 120000 /dev/zero | tr '\\0' x","skill":null,"workingDir":null,"timeoutSeconds":1,"runInBackground":false}`,
			}}},
			nil,
			{{Type: ProviderEventDelta, Delta: "recovered"}},
		},
		syncErrors: map[int]error{
			1: newProviderFailure(ProviderFailureContextOverflow, "bounded"),
		},
	}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "run fixture", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		LocalSkills: runtime,
	})
	var answer strings.Builder
	replacements := make([]ProviderContextReplacementEvent, 0, 2)
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			answer.WriteString(event.Delta)
		}
		if event.ContextReplacement != nil {
			replacements = append(replacements, *event.ContextReplacement)
		}
	}
	if answer.String() != "recovered" || len(provider.inputs) != 3 || len(replacements) != 2 {
		t.Fatalf("answer=%q inputs=%d replacements=%#v", answer.String(), len(provider.inputs), replacements)
	}
	secondSize := providerContinuationSize(provider.inputs[1].Continuation)
	thirdSize := providerContinuationSize(provider.inputs[2].Continuation)
	if thirdSize >= secondSize || replacements[1].Reason != "provider_context_overflow" {
		t.Fatalf("continuation sizes=%d/%d replacements=%#v", secondSize, thirdSize, replacements)
	}
}

func TestToolLoopDoesNotRetryContextOverflowTwice(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{
			{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "large-output", Name: localTerminalToolName,
				Arguments: `{"command":"head -c 120000 /dev/zero | tr '\\0' x","skill":null,"workingDir":null,"timeoutSeconds":1,"runInBackground":false}`,
			}}},
		},
		syncErrors: map[int]error{
			1: newProviderFailure(ProviderFailureContextOverflow, "first overflow"),
			2: newProviderFailure(ProviderFailureContextOverflow, "second overflow"),
		},
	}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "run fixture", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		LocalSkills: newCompactionTestRuntime(t),
	})
	var terminalError error
	for event := range events {
		if event.Error != nil {
			terminalError = event.Error
		}
	}
	category, ok := ProviderFailureCategoryOf(terminalError)
	if !ok || category != ProviderFailureContextOverflow || len(provider.inputs) != 3 {
		t.Fatalf("category=%q/%t inputs=%d error=%v", category, ok, len(provider.inputs), terminalError)
	}
}

func TestToolLoopRetriesFirstStreamContextOverflowAfterContinuationShrinks(t *testing.T) {
	provider := &scriptedToolRoundProvider{
		rounds: [][]ProviderEvent{
			{{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "large-output", Name: localTerminalToolName,
				Arguments: `{"command":"head -c 120000 /dev/zero | tr '\\0' x","skill":null,"workingDir":null,"timeoutSeconds":1,"runInBackground":false}`,
			}}},
			{{Error: newProviderFailure(ProviderFailureContextOverflow, "stream overflow")}},
			{{Type: ProviderEventDelta, Delta: "stream-recovered"}},
		},
	}
	events := startRetrievalToolLoop(context.Background(), externalWebToolLoopInput{
		Provider: provider,
		Request: ProviderRequest{
			Prompt: "run fixture", ModelRef: ModelRef{ProviderID: "fixture", ModelID: "model"},
		},
		LocalSkills: newCompactionTestRuntime(t),
	})
	var answer strings.Builder
	replacements := make([]ProviderContextReplacementEvent, 0, 2)
	for event := range events {
		if event.Error != nil {
			t.Fatal(event.Error)
		}
		if event.Type == ProviderEventDelta {
			answer.WriteString(event.Delta)
		}
		if event.ContextReplacement != nil {
			replacements = append(replacements, *event.ContextReplacement)
		}
	}
	if answer.String() != "stream-recovered" || len(provider.inputs) != 3 || len(replacements) != 2 {
		t.Fatalf("answer=%q inputs=%d replacements=%#v", answer.String(), len(provider.inputs), replacements)
	}
	beforeRetry := providerContinuationSize(provider.inputs[1].Continuation)
	afterRetry := providerContinuationSize(provider.inputs[2].Continuation)
	if afterRetry >= beforeRetry || replacements[1].Reason != "provider_context_overflow" {
		t.Fatalf("continuation sizes=%d/%d replacements=%#v", beforeRetry, afterRetry, replacements)
	}
}

func newCompactionTestRuntime(t *testing.T) *localSkillToolRuntime {
	t.Helper()
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: workspace + "/skills", WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: 5 * time.Second, MaxOutput: 256 << 10,
		MaxCalls: 8, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := newLocalSkillToolRuntime(executor, nil)
	runtime.bindJobScope("user-1", "conversation-1")
	return runtime
}
