package voicejobs

import (
	"context"
	"errors"

	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
)

var (
	ErrVoiceJobsUnavailable          = errors.New("voice jobs are not configured")
	ErrVoiceArtifactStoreUnavailable = errors.New("voice artifact store is not configured")
)

type ServiceOption func(*Service)

type Service struct {
	auditRecorder jobaudit.Recorder
	executor      Executor
	artifactStore ArtifactStore
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

func WithArtifactStore(store ArtifactStore) ServiceOption {
	return func(service *Service) {
		service.artifactStore = store
	}
}

func (s *Service) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResponse, error) {
	if err := ctx.Err(); err != nil {
		return TranscribeResponse{}, err
	}
	if s == nil || s.executor == nil {
		if err := s.recordUnavailable(ctx, jobaudit.KindVoiceTranscribe, request, "VOICE_JOBS_UNAVAILABLE"); err != nil {
			return TranscribeResponse{}, err
		}
		return TranscribeResponse{}, ErrVoiceJobsUnavailable
	}
	if err := s.recordAdmitted(ctx, jobaudit.KindVoiceTranscribe, request); err != nil {
		return TranscribeResponse{}, err
	}
	return s.executor.Transcribe(ctx, request)
}

func (s *Service) Synthesize(ctx context.Context, request SynthesizeRequest) (SynthesizeResponse, error) {
	if err := ctx.Err(); err != nil {
		return SynthesizeResponse{}, err
	}
	if s == nil || s.executor == nil {
		if err := s.recordUnavailable(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
			Provider: request.Provider,
			ModelID:  request.ModelID,
		}, "VOICE_JOBS_UNAVAILABLE"); err != nil {
			return SynthesizeResponse{}, err
		}
		return SynthesizeResponse{}, ErrVoiceJobsUnavailable
	}
	if s.artifactStore == nil {
		if err := s.recordUnavailable(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
			Provider: request.Provider,
			ModelID:  request.ModelID,
		}, "VOICE_ARTIFACT_STORE_UNAVAILABLE"); err != nil {
			return SynthesizeResponse{}, err
		}
		return SynthesizeResponse{}, ErrVoiceArtifactStoreUnavailable
	}

	if err := s.recordAdmitted(ctx, jobaudit.KindVoiceSynthesize, TranscribeRequest{
		Provider: request.Provider,
		ModelID:  request.ModelID,
	}); err != nil {
		return SynthesizeResponse{}, err
	}
	result, err := s.executor.Synthesize(ctx, request)
	if err != nil {
		return SynthesizeResponse{}, err
	}
	artifact, err := s.artifactStore.Store(ctx, jobartifacts.StoreInput{
		JobID:       result.JobID,
		Kind:        jobartifacts.KindAudio,
		Filename:    result.Filename,
		ContentType: result.ContentType,
		Size:        result.Size,
		Body:        result.Body,
	})
	if err != nil {
		return SynthesizeResponse{}, err
	}
	return SynthesizeResponse{
		FileID:      artifact.FileID,
		Purpose:     artifact.Purpose,
		ContentType: artifact.ContentType,
		Size:        artifact.Size,
	}, nil
}

func (s *Service) recordAdmitted(ctx context.Context, kind jobaudit.JobKind, request TranscribeRequest) error {
	var recorder jobaudit.Recorder
	if s != nil {
		recorder = s.auditRecorder
	}
	if recorder == nil {
		return jobaudit.ErrAuditUnavailable
	}
	return jobaudit.Record(ctx, recorder, jobaudit.Event{
		Kind:       kind,
		Status:     jobaudit.StatusAdmitted,
		ProviderID: string(request.Provider),
		ModelID:    request.ModelID,
		Language:   request.Language,
	})
}

func (s *Service) recordUnavailable(
	ctx context.Context,
	kind jobaudit.JobKind,
	request TranscribeRequest,
	reason string,
) error {
	var recorder jobaudit.Recorder
	if s != nil {
		recorder = s.auditRecorder
	}
	return jobaudit.Record(ctx, recorder, jobaudit.Event{
		Kind:       kind,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: string(request.Provider),
		ModelID:    request.ModelID,
		Language:   request.Language,
		Reason:     reason,
	})
}
