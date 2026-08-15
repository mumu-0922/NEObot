package agentproductcanary

import (
	"context"
	"errors"
	"regexp"
	"time"
)

type Config struct {
	ActivationID string
	ClaimOwner   string
	PollInterval time.Duration
	ClaimTTL     time.Duration
	BatchSize    int
}

type Service struct {
	config     Config
	plan       Plan
	gate       Gate
	repository Repository
	executor   Executor
	now        func() time.Time
}

func NewService(config Config, plan Plan, gate Gate, repository Repository, executor Executor) (*Service, error) {
	if !idPattern.MatchString(config.ActivationID) || len(config.ClaimOwner) < 1 || len(config.ClaimOwner) > 128 ||
		config.PollInterval < time.Second || config.PollInterval > time.Minute ||
		config.ClaimTTL < 30*time.Second || config.ClaimTTL > 5*time.Minute ||
		config.ClaimTTL <= time.Duration(plan.LeaseSeconds)*time.Second ||
		config.BatchSize < 1 || config.BatchSize > 20 || ValidatePlan(plan) != nil ||
		plan.ActivationID != config.ActivationID || gate == nil || repository == nil || executor == nil {
		return nil, ErrInvalid
	}
	return &Service{config: config, plan: plan, gate: gate, repository: repository,
		executor: executor, now: time.Now}, nil
}

func (service *Service) Run(ctx context.Context) error {
	for {
		if _, err := service.Cycle(ctx); err != nil && !errors.Is(err, ErrUnavailable) {
			return err
		}
		if err := wait(ctx, service.config.PollInterval); err != nil {
			return err
		}
	}
}

func (service *Service) Health(ctx context.Context) error {
	now := service.now().UTC()
	if err := service.gate.Verify(now); err != nil {
		return ErrUnavailable
	}
	activation, err := service.repository.GetActivation(ctx, service.config.ActivationID)
	if err != nil || !service.activationCurrent(activation, now) {
		return ErrUnavailable
	}
	health, err := service.repository.Health(ctx, activation.ID, now)
	if err != nil || health.StaleClaims != 0 || health.PendingTerminalizations != 0 ||
		service.executor.Health(ctx) != nil {
		return ErrUnavailable
	}
	return nil
}

func (service *Service) Cycle(ctx context.Context) (bool, error) {
	now := service.now().UTC()
	if err := service.gate.Verify(now); err != nil {
		return false, ErrUnavailable
	}
	activation, err := service.repository.GetActivation(ctx, service.config.ActivationID)
	if err != nil || !service.activationCurrent(activation, now) {
		return false, ErrUnavailable
	}
	if _, err := service.repository.Reconcile(ctx, activation.ID, now, service.config.BatchSize); err != nil {
		return false, ErrUnavailable
	}
	requests, err := service.repository.Claim(ctx, activation.ID, service.config.ClaimOwner,
		now, service.config.ClaimTTL, 1)
	if err != nil {
		return false, ErrUnavailable
	}
	if len(requests) == 0 {
		return false, nil
	}
	request := requests[0]
	if !service.requestCurrent(request, activation) {
		_, _ = service.repository.Release(ctx, request, "REQUEST_BINDING_DRIFT", false)
		return true, ErrUnavailable
	}
	result, err := service.executor.Execute(ctx, request)
	if err != nil {
		_, _ = service.repository.Release(ctx, request, "PRODUCT_CANARY_EXECUTION_FAILED", true)
		return true, ErrUnavailable
	}
	if _, err := service.repository.Complete(ctx, request, result); err != nil {
		return true, ErrUnavailable
	}
	return true, nil
}

func (service *Service) activationCurrent(activation Activation, now time.Time) bool {
	return activation.ID == service.config.ActivationID && activation.Enabled &&
		activation.PlanFingerprint == service.plan.Fingerprint &&
		activation.PackageFingerprint == service.plan.Sandbox.PackageFingerprint &&
		activation.RuntimeBundleFingerprint == service.plan.Sandbox.RuntimeBundleFingerprint &&
		!now.Before(activation.ValidFrom) && now.Before(activation.ValidUntil)
}

func (service *Service) requestCurrent(request Request, activation Activation) bool {
	return request.ActivationID == activation.ID && request.State == "claimed" &&
		request.ClaimGeneration >= 1 && request.ClaimOwner == service.config.ClaimOwner &&
		request.PolicyRevision == activation.PolicyRevision &&
		request.PackageFingerprint == activation.PackageFingerprint &&
		request.RuntimeBundleFingerprint == activation.RuntimeBundleFingerprint &&
		request.PlanFingerprint == activation.PlanFingerprint &&
		idPattern.MatchString(request.ID) && uuidPattern.MatchString(request.UserID) &&
		fingerprintPattern.MatchString(request.RequestFingerprint)
}

var (
	uuidPattern        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	fingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
