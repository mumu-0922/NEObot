package memoryjudge

import (
	"context"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const (
	TransportStableFirstRetryDelay  = 5 * time.Second
	TransportStableSecondRetryDelay = 10 * time.Second
	TransportStableMaximumRetries   = 2
)

type transportStableWait func(context.Context, time.Duration) error

// TransportStableCandidateJudge applies the owner-promoted schema-v14 retry
// boundary to one strict candidate judge. Only typed transient Provider
// failures retry; deterministic input/output/provenance failures fail closed.
type TransportStableCandidateJudge struct {
	delegate usermemory.HybridCandidateJudge
	wait     transportStableWait
}

func NewTransportStableCandidateJudge(
	delegate usermemory.HybridCandidateJudge,
) (*TransportStableCandidateJudge, error) {
	return newTransportStableCandidateJudge(delegate, waitTransportStable)
}

func newTransportStableCandidateJudge(
	delegate usermemory.HybridCandidateJudge,
	wait transportStableWait,
) (*TransportStableCandidateJudge, error) {
	if delegate == nil || wait == nil {
		return nil, errors.New("Memory candidate judge retry dependency is required")
	}
	return &TransportStableCandidateJudge{delegate: delegate, wait: wait}, nil
}

func (judge *TransportStableCandidateJudge) JudgeHybridCandidates(
	ctx context.Context,
	input usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	if judge == nil || judge.delegate == nil || judge.wait == nil {
		return usermemory.HybridCandidateJudgeResult{}, NewFailure(
			FailureInputInvalid,
			errors.New("Memory candidate judge retry dependency is unavailable"),
		)
	}
	for attempt := 0; attempt <= TransportStableMaximumRetries; attempt++ {
		result, err := judge.delegate.JudgeHybridCandidates(ctx, input)
		if err == nil {
			return result, nil
		}
		if attempt == TransportStableMaximumRetries || ctx.Err() != nil {
			return usermemory.HybridCandidateJudgeResult{}, err
		}
		if _, retryable := chat.ProviderRetryDelay(err); !retryable {
			return usermemory.HybridCandidateJudgeResult{}, err
		}
		delay := TransportStableFirstRetryDelay
		if attempt == 1 {
			delay = TransportStableSecondRetryDelay
		}
		if explicit, ok := chat.ProviderExplicitRetryDelay(err); ok {
			delay = explicit
		}
		if err := judge.wait(ctx, delay); err != nil {
			return usermemory.HybridCandidateJudgeResult{}, err
		}
	}
	return usermemory.HybridCandidateJudgeResult{}, NewFailure(
		FailureUnclassified,
		errors.New("Memory candidate judge retry state is invalid"),
	)
}

func waitTransportStable(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

var _ usermemory.HybridCandidateJudge = (*TransportStableCandidateJudge)(nil)
