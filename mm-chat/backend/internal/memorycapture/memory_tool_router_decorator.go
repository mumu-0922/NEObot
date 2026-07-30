package memorycapture

import (
	"context"
	"errors"

	"neo-chat/mm-chat/backend/internal/usermemory"
)

type MemoryToolRouterDecorator struct {
	router          usermemory.HybridMemoryToolRouter
	recorder        *Recorder
	expectedModelID string
}

type memoryToolRouterCallResult struct {
	result usermemory.HybridMemoryToolRouteResult
	err    error
}

func NewMemoryToolRouterDecorator(
	router usermemory.HybridMemoryToolRouter,
	recorder *Recorder,
	expectedModelID string,
) (*MemoryToolRouterDecorator, error) {
	if router == nil || recorder == nil || expectedModelID == "" {
		return nil, ErrCaptureInvalid
	}
	return &MemoryToolRouterDecorator{
		router: router, recorder: recorder, expectedModelID: expectedModelID,
	}, nil
}

func (decorator *MemoryToolRouterDecorator) RouteHybridMemory(
	ctx context.Context,
	input usermemory.HybridMemoryToolRouteInput,
) (usermemory.HybridMemoryToolRouteResult, error) {
	token, err := decorator.recorder.recordMemoryToolRouteInput(input)
	if err != nil {
		return usermemory.HybridMemoryToolRouteResult{},
			usermemory.NewHybridMemoryToolRouteError(
				usermemory.HybridMemoryToolRouteFailureRecorderStateConflict,
			)
	}
	callResult := make(chan memoryToolRouterCallResult, 1)
	go func() {
		result, callErr := decorator.router.RouteHybridMemory(ctx, input)
		callResult <- memoryToolRouterCallResult{result: result, err: callErr}
	}()
	var completed memoryToolRouterCallResult
	select {
	case completed = <-callResult:
	case <-ctx.Done():
		category := memoryToolRouteFailureCategory(ctx, ctx.Err())
		if recordErr := decorator.recorder.recordMemoryToolRouteFailure(
			token,
			category,
		); recordErr != nil {
			return usermemory.HybridMemoryToolRouteResult{},
				usermemory.NewHybridMemoryToolRouteError(
					usermemory.HybridMemoryToolRouteFailureRecorderStateConflict,
				)
		}
		return usermemory.HybridMemoryToolRouteResult{},
			usermemory.NewHybridMemoryToolRouteError(category)
	}
	result, err := completed.result, completed.err
	if err != nil || ctx.Err() != nil {
		category := memoryToolRouteFailureCategory(ctx, err)
		if recordErr := decorator.recorder.recordMemoryToolRouteFailure(
			token,
			category,
		); recordErr != nil {
			return usermemory.HybridMemoryToolRouteResult{},
				usermemory.NewHybridMemoryToolRouteError(
					usermemory.HybridMemoryToolRouteFailureRecorderStateConflict,
				)
		}
		return result, usermemory.NewHybridMemoryToolRouteError(category)
	}
	if result.ModelID != decorator.expectedModelID ||
		result.ContractVersion != usermemory.HybridMemoryToolContractVersion ||
		result.ContractSHA256 != usermemory.HybridMemoryToolContractSHA256 ||
		result.OutputTokenUpperBound <= 0 {
		if recordErr := decorator.recorder.recordMemoryToolRouteFailure(
			token,
			usermemory.HybridMemoryToolRouteFailureProvenanceDrift,
		); recordErr != nil {
			return usermemory.HybridMemoryToolRouteResult{},
				usermemory.NewHybridMemoryToolRouteError(
					usermemory.HybridMemoryToolRouteFailureRecorderStateConflict,
				)
		}
		return usermemory.HybridMemoryToolRouteResult{},
			usermemory.NewHybridMemoryToolRouteError(
				usermemory.HybridMemoryToolRouteFailureProvenanceDrift,
			)
	}
	if err := decorator.recorder.recordMemoryToolRouteResult(token, result); err != nil {
		return usermemory.HybridMemoryToolRouteResult{},
			usermemory.NewHybridMemoryToolRouteError(
				usermemory.HybridMemoryToolRouteFailureRecorderStateConflict,
			)
	}
	return result, nil
}

func memoryToolRouteFailureCategory(ctx context.Context, err error) string {
	if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return usermemory.HybridMemoryToolRouteFailureContextDeadline
	}
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return usermemory.HybridMemoryToolRouteFailureContextCanceled
	}
	if category := usermemory.HybridMemoryToolRouteFailureCategory(err); category != "" {
		return category
	}
	return usermemory.HybridMemoryToolRouteFailureUnclassified
}
