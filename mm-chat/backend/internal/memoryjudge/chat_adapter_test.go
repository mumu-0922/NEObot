package memoryjudge

import (
	"context"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestChatAdapterUsesSharedStrictPromptAndBoundedOutput(t *testing.T) {
	provider := &judgeChatProvider{chunks: []string{
		`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1",`,
		`"selectedOrdinals":[1]}`,
	}}
	adapter, err := NewChatAdapter(provider, chat.ModelRef{
		ProviderID: "siliconflow", ModelID: "Pro/test/judge",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.JudgeHybridCandidates(
		context.Background(),
		usermemory.HybridCandidateJudgeInput{
			Query: "query",
			Candidates: []usermemory.HybridCandidateJudgeCandidate{
				{Ordinal: 0, Content: "zero"},
				{Ordinal: 1, Content: "one"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := usermemory.DecodeHybridCandidateJudgeOutput(result.RawOutput, 2)
	if err != nil || len(selected) != 1 || selected[0] != 1 ||
		result.ModelID != "Pro/test/judge" ||
		result.PromptVersion != usermemory.HybridCandidateJudgePromptVersion ||
		result.PromptSHA256 != usermemory.HybridCandidateJudgePromptSHA256 {
		t.Fatalf("result=%#v selected=%v err=%v", result, selected, err)
	}
	if provider.request.UseReasoning || !provider.request.DisableThinking ||
		provider.request.MaxOutputTokens != usermemory.HybridCandidateJudgeMaximumOutputTokens ||
		provider.request.Temperature == nil || *provider.request.Temperature != 0 ||
		provider.request.SystemPrompt == "" ||
		!strings.Contains(provider.request.SystemPrompt, "untrusted data") ||
		!strings.Contains(provider.request.Prompt, `"ordinal":1`) {
		t.Fatalf("request=%#v", provider.request)
	}
}

func TestChatAdapterRejectsMalformedAndOversizeOutput(t *testing.T) {
	for _, chunks := range [][]string{
		{`{"selectedOrdinals":[]}`},
		{strings.Repeat("x", usermemory.HybridCandidateJudgeMaximumOutputBytes+1)},
	} {
		provider := &judgeChatProvider{chunks: chunks}
		adapter, err := NewChatAdapter(provider, chat.ModelRef{
			ProviderID: "siliconflow", ModelID: "Pro/test/judge",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.JudgeHybridCandidates(
			context.Background(),
			usermemory.HybridCandidateJudgeInput{
				Query: "query",
				Candidates: []usermemory.HybridCandidateJudgeCandidate{
					{Ordinal: 0, Content: "zero"},
				},
			},
		); err == nil {
			t.Fatalf("output accepted: %q", chunks[0])
		}
	}
}

type judgeChatProvider struct {
	chunks  []string
	request chat.ProviderRequest
}

func (provider *judgeChatProvider) StreamChat(
	_ context.Context,
	request chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	provider.request = request
	events := make(chan chat.ProviderEvent, len(provider.chunks))
	for _, chunk := range provider.chunks {
		events <- chat.ProviderEvent{Type: chat.ProviderEventDelta, Delta: chunk}
	}
	close(events)
	return events, nil
}

var _ chat.Provider = (*judgeChatProvider)(nil)
