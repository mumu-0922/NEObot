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
