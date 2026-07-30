package memoryroute

import (
	"context"
	"errors"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

// SearchMemoryToolDefinition remains as a compatibility wrapper. The chat Tool
// Loop owns the canonical contract so product execution and Development
// capture cannot drift.
func SearchMemoryToolDefinition() chat.ToolDefinition {
	return chat.SearchMemoryToolDefinition()
}

func ToolContractSHA256() (string, error) {
	return chat.SearchMemoryToolContractSHA256()
}

// ChatToolAdapter binds one exact current chat Provider/model to the Memory
// route interface. It receives no candidate content and returns no free-form
// model text.
type ChatToolAdapter struct {
	provider chat.ToolRoundProvider
	modelRef chat.ModelRef
}

func NewChatToolAdapter(
	provider chat.ToolRoundProvider,
	modelRef chat.ModelRef,
) (*ChatToolAdapter, error) {
	modelRef.ProviderID = strings.TrimSpace(modelRef.ProviderID)
	modelRef.ModelID = strings.TrimSpace(modelRef.ModelID)
	if provider == nil || modelRef.ProviderID == "" || modelRef.ModelID == "" {
		return nil, errors.New("Memory Tool route Provider/model is required")
	}
	contractSHA256, err := ToolContractSHA256()
	if err != nil || contractSHA256 != usermemory.HybridMemoryToolContractSHA256 {
		return nil, errors.New("Memory Tool route contract drifted")
	}
	return &ChatToolAdapter{provider: provider, modelRef: modelRef}, nil
}

func (adapter *ChatToolAdapter) RouteHybridMemory(
	ctx context.Context,
	input usermemory.HybridMemoryToolRouteInput,
) (usermemory.HybridMemoryToolRouteResult, error) {
	if adapter == nil || adapter.provider == nil || strings.TrimSpace(input.Query) == "" {
		return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route is unavailable")
	}
	roundEvents, err := adapter.provider.StreamToolRound(ctx, chat.ProviderRoundRequest{
		ProviderRequest: chat.ProviderRequest{
			Prompt: input.Query,
			Messages: []chat.ProviderMessage{{
				Role: "user", Content: input.Query,
			}},
			ModelRef: adapter.modelRef,
		},
		Tools:      []chat.ToolDefinition{SearchMemoryToolDefinition()},
		ToolChoice: chat.ProviderToolChoiceAuto,
	})
	if err != nil || ctx.Err() != nil || roundEvents == nil {
		return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route Provider failed")
	}
	calls := make([]chat.ProviderToolCall, 0, 1)
	outputTokenUpperBound := 32
	for event := range roundEvents {
		if event.Error != nil {
			return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route Provider failed")
		}
		switch event.Type {
		case chat.ProviderEventDelta:
			outputTokenUpperBound += len(event.Delta)
		case chat.ProviderEventReasoningDelta:
			outputTokenUpperBound += len(event.ReasoningDelta)
		case chat.ProviderEventToolCallDelta, chat.ProviderEventRoundCompleted:
		case chat.ProviderEventToolCallCompleted:
			if event.ToolCall == nil {
				return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route call is invalid")
			}
			calls = append(calls, *event.ToolCall)
			outputTokenUpperBound += len(event.ToolCall.ID) +
				len(event.ToolCall.Name) + len(event.ToolCall.Arguments) + 32
		case chat.ProviderEventUsage:
			if event.Usage != nil && event.Usage.CompletionTokens > outputTokenUpperBound {
				outputTokenUpperBound = event.Usage.CompletionTokens
			}
		default:
			return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route Provider event is invalid")
		}
	}
	if ctx.Err() != nil {
		return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route Provider failed")
	}
	useMemory, err := chat.ValidateSearchMemoryOnlyFirstRound(calls)
	if err != nil {
		return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route call is invalid")
	}
	return usermemory.HybridMemoryToolRouteResult{
		UseMemory:             useMemory,
		ModelID:               adapter.modelRef.ModelID,
		ContractVersion:       usermemory.HybridMemoryToolContractVersion,
		ContractSHA256:        usermemory.HybridMemoryToolContractSHA256,
		OutputTokenUpperBound: outputTokenUpperBound,
	}, nil
}

var _ usermemory.HybridMemoryToolRouter = (*ChatToolAdapter)(nil)
