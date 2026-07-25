package ragproviders

import "context"

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

var (
	_ QueryEmbedder = (*RetrievalProfileGateway)(nil)
	_ Reranker      = (*RetrievalProfileGateway)(nil)
)
