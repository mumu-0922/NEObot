package agentcronworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentcron"
)

func TestRunOnceReconcilesBeforeClaimAndPrunesOnBoundedCadence(t *testing.T) {
	fake := &fakeScheduler{}
	service, err := NewService(Config{Owner: "cron-worker-g21-5", PollInterval: time.Second,
		LeaseDuration: 30 * time.Second, BatchSize: 10, Retention: 24 * time.Hour,
		MaintenanceEvery: 2}, fake)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }
	if _, err := service.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := service.RunOnce(context.Background())
	if err != nil || result.Schedule.Enqueued != 1 || fake.calls != "reconcile,cycle,reconcile,cycle,prune" {
		t.Fatalf("RunOnce() = %#v, %v; calls=%s", result, err, fake.calls)
	}
}

func TestRunOnceStopsBeforeClaimWhenReconcileFails(t *testing.T) {
	want := errors.New("reconcile failed")
	fake := &fakeScheduler{reconcileErr: want}
	service, _ := NewService(Config{Owner: "cron-worker-g21-5", PollInterval: time.Second,
		LeaseDuration: 30 * time.Second, BatchSize: 10, Retention: 24 * time.Hour,
		MaintenanceEvery: 2}, fake)
	if _, err := service.RunOnce(context.Background()); !errors.Is(err, want) || fake.calls != "reconcile" {
		t.Fatalf("RunOnce() error = %v; calls=%s", err, fake.calls)
	}
}

type fakeScheduler struct {
	calls        string
	reconcileErr error
}

func (fake *fakeScheduler) append(value string) {
	if fake.calls != "" {
		fake.calls += ","
	}
	fake.calls += value
}

func (fake *fakeScheduler) Reconcile(context.Context, time.Time, int) (agentcron.CleanupResult, error) {
	fake.append("reconcile")
	return agentcron.CleanupResult{}, fake.reconcileErr
}

func (fake *fakeScheduler) RunCycle(context.Context, agentcron.ClaimRequest) (agentcron.CycleResult, error) {
	fake.append("cycle")
	return agentcron.CycleResult{Enqueued: 1}, nil
}

func (fake *fakeScheduler) Prune(context.Context, time.Time, int) (agentcron.CleanupResult, error) {
	fake.append("prune")
	return agentcron.CleanupResult{AuditsPruned: 1}, nil
}
