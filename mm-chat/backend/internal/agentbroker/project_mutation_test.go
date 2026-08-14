package agentbroker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProjectEffectExecutorBindsExactSyntheticPatchAndCleanup(t *testing.T) {
	authority := testProjectMutationAuthority()
	repository := &projectMutationRepositoryFake{commit: ProjectMutationReceipt{
		Outcome: OutcomeCommitted, ReceiptFingerprint: testPackage, CommittedRevision: testRuntime},
		status: ProjectMutationReceipt{Outcome: OutcomeCommitted,
			ReceiptFingerprint: testPackage, CommittedRevision: testRuntime}, cleanupRevision: authority.BaseRevision}
	executor, err := NewProjectEffectExecutor(repository, authority)
	if err != nil {
		t.Fatal(err)
	}
	request := projectMutationRequest(executor)
	receipt, err := executor.Commit(context.Background(), request)
	if err != nil || receipt.ReceiptFingerprint != testPackage || receipt.StatusToken != testRuntime || repository.commitCalls != 1 {
		t.Fatalf("Commit = %#v, %v, calls=%d", receipt, err, repository.commitCalls)
	}
	status, err := executor.Status(context.Background(), request)
	if err != nil || status.Outcome != OutcomeCommitted || repository.statusCalls != 1 {
		t.Fatalf("Status = %#v, %v, calls=%d", status, err, repository.statusCalls)
	}
	revision, err := executor.Cleanup(context.Background(), request, testPackage)
	if err != nil || revision != authority.BaseRevision || repository.cleanupCalls != 1 {
		t.Fatalf("Cleanup = %q, %v, calls=%d", revision, err, repository.cleanupCalls)
	}
	if repository.lastContent != "synthetic mutation\n" || repository.last.IntentID != request.IntentID ||
		repository.last.IdempotencyKey != request.IdempotencyKey {
		t.Fatalf("bound authority = %#v", repository.last)
	}

	widened := request
	widened.Resource = "project-canary/other"
	if _, err := executor.Commit(context.Background(), widened); err == nil || repository.commitCalls != 1 {
		t.Fatalf("widened Commit = %v, calls=%d", err, repository.commitCalls)
	}
}

func TestProjectEffectExecutorMarksUnknownDatabaseFailureAsPossibleSend(t *testing.T) {
	authority := testProjectMutationAuthority()
	repository := &projectMutationRepositoryFake{commitErr: ErrExecutorUnavailable,
		status: ProjectMutationReceipt{Outcome: OutcomeRejected, CommittedRevision: authority.BaseRevision}}
	executor, _ := NewProjectEffectExecutor(repository, authority)
	_, err := executor.Commit(context.Background(), projectMutationRequest(executor))
	var dispatch *DispatchError
	if !errors.As(err, &dispatch) || !dispatch.PossibleSend {
		t.Fatalf("Commit error = %#v", err)
	}
	status, err := executor.Status(context.Background(), projectMutationRequest(executor))
	if err != nil || status.Outcome != OutcomeRejected || status.StatusToken != authority.BaseRevision {
		t.Fatalf("Status = %#v, %v", status, err)
	}
}

func TestProjectMutationFingerprintRejectsContentOrBindingDrift(t *testing.T) {
	authority := testProjectMutationAuthority()
	for _, mutate := range []func(*ProjectMutationAuthority){
		func(value *ProjectMutationAuthority) { value.Content = []byte("other") },
		func(value *ProjectMutationAuthority) { value.Path = "nested/file.txt" },
		func(value *ProjectMutationAuthority) { value.Resource = "user-project/value" },
		func(value *ProjectMutationAuthority) { value.MutationFingerprint = testRuntime },
	} {
		candidate := authority
		candidate.Content = append([]byte(nil), authority.Content...)
		mutate(&candidate)
		if _, err := NewProjectEffectExecutor(&projectMutationRepositoryFake{}, candidate); err == nil {
			t.Fatalf("drift accepted: %#v", candidate)
		}
	}
}

func TestProjectMutationCommitCrashMatrixNeverRedispatches(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name        string
		commitErr   error
		status      ProjectMutationReceipt
		statusErr   error
		wantOutcome string
		wantError   error
		wantStatus  int
	}{
		{name: "after CAS acknowledgement loss", commitErr: ErrExecutorUnavailable,
			status: ProjectMutationReceipt{Outcome: OutcomeCommitted, ReceiptFingerprint: testPackage,
				CommittedRevision: testRuntime}, wantOutcome: OutcomeCommitted, wantStatus: 1},
		{name: "definitely not sent", commitErr: ErrProjectMutationDenied,
			status: ProjectMutationReceipt{Outcome: OutcomeCommitted, ReceiptFingerprint: testPackage,
				CommittedRevision: testRuntime}, wantOutcome: OutcomeRejected, wantStatus: 0},
		{name: "ambiguous status", commitErr: ErrExecutorUnavailable, statusErr: ErrExecutorUnavailable,
			wantOutcome: OutcomeUnknown, wantError: ErrOutcomeUnknown, wantStatus: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			effectRepository := &projectMutationRepositoryFake{commitErr: test.commitErr,
				status: test.status, statusErr: test.statusErr}
			executor, err := NewProjectEffectExecutor(effectRepository, testProjectMutationAuthority())
			if err != nil {
				t.Fatal(err)
			}
			brokerRepository := newMemoryRepository()
			broker, err := NewService(brokerRepository, map[string]EffectExecutor{"project.patch": executor})
			if err != nil {
				t.Fatal(err)
			}
			prepare := projectMutationPrepareInput(t, now, executor)
			prepared, err := broker.Prepare(context.Background(), prepare)
			if err != nil {
				t.Fatal(err)
			}
			approvalID := "approval_4444444444444444"
			if _, err := broker.DecideApproval(context.Background(), ApprovalInput{ApprovalID: approvalID,
				UserID: prepare.Attempt.UserID, IntentID: prepared.IntentID,
				IntentFingerprint: prepared.IntentFingerprint, Decision: "approved", ActorType: "operator",
				ActorID: "project-canary-test", ReasonCode: "PROJECT_CANARY_TEST", ExpectedRevision: 1}); err != nil {
				t.Fatal(err)
			}
			commit := testCommitInput(prepared, prepare.Attempt)
			commit.ApprovalID = approvalID
			result, commitErr := broker.Commit(context.Background(), commit)
			if !errors.Is(commitErr, test.wantError) || result.Outcome != test.wantOutcome ||
				effectRepository.commitCalls != 1 || effectRepository.statusCalls != test.wantStatus {
				t.Fatalf("Commit = %#v, %v, dispatch=%d status=%d", result, commitErr,
					effectRepository.commitCalls, effectRepository.statusCalls)
			}
			replay, replayErr := broker.Commit(context.Background(), commit)
			if !errors.Is(replayErr, test.wantError) || effectRepository.commitCalls != 1 ||
				effectRepository.statusCalls != test.wantStatus ||
				(test.wantOutcome == OutcomeCommitted && replay.Outcome != OutcomeReplayed) ||
				(test.wantOutcome != OutcomeCommitted && replay.Outcome != test.wantOutcome) {
				t.Fatalf("replay = %#v, %v, dispatch=%d status=%d", replay, replayErr,
					effectRepository.commitCalls, effectRepository.statusCalls)
			}
		})
	}
}

func projectMutationPrepareInput(t *testing.T, now time.Time, executor *ProjectEffectExecutor) PrepareInput {
	t.Helper()
	grant := testGrantAt(now)
	grant.Capabilities = []Capability{{Capability: "project.write", Actions: []string{"apply_patch"},
		Resources: Selector{Kind: "exact", Values: []string{executor.authority.Resource}},
		Approval:  ApprovalPerCommit, MaxCalls: 1}}
	grant.Budget.MaxToolCalls = 1
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "project.patch", Capability: "project.write",
		Actions: []string{"apply_patch"}, Classification: ClassificationMutable}}, []string{"project.patch"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, err := GrantFingerprint(grant)
	if err != nil {
		t.Fatal(err)
	}
	attempt := AttemptAuthority{UserID: testUser, RunID: testRun, StepID: testStep, AttemptID: testAttempt,
		Generation: 1, LeaseOwner: "neo-runner-primary", LeaseToken: "lease_0123456789abcdef01234567",
		SnapshotFingerprint: testSnapshot, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: registry.Fingerprint, KillSwitchEpoch: 0}
	return PrepareInput{RequestID: "request_5555555555555555", Attempt: attempt, Grant: grant,
		Registry: registry, ToolIdentity: "project.patch", Action: "apply_patch", Resource: executor.authority.Resource,
		Arguments: append([]byte(nil), executor.arguments...), BaseRevision: executor.authority.BaseRevision,
		TTL: 10 * time.Minute}
}

func testProjectMutationAuthority() ProjectMutationAuthority {
	content := []byte("synthetic mutation\n")
	contentFingerprint := fingerprint("neo-project-file-v1", content)
	base := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	resource := "project-canary/g21-3"
	pathValue := "g21-3-canary.txt"
	return ProjectMutationAuthority{Resource: resource, BaseRevision: base, Path: pathValue,
		Content: content, ContentFingerprint: contentFingerprint,
		MutationFingerprint: ProjectMutationFingerprint(base, resource, pathValue, contentFingerprint)}
}

func projectMutationRequest(executor *ProjectEffectExecutor) ExecutionRequest {
	return ExecutionRequest{UserID: testUser, RunID: testRun, StepID: testStep, AttemptID: testAttempt,
		Generation: 1, SnapshotFingerprint: testSnapshot, GrantFingerprint: testPackage,
		RegistryFingerprint: testRuntime, IntentID: "intent_0123456789abcdef",
		IdempotencyKey: "commit_0123456789abcdefghijklmn", ToolIdentity: "project.patch",
		Capability: "project.write", Action: "apply_patch", Resource: executor.authority.Resource,
		Arguments: append([]byte(nil), executor.arguments...), BaseRevision: executor.authority.BaseRevision}
}

type projectMutationRepositoryFake struct {
	commit, status           ProjectMutationReceipt
	commitErr, statusErr     error
	cleanupRevision          string
	cleanupErr               error
	commitCalls, statusCalls int
	cleanupCalls             int
	last                     ProjectMutationAuthority
	lastContent              string
}

func (repository *projectMutationRepositoryFake) CommitProjectMutation(_ context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	repository.commitCalls++
	repository.last = authority
	repository.lastContent = string(authority.Content)
	return repository.commit, repository.commitErr
}

func (repository *projectMutationRepositoryFake) ProjectMutationStatus(_ context.Context,
	authority ProjectMutationAuthority,
) (ProjectMutationReceipt, error) {
	repository.statusCalls++
	repository.last = authority
	return repository.status, repository.statusErr
}

func (repository *projectMutationRepositoryFake) CleanupProjectMutation(_ context.Context,
	authority ProjectMutationAuthority, _ string,
) (string, error) {
	repository.cleanupCalls++
	repository.last = authority
	return repository.cleanupRevision, repository.cleanupErr
}
