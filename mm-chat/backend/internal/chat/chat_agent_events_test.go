package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestChatAgentEventProjectionInterruptsActiveStepsAndOverridesLegacy(t *testing.T) {
	started := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	messageID := "22222222-2222-4222-8222-222222222222"
	events := []ChatAgentEvent{
		{
			EventID:   "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			MessageID: messageID, Sequence: 2, Type: ChatAgentEventStepStarted,
			Payload: chatAgentProcessStepPayload(ProcessStep{
				ID: messageID + ":tool:1", Kind: ProcessStepKindTool,
				Status: ProcessStepStatusRunning, LabelKey: "process.tool",
				StartedAt: formatTime(started), Detail: map[string]any{
					"toolName": "terminal", "mode": "local_direct", "round": 1,
				},
			}),
			OccurredAt: started,
		},
		{
			EventID:   "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			MessageID: messageID, Sequence: 3, Type: ChatAgentEventTurnEnded,
			Payload:    map[string]any{"status": ChatAgentTurnInterrupted},
			OccurredAt: started.Add(2 * time.Second),
		},
	}
	legacy := []ProcessStep{{
		ID: messageID + ":generation:legacy", Kind: ProcessStepKindGeneration,
		Status: ProcessStepStatusCompleted, LabelKey: "process.generation",
	}}

	projected := projectChatAgentProcessTrace(events, legacy)
	if len(projected) != 1 {
		t.Fatalf("projected steps = %#v", projected)
	}
	step := projected[0]
	if step.Status != ProcessStepStatusInterrupted || step.CompletedAt == "" ||
		step.Detail["failureCategory"] != ChatAgentTurnInterrupted {
		t.Fatalf("interrupted step = %#v", step)
	}
}

func TestChatAgentToolEventPayloadDropsCommandsArgumentsResultsAndPrivateServerRef(t *testing.T) {
	payload := chatAgentToolEventPayload(&ProviderToolExecutionEvent{
		ExecutionID: "execution-1", CallID: "call-1", Name: "terminal",
		Server: "private:secret-server", ServerName: "Local Skill",
		Classification: "execute", Status: ProcessStepStatusCompleted,
		CallStatus: "succeeded", Round: 2,
		Arguments: map[string]any{
			"command": "cat /home/private/.env",
			"token":   "sk-fixture-secret-value",
		},
		Query: "private query", Mode: "local_direct", Durability: "process_local",
	}, nil)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{
		"cat /home/private/.env", "sk-fixture-secret-value", "private query",
		"private:secret-server", "arguments", "result",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("durable Tool payload leaked %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"terminal", "local_direct", "succeeded", "process_local"} {
		if !strings.Contains(text, required) {
			t.Fatalf("durable Tool payload missing %q: %s", required, text)
		}
	}
	projected := projectChatAgentToolExecution(ChatAgentEvent{Payload: payload})
	if projected == nil || projected.Durability != "process_local" {
		t.Fatalf("projected durability=%#v", projected)
	}
}

func TestChatAgentTerminalPresentationReplaysFromDurableProcessSteps(t *testing.T) {
	exitCode := 0
	step := ProcessStep{
		ID: "message-1:tool:1", Kind: ProcessStepKindTool,
		Status: ProcessStepStatusCompleted, LabelKey: "process.tool",
		Detail: map[string]any{
			"toolName": localTerminalToolName, "mode": "local_direct", "round": 1,
		},
		Presentation: &ProcessStepPresentation{
			Card: "terminal", Command: "go test ./internal/chat", CWD: "$NEO_CHAT_WORKSPACE",
			ExitCode: &exitCode, Truncated: true,
		},
	}
	payload := chatAgentToolEventPayload(&ProviderToolExecutionEvent{
		ExecutionID: "terminal-1", Name: localTerminalToolName,
		Status: ProcessStepStatusCompleted, Round: 1, Mode: "local_direct",
		Classification: "execute",
	}, []ProcessStep{step})
	payload, err := normalizeChatAgentEventPayload(ChatAgentEventToolResult, payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`"card":"terminal"`, `"command":"go test ./internal/chat"`,
		`"cwd":"$NEO_CHAT_WORKSPACE"`, `"exitCode":0`, `"truncated":true`,
	} {
		if !strings.Contains(string(encoded), required) {
			t.Fatalf("durable payload missing %q: %s", required, encoded)
		}
	}
	for _, forbidden := range []string{"stdout", "stderr"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("durable payload retained %q: %s", forbidden, encoded)
		}
	}
	projected := projectChatAgentProcessTrace([]ChatAgentEvent{{
		EventID: "event-1", MessageID: "message-1", Sequence: 1,
		Type: ChatAgentEventToolResult, Payload: payload,
	}}, nil)
	if len(projected) != 1 || projected[0].Presentation == nil ||
		projected[0].Presentation.ExitCode == nil ||
		*projected[0].Presentation.ExitCode != 0 ||
		projected[0].Presentation.Command != step.Presentation.Command {
		t.Fatalf("durable presentation projection = %#v", projected)
	}
}

func TestNormalizeChatAgentEventPayloadBoundsAssistantMessageOnUTF8Boundary(t *testing.T) {
	content := strings.Repeat("界", maxChatAgentMessageEventBytes)
	normalized, err := normalizeChatAgentEventPayload(
		ChatAgentEventAssistantMessage,
		map[string]any{"status": ChatAgentTurnCompleted, "content": content},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := normalized["content"].(string)
	if len(got) > maxChatAgentMessageEventBytes || !strings.HasPrefix(content, got) {
		t.Fatalf("bounded content bytes = %d", len(got))
	}
}

func TestNormalizeChatAgentContextReplacementPayloadIsContentFreeAndStrict(t *testing.T) {
	payload, err := normalizeChatAgentEventPayload(ChatAgentEventContextReplaced, map[string]any{
		"reason": "provider_context_overflow", "beforeBytes": 100, "afterBytes": 40,
		"resultsPruned": 1, "exchangesReplaced": 2,
	})
	if err != nil || len(payload) != 5 || chatAgentPayloadInt(payload, "beforeBytes") != 100 {
		t.Fatalf("payload=%#v error=%v", payload, err)
	}
	for _, invalid := range []map[string]any{
		{"reason": "private content", "beforeBytes": 100, "afterBytes": 40, "resultsPruned": 1, "exchangesReplaced": 0},
		{"reason": "tool_result_pruning", "beforeBytes": 40, "afterBytes": 40, "resultsPruned": 1, "exchangesReplaced": 0},
		{"reason": "tool_result_pruning", "beforeBytes": 100, "afterBytes": 40, "resultsPruned": 0, "exchangesReplaced": 0},
		{"reason": "tool_result_pruning", "beforeBytes": 100, "afterBytes": 40, "resultsPruned": 1, "exchangesReplaced": 0, "content": "forbidden"},
	} {
		if _, err := normalizeChatAgentEventPayload(ChatAgentEventContextReplaced, invalid); err == nil {
			t.Fatalf("invalid payload accepted: %#v", invalid)
		}
	}
}

func TestNormalizeChatAgentTurnStatusPreservesCommittedTerminalState(t *testing.T) {
	tests := []struct {
		messageStatus string
		want          string
	}{
		{messageStatus: "completed", want: ChatAgentTurnCompleted},
		{messageStatus: "failed", want: ChatAgentTurnFailed},
		{messageStatus: "cancelled", want: ChatAgentTurnCancelled},
		{messageStatus: "streaming", want: ChatAgentTurnInterrupted},
		{messageStatus: "pending", want: ChatAgentTurnInterrupted},
		{messageStatus: "unknown", want: ChatAgentTurnInterrupted},
	}
	for _, test := range tests {
		t.Run(test.messageStatus, func(t *testing.T) {
			if got := normalizeChatAgentTurnStatus(test.messageStatus); got != test.want {
				t.Fatalf("terminal status = %q, want %q", got, test.want)
			}
		})
	}
}
