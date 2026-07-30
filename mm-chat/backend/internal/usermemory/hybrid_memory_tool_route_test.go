package usermemory

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/ragproviders"
)

const hybridMemoryToolRouteTestModel = "configured-main-model"

func TestMemoryFirstToolRoundPolicyDoesNotReusePreflightDecoding(t *testing.T) {
	policy, ok := DescribeHybridShadowRelevancePolicy(
		HybridShadowMemoryFirstToolRoundCalibrationPolicy(
			hybridMemoryToolRouteTestModel,
		),
	)
	if !ok || policy.ID != HybridRelevanceMemoryFirstToolRoundPolicyID ||
		!policy.MemoryToolRouteRequired ||
		policy.MemoryToolRouteContractSHA256 != HybridMemoryToolContractSHA256 ||
		policy.MemoryToolRouteDecodingProfile != "none" ||
		policy.MemoryToolRouteMaximumOutputTokens != 0 ||
		policy.MemoryToolRouteTemperature != 0 || policy.MemoryToolRouteDisableThinking {
		t.Fatalf("first Tool-round policy = %#v", policy)
	}
}

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

func TestMemoryToolRouteCompletesWhenAdmissionIsUnavailable(t *testing.T) {
	repository := cloudJudgeTestRepository()
	repository.recordSummary.FallbackCode = "RELEVANCE_ADMISSION_UNAVAILABLE"
	provider := &hybridTestProvider{embedErr: errors.New("fixture embedding failure")}
	router := &delayedMemoryToolRouter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	type outcome struct {
		summary HybridShadowSummary
		err     error
	}
	completed := make(chan outcome, 1)
	go func() {
		_, summary, err := NewService(
			repository,
			WithHybridShadowProvider(provider),
			WithHybridMemoryToolRouter(router),
			WithHybridShadowRelevancePolicy(
				HybridShadowMemoryToolRouteCalibrationPolicy(
					hybridMemoryToolRouteTestModel,
				),
			),
		).SearchRelevantWithHybridShadow(
			context.Background(), "saved preference", hybridTestConversation,
			hybridTestAssistant, MaxSearchResults,
		)
		completed <- outcome{summary: summary, err: err}
	}()

	select {
	case <-router.started:
	case <-time.After(time.Second):
		t.Fatal("Memory Tool route did not start")
	}
	select {
	case result := <-completed:
		t.Fatalf("retrieval returned before route completed: %#v", result)
	case <-time.After(25 * time.Millisecond):
	}
	close(router.release)
	select {
	case result := <-completed:
		if result.err != nil ||
			result.summary.FallbackCode != "RELEVANCE_ADMISSION_UNAVAILABLE" ||
			repository.recordInput.FallbackCode != "RELEVANCE_ADMISSION_UNAVAILABLE" ||
			provider.rerankCalls != 0 || len(repository.recordInput.Final) != 0 {
			t.Fatalf("fail-closed route completion = result:%#v repo:%#v provider:%#v",
				result, repository.recordInput, provider)
		}
	case <-time.After(time.Second):
		t.Fatal("retrieval did not complete after route result")
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

type delayedMemoryToolRouter struct {
	started chan struct{}
	release chan struct{}
}

func (router *delayedMemoryToolRouter) RouteHybridMemory(
	ctx context.Context,
	_ HybridMemoryToolRouteInput,
) (HybridMemoryToolRouteResult, error) {
	close(router.started)
	select {
	case <-router.release:
		return validHybridMemoryToolRouteResult(true), nil
	case <-ctx.Done():
		return HybridMemoryToolRouteResult{}, ctx.Err()
	}
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
	_ HybridMemoryToolRouter = (*delayedMemoryToolRouter)(nil)
	_ HybridShadowProvider   = (*coordinatedMemoryToolProvider)(nil)
)
