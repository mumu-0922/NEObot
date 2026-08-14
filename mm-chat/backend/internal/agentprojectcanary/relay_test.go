package agentprojectcanary

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestRelayUsesOnlyProjectPlanAuthority(t *testing.T) {
	now := time.Date(2026, 8, 15, 1, 0, 0, 0, time.UTC)
	bindings, err := NewBindings(validProjectPlan(t, now), testProjectRunner, now)
	if err != nil {
		t.Fatal(err)
	}
	binding, _ := bindings.Action()
	broker := &recordingProjectBroker{prepared: agentbroker.PreparedIntent{
		IntentID: "intent_0123456789abcdef", IntentFingerprint: repeatedFingerprint("8"),
		IdempotencyKey: "commit_0123456789abcdefghijklmn", ApprovalClass: agentbroker.ApprovalPerCommit,
		ExpiresAt: now.Add(time.Minute)}}
	target, err := NewRelayTarget(broker, bindings)
	if err != nil {
		t.Fatal(err)
	}
	target.now = func() time.Time { return now }
	attempt := agentrunner.AttemptRef{RunID: binding.RunID, StepID: binding.StepID,
		AttemptID: "attempt_0123456789abcdef", LeaseGeneration: 1,
		LeaseOwner: testProjectRunner, LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"}
	prepare := agentrunner.PrepareRequest{Attempt: attempt, Authority: agentrunner.AuthorityTicket{
		AuthorityClaims: agentrunner.AuthorityClaims{CallerIdentity: ProjectCanaryIdentity,
			Method: agentrunner.MethodPrepare, RequestID: "rpc_0123456789abcdef", KillSwitchEpoch: 3}},
		SnapshotFingerprint: binding.SnapshotFingerprint, GrantID: binding.Grant.GrantID,
		GrantFingerprint: binding.GrantFingerprint(), RegistryFingerprint: binding.Registry.Fingerprint,
		ToolIdentity: binding.Plan.ToolIdentity, Capability: binding.Plan.Capability,
		Action: binding.Plan.Action, Resource: binding.Plan.Resource, Arguments: binding.Plan.Arguments,
		ArgumentsFingerprint: binding.ArgumentsFingerprint, BaseRevision: binding.Plan.BaseRevision,
		TTLSeconds: binding.Plan.TTLSeconds}
	result, err := target.Prepare(context.Background(), prepare)
	if err != nil || !result.Prepared || broker.prepare.RequestID != "request_0123456789abcdef" ||
		broker.prepare.Grant.GrantID != binding.Grant.GrantID || broker.prepare.Attempt.KillSwitchEpoch != 3 {
		t.Fatalf("Prepare = %#v, %v, input=%#v", result, err, broker.prepare)
	}
	commitRequest := agentrunner.CommitRequest{Attempt: attempt, Authority: agentrunner.AuthorityTicket{
		AuthorityClaims: agentrunner.AuthorityClaims{CallerIdentity: ProjectCanaryIdentity,
			Method: agentrunner.MethodCommit, RequestID: "rpc_fedcba9876543210", KillSwitchEpoch: 3}},
		SnapshotFingerprint: binding.SnapshotFingerprint, GrantFingerprint: binding.GrantFingerprint(),
		RegistryFingerprint: binding.Registry.Fingerprint, IntentID: result.IntentID,
		IntentFingerprint: result.IntentFingerprint, ApprovalID: "approval_3131313131313131",
		IdempotencyKey: result.IdempotencyKey}
	broker.committed = agentbroker.CommitResult{Outcome: agentbroker.OutcomeCommitted,
		IdempotencyKey: result.IdempotencyKey, ReceiptFingerprint: repeatedFingerprint("9")}
	committed, err := target.Commit(context.Background(), commitRequest)
	if err != nil || committed.Outcome != agentbroker.OutcomeCommitted ||
		broker.commit.ApprovalID != commitRequest.ApprovalID {
		t.Fatalf("Commit = %#v, %v, input=%#v", committed, err, broker.commit)
	}
	tampered := prepare
	tampered.Arguments = json.RawMessage(`{"path":"other.txt"}`)
	if _, err := target.Prepare(context.Background(), tampered); !errors.Is(err, agentbroker.ErrSnapshotMismatch) ||
		broker.prepares != 1 {
		t.Fatalf("tampered Prepare = %v, calls=%d", err, broker.prepares)
	}
}

type recordingProjectBroker struct {
	prepare   agentbroker.PrepareInput
	commit    agentbroker.CommitInput
	prepared  agentbroker.PreparedIntent
	committed agentbroker.CommitResult
	prepares  int
}

func (broker *recordingProjectBroker) Prepare(_ context.Context, input agentbroker.PrepareInput) (agentbroker.PreparedIntent, error) {
	broker.prepares++
	broker.prepare = input
	return broker.prepared, nil
}

func (broker *recordingProjectBroker) DecideApproval(context.Context, agentbroker.ApprovalInput) (agentbroker.PreparedIntent, error) {
	return agentbroker.PreparedIntent{}, nil
}

func (broker *recordingProjectBroker) Commit(_ context.Context, input agentbroker.CommitInput) (agentbroker.CommitResult, error) {
	broker.commit = input
	return broker.committed, nil
}
