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
	DirectActionSchemaMajor = 1
)

var (
	ErrDatabaseRequired         = errors.New("memory database is required")
	ErrMemoryNotFound           = errors.New("memory not found")
	ErrMemoryConflict           = errors.New("memory content already exists")
	ErrActionRepositoryRequired = errors.New("memory action repository is required")
	ErrActivityNotFound         = errors.New("memory activity not found")
	ErrActivityUndoUnavailable  = errors.New("memory activity undo is unavailable")
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

type Settings struct {
	Enabled           bool `json:"enabled"`
	SearchEnabled     bool `json:"searchEnabled"`
	AutoRecordEnabled bool `json:"autoRecordEnabled"`
}

type SettingsPatch struct {
	Enabled           *bool `json:"enabled"`
	SearchEnabled     *bool `json:"searchEnabled"`
	AutoRecordEnabled *bool `json:"autoRecordEnabled"`
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
	MemoryType         string
	MemoryContent      string
	MemoryRevision     *int64
	MemoryDeleted      bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
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
