package memoryworker

import (
	"context"
	"encoding/json"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const CurrentEventSchemaMajor = chat.MemoryCaptureEventSchemaMajor

type Job struct {
	JobID                  string
	UserID                 string
	EventID                string
	EventSchemaMajor       int
	Stage                  string
	AttemptCount           int
	MaxAttempts            int
	SourceConversationID   string
	SourceMessageID        string
	AssistantMessageID     string
	SourceHash             string
	ProviderSource         string
	ProviderID             string
	ProviderRecordID       string
	ModelID                string
	ProcessingProfile      string
	ScopeGeneration        int64
	ProjectScopeGeneration *int64
	VisibilityEpoch        int64
	WorkerID               string
	LeaseToken             string
	LeaseExpiresAt         time.Time
}

type Capture struct {
	UserID             string
	UserMessageContent string
	ProviderRecordID   string
	ProviderID         string
	ProviderLabel      string
	EncryptedSecretRef string
	ProviderConfig     json.RawMessage
	ModelID            string
	ProcessingProfile  string
}

type Readiness struct {
	ConsumerReady   bool
	PendingCount    int64
	ProcessingCount int64
	DeadLetterCount int64
	OldestPendingAt *time.Time
}

type Repository interface {
	Claim(context.Context, string, string, time.Duration) (Job, bool, error)
	Hydrate(context.Context, Job) (Capture, error)
	ApplyCandidate(context.Context, Job, usermemory.CreateInput) (usermemory.Memory, error)
	Complete(context.Context, Job) error
	Retry(context.Context, Job, string, time.Time, bool) (string, error)
	CheckReady(context.Context) (Readiness, error)
}

type ProviderResolver interface {
	Resolve(context.Context, Capture) (chat.Provider, error)
}
