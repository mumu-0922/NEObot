package usermemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const (
	hybridTestConversation = "22222222-2222-4222-8222-222222222222"
	hybridTestAssistant    = "33333333-3333-4333-8333-333333333333"
)

func TestSearchRelevantWithHybridShadowKeepsV1Authority(t *testing.T) {
	base := hybridV1Repository()
	repository := &hybridTestRepository{
		fakeRepository: base,
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID,
				Status:    "pending", ResultCode: "CANDIDATES_READY",
				FallbackCode: "NONE", BaselineCount: 1,
				ExactCount: 2, BM25Count: 3, VectorCount: 3, RRFCount: 3,
			},
			Candidates: []HybridShadowCandidate{
				hybridCandidate("44444444-4444-4444-8444-444444444444", "Global preference"),
				hybridCandidate("55555555-5555-4555-8555-555555555555", "Project preference"),
				hybridCandidate("66666666-6666-4666-8666-666666666666", "Conversation preference"),
			},
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
			FallbackCode: "NONE", BaselineCount: 1, ExactCount: 2,
			BM25Count: 3, VectorCount: 3, RRFCount: 3,
			RerankCount: 3, FinalCount: 3, OverlapCount: 0,
		},
	}
	provider := &hybridTestProvider{
		embedding: validHybridTestEmbedding(),
		rerank: []ragproviders.RerankResult{
			{Index: 0, RelevanceScore: 0.4},
			{Index: 1, RelevanceScore: 0.2},
			{Index: 2, RelevanceScore: 0.9},
		},
	}
	service := NewService(repository, WithHybridShadowProvider(provider))

	query := "Please keep this concise"
	items, summary, err := service.SearchRelevantWithHybridShadow(
		context.Background(), query, hybridTestConversation, hybridTestAssistant,
		MaxSearchResults,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != base.memories[0].ID {
		t.Fatalf("hybrid changed v1 items = %#v", items)
	}
	if repository.prepareCalls != 1 || repository.recordCalls != 1 ||
		provider.embedCalls != 1 || provider.rerankCalls != 1 {
		t.Fatalf("calls = prepare:%d record:%d embed:%d rerank:%d",
			repository.prepareCalls, repository.recordCalls,
			provider.embedCalls, provider.rerankCalls)
	}
	if repository.prepareInput.QueryText != query ||
		repository.prepareInput.QueryHash != sha256String(query) ||
		repository.prepareInput.QueryEmbeddingState != "ready" ||
		len(repository.prepareInput.QueryEmbedding) != ragproviders.SiliconFlowEmbeddingDimensions ||
		len(repository.prepareInput.Baseline) != 1 ||
		repository.prepareInput.Baseline[0].MemoryID != items[0].ID {
		t.Fatalf("prepare input = %#v", repository.prepareInput)
	}
	wantOrder := []string{
		"66666666-6666-4666-8666-666666666666",
		"44444444-4444-4444-8444-444444444444",
		"55555555-5555-4555-8555-555555555555",
	}
	if len(repository.recordInput.Reranked) != len(wantOrder) ||
		len(repository.recordInput.Final) != len(wantOrder) {
		t.Fatalf("record input = %#v", repository.recordInput)
	}
	for index, memoryID := range wantOrder {
		if repository.recordInput.Reranked[index].MemoryID != memoryID ||
			repository.recordInput.Final[index].MemoryID != memoryID {
			t.Fatalf("record order = %#v", repository.recordInput)
		}
	}
	if summary.Status != "completed" || summary.ResultCode != "OK" ||
		summary.ProfileID != HybridShadowProfileID {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestSearchRelevantWithHybridShadowProviderFailuresDegradeToLexicalRRF(t *testing.T) {
	base := hybridV1Repository()
	repository := &hybridTestRepository{
		fakeRepository: base,
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "QUERY_EMBEDDING_FAILED",
				RRFCount: 1,
			},
			Candidates: []HybridShadowCandidate{
				hybridCandidate("44444444-4444-4444-8444-444444444444", "Lexical fallback"),
			},
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
			FallbackCode: "QUERY_EMBEDDING_FAILED", RRFCount: 1, FinalCount: 1,
		},
	}
	provider := &hybridTestProvider{
		embedErr:  errors.New("private provider error"),
		rerankErr: errors.New("private rerank error"),
	}
	items, summary, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantWithHybridShadow(
		context.Background(), "Please keep this concise",
		hybridTestConversation, hybridTestAssistant, MaxSearchResults,
	)
	if err != nil || len(items) != 1 {
		t.Fatalf("fallback changed v1 = %#v/%v", items, err)
	}
	if repository.prepareInput.QueryEmbeddingState != "failed" ||
		len(repository.prepareInput.QueryEmbedding) != 0 ||
		repository.recordInput.RerankStatus != "fallback" ||
		repository.recordInput.FallbackCode != "QUERY_EMBEDDING_FAILED" ||
		len(repository.recordInput.Reranked) != 0 ||
		len(repository.recordInput.Final) != 1 ||
		summary.FallbackCode != "QUERY_EMBEDDING_FAILED" {
		t.Fatalf("fallback = prepare:%#v record:%#v summary:%#v",
			repository.prepareInput, repository.recordInput, summary)
	}
}

func TestSearchRelevantWithHybridShadowRedactsProviderQueryAndDocuments(t *testing.T) {
	base := hybridV1Repository()
	repository := &hybridTestRepository{
		fakeRepository: base,
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []HybridShadowCandidate{hybridCandidate(
				"44444444-4444-4444-8444-444444444444",
				"api_key=fixture-private-value. Keep answers concise.",
			)},
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
			FallbackCode: "NONE", RRFCount: 1, RerankCount: 1, FinalCount: 1,
		},
	}
	provider := &hybridTestProvider{
		embedding: validHybridTestEmbedding(),
		rerank:    []ragproviders.RerankResult{{Index: 0, RelevanceScore: 1}},
	}
	rawQuery := "password: fixture-secret-value. Please keep this concise."
	_, _, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantWithHybridShadow(
		context.Background(), rawQuery, hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil {
		t.Fatal(err)
	}
	if repository.prepareInput.QueryText != rawQuery ||
		repository.prepareInput.QueryHash != sha256String(rawQuery) {
		t.Fatalf("raw SQL authority changed = %#v", repository.prepareInput)
	}
	providerPayload := provider.embedQuery + "\n" + provider.rerankQuery + "\n" +
		strings.Join(provider.rerankDocuments, "\n")
	for _, forbidden := range []string{"fixture-secret-value", "fixture-private-value"} {
		if strings.Contains(providerPayload, forbidden) {
			t.Fatalf("provider payload retained %q: %q", forbidden, providerPayload)
		}
	}
	for _, required := range []string{"Please keep this concise", "Keep answers concise"} {
		if !strings.Contains(providerPayload, required) {
			t.Fatalf("provider payload lost safe text %q: %q", required, providerPayload)
		}
	}
}

func TestSearchRelevantWithHybridShadowSecretOnlyQueryMakesZeroProviderCalls(t *testing.T) {
	base := hybridV1Repository()
	repository := &hybridTestRepository{
		fakeRepository: base,
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "SECRET_REDACTED",
				RRFCount: 1,
			},
			Candidates: []HybridShadowCandidate{hybridCandidate(
				"44444444-4444-4444-8444-444444444444", "Safe lexical result",
			)},
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
			FallbackCode: "SECRET_REDACTED", RRFCount: 1, FinalCount: 1,
		},
	}
	provider := &hybridTestProvider{
		embedding: validHybridTestEmbedding(),
		rerank:    []ragproviders.RerankResult{{Index: 0, RelevanceScore: 1}},
	}
	rawQuery := "password: fixture-secret-value"
	_, _, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantWithHybridShadow(
		context.Background(), rawQuery, hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.embedCalls != 0 || provider.rerankCalls != 0 ||
		repository.prepareInput.QueryText != rawQuery ||
		repository.prepareInput.QueryEmbeddingState != "redacted" ||
		repository.recordInput.FallbackCode != "SECRET_REDACTED" {
		t.Fatalf("secret query egress = provider:%#v prepare:%#v record:%#v",
			provider, repository.prepareInput, repository.recordInput)
	}
}

func TestSearchRelevantWithHybridShadowSecretOnlyDocumentSkipsRerank(t *testing.T) {
	base := hybridV1Repository()
	repository := &hybridTestRepository{
		fakeRepository: base,
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []HybridShadowCandidate{hybridCandidate(
				"44444444-4444-4444-8444-444444444444",
				"api_key=fixture-private-value",
			)},
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
			FallbackCode: "SECRET_REDACTED", RRFCount: 1, FinalCount: 1,
		},
	}
	provider := &hybridTestProvider{embedding: validHybridTestEmbedding()}
	_, _, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantWithHybridShadow(
		context.Background(), "safe query", hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider.embedCalls != 1 || provider.rerankCalls != 0 ||
		repository.recordInput.RerankStatus != "fallback" ||
		repository.recordInput.FallbackCode != "SECRET_REDACTED" {
		t.Fatalf("secret document rerank = provider:%#v record:%#v",
			provider, repository.recordInput)
	}
}

func TestSearchRelevantWithHybridShadowRerankCutoffStillRecordsRRF(t *testing.T) {
	base := hybridV1Repository()
	repository := &hybridTestRepository{
		fakeRepository: base,
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []HybridShadowCandidate{
				hybridCandidate("44444444-4444-4444-8444-444444444444", "RRF survives timeout"),
			},
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
			FallbackCode: "HARD_CUTOFF", RRFCount: 1, FinalCount: 1,
		},
	}
	provider := &hybridTestProvider{
		embedding:                 validHybridTestEmbedding(),
		returnRerankAfterDeadline: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	items, summary, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantWithHybridShadow(
		ctx, "Please keep this concise", hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil || len(items) != 1 || time.Since(started) >= 300*time.Millisecond {
		t.Fatalf("cutoff result = items:%#v summary:%#v duration:%s err:%v",
			items, summary, time.Since(started), err)
	}
	if repository.recordCalls != 1 || repository.recordInput.RerankStatus != "fallback" ||
		repository.recordInput.FallbackCode != "HARD_CUTOFF" ||
		len(repository.recordInput.Final) != 1 {
		t.Fatalf("cutoff was not recorded = %#v", repository.recordInput)
	}
}

func TestHybridQueryEmbeddingRejectsResultReturnedAfterCutoff(t *testing.T) {
	provider := &hybridTestProvider{
		embedding:                validHybridTestEmbedding(),
		returnEmbedAfterDeadline: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	vector, state := NewService(
		hybridV1Repository(),
		WithHybridShadowProvider(provider),
	).hybridQueryEmbedding(ctx, "late query")
	if state != "cutoff" || len(vector) != 0 {
		t.Fatalf("late query embedding = state:%q vector:%d", state, len(vector))
	}
}

func TestSelectHybridShadowFinalEnforcesFiveAndHardTokenBudget(t *testing.T) {
	candidates := []HybridShadowCandidate{
		hybridCandidate("11111111-1111-4111-8111-111111111111", strings.Repeat("中", 500)),
		hybridCandidate("22222222-2222-4222-8222-222222222222", strings.Repeat("中", 100)),
		hybridCandidate("33333333-3333-4333-8333-333333333333", strings.Repeat("中", 100)),
		hybridCandidate("44444444-4444-4444-8444-444444444444", strings.Repeat("中", 100)),
		hybridCandidate("55555555-5555-4555-8555-555555555555", strings.Repeat("中", 100)),
		hybridCandidate("66666666-6666-4666-8666-666666666666", "small fallback"),
	}
	selected, tokens := selectHybridShadowFinal(candidates)
	if len(selected) != 4 || tokens <= hybridShadowTargetTokens ||
		tokens > hybridShadowMaximumTokens ||
		selected[0].MemoryID != candidates[1].MemoryID {
		t.Fatalf("budget selection = %#v tokens=%d", selected, tokens)
	}
}

func TestSearchRelevantWithHybridShadowFailureNeverReturnsPrivateError(t *testing.T) {
	base := hybridV1Repository()
	repository := &hybridTestRepository{
		fakeRepository: base,
		prepareErr:     errors.New("query and memory content leaked"),
	}
	items, summary, err := NewService(repository).SearchRelevantWithHybridShadow(
		context.Background(), "Please keep this concise",
		hybridTestConversation, hybridTestAssistant, MaxSearchResults,
	)
	if err != nil || len(items) != 1 || summary.Status != "failed" ||
		summary.ResultCode != "PREPARE_FAILED" ||
		strings.Contains(summary.ResultCode, "query") {
		t.Fatalf("failure changed authority = %#v/%#v/%v", items, summary, err)
	}
}

func TestSanitizeHybridShadowSummaryBoundsDiagnostics(t *testing.T) {
	summary := sanitizeHybridShadowSummary(HybridShadowSummary{
		ProfileID: "forged", Status: "private status", ResultCode: "raw query",
		FallbackCode: "private fallback", BaselineCount: 99, ExactCount: 99,
		BM25Count: 99, VectorCount: 99, RRFCount: 99, RerankCount: 99,
		FinalCount: 99, OverlapCount: 99, EstimatedTokens: 9999,
		DurationMillis: 999999,
	})
	if summary.ProfileID != HybridShadowProfileID || summary.Status != "failed" ||
		summary.ResultCode != "HYBRID_FAILED" || summary.FallbackCode != "NONE" ||
		summary.BaselineCount != MaxSearchResults || summary.ExactCount != 20 ||
		summary.BM25Count != 30 || summary.VectorCount != 30 ||
		summary.RRFCount != MaxHybridShadowResults ||
		summary.RerankCount != MaxHybridShadowResults ||
		summary.FinalCount != hybridShadowFinalLimit ||
		summary.OverlapCount != MaxSearchResults ||
		summary.EstimatedTokens != hybridShadowMaximumTokens ||
		summary.DurationMillis != 120000 {
		t.Fatalf("sanitized summary = %#v", summary)
	}
}

func hybridV1Repository() *fakeRepository {
	repository := &fakeRepository{
		settings:      Settings{Enabled: true, SearchEnabled: true},
		settingsFound: true,
		memories: []Memory{
			memoryFixture(
				"11111111-1111-4111-8111-111111111111",
				"preference",
				"Keep answers concise",
				5,
			),
		},
	}
	repository.memories[0].Revision = 3
	repository.memories[0].ScopeType = "global"
	return repository
}

func hybridCandidate(id string, content string) HybridShadowCandidate {
	return HybridShadowCandidate{
		MemoryID: id, Revision: 1, ScopeType: "global", Content: content,
	}
}

func validHybridTestEmbedding() ragproviders.QueryEmbedding {
	vector := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	vector[0] = 1
	return ragproviders.QueryEmbedding{
		Vector: vector, ModelID: ragproviders.SiliconFlowEmbeddingModel,
		Dimensions: ragproviders.SiliconFlowEmbeddingDimensions,
	}
}

func sha256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

type hybridTestRepository struct {
	*fakeRepository
	prepareInput  HybridShadowPrepareInput
	recordInput   HybridShadowRecordInput
	preparation   HybridShadowPreparation
	recordSummary HybridShadowSummary
	prepareErr    error
	recordErr     error
	prepareCalls  int
	recordCalls   int
}

func (repository *hybridTestRepository) PrepareHybridShadow(
	_ context.Context,
	input HybridShadowPrepareInput,
) (HybridShadowPreparation, error) {
	repository.prepareCalls++
	repository.prepareInput = input
	return repository.preparation, repository.prepareErr
}

func (repository *hybridTestRepository) RecordHybridShadow(
	_ context.Context,
	input HybridShadowRecordInput,
) (HybridShadowSummary, error) {
	repository.recordCalls++
	repository.recordInput = input
	return repository.recordSummary, repository.recordErr
}

type hybridTestProvider struct {
	embedding                 ragproviders.QueryEmbedding
	embedErr                  error
	rerank                    []ragproviders.RerankResult
	rerankErr                 error
	blockRerankUntilCanceled  bool
	returnEmbedAfterDeadline  bool
	returnRerankAfterDeadline bool
	embedQuery                string
	rerankQuery               string
	rerankDocuments           []string
	embedCalls                int
	rerankCalls               int
}

func (provider *hybridTestProvider) EmbedQuery(
	ctx context.Context,
	query string,
) (ragproviders.QueryEmbedding, error) {
	provider.embedCalls++
	provider.embedQuery = query
	if provider.returnEmbedAfterDeadline {
		<-ctx.Done()
	}
	return provider.embedding, provider.embedErr
}

func (provider *hybridTestProvider) Rerank(
	ctx context.Context,
	query string,
	documents []string,
) ([]ragproviders.RerankResult, error) {
	provider.rerankCalls++
	provider.rerankQuery = query
	provider.rerankDocuments = append([]string(nil), documents...)
	if provider.blockRerankUntilCanceled {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if provider.returnRerankAfterDeadline {
		<-ctx.Done()
		return []ragproviders.RerankResult{{Index: 0, RelevanceScore: 1}}, nil
	}
	return append([]ragproviders.RerankResult(nil), provider.rerank...), provider.rerankErr
}

var (
	_ HybridShadowRepository = (*hybridTestRepository)(nil)
	_ HybridShadowProvider   = (*hybridTestProvider)(nil)
)
