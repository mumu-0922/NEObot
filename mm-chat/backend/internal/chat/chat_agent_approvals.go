package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	ChatAgentApprovalPending = "pending"
	ChatAgentApprovalAllowed = "allowed"
	ChatAgentApprovalDenied  = "denied"
	ChatAgentApprovalExpired = "expired"

	ChatAgentApprovalAllowOnce         = "allow_once"
	ChatAgentApprovalAllowConversation = "allow_conversation"
	ChatAgentApprovalDeny              = "deny"
	chatAgentApprovalExpire            = "expired"
	chatAgentApprovalRestartDeny       = "restart_denied"

	chatAgentApprovalTTL = 5 * time.Minute
)

var errChatAgentApprovalPersistence = errors.New("chat Agent approval persistence failed")

type ChatAgentApproval struct {
	ID                string     `json:"id"`
	TurnID            string     `json:"turnId"`
	UserID            string     `json:"-"`
	ConversationID    string     `json:"conversationId"`
	MessageID         string     `json:"messageId"`
	RunID             string     `json:"runId"`
	ExecutionID       string     `json:"executionId"`
	ToolName          string     `json:"toolName"`
	RiskClass         string     `json:"riskClass"`
	Status            string     `json:"status"`
	Decision          string     `json:"decision,omitempty"`
	Revision          int64      `json:"revision"`
	AllowConversation bool       `json:"allowConversation"`
	ExpiresAt         time.Time  `json:"expiresAt"`
	CreatedAt         time.Time  `json:"createdAt"`
	DecidedAt         *time.Time `json:"decidedAt,omitempty"`
}

type CreateChatAgentApprovalInput struct {
	ID                string
	TurnID            string
	ExecutionID       string
	ToolName          string
	RiskClass         string
	AllowConversation bool
	ExpiresAt         time.Time
	OccurredAt        time.Time
}

type DecideChatAgentApprovalInput struct {
	ApprovalID       string
	ExpectedRevision int64
	Decision         string
	OccurredAt       time.Time
}

type ChatAgentApprovalRepository interface {
	CreateChatAgentApproval(context.Context, CreateChatAgentApprovalInput) (ChatAgentApproval, error)
	DecideChatAgentApproval(context.Context, DecideChatAgentApprovalInput) (ChatAgentApproval, error)
	RecoverPendingChatAgentApprovals(context.Context, time.Time) (int, error)
}

type ChatAgentApprovalError struct{ Code string }

func (e ChatAgentApprovalError) Error() string { return e.Code }

func (s *Service) CreateChatAgentApproval(
	ctx context.Context,
	input CreateChatAgentApprovalInput,
) (ChatAgentApproval, error) {
	repository, err := s.chatAgentApprovalRepository()
	if err != nil {
		return ChatAgentApproval{}, err
	}
	input.ID = strings.TrimSpace(input.ID)
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.ExecutionID = strings.TrimSpace(input.ExecutionID)
	input.ToolName = normalizedToolName(input.ToolName)
	input.RiskClass = strings.TrimSpace(input.RiskClass)
	if !isUUID(input.ID) || !isUUID(input.TurnID) || input.ExecutionID == "" ||
		len(input.ExecutionID) > 256 || input.ToolName == "unknown" ||
		(input.RiskClass != string(chatToolRiskWrite) &&
			input.RiskClass != string(chatToolRiskExecute) &&
			input.RiskClass != string(chatToolRiskExternal)) ||
		input.OccurredAt.IsZero() || input.ExpiresAt.IsZero() ||
		!input.ExpiresAt.After(input.OccurredAt) ||
		input.ExpiresAt.After(input.OccurredAt.Add(chatAgentApprovalTTL)) {
		return ChatAgentApproval{}, newValidationError(
			"INVALID_CHAT_AGENT_APPROVAL", "chat Agent approval input is invalid",
		)
	}
	return repository.CreateChatAgentApproval(ctx, input)
}

func (s *Service) DecideChatAgentApproval(
	ctx context.Context,
	input DecideChatAgentApprovalInput,
) (ChatAgentApproval, error) {
	repository, err := s.chatAgentApprovalRepository()
	if err != nil {
		return ChatAgentApproval{}, err
	}
	input.ApprovalID = strings.TrimSpace(input.ApprovalID)
	input.Decision = strings.TrimSpace(input.Decision)
	if !isUUID(input.ApprovalID) || input.ExpectedRevision < 1 || input.OccurredAt.IsZero() {
		return ChatAgentApproval{}, newValidationError(
			"INVALID_CHAT_AGENT_APPROVAL_DECISION", "chat Agent approval decision is invalid",
		)
	}
	switch input.Decision {
	case ChatAgentApprovalAllowOnce, ChatAgentApprovalAllowConversation,
		ChatAgentApprovalDeny, chatAgentApprovalExpire, chatAgentApprovalRestartDeny:
	default:
		return ChatAgentApproval{}, newValidationError(
			"INVALID_CHAT_AGENT_APPROVAL_DECISION", "chat Agent approval decision is invalid",
		)
	}
	return repository.DecideChatAgentApproval(ctx, input)
}

func (s *Service) chatAgentApprovalRepository() (ChatAgentApprovalRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repository, ok := s.repo.(ChatAgentApprovalRepository)
	if !ok {
		return nil, ErrDatabaseRequired
	}
	return repository, nil
}

type chatAgentApprovalWaiters struct {
	mu      sync.Mutex
	waiters map[string]chan ChatAgentApproval
}

func newChatAgentApprovalWaiters() *chatAgentApprovalWaiters {
	return &chatAgentApprovalWaiters{waiters: make(map[string]chan ChatAgentApproval)}
}

func (waiters *chatAgentApprovalWaiters) register(approvalID string) <-chan ChatAgentApproval {
	waiters.mu.Lock()
	defer waiters.mu.Unlock()
	channel := make(chan ChatAgentApproval, 1)
	waiters.waiters[approvalID] = channel
	return channel
}

func (waiters *chatAgentApprovalWaiters) unregister(approvalID string) {
	waiters.mu.Lock()
	delete(waiters.waiters, approvalID)
	waiters.mu.Unlock()
}

func (waiters *chatAgentApprovalWaiters) resolve(approval ChatAgentApproval) {
	waiters.mu.Lock()
	channel := waiters.waiters[approval.ID]
	if channel != nil {
		delete(waiters.waiters, approval.ID)
	}
	waiters.mu.Unlock()
	if channel != nil {
		channel <- approval
	}
}

type chatToolApprovalRuntime struct {
	service *Service
	waiters *chatAgentApprovalWaiters
	turnID  string
}

func newChatToolApprovalRuntime(
	service *Service,
	waiters *chatAgentApprovalWaiters,
	turnID string,
) *chatToolApprovalRuntime {
	if service == nil || waiters == nil || !isUUID(strings.TrimSpace(turnID)) {
		return nil
	}
	if _, err := service.chatAgentApprovalRepository(); err != nil {
		return nil
	}
	return &chatToolApprovalRuntime{service: service, waiters: waiters, turnID: turnID}
}

func (runtime *chatToolApprovalRuntime) request(
	ctx context.Context,
	execution ProviderToolExecutionEvent,
	allowConversation bool,
) (ChatAgentApproval, <-chan ChatAgentApproval, error) {
	if runtime == nil || runtime.service == nil || runtime.waiters == nil {
		return ChatAgentApproval{}, nil, ErrDatabaseRequired
	}
	approvalID, err := NewUUID()
	if err != nil {
		return ChatAgentApproval{}, nil, err
	}
	now := time.Now().UTC()
	approval, err := runtime.service.CreateChatAgentApproval(ctx, CreateChatAgentApprovalInput{
		ID: approvalID, TurnID: runtime.turnID, ExecutionID: execution.ExecutionID,
		ToolName: execution.Name, RiskClass: execution.Classification,
		AllowConversation: allowConversation,
		OccurredAt:        now, ExpiresAt: now.Add(chatAgentApprovalTTL),
	})
	if err != nil || approval.Status != ChatAgentApprovalPending {
		return approval, nil, err
	}
	return approval, runtime.waiters.register(approval.ID), nil
}

func (runtime *chatToolApprovalRuntime) wait(
	ctx context.Context,
	approval ChatAgentApproval,
	decisions <-chan ChatAgentApproval,
) (ChatAgentApproval, error) {
	if decisions == nil || approval.Status != ChatAgentApprovalPending {
		return approval, nil
	}
	defer runtime.waiters.unregister(approval.ID)
	delay := time.Until(approval.ExpiresAt)
	if delay < 0 {
		delay = 0
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case decision := <-decisions:
		return decision, nil
	case <-timer.C:
		return runtime.finishPending(ctx, approval, chatAgentApprovalExpire)
	case <-ctx.Done():
		_, _ = runtime.finishPending(context.WithoutCancel(ctx), approval, ChatAgentApprovalDeny)
		return ChatAgentApproval{}, ctx.Err()
	}
}

func (runtime *chatToolApprovalRuntime) finishPending(
	ctx context.Context,
	approval ChatAgentApproval,
	decision string,
) (ChatAgentApproval, error) {
	if runtime == nil || runtime.service == nil {
		return ChatAgentApproval{}, errors.New("chat Agent approval runtime unavailable")
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	resolved, err := runtime.service.DecideChatAgentApproval(finishCtx, DecideChatAgentApprovalInput{
		ApprovalID: approval.ID, ExpectedRevision: approval.Revision,
		Decision: decision, OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		return ChatAgentApproval{}, fmt.Errorf("finish chat Agent approval: %w", err)
	}
	runtime.waiters.resolve(resolved)
	return resolved, nil
}

func chatAgentApprovalAllowsExecution(approval ChatAgentApproval) bool {
	return approval.Status == ChatAgentApprovalAllowed &&
		(approval.Decision == ChatAgentApprovalAllowOnce ||
			approval.Decision == ChatAgentApprovalAllowConversation)
}
