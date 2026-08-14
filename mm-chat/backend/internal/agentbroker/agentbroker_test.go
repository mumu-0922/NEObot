package agentbroker

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"
)

const (
	testUser     = "123e4567-e89b-42d3-a456-426614174000"
	testRun      = "run_0123456789abcdef"
	testStep     = "step_0123456789abcdef"
	testAttempt  = "attempt_0123456789abcdef"
	testGrant    = "grant_0123456789abcdef"
	testSnapshot = "sha256:6666666666666666666666666666666666666666666666666666666666666666"
	testPackage  = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testRuntime  = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func TestBuildRegistryIsDeterministicAndPhysicallyRemovesChildAuthority(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	grant := testGrantAt(now)
	grant.Run = RunBinding{RunID: testRun, ParentRunID: "run_fedcba9876543210", Depth: 1}
	grant.Capabilities = append(grant.Capabilities, Capability{Capability: "delegate_task",
		Actions: []string{"create"}, Resources: Selector{Kind: "exact", Values: []string{"child"}},
		Approval: ApprovalAutomatic, MaxCalls: 1})
	sort.Slice(grant.Capabilities, func(i, j int) bool {
		return grant.Capabilities[i].Capability < grant.Capabilities[j].Capability
	})
	catalog := []ToolDefinition{
		{Identity: "delegate_task", Capability: "delegate_task", Actions: []string{"create"}, Classification: ClassificationMutable},
		{Identity: "workspace_read", Capability: "workspace.read", Actions: []string{"read", "list"}, Classification: ClassificationRead, Idempotent: true},
	}
	first, err := BuildRegistry(catalog, []string{"delegate_task", "workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildRegistry([]ToolDefinition{catalog[1], catalog[0]}, []string{"workspace_read", "delegate_task"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint || first.Secrets == nil || len(first.Tools) != 1 || first.Tools[0].Identity != "workspace_read" {
		t.Fatalf("registry drift or child authority present: %#v %#v", first, second)
	}
	forged := first
	forged.Tools = append(forged.Tools, RegistryTool{Identity: "delegate_task", Capability: "delegate_task",
		Actions: []string{"create"}, Resources: Selector{Kind: "exact", Values: []string{"child"}},
		Approval: ApprovalAutomatic, MaxCalls: 1, Classification: ClassificationMutable})
	forged.Fingerprint, _ = registryFingerprint(forged)
	if _, err := RegistryToolFor(forged, "delegate_task", "create", "child"); !errors.Is(err, ErrGrantDenied) && !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("forged child registry error = %v", err)
	}
}

func TestBuildRegistryRejectsChildForbiddenCapabilityAlias(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	grant := testGrantAt(now)
	grant.Run = RunBinding{RunID: testRun, ParentRunID: "run_fedcba9876543210", Depth: 1}
	grant.Capabilities = append(grant.Capabilities, Capability{Capability: "delegate_task",
		Actions: []string{"create"}, Resources: Selector{Kind: "exact", Values: []string{"child"}},
		Approval: ApprovalAutomatic, MaxCalls: 1})
	sort.Slice(grant.Capabilities, func(i, j int) bool { return grant.Capabilities[i].Capability < grant.Capabilities[j].Capability })
	_, err := BuildRegistry([]ToolDefinition{{Identity: "innocent_alias", Capability: "delegate_task",
		Actions: []string{"create"}, Classification: ClassificationMutable}}, []string{"innocent_alias"}, grant, now)
	if !errors.Is(err, ErrGrantDenied) {
		t.Fatalf("forbidden capability alias error = %v", err)
	}
}

func TestBuildRegistryRejectsUnknownUnauthorizedAndExhaustedRequests(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	grant := testGrantAt(now)
	catalog := []ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"read"}, Classification: ClassificationRead, Idempotent: true},
		{Identity: "network_read", Capability: "network.read", Actions: []string{"get"},
			Classification: ClassificationRead, Idempotent: true}}
	for _, test := range []struct {
		name      string
		requested []string
		mutate    func(*CapabilityGrant)
		want      error
	}{
		{name: "unknown Tool", requested: []string{"missing_tool"}, want: ErrGrantDenied},
		{name: "missing capability", requested: []string{"network_read"}, want: ErrGrantDenied},
		{name: "total budget exhausted", requested: []string{"workspace_read"},
			mutate: func(value *CapabilityGrant) { value.Budget.MaxToolCalls = 0 }, want: ErrBudgetExhausted},
		{name: "capability budget exhausted", requested: []string{"workspace_read"},
			mutate: func(value *CapabilityGrant) { value.Capabilities[0].MaxCalls = 0 }, want: ErrGrantDenied},
		{name: "future issuance", requested: []string{"workspace_read"},
			mutate: func(value *CapabilityGrant) { value.IssuedAt = now.Add(time.Second) }, want: ErrGrantDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := grant
			candidate.Capabilities = append([]Capability(nil), grant.Capabilities...)
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			if _, err := BuildRegistry(catalog, test.requested, candidate, now); !errors.Is(err, test.want) {
				t.Fatalf("BuildRegistry error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestBuildRegistryRejectsOutOfRangeWallAndModelBudgets(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*CapabilityGrant){
		"wall":  func(grant *CapabilityGrant) { grant.Budget.MaxWallSeconds = 86401 },
		"model": func(grant *CapabilityGrant) { grant.Budget.MaxModelTokens = 10000001 },
	} {
		t.Run(name, func(t *testing.T) {
			grant := testGrantAt(now)
			mutate(&grant)
			catalog := []ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
				Actions: []string{"read"}, Classification: ClassificationRead, Idempotent: true}}
			if _, err := BuildRegistry(catalog, []string{"workspace_read"}, grant, now); !errors.Is(err, ErrGrantDenied) {
				t.Fatalf("budget validation error = %v", err)
			}
		})
	}
}

func TestRegistryLookupRejectsUnsortedSignedShape(t *testing.T) {
	now := time.Now().UTC()
	grant := testGrantAt(now)
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	unsorted := registry
	unsorted.Tools = append([]RegistryTool{{Identity: "z_tool", Capability: "workspace.read",
		Actions: []string{"read"}, Resources: Selector{Kind: "prefix", Values: []string{"project/"}},
		Approval: ApprovalAutomatic, MaxCalls: 1, Classification: ClassificationRead, Idempotent: true}},
		unsorted.Tools...)
	unsorted.Fingerprint, _ = registryFingerprint(unsorted)
	if _, err := RegistryToolFor(unsorted, "workspace_read", "read", "project/a"); !errors.Is(err, ErrSnapshotMismatch) {
		t.Fatalf("unsorted Registry error = %v", err)
	}
}

func TestReadRetryRequiresExplicitIdempotenceAndNoObservedResult(t *testing.T) {
	base := RegistryTool{Classification: ClassificationRead, Idempotent: true, Approval: ApprovalAutomatic}
	if !ReadRetryAllowed(base, false) {
		t.Fatal("explicit idempotent read without a result must be retryable")
	}
	for _, mutate := range []func(*RegistryTool){
		func(value *RegistryTool) { value.Idempotent = false },
		func(value *RegistryTool) { value.Classification = ClassificationMutable },
		func(value *RegistryTool) { value.Approval = ApprovalOnce },
	} {
		candidate := base
		mutate(&candidate)
		if ReadRetryAllowed(candidate, false) {
			t.Fatalf("unsafe read retry allowed: %#v", candidate)
		}
	}
	if ReadRetryAllowed(base, true) {
		t.Fatal("observed read result must not be retried")
	}
}

func TestPrepareCanonicalizesArgumentsAndExactReplayDoesNotMutate(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	service, repository, _ := testService(t, now, &fakeExecutor{})
	input := testPrepareInput(t, now, ApprovalOnce)
	input.Arguments = json.RawMessage(`{"path":"project/a","nested":{"b":2,"a":1}}`)
	first, err := service.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.Arguments = json.RawMessage(` { "nested" : { "a":1, "b":2 }, "path":"project/a" } `)
	second, err := service.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.IntentID != second.IntentID || !second.Replay || first.IntentFingerprint != second.IntentFingerprint {
		t.Fatalf("exact Prepare replay mismatch: %#v %#v", first, second)
	}
	input.Arguments = json.RawMessage(`{"path":"project/b"}`)
	if _, err := service.Prepare(context.Background(), input); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("mismatched Prepare replay error = %v", err)
	}
	if repository.prepareCount != 3 || repository.effectCount != 0 {
		t.Fatalf("prepare/effect counts = %d/%d", repository.prepareCount, repository.effectCount)
	}
}

func TestPrepareBindsSubjectLeaseTTLAndKillSwitchEpoch(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	service, _, _ := testService(t, now, &fakeExecutor{})
	base := testPrepareInput(t, now, ApprovalOnce)

	mismatch := base
	mismatch.Grant.Subject.UserID = "123e4567-e89b-42d3-a456-426614174001"
	if _, err := service.Prepare(context.Background(), mismatch); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("subject mismatch error = %v", err)
	}

	first, err := service.Prepare(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*PrepareInput){
		func(value *PrepareInput) { value.TTL++ },
		func(value *PrepareInput) { value.Attempt.LeaseToken = "lease_abcdef0123456789abcdef01" },
		func(value *PrepareInput) { value.Attempt.KillSwitchEpoch++ },
	} {
		changed := base
		mutate(&changed)
		if _, err := service.Prepare(context.Background(), changed); !errors.Is(err, ErrReplayDetected) {
			t.Fatalf("changed replay error = %v for intent %s", err, first.IntentID)
		}
	}
}

func TestApprovalCannotMoveAcrossIntentOrReplayDifferently(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	service, _, _ := testService(t, now, &fakeExecutor{})
	firstInput := testPrepareInput(t, now, ApprovalOnce)
	first, err := service.Prepare(context.Background(), firstInput)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := testPrepareInput(t, now, ApprovalOnce)
	secondInput.RequestID = "request_fedcba9876543210"
	second, err := service.Prepare(context.Background(), secondInput)
	if err != nil {
		t.Fatal(err)
	}
	decision := ApprovalInput{ApprovalID: "approval_0123456789abcdef", UserID: testUser,
		IntentID: first.IntentID, IntentFingerprint: first.IntentFingerprint, Decision: "approved",
		ActorType: "user", ActorID: testUser, ReasonCode: "USER_APPROVED", ExpectedRevision: 1}
	if _, err := service.DecideApproval(context.Background(), decision); err != nil {
		t.Fatal(err)
	}
	moved := decision
	moved.IntentID, moved.IntentFingerprint = second.IntentID, second.IntentFingerprint
	if _, err := service.DecideApproval(context.Background(), moved); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("moved approval error = %v", err)
	}
	changed := decision
	changed.Decision = "denied"
	if _, err := service.DecideApproval(context.Background(), changed); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("changed approval replay error = %v", err)
	}
	forgedActor := decision
	forgedActor.ApprovalID = "approval_fedcba9876543210"
	forgedActor.ActorID = "99999999-9999-4999-8999-999999999999"
	if _, err := service.DecideApproval(context.Background(), forgedActor); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("forged user approval error = %v", err)
	}
}

func TestCommitDispatchesOnceAndExactReplayReturnsReceipt(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	executor := &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage, StatusToken: "status-one"}}
	service, repository, _ := testService(t, now, executor)
	prepared, err := service.Prepare(context.Background(), testPrepareInput(t, now, ApprovalAutomatic))
	if err != nil {
		t.Fatal(err)
	}
	commit := testCommitInput(prepared, testAttemptAuthority(t, now))
	first, err := service.Commit(context.Background(), commit)
	if err != nil || first.Outcome != OutcomeCommitted {
		t.Fatalf("first Commit = %#v, %v", first, err)
	}
	second, err := service.Commit(context.Background(), commit)
	if err != nil || second.Outcome != OutcomeReplayed || !second.Replay {
		t.Fatalf("replay Commit = %#v, %v", second, err)
	}
	if executor.commits != 1 || repository.effectCount != 1 {
		t.Fatalf("dispatch counts = executor:%d repository:%d", executor.commits, repository.effectCount)
	}
	commit.IntentFingerprint = testRuntime
	if _, err := service.Commit(context.Background(), commit); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("mismatched Commit error = %v", err)
	}
}

func TestCommitAcknowledgementLossUsesStatusOrOutcomeUnknown(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		status      ExecutorStatus
		statusErr   error
		wantOutcome string
		wantErr     error
	}{
		{name: "receipt recovered", status: ExecutorStatus{Outcome: OutcomeCommitted, ReceiptFingerprint: testPackage, StatusToken: "known"}, wantOutcome: OutcomeCommitted},
		{name: "unknown", statusErr: errors.New("status unavailable"), wantOutcome: OutcomeUnknown, wantErr: ErrOutcomeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := &fakeExecutor{commitErr: &DispatchError{Cause: errors.New("ack lost"), PossibleSend: true}, status: test.status, statusErr: test.statusErr}
			service, _, _ := testService(t, now, executor)
			prepared, err := service.Prepare(context.Background(), testPrepareInput(t, now, ApprovalAutomatic))
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Commit(context.Background(), testCommitInput(prepared, testAttemptAuthority(t, now)))
			if result.Outcome != test.wantOutcome || !errors.Is(err, test.wantErr) {
				t.Fatalf("Commit = %#v, %v", result, err)
			}
			if executor.commits != 1 || executor.statuses != 1 {
				t.Fatalf("executor calls = commit:%d status:%d", executor.commits, executor.statuses)
			}
		})
	}
}

func TestCommitConcurrencyClaimsOneDispatch(t *testing.T) {
	now := time.Date(2026, 8, 13, 1, 0, 0, 0, time.UTC)
	executor := &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage}}
	service, _, _ := testService(t, now, executor)
	prepared, err := service.Prepare(context.Background(), testPrepareInput(t, now, ApprovalAutomatic))
	if err != nil {
		t.Fatal(err)
	}
	input := testCommitInput(prepared, testAttemptAuthority(t, now))
	start := make(chan struct{})
	var group sync.WaitGroup
	errorsSeen := make(chan error, 2)
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := service.Commit(context.Background(), input)
			errorsSeen <- err
		}()
	}
	close(start)
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil && !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("concurrent Commit error = %v", err)
		}
	}
	if executor.commits != 1 {
		t.Fatalf("executor Commit count = %d", executor.commits)
	}
}

func testGrantAt(now time.Time) CapabilityGrant {
	return CapabilityGrant{SchemaVersion: GrantVersion, GrantID: testGrant,
		Subject: Subject{UserID: testUser, ProjectID: "project_01234567", AssistantID: "assistant_01234567"},
		Run:     RunBinding{RunID: testRun}, PackageFingerprint: testPackage,
		RuntimeBundleFingerprint: testRuntime, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour),
		Capabilities: []Capability{{Capability: "workspace.read", Actions: []string{"list", "read"},
			Resources: Selector{Kind: "prefix", Values: []string{"project/"}}, Approval: ApprovalAutomatic, MaxCalls: 100}},
		Egress: EgressPolicy{Mode: "none", Rules: []EgressRule{}}, Secrets: []SecretGrant{},
		Budget: Budget{MaxWallSeconds: 300, MaxModelTokens: 20000, MaxToolCalls: 100, MaxArtifactBytes: 8 << 20}}
}

func testAttemptAuthority(t *testing.T, now time.Time) AttemptAuthority {
	t.Helper()
	grant := testGrantAt(now)
	grantFingerprint, err := canonicalGrantFingerprint(grant)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"list", "read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	return AttemptAuthority{UserID: testUser, RunID: testRun, StepID: testStep, AttemptID: testAttempt,
		Generation: 1, LeaseOwner: "neo-runner-primary", LeaseToken: "lease_0123456789abcdef01234567",
		SnapshotFingerprint: testSnapshot, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: registry.Fingerprint, KillSwitchEpoch: 0}
}

func testPrepareInput(t *testing.T, now time.Time, approval string) PrepareInput {
	t.Helper()
	grant := testGrantAt(now)
	grant.Capabilities[0].Approval = approval
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"list", "read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, _ := canonicalGrantFingerprint(grant)
	attempt := AttemptAuthority{UserID: testUser, RunID: testRun, StepID: testStep, AttemptID: testAttempt,
		Generation: 1, LeaseOwner: "neo-runner-primary", LeaseToken: "lease_0123456789abcdef01234567",
		SnapshotFingerprint: testSnapshot, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: registry.Fingerprint, KillSwitchEpoch: 0}
	return PrepareInput{RequestID: "request_0123456789abcdef", Attempt: attempt, Grant: grant,
		Registry: registry, ToolIdentity: "workspace_read", Action: "read", Resource: "project/a",
		Arguments: json.RawMessage(`{"path":"project/a"}`), BaseRevision: "rev_01234567", TTL: 10 * time.Minute}
}

func testCommitInput(prepared PreparedIntent, attempt AttemptAuthority) CommitInput {
	return CommitInput{Attempt: attempt, IntentID: prepared.IntentID,
		IntentFingerprint: prepared.IntentFingerprint, IdempotencyKey: prepared.IdempotencyKey}
}

type memoryRepository struct {
	mu            sync.Mutex
	intents       map[string]PreparedIntent
	requests      map[string]string
	prepareCount  int
	effectCount   int
	approvals     map[string]ApprovalInput
	cancellations map[string]CancelInput
}

func (repository *memoryRepository) Cancel(_ context.Context, input CancelInput) (PreparedIntent, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.cancellations == nil {
		repository.cancellations = map[string]CancelInput{}
	}
	intent, ok := repository.intents[input.IntentID]
	if !ok || intent.Subject.UserID != input.UserID || intent.IntentFingerprint != input.IntentFingerprint {
		return PreparedIntent{}, ErrNotFound
	}
	if existing, ok := repository.cancellations[input.CancellationID]; ok {
		if existing != input {
			return PreparedIntent{}, ErrReplayDetected
		}
		if intent.State != IntentCanceled {
			return PreparedIntent{}, ErrInvalidTransition
		}
		return intent, nil
	}
	for _, existing := range repository.cancellations {
		if existing.IntentID == input.IntentID {
			return PreparedIntent{}, ErrReplayDetected
		}
	}
	if !member(intent.State, IntentAwaitingApproval, IntentApproved) {
		return PreparedIntent{}, ErrInvalidTransition
	}
	intent.State, intent.ErrorCode = IntentCanceled, "INTENT_CANCELED"
	repository.intents[input.IntentID] = intent
	repository.cancellations[input.CancellationID] = input
	return intent, nil
}

func (repository *memoryRepository) Prepare(_ context.Context, input PreparedIntent) (PreparedIntent, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.prepareCount++
	if fingerprint, ok := repository.requests[input.RequestID]; ok {
		for _, intent := range repository.intents {
			if intent.RequestID == input.RequestID {
				if fingerprint != input.RequestFingerprint {
					return PreparedIntent{}, ErrReplayDetected
				}
				intent.Replay = true
				return intent, nil
			}
		}
	}
	repository.requests[input.RequestID] = input.RequestFingerprint
	repository.intents[input.IntentID] = input
	return input, nil
}

func (repository *memoryRepository) DecideApproval(_ context.Context, input ApprovalInput) (PreparedIntent, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if existing, ok := repository.approvals[input.ApprovalID]; ok {
		if existing.IntentID != input.IntentID || existing.IntentFingerprint != input.IntentFingerprint ||
			existing.Decision != input.Decision || existing.ActorType != input.ActorType ||
			existing.ActorID != input.ActorID || existing.ReasonCode != input.ReasonCode ||
			existing.ExpectedRevision != input.ExpectedRevision || existing.UserID != input.UserID {
			return PreparedIntent{}, ErrReplayDetected
		}
		return repository.intents[input.IntentID], nil
	}
	intent, ok := repository.intents[input.IntentID]
	if !ok || intent.Subject.UserID != input.UserID || intent.IntentFingerprint != input.IntentFingerprint {
		return PreparedIntent{}, ErrApprovalDenied
	}
	if intent.ApprovalClass == ApprovalAutomatic {
		return PreparedIntent{}, ErrApprovalDenied
	}
	if intent.State != IntentAwaitingApproval {
		return PreparedIntent{}, ErrApprovalDenied
	}
	if input.Decision == "denied" {
		intent.State, intent.ErrorCode = IntentRejected, "APPROVAL_DENIED"
	} else {
		intent.State = IntentApproved
	}
	repository.intents[input.IntentID] = intent
	repository.approvals[input.ApprovalID] = input
	return intent, nil
}

func (repository *memoryRepository) ClaimCommit(_ context.Context, input CommitInput, tokenHash string) (CommitClaim, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	intent, ok := repository.intents[input.IntentID]
	if !ok {
		return CommitClaim{}, ErrNotFound
	}
	if intent.IntentFingerprint != input.IntentFingerprint || intent.IdempotencyKey != input.IdempotencyKey {
		return CommitClaim{}, ErrReplayDetected
	}
	if intent.AttemptID != input.Attempt.AttemptID || intent.Generation != input.Attempt.Generation ||
		intent.LeaseTokenDigest != tokenHash {
		return CommitClaim{}, ErrLeaseStale
	}
	if member(intent.State, IntentCommitted, IntentFailed, IntentRejected, IntentCanceled, IntentExpired, IntentOutcomeUnknown) {
		return CommitClaim{Intent: intent, Replay: true}, nil
	}
	if intent.State == IntentCommitting {
		return CommitClaim{}, ErrInvalidTransition
	}
	if intent.State != IntentApproved {
		return CommitClaim{}, ErrApprovalRequired
	}
	intent.State = IntentCommitting
	repository.intents[input.IntentID] = intent
	repository.effectCount++
	return CommitClaim{Intent: intent}, nil
}

func (repository *memoryRepository) CompleteCommit(_ context.Context, intentID, state, receipt, statusDigest, code string) (PreparedIntent, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	intent, ok := repository.intents[intentID]
	if !ok || intent.State != IntentCommitting {
		return PreparedIntent{}, ErrInvalidTransition
	}
	intent.State, intent.ReceiptFingerprint, intent.ExecutorStatusDigest, intent.ErrorCode = state, receipt, statusDigest, code
	repository.intents[intentID] = intent
	return intent, nil
}

func (repository *memoryRepository) RevokeGrant(_ context.Context, input GrantRevocationInput) (bool, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for id, intent := range repository.intents {
		if intent.GrantID == input.GrantID {
			intent.State = IntentRejected
			intent.ErrorCode = "GRANT_DENIED"
			repository.intents[id] = intent
		}
	}
	return true, nil
}

func (repository *memoryRepository) GetIntent(_ context.Context, userID, intentID string) (PreparedIntent, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	intent, ok := repository.intents[intentID]
	if !ok || intent.Subject.UserID != userID {
		return PreparedIntent{}, ErrNotFound
	}
	return intent, nil
}

func (repository *memoryRepository) ListCommitting(_ context.Context, limit int) ([]PreparedIntent, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	result := []PreparedIntent{}
	for _, intent := range repository.intents {
		if intent.State == IntentCommitting && len(result) < limit {
			result = append(result, intent)
		}
	}
	return result, nil
}

func (repository *memoryRepository) Expire(_ context.Context, cutoff time.Time, limit int) (int, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	count := 0
	for id, intent := range repository.intents {
		if count == limit {
			break
		}
		if member(intent.State, IntentPrepared, IntentAwaitingApproval, IntentApproved) && !intent.ExpiresAt.After(cutoff) {
			intent.State = IntentExpired
			repository.intents[id] = intent
			count++
		}
	}
	return count, nil
}

type fakeExecutor struct {
	mu        sync.Mutex
	commits   int
	statuses  int
	receipt   ExecutorReceipt
	commitErr error
	status    ExecutorStatus
	statusErr error
}

func (executor *fakeExecutor) Commit(_ context.Context, _ ExecutionRequest) (ExecutorReceipt, error) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.commits++
	return executor.receipt, executor.commitErr
}

func (executor *fakeExecutor) Status(_ context.Context, _ ExecutionRequest) (ExecutorStatus, error) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.statuses++
	return executor.status, executor.statusErr
}

func testService(t *testing.T, now time.Time, executor EffectExecutor) (*Service, *memoryRepository, CapabilityGrant) {
	t.Helper()
	repository := &memoryRepository{intents: map[string]PreparedIntent{}, requests: map[string]string{},
		approvals: map[string]ApprovalInput{}}
	service, err := NewService(repository, map[string]EffectExecutor{"workspace_read": executor})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }
	return service, repository, testGrantAt(now)
}
