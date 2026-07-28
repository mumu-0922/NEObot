package usermemory

import (
	"context"
	"errors"
	"time"
)

const (
	MaxMemories             = 500
	MaxContentChars         = 2000
	MaxTags                 = 12
	MaxTagChars             = 40
	MaxSearchResults        = 5
	MaxExtractedItems       = 5
	MaxActionTargets        = 5
	MaxActivityPage         = 100
	MaxLexicalShadowResults = 20
	MaxHybridShadowResults  = 20
	DirectActionSchemaMajor = 1
	LexicalShadowProfileID  = "memory_lexical_cjk_bm25_v1"
	HybridShadowProfileID   = "memory_hybrid_bge_m3_rrf60_v1"
	HybridEmbeddingProfile  = "siliconflow_bge_m3_v1"
)

var (
	ErrDatabaseRequired             = errors.New("memory database is required")
	ErrMemoryNotFound               = errors.New("memory not found")
	ErrMemoryConflict               = errors.New("memory content already exists")
	ErrActionRepositoryRequired     = errors.New("memory action repository is required")
	ErrActivityNotFound             = errors.New("memory activity not found")
	ErrActivityUndoUnavailable      = errors.New("memory activity undo is unavailable")
	ErrGovernanceRepositoryRequired = errors.New("memory governance repository is required")
	ErrMemoryProjectNotFound        = errors.New("memory project not found")
	ErrConversationPolicyNotFound   = errors.New("conversation memory policy not found")
	ErrMemoryReviewNotFound         = errors.New("memory review not found")
)

type Repository interface {
	GetSettings(context.Context) (Settings, bool, error)
	UpsertSettings(context.Context, Settings) (Settings, error)
	List(context.Context) ([]Memory, error)
	Create(context.Context, CreateInput) (Memory, error)
	Update(context.Context, string, UpdateInput) (Memory, error)
	Delete(context.Context, string) error
	MarkUsed(context.Context, []string, time.Time) error
}

// ActionRepository is optional so the v1 Repository contract and its test
// doubles remain source-compatible while PR6 capabilities roll out.
type ActionRepository interface {
	HydrateDirectAction(context.Context, DirectActionHydrationInput) (DirectActionContext, error)
	ApplyDirectAction(context.Context, DirectActionApplyInput) (DirectActionResult, error)
	ListActivities(context.Context, string, int) ([]MemoryActivity, error)
	ListMessageUsages(context.Context, string) ([]MessageMemoryUsage, error)
	UndoActivity(context.Context, UndoActivityInput) (UndoActivityResult, error)
}

// LexicalShadowRepository is optional so the v1 Repository remains the only
// prompt/Usage authority and existing repository doubles stay compatible.
type LexicalShadowRepository interface {
	CompareLexicalShadow(context.Context, LexicalShadowInput) (LexicalShadowSummary, error)
}

// HybridShadowRepository is optional. It exposes transient authorized
// candidate content to the in-process reranker, while durable observations
// retain only IDs, revisions, ordinals, bounded status, and counts.
type HybridShadowRepository interface {
	PrepareHybridShadow(context.Context, HybridShadowPrepareInput) (HybridShadowPreparation, error)
	RecordHybridShadow(context.Context, HybridShadowRecordInput) (HybridShadowSummary, error)
}

type Settings struct {
	Enabled                bool   `json:"enabled"`
	SearchEnabled          bool   `json:"searchEnabled"`
	AutoRecordEnabled      bool   `json:"autoRecordEnabled"`
	SensitiveMemoryEnabled bool   `json:"sensitiveMemoryEnabled"`
	L2Mode                 string `json:"l2Mode"`
	L3Mode                 string `json:"l3Mode"`
}

type SettingsPatch struct {
	Enabled                *bool   `json:"enabled"`
	SearchEnabled          *bool   `json:"searchEnabled"`
	AutoRecordEnabled      *bool   `json:"autoRecordEnabled"`
	SensitiveMemoryEnabled *bool   `json:"sensitiveMemoryEnabled"`
	L2Mode                 *string `json:"l2Mode"`
	L3Mode                 *string `json:"l3Mode"`
}

type Memory struct {
	ID                   string
	UserID               string
	Type                 string
	Content              string
	NormalizedContent    string
	Importance           int
	Tags                 []string
	Source               string
	SourceConversationID string
	SourceMessageID      string
	Enabled              bool
	LastUsedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time
	Revision             int64
	ScopeType            string
}

type CreateInput struct {
	ID                   string
	Type                 string
	Content              string
	NormalizedContent    string
	Importance           int
	Tags                 []string
	Source               string
	SourceConversationID string
	SourceMessageID      string
	Enabled              bool
}

type UpdateInput struct {
	Type              string
	Content           string
	NormalizedContent string
	Importance        int
	Tags              []string
	Enabled           bool
}

type Candidate struct {
	Type       string   `json:"type"`
	Content    string   `json:"content"`
	Importance int      `json:"importance"`
	Tags       []string `json:"tags"`
}

type ExtractionInput struct {
	ConversationID string
	MessageID      string
	Candidates     []Candidate
}

type DirectActionHydrationInput struct {
	ConversationID     string
	SourceMessageID    string
	AssistantMessageID string
}

type DirectActionContext struct {
	ProjectID                   string
	ConversationScopeGeneration int64
	ProjectScopeGeneration      *int64
	SensitiveMemoryEnabled      bool
	Memories                    []DirectActionMemory
}

type DirectActionMemory struct {
	ID             string `json:"id"`
	Revision       int64  `json:"revision"`
	Type           string `json:"type"`
	Content        string `json:"content"`
	AuthorityKind  string `json:"authorityKind"`
	ScopeType      string `json:"scopeType"`
	ProjectID      string `json:"projectId"`
	ConversationID string `json:"conversationId"`
	Sensitivity    string `json:"sensitivity"`
}

type DirectActionTarget struct {
	MemoryID         string `json:"memoryId"`
	ExpectedRevision int64  `json:"expectedRevision"`
}

type DirectActionExecution struct {
	ConversationID      string
	SourceMessageID     string
	AssistantMessageID  string
	RequestedAction     string
	Candidate           *Candidate
	Sensitivity         string
	ScopeType           string
	Confidence          float64
	Targets             []DirectActionTarget
	RequestHash         string
	PreflightStatus     string
	PreflightResultCode string
}

type DirectActionApplyInput struct {
	ActionID            string
	ActivityID          string
	MemoryID            string
	EventID             string
	JobID               string
	TombstoneID         string
	ManifestID          string
	ConversationID      string
	SourceMessageID     string
	AssistantMessageID  string
	SchemaMajor         int
	RequestedAction     string
	MemoryType          string
	Content             string
	NormalizedContent   string
	CandidateHash       string
	Importance          int
	Tags                []string
	Sensitivity         string
	ScopeType           string
	Confidence          float64
	Targets             []DirectActionTarget
	PreflightStatus     string
	PreflightResultCode string
}

type DirectActionResult struct {
	ActionID       string `json:"actionId"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	ResultCode     string `json:"resultCode"`
	MemoryID       string `json:"memoryId,omitempty"`
	MemoryRevision int64  `json:"memoryRevision,omitempty"`
	ScopeType      string `json:"scopeType"`
	ActivityID     string `json:"activityId,omitempty"`
}

type MemoryActivity struct {
	ID                 string
	AssistantMessageID string
	Ordinal            int
	SubjectType        string
	SubjectID          string
	SubjectRevision    *int64
	Action             string
	Status             string
	ReasonCode         string
	UndoKind           string
	UndoStatus         string
	SourceKind         string
	ScopeType          string
	MemoryType         string
	MemoryContent      string
	MemoryRevision     *int64
	MemoryDeleted      bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func IsStateConflict(err error) bool {
	var validationError ValidationError
	if !errors.As(err, &validationError) {
		return false
	}
	switch validationError.Code {
	case "MEMORY_GOVERNANCE_REVISION_STALE",
		"MEMORY_GOVERNANCE_SCOPE_STALE",
		"MEMORY_GOVERNANCE_REVIEW_STALE",
		"MEMORY_GOVERNANCE_REPLAY_CONFLICT":
		return true
	default:
		return false
	}
}

type MessageMemoryUsage struct {
	AssistantMessageID string
	Ordinal            int
	MemoryID           string
	MemoryRevision     int64
	ScopeType          string
	MemoryType         string
	MemoryContent      string
	MemoryDeleted      bool
	CreatedAt          time.Time
}

type LexicalShadowInput struct {
	ObservationID      string
	ConversationID     string
	AssistantMessageID string
	QueryHash          string
	QueryText          string
	Baseline           []LexicalShadowBaseline
	LexicalLimit       int
}

type LexicalShadowBaseline struct {
	MemoryID  string `json:"memoryId"`
	Revision  int64  `json:"revision"`
	ScopeType string `json:"scopeType"`
}

type LexicalShadowSummary struct {
	ProfileID      string `json:"profile"`
	Status         string `json:"status"`
	ResultCode     string `json:"resultCode"`
	BaselineCount  int    `json:"baselineCount"`
	ExactCount     int    `json:"exactCount"`
	BM25Count      int    `json:"bm25Count"`
	LexicalCount   int    `json:"lexicalCount"`
	OverlapCount   int    `json:"overlapCount"`
	DurationMillis int    `json:"durationMillis"`
}

type HybridShadowPrepareInput struct {
	ObservationID       string
	ConversationID      string
	AssistantMessageID  string
	QueryHash           string
	QueryText           string
	Baseline            []LexicalShadowBaseline
	QueryEmbedding      []float32
	QueryEmbeddingState string
}

type HybridShadowCandidate struct {
	MemoryID  string `json:"memoryId"`
	Revision  int64  `json:"revision"`
	ScopeType string `json:"scopeType"`
	Content   string `json:"content"`
}

type HybridShadowRankedItem struct {
	MemoryID  string `json:"memoryId"`
	Revision  int64  `json:"revision"`
	ScopeType string `json:"scopeType"`
}

type HybridShadowPreparation struct {
	Summary    HybridShadowSummary
	Replayed   bool
	Candidates []HybridShadowCandidate
}

type HybridShadowRecordInput struct {
	ObservationID        string
	AssistantMessageID   string
	RerankStatus         string
	FallbackCode         string
	Reranked             []HybridShadowRankedItem
	Final                []HybridShadowRankedItem
	EstimatedTokens      int
	TargetTokensExceeded bool
	DurationMillis       int
}

type HybridShadowSummary struct {
	ProfileID            string `json:"profile"`
	Status               string `json:"status"`
	ResultCode           string `json:"resultCode"`
	FallbackCode         string `json:"fallbackCode"`
	BaselineCount        int    `json:"baselineCount"`
	ExactCount           int    `json:"exactCount"`
	BM25Count            int    `json:"bm25Count"`
	VectorCount          int    `json:"vectorCount"`
	RRFCount             int    `json:"rrfCount"`
	RerankCount          int    `json:"rerankCount"`
	FinalCount           int    `json:"finalCount"`
	OverlapCount         int    `json:"overlapCount"`
	EstimatedTokens      int    `json:"estimatedTokens"`
	TargetTokensExceeded bool   `json:"targetTokensExceeded"`
	DurationMillis       int    `json:"durationMillis"`
}

type UndoActivityInput struct {
	ActivityID       string
	ExpectedRevision int64
	EventID          string
	JobID            string
	TombstoneID      string
	ManifestID       string
}

type UndoActivityResult struct {
	Status         string `json:"status"`
	ResultCode     string `json:"resultCode"`
	MemoryID       string `json:"memoryId"`
	MemoryRevision int64  `json:"memoryRevision,omitempty"`
}

type ValidationError struct {
	Code    string
	Message string
}

func (e ValidationError) Error() string { return e.Message }
