package memorycapture

import (
	"context"
	"crypto/sha256"
	"encoding/json"

	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const DefaultSiliconFlowCloudJudgeModelID = "Qwen/Qwen3-8B"

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

func (*FakeProtocolProvider) ClassifyMemoryIntent(
	_ context.Context,
	query string,
) (ragproviders.MemoryIntentSignal, error) {
	digest := sha256.Sum256([]byte(query))
	margin := float64(int(digest[0])-127) / 255
	positive := (margin + 1) / 2
	negative := 1 - positive
	return ragproviders.MemoryIntentSignal{
		AnchorVersion: ragproviders.MemoryIntentAnchorVersion,
		AnchorSHA256:  ragproviders.MemoryIntentAnchorSHA256,
		PositiveScore: positive,
		NegativeScore: negative,
		Margin:        positive - negative,
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
			Index:          index,
			RelevanceScore: float64(len(documents)-index) / float64(len(documents)),
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

type FakeProtocolCandidateJudge struct{ modelID string }

func NewFakeProtocolCandidateJudge(modelID string) *FakeProtocolCandidateJudge {
	return &FakeProtocolCandidateJudge{modelID: modelID}
}

func (judge *FakeProtocolCandidateJudge) JudgeHybridCandidates(
	_ context.Context,
	input usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	selected := make([]int, 0, min(len(input.Candidates), usermemory.HybridShadowFinalLimit))
	for ordinal := range input.Candidates {
		if len(selected) == usermemory.HybridShadowFinalLimit {
			break
		}
		selected = append(selected, ordinal)
	}
	body, _ := json.Marshal(map[string]any{
		"schemaVersion":    usermemory.HybridCandidateJudgeOutputSchemaVersion,
		"selectedOrdinals": selected,
	})
	return usermemory.HybridCandidateJudgeResult{
		RawOutput:     body,
		ModelID:       judge.modelID,
		PromptVersion: usermemory.HybridCandidateJudgePromptVersion,
		PromptSHA256:  usermemory.HybridCandidateJudgePromptSHA256,
	}, nil
}

// FakeProtocolMemoryToolRouter proves only the exact Tool-route protocol and
// provenance plumbing. Its digest decision is deliberately content-agnostic
// and must never be interpreted as relevance-quality evidence.
type FakeProtocolMemoryToolRouter struct{ modelID string }

func NewFakeProtocolMemoryToolRouter(modelID string) *FakeProtocolMemoryToolRouter {
	return &FakeProtocolMemoryToolRouter{modelID: modelID}
}

func (router *FakeProtocolMemoryToolRouter) RouteHybridMemory(
	_ context.Context,
	input usermemory.HybridMemoryToolRouteInput,
) (usermemory.HybridMemoryToolRouteResult, error) {
	digest := sha256.Sum256([]byte(input.Query))
	return usermemory.HybridMemoryToolRouteResult{
		UseMemory:       digest[0]&1 == 0,
		ModelID:         router.modelID,
		ContractVersion: usermemory.HybridMemoryToolContractVersion,
		ContractSHA256:  usermemory.HybridMemoryToolContractSHA256,
	}, nil
}

var (
	_ PassageEmbedder                   = (*FakeProtocolProvider)(nil)
	_ usermemory.HybridShadowProvider   = (*FakeProtocolProvider)(nil)
	_ usermemory.HybridCandidateJudge   = (*FakeProtocolCandidateJudge)(nil)
	_ usermemory.HybridMemoryToolRouter = (*FakeProtocolMemoryToolRouter)(nil)
)
