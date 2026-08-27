package chat

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/localskills"
	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/resourceorchestrator"
)

func TestAgentTimelineInterleavesNarrationBeforeToolAndKeepsFinalAnswerSeparate(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "fixture.txt"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"),
		WorkspaceRoot: workspace, ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: time.Second, RunTimeout: time.Second, MaxOutput: 4096,
		MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{
		{
			{Type: ProviderEventDelta, Delta: "先读取文件。"},
			{Type: ProviderEventToolCallCompleted, ToolCall: &ProviderToolCall{
				ID: "read-fixture", Name: localFileReadToolName,
				Arguments: `{"path":"fixture.txt","offset":null,"limit":null}`,
			}},
		},
		{{Type: ProviderEventDelta, Delta: "读取完成。"}},
	}}
	repository := newFakeRepository()
	conversation := fakeConversation(testConversationID, "Interleaved transcript", 1)
	conversation.Metadata = map[string]any{"toolMode": "agent"}
	repository.conversations = append(repository.conversations, conversation)
	repository.messages[testConversationID] = []Message{
		fakeMessage(testMessageID, testConversationID, 0, "user", "读取 fixture.txt"),
	}
	handler := NewHandler(
		NewService(repository),
		WithProvider(provider),
		WithAgentTimelineCanary(true, []string{DevUserID}),
		WithLocalSkillRuntime(nil, executor),
	)
	recorder := performRequest(
		handler,
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		`{"userMessageId":"`+testMessageID+`","modelRef":{"providerId":"mock","modelId":"tool-model"},"config":{"toolMode":"agent"},"idempotencyKey":"interleaved-timeline"}`,
	)
	assertStreamStatus(t, recorder, http.StatusOK)
	body := recorder.Body.String()
	narrationIndex := strings.Index(body, `"chunkType":"narration-delta"`)
	toolIndex := strings.Index(body, `"type":"tool.called"`)
	finalIndex := strings.LastIndex(body, `"content":"读取完成。"`)
	if narrationIndex < 0 || toolIndex <= narrationIndex || finalIndex <= toolIndex {
		t.Fatalf("narration/Tool/final order is invalid; body=%s", body)
	}
	messages := repository.messages[testConversationID]
	if len(messages) != 2 || messages[1].Content != "读取完成。" ||
		strings.Contains(messages[1].Content, "先读取文件") {
		t.Fatalf("persisted final message=%#v", messages)
	}
	if len(provider.inputs) != 2 ||
		!strings.Contains(provider.inputs[0].SystemPrompt, chatAgentNarrationSystemInstruction) {
		t.Fatalf("Agent narration prompt was not applied: %#v", provider.inputs)
	}
	events := repository.agentEvents[testConversationID]
	var narrationDelta, narrationCompleted, toolCalled, toolResult int64
	for _, event := range events {
		switch {
		case event.Type == ChatAgentEventAssistantChunk &&
			chatAgentPayloadString(event.Payload, "chunkType") == "narration-delta":
			narrationDelta = event.Sequence
		case event.Type == ChatAgentEventBlockCompleted &&
			chatAgentPayloadString(event.Payload, "blockType") == "narration":
			narrationCompleted = event.Sequence
		case event.Type == ChatAgentEventToolCalled:
			toolCalled = event.Sequence
		case event.Type == ChatAgentEventToolResult:
			toolResult = event.Sequence
		}
	}
	if narrationDelta == 0 || narrationCompleted <= narrationDelta ||
		toolCalled <= narrationCompleted || toolResult <= toolCalled {
		t.Fatalf("durable narration/Tool order is invalid: %#v", events)
	}

	reloaded := performRequest(
		handler,
		http.MethodGet,
		conversationsPath+"/"+testConversationID+"/messages",
		"",
	)
	assertStatus(t, reloaded, http.StatusOK)
	var page Page[ChatMessageDTO]
	decodeBody(t, reloaded, &page)
	if len(page.Items) != 2 || page.Items[1].Content != "读取完成。" {
		t.Fatalf("reloaded messages=%#v", page.Items)
	}
	reloadedEvents := page.Items[1].AgentEvents
	if len(reloadedEvents) != len(events) {
		t.Fatalf("reloaded Agent events=%#v, want %#v", reloadedEvents, events)
	}
	for index := range events {
		if reloadedEvents[index].EventID != events[index].EventID ||
			reloadedEvents[index].Sequence != events[index].Sequence ||
			reloadedEvents[index].Type != events[index].Type {
			t.Fatalf(
				"reloaded Agent event[%d]=%#v, want %#v",
				index, reloadedEvents[index], events[index],
			)
		}
	}
}

type contextEventFailRepository struct {
	*fakeRepository
}

func (repository *contextEventFailRepository) AppendChatAgentEvent(
	ctx context.Context,
	turnID string,
	input AppendChatAgentEventInput,
) (ChatAgentEvent, error) {
	if input.Type == ChatAgentEventContextInjected {
		return ChatAgentEvent{}, errors.New("fixture context persistence failure")
	}
	return repository.fakeRepository.AppendChatAgentEvent(ctx, turnID, input)
}

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

func TestAgentTimelineRecordsRuntimeResourceContext(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	repository := newFakeRepository()
	conversation := fakeConversation(testConversationID, "Runtime resource context", 0)
	conversation.Metadata = map[string]any{"toolMode": "agent"}
	repository.conversations = append(repository.conversations, conversation)
	repository.messages[testConversationID] = append(
		repository.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "install a skill"),
	)
	provider := &scriptedToolRoundProvider{rounds: [][]ProviderEvent{{{
		Type: ProviderEventDelta, Delta: "Resource answer.",
	}}}}
	mcpRepository := newMCPChatRepository(
		canaryID,
		testConversationID,
		mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: "unused-fixture"},
	)
	mcpRepository.selection.Servers = nil
	mcpConfig := mcpclient.DefaultConfig()
	mcpConfig.Enabled = true
	mcpService, err := mcpclient.NewService(
		mcpConfig,
		mcpRepository,
		nil,
		nil,
		nil,
		mcpclient.Catalog{},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(
		NewService(repository),
		WithProvider(provider),
		WithAgentTimelineCanary(true, []string{canaryID}),
		WithMCPService(mcpService),
		WithResourceOrchestrator(resourceorchestrator.NewService(nil, nil)),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		bytes.NewBufferString(
			`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"tool-capable"},"config":{"toolMode":"agent"},"idempotencyKey":"runtime-resource-context"}`,
		),
	)
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(auth.WithUser(request.Context(), auth.User{ID: canaryID}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status=%d tool rounds=%d chat rounds=%d body=%s",
			recorder.Code, len(provider.inputs), len(provider.chatInputs), recorder.Body.String(),
		)
	}
	body := recorder.Body.String()
	for _, required := range []string{
		`"type":"context.injected"`,
		`"source":"runtime-context"`,
		`"label":"Resource orchestration"`,
		`event: message.completed`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("runtime context stream missing %q; body=%s", required, body)
		}
	}
}

func TestContextEventPersistenceFailureFinalizesAssistantAndTurn(t *testing.T) {
	const canaryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	base := newFakeRepository()
	base.conversations = append(
		base.conversations,
		fakeConversation(testConversationID, "Context persistence failure", 0),
	)
	base.messages[testConversationID] = append(
		base.messages[testConversationID],
		fakeMessage(testMessageID, testConversationID, 0, "user", "hello"),
	)
	repository := &contextEventFailRepository{fakeRepository: base}
	handler := NewHandler(
		NewService(repository),
		WithProvider(NewMockProvider()),
		WithAgentTimelineCanary(true, []string{canaryID}),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		conversationsPath+"/"+testConversationID+"/stream",
		bytes.NewBufferString(
			`{"userMessageId":"22222222-2222-4222-8222-222222222222","modelRef":{"providerId":"mock","modelId":"mock-chat"},"systemInstruction":"persist this context","idempotencyKey":"context-persistence-failure"}`,
		),
	)
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(auth.WithUser(request.Context(), auth.User{ID: canaryID}))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	assertErrorCode(t, recorder, "AGENT_EVENT_PERSISTENCE_FAILED")
	messages := repository.messages[testConversationID]
	if len(messages) != 2 {
		t.Fatalf("persisted messages=%#v", messages)
	}
	assistant := messages[1]
	if assistant.Status != "failed" || assistant.CompletedAt == nil ||
		assistant.Metadata["errorCode"] != "AGENT_EVENT_PERSISTENCE_FAILED" {
		t.Fatalf("assistant terminal state=%#v", assistant)
	}
	if len(repository.agentStatuses) != 1 {
		t.Fatalf("turn statuses=%#v", repository.agentStatuses)
	}
	for turnID, status := range repository.agentStatuses {
		if status != ChatAgentTurnFailed {
			t.Fatalf("turn %s status=%q, want failed", turnID, status)
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
