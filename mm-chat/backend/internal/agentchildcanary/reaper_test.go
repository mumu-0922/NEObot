package agentchildcanary

import (
	"context"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agentdelegation"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

type reaperClient struct {
	before            []agentrunner.SandboxDescriptor
	after             []agentrunner.SandboxDescriptor
	lists             int
	reconcileExpected []agentrunner.SandboxDescriptor
}

func (client *reaperClient) Call(_ context.Context, request agentrunner.Request) (agentrunner.Response, error) {
	switch request.Method {
	case agentrunner.MethodList:
		client.lists++
		items := client.before
		if client.lists > 1 {
			items = client.after
		}
		return agentrunner.Response{Body: agentrunner.ListResult{RunnerID: "neo-runner-primary", Sandboxes: items}}, nil
	case agentrunner.MethodReconcile:
		client.reconcileExpected = append([]agentrunner.SandboxDescriptor(nil), request.Reconcile.Expected...)
		return agentrunner.Response{Body: agentrunner.ReconcileResult{RunnerID: "neo-runner-primary", Cleaned: 1}}, nil
	default:
		return agentrunner.Response{}, ErrUnavailable
	}
}

func TestRunnerReaperWaitsAuthorityAndRetainsParent(t *testing.T) {
	now := testNow
	parent := descriptor("sandbox_0000000000000001", "run_0000000000000001",
		"step_0000000000000001", "attempt_0000000000000001", fingerprint('a'), fingerprint('b'), fingerprint('c'))
	child := descriptor("sandbox_0000000000000002", "run_0000000000000002",
		"step_0000000000000002", "attempt_0000000000000002", fingerprint('d'), fingerprint('e'), fingerprint('f'))
	client := &reaperClient{before: []agentrunner.SandboxDescriptor{parent, child}, after: []agentrunner.SandboxDescriptor{parent}}
	reaper, err := NewRunnerReaper("neo-runner-primary", client)
	if err != nil {
		t.Fatal(err)
	}
	reaper.now = func() time.Time { return now }
	waits := 0
	reaper.wait = func(_ context.Context, delay time.Duration) error {
		waits++
		if delay != 5*time.Second {
			t.Fatalf("delay = %s", delay)
		}
		now = now.Add(delay)
		return nil
	}
	expiresAt := now.Add(5 * time.Second)
	target := agentdelegation.ReapTarget{ReapID: "reap_0000000000000001", ParentRunID: parent.Attempt.RunID,
		ChildRunID: child.Attempt.RunID, StepID: child.Attempt.StepID, AttemptID: child.Attempt.AttemptID,
		Generation: 1, LeaseOwner: "neo-runner-primary", AuthoritySnapshotFingerprint: child.SnapshotFingerprint,
		SandboxID: child.SandboxID, SandboxGeneration: 1, SandboxRunnerID: "neo-runner-primary",
		SandboxSnapshotFingerprint: child.SnapshotFingerprint, SpecFingerprint: child.SpecFingerprint,
		ProbeFingerprint: child.ProbeFingerprint, LaunchAuthorityExpiresAt: &expiresAt}
	if err := reaper.Reap(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if waits != 1 || len(client.reconcileExpected) != 1 || client.reconcileExpected[0] != parent {
		t.Fatalf("waits=%d expected=%#v", waits, client.reconcileExpected)
	}
}

func TestRunnerReaperRejectsSecondOrMismatchedChild(t *testing.T) {
	child := descriptor("sandbox_0000000000000002", "run_0000000000000002",
		"step_0000000000000002", "attempt_0000000000000002", fingerprint('d'), fingerprint('e'), fingerprint('f'))
	target := agentdelegation.ReapTarget{ReapID: "reap_0000000000000001", ChildRunID: child.Attempt.RunID,
		StepID: child.Attempt.StepID, AttemptID: child.Attempt.AttemptID, Generation: 1,
		LeaseOwner: "neo-runner-primary", AuthoritySnapshotFingerprint: child.SnapshotFingerprint}
	for name, mutate := range map[string]func(*agentrunner.SandboxDescriptor){
		"attempt":  func(value *agentrunner.SandboxDescriptor) { value.Attempt.AttemptID = "attempt_0000000000000003" },
		"snapshot": func(value *agentrunner.SandboxDescriptor) { value.SnapshotFingerprint = fingerprint('9') },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := child
			mutate(&invalid)
			client := &reaperClient{before: []agentrunner.SandboxDescriptor{invalid}}
			reaper, _ := NewRunnerReaper("neo-runner-primary", client)
			reaper.now = func() time.Time { return testNow }
			if reaper.Reap(context.Background(), target) == nil {
				t.Fatal("mismatched Child accepted")
			}
		})
	}
}

func descriptor(sandboxID, runID, stepID, attemptID, snapshot, spec, probe string) agentrunner.SandboxDescriptor {
	return agentrunner.SandboxDescriptor{SandboxID: sandboxID,
		Attempt:             agentrunner.AttemptIdentity{RunID: runID, StepID: stepID, AttemptID: attemptID, LeaseGeneration: 1},
		SnapshotFingerprint: snapshot, SpecFingerprint: spec, ProbeFingerprint: probe,
		State: "running", UpdatedAt: testNow}
}
