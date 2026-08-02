package memoryjudge

import (
	"context"
	"errors"
	"strings"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const ChatAdapterVersion = "chat-configured-candidate-judge-v1"

// ChatAdapter binds the shared strict Memory judge contract to one already
// authorized chat Provider/model. Query, candidate content, and raw output
// remain request-local and are never logged by this adapter.
type ChatAdapter struct {
	provider chat.Provider
	modelRef chat.ModelRef
}

func NewChatAdapter(provider chat.Provider, modelRef chat.ModelRef) (*ChatAdapter, error) {
	modelRef.ProviderID = strings.TrimSpace(modelRef.ProviderID)
	modelRef.ModelID = strings.TrimSpace(modelRef.ModelID)
	if provider == nil || modelRef.ProviderID == "" || modelRef.ModelID == "" {
		return nil, errors.New("Memory candidate judge Provider/model is required")
	}
	return &ChatAdapter{provider: provider, modelRef: modelRef}, nil
}

func (adapter *ChatAdapter) JudgeHybridCandidates(
	ctx context.Context,
	input usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	if adapter == nil || adapter.provider == nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureInputInvalid,
			errors.New("Memory candidate judge is unavailable"),
		)
	}
	systemPrompt, prompt, err := usermemory.BuildHybridCandidateJudgePrompt(input)
	if err != nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(FailureInputInvalid, err)
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	temperature := 0.0
	events, err := adapter.provider.StreamChat(requestCtx, chat.ProviderRequest{
		Prompt:          prompt,
		SystemPrompt:    systemPrompt,
		UseReasoning:    false,
		DisableThinking: true,
		MaxOutputTokens: usermemory.HybridCandidateJudgeMaximumOutputTokens,
		Temperature:     &temperature,
		ModelRef:        adapter.modelRef,
	})
	if err != nil {
		return usermemory.HybridCandidateJudgeResult{}, errors.Join(
			errors.New("Memory candidate judge Provider failed"),
			err,
		)
	}
	output := make([]byte, 0, 256)
	for event := range events {
		if event.Error != nil {
			return usermemory.HybridCandidateJudgeResult{}, errors.Join(
				errors.New("Memory candidate judge Provider failed"),
				event.Error,
			)
		}
		switch event.Type {
		case chat.ProviderEventDelta:
			if len(output)+len(event.Delta) > usermemory.HybridCandidateJudgeMaximumOutputBytes {
				cancel()
				return usermemory.HybridCandidateJudgeResult{}, NewFailure(
					FailureOutputTooLarge,
					errors.New("Memory candidate judge output is too large"),
				)
			}
			output = append(output, event.Delta...)
		case chat.ProviderEventReasoningDelta, chat.ProviderEventUsage:
			// Reasoning is never accepted as contract output. Usage is not
			// retained; isolated capture uses a conservative token upper bound.
		default:
			return usermemory.HybridCandidateJudgeResult{}, NewFailure(
				FailureEventInvalid,
				errors.New("Memory candidate judge Provider event is invalid"),
			)
		}
	}
	if requestCtx.Err() != nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureCategory(requestCtx.Err()),
			requestCtx.Err(),
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

var _ usermemory.HybridCandidateJudge = (*ChatAdapter)(nil)
