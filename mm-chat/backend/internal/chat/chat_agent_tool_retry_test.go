package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/localskills"
)

const retryCanaryUserID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestChatAgentToolRecorderIssuesRetryOnlyForSafeFailedRead(t *testing.T) {
	repository := newFakeRepository()
	service := NewService(repository)
	ctx := auth.WithUser(context.Background(), auth.User{ID: retryCanaryUserID})
	recorder, err := startChatAgentEventRecorder(
		ctx, service, testConversationID, testMessageID,
		"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", testNow(),
	)
	if err != nil {
		t.Fatal(err)
	}
	execution := &ProviderToolExecutionEvent{
		ExecutionID: "local-skill-1-1", CallID: "source-call",
		Name: localFileReadToolName, Status: ProcessStepStatusFailed,
		CallStatus: "failed", Round: 1, Mode: "local_direct",
		Classification: "read", FailureCategory: "file_not_found",
		Presentation: &ProcessStepPresentation{
			Version: 1, Card: "file", Operation: "read", Path: "missing.txt",
		},
	}
	event, err := recorder.recordToolExecution(ctx, execution, []ProcessStep{{
		ID: "tool-1", Kind: ProcessStepKindTool, Status: ProcessStepStatusFailed,
		LabelKey: "process.tool",
		Detail: map[string]any{
			"toolName": localFileReadToolName, "mode": "local_direct", "round": 1,
		},
		Presentation: execution.Presentation,
	}}, testNow())
	if err != nil {
		t.Fatal(err)
	}
	steps := processStepsFromChatAgentEvent(event)
	if len(steps) != 1 || steps[0].Presentation == nil ||
		steps[0].Presentation.Retry == nil ||
		steps[0].Presentation.Retry.EventID != event.EventID ||
		steps[0].Presentation.Retry.RetryOf != "source-call" {
		t.Fatalf("retry presentation=%#v event=%#v", steps, event)
	}

	unsafe := *execution
	unsafe.Name = localFileWriteToolName
	unsafe.Presentation = &ProcessStepPresentation{
		Version: 1, Card: "file", Operation: "write", Path: "missing.txt",
	}
	if chatAgentToolRetryEligible(&unsafe) {
		t.Fatal("write Tool became retryable")
	}
	unknown := *execution
	unknown.Status = ProcessStepStatusOutcomeUnknown
	if chatAgentToolRetryEligible(&unknown) {
		t.Fatal("outcome_unknown Tool became retryable")
	}
}

func TestRetryChatAgentToolCreatesLinkedDurableReadAttempt(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "fixture.txt"), []byte("retry-ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: t.TempDir(), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 64 << 10, MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()

	repository := newFakeRepository()
	repository.conversations = []Conversation{{ID: testConversationID, UserID: retryCanaryUserID}}
	parentMessageID := "ffffffff-ffff-4fff-8fff-ffffffffffff"
	userMessage := fakeMessage(parentMessageID, testConversationID, 0, "user", "read fixture")
	sourceMessage := fakeMessage(testMessageID, testConversationID, 0, "assistant", "")
	sourceMessage.Status = "completed"
	sourceMessage.ParentMessageID = parentMessageID
	sourceMessage.SequenceNo = 1
	sourceMessage.ModelProvider = "fixture"
	sourceMessage.ModelID = "fixture-model"
	repository.messages[testConversationID] = []Message{userMessage, sourceMessage}
	sourceEventID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	payload, err := normalizeChatAgentEventPayload(ChatAgentEventToolResult, chatAgentToolEventPayload(
		&ProviderToolExecutionEvent{
			ExecutionID: "local-skill-1-1", CallID: "source-call",
			Name: localFileReadToolName, Status: ProcessStepStatusFailed,
			CallStatus: "failed", Round: 1, Mode: "local_direct",
			Classification: "read", FailureCategory: "file_not_found",
		},
		[]ProcessStep{{
			ID: "source-tool", Kind: ProcessStepKindTool, Status: ProcessStepStatusFailed,
			LabelKey: "process.tool",
			Detail: map[string]any{
				"toolName": localFileReadToolName, "mode": "local_direct", "round": 1,
			},
			Presentation: &ProcessStepPresentation{
				Version: 1, Card: "file", Operation: "read", Path: "fixture.txt",
				Retry: &ProcessRetryPresentation{EventID: sourceEventID, RetryOf: "source-call"},
			},
		}},
	))
	if err != nil {
		t.Fatal(err)
	}
	repository.agentEvents[testConversationID] = []ChatAgentEvent{{
		EventID: sourceEventID, TurnID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		UserID: retryCanaryUserID, ConversationID: testConversationID,
		MessageID: testMessageID, RunID: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
		Sequence: 3, Type: ChatAgentEventToolResult, StepSequence: 1,
		Payload: payload, OccurredAt: testNow(),
	}}

	handler := NewHandler(NewService(repository),
		WithAgentTimelineCanary(true, []string{retryCanaryUserID}),
		WithLocalSkillRuntime(nil, executor),
	)
	body, _ := json.Marshal(retryChatAgentToolRequest{IdempotencyKey: "retry-fixture-1"})
	nonCanaryRequest := httptest.NewRequest(
		http.MethodPost, agentEventsPathBase+sourceEventID+"/retry", bytes.NewReader(body),
	)
	nonCanaryRequest = nonCanaryRequest.WithContext(auth.WithUser(
		nonCanaryRequest.Context(), auth.User{ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},
	))
	nonCanaryRecorder := httptest.NewRecorder()
	handler.ServeHTTP(nonCanaryRecorder, nonCanaryRequest)
	if nonCanaryRecorder.Code != http.StatusNotFound || len(repository.messages[testConversationID]) != 2 {
		t.Fatalf("non-canary status=%d messages=%d", nonCanaryRecorder.Code,
			len(repository.messages[testConversationID]))
	}
	request := httptest.NewRequest(
		http.MethodPost, agentEventsPathBase+sourceEventID+"/retry", bytes.NewReader(body),
	)
	request = request.WithContext(auth.WithUser(request.Context(), auth.User{ID: retryCanaryUserID}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response ChatMessageDTO
	decodeBody(t, recorder, &response)
	if response.ID == testMessageID || len(response.AgentEvents) < 4 {
		t.Fatalf("retry response=%#v", response)
	}
	var linked bool
	for _, event := range response.AgentEvents {
		execution := projectChatAgentToolExecution(event)
		if execution != nil && execution.RetryOf == "source-call" &&
			execution.CallID != "" && execution.CallID != "source-call" {
			linked = true
		}
	}
	if !linked {
		t.Fatalf("retry events are not linked: %#v", response.AgentEvents)
	}
	steps := projectChatAgentProcessTrace(response.AgentEvents, nil)
	if len(steps) != 1 || steps[0].Status != ProcessStepStatusCompleted ||
		steps[0].Presentation == nil || steps[0].Presentation.Content != "retry-ok" {
		t.Fatalf("retry trace=%#v", steps)
	}
	replayRequest := httptest.NewRequest(
		http.MethodPost, agentEventsPathBase+sourceEventID+"/retry", bytes.NewReader(body),
	)
	replayRequest = replayRequest.WithContext(auth.WithUser(
		replayRequest.Context(), auth.User{ID: retryCanaryUserID},
	))
	replayRecorder := httptest.NewRecorder()
	handler.ServeHTTP(replayRecorder, replayRequest)
	if replayRecorder.Code != http.StatusOK || len(repository.messages[testConversationID]) != 3 {
		t.Fatalf("replay status=%d messages=%d body=%s", replayRecorder.Code,
			len(repository.messages[testConversationID]), replayRecorder.Body.String())
	}
}
