package usermemory

import (
	"context"
	"errors"
	"time"
)

const (
	MaxMemories       = 500
	MaxContentChars   = 2000
	MaxTags           = 12
	MaxTagChars       = 40
	MaxSearchResults  = 5
	MaxExtractedItems = 5
)

var (
	ErrDatabaseRequired = errors.New("memory database is required")
	ErrMemoryNotFound   = errors.New("memory not found")
	ErrMemoryConflict   = errors.New("memory content already exists")
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

type ValidationError struct {
	Code    string
	Message string
}

func (e ValidationError) Error() string { return e.Message }
