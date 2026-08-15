// Package agentcronworker runs the G21.5 exact-target Cron loop. Database
// activation wrappers, not Go post-filtering, define the only claimable cohort.
package agentcronworker

import (
	"context"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/agentcron"
)

var ErrInvalidConfig = errors.New("AGENT_CRON_WORKER_CONFIG_INVALID")

type Scheduler interface {
	RunCycle(context.Context, agentcron.ClaimRequest) (agentcron.CycleResult, error)
	Reconcile(context.Context, time.Time, int) (agentcron.CleanupResult, error)
	Prune(context.Context, time.Time, int) (agentcron.CleanupResult, error)
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
	Schedule agentcron.CycleResult
	Cleanup  agentcron.CleanupResult
}

type Service struct {
	config    Config
	scheduler Scheduler
	now       func() time.Time
	cycles    int
}

func NewService(config Config, scheduler Scheduler) (*Service, error) {
	if scheduler == nil || len(config.Owner) < 1 || len(config.Owner) > 128 ||
		config.PollInterval < 100*time.Millisecond || config.PollInterval > time.Minute ||
		config.LeaseDuration < 5*time.Second || config.LeaseDuration > 5*time.Minute ||
		config.BatchSize < 1 || config.BatchSize > 1000 ||
		config.Retention < time.Hour || config.Retention > 365*24*time.Hour ||
		config.MaintenanceEvery < 1 || config.MaintenanceEvery > 10_000 {
		return nil, ErrInvalidConfig
	}
	return &Service{config: config, scheduler: scheduler, now: time.Now}, nil
}

func (service *Service) RunOnce(ctx context.Context) (CycleResult, error) {
	if service == nil || service.scheduler == nil {
		return CycleResult{}, ErrInvalidConfig
	}
	now := service.now().UTC()
	cleanup, err := service.scheduler.Reconcile(ctx, now, service.config.BatchSize)
	if err != nil {
		return CycleResult{}, err
	}
	result, err := service.scheduler.RunCycle(ctx, agentcron.ClaimRequest{
		Owner: service.config.Owner, Now: now,
		LeaseDuration: service.config.LeaseDuration, Limit: service.config.BatchSize,
	})
	if err != nil {
		return CycleResult{Cleanup: cleanup}, err
	}
	service.cycles++
	if service.cycles%service.config.MaintenanceEvery == 0 {
		pruned, pruneErr := service.scheduler.Prune(ctx, now.Add(-service.config.Retention), service.config.BatchSize)
		cleanup.TriggersPruned += pruned.TriggersPruned
		cleanup.AuditsPruned += pruned.AuditsPruned
		cleanup.TemplatesPruned += pruned.TemplatesPruned
		if pruneErr != nil {
			return CycleResult{Schedule: result, Cleanup: cleanup}, pruneErr
		}
	}
	return CycleResult{Schedule: result, Cleanup: cleanup}, nil
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
