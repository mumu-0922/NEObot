package usermemory

import (
	"context"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const hybridMemoryToolRouteTestModel = "configured-main-model"

func TestSearchRelevantWithMemoryToolRouteGatesUnchangedBGEFinal(t *testing.T) {
	for _, test := range []struct {
		name       string
		useMemory  bool
		finalCount int
		fallback   string
	}{
		{name: "called", useMemory: true, finalCount: 3, fallback: "NONE"},
		{name: "not called", finalCount: 0, fallback: "MEMORY_TOOL_ROUTE_ABSTAINED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := cloudJudgeTestRepository()
			provider := &hybridTestProvider{
				embedding: validHybridTestEmbedding(),
				rerank: []ragproviders.RerankResult{
					{Index: 0, RelevanceScore: 1},
					{Index: 1, RelevanceScore: 0.9},
					{Index: 2, RelevanceScore: 0.8},
				},
			}
			router := &hybridTestMemoryToolRouter{
				result: validHybridMemoryToolRouteResult(test.useMemory),
			}
			_, _, err := NewService(
				repository,
				WithHybridShadowProvider(provider),
				WithHybridMemoryToolRouter(router),
				WithHybridShadowRelevancePolicy(
					HybridShadowMemoryToolRouteCalibrationPolicy(
						hybridMemoryToolRouteTestModel,
					),
				),
			).SearchRelevantWithHybridShadow(
				context.Background(), "Should I use my saved preference?",
				hybridTestConversation, hybridTestAssistant, MaxSearchResults,
			)
			if err != nil || router.calls != 1 || provider.rerankCalls != 1 ||
				len(repository.recordInput.Final) != test.finalCount ||
				repository.recordInput.FallbackCode != test.fallback {
				t.Fatalf("route result = router:%#v provider:%#v record:%#v err:%v",
					router, provider, repository.recordInput, err)
			}
			if router.input.Query != "Should I use my saved preference?" {
				t.Fatalf("route input = %#v", router.input)
			}
			for _, candidate := range repository.preparation.Candidates {
				if router.input.Query == candidate.Content {
					t.Fatalf("candidate body reached route input: %#v", router.input)
				}
			}
		})
	}
}

func TestSearchRelevantWithMemoryToolRouteFailsClosedOnDriftOrFailure(t *testing.T) {
	tests := []struct {
		name   string
		result HybridMemoryToolRouteResult
		err    error
	}{
		{name: "model drift", result: func() HybridMemoryToolRouteResult {
			result := validHybridMemoryToolRouteResult(true)
			result.ModelID = "drifted"
			return result
		}()},
		{name: "contract drift", result: func() HybridMemoryToolRouteResult {
			result := validHybridMemoryToolRouteResult(true)
			result.ContractSHA256 = "drifted"
			return result
		}()},
		{name: "provider failure", err: errors.New("private Provider failure")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := cloudJudgeTestRepository()
			provider := &hybridTestProvider{
				embedding: validHybridTestEmbedding(),
				rerank: []ragproviders.RerankResult{
					{Index: 0, RelevanceScore: 1},
					{Index: 1, RelevanceScore: 0.9},
					{Index: 2, RelevanceScore: 0.8},
				},
			}
			router := &hybridTestMemoryToolRouter{result: test.result, err: test.err}
			_, _, err := NewService(
				repository,
				WithHybridShadowProvider(provider),
				WithHybridMemoryToolRouter(router),
				WithHybridShadowRelevancePolicy(
					HybridShadowMemoryToolRouteCalibrationPolicy(
						hybridMemoryToolRouteTestModel,
					),
				),
			).SearchRelevantWithHybridShadow(
				context.Background(), "current query", hybridTestConversation,
				hybridTestAssistant, MaxSearchResults,
			)
			if err != nil || len(repository.recordInput.Final) != 0 ||
				repository.recordInput.EstimatedTokens != 0 ||
				repository.recordInput.FallbackCode != "MEMORY_TOOL_ROUTE_FAILED" {
				t.Fatalf("route failure = record:%#v err:%v", repository.recordInput, err)
			}
		})
	}
}

func TestMemoryToolRouteStartsBeforeQueryEmbeddingCompletes(t *testing.T) {
	repository := cloudJudgeTestRepository()
	routeStarted := make(chan struct{})
	embedStarted := make(chan struct{})
	router := &coordinatedMemoryToolRouter{
		routeStarted: routeStarted,
		embedStarted: embedStarted,
	}
	provider := &coordinatedMemoryToolProvider{
		routeStarted: routeStarted,
		embedStarted: embedStarted,
	}
	_, _, err := NewService(
		repository,
		WithHybridShadowProvider(provider),
		WithHybridMemoryToolRouter(router),
		WithHybridShadowRelevancePolicy(
			HybridShadowMemoryToolRouteCalibrationPolicy(hybridMemoryToolRouteTestModel),
		),
	).SearchRelevantWithHybridShadow(
		context.Background(), "saved preference", hybridTestConversation,
		hybridTestAssistant, MaxSearchResults,
	)
	if err != nil || len(repository.recordInput.Final) != 3 {
		t.Fatalf("concurrent route/embedding = %#v err=%v", repository.recordInput, err)
	}
}

func validHybridMemoryToolRouteResult(useMemory bool) HybridMemoryToolRouteResult {
	return HybridMemoryToolRouteResult{
		UseMemory:       useMemory,
		ModelID:         hybridMemoryToolRouteTestModel,
		ContractVersion: HybridMemoryToolContractVersion,
		ContractSHA256:  HybridMemoryToolContractSHA256,
	}
}

type hybridTestMemoryToolRouter struct {
	input  HybridMemoryToolRouteInput
	result HybridMemoryToolRouteResult
	err    error
	calls  int
}

func (router *hybridTestMemoryToolRouter) RouteHybridMemory(
	_ context.Context,
	input HybridMemoryToolRouteInput,
) (HybridMemoryToolRouteResult, error) {
	router.calls++
	router.input = input
	return router.result, router.err
}

type coordinatedMemoryToolRouter struct {
	routeStarted chan struct{}
	embedStarted chan struct{}
}

func (router *coordinatedMemoryToolRouter) RouteHybridMemory(
	ctx context.Context,
	_ HybridMemoryToolRouteInput,
) (HybridMemoryToolRouteResult, error) {
	close(router.routeStarted)
	select {
	case <-router.embedStarted:
		return validHybridMemoryToolRouteResult(true), nil
	case <-ctx.Done():
		return HybridMemoryToolRouteResult{}, ctx.Err()
	}
}

type coordinatedMemoryToolProvider struct {
	routeStarted chan struct{}
	embedStarted chan struct{}
}

func (provider *coordinatedMemoryToolProvider) EmbedQuery(
	ctx context.Context,
	_ string,
) (ragproviders.QueryEmbedding, error) {
	close(provider.embedStarted)
	select {
	case <-provider.routeStarted:
		return validHybridTestEmbedding(), nil
	case <-ctx.Done():
		return ragproviders.QueryEmbedding{}, ctx.Err()
	}
}

func (provider *coordinatedMemoryToolProvider) Rerank(
	_ context.Context,
	_ string,
	documents []string,
) ([]ragproviders.RerankResult, error) {
	results := make([]ragproviders.RerankResult, len(documents))
	for index := range documents {
		results[index] = ragproviders.RerankResult{Index: index, RelevanceScore: 1}
	}
	return results, nil
}

var (
	_ HybridMemoryToolRouter = (*hybridTestMemoryToolRouter)(nil)
	_ HybridMemoryToolRouter = (*coordinatedMemoryToolRouter)(nil)
	_ HybridShadowProvider   = (*coordinatedMemoryToolProvider)(nil)
)
