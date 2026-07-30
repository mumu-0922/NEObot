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
	service := NewService(repository, WithHybridShadowProvider(provider),
		WithHybridShadowRelevancePolicy(HybridShadowCalibrationPolicy()))

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

func TestSearchRelevantWithHybridShadowRequiresExplicitRelevancePolicy(t *testing.T) {
	repository := &hybridTestRepository{fakeRepository: hybridV1Repository()}
	provider := &hybridTestProvider{embedding: validHybridTestEmbedding()}
	items, summary, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantWithHybridShadow(
		context.Background(), "keep answers concise", hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil || len(items) != 1 || summary.ResultCode != "POLICY_UNAVAILABLE" ||
		repository.prepareCalls != 0 || repository.admissionCalls != 0 ||
		provider.embedCalls != 0 || provider.rerankCalls != 0 {
		t.Fatalf("missing policy authority = items:%#v summary:%#v repo:%#v provider:%#v err:%v",
			items, summary, repository, provider, err)
	}
}

func TestSearchRelevantAfterMemoryToolCallReturnsOnlyReauthorizedFinal(t *testing.T) {
	query := "  How should project answers be written?  "
	repository := &hybridTestRepository{
		fakeRepository: hybridV1Repository(),
		preparation: HybridShadowPreparation{
			Summary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "pending",
				ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
			},
			Candidates: []HybridShadowCandidate{hybridCandidate(
				"44444444-4444-4444-8444-444444444444",
				"Keep project answers concise",
			)},
		},
		recordSummary: HybridShadowSummary{
			ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
			FallbackCode: "NONE", RRFCount: 1, RerankCount: 1, FinalCount: 1,
		},
		hydrated: []Memory{{
			ID: "44444444-4444-4444-8444-444444444444", Revision: 1,
			ScopeType: "global", Type: "preference", Content: "Keep project answers concise",
		}},
	}
	provider := &hybridTestProvider{
		embedding: validHybridTestEmbedding(),
		rerank:    []ragproviders.RerankResult{{Index: 0, RelevanceScore: 0.9}},
	}
	result := NewService(
		repository,
		WithHybridShadowProvider(provider),
	).SearchRelevantAfterMemoryToolCall(context.Background(), HybridMemoryToolSearchInput{
		ConversationID: hybridTestConversation, AssistantMessageID: hybridTestAssistant,
		Query:           query,
		ContractVersion: HybridMemoryToolContractVersion,
		ContractSHA256:  HybridMemoryToolContractSHA256,
	})
	if result.FailureCategory != "" || len(result.Memories) != 1 ||
		result.Memories[0].Content != "Keep project answers concise" ||
		repository.hydrateCalls != 1 || repository.fakeRepository.listCalls != 0 ||
		len(repository.fakeRepository.markedUsed) != 0 {
		t.Fatalf("Memory Tool result = %#v repository=%#v", result, repository)
	}
	if len(repository.prepareInput.Baseline) != 0 ||
		repository.prepareInput.QueryHash != sha256String(query) ||
		repository.prepareInput.QueryText != query ||
		repository.hydrateInput.ObservationID != repository.preparation.ObservationID ||
		repository.hydrateInput.AssistantMessageID != hybridTestAssistant {
		t.Fatalf("Memory Tool authority = prepare:%#v hydrate:%#v",
			repository.prepareInput, repository.hydrateInput)
	}
}

func TestSearchRelevantAfterMemoryToolCallFailsClosedOnStaleOrRedactedFinal(t *testing.T) {
	for _, test := range []struct {
		name     string
		hydrated []Memory
		hydrate  error
		want     string
	}{
		{name: "stale final", hydrate: errors.New("private stale detail"), want: "authority_stale"},
		{name: "secret only final", hydrated: []Memory{{
			ID: "44444444-4444-4444-8444-444444444444", Revision: 1,
			ScopeType: "global", Type: "fact", Content: "password: fixture-secret-value",
		}}, want: "secret_redacted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &hybridTestRepository{
				fakeRepository: hybridV1Repository(),
				preparation: HybridShadowPreparation{
					Summary: HybridShadowSummary{
						ProfileID: HybridShadowProfileID, Status: "pending",
						ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
					},
					Candidates: []HybridShadowCandidate{hybridCandidate(
						"44444444-4444-4444-8444-444444444444", "Safe rerank body",
					)},
				},
				recordSummary: HybridShadowSummary{
					ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
					FallbackCode: "NONE", RRFCount: 1, RerankCount: 1, FinalCount: 1,
				},
				hydrated: test.hydrated, hydrateErr: test.hydrate,
			}
			provider := &hybridTestProvider{
				embedding: validHybridTestEmbedding(),
				rerank:    []ragproviders.RerankResult{{Index: 0, RelevanceScore: 1}},
			}
			result := NewService(
				repository,
				WithHybridShadowProvider(provider),
			).SearchRelevantAfterMemoryToolCall(context.Background(), HybridMemoryToolSearchInput{
				ConversationID: hybridTestConversation, AssistantMessageID: hybridTestAssistant,
				Query: "safe query", ContractVersion: HybridMemoryToolContractVersion,
				ContractSHA256: HybridMemoryToolContractSHA256,
			})
			if result.FailureCategory != test.want || len(result.Memories) != 0 {
				t.Fatalf("fail-closed result = %#v", result)
			}
		})
	}
}

func TestSearchRelevantWithHybridShadowAppliesBothRelevanceGates(t *testing.T) {
	newRepository := func(similarity float64, fallback string) *hybridTestRepository {
		admission := HybridShadowAdmission{
			CandidateCount: 1, VectorCandidateCount: 1,
			MaximumVectorSimilarity: similarity,
		}
		return &hybridTestRepository{
			fakeRepository: hybridV1Repository(),
			preparation: HybridShadowPreparation{
				Summary: HybridShadowSummary{
					ProfileID: HybridShadowProfileID, Status: "pending",
					ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
				},
				Candidates: []HybridShadowCandidate{hybridCandidate(
					"44444444-4444-4444-8444-444444444444", "Relevant only when admitted",
				)},
			},
			admission: &admission,
			recordSummary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
				FallbackCode: fallback, RRFCount: 1,
			},
		}
	}
	t.Run("pre-rerank admission blocks document egress", func(t *testing.T) {
		repository := newRepository(0.2, "RELEVANCE_ABSTAINED")
		provider := &hybridTestProvider{
			embedding: validHybridTestEmbedding(),
			rerank:    []ragproviders.RerankResult{{Index: 0, RelevanceScore: 0.99}},
		}
		policy := HybridShadowRelevancePolicy{
			ID: HybridRelevanceFrozenPolicyID, Mode: hybridPolicyModeFrozen,
			MemoryIntentRequired: true, MinimumMemoryIntentMargin: -1,
			MinimumProviderSimilarity: 0.5, MinimumFinalRelevanceScore: 0.5,
		}
		_, _, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
			WithHybridShadowRelevancePolicy(policy),
		).SearchRelevantWithHybridShadow(
			context.Background(), "safe query", hybridTestConversation,
			hybridTestAssistant, MaxSearchResults,
		)
		if err != nil || provider.embedCalls != 1 || provider.rerankCalls != 0 ||
			repository.admissionCalls != 1 || len(repository.recordInput.Reranked) != 0 ||
			len(repository.recordInput.Final) != 0 ||
			repository.recordInput.FallbackCode != "RELEVANCE_ABSTAINED" {
			t.Fatalf("pre-rerank gate = repo:%#v provider:%#v err:%v", repository, provider, err)
		}
	})
	t.Run("post-rerank threshold emits explicit abstention", func(t *testing.T) {
		repository := newRepository(0.8, "RELEVANCE_FINAL_ABSTAINED")
		provider := &hybridTestProvider{
			embedding: validHybridTestEmbedding(),
			rerank:    []ragproviders.RerankResult{{Index: 0, RelevanceScore: 0.8}},
		}
		policy := HybridShadowRelevancePolicy{
			ID: HybridRelevanceFrozenPolicyID, Mode: hybridPolicyModeFrozen,
			MemoryIntentRequired: true, MinimumMemoryIntentMargin: -1,
			MinimumProviderSimilarity: 0.5, MinimumFinalRelevanceScore: 0.9,
		}
		_, _, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
			WithHybridShadowRelevancePolicy(policy),
		).SearchRelevantWithHybridShadow(
			context.Background(), "safe query", hybridTestConversation,
			hybridTestAssistant, MaxSearchResults,
		)
		if err != nil || provider.rerankCalls != 1 ||
			len(repository.recordInput.Reranked) != 1 ||
			len(repository.recordInput.Final) != 0 ||
			repository.recordInput.EstimatedTokens != 0 ||
			repository.recordInput.FallbackCode != "RELEVANCE_FINAL_ABSTAINED" {
			t.Fatalf("post-rerank gate = repo:%#v provider:%#v err:%v", repository, provider, err)
		}
	})
}

func TestSearchRelevantWithHybridShadowMemoryIntentGatePrecedesMemoryEgress(t *testing.T) {
	newRepository := func(fallback string) *hybridTestRepository {
		return &hybridTestRepository{
			fakeRepository: hybridV1Repository(),
			preparation: HybridShadowPreparation{
				Summary: HybridShadowSummary{
					ProfileID: HybridShadowProfileID, Status: "pending",
					ResultCode: "CANDIDATES_READY", FallbackCode: "NONE", RRFCount: 1,
				},
				Candidates: []HybridShadowCandidate{hybridCandidate(
					"44444444-4444-4444-8444-444444444444",
					"Memory plaintext must not leave on an unrelated query",
				)},
			},
			recordSummary: HybridShadowSummary{
				ProfileID: HybridShadowProfileID, Status: "completed", ResultCode: "OK",
				FallbackCode: fallback, RRFCount: 1,
			},
		}
	}
	policy := HybridShadowRelevancePolicy{
		ID: HybridRelevanceFrozenPolicyID, Mode: hybridPolicyModeFrozen,
		MemoryIntentRequired: true, MinimumMemoryIntentMargin: 0,
		MinimumProviderSimilarity: -1, MinimumFinalRelevanceScore: 0,
	}
	t.Run("low margin", func(t *testing.T) {
		repository := newRepository("MEMORY_INTENT_ABSTAINED")
		provider := &hybridTestProvider{
			embedding: validHybridTestEmbedding(),
			memoryIntent: ragproviders.MemoryIntentSignal{
				AnchorVersion: ragproviders.MemoryIntentAnchorVersion,
				AnchorSHA256:  ragproviders.MemoryIntentAnchorSHA256,
				PositiveScore: 0.2, NegativeScore: 0.8, Margin: -0.6,
			},
		}
		_, _, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
			WithHybridShadowRelevancePolicy(policy),
		).SearchRelevantWithHybridShadow(
			context.Background(), "public fact", hybridTestConversation,
			hybridTestAssistant, MaxSearchResults,
		)
		if err != nil || provider.memoryIntentCalls != 1 || provider.rerankCalls != 0 ||
			repository.admissionCalls != 0 || len(repository.recordInput.Final) != 0 ||
			repository.recordInput.FallbackCode != "MEMORY_INTENT_ABSTAINED" {
			t.Fatalf("intent abstention = repo:%#v provider:%#v err:%v", repository, provider, err)
		}
	})
	t.Run("invalid signal", func(t *testing.T) {
		repository := newRepository("MEMORY_INTENT_FAILED")
		provider := &hybridTestProvider{
			embedding: validHybridTestEmbedding(),
			memoryIntent: ragproviders.MemoryIntentSignal{
				AnchorVersion: "drifted", AnchorSHA256: ragproviders.MemoryIntentAnchorSHA256,
				PositiveScore: 0.8, NegativeScore: 0.2, Margin: 0.6,
			},
		}
		_, _, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
			WithHybridShadowRelevancePolicy(policy),
		).SearchRelevantWithHybridShadow(
			context.Background(), "personal query", hybridTestConversation,
			hybridTestAssistant, MaxSearchResults,
		)
		if err != nil || provider.memoryIntentCalls != 1 || provider.rerankCalls != 0 ||
			repository.admissionCalls != 0 ||
			repository.recordInput.FallbackCode != "MEMORY_INTENT_FAILED" {
			t.Fatalf("intent failure = repo:%#v provider:%#v err:%v", repository, provider, err)
		}
	})
}

func TestSearchRelevantWithHybridShadowProviderFailuresAbstain(t *testing.T) {
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
			FallbackCode: "RELEVANCE_ADMISSION_UNAVAILABLE", RRFCount: 1,
		},
	}
	provider := &hybridTestProvider{
		embedErr:  errors.New("private provider error"),
		rerankErr: errors.New("private rerank error"),
	}
	items, summary, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
		WithHybridShadowRelevancePolicy(HybridShadowCalibrationPolicy()),
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
		repository.recordInput.FallbackCode != "RELEVANCE_ADMISSION_UNAVAILABLE" ||
		len(repository.recordInput.Reranked) != 0 ||
		len(repository.recordInput.Final) != 0 ||
		summary.FallbackCode != "RELEVANCE_ADMISSION_UNAVAILABLE" {
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
		WithHybridShadowRelevancePolicy(HybridShadowCalibrationPolicy()),
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
			FallbackCode: "RELEVANCE_ADMISSION_UNAVAILABLE", RRFCount: 1,
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
		WithHybridShadowRelevancePolicy(HybridShadowCalibrationPolicy()),
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
		repository.recordInput.FallbackCode != "RELEVANCE_ADMISSION_UNAVAILABLE" ||
		len(repository.recordInput.Final) != 0 {
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
		WithHybridShadowRelevancePolicy(HybridShadowCalibrationPolicy()),
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

func TestSearchRelevantWithHybridShadowRerankCutoffAbstains(t *testing.T) {
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
			FallbackCode: "HARD_CUTOFF", RRFCount: 1,
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
		WithHybridShadowRelevancePolicy(HybridShadowCalibrationPolicy()),
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
		len(repository.recordInput.Final) != 0 {
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
	scored := make([]hybridShadowScoredCandidate, len(candidates))
	for index, candidate := range candidates {
		scored[index] = hybridShadowScoredCandidate{Candidate: candidate, RelevanceScore: 1}
	}
	selected, tokens := selectHybridShadowFinal(scored, 0)
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
	items, summary, err := NewService(
		repository,
		WithHybridShadowRelevancePolicy(HybridShadowCalibrationPolicy()),
	).SearchRelevantWithHybridShadow(
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
	prepareInput   HybridShadowPrepareInput
	recordInput    HybridShadowRecordInput
	preparation    HybridShadowPreparation
	recordSummary  HybridShadowSummary
	admission      *HybridShadowAdmission
	admissionErr   error
	prepareErr     error
	recordErr      error
	hydrated       []Memory
	hydrateInput   HybridFinalHydrationInput
	hydrateErr     error
	prepareCalls   int
	admissionCalls int
	recordCalls    int
	hydrateCalls   int
}

func (repository *hybridTestRepository) HydrateHybridFinal(
	_ context.Context,
	input HybridFinalHydrationInput,
) ([]Memory, error) {
	repository.hydrateCalls++
	repository.hydrateInput = input
	return append([]Memory(nil), repository.hydrated...), repository.hydrateErr
}

func (repository *hybridTestRepository) PrepareHybridShadow(
	_ context.Context,
	input HybridShadowPrepareInput,
) (HybridShadowPreparation, error) {
	repository.prepareCalls++
	repository.prepareInput = input
	if repository.preparation.ObservationID == "" {
		repository.preparation.ObservationID = input.ObservationID
	}
	return repository.preparation, repository.prepareErr
}

func (repository *hybridTestRepository) AuthorizeHybridRerank(
	_ context.Context,
	_ HybridShadowAdmissionInput,
) (HybridShadowAdmission, error) {
	repository.admissionCalls++
	if repository.admissionErr != nil {
		return HybridShadowAdmission{}, repository.admissionErr
	}
	if repository.admission != nil {
		return *repository.admission, nil
	}
	return HybridShadowAdmission{
		CandidateCount:          len(repository.preparation.Candidates),
		VectorCandidateCount:    len(repository.preparation.Candidates),
		MaximumVectorSimilarity: 1,
	}, nil
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
	memoryIntent              ragproviders.MemoryIntentSignal
	memoryIntentErr           error
	blockRerankUntilCanceled  bool
	returnEmbedAfterDeadline  bool
	returnRerankAfterDeadline bool
	embedQuery                string
	rerankQuery               string
	rerankDocuments           []string
	embedCalls                int
	rerankCalls               int
	memoryIntentCalls         int
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

func (provider *hybridTestProvider) ClassifyMemoryIntent(
	_ context.Context,
	_ string,
) (ragproviders.MemoryIntentSignal, error) {
	provider.memoryIntentCalls++
	if provider.memoryIntent.AnchorVersion == "" && provider.memoryIntentErr == nil {
		return ragproviders.MemoryIntentSignal{
			AnchorVersion: ragproviders.MemoryIntentAnchorVersion,
			AnchorSHA256:  ragproviders.MemoryIntentAnchorSHA256,
			PositiveScore: 1, NegativeScore: 0, Margin: 1,
		}, nil
	}
	return provider.memoryIntent, provider.memoryIntentErr
}

var (
	_ HybridShadowRepository          = (*hybridTestRepository)(nil)
	_ HybridShadowAdmissionRepository = (*hybridTestRepository)(nil)
	_ HybridFinalRepository           = (*hybridTestRepository)(nil)
	_ HybridShadowProvider            = (*hybridTestProvider)(nil)
)
