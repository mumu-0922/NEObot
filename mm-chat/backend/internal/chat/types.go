package chat

import (
	"context"
	"time"
)

const DevUserID = "00000000-0000-0000-0000-000000000001"

type Repository interface {
	CreateConversation(ctx context.Context, input CreateConversationInput) (Conversation, error)
	ListConversations(ctx context.Context) ([]Conversation, error)
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
	CreateMessage(ctx context.Context, conversationID string, input CreateMessageInput) (Message, error)
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
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CompletedAt       *time.Time
	DeletedAt         *time.Time
}
