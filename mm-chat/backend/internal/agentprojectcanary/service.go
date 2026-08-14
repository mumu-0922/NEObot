package agentprojectcanary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

var (
	ErrUnavailable     = errors.New("PROJECT_CANARY_UNAVAILABLE")
	ErrRecoveryPending = errors.New("PROJECT_CANARY_RECOVERY_PENDING")
)

var canaryRequiredFeatures = []string{
	"rootless_userns", "cgroup_v2", "seccomp", "readonly_rootfs",
	"snapshot_workspace", "network_none", "pidfd_kill", "cgroup_reap",
}

type Gate interface{ Verify(time.Time) error }

type Orchestrator interface {
	EnqueueRunWithID(context.Context, string, agentorchestrator.EnqueueInput) (agentorchestrator.EnqueueResult, error)
	AcquireStep(context.Context, agentorchestrator.AcquireInput) (agentorchestrator.Lease, error)
	TransitionAttempt(context.Context, agentorchestrator.TransitionInput) error
	HeartbeatAttempt(context.Context, agentorchestrator.HeartbeatInput) (time.Time, error)
}

type AuthorityService interface {
	IssueAuthority(context.Context, agentrunner.IssueAuthorityInput) (agentrunner.AuthorityTicket, json.RawMessage, error)
	CompleteRequest(context.Context, string, string, string, string, []byte) error
}

type RunnerRepository interface {
	ExpectSandbox(context.Context, agentrunner.ExpectedSandbox) error
	UpdateSandbox(context.Context, string, string, int64, string, string, string) error
	RecoverySandboxes(context.Context, int) ([]agentrunner.RecoverySandbox, error)
}

type RunnerClient interface {
	Call(context.Context, agentrunner.Request) (agentrunner.Response, error)
}

type Config struct {
	RunnerID       string
	CallerIdentity string
	PollInterval   time.Duration
	AuthorityTTL   time.Duration
	BatchSize      int
}

type Service struct {
	config       Config
	plan         Plan
	bindings     *Bindings
	gate         Gate
	orchestrator Orchestrator
	authority    AuthorityService
	runnerState  RunnerRepository
	client       RunnerClient
	broker       Broker
	approval     *Approval
	cleanup      MutationCleanup
	now          func() time.Time
}

type MutationCleanup interface {
	Cleanup(context.Context, agentbroker.ExecutionRequest, string) (string, error)
	ReconcileCleanup(context.Context, string, string) (bool, string, error)
}

type CycleResult struct {
	Completed int
	Terminal  int
}

func NewService(config Config, plan Plan, bindings *Bindings, gate Gate, orchestrator Orchestrator,
	authority AuthorityService, runnerState RunnerRepository, client RunnerClient, broker Broker,
	approval *Approval, cleanup MutationCleanup,
) (*Service, error) {
	if config.RunnerID == "" || config.CallerIdentity != ProjectCanaryIdentity ||
		config.PollInterval < time.Second || config.PollInterval > time.Minute ||
		config.AuthorityTTL < time.Second || config.AuthorityTTL > 15*time.Second ||
		config.BatchSize < 1 || config.BatchSize > 1000 || bindings == nil ||
		bindings.runnerID != config.RunnerID || gate == nil || orchestrator == nil || authority == nil ||
		runnerState == nil || client == nil || broker == nil || approval == nil || cleanup == nil {
		return nil, ErrInvalidPlan
	}
	return &Service{config: config, plan: plan, bindings: bindings, gate: gate,
		orchestrator: orchestrator, authority: authority, runnerState: runnerState,
		client: client, broker: broker, approval: approval, cleanup: cleanup, now: time.Now}, nil
}

func (service *Service) Run(ctx context.Context) error {
	for {
		_, err := service.Cycle(ctx)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrRecoveryPending) {
			return err
		}
		if err := canaryWait(ctx, service.config.PollInterval); err != nil {
			return err
		}
	}
	for {
		if err := canaryWait(ctx, service.config.PollInterval); err != nil {
			return err
		}
		if err := service.Health(ctx); err != nil {
			return err
		}
	}
}

func (service *Service) Health(ctx context.Context) error {
	now := service.now().UTC()
	if err := service.gate.Verify(now); err != nil {
		return ErrUnavailable
	}
	probe, err := service.probe(ctx, now)
	if err != nil {
		return err
	}
	return service.reconcileEmpty(ctx, probe.ProbeFingerprint, now)
}

func (service *Service) Cycle(ctx context.Context) (CycleResult, error) {
	now := service.now().UTC()
	if err := service.gate.Verify(now); err != nil {
		return CycleResult{}, ErrUnavailable
	}
	probe, err := service.probe(ctx, now)
	if err != nil {
		return CycleResult{}, err
	}
	if err := service.reconcileEmpty(ctx, probe.ProbeFingerprint, now); err != nil {
		return CycleResult{}, err
	}
	result := CycleResult{}
	binding, ok := service.bindings.Action()
	if !ok {
		return result, ErrUnavailable
	}
	completed, terminal, err := service.runAction(ctx, binding, probe.ProbeFingerprint)
	if terminal {
		result.Terminal++
	}
	if completed {
		result.Completed++
	}
	if err != nil {
		return result, err
	}
	if err := service.reconcileEmpty(ctx, probe.ProbeFingerprint, service.now().UTC()); err != nil {
		return result, err
	}
	return result, nil
}

func (service *Service) runAction(ctx context.Context, binding Binding, probeFingerprint string) (bool, bool, error) {
	enqueued, err := service.orchestrator.EnqueueRunWithID(ctx, binding.RunID, agentorchestrator.EnqueueInput{
		UserID: service.plan.UserID, IdempotencyKey: binding.Plan.IdempotencyKey,
		Snapshot: binding.Snapshot, Steps: []agentorchestrator.StepPlan{{ID: binding.StepID, Kind: binding.Plan.ID}},
		ScopeBindings: []agentorchestrator.ScopeBinding{{Type: "runner", Value: service.config.RunnerID},
			{Type: "skill", Value: service.plan.PackageFingerprint}},
	})
	if err != nil || len(enqueued.Run.Steps) != 1 || enqueued.Run.ID != binding.RunID ||
		enqueued.Run.SnapshotFingerprint != binding.SnapshotFingerprint {
		return false, false, ErrUnavailable
	}
	if agentorchestrator.IsRunTerminal(enqueued.Run.State) {
		found, restored, cleanupErr := service.cleanup.ReconcileCleanup(context.WithoutCancel(ctx),
			service.plan.UserID, binding.RunID)
		if cleanupErr != nil || found && restored != binding.Plan.BaseRevision {
			return false, true, ErrRecoveryPending
		}
		if err := service.reconcileEmpty(ctx, probeFingerprint, service.now().UTC()); err != nil {
			return false, true, err
		}
		return true, true, nil
	}
	if len(enqueued.Run.Attempts) > 0 {
		latest := enqueued.Run.Attempts[len(enqueued.Run.Attempts)-1]
		if !agentorchestrator.IsAttemptTerminal(latest.State) && service.now().UTC().Before(latest.LeaseExpiresAt) {
			return false, false, ErrRecoveryPending
		}
	}
	if err := service.reconcileEmpty(ctx, probeFingerprint, service.now().UTC()); err != nil {
		return false, false, err
	}
	actor := agentorchestrator.Actor{Type: "orchestrator", ID: "g21.3-project-canary"}
	leaseDuration := time.Duration(service.plan.LeaseSeconds) * time.Second
	lease, err := service.orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{
		UserID: service.plan.UserID, RunID: binding.RunID, StepID: binding.StepID,
		LeaseOwner: service.config.RunnerID, LeaseDuration: leaseDuration, Actor: actor,
		ReasonCode: "PROJECT_CANARY_CLAIMED"})
	if err != nil {
		return false, false, ErrUnavailable
	}
	transition := agentorchestrator.TransitionInput{UserID: service.plan.UserID, RunID: lease.RunID,
		StepID: lease.StepID, AttemptID: lease.ID, Generation: lease.Generation,
		LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token, Actor: actor}
	transition.Expected, transition.To = agentorchestrator.AttemptLeased, agentorchestrator.AttemptStarting
	transition.ReasonCode = "PROJECT_CANARY_STARTING"
	if service.orchestrator.TransitionAttempt(ctx, transition) != nil {
		return false, false, ErrUnavailable
	}
	attempt := agentrunner.AttemptRef{RunID: lease.RunID, StepID: lease.StepID, AttemptID: lease.ID,
		LeaseGeneration: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token}
	grantFingerprint := binding.GrantFingerprint()
	launch := agentrunner.LaunchRequest{Attempt: attempt,
		Lineage: agentrunner.RunLineage{RootRunID: binding.RunID, Depth: 0},
		GrantID: binding.Grant.GrantID, GrantFingerprint: grantFingerprint,
		SnapshotFingerprint: binding.SnapshotFingerprint, Sandbox: service.plan.Sandbox,
		ToolRegistry: agentrunner.ToolRegistry{Depth: 0, Tools: []string{binding.Plan.ToolIdentity},
			RegistryFingerprint: binding.Registry.Fingerprint}, Argv: append([]string(nil), service.plan.Argv...)}
	response, signedLaunch, err := service.authorizedCall(ctx, agentrunner.MethodLaunch, launch,
		binding.SnapshotFingerprint, attempt, service.now().UTC())
	if err != nil {
		return false, false, err
	}
	launchResult, ok := response.Body.(agentrunner.LaunchResult)
	if !ok || !launchResult.Accepted || launchResult.SandboxID == "" || launchResult.Attempt != attempt.Identity() {
		return false, false, ErrUnavailable
	}
	expected := agentrunner.ExpectedSandbox{SandboxID: launchResult.SandboxID,
		CallerIdentity: service.config.CallerIdentity, RequestID: signedLaunch.RequestID,
		Nonce: signedLaunch.Nonce, UserID: service.plan.UserID, Attempt: attempt.Identity(),
		RunnerID: service.config.RunnerID, SnapshotFingerprint: binding.SnapshotFingerprint,
		SpecFingerprint: agentrunner.SandboxFingerprint(launch), ProbeFingerprint: probeFingerprint}
	if service.runnerState.ExpectSandbox(ctx, expected) != nil ||
		service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID,
			attempt.LeaseGeneration, "expected", "starting", "") != nil ||
		service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID,
			attempt.LeaseGeneration, "starting", "running", "") != nil {
		return false, false, ErrUnavailable
	}
	transition.Expected, transition.To, transition.ReasonCode = agentorchestrator.AttemptStarting,
		agentorchestrator.AttemptRunning, "PROJECT_CANARY_RUNNING"
	if service.orchestrator.TransitionAttempt(ctx, transition) != nil {
		return false, false, ErrUnavailable
	}
	heartbeat := agentrunner.HeartbeatRequest{Attempt: attempt, ObservedState: "running",
		LastEventSequence: enqueued.Run.NextSequence}
	if _, _, err := service.authorizedCall(ctx, agentrunner.MethodHeartbeat, heartbeat,
		binding.SnapshotFingerprint, attempt, service.now().UTC()); err != nil {
		return false, false, err
	}
	if _, err := service.orchestrator.HeartbeatAttempt(ctx, agentorchestrator.HeartbeatInput{
		UserID: service.plan.UserID, RunID: lease.RunID, StepID: lease.StepID, AttemptID: lease.ID,
		Generation: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token,
		LeaseDuration: leaseDuration, Actor: actor, ReasonCode: "PROJECT_CANARY_HEARTBEAT"}); err != nil {
		return false, false, ErrUnavailable
	}
	prepare := agentrunner.PrepareRequest{Attempt: attempt, SnapshotFingerprint: binding.SnapshotFingerprint,
		GrantID: binding.Grant.GrantID, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: binding.Registry.Fingerprint, ToolIdentity: binding.Plan.ToolIdentity,
		Capability: binding.Plan.Capability, Action: binding.Plan.Action, Resource: binding.Plan.Resource,
		Arguments:            append(json.RawMessage(nil), binding.Plan.Arguments...),
		ArgumentsFingerprint: binding.ArgumentsFingerprint, BaseRevision: binding.Plan.BaseRevision,
		TTLSeconds: binding.Plan.TTLSeconds}
	prepareResponse, signedPrepare, err := service.authorizedCall(ctx, agentrunner.MethodPrepare, prepare,
		binding.SnapshotFingerprint, attempt, service.now().UTC())
	if err != nil {
		return false, false, err
	}
	prepared, ok := prepareResponse.Body.(agentrunner.PrepareResult)
	if !ok || service.approval.VerifyPrepareResult(prepared, binding, service.now().UTC()) != nil {
		return false, false, ErrUnavailable
	}
	if err := service.assertReplay(ctx, signedPrepare, prepareResponse, nil); err != nil {
		return false, false, err
	}
	approved, err := service.broker.DecideApproval(ctx,
		service.approval.DecisionInputForResult(service.plan.UserID, prepared))
	if err != nil || approved.State != agentbroker.IntentApproved || approved.IntentID != prepared.IntentID ||
		approved.IntentFingerprint != prepared.IntentFingerprint || approved.IdempotencyKey != prepared.IdempotencyKey ||
		approved.ApprovalClass != agentbroker.ApprovalPerCommit ||
		approved.ApprovalRevision != 1 || approved.ToolIdentity != binding.Plan.ToolIdentity ||
		approved.Resource != binding.Plan.Resource || approved.ArgumentsFingerprint != binding.ArgumentsFingerprint {
		return false, false, ErrUnavailable
	}
	cancelRequest, err := service.authorizedRequest(ctx, agentrunner.MethodCancel,
		agentrunner.CancelRequest{Attempt: attempt, Mode: "cancel", ReasonCode: "project_canary_complete"},
		binding.SnapshotFingerprint, attempt, service.now().UTC())
	if err != nil {
		return false, false, err
	}
	commit := agentrunner.CommitRequest{Attempt: attempt, SnapshotFingerprint: binding.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: binding.Registry.Fingerprint,
		IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint,
		ApprovalID: service.approval.Payload.ApprovalID, IdempotencyKey: prepared.IdempotencyKey}
	commitResponse, signedCommit, commitErr := service.authorizedCall(ctx, agentrunner.MethodCommit, commit,
		binding.SnapshotFingerprint, attempt, service.now().UTC())
	committed, ok := commitResponse.Body.(agentrunner.CommitResult)
	if !ok || (commitErr != nil && !errors.Is(commitErr, agentrunner.ErrOutcomeUnknown)) ||
		(committed.Outcome != "committed" && committed.Outcome != "replayed" && committed.Outcome != "outcome_unknown") {
		return false, false, ErrUnavailable
	}
	if err := service.assertReplay(ctx, signedCommit, commitResponse, commitErr); err != nil {
		return false, false, err
	}
	if committed.Outcome == "committed" || committed.Outcome == "replayed" {
		cleanupRequest := agentbroker.ExecutionRequest{UserID: service.plan.UserID,
			RunID: attempt.RunID, StepID: attempt.StepID, AttemptID: attempt.AttemptID,
			Generation: attempt.LeaseGeneration, SnapshotFingerprint: binding.SnapshotFingerprint,
			GrantFingerprint: grantFingerprint, RegistryFingerprint: binding.Registry.Fingerprint,
			IntentID: prepared.IntentID, IdempotencyKey: prepared.IdempotencyKey,
			ToolIdentity: binding.Plan.ToolIdentity, Capability: binding.Plan.Capability,
			Action: binding.Plan.Action, Resource: binding.Plan.Resource,
			Arguments:            append(json.RawMessage(nil), binding.Plan.Arguments...),
			ArgumentsFingerprint: binding.ArgumentsFingerprint, BaseRevision: binding.Plan.BaseRevision}
		restored, cleanupErr := service.cleanup.Cleanup(context.WithoutCancel(ctx), cleanupRequest,
			committed.ReceiptFingerprint)
		clear(cleanupRequest.Arguments)
		if cleanupErr != nil || restored != binding.Plan.BaseRevision {
			return false, true, ErrRecoveryPending
		}
	}
	cancelResponse, err := service.sendAuthorized(ctx, cancelRequest)
	if err != nil {
		return false, false, err
	}
	canceled, ok := cancelResponse.Body.(agentrunner.CancelResult)
	if !ok || !canceled.Accepted || canceled.ObservedTerminal != "canceled" {
		return false, false, ErrUnavailable
	}
	if service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID,
		attempt.LeaseGeneration, "running", "stopping", "") != nil ||
		service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID,
			attempt.LeaseGeneration, "stopping", "terminal", "canceled") != nil {
		return false, false, ErrUnavailable
	}
	if err := service.reconcileEmpty(ctx, probeFingerprint, service.now().UTC()); err != nil {
		return false, false, err
	}
	return true, true, nil
}

func (service *Service) authorizedCall(ctx context.Context, method string, body any, snapshot string,
	attempt agentrunner.AttemptRef, now time.Time,
) (agentrunner.Response, agentrunner.Request, error) {
	request, err := service.authorizedRequest(ctx, method, body, snapshot, attempt, now)
	if err != nil {
		return agentrunner.Response{}, agentrunner.Request{}, err
	}
	response, err := service.sendAuthorized(ctx, request)
	return response, request, err
}

func (service *Service) authorizedRequest(ctx context.Context, method string, body any, snapshot string,
	attempt agentrunner.AttemptRef, now time.Time,
) (agentrunner.Request, error) {
	unsigned, err := agentrunner.NewRequest(method, body, now)
	if err != nil {
		return agentrunner.Request{}, ErrUnavailable
	}
	fingerprint := agentrunner.AuthorityRequestFingerprint(method, body)
	ticket, replay, err := service.authority.IssueAuthority(ctx, agentrunner.IssueAuthorityInput{
		CallerIdentity: service.config.CallerIdentity, RunnerID: service.config.RunnerID,
		RequestID: unsigned.RequestID, Nonce: unsigned.Nonce, Method: method,
		RequestFingerprint: fingerprint, UserID: service.plan.UserID, Attempt: attempt,
		SnapshotFingerprint: snapshot, TTL: service.config.AuthorityTTL})
	if err != nil || len(replay) != 0 {
		return agentrunner.Request{}, ErrUnavailable
	}
	signed, err := agentrunner.BindAuthority(unsigned, ticket)
	if err != nil {
		return agentrunner.Request{}, ErrUnavailable
	}
	return signed, nil
}

func (service *Service) sendAuthorized(ctx context.Context, request agentrunner.Request) (agentrunner.Response, error) {
	response, operationErr := service.client.Call(ctx, request)
	if response.SchemaVersion != "" {
		encoded, err := json.Marshal(response)
		if err != nil || service.authority.CompleteRequest(ctx, service.config.CallerIdentity,
			request.RequestID, request.Nonce, agentrunner.AuthorityRequestFingerprint(request.Method, requestBody(request)), encoded) != nil {
			return agentrunner.Response{}, ErrUnavailable
		}
	}
	if operationErr != nil {
		return response, operationErr
	}
	return response, nil
}

func (service *Service) assertReplay(ctx context.Context, request agentrunner.Request,
	expected agentrunner.Response, expectedErr error,
) error {
	replayed, err := service.client.Call(ctx, request)
	encodedExpected, expectedMarshalErr := json.Marshal(expected)
	encodedReplay, replayMarshalErr := json.Marshal(replayed)
	if expectedMarshalErr != nil || replayMarshalErr != nil || !bytes.Equal(encodedExpected, encodedReplay) ||
		!sameOperationError(err, expectedErr) {
		return ErrUnavailable
	}
	return nil
}

func requestBody(request agentrunner.Request) any {
	switch request.Method {
	case agentrunner.MethodLaunch:
		return *request.Launch
	case agentrunner.MethodHeartbeat:
		return *request.Heartbeat
	case agentrunner.MethodCancel:
		return *request.Cancel
	case agentrunner.MethodPrepare:
		return *request.Prepare
	case agentrunner.MethodCommit:
		return *request.Commit
	default:
		return nil
	}
}

func sameOperationError(left, right error) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return agentrunner.ErrorCode(left) == agentrunner.ErrorCode(right)
}

func (service *Service) probe(ctx context.Context, now time.Time) (agentrunner.ProbeResult, error) {
	request, err := agentrunner.NewRequest(agentrunner.MethodProbe,
		agentrunner.ProbeRequest{RequiredFeatures: append([]string(nil), canaryRequiredFeatures...)}, now)
	if err != nil {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	response, err := service.client.Call(ctx, request)
	result, ok := response.Body.(agentrunner.ProbeResult)
	if err != nil || !ok || !result.Ready || result.Error != nil || result.ProbeFingerprint == "" {
		return agentrunner.ProbeResult{}, ErrUnavailable
	}
	return result, nil
}

func (service *Service) reconcileEmpty(ctx context.Context, probeFingerprint string, now time.Time) error {
	items, err := service.runnerState.RecoverySandboxes(ctx, service.config.BatchSize)
	if err != nil {
		return ErrUnavailable
	}
	expected := make([]agentrunner.SandboxDescriptor, 0, len(items))
	for _, item := range items {
		if item.RunnerID != service.config.RunnerID || item.LeaseExpired || !now.Before(item.LeaseExpiresAt) ||
			item.ProbeFingerprint != probeFingerprint || agentorchestrator.IsAttemptTerminal(item.AttemptState) {
			continue
		}
		expected = append(expected, agentrunner.SandboxDescriptor{SandboxID: item.SandboxID,
			Attempt: item.Attempt, SnapshotFingerprint: item.SnapshotFingerprint,
			SpecFingerprint: item.SpecFingerprint, ProbeFingerprint: item.ProbeFingerprint,
			State: item.SandboxState, UpdatedAt: now})
	}
	reconcile, err := agentrunner.NewRequest(agentrunner.MethodReconcile,
		agentrunner.ReconcileRequest{RunnerID: service.config.RunnerID, Expected: expected}, now)
	if err != nil {
		return ErrUnavailable
	}
	if _, err := service.client.Call(ctx, reconcile); err != nil {
		return ErrUnavailable
	}
	list, err := agentrunner.NewRequest(agentrunner.MethodList,
		agentrunner.ListRequest{RunnerID: service.config.RunnerID}, service.now().UTC())
	if err != nil {
		return ErrUnavailable
	}
	response, err := service.client.Call(ctx, list)
	result, ok := response.Body.(agentrunner.ListResult)
	if err != nil || !ok || result.RunnerID != service.config.RunnerID || len(result.Sandboxes) != len(expected) {
		return ErrUnavailable
	}
	return nil
}

func canaryWait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
