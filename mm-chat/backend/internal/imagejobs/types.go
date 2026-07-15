package imagejobs

import (
	"context"
	"io"

	"neo-chat/mm-chat/backend/internal/jobartifacts"
)

type ModelRef struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

type GenerateRequest struct {
	ModelRef ModelRef `json:"modelRef"`
	Prompt   string   `json:"prompt"`
	Size     string   `json:"size,omitempty"`
	Count    int      `json:"count,omitempty"`
}

type GeneratedImage struct {
	FileID      string `json:"fileId,omitempty"`
	Purpose     string `json:"purpose,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

type GenerateResponse struct {
	Images  []GeneratedImage `json:"images"`
	Message string           `json:"message"`
}

type GeneratedImageResult struct {
	JobID       string
	Filename    string
	ContentType string
	Size        int64
	Body        io.Reader
}

type GenerateResult struct {
	Images  []GeneratedImageResult
	Message string
}

type Executor interface {
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
}

type ArtifactStore interface {
	Store(context.Context, jobartifacts.StoreInput) (jobartifacts.Artifact, error)
}
