package codejobs

import (
	"context"
	"errors"
)

var ErrCodeExecutionUnavailable = errors.New("code execution jobs are not configured")

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResponse, error) {
	if err := ctx.Err(); err != nil {
		return ExecuteResponse{}, err
	}
	_ = request
	return ExecuteResponse{}, ErrCodeExecutionUnavailable
}
