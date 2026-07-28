package memoryworker

import (
	"context"
	"encoding/json"
	"time"
)

const (
	SceneSynthesisProfileID = "memory_l2_scene_synthesis_v1"
	SceneRetrievalProfileID = "memory_l2_scene_hybrid_bge_m3_rrf60_v1"
)

type SceneJob struct {
	JobID                   string
	Stage                   string
	UserID                  string
	ScopeType               string
	ProjectID               string
	TargetSceneID           string
	ScopeGeneration         int64
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

type SceneCapture struct {
	UserID                  string
	ScopeType               string
	ProjectID               string
	ScopeGeneration         int64
	VisibilityEpoch         int64
	Generation              int64
	ProfileID               string
	SourceWatermark         string
	SensitiveMemoryEnabled  bool
	Memories                []SceneMemory
	ProviderRecordID        string
	ProviderID              string
	ProviderLabel           string
	EncryptedSecretRef      string
	ProviderConfig          json.RawMessage
	ProviderConfigUpdatedAt time.Time
	ModelID                 string
}

type SceneMemory struct {
	ID          string    `json:"id"`
	Revision    int64     `json:"revision"`
	Type        string    `json:"type"`
	Content     string    `json:"content"`
	ContentHash string    `json:"contentHash"`
	Sensitivity string    `json:"sensitivity"`
	Importance  int       `json:"importance"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type SceneProposal struct {
	SceneID         string   `json:"sceneId"`
	TopicKey        string   `json:"topicKey"`
	Content         string   `json:"content"`
	ContentHash     string   `json:"contentHash"`
	Sensitivity     string   `json:"sensitivity"`
	MemberMemoryIDs []string `json:"memberMemoryIds"`
}

type SceneEmbeddingJob struct {
	JobID                   string
	UserID                  string
	SceneID                 string
	SceneRevision           int64
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

type SceneEmbeddingCapture struct {
	UserID                  string
	SceneID                 string
	Content                 string
	ContentHash             string
	SceneRevision           int64
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

type SceneRepository interface {
	ClaimScene(
		context.Context,
		string,
		string,
		time.Duration,
		bool,
	) (SceneJob, bool, error)
	HydrateSceneRefresh(context.Context, SceneJob) (SceneCapture, error)
	CompleteSceneRefresh(context.Context, SceneJob, []SceneProposal) error
	CompleteScenePurge(context.Context, SceneJob) error
	RetryScene(
		context.Context,
		SceneJob,
		string,
		time.Time,
		bool,
	) (string, error)
	ClaimSceneEmbedding(
		context.Context,
		string,
		string,
		time.Duration,
	) (SceneEmbeddingJob, bool, error)
	HydrateSceneEmbedding(
		context.Context,
		SceneEmbeddingJob,
	) (SceneEmbeddingCapture, error)
	CompleteSceneEmbedding(
		context.Context,
		SceneEmbeddingJob,
		[]float32,
	) error
	RetrySceneEmbedding(
		context.Context,
		SceneEmbeddingJob,
		string,
		time.Time,
		bool,
	) (string, error)
}
