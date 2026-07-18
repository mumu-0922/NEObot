package websearch

import (
	"context"
	"errors"
)

type Service struct {
	resolver Resolver
}

func NewService(resolver Resolver) *Service {
	return &Service{resolver: resolver}
}

func (s *Service) Configured() bool {
	return s != nil && s.resolver != nil
}

func (s *Service) ResolveActive(ctx context.Context) (ActiveExecution, error) {
	if !s.Configured() {
		return ActiveExecution{}, ErrNotConfigured
	}
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
	result, err := execution.External.Search(ctx, normalized)
	if err != nil {
		return Result{}, err
	}
	return NormalizeResult(result, normalized.MaxResults), nil
}

func validateActiveExecution(execution ActiveExecution) error {
	switch execution.Mode {
	case ExecutionExternal:
		if execution.External == nil || execution.ModelBuiltIn != "" {
			return ErrInvalidConfig
		}
	case ExecutionModelBuiltIn:
		if execution.External != nil || execution.ModelBuiltIn != ModelBuiltInOpenAI {
			return ErrInvalidConfig
		}
	default:
		return ErrInvalidConfig
	}
	return nil
}
