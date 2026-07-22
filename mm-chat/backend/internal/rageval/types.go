// Package rageval evaluates redacted, versioned RAG retrieval observations.
// It intentionally accepts evidence identifiers rather than source text so
// reports can be committed without publishing private Knowledge content.
package rageval

const (
	GoldenSchemaVersion      = "mm-chat.rag-golden.v1"
	ObservationSchemaVersion = "mm-chat.rag-observations.v1"
	ReportSchemaVersion      = "mm-chat.rag-evaluation.v1"
)

type GoldenSet struct {
	SchemaVersion string       `json:"schemaVersion"`
	ID            string       `json:"id"`
	Description   string       `json:"description"`
	Criteria      Criteria     `json:"criteria"`
	Cases         []GoldenCase `json:"cases"`
}

type Criteria struct {
	MinimumLaneRecall                map[string]float64 `json:"minimumLaneRecall"`
	MinimumFinalContextPrecision     float64            `json:"minimumFinalContextPrecision"`
	MaximumNegativeFalseCitationRate float64            `json:"maximumNegativeFalseCitationRate"`
	MinimumNoEvidenceAccuracy        float64            `json:"minimumNoEvidenceAccuracy"`
	MaximumP95LatencyMilliseconds    int64              `json:"maximumP95LatencyMilliseconds"`
}

type GoldenCase struct {
	ID                          string   `json:"id"`
	Category                    string   `json:"category"`
	Query                       string   `json:"query"`
	RewrittenQuery              string   `json:"rewrittenQuery,omitempty"`
	SelectedCollectionAliases   []string `json:"selectedCollectionAliases"`
	ExpectedRelevantEvidenceIDs []string `json:"expectedRelevantEvidenceIds"`
	ExpectedNoEvidence          bool     `json:"expectedNoEvidence"`
	RequiredLanes               []string `json:"requiredLanes"`
}

type ObservationSet struct {
	SchemaVersion string            `json:"schemaVersion"`
	GoldenSetID   string            `json:"goldenSetId"`
	CapturedOn    string            `json:"capturedOn"`
	CaptureKind   string            `json:"captureKind"`
	Profile       RetrievalProfile  `json:"profile"`
	Cases         []CaseObservation `json:"cases"`
}

type RetrievalProfile struct {
	ID              string  `json:"id"`
	LexicalEngine   string  `json:"lexicalEngine"`
	DenseEngine     string  `json:"denseEngine"`
	EmbeddingModel  string  `json:"embeddingModel"`
	RerankerModel   string  `json:"rerankerModel"`
	RelevancePolicy string  `json:"relevancePolicy"`
	CandidateLimit  int     `json:"candidateLimit"`
	EvidenceLimit   int     `json:"evidenceLimit"`
	RRFConstant     float64 `json:"rrfConstant"`
}

type CaseObservation struct {
	CaseID                  string                      `json:"caseId"`
	LaneResults             map[string][]RankedEvidence `json:"laneResults"`
	FinalContextEvidenceIDs []string                    `json:"finalContextEvidenceIds"`
	CitationEvidenceIDs     []string                    `json:"citationEvidenceIds"`
	NoEvidence              bool                        `json:"noEvidence"`
	LatencyMilliseconds     int64                       `json:"latencyMilliseconds"`
}

type RankedEvidence struct {
	EvidenceID string  `json:"evidenceId"`
	Rank       int     `json:"rank"`
	Score      float64 `json:"score"`
}

type Report struct {
	SchemaVersion                  string       `json:"schemaVersion"`
	GoldenSetID                    string       `json:"goldenSetId"`
	ProfileID                      string       `json:"profileId"`
	CaseCount                      int          `json:"caseCount"`
	RelevantCaseCount              int          `json:"relevantCaseCount"`
	NegativeCaseCount              int          `json:"negativeCaseCount"`
	LaneRecall                     []LaneRecall `json:"laneRecall"`
	FinalContextPrecision          float64      `json:"finalContextPrecision"`
	NegativeFalseCitationRate      float64      `json:"negativeFalseCitationRate"`
	NegativeFalseCitationCaseCount int          `json:"negativeFalseCitationCaseCount"`
	NoEvidenceAccuracy             float64      `json:"noEvidenceAccuracy"`
	P95LatencyMilliseconds         int64        `json:"p95LatencyMilliseconds"`
	Passed                         bool         `json:"passed"`
	Failures                       []string     `json:"failures"`
}

type LaneRecall struct {
	Lane     string  `json:"lane"`
	Hits     int     `json:"hits"`
	Expected int     `json:"expected"`
	Recall   float64 `json:"recall"`
}
