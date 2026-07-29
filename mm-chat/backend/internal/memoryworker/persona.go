package memoryworker

import (
	"context"
	"encoding/json"
	"time"
)

const (
	PersonaProfileID          = "memory_l3_persona_v1"
	PersonaSynthesisProfileID = "memory_l3_persona_synthesis_v1"
	PersonaRetrievalProfileID = "memory_l3_persona_hybrid_bge_m3_rrf60_v1"
)

type PersonaJob struct {
	JobID                   string
	Stage                   string
	UserID                  string
	TargetPersonaID         string
	VisibilityEpoch         int64
	Generation              int64
	ProfileID               string
	SourceWatermark         string
	AttemptCount            int
	MaxAttempts             int
	ProviderRecordID        string
	ProviderConfigUpdatedAt time.Time
	ModelID                 string
	WorkerID                string
	LeaseToken              string
	LeaseExpiresAt          time.Time
}

type PersonaCapture struct {
	UserID                  string
	VisibilityEpoch         int64
	Generation              int64
	ProfileID               string
	SourceWatermark         string
	SensitiveMemoryEnabled  bool
	Memories                []PersonaMemory
	ProviderRecordID        string
	ProviderID              string
	ProviderLabel           string
	EncryptedSecretRef      string
	ProviderConfig          json.RawMessage
	ProviderConfigUpdatedAt time.Time
	ModelID                 string
}

type PersonaMemory struct {
	ID          string    `json:"id"`
	Revision    int64     `json:"revision"`
	Type        string    `json:"type"`
	Content     string    `json:"content"`
	ContentHash string    `json:"contentHash"`
	Sensitivity string    `json:"sensitivity"`
	Importance  int       `json:"importance"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type PersonaProposal struct {
	Content         string   `json:"content"`
	MemberMemoryIDs []string `json:"memberMemoryIds"`
}

type PersonaEmbeddingJob struct {
	JobID                   string
	UserID                  string
	PersonaID               string
	PersonaRevision         int64
	ContentHash             string
	SourceWatermark         string
	VisibilityEpoch         int64
	Generation              int64
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

type PersonaEmbeddingCapture struct {
	UserID                  string
	PersonaID               string
	Content                 string
	ContentHash             string
	PersonaRevision         int64
	SourceWatermark         string
	VisibilityEpoch         int64
	Generation              int64
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

type PersonaRepository interface {
	ClaimPersona(
		context.Context,
		string,
		string,
		time.Duration,
		bool,
	) (PersonaJob, bool, error)
	HydratePersonaRefresh(context.Context, PersonaJob) (PersonaCapture, error)
	CompletePersonaRefresh(context.Context, PersonaJob, *PersonaProposal) error
	CompletePersonaPurge(context.Context, PersonaJob) error
	RetryPersona(
		context.Context,
		PersonaJob,
		string,
		time.Time,
		bool,
	) (string, error)
	ClaimPersonaEmbedding(
		context.Context,
		string,
		string,
		time.Duration,
	) (PersonaEmbeddingJob, bool, error)
	HydratePersonaEmbedding(
		context.Context,
		PersonaEmbeddingJob,
	) (PersonaEmbeddingCapture, error)
	CompletePersonaEmbedding(
		context.Context,
		PersonaEmbeddingJob,
		[]float32,
	) error
	RetryPersonaEmbedding(
		context.Context,
		PersonaEmbeddingJob,
		string,
		time.Time,
		bool,
	) (string, error)
}
