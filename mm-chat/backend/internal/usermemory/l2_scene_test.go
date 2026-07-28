package usermemory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestSearchRelevantL2ScenesShadowRecordsWithoutInjection(t *testing.T) {
	repository := &l2SceneTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L2ScenePreparation{
			Summary: L2SceneSearchSummary{
				ProfileID: L2SceneProfileID, Mode: "shadow", Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE",
				ExactCount: 2, BM25Count: 2, VectorCount: 1, RRFCount: 2,
			},
			Candidates: []L2SceneCandidate{
				l2SceneCandidate("44444444-4444-4444-8444-444444444444", "Global scene"),
				l2SceneCandidate("55555555-5555-4555-8555-555555555555", "Project scene"),
			},
		},
		recordResult: L2SceneSearchResult{
			Summary: L2SceneSearchSummary{
				ProfileID: L2SceneProfileID, Mode: "shadow", Status: "completed",
				ResultCode: "COMPLETED", FallbackCode: "NONE",
				RerankCount: 2, FinalCount: 2, InjectedCount: 0,
			},
			Scenes: []L2SceneCandidate{
				l2SceneCandidate("55555555-5555-4555-8555-555555555555", "Project scene"),
				l2SceneCandidate("44444444-4444-4444-8444-444444444444", "Global scene"),
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
	).SearchRelevantL2Scenes(
		context.Background(), "project preferences", hybridTestConversation,
		hybridTestAssistant, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Scenes) != 0 || result.Summary.Mode != "shadow" ||
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
	if len(repository.recordInput.Final) != 2 ||
		repository.recordInput.Final[0].SceneID !=
			"55555555-5555-4555-8555-555555555555" ||
		repository.recordInput.RerankStatus != "applied" {
		t.Fatalf("record input = %#v", repository.recordInput)
	}
}

func TestSearchRelevantL2ScenesActiveReturnsOnlyAuthorizedRecordResult(t *testing.T) {
	scene := l2SceneCandidate(
		"44444444-4444-4444-8444-444444444444",
		"Current authorized scene",
	)
	repository := &l2SceneTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L2ScenePreparation{
			Summary: L2SceneSearchSummary{
				ProfileID: L2SceneProfileID, Mode: "active", Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []L2SceneCandidate{scene},
		},
		recordResult: L2SceneSearchResult{
			Summary: L2SceneSearchSummary{
				ProfileID: L2SceneProfileID, Mode: "active", Status: "completed",
				ResultCode: "COMPLETED", FallbackCode: "NONE",
				FinalCount: 1, InjectedCount: 1,
			},
			Scenes: []L2SceneCandidate{scene},
		},
	}
	result, err := NewService(repository).SearchRelevantL2Scenes(
		context.Background(), "current project", hybridTestConversation,
		hybridTestAssistant, true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !repository.prepareInput.ActiveRequested || len(result.Scenes) != 1 ||
		result.Scenes[0].Content != scene.Content ||
		result.Summary.InjectedCount != 1 {
		t.Fatalf("active result = %#v prepare=%#v", result, repository.prepareInput)
	}
}

func TestSearchRelevantL2ScenesDatabaseAuthorityFailureFailsOpen(t *testing.T) {
	repository := &l2SceneTestRepository{
		fakeRepository: &fakeRepository{},
		prepareErr:     errors.New("MEMORY_L2_SCENE_READER_DISABLED private detail"),
	}
	result, err := NewService(repository).SearchRelevantL2Scenes(
		context.Background(), "current project", hybridTestConversation,
		hybridTestAssistant, true,
	)
	if err != nil || len(result.Scenes) != 0 || result.Summary.Status != "failed" ||
		result.Summary.ResultCode != "PREPARE_FAILED" ||
		strings.Contains(result.Summary.ResultCode, "private") {
		t.Fatalf("authority failure = %#v/%v", result, err)
	}
}

func TestSearchRelevantL2ScenesProviderFailureUsesRRFFallback(t *testing.T) {
	scene := l2SceneCandidate(
		"44444444-4444-4444-8444-444444444444",
		"Lexical fallback scene",
	)
	repository := &l2SceneTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L2ScenePreparation{
			Summary: L2SceneSearchSummary{
				ProfileID: L2SceneProfileID, Mode: "shadow", Status: "pending",
				ResultCode:   "CANDIDATES_READY",
				FallbackCode: "QUERY_EMBEDDING_FAILED", RRFCount: 1,
			},
			Candidates: []L2SceneCandidate{scene},
		},
		recordResult: L2SceneSearchResult{Summary: L2SceneSearchSummary{
			ProfileID: L2SceneProfileID, Mode: "shadow", Status: "completed",
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
	).SearchRelevantL2Scenes(
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

func TestSearchRelevantL2ScenesSecretQueryAndDocumentMakeZeroCalls(t *testing.T) {
	t.Run("query", func(t *testing.T) {
		repository := l2SceneFallbackRepository("Safe lexical Scene")
		provider := &hybridTestProvider{embedding: validHybridTestEmbedding()}
		_, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
		).SearchRelevantL2Scenes(
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
		repository := l2SceneFallbackRepository("api_key=fixture-private-value")
		provider := &hybridTestProvider{embedding: validHybridTestEmbedding()}
		_, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
		).SearchRelevantL2Scenes(
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

func TestSearchRelevantL2ScenesLateRerankFallsBackBeforeCallerDeadline(t *testing.T) {
	repository := l2SceneFallbackRepository("RRF survives timeout")
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
	).SearchRelevantL2Scenes(
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

func TestSelectL2SceneFinalEnforcesTwoAndFiveHundredTokens(t *testing.T) {
	candidates := []L2SceneCandidate{
		l2SceneCandidate("11111111-1111-4111-8111-111111111111", strings.Repeat("中", 300)),
		l2SceneCandidate("22222222-2222-4222-8222-222222222222", strings.Repeat("中", 100)),
		l2SceneCandidate("33333333-3333-4333-8333-333333333333", strings.Repeat("中", 100)),
		l2SceneCandidate("44444444-4444-4444-8444-444444444444", "unused"),
	}
	selected, tokens := selectL2SceneFinal(candidates)
	if len(selected) != 2 || tokens > L2SceneMaximumTokens ||
		selected[0].SceneID != candidates[1].SceneID ||
		selected[1].SceneID != candidates[2].SceneID {
		t.Fatalf("budget selection = %#v tokens=%d", selected, tokens)
	}
}

func TestSearchRelevantL2ScenesRejectsUnmatchedActiveInjectionCount(t *testing.T) {
	repository := l2SceneFallbackRepository("Current scene")
	repository.preparation.Summary.Mode = "active"
	repository.recordResult.Summary.Mode = "active"
	repository.recordResult.Summary.InjectedCount = 0
	repository.recordResult.Scenes = repository.preparation.Candidates
	result, err := NewService(repository).SearchRelevantL2Scenes(
		context.Background(), "current project", hybridTestConversation,
		hybridTestAssistant, true,
	)
	if err != nil || len(result.Scenes) != 0 {
		t.Fatalf("mismatched active result = %#v/%v", result, err)
	}
}

func TestSanitizeL2SceneSummaryBoundsDiagnostics(t *testing.T) {
	summary := sanitizeL2SceneSummary(L2SceneSearchSummary{
		ProfileID: "forged", Mode: "private", Status: "private",
		ResultCode: "raw query", FallbackCode: "private fallback",
		ExactCount: 99, BM25Count: 99, VectorCount: 99, RRFCount: 99,
		RerankCount: 99, FinalCount: 99, InjectedCount: 99,
		EstimatedTokens: 9999, DurationMillis: 999999,
	})
	if summary.ProfileID != L2SceneProfileID || summary.Mode != "shadow" ||
		summary.Status != "failed" || summary.ResultCode != "L2_SCENE_FAILED" ||
		summary.FallbackCode != "NONE" || summary.ExactCount != 20 ||
		summary.BM25Count != 30 || summary.VectorCount != 30 ||
		summary.RRFCount != L2SceneCandidateLimit ||
		summary.RerankCount != L2SceneCandidateLimit ||
		summary.FinalCount != L2SceneFinalLimit ||
		summary.InjectedCount != L2SceneFinalLimit ||
		summary.EstimatedTokens != L2SceneMaximumTokens ||
		summary.DurationMillis != 120000 {
		t.Fatalf("sanitized summary = %#v", summary)
	}
}

func l2SceneFallbackRepository(content string) *l2SceneTestRepository {
	scene := l2SceneCandidate("44444444-4444-4444-8444-444444444444", content)
	return &l2SceneTestRepository{
		fakeRepository: &fakeRepository{},
		preparation: L2ScenePreparation{
			Summary: L2SceneSearchSummary{
				ProfileID: L2SceneProfileID, Mode: "shadow", Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []L2SceneCandidate{scene},
		},
		recordResult: L2SceneSearchResult{Summary: L2SceneSearchSummary{
			ProfileID: L2SceneProfileID, Mode: "shadow", Status: "completed",
			ResultCode: "COMPLETED", FallbackCode: "NONE", FinalCount: 1,
		}},
	}
}

func l2SceneCandidate(id string, content string) L2SceneCandidate {
	return L2SceneCandidate{
		SceneID: id, Revision: 1, ScopeType: "global", Content: content,
	}
}

type l2SceneTestRepository struct {
	*fakeRepository
	prepareInput L2ScenePrepareInput
	recordInput  L2SceneRecordInput
	preparation  L2ScenePreparation
	recordResult L2SceneSearchResult
	prepareErr   error
	recordErr    error
	prepareCalls int
	recordCalls  int
}

func (repository *l2SceneTestRepository) PrepareL2SceneSearch(
	_ context.Context,
	input L2ScenePrepareInput,
) (L2ScenePreparation, error) {
	repository.prepareCalls++
	repository.prepareInput = input
	return repository.preparation, repository.prepareErr
}

func (repository *l2SceneTestRepository) RecordL2SceneSearch(
	_ context.Context,
	input L2SceneRecordInput,
) (L2SceneSearchResult, error) {
	repository.recordCalls++
	repository.recordInput = input
	return repository.recordResult, repository.recordErr
}

var _ L2SceneRepository = (*l2SceneTestRepository)(nil)
