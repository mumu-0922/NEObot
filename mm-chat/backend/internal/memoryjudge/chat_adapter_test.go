package memoryjudge

import (
	"context"
	"errors"
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

func TestChatAdapterReturnsBoundedFailureCategories(t *testing.T) {
	validInput := usermemory.HybridCandidateJudgeInput{
		Query: "private query",
		Candidates: []usermemory.HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "private Memory"},
		},
	}
	tests := []struct {
		name     string
		provider *judgeChatProvider
		input    usermemory.HybridCandidateJudgeInput
		ctx      func() context.Context
		want     string
	}{
		{
			name: "input", provider: &judgeChatProvider{},
			input: usermemory.HybridCandidateJudgeInput{}, want: FailureInputInvalid,
		},
		{
			name: "too large",
			provider: &judgeChatProvider{chunks: []string{
				strings.Repeat("x", usermemory.HybridCandidateJudgeMaximumOutputBytes+1),
			}},
			input: validInput, want: FailureOutputTooLarge,
		},
		{
			name: "event", provider: &judgeChatProvider{
				events: []chat.ProviderEvent{{Type: "private-event"}},
			},
			input: validInput, want: FailureEventInvalid,
		},
		{
			name: "json", provider: &judgeChatProvider{chunks: []string{"{"}},
			input: validInput, want: FailureOutputJSONInvalid,
		},
		{
			name: "schema", provider: &judgeChatProvider{chunks: []string{
				`{"schemaVersion":"drifted","selectedOrdinals":[]}`,
			}},
			input: validInput, want: FailureOutputSchemaInvalid,
		},
		{
			name: "ordinal", provider: &judgeChatProvider{chunks: []string{
				`{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[1]}`,
			}},
			input: validInput, want: FailureOutputOrdinalInvalid,
		},
		{
			name: "Provider cancellation", provider: &judgeChatProvider{err: context.Canceled},
			input: validInput, want: string(chat.ProviderFailureContextCanceled),
		},
		{
			name: "unknown", provider: &judgeChatProvider{
				err: errors.New("private Provider response"),
			},
			input: validInput, want: FailureUnclassified,
		},
		{
			name: "canceled context", provider: &judgeChatProvider{},
			input: validInput, ctx: canceledJudgeContext,
			want: string(chat.ProviderFailureContextCanceled),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, err := NewChatAdapter(test.provider, chat.ModelRef{
				ProviderID: "fixture", ModelID: "fixture-model",
			})
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if test.ctx != nil {
				ctx = test.ctx()
			}
			_, err = adapter.JudgeHybridCandidates(ctx, test.input)
			if got := FailureCategory(err); got != test.want {
				t.Fatalf("category=%q want=%q err=%v", got, test.want, err)
			}
		})
	}
}

func canceledJudgeContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

type judgeChatProvider struct {
	chunks  []string
	events  []chat.ProviderEvent
	err     error
	request chat.ProviderRequest
}

func (provider *judgeChatProvider) StreamChat(
	_ context.Context,
	request chat.ProviderRequest,
) (<-chan chat.ProviderEvent, error) {
	provider.request = request
	if provider.err != nil {
		return nil, provider.err
	}
	events := make(chan chat.ProviderEvent, len(provider.chunks)+len(provider.events))
	for _, chunk := range provider.chunks {
		events <- chat.ProviderEvent{Type: chat.ProviderEventDelta, Delta: chunk}
	}
	for _, event := range provider.events {
		events <- event
	}
	close(events)
	return events, nil
}

var _ chat.Provider = (*judgeChatProvider)(nil)
