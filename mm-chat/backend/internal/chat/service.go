package chat

import (
	"context"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateConversation(
	ctx context.Context,
	input CreateConversationInput,
) (Conversation, error) {
	if err := s.requireRepository(); err != nil {
		return Conversation{}, err
	}

	input.Title = strings.TrimSpace(input.Title)
	input.ModelProvider = strings.TrimSpace(input.ModelProvider)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}

	return s.repo.CreateConversation(ctx, input)
}

func (s *Service) ListConversations(ctx context.Context) ([]Conversation, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}

	return s.repo.ListConversations(ctx)
}

func (s *Service) ListMessages(ctx context.Context, conversationID string) ([]Message, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return nil, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}

	return s.repo.ListMessages(ctx, conversationID)
}

func (s *Service) CreateMessage(
	ctx context.Context,
	conversationID string,
	input CreateMessageInput,
) (Message, error) {
	if err := s.requireRepository(); err != nil {
		return Message{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return Message{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}

	role, err := normalizeClientMessageRole(input.Role)
	if err != nil {
		return Message{}, err
	}
	input.Role = role
	if strings.TrimSpace(input.Content) == "" {
		return Message{}, newValidationError("EMPTY_CONTENT", "message content is required")
	}
	input.ParentMessageID = strings.TrimSpace(input.ParentMessageID)
	if input.ParentMessageID != "" && !isUUID(input.ParentMessageID) {
		return Message{}, newValidationError("INVALID_PARENT_MESSAGE_ID", "parent message id must be a UUID")
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}

	return s.repo.CreateMessage(ctx, conversationID, input)
}

func (s *Service) requireRepository() error {
	if s == nil || s.repo == nil {
		return ErrDatabaseRequired
	}

	return nil
}

func normalizeClientMessageRole(role string) (string, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return "user", nil
	}

	switch role {
	case "user":
		return role, nil
	default:
		return "", newValidationError(
			"FORBIDDEN_MESSAGE_FIELD",
			"only user messages can be created by this endpoint",
		)
	}
}
