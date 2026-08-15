package agentlearningworker

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentlearning"
)

func TestServiceReconcilesRunnerBeforeClaimsAndCleanup(t *testing.T) {
	order := []string{}
	runner := &serviceRunnerFake{order: &order}
	learning := &serviceLearningFake{order: &order}
	service, err := NewService(Config{Owner: "draft-worker", PollInterval: time.Second,
		LeaseDuration: time.Minute, BatchSize: 10, Retention: 24 * time.Hour,
		MaintenanceEvery: 1}, learning, runner)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 8, 15, 7, 0, 0, 0, time.UTC) }
	if _, err := service.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"runner-reconcile", "learning-reconcile", "checks", "cleanup", "runner-prune", "learning-prune"}
	if len(order) != len(want) {
		t.Fatalf("order = %v", order)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("order = %v", order)
		}
	}
}

type serviceRunnerFake struct{ order *[]string }

func (fake *serviceRunnerFake) Reconcile(context.Context, time.Time, int) (RecoveryResult, error) {
	*fake.order = append(*fake.order, "runner-reconcile")
	return RecoveryResult{}, nil
}
func (fake *serviceRunnerFake) Prune(context.Context, time.Time, int) error {
	*fake.order = append(*fake.order, "runner-prune")
	return nil
}
func (*serviceRunnerFake) Residue(context.Context, int) (int, error) { return 0, nil }

type serviceLearningFake struct{ order *[]string }

func (fake *serviceLearningFake) RunChecks(context.Context, agentlearning.ClaimRequest) ([]agentlearning.Draft, error) {
	*fake.order = append(*fake.order, "checks")
	return nil, nil
}
func (fake *serviceLearningFake) Cleanup(context.Context, agentlearning.ClaimRequest) (agentlearning.CleanupResult, error) {
	*fake.order = append(*fake.order, "cleanup")
	return agentlearning.CleanupResult{}, nil
}
func (fake *serviceLearningFake) Reconcile(context.Context, time.Time, int) (agentlearning.ReconcileResult, error) {
	*fake.order = append(*fake.order, "learning-reconcile")
	return agentlearning.ReconcileResult{}, nil
}
func (fake *serviceLearningFake) Prune(context.Context, time.Time, int) (agentlearning.PruneResult, error) {
	*fake.order = append(*fake.order, "learning-prune")
	return agentlearning.PruneResult{}, nil
}
