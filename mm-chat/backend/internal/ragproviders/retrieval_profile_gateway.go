package ragproviders

import (
	"context"
	"math"
)

// RetrievalProfileGateway binds every query-time provider call to one immutable
// search profile. It prevents callers from accidentally pairing a query vector
// or reranker with a different generation's vector space.
type RetrievalProfileGateway struct {
	gateway *ProviderGateway
	profile RetrievalProfile
}

func (gateway *ProviderGateway) ForRetrievalProfile(
	profileID RetrievalProfileID,
) (*RetrievalProfileGateway, error) {
	profile, err := ResolveRetrievalProfile(profileID)
	if err != nil || gateway == nil {
		return nil, ErrProviderGatewayOperationUnsupported
	}
	return &RetrievalProfileGateway{gateway: gateway, profile: profile}, nil
}

func (gateway *RetrievalProfileGateway) Profile() RetrievalProfile {
	if gateway == nil {
		return RetrievalProfile{}
	}
	return gateway.profile
}

func (gateway *RetrievalProfileGateway) EmbedQuery(
	ctx context.Context,
	query string,
) (QueryEmbedding, error) {
	if gateway == nil || gateway.gateway == nil {
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
	switch gateway.profile.ID {
	case RetrievalProfileSiliconFlow:
		return gateway.gateway.embedSiliconFlowQuery(ctx, query)
	default:
		return QueryEmbedding{}, ErrQueryEmbeddingUnavailable
	}
}

func (gateway *RetrievalProfileGateway) Rerank(
	ctx context.Context,
	query string,
	documents []string,
) ([]RerankResult, error) {
	if gateway == nil || gateway.gateway == nil {
		return nil, ErrRerankUnavailable
	}
	switch gateway.profile.ID {
	case RetrievalProfileSiliconFlow:
		return gateway.gateway.rerankSiliconFlow(ctx, query, documents)
	default:
		return nil, ErrRerankUnavailable
	}
}

// ClassifyMemoryIntent compares a query only with two fixed, non-user anchor
// documents. It never receives Memory plaintext and returns request-local
// evidence for a separately calibrated pre-egress gate.
func (gateway *RetrievalProfileGateway) ClassifyMemoryIntent(
	ctx context.Context,
	query string,
) (MemoryIntentSignal, error) {
	if gateway == nil || gateway.gateway == nil ||
		gateway.profile.ID != RetrievalProfileSiliconFlow {
		return MemoryIntentSignal{}, ErrMemoryIntentUnavailable
	}
	results, err := gateway.gateway.rerankSiliconFlow(
		ctx,
		query,
		append([]string(nil), memoryIntentAnchorDocuments...),
	)
	if err != nil || len(results) != len(memoryIntentAnchorDocuments) {
		return MemoryIntentSignal{}, ErrMemoryIntentUnavailable
	}
	scores := make([]float64, len(memoryIntentAnchorDocuments))
	seen := make(map[int]struct{}, len(results))
	for _, result := range results {
		if result.Index < 0 || result.Index >= len(scores) ||
			math.IsNaN(result.RelevanceScore) || math.IsInf(result.RelevanceScore, 0) ||
			result.RelevanceScore < 0 || result.RelevanceScore > 1 {
			return MemoryIntentSignal{}, ErrMemoryIntentInvalid
		}
		if _, duplicate := seen[result.Index]; duplicate {
			return MemoryIntentSignal{}, ErrMemoryIntentInvalid
		}
		seen[result.Index] = struct{}{}
		scores[result.Index] = result.RelevanceScore
	}
	margin := scores[0] - scores[1]
	if math.IsNaN(margin) || math.IsInf(margin, 0) || margin < -1 || margin > 1 {
		return MemoryIntentSignal{}, ErrMemoryIntentInvalid
	}
	return MemoryIntentSignal{
		AnchorVersion: MemoryIntentAnchorVersion,
		AnchorSHA256:  MemoryIntentAnchorSHA256,
		PositiveScore: scores[0],
		NegativeScore: scores[1],
		Margin:        margin,
	}, nil
}

var (
	_ QueryEmbedder          = (*RetrievalProfileGateway)(nil)
	_ Reranker               = (*RetrievalProfileGateway)(nil)
	_ MemoryIntentClassifier = (*RetrievalProfileGateway)(nil)
)
