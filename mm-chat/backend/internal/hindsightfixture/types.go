// Package hindsightfixture runs the synthetic-only Hindsight comparison.
// It is an offline fixture adapter and is not part of the active Memory reader.
package hindsightfixture

const (
	ManifestSchemaVersion = "neo-chat.memory-hindsight-fixtures.v1"
	ReportSchemaVersion   = "neo-chat.memory-hindsight-fixture-report.v1"
	AdapterVersion        = "neo-chat.memory-hindsight-fixture-adapter.v1"

	UpstreamVersion     = "0.8.5"
	UpstreamCommit      = "e5b4c52d7ea9bf8ed45ba910f3ad4f92a7bb824a"
	UpstreamImageDigest = "sha256:35d88f6fc2d63ba37e8118dc02945097bf34e4ad04d4f3299e3c426db72c04ba"

	ModeEndToEnd      Mode = "end_to_end"
	ModeRetrievalOnly Mode = "retrieval_only"

	StateActive            MemoryState = "active"
	StateDeleted           MemoryState = "deleted"
	StateSecretRejected    MemoryState = "secret_rejected"
	StateUntrustedRejected MemoryState = "untrusted_rejected"
)

type Mode string

type MemoryState string

type Manifest struct {
	SchemaVersion     string       `json:"schemaVersion"`
	ID                string       `json:"id"`
	PromotionEligible *bool        `json:"promotionEligible"`
	DataPolicy        DataPolicy   `json:"dataPolicy"`
	ContentSHA256     string       `json:"contentSha256"`
	Fixtures          []FixtureSet `json:"fixtures"`
}

type DataPolicy struct {
	SyntheticOnly         bool `json:"syntheticOnly"`
	ContainsRealUserData  bool `json:"containsRealUserData"`
	ContainsSensitiveData bool `json:"containsSensitiveData"`
}

type FixtureSet struct {
	Alias     string   `json:"alias"`
	UserAlias string   `json:"userAlias"`
	Memories  []Memory `json:"memories"`
}

type Memory struct {
	ID               string      `json:"id"`
	Scope            MemoryScope `json:"scope"`
	CanonicalContent string      `json:"canonicalContent"`
	RawEventContent  string      `json:"rawEventContent"`
	OccurredAt       string      `json:"occurredAt"`
	State            MemoryState `json:"state"`
}

type MemoryScope struct {
	ProjectAlias      string `json:"projectAlias,omitempty"`
	ConversationAlias string `json:"conversationAlias,omitempty"`
}

type Report struct {
	SchemaVersion         string           `json:"schemaVersion"`
	AdapterVersion        string           `json:"adapterVersion"`
	ManifestID            string           `json:"manifestId"`
	ManifestContentSHA256 string           `json:"manifestContentSha256"`
	GoldenSetID           string           `json:"goldenSetId"`
	GoldenRawSHA256       string           `json:"goldenRawSha256"`
	PromotionEligible     bool             `json:"promotionEligible"`
	Passed                bool             `json:"passed"`
	ErrorCode             string           `json:"errorCode,omitempty"`
	Profile               ReportProfile    `json:"profile"`
	Fixtures              []FixtureSummary `json:"fixtures"`
	Cases                 []CaseResult     `json:"cases"`
}

type ReportProfile struct {
	Mode                Mode   `json:"mode"`
	UpstreamVersion     string `json:"upstreamVersion"`
	UpstreamCommit      string `json:"upstreamCommit"`
	UpstreamImageDigest string `json:"upstreamImageDigest"`
	ConfigurationSHA256 string `json:"configurationSha256"`
	RemoteProviderCalls int    `json:"remoteProviderCalls"`
	CandidateLimit      int    `json:"candidateLimit"`
	FinalLimit          int    `json:"finalLimit"`
}

type FixtureSummary struct {
	FixtureAlias      string   `json:"fixtureAlias"`
	RetainedMemoryIDs []string `json:"retainedMemoryIds"`
	DeletedMemoryIDs  []string `json:"deletedMemoryIds"`
	RejectedMemoryIDs []string `json:"rejectedMemoryIds"`
}

type CaseResult struct {
	CaseID                string   `json:"caseId"`
	FixtureAlias          string   `json:"fixtureAlias"`
	Status                string   `json:"status"`
	ErrorCode             string   `json:"errorCode,omitempty"`
	CandidateMemoryIDs    []string `json:"candidateMemoryIds"`
	FinalMemoryIDs        []string `json:"finalMemoryIds"`
	PersistedMemoryIDs    []string `json:"persistedMemoryIds"`
	ProviderSentMemoryIDs []string `json:"providerSentMemoryIds"`
	LatencyMilliseconds   int64    `json:"latencyMilliseconds"`
}

type RecallScope struct {
	ProjectAlias      string
	ConversationAlias string
}
