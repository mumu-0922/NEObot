package codejobs

import (
	"context"
	"errors"

	"neo-chat/mm-chat/backend/internal/jobaudit"
)

var ErrCodeExecutionUnavailable = errors.New("code execution jobs are not configured")

type ServiceOption func(*Service)

type Service struct {
	auditRecorder jobaudit.Recorder
}

func NewService(options ...ServiceOption) *Service {
	service := &Service{}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func WithAuditRecorder(recorder jobaudit.Recorder) ServiceOption {
	return func(service *Service) {
		service.auditRecorder = recorder
	}
}

func (s *Service) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResponse, error) {
	if err := ctx.Err(); err != nil {
		return ExecuteResponse{}, err
	}
	if err := jobaudit.Record(ctx, s.auditRecorder, jobaudit.Event{
		Kind:       jobaudit.KindCodeExecute,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: request.ModelRef.ProviderID,
		ModelID:    request.ModelRef.ModelID,
		Language:   request.Language,
		Reason:     "CODE_EXECUTION_UNAVAILABLE",
	}); err != nil {
		return ExecuteResponse{}, err
	}
	return ExecuteResponse{}, ErrCodeExecutionUnavailable
}
