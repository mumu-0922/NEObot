package memorycapture

import (
	"context"
	"errors"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

type CandidateJudgeDecorator struct {
	judge                 usermemory.HybridCandidateJudge
	recorder              *Recorder
	expectedModelID       string
	expectedPromptVersion string
	expectedPromptSHA256  string
	promptBuilder         candidateJudgeCapturePromptBuilder
}

func NewCandidateJudgeDecorator(
	judge usermemory.HybridCandidateJudge,
	recorder *Recorder,
	expectedModelID string,
) (*CandidateJudgeDecorator, error) {
	if judge == nil || recorder == nil || expectedModelID == "" {
		return nil, ErrCaptureInvalid
	}
	return newCandidateJudgeDecorator(
		judge,
		recorder,
		expectedModelID,
		usermemory.HybridCandidateJudgePromptVersion,
		usermemory.HybridCandidateJudgePromptSHA256,
		usermemory.BuildHybridCandidateJudgePrompt,
	)
}

func NewAccuracyRepairCandidateJudgeDecorator(
	judge usermemory.HybridCandidateJudge,
	recorder *Recorder,
	expectedModelID string,
) (*CandidateJudgeDecorator, error) {
	return newCandidateJudgeDecorator(
		judge,
		recorder,
		expectedModelID,
		usermemory.HybridCandidateJudgeAccuracyPromptVersion,
		usermemory.HybridCandidateJudgeAccuracyPromptSHA256,
		usermemory.BuildHybridCandidateJudgeAccuracyPrompt,
	)
}

func newCandidateJudgeDecorator(
	judge usermemory.HybridCandidateJudge,
	recorder *Recorder,
	expectedModelID string,
	expectedPromptVersion string,
	expectedPromptSHA256 string,
	promptBuilder candidateJudgeCapturePromptBuilder,
) (*CandidateJudgeDecorator, error) {
	if judge == nil || recorder == nil || expectedModelID == "" ||
		expectedPromptVersion == "" || expectedPromptSHA256 == "" || promptBuilder == nil {
		return nil, ErrCaptureInvalid
	}
	return &CandidateJudgeDecorator{
		judge: judge, recorder: recorder, expectedModelID: expectedModelID,
		expectedPromptVersion: expectedPromptVersion,
		expectedPromptSHA256:  expectedPromptSHA256,
		promptBuilder:         promptBuilder,
	}, nil
}

func (decorator *CandidateJudgeDecorator) JudgeHybridCandidates(
	ctx context.Context,
	input usermemory.HybridCandidateJudgeInput,
) (usermemory.HybridCandidateJudgeResult, error) {
	if err := decorator.recorder.recordProviderSent(
		"cloud_judge",
		len(input.Candidates),
	); err != nil {
		return usermemory.HybridCandidateJudgeResult{}, memoryjudge.NewFailure(
			memoryjudge.FailureRecorderStateConflict,
			fmt.Errorf("capture hybrid cloud-judge egress: %w", err),
		)
	}
	if err := decorator.recorder.recordCloudJudgeInputWithPrompt(
		input,
		decorator.promptBuilder,
	); err != nil {
		return usermemory.HybridCandidateJudgeResult{}, memoryjudge.NewFailure(
			memoryjudge.FailureRecorderStateConflict,
			fmt.Errorf("capture hybrid cloud-judge input: %w", err),
		)
	}
	result, err := decorator.judge.JudgeHybridCandidates(ctx, input)
	if err != nil {
		return result, decorator.recordCloudJudgeFailure(err)
	}
	if ctx.Err() != nil {
		return usermemory.HybridCandidateJudgeResult{},
			decorator.recordCloudJudgeFailure(ctx.Err())
	}
	if result.ModelID != decorator.expectedModelID ||
		result.PromptVersion != decorator.expectedPromptVersion ||
		result.PromptSHA256 != decorator.expectedPromptSHA256 {
		provenanceErr := memoryjudge.NewFailure(
			memoryjudge.FailureProvenanceDrift,
			ErrCaptureStateConflict,
		)
		return usermemory.HybridCandidateJudgeResult{},
			decorator.recordCloudJudgeFailure(provenanceErr)
	}
	if _, err := usermemory.DecodeHybridCandidateJudgeOutput(
		result.RawOutput,
		len(input.Candidates),
	); err != nil {
		return usermemory.HybridCandidateJudgeResult{},
			decorator.recordCloudJudgeFailure(err)
	}
	if err := decorator.recorder.recordCloudJudgeResultWithPrompt(
		result,
		len(input.Candidates),
		decorator.expectedPromptVersion,
		decorator.expectedPromptSHA256,
	); err != nil {
		recorderErr := memoryjudge.NewFailure(
			memoryjudge.FailureRecorderStateConflict,
			err,
		)
		return usermemory.HybridCandidateJudgeResult{},
			decorator.recordCloudJudgeFailure(recorderErr)
	}
	return result, nil
}

func (decorator *CandidateJudgeDecorator) recordCloudJudgeFailure(cause error) error {
	category := memoryjudge.FailureCategory(cause)
	typed := memoryjudge.NewFailure(category, cause)
	if err := decorator.recorder.recordCloudJudgeFailure(category); err != nil {
		return memoryjudge.NewFailure(
			memoryjudge.FailureRecorderStateConflict,
			errors.Join(typed, err),
		)
	}
	return typed
}
