package imagejobs

import (
	"context"
	"errors"

	"neo-chat/mm-chat/backend/internal/jobaudit"
)

var ErrImageJobsUnavailable = errors.New("image generation jobs are not configured")

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

func (s *Service) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return GenerateResponse{}, err
	}
	if err := jobaudit.Record(ctx, s.auditRecorder, jobaudit.Event{
		Kind:       jobaudit.KindImageGenerate,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: request.ModelRef.ProviderID,
		ModelID:    request.ModelRef.ModelID,
		Reason:     "IMAGE_JOBS_UNAVAILABLE",
	}); err != nil {
		return GenerateResponse{}, err
	}
	return GenerateResponse{}, ErrImageJobsUnavailable
}
