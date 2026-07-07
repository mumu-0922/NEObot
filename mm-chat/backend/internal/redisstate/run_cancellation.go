package redisstate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RunCancellationStore struct {
	rdb       *redis.Client
	keyPrefix string
	ttl       time.Duration
}

func (s *RunCancellationStore) MarkRunCancelled(ctx context.Context, runID string) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	key, err := s.runCancelKey(runID)
	if err != nil {
		return err
	}

	return s.rdb.Set(ctx, key, "1", s.ttl).Err()
}

func (s *RunCancellationStore) IsRunCancelled(ctx context.Context, runID string) (bool, error) {
	if err := s.requireReady(); err != nil {
		return false, err
	}
	key, err := s.runCancelKey(runID)
	if err != nil {
		return false, err
	}

	value, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return value != "", nil
}

func (s *RunCancellationStore) ClearRunCancelled(ctx context.Context, runID string) error {
	if err := s.requireReady(); err != nil {
		return err
	}
	key, err := s.runCancelKey(runID)
	if err != nil {
		return err
	}

	return s.rdb.Del(ctx, key).Err()
}

func (s *RunCancellationStore) requireReady() error {
	if s == nil || s.rdb == nil {
		return errors.New("redis cancellation store is not initialized")
	}
	if s.ttl <= 0 {
		return errors.New("redis cancellation ttl is invalid")
	}

	return nil
}

func (s *RunCancellationStore) runCancelKey(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", errors.New("run id is required")
	}
	if strings.ContainsAny(runID, " \t\r\n") {
		return "", errors.New("run id must not contain whitespace")
	}

	prefix := normalizeKeyPrefix(s.keyPrefix)
	return fmt.Sprintf("%s:runs:%s:cancelled", prefix, runID), nil
}
