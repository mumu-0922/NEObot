package rageval

const (
	PromotionGoldenSchemaVersion      = "neo-chat.rag-promotion-golden.v1"
	PromotionObservationSchemaVersion = "neo-chat.rag-promotion-observations.v1"
	PromotionGateReportSchemaVersion  = "neo-chat-rag-candidate-gate-report.v2"
	PromotionEvaluatorVersion         = "neo-chat.rag-promotion-evaluator.v2"
)

var criticalPromotionSlices = [...]string{
	"pdf",
	"text_markdown_docx",
	"pptx",
	"xlsx_table",
	"json_code",
	"chinese",
	"english",
	"short_fact",
	"cross_section",
	"exact_numeric",
}

// PromotionCriticalSlices returns a defensive copy of the frozen slice names.
func PromotionCriticalSlices() []string {
	return append([]string(nil), criticalPromotionSlices[:]...)
}

type PromotionGoldenSet struct {
	SchemaVersion string                   `json:"schemaVersion"`
	ID            string                   `json:"id"`
	Description   string                   `json:"description"`
	Lifecycle     PromotionGoldenLifecycle `json:"lifecycle"`
	Criteria      PromotionCriteria        `json:"criteria"`
	Cases         []PromotionGoldenCase    `json:"cases"`
}

type PromotionGoldenLifecycle struct {
	State               string `json:"state"`
	FrozenAt            string `json:"frozenAt,omitempty"`
	HoldoutRunID        string `json:"holdoutRunId,omitempty"`
	FrozenContentSHA256 string `json:"frozenContentSha256,omitempty"`
}

type PromotionCriteria struct {
	MaximumP95LatencyMilliseconds int64   `json:"maximumP95LatencyMilliseconds"`
	MaximumAverageContextTokens   float64 `json:"maximumAverageContextTokens"`
	// MinimumAggregateQualityImprovement remains decode-compatible with the
	// frozen v1 Golden corpus. Candidate-only v2 evaluation never reads it.
	MinimumAggregateQualityImprovement float64 `json:"minimumAggregateQualityImprovement"`
}

type PromotionGoldenCase struct {
	ID                          string          `json:"id"`
	Query                       string          `json:"query"`
	Split                       string          `json:"split"`
	Slices                      []string        `json:"slices"`
	SelectedCollectionAliases   []string        `json:"selectedCollectionAliases"`
	ExpectedRelevantEvidenceIDs []string        `json:"expectedRelevantEvidenceIds"`
	ExpectedNoAnswer            bool            `json:"expectedNoAnswer"`
	TableExactAnswerRequired    bool            `json:"tableExactAnswerRequired"`
	Review                      PromotionReview `json:"review"`
}

type PromotionReview struct {
	State      string `json:"state"`
	ReviewerID string `json:"reviewerId,omitempty"`
	ReviewedAt string `json:"reviewedAt,omitempty"`
}

type PromotionObservationSet struct {
	SchemaVersion        string                     `json:"schemaVersion"`
	GoldenSetID          string                     `json:"goldenSetId"`
	GoldenCorpusSHA256   string                     `json:"goldenCorpusSha256"`
	CapturedAt           string                     `json:"capturedAt"`
	CaptureID            string                     `json:"captureId"`
	ProfileRole          string                     `json:"profileRole"`
	GenerationID         string                     `json:"generationId"`
	ArtifactManifestHash string                     `json:"artifactManifestHash"`
	ProfileID            string                     `json:"profileId"`
	HoldoutRun           PromotionHoldoutRun        `json:"holdoutRun"`
	Cases                []PromotionCaseObservation `json:"cases"`
}

type PromotionHoldoutRun struct {
	ID         string `json:"id"`
	Ordinal    int    `json:"ordinal"`
	ExecutedAt string `json:"executedAt"`
}

type PromotionCaseObservation struct {
	CaseID               string                 `json:"caseId"`
	RetrievedEvidenceIDs []string               `json:"retrievedEvidenceIds"`
	FinalEvidenceIDs     []string               `json:"finalEvidenceIds"`
	CitationEvidenceIDs  []string               `json:"citationEvidenceIds"`
	Answered             bool                   `json:"answered"`
	AnswerCorrectness    float64                `json:"answerCorrectness"`
	Faithfulness         float64                `json:"faithfulness"`
	TableExactAnswer     bool                   `json:"tableExactAnswer"`
	LatencyMilliseconds  int64                  `json:"latencyMilliseconds"`
	ContextTokens        int                    `json:"contextTokens"`
	Integrity            PromotionCaseIntegrity `json:"integrity"`
	Leakage              PromotionCaseLeakage   `json:"leakage"`
}

type PromotionCaseIntegrity struct {
	CitationLocatorValid bool `json:"citationLocatorValid"`
	ProvenanceValid      bool `json:"provenanceValid"`
	CellLineageValid     bool `json:"cellLineageValid"`
}

type PromotionCaseLeakage struct {
	ACL                  bool `json:"acl"`
	Deletion             bool `json:"deletion"`
	Secret               bool `json:"secret"`
	UnauthorizedEvidence bool `json:"unauthorizedEvidence"`
}

type PromotionEvaluationInput struct {
	Golden                        PromotionGoldenSet
	GoldenRawSHA256               string
	Candidate                     PromotionObservationSet
	CandidateRawSHA256            string
	CandidateGenerationID         string
	CandidateArtifactManifestHash string
}

type PromotionGateReport struct {
	SchemaVersion         string                          `json:"schemaVersion"`
	CandidateGenerationID string                          `json:"candidateGenerationId"`
	ArtifactManifestHash  string                          `json:"artifactManifestHash"`
	Passed                bool                            `json:"passed"`
	Evaluation            PromotionEvaluationProvenance   `json:"evaluation"`
	Golden                PromotionGoldenSummary          `json:"golden"`
	Slices                map[string]PromotionSliceResult `json:"slices"`
	Metrics               PromotionMetrics                `json:"metrics"`
	Budgets               PromotionBudgets                `json:"budgets"`
	Integrity             PromotionIntegrity              `json:"integrity"`
	Failures              []string                        `json:"failures"`
}

type PromotionEvaluationProvenance struct {
	EvaluatorVersion            string `json:"evaluatorVersion"`
	GoldenCorpusRawSHA256       string `json:"goldenCorpusRawSha256"`
	GoldenFrozenContentSHA256   string `json:"goldenFrozenContentSha256"`
	CandidateObservationsSHA256 string `json:"candidateObservationsSha256"`
	CandidateCaptureID          string `json:"candidateCaptureId"`
	HoldoutRunID                string `json:"holdoutRunId"`
}

type PromotionGoldenSummary struct {
	CorpusID         string `json:"corpusId"`
	State            string `json:"state"`
	FrozenAt         string `json:"frozenAt"`
	TotalReviewed    int    `json:"totalReviewed"`
	DevelopmentCount int    `json:"developmentCount"`
	ValidationCount  int    `json:"validationCount"`
	HoldoutCount     int    `json:"holdoutCount"`
	HoldoutRuns      int    `json:"holdoutRuns"`
}

type PromotionSliceResult struct {
	Metrics   PromotionMetrics   `json:"metrics"`
	Integrity PromotionIntegrity `json:"integrity"`
	Cases     int                `json:"cases"`
	Passed    bool               `json:"passed"`
	Failures  []string           `json:"failures"`
}

type PromotionMetrics struct {
	RecallAt50                    float64 `json:"recallAt50"`
	FinalRecallAt10               float64 `json:"finalRecallAt10"`
	NDCGAt10                      float64 `json:"ndcgAt10"`
	MRRAt10                       float64 `json:"mrrAt10"`
	CitationCorrectness           float64 `json:"citationCorrectness"`
	CitationCompleteness          float64 `json:"citationCompleteness"`
	Faithfulness                  float64 `json:"faithfulness"`
	AnswerCorrectness             float64 `json:"answerCorrectness"`
	NoAnswerFalseAnswerRate       float64 `json:"noAnswerFalseAnswerRate"`
	TableExactAnswer              float64 `json:"tableExactAnswer"`
	ProvenanceCellLineage         float64 `json:"provenanceCellLineage"`
	ACLLeakCount                  int     `json:"aclLeakCount"`
	DeletionLeakCount             int     `json:"deletionLeakCount"`
	SecretLeakCount               int     `json:"secretLeakCount"`
	UnauthorizedEvidenceLeakCount int     `json:"unauthorizedEvidenceLeakCount"`
}

// PromotionProfileSummary exposes the exact aggregate implementation used by
// the formal promotion evaluator. Preflight callers may use it on a strict
// Development/Validation subset without manufacturing a Holdout observation.
type PromotionProfileSummary struct {
	Metrics                     PromotionMetrics            `json:"metrics"`
	QualityScore                float64                     `json:"qualityScore"`
	P95LatencyMilliseconds      int64                       `json:"p95LatencyMilliseconds"`
	AverageContextTokens        float64                     `json:"averageContextTokens"`
	QualityScoreByCriticalSlice map[string]float64          `json:"qualityScoreByCriticalSlice"`
	MetricsByCriticalSlice      map[string]PromotionMetrics `json:"metricsByCriticalSlice"`
}

type PromotionBudgets struct {
	CandidateP95LatencyMilliseconds int64   `json:"candidateP95LatencyMilliseconds"`
	MaximumP95LatencyMilliseconds   int64   `json:"maximumP95LatencyMilliseconds"`
	CandidateAverageContextTokens   float64 `json:"candidateAverageContextTokens"`
	MaximumAverageContextTokens     float64 `json:"maximumAverageContextTokens"`
	LatencyPassed                   bool    `json:"latencyPassed"`
	ContextTokenCostPassed          bool    `json:"contextTokenCostPassed"`
}

type PromotionIntegrity struct {
	Passed              bool    `json:"passed"`
	CitationLocatorRate float64 `json:"citationLocatorRate"`
	ProvenanceRate      float64 `json:"provenanceRate"`
	CellLineageRate     float64 `json:"cellLineageRate"`
}
