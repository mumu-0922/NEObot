package agentlearningworker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestReconcileWaitsForExpiryAndPreservesUnrelatedRunnerWork(t *testing.T) {
	now := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	plan := validPlan()
	item := recoveryInventory(plan, now.Add(-time.Minute))
	stale := recoveryDescriptor(plan, item, "sandbox_0123456789abcdef")
	unrelated := stale
	unrelated.SandboxID = "sandbox_fedcba9876543210"
	unrelated.Attempt = agentrunner.AttemptIdentity{RunID: "run_fedcba9876543210",
		StepID: "step_fedcba9876543210", AttemptID: "attempt_fedcba9876543210",
		LeaseGeneration: 8}
	repository := &recoveryRepository{inventory: []InventoryAttempt{item}}
	client := &recoveryClient{lists: [][]agentrunner.SandboxDescriptor{
		{stale, unrelated}, {unrelated},
	}}
	runtime := &Runtime{plan: plan, repository: repository, client: client, now: func() time.Time { return now }}

	result, err := runtime.Reconcile(context.Background(), now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inventory != 1 || result.Reaped != 1 || result.Completed != 1 {
		t.Fatalf("Reconcile() = %#v", result)
	}
	if len(client.expected) != 1 || client.expected[0] != unrelated {
		t.Fatalf("reconcile expected = %#v", client.expected)
	}
	if repository.marked != 1 || repository.completed != 1 {
		t.Fatalf("repository transitions = mark %d complete %d", repository.marked, repository.completed)
	}
}

func TestReconcileDoesNotTouchLiveLostToken(t *testing.T) {
	now := time.Date(2026, 8, 15, 6, 0, 0, 0, time.UTC)
	plan := validPlan()
	item := recoveryInventory(plan, now.Add(time.Minute))
	descriptor := recoveryDescriptor(plan, item, "sandbox_0123456789abcdef")
	repository := &recoveryRepository{inventory: []InventoryAttempt{item}}
	client := &recoveryClient{lists: [][]agentrunner.SandboxDescriptor{{descriptor}}}
	runtime := &Runtime{plan: plan, repository: repository, client: client, now: func() time.Time { return now }}

	result, err := runtime.Reconcile(context.Background(), now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inventory != 1 || client.reconciles != 0 || repository.marked != 0 || repository.completed != 0 {
		t.Fatalf("live attempt was touched: result=%#v client=%#v repository=%#v", result, client, repository)
	}
}

func recoveryInventory(plan Plan, expires time.Time) InventoryAttempt {
	check, _ := plan.Check("isolation")
	item := InventoryAttempt{Attempt: agentrunner.AttemptIdentity{
		RunID: "run_0123456789abcdef", StepID: "step_0123456789abcdef",
		AttemptID: "attempt_0123456789abcdef", LeaseGeneration: 1,
	}, DraftID: plan.DraftID, UserID: plan.UserID, CheckGeneration: 3,
		Kind: check.Kind, LeaseOwner: plan.RunnerID,
		SnapshotFingerprint: plan.RunnerSnapshotFingerprint, State: "launched",
		CleanupState: "none", LeaseExpiresAt: expires}
	authorityExpiry := expires.Add(-time.Second)
	item.AuthorityExpiresAt = &authorityExpiry
	return item
}

func recoveryDescriptor(plan Plan, item InventoryAttempt, sandboxID string) agentrunner.SandboxDescriptor {
	check, _ := plan.Check(item.Kind)
	launch := agentrunner.LaunchRequest{Attempt: agentrunner.AttemptRef{
		RunID: item.Attempt.RunID, StepID: item.Attempt.StepID, AttemptID: item.Attempt.AttemptID,
		LeaseGeneration: item.Attempt.LeaseGeneration, LeaseOwner: plan.RunnerID,
		LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv",
	}, Lineage: agentrunner.RunLineage{RootRunID: item.Attempt.RunID, Depth: 0},
		GrantID: plan.GrantID, GrantFingerprint: plan.GrantFingerprint,
		SnapshotFingerprint: plan.RunnerSnapshotFingerprint, Sandbox: plan.Sandbox,
		ToolRegistry: plan.ToolRegistry, Argv: check.Argv}
	return agentrunner.SandboxDescriptor{SandboxID: sandboxID, Attempt: item.Attempt,
		SnapshotFingerprint: plan.RunnerSnapshotFingerprint,
		SpecFingerprint:     agentrunner.SandboxFingerprint(launch), ProbeFingerprint: fingerprint("d"),
		State: "running", UpdatedAt: time.Date(2026, 8, 15, 5, 0, 0, 0, time.UTC)}
}

type recoveryRepository struct {
	inventory         []InventoryAttempt
	marked, completed int
}

func (*recoveryRepository) Begin(context.Context, BeginInput) (BeginResult, error) {
	return BeginResult{}, nil
}
func (*recoveryRepository) RecordLaunch(context.Context, agentrunner.AttemptRef, string, string, string) error {
	return nil
}
func (*recoveryRepository) RecordResult(context.Context, agentrunner.AttemptRef, RunnerResult) error {
	return nil
}
func (repository *recoveryRepository) MarkCancelPending(context.Context, agentrunner.AttemptRef, string) error {
	repository.marked++
	return nil
}
func (repository *recoveryRepository) CompleteCleanup(context.Context, agentrunner.AttemptRef, bool, string) error {
	repository.completed++
	return nil
}
func (repository *recoveryRepository) RunnerInventory(context.Context, int) ([]InventoryAttempt, error) {
	return append([]InventoryAttempt(nil), repository.inventory...), nil
}
func (*recoveryRepository) ReconcileRunner(context.Context, time.Time, int) (int, error) {
	return 0, nil
}
func (*recoveryRepository) PruneRunner(context.Context, time.Time, int) (int, int, int, error) {
	return 0, 0, 0, nil
}

type recoveryClient struct {
	lists      [][]agentrunner.SandboxDescriptor
	expected   []agentrunner.SandboxDescriptor
	reconciles int
}

func (client *recoveryClient) Call(_ context.Context, request agentrunner.Request) (agentrunner.Response, error) {
	switch request.Method {
	case agentrunner.MethodList:
		items := client.lists[0]
		client.lists = client.lists[1:]
		return agentrunner.Response{Body: agentrunner.ListResult{RunnerID: request.List.RunnerID,
			Sandboxes: append([]agentrunner.SandboxDescriptor(nil), items...)}}, nil
	case agentrunner.MethodReconcile:
		client.reconciles++
		client.expected = append([]agentrunner.SandboxDescriptor(nil), request.Reconcile.Expected...)
		return agentrunner.Response{Body: agentrunner.ReconcileResult{RunnerID: request.Reconcile.RunnerID,
			Cleaned: 1}}, nil
	default:
		return agentrunner.Response{}, ErrUnavailable
	}
}

type recoveryAuthority struct{}

func (recoveryAuthority) IssueAuthority(context.Context, agentrunner.IssueAuthorityInput) (
	agentrunner.AuthorityTicket, json.RawMessage, error,
) {
	return agentrunner.AuthorityTicket{}, nil, ErrUnavailable
}
func (recoveryAuthority) CompleteRequest(context.Context, string, string, string, string, []byte) error {
	return ErrUnavailable
}
