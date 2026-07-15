package jobcontrol

import (
	"context"
	"errors"
)

var ErrJobCancellationUnavailable = errors.New("job cancellation is not configured")

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Cancel(ctx context.Context, jobID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_ = jobID
	return ErrJobCancellationUnavailable
}
