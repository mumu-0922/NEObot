package chat

import (
	"context"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

const DevUserID = auth.DevelopmentUserID

type Repository interface {
	CreateConversation(ctx context.Context, input CreateConversationInput) (Conversation, error)
	ListConversations(ctx context.Context) ([]Conversation, error)
	GetConversation(ctx context.Context, conversationID string) (Conversation, error)
	UpdateConversation(ctx context.Context, conversationID string, input UpdateConversationInput) (Conversation, error)
	DeleteConversation(ctx context.Context, conversationID string) error
	DuplicateConversation(ctx context.Context, conversationID string, input DuplicateConversationInput) (Conversation, error)
	UpdateMessage(ctx context.Context, conversationID string, messageID string, input UpdateMessageInput) (Message, error)
	DeleteMessage(ctx context.Context, conversationID string, messageID string, input DeleteMessageInput) error
	GetMessage(ctx context.Context, conversationID string, messageID string) (Message, error)
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
	CreateMessage(ctx context.Context, conversationID string, input CreateMessageInput) (Message, error)
	CreateAssistantMessage(ctx context.Context, conversationID string, input CreateAssistantMessageInput) (Message, error)
	FinalizeAssistantMessage(ctx context.Context, conversationID string, messageID string, input FinalizeAssistantMessageInput) (Message, error)
	CancelRun(ctx context.Context, runID string, input CancelRunInput) (Message, error)
	GetConversationContextSummary(ctx context.Context, conversationID string) (ConversationContextSummary, bool, error)
	UpsertConversationContextSummary(ctx context.Context, conversationID string, input UpsertConversationContextSummaryInput) (ConversationContextSummary, error)
}

type ModelRef struct {
	ProviderID  string `json:"providerId"`
	ModelID     string `json:"modelId"`
	DisplayName string `json:"displayName,omitempty"`
}

type CreateConversationInput struct {
	Title          string
	ModelProvider  string
	ModelID        string
	SystemPrompt   string
	Metadata       map[string]any
	IdempotencyKey string
}

type DeleteMessageInput struct {
	DeleteSubsequent bool
}

type DuplicateConversationInput struct {
	Title          string
	IdempotencyKey string
}

type UpdateMessageInput struct {
	Content *string
}

type UpdateConversationInput struct {
	Title              *string
	SystemPrompt       *string
	ModelProvider      *string
	ModelID            *string
	MetadataMerge      map[string]any
	MetadataDeleteKeys []string
	ReplaceMetadata    *map[string]any
}

type Conversation struct {
	ID             string
	UserID         string
	Title          string
	Status         string
	ModelProvider  string
	ModelID        string
	SystemPrompt   string
	Metadata       map[string]any
	MessageCount   int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
	IdempotencyKey string
}

type CreateMessageInput struct {
	Role            string
	Content         string
	ParentMessageID string
	Metadata        map[string]any
	IdempotencyKey  string
	Attachments     []AttachmentInput
}

type AttachmentInput struct {
	Source  string
	FileID  string
	Purpose string
}

type Attachment struct {
	ID       string
	FileID   string
	FileName string
	MimeType string
	Size     int64
	SHA256   string
	Purpose  string
}

type CreateAssistantMessageInput struct {
	ID                string
	ParentMessageID   string
	ModelProvider     string
	ModelID           string
	ProviderMessageID string
	Metadata          map[string]any
	IdempotencyKey    string
	Attachments       []AttachmentInput
}

type FinalizeAssistantMessageInput struct {
	Status        string
	Content       string
	OutputBlocks  []any
	Metadata      map[string]any
	Attachments   []AttachmentInput
	MemoryCapture *MemoryCaptureInput
}

const MemoryCaptureEventSchemaMajor = 2

type MemoryCaptureInput struct {
	EventID          string
	JobID            string
	UserMessageID    string
	ProviderSource   string
	ProviderID       string
	ModelID          string
	EventSchemaMajor int
}

type CancelRunInput struct {
	Metadata map[string]any
}

type Message struct {
	ID                string
	ConversationID    string
	UserID            string
	ParentMessageID   string
	SequenceNo        int
	Role              string
	Status            string
	Content           string
	ModelProvider     string
	ModelID           string
	ProviderMessageID string
	IdempotencyKey    string
	OutputBlocks      []any
	Metadata          map[string]any
	Attachments       []Attachment
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CompletedAt       *time.Time
	DeletedAt         *time.Time
	memoryEventID     string
}

type ConversationContextSummary struct {
	ConversationID         string
	Version                int
	ModelProvider          string
	ModelID                string
	SourceFirstMessageID   string
	SourceLastMessageID    string
	SourceMessageCount     int
	SourceDigest           string
	Summary                string
	EstimatedSourceTokens  int
	EstimatedSummaryTokens int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type UpsertConversationContextSummaryInput struct {
	ModelProvider          string
	ModelID                string
	SourceFirstMessageID   string
	SourceLastMessageID    string
	SourceMessageCount     int
	SourceDigest           string
	Summary                string
	EstimatedSourceTokens  int
	EstimatedSummaryTokens int
}
