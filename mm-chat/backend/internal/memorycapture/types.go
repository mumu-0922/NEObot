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
	BaselineProfileID           = "native_v1_lexical"
	CandidateProfileID          = "native_v2_hybrid"
	FakeCandidateProfileID      = "native_v2_hybrid_fake_protocol"
	ReaderVersion               = "neo-chat.native-memory-reader-capture.v1"
	ProviderModeNone            = "none"
	ProviderModeFakeProtocol    = "fake_protocol"
	ProviderModeLiveSiliconFlow = "live_siliconflow"
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
	SchemaVersion        string `json:"schemaVersion"`
	ProfileID            string `json:"profileId"`
	ReaderVersion        string `json:"readerVersion"`
	FixtureRawSHA256     string `json:"fixtureRawSha256"`
	CorpusRawSHA256      string `json:"corpusRawSha256"`
	AuditRawSHA256       string `json:"auditRawSha256"`
	ManifestRawSHA256    string `json:"manifestRawSha256"`
	CostBasisSHA256      string `json:"costBasisSha256"`
	ProviderMode         string `json:"providerMode"`
	EmbeddingProfileID   string `json:"embeddingProfileId"`
	EmbeddingModelID     string `json:"embeddingModelId"`
	EmbeddingDimensions  int    `json:"embeddingDimensions"`
	RerankModelID        string `json:"rerankModelId"`
	CandidateLimit       int    `json:"candidateLimit"`
	FinalLimit           int    `json:"finalLimit"`
	TargetTokens         int    `json:"targetTokens"`
	MaximumTokens        int    `json:"maximumTokens"`
	HardCutoffMillis     int    `json:"hardCutoffMillis"`
	FixtureMapping       string `json:"fixtureMapping"`
	CounterfactualInject bool   `json:"counterfactualInject"`
}

// CostBasis is supplied by an operator and hash-bound to observations. The
// capture never invents a nominal cost to satisfy the evaluator.
type CostBasis struct {
	SchemaVersion string                   `json:"schemaVersion"`
	Baseline      memoryeval.ProviderCosts `json:"baseline"`
	Candidate     memoryeval.ProviderCosts `json:"candidate"`
	Source        string                   `json:"source"`
	EffectiveAt   string                   `json:"effectiveAt"`
}

// CapturedProfile is an ordered set of observations for one immutable reader
// profile before it is bound to the regression corpus header.
type CapturedProfile struct {
	Profile memoryeval.Profile
	Costs   memoryeval.ProviderCosts
	Cases   []memoryeval.CaseObservation
}

type clock func() time.Time
