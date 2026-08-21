package chat

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const interruptedAssistantID = "77777777-7777-4777-8777-777777777777"
const interruptedAssistantPrefix = "Partial [W9] answer: "

func TestPrepareAnswerContinuationRequiresExactRecoverableSource(t *testing.T) {
	user := fakeMessage(testMessageID, testConversationID, 0, "user", "inspect")
	base := fakeMessage(interruptedAssistantID, testConversationID, 1, "assistant", interruptedAssistantPrefix)
	base.ParentMessageID = user.ID
	base.Status = "failed"
	base.Metadata = map[string]any{"errorCode": providerStreamInterruptedCode}

	tests := []struct {
		name   string
		mutate func(*Message, *Message)
		code   string
	}{
		{name: "wrong role", mutate: func(source *Message, _ *Message) { source.Role = "user" }, code: "ANSWER_CONTINUATION_NOT_ALLOWED"},
		{name: "completed", mutate: func(source *Message, _ *Message) { source.Status = "completed" }, code: "ANSWER_CONTINUATION_NOT_ALLOWED"},
		{name: "wrong error", mutate: func(source *Message, _ *Message) { source.Metadata["errorCode"] = "PROVIDER_ERROR" }, code: "ANSWER_CONTINUATION_NOT_ALLOWED"},
		{name: "empty", mutate: func(source *Message, _ *Message) { source.Content = "  " }, code: "ANSWER_CONTINUATION_EMPTY"},
		{name: "wrong parent", mutate: func(source *Message, _ *Message) { source.ParentMessageID = "88888888-8888-4888-8888-888888888888" }, code: "ANSWER_CONTINUATION_PARENT_MISMATCH"},
		{name: "wrong user role", mutate: func(_ *Message, user *Message) { user.Role = "assistant" }, code: "ANSWER_CONTINUATION_PARENT_MISMATCH"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := base
			source.Metadata = cloneJSONObject(base.Metadata)
			candidateUser := user
			test.mutate(&source, &candidateUser)
			_, err := prepareAnswerContinuation(source, candidateUser)
			var validation ValidationError
			if !errors.As(err, &validation) || validation.Code != test.code {
				t.Fatalf("error = %#v, want %s", err, test.code)
			}
		})
	}
}

func TestPrepareAnswerContinuationAcceptsOnlyTerminalToolStates(t *testing.T) {
	user := fakeMessage(testMessageID, testConversationID, 0, "user", "inspect")
	source := fakeMessage(interruptedAssistantID, testConversationID, 1, "assistant", interruptedAssistantPrefix)
	source.ParentMessageID = user.ID
	source.Status = "failed"
	source.Metadata = map[string]any{"errorCode": providerStreamInterruptedCode}
	source.AgentEvents = []ChatAgentEvent{
		continuationToolEvent(1, ChatAgentEventToolCalled, ProcessStepStatusRunning),
		continuationToolEvent(2, ChatAgentEventToolResult, ProcessStepStatusCompleted),
	}

	prepared, err := prepareAnswerContinuation(source, user)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.prefix != source.Content || !strings.Contains(prepared.evidence, `"tool":"terminal"`) ||
		!strings.Contains(prepared.evidence, `"status":"completed"`) {
		t.Fatalf("prepared continuation = %#v", prepared)
	}

	for _, status := range []string{
		ProcessStepStatusPending,
		ProcessStepStatusRunning,
		ProcessStepStatusAwaitingApproval,
		ProcessStepStatusInterrupted,
		ProcessStepStatusOutcomeUnknown,
	} {
		t.Run(status, func(t *testing.T) {
			unsafe := source
			unsafe.AgentEvents = []ChatAgentEvent{
				continuationToolEvent(1, ChatAgentEventToolCalled, status),
			}
			_, err := prepareAnswerContinuation(unsafe, user)
			var validation ValidationError
			if !errors.As(err, &validation) || validation.Code != "ANSWER_CONTINUATION_UNSAFE_TOOL_STATE" {
				t.Fatalf("status %q error = %#v", status, err)
			}
		})
	}

	agentWithoutEvents := source
	agentWithoutEvents.AgentEvents = nil
	agentWithoutEvents.Metadata = cloneJSONObject(source.Metadata)
	agentWithoutEvents.Metadata["toolMode"] = string(chatToolModeAgent)
	_, err = prepareAnswerContinuation(agentWithoutEvents, user)
	var validation ValidationError
	if !errors.As(err, &validation) || validation.Code != "ANSWER_CONTINUATION_UNSAFE_TOOL_STATE" {
		t.Fatalf("Agent without events error = %#v", err)
	}
}

func TestHandlerContinuesInterruptedAnswerWithoutToolRound(t *testing.T) {
	repo := continuationFixtureRepository()
	provider := &continuationToolProvider{events: []ProviderEvent{
		{Type: ProviderEventDelta, Delta: "completed safely."},
	}}
	handler := NewHandler(NewService(repo), WithProvider(provider))

	recorder := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"`+testMessageID+`","continuationOfMessageId":"`+interruptedAssistantID+`","modelRef":{"providerId":"fixture","modelId":"fixture-agent"},"config":{"searchMode":"external"},"idempotencyKey":"continue-1"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	if provider.toolRoundCalls != 0 || provider.streamCalls != 1 {
		t.Fatalf("provider calls stream=%d toolRound=%d", provider.streamCalls, provider.toolRoundCalls)
	}
	if !strings.Contains(provider.input.SystemPrompt, "No tools are available") ||
		len(provider.input.Messages) < 2 ||
		provider.input.Messages[len(provider.input.Messages)-2].Role != "assistant" ||
		provider.input.Messages[len(provider.input.Messages)-2].Content != interruptedAssistantPrefix ||
		provider.input.Messages[len(provider.input.Messages)-1].Role != "user" ||
		!strings.Contains(provider.input.Messages[len(provider.input.Messages)-1].Content, "missing suffix") {
		t.Fatalf("continuation provider input = %#v", provider.input)
	}

	messages := repo.messages[testConversationID]
	if len(messages) != 3 {
		t.Fatalf("messages = %#v", messages)
	}
	source, continued := messages[1], messages[2]
	if source.Content != interruptedAssistantPrefix || source.Status != "failed" {
		t.Fatalf("source mutated = %#v", source)
	}
	if continued.Status != "completed" || continued.Content != interruptedAssistantPrefix+"completed safely." ||
		continued.ParentMessageID != testMessageID ||
		continued.Metadata[continuationOfMessageIDMetadataKey] != interruptedAssistantID ||
		continued.Metadata[continuationModeMetadataKey] != answerOnlyContinuationMode {
		t.Fatalf("continued message = %#v", continued)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"delta":"Partial [W9] answer: "`) ||
		!strings.Contains(body, `"delta":"completed safely."`) ||
		!strings.Contains(body, `"content":"Partial [W9] answer: completed safely."`) {
		t.Fatalf("continuation SSE = %s", body)
	}
	for _, event := range continued.AgentEvents {
		if event.Type == ChatAgentEventToolCalled || event.Type == ChatAgentEventToolResult {
			t.Fatalf("continuation copied Tool event: %#v", event)
		}
	}
}

func TestHandlerPreservesLongerPartialAfterContinuationInterruptsAgain(t *testing.T) {
	repo := continuationFixtureRepository()
	provider := &continuationToolProvider{events: []ProviderEvent{
		{Type: ProviderEventDelta, Delta: "more text"},
		{Error: newProviderFailure(ProviderFailureStreamIncomplete, "private")},
	}}
	handler := NewHandler(NewService(repo), WithProvider(provider))

	recorder := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"`+testMessageID+`","continuationOfMessageId":"`+interruptedAssistantID+`","modelRef":{"providerId":"fixture","modelId":"fixture-agent"},"idempotencyKey":"continue-2"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	messages := repo.messages[testConversationID]
	continued := messages[len(messages)-1]
	if continued.Status != "failed" || continued.Content != interruptedAssistantPrefix+"more text" ||
		chatAgentErrorCode(continued.Metadata) != providerStreamInterruptedCode {
		t.Fatalf("continued interruption = %#v", continued)
	}
	if _, err := prepareAnswerContinuation(continued, messages[0]); err != nil {
		t.Fatalf("second continuation was not eligible: %v", err)
	}
}

func continuationFixtureRepository() *fakeRepository {
	repo := newFakeRepository()
	conversation := fakeConversation(testConversationID, "Continuation", 2)
	conversation.Metadata = map[string]any{"toolMode": "agent"}
	repo.conversations = append(repo.conversations, conversation)
	user := fakeMessage(testMessageID, testConversationID, 0, "user", "inspect")
	source := fakeMessage(interruptedAssistantID, testConversationID, 1, "assistant", interruptedAssistantPrefix)
	source.ParentMessageID = user.ID
	source.Status = "failed"
	source.Metadata = map[string]any{
		"runId":     "66666666-6666-4666-8666-666666666666",
		"errorCode": providerStreamInterruptedCode,
		"toolMode":  string(chatToolModeAgent),
	}
	repo.messages[testConversationID] = []Message{user, source}
	repo.agentEvents[testConversationID] = []ChatAgentEvent{
		continuationToolEvent(1, ChatAgentEventToolCalled, ProcessStepStatusRunning),
		continuationToolEvent(2, ChatAgentEventToolResult, ProcessStepStatusCompleted),
	}
	for index := range repo.agentEvents[testConversationID] {
		repo.agentEvents[testConversationID][index].MessageID = source.ID
	}
	return repo
}

func continuationToolEvent(sequence int64, eventType string, status string) ChatAgentEvent {
	presentation := &ProcessStepPresentation{
		Version: 1, Card: "terminal", Title: "Terminal", Summary: "bounded result",
	}
	return ChatAgentEvent{
		EventID:  "99999999-9999-4999-8999-999999999999",
		Sequence: sequence,
		Type:     eventType,
		Payload: map[string]any{
			"toolCall": map[string]any{
				"executionId":   "terminal-1",
				"toolName":      "terminal",
				"processStatus": status,
				"mode":          "local_direct",
			},
			"processSteps": []any{ProcessStep{
				ID: "terminal-1", Kind: ProcessStepKindTool, Status: status,
				LabelKey: "process.tool", Presentation: presentation,
			}},
		},
	}
}

type continuationToolProvider struct {
	input          ProviderRequest
	events         []ProviderEvent
	streamCalls    int
	toolRoundCalls int
}

func (provider *continuationToolProvider) StreamChat(
	_ context.Context,
	input ProviderRequest,
) (<-chan ProviderEvent, error) {
	provider.streamCalls++
	provider.input = input
	events := make(chan ProviderEvent, len(provider.events))
	for _, event := range provider.events {
		events <- event
	}
	close(events)
	return events, nil
}

func (provider *continuationToolProvider) StreamToolRound(
	context.Context,
	ProviderRoundRequest,
) (<-chan ProviderEvent, error) {
	provider.toolRoundCalls++
	return nil, errors.New("Tool round must not run during answer continuation")
}
