package agentprojectcanary

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
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestServiceCycleApprovesCommitsAndCleansOneProjectMutation(t *testing.T) {
	service, orchestrator, broker, client, cleanup := newProjectCanaryServiceFixture(t, "committed")

	result, err := service.Cycle(context.Background())
	if err != nil || result.Completed != 1 || result.Terminal != 1 {
		t.Fatalf("Cycle() = %#v, %v", result, err)
	}
	if orchestrator.acquires != 1 || broker.approvals != 1 || client.prepareDispatches != 1 ||
		client.commitDispatches != 1 || client.cancelDispatches != 1 || cleanup.cleanups != 1 ||
		cleanup.reconciles != 0 {
		t.Fatalf("calls orchestrator=%#v broker=%#v client=%#v cleanup=%#v",
			orchestrator, broker, client, cleanup)
	}
	binding, _ := service.bindings.Action()
	if cleanup.request.Resource != binding.Plan.Resource || cleanup.request.BaseRevision != binding.Plan.BaseRevision ||
		cleanup.request.ArgumentsFingerprint != binding.ArgumentsFingerprint || cleanup.receipt != repeatedFingerprint("9") {
		t.Fatalf("cleanup binding = %#v receipt=%q", cleanup.request, cleanup.receipt)
	}
}

func TestServiceApprovalRejectionPerformsZeroProjectWrites(t *testing.T) {
	service, _, broker, client, cleanup := newProjectCanaryServiceFixture(t, "committed")
	broker.approvalErr = agentbroker.ErrApprovalDenied

	result, err := service.Cycle(context.Background())
	if !errors.Is(err, ErrUnavailable) || result.Completed != 0 || result.Terminal != 0 {
		t.Fatalf("Cycle() = %#v, %v", result, err)
	}
	if broker.approvals != 1 || client.commitDispatches != 0 || cleanup.cleanups != 0 ||
		client.cancelDispatches != 0 {
		t.Fatalf("approval rejection dispatched mutation: broker=%#v client=%#v cleanup=%#v",
			broker, client, cleanup)
	}
}

func TestServiceOutcomeUnknownNeverCleansOrRedispatches(t *testing.T) {
	service, _, broker, client, cleanup := newProjectCanaryServiceFixture(t, "outcome_unknown")

	result, err := service.Cycle(context.Background())
	if err != nil || result.Completed != 1 || result.Terminal != 1 {
		t.Fatalf("Cycle() = %#v, %v", result, err)
	}
	if broker.approvals != 1 || client.commitDispatches != 1 || cleanup.cleanups != 0 ||
		client.cancelDispatches != 1 {
		t.Fatalf("outcome_unknown calls broker=%#v client=%#v cleanup=%#v", broker, client, cleanup)
	}
}

func TestServiceTerminalRestartReconcilesCleanupWithoutMutation(t *testing.T) {
	service, orchestrator, broker, client, cleanup := newProjectCanaryServiceFixture(t, "committed")
	orchestrator.terminal = true
	cleanup.reconcileFound = true
	binding, _ := service.bindings.Action()
	cleanup.reconcileRevision = binding.Plan.BaseRevision

	result, err := service.Cycle(context.Background())
	if err != nil || result.Completed != 1 || result.Terminal != 1 {
		t.Fatalf("Cycle() = %#v, %v", result, err)
	}
	if orchestrator.acquires != 0 || broker.approvals != 0 || client.prepareDispatches != 0 ||
		client.commitDispatches != 0 || cleanup.cleanups != 0 || cleanup.reconciles != 1 {
		t.Fatalf("terminal replay mutated: orchestrator=%#v broker=%#v client=%#v cleanup=%#v",
			orchestrator, broker, client, cleanup)
	}
}

func TestServiceCommittedCleanupFailureRequiresRestartRecovery(t *testing.T) {
	service, _, _, client, cleanup := newProjectCanaryServiceFixture(t, "committed")
	cleanup.cleanupErr = errors.New("cleanup unavailable")

	result, err := service.Cycle(context.Background())
	if !errors.Is(err, ErrRecoveryPending) || result.Completed != 0 || result.Terminal != 1 {
		t.Fatalf("Cycle() = %#v, %v", result, err)
	}
	if client.commitDispatches != 1 || cleanup.cleanups != 1 || client.cancelDispatches != 0 {
		t.Fatalf("cleanup failure calls client=%#v cleanup=%#v", client, cleanup)
	}
}

type projectCanaryGateFake struct{ err error }

func (fake projectCanaryGateFake) Verify(time.Time) error { return fake.err }

type projectCanaryOrchestratorFake struct {
	now                 time.Time
	snapshotFingerprint string
	terminal            bool
	enqueues            int
	acquires            int
}

func (fake *projectCanaryOrchestratorFake) EnqueueRunWithID(_ context.Context, runID string,
	input agentorchestrator.EnqueueInput,
) (agentorchestrator.EnqueueResult, error) {
	fake.enqueues++
	state := agentorchestrator.RunQueued
	if fake.terminal {
		state = agentorchestrator.RunCanceled
	}
	return agentorchestrator.EnqueueResult{Created: !fake.terminal, Run: agentorchestrator.Run{
		ID: runID, UserID: input.UserID, State: state, SnapshotFingerprint: fake.snapshotFingerprint,
		NextSequence: 3, Steps: []agentorchestrator.Step{{ID: input.Steps[0].ID, RunID: runID,
			Kind: input.Steps[0].Kind, State: agentorchestrator.StepReady}}}}, nil
}

func (fake *projectCanaryOrchestratorFake) AcquireStep(_ context.Context,
	input agentorchestrator.AcquireInput,
) (agentorchestrator.Lease, error) {
	fake.acquires++
	return agentorchestrator.Lease{Attempt: agentorchestrator.Attempt{
		ID: fmt.Sprintf("attempt_%016x", fake.acquires), RunID: input.RunID, StepID: input.StepID,
		Generation: 1, State: agentorchestrator.AttemptLeased, LeaseOwner: input.LeaseOwner,
		LeaseExpiresAt: fake.now.Add(time.Minute)}, Token: fmt.Sprintf("lease_%024d", fake.acquires)}, nil
}

func (*projectCanaryOrchestratorFake) TransitionAttempt(context.Context, agentorchestrator.TransitionInput) error {
	return nil
}

func (fake *projectCanaryOrchestratorFake) HeartbeatAttempt(context.Context,
	agentorchestrator.HeartbeatInput,
) (time.Time, error) {
	return fake.now.Add(time.Minute), nil
}

type projectCanaryAuthorityFake struct {
	private ed25519.PrivateKey
}

func newProjectCanaryAuthorityFake(t *testing.T) *projectCanaryAuthorityFake {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &projectCanaryAuthorityFake{private: privateKey}
}

func (fake *projectCanaryAuthorityFake) IssueAuthority(_ context.Context,
	input agentrunner.IssueAuthorityInput,
) (agentrunner.AuthorityTicket, json.RawMessage, error) {
	claims := agentrunner.NewAuthorityClaims(input.CallerIdentity, input.RunnerID, input.Method,
		input.RequestID, input.Nonce, input.RequestFingerprint, input.SnapshotFingerprint,
		input.Attempt, 0, time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 15, 1, 0, 10, 0, time.UTC))
	ticket, err := agentrunner.SignAuthority(fake.private, claims)
	return ticket, nil, err
}

func (*projectCanaryAuthorityFake) CompleteRequest(context.Context, string, string, string, string, []byte) error {
	return nil
}

type projectCanaryRunnerStateFake struct{}

func (*projectCanaryRunnerStateFake) ExpectSandbox(context.Context, agentrunner.ExpectedSandbox) error {
	return nil
}

func (*projectCanaryRunnerStateFake) UpdateSandbox(context.Context, string, string, int64, string, string, string) error {
	return nil
}

func (*projectCanaryRunnerStateFake) RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error) {
	return nil, nil
}

type projectCanaryBrokerFake struct {
	binding     Binding
	prepared    agentrunner.PrepareResult
	approvals   int
	approvalErr error
}

func (*projectCanaryBrokerFake) Prepare(context.Context, agentbroker.PrepareInput) (agentbroker.PreparedIntent, error) {
	return agentbroker.PreparedIntent{}, errors.New("direct Prepare is not allowed in the controller")
}

func (fake *projectCanaryBrokerFake) DecideApproval(_ context.Context,
	input agentbroker.ApprovalInput,
) (agentbroker.PreparedIntent, error) {
	fake.approvals++
	if fake.approvalErr != nil {
		return agentbroker.PreparedIntent{}, fake.approvalErr
	}
	return agentbroker.PreparedIntent{IntentID: fake.prepared.IntentID,
		IntentFingerprint: fake.prepared.IntentFingerprint, IdempotencyKey: fake.prepared.IdempotencyKey,
		ApprovalClass: agentbroker.ApprovalPerCommit, ApprovalRevision: 1,
		State: agentbroker.IntentApproved, ToolIdentity: fake.binding.Plan.ToolIdentity,
		Resource: fake.binding.Plan.Resource, ArgumentsFingerprint: fake.binding.ArgumentsFingerprint}, nil
}

func (*projectCanaryBrokerFake) Commit(context.Context, agentbroker.CommitInput) (agentbroker.CommitResult, error) {
	return agentbroker.CommitResult{}, errors.New("direct Commit is not allowed in the controller")
}

type projectCanaryRunnerClientFake struct {
	now               time.Time
	runnerID          string
	commitOutcome     string
	responses         map[string]storedProjectRunnerResponse
	prepareDispatches int
	commitDispatches  int
	cancelDispatches  int
}

type storedProjectRunnerResponse struct {
	response agentrunner.Response
	err      error
}

func (fake *projectCanaryRunnerClientFake) Call(_ context.Context,
	request agentrunner.Request,
) (agentrunner.Response, error) {
	if stored, ok := fake.responses[request.RequestID]; ok {
		return stored.response, stored.err
	}
	response := agentrunner.Response{SchemaVersion: agentrunner.ProtocolVersion,
		Method: request.Method + ".result", RequestID: request.RequestID, SentAt: fake.now, Nonce: request.Nonce}
	var operationErr error
	switch request.Method {
	case agentrunner.MethodProbe:
		response.Body = agentrunner.ProbeResult{Ready: true, ProbeFingerprint: repeatedFingerprint("8")}
	case agentrunner.MethodReconcile:
		response.Body = agentrunner.ReconcileResult{RunnerID: fake.runnerID}
	case agentrunner.MethodList:
		response.Body = agentrunner.ListResult{RunnerID: fake.runnerID, Sandboxes: []agentrunner.SandboxDescriptor{}}
	case agentrunner.MethodLaunch:
		response.Body = agentrunner.LaunchResult{Accepted: true, Attempt: request.Launch.Attempt.Identity(),
			SandboxID: "sandbox_0123456789abcdef"}
	case agentrunner.MethodHeartbeat:
		response.Body = agentrunner.HeartbeatResult{Accepted: true}
	case agentrunner.MethodPrepare:
		fake.prepareDispatches++
		expiresAt := fake.now.Add(5 * time.Minute)
		response.Body = agentrunner.PrepareResult{Prepared: true, IntentID: "intent_0123456789abcdef",
			IntentFingerprint: repeatedFingerprint("e"), IdempotencyKey: "commit_0123456789abcdefghijklmn",
			Approval: agentbroker.ApprovalPerCommit, ExpiresAt: &expiresAt}
	case agentrunner.MethodCommit:
		fake.commitDispatches++
		if fake.commitOutcome == "outcome_unknown" {
			response.Body = agentrunner.CommitResult{Outcome: "outcome_unknown",
				IdempotencyKey: request.Commit.IdempotencyKey,
				Error:          &agentrunner.RPCError{Code: agentrunner.ErrorOutcomeUnknown}}
			operationErr = agentrunner.ErrOutcomeUnknown
		} else {
			response.Body = agentrunner.CommitResult{Outcome: fake.commitOutcome,
				IdempotencyKey: request.Commit.IdempotencyKey, ReceiptFingerprint: repeatedFingerprint("9")}
		}
	case agentrunner.MethodCancel:
		fake.cancelDispatches++
		response.Body = agentrunner.CancelResult{Accepted: true, ObservedTerminal: "canceled"}
	default:
		return agentrunner.Response{}, agentrunner.ErrInvalidTransition
	}
	fake.responses[request.RequestID] = storedProjectRunnerResponse{response: response, err: operationErr}
	return response, operationErr
}

type projectCanaryCleanupFake struct {
	cleanups          int
	reconciles        int
	reconcileFound    bool
	reconcileRevision string
	cleanupErr        error
	request           agentbroker.ExecutionRequest
	receipt           string
}

func (fake *projectCanaryCleanupFake) Cleanup(_ context.Context, request agentbroker.ExecutionRequest,
	receipt string,
) (string, error) {
	fake.cleanups++
	fake.request = request
	fake.receipt = receipt
	return request.BaseRevision, fake.cleanupErr
}

func (fake *projectCanaryCleanupFake) ReconcileCleanup(context.Context, string, string) (bool, string, error) {
	fake.reconciles++
	return fake.reconcileFound, fake.reconcileRevision, nil
}

func newProjectCanaryServiceFixture(t *testing.T, commitOutcome string) (*Service,
	*projectCanaryOrchestratorFake, *projectCanaryBrokerFake, *projectCanaryRunnerClientFake,
	*projectCanaryCleanupFake,
) {
	t.Helper()
	now := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	plan := validProjectPlan(t, now)
	bindings, err := NewBindings(plan, testProjectRunner, now)
	if err != nil {
		t.Fatal(err)
	}
	binding, _ := bindings.Action()
	prepared := agentrunner.PrepareResult{Prepared: true, IntentID: "intent_0123456789abcdef",
		IntentFingerprint: repeatedFingerprint("e"), IdempotencyKey: "commit_0123456789abcdefghijklmn",
		Approval: agentbroker.ApprovalPerCommit, ExpiresAt: timePointer(now.Add(5 * time.Minute))}
	approval := &Approval{Payload: approvalPayloadFor(binding, plan, repeatedFingerprint("a")[7:47],
		repeatedFingerprint("c"), repeatedFingerprint("d"), now)}
	orchestrator := &projectCanaryOrchestratorFake{now: now, snapshotFingerprint: binding.SnapshotFingerprint}
	broker := &projectCanaryBrokerFake{binding: binding, prepared: prepared}
	client := &projectCanaryRunnerClientFake{now: now, runnerID: testProjectRunner,
		commitOutcome: commitOutcome, responses: map[string]storedProjectRunnerResponse{}}
	cleanup := &projectCanaryCleanupFake{}
	service, err := NewService(Config{RunnerID: testProjectRunner, CallerIdentity: ProjectCanaryIdentity,
		PollInterval: time.Second, AuthorityTTL: 10 * time.Second, BatchSize: 100}, plan, bindings,
		projectCanaryGateFake{}, orchestrator, newProjectCanaryAuthorityFake(t),
		&projectCanaryRunnerStateFake{}, client, broker, approval, cleanup)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service, orchestrator, broker, client, cleanup
}
