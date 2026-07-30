package memorycapture

import (
	"context"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryroute"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestFakeProtocolMemoryToolRoundIsDeterministicAndProvenanceBound(t *testing.T) {
	provider := NewFakeProtocolMemoryToolRoundProvider("fixture-model")
	router, err := memoryroute.NewChatToolAdapter(provider, chat.ModelRef{
		ProviderID: "fixture", ModelID: "fixture-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := router.RouteHybridMemory(
		context.Background(),
		usermemory.HybridMemoryToolRouteInput{Query: "fixture query"},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := router.RouteHybridMemory(
		context.Background(),
		usermemory.HybridMemoryToolRouteInput{Query: "fixture query"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.ModelID != "fixture-model" ||
		first.ContractVersion != usermemory.HybridMemoryToolContractVersion ||
		first.ContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
		first.OutputTokenUpperBound <= 0 {
		t.Fatalf("fake Memory first Tool-round result = %#v / %#v", first, second)
	}
}
