// Package memorycapture executes synthetic Memory regression cases through
// the native reader seams and assembles machine-regression observations. It
// has no reader-promotion authority.
package memorycapture

import (
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const (
	BaselineProfileID                           = "native_v1_lexical"
	CandidateProfileID                          = "native_v2_hybrid"
	FakeCandidateProfileID                      = "native_v2_hybrid_fake_protocol"
	ReaderVersion                               = "neo-chat.native-memory-reader-capture.v2"
	CloudJudgeReaderVersion                     = "neo-chat.native-memory-reader-capture.v3"
	MemoryToolRouteReaderVersion                = "neo-chat.native-memory-reader-capture.v4"
	MemoryToolFirstRoundReaderVersion           = "neo-chat.native-memory-reader-capture.v5"
	MemoryToolFirstRoundDiagnosticReaderVersion = "neo-chat.native-memory-reader-capture.v7"
	ProviderCostPolicyOwnerAuthorizedAbsoluteV1 = "owner_authorized_absolute_cap_v1"
	ProviderModeNone                            = "none"
	ProviderModeFakeProtocol                    = "fake_protocol"
	ProviderModeLiveSiliconFlow                 = "live_siliconflow"
	CaptureModeFullRegression                   = "full_regression"
	CaptureModeCalibration                      = "development_calibration"
	CaptureModeCloudJudgeDevelopment            = "development_cloud_judge"
	CaptureModeMemoryToolRouteDevelopment       = "development_memory_tool_route"
	CaptureModeMemoryToolRouteDiagnostic        = "development_memory_tool_route_diagnostic"
	CaptureModeFrozenValidation                 = "frozen_validation"
)

var (
	ErrCaptureInvalid       = errors.New("native Memory capture is invalid")
	ErrCaptureUnavailable   = errors.New("native Memory capture is unavailable")
	ErrCaptureStateConflict = errors.New("native Memory capture state conflicts")
)

// RuntimeCase contains only the server-authoritative identities needed to run
// one already-admitted synthetic regression case.
type RuntimeCase struct {
	CaseID             string
	Query              string
	UserID             string
	ConversationID     string
	AssistantMessageID string
}

// ProfileConfig is the immutable, hashed description of one reader capture.
// Input bytes and cost authority are represented by hashes rather than paths.
type ProfileConfig struct {
	SchemaVersion                         string                 `json:"schemaVersion"`
	ProfileID                             string                 `json:"profileId"`
	ReaderVersion                         string                 `json:"readerVersion"`
	FixtureRawSHA256                      string                 `json:"fixtureRawSha256"`
	CorpusRawSHA256                       string                 `json:"corpusRawSha256"`
	AuditRawSHA256                        string                 `json:"auditRawSha256"`
	ManifestRawSHA256                     string                 `json:"manifestRawSha256"`
	CostBasisSHA256                       string                 `json:"costBasisSha256"`
	ProviderMode                          string                 `json:"providerMode"`
	EmbeddingProfileID                    string                 `json:"embeddingProfileId"`
	EmbeddingModelID                      string                 `json:"embeddingModelId"`
	EmbeddingDimensions                   int                    `json:"embeddingDimensions"`
	RerankModelID                         string                 `json:"rerankModelId"`
	CandidateLimit                        int                    `json:"candidateLimit"`
	FinalLimit                            int                    `json:"finalLimit"`
	TargetTokens                          int                    `json:"targetTokens"`
	MaximumTokens                         int                    `json:"maximumTokens"`
	HardCutoffMillis                      int                    `json:"hardCutoffMillis"`
	FixtureMapping                        string                 `json:"fixtureMapping"`
	CounterfactualInject                  bool                   `json:"counterfactualInject"`
	CaptureMode                           string                 `json:"captureMode"`
	EvaluationSplit                       string                 `json:"evaluationSplit"`
	RelevancePolicyID                     string                 `json:"relevancePolicyId"`
	RelevancePolicyMode                   string                 `json:"relevancePolicyMode"`
	MemoryIntentRequired                  bool                   `json:"memoryIntentRequired"`
	MemoryIntentAnchorVersion             string                 `json:"memoryIntentAnchorVersion"`
	MemoryIntentAnchorSHA256              string                 `json:"memoryIntentAnchorSha256"`
	MinimumMemoryIntentMarginBasisPoints  int                    `json:"minimumMemoryIntentMarginBasisPoints"`
	MinimumProviderSimilarityBasisPoints  int                    `json:"minimumProviderSimilarityBasisPoints"`
	MinimumFinalRelevanceBasisPoints      int                    `json:"minimumFinalRelevanceBasisPoints"`
	CloudCandidateJudgeRequired           bool                   `json:"cloudCandidateJudgeRequired,omitempty"`
	CloudCandidateJudgeModelID            string                 `json:"cloudCandidateJudgeModelId,omitempty"`
	CloudCandidateJudgePromptVersion      string                 `json:"cloudCandidateJudgePromptVersion,omitempty"`
	CloudCandidateJudgePromptSHA256       string                 `json:"cloudCandidateJudgePromptSha256,omitempty"`
	CloudCandidateJudgeDecodingProfile    string                 `json:"cloudCandidateJudgeDecodingProfile,omitempty"`
	MemoryToolRouteRequired               bool                   `json:"memoryToolRouteRequired,omitempty"`
	MemoryToolRouteProviderID             string                 `json:"memoryToolRouteProviderId,omitempty"`
	MemoryToolRouteProviderType           string                 `json:"memoryToolRouteProviderType,omitempty"`
	MemoryToolRouteBaseURLSHA256          string                 `json:"memoryToolRouteBaseUrlSha256,omitempty"`
	MemoryToolRouteModelID                string                 `json:"memoryToolRouteModelId,omitempty"`
	MemoryToolRouteContractVersion        string                 `json:"memoryToolRouteContractVersion,omitempty"`
	MemoryToolRouteContractSHA256         string                 `json:"memoryToolRouteContractSha256,omitempty"`
	MemoryToolRouteAdapterVersion         string                 `json:"memoryToolRouteAdapterVersion,omitempty"`
	MemoryToolRouteDecodingProfile        string                 `json:"memoryToolRouteDecodingProfile,omitempty"`
	MemoryToolRouteMaximumOutputTokens    int                    `json:"memoryToolRouteMaximumOutputTokens,omitempty"`
	MemoryToolRouteTemperature            *float64               `json:"memoryToolRouteTemperature,omitempty"`
	MemoryToolRouteDisableThinking        bool                   `json:"memoryToolRouteDisableThinking,omitempty"`
	MemoryToolRouteFailureTaxonomyVersion string                 `json:"memoryToolRouteFailureTaxonomyVersion,omitempty"`
	MemoryToolRouteFailureTaxonomySHA256  string                 `json:"memoryToolRouteFailureTaxonomySha256,omitempty"`
	MemoryToolRouteDiagnosticCompleteness string                 `json:"memoryToolRouteDiagnosticCompleteness,omitempty"`
	ProviderEgressPolicy                  string                 `json:"providerEgressPolicy,omitempty"`
	ProviderCostPolicy                    string                 `json:"providerCostPolicy,omitempty"`
	CalibrationPlan                       *CalibrationPlanConfig `json:"calibrationPlan,omitempty"`
}

// CostBasis is supplied by an operator and hash-bound to observations. The
// capture never invents a nominal cost to satisfy the evaluator.
type CostBasis struct {
	SchemaVersion            string                        `json:"schemaVersion"`
	Baseline                 memoryeval.ProviderCosts      `json:"baseline"`
	Candidate                memoryeval.ProviderCosts      `json:"candidate"`
	Source                   string                        `json:"source"`
	EffectiveAt              string                        `json:"effectiveAt"`
	CloudJudgeAuthority      *CloudJudgeCostAuthority      `json:"cloudJudgeAuthority,omitempty"`
	MemoryToolRouteAuthority *MemoryToolRouteCostAuthority `json:"memoryToolRouteAuthority,omitempty"`
	ProviderCostPolicy       string                        `json:"providerCostPolicy,omitempty"`
}

type CloudJudgeCostAuthority struct {
	ModelID                          string `json:"modelId"`
	RequestCount                     int    `json:"requestCount"`
	MaximumInputTokens               uint64 `json:"maximumInputTokens"`
	MaximumOutputTokens              uint64 `json:"maximumOutputTokens"`
	InputMicrounitsPerMillionTokens  uint64 `json:"inputMicrounitsPerMillionTokens"`
	OutputMicrounitsPerMillionTokens uint64 `json:"outputMicrounitsPerMillionTokens"`
	MaximumCostMicrounits            uint64 `json:"maximumCostMicrounits"`
}

type MemoryToolRouteProfileAuthority struct {
	ProviderID    string
	ProviderType  string
	BaseURLSHA256 string
	ModelID       string
}

type MemoryToolRouteCostAuthority struct {
	ProviderID                       string `json:"providerId"`
	ProviderType                     string `json:"providerType"`
	BaseURLSHA256                    string `json:"baseUrlSha256"`
	ModelID                          string `json:"modelId"`
	RequestCount                     int    `json:"requestCount"`
	MaximumInputTokens               uint64 `json:"maximumInputTokens"`
	MaximumOutputTokens              uint64 `json:"maximumOutputTokens"`
	InputMicrounitsPerMillionTokens  uint64 `json:"inputMicrounitsPerMillionTokens"`
	OutputMicrounitsPerMillionTokens uint64 `json:"outputMicrounitsPerMillionTokens"`
	MaximumCostMicrounits            uint64 `json:"maximumCostMicrounits"`
}

// CapturedProfile is an ordered set of observations for one immutable reader
// profile before it is bound to the regression corpus header.
type CapturedProfile struct {
	Profile     memoryeval.Profile
	Costs       memoryeval.ProviderCosts
	Cases       []memoryeval.CaseObservation
	Calibration []CandidateCalibrationTrace
}

// CandidateCalibrationTrace exists only in process memory. It deliberately
// has no JSON tags and is never accepted by retained observation schemas.
type CandidateCalibrationTrace struct {
	CaseID                               string
	PreparedReady                        bool
	MemoryIntentMargin                   float64
	MemoryIntentReady                    bool
	AdmissionSimilarity                  float64
	AdmissionReady                       bool
	RerankReady                          bool
	CloudJudgeReady                      bool
	CloudJudgeInputTokenUpperBound       int
	MemoryToolRouteReady                 bool
	MemoryToolRouteUsed                  bool
	MemoryToolRouteFailureCategory       string
	MemoryToolRouteInputTokenUpperBound  int
	MemoryToolRouteOutputTokenUpperBound int
	AbstentionCode                       string
	ResultCode                           string
	FullObservation                      memoryeval.CaseObservation
	FinalRelevanceScores                 []float64
}

type clock func() time.Time
