package memoryworker

import (
	"context"
	"encoding/json"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
)

const CurrentEventSchemaMajor = chat.MemoryCaptureEventSchemaMajor
const CandidateSchemaMajor = 1

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
	UserID                 string
	Messages               []CaptureMessage
	CurrentMemories        []CaptureMemory
	SensitiveMemoryEnabled bool
	ProjectID              string
	ProviderRecordID       string
	ProviderID             string
	ProviderLabel          string
	EncryptedSecretRef     string
	ProviderConfig         json.RawMessage
	ModelID                string
	ProcessingProfile      string
	ProposalCommitted      bool
}

type CaptureMessage struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	SequenceNo int       `json:"sequenceNo"`
	ObservedAt time.Time `json:"observedAt"`
}

type CaptureMemory struct {
	ID             string `json:"id"`
	Revision       int64  `json:"revision"`
	Type           string `json:"type"`
	Content        string `json:"content"`
	AuthorityKind  string `json:"authorityKind"`
	ScopeType      string `json:"scopeType"`
	ProjectID      string `json:"projectId"`
	ConversationID string `json:"conversationId"`
	FactKey        string `json:"factKey"`
	Sensitivity    string `json:"sensitivity"`
}

type CaptureProposal struct {
	ID                      string     `json:"id"`
	Type                    string     `json:"type"`
	Content                 *string    `json:"content"`
	NormalizedContent       *string    `json:"normalizedContent"`
	CandidateHash           string     `json:"candidateHash"`
	Importance              int        `json:"importance"`
	Tags                    []string   `json:"tags"`
	SubjectKey              *string    `json:"subjectKey"`
	FactKey                 *string    `json:"factKey"`
	Sensitivity             string     `json:"sensitivity"`
	Confidence              float64    `json:"confidence"`
	ConfidenceBand          string     `json:"confidenceBand"`
	AuthorityUserMessageIDs []string   `json:"authorityUserMessageIds"`
	ContextMessageIDs       []string   `json:"contextMessageIds"`
	ConfirmationKind        string     `json:"confirmationKind"`
	ProposedScopeType       string     `json:"proposedScopeType"`
	ProposedProjectID       *string    `json:"proposedProjectId"`
	ProposedConversationID  *string    `json:"proposedConversationId"`
	ScopeConfidence         float64    `json:"scopeConfidence"`
	TemporalBasis           string     `json:"temporalBasis"`
	TemporalParserVersion   *string    `json:"temporalParserVersion"`
	ObservedAt              time.Time  `json:"observedAt"`
	ValidFrom               *time.Time `json:"validFrom"`
	ValidTo                 *time.Time `json:"validTo"`
	FactExpiresAt           *time.Time `json:"factExpiresAt"`
	ProposedAction          string     `json:"proposedAction"`
	TargetMemoryIDs         []string   `json:"targetMemoryIds"`
}

type ProposalBatch struct {
	ExpiryJobID          string
	CandidateSchemaMajor int
	ExtractionProfileID  string
	DecisionProfileID    string
	Candidates           []CaptureProposal
}

type ProposalSummary struct {
	ProposalCount int
	ShadowCount   int
	ReviewCount   int
	RejectedCount int
}

type PromotionSummary struct {
	PromotedCount int
	ReviewCount   int
	RejectedCount int
}

type Readiness struct {
	ConsumerReady   bool
	PendingCount    int64
	ProcessingCount int64
	DeadLetterCount int64
	OldestPendingAt *time.Time
}

type EmbeddingJob struct {
	JobID                   string
	UserID                  string
	MemoryID                string
	ProjectionGeneration    int64
	MemoryRevision          int64
	ContentHash             string
	VisibilityEpoch         int64
	ScopeType               string
	ProjectID               string
	ScopeConversationID     string
	ScopeGeneration         int64
	EmbeddingProfileID      string
	EmbeddingModelID        string
	EmbeddingDimensions     int
	AttemptCount            int
	MaxAttempts             int
	ProviderRecordID        string
	ProviderConfigUpdatedAt time.Time
	WorkerID                string
	LeaseToken              string
	LeaseExpiresAt          time.Time
}

type EmbeddingCapture struct {
	UserID                  string
	MemoryID                string
	Content                 string
	ContentHash             string
	MemoryRevision          int64
	ProjectionGeneration    int64
	VisibilityEpoch         int64
	ScopeType               string
	ProjectID               string
	ScopeConversationID     string
	ScopeGeneration         int64
	EmbeddingProfileID      string
	EmbeddingModelID        string
	EmbeddingDimensions     int
	ProviderRecordID        string
	ProviderID              string
	ProviderLabel           string
	EncryptedSecretRef      string
	ProviderConfig          json.RawMessage
	ProviderConfigUpdatedAt time.Time
}

type Repository interface {
	Heartbeat(context.Context, string, time.Duration, bool) error
	Retire(context.Context, string) error
	Claim(context.Context, string, string, time.Duration) (Job, bool, error)
	Hydrate(context.Context, Job) (Capture, error)
	ProposeCandidates(context.Context, Job, ProposalBatch) (ProposalSummary, error)
	PromoteCandidates(context.Context, Job) (PromotionSummary, error)
	Purge(context.Context, Job) error
	ExpireReviews(context.Context, Job) (int, error)
	Complete(context.Context, Job) error
	Retry(context.Context, Job, string, time.Time, bool) (string, error)
	CheckReady(context.Context) (Readiness, error)
}

type ProviderResolver interface {
	Resolve(context.Context, Capture) (chat.Provider, error)
}

type EmbeddingRepository interface {
	ClaimEmbedding(context.Context, string, string, time.Duration) (EmbeddingJob, bool, error)
	HydrateEmbedding(context.Context, EmbeddingJob) (EmbeddingCapture, error)
	CompleteEmbedding(context.Context, EmbeddingJob, []float32) error
	RetryEmbedding(context.Context, EmbeddingJob, string, time.Time, bool) (string, error)
}

type MemoryEmbeddingProvider interface {
	EmbedMemory(context.Context, EmbeddingCapture) ([]float32, error)
}
