package memorycapture

import (
	"context"
	"errors"
	"fmt"

	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

type CandidateJudgeDecorator struct {
	judge                     usermemory.HybridCandidateJudge
	recorder                  *Recorder
	expectedModelID           string
	expectedPromptVersion     string
	expectedPromptSHA256      string
	promptBuilder             candidateJudgeCapturePromptBuilder
	confirmationRequired      bool
	maximumConfirmations      int
	confirmationPromptBuilder candidateJudgeCapturePromptBuilder
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

func NewAbstentionConfirmationCandidateJudgeDecorator(
	judge usermemory.HybridCandidateJudge,
	recorder *Recorder,
	expectedModelID string,
) (*CandidateJudgeDecorator, error) {
	return newAbstentionConfirmationCandidateJudgeDecorator(
		judge,
		recorder,
		expectedModelID,
		1,
	)
}

func NewDoubleConfirmationCandidateJudgeDecorator(
	judge usermemory.HybridCandidateJudge,
	recorder *Recorder,
	expectedModelID string,
) (*CandidateJudgeDecorator, error) {
	return newAbstentionConfirmationCandidateJudgeDecorator(
		judge,
		recorder,
		expectedModelID,
		2,
	)
}

func newAbstentionConfirmationCandidateJudgeDecorator(
	judge usermemory.HybridCandidateJudge,
	recorder *Recorder,
	expectedModelID string,
	maximumConfirmations int,
) (*CandidateJudgeDecorator, error) {
	if maximumConfirmations < 1 || maximumConfirmations > 2 {
		return nil, ErrCaptureInvalid
	}
	decorator, err := newCandidateJudgeDecorator(
		judge,
		recorder,
		expectedModelID,
		usermemory.HybridCandidateJudgeAccuracyPromptVersion,
		usermemory.HybridCandidateJudgeAccuracyPromptSHA256,
		usermemory.BuildHybridCandidateJudgeAccuracyPrompt,
	)
	if err != nil {
		return nil, err
	}
	decorator.confirmationRequired = true
	decorator.maximumConfirmations = maximumConfirmations
	decorator.confirmationPromptBuilder =
		usermemory.BuildHybridCandidateJudgeConfirmationPrompt
	return decorator, nil
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
	stage := "cloud_judge"
	promptBuilder := decorator.promptBuilder
	expectedPromptVersion := decorator.expectedPromptVersion
	expectedPromptSHA256 := decorator.expectedPromptSHA256
	confirmation := false
	if decorator.confirmationRequired {
		switch input.PromptPurpose {
		case usermemory.HybridCandidateJudgePromptPurposeAccuracyPrimary:
		case usermemory.HybridCandidateJudgePromptPurposeAbstentionConfirmation:
			confirmation = true
			stage = "cloud_judge_confirmation"
			promptBuilder = decorator.confirmationPromptBuilder
			expectedPromptVersion =
				usermemory.HybridCandidateJudgeConfirmationPromptVersion
			expectedPromptSHA256 =
				usermemory.HybridCandidateJudgeConfirmationPromptSHA256
		default:
			return usermemory.HybridCandidateJudgeResult{}, memoryjudge.NewFailure(
				memoryjudge.FailureInputInvalid,
				ErrCaptureStateConflict,
			)
		}
	}
	if err := decorator.recorder.recordProviderSent(
		stage,
		len(input.Candidates),
		decorator.maximumConfirmations,
	); err != nil {
		return usermemory.HybridCandidateJudgeResult{}, memoryjudge.NewFailure(
			memoryjudge.FailureRecorderStateConflict,
			fmt.Errorf("capture hybrid cloud-judge egress: %w", err),
		)
	}
	var inputErr error
	if confirmation {
		inputErr = decorator.recorder.recordCloudJudgeConfirmationInputWithPrompt(
			input,
			promptBuilder,
		)
	} else {
		inputErr = decorator.recorder.recordCloudJudgeInputWithPrompt(input, promptBuilder)
	}
	if inputErr != nil {
		return usermemory.HybridCandidateJudgeResult{}, memoryjudge.NewFailure(
			memoryjudge.FailureRecorderStateConflict,
			fmt.Errorf("capture hybrid cloud-judge input: %w", inputErr),
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
		result.PromptVersion != expectedPromptVersion ||
		result.PromptSHA256 != expectedPromptSHA256 {
		provenanceErr := memoryjudge.NewFailure(
			memoryjudge.FailureProvenanceDrift,
			ErrCaptureStateConflict,
		)
		return usermemory.HybridCandidateJudgeResult{},
			decorator.recordCloudJudgeFailure(provenanceErr)
	}
	selected, err := usermemory.DecodeHybridCandidateJudgeOutput(
		result.RawOutput,
		len(input.Candidates),
	)
	if err != nil {
		return usermemory.HybridCandidateJudgeResult{},
			decorator.recordCloudJudgeFailure(err)
	}
	if decorator.confirmationRequired && len(selected) == 0 {
		if err := decorator.recorder.recordCloudJudgeEmptyResultWithPrompt(
			result,
			len(input.Candidates),
			expectedPromptVersion,
			expectedPromptSHA256,
			confirmation,
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
	if err := decorator.recorder.recordCloudJudgeResultWithPrompt(
		result,
		len(input.Candidates),
		expectedPromptVersion,
		expectedPromptSHA256,
		confirmation,
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
