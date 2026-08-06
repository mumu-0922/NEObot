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
	BaselineProfileID                            = "native_v1_lexical"
	CandidateProfileID                           = "native_v2_hybrid"
	FakeCandidateProfileID                       = "native_v2_hybrid_fake_protocol"
	ReaderVersion                                = "neo-chat.native-memory-reader-capture.v2"
	CloudJudgeReaderVersion                      = "neo-chat.native-memory-reader-capture.v3"
	MemoryToolRouteReaderVersion                 = "neo-chat.native-memory-reader-capture.v4"
	MemoryToolFirstRoundReaderVersion            = "neo-chat.native-memory-reader-capture.v5"
	MemoryToolFirstRoundDiagnosticReaderVersion  = "neo-chat.native-memory-reader-capture.v7"
	ConfiguredCandidateJudgeReaderVersion        = "neo-chat.native-memory-reader-capture.v8"
	FixedMemoryJudgeReaderVersion                = "neo-chat.native-memory-reader-capture.v9"
	AccuracyFirstMemoryJudgeReaderVersion        = "neo-chat.native-memory-reader-capture.v10"
	JudgeFailureDiagnosticReaderVersion          = "neo-chat.native-memory-reader-capture.v11"
	TransportStableMemoryJudgeReaderVersion      = "neo-chat.native-memory-reader-capture.v12"
	ProductionMemoryJudgeValidationReaderVersion = "neo-chat.native-memory-reader-capture.v13"
	NegativePolicyGuardMemoryJudgeReaderVersion  = "neo-chat.native-memory-reader-capture.v14"
	ProviderCostPolicyOwnerAuthorizedAbsoluteV1  = "owner_authorized_absolute_cap_v1"
	AccuracyFirstExecutionSequenceV1             = "bge_query_admission_bge_rerank_luna_judge_record_serial_v1"
	AccuracyFirstRetryPolicyV1                   = "transient_408_429_5xx_transport_read_once_v1"
	TransportStableExecutionSequenceV2           = "bge_query_admission_bge_rerank_luna_judge_record_serial_judge_retry_v2"
	TransportStableRetryPolicyV2                 = "transient_408_429_5xx_transport_read_judge_twice_v2"
	ProductionValidationExecutionSequenceV1      = "production_bge_m3_rerank_fixed_luna_judge_record_serial_v1"
	AccuracyFirstCooldownWallClockV1             = "wall_clock_v1"
	AccuracyFirstCooldownVirtualProtocolV1       = "virtual_protocol_v1"
	ProviderModeNone                             = "none"
	ProviderModeFakeProtocol                     = "fake_protocol"
	ProviderModeLiveSiliconFlow                  = "live_siliconflow"
	CaptureModeFullRegression                    = "full_regression"
	CaptureModeCalibration                       = "development_calibration"
	CaptureModeCloudJudgeDevelopment             = "development_cloud_judge"
	CaptureModeMemoryToolRouteDevelopment        = "development_memory_tool_route"
	CaptureModeMemoryToolRouteDiagnostic         = "development_memory_tool_route_diagnostic"
	CaptureModeConfiguredCandidateJudge          = "development_configured_candidate_judge"
	CaptureModeFixedMemoryJudge                  = "development_fixed_memory_judge"
	CaptureModeAccuracyFirstMemoryJudge          = "development_fixed_memory_judge_accuracy"
	CaptureModeJudgeFailureDiagnostic            = "development_fixed_memory_judge_failure_diagnostic"
	CaptureModeTransportStableMemoryJudge        = "development_fixed_memory_judge_transport_stable"
	CaptureModeNegativePolicyGuardMemoryJudge    = "development_fixed_memory_judge_negative_guard"
	CaptureModeFrozenValidation                  = "frozen_validation"
	CaptureModeProductionMemoryJudgeValidation   = "production_fixed_memory_judge_validation"
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
	SchemaVersion                         string                        `json:"schemaVersion"`
	ProfileID                             string                        `json:"profileId"`
	ReaderVersion                         string                        `json:"readerVersion"`
	FixtureRawSHA256                      string                        `json:"fixtureRawSha256"`
	CorpusRawSHA256                       string                        `json:"corpusRawSha256"`
	AuditRawSHA256                        string                        `json:"auditRawSha256"`
	ManifestRawSHA256                     string                        `json:"manifestRawSha256"`
	CostBasisSHA256                       string                        `json:"costBasisSha256"`
	ProviderMode                          string                        `json:"providerMode"`
	EmbeddingProfileID                    string                        `json:"embeddingProfileId"`
	EmbeddingModelID                      string                        `json:"embeddingModelId"`
	EmbeddingDimensions                   int                           `json:"embeddingDimensions"`
	RerankModelID                         string                        `json:"rerankModelId"`
	CandidateLimit                        int                           `json:"candidateLimit"`
	FinalLimit                            int                           `json:"finalLimit"`
	TargetTokens                          int                           `json:"targetTokens"`
	MaximumTokens                         int                           `json:"maximumTokens"`
	HardCutoffMillis                      int                           `json:"hardCutoffMillis"`
	EvaluationCriteriaVersion             string                        `json:"evaluationCriteriaVersion,omitempty"`
	MaximumP95LatencyMillis               int                           `json:"maximumP95LatencyMillis,omitempty"`
	MaximumP99LatencyMillis               int                           `json:"maximumP99LatencyMillis,omitempty"`
	FixtureMapping                        string                        `json:"fixtureMapping"`
	CounterfactualInject                  bool                          `json:"counterfactualInject"`
	CaptureMode                           string                        `json:"captureMode"`
	EvaluationSplit                       string                        `json:"evaluationSplit"`
	RelevancePolicyID                     string                        `json:"relevancePolicyId"`
	RelevancePolicyMode                   string                        `json:"relevancePolicyMode"`
	MemoryIntentRequired                  bool                          `json:"memoryIntentRequired"`
	MemoryIntentAnchorVersion             string                        `json:"memoryIntentAnchorVersion"`
	MemoryIntentAnchorSHA256              string                        `json:"memoryIntentAnchorSha256"`
	MinimumMemoryIntentMarginBasisPoints  int                           `json:"minimumMemoryIntentMarginBasisPoints"`
	MinimumProviderSimilarityBasisPoints  int                           `json:"minimumProviderSimilarityBasisPoints"`
	MinimumFinalRelevanceBasisPoints      int                           `json:"minimumFinalRelevanceBasisPoints"`
	CloudCandidateJudgeRequired           bool                          `json:"cloudCandidateJudgeRequired,omitempty"`
	CloudCandidateJudgeModelID            string                        `json:"cloudCandidateJudgeModelId,omitempty"`
	CloudCandidateJudgePromptVersion      string                        `json:"cloudCandidateJudgePromptVersion,omitempty"`
	CloudCandidateJudgePromptSHA256       string                        `json:"cloudCandidateJudgePromptSha256,omitempty"`
	CloudCandidateJudgeDecodingProfile    string                        `json:"cloudCandidateJudgeDecodingProfile,omitempty"`
	ConfiguredCandidateJudgeProviderID    string                        `json:"configuredCandidateJudgeProviderId,omitempty"`
	ConfiguredCandidateJudgeProviderType  string                        `json:"configuredCandidateJudgeProviderType,omitempty"`
	ConfiguredCandidateJudgeBaseURLSHA256 string                        `json:"configuredCandidateJudgeBaseUrlSha256,omitempty"`
	ConfiguredCandidateJudgeAdapter       string                        `json:"configuredCandidateJudgeAdapter,omitempty"`
	MemoryToolRouteRequired               bool                          `json:"memoryToolRouteRequired,omitempty"`
	MemoryToolRouteProviderID             string                        `json:"memoryToolRouteProviderId,omitempty"`
	MemoryToolRouteProviderType           string                        `json:"memoryToolRouteProviderType,omitempty"`
	MemoryToolRouteBaseURLSHA256          string                        `json:"memoryToolRouteBaseUrlSha256,omitempty"`
	MemoryToolRouteModelID                string                        `json:"memoryToolRouteModelId,omitempty"`
	MemoryToolRouteContractVersion        string                        `json:"memoryToolRouteContractVersion,omitempty"`
	MemoryToolRouteContractSHA256         string                        `json:"memoryToolRouteContractSha256,omitempty"`
	MemoryToolRouteAdapterVersion         string                        `json:"memoryToolRouteAdapterVersion,omitempty"`
	MemoryToolRouteDecodingProfile        string                        `json:"memoryToolRouteDecodingProfile,omitempty"`
	MemoryToolRouteMaximumOutputTokens    int                           `json:"memoryToolRouteMaximumOutputTokens,omitempty"`
	MemoryToolRouteTemperature            *float64                      `json:"memoryToolRouteTemperature,omitempty"`
	MemoryToolRouteDisableThinking        bool                          `json:"memoryToolRouteDisableThinking,omitempty"`
	MemoryToolRouteFailureTaxonomyVersion string                        `json:"memoryToolRouteFailureTaxonomyVersion,omitempty"`
	MemoryToolRouteFailureTaxonomySHA256  string                        `json:"memoryToolRouteFailureTaxonomySha256,omitempty"`
	MemoryToolRouteDiagnosticCompleteness string                        `json:"memoryToolRouteDiagnosticCompleteness,omitempty"`
	CandidateJudgeFailureTaxonomyVersion  string                        `json:"candidateJudgeFailureTaxonomyVersion,omitempty"`
	CandidateJudgeFailureTaxonomySHA256   string                        `json:"candidateJudgeFailureTaxonomySha256,omitempty"`
	CandidateJudgeDiagnosticCompleteness  string                        `json:"candidateJudgeDiagnosticCompleteness,omitempty"`
	ProviderEgressPolicy                  string                        `json:"providerEgressPolicy,omitempty"`
	ProviderCostPolicy                    string                        `json:"providerCostPolicy,omitempty"`
	CalibrationPlan                       *CalibrationPlanConfig        `json:"calibrationPlan,omitempty"`
	AccuracyFirstExecutionPolicy          *AccuracyFirstExecutionPolicy `json:"accuracyFirstExecutionPolicy,omitempty"`
	ValidationCaseOrderSHA256             string                        `json:"validationCaseOrderSha256,omitempty"`
	EvaluationCriteriaSHA256              string                        `json:"evaluationCriteriaSha256,omitempty"`
	ProductionRelevancePolicySHA256       string                        `json:"productionRelevancePolicySha256,omitempty"`
	MemoryReadIntentPolicyVersion         string                        `json:"memoryReadIntentPolicyVersion,omitempty"`
	MemoryReadIntentPolicySHA256          string                        `json:"memoryReadIntentPolicySha256,omitempty"`
	NegativePolicyQueryGuardRequired      bool                          `json:"negativePolicyQueryGuardRequired,omitempty"`
	NegativePolicyQueryGuardVersion       string                        `json:"negativePolicyQueryGuardVersion,omitempty"`
	NegativePolicyQueryGuardSHA256        string                        `json:"negativePolicyQueryGuardSha256,omitempty"`
	RelevancePolicyDescriptorSHA256       string                        `json:"relevancePolicyDescriptorSha256,omitempty"`
}

// AccuracyFirstExecutionPolicy is hash-bound by each accuracy-first schema. It
// makes the absence of elapsed-time cutoffs, strict Provider serialization,
// cooldown, and bounded retry behavior reviewable without changing historical
// profiles.
type AccuracyFirstExecutionPolicy struct {
	SequenceVersion                   string `json:"sequenceVersion"`
	GlobalProviderRequestConcurrency  int    `json:"globalProviderRequestConcurrency"`
	ApplicationDeadlineMode           string `json:"applicationDeadlineMode"`
	ProviderElapsedTimeoutMode        string `json:"providerElapsedTimeoutMode"`
	LatencyEvaluationMode             string `json:"latencyEvaluationMode"`
	InterCaseCooldownMilliseconds     int    `json:"interCaseCooldownMilliseconds"`
	InterCaseCooldownClock            string `json:"interCaseCooldownClock"`
	RetryPolicyVersion                string `json:"retryPolicyVersion"`
	MaximumRetriesPerProviderRequest  int    `json:"maximumRetriesPerProviderRequest"`
	RetryFallbackDelayMilliseconds    int    `json:"retryFallbackDelayMilliseconds"`
	MaximumJudgeRetriesPerRequest     int    `json:"maximumJudgeRetriesPerRequest,omitempty"`
	SecondJudgeRetryDelayMilliseconds int    `json:"secondJudgeRetryDelayMilliseconds,omitempty"`
}

// CostBasis is supplied by an operator and hash-bound to observations. The
// capture never invents a nominal cost to satisfy the evaluator.
type CostBasis struct {
	SchemaVersion                     string                                 `json:"schemaVersion"`
	Baseline                          memoryeval.ProviderCosts               `json:"baseline"`
	Candidate                         memoryeval.ProviderCosts               `json:"candidate"`
	Source                            string                                 `json:"source"`
	EffectiveAt                       string                                 `json:"effectiveAt"`
	CloudJudgeAuthority               *CloudJudgeCostAuthority               `json:"cloudJudgeAuthority,omitempty"`
	MemoryToolRouteAuthority          *MemoryToolRouteCostAuthority          `json:"memoryToolRouteAuthority,omitempty"`
	ConfiguredCandidateJudgeAuthority *ConfiguredCandidateJudgeCostAuthority `json:"configuredCandidateJudgeAuthority,omitempty"`
	ProviderCostPolicy                string                                 `json:"providerCostPolicy,omitempty"`
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

type ConfiguredCandidateJudgeProfileAuthority struct {
	ProviderID    string
	ProviderType  string
	BaseURLSHA256 string
	ModelID       string
}

// ConfiguredCandidateJudgeCostAuthority binds the exact configured chat
// Provider independently from the fixed SiliconFlow retrieval Provider.
type ConfiguredCandidateJudgeCostAuthority struct {
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
	Profile          memoryeval.Profile
	Costs            memoryeval.ProviderCosts
	Cases            []memoryeval.CaseObservation
	Calibration      []CandidateCalibrationTrace
	ProviderAttempts AccuracyFirstProviderTelemetry
}

// AccuracyFirstProviderTelemetry contains aggregate request counts only. It
// never retains request/response bodies, URLs, credentials, or case identity.
type AccuracyFirstProviderTelemetry struct {
	PassageEmbeddingAttempts       int `json:"passageEmbeddingAttempts"`
	PassageEmbeddingRetries        int `json:"passageEmbeddingRetries"`
	QueryEmbeddingAttempts         int `json:"queryEmbeddingAttempts"`
	QueryEmbeddingRetries          int `json:"queryEmbeddingRetries"`
	RerankAttempts                 int `json:"rerankAttempts"`
	RerankRetries                  int `json:"rerankRetries"`
	JudgeAttempts                  int `json:"judgeAttempts"`
	JudgeRetries                   int `json:"judgeRetries"`
	JudgeInputTokenUpperBound      int `json:"judgeInputTokenUpperBound"`
	JudgeRetryInputTokenUpperBound int `json:"judgeRetryInputTokenUpperBound"`
	// JudgeAttemptFailureCategoryCounts is process-local for historical
	// schema-v12 reports. The schema-v13 diagnostic report copies it into an
	// explicitly required JSON field without changing v12 bytes.
	JudgeAttemptFailureCategoryCounts map[string]int                  `json:"-"`
	InterCaseCooldownCount            int                             `json:"interCaseCooldownCount"`
	InterCaseCooldownMilliseconds     int                             `json:"interCaseCooldownMilliseconds"`
	InterCaseCooldownElapsedMillis    int64                           `json:"interCaseCooldownElapsedMilliseconds"`
	PassageEmbeddingLatency           AccuracyFirstLatencyDiagnostics `json:"passageEmbeddingLatency"`
	QueryEmbeddingLatency             AccuracyFirstLatencyDiagnostics `json:"queryEmbeddingLatency"`
	RerankLatency                     AccuracyFirstLatencyDiagnostics `json:"rerankLatency"`
	JudgeLatency                      AccuracyFirstLatencyDiagnostics `json:"judgeLatency"`
}

// AccuracyFirstLatencyDiagnostics retains aggregate request timing only.
// Retry waits and inter-case cooldown are deliberately excluded.
type AccuracyFirstLatencyDiagnostics struct {
	SampleCount                int   `json:"sampleCount"`
	TotalMilliseconds          int64 `json:"totalMilliseconds"`
	P95LatencyMilliseconds     int64 `json:"p95LatencyMilliseconds"`
	P99LatencyMilliseconds     int64 `json:"p99LatencyMilliseconds"`
	MaximumLatencyMilliseconds int64 `json:"maximumLatencyMilliseconds"`
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
	CloudJudgeFailureCategory            string
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
