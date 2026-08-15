package agentrootcanary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

var requiredFeatures = []string{
	"rootless_userns", "cgroup_v2", "seccomp", "readonly_rootfs",
	"snapshot_workspace", "network_none", "pidfd_kill", "cgroup_reap",
}

type Gate interface{ Verify(time.Time) error }

type Orchestrator interface {
	EnqueueRun(context.Context, agentorchestrator.EnqueueInput) (agentorchestrator.EnqueueResult, error)
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

type TerminalRepository interface {
	FinalizeCanceled(context.Context, TerminalInput) error
}

type TerminalInput struct {
	UserID, RunID, StepID, AttemptID string
	Generation                       int64
	LeaseOwner, LeaseToken           string
	SandboxID                        string
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
	gate         Gate
	orchestrator Orchestrator
	authority    AuthorityService
	runnerState  RunnerRepository
	client       RunnerClient
	terminal     TerminalRepository
	now          func() time.Time
}

type Result struct {
	RunID, AttemptID    string
	Generation          int64
	SnapshotFingerprint string
	Completed           bool
}

func NewService(config Config, plan Plan, gate Gate, orchestrator Orchestrator,
	authority AuthorityService, runnerState RunnerRepository, client RunnerClient,
	terminal TerminalRepository) (*Service, error) {
	validPlan := (config.CallerIdentity == RootCallerIdentity && ValidatePlan(plan) == nil) ||
		(config.CallerIdentity == ProductCallerIdentity && ValidateProductPlan(plan) == nil)
	if config.RunnerID == "" || !validPlan ||
		config.PollInterval < time.Second || config.PollInterval > time.Minute ||
		config.AuthorityTTL < time.Second || config.AuthorityTTL > 15*time.Second ||
		config.BatchSize < 1 || config.BatchSize > 1000 || gate == nil ||
		orchestrator == nil || authority == nil || runnerState == nil || client == nil || terminal == nil {
		return nil, ErrInvalidPlan
	}
	return &Service{config: config, plan: plan, gate: gate, orchestrator: orchestrator,
		authority: authority, runnerState: runnerState, client: client, terminal: terminal, now: time.Now}, nil
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
		if err := wait(ctx, service.config.PollInterval); err != nil {
			return err
		}
	}
	for {
		if err := wait(ctx, service.config.PollInterval); err != nil {
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
	_ = probe
	return service.listEmpty(ctx, now)
}

func (service *Service) Cycle(ctx context.Context) (Result, error) {
	now := service.now().UTC()
	if err := service.gate.Verify(now); err != nil {
		return Result{}, ErrUnavailable
	}
	probe, err := service.probe(ctx, now)
	if err != nil {
		return Result{}, err
	}
	snapshot, _ := json.Marshal(service.plan.Snapshot)
	enqueued, err := service.orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{
		UserID: service.plan.UserID, IdempotencyKey: service.plan.IdempotencyKey,
		Snapshot: snapshot, Steps: []agentorchestrator.StepPlan{{Kind: service.plan.StepKind}},
		ScopeBindings: []agentorchestrator.ScopeBinding{{Type: "runner", Value: service.config.RunnerID},
			{Type: "skill", Value: service.plan.Sandbox.PackageFingerprint}},
	})
	if err != nil || len(enqueued.Run.Steps) != 1 {
		return Result{}, ErrUnavailable
	}
	if agentorchestrator.IsRunTerminal(enqueued.Run.State) {
		if enqueued.Run.State != agentorchestrator.RunCanceled || service.reconcileEmpty(ctx, probe.ProbeFingerprint, now) != nil {
			return Result{}, ErrUnavailable
		}
		result := Result{RunID: enqueued.Run.ID,
			SnapshotFingerprint: enqueued.Run.SnapshotFingerprint, Completed: true}
		if len(enqueued.Run.Attempts) > 0 {
			latest := enqueued.Run.Attempts[len(enqueued.Run.Attempts)-1]
			result.AttemptID, result.Generation = latest.ID, latest.Generation
		}
		return result, nil
	}
	if len(enqueued.Run.Attempts) > 0 {
		latest := enqueued.Run.Attempts[len(enqueued.Run.Attempts)-1]
		if !agentorchestrator.IsAttemptTerminal(latest.State) && now.Before(latest.LeaseExpiresAt) {
			return Result{RunID: enqueued.Run.ID, AttemptID: latest.ID, Generation: latest.Generation}, ErrRecoveryPending
		}
	}
	if err := service.reconcileEmpty(ctx, probe.ProbeFingerprint, now); err != nil {
		return Result{}, err
	}
	step := enqueued.Run.Steps[0]
	profile := service.profile()
	actor := agentorchestrator.Actor{Type: "orchestrator", ID: profile.actorID}
	leaseDuration := time.Duration(service.plan.LeaseSeconds) * time.Second
	lease, err := service.orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{
		UserID: service.plan.UserID, RunID: enqueued.Run.ID, StepID: step.ID,
		LeaseOwner: service.config.RunnerID, LeaseDuration: leaseDuration,
		Actor: actor, ReasonCode: profile.claimed,
	})
	if err != nil {
		return Result{}, ErrUnavailable
	}
	transition := agentorchestrator.TransitionInput{UserID: service.plan.UserID, RunID: lease.RunID,
		StepID: lease.StepID, AttemptID: lease.ID, Generation: lease.Generation,
		LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token, Actor: actor}
	transition.Expected, transition.To, transition.ReasonCode = agentorchestrator.AttemptLeased, agentorchestrator.AttemptStarting, profile.starting
	if err := service.orchestrator.TransitionAttempt(ctx, transition); err != nil {
		return Result{}, ErrUnavailable
	}
	attempt := agentrunner.AttemptRef{RunID: lease.RunID, StepID: lease.StepID, AttemptID: lease.ID,
		LeaseGeneration: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token}
	launch := agentrunner.LaunchRequest{Attempt: attempt,
		Lineage: agentrunner.RunLineage{RootRunID: lease.RunID, Depth: 0},
		GrantID: service.plan.GrantID, GrantFingerprint: service.plan.GrantFingerprint,
		SnapshotFingerprint: enqueued.Run.SnapshotFingerprint, Sandbox: service.plan.Sandbox,
		ToolRegistry: service.plan.ToolRegistry, Argv: append([]string(nil), service.plan.Argv...)}
	response, request, err := service.authorizedCall(ctx, agentrunner.MethodLaunch, launch,
		enqueued.Run.SnapshotFingerprint, attempt, now)
	if err != nil {
		return Result{}, err
	}
	launchResult, ok := response.Body.(agentrunner.LaunchResult)
	if !ok || !launchResult.Accepted || launchResult.SandboxID == "" || launchResult.Attempt != attempt.Identity() {
		return Result{}, ErrUnavailable
	}
	expected := agentrunner.ExpectedSandbox{SandboxID: launchResult.SandboxID,
		CallerIdentity: service.config.CallerIdentity, RequestID: request.RequestID, Nonce: request.Nonce,
		UserID: service.plan.UserID, Attempt: attempt.Identity(), RunnerID: service.config.RunnerID,
		SnapshotFingerprint: enqueued.Run.SnapshotFingerprint, SpecFingerprint: agentrunner.SandboxFingerprint(launch),
		ProbeFingerprint: probe.ProbeFingerprint}
	if service.runnerState.ExpectSandbox(ctx, expected) != nil ||
		service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID, attempt.LeaseGeneration, "expected", "starting", "") != nil ||
		service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID, attempt.LeaseGeneration, "starting", "running", "") != nil {
		_ = service.cancelOnly(ctx, enqueued.Run.SnapshotFingerprint, attempt)
		return Result{}, ErrUnavailable
	}
	transition.Expected, transition.To, transition.ReasonCode = agentorchestrator.AttemptStarting, agentorchestrator.AttemptRunning, profile.running
	if err := service.orchestrator.TransitionAttempt(ctx, transition); err != nil {
		_ = service.cancelOnly(ctx, enqueued.Run.SnapshotFingerprint, attempt)
		return Result{}, ErrUnavailable
	}
	heartbeat := agentrunner.HeartbeatRequest{Attempt: attempt, ObservedState: "running", LastEventSequence: enqueued.Run.NextSequence}
	if _, _, err := service.authorizedCall(ctx, agentrunner.MethodHeartbeat, heartbeat,
		enqueued.Run.SnapshotFingerprint, attempt, service.now().UTC()); err != nil {
		service.cancelRunning(ctx, enqueued.Run.SnapshotFingerprint, attempt, launchResult.SandboxID,
			TerminalInput{UserID: service.plan.UserID, RunID: lease.RunID, StepID: lease.StepID,
				AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
				LeaseToken: lease.Token, SandboxID: launchResult.SandboxID}, probe.ProbeFingerprint)
		return Result{}, err
	}
	if _, err := service.orchestrator.HeartbeatAttempt(ctx, agentorchestrator.HeartbeatInput{
		UserID: service.plan.UserID, RunID: lease.RunID, StepID: lease.StepID, AttemptID: lease.ID,
		Generation: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token,
		LeaseDuration: leaseDuration, Actor: actor, ReasonCode: profile.heartbeat,
	}); err != nil {
		service.cancelRunning(ctx, enqueued.Run.SnapshotFingerprint, attempt, launchResult.SandboxID,
			TerminalInput{UserID: service.plan.UserID, RunID: lease.RunID, StepID: lease.StepID,
				AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
				LeaseToken: lease.Token, SandboxID: launchResult.SandboxID}, probe.ProbeFingerprint)
		return Result{}, ErrUnavailable
	}
	terminal := TerminalInput{UserID: service.plan.UserID,
		RunID: lease.RunID, StepID: lease.StepID, AttemptID: lease.ID, Generation: lease.Generation,
		LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token, SandboxID: launchResult.SandboxID}
	if err := service.cancelRunning(ctx, enqueued.Run.SnapshotFingerprint, attempt,
		launchResult.SandboxID, terminal, probe.ProbeFingerprint); err != nil {
		return Result{}, err
	}
	return Result{RunID: lease.RunID, AttemptID: lease.ID, Generation: lease.Generation,
		SnapshotFingerprint: enqueued.Run.SnapshotFingerprint, Completed: true}, nil
}

func (service *Service) cancelRunning(ctx context.Context, snapshot string, attempt agentrunner.AttemptRef,
	sandboxID string, terminal TerminalInput, probeFingerprint string,
) error {
	if terminal.SandboxID != sandboxID {
		return ErrUnavailable
	}
	if err := service.cancelOnly(ctx, snapshot, attempt); err != nil {
		return err
	}
	if err := service.terminal.FinalizeCanceled(ctx, terminal); err != nil {
		return ErrUnavailable
	}
	return service.reconcileEmpty(ctx, probeFingerprint, service.now().UTC())
}

func (service *Service) cancelOnly(ctx context.Context, snapshot string, attempt agentrunner.AttemptRef) error {
	cancel := agentrunner.CancelRequest{Attempt: attempt, Mode: "cancel", ReasonCode: service.profile().cancel}
	response, _, err := service.authorizedCall(ctx, agentrunner.MethodCancel, cancel,
		snapshot, attempt, service.now().UTC())
	if err != nil {
		return err
	}
	result, ok := response.Body.(agentrunner.CancelResult)
	if !ok || !result.Accepted || result.ObservedTerminal != "canceled" {
		return ErrUnavailable
	}
	return nil
}

type executionProfile struct {
	actorID, claimed, starting, running, heartbeat, cancel string
}

func (service *Service) profile() executionProfile {
	if service.config.CallerIdentity == ProductCallerIdentity {
		return executionProfile{actorID: "g21.6-product-canary",
			claimed: "PRODUCT_CANARY_CLAIMED", starting: "PRODUCT_CANARY_STARTING",
			running: "PRODUCT_CANARY_RUNNING", heartbeat: "PRODUCT_CANARY_HEARTBEAT",
			cancel: "product_canary_complete"}
	}
	return executionProfile{actorID: "g21.1-root-canary",
		claimed: "ROOT_CANARY_CLAIMED", starting: "ROOT_CANARY_STARTING",
		running: "ROOT_CANARY_RUNNING", heartbeat: "ROOT_CANARY_HEARTBEAT",
		cancel: "root_canary_complete"}
}

func (service *Service) authorizedCall(ctx context.Context, method string, body any, snapshot string,
	attempt agentrunner.AttemptRef, now time.Time) (agentrunner.Response, agentrunner.Request, error) {
	unsigned, err := agentrunner.NewRequest(method, body, now)
	if err != nil {
		return agentrunner.Response{}, agentrunner.Request{}, ErrUnavailable
	}
	fingerprint := agentrunner.AuthorityRequestFingerprint(method, body)
	ticket, replay, err := service.authority.IssueAuthority(ctx, agentrunner.IssueAuthorityInput{
		CallerIdentity: service.config.CallerIdentity, RunnerID: service.config.RunnerID,
		RequestID: unsigned.RequestID, Nonce: unsigned.Nonce, Method: method,
		RequestFingerprint: fingerprint, UserID: service.plan.UserID, Attempt: attempt,
		SnapshotFingerprint: snapshot, TTL: service.config.AuthorityTTL,
	})
	if err != nil || len(replay) != 0 {
		return agentrunner.Response{}, agentrunner.Request{}, ErrUnavailable
	}
	signed, err := agentrunner.BindAuthority(unsigned, ticket)
	if err != nil {
		return agentrunner.Response{}, agentrunner.Request{}, ErrUnavailable
	}
	response, operationErr := service.client.Call(ctx, signed)
	if response.SchemaVersion != "" {
		encoded, marshalErr := json.Marshal(response)
		if marshalErr != nil || service.authority.CompleteRequest(ctx, service.config.CallerIdentity,
			signed.RequestID, signed.Nonce, fingerprint, encoded) != nil {
			return agentrunner.Response{}, signed, ErrUnavailable
		}
	}
	if operationErr != nil {
		return response, signed, fmt.Errorf("%w: %s", ErrUnavailable, agentrunner.ErrorCode(operationErr))
	}
	return response, signed, nil
}

func (service *Service) probe(ctx context.Context, now time.Time) (agentrunner.ProbeResult, error) {
	request, err := agentrunner.NewRequest(agentrunner.MethodProbe,
		agentrunner.ProbeRequest{RequiredFeatures: append([]string(nil), requiredFeatures...)}, now)
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
			item.ProbeFingerprint != probeFingerprint {
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
	return service.listEmpty(ctx, service.now().UTC())
}

func (service *Service) listEmpty(ctx context.Context, now time.Time) error {
	list, err := agentrunner.NewRequest(agentrunner.MethodList,
		agentrunner.ListRequest{RunnerID: service.config.RunnerID}, now)
	if err != nil {
		return ErrUnavailable
	}
	response, err := service.client.Call(ctx, list)
	result, ok := response.Body.(agentrunner.ListResult)
	if err != nil || !ok || result.RunnerID != service.config.RunnerID || len(result.Sandboxes) != 0 {
		return ErrUnavailable
	}
	return nil
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
