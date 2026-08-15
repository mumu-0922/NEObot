package agentproductcanary

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrootcanary"
)

type RootServiceFactory interface {
	Build(agentrootcanary.Plan) (*agentrootcanary.Service, error)
}

type RootExecutor struct {
	plan         Plan
	factory      RootServiceFactory
	pollInterval time.Duration
}

func NewRootExecutor(plan Plan, factory RootServiceFactory, pollInterval time.Duration) (*RootExecutor, error) {
	if ValidatePlan(plan) != nil || factory == nil || pollInterval < time.Second || pollInterval > time.Minute {
		return nil, ErrInvalid
	}
	return &RootExecutor{plan: plan, factory: factory, pollInterval: pollInterval}, nil
}

func (executor *RootExecutor) Health(ctx context.Context) error {
	if executor == nil {
		return ErrUnavailable
	}
	request := Request{ID: "product_request_healthcheck00001",
		UserID: "00000000-0000-4000-8000-000000000001", ActivationID: executor.plan.ActivationID,
		PackageFingerprint:       executor.plan.Sandbox.PackageFingerprint,
		RuntimeBundleFingerprint: executor.plan.Sandbox.RuntimeBundleFingerprint,
		PlanFingerprint:          executor.plan.Fingerprint}
	service, err := executor.factory.Build(executor.plan.ExecutionPlan(request))
	if err != nil || service.Health(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}

func (executor *RootExecutor) Execute(ctx context.Context, request Request) (ExecutionResult, error) {
	if executor == nil || request.ActivationID != executor.plan.ActivationID ||
		request.PlanFingerprint != executor.plan.Fingerprint ||
		request.PackageFingerprint != executor.plan.Sandbox.PackageFingerprint ||
		request.RuntimeBundleFingerprint != executor.plan.Sandbox.RuntimeBundleFingerprint {
		return ExecutionResult{}, ErrUnavailable
	}
	service, err := executor.factory.Build(executor.plan.ExecutionPlan(request))
	if err != nil {
		return ExecutionResult{}, ErrUnavailable
	}
	var result agentrootcanary.Result
	for {
		result, err = service.Cycle(ctx)
		if !errors.Is(err, agentrootcanary.ErrRecoveryPending) {
			break
		}
		if wait(ctx, executor.pollInterval) != nil {
			return ExecutionResult{}, ErrUnavailable
		}
	}
	if err != nil || !result.Completed || result.RunID == "" || result.AttemptID == "" ||
		!fingerprintPattern.MatchString(result.SnapshotFingerprint) {
		return ExecutionResult{}, ErrUnavailable
	}
	binding := []byte("neo.agent-product-canary-receipt/v1\x00" + request.ActivationID + "\x00" +
		request.ID + "\x00" + result.RunID + "\x00" + result.AttemptID + "\x00" +
		result.SnapshotFingerprint + "\x00" + request.PlanFingerprint)
	digest := sha256.Sum256(binding)
	return ExecutionResult{RunID: result.RunID, AttemptID: result.AttemptID,
		SnapshotFingerprint: result.SnapshotFingerprint,
		ReceiptFingerprint:  "sha256:" + hex.EncodeToString(digest[:])}, nil
}

var _ Executor = (*RootExecutor)(nil)
