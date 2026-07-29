package memorycapture

import (
	"context"
	"crypto/sha256"

	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

// FakeProtocolProvider is deterministic and performs no network I/O. Its
// profile is permanently labelled fake_protocol and cannot represent native
// reader quality or live Provider cost/latency.
type FakeProtocolProvider struct{}

func NewFakeProtocolProvider() *FakeProtocolProvider { return &FakeProtocolProvider{} }

func (*FakeProtocolProvider) EmbedSiliconFlowPassages(
	_ context.Context,
	request ragproviders.PassageEmbeddingRequest,
) (ragproviders.PassageEmbeddingResponse, error) {
	response := ragproviders.PassageEmbeddingResponse{
		Model:      ragproviders.SiliconFlowEmbeddingModel,
		Dimensions: ragproviders.SiliconFlowEmbeddingDimensions,
		Vectors:    make([]ragproviders.PassageEmbeddingVector, len(request.Passages)),
	}
	for index, passage := range request.Passages {
		response.Vectors[index] = ragproviders.PassageEmbeddingVector{
			PassageID: passage.PassageID,
			Embedding: fakeProtocolVector(passage.Text),
		}
	}
	return response, nil
}

func (*FakeProtocolProvider) EmbedQuery(
	_ context.Context,
	query string,
) (ragproviders.QueryEmbedding, error) {
	return ragproviders.QueryEmbedding{
		ModelID:    ragproviders.SiliconFlowEmbeddingModel,
		Dimensions: ragproviders.SiliconFlowEmbeddingDimensions,
		Vector:     fakeProtocolVector(query),
	}, nil
}

func (*FakeProtocolProvider) Rerank(
	_ context.Context,
	_ string,
	documents []string,
) ([]ragproviders.RerankResult, error) {
	results := make([]ragproviders.RerankResult, len(documents))
	for index := range documents {
		results[index] = ragproviders.RerankResult{
			Index: index, RelevanceScore: float64(len(documents) - index),
		}
	}
	return results, nil
}

func fakeProtocolVector(value string) []float32 {
	digest := sha256.Sum256([]byte(value))
	vector := make([]float32, ragproviders.SiliconFlowEmbeddingDimensions)
	vector[0] = 1
	for index, current := range digest {
		vector[index+1] = (float32(current) - 127.5) / 255
	}
	return vector
}

var (
	_ PassageEmbedder                 = (*FakeProtocolProvider)(nil)
	_ usermemory.HybridShadowProvider = (*FakeProtocolProvider)(nil)
)
