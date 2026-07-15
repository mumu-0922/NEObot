package voicejobs

import (
	"context"
	"errors"
)

var ErrVoiceJobsUnavailable = errors.New("voice jobs are not configured")

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Transcribe(ctx context.Context, request TranscribeRequest) (TranscribeResponse, error) {
	if err := ctx.Err(); err != nil {
		return TranscribeResponse{}, err
	}
	_ = request
	return TranscribeResponse{}, ErrVoiceJobsUnavailable
}

func (s *Service) Synthesize(ctx context.Context, request SynthesizeRequest) (SynthesizeResponse, error) {
	if err := ctx.Err(); err != nil {
		return SynthesizeResponse{}, err
	}
	_ = request
	return SynthesizeResponse{}, ErrVoiceJobsUnavailable
}
