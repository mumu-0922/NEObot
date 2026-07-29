package memoryroute

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestSearchMemoryToolContractHash(t *testing.T) {
	digest, err := ToolContractSHA256()
	if err != nil {
		t.Fatal(err)
	}
	tool := SearchMemoryToolDefinition()
	if digest != usermemory.HybridMemoryToolContractSHA256 ||
		tool.Function.Name != usermemory.HybridMemoryToolName {
		t.Fatalf("tool=%#v digest=%q", tool, digest)
	}
	body, err := json.Marshal(tool)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "" {
		t.Fatal("empty Tool contract")
	}
}

func TestChatToolAdapterAcceptsNoCallOrExactMemoryCall(t *testing.T) {
	for _, test := range []struct {
		name  string
		calls []chat.ToolCall
		want  bool
	}{
		{name: "no memory"},
		{name: "use memory", calls: []chat.ToolCall{{
			ID: "call-1", Name: usermemory.HybridMemoryToolName, Args: map[string]any{},
		}}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			planner := &routePlanner{calls: test.calls}
			adapter, err := NewChatToolAdapter(planner, chat.ModelRef{
				ProviderID: "configured", ModelID: "current-model",
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := adapter.RouteHybridMemory(
				context.Background(),
				usermemory.HybridMemoryToolRouteInput{Query: "current request"},
			)
			if err != nil || result.UseMemory != test.want ||
				result.ModelID != "current-model" ||
				result.ContractVersion != usermemory.HybridMemoryToolContractVersion ||
				result.ContractSHA256 != usermemory.HybridMemoryToolContractSHA256 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if planner.input.Prompt != "current request" || len(planner.input.Tools) != 1 ||
				planner.input.Tools[0].Function.Name != usermemory.HybridMemoryToolName ||
				!planner.input.DisableThinking ||
				planner.input.MaxOutputTokens != usermemory.HybridMemoryToolMaximumOutputTokens ||
				planner.input.Temperature == nil || *planner.input.Temperature != 0 {
				t.Fatalf("planner input=%#v", planner.input)
			}
		})
	}
}

func TestChatToolAdapterFailsClosedOnInvalidOrFailedPlan(t *testing.T) {
	tests := []routePlanner{
		{calls: []chat.ToolCall{{ID: "", Name: usermemory.HybridMemoryToolName}}},
		{calls: []chat.ToolCall{{ID: "call-1", Name: "other"}}},
		{calls: []chat.ToolCall{{ID: "call-1", Name: usermemory.HybridMemoryToolName, Args: nil}}},
		{calls: []chat.ToolCall{{ID: "call-1", Name: usermemory.HybridMemoryToolName, Args: map[string]any{"query": "leak"}}}},
		{calls: []chat.ToolCall{{ID: "call-1", Name: usermemory.HybridMemoryToolName}, {ID: "call-2", Name: usermemory.HybridMemoryToolName}}},
		{err: errors.New("private Provider failure")},
	}
	for index := range tests {
		planner := &tests[index]
		adapter, err := NewChatToolAdapter(planner, chat.ModelRef{
			ProviderID: "configured", ModelID: "current-model",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.RouteHybridMemory(
			context.Background(),
			usermemory.HybridMemoryToolRouteInput{Query: "query"},
		); err == nil {
			t.Fatalf("invalid plan %d accepted", index)
		}
	}
}

type routePlanner struct {
	calls []chat.ToolCall
	err   error
	input chat.ToolPlanRequest
}

func (planner *routePlanner) PlanTools(
	_ context.Context,
	input chat.ToolPlanRequest,
) ([]chat.ToolCall, error) {
	planner.input = input
	return planner.calls, planner.err
}

var _ chat.ToolPlanner = (*routePlanner)(nil)
