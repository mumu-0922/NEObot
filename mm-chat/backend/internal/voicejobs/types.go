package voicejobs

import (
	"context"
	"io"
	"time"

	"neo-chat/mm-chat/backend/internal/jobartifacts"
)

type Provider string

const (
	ProviderDefault    Provider = "default"
	ProviderElevenLabs Provider = "elevenlabs"
	ProviderMimo       Provider = "mimo"
	ProviderModel      Provider = "model"
)

type TranscribeRequest struct {
	Provider         Provider
	ModelID          string
	Language         string
	AudioFilename    string
	AudioContentType string
	AudioSize        int64
	Audio            io.Reader
}

type TranscribeResponse struct {
	Text string `json:"text"`
}

type SynthesizeRequest struct {
	Text      string   `json:"text"`
	Provider  Provider `json:"provider"`
	MessageID string   `json:"messageId,omitempty"`
	JobID     string   `json:"jobId,omitempty"`
	VoiceID   string   `json:"voiceId,omitempty"`
	ModelID   string   `json:"modelId,omitempty"`
}

type SynthesizeResponse struct {
	FileID      string `json:"fileId,omitempty"`
	Purpose     string `json:"purpose,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Cached      bool   `json:"cached"`
}

type SynthesizeResult struct {
	JobID       string
	Filename    string
	ContentType string
	Size        int64
	Body        io.Reader
}

type Executor interface {
	Transcribe(context.Context, TranscribeRequest) (TranscribeResponse, error)
	Synthesize(context.Context, SynthesizeRequest) (SynthesizeResult, error)
}

type SynthesisExecution struct {
	Executor   Executor
	ProviderID string
	ModelID    string
	VoiceID    string
}

type SynthesisExecutorResolver interface {
	ResolveSynthesisExecutor(context.Context) (SynthesisExecution, error)
}

type ArtifactStore interface {
	Store(context.Context, jobartifacts.StoreInput) (jobartifacts.Artifact, error)
}

type ArtifactDeleter interface {
	Delete(context.Context, string) error
}

type SynthesisSource struct {
	MessageID string
	Text      string
	UpdatedAt time.Time
}

type SynthesisCacheKey struct {
	MessageID       string
	TextSHA256      string
	SourceUpdatedAt time.Time
	ProviderID      string
	ModelID         string
	VoiceID         string
}

type CachedSynthesis struct {
	FileID      string
	ContentType string
	Size        int64
}

type CommitCachedSynthesisInput struct {
	ID          string
	Key         SynthesisCacheKey
	FileID      string
	ContentType string
	Size        int64
	AccessedAt  time.Time
	MaxBytes    int64
}

type ClaimedArtifactCleanup struct {
	ID     string
	UserID string
	FileID string
}

type SynthesisCacheRepository interface {
	ResolveSynthesisSource(context.Context, string) (SynthesisSource, error)
	GetCachedSynthesis(context.Context, SynthesisCacheKey, time.Time) (CachedSynthesis, bool, error)
	CommitCachedSynthesis(context.Context, CommitCachedSynthesisInput) error
	QueueArtifactCleanup(context.Context, string, string) error
	PrepareArtifactCleanup(context.Context, time.Time, int64, int) error
	ClaimArtifactCleanup(context.Context, string, time.Time, int) ([]ClaimedArtifactCleanup, error)
	CompleteArtifactCleanup(context.Context, string, string) error
	ReleaseArtifactCleanup(context.Context, string, string) error
}
