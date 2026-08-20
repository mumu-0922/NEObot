package chat

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/localskills"
)

const (
	testApprovalID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testTurnID     = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

type approvalTestRepository struct {
	*fakeRepository
	mu        sync.Mutex
	approvals map[string]ChatAgentApproval
	created   chan ChatAgentApproval
}

func newApprovalTestRepository() *approvalTestRepository {
	return &approvalTestRepository{
		fakeRepository: newFakeRepository(),
		approvals:      make(map[string]ChatAgentApproval),
		created:        make(chan ChatAgentApproval, 8),
	}
}

func (repository *approvalTestRepository) CreateChatAgentApproval(
	_ context.Context,
	input CreateChatAgentApprovalInput,
) (ChatAgentApproval, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, current := range repository.approvals {
		if current.TurnID == input.TurnID && current.ExecutionID == input.ExecutionID {
			return current, nil
		}
	}
	approval := ChatAgentApproval{
		ID: input.ID, TurnID: input.TurnID, UserID: DevUserID,
		ConversationID: testConversationID, MessageID: testMessageID, RunID: testRunID,
		ExecutionID: input.ExecutionID, ToolName: input.ToolName, RiskClass: input.RiskClass,
		Status: ChatAgentApprovalPending, Revision: 1,
		AllowConversation: input.AllowConversation,
		ExpiresAt:         input.ExpiresAt, CreatedAt: input.OccurredAt,
	}
	repository.approvals[approval.ID] = approval
	repository.created <- approval
	return approval, nil
}

func (repository *approvalTestRepository) DecideChatAgentApproval(
	_ context.Context,
	input DecideChatAgentApprovalInput,
) (ChatAgentApproval, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	approval, ok := repository.approvals[input.ApprovalID]
	if !ok {
		return ChatAgentApproval{}, ChatAgentApprovalError{Code: "CHAT_AGENT_APPROVAL_NOT_FOUND"}
	}
	if approval.Status != ChatAgentApprovalPending {
		return approval, nil
	}
	if approval.Revision != input.ExpectedRevision {
		return ChatAgentApproval{}, ChatAgentApprovalError{Code: "CHAT_AGENT_APPROVAL_STALE_REVISION"}
	}
	decision := input.Decision
	status := ChatAgentApprovalAllowed
	if !input.OccurredAt.Before(approval.ExpiresAt) || decision == chatAgentApprovalExpire {
		status, decision = ChatAgentApprovalExpired, chatAgentApprovalExpire
	} else if decision == ChatAgentApprovalDeny || decision == chatAgentApprovalRestartDeny {
		status = ChatAgentApprovalDenied
	}
	approval.Status = status
	approval.Decision = decision
	approval.Revision++
	decidedAt := input.OccurredAt
	approval.DecidedAt = &decidedAt
	repository.approvals[approval.ID] = approval
	return approval, nil
}

func (repository *approvalTestRepository) RecoverPendingChatAgentApprovals(
	_ context.Context,
	at time.Time,
) (int, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	count := 0
	for id, approval := range repository.approvals {
		if approval.Status != ChatAgentApprovalPending {
			continue
		}
		approval.Status = ChatAgentApprovalDenied
		approval.Decision = chatAgentApprovalRestartDeny
		approval.Revision++
		approval.DecidedAt = &at
		repository.approvals[id] = approval
		count++
	}
	return count, nil
}

func TestChatAgentApprovalEndpointFirstDecisionWins(t *testing.T) {
	repository := newApprovalTestRepository()
	now := time.Now().UTC()
	repository.approvals[testApprovalID] = ChatAgentApproval{
		ID: testApprovalID, TurnID: testTurnID, UserID: DevUserID,
		ConversationID: testConversationID, MessageID: testMessageID, RunID: testRunID,
		ExecutionID: "local-skill-1-1", ToolName: localTerminalToolName,
		RiskClass: string(chatToolRiskExecute), Status: ChatAgentApprovalPending,
		Revision: 1, AllowConversation: true, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	handler := NewHandler(NewService(repository))
	path := approvalsPathBase + testApprovalID + "/decision"

	first := performRequest(
		handler, http.MethodPost, path,
		`{"expectedRevision":1,"decision":"allow_once"}`,
	)
	assertStatus(t, first, http.StatusOK)
	var allowed ChatAgentApproval
	decodeBody(t, first, &allowed)
	if allowed.Status != ChatAgentApprovalAllowed ||
		allowed.Decision != ChatAgentApprovalAllowOnce || allowed.Revision != 2 {
		t.Fatalf("allowed=%#v", allowed)
	}

	duplicate := performRequest(
		handler, http.MethodPost, path,
		`{"expectedRevision":1,"decision":"deny"}`,
	)
	assertStatus(t, duplicate, http.StatusOK)
	var current ChatAgentApproval
	decodeBody(t, duplicate, &current)
	if current.Status != ChatAgentApprovalAllowed ||
		current.Decision != ChatAgentApprovalAllowOnce || current.Revision != 2 {
		t.Fatalf("duplicate changed first decision: %#v", current)
	}
}

func TestLocalTerminalWaitsForDurableApprovalAndResumes(t *testing.T) {
	workspace := t.TempDir()
	executor, err := localskills.NewExecutor(localskills.Config{
		Enabled: true, RuntimeRoot: filepath.Join(workspace, ".skills"), WorkspaceRoot: workspace,
		ShellPath: "/bin/sh", ApprovalMode: localskills.ApprovalSmart,
		CallTimeout: 2 * time.Second, RunTimeout: 5 * time.Second,
		MaxOutput: 4096, MaxCalls: 4, MaxRounds: 4, MaxConcurrent: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := newApprovalTestRepository()
	service := NewService(repository)
	waiters := newChatAgentApprovalWaiters()
	runtime := newLocalSkillToolRuntime(executor, nil)
	runtime.bindApprovalRuntime(newChatToolApprovalRuntime(service, waiters, testTurnID))
	events := make(chan ProviderEvent, 16)
	resultChannel := make(chan ProviderToolResult, 1)
	errorChannel := make(chan error, 1)
	go func() {
		result, executeErr := runtime.execute(context.Background(), events, ProviderToolCall{
			ID: "terminal-approved", Name: localTerminalToolName,
			Arguments: `{"command":"rm -rf marker; printf approved > marker","skill":"","workingDir":"","timeoutSeconds":2,"runInBackground":false}`,
		}, 1, 1)
		resultChannel <- result
		errorChannel <- executeErr
	}()

	var approval ChatAgentApproval
	select {
	case approval = <-repository.created:
	case <-time.After(time.Second):
		t.Fatal("approval request was not persisted")
	}
	decision, err := service.DecideChatAgentApproval(context.Background(), DecideChatAgentApprovalInput{
		ApprovalID: approval.ID, ExpectedRevision: approval.Revision,
		Decision: ChatAgentApprovalAllowOnce, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	waiters.resolve(decision)

	result := <-resultChannel
	if executeErr := <-errorChannel; executeErr != nil || result.IsError {
		t.Fatalf("result=%#v error=%v", result, executeErr)
	}
	body, err := os.ReadFile(filepath.Join(workspace, "marker"))
	if err != nil || string(body) != "approved" {
		t.Fatalf("marker=%q error=%v", body, err)
	}
	close(events)
	statuses := []string{}
	for event := range events {
		if event.ToolExecution == nil || event.ToolExecution.Transient {
			continue
		}
		statuses = append(statuses, event.ToolExecution.Status)
		if event.ToolExecution.Status == ProcessStepStatusAwaitingApproval {
			presentation := event.ToolExecution.Presentation
			if presentation == nil || presentation.Approval == nil ||
				presentation.Approval.ID != approval.ID ||
				strings.Contains(presentation.Command, "approved-secret") {
				t.Fatalf("unsafe approval presentation=%#v", presentation)
			}
		}
	}
	want := []string{
		ProcessStepStatusRunning, ProcessStepStatusAwaitingApproval,
		ProcessStepStatusRunning, ProcessStepStatusCompleted,
	}
	if strings.Join(statuses, ",") != strings.Join(want, ",") {
		t.Fatalf("statuses=%#v want=%#v", statuses, want)
	}
}

func TestChatAgentApprovalValidationAndRestartDenial(t *testing.T) {
	repository := newApprovalTestRepository()
	service := NewService(repository)
	if _, err := service.DecideChatAgentApproval(context.Background(), DecideChatAgentApprovalInput{
		ApprovalID: testApprovalID, ExpectedRevision: 0,
		Decision: ChatAgentApprovalAllowOnce, OccurredAt: time.Now(),
	}); err == nil {
		t.Fatal("invalid revision was accepted")
	}
	now := time.Now().UTC()
	repository.approvals[testApprovalID] = ChatAgentApproval{
		ID: testApprovalID, Status: ChatAgentApprovalPending, Revision: 1,
		ExpiresAt: now.Add(time.Minute), CreatedAt: now,
	}
	count, err := repository.RecoverPendingChatAgentApprovals(context.Background(), now)
	if err != nil || count != 1 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	current, err := repository.DecideChatAgentApproval(context.Background(), DecideChatAgentApprovalInput{
		ApprovalID: testApprovalID, ExpectedRevision: 1,
		Decision: ChatAgentApprovalAllowOnce, OccurredAt: now,
	})
	if err != nil || current.Status != ChatAgentApprovalDenied ||
		current.Decision != chatAgentApprovalRestartDeny {
		t.Fatalf("restart current=%#v error=%v", current, err)
	}
}
