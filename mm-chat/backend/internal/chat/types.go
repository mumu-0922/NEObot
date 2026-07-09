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
	GetMessage(ctx context.Context, conversationID string, messageID string) (Message, error)
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
	CreateMessage(ctx context.Context, conversationID string, input CreateMessageInput) (Message, error)
	CreateAssistantMessage(ctx context.Context, conversationID string, input CreateAssistantMessageInput) (Message, error)
	FinalizeAssistantMessage(ctx context.Context, conversationID string, messageID string, input FinalizeAssistantMessageInput) (Message, error)
	CancelRun(ctx context.Context, runID string, input CancelRunInput) (Message, error)
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
}

type FinalizeAssistantMessageInput struct {
	Status       string
	Content      string
	OutputBlocks []any
	Metadata     map[string]any
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
}
