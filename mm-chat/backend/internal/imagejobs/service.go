package imagejobs

import (
	"context"
	"errors"
)

var ErrImageJobsUnavailable = errors.New("image generation jobs are not configured")

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return GenerateResponse{}, err
	}
	_ = request
	return GenerateResponse{}, ErrImageJobsUnavailable
}
