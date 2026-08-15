package agentchildcanary

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentdelegation"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

const (
	recoveryParentRun     = "run_0000000000000004"
	recoveryChildRun      = "run_0000000000000005"
	recoveryParentStep    = "step_0000000000000004"
	recoveryChildStep     = "step_0000000000000005"
	recoveryParentAttempt = "attempt_0000000000000004"
	recoveryChildAttempt  = "attempt_0000000000000005"
)

func TestServiceRestartNeverCreatesSecondChildAcrossCrashPoints(t *testing.T) {
	tests := []struct {
		name          string
		childAttempts []agentorchestrator.Attempt
		wantLaunches  int
		wantCascades  int
	}{
		{name: "after Child enqueue before acquire", wantLaunches: 1, wantCascades: 1},
		{name: "after launch authority or Runner launch with live lost token",
			childAttempts: []agentorchestrator.Attempt{{ID: recoveryChildAttempt,
				RunID: recoveryChildRun, StepID: recoveryChildStep, Generation: 1,
				State: agentorchestrator.AttemptStarting, LeaseOwner: "neo-runner-primary",
				LeaseExpiresAt: testNow.Add(time.Minute)}}},
		{name: "after Child launch heartbeat or before cascade with expired token",
			childAttempts: []agentorchestrator.Attempt{{ID: recoveryChildAttempt,
				RunID: recoveryChildRun, StepID: recoveryChildStep, Generation: 1,
				State: agentorchestrator.AttemptRunning, LeaseOwner: "neo-runner-primary",
				LeaseExpiresAt: testNow.Add(-time.Second)}}, wantCascades: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, orchestrator, delegation, client := recoveryServiceFixture(t)
			child := orchestrator.runs[recoveryChildRun]
			child.Attempts = append([]agentorchestrator.Attempt(nil), test.childAttempts...)
			if len(test.childAttempts) > 0 {
				child.State = agentorchestrator.RunRunning
				child.Steps[0].State = agentorchestrator.StepRunning
			}
			orchestrator.runs[recoveryChildRun] = child
			parent := orchestrator.runs[recoveryParentRun]
			result, err := service.recoverExisting(context.Background(), parent,
				delegation.children[0], agentrunner.ProbeResult{Ready: true, ProbeFingerprint: fingerprint('9')})
			if !errors.Is(err, ErrRecoveryPending) || result.ParentRunID != recoveryParentRun ||
				result.ChildRunID != recoveryChildRun {
				t.Fatalf("recover=%#v err=%v", result, err)
			}
			if client.calls[agentrunner.MethodLaunch] != test.wantLaunches ||
				delegation.cascades != test.wantCascades || delegation.enqueues != 0 ||
				len(delegation.children) != 1 {
				t.Fatalf("launches=%d cascades=%d enqueues=%d children=%#v",
					client.calls[agentrunner.MethodLaunch], delegation.cascades,
					delegation.enqueues, delegation.children)
			}
			if test.wantCascades == 1 && delegation.settlements != 1 {
				t.Fatalf("settlements=%d", delegation.settlements)
			}
		})
	}
}

func TestServiceRestartFinishesParentOnlyAfterChildReapCompletion(t *testing.T) {
	service, orchestrator, delegation, client := recoveryServiceFixture(t)
	child := orchestrator.runs[recoveryChildRun]
	child.State = agentorchestrator.RunCanceled
	child.Steps[0].State = agentorchestrator.StepCanceled
	orchestrator.runs[recoveryChildRun] = child
	parent := orchestrator.runs[recoveryParentRun]
	parent.Attempts[0].LeaseExpiresAt = testNow.Add(-time.Second)
	orchestrator.runs[recoveryParentRun] = parent

	result, err := service.recoverExisting(context.Background(), parent, delegation.children[0],
		agentrunner.ProbeResult{Ready: true, ProbeFingerprint: fingerprint('9')})
	if err != nil || !result.Completed || result.ParentRunID != recoveryParentRun ||
		result.ChildRunID != recoveryChildRun {
		t.Fatalf("recover=%#v err=%v", result, err)
	}
	if delegation.enqueues != 0 || delegation.cascades != 0 ||
		orchestrator.acquired[recoveryParentRun] != 1 ||
		client.calls[agentrunner.MethodLaunch] != 1 || client.calls[agentrunner.MethodCancel] != 1 ||
		orchestrator.runs[recoveryParentRun].State != agentorchestrator.RunCanceled {
		t.Fatalf("acquired=%#v calls=%#v Parent=%#v enqueues=%d cascades=%d",
			orchestrator.acquired, client.calls, orchestrator.runs[recoveryParentRun],
			delegation.enqueues, delegation.cascades)
	}
}

func TestServiceHealthRejectsPendingOrFailedReapBeforeTerminalLineage(t *testing.T) {
	service, _, delegation, _ := recoveryServiceFixture(t)
	service.completedRun = recoveryParentRun
	delegation.pending = []agentdelegation.ReapTarget{{ReapID: "reap_0000000000000004", State: "failed"}}
	if err := service.Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Health() error=%v", err)
	}
}

type recoveryGate struct{}

func (recoveryGate) Verify(time.Time) error { return nil }

type recoveryOrchestrator struct {
	runs     map[string]agentorchestrator.Run
	acquired map[string]int
}

func (fake *recoveryOrchestrator) EnqueueRun(_ context.Context,
	_ agentorchestrator.EnqueueInput,
) (agentorchestrator.EnqueueResult, error) {
	return agentorchestrator.EnqueueResult{Run: fake.runs[recoveryParentRun]}, nil
}

func (fake *recoveryOrchestrator) GetRun(_ context.Context, _, runID string) (agentorchestrator.Run, error) {
	run, ok := fake.runs[runID]
	if !ok {
		return agentorchestrator.Run{}, errors.New("run not found")
	}
	return run, nil
}

func (fake *recoveryOrchestrator) AcquireStep(_ context.Context,
	input agentorchestrator.AcquireInput,
) (agentorchestrator.Lease, error) {
	fake.acquired[input.RunID]++
	run := fake.runs[input.RunID]
	generation := int64(len(run.Attempts) + 1)
	attemptID := recoveryChildAttempt
	if input.RunID == recoveryParentRun {
		attemptID = "attempt_0000000000000006"
	}
	lease := agentorchestrator.Lease{Attempt: agentorchestrator.Attempt{
		ID: attemptID, RunID: input.RunID, StepID: input.StepID, Generation: generation,
		State: agentorchestrator.AttemptLeased, LeaseOwner: input.LeaseOwner,
		LeaseExpiresAt: testNow.Add(input.LeaseDuration),
	}, Token: "lease_00000000000000000000000000000004"}
	return lease, nil
}

func (*recoveryOrchestrator) TransitionAttempt(context.Context, agentorchestrator.TransitionInput) error {
	return nil
}

func (*recoveryOrchestrator) HeartbeatAttempt(_ context.Context,
	_ agentorchestrator.HeartbeatInput,
) (time.Time, error) {
	return testNow.Add(time.Minute), nil
}

type recoveryDelegation struct {
	orchestrator *recoveryOrchestrator
	authorities  map[string]agentdelegation.Authority
	children     []agentdelegation.ChildLineage
	pending      []agentdelegation.ReapTarget
	enqueues     int
	cascades     int
	settlements  int
}

func (fake *recoveryDelegation) GetAuthority(_ context.Context, _, runID string) (agentdelegation.Authority, error) {
	authority, ok := fake.authorities[runID]
	if !ok {
		return agentdelegation.Authority{}, agentdelegation.ErrNotFound
	}
	return authority, nil
}

func (fake *recoveryDelegation) ListChildren(context.Context, string, string) ([]agentdelegation.ChildLineage, error) {
	return append([]agentdelegation.ChildLineage(nil), fake.children...), nil
}

func (fake *recoveryDelegation) PendingReaps(context.Context, int) ([]agentdelegation.ReapTarget, error) {
	return append([]agentdelegation.ReapTarget(nil), fake.pending...), nil
}

func (fake *recoveryDelegation) RegisterRoot(_ context.Context,
	_ agentdelegation.RegisterRootInput,
) (agentdelegation.Authority, bool, error) {
	return fake.authorities[recoveryParentRun], false, nil
}

func (fake *recoveryDelegation) EnqueueChild(_ context.Context,
	_ agentdelegation.ChildProposal,
) (agentdelegation.EnqueueResult, error) {
	fake.enqueues++
	return agentdelegation.EnqueueResult{}, errors.New("second Child forbidden")
}

func (*recoveryDelegation) AdmitLaunch(context.Context, agentdelegation.LaunchAdmissionInput) error {
	return nil
}

func (fake *recoveryDelegation) Settle(_ context.Context,
	_ agentdelegation.SettleInput,
) (bool, error) {
	fake.settlements++
	return fake.settlements == 1, nil
}

func (fake *recoveryDelegation) Cascade(_ context.Context,
	_ agentdelegation.CascadeInput,
) ([]agentdelegation.ReapTarget, error) {
	fake.cascades++
	child := fake.orchestrator.runs[recoveryChildRun]
	child.State = agentorchestrator.RunCanceled
	child.Steps[0].State = agentorchestrator.StepCanceled
	for index := range child.Attempts {
		child.Attempts[index].State = agentorchestrator.AttemptCanceled
	}
	fake.orchestrator.runs[recoveryChildRun] = child
	return nil, nil
}

func (*recoveryDelegation) Reconcile(context.Context, int) (int, error) { return 0, nil }

type recoveryAuthority struct{ privateKey ed25519.PrivateKey }

func (fake *recoveryAuthority) IssueAuthority(_ context.Context,
	input agentrunner.IssueAuthorityInput,
) (agentrunner.AuthorityTicket, json.RawMessage, error) {
	claims := agentrunner.NewAuthorityClaims(input.CallerIdentity, input.RunnerID, input.Method,
		input.RequestID, input.Nonce, input.RequestFingerprint, input.SnapshotFingerprint,
		input.Attempt, 0, testNow, testNow.Add(input.TTL))
	ticket, err := agentrunner.SignAuthority(fake.privateKey, claims)
	return ticket, nil, err
}

func (*recoveryAuthority) CompleteRequest(context.Context, string, string, string, string, []byte) error {
	return nil
}

type recoveryRunnerState struct{}

func (*recoveryRunnerState) ExpectSandbox(context.Context, agentrunner.ExpectedSandbox) error {
	return nil
}
func (*recoveryRunnerState) UpdateSandbox(context.Context, string, string, int64, string, string, string) error {
	return nil
}
func (*recoveryRunnerState) RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error) {
	return nil, nil
}

type recoveryRunnerClient struct{ calls map[string]int }

func (fake *recoveryRunnerClient) Call(_ context.Context,
	request agentrunner.Request,
) (agentrunner.Response, error) {
	fake.calls[request.Method]++
	switch request.Method {
	case agentrunner.MethodLaunch:
		return agentrunner.Response{Body: agentrunner.LaunchResult{Accepted: true,
			Attempt:   request.Launch.Attempt.Identity(),
			SandboxID: fmt.Sprintf("sandbox_%016d", fake.calls[request.Method])}}, nil
	case agentrunner.MethodHeartbeat:
		expiresAt := testNow.Add(time.Minute)
		return agentrunner.Response{Body: agentrunner.HeartbeatResult{Accepted: true, LeaseExpiresAt: &expiresAt}}, nil
	case agentrunner.MethodCancel:
		return agentrunner.Response{Body: agentrunner.CancelResult{Accepted: true, ObservedTerminal: "canceled"}}, nil
	case agentrunner.MethodReconcile:
		return agentrunner.Response{Body: agentrunner.ReconcileResult{RunnerID: "neo-runner-primary"}}, nil
	case agentrunner.MethodList:
		return agentrunner.Response{Body: agentrunner.ListResult{RunnerID: "neo-runner-primary"}}, nil
	default:
		return agentrunner.Response{}, errors.New("unsupported Runner method")
	}
}

type recoveryTerminal struct{ orchestrator *recoveryOrchestrator }

func (fake *recoveryTerminal) FinalizeParentCanceled(_ context.Context,
	_ ParentTerminalInput,
) error {
	parent := fake.orchestrator.runs[recoveryParentRun]
	parent.State = agentorchestrator.RunCanceled
	parent.Steps[0].State = agentorchestrator.StepCanceled
	fake.orchestrator.runs[recoveryParentRun] = parent
	return nil
}

type recoveryState struct{}

func (recoveryState) ResolveParent(context.Context, string, string) (string, error) {
	return recoveryParentRun, nil
}

func recoveryServiceFixture(t *testing.T) (*Service, *recoveryOrchestrator,
	*recoveryDelegation, *recoveryRunnerClient,
) {
	t.Helper()
	plan := validPlan()
	rootGrant, rootRegistry, err := plan.RootAuthority(recoveryParentRun, testNow)
	if err != nil {
		t.Fatal(err)
	}
	rootGrantFingerprint, _ := agentbroker.GrantFingerprint(rootGrant)
	childGrant := plan.ChildGrantTemplate(testNow)
	childGrant.Run = agentbroker.RunBinding{RunID: recoveryChildRun,
		ParentRunID: recoveryParentRun, Depth: 1}
	childGrant.GrantID = "grant_0000000000000005"
	childGrantFingerprint, _ := agentbroker.GrantFingerprint(childGrant)
	childRegistry, err := agentbroker.BuildRegistry(plan.catalog(), plan.ChildRequestedTools, childGrant, testNow)
	if err != nil {
		t.Fatal(err)
	}
	orchestrator := &recoveryOrchestrator{runs: map[string]agentorchestrator.Run{
		recoveryParentRun: {ID: recoveryParentRun, UserID: plan.UserID,
			SnapshotFingerprint: fingerprint('7'), State: agentorchestrator.RunRunning,
			Steps: []agentorchestrator.Step{{ID: recoveryParentStep, RunID: recoveryParentRun,
				State: agentorchestrator.StepRunning}},
			Attempts: []agentorchestrator.Attempt{{ID: recoveryParentAttempt,
				RunID: recoveryParentRun, StepID: recoveryParentStep, Generation: 1,
				State: agentorchestrator.AttemptRunning, LeaseOwner: "neo-runner-primary",
				LeaseExpiresAt: testNow.Add(time.Minute)}}},
		recoveryChildRun: {ID: recoveryChildRun, UserID: plan.UserID,
			SnapshotFingerprint: fingerprint('8'), State: agentorchestrator.RunQueued,
			Steps: []agentorchestrator.Step{{ID: recoveryChildStep, RunID: recoveryChildRun,
				State: agentorchestrator.StepReady}}},
	}, acquired: map[string]int{}}
	delegation := &recoveryDelegation{orchestrator: orchestrator,
		authorities: map[string]agentdelegation.Authority{
			recoveryParentRun: {RunID: recoveryParentRun, RootRunID: recoveryParentRun,
				Depth: 0, UserID: plan.UserID, Model: DelegationModel(plan.Model),
				Grant: rootGrant, GrantFingerprint: rootGrantFingerprint,
				Registry: rootRegistry, RegistryFingerprint: rootRegistry.Fingerprint,
				ExpiresAt: rootGrant.ExpiresAt, State: "active"},
			recoveryChildRun: {RunID: recoveryChildRun, RootRunID: recoveryParentRun,
				ParentRunID: recoveryParentRun, Depth: 1, UserID: plan.UserID,
				Model: DelegationModel(plan.Model), Grant: childGrant,
				GrantFingerprint: childGrantFingerprint, Registry: childRegistry,
				RegistryFingerprint: childRegistry.Fingerprint,
				ExpiresAt:           childGrant.ExpiresAt, State: "active"},
		}, children: []agentdelegation.ChildLineage{{ChildRunID: recoveryChildRun,
			IdempotencyKey: plan.ChildIdempotencyKey, ParentAttemptID: recoveryParentAttempt,
			ParentGeneration: 1, State: "reserved"}}}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	client := &recoveryRunnerClient{calls: map[string]int{}}
	service, err := NewService(Config{RunnerID: "neo-runner-primary",
		CallerIdentity: "spiffe://neo-chat/agent-runtime-child-canary",
		PollInterval:   time.Second, AuthorityTTL: 10 * time.Second, BatchSize: 100},
		plan, recoveryGate{}, orchestrator, delegation, &recoveryAuthority{privateKey: privateKey},
		&recoveryRunnerState{}, client, &recoveryTerminal{orchestrator: orchestrator}, recoveryState{})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return testNow }
	return service, orchestrator, delegation, client
}
