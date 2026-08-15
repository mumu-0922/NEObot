package agentchildcanary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentdelegation"
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
	GetRun(context.Context, string, string) (agentorchestrator.Run, error)
	AcquireStep(context.Context, agentorchestrator.AcquireInput) (agentorchestrator.Lease, error)
	TransitionAttempt(context.Context, agentorchestrator.TransitionInput) error
	HeartbeatAttempt(context.Context, agentorchestrator.HeartbeatInput) (time.Time, error)
}

type Delegation interface {
	GetAuthority(context.Context, string, string) (agentdelegation.Authority, error)
	ListChildren(context.Context, string, string) ([]agentdelegation.ChildLineage, error)
	PendingReaps(context.Context, int) ([]agentdelegation.ReapTarget, error)
	RegisterRoot(context.Context, agentdelegation.RegisterRootInput) (agentdelegation.Authority, bool, error)
	EnqueueChild(context.Context, agentdelegation.ChildProposal) (agentdelegation.EnqueueResult, error)
	AdmitLaunch(context.Context, agentdelegation.LaunchAdmissionInput) error
	Settle(context.Context, agentdelegation.SettleInput) (bool, error)
	Cascade(context.Context, agentdelegation.CascadeInput) ([]agentdelegation.ReapTarget, error)
	Reconcile(context.Context, int) (int, error)
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

type StateRepository interface {
	ResolveParent(context.Context, string, string) (string, error)
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
	delegation   Delegation
	authority    AuthorityService
	runnerState  RunnerRepository
	client       RunnerClient
	terminal     ParentTerminalRepository
	state        StateRepository
	now          func() time.Time
	completedRun string
}

type Result struct {
	ParentRunID string
	ChildRunID  string
	Completed   bool
}

type activeSandbox struct {
	run       agentorchestrator.Run
	lease     agentorchestrator.Lease
	attempt   agentrunner.AttemptRef
	sandboxID string
}

func NewService(config Config, plan Plan, gate Gate, orchestrator Orchestrator,
	delegation Delegation, authority AuthorityService, runnerState RunnerRepository,
	client RunnerClient, terminal ParentTerminalRepository, state StateRepository,
) (*Service, error) {
	if config.RunnerID == "" || config.CallerIdentity != "spiffe://neo-chat/agent-runtime-child-canary" ||
		config.PollInterval < time.Second || config.PollInterval > time.Minute ||
		config.AuthorityTTL < time.Second || config.AuthorityTTL > 15*time.Second ||
		config.BatchSize < 1 || config.BatchSize > 1000 || ValidatePlan(plan) != nil || gate == nil ||
		orchestrator == nil || delegation == nil || authority == nil || runnerState == nil ||
		client == nil || terminal == nil || state == nil {
		return nil, ErrInvalidPlan
	}
	return &Service{config: config, plan: plan, gate: gate, orchestrator: orchestrator,
		delegation: delegation, authority: authority, runnerState: runnerState,
		client: client, terminal: terminal, state: state, now: time.Now}, nil
}

func (service *Service) Run(ctx context.Context) error {
	for {
		result, err := service.Cycle(ctx)
		if err == nil && result.Completed {
			service.completedRun = result.ParentRunID
			break
		}
		if err != nil && !errors.Is(err, ErrRecoveryPending) {
			return err
		}
		if err := waitContext(ctx, service.config.PollInterval); err != nil {
			return err
		}
	}
	for {
		if err := waitContext(ctx, service.config.PollInterval); err != nil {
			return err
		}
		if err := service.Health(ctx); err != nil {
			return err
		}
	}
}

func (service *Service) Cycle(ctx context.Context) (Result, error) {
	now := service.now().UTC()
	if service.gate.Verify(now) != nil {
		return Result{}, ErrUnavailable
	}
	if _, err := service.delegation.Reconcile(ctx, service.config.BatchSize); err != nil {
		return Result{}, ErrUnavailable
	}
	probe, err := service.probe(ctx, now)
	if err != nil {
		return Result{}, err
	}
	snapshot, err := parentSnapshot(service.plan)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	enqueued, err := service.orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{
		UserID: service.plan.UserID, IdempotencyKey: service.plan.ParentIdempotencyKey,
		Snapshot: snapshot, Steps: []agentorchestrator.StepPlan{{Kind: service.plan.ParentStepKind}},
		ScopeBindings: []agentorchestrator.ScopeBinding{{Type: "runner", Value: service.config.RunnerID},
			{Type: "skill", Value: service.plan.PackageFingerprint}},
	})
	if err != nil || len(enqueued.Run.Steps) != 1 {
		return Result{}, ErrUnavailable
	}
	children, err := service.delegation.ListChildren(ctx, service.plan.UserID, enqueued.Run.ID)
	if err != nil || len(children) > 1 || len(children) == 1 && children[0].IdempotencyKey != service.plan.ChildIdempotencyKey {
		return Result{}, ErrUnavailable
	}
	if len(children) == 1 {
		return service.recoverExisting(ctx, enqueued.Run, children[0], probe)
	}
	if agentorchestrator.IsRunTerminal(enqueued.Run.State) {
		return Result{}, ErrUnavailable
	}
	if liveAttempt(enqueued.Run, now) {
		return Result{ParentRunID: enqueued.Run.ID}, ErrRecoveryPending
	}
	if err := service.reconcileExcluding(ctx, probe.ProbeFingerprint, map[string]struct{}{enqueued.Run.ID: {}}); err != nil {
		return Result{}, err
	}
	rootGrant, rootRegistry, err := service.plan.RootAuthority(enqueued.Run.ID, now)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	rootAuthority, authorityErr := service.delegation.GetAuthority(ctx, service.plan.UserID, enqueued.Run.ID)
	hasRootAuthority := authorityErr == nil
	if authorityErr != nil && !errors.Is(authorityErr, agentdelegation.ErrNotFound) {
		return Result{}, ErrUnavailable
	}
	if hasRootAuthority {
		if !service.validRootAuthority(rootAuthority, enqueued.Run.ID, now) {
			return Result{}, ErrUnavailable
		}
		rootGrant, rootRegistry = rootAuthority.Grant, rootAuthority.Registry
	}
	parent, err := service.launch(ctx, enqueued.Run, rootGrant, rootRegistry, service.plan.ParentSandbox,
		service.plan.ParentArgv, agentrunner.RunLineage{RootRunID: enqueued.Run.ID, Depth: 0},
		service.plan.ParentLeaseSeconds, false, probe)
	if err != nil {
		return Result{}, err
	}
	if !hasRootAuthority {
		rootAuthority, _, err = service.delegation.RegisterRoot(ctx, agentdelegation.RegisterRootInput{
			UserID: service.plan.UserID, RunID: enqueued.Run.ID, Grant: rootGrant,
			Registry: rootRegistry, Model: DelegationModel(service.plan.Model),
		})
		if err != nil {
			_ = service.cancelParent(ctx, parent, rootGrant, rootRegistry, probe.ProbeFingerprint)
			return Result{}, ErrUnavailable
		}
	}
	proposal := agentdelegation.ChildProposal{UserID: service.plan.UserID,
		IdempotencyKey: service.plan.ChildIdempotencyKey, ParentRunID: enqueued.Run.ID,
		ParentAttempt: agentdelegation.ParentAttempt{StepID: parent.lease.StepID,
			AttemptID: parent.lease.ID, Generation: parent.lease.Generation,
			LeaseOwner: parent.lease.LeaseOwner, LeaseToken: parent.lease.Token},
		Model: DelegationModel(service.plan.Model), Grant: service.plan.ChildGrantTemplate(now),
		RequestedTools: append([]string(nil), service.plan.ChildRequestedTools...),
		Catalog:        service.plan.catalog(), Steps: []agentorchestrator.StepPlan{{Kind: service.plan.ChildStepKind}}}
	childResult, err := service.delegation.EnqueueChild(ctx, proposal)
	if err != nil || !childResult.Created || childResult.Authority.Depth != 1 ||
		childResult.Authority.ParentRunID != rootAuthority.RunID || len(childResult.Authority.Registry.Tools) != 0 {
		return Result{}, ErrUnavailable
	}
	childRun, err := service.orchestrator.GetRun(ctx, service.plan.UserID, childResult.Authority.RunID)
	if err != nil || len(childRun.Steps) != 1 {
		return Result{}, ErrUnavailable
	}
	child, err := service.launch(ctx, childRun, childResult.Authority.Grant,
		childResult.Authority.Registry, service.plan.ChildSandbox, service.plan.ChildArgv,
		agentrunner.RunLineage{RootRunID: enqueued.Run.ID, ParentRunID: enqueued.Run.ID, Depth: 1},
		service.plan.ChildLeaseSeconds, true, probe)
	if err != nil {
		return Result{}, err
	}
	if _, err := service.delegation.Cascade(ctx, agentdelegation.CascadeInput{UserID: service.plan.UserID,
		ParentRunID: enqueued.Run.ID, Mode: "cancel", ActorType: "orchestrator",
		ActorID: "g21.4-child-canary", ReasonCode: "CHILD_CANARY_COMPLETE"}); err != nil {
		return Result{}, ErrUnavailable
	}
	if _, err := service.delegation.Settle(ctx, agentdelegation.SettleInput{UserID: service.plan.UserID,
		ChildRunID: child.run.ID, Usage: agentbroker.Budget{}, Outcome: "canceled"}); err != nil {
		return Result{}, ErrUnavailable
	}
	if err := service.cancelParent(ctx, parent, rootGrant, rootRegistry, probe.ProbeFingerprint); err != nil {
		return Result{}, err
	}
	service.completedRun = enqueued.Run.ID
	return Result{ParentRunID: enqueued.Run.ID, ChildRunID: child.run.ID, Completed: true}, nil
}

func (service *Service) recoverExisting(ctx context.Context, parentRun agentorchestrator.Run,
	lineage agentdelegation.ChildLineage, probe agentrunner.ProbeResult,
) (Result, error) {
	childRun, err := service.orchestrator.GetRun(ctx, service.plan.UserID, lineage.ChildRunID)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	if !agentorchestrator.IsRunTerminal(childRun.State) {
		now := service.now().UTC()
		if liveAttempt(childRun, now) {
			return Result{ParentRunID: parentRun.ID, ChildRunID: childRun.ID}, ErrRecoveryPending
		}
		if len(childRun.Attempts) == 0 && lineageParentLive(parentRun, lineage, now) {
			authority, authorityErr := service.delegation.GetAuthority(ctx, service.plan.UserID, childRun.ID)
			if authorityErr != nil || authority.Depth != 1 || authority.ParentRunID != parentRun.ID ||
				authority.RootRunID != parentRun.ID || len(authority.Registry.Tools) != 0 {
				return Result{}, ErrUnavailable
			}
			if _, launchErr := service.launch(ctx, childRun, authority.Grant, authority.Registry,
				service.plan.ChildSandbox, service.plan.ChildArgv,
				agentrunner.RunLineage{RootRunID: parentRun.ID, ParentRunID: parentRun.ID, Depth: 1},
				service.plan.ChildLeaseSeconds, true, probe); launchErr != nil {
				return Result{}, launchErr
			}
		}
		if _, cascadeErr := service.delegation.Cascade(ctx, agentdelegation.CascadeInput{
			UserID: service.plan.UserID, ParentRunID: parentRun.ID, Mode: "cancel",
			ActorType: "orchestrator", ActorID: "g21.4-child-canary",
			ReasonCode: "CHILD_CANARY_RECOVERY",
		}); cascadeErr != nil {
			return Result{}, ErrUnavailable
		}
		if _, settleErr := service.delegation.Settle(ctx, agentdelegation.SettleInput{
			UserID: service.plan.UserID, ChildRunID: childRun.ID,
			Usage: agentbroker.Budget{}, Outcome: "canceled",
		}); settleErr != nil {
			return Result{}, ErrUnavailable
		}
		childRun, err = service.orchestrator.GetRun(ctx, service.plan.UserID, childRun.ID)
		if err != nil || childRun.State != agentorchestrator.RunCanceled {
			return Result{}, ErrUnavailable
		}
	}
	if agentorchestrator.IsRunTerminal(parentRun.State) {
		if parentRun.State != agentorchestrator.RunCanceled ||
			service.ensureRunsAbsent(ctx, parentRun.ID, childRun.ID) != nil {
			return Result{}, ErrUnavailable
		}
		return Result{ParentRunID: parentRun.ID, ChildRunID: childRun.ID, Completed: true}, nil
	}
	if liveAttempt(parentRun, service.now().UTC()) {
		return Result{ParentRunID: parentRun.ID, ChildRunID: childRun.ID}, ErrRecoveryPending
	}
	authority, err := service.delegation.GetAuthority(ctx, service.plan.UserID, parentRun.ID)
	if err != nil || !service.validRootAuthority(authority, parentRun.ID, authority.Grant.IssuedAt.Add(time.Second)) {
		return Result{}, ErrUnavailable
	}
	if err := service.reconcileExcluding(ctx, probe.ProbeFingerprint,
		map[string]struct{}{parentRun.ID: {}, childRun.ID: {}}); err != nil {
		return Result{}, err
	}
	refreshed, err := service.orchestrator.GetRun(ctx, service.plan.UserID, parentRun.ID)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	parent, err := service.launch(ctx, refreshed, authority.Grant, authority.Registry,
		service.plan.ParentSandbox, service.plan.ParentArgv,
		agentrunner.RunLineage{RootRunID: refreshed.ID, Depth: 0}, service.plan.ParentLeaseSeconds, false, probe)
	if err != nil {
		return Result{}, err
	}
	if err := service.cancelParent(ctx, parent, authority.Grant, authority.Registry, probe.ProbeFingerprint); err != nil {
		return Result{}, err
	}
	service.completedRun = refreshed.ID
	return Result{ParentRunID: refreshed.ID, ChildRunID: childRun.ID, Completed: true}, nil
}

func (service *Service) validRootAuthority(authority agentdelegation.Authority, runID string,
	validationTime time.Time,
) bool {
	if authority.RunID != runID || authority.RootRunID != runID || authority.ParentRunID != "" ||
		authority.Depth != 0 || authority.UserID != service.plan.UserID || authority.State != "active" ||
		authority.Model != DelegationModel(service.plan.Model) {
		return false
	}
	expectedGrant, expectedRegistry, err := service.plan.RootAuthority(runID, authority.Grant.IssuedAt.Add(time.Second))
	if err != nil {
		return false
	}
	expectedGrantFingerprint, err := agentbroker.GrantFingerprint(expectedGrant)
	if err != nil {
		return false
	}
	return authority.GrantFingerprint == expectedGrantFingerprint &&
		authority.RegistryFingerprint == expectedRegistry.Fingerprint &&
		authority.Grant.GrantID == service.plan.ParentGrantID &&
		authority.Registry.RunID == runID && validationTime.UTC().Before(authority.ExpiresAt)
}

func (service *Service) launch(ctx context.Context, run agentorchestrator.Run,
	grant agentbroker.CapabilityGrant, registry agentbroker.ToolRegistry,
	sandbox agentrunner.SandboxSpec, argv []string, lineage agentrunner.RunLineage,
	leaseSeconds int, child bool, probe agentrunner.ProbeResult,
) (activeSandbox, error) {
	if len(run.Steps) != 1 {
		return activeSandbox{}, ErrUnavailable
	}
	actor := agentorchestrator.Actor{Type: "orchestrator", ID: "g21.4-child-canary"}
	lease, err := service.orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: service.plan.UserID,
		RunID: run.ID, StepID: run.Steps[0].ID, LeaseOwner: service.config.RunnerID,
		LeaseDuration: time.Duration(leaseSeconds) * time.Second, Actor: actor, ReasonCode: "CHILD_CANARY_CLAIMED"})
	if err != nil {
		return activeSandbox{}, ErrUnavailable
	}
	transition := agentorchestrator.TransitionInput{UserID: service.plan.UserID, RunID: run.ID,
		StepID: lease.StepID, AttemptID: lease.ID, Generation: lease.Generation,
		LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token, Actor: actor,
		Expected: agentorchestrator.AttemptLeased, To: agentorchestrator.AttemptStarting,
		ReasonCode: "CHILD_CANARY_STARTING"}
	if service.orchestrator.TransitionAttempt(ctx, transition) != nil {
		return activeSandbox{}, ErrUnavailable
	}
	attempt := agentrunner.AttemptRef{RunID: run.ID, StepID: lease.StepID, AttemptID: lease.ID,
		LeaseGeneration: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token}
	if child {
		tools := make([]string, 0, len(registry.Tools))
		for _, tool := range registry.Tools {
			tools = append(tools, tool.Identity)
		}
		if err := service.delegation.AdmitLaunch(ctx, agentdelegation.LaunchAdmissionInput{UserID: service.plan.UserID,
			RunID: run.ID, AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
			LeaseToken: lease.Token, SnapshotFingerprint: run.SnapshotFingerprint,
			GrantFingerprint: mustGrantFingerprint(grant), RegistryFingerprint: registry.Fingerprint,
			RegistryTools: tools}); err != nil {
			return activeSandbox{}, ErrUnavailable
		}
	}
	launch := agentrunner.LaunchRequest{Attempt: attempt, Lineage: lineage, GrantID: grant.GrantID,
		GrantFingerprint: mustGrantFingerprint(grant), SnapshotFingerprint: run.SnapshotFingerprint,
		Sandbox: sandbox, ToolRegistry: runnerRegistry(registry), Argv: append([]string(nil), argv...)}
	response, request, err := service.authorizedCall(ctx, agentrunner.MethodLaunch, launch,
		run.SnapshotFingerprint, attempt, service.now().UTC())
	if err != nil {
		return activeSandbox{}, err
	}
	launchResult, ok := response.Body.(agentrunner.LaunchResult)
	if !ok || !launchResult.Accepted || launchResult.SandboxID == "" || launchResult.Attempt != attempt.Identity() {
		return activeSandbox{}, ErrUnavailable
	}
	expected := agentrunner.ExpectedSandbox{SandboxID: launchResult.SandboxID,
		CallerIdentity: service.config.CallerIdentity, RequestID: request.RequestID, Nonce: request.Nonce,
		UserID: service.plan.UserID, Attempt: attempt.Identity(), RunnerID: service.config.RunnerID,
		SnapshotFingerprint: run.SnapshotFingerprint, SpecFingerprint: agentrunner.SandboxFingerprint(launch),
		ProbeFingerprint: probe.ProbeFingerprint}
	if service.runnerState.ExpectSandbox(ctx, expected) != nil ||
		service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID, attempt.LeaseGeneration,
			"expected", "starting", "") != nil ||
		service.runnerState.UpdateSandbox(ctx, expected.SandboxID, attempt.AttemptID, attempt.LeaseGeneration,
			"starting", "running", "") != nil {
		return activeSandbox{}, ErrUnavailable
	}
	transition.Expected, transition.To, transition.ReasonCode = agentorchestrator.AttemptStarting,
		agentorchestrator.AttemptRunning, "CHILD_CANARY_RUNNING"
	if service.orchestrator.TransitionAttempt(ctx, transition) != nil {
		return activeSandbox{}, ErrUnavailable
	}
	heartbeat := agentrunner.HeartbeatRequest{Attempt: attempt, ObservedState: "running",
		LastEventSequence: run.NextSequence}
	if _, _, err := service.authorizedCall(ctx, agentrunner.MethodHeartbeat, heartbeat,
		run.SnapshotFingerprint, attempt, service.now().UTC()); err != nil {
		return activeSandbox{}, err
	}
	if _, err := service.orchestrator.HeartbeatAttempt(ctx, agentorchestrator.HeartbeatInput{
		UserID: service.plan.UserID, RunID: run.ID, StepID: lease.StepID, AttemptID: lease.ID,
		Generation: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token,
		LeaseDuration: time.Duration(leaseSeconds) * time.Second, Actor: actor,
		ReasonCode: "CHILD_CANARY_HEARTBEAT"}); err != nil {
		return activeSandbox{}, ErrUnavailable
	}
	return activeSandbox{run: run, lease: lease, attempt: attempt, sandboxID: launchResult.SandboxID}, nil
}

func (service *Service) cancelParent(ctx context.Context, parent activeSandbox,
	grant agentbroker.CapabilityGrant, registry agentbroker.ToolRegistry, probeFingerprint string,
) error {
	_ = grant
	_ = registry
	cancel := agentrunner.CancelRequest{Attempt: parent.attempt, Mode: "cancel", ReasonCode: "child_canary_complete"}
	response, _, err := service.authorizedCall(ctx, agentrunner.MethodCancel, cancel,
		parent.run.SnapshotFingerprint, parent.attempt, service.now().UTC())
	result, ok := response.Body.(agentrunner.CancelResult)
	if err != nil || !ok || !result.Accepted || result.ObservedTerminal != "canceled" {
		return ErrUnavailable
	}
	if err := service.terminal.FinalizeParentCanceled(ctx, ParentTerminalInput{UserID: service.plan.UserID,
		RunID: parent.run.ID, StepID: parent.lease.StepID, AttemptID: parent.lease.ID,
		Generation: parent.lease.Generation, LeaseOwner: parent.lease.LeaseOwner,
		LeaseToken: parent.lease.Token, SandboxID: parent.sandboxID}); err != nil {
		return ErrUnavailable
	}
	return service.ensureRunsAbsent(ctx, parent.run.ID)
}

func (service *Service) authorizedCall(ctx context.Context, method string, body any, snapshot string,
	attempt agentrunner.AttemptRef, now time.Time,
) (agentrunner.Response, agentrunner.Request, error) {
	unsigned, err := agentrunner.NewRequest(method, body, now)
	if err != nil {
		return agentrunner.Response{}, agentrunner.Request{}, ErrUnavailable
	}
	fingerprint := agentrunner.AuthorityRequestFingerprint(method, body)
	ticket, replay, err := service.authority.IssueAuthority(ctx, agentrunner.IssueAuthorityInput{
		CallerIdentity: service.config.CallerIdentity, RunnerID: service.config.RunnerID,
		RequestID: unsigned.RequestID, Nonce: unsigned.Nonce, Method: method,
		RequestFingerprint: fingerprint, UserID: service.plan.UserID, Attempt: attempt,
		SnapshotFingerprint: snapshot, TTL: service.config.AuthorityTTL})
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

func (service *Service) reconcileExcluding(ctx context.Context, probeFingerprint string,
	excludedRuns map[string]struct{},
) error {
	items, err := service.runnerState.RecoverySandboxes(ctx, service.config.BatchSize)
	if err != nil {
		return ErrUnavailable
	}
	now := service.now().UTC()
	expected := make([]agentrunner.SandboxDescriptor, 0, len(items))
	for _, item := range items {
		if _, excluded := excludedRuns[item.Attempt.RunID]; excluded || item.RunnerID != service.config.RunnerID ||
			item.LeaseExpired || !now.Before(item.LeaseExpiresAt) || item.ProbeFingerprint != probeFingerprint {
			continue
		}
		expected = append(expected, agentrunner.SandboxDescriptor{SandboxID: item.SandboxID,
			Attempt: item.Attempt, SnapshotFingerprint: item.SnapshotFingerprint,
			SpecFingerprint: item.SpecFingerprint, ProbeFingerprint: item.ProbeFingerprint,
			State: item.SandboxState, UpdatedAt: now})
	}
	request, err := agentrunner.NewRequest(agentrunner.MethodReconcile,
		agentrunner.ReconcileRequest{RunnerID: service.config.RunnerID, Expected: expected}, now)
	if err != nil {
		return ErrUnavailable
	}
	if _, err := service.client.Call(ctx, request); err != nil {
		return ErrUnavailable
	}
	return nil
}

func (service *Service) ensureRunsAbsent(ctx context.Context, runIDs ...string) error {
	request, err := agentrunner.NewRequest(agentrunner.MethodList,
		agentrunner.ListRequest{RunnerID: service.config.RunnerID}, service.now().UTC())
	if err != nil {
		return ErrUnavailable
	}
	response, err := service.client.Call(ctx, request)
	result, ok := response.Body.(agentrunner.ListResult)
	if err != nil || !ok || result.RunnerID != service.config.RunnerID {
		return ErrUnavailable
	}
	denied := make(map[string]struct{}, len(runIDs))
	for _, runID := range runIDs {
		denied[runID] = struct{}{}
	}
	for _, descriptor := range result.Sandboxes {
		if _, found := denied[descriptor.Attempt.RunID]; found {
			return ErrUnavailable
		}
	}
	return nil
}

func (service *Service) Health(ctx context.Context) error {
	if service.gate.Verify(service.now().UTC()) != nil {
		return ErrUnavailable
	}
	if service.completedRun == "" {
		runID, err := service.state.ResolveParent(ctx, service.plan.UserID, service.plan.ParentIdempotencyKey)
		if err != nil {
			return ErrUnavailable
		}
		service.completedRun = runID
	}
	if _, err := service.delegation.Reconcile(ctx, service.config.BatchSize); err != nil {
		return ErrUnavailable
	}
	pending, err := service.delegation.PendingReaps(ctx, service.config.BatchSize)
	if err != nil || len(pending) != 0 {
		return ErrUnavailable
	}
	parent, err := service.orchestrator.GetRun(ctx, service.plan.UserID, service.completedRun)
	if err != nil || parent.State != agentorchestrator.RunCanceled {
		return ErrUnavailable
	}
	children, err := service.delegation.ListChildren(ctx, service.plan.UserID, parent.ID)
	if err != nil || len(children) != 1 || children[0].IdempotencyKey != service.plan.ChildIdempotencyKey {
		return ErrUnavailable
	}
	child, err := service.orchestrator.GetRun(ctx, service.plan.UserID, children[0].ChildRunID)
	if err != nil || child.State != agentorchestrator.RunCanceled {
		return ErrUnavailable
	}
	return service.ensureRunsAbsent(ctx, parent.ID, child.ID)
}

func parentSnapshot(plan Plan) ([]byte, error) {
	return json.Marshal(struct {
		SchemaVersion            string             `json:"schemaVersion"`
		Mode                     string             `json:"mode"`
		Model                    Model              `json:"model"`
		PackageFingerprint       string             `json:"packageFingerprint"`
		RuntimeBundleFingerprint string             `json:"runtimeBundleFingerprint"`
		Budget                   agentbroker.Budget `json:"budget"`
		DelegationResource       string             `json:"delegationResource"`
		NoEgress                 bool               `json:"noEgress"`
		NoSecrets                bool               `json:"noSecrets"`
	}{"neo.agent-snapshot/v1", "synthetic-child-canary", plan.Model, plan.PackageFingerprint,
		plan.RuntimeBundleFingerprint, plan.ParentBudget, plan.DelegationResource, true, true})
}

func mustGrantFingerprint(grant agentbroker.CapabilityGrant) string {
	fingerprint, _ := agentbroker.GrantFingerprint(grant)
	return fingerprint
}

func liveAttempt(run agentorchestrator.Run, now time.Time) bool {
	if len(run.Attempts) == 0 {
		return false
	}
	latest := run.Attempts[len(run.Attempts)-1]
	return !agentorchestrator.IsAttemptTerminal(latest.State) && now.Before(latest.LeaseExpiresAt)
}

func lineageParentLive(parent agentorchestrator.Run, lineage agentdelegation.ChildLineage,
	now time.Time,
) bool {
	for index := len(parent.Attempts) - 1; index >= 0; index-- {
		attempt := parent.Attempts[index]
		if attempt.ID == lineage.ParentAttemptID && attempt.Generation == lineage.ParentGeneration {
			return !agentorchestrator.IsAttemptTerminal(attempt.State) && now.Before(attempt.LeaseExpiresAt)
		}
	}
	return false
}
