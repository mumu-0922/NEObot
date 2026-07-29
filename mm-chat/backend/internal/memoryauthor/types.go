// Package memoryauthor provides offline-only authoring for the synthetic
// Memory benchmark. It never reads live Memory, calls a Provider, or changes a
// production reader.
package memoryauthor

import "neo-chat/mm-chat/backend/internal/memoryeval"

const (
	FixtureSchemaVersion   = "neo-chat.memory-benchmark-fixtures.v1"
	CandidateSchemaVersion = "neo-chat.memory-benchmark-candidates.v1"
	ReviewEventVersion     = "neo-chat.memory-benchmark-review-event.v1"
	CheckpointVersion      = "neo-chat.memory-benchmark-review-checkpoint.v1"
	FreezeManifestVersion  = "neo-chat.memory-benchmark-freeze-manifest.v1"
	HoldoutBundleVersion   = "neo-chat.memory-benchmark-holdout-bundle.v1"
	HoldoutUseVersion      = "neo-chat.memory-benchmark-holdout-use.v1"
	StatusSchemaVersion    = "neo-chat.memory-benchmark-author-status.v1"
	GeneratorVersion       = "neo-chat.memory-benchmark-generator.v1"
	ProfileID              = "memory-benchmark-zh-mixed-v1"
	ProfileSeed            = "2026072901"

	CandidateFixtureFile  = "candidate-fixtures.json"
	CandidateGoldenFile   = "candidate-golden.json"
	CandidateManifestFile = "candidate-manifest.json"
	ReviewDirectory       = "review"
	ReviewEventsDirectory = "events"
	ReviewCheckpointFile  = "checkpoint.json"
	FrozenDirectory       = "frozen"
	FrozenFixtureFile     = "fixture-manifest.json"
	FrozenGoldenFile      = "golden.json"
	FreezeManifestFile    = "freeze-manifest.json"
	HoldoutDirectory      = "holdout"
	HoldoutUseFile        = "consumed.json"
)

const (
	StateActive            MemoryState = "active"
	StateSuperseded        MemoryState = "superseded"
	StateDeleted           MemoryState = "deleted"
	StateSecretRejected    MemoryState = "secret_rejected"
	StateUntrustedRejected MemoryState = "untrusted_rejected"
	StateIrrelevant        MemoryState = "irrelevant"
	StateOutOfScope        MemoryState = "out_of_scope"
)

type MemoryState string

type DataPolicy struct {
	SyntheticOnly         bool `json:"syntheticOnly"`
	ContainsRealUserData  bool `json:"containsRealUserData"`
	ContainsSensitiveData bool `json:"containsSensitiveData"`
}

type GeneratorProvenance struct {
	Version string `json:"version"`
	Profile string `json:"profile"`
	Seed    string `json:"seed"`
}

type FixtureManifest struct {
	SchemaVersion     string              `json:"schemaVersion"`
	ID                string              `json:"id"`
	Description       string              `json:"description"`
	PromotionEligible *bool               `json:"promotionEligible"`
	DataPolicy        DataPolicy          `json:"dataPolicy"`
	Generator         GeneratorProvenance `json:"generator"`
	ContentSHA256     string              `json:"contentSha256"`
	Fixtures          []Fixture           `json:"fixtures"`
}

type Fixture struct {
	Alias     string          `json:"alias"`
	UserAlias string          `json:"userAlias"`
	Memories  []FixtureMemory `json:"memories"`
}

type FixtureMemory struct {
	ID               string           `json:"id"`
	UserAlias        string           `json:"userAlias"`
	Scope            memoryeval.Scope `json:"scope"`
	CanonicalContent string           `json:"canonicalContent"`
	RawEventContent  string           `json:"rawEventContent"`
	OccurredAt       string           `json:"occurredAt"`
	State            MemoryState      `json:"state"`
}

type CountBySplit struct {
	Development int `json:"development"`
	Validation  int `json:"validation"`
	Holdout     int `json:"holdout"`
}

type CountByLanguage struct {
	Chinese int `json:"zh"`
	Mixed   int `json:"mixed"`
	English int `json:"en"`
}

type SliceCount struct {
	Name        string `json:"name"`
	Total       int    `json:"total"`
	Development int    `json:"development"`
	Validation  int    `json:"validation"`
	Holdout     int    `json:"holdout"`
}

type CandidateManifest struct {
	SchemaVersion            string              `json:"schemaVersion"`
	ID                       string              `json:"id"`
	PromotionEligible        *bool               `json:"promotionEligible"`
	DataPolicy               DataPolicy          `json:"dataPolicy"`
	Generator                GeneratorProvenance `json:"generator"`
	CaseCount                int                 `json:"caseCount"`
	SplitCounts              CountBySplit        `json:"splitCounts"`
	LanguageCounts           CountByLanguage     `json:"languageCounts"`
	SliceCounts              []SliceCount        `json:"sliceCounts"`
	FixtureContentSHA256     string              `json:"fixtureContentSha256"`
	FixtureRawSHA256         string              `json:"fixtureRawSha256"`
	GoldenRawSHA256          string              `json:"goldenRawSha256"`
	FeasibilityWitnessSHA256 string              `json:"feasibilityWitnessSha256"`
}

type GeneratedPool struct {
	FixtureManifest FixtureManifest
	Golden          memoryeval.GoldenSet
	Manifest        CandidateManifest
	FixtureJSON     []byte
	GoldenJSON      []byte
	ManifestJSON    []byte
}

type Diagnostic struct {
	CaseCount      int
	SplitCounts    CountBySplit
	LanguageCounts CountByLanguage
	SliceCounts    []SliceCount
	WitnessCaseIDs []string
}

type Decision string

const (
	DecisionPending  Decision = "pending"
	DecisionAccepted Decision = "accepted"
	DecisionRejected Decision = "rejected"
)

type ReviewAction string

const (
	ReviewActionAccept ReviewAction = "accept"
	ReviewActionReject ReviewAction = "reject"
	ReviewActionEdit   ReviewAction = "edit"
)

type CaseSnapshot struct {
	Case    memoryeval.GoldenCase `json:"case"`
	Fixture Fixture               `json:"fixture"`
}

type ReviewEvent struct {
	SchemaVersion        string        `json:"schemaVersion"`
	Sequence             uint64        `json:"sequence"`
	PreviousEventSHA256  string        `json:"previousEventSha256"`
	Action               ReviewAction  `json:"action"`
	CaseID               string        `json:"caseId"`
	BeforeContentSHA256  string        `json:"beforeContentSha256"`
	AfterContentSHA256   string        `json:"afterContentSha256"`
	FixtureContentSHA256 string        `json:"fixtureContentSha256"`
	ReviewerID           string        `json:"reviewerId"`
	OccurredAt           string        `json:"occurredAt"`
	Snapshot             *CaseSnapshot `json:"snapshot,omitempty"`
}

type CaseState struct {
	Snapshot      CaseSnapshot
	ContentSHA256 string
	Decision      Decision
	ReviewerID    string
	ReviewedAt    string
}

type ReviewState struct {
	CandidateManifest CandidateManifest
	FixtureManifest   FixtureManifest
	Golden            memoryeval.GoldenSet
	Cases             []CaseState
	LastSequence      uint64
	LastEventSHA256   string
	LastOccurredAt    string
}

type ReviewCheckpoint struct {
	SchemaVersion       string `json:"schemaVersion"`
	CandidateManifestID string `json:"candidateManifestId"`
	LastSequence        uint64 `json:"lastSequence"`
	LastEventSHA256     string `json:"lastEventSha256"`
	Accepted            int    `json:"accepted"`
	Rejected            int    `json:"rejected"`
	Pending             int    `json:"pending"`
}

type FreezeManifest struct {
	SchemaVersion            string   `json:"schemaVersion"`
	CandidateManifestID      string   `json:"candidateManifestId"`
	FrozenAt                 string   `json:"frozenAt"`
	HoldoutRunID             string   `json:"holdoutRunId"`
	FixtureContentSHA256     string   `json:"fixtureContentSha256"`
	FixtureRawSHA256         string   `json:"fixtureRawSha256"`
	GoldenRawSHA256          string   `json:"goldenRawSha256"`
	GoldenContentSHA256      string   `json:"goldenContentSha256"`
	ReviewLastSequence       uint64   `json:"reviewLastSequence"`
	ReviewLastEventSHA256    string   `json:"reviewLastEventSha256"`
	OrderedHoldoutCaseIDs    []string `json:"orderedHoldoutCaseIds"`
	OrderedHoldoutCaseHashes []string `json:"orderedHoldoutCaseHashes"`
}

type FrozenArtifacts struct {
	Fixtures FixtureManifest
	Golden   memoryeval.GoldenSet
	Manifest FreezeManifest
}

type HoldoutUse struct {
	SchemaVersion       string `json:"schemaVersion"`
	State               string `json:"state"`
	HoldoutRunID        string `json:"holdoutRunId"`
	Ordinal             int    `json:"ordinal"`
	ConsumedAt          string `json:"consumedAt"`
	GoldenContentSHA256 string `json:"goldenContentSha256"`
	FixtureRawSHA256    string `json:"fixtureRawSha256"`
	OutputPath          string `json:"outputPath"`
}

type HoldoutBundle struct {
	SchemaVersion       string                  `json:"schemaVersion"`
	HoldoutRunID        string                  `json:"holdoutRunId"`
	Ordinal             int                     `json:"ordinal"`
	GoldenSetID         string                  `json:"goldenSetId"`
	GoldenContentSHA256 string                  `json:"goldenContentSha256"`
	FixtureRawSHA256    string                  `json:"fixtureRawSha256"`
	Cases               []memoryeval.GoldenCase `json:"cases"`
	Fixtures            []Fixture               `json:"fixtures"`
}

type Status struct {
	SchemaVersion         string          `json:"schemaVersion"`
	Profile               string          `json:"profile"`
	GeneratorVersion      string          `json:"generatorVersion"`
	CandidateManifestID   string          `json:"candidateManifestId"`
	CandidateManifestHash string          `json:"candidateManifestSha256"`
	FixtureContentSHA256  string          `json:"fixtureContentSha256"`
	CandidateCount        int             `json:"candidateCount"`
	Accepted              int             `json:"accepted"`
	Rejected              int             `json:"rejected"`
	Pending               int             `json:"pending"`
	Frozen                bool            `json:"frozen"`
	GoldenContentSHA256   string          `json:"goldenContentSha256,omitempty"`
	HoldoutState          string          `json:"holdoutState"`
	SplitCounts           CountBySplit    `json:"splitCounts"`
	LanguageCounts        CountByLanguage `json:"languageCounts"`
	SliceCounts           []SliceCount    `json:"sliceCounts"`
}
