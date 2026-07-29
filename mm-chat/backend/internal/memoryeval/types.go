// Package memoryeval validates and scores redacted, synthetic Memory benchmark
// artifacts. It is an offline contract only: importing this package must not
// read live Memory, call a provider, or change the production reader.
package memoryeval

const (
	GoldenSchemaVersion                = "neo-chat.memory-benchmark-golden.v1"
	ObservationSchemaVersion           = "neo-chat.memory-benchmark-observations.v1"
	ReportSchemaVersion                = "neo-chat.memory-benchmark-report.v1"
	EvaluatorVersion                   = "neo-chat.memory-benchmark-evaluator.v1"
	FreezeHashSchemaVersion            = "neo-chat.memory-benchmark-freeze-hash.v1"
	RegressionCorpusSchemaVersion      = "neo-chat.memory-benchmark-regression-corpus.v1"
	RegressionAuditSchemaVersion       = "neo-chat.memory-benchmark-regression-audit.v1"
	RegressionObservationSchemaVersion = "neo-chat.memory-benchmark-regression-observations.v1"
	RegressionReportSchemaVersion      = "neo-chat.memory-benchmark-regression-report.v1"
	RegressionEvaluatorVersion         = "neo-chat.memory-benchmark-regression-evaluator.v1"
	RegressionCorpusClass              = "machine_reviewed_regression"
	RegressionAdmissionMode            = "regression_only"
)

var criticalSlices = [...]string{
	"stable_fact",
	"preference_instruction",
	"project_decision",
	"chinese_paraphrase",
	"mixed_language_entity",
	"temporal_correction",
	"unrelated_negative",
	"untrusted_source",
	"secret_rejection",
	"scope_isolation",
	"deletion",
	"failure_fallback",
	"multi_hop",
}

// CriticalSlices returns a defensive copy of the mandatory benchmark slices.
func CriticalSlices() []string {
	return append([]string(nil), criticalSlices[:]...)
}

type GoldenSet struct {
	SchemaVersion         string          `json:"schemaVersion"`
	ID                    string          `json:"id"`
	Description           string          `json:"description"`
	PromotionEligible     *bool           `json:"promotionEligible"`
	DataPolicy            DataPolicy      `json:"dataPolicy"`
	FixtureManifestSHA256 string          `json:"fixtureManifestSha256,omitempty"`
	Lifecycle             GoldenLifecycle `json:"lifecycle"`
	Criteria              Criteria        `json:"criteria"`
	Cases                 []GoldenCase    `json:"cases"`
}

type DataPolicy struct {
	SyntheticOnly         bool `json:"syntheticOnly"`
	ContainsRealUserData  bool `json:"containsRealUserData"`
	ContainsSensitiveData bool `json:"containsSensitiveData"`
}

type GoldenLifecycle struct {
	State               string `json:"state"`
	FrozenAt            string `json:"frozenAt,omitempty"`
	HoldoutRunID        string `json:"holdoutRunId,omitempty"`
	FrozenContentSHA256 string `json:"frozenContentSha256,omitempty"`
}

type Criteria struct {
	MinimumCandidateRecallAt20       float64 `json:"minimumCandidateRecallAt20"`
	MinimumFinalRecallAt5            float64 `json:"minimumFinalRecallAt5"`
	MinimumCurrentFactAccuracy       float64 `json:"minimumCurrentFactAccuracy"`
	MaximumFalseInjectionRate        float64 `json:"maximumFalseInjectionRate"`
	MaximumP95LatencyMilliseconds    int64   `json:"maximumP95LatencyMilliseconds"`
	MaximumP99LatencyMilliseconds    int64   `json:"maximumP99LatencyMilliseconds"`
	HardCutoffMilliseconds           int64   `json:"hardCutoffMilliseconds"`
	MaximumAveragePromptMemoryTokens float64 `json:"maximumAveragePromptMemoryTokens"`
	MaximumPromptMemoryTokens        int     `json:"maximumPromptMemoryTokens"`
	MaximumProviderCostRatio         float64 `json:"maximumProviderCostRatio"`
}

type GoldenCase struct {
	ID                        string      `json:"id"`
	Query                     string      `json:"query"`
	Split                     string      `json:"split"`
	Language                  string      `json:"language"`
	Slices                    []string    `json:"slices"`
	FixtureAlias              string      `json:"fixtureAlias"`
	Scope                     Scope       `json:"scope"`
	ExpectedRelevantMemoryIDs []string    `json:"expectedRelevantMemoryIds"`
	ExpectedCurrentMemoryIDs  []string    `json:"expectedCurrentMemoryIds"`
	Exclusions                []Exclusion `json:"exclusions"`
	ExpectedNoMemory          bool        `json:"expectedNoMemory"`
	Review                    Review      `json:"review"`
}

type Scope struct {
	UserAlias         string `json:"userAlias"`
	ProjectAlias      string `json:"projectAlias,omitempty"`
	ConversationAlias string `json:"conversationAlias,omitempty"`
}

type Exclusion struct {
	MemoryID string `json:"memoryId"`
	Reason   string `json:"reason"`
}

type Review struct {
	State      string `json:"state"`
	ReviewerID string `json:"reviewerId,omitempty"`
	ReviewedAt string `json:"reviewedAt,omitempty"`
}

type ObservationSet struct {
	SchemaVersion         string            `json:"schemaVersion"`
	GoldenSetID           string            `json:"goldenSetId"`
	GoldenCorpusSHA256    string            `json:"goldenCorpusSha256"`
	FixtureManifestSHA256 string            `json:"fixtureManifestSha256"`
	CapturedAt            string            `json:"capturedAt"`
	CaptureID             string            `json:"captureId"`
	Profile               Profile           `json:"profile"`
	HoldoutRun            HoldoutRun        `json:"holdoutRun"`
	Costs                 ProviderCosts     `json:"costs"`
	Cases                 []CaseObservation `json:"cases"`
}

type Profile struct {
	ID                  string `json:"id"`
	Role                string `json:"role"`
	ReaderVersion       string `json:"readerVersion"`
	ConfigurationSHA256 string `json:"configurationSha256"`
	CandidateLimit      int    `json:"candidateLimit"`
	FinalLimit          int    `json:"finalLimit"`
}

type HoldoutRun struct {
	ID         string `json:"id"`
	Ordinal    int    `json:"ordinal"`
	ExecutedAt string `json:"executedAt"`
}

type ProviderCosts struct {
	Unit                         string `json:"unit"`
	MemoryProviderCostMicrounits uint64 `json:"memoryProviderCostMicrounits"`
	ChatProviderCostMicrounits   uint64 `json:"chatProviderCostMicrounits"`
}

type CaseObservation struct {
	CaseID                string   `json:"caseId"`
	CandidateMemoryIDs    []string `json:"candidateMemoryIds"`
	FinalMemoryIDs        []string `json:"finalMemoryIds"`
	InjectedMemoryIDs     []string `json:"injectedMemoryIds"`
	PersistedMemoryIDs    []string `json:"persistedMemoryIds"`
	ProviderSentMemoryIDs []string `json:"providerSentMemoryIds"`
	LatencyMilliseconds   int64    `json:"latencyMilliseconds"`
	PromptMemoryTokens    int      `json:"promptMemoryTokens"`
	HardCutoffApplied     bool     `json:"hardCutoffApplied"`
	Fallback              string   `json:"fallback"`
}

type EvaluationInput struct {
	Golden             GoldenSet
	GoldenRawSHA256    string
	Observations       ObservationSet
	ObservationsSHA256 string
}

// RegressionCorpus is deliberately separate from GoldenSet. It can exercise
// the same scoring implementation, but it has no human-review, frozen, or
// one-shot Holdout authority.
type RegressionCorpus struct {
	SchemaVersion         string                 `json:"schemaVersion"`
	ID                    string                 `json:"id"`
	Description           string                 `json:"description"`
	CorpusClass           string                 `json:"corpusClass"`
	AdmissionMode         string                 `json:"admissionMode"`
	PromotionEligible     *bool                  `json:"promotionEligible"`
	DataPolicy            DataPolicy             `json:"dataPolicy"`
	FixtureManifestSHA256 string                 `json:"fixtureManifestSha256"`
	CorpusContentSHA256   string                 `json:"corpusContentSha256"`
	MachineAudit          RegressionAuditBinding `json:"machineAudit"`
	Criteria              Criteria               `json:"criteria"`
	Cases                 []GoldenCase           `json:"cases"`
}

type RegressionAuditBinding struct {
	SchemaVersion string `json:"schemaVersion"`
	Verdict       string `json:"verdict"`
	Auditor       string `json:"auditor"`
	AuditedAt     string `json:"auditedAt"`
	ContentSHA256 string `json:"contentSha256"`
}

type RegressionAudit struct {
	SchemaVersion         string                   `json:"schemaVersion"`
	CorpusID              string                   `json:"corpusId"`
	CorpusClass           string                   `json:"corpusClass"`
	AdmissionMode         string                   `json:"admissionMode"`
	PromotionEligible     *bool                    `json:"promotionEligible"`
	CorpusContentSHA256   string                   `json:"corpusContentSha256"`
	FixtureManifestSHA256 string                   `json:"fixtureManifestSha256"`
	Auditor               string                   `json:"auditor"`
	AuditedAt             string                   `json:"auditedAt"`
	Verdict               string                   `json:"verdict"`
	CaseCount             int                      `json:"caseCount"`
	SplitCounts           RegressionSplitCounts    `json:"splitCounts"`
	LanguageCounts        RegressionLanguageCounts `json:"languageCounts"`
	SliceCounts           []RegressionSliceCount   `json:"sliceCounts"`
	Semantic              RegressionSemanticAudit  `json:"semantic"`
	ContentSHA256         string                   `json:"contentSha256"`
}

type RegressionSplitCounts struct {
	Development int `json:"development"`
	Validation  int `json:"validation"`
	Holdout     int `json:"holdout"`
}

type RegressionLanguageCounts struct {
	Chinese int `json:"zh"`
	Mixed   int `json:"mixed"`
	English int `json:"en"`
}

type RegressionSliceCount struct {
	Name        string `json:"name"`
	Total       int    `json:"total"`
	Development int    `json:"development"`
	Validation  int    `json:"validation"`
	Holdout     int    `json:"holdout"`
}

type RegressionSemanticAudit struct {
	QuerySkeletonCount             int `json:"querySkeletonCount"`
	NormalizedDuplicateCount       int `json:"normalizedDuplicateCount"`
	OrdinalShortcutCount           int `json:"ordinalShortcutCount"`
	IdentifierShortcutCount        int `json:"identifierShortcutCount"`
	FixtureBindingFailureCount     int `json:"fixtureBindingFailureCount"`
	SliceSemanticFailureCount      int `json:"sliceSemanticFailureCount"`
	LanguageMismatchCount          int `json:"languageMismatchCount"`
	ScopeTextMismatchCount         int `json:"scopeTextMismatchCount"`
	PreferenceSemanticFailureCount int `json:"preferenceSemanticFailureCount"`
	FallbackSemanticFailureCount   int `json:"fallbackSemanticFailureCount"`
	MultiHopSemanticFailureCount   int `json:"multiHopSemanticFailureCount"`
}

type RegressionObservationSet struct {
	SchemaVersion         string            `json:"schemaVersion"`
	CorpusID              string            `json:"corpusId"`
	CorpusContentSHA256   string            `json:"corpusContentSha256"`
	AuditContentSHA256    string            `json:"auditContentSha256"`
	FixtureManifestSHA256 string            `json:"fixtureManifestSha256"`
	CapturedAt            string            `json:"capturedAt"`
	CaptureID             string            `json:"captureId"`
	Profile               Profile           `json:"profile"`
	Costs                 ProviderCosts     `json:"costs"`
	Cases                 []CaseObservation `json:"cases"`
}

type RegressionEvaluationInput struct {
	Corpus             RegressionCorpus
	CorpusRawSHA256    string
	Audit              RegressionAudit
	AuditRawSHA256     string
	Observations       RegressionObservationSet
	ObservationsSHA256 string
}

type RegressionReport struct {
	SchemaVersion     string                         `json:"schemaVersion"`
	Passed            bool                           `json:"passed"`
	PromotionEligible bool                           `json:"promotionEligible"`
	CorpusClass       string                         `json:"corpusClass"`
	AdmissionMode     string                         `json:"admissionMode"`
	Evaluation        RegressionEvaluationProvenance `json:"evaluation"`
	Corpus            RegressionCorpusSummary        `json:"corpus"`
	Profile           ProfileSummary                 `json:"profile"`
	Slices            map[string]SliceResult         `json:"slices"`
	Failures          []string                       `json:"failures"`
}

type RegressionEvaluationProvenance struct {
	EvaluatorVersion      string `json:"evaluatorVersion"`
	CorpusRawSHA256       string `json:"corpusRawSha256"`
	CorpusContentSHA256   string `json:"corpusContentSha256"`
	AuditRawSHA256        string `json:"auditRawSha256"`
	AuditContentSHA256    string `json:"auditContentSha256"`
	ObservationsRawSHA256 string `json:"observationsRawSha256"`
	CaptureID             string `json:"captureId"`
	FixtureManifestSHA256 string `json:"fixtureManifestSha256"`
}

type RegressionCorpusSummary struct {
	CorpusID         string `json:"corpusId"`
	AuditVerdict     string `json:"auditVerdict"`
	TotalCases       int    `json:"totalCases"`
	DevelopmentCount int    `json:"developmentCount"`
	ValidationCount  int    `json:"validationCount"`
	HoldoutCount     int    `json:"holdoutCount"`
}

type Report struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Passed        bool                   `json:"passed"`
	Evaluation    EvaluationProvenance   `json:"evaluation"`
	Golden        GoldenSummary          `json:"golden"`
	Profile       ProfileSummary         `json:"profile"`
	Slices        map[string]SliceResult `json:"slices"`
	Failures      []string               `json:"failures"`
}

type EvaluationProvenance struct {
	EvaluatorVersion          string `json:"evaluatorVersion"`
	GoldenCorpusRawSHA256     string `json:"goldenCorpusRawSha256"`
	GoldenFrozenContentSHA256 string `json:"goldenFrozenContentSha256"`
	ObservationsRawSHA256     string `json:"observationsRawSha256"`
	CaptureID                 string `json:"captureId"`
	HoldoutRunID              string `json:"holdoutRunId"`
	FixtureManifestSHA256     string `json:"fixtureManifestSha256"`
}

type GoldenSummary struct {
	CorpusID         string `json:"corpusId"`
	State            string `json:"state"`
	FrozenAt         string `json:"frozenAt"`
	TotalReviewed    int    `json:"totalReviewed"`
	DevelopmentCount int    `json:"developmentCount"`
	ValidationCount  int    `json:"validationCount"`
	HoldoutCount     int    `json:"holdoutCount"`
	HoldoutRuns      int    `json:"holdoutRuns"`
}

type ProfileSummary struct {
	ProfileID          string         `json:"profileId"`
	ProfileRole        string         `json:"profileRole"`
	ReaderVersion      string         `json:"readerVersion"`
	Metrics            Metrics        `json:"metrics"`
	RankingDiagnostics RankingMetrics `json:"rankingDiagnostics"`
	Budgets            Budgets        `json:"budgets"`
	Safety             SafetyMetrics  `json:"safety"`
	ProviderCostRatio  float64        `json:"providerCostRatio"`
	ProviderCostPassed bool           `json:"providerCostPassed"`
}

type Metrics struct {
	CandidateRecallAt20  float64 `json:"candidateRecallAt20"`
	FinalRecallAt5       float64 `json:"finalRecallAt5"`
	CurrentFactAccuracy  float64 `json:"currentFactAccuracy"`
	FalseInjectionRate   float64 `json:"falseInjectionRate"`
	RelevantCaseCount    int     `json:"relevantCaseCount"`
	NegativeCaseCount    int     `json:"negativeCaseCount"`
	CurrentFactCaseCount int     `json:"currentFactCaseCount"`
	FalseInjectionCases  int     `json:"falseInjectionCases"`
}

type RankingMetrics struct {
	NDCGAt5 float64 `json:"ndcgAt5"`
	MRRAt5  float64 `json:"mrrAt5"`
}

type Budgets struct {
	P95LatencyMilliseconds    int64   `json:"p95LatencyMilliseconds"`
	P99LatencyMilliseconds    int64   `json:"p99LatencyMilliseconds"`
	AveragePromptMemoryTokens float64 `json:"averagePromptMemoryTokens"`
	MaximumPromptMemoryTokens int     `json:"maximumPromptMemoryTokens"`
	HardCutoffViolationCount  int     `json:"hardCutoffViolationCount"`
	LatencyPassed             bool    `json:"latencyPassed"`
	PromptTokenPassed         bool    `json:"promptTokenPassed"`
	HardCutoffPassed          bool    `json:"hardCutoffPassed"`
}

type SafetyMetrics struct {
	CrossUserLeakCount              int  `json:"crossUserLeakCount"`
	DeletedMemoryLeakCount          int  `json:"deletedMemoryLeakCount"`
	SecretLeakCount                 int  `json:"secretLeakCount"`
	UntrustedSourceLeakCount        int  `json:"untrustedSourceLeakCount"`
	UnauthorizedProviderEgressCount int  `json:"unauthorizedProviderEgressCount"`
	Passed                          bool `json:"passed"`
}

type SliceResult struct {
	Cases              int            `json:"cases"`
	Metrics            Metrics        `json:"metrics"`
	RankingDiagnostics RankingMetrics `json:"rankingDiagnostics"`
	Budgets            Budgets        `json:"budgets"`
	Safety             SafetyMetrics  `json:"safety"`
	Passed             bool           `json:"passed"`
	Failures           []string       `json:"failures"`
}

type FreezeHashReport struct {
	SchemaVersion       string `json:"schemaVersion"`
	CorpusID            string `json:"corpusId"`
	State               string `json:"state"`
	CaseCount           int    `json:"caseCount"`
	FrozenContentSHA256 string `json:"frozenContentSha256"`
	PromotionEligible   bool   `json:"promotionEligible"`
}
