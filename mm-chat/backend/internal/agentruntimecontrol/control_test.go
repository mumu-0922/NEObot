package agentruntimecontrol

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestReconcileUsesOnlyActivationBoundRecoveryInventory(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	probeFingerprint := fingerprint('1')
	expected := descriptor("a", probeFingerprint, now)
	unknown := descriptor("b", probeFingerprint, now)
	client := &fakeClient{now: now, probeFingerprint: probeFingerprint,
		inventory: map[string]agentrunner.SandboxDescriptor{
			expected.Attempt.AttemptID: expected,
			unknown.Attempt.AttemptID:  unknown,
		}}
	repository := &fakeRepository{items: []agentrunner.RecoverySandbox{{
		ExpectedSandbox: agentrunner.ExpectedSandbox{
			SandboxID: expected.SandboxID, Attempt: expected.Attempt,
			RunnerID: "neo-runner-primary", SnapshotFingerprint: expected.SnapshotFingerprint,
			SpecFingerprint: expected.SpecFingerprint, ProbeFingerprint: expected.ProbeFingerprint,
		}, SandboxState: "running", AttemptState: "running", LeaseExpiresAt: now.Add(time.Minute),
	}}}
	service, err := NewService(Config{RunnerID: "neo-runner-primary", PollInterval: 30 * time.Second, BatchSize: 100},
		fakeGate{}, repository, client)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(client.inventory) != 1 || client.inventory[expected.Attempt.AttemptID].SandboxID != expected.SandboxID {
		t.Fatalf("inventory after reconcile = %#v", client.inventory)
	}
	if client.cleaned != 1 {
		t.Fatalf("cleaned = %d, want 1", client.cleaned)
	}
	wantFirstCycle := []string{
		agentrunner.MethodProbe,
		agentrunner.MethodList,
		agentrunner.MethodReconcile,
		agentrunner.MethodList,
	}
	if len(client.methods) != len(wantFirstCycle) {
		t.Fatalf("first-cycle methods = %v", client.methods)
	}
	for index, method := range wantFirstCycle {
		if client.methods[index] != method {
			t.Fatalf("first-cycle methods = %v", client.methods)
		}
	}
	beforeHealth := len(client.methods)
	if err := service.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if got := client.methods[beforeHealth:]; len(got) != 2 || got[0] != agentrunner.MethodProbe || got[1] != agentrunner.MethodList {
		t.Fatalf("healthcheck methods = %v, want probe/list", got)
	}
	for _, method := range client.methods {
		if method != agentrunner.MethodProbe && method != agentrunner.MethodList && method != agentrunner.MethodReconcile {
			t.Fatalf("control worker called forbidden method %q", method)
		}
	}
}

func TestHealthRejectsSandboxStateDrift(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	probeFingerprint := fingerprint('1')
	actual := descriptor("a", probeFingerprint, now)
	actual.State = "stopped"
	client := &fakeClient{now: now, probeFingerprint: probeFingerprint,
		inventory: map[string]agentrunner.SandboxDescriptor{actual.Attempt.AttemptID: actual}}
	repository := &fakeRepository{items: []agentrunner.RecoverySandbox{{
		ExpectedSandbox: agentrunner.ExpectedSandbox{
			SandboxID: actual.SandboxID, Attempt: actual.Attempt,
			RunnerID: "neo-runner-primary", SnapshotFingerprint: actual.SnapshotFingerprint,
			SpecFingerprint: actual.SpecFingerprint, ProbeFingerprint: actual.ProbeFingerprint,
		}, SandboxState: "running", AttemptState: "running", LeaseExpiresAt: now.Add(time.Minute),
	}}}
	service, _ := NewService(Config{RunnerID: "neo-runner-primary", PollInterval: 30 * time.Second, BatchSize: 100},
		fakeGate{}, repository, client)
	service.now = func() time.Time { return now }
	if err := service.Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Health() state drift error = %v", err)
	}
}

func TestReconcileDropsExpiredOrProbeDriftedAuthority(t *testing.T) {
	now := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	currentProbe := fingerprint('1')
	stale := descriptor("a", fingerprint('2'), now)
	client := &fakeClient{now: now, probeFingerprint: currentProbe,
		inventory: map[string]agentrunner.SandboxDescriptor{stale.Attempt.AttemptID: stale}}
	repository := &fakeRepository{items: []agentrunner.RecoverySandbox{{
		ExpectedSandbox: agentrunner.ExpectedSandbox{SandboxID: stale.SandboxID, Attempt: stale.Attempt,
			RunnerID: "neo-runner-primary", SnapshotFingerprint: stale.SnapshotFingerprint,
			SpecFingerprint: stale.SpecFingerprint, ProbeFingerprint: stale.ProbeFingerprint},
		SandboxState: "running", AttemptState: "running", LeaseExpiresAt: now.Add(time.Minute),
	}}}
	service, _ := NewService(Config{RunnerID: "neo-runner-primary", PollInterval: 30 * time.Second, BatchSize: 100},
		fakeGate{}, repository, client)
	service.now = func() time.Time { return now }
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(client.inventory) != 0 || client.cleaned != 1 {
		t.Fatalf("drifted inventory=%#v cleaned=%d", client.inventory, client.cleaned)
	}
}

func TestActivationFailurePerformsNoRunnerOrDatabaseCall(t *testing.T) {
	client := &fakeClient{}
	repository := &fakeRepository{}
	service, _ := NewService(Config{RunnerID: "neo-runner-primary", PollInterval: 30 * time.Second, BatchSize: 100},
		fakeGate{err: errors.New("held")}, repository, client)
	if err := service.Reconcile(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(client.methods) != 0 || repository.calls != 0 {
		t.Fatalf("calls after held activation = %v/%d", client.methods, repository.calls)
	}
}

type fakeGate struct{ err error }

func (gate fakeGate) Verify(time.Time) error { return gate.err }

type fakeRepository struct {
	items []agentrunner.RecoverySandbox
	calls int
}

func (repository *fakeRepository) RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error) {
	repository.calls++
	return append([]agentrunner.RecoverySandbox(nil), repository.items...), nil
}

type fakeClient struct {
	now              time.Time
	probeFingerprint string
	inventory        map[string]agentrunner.SandboxDescriptor
	methods          []string
	cleaned          int
}

func (client *fakeClient) Call(_ context.Context, request agentrunner.Request) (agentrunner.Response, error) {
	client.methods = append(client.methods, request.Method)
	response := agentrunner.Response{SchemaVersion: agentrunner.ProtocolVersion,
		Method: request.Method + ".result", RequestID: request.RequestID, Nonce: request.Nonce, SentAt: client.now}
	switch request.Method {
	case agentrunner.MethodProbe:
		response.Body = agentrunner.ProbeResult{Ready: true, Runtime: "podman", RuntimeVersion: "6.1.0",
			Features: append([]string(nil), RequiredFeatures...), ProbeFingerprint: client.probeFingerprint}
	case agentrunner.MethodList:
		items := make([]agentrunner.SandboxDescriptor, 0, len(client.inventory))
		for _, item := range client.inventory {
			items = append(items, item)
		}
		response.Body = agentrunner.ListResult{RunnerID: "neo-runner-primary", Sandboxes: items}
	case agentrunner.MethodReconcile:
		next := make(map[string]agentrunner.SandboxDescriptor, len(request.Reconcile.Expected))
		for _, item := range request.Reconcile.Expected {
			next[item.Attempt.AttemptID] = item
		}
		client.cleaned += len(client.inventory) - len(next)
		client.inventory = next
		response.Body = agentrunner.ReconcileResult{RunnerID: "neo-runner-primary", Cleaned: client.cleaned}
	default:
		return agentrunner.Response{}, errors.New("forbidden method")
	}
	return response, nil
}

func descriptor(suffix string, probe string, now time.Time) agentrunner.SandboxDescriptor {
	return agentrunner.SandboxDescriptor{
		SandboxID: "sandbox_0123456789abcde" + suffix,
		Attempt: agentrunner.AttemptIdentity{
			RunID: "run_0123456789abcde" + suffix, StepID: "step_0123456789abcde" + suffix,
			AttemptID: "attempt_0123456789abcde" + suffix, LeaseGeneration: 1,
		},
		SnapshotFingerprint: fingerprint('3'), SpecFingerprint: fingerprint('4'),
		ProbeFingerprint: probe, State: "running", UpdatedAt: now,
	}
}
func fingerprint(value byte) string {
	body := make([]byte, 64)
	for index := range body {
		body[index] = value
	}
	return "sha256:" + string(body)
}
