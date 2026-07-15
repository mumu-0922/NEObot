package voicejobs

import (
	"context"
	"errors"

	"neo-chat/mm-chat/backend/internal/jobaudit"
)

var ErrVoiceJobsUnavailable = errors.New("voice jobs are not configured")

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

func (s *Service) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResponse, error) {
	if err := ctx.Err(); err != nil {
		return TranscribeResponse{}, err
	}
	if err := jobaudit.Record(ctx, s.auditRecorder, jobaudit.Event{
		Kind:       jobaudit.KindVoiceTranscribe,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: string(request.Provider),
		ModelID:    request.ModelID,
		Language:   request.Language,
		Reason:     "VOICE_JOBS_UNAVAILABLE",
	}); err != nil {
		return TranscribeResponse{}, err
	}
	return TranscribeResponse{}, ErrVoiceJobsUnavailable
}

func (s *Service) Synthesize(ctx context.Context, request SynthesizeRequest) (SynthesizeResponse, error) {
	if err := ctx.Err(); err != nil {
		return SynthesizeResponse{}, err
	}
	if err := jobaudit.Record(ctx, s.auditRecorder, jobaudit.Event{
		Kind:       jobaudit.KindVoiceSynthesize,
		Status:     jobaudit.StatusUnavailable,
		ProviderID: string(request.Provider),
		ModelID:    request.ModelID,
		Reason:     "VOICE_JOBS_UNAVAILABLE",
	}); err != nil {
		return SynthesizeResponse{}, err
	}
	return SynthesizeResponse{}, ErrVoiceJobsUnavailable
}
