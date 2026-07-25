package ragevalcapture

import (
	"context"
	"encoding/json"
	"time"

	"neo-chat/mm-chat/backend/internal/rageval"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	PreflightSchemaVersion = "neo-chat.rag-promotion-preflight.v2"
	CaptureVersion         = "neo-chat.rag-promotion-capture.v6"
	HoldoutSealVersion     = "neo-chat.rag-promotion-holdout-seal.v1"
)

type CurationQueue struct {
	SchemaVersion        string                 `json:"schemaVersion"`
	Synthetic            bool                   `json:"synthetic"`
	ReviewState          string                 `json:"reviewState"`
	PromotionEligible    bool                   `json:"promotionEligible"`
	SourceManifest       CurationSourceManifest `json:"sourceManifest"`
	ImportReceiptSHA256  string                 `json:"importReceiptSha256"`
	CollectionBinding    CurationCollection     `json:"collectionBinding"`
	PromotionGoldenDraft CurationGoldenDraft    `json:"promotionGoldenDraft"`
	Counts               json.RawMessage        `json:"counts"`
	Cases                []CurationCase         `json:"cases"`
}

type CurationSourceManifest struct {
	SchemaVersion string `json:"schemaVersion"`
	SHA256        string `json:"sha256"`
}

type CurationCollection struct {
	Alias        string `json:"alias"`
	CollectionID string `json:"collectionId"`
}

type CurationGoldenDraft struct {
	SchemaVersion string `json:"schemaVersion"`
	ID            string `json:"id"`
	SHA256        string `json:"sha256"`
}

type CurationCase struct {
	PromotionCase  rageval.PromotionGoldenCase `json:"promotionCase"`
	SourceBinding  CurationSourceBinding       `json:"sourceBinding"`
	ExpectedLabel  string                      `json:"expectedLabel"`
	ExpectedAnswer string                      `json:"expectedAnswer"`
}

type CurationSourceBinding struct {
	SourceID     string `json:"sourceId"`
	Anchor       string `json:"anchor"`
	Section      string `json:"section"`
	Filename     string `json:"filename"`
	SourceSHA256 string `json:"sourceSha256"`
	FileID       string `json:"fileId"`
	DocumentID   string `json:"documentId"`
	FormatLane   string `json:"formatLane"`
	Language     string `json:"language"`
}

type HumanReviewReceipt struct {
	SchemaVersion                 string `json:"schemaVersion"`
	Decision                      string `json:"decision"`
	Attestation                   string `json:"attestation"`
	CaseCount                     int    `json:"caseCount"`
	ReviewerID                    string `json:"reviewerId"`
	ReviewedAt                    string `json:"reviewedAt"`
	FrozenAt                      string `json:"frozenAt"`
	HoldoutRunID                  string `json:"holdoutRunId"`
	SourceDraftSHA256             string `json:"sourceDraftSha256"`
	CurationQueueSHA256           string `json:"curationQueueSha256"`
	CurationApprovalReceiptSHA256 string `json:"curationApprovalReceiptSha256"`
	FrozenGoldenRawSHA256         string `json:"frozenGoldenRawSha256"`
	FrozenContentSHA256           string `json:"frozenContentSha256"`
	CaseReviewState               string `json:"caseReviewState"`
	PromotionGateEvaluated        bool   `json:"promotionGateEvaluated"`
	PromotionEligible             bool   `json:"promotionEligible"`
	ActivationExecuted            bool   `json:"activationExecuted"`
}

type SourceImportReceipt struct {
	SchemaVersion  string                 `json:"schemaVersion"`
	CollectionID   string                 `json:"collectionId"`
	ManifestSHA256 string                 `json:"manifestSha256"`
	Documents      []SourceImportDocument `json:"documents"`
}

type SourceImportDocument struct {
	SourceID      string `json:"sourceId"`
	Filename      string `json:"filename"`
	SHA256        string `json:"sha256"`
	FileID        string `json:"fileId"`
	DocumentID    string `json:"documentId"`
	BindStatus    string `json:"bindStatus"`
	FinalStatus   string `json:"finalStatus"`
	VersionStatus string `json:"versionStatus"`
	MIMEType      string `json:"mimeType"`
	ByteSize      int64  `json:"byteSize"`
}

type GenerationStatus struct {
	HeadRevision                  int64
	CorpusProjectionRevision      int64
	ActiveGenerationID            string
	ActiveGenerationSequence      int64
	ActiveChunkProfileHash        string
	ActiveArtifactManifestHash    string
	CandidateGenerationID         string
	CandidateGenerationSequence   int64
	CandidateStatus               string
	CandidateChunkProfileHash     string
	CandidateArtifactManifestHash string
	CandidateReadiness            string
}

type CandidateReference struct {
	CollectionID      string  `json:"collection_id"`
	DocumentID        string  `json:"document_id"`
	DocumentVersionID string  `json:"document_version_id"`
	IndexGenerationID string  `json:"index_generation_id"`
	MaterializationID string  `json:"materialization_id"`
	ParentChunkID     string  `json:"parent_chunk_id"`
	ChildChunkID      string  `json:"child_chunk_id"`
	SourceSpanHash    string  `json:"source_span_hash"`
	ContentHash       string  `json:"content_hash"`
	RankScore         float64 `json:"-"`
}

type HydratedEvidence struct {
	CandidateReference
	SourceText       string
	SourceName       string
	ChildTokenCount  int
	ParentSourceText string
	ParentTokenCount int
	Locator          json.RawMessage
	ProvenanceValid  bool
	CellLineageValid bool
}

type Store interface {
	Status(context.Context) (GenerationStatus, error)
	FetchCandidates(
		context.Context,
		string,
		[]string,
		string,
		[]float32,
		int,
	) ([]CandidateReference, error)
	Hydrate(
		context.Context,
		string,
		[]string,
		[]CandidateReference,
	) ([]HydratedEvidence, error)
}

type QueryEmbedder interface {
	EmbedQuery(context.Context, string) (ragproviders.QueryEmbedding, error)
}

type Reranker interface {
	Rerank(context.Context, string, []string) ([]ragproviders.RerankResult, error)
}

type AnswerResult struct {
	Content string
	Usage   AnswerUsage
}

type AnswerUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

type Answerer interface {
	Answer(context.Context, string, string) (AnswerResult, error)
}

type CaptureRetrievalProvider struct {
	Profile  ragproviders.RetrievalProfile
	Embedder QueryEmbedder
	Reranker Reranker
}

type LoadedInputs struct {
	Golden             rageval.PromotionGoldenSet
	GoldenRawSHA256    string
	Curation           CurationQueue
	CurationRawSHA256  string
	ReviewReceipt      HumanReviewReceipt
	ReviewRawSHA256    string
	ImportReceipt      SourceImportReceipt
	ImportRawSHA256    string
	CuratedByCaseID    map[string]CurationCase
	SourceByDocumentID map[string]string
}

type CaptureInput struct {
	LoadedInputs
	Store                 Store
	CandidateProvider     CaptureRetrievalProvider
	Answerer              Answerer
	CandidateGenerationID string
	CandidateManifestHash string
	AnswerProviderID      string
	AnswerModelID         string
	Splits                []string
	CaseID                string
	MaximumCases          int
	Concurrency           int
	Clock                 func() time.Time
	NewUUID               func() string
}

type LoadedPreflightReport struct {
	Report    PreflightReport
	RawSHA256 string
}

type FrozenHoldoutInput struct {
	CaptureInput
	Development           LoadedPreflightReport
	Validation            LoadedPreflightReport
	ObservationOutputPath string
	Seal                  func(HoldoutSeal) error
}

type HoldoutSeal struct {
	SchemaVersion            string                                  `json:"schemaVersion"`
	CaptureVersion           string                                  `json:"captureVersion"`
	State                    string                                  `json:"state"`
	HoldoutRunID             string                                  `json:"holdoutRunId"`
	Ordinal                  int                                     `json:"ordinal"`
	ExecutedAt               string                                  `json:"executedAt"`
	CaptureID                string                                  `json:"captureId"`
	GoldenSetID              string                                  `json:"goldenSetId"`
	GoldenRawSHA256          string                                  `json:"goldenRawSha256"`
	GoldenContentSHA256      string                                  `json:"goldenContentSha256"`
	CurationRawSHA256        string                                  `json:"curationRawSha256"`
	HumanReviewRawSHA256     string                                  `json:"humanReviewRawSha256"`
	SourceImportRawSHA256    string                                  `json:"sourceImportRawSha256"`
	DevelopmentRawSHA256     string                                  `json:"developmentRawSha256"`
	ValidationRawSHA256      string                                  `json:"validationRawSha256"`
	CandidateGenerationID    string                                  `json:"candidateGenerationId"`
	ArtifactManifestHash     string                                  `json:"artifactManifestHash"`
	ChunkProfileHash         string                                  `json:"chunkProfileHash"`
	RetrievalProfile         PreflightRetrievalProviderConfiguration `json:"retrievalProfile"`
	AnswerProviderID         string                                  `json:"answerProviderId"`
	AnswerModelID            string                                  `json:"answerModelId"`
	GenerationHeadRevision   int64                                   `json:"generationHeadRevision"`
	CorpusProjectionRevision int64                                   `json:"corpusProjectionRevision"`
	ObservationOutputPath    string                                  `json:"observationOutputPath"`
}

type PreflightReport struct {
	SchemaVersion     string                          `json:"schemaVersion"`
	CaptureVersion    string                          `json:"captureVersion"`
	PromotionEligible bool                            `json:"promotionEligible"`
	Complete          bool                            `json:"complete"`
	CapturedAt        string                          `json:"capturedAt"`
	Inputs            PreflightInputHashes            `json:"inputs"`
	Configuration     PreflightConfiguration          `json:"configuration"`
	Holdout           PreflightHoldout                `json:"holdout"`
	Candidate         PreflightProfileCapture         `json:"candidate"`
	Slices            map[string]PreflightSliceResult `json:"slices"`
	Budgets           PreflightBudgets                `json:"budgets"`
}

type PreflightInputHashes struct {
	GoldenRawSHA256       string `json:"goldenRawSha256"`
	GoldenContentSHA256   string `json:"goldenContentSha256"`
	CurationRawSHA256     string `json:"curationRawSha256"`
	HumanReviewRawSHA256  string `json:"humanReviewRawSha256"`
	SourceImportRawSHA256 string `json:"sourceImportRawSha256"`
}

type PreflightConfiguration struct {
	Splits                   []string                                `json:"splits"`
	CaseID                   string                                  `json:"caseId,omitempty"`
	CandidateRetrieval       PreflightRetrievalProviderConfiguration `json:"candidateRetrieval"`
	AnswerProviderID         string                                  `json:"answerProviderId"`
	AnswerModelID            string                                  `json:"answerModelId"`
	ProviderMaximumAttempts  int                                     `json:"providerMaximumAttempts"`
	ProviderInitialRetryMS   int64                                   `json:"providerInitialRetryMilliseconds"`
	ProviderMaximumRetryMS   int64                                   `json:"providerMaximumRetryMilliseconds"`
	CandidateLimit           int                                     `json:"candidateLimit"`
	RerankLimit              int                                     `json:"rerankLimit"`
	FinalLimit               int                                     `json:"finalLimit"`
	MaximumContextTokens     int                                     `json:"maximumContextTokens"`
	Concurrency              int                                     `json:"concurrency"`
	ScoringPolicy            string                                  `json:"scoringPolicy"`
	GenerationHeadRevision   int64                                   `json:"generationHeadRevision"`
	CorpusProjectionRevision int64                                   `json:"corpusProjectionRevision"`
}

type PreflightRetrievalProviderConfiguration struct {
	ProfileID           ragproviders.RetrievalProfileID `json:"profileId"`
	ProviderID          string                          `json:"providerId"`
	EmbeddingModelID    string                          `json:"embeddingModelId"`
	EmbeddingDimensions int                             `json:"embeddingDimensions"`
	RerankModelID       string                          `json:"rerankModelId"`
}

type PreflightHoldout struct {
	State             string `json:"state"`
	PrecommittedRunID string `json:"precommittedRunId"`
}

type PreflightProfileCapture struct {
	CaptureID            string                          `json:"captureId"`
	ProfileRole          string                          `json:"profileRole"`
	GenerationID         string                          `json:"generationId"`
	ArtifactManifestHash string                          `json:"artifactManifestHash"`
	ChunkProfileHash     string                          `json:"chunkProfileHash"`
	Summary              rageval.PromotionProfileSummary `json:"summary"`
	Cases                []PreflightObservation          `json:"cases"`
}

type PreflightSliceResult struct {
	Cases     int                        `json:"cases"`
	Evaluated bool                       `json:"evaluated"`
	Metrics   rageval.PromotionMetrics   `json:"metrics"`
	Integrity rageval.PromotionIntegrity `json:"integrity"`
	Passed    bool                       `json:"passed"`
	Failures  []string                   `json:"failures"`
}

type PreflightBudgets struct {
	CandidateP95LatencyMilliseconds int64   `json:"candidateP95LatencyMilliseconds"`
	MaximumP95LatencyMilliseconds   int64   `json:"maximumP95LatencyMilliseconds"`
	CandidateAverageContextTokens   float64 `json:"candidateAverageContextTokens"`
	MaximumAverageContextTokens     float64 `json:"maximumAverageContextTokens"`
	LatencyPassed                   bool    `json:"latencyPassed"`
	ContextTokenCostPassed          bool    `json:"contextTokenCostPassed"`
}

type PreflightObservation struct {
	CaseID               string                         `json:"caseId"`
	RetrievedEvidenceIDs []string                       `json:"retrievedEvidenceIds"`
	FinalEvidenceIDs     []string                       `json:"finalEvidenceIds"`
	CitationEvidenceIDs  []string                       `json:"citationEvidenceIds"`
	AnswerSHA256         string                         `json:"answerSha256"`
	Answered             bool                           `json:"answered"`
	AnswerCorrectness    float64                        `json:"answerCorrectness"`
	Faithfulness         float64                        `json:"faithfulness"`
	TableExactAnswer     bool                           `json:"tableExactAnswer"`
	LatencyMilliseconds  int64                          `json:"latencyMilliseconds"`
	LatencyBreakdown     PreflightLatencyBreakdown      `json:"latencyBreakdown"`
	ContextTokens        int                            `json:"contextTokens"`
	AnswerUsage          AnswerUsage                    `json:"answerUsage"`
	Integrity            rageval.PromotionCaseIntegrity `json:"integrity"`
	Leakage              rageval.PromotionCaseLeakage   `json:"leakage"`
}

type PreflightLatencyBreakdown struct {
	EmbedQueryMilliseconds      int64 `json:"embedQueryMilliseconds"`
	FetchCandidatesMilliseconds int64 `json:"fetchCandidatesMilliseconds"`
	HydrateEvidenceMilliseconds int64 `json:"hydrateEvidenceMilliseconds"`
	RerankMilliseconds          int64 `json:"rerankMilliseconds"`
	PipelineTotalMilliseconds   int64 `json:"pipelineTotalMilliseconds"`
}
