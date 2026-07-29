package usermemory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestSearchRelevantL3PersonaShadowRecordsWithoutInjection(t *testing.T) {
	repository := &l3PersonaTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L3PersonaPreparation{
			Summary: L3PersonaSearchSummary{
				ProfileID: L3PersonaProfileID, Mode: "shadow", Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE",
				ExactCount: 2, BM25Count: 2, VectorCount: 1, RRFCount: 2,
			},
			Candidates: []L3PersonaCandidate{
				l3PersonaCandidate("44444444-4444-4444-8444-444444444444", "Global persona"),
				l3PersonaCandidate("55555555-5555-4555-8555-555555555555", "Project persona"),
			},
		},
		recordResult: L3PersonaSearchResult{
			Summary: L3PersonaSearchSummary{
				ProfileID: L3PersonaProfileID, Mode: "shadow", Status: "completed",
				ResultCode: "COMPLETED", FallbackCode: "NONE",
				RerankCount: 2, FinalCount: 1, InjectedCount: 0,
			},
			Personas: []L3PersonaCandidate{
				l3PersonaCandidate("55555555-5555-4555-8555-555555555555", "Project persona"),
			},
		},
	}
	provider := &hybridTestProvider{
		embedding: validHybridTestEmbedding(),
		rerank: []ragproviders.RerankResult{
			{Index: 0, RelevanceScore: 0.2},
			{Index: 1, RelevanceScore: 0.9},
		},
	}
	result, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantL3Persona(
		context.Background(), "project preferences", hybridTestConversation,
		hybridTestAssistant, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Personas) != 0 || result.Summary.Mode != "shadow" ||
		result.Summary.InjectedCount != 0 {
		t.Fatalf("shadow result = %#v", result)
	}
	if repository.prepareCalls != 1 || repository.recordCalls != 1 ||
		provider.embedCalls != 1 || provider.rerankCalls != 1 {
		t.Fatalf("calls = prepare:%d record:%d embed:%d rerank:%d",
			repository.prepareCalls, repository.recordCalls,
			provider.embedCalls, provider.rerankCalls)
	}
	if repository.prepareInput.ActiveRequested ||
		repository.prepareInput.QueryHash != sha256String("project preferences") ||
		repository.prepareInput.QueryEmbeddingState != "ready" {
		t.Fatalf("prepare input = %#v", repository.prepareInput)
	}
	if len(repository.recordInput.Final) != 1 ||
		repository.recordInput.Final[0].PersonaID !=
			"55555555-5555-4555-8555-555555555555" ||
		repository.recordInput.RerankStatus != "applied" {
		t.Fatalf("record input = %#v", repository.recordInput)
	}
}

func TestSearchRelevantL3PersonaActiveReturnsOnlyAuthorizedRecordResult(t *testing.T) {
	persona := l3PersonaCandidate(
		"44444444-4444-4444-8444-444444444444",
		"Current authorized persona",
	)
	repository := &l3PersonaTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L3PersonaPreparation{
			Summary: L3PersonaSearchSummary{
				ProfileID: L3PersonaProfileID, Mode: "active", Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []L3PersonaCandidate{persona},
		},
		recordResult: L3PersonaSearchResult{
			Summary: L3PersonaSearchSummary{
				ProfileID: L3PersonaProfileID, Mode: "active", Status: "completed",
				ResultCode: "COMPLETED", FallbackCode: "NONE",
				FinalCount: 1, InjectedCount: 1,
			},
			Personas: []L3PersonaCandidate{persona},
		},
	}
	result, err := NewService(repository).SearchRelevantL3Persona(
		context.Background(), "current project", hybridTestConversation,
		hybridTestAssistant, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !repository.prepareInput.ActiveRequested || len(result.Personas) != 1 ||
		result.Personas[0].Content != persona.Content ||
		result.Summary.InjectedCount != 1 {
		t.Fatalf("active result = %#v prepare=%#v", result, repository.prepareInput)
	}
}

func TestSearchRelevantL3PersonaDatabaseAuthorityFailureFailsOpen(t *testing.T) {
	repository := &l3PersonaTestRepository{
		fakeRepository: &fakeRepository{},
		prepareErr:     errors.New("MEMORY_L2_SCENE_READER_DISABLED private detail"),
	}
	result, err := NewService(repository).SearchRelevantL3Persona(
		context.Background(), "current project", hybridTestConversation,
		hybridTestAssistant, true,
	)
	if err != nil || len(result.Personas) != 0 || result.Summary.Status != "failed" ||
		result.Summary.ResultCode != "PREPARE_FAILED" ||
		strings.Contains(result.Summary.ResultCode, "private") {
		t.Fatalf("authority failure = %#v/%v", result, err)
	}
}

func TestSearchRelevantL3PersonaProviderFailureUsesRRFFallback(t *testing.T) {
	persona := l3PersonaCandidate(
		"44444444-4444-4444-8444-444444444444",
		"Lexical fallback persona",
	)
	repository := &l3PersonaTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L3PersonaPreparation{
			Summary: L3PersonaSearchSummary{
				ProfileID: L3PersonaProfileID, Mode: "shadow", Status: "pending",
				ResultCode:   "CANDIDATES_READY",
				FallbackCode: "QUERY_EMBEDDING_FAILED", RRFCount: 1,
			},
			Candidates: []L3PersonaCandidate{persona},
		},
		recordResult: L3PersonaSearchResult{Summary: L3PersonaSearchSummary{
			ProfileID: L3PersonaProfileID, Mode: "shadow", Status: "completed",
			ResultCode: "COMPLETED", FallbackCode: "QUERY_EMBEDDING_FAILED",
			FinalCount: 1,
		}},
	}
	provider := &hybridTestProvider{
		embedErr:  errors.New("private embedding error"),
		rerankErr: errors.New("private rerank error"),
	}
	result, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantL3Persona(
		context.Background(), "current project", hybridTestConversation,
		hybridTestAssistant, false,
	)
	if err != nil || result.Summary.FallbackCode != "QUERY_EMBEDDING_FAILED" ||
		repository.prepareInput.QueryEmbeddingState != "failed" ||
		repository.recordInput.RerankStatus != "fallback" ||
		len(repository.recordInput.Reranked) != 0 ||
		len(repository.recordInput.Final) != 1 {
		t.Fatalf("provider fallback = result:%#v prepare:%#v record:%#v err:%v",
			result, repository.prepareInput, repository.recordInput, err)
	}
}

func TestSearchRelevantL3PersonaSecretQueryAndDocumentMakeZeroCalls(t *testing.T) {
	t.Run("query", func(t *testing.T) {
		repository := l3PersonaFallbackRepository("Safe lexical Persona")
		provider := &hybridTestProvider{embedding: validHybridTestEmbedding()}
		_, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
		).SearchRelevantL3Persona(
			context.Background(), "password: fixture-secret-value",
			hybridTestConversation, hybridTestAssistant, false,
		)
		if err != nil {
			t.Fatal(err)
		}
		if provider.embedCalls != 0 || provider.rerankCalls != 0 ||
			repository.prepareInput.QueryEmbeddingState != "redacted" ||
			repository.recordInput.FallbackCode != "SECRET_REDACTED" {
			t.Fatalf("secret query egress = provider:%#v prepare:%#v record:%#v",
				provider, repository.prepareInput, repository.recordInput)
		}
	})
	t.Run("document", func(t *testing.T) {
		repository := l3PersonaFallbackRepository("api_key=fixture-private-value")
		provider := &hybridTestProvider{embedding: validHybridTestEmbedding()}
		_, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
		).SearchRelevantL3Persona(
			context.Background(), "safe query", hybridTestConversation,
			hybridTestAssistant, false,
		)
		if err != nil {
			t.Fatal(err)
		}
		if provider.embedCalls != 1 || provider.rerankCalls != 0 ||
			repository.recordInput.RerankStatus != "fallback" ||
			repository.recordInput.FallbackCode != "SECRET_REDACTED" {
			t.Fatalf("secret document egress = provider:%#v record:%#v",
				provider, repository.recordInput)
		}
	})
}

func TestSearchRelevantL3PersonaLateRerankFallsBackBeforeCallerDeadline(t *testing.T) {
	repository := l3PersonaFallbackRepository("RRF survives timeout")
	provider := &hybridTestProvider{
		embedding:                 validHybridTestEmbedding(),
		returnRerankAfterDeadline: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	startedAt := time.Now()
	_, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantL3Persona(
		ctx, "current project", hybridTestConversation, hybridTestAssistant, false,
	)
	if err != nil || time.Since(startedAt) >= 300*time.Millisecond ||
		repository.recordCalls != 1 ||
		repository.recordInput.FallbackCode != "HARD_CUTOFF" ||
		len(repository.recordInput.Final) != 1 {
		t.Fatalf("late rerank = duration:%s record:%#v err:%v",
			time.Since(startedAt), repository.recordInput, err)
	}
}

func TestSelectL3PersonaFinalEnforcesOneAndThreeHundredTokens(t *testing.T) {
	candidates := []L3PersonaCandidate{
		l3PersonaCandidate("11111111-1111-4111-8111-111111111111", strings.Repeat("中", 300)),
		l3PersonaCandidate("22222222-2222-4222-8222-222222222222", strings.Repeat("中", 100)),
		l3PersonaCandidate("33333333-3333-4333-8333-333333333333", strings.Repeat("中", 100)),
		l3PersonaCandidate("44444444-4444-4444-8444-444444444444", "unused"),
	}
	selected, tokens := selectL3PersonaFinal(candidates)
	if len(selected) != 1 || tokens > L3PersonaMaximumTokens ||
		selected[0].PersonaID != candidates[1].PersonaID {
		t.Fatalf("budget selection = %#v tokens=%d", selected, tokens)
	}
}

func TestSearchRelevantL3PersonaRejectsUnmatchedActiveInjectionCount(t *testing.T) {
	repository := l3PersonaFallbackRepository("Current persona")
	repository.preparation.Summary.Mode = "active"
	repository.recordResult.Summary.Mode = "active"
	repository.recordResult.Summary.InjectedCount = 0
	repository.recordResult.Personas = repository.preparation.Candidates
	result, err := NewService(repository).SearchRelevantL3Persona(
		context.Background(), "current project", hybridTestConversation,
		hybridTestAssistant, true,
	)
	if err != nil || len(result.Personas) != 0 {
		t.Fatalf("mismatched active result = %#v/%v", result, err)
	}
}

func TestSanitizeL3PersonaSummaryBoundsDiagnostics(t *testing.T) {
	summary := sanitizeL3PersonaSummary(L3PersonaSearchSummary{
		ProfileID: "forged", Mode: "private", Status: "private",
		ResultCode: "raw query", FallbackCode: "private fallback",
		ExactCount: 99, BM25Count: 99, VectorCount: 99, RRFCount: 99,
		RerankCount: 99, FinalCount: 99, InjectedCount: 99,
		EstimatedTokens: 9999, DurationMillis: 999999,
	})
	if summary.ProfileID != L3PersonaProfileID || summary.Mode != "shadow" ||
		summary.Status != "failed" || summary.ResultCode != "L3_PERSONA_FAILED" ||
		summary.FallbackCode != "NONE" || summary.ExactCount != L3PersonaCandidateLimit ||
		summary.BM25Count != L3PersonaCandidateLimit ||
		summary.VectorCount != L3PersonaCandidateLimit ||
		summary.RRFCount != L3PersonaCandidateLimit ||
		summary.RerankCount != L3PersonaCandidateLimit ||
		summary.FinalCount != L3PersonaFinalLimit ||
		summary.InjectedCount != L3PersonaFinalLimit ||
		summary.EstimatedTokens != L3PersonaMaximumTokens ||
		summary.DurationMillis != 120000 {
		t.Fatalf("sanitized summary = %#v", summary)
	}
}

func l3PersonaFallbackRepository(content string) *l3PersonaTestRepository {
	persona := l3PersonaCandidate("44444444-4444-4444-8444-444444444444", content)
	return &l3PersonaTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L3PersonaPreparation{
			Summary: L3PersonaSearchSummary{
				ProfileID: L3PersonaProfileID, Mode: "shadow", Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []L3PersonaCandidate{persona},
		},
		recordResult: L3PersonaSearchResult{Summary: L3PersonaSearchSummary{
			ProfileID: L3PersonaProfileID, Mode: "shadow", Status: "completed",
			ResultCode: "COMPLETED", FallbackCode: "NONE", FinalCount: 1,
		}},
	}
}

func l3PersonaCandidate(id string, content string) L3PersonaCandidate {
	return L3PersonaCandidate{
		PersonaID: id, Revision: 1, Content: content,
	}
}

type l3PersonaTestRepository struct {
	*fakeRepository
	prepareInput L3PersonaPrepareInput
	recordInput  L3PersonaRecordInput
	preparation  L3PersonaPreparation
	recordResult L3PersonaSearchResult
	prepareErr   error
	recordErr    error
	prepareCalls int
	recordCalls  int
}

func (repository *l3PersonaTestRepository) PrepareL3PersonaSearch(
	_ context.Context,
	input L3PersonaPrepareInput,
) (L3PersonaPreparation, error) {
	repository.prepareCalls++
	repository.prepareInput = input
	return repository.preparation, repository.prepareErr
}

func (repository *l3PersonaTestRepository) RecordL3PersonaSearch(
	_ context.Context,
	input L3PersonaRecordInput,
) (L3PersonaSearchResult, error) {
	repository.recordCalls++
	repository.recordInput = input
	return repository.recordResult, repository.recordErr
}

var _ L3PersonaRepository = (*l3PersonaTestRepository)(nil)
