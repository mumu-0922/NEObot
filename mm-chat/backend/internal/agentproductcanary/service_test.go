package agentproductcanary

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrootcanary"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

const testUserID = "11111111-1111-4111-8111-111111111111"

func TestServiceClaimsExecutesAndCompletesExactRequest(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	plan := testProductPlan()
	request := testRequest(plan)
	repository := &fakeRepository{activation: testActivation(plan, now), requests: []Request{request}}
	executor := &fakeExecutor{result: ExecutionResult{RunID: "run_1234567890abcdef",
		AttemptID: "attempt_1234567890abcdef", SnapshotFingerprint: fp("9"),
		ReceiptFingerprint: fp("8")}}
	service, err := NewService(Config{ActivationID: plan.ActivationID,
		ClaimOwner: "product-canary-worker", PollInterval: time.Second,
		ClaimTTL: time.Minute, BatchSize: 10}, plan, fakeGate{}, repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	worked, err := service.Cycle(context.Background())
	if err != nil || !worked || repository.completed.ID != request.ID || executor.request.ID != request.ID {
		t.Fatalf("cycle worked=%v err=%v completed=%#v executed=%#v",
			worked, err, repository.completed, executor.request)
	}
}

func TestServiceTerminalizesBindingDriftWithoutExecution(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	plan := testProductPlan()
	request := testRequest(plan)
	request.PlanFingerprint = fp("d")
	repository := &fakeRepository{activation: testActivation(plan, now), requests: []Request{request}}
	executor := &fakeExecutor{}
	service, _ := NewService(Config{ActivationID: plan.ActivationID,
		ClaimOwner: "product-canary-worker", PollInterval: time.Second,
		ClaimTTL: time.Minute, BatchSize: 10}, plan, fakeGate{}, repository, executor)
	service.now = func() time.Time { return now }
	if worked, err := service.Cycle(context.Background()); !worked || !errors.Is(err, ErrUnavailable) ||
		repository.released != "REQUEST_BINDING_DRIFT" || executor.request.ID != "" {
		t.Fatalf("worked=%v err=%v release=%q execute=%#v", worked, err,
			repository.released, executor.request)
	}
}

func TestHealthRequiresNoStalePendingOrRunnerResidue(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	plan := testProductPlan()
	repository := &fakeRepository{activation: testActivation(plan, now)}
	executor := &fakeExecutor{}
	service, err := NewService(Config{ActivationID: plan.ActivationID,
		ClaimOwner: "product-canary-worker", PollInterval: time.Second,
		ClaimTTL: time.Minute, BatchSize: 10}, plan, fakeGate{}, repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	if err := service.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	repository.health.StaleClaims = 1
	if err := service.Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("stale health err=%v", err)
	}
	repository.health = HealthStatus{}
	executor.healthErr = ErrUnavailable
	if err := service.Health(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Runner residue health err=%v", err)
	}
}

func TestProductPlanDerivesOnlyUserAndStableRequestIdentity(t *testing.T) {
	plan := testProductPlan()
	request := testRequest(plan)
	execution := plan.ExecutionPlan(request)
	if err := agentrootcanary.ValidateProductPlan(execution); err != nil {
		t.Fatal(err)
	}
	if execution.UserID != request.UserID || execution.Synthetic ||
		execution.IdempotencyKey != "g21.6-product-canary-1234567890abcdef" ||
		len(execution.ToolRegistry.Tools) != 0 || execution.Sandbox.NetworkMode != "none" {
		t.Fatalf("execution=%#v", execution)
	}
}

type fakeGate struct{ err error }

func (gate fakeGate) Verify(time.Time) error { return gate.err }

type fakeRepository struct {
	activation Activation
	requests   []Request
	completed  Request
	released   string
	health     HealthStatus
}

func (repository *fakeRepository) GetActivation(context.Context, string) (Activation, error) {
	return repository.activation, nil
}
func (repository *fakeRepository) Health(context.Context, string, time.Time) (HealthStatus, error) {
	return repository.health, nil
}
func (repository *fakeRepository) Claim(_ context.Context, _ string, owner string,
	_ time.Time, _ time.Duration, _ int,
) ([]Request, error) {
	result := append([]Request(nil), repository.requests...)
	for index := range result {
		result[index].State = "claimed"
		result[index].ClaimOwner = owner
		result[index].ClaimGeneration++
	}
	repository.requests = nil
	return result, nil
}
func (repository *fakeRepository) Complete(_ context.Context, request Request,
	result ExecutionResult,
) (Receipt, error) {
	repository.completed = request
	return Receipt{RequestID: request.ID, RunID: result.RunID}, nil
}
func (repository *fakeRepository) Release(_ context.Context, _ Request, code string, _ bool) (bool, error) {
	repository.released = code
	return true, nil
}
func (*fakeRepository) Reconcile(context.Context, string, time.Time, int) (int, error) {
	return 0, nil
}

type fakeExecutor struct {
	request   Request
	result    ExecutionResult
	err       error
	healthErr error
}

func (executor *fakeExecutor) Health(context.Context) error { return executor.healthErr }

func (executor *fakeExecutor) Execute(_ context.Context, request Request) (ExecutionResult, error) {
	executor.request = request
	return executor.result, executor.err
}

func testProductPlan() Plan {
	sandbox := agentrunner.SandboxSpec{RuntimeBundleFingerprint: fp("1"),
		PackageFingerprint: fp("2"), Image: "registry.example/neo/product-canary@" + fp("3"),
		UID: 10001, GID: 10001, RootfsReadOnly: true, NoNewPrivileges: true,
		Capabilities: []string{}, SeccompProfileFingerprint: fp("4"), NetworkMode: "none",
		WorkspaceSnapshotID: "workspace_snapshot_0123456789abcdef", WorkspaceFingerprint: fp("5"),
		Resources: agentrunner.ResourceLimits{CPUMillis: 250, MemoryMiB: 128, PIDs: 16,
			WallSeconds: 30, OutputBytes: 4096, ScratchBytes: 1 << 20}}
	registry := agentrunner.ToolRegistry{Depth: 0, Tools: []string{}, RegistryFingerprint: fp("6")}
	grant := fp("7")
	return Plan{SchemaVersion: PlanSchemaVersion, ActivationID: "activation_1234567890abcdef",
		StepKind: "product_canary", GrantID: "grant_0123456789abcdef",
		GrantFingerprint: grant, Fingerprint: fp("a"),
		Snapshot: agentrootcanary.Snapshot{SchemaVersion: "neo.agent-snapshot/v1", Mode: "read_only",
			PackageFingerprint:       sandbox.PackageFingerprint,
			RuntimeBundleFingerprint: sandbox.RuntimeBundleFingerprint,
			GrantFingerprint:         grant, RegistryFingerprint: registry.RegistryFingerprint,
			WorkspaceFingerprint: sandbox.WorkspaceFingerprint, NoEgress: true,
			NoSecrets: true, MaxWallSeconds: 30}, Sandbox: sandbox, ToolRegistry: registry,
		Argv: []string{"/opt/neo/bin/product-canary", "--bounded-smoke"}, LeaseSeconds: 30}
}

func testActivation(plan Plan, now time.Time) Activation {
	return Activation{ID: plan.ActivationID, PolicyRevision: 4,
		PackageFingerprint:       plan.Sandbox.PackageFingerprint,
		RuntimeBundleFingerprint: plan.Sandbox.RuntimeBundleFingerprint,
		PlanFingerprint:          plan.Fingerprint, ValidFrom: now.Add(-time.Minute),
		ValidUntil: now.Add(time.Hour), Enabled: true}
}

func testRequest(plan Plan) Request {
	return Request{ID: "product_request_1234567890abcdef", ActivationID: plan.ActivationID,
		UserID: testUserID, PolicyRevision: 4, OptGeneration: 2,
		PackageFingerprint:       plan.Sandbox.PackageFingerprint,
		RuntimeBundleFingerprint: plan.Sandbox.RuntimeBundleFingerprint,
		PlanFingerprint:          plan.Fingerprint, RequestFingerprint: fp("b"), State: "queued"}
}

func fp(value string) string { return "sha256:" + strings.Repeat(value, 64) }

var _ Repository = (*fakeRepository)(nil)
var _ Executor = (*fakeExecutor)(nil)
