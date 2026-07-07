package chat

import (
	"context"
	"strings"
)

const maxMessageAttachments = 20

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

func (s *Service) GetMessage(
	ctx context.Context,
	conversationID string,
	messageID string,
) (Message, error) {
	if err := s.requireRepository(); err != nil {
		return Message{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return Message{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}
	messageID = strings.TrimSpace(messageID)
	if !isUUID(messageID) {
		return Message{}, newValidationError("INVALID_USER_MESSAGE_ID", "userMessageId must be a UUID")
	}

	return s.repo.GetMessage(ctx, conversationID, messageID)
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
	attachments, err := normalizeAttachmentInputs(input.Attachments)
	if err != nil {
		return Message{}, err
	}
	input.Attachments = attachments

	return s.repo.CreateMessage(ctx, conversationID, input)
}

func (s *Service) CreateAssistantMessage(
	ctx context.Context,
	conversationID string,
	input CreateAssistantMessageInput,
) (Message, error) {
	if err := s.requireRepository(); err != nil {
		return Message{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return Message{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}

	input.ID = strings.TrimSpace(input.ID)
	if input.ID != "" && !isUUID(input.ID) {
		return Message{}, newValidationError("INVALID_MESSAGE_ID", "message id must be a UUID")
	}
	input.ParentMessageID = strings.TrimSpace(input.ParentMessageID)
	if input.ParentMessageID != "" && !isUUID(input.ParentMessageID) {
		return Message{}, newValidationError("INVALID_PARENT_MESSAGE_ID", "parent message id must be a UUID")
	}
	input.ModelProvider = strings.TrimSpace(input.ModelProvider)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.ProviderMessageID = strings.TrimSpace(input.ProviderMessageID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" {
		return Message{}, newValidationError("IDEMPOTENCY_KEY_REQUIRED", "idempotencyKey is required")
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}

	return s.repo.CreateAssistantMessage(ctx, conversationID, input)
}

func (s *Service) FinalizeAssistantMessage(
	ctx context.Context,
	conversationID string,
	messageID string,
	input FinalizeAssistantMessageInput,
) (Message, error) {
	if err := s.requireRepository(); err != nil {
		return Message{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return Message{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}
	messageID = strings.TrimSpace(messageID)
	if !isUUID(messageID) {
		return Message{}, newValidationError("INVALID_MESSAGE_ID", "message id must be a UUID")
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	switch input.Status {
	case "completed", "failed", "cancelled":
	default:
		return Message{}, newValidationError("INVALID_MESSAGE_STATUS", "assistant status must be completed, failed, or cancelled")
	}
	if input.OutputBlocks == nil {
		input.OutputBlocks = []any{}
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}

	return s.repo.FinalizeAssistantMessage(ctx, conversationID, messageID, input)
}

func (s *Service) CancelRun(
	ctx context.Context,
	runID string,
	input CancelRunInput,
) (Message, error) {
	if err := s.requireRepository(); err != nil {
		return Message{}, err
	}
	runID = strings.TrimSpace(runID)
	if !isUUID(runID) {
		return Message{}, newValidationError("INVALID_RUN_ID", "run id must be a UUID")
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}

	return s.repo.CancelRun(ctx, runID, input)
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

func normalizeAttachmentInputs(inputs []AttachmentInput) ([]AttachmentInput, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) > maxMessageAttachments {
		return nil, newValidationError("TOO_MANY_ATTACHMENTS", "too many message attachments")
	}

	seen := make(map[string]struct{}, len(inputs))
	normalized := make([]AttachmentInput, 0, len(inputs))
	for _, input := range inputs {
		source := strings.ToLower(strings.TrimSpace(input.Source))
		if source != "" && source != "server" {
			return nil, newValidationError("UNSUPPORTED_ATTACHMENT_SOURCE", "only server attachments can be linked")
		}

		fileID := strings.TrimSpace(input.FileID)
		if !isUUID(fileID) {
			return nil, newValidationError("INVALID_ATTACHMENT_FILE_ID", "attachment fileId must be a UUID")
		}
		fileKey := strings.ToLower(fileID)
		if _, ok := seen[fileKey]; ok {
			return nil, newValidationError("DUPLICATE_ATTACHMENT", "duplicate attachment fileId")
		}
		seen[fileKey] = struct{}{}

		purpose, err := normalizeAttachmentPurpose(input.Purpose)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, AttachmentInput{
			Source:  "server",
			FileID:  fileID,
			Purpose: purpose,
		})
	}

	return normalized, nil
}

func normalizeAttachmentPurpose(purpose string) (string, error) {
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	switch purpose {
	case "", "input", "chat":
		return "input", nil
	case "image":
		return "image", nil
	case "knowledge", "knowledge_source":
		return "knowledge_source", nil
	default:
		return "", newValidationError("INVALID_ATTACHMENT_PURPOSE", "attachment purpose is unsupported")
	}
}
