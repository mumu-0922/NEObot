package memorycapture

import (
	"context"
	"testing"

	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestFakeProtocolMemoryToolRouterIsDeterministicAndProvenanceBound(t *testing.T) {
	router := NewFakeProtocolMemoryToolRouter("fixture-model")
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
		first.ContractSHA256 != usermemory.HybridMemoryToolContractSHA256 {
		t.Fatalf("fake Memory Tool-route result = %#v / %#v", first, second)
	}
}
