package voicejobs

import (
	"context"
	"io"

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
	Text     string   `json:"text"`
	Provider Provider `json:"provider"`
	JobID    string   `json:"jobId,omitempty"`
	VoiceID  string   `json:"voiceId,omitempty"`
	ModelID  string   `json:"modelId,omitempty"`
}

type SynthesizeResponse struct {
	FileID      string `json:"fileId,omitempty"`
	Purpose     string `json:"purpose,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
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

type ArtifactStore interface {
	Store(context.Context, jobartifacts.StoreInput) (jobartifacts.Artifact, error)
}
