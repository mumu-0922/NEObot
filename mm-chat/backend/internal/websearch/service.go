package websearch

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const (
	externalSearchMaxAttempts = 2
	externalSearchRetryDelay  = 250 * time.Millisecond
)

type Service struct {
	resolver   Resolver
	retryDelay time.Duration
}

func NewService(resolver Resolver) *Service {
	return &Service{
		resolver:   resolver,
		retryDelay: externalSearchRetryDelay,
	}
}

func (s *Service) Configured() bool {
	return s != nil && s.resolver != nil
}

func (s *Service) ResolveActive(ctx context.Context) (ActiveExecution, error) {
	return s.ResolveExternal(ctx)
}

func (s *Service) ResolveExternal(ctx context.Context) (ActiveExecution, error) {
	if !s.Configured() {
		return ActiveExecution{}, ErrNotConfigured
	}
	var execution ActiveExecution
	var err error
	if resolver, ok := s.resolver.(ModeResolver); ok {
		execution, err = resolver.ResolveExternal(ctx)
	} else {
		execution, err = s.resolver.ResolveActive(ctx)
		if err == nil && execution.Mode == ExecutionModelBuiltIn {
			if err := validateActiveExecution(execution); err != nil {
				return ActiveExecution{}, err
			}
			return ActiveExecution{}, ErrModelBuiltInRequiresChat
		}
	}
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return ActiveExecution{}, ErrNotConfigured
		}
		return ActiveExecution{}, ErrResolutionFailed
	}
	if err := validateActiveExecution(execution); err != nil {
		return ActiveExecution{}, err
	}
	if execution.Mode != ExecutionExternal {
		return ActiveExecution{}, ErrInvalidConfig
	}
	return execution, nil
}

func (s *Service) ResolveModelBuiltIn(
	ctx context.Context,
	request ModelBuiltInResolutionRequest,
) (ActiveExecution, error) {
	if !s.Configured() {
		return ActiveExecution{}, ErrNotConfigured
	}
	resolver, ok := s.resolver.(ModeResolver)
	if !ok {
		execution, err := s.resolver.ResolveActive(ctx)
		if err != nil {
			if errors.Is(err, ErrNotConfigured) {
				return ActiveExecution{}, ErrNotConfigured
			}
			return ActiveExecution{}, ErrResolutionFailed
		}
		if err := validateActiveExecution(execution); err != nil {
			return ActiveExecution{}, err
		}
		if execution.Mode != ExecutionModelBuiltIn ||
			execution.ModelBuiltIn != request.Protocol {
			return ActiveExecution{}, ErrInvalidConfig
		}
		return execution, nil
	}
	execution, err := resolver.ResolveModelBuiltIn(ctx, request)
	if err != nil {
		if errors.Is(err, ErrNotConfigured) {
			return ActiveExecution{}, ErrNotConfigured
		}
		return ActiveExecution{}, ErrResolutionFailed
	}
	if err := validateActiveExecution(execution); err != nil {
		return ActiveExecution{}, err
	}
	if execution.Mode != ExecutionModelBuiltIn ||
		execution.ModelBuiltIn != request.Protocol {
		return ActiveExecution{}, ErrInvalidConfig
	}
	return execution, nil
}

func (s *Service) Search(ctx context.Context, input Request) (Result, error) {
	normalized, err := normalizeRequest(input)
	if err != nil {
		return Result{}, err
	}
	execution, err := s.ResolveActive(ctx)
	if err != nil {
		return Result{}, err
	}
	return s.executeNormalized(ctx, execution, normalized)
}

// Execute runs a request against an already resolved execution. Chat uses this
// entry point so one request cannot observe two different active providers
// between capability selection and the outbound search call.
func (s *Service) Execute(
	ctx context.Context,
	execution ActiveExecution,
	input Request,
) (Result, error) {
	if err := validateActiveExecution(execution); err != nil {
		return Result{}, err
	}
	normalized, err := normalizeRequest(input)
	if err != nil {
		return Result{}, err
	}
	return s.executeNormalized(ctx, execution, normalized)
}

func (s *Service) executeNormalized(
	ctx context.Context,
	execution ActiveExecution,
	normalized Request,
) (Result, error) {
	if execution.Mode == ExecutionModelBuiltIn {
		return Result{}, ErrModelBuiltInRequiresChat
	}
	var lastErr error
	for attempt := 0; attempt < externalSearchMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		result, err := execution.External.Search(ctx, normalized)
		if err == nil {
			return NormalizeResult(result, normalized.MaxResults), nil
		}
		lastErr = err
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if attempt > 0 || !isTransientProviderError(err) {
			return Result{}, err
		}
		if err := waitForRetry(ctx, s.retryDelay); err != nil {
			return Result{}, err
		}
	}
	return Result{}, lastErr
}

func isTransientProviderError(err error) bool {
	var providerError *ProviderError
	if !errors.As(err, &providerError) {
		return false
	}
	if providerError.Code == "REQUEST_FAILED" {
		return true
	}
	if providerError.Code != "UPSTREAM_STATUS" {
		return false
	}
	return providerError.Status == http.StatusRequestTimeout ||
		providerError.Status == http.StatusTooManyRequests ||
		providerError.Status >= http.StatusInternalServerError &&
			providerError.Status <= 599
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validateActiveExecution(execution ActiveExecution) error {
	switch execution.Mode {
	case ExecutionExternal:
		if execution.External == nil || execution.ModelBuiltIn != "" {
			return ErrInvalidConfig
		}
	case ExecutionModelBuiltIn:
		if execution.External != nil || !isModelBuiltInProviderID(execution.ModelBuiltIn) {
			return ErrInvalidConfig
		}
	default:
		return ErrInvalidConfig
	}
	return nil
}

func isModelBuiltInProviderID(value ModelBuiltInProviderID) bool {
	switch value {
	case ModelBuiltInOpenAI, ModelBuiltInGemini, ModelBuiltInAnthropic:
		return true
	default:
		return false
	}
}
