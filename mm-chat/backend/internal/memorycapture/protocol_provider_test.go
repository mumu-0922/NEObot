package memorycapture

import (
	"context"
	"encoding/json"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/memoryroute"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestFakeProtocolAbstentionConfirmationJudgeExercisesBothPrompts(t *testing.T) {
	judge := NewFakeProtocolAbstentionConfirmationCandidateJudge("fixture-model")
	input := usermemory.HybridCandidateJudgeInput{
		Query: "fixture query",
		Candidates: []usermemory.HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "fixture memory"},
		},
		PromptPurpose: usermemory.HybridCandidateJudgePromptPurposeAccuracyPrimary,
	}
	primary, err := judge.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	var primaryOutput struct {
		SelectedOrdinals []int `json:"selectedOrdinals"`
	}
	if err := json.Unmarshal(primary.RawOutput, &primaryOutput); err != nil {
		t.Fatal(err)
	}
	if len(primaryOutput.SelectedOrdinals) != 0 ||
		primary.PromptVersion != usermemory.HybridCandidateJudgeAccuracyPromptVersion {
		t.Fatalf("primary result = %#v / %#v", primary, primaryOutput)
	}

	input.PromptPurpose = usermemory.HybridCandidateJudgePromptPurposeAbstentionConfirmation
	confirmation, err := judge.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	var confirmationOutput struct {
		SelectedOrdinals []int `json:"selectedOrdinals"`
	}
	if err := json.Unmarshal(confirmation.RawOutput, &confirmationOutput); err != nil {
		t.Fatal(err)
	}
	if len(confirmationOutput.SelectedOrdinals) != 1 ||
		confirmationOutput.SelectedOrdinals[0] != 0 ||
		confirmation.PromptVersion != usermemory.HybridCandidateJudgeConfirmationPromptVersion ||
		confirmation.PromptSHA256 != usermemory.HybridCandidateJudgeConfirmationPromptSHA256 {
		t.Fatalf("confirmation result = %#v / %#v", confirmation, confirmationOutput)
	}
}

func TestFakeProtocolDoubleConfirmationJudgeExercisesFullV4Path(t *testing.T) {
	judge := NewFakeProtocolDoubleConfirmationCandidateJudge("fixture-model")
	input := usermemory.HybridCandidateJudgeInput{
		PromptPurpose: usermemory.HybridCandidateJudgePromptPurposeAccuracyPrimary,
		Candidates: []usermemory.HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "fixture memory"},
		},
	}
	primary, err := judge.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.PromptPurpose = usermemory.HybridCandidateJudgePromptPurposeAbstentionConfirmation
	first, err := judge.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := judge.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	decode := func(body []byte) []int {
		var value struct {
			SelectedOrdinals []int `json:"selectedOrdinals"`
		}
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		return value.SelectedOrdinals
	}
	if len(decode(primary.RawOutput)) != 0 || len(decode(first.RawOutput)) != 0 ||
		len(decode(second.RawOutput)) != 1 {
		t.Fatalf("unexpected v4 fake decisions: primary=%s first=%s second=%s",
			primary.RawOutput, first.RawOutput, second.RawOutput)
	}
}

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
