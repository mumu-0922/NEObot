package chat

import (
	"context"
	"encoding/hex"
	"strings"
)

const (
	maxMessageAttachments        = 20
	maxContextSummaryBytes       = 64 * 1024
	contextSummaryDigestHexBytes = 32
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
	if _, hasMode := input.Metadata["searchMode"]; !hasMode {
		if _, hasLegacy := input.Metadata["useSearch"]; !hasLegacy {
			conversations, err := s.repo.ListConversations(ctx)
			if err != nil {
				return Conversation{}, err
			}
			inherited := inheritedConversationSearchMode(conversations)
			if err := normalizeConversationSearchMetadata(input.Metadata, &inherited); err != nil {
				return Conversation{}, err
			}
		}
	}
	if err := normalizeConversationSearchMetadata(input.Metadata, nil); err != nil {
		return Conversation{}, err
	}
	if err := normalizeConversationRAGMetadata(input.Metadata); err != nil {
		return Conversation{}, err
	}

	return s.repo.CreateConversation(ctx, input)
}

func (s *Service) ListConversations(ctx context.Context) ([]Conversation, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}

	return s.repo.ListConversations(ctx)
}

func (s *Service) GetConversation(ctx context.Context, conversationID string) (Conversation, error) {
	if err := s.requireRepository(); err != nil {
		return Conversation{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return Conversation{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}

	return s.repo.GetConversation(ctx, conversationID)
}

func (s *Service) UpdateConversation(
	ctx context.Context,
	conversationID string,
	input UpdateConversationInput,
) (Conversation, error) {
	if err := s.requireRepository(); err != nil {
		return Conversation{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return Conversation{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}

	if input.Title != nil {
		value := strings.TrimSpace(*input.Title)
		input.Title = &value
	}
	if input.SystemPrompt != nil {
		value := strings.TrimSpace(*input.SystemPrompt)
		input.SystemPrompt = &value
	}
	if input.ModelProvider != nil {
		value := strings.TrimSpace(*input.ModelProvider)
		input.ModelProvider = &value
	}
	if input.ModelID != nil {
		value := strings.TrimSpace(*input.ModelID)
		input.ModelID = &value
	}
	if input.MetadataMerge == nil {
		input.MetadataMerge = map[string]any{}
	}
	if err := normalizeConversationSearchMetadata(input.MetadataMerge, nil); err != nil {
		return Conversation{}, err
	}
	if err := normalizeConversationRAGMetadata(input.MetadataMerge); err != nil {
		return Conversation{}, err
	}
	if input.ReplaceMetadata != nil && *input.ReplaceMetadata == nil {
		empty := map[string]any{}
		input.ReplaceMetadata = &empty
	}
	if input.ReplaceMetadata != nil {
		if err := normalizeConversationSearchMetadata(*input.ReplaceMetadata, nil); err != nil {
			return Conversation{}, err
		}
		if err := normalizeConversationRAGMetadata(*input.ReplaceMetadata); err != nil {
			return Conversation{}, err
		}
	}

	return s.repo.UpdateConversation(ctx, conversationID, input)
}

func (s *Service) DeleteConversation(ctx context.Context, conversationID string) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}

	return s.repo.DeleteConversation(ctx, conversationID)
}

func (s *Service) DuplicateConversation(
	ctx context.Context,
	conversationID string,
	input DuplicateConversationInput,
) (Conversation, error) {
	if err := s.requireRepository(); err != nil {
		return Conversation{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return Conversation{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)

	return s.repo.DuplicateConversation(ctx, conversationID, input)
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

func (s *Service) GetConversationContextSummary(
	ctx context.Context,
	conversationID string,
) (ConversationContextSummary, bool, error) {
	if err := s.requireRepository(); err != nil {
		return ConversationContextSummary{}, false, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return ConversationContextSummary{}, false, newValidationError(
			"INVALID_CONVERSATION_ID",
			"conversation id must be a UUID",
		)
	}
	return s.repo.GetConversationContextSummary(ctx, conversationID)
}

func (s *Service) UpsertConversationContextSummary(
	ctx context.Context,
	conversationID string,
	input UpsertConversationContextSummaryInput,
) (ConversationContextSummary, error) {
	if err := s.requireRepository(); err != nil {
		return ConversationContextSummary{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return ConversationContextSummary{}, newValidationError(
			"INVALID_CONVERSATION_ID",
			"conversation id must be a UUID",
		)
	}
	input.ModelProvider = strings.TrimSpace(input.ModelProvider)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.SourceFirstMessageID = strings.TrimSpace(input.SourceFirstMessageID)
	input.SourceLastMessageID = strings.TrimSpace(input.SourceLastMessageID)
	if !isUUID(input.SourceFirstMessageID) || !isUUID(input.SourceLastMessageID) {
		return ConversationContextSummary{}, newValidationError(
			"INVALID_CONTEXT_SUMMARY_BOUNDARY",
			"context summary boundaries must be message UUIDs",
		)
	}
	if input.SourceMessageCount <= 0 {
		return ConversationContextSummary{}, newValidationError(
			"INVALID_CONTEXT_SUMMARY_COUNT",
			"context summary source message count must be positive",
		)
	}
	input.SourceDigest = strings.ToLower(strings.TrimSpace(input.SourceDigest))
	decodedDigest, err := hex.DecodeString(input.SourceDigest)
	if err != nil || len(decodedDigest) != contextSummaryDigestHexBytes {
		return ConversationContextSummary{}, newValidationError(
			"INVALID_CONTEXT_SUMMARY_DIGEST",
			"context summary source digest must be a SHA-256 hex value",
		)
	}
	input.Summary = strings.TrimSpace(input.Summary)
	if input.Summary == "" || len(input.Summary) > maxContextSummaryBytes {
		return ConversationContextSummary{}, newValidationError(
			"INVALID_CONTEXT_SUMMARY",
			"context summary must be non-empty and at most 64 KiB",
		)
	}
	if input.EstimatedSourceTokens < 0 || input.EstimatedSummaryTokens < 0 {
		return ConversationContextSummary{}, newValidationError(
			"INVALID_CONTEXT_SUMMARY_TOKENS",
			"context summary token estimates must be non-negative",
		)
	}

	return s.repo.UpsertConversationContextSummary(ctx, conversationID, input)
}

func (s *Service) UpdateMessage(
	ctx context.Context,
	conversationID string,
	messageID string,
	input UpdateMessageInput,
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
	if input.Content == nil {
		return Message{}, newValidationError("NO_MESSAGE_UPDATES", "message update requires at least one editable field")
	}
	content := strings.TrimSpace(*input.Content)
	if content == "" {
		return Message{}, newValidationError("EMPTY_CONTENT", "message content is required")
	}
	input.Content = &content

	return s.repo.UpdateMessage(ctx, conversationID, messageID, input)
}

func (s *Service) DeleteMessage(
	ctx context.Context,
	conversationID string,
	messageID string,
	input DeleteMessageInput,
) error {
	if err := s.requireRepository(); err != nil {
		return err
	}
	conversationID = strings.TrimSpace(conversationID)
	if !isUUID(conversationID) {
		return newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}
	messageID = strings.TrimSpace(messageID)
	if !isUUID(messageID) {
		return newValidationError("INVALID_MESSAGE_ID", "message id must be a UUID")
	}

	return s.repo.DeleteMessage(ctx, conversationID, messageID, input)
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
	attachments, err := normalizeAttachmentInputs(input.Attachments)
	if err != nil {
		return Message{}, err
	}
	input.Attachments = attachments
	if strings.TrimSpace(input.Content) == "" && len(input.Attachments) == 0 {
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
	attachments, err := normalizeAttachmentInputs(input.Attachments)
	if err != nil {
		return Message{}, err
	}
	input.Attachments = attachments

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
	attachments, err := normalizeAttachmentInputs(input.Attachments)
	if err != nil {
		return Message{}, err
	}
	input.Attachments = attachments
	if input.MemoryCapture != nil {
		capture := *input.MemoryCapture
		capture.EventID = strings.TrimSpace(capture.EventID)
		capture.JobID = strings.TrimSpace(capture.JobID)
		capture.UserMessageID = strings.TrimSpace(capture.UserMessageID)
		capture.ProviderSource = strings.ToLower(strings.TrimSpace(capture.ProviderSource))
		capture.ProviderID = strings.TrimSpace(capture.ProviderID)
		capture.ModelID = strings.TrimSpace(capture.ModelID)
		if input.Status != "completed" {
			return Message{}, newValidationError(
				"INVALID_MEMORY_CAPTURE_STATUS",
				"memory capture requires a completed assistant message",
			)
		}
		if !isUUID(capture.EventID) || !isUUID(capture.JobID) ||
			!isUUID(capture.UserMessageID) {
			return Message{}, newValidationError(
				"INVALID_MEMORY_CAPTURE_ID",
				"memory capture ids must be UUIDs",
			)
		}
		switch capture.ProviderSource {
		case "server-default", "server-stored", "request", "legacy":
		default:
			return Message{}, newValidationError(
				"INVALID_MEMORY_PROVIDER_SOURCE",
				"memory capture provider source is invalid",
			)
		}
		if capture.ProviderID == "" || len(capture.ProviderID) > 512 ||
			capture.ModelID == "" || len(capture.ModelID) > 512 ||
			capture.EventSchemaMajor != MemoryCaptureEventSchemaMajor {
			return Message{}, newValidationError(
				"INVALID_MEMORY_PROVIDER_PROFILE",
				"memory capture provider profile is invalid",
			)
		}
		input.MemoryCapture = &capture
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
