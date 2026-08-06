package memoryjudge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

func TestBufferedChatAdapterPreservesStrictStreamingRequestContract(t *testing.T) {
	streaming := &judgeChatProvider{chunks: []string{validBufferedJudgeOutput()}}
	buffered := &bufferedJudgeProvider{completion: chat.BufferedChatCompletion{
		Content: validBufferedJudgeOutput(),
	}}
	modelRef := chat.ModelRef{ProviderID: "fixture", ModelID: "fixture-model"}
	streamingAdapter, err := NewChatAdapter(streaming, modelRef)
	if err != nil {
		t.Fatal(err)
	}
	bufferedAdapter, err := NewBufferedChatAdapter(buffered, modelRef)
	if err != nil {
		t.Fatal(err)
	}
	input := transportStableTestInput()
	streamingResult, err := streamingAdapter.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	bufferedResult, err := bufferedAdapter.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(streaming.request, buffered.request) ||
		!reflect.DeepEqual(streamingResult.RawOutput, bufferedResult.RawOutput) ||
		bufferedResult.ModelID != modelRef.ModelID ||
		bufferedResult.PromptVersion != usermemory.HybridCandidateJudgePromptVersion ||
		bufferedResult.PromptSHA256 != usermemory.HybridCandidateJudgePromptSHA256 {
		t.Fatalf(
			"streamingRequest=%#v bufferedRequest=%#v streamingResult=%#v bufferedResult=%#v",
			streaming.request,
			buffered.request,
			streamingResult,
			bufferedResult,
		)
	}
}

func TestBufferedChatAdapterFailsClosedWithoutLeakingProviderDetails(t *testing.T) {
	validInput := transportStableTestInput()
	tests := []struct {
		name     string
		provider *bufferedJudgeProvider
		input    usermemory.HybridCandidateJudgeInput
		want     string
	}{
		{
			name: "input", provider: &bufferedJudgeProvider{},
			input: usermemory.HybridCandidateJudgeInput{}, want: FailureInputInvalid,
		},
		{
			name: "empty", provider: &bufferedJudgeProvider{}, input: validInput,
			want: FailureOutputJSONInvalid,
		},
		{
			name: "oversize", provider: &bufferedJudgeProvider{
				completion: chat.BufferedChatCompletion{
					Content: strings.Repeat("x", usermemory.HybridCandidateJudgeMaximumOutputBytes+1),
				},
			}, input: validInput, want: FailureOutputTooLarge,
		},
		{
			name: "schema", provider: &bufferedJudgeProvider{
				completion: chat.BufferedChatCompletion{Content: `{"selectedOrdinals":[]}`},
			}, input: validInput, want: FailureOutputSchemaInvalid,
		},
		{
			name: "Provider", provider: &bufferedJudgeProvider{
				err: errors.New("private upstream response"),
			}, input: validInput, want: FailureUnclassified,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter, err := NewBufferedChatAdapter(test.provider, chat.ModelRef{
				ProviderID: "fixture", ModelID: "fixture-model",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.JudgeHybridCandidates(context.Background(), test.input)
			if got := FailureCategory(err); got != test.want ||
				strings.Contains(err.Error(), "private upstream response") {
				t.Fatalf("category=%q want=%q err=%v", got, test.want, err)
			}
		})
	}
}

func validBufferedJudgeOutput() string {
	return `{"schemaVersion":"neo-chat.memory-cloud-candidate-judge-output.v1","selectedOrdinals":[0]}`
}

type bufferedJudgeProvider struct {
	completion chat.BufferedChatCompletion
	err        error
	request    chat.ProviderRequest
}

func (provider *bufferedJudgeProvider) CompleteChat(
	_ context.Context,
	request chat.ProviderRequest,
) (chat.BufferedChatCompletion, error) {
	provider.request = request
	return provider.completion, provider.err
}

var _ chat.BufferedChatProvider = (*bufferedJudgeProvider)(nil)
