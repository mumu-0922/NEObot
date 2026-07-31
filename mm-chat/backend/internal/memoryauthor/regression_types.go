package memoryauthor

import "neo-chat/mm-chat/backend/internal/memoryeval"

const (
	RegressionFixtureSchemaVersion     = "neo-chat.memory-benchmark-regression-fixtures.v1"
	RegressionManifestSchemaVersion    = "neo-chat.memory-benchmark-regression-manifest.v1"
	RegressionStatusSchemaVersion      = "neo-chat.memory-benchmark-regression-status.v1"
	RegressionGeneratorVersion         = "neo-chat.memory-benchmark-regression-generator.v1"
	RegressionProfileID                = "memory-regression-zh-mixed-v2"
	RegressionProfileSeed              = "2026072902"
	RegressionAuditor                  = "deterministic-semantic-audit.v1"
	RegressionAuditedAt                = "2026-07-29T12:00:00Z"
	RegressionRepairedGeneratorVersion = "neo-chat.memory-benchmark-regression-generator.v2"
	RegressionRepairedProfileID        = "memory-regression-zh-mixed-v3"
	RegressionRepairedProfileSeed      = "2026073101"
	RegressionRepairedAuditor          = "deterministic-semantic-audit.v2"
	RegressionRepairedAuditedAt        = "2026-07-31T08:30:00Z"

	RegressionFixtureFile  = "fixtures.json"
	RegressionCorpusFile   = "corpus.json"
	RegressionAuditFile    = "audit.json"
	RegressionManifestFile = "manifest.json"
)

type RegressionFixtureManifest struct {
	SchemaVersion     string              `json:"schemaVersion"`
	ID                string              `json:"id"`
	Description       string              `json:"description"`
	CorpusClass       string              `json:"corpusClass"`
	AdmissionMode     string              `json:"admissionMode"`
	PromotionEligible *bool               `json:"promotionEligible"`
	DataPolicy        DataPolicy          `json:"dataPolicy"`
	Generator         GeneratorProvenance `json:"generator"`
	ContentSHA256     string              `json:"contentSha256"`
	Fixtures          []Fixture           `json:"fixtures"`
}

type RegressionManifest struct {
	SchemaVersion        string              `json:"schemaVersion"`
	ID                   string              `json:"id"`
	CorpusClass          string              `json:"corpusClass"`
	AdmissionMode        string              `json:"admissionMode"`
	PromotionEligible    *bool               `json:"promotionEligible"`
	DataPolicy           DataPolicy          `json:"dataPolicy"`
	Generator            GeneratorProvenance `json:"generator"`
	CaseCount            int                 `json:"caseCount"`
	SplitCounts          CountBySplit        `json:"splitCounts"`
	LanguageCounts       CountByLanguage     `json:"languageCounts"`
	SliceCounts          []SliceCount        `json:"sliceCounts"`
	QuerySkeletonCount   int                 `json:"querySkeletonCount"`
	FixtureContentSHA256 string              `json:"fixtureContentSha256"`
	FixtureRawSHA256     string              `json:"fixtureRawSha256"`
	CorpusContentSHA256  string              `json:"corpusContentSha256"`
	CorpusRawSHA256      string              `json:"corpusRawSha256"`
	AuditContentSHA256   string              `json:"auditContentSha256"`
	AuditRawSHA256       string              `json:"auditRawSha256"`
}

type RegressionPool struct {
	Fixtures     RegressionFixtureManifest
	Corpus       memoryeval.RegressionCorpus
	Audit        memoryeval.RegressionAudit
	Manifest     RegressionManifest
	FixtureJSON  []byte
	CorpusJSON   []byte
	AuditJSON    []byte
	ManifestJSON []byte
}

type RegressionStatus struct {
	SchemaVersion        string          `json:"schemaVersion"`
	CorpusClass          string          `json:"corpusClass"`
	AdmissionMode        string          `json:"admissionMode"`
	PromotionEligible    bool            `json:"promotionEligible"`
	Profile              string          `json:"profile"`
	GeneratorVersion     string          `json:"generatorVersion"`
	CaseCount            int             `json:"caseCount"`
	SplitCounts          CountBySplit    `json:"splitCounts"`
	LanguageCounts       CountByLanguage `json:"languageCounts"`
	SliceCounts          []SliceCount    `json:"sliceCounts"`
	QuerySkeletonCount   int             `json:"querySkeletonCount"`
	AuditVerdict         string          `json:"auditVerdict"`
	FixtureContentSHA256 string          `json:"fixtureContentSha256"`
	CorpusContentSHA256  string          `json:"corpusContentSha256"`
	AuditContentSHA256   string          `json:"auditContentSha256"`
	ManifestSHA256       string          `json:"manifestSha256"`
}
