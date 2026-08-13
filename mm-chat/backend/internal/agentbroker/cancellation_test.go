package agentbroker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancelBeforeCommitWinsAndExactReplayIsStable(t *testing.T) {
	now := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	executor := &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage}}
	service, repository, _ := testService(t, now, executor)
	input := testPrepareInput(t, now, ApprovalOnce)
	prepared, err := service.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	decision := ApprovalInput{ApprovalID: "approval_cccccccccccccccc", UserID: input.Attempt.UserID,
		IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint, Decision: "approved",
		ActorType: "user", ActorID: input.Attempt.UserID, ReasonCode: "USER_APPROVED", ExpectedRevision: 1}
	if _, err := service.DecideApproval(context.Background(), decision); err != nil {
		t.Fatal(err)
	}
	cancel := CancelInput{CancellationID: "cancellation_aaaaaaaaaaaaaaaa", UserID: input.Attempt.UserID,
		IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint,
		ActorType: "user", ActorID: input.Attempt.UserID, ReasonCode: "USER_CANCELED"}
	canceled, err := service.Cancel(context.Background(), cancel)
	if err != nil || canceled.State != IntentCanceled || canceled.ErrorCode != "INTENT_CANCELED" {
		t.Fatalf("Cancel = %#v, %v", canceled, err)
	}
	replayed, err := service.Cancel(context.Background(), cancel)
	if err != nil || replayed.State != IntentCanceled {
		t.Fatalf("Cancel replay = %#v, %v", replayed, err)
	}
	changed := cancel
	changed.ReasonCode = "OPERATOR_CANCELED"
	if _, err := service.Cancel(context.Background(), changed); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("Cancel mismatch = %v", err)
	}
	commit := testCommitInput(prepared, input.Attempt)
	commit.ApprovalID = decision.ApprovalID
	result, err := service.Commit(context.Background(), commit)
	if err != nil || result.Outcome != OutcomeRejected || executor.commits != 0 || repository.effectCount != 0 {
		t.Fatalf("Commit after Cancel = %#v, %v, calls=%d/%d", result, err, executor.commits, repository.effectCount)
	}
}

func TestCommitBeforeCancelCannotAssertRollback(t *testing.T) {
	now := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	service, _, _ := testService(t, now, &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage}})
	input := testPrepareInput(t, now, ApprovalAutomatic)
	prepared, err := service.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Commit(context.Background(), testCommitInput(prepared, input.Attempt)); err != nil {
		t.Fatal(err)
	}
	cancel := CancelInput{CancellationID: "cancellation_bbbbbbbbbbbbbbbb", UserID: input.Attempt.UserID,
		IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint,
		ActorType: "user", ActorID: input.Attempt.UserID, ReasonCode: "USER_CANCELED"}
	if _, err := service.Cancel(context.Background(), cancel); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Cancel after Commit = %v", err)
	}
}

func TestCancelClearsInMemorySecretAfterDurableTransition(t *testing.T) {
	now := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	repository := &combinedMemoryRepository{
		memoryRepository:       &memoryRepository{intents: map[string]PreparedIntent{}, requests: map[string]string{}, approvals: map[string]ApprovalInput{}},
		memorySecretRepository: &memorySecretRepository{records: map[string]SecretHandleRecord{}},
	}
	secretBroker, err := NewSecretBroker(repository, secretResolverFunc(func(context.Context, string, Subject) ([]byte, error) {
		return []byte("G204_CANCEL_SECRET_CANARY"), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	secretBroker.now = func() time.Time { return now }
	service, err := NewService(repository, nil, secretBroker)
	if err != nil {
		t.Fatal(err)
	}
	intent, registry := testSecretAuthority(t, now)
	intent.State = IntentApproved
	repository.intents[intent.IntentID] = intent
	committing := intent
	committing.State = IntentCommitting
	if _, err := secretBroker.Issue(context.Background(), committing, registry,
		"secret_ref_0123456789abcdef", testSecretBinding(), time.Minute); err != nil {
		t.Fatal(err)
	}
	canceled, err := service.Cancel(context.Background(), CancelInput{
		CancellationID: "cancellation_cccccccccccccccc", UserID: intent.Subject.UserID,
		IntentID: intent.IntentID, IntentFingerprint: intent.IntentFingerprint,
		ActorType: "user", ActorID: intent.Subject.UserID, ReasonCode: "USER_CANCELED",
	})
	if err != nil || canceled.State != IntentCanceled {
		t.Fatalf("Cancel = %#v, %v", canceled, err)
	}
	secretBroker.mu.Lock()
	remaining := len(secretBroker.handles)
	secretBroker.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("Cancel retained %d in-memory handles", remaining)
	}
}

type combinedMemoryRepository struct {
	*memoryRepository
	*memorySecretRepository
}
