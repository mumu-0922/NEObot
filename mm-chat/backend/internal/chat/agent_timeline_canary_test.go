package chat

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestAgentTimelineCanaryAdmissionIsExactAndFailClosed(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	handler := NewHandler(NewService(newFakeRepository()),
		WithAgentTimelineCanary(true, []string{canaryID}),
	)
	if !handler.agentTimelineEnabledFor(canaryID) {
		t.Fatal("exact canary was denied")
	}
	if handler.agentTimelineEnabledFor("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb") {
		t.Fatal("non-canary was admitted")
	}
	WithAgentTimelineCanary(false, []string{canaryID})(handler)
	if handler.agentTimelineEnabledFor(canaryID) {
		t.Fatal("disabled global gate admitted canary")
	}
	WithAgentTimelineCanary(true, nil)(handler)
	if handler.agentTimelineEnabledFor(canaryID) {
		t.Fatal("empty canary set admitted user")
	}
}

func TestAgentTimelineCanaryStreamsDurableEventsBeforeTerminalMessage(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	repository := newFakeRepository()
	repository.conversations = append(
		repository.conversations,
		fakeConversation(testConversationID, "Agent timeline", 0),
	)
	repository.messages[testConversationID] = append(
		repository.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	handler := NewHandler(
		NewService(repository),
		WithProvider(NewMockProvider()),
		WithAgentTimelineCanary(true, []string{canaryID}),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		bytes.NewBufferString(
			`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"idempotencyKey":"timeline-live-events"}`,
		),
	)
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(auth.WithUser(request.Context(), auth.User{ID: canaryID}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assertStreamStatus(t, recorder, http.StatusOK)
	body := recorder.Body.String()
	if count := strings.Count(body, "event: agent.event"); count != 5 {
		t.Fatalf("agent.event frame count=%d, want 5; body=%s", count, body)
	}
	if strings.Contains(body, "event: process.step.updated") {
		t.Fatalf("canary stream retained legacy ProcessStep frames; body=%s", body)
	}
	terminalFrame := strings.LastIndex(body, "event: message.completed")
	turnEnded := strings.Index(body, `"type":"turn.ended"`)
	if terminalFrame < 0 || turnEnded < 0 || turnEnded > terminalFrame {
		t.Fatalf("turn.ended must precede terminal frame; body=%s", body)
	}
	if !strings.Contains(body[terminalFrame:], `"agentEvents":[`) {
		t.Fatalf("terminal Message omits authoritative Agent events; body=%s", body)
	}
	if strings.Contains(body[terminalFrame:], `"processTrace":`) {
		t.Fatalf("terminal Message retained legacy ProcessTrace metadata; body=%s", body)
	}
	messages := repository.messages[testConversationID]
	if len(messages) != 2 {
		t.Fatalf("persisted messages=%#v", messages)
	}
	if _, rollbackProjection := messages[1].Metadata[processTraceMetadataKey]; !rollbackProjection {
		t.Fatalf("persisted rollback projection missing: %#v", messages[1].Metadata)
	}
}

func TestAgentTimelineCanaryStreamsDurableContextAndReasoningBlocks(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	repository := newFakeRepository()
	repository.conversations = append(
		repository.conversations,
		fakeConversation(testConversationID, "Transcript v2", 0),
	)
	repository.messages[testConversationID] = append(
		repository.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "solve this"),
	)
	handler := NewHandler(
		NewService(repository),
		WithProvider(reasoningFixtureProvider{}),
		WithAgentTimelineCanary(true, []string{canaryID}),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		bytes.NewBufferString(
			`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"reasoning"},"systemInstruction":"Be precise and keep token=fixture-secret-value private.","config":{"useReasoning":true},"idempotencyKey":"timeline-reasoning-blocks"}`,
		),
	)
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(auth.WithUser(request.Context(), auth.User{ID: canaryID}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	assertStreamStatus(t, recorder, http.StatusOK)
	body := recorder.Body.String()
	for _, required := range []string{
		`"type":"context.injected"`,
		`"source":"system-prompt"`,
		`"type":"assistant.chunk"`,
		`"chunkType":"block-start"`,
		`"chunkType":"reasoning-delta"`,
		`"type":"assistant.block.completed"`,
		`[REDACTED]`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("Transcript stream missing %q; body=%s", required, body)
		}
	}
	if strings.Contains(body, "fixture-secret-value") ||
		strings.Contains(body, "super-secret-value") {
		t.Fatalf("Transcript stream leaked a secret; body=%s", body)
	}
	if strings.Contains(body, "event: reasoning.delta\n") {
		t.Fatalf("typed canary retained duplicate legacy reasoning transport; body=%s", body)
	}
	contextIndex := strings.Index(body, `"type":"context.injected"`)
	blockIndex := strings.Index(body, `"chunkType":"block-start"`)
	deltaIndex := strings.Index(body, `"chunkType":"reasoning-delta"`)
	completedIndex := strings.Index(body, `"type":"assistant.block.completed"`)
	if contextIndex < 0 || blockIndex <= contextIndex || deltaIndex <= blockIndex ||
		completedIndex <= deltaIndex {
		t.Fatalf("Transcript event order is invalid; body=%s", body)
	}

	events := repository.agentEvents[testConversationID]
	types := make([]string, 0, len(events))
	reasoningDeltas := 0
	for _, event := range events {
		types = append(types, event.Type)
		if event.Type == ChatAgentEventAssistantChunk &&
			chatAgentPayloadString(event.Payload, "chunkType") == "reasoning-delta" {
			reasoningDeltas++
		}
	}
	if reasoningDeltas != 1 {
		t.Fatalf("coalesced reasoning delta events=%d, want 1; events=%#v", reasoningDeltas, events)
	}
	for _, required := range []string{
		ChatAgentEventContextInjected,
		ChatAgentEventAssistantChunk,
		ChatAgentEventBlockCompleted,
	} {
		if !containsString(types, required) {
			t.Fatalf("durable Transcript events=%v, missing %q", types, required)
		}
	}
}

func TestAgentTimelineGateKeepsDurableAuthorityAndReturnsLegacyProjection(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	presentation := &ProcessStepPresentation{
		Version: 1, Card: "terminal", Command: "printf ok",
	}
	message := Message{
		ID: testMessageID, ConversationID: testConversationID, Role: "assistant",
		Status: "completed", Metadata: map[string]any{
			"fixture": "kept",
			processTraceMetadataKey: []ProcessStep{{
				ID: testMessageID + ":tool:1", Kind: ProcessStepKindTool,
				Status: ProcessStepStatusCompleted, LabelKey: "process.tool",
				Detail: map[string]any{
					"toolName": localTerminalToolName, "mode": "local_direct",
				},
				Presentation: presentation,
			}},
		},
		AgentEvents: []ChatAgentEvent{{EventID: "event-1"}},
	}
	handler := NewHandler(NewService(newFakeRepository()),
		WithAgentTimelineCanary(true, []string{canaryID}),
	)
	nonCanaryDTO := handler.newMessageDTO(context.Background(), message)
	if len(nonCanaryDTO.AgentEvents) != 0 || nonCanaryDTO.Metadata["fixture"] != "kept" {
		t.Fatalf("non-canary DTO=%#v", nonCanaryDTO)
	}
	steps, ok := nonCanaryDTO.Metadata[processTraceMetadataKey].([]map[string]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("legacy process trace=%#v", nonCanaryDTO.Metadata[processTraceMetadataKey])
	}
	if _, exposed := steps[0]["presentation"]; exposed {
		t.Fatalf("non-canary presentation escaped=%#v", steps[0])
	}
	if message.Metadata[processTraceMetadataKey].([]ProcessStep)[0].Presentation == nil ||
		len(message.AgentEvents) != 1 {
		t.Fatal("display gate mutated durable message authority")
	}

	ctx := auth.WithUser(context.Background(), auth.User{ID: canaryID})
	canaryDTO := handler.newMessageDTO(ctx, message)
	if len(canaryDTO.AgentEvents) != 1 {
		t.Fatalf("canary events=%#v", canaryDTO.AgentEvents)
	}
	if _, legacy := canaryDTO.Metadata[processTraceMetadataKey]; legacy {
		t.Fatalf("canary DTO exposed legacy processTrace=%#v", canaryDTO.Metadata)
	}
	if message.Metadata[processTraceMetadataKey].([]ProcessStep)[0].Presentation == nil {
		t.Fatal("canary DTO projection mutated stored rollback metadata")
	}
}
