package memoryjudge

import (
	"context"
	"errors"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const BufferedChatAdapterVersion = "chat-configured-candidate-judge-buffered-v1"

// BufferedChatAdapter preserves the shared strict candidate-Judge contract
// while selecting the Provider's explicit non-streaming completion capability.
type BufferedChatAdapter struct {
	provider chat.BufferedChatProvider
	modelRef chat.ModelRef
}

func NewBufferedChatAdapter(
	provider chat.BufferedChatProvider,
	modelRef chat.ModelRef,
) (*BufferedChatAdapter, error) {
	modelRef.ProviderID = strings.TrimSpace(modelRef.ProviderID)
	modelRef.ModelID = strings.TrimSpace(modelRef.ModelID)
	if provider == nil || modelRef.ProviderID == "" || modelRef.ModelID == "" {
		return nil, errors.New("Memory buffered candidate judge Provider/model is required")
	}
	return &BufferedChatAdapter{provider: provider, modelRef: modelRef}, nil
}

func (adapter *BufferedChatAdapter) JudgeHybridCandidates(
	ctx context.Context,
	input usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	if adapter == nil || adapter.provider == nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureInputInvalid,
			errors.New("Memory buffered candidate judge is unavailable"),
		)
	}
	systemPrompt, prompt, err := usermemory.BuildHybridCandidateJudgePrompt(input)
	if err != nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(FailureInputInvalid, err)
	}
	temperature := 0.0
	completed, err := adapter.provider.CompleteChat(ctx, chat.ProviderRequest{
		Prompt:          prompt,
		SystemPrompt:    systemPrompt,
		UseReasoning:    false,
		DisableThinking: true,
		MaxOutputTokens: usermemory.HybridCandidateJudgeMaximumOutputTokens,
		Temperature:     &temperature,
		ModelRef:        adapter.modelRef,
	})
	if err != nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureCategory(err),
			err,
		)
	}
	if ctx.Err() != nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureCategory(ctx.Err()),
			ctx.Err(),
		)
	}
	output := []byte(completed.Content)
	if len(output) > usermemory.HybridCandidateJudgeMaximumOutputBytes {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureOutputTooLarge,
			errors.New("Memory buffered candidate judge output is too large"),
		)
	}
	if _, err := usermemory.DecodeHybridCandidateJudgeOutput(output, len(input.Candidates)); err != nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureCategory(err),
			err,
		)
	}
	return usermemory.HybridCandidateJudgeResult{
		RawOutput:     append([]byte(nil), output...),
		ModelID:       adapter.modelRef.ModelID,
		PromptVersion: usermemory.HybridCandidateJudgePromptVersion,
		PromptSHA256:  usermemory.HybridCandidateJudgePromptSHA256,
	}, nil
}

var _ usermemory.HybridCandidateJudge = (*BufferedChatAdapter)(nil)
