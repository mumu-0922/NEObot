package imagejobs

import (
	"context"
	"errors"

	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

var (
	ErrImageJobsUnavailable          = errors.New("image generation jobs are not configured")
	ErrImageArtifactStoreUnavailable = errors.New("image artifact store is not configured")
)

type ServiceOption func(*Service)

type Service struct {
	auditRecorder    jobaudit.Recorder
	executor         Executor
	executorResolver ExecutorResolver
	artifactStore    ArtifactStore
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

func WithExecutor(executor Executor) ServiceOption {
	return func(service *Service) {
		service.executor = executor
	}
}

func WithExecutorResolver(resolver ExecutorResolver) ServiceOption {
	return func(service *Service) {
		service.executorResolver = resolver
	}
}

func WithArtifactStore(store ArtifactStore) ServiceOption {
	return func(service *Service) {
		service.artifactStore = store
	}
}

func (s *Service) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return GenerateResponse{}, err
	}
	var executor Executor
	if s != nil {
		executor = s.executor
		if s.executorResolver != nil {
			resolved, err := s.executorResolver.ResolveImageExecutor(ctx, request.ModelRef)
			if err != nil {
				if auditErr := s.recordUnavailable(
					ctx,
					request,
					"IMAGE_EXECUTOR_RESOLUTION_FAILED",
				); auditErr != nil {
					return GenerateResponse{}, auditErr
				}
				return GenerateResponse{}, err
			}
			executor = resolved
		}
	}
	if executor == nil {
		if err := s.recordUnavailable(ctx, request, "IMAGE_JOBS_UNAVAILABLE"); err != nil {
			return GenerateResponse{}, err
		}
		return GenerateResponse{}, ErrImageJobsUnavailable
	}
	if s.artifactStore == nil {
		if err := s.recordUnavailable(ctx, request, "IMAGE_ARTIFACT_STORE_UNAVAILABLE"); err != nil {
			return GenerateResponse{}, err
		}
		return GenerateResponse{}, ErrImageArtifactStoreUnavailable
	}
	if err := s.recordAdmitted(ctx, request); err != nil {
		return GenerateResponse{}, err
	}

	result, err := executor.Generate(ctx, request)
	if err != nil {
		return GenerateResponse{}, err
	}
	images := make([]GeneratedImage, 0, len(result.Images))
	for _, generated := range result.Images {
		artifact, err := s.artifactStore.Store(ctx, jobartifacts.StoreInput{
			JobID:       generated.JobID,
			Kind:        jobartifacts.KindImage,
			Filename:    generated.Filename,
			ContentType: generated.ContentType,
			Size:        generated.Size,
			Body:        generated.Body,
		})
		if err != nil {
			return GenerateResponse{}, err
		}
		images = append(images, GeneratedImage{
			FileID:      artifact.FileID,
			Purpose:     artifact.Purpose,
			ContentType: artifact.ContentType,
			Size:        artifact.Size,
		})
	}
	return GenerateResponse{Images: images, Message: result.Message}, nil
}

func (s *Service) recordAdmitted(ctx context.Context, request GenerateRequest) error {
	var recorder jobaudit.Recorder
	if s != nil {
		recorder = s.auditRecorder
	}
	if recorder == nil {
		return jobaudit.ErrAuditUnavailable
	}
	return jobaudit.Record(ctx, recorder, jobaudit.Event{
		Kind:       jobaudit.KindImageGenerate,
		Status:     jobaudit.StatusAdmitted,
		ProviderID: request.ModelRef.ProviderID,
		ModelID:    request.ModelRef.ModelID,
	})
}

func (s *Service) recordUnavailable(ctx context.Context, request GenerateRequest, reason string) error {
	var recorder jobaudit.Recorder
	if s != nil {
		recorder = s.auditRecorder
	}
	return jobaudit.Record(ctx, recorder, jobaudit.Event{
		Kind:       jobaudit.KindImageGenerate,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: request.ModelRef.ProviderID,
		ModelID:    request.ModelRef.ModelID,
		Reason:     reason,
	})
}
