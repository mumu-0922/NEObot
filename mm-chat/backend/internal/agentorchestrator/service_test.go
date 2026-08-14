package agentorchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	prepared preparedEnqueue
	lease    preparedLease
	run      Run
}

func (repository *fakeRepository) EnqueueRun(_ context.Context, prepared preparedEnqueue) (string, bool, error) {
	repository.prepared = prepared
	repository.run = Run{ID: prepared.RunID, UserID: prepared.Input.UserID,
		SnapshotID: prepared.SnapshotID, SnapshotFingerprint: prepared.SnapshotFingerprint,
		RequestFingerprint: prepared.RequestFingerprint, State: RunQueued}
	return prepared.RunID, true, nil
}
func (repository *fakeRepository) GetRun(_ context.Context, _, _ string) (Run, error) {
	return repository.run, nil
}
func (*fakeRepository) TransitionRun(context.Context, TransitionInput) error           { return nil }
func (*fakeRepository) TransitionStep(context.Context, TransitionInput) error          { return nil }
func (*fakeRepository) TransitionAttempt(context.Context, TransitionInput) error       { return nil }
func (*fakeRepository) ObserveTerminalConflict(context.Context, TransitionInput) error { return nil }
func (repository *fakeRepository) AcquireStep(_ context.Context, input AcquireInput, lease preparedLease) (Lease, error) {
	repository.lease = lease
	return Lease{Attempt: Attempt{ID: lease.AttemptID, RunID: input.RunID,
		StepID: input.StepID, Generation: 1, State: AttemptLeased,
		LeaseOwner: input.LeaseOwner}, Token: lease.Token}, nil
}
func (*fakeRepository) HeartbeatAttempt(context.Context, HeartbeatInput, string) (time.Time, error) {
	return time.Now(), nil
}
func (*fakeRepository) ListRecoveryRuns(context.Context, int) ([]RecoveryRun, error) { return nil, nil }
func (*fakeRepository) RebuildProjection(context.Context, string, string) error      { return nil }
func (*fakeRepository) AppendKillSwitch(_ context.Context, input KillSwitchInput) (KillSwitch, error) {
	return KillSwitch{ID: input.ID}, nil
}
func (*fakeRepository) ResolveKillSwitch(context.Context, string, string) (KillResolution, error) {
	return KillResolution{}, nil
}
func (*fakeRepository) PruneTerminalRuns(context.Context, time.Time, int) (int, error) { return 0, nil }

func TestEnqueueCanonicalizesSnapshotAndDoesNotEmbedAuthoritySecrets(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	counter := 0
	service.newID = func(prefix string) string {
		counter++
		return prefix + "_" + strings.Repeat("a", 15) + string(rune('a'+counter))
	}
	userID := "10000000-0000-4000-8000-000000000001"
	result, err := service.EnqueueRun(context.Background(), EnqueueInput{
		UserID: userID, IdempotencyKey: "fixture-enqueue",
		Snapshot:      json.RawMessage(`{"model":{"id":"fixture","provider":"fixture"},"budget":{"wallSeconds":30}}`),
		Steps:         []StepPlan{{Kind: "reason"}, {Kind: "respond"}},
		ScopeBindings: []ScopeBinding{{Type: "skill", Value: "sha256:" + strings.Repeat("a", 64)}},
	})
	if err != nil || !result.Created || result.Run.State != RunQueued {
		t.Fatalf("EnqueueRun() = %#v, %v", result, err)
	}
	if len(repository.prepared.EventIDs) != 7 || len(repository.prepared.ScopeKeys) != 5 {
		t.Fatalf("prepared enqueue = %#v", repository.prepared)
	}
	if strings.Contains(string(repository.prepared.CanonicalSnapshot), "prompt") ||
		!strings.HasPrefix(repository.prepared.SnapshotFingerprint, "sha256:") {
		t.Fatalf("canonical snapshot = %s", repository.prepared.CanonicalSnapshot)
	}

	for _, snapshot := range []string{
		`{"prompt":"persist me"}`,
		`{"nested":{"leaseToken":"secret"}}`,
		`{"stdout":"private output"}`,
	} {
		_, err := service.EnqueueRun(context.Background(), EnqueueInput{
			UserID: userID, IdempotencyKey: "denied", Snapshot: json.RawMessage(snapshot),
			Steps: []StepPlan{{Kind: "reason"}},
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("forbidden snapshot %s error = %v", snapshot, err)
		}
	}
}

func TestEnqueueRunWithIDBindsDeterministicRunIDIntoRequestFingerprint(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	input := EnqueueInput{UserID: "10000000-0000-4000-8000-000000000001",
		IdempotencyKey: "controller/action", Snapshot: json.RawMessage(`{"schemaVersion":"test/v1"}`),
		Steps: []StepPlan{{ID: "step_0123456789abcdef", Kind: "execute"}}}
	first, err := service.prepareEnqueue(input, "run_0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.prepareEnqueue(input, "run_fedcba9876543210")
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestFingerprint == second.RequestFingerprint {
		t.Fatal("deterministic Run ID drift did not change the enqueue request fingerprint")
	}
}

func TestAcquireReturnsOpaqueTokenButPersistsOnlyDigest(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	lease, err := service.AcquireStep(context.Background(), AcquireInput{
		UserID: "10000000-0000-4000-8000-000000000001",
		RunID:  "run_0123456789abcdef", StepID: "step_0123456789abcdef",
		LeaseOwner: "fixture-orchestrator", LeaseDuration: 30 * time.Second,
		Actor: Actor{Type: "orchestrator", ID: "fixture"}, ReasonCode: "LEASE_REQUESTED",
	})
	if err != nil || !strings.HasPrefix(lease.Token, "lease_") || len(repository.lease.TokenHash) != 64 {
		t.Fatalf("AcquireStep() = %#v, %v, prepared=%#v", lease, err, repository.lease)
	}
	if lease.Token == repository.lease.TokenHash || strings.Contains(strings.Join(repository.lease.EventIDs, ","), lease.Token) {
		t.Fatal("opaque lease token crossed the durable event/hash boundary")
	}
}

func TestDetailAllowlistRejectsRawPayloadShapes(t *testing.T) {
	for _, detail := range []map[string]any{
		{"observationCode": "STALE_RESULT"},
		{"durationMs": int64(12), "outcome": "ignored"},
	} {
		if err := validateDetail(detail); err != nil {
			t.Fatalf("safe detail rejected: %v", err)
		}
	}
	for _, detail := range []map[string]any{
		{"rawPayload": "private"},
		{"observationCode": strings.Repeat("x", 257)},
		{"observationCode": "contains spaces and private content"},
		{"outcome": map[string]any{"secret": "value"}},
	} {
		if err := validateDetail(detail); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("unsafe detail error = %v", err)
		}
	}
}
