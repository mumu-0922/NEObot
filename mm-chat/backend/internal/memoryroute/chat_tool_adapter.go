package memoryroute

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const searchMemoryToolDescription = "Search the current user’s saved personal Memory only when a prior preference, fact, decision, instruction, warning, correction, or project context is genuinely needed to answer the current request. Do not call for general knowledge, standalone tasks, content already present in visible conversation, or unrelated requests. The server enforces ownership and scope."

// SearchMemoryToolDefinition is the single contract used by Development
// capture and the eventual product Tool Loop. It intentionally has no query
// argument: the backend retains authority over the current request text and
// the model can only decide whether Memory is needed.
func SearchMemoryToolDefinition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Type: "function",
		Function: chat.ToolFunctionDefinition{
			Name:        usermemory.HybridMemoryToolName,
			Description: searchMemoryToolDescription,
			Parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
			},
		},
	}
}

func ToolContractSHA256() (string, error) {
	body, err := json.Marshal(SearchMemoryToolDefinition())
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

// ChatToolAdapter binds one exact current chat Provider/model to the Memory
// route interface. It receives no candidate content and returns no free-form
// model text.
type ChatToolAdapter struct {
	planner  chat.ToolPlanner
	modelRef chat.ModelRef
}

func NewChatToolAdapter(
	planner chat.ToolPlanner,
	modelRef chat.ModelRef,
) (*ChatToolAdapter, error) {
	modelRef.ProviderID = strings.TrimSpace(modelRef.ProviderID)
	modelRef.ModelID = strings.TrimSpace(modelRef.ModelID)
	if planner == nil || modelRef.ProviderID == "" || modelRef.ModelID == "" {
		return nil, errors.New("Memory Tool route Provider/model is required")
	}
	contractSHA256, err := ToolContractSHA256()
	if err != nil || contractSHA256 != usermemory.HybridMemoryToolContractSHA256 {
		return nil, errors.New("Memory Tool route contract drifted")
	}
	return &ChatToolAdapter{planner: planner, modelRef: modelRef}, nil
}

func (adapter *ChatToolAdapter) RouteHybridMemory(
	ctx context.Context,
	input usermemory.HybridMemoryToolRouteInput,
) (usermemory.HybridMemoryToolRouteResult, error) {
	if adapter == nil || adapter.planner == nil || strings.TrimSpace(input.Query) == "" {
		return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route is unavailable")
	}
	temperature := usermemory.HybridMemoryToolTemperature
	calls, err := adapter.planner.PlanTools(ctx, chat.ToolPlanRequest{
		Prompt:          input.Query,
		ModelRef:        adapter.modelRef,
		Tools:           []chat.ToolDefinition{SearchMemoryToolDefinition()},
		DisableThinking: usermemory.HybridMemoryToolDisableThinking,
		MaxOutputTokens: usermemory.HybridMemoryToolMaximumOutputTokens,
		Temperature:     &temperature,
	})
	if err != nil || ctx.Err() != nil {
		return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route Provider failed")
	}
	useMemory := false
	switch len(calls) {
	case 0:
	case 1:
		call := calls[0]
		if strings.TrimSpace(call.ID) == "" ||
			strings.TrimSpace(call.Name) != usermemory.HybridMemoryToolName ||
			call.Args == nil ||
			len(call.Args) != 0 {
			return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route call is invalid")
		}
		useMemory = true
	default:
		return usermemory.HybridMemoryToolRouteResult{}, errors.New("Memory Tool route returned multiple calls")
	}
	return usermemory.HybridMemoryToolRouteResult{
		UseMemory:       useMemory,
		ModelID:         adapter.modelRef.ModelID,
		ContractVersion: usermemory.HybridMemoryToolContractVersion,
		ContractSHA256:  usermemory.HybridMemoryToolContractSHA256,
	}, nil
}

var _ usermemory.HybridMemoryToolRouter = (*ChatToolAdapter)(nil)
