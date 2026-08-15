package agentlearningworker

import (
	"context"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/agentlearning"
)

var ErrInvalidConfig = errors.New("AGENT_DRAFT_LEARNING_WORKER_CONFIG_INVALID")

type LearningService interface {
	RunChecks(context.Context, agentlearning.ClaimRequest) ([]agentlearning.Draft, error)
	Cleanup(context.Context, agentlearning.ClaimRequest) (agentlearning.CleanupResult, error)
	Reconcile(context.Context, time.Time, int) (agentlearning.ReconcileResult, error)
	Prune(context.Context, time.Time, int) (agentlearning.PruneResult, error)
}

type RunnerRuntime interface {
	Reconcile(context.Context, time.Time, int) (RecoveryResult, error)
	Prune(context.Context, time.Time, int) error
	Residue(context.Context, int) (int, error)
}

type Config struct {
	Owner            string
	PollInterval     time.Duration
	LeaseDuration    time.Duration
	BatchSize        int
	Retention        time.Duration
	MaintenanceEvery int
}

type CycleResult struct {
	Recovery RecoveryResult
	Checked  int
	Cleanup  agentlearning.CleanupResult
}

type Service struct {
	config   Config
	learning LearningService
	runner   RunnerRuntime
	now      func() time.Time
	cycles   int
}

func NewService(config Config, learning LearningService, runner RunnerRuntime) (*Service, error) {
	if learning == nil || runner == nil || len(config.Owner) < 1 || len(config.Owner) > 128 ||
		config.PollInterval < 100*time.Millisecond || config.PollInterval > time.Minute ||
		config.LeaseDuration < 30*time.Second || config.LeaseDuration > 5*time.Minute ||
		config.BatchSize < 1 || config.BatchSize > 1000 ||
		config.Retention < time.Hour || config.Retention > 365*24*time.Hour ||
		config.MaintenanceEvery < 1 || config.MaintenanceEvery > 10_000 {
		return nil, ErrInvalidConfig
	}
	return &Service{config: config, learning: learning, runner: runner, now: time.Now}, nil
}

func (service *Service) RunOnce(ctx context.Context) (CycleResult, error) {
	if service == nil || service.learning == nil || service.runner == nil {
		return CycleResult{}, ErrInvalidConfig
	}
	now := service.now().UTC()
	recovery, err := service.runner.Reconcile(ctx, now, service.config.BatchSize)
	if err != nil {
		return CycleResult{}, err
	}
	if _, err := service.learning.Reconcile(ctx, now, service.config.BatchSize); err != nil {
		return CycleResult{Recovery: recovery}, err
	}
	request := agentlearning.ClaimRequest{Owner: service.config.Owner, Now: now,
		LeaseDuration: service.config.LeaseDuration, Limit: service.config.BatchSize}
	checked, err := service.learning.RunChecks(ctx, request)
	if err != nil {
		return CycleResult{Recovery: recovery}, err
	}
	cleanup, err := service.learning.Cleanup(ctx, request)
	if err != nil {
		return CycleResult{Recovery: recovery, Checked: len(checked)}, err
	}
	service.cycles++
	if service.cycles%service.config.MaintenanceEvery == 0 {
		cutoff := now.Add(-service.config.Retention)
		if err := service.runner.Prune(ctx, cutoff, service.config.BatchSize); err != nil {
			return CycleResult{Recovery: recovery, Checked: len(checked), Cleanup: cleanup}, err
		}
		if _, err := service.learning.Prune(ctx, cutoff, service.config.BatchSize); err != nil {
			return CycleResult{Recovery: recovery, Checked: len(checked), Cleanup: cleanup}, err
		}
	}
	return CycleResult{Recovery: recovery, Checked: len(checked), Cleanup: cleanup}, nil
}

func (service *Service) Health(ctx context.Context) error {
	if service == nil || service.learning == nil || service.runner == nil {
		return ErrInvalidConfig
	}
	now := service.now().UTC()
	if _, err := service.runner.Reconcile(ctx, now, service.config.BatchSize); err != nil {
		return err
	}
	result, err := service.learning.Reconcile(ctx, now, service.config.BatchSize)
	if err != nil || result.ChecksReclaimed != 0 || result.CleanupReclaimed != 0 {
		return ErrUnavailable
	}
	residue, err := service.runner.Residue(ctx, service.config.BatchSize)
	if err != nil || residue != 0 {
		return ErrUnavailable
	}
	return nil
}

func (service *Service) Run(ctx context.Context) error {
	if service == nil {
		return ErrInvalidConfig
	}
	for {
		if _, err := service.RunOnce(ctx); err != nil {
			return err
		}
		timer := time.NewTimer(service.config.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
