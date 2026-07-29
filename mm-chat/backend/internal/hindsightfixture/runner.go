package hindsightfixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const (
	candidateLimit = 20
	finalLimit     = 5
)

type Adapter interface {
	ConfigureBank(context.Context, string, Mode) error
	Retain(context.Context, string, RetainItem) error
	Recall(context.Context, string, string, RecallScope) ([]string, error)
	DeleteDocument(context.Context, string, string) error
	DeleteBank(context.Context, string) error
}

type Runner struct {
	adapter Adapter
	apiKey  string
}

func NewRunner(adapter Adapter, apiKey string) (*Runner, error) {
	if adapter == nil || len(apiKey) < 32 {
		return nil, errors.New("Hindsight fixture runner configuration is invalid")
	}
	return &Runner{adapter: adapter, apiKey: apiKey}, nil
}

func (runner *Runner) Run(
	ctx context.Context,
	manifest Manifest,
	golden memoryeval.GoldenSet,
	goldenRawSHA256 string,
	mode Mode,
) Report {
	report := newReport(manifest, golden, goldenRawSHA256, mode)
	if mode != ModeEndToEnd && mode != ModeRetrievalOnly {
		report.ErrorCode = "invalid_mode"
		return report
	}
	if err := ValidateGoldenBinding(manifest, golden, goldenRawSHA256); err != nil {
		report.ErrorCode = "fixture_binding_invalid"
		return report
	}

	type fixtureRuntime struct {
		fixture FixtureSet
		bankID  string
	}
	runtimes := make(map[string]fixtureRuntime, len(manifest.Fixtures))
	bankIDs := make([]string, 0, len(manifest.Fixtures))
	owners := make(map[string]string)
	states := make(map[string]MemoryState)
	for _, fixture := range manifest.Fixtures {
		bankID, err := DeriveBankID(
			runner.apiKey,
			manifest.ContentSHA256,
			mode,
			fixture.Alias,
			fixture.UserAlias,
		)
		if err != nil {
			report.ErrorCode = "bank_derivation_failed"
			return report
		}
		runtimes[fixture.Alias] = fixtureRuntime{fixture: fixture, bankID: bankID}
		bankIDs = append(bankIDs, bankID)
		for _, memory := range fixture.Memories {
			owners[memory.ID] = fixture.Alias
			states[memory.ID] = memory.State
		}
	}
	defer runner.deleteBanks(bankIDs, &report)

	for fixtureIndex, fixture := range manifest.Fixtures {
		runtime := runtimes[fixture.Alias]
		if err := runner.adapter.ConfigureBank(ctx, runtime.bankID, mode); err != nil {
			report.ErrorCode = FaultCode(err)
			return report
		}
		for _, memory := range fixture.Memories {
			summary := &report.Fixtures[fixtureIndex]
			switch memory.State {
			case StateSecretRejected, StateUntrustedRejected:
				summary.RejectedMemoryIDs = append(summary.RejectedMemoryIDs, memory.ID)
				continue
			}
			content := memory.CanonicalContent
			if mode == ModeEndToEnd {
				content = memory.RawEventContent
			}
			documentID := deriveDocumentID(runner.apiKey, runtime.bankID, memory.ID)
			if err := runner.adapter.Retain(ctx, runtime.bankID, RetainItem{
				Content:    content,
				Timestamp:  memory.OccurredAt,
				Metadata:   map[string]string{"neo_memory_id": memory.ID},
				DocumentID: documentID,
				Tags:       memoryTags(memory.Scope),
			}); err != nil {
				report.ErrorCode = FaultCode(err)
				return report
			}
			if memory.State == StateDeleted {
				if err := runner.adapter.DeleteDocument(ctx, runtime.bankID, documentID); err != nil {
					report.ErrorCode = FaultCode(err)
					return report
				}
				summary.DeletedMemoryIDs = append(summary.DeletedMemoryIDs, memory.ID)
				continue
			}
			summary.RetainedMemoryIDs = append(summary.RetainedMemoryIDs, memory.ID)
		}
	}

	report.Passed = true
	for _, goldenCase := range golden.Cases {
		runtime := runtimes[goldenCase.FixtureAlias]
		startedAt := time.Now()
		logicalIDs, err := runner.adapter.Recall(ctx, runtime.bankID, goldenCase.Query, RecallScope{
			ProjectAlias:      goldenCase.Scope.ProjectAlias,
			ConversationAlias: goldenCase.Scope.ConversationAlias,
		})
		latency := time.Since(startedAt).Milliseconds()
		if latency < 0 {
			latency = 0
		}
		caseResult := CaseResult{
			CaseID:                goldenCase.ID,
			FixtureAlias:          goldenCase.FixtureAlias,
			Status:                "passed",
			CandidateMemoryIDs:    []string{},
			FinalMemoryIDs:        []string{},
			PersistedMemoryIDs:    retainedIDs(report.Fixtures, goldenCase.FixtureAlias),
			ProviderSentMemoryIDs: []string{},
			LatencyMilliseconds:   latency,
		}
		if err != nil {
			caseResult.Status = "failed"
			caseResult.ErrorCode = FaultCode(err)
			report.Passed = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		caseResult.CandidateMemoryIDs = uniqueLimited(logicalIDs, candidateLimit)
		caseResult.FinalMemoryIDs = append(
			[]string{},
			caseResult.CandidateMemoryIDs[:min(len(caseResult.CandidateMemoryIDs), finalLimit)]...,
		)
		caseResult.ErrorCode = validateCaseResult(
			goldenCase,
			caseResult,
			owners,
			states,
		)
		if caseResult.ErrorCode != "" {
			caseResult.Status = "failed"
			report.Passed = false
		}
		report.Cases = append(report.Cases, caseResult)
	}
	return report
}

func newReport(
	manifest Manifest,
	golden memoryeval.GoldenSet,
	goldenRawSHA256 string,
	mode Mode,
) Report {
	fixtures := make([]FixtureSummary, 0, len(manifest.Fixtures))
	for _, fixture := range manifest.Fixtures {
		fixtures = append(fixtures, FixtureSummary{
			FixtureAlias:      fixture.Alias,
			RetainedMemoryIDs: []string{},
			DeletedMemoryIDs:  []string{},
			RejectedMemoryIDs: []string{},
		})
	}
	return Report{
		SchemaVersion:         ReportSchemaVersion,
		AdapterVersion:        AdapterVersion,
		ManifestID:            manifest.ID,
		ManifestContentSHA256: manifest.ContentSHA256,
		GoldenSetID:           golden.ID,
		GoldenRawSHA256:       goldenRawSHA256,
		PromotionEligible:     false,
		Passed:                false,
		Profile: ReportProfile{
			Mode:                mode,
			UpstreamVersion:     UpstreamVersion,
			UpstreamCommit:      UpstreamCommit,
			UpstreamImageDigest: UpstreamImageDigest,
			ConfigurationSHA256: profileConfigurationSHA256(mode),
			RemoteProviderCalls: 0,
			CandidateLimit:      candidateLimit,
			FinalLimit:          finalLimit,
		},
		Fixtures: fixtures,
		Cases:    []CaseResult{},
	}
}

func profileConfigurationSHA256(mode Mode) string {
	extractionMode := "concise"
	if mode == ModeRetrievalOnly {
		extractionMode = "chunks"
	}
	configuration := struct {
		Mode              Mode   `json:"mode"`
		ExtractionMode    string `json:"extractionMode"`
		LLMProvider       string `json:"llmProvider"`
		EmbeddingProvider string `json:"embeddingProvider"`
		EmbeddingModel    string `json:"embeddingModel"`
		RerankerProvider  string `json:"rerankerProvider"`
		RerankerModel     string `json:"rerankerModel"`
		CandidateLimit    int    `json:"candidateLimit"`
		FinalLimit        int    `json:"finalLimit"`
		GlobalTagsMatch   string `json:"globalTagsMatch"`
		ScopedTagsMatch   string `json:"scopedTagsMatch"`
	}{
		Mode: mode, ExtractionMode: extractionMode, LLMProvider: "mock",
		EmbeddingProvider: "local", EmbeddingModel: "BAAI/bge-small-en-v1.5",
		RerankerProvider: "local", RerankerModel: "cross-encoder/ms-marco-MiniLM-L-6-v2",
		CandidateLimit: candidateLimit, FinalLimit: finalLimit,
		GlobalTagsMatch: "exact", ScopedTagsMatch: "any",
	}
	body, _ := json.Marshal(configuration)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func retainedIDs(fixtures []FixtureSummary, alias string) []string {
	for _, fixture := range fixtures {
		if fixture.FixtureAlias == alias {
			return append([]string{}, fixture.RetainedMemoryIDs...)
		}
	}
	return []string{}
}

func uniqueLimited(values []string, limit int) []string {
	result := make([]string, 0, min(len(values), limit))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func validateCaseResult(
	goldenCase memoryeval.GoldenCase,
	result CaseResult,
	owners map[string]string,
	states map[string]MemoryState,
) string {
	for _, memoryID := range result.CandidateMemoryIDs {
		owner, known := owners[memoryID]
		if !known {
			return "unknown_memory_result"
		}
		if owner != goldenCase.FixtureAlias {
			return "cross_bank_result"
		}
		switch states[memoryID] {
		case StateDeleted:
			return "deleted_memory_result"
		case StateSecretRejected:
			return "secret_memory_result"
		case StateUntrustedRejected:
			return "untrusted_memory_result"
		}
	}
	for _, expected := range goldenCase.ExpectedRelevantMemoryIDs {
		if !slices.Contains(result.CandidateMemoryIDs, expected) {
			return "quality_mismatch"
		}
	}
	for _, expected := range goldenCase.ExpectedCurrentMemoryIDs {
		if !slices.Contains(result.FinalMemoryIDs, expected) {
			return "quality_mismatch"
		}
	}
	for _, exclusion := range goldenCase.Exclusions {
		if slices.Contains(result.CandidateMemoryIDs, exclusion.MemoryID) {
			return "forbidden_memory_result"
		}
	}
	if goldenCase.ExpectedNoMemory && len(result.CandidateMemoryIDs) != 0 {
		return "false_positive_result"
	}
	return ""
}

func (runner *Runner) deleteBanks(bankIDs []string, report *Report) {
	cleanupContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for index := len(bankIDs) - 1; index >= 0; index-- {
		if err := runner.adapter.DeleteBank(cleanupContext, bankIDs[index]); err != nil {
			report.Passed = false
			if report.ErrorCode == "" {
				report.ErrorCode = "bank_cleanup_failed"
			}
		}
	}
}
