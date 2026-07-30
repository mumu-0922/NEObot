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

func TestChatToolAdapterAcceptsNoCallOrExactFirstRoundMemoryCall(t *testing.T) {
	for _, test := range []struct {
		name   string
		events []chat.ProviderEvent
		want   bool
	}{
		{name: "no memory", events: []chat.ProviderEvent{{
			Type: chat.ProviderEventDelta, Delta: "ordinary answer",
		}}},
		{name: "use memory", events: []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			},
		}}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &routeToolRoundProvider{events: test.events}
			adapter, err := NewChatToolAdapter(provider, chat.ModelRef{
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
				result.ContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
				result.OutputTokenUpperBound <= 0 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			input := provider.input
			if input.Prompt != "current request" || len(input.Messages) != 1 ||
				input.Messages[0].Role != "user" ||
				input.Messages[0].Content != "current request" ||
				len(input.Tools) != 1 ||
				input.Tools[0].Function.Name != usermemory.HybridMemoryToolName ||
				input.ToolChoice != chat.ProviderToolChoiceAuto ||
				input.DisableThinking || input.MaxOutputTokens != 0 ||
				input.Temperature != nil || len(input.Continuation) != 0 {
				t.Fatalf("first-round input=%#v", input)
			}
		})
	}
}

func TestChatToolAdapterFailsClosedOnInvalidOrFailedFirstRound(t *testing.T) {
	tests := []routeToolRoundProvider{
		{events: []chat.ProviderEvent{{
			Type:     chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{ID: "", Name: usermemory.HybridMemoryToolName, Arguments: `{}`},
		}}},
		{events: []chat.ProviderEvent{{
			Type:     chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{ID: "call-1", Name: "other", Arguments: `{}`},
		}}},
		{events: []chat.ProviderEvent{{
			Type: chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{
				ID: "call-1", Name: " search_memory ", Arguments: `{}`,
			},
		}}},
		{events: []chat.ProviderEvent{{
			Type:     chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{ID: "call-1", Name: usermemory.HybridMemoryToolName, Arguments: `null`},
		}}},
		{events: []chat.ProviderEvent{{
			Type:     chat.ProviderEventToolCallCompleted,
			ToolCall: &chat.ProviderToolCall{ID: "call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{"query":"leak"}`},
		}}},
		{events: []chat.ProviderEvent{
			{Type: chat.ProviderEventToolCallCompleted, ToolCall: &chat.ProviderToolCall{
				ID: "call-1", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			}},
			{Type: chat.ProviderEventToolCallCompleted, ToolCall: &chat.ProviderToolCall{
				ID: "call-2", Name: usermemory.HybridMemoryToolName, Arguments: `{}`,
			}},
		}},
		{events: []chat.ProviderEvent{{Error: errors.New("private Provider failure")}}},
		{err: errors.New("private Provider startup failure")},
	}
	for index := range tests {
		provider := &tests[index]
		adapter, err := NewChatToolAdapter(provider, chat.ModelRef{
			ProviderID: "configured", ModelID: "current-model",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.RouteHybridMemory(
			context.Background(),
			usermemory.HybridMemoryToolRouteInput{Query: "query"},
		); err == nil {
			t.Fatalf("invalid first round %d accepted", index)
		}
	}
}

type routeToolRoundProvider struct {
	events []chat.ProviderEvent
	err    error
	input  chat.ProviderRoundRequest
}

func (provider *routeToolRoundProvider) StreamToolRound(
	ctx context.Context,
	input chat.ProviderRoundRequest,
) (<-chan chat.ProviderEvent, error) {
	provider.input = input
	if provider.err != nil {
		return nil, provider.err
	}
	events := make(chan chat.ProviderEvent, len(provider.events))
	for _, event := range provider.events {
		select {
		case <-ctx.Done():
			close(events)
			return events, nil
		case events <- event:
		}
	}
	close(events)
	return events, nil
}

func (provider *routeToolRoundProvider) StreamChat(
	context.Context,
	chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	return nil, errors.New("unexpected ordinary chat request")
}

var _ chat.ToolRoundProvider = (*routeToolRoundProvider)(nil)
