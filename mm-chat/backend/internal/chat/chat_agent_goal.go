package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ChatAgentGoalActive   = "active"
	ChatAgentGoalPaused   = "paused"
	ChatAgentGoalBlocked  = "blocked"
	ChatAgentGoalComplete = "complete"

	ChatAgentGoalActionEdit     = "edit"
	ChatAgentGoalActionPause    = "pause"
	ChatAgentGoalActionResume   = "resume"
	ChatAgentGoalActionComplete = "complete"
	ChatAgentGoalActionBlocked  = "blocked"
	ChatAgentGoalActionCancel   = "cancel"

	minChatAgentMaxGoalRounds     = 3
	defaultChatAgentMaxGoalRounds = 8
	maxChatAgentMaxGoalRounds     = 32
	maxChatAgentGoalObjective     = 8192
	maxChatAgentGoalBlockReason   = 4096
)

type ChatAgentGoalBlockReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ChatAgentGoal struct {
	ID             string                    `json:"id"`
	ConversationID string                    `json:"conversationId"`
	UserID         string                    `json:"-"`
	Objective      string                    `json:"objective"`
	Phase          string                    `json:"phase"`
	Revision       int64                     `json:"revision"`
	RoundsStarted  int                       `json:"roundsStarted"`
	MaxGoalRounds  int                       `json:"maxGoalRounds"`
	BlockedReason  *ChatAgentGoalBlockReason `json:"blockedReason,omitempty"`
	CreatedAt      time.Time                 `json:"createdAt"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
}

type ChatAgentGoalRef struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

type CreateChatAgentGoalInput struct {
	TurnID        string
	EventID       string
	GoalID        string
	Objective     string
	MaxGoalRounds int
	OccurredAt    time.Time
}

type ChangeChatAgentGoalInput struct {
	TurnID           string
	EventID          string
	GoalID           string
	ExpectedRevision int64
	Action           string
	Objective        string
	MaxGoalRounds    *int
	BlockedReason    string
	OccurredAt       time.Time
}

type CancelChatAgentGoalInput struct {
	TurnID           string
	EventID          string
	GoalID           string
	ExpectedRevision int64
	OccurredAt       time.Time
}

type StartChatAgentGoalRoundInput struct {
	TurnID           string
	EventID          string
	GoalID           string
	ExpectedRevision int64
	Round            int
	OccurredAt       time.Time
}

type ChatAgentGoalRepository interface {
	GetChatAgentGoal(context.Context, string) (*ChatAgentGoal, error)
	CreateChatAgentGoal(context.Context, CreateChatAgentGoalInput) (ChatAgentGoal, error)
	ChangeChatAgentGoal(context.Context, ChangeChatAgentGoalInput) (ChatAgentGoal, error)
	CancelChatAgentGoal(context.Context, CancelChatAgentGoalInput) (ChatAgentGoalRef, error)
	StartChatAgentGoalRound(context.Context, StartChatAgentGoalRoundInput) (ChatAgentGoal, error)
}

type ChatAgentGoalError struct {
	Code string
}

func (err ChatAgentGoalError) Error() string {
	return err.Code
}

func (s *Service) GetChatAgentGoal(
	ctx context.Context,
	conversationID string,
) (*ChatAgentGoal, error) {
	repository, err := s.chatAgentGoalRepository()
	if err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return nil, newValidationError(
			"INVALID_CONVERSATION_ID", "conversation id must be a UUID",
		)
	}
	return repository.GetChatAgentGoal(ctx, conversationID)
}

func (s *Service) CreateChatAgentGoal(
	ctx context.Context,
	input CreateChatAgentGoalInput,
) (ChatAgentGoal, error) {
	repository, err := s.chatAgentGoalRepository()
	if err != nil {
		return ChatAgentGoal{}, err
	}
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.GoalID = strings.TrimSpace(input.GoalID)
	input.Objective = strings.TrimSpace(input.Objective)
	if !isUUID(input.TurnID) || !isUUID(input.EventID) || !isUUID(input.GoalID) ||
		input.OccurredAt.IsZero() || input.Objective == "" ||
		len(input.Objective) > maxChatAgentGoalObjective ||
		input.MaxGoalRounds < minChatAgentMaxGoalRounds ||
		input.MaxGoalRounds > maxChatAgentMaxGoalRounds {
		return ChatAgentGoal{}, newValidationError(
			"INVALID_CHAT_AGENT_GOAL", "chat Agent Goal input is invalid",
		)
	}
	return repository.CreateChatAgentGoal(ctx, input)
}

func (s *Service) ChangeChatAgentGoal(
	ctx context.Context,
	input ChangeChatAgentGoalInput,
) (ChatAgentGoal, error) {
	repository, err := s.chatAgentGoalRepository()
	if err != nil {
		return ChatAgentGoal{}, err
	}
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.GoalID = strings.TrimSpace(input.GoalID)
	input.Action = strings.TrimSpace(input.Action)
	input.Objective = strings.TrimSpace(input.Objective)
	input.BlockedReason = strings.TrimSpace(input.BlockedReason)
	if !isUUID(input.TurnID) || !isUUID(input.EventID) || !isUUID(input.GoalID) ||
		input.ExpectedRevision < 1 || input.OccurredAt.IsZero() ||
		!isChatAgentGoalChangeAction(input.Action) ||
		len(input.Objective) > maxChatAgentGoalObjective ||
		len(input.BlockedReason) > maxChatAgentGoalBlockReason ||
		(input.MaxGoalRounds != nil &&
			(*input.MaxGoalRounds < minChatAgentMaxGoalRounds ||
				*input.MaxGoalRounds > maxChatAgentMaxGoalRounds)) {
		return ChatAgentGoal{}, newValidationError(
			"INVALID_CHAT_AGENT_GOAL", "chat Agent Goal change is invalid",
		)
	}
	return repository.ChangeChatAgentGoal(ctx, input)
}

func (s *Service) CancelChatAgentGoal(
	ctx context.Context,
	input CancelChatAgentGoalInput,
) (ChatAgentGoalRef, error) {
	repository, err := s.chatAgentGoalRepository()
	if err != nil {
		return ChatAgentGoalRef{}, err
	}
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.GoalID = strings.TrimSpace(input.GoalID)
	if !isUUID(input.TurnID) || !isUUID(input.EventID) || !isUUID(input.GoalID) ||
		input.ExpectedRevision < 1 || input.OccurredAt.IsZero() {
		return ChatAgentGoalRef{}, newValidationError(
			"INVALID_CHAT_AGENT_GOAL", "chat Agent Goal cancellation is invalid",
		)
	}
	return repository.CancelChatAgentGoal(ctx, input)
}

func (s *Service) StartChatAgentGoalRound(
	ctx context.Context,
	input StartChatAgentGoalRoundInput,
) (ChatAgentGoal, error) {
	repository, err := s.chatAgentGoalRepository()
	if err != nil {
		return ChatAgentGoal{}, err
	}
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.GoalID = strings.TrimSpace(input.GoalID)
	if !isUUID(input.TurnID) || !isUUID(input.EventID) || !isUUID(input.GoalID) ||
		input.ExpectedRevision < 1 || input.Round < 1 || input.OccurredAt.IsZero() {
		return ChatAgentGoal{}, newValidationError(
			"INVALID_CHAT_AGENT_GOAL_ROUND", "chat Agent Goal round is invalid",
		)
	}
	return repository.StartChatAgentGoalRound(ctx, input)
}

func (s *Service) chatAgentGoalRepository() (ChatAgentGoalRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repository, ok := s.repo.(ChatAgentGoalRepository)
	if !ok {
		return nil, ErrDatabaseRequired
	}
	return repository, nil
}

func isChatAgentGoalChangeAction(action string) bool {
	switch action {
	case ChatAgentGoalActionEdit,
		ChatAgentGoalActionPause,
		ChatAgentGoalActionResume,
		ChatAgentGoalActionComplete,
		ChatAgentGoalActionBlocked:
		return true
	default:
		return false
	}
}

func chatAgentGoalErrorCode(err error) string {
	var goalError ChatAgentGoalError
	if errors.As(err, &goalError) {
		return strings.TrimSpace(goalError.Code)
	}
	var validationError ValidationError
	if errors.As(err, &validationError) {
		return strings.TrimSpace(validationError.Code)
	}
	return ""
}

func newChatAgentGoalIDs() (goalID string, eventID string, err error) {
	goalID, err = NewUUID()
	if err != nil {
		return "", "", fmt.Errorf("create Goal id: %w", err)
	}
	eventID, err = NewUUID()
	if err != nil {
		return "", "", fmt.Errorf("create Goal event id: %w", err)
	}
	return goalID, eventID, nil
}
