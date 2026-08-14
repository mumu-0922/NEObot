package agentbrokercanary

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestServiceCycleRunsFiveIndependentActionsThroughRunnerAndReplaysExactly(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	plan := validCanaryPlan(t, now, t.TempDir())
	bindings := mustBindings(t, plan, now)
	orchestrator := &canaryOrchestratorFake{}
	authority := newCanaryAuthorityFake(t)
	runnerState := &canaryRunnerStateFake{}
	client := newCanaryRunnerClientFake(bindings, now)
	service, err := NewService(Config{RunnerID: testCanaryRunner, CallerIdentity: BrokerCanaryIdentity,
		PollInterval: time.Second, AuthorityTTL: 10 * time.Second, BatchSize: 100},
		plan, bindings, canaryGateFake{}, orchestrator, authority, runnerState, client)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	result, err := service.Cycle(context.Background())
	if err != nil || result.Completed != 5 || result.Terminal != 5 || client.prepareDispatches != 5 ||
		client.commitDispatches != 5 || client.possibleSendDispatches != 1 ||
		orchestrator.enqueues != 5 || orchestrator.acquires != 5 || runnerState.terminals != 5 {
		t.Fatalf("Cycle = %#v, %v, client=%#v orchestrator=%#v state=%#v",
			result, err, client, orchestrator, runnerState)
	}
	for _, binding := range bindings.All() {
		methods := authority.methods[binding.RunID]
		cancelIndex, commitIndex := -1, -1
		for index, method := range methods {
			if method == agentrunner.MethodCancel {
				cancelIndex = index
			}
			if method == agentrunner.MethodCommit {
				commitIndex = index
			}
		}
		if cancelIndex < 0 || commitIndex < 0 || cancelIndex > commitIndex {
			t.Fatalf("cancel authority was not issued before terminal Commit for %s: %v", binding.Plan.ID, methods)
		}
	}
}

func TestServiceRejectsWrongIdentityAndSurfacesRecoveryPending(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	plan := validCanaryPlan(t, now, t.TempDir())
	bindings := mustBindings(t, plan, now)
	if _, err := NewService(Config{RunnerID: testCanaryRunner,
		CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary", PollInterval: time.Second,
		AuthorityTTL: 10 * time.Second, BatchSize: 100}, plan, bindings, canaryGateFake{},
		&canaryOrchestratorFake{}, newCanaryAuthorityFake(t), &canaryRunnerStateFake{},
		newCanaryRunnerClientFake(bindings, now)); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("shared identity = %v", err)
	}
	orchestrator := &canaryOrchestratorFake{recovering: true}
	service, _ := NewService(Config{RunnerID: testCanaryRunner, CallerIdentity: BrokerCanaryIdentity,
		PollInterval: time.Second, AuthorityTTL: 10 * time.Second, BatchSize: 100}, plan, bindings,
		canaryGateFake{}, orchestrator, newCanaryAuthorityFake(t), &canaryRunnerStateFake{},
		newCanaryRunnerClientFake(bindings, now))
	service.now = func() time.Time { return now }
	if _, err := service.Cycle(context.Background()); !errors.Is(err, ErrRecoveryPending) || orchestrator.acquires != 0 {
		t.Fatalf("recovery Cycle = %v, acquires=%d", err, orchestrator.acquires)
	}
}

type canaryGateFake struct{ err error }

func (gate canaryGateFake) Verify(time.Time) error { return gate.err }

type canaryOrchestratorFake struct {
	enqueues, acquires int
	recovering         bool
}

func (fake *canaryOrchestratorFake) EnqueueRunWithID(_ context.Context, runID string,
	input agentorchestrator.EnqueueInput,
) (agentrunnerResult agentorchestrator.EnqueueResult, err error) {
	fake.enqueues++
	step := input.Steps[0]
	run := agentorchestrator.Run{ID: runID, UserID: input.UserID, State: agentorchestrator.RunQueued,
		SnapshotFingerprint: mustSnapshotFingerprint(input.Snapshot), NextSequence: 3,
		Steps: []agentorchestrator.Step{{ID: step.ID, RunID: runID, Kind: step.Kind, State: agentorchestrator.StepReady}}}
	if fake.recovering {
		run.Attempts = []agentorchestrator.Attempt{{ID: "attempt_0123456789abcdef", RunID: runID,
			StepID: step.ID, Generation: 1, State: agentorchestrator.AttemptRunning,
			LeaseOwner: testCanaryRunner, LeaseExpiresAt: time.Date(2026, 8, 14, 10, 1, 0, 0, time.UTC)}}
	}
	return agentorchestrator.EnqueueResult{Run: run, Created: true}, nil
}

func (fake *canaryOrchestratorFake) AcquireStep(_ context.Context,
	input agentorchestrator.AcquireInput,
) (agentrunnerLease agentorchestrator.Lease, err error) {
	fake.acquires++
	return agentorchestrator.Lease{Attempt: agentorchestrator.Attempt{
		ID: fmt.Sprintf("attempt_%016x", fake.acquires), RunID: input.RunID, StepID: input.StepID,
		Generation: 1, State: agentorchestrator.AttemptLeased, LeaseOwner: input.LeaseOwner,
		LeaseExpiresAt: time.Date(2026, 8, 14, 10, 1, 0, 0, time.UTC)},
		Token: fmt.Sprintf("lease_%024d", fake.acquires)}, nil
}
func (*canaryOrchestratorFake) TransitionAttempt(context.Context, agentorchestrator.TransitionInput) error {
	return nil
}
func (*canaryOrchestratorFake) HeartbeatAttempt(context.Context, agentorchestrator.HeartbeatInput) (time.Time, error) {
	return time.Date(2026, 8, 14, 10, 1, 0, 0, time.UTC), nil
}

type canaryAuthorityFake struct {
	private ed25519.PrivateKey
	methods map[string][]string
}

func newCanaryAuthorityFake(t *testing.T) *canaryAuthorityFake {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &canaryAuthorityFake{private: privateKey, methods: map[string][]string{}}
}

func (fake *canaryAuthorityFake) IssueAuthority(_ context.Context, input agentrunner.IssueAuthorityInput) (agentrunner.AuthorityTicket, json.RawMessage, error) {
	fake.methods[input.Attempt.RunID] = append(fake.methods[input.Attempt.RunID], input.Method)
	claims := agentrunner.NewAuthorityClaims(input.CallerIdentity, input.RunnerID, input.Method,
		input.RequestID, input.Nonce, input.RequestFingerprint, input.SnapshotFingerprint,
		input.Attempt, 0, time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 14, 10, 0, 10, 0, time.UTC))
	ticket, err := agentrunner.SignAuthority(fake.private, claims)
	return ticket, nil, err
}
func (*canaryAuthorityFake) CompleteRequest(context.Context, string, string, string, string, []byte) error {
	return nil
}

type canaryRunnerStateFake struct{ terminals int }

func (*canaryRunnerStateFake) ExpectSandbox(context.Context, agentrunner.ExpectedSandbox) error {
	return nil
}
func (fake *canaryRunnerStateFake) UpdateSandbox(_ context.Context, _, _ string, _ int64, expected, to, _ string) error {
	if expected == "stopping" && to == "terminal" {
		fake.terminals++
	}
	return nil
}
func (*canaryRunnerStateFake) RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error) {
	return nil, nil
}

type storedRunnerResponse struct {
	response agentrunner.Response
	err      error
}

type canaryRunnerClientFake struct {
	bindings               *Bindings
	now                    time.Time
	responses              map[string]storedRunnerResponse
	prepareDispatches      int
	commitDispatches       int
	possibleSendDispatches int
}

func newCanaryRunnerClientFake(bindings *Bindings, now time.Time) *canaryRunnerClientFake {
	return &canaryRunnerClientFake{bindings: bindings, now: now, responses: map[string]storedRunnerResponse{}}
}

func (fake *canaryRunnerClientFake) Call(_ context.Context, request agentrunner.Request) (agentrunner.Response, error) {
	if stored, ok := fake.responses[request.RequestID]; ok {
		return stored.response, stored.err
	}
	response := agentrunner.Response{SchemaVersion: agentrunner.ProtocolVersion, Method: request.Method,
		RequestID: request.RequestID, SentAt: fake.now, Nonce: request.Nonce}
	var operationErr error
	switch request.Method {
	case agentrunner.MethodProbe:
		response.Body = agentrunner.ProbeResult{Ready: true, ProbeFingerprint: testCanaryFingerprint("6")}
	case agentrunner.MethodReconcile:
		response.Body = agentrunner.ReconcileResult{RunnerID: testCanaryRunner}
	case agentrunner.MethodList:
		response.Body = agentrunner.ListResult{RunnerID: testCanaryRunner, Sandboxes: []agentrunner.SandboxDescriptor{}}
	case agentrunner.MethodLaunch:
		response.Body = agentrunner.LaunchResult{Accepted: true, Attempt: request.Launch.Attempt.Identity(),
			SandboxID: "sandbox_" + request.Launch.Attempt.AttemptID[len("attempt_"):]}
	case agentrunner.MethodHeartbeat:
		response.Body = agentrunner.HeartbeatResult{Accepted: true}
	case agentrunner.MethodPrepare:
		fake.prepareDispatches++
		response.Body = agentrunner.PrepareResult{Prepared: true,
			IntentID:          "intent_" + request.Prepare.Attempt.AttemptID[len("attempt_"):],
			IntentFingerprint: testCanaryFingerprint("7"),
			IdempotencyKey:    "commit_0123456789abcdefghijklmn", Approval: "automatic"}
	case agentrunner.MethodCommit:
		fake.commitDispatches++
		binding := fake.bindings.byRun[request.Commit.Attempt.RunID]
		if binding.Plan.ID == ActionPossibleSend {
			fake.possibleSendDispatches++
			response.Body = agentrunner.CommitResult{Outcome: "outcome_unknown",
				IdempotencyKey: request.Commit.IdempotencyKey,
				Error:          &agentrunner.RPCError{Code: agentrunner.ErrorOutcomeUnknown}}
			operationErr = agentrunner.ErrOutcomeUnknown
		} else {
			response.Body = agentrunner.CommitResult{Outcome: "committed",
				IdempotencyKey:     request.Commit.IdempotencyKey,
				ReceiptFingerprint: testCanaryFingerprint("8")}
		}
	case agentrunner.MethodCancel:
		response.Body = agentrunner.CancelResult{Accepted: true, ObservedTerminal: "canceled"}
	default:
		return agentrunner.Response{}, agentrunner.ErrInvalidTransition
	}
	fake.responses[request.RequestID] = storedRunnerResponse{response: response, err: operationErr}
	return response, operationErr
}

func mustSnapshotFingerprint(snapshot json.RawMessage) string {
	fingerprint, _ := agentorchestrator.SnapshotFingerprint(snapshot)
	return fingerprint
}
