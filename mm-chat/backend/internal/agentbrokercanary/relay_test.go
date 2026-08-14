package agentbrokercanary

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestRelayTargetUsesPlanOwnedGrantRegistryAndDurableRequestID(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	bindings := mustBindings(t, validCanaryPlan(t, now, t.TempDir()), now)
	binding, _ := bindings.Action(ActionWorkspaceRead)
	grantFingerprint, _ := agentbroker.GrantFingerprint(binding.Grant)
	broker := &recordingCanaryBroker{prepared: agentbroker.PreparedIntent{
		IntentID: "intent_0123456789abcdef", IntentFingerprint: testCanaryFingerprint("7"),
		IdempotencyKey: "commit_0123456789abcdefghijklmn", ApprovalClass: agentbroker.ApprovalAutomatic,
		ExpiresAt: now.Add(time.Minute)}}
	target, err := NewRelayTarget(broker, bindings)
	if err != nil {
		t.Fatal(err)
	}
	target.now = func() time.Time { return now }
	attempt := agentrunner.AttemptRef{RunID: binding.RunID, StepID: binding.StepID,
		AttemptID: "attempt_0123456789abcdef", LeaseGeneration: 1,
		LeaseOwner: testCanaryRunner, LeaseToken: "lease_0123456789abcdefghijklmnopqrstuv"}
	prepare := agentrunner.PrepareRequest{Attempt: attempt, Authority: agentrunner.AuthorityTicket{
		AuthorityClaims: agentrunner.AuthorityClaims{CallerIdentity: BrokerCanaryIdentity,
			Method: agentrunner.MethodPrepare, RequestID: "rpc_0123456789abcdef", KillSwitchEpoch: 3}},
		SnapshotFingerprint: binding.SnapshotFingerprint, GrantID: binding.Grant.GrantID,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: binding.Registry.Fingerprint,
		ToolIdentity: binding.Plan.ToolIdentity, Capability: binding.Plan.Capability,
		Action: binding.Plan.Action, Resource: binding.Plan.Resource, Arguments: binding.Plan.Arguments,
		ArgumentsFingerprint: binding.ArgumentsFingerprint, TTLSeconds: binding.Plan.TTLSeconds}
	result, err := target.Prepare(context.Background(), prepare)
	if err != nil || !result.Prepared || broker.prepare.RequestID != "request_0123456789abcdef" ||
		broker.prepare.Grant.GrantID != binding.Grant.GrantID ||
		broker.prepare.Registry.Fingerprint != binding.Registry.Fingerprint ||
		broker.prepare.Attempt.KillSwitchEpoch != 3 {
		t.Fatalf("Prepare = %#v, %v, input=%#v", result, err, broker.prepare)
	}

	commitRequest := agentrunner.CommitRequest{Attempt: attempt, Authority: agentrunner.AuthorityTicket{
		AuthorityClaims: agentrunner.AuthorityClaims{CallerIdentity: BrokerCanaryIdentity,
			Method: agentrunner.MethodCommit, RequestID: "rpc_fedcba9876543210", KillSwitchEpoch: 3}},
		SnapshotFingerprint: binding.SnapshotFingerprint, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: binding.Registry.Fingerprint, IntentID: result.IntentID,
		IntentFingerprint: result.IntentFingerprint, IdempotencyKey: result.IdempotencyKey}
	broker.committed = agentbroker.CommitResult{Outcome: agentbroker.OutcomeCommitted,
		IdempotencyKey: result.IdempotencyKey, ReceiptFingerprint: testCanaryFingerprint("8")}
	committed, err := target.Commit(context.Background(), commitRequest)
	if err != nil || committed.Outcome != agentbroker.OutcomeCommitted ||
		broker.commit.Attempt.UserID != binding.Grant.Subject.UserID {
		t.Fatalf("Commit = %#v, %v, input=%#v", committed, err, broker.commit)
	}

	tampered := prepare
	tampered.Arguments = json.RawMessage(`{"path":"other.txt"}`)
	if _, err := target.Prepare(context.Background(), tampered); !errors.Is(err, agentbroker.ErrSnapshotMismatch) || broker.prepares != 1 {
		t.Fatalf("tampered Prepare = %v, calls=%d", err, broker.prepares)
	}
}

type recordingCanaryBroker struct {
	prepare   agentbroker.PrepareInput
	commit    agentbroker.CommitInput
	prepared  agentbroker.PreparedIntent
	committed agentbroker.CommitResult
	prepares  int
	commits   int
}

func (broker *recordingCanaryBroker) Prepare(_ context.Context, input agentbroker.PrepareInput) (agentbroker.PreparedIntent, error) {
	broker.prepares++
	broker.prepare = input
	return broker.prepared, nil
}

func (broker *recordingCanaryBroker) Commit(_ context.Context, input agentbroker.CommitInput) (agentbroker.CommitResult, error) {
	broker.commits++
	broker.commit = input
	return broker.committed, nil
}
