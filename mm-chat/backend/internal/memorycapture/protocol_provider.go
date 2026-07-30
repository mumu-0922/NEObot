package memorycapture

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	"neo-chat/mm-chat/backend/internal/chat"
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

// FakeProtocolMemoryToolRoundProvider proves the exact first ToolRound seam.
// Its digest decision is deliberately content-agnostic and must never be
// interpreted as relevance-quality evidence.
type FakeProtocolMemoryToolRoundProvider struct{ modelID string }

func NewFakeProtocolMemoryToolRoundProvider(
	modelID string,
) *FakeProtocolMemoryToolRoundProvider {
	return &FakeProtocolMemoryToolRoundProvider{modelID: modelID}
}

func (provider *FakeProtocolMemoryToolRoundProvider) StreamToolRound(
	ctx context.Context,
	input chat.ProviderRoundRequest,
) (<-chan chat.ProviderEvent, error) {
	if provider == nil || input.ModelRef.ModelID != provider.modelID ||
		len(input.Tools) != 1 ||
		input.Tools[0].Function.Name != usermemory.HybridMemoryToolName ||
		input.ToolChoice != chat.ProviderToolChoiceAuto ||
		len(input.Continuation) != 0 {
		return nil, errors.New("fake Memory first Tool round is invalid")
	}
	digest := sha256.Sum256([]byte(input.Prompt))
	events := make(chan chat.ProviderEvent, 2)
	if digest[0]&1 == 0 {
		events <- chat.ProviderEvent{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "fake-memory-call", Name: usermemory.HybridMemoryToolName,
				Arguments: `{}`,
			},
		}
	} else {
		events <- chat.ProviderEvent{
			Type: chat.ProviderEventDelta, Delta: "fake direct answer",
		}
	}
	events <- chat.ProviderEvent{
		Type: chat.ProviderEventUsage,
		Usage: &chat.TokenUsage{
			PromptTokens: len(input.Prompt) + 1024, CompletionTokens: 32,
		},
	}
	close(events)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return events, nil
	}
}

func (*FakeProtocolMemoryToolRoundProvider) StreamChat(
	context.Context,
	chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	return nil, errors.New("fake Memory ToolRound fixture does not answer chat")
}

var (
	_ PassageEmbedder                 = (*FakeProtocolProvider)(nil)
	_ usermemory.HybridShadowProvider = (*FakeProtocolProvider)(nil)
	_ usermemory.HybridCandidateJudge = (*FakeProtocolCandidateJudge)(nil)
	_ chat.ToolRoundProvider          = (*FakeProtocolMemoryToolRoundProvider)(nil)
)
