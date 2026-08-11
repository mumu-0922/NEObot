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

func TestBufferedChatAccuracyAdapterChangesOnlyPromptIdentity(t *testing.T) {
	legacyProvider := &bufferedJudgeProvider{completion: chat.BufferedChatCompletion{
		Content: validBufferedJudgeOutput(),
	}}
	accuracyProvider := &bufferedJudgeProvider{completion: chat.BufferedChatCompletion{
		Content: validBufferedJudgeOutput(),
	}}
	modelRef := chat.ModelRef{ProviderID: "fixture", ModelID: "fixture-model"}
	legacy, err := NewBufferedChatAdapter(legacyProvider, modelRef)
	if err != nil {
		t.Fatal(err)
	}
	accuracy, err := NewBufferedChatAccuracyAdapter(accuracyProvider, modelRef)
	if err != nil {
		t.Fatal(err)
	}
	input := transportStableTestInput()
	legacyResult, err := legacy.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	accuracyResult, err := accuracy.JudgeHybridCandidates(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	legacySystem := legacyProvider.request.SystemPrompt
	accuracySystem := accuracyProvider.request.SystemPrompt
	legacyRequest := legacyProvider.request
	accuracyRequest := accuracyProvider.request
	legacyRequest.SystemPrompt = ""
	accuracyRequest.SystemPrompt = ""
	if legacySystem == accuracySystem || !reflect.DeepEqual(legacyRequest, accuracyRequest) ||
		legacyResult.PromptVersion != usermemory.HybridCandidateJudgePromptVersion ||
		accuracyResult.PromptVersion != usermemory.HybridCandidateJudgeAccuracyPromptVersion ||
		accuracyResult.PromptSHA256 != usermemory.HybridCandidateJudgeAccuracyPromptSHA256 {
		t.Fatalf("legacyRequest=%#v accuracyRequest=%#v legacyResult=%#v accuracyResult=%#v",
			legacyProvider.request, accuracyProvider.request, legacyResult, accuracyResult)
	}
}

func TestBufferedChatAbstentionConfirmationAdapterSelectsExactPromptPurpose(t *testing.T) {
	provider := &bufferedJudgeProvider{completion: chat.BufferedChatCompletion{
		Content: validBufferedJudgeOutput(),
	}}
	adapter, err := NewBufferedChatAbstentionConfirmationAdapter(
		provider,
		chat.ModelRef{ProviderID: "fixture", ModelID: "fixture-model"},
	)
	if err != nil {
		t.Fatal(err)
	}
	input := transportStableTestInput()
	input.PromptPurpose = usermemory.HybridCandidateJudgePromptPurposeAccuracyPrimary
	primary, err := adapter.JudgeHybridCandidates(context.Background(), input)
	if err != nil || primary.PromptVersion != usermemory.HybridCandidateJudgeAccuracyPromptVersion {
		t.Fatalf("primary=%#v err=%v", primary, err)
	}
	primarySystem := provider.request.SystemPrompt
	input.PromptPurpose =
		usermemory.HybridCandidateJudgePromptPurposeAbstentionConfirmation
	confirmation, err := adapter.JudgeHybridCandidates(context.Background(), input)
	if err != nil ||
		confirmation.PromptVersion != usermemory.HybridCandidateJudgeConfirmationPromptVersion ||
		confirmation.PromptSHA256 != usermemory.HybridCandidateJudgeConfirmationPromptSHA256 ||
		provider.request.SystemPrompt == primarySystem {
		t.Fatalf("confirmation=%#v err=%v request=%#v", confirmation, err, provider.request)
	}
	input.PromptPurpose = "drifted"
	if _, err := adapter.JudgeHybridCandidates(context.Background(), input); err == nil ||
		FailureCategory(err) != FailureInputInvalid {
		t.Fatalf("invalid purpose err=%v", err)
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
