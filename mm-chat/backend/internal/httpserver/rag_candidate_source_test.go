package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/knowledge"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

type fakeRAGCandidateFetcher struct {
	keyword      []knowledge.EvidenceCandidateReference
	hybrid       []knowledge.EvidenceCandidateReference
	hybridErr    error
	keywordCalls int
	hybridCalls  int
}

func (fetcher *fakeRAGCandidateFetcher) FetchQueryEvidenceCandidates(
	_ context.Context,
	_ knowledge.QueryEvidenceCandidatesInput,
) ([]knowledge.EvidenceCandidateReference, error) {
	fetcher.keywordCalls++
	return append([]knowledge.EvidenceCandidateReference(nil), fetcher.keyword...), nil
}

func (fetcher *fakeRAGCandidateFetcher) FetchHybridQueryEvidenceCandidates(
	_ context.Context,
	_ knowledge.HybridQueryEvidenceCandidatesInput,
) ([]knowledge.EvidenceCandidateReference, error) {
	fetcher.hybridCalls++
	if fetcher.hybridErr != nil {
		return nil, fetcher.hybridErr
	}
	return append([]knowledge.EvidenceCandidateReference(nil), fetcher.hybrid...), nil
}

type fakeRAGQueryEmbedder struct {
	err error
}

type fakeProfiledRAGCandidateFetcher struct {
	fakeRAGCandidateFetcher
	bindings           []knowledge.RetrievalProfileBinding
	resolveErrors      []error
	resolveCalls       int
	fencedHybrid       []knowledge.EvidenceCandidateReference
	fencedHybridErrors []error
	fencedHybridInputs []knowledge.FencedHybridQueryEvidenceCandidatesInput
	fencedLexical      []knowledge.EvidenceCandidateReference
	fencedLexicalErr   error
	fencedLexicalInput []knowledge.FencedQueryEvidenceCandidatesInput
}

func (fetcher *fakeProfiledRAGCandidateFetcher) ResolveActiveRetrievalProfile(
	_ context.Context,
) (knowledge.RetrievalProfileBinding, error) {
	index := fetcher.resolveCalls
	fetcher.resolveCalls++
	if index < len(fetcher.resolveErrors) && fetcher.resolveErrors[index] != nil {
		return knowledge.RetrievalProfileBinding{}, fetcher.resolveErrors[index]
	}
	if index >= len(fetcher.bindings) {
		return knowledge.RetrievalProfileBinding{}, errors.New("missing retrieval binding")
	}
	return fetcher.bindings[index], nil
}

func (fetcher *fakeProfiledRAGCandidateFetcher) FetchFencedHybridQueryEvidenceCandidates(
	_ context.Context,
	input knowledge.FencedHybridQueryEvidenceCandidatesInput,
) ([]knowledge.EvidenceCandidateReference, error) {
	index := len(fetcher.fencedHybridInputs)
	fetcher.fencedHybridInputs = append(fetcher.fencedHybridInputs, input)
	if index < len(fetcher.fencedHybridErrors) && fetcher.fencedHybridErrors[index] != nil {
		return nil, fetcher.fencedHybridErrors[index]
	}
	return append([]knowledge.EvidenceCandidateReference(nil), fetcher.fencedHybrid...), nil
}

func (fetcher *fakeProfiledRAGCandidateFetcher) FetchFencedQueryEvidenceCandidates(
	_ context.Context,
	input knowledge.FencedQueryEvidenceCandidatesInput,
) ([]knowledge.EvidenceCandidateReference, error) {
	fetcher.fencedLexicalInput = append(fetcher.fencedLexicalInput, input)
	if fetcher.fencedLexicalErr != nil {
		return nil, fetcher.fencedLexicalErr
	}
	return append([]knowledge.EvidenceCandidateReference(nil), fetcher.fencedLexical...), nil
}

type fakeRAGQueryEmbeddingGate struct {
	bindings []knowledge.RetrievalProfileBinding
	err      error
}

func (gate *fakeRAGQueryEmbeddingGate) AuthorizeRAGQueryEmbedding(
	_ context.Context,
	binding knowledge.RetrievalProfileBinding,
) error {
	gate.bindings = append(gate.bindings, binding)
	return gate.err
}

type fakeRAGProviderCredentialResolver struct {
	providers []string
}

func (resolver *fakeRAGProviderCredentialResolver) ResolveRAGProviderCredential(
	_ context.Context,
	providerID string,
) (string, error) {
	resolver.providers = append(resolver.providers, providerID)
	return "fixture-provider-credential", nil
}

type fakeGenerationRetrievalProfileResolver struct {
	binding            knowledge.RetrievalProfileBinding
	indexGenerationIDs []string
}

func (resolver *fakeGenerationRetrievalProfileResolver) ResolveGenerationRetrievalProfile(
	_ context.Context,
	indexGenerationID string,
) (knowledge.RetrievalProfileBinding, error) {
	resolver.indexGenerationIDs = append(resolver.indexGenerationIDs, indexGenerationID)
	return resolver.binding, nil
}

func (embedder fakeRAGQueryEmbedder) EmbedQuery(
	_ context.Context,
	_ string,
) (ragproviders.QueryEmbedding, error) {
	if embedder.err != nil {
		return ragproviders.QueryEmbedding{}, embedder.err
	}
	return ragproviders.QueryEmbedding{
		ModelID: ragproviders.SiliconFlowEmbeddingModel, Dimensions: 1024,
		Vector: repeatedRAGQueryVector(0.001),
	}, nil
}

func TestKnowledgeRAGCandidateSourceUsesHybridCandidates(t *testing.T) {
	reference := knowledge.EvidenceCandidateReference{ChildChunkID: "hybrid"}
	fetcher := &fakeRAGCandidateFetcher{hybrid: []knowledge.EvidenceCandidateReference{reference}}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   fakeRAGQueryEmbedder{},
	}

	candidates, err := source.FetchEvidenceCandidates(context.Background(), validRAGCandidateQuery())
	if err != nil {
		t.Fatalf("FetchEvidenceCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ChildChunkID != "hybrid" ||
		fetcher.hybridCalls != 1 || fetcher.keywordCalls != 0 {
		t.Fatalf("candidates/calls = %#v/%d/%d", candidates, fetcher.hybridCalls, fetcher.keywordCalls)
	}
}

func TestKnowledgeRAGCandidateSourceDegradesToKeywordWhenEmbeddingFails(t *testing.T) {
	reference := knowledge.EvidenceCandidateReference{ChildChunkID: "keyword"}
	fetcher := &fakeRAGCandidateFetcher{keyword: []knowledge.EvidenceCandidateReference{reference}}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder: fakeRAGQueryEmbedder{
			err: ragproviders.ErrQueryEmbeddingUnavailable,
		},
	}

	candidates, err := source.FetchEvidenceCandidates(context.Background(), validRAGCandidateQuery())
	if err != nil {
		t.Fatalf("FetchEvidenceCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].ChildChunkID != "keyword" ||
		fetcher.hybridCalls != 0 || fetcher.keywordCalls != 1 {
		t.Fatalf("candidates/calls = %#v/%d/%d", candidates, fetcher.hybridCalls, fetcher.keywordCalls)
	}
}

func TestKnowledgeRAGCandidateSourceDoesNotHideHybridDatabaseFailure(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	fetcher := &fakeRAGCandidateFetcher{hybridErr: databaseErr}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   fakeRAGQueryEmbedder{},
	}

	_, err := source.FetchEvidenceCandidates(context.Background(), validRAGCandidateQuery())
	if !errors.Is(err, databaseErr) || fetcher.keywordCalls != 0 {
		t.Fatalf("error/keyword calls = %v/%d", err, fetcher.keywordCalls)
	}
}

func TestKnowledgeRAGCandidateSourceSelectsActiveEmbeddingProfile(t *testing.T) {
	binding := retrievalBinding(
		"82000000-0000-4000-8000-000000000001",
		"82000000-0000-4000-8000-000000000002",
		ragproviders.SiliconFlowRetrievalProfile,
	)
	var endpoints []string
	resolver := &fakeRAGProviderCredentialResolver{}
	gateway := ragproviders.NewProviderGateway(
		resolver,
		ragproviders.WithProviderGatewayHTTPClient(&http.Client{
			Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				endpoints = append(endpoints, request.URL.String())
				return retrievalProviderResponse(request.URL.String())
			}),
		}),
	)
	fetcher := &fakeProfiledRAGCandidateFetcher{
		bindings:     []knowledge.RetrievalProfileBinding{binding},
		fencedHybrid: []knowledge.EvidenceCandidateReference{{ChildChunkID: "profiled"}},
	}
	gate := &fakeRAGQueryEmbeddingGate{}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   gateway,
		queryGate:  gate,
	}

	candidates, err := source.FetchEvidenceCandidates(
		context.Background(),
		validRAGCandidateQuery(),
	)
	if err != nil || len(candidates) != 1 ||
		candidates[0].ChildChunkID != "profiled" {
		t.Fatalf("FetchEvidenceCandidates() = %#v, %v", candidates, err)
	}
	if len(endpoints) != 1 ||
		endpoints[0] != ragproviders.SiliconFlowEmbeddingsEndpoint ||
		len(resolver.providers) != 1 || resolver.providers[0] != "siliconflow" ||
		len(gate.bindings) != 1 || gate.bindings[0] != binding ||
		len(fetcher.fencedHybridInputs) != 1 ||
		fetcher.fencedHybridInputs[0].Binding != binding {
		t.Fatalf(
			"endpoints=%v providers=%v gate=%#v hybrid=%#v",
			endpoints,
			resolver.providers,
			gate.bindings,
			fetcher.fencedHybridInputs,
		)
	}
}

func TestKnowledgeRAGCandidateSourceRetriesOneProfileActivationRace(t *testing.T) {
	firstBinding := retrievalBinding(
		"83000000-0000-4000-8000-000000000001",
		"83000000-0000-4000-8000-000000000002",
		ragproviders.SiliconFlowRetrievalProfile,
	)
	bgeBinding := retrievalBinding(
		"83000000-0000-4000-8000-000000000003",
		"83000000-0000-4000-8000-000000000004",
		ragproviders.SiliconFlowRetrievalProfile,
	)
	var endpoints []string
	gateway := ragproviders.NewProviderGateway(
		&fakeRAGProviderCredentialResolver{},
		ragproviders.WithProviderGatewayHTTPClient(&http.Client{
			Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				endpoints = append(endpoints, request.URL.String())
				return retrievalProviderResponse(request.URL.String())
			}),
		}),
	)
	fetcher := &fakeProfiledRAGCandidateFetcher{
		bindings:           []knowledge.RetrievalProfileBinding{firstBinding, bgeBinding},
		fencedHybrid:       []knowledge.EvidenceCandidateReference{{ChildChunkID: "after-activation"}},
		fencedHybridErrors: []error{knowledge.ErrRetrievalProfileChanged, nil},
	}
	gate := &fakeRAGQueryEmbeddingGate{}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   gateway,
		queryGate:  gate,
	}

	candidates, err := source.FetchEvidenceCandidates(
		context.Background(),
		validRAGCandidateQuery(),
	)
	if err != nil || len(candidates) != 1 ||
		candidates[0].ChildChunkID != "after-activation" {
		t.Fatalf("FetchEvidenceCandidates() = %#v, %v", candidates, err)
	}
	if fetcher.resolveCalls != 2 || len(fetcher.fencedHybridInputs) != 2 ||
		len(gate.bindings) != 2 || len(endpoints) != 2 ||
		endpoints[0] != ragproviders.SiliconFlowEmbeddingsEndpoint ||
		endpoints[1] != ragproviders.SiliconFlowEmbeddingsEndpoint ||
		fetcher.fencedHybridInputs[1].Binding != bgeBinding {
		t.Fatalf(
			"resolve=%d endpoints=%v gate=%#v hybrid=%#v",
			fetcher.resolveCalls,
			endpoints,
			gate.bindings,
			fetcher.fencedHybridInputs,
		)
	}
}

func TestKnowledgeRAGCandidateSourceKeepsLegacyReaderBeforeCutover(t *testing.T) {
	var endpoints []string
	gateway := ragproviders.NewProviderGateway(
		&fakeRAGProviderCredentialResolver{},
		ragproviders.WithProviderGatewayHTTPClient(&http.Client{
			Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				endpoints = append(endpoints, request.URL.String())
				return retrievalProviderResponse(request.URL.String())
			}),
		}),
	)
	fetcher := &fakeProfiledRAGCandidateFetcher{
		fakeRAGCandidateFetcher: fakeRAGCandidateFetcher{
			keyword: []knowledge.EvidenceCandidateReference{{ChildChunkID: "legacy-bm25"}},
		},
		resolveErrors: []error{knowledge.ErrActiveRetrievalProfileUnavailable},
	}
	source := knowledgeRAGCandidateSource{candidates: fetcher, embedder: gateway}

	candidates, err := source.FetchEvidenceCandidates(
		context.Background(),
		validRAGCandidateQuery(),
	)
	if err != nil || len(candidates) != 1 || candidates[0].ChildChunkID != "legacy-bm25" ||
		fetcher.resolveCalls != 1 || fetcher.keywordCalls != 1 ||
		len(fetcher.fencedHybridInputs) != 0 || len(endpoints) != 0 {
		t.Fatalf(
			"candidates=%#v err=%v resolve=%d legacy=%d fenced=%d endpoints=%v",
			candidates,
			err,
			fetcher.resolveCalls,
			fetcher.keywordCalls,
			len(fetcher.fencedHybridInputs),
			endpoints,
		)
	}
}

func TestKnowledgeRAGCandidateSourceHardFencesRetiredJinaToBM25(t *testing.T) {
	binding := knowledge.RetrievalProfileBinding{
		IndexGenerationID:   "83500000-0000-4000-8000-000000000001",
		SearchProfileID:     "83500000-0000-4000-8000-000000000002",
		RetrievalProfileID:  "jina_v4_v3",
		ProviderID:          "jina",
		EmbeddingModelID:    "jina-embeddings-v4",
		EmbeddingDimensions: 1024,
		RerankModelID:       "jina-reranker-v3",
	}
	resolver := &fakeRAGProviderCredentialResolver{}
	providerCalls := 0
	gateway := ragproviders.NewProviderGateway(
		resolver,
		ragproviders.WithProviderGatewayHTTPClient(&http.Client{
			Transport: httpRoundTripFunc(func(*http.Request) (*http.Response, error) {
				providerCalls++
				return nil, errors.New("retired provider call")
			}),
		}),
	)
	fetcher := &fakeProfiledRAGCandidateFetcher{
		bindings:      []knowledge.RetrievalProfileBinding{binding},
		fencedLexical: []knowledge.EvidenceCandidateReference{{ChildChunkID: "jina-generation-bm25"}},
	}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   gateway,
		queryGate:  &fakeRAGQueryEmbeddingGate{},
	}

	candidates, err := source.FetchEvidenceCandidates(
		context.Background(),
		validRAGCandidateQuery(),
	)
	if err != nil || len(candidates) != 1 ||
		candidates[0].ChildChunkID != "jina-generation-bm25" ||
		len(fetcher.fencedLexicalInput) != 1 ||
		len(fetcher.fencedHybridInputs) != 0 || providerCalls != 0 ||
		len(resolver.providers) != 0 {
		t.Fatalf(
			"candidates=%#v err=%v lexical=%#v hybrid=%#v providerCalls=%d credentials=%v",
			candidates,
			err,
			fetcher.fencedLexicalInput,
			fetcher.fencedHybridInputs,
			providerCalls,
			resolver.providers,
		)
	}
}

func TestKnowledgeRAGCandidateSourceUsesSameProfileBM25OnEmbeddingFailure(t *testing.T) {
	binding := retrievalBinding(
		"84000000-0000-4000-8000-000000000001",
		"84000000-0000-4000-8000-000000000002",
		ragproviders.SiliconFlowRetrievalProfile,
	)
	gateway := ragproviders.NewProviderGateway(
		&fakeRAGProviderCredentialResolver{},
		ragproviders.WithProviderGatewayHTTPClient(&http.Client{
			Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(`{"error":"unavailable"}`)),
					Request:    request,
				}, nil
			}),
		}),
	)
	fetcher := &fakeProfiledRAGCandidateFetcher{
		bindings:      []knowledge.RetrievalProfileBinding{binding},
		fencedLexical: []knowledge.EvidenceCandidateReference{{ChildChunkID: "bm25"}},
	}
	source := knowledgeRAGCandidateSource{
		candidates: fetcher,
		embedder:   gateway,
		queryGate:  &fakeRAGQueryEmbeddingGate{},
	}

	candidates, err := source.FetchEvidenceCandidates(
		context.Background(),
		validRAGCandidateQuery(),
	)
	if err != nil || len(candidates) != 1 || candidates[0].ChildChunkID != "bm25" ||
		len(fetcher.fencedLexicalInput) != 1 ||
		fetcher.fencedLexicalInput[0].Binding != binding ||
		fetcher.hybridCalls != 0 || fetcher.keywordCalls != 0 {
		t.Fatalf(
			"candidates=%#v err=%v lexical=%#v legacy=%d/%d",
			candidates,
			err,
			fetcher.fencedLexicalInput,
			fetcher.hybridCalls,
			fetcher.keywordCalls,
		)
	}
}

func TestKnowledgeRAGRerankerUsesEvidenceGenerationProfile(t *testing.T) {
	binding := retrievalBinding(
		"85000000-0000-4000-8000-000000000001",
		"85000000-0000-4000-8000-000000000002",
		ragproviders.SiliconFlowRetrievalProfile,
	)
	var endpoints []string
	gateway := ragproviders.NewProviderGateway(
		&fakeRAGProviderCredentialResolver{},
		ragproviders.WithProviderGatewayHTTPClient(&http.Client{
			Transport: httpRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				endpoints = append(endpoints, request.URL.String())
				return retrievalProviderResponse(request.URL.String())
			}),
		}),
	)
	resolver := &fakeGenerationRetrievalProfileResolver{binding: binding}
	reranker := knowledgeRAGReranker{client: gateway, profiles: resolver}

	results, err := reranker.Rerank(
		context.Background(),
		binding.IndexGenerationID,
		"semantic question",
		[]string{"first", "second"},
	)
	if err != nil || len(results) != 2 || results[0].Index != 1 ||
		len(endpoints) != 1 || endpoints[0] != ragproviders.SiliconFlowRerankEndpoint ||
		len(resolver.indexGenerationIDs) != 1 ||
		resolver.indexGenerationIDs[0] != binding.IndexGenerationID {
		t.Fatalf(
			"results=%#v err=%v endpoints=%v generations=%v",
			results,
			err,
			endpoints,
			resolver.indexGenerationIDs,
		)
	}
}

func validRAGCandidateQuery() chat.RAGCandidateQuery {
	return chat.RAGCandidateQuery{
		CollectionIDs: []string{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		QueryText:     "semantic question",
		Limit:         20,
	}
}

func repeatedRAGQueryVector(value float32) []float32 {
	vector := make([]float32, 1024)
	for index := range vector {
		vector[index] = value
	}
	return vector
}

func retrievalBinding(
	indexGenerationID string,
	searchProfileID string,
	profile ragproviders.RetrievalProfile,
) knowledge.RetrievalProfileBinding {
	return knowledge.RetrievalProfileBinding{
		IndexGenerationID:   indexGenerationID,
		SearchProfileID:     searchProfileID,
		RetrievalProfileID:  string(profile.ID),
		ProviderID:          profile.ProviderID,
		EmbeddingModelID:    profile.EmbeddingModelID,
		EmbeddingDimensions: profile.EmbeddingDimensions,
		RerankModelID:       profile.RerankModelID,
	}
}

func retrievalProviderResponse(endpoint string) (*http.Response, error) {
	vector := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	for index := range vector {
		vector[index] = 0.001
	}
	var payload any
	switch endpoint {
	case ragproviders.SiliconFlowEmbeddingsEndpoint:
		payload = map[string]any{
			"object": "list",
			"model":  ragproviders.SiliconFlowEmbeddingModel,
			"data": []any{map[string]any{
				"object": "embedding", "index": 0, "embedding": vector,
			}},
			"usage": map[string]any{
				"prompt_tokens": 1, "completion_tokens": 0, "total_tokens": 1,
			},
		}
	case ragproviders.SiliconFlowRerankEndpoint:
		payload = map[string]any{
			"id": "rerank-httpserver-fixture",
			"results": []any{
				map[string]any{"index": 1, "relevance_score": 0.9},
				map[string]any{"index": 0, "relevance_score": 0.1},
			},
			"meta": map[string]any{"tokens": map[string]any{"input_tokens": 2}},
		}
	default:
		return nil, errors.New("unexpected retrieval provider endpoint")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}, nil
}
