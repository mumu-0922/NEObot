package agentbroker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

func TestPostgresAgentBrokerPrepareApprovalCommitReplayAndFences(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx := context.Background()
	userID := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	idempotencyKey := "agent-broker-test"
	retainFixture := os.Getenv("MM_CHAT_AGENT_BROKER_RETAIN_FIXTURE") != ""
	if retainFixture {
		userID = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		idempotencyKey = "agent-broker-retained"
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Broker Test') ON CONFLICT(id) DO NOTHING`, userID)

	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	run, lease := preparePostgresBrokerAttempt(t, ctx, orchestrator, userID, idempotencyKey)
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.Subject.UserID = userID
	grant.Run.RunID = run.ID
	grant.Capabilities[0].Approval = ApprovalOnce
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"list", "read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, _ := canonicalGrantFingerprint(grant)
	authority := AttemptAuthority{UserID: userID, RunID: run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, SnapshotFingerprint: run.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint,
		KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, db)}
	input := PrepareInput{RequestID: "request_aaaaaaaaaaaaaaaa", Attempt: authority,
		Grant: grant, Registry: registry, ToolIdentity: "workspace_read", Action: "read",
		Resource: "project/a", Arguments: json.RawMessage(`{"path":"project/a"}`),
		BaseRevision: "rev_01234567", TTL: 10 * time.Minute}
	if retainFixture {
		input.RequestID = "request_eeeeeeeeeeeeeeee"
	}

	executor := &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage, StatusToken: "postgres-status"}}
	service, err := NewService(NewPostgresRepository(db), map[string]EffectExecutor{"workspace_read": executor})
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Prepare(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Prepare(ctx, input)
	if err != nil || !replayed.Replay || replayed.IntentID != first.IntentID {
		t.Fatalf("Prepare replay = %#v, %v", replayed, err)
	}
	changed := input
	changed.TTL++
	if _, err := service.Prepare(ctx, changed); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("Prepare mismatch = %v", err)
	}

	approvalID := "approval_aaaaaaaaaaaaaaaa"
	if retainFixture {
		approvalID = "approval_eeeeeeeeeeeeeeee"
	}
	decision := ApprovalInput{ApprovalID: approvalID, UserID: userID,
		IntentID: first.IntentID, IntentFingerprint: first.IntentFingerprint, Decision: "approved",
		ActorType: "user", ActorID: userID, ReasonCode: "USER_APPROVED", ExpectedRevision: 1}
	approved, err := service.DecideApproval(ctx, decision)
	if err != nil || approved.State != IntentApproved {
		t.Fatalf("approval = %#v, %v", approved, err)
	}
	if _, err := service.DecideApproval(ctx, decision); err != nil {
		t.Fatalf("approval replay = %v", err)
	}

	commit := testCommitInput(first, authority)
	commit.ApprovalID = decision.ApprovalID
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, commitErr := service.Commit(ctx, commit)
			errorsSeen <- commitErr
		}()
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	for commitErr := range errorsSeen {
		if commitErr != nil && !errors.Is(commitErr, ErrInvalidTransition) {
			t.Fatalf("concurrent Commit = %v", commitErr)
		}
	}
	if executor.commits != 1 {
		t.Fatalf("executor Commit count = %d", executor.commits)
	}
	terminal, err := service.Commit(ctx, commit)
	if err != nil || terminal.Outcome != OutcomeReplayed {
		t.Fatalf("terminal replay = %#v, %v", terminal, err)
	}
	if retainFixture {
		return
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2`, run.ID, run.SnapshotID)
}

func TestPostgresAgentBrokerBudgetKillAndStaleLeasePerformZeroDispatch(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, _ := pgx.ParseConfig(databaseURL)
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx := context.Background()
	userID := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Broker Fence Test') ON CONFLICT(id) DO NOTHING`, userID)
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	run, lease := preparePostgresBrokerAttempt(t, ctx, orchestrator, userID, "agent-broker-fence-test")
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.Subject.UserID, grant.Run.RunID, grant.Budget.MaxToolCalls, grant.Capabilities[0].MaxCalls = userID, run.ID, 1, 1
	registry, _ := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"read"}, Classification: ClassificationRead, Idempotent: true}}, []string{"workspace_read"}, grant, now)
	grantFingerprint, _ := canonicalGrantFingerprint(grant)
	authority := AttemptAuthority{UserID: userID, RunID: run.ID, StepID: lease.StepID, AttemptID: lease.ID,
		Generation: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token,
		SnapshotFingerprint: run.SnapshotFingerprint, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: registry.Fingerprint, KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, db)}
	executor := &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage}}
	service, _ := NewService(NewPostgresRepository(db), map[string]EffectExecutor{"workspace_read": executor})
	base := PrepareInput{RequestID: "request_bbbbbbbbbbbbbbbb", Attempt: authority, Grant: grant, Registry: registry,
		ToolIdentity: "workspace_read", Action: "read", Resource: "project/a", Arguments: json.RawMessage(`{}`), TTL: 5 * time.Minute}
	prepared, err := service.Prepare(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	second := base
	second.RequestID = "request_cccccccccccccccc"
	if _, err := service.Prepare(ctx, second); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("budget fence = %v", err)
	}
	stale := testCommitInput(prepared, authority)
	stale.Attempt.LeaseToken = "lease_abcdef0123456789abcdef01"
	if _, err := service.Commit(ctx, stale); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("stale lease fence = %v", err)
	}
	if executor.commits != 0 {
		t.Fatalf("stale lease dispatched %d times", executor.commits)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2`, run.ID, run.SnapshotID)
}

func TestPostgresAgentBrokerSecretAuthorityAndKillRevocation(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx := context.Background()
	userID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Broker Secret Test') ON CONFLICT(id) DO NOTHING`, userID)
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	run, lease := preparePostgresBrokerAttempt(t, ctx, orchestrator, userID, "agent-broker-secret-test")
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.Subject.UserID, grant.Run.RunID = userID, run.ID
	grant.Secrets = []SecretGrant{{Slot: "workspace", BrokerRef: "secret_ref_0123456789abcdef",
		Actions: []string{"read"}, TTLSeconds: 300}}
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"list", "read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, _ := canonicalGrantFingerprint(grant)
	authority := AttemptAuthority{UserID: userID, RunID: run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, SnapshotFingerprint: run.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint,
		KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, db)}
	repository := NewPostgresRepository(db)
	service, _ := NewService(repository, nil)
	prepared, err := service.Prepare(ctx, PrepareInput{RequestID: "request_dddddddddddddddd",
		Attempt: authority, Grant: grant, Registry: registry, ToolIdentity: "workspace_read",
		Action: "read", Resource: "project/a", Arguments: json.RawMessage(`{"path":"project/a"}`),
		TTL: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	binding := SecretBinding{Subject: grant.Subject, RunID: run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, Capability: prepared.Capability,
		Action: prepared.Action, DestinationFingerprint: testPackage,
		IntentFingerprint: prepared.IntentFingerprint}
	resolutions := 0
	secretBroker, _ := NewSecretBroker(repository, secretResolverFunc(func(context.Context, string, Subject) ([]byte, error) {
		resolutions++
		return []byte("G204_POSTGRES_SECRET_CANARY"), nil
	}))
	if _, err := secretBroker.Issue(ctx, prepared, registry, grant.Secrets[0].BrokerRef,
		binding, time.Minute); !errors.Is(err, ErrSecretDenied) || resolutions != 0 {
		t.Fatalf("pre-Commit secret Issue = %v, resolutions=%d", err, resolutions)
	}
	claim, err := repository.ClaimCommit(ctx, testCommitInput(prepared, authority), tokenDigest(authority.LeaseToken))
	if err != nil || claim.Replay || claim.Intent.State != IntentCommitting {
		t.Fatalf("Commit claim = %#v, %v", claim, err)
	}
	handle, err := secretBroker.Issue(ctx, claim.Intent, registry, grant.Secrets[0].BrokerRef,
		binding, time.Minute)
	if err != nil || resolutions != 1 {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) {
			t.Fatalf("secret Issue = %q, %v, detail=%q where=%q, resolutions=%d",
				handle, err, pgError.Detail, pgError.Where, resolutions)
		}
		t.Fatalf("secret Issue = %q, %v, resolutions=%d", handle, err, resolutions)
	}
	value, err := secretBroker.Consume(ctx, handle, binding)
	if err != nil || string(value) != "G204_POSTGRES_SECRET_CANARY" {
		t.Fatalf("secret Consume = %q, %v", value, err)
	}
	clear(value)
	handle, err = secretBroker.Issue(ctx, claim.Intent, registry, grant.Secrets[0].BrokerRef,
		binding, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.AppendKillSwitch(ctx, agentorchestrator.KillSwitchInput{
		ScopeType: "run", ScopeValue: run.ID, Mode: agentorchestrator.KillKill, Active: true,
		Actor:      agentorchestrator.Actor{Type: "operator", ID: "agent-broker-test"},
		ReasonCode: "BROKER_SECRET_TEST",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := secretBroker.Consume(ctx, handle, binding); !errors.Is(err, ErrKillSwitchActive) {
		t.Fatalf("Kill Switch secret Consume = %v", err)
	}
	if _, err := repository.CompleteCommit(ctx, claim.Intent.IntentID, IntentFailed, "", "", "EXECUTOR_BLOCKED"); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM agent_secret_handles WHERE intent_id=$1 AND state='active'`,
		claim.Intent.IntentID).Scan(&active); err != nil || active != 0 {
		t.Fatalf("active secret handles = %d, %v", active, err)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2`, run.ID, run.SnapshotID)
}

func TestPostgresAgentBrokerOutcomeUnknownTerminalizesRunAndStep(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx := context.Background()
	userID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Broker Unknown Test') ON CONFLICT(id) DO NOTHING`, userID)
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	run, lease := preparePostgresBrokerAttempt(t, ctx, orchestrator, userID, "agent-broker-outcome-unknown-test")
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.Subject.UserID, grant.Run.RunID = userID, run.ID
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, _ := canonicalGrantFingerprint(grant)
	authority := AttemptAuthority{UserID: userID, RunID: run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, SnapshotFingerprint: run.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint,
		KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, db)}
	executor := &fakeExecutor{commitErr: &DispatchError{Cause: errors.New("ack lost"), PossibleSend: true},
		statusErr: errors.New("status unavailable")}
	service, err := NewService(NewPostgresRepository(db), map[string]EffectExecutor{"workspace_read": executor})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Prepare(ctx, PrepareInput{RequestID: "request_ffffffffffffffff",
		Attempt: authority, Grant: grant, Registry: registry, ToolIdentity: "workspace_read",
		Action: "read", Resource: "project/a", Arguments: json.RawMessage(`{"path":"project/a"}`),
		TTL: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Commit(ctx, testCommitInput(prepared, authority))
	if !errors.Is(err, ErrOutcomeUnknown) || result.Outcome != OutcomeUnknown || executor.commits != 1 || executor.statuses != 1 {
		t.Fatalf("Commit = %#v, %v, calls=%d/%d", result, err, executor.commits, executor.statuses)
	}
	var runState, stepState, attemptState string
	if err := db.QueryRowContext(ctx, `SELECT run.state,step.state,attempt.state
FROM agent_runs run JOIN agent_steps step ON step.run_id=run.id
JOIN agent_attempts attempt ON attempt.step_id=step.id
WHERE run.id=$1 AND step.id=$2 AND attempt.id=$3`, run.ID, lease.StepID, lease.ID).
		Scan(&runState, &stepState, &attemptState); err != nil {
		t.Fatal(err)
	}
	if runState != "outcome_unknown" || stepState != "outcome_unknown" || attemptState != "outcome_unknown" {
		t.Fatalf("terminal states = run:%s step:%s attempt:%s", runState, stepState, attemptState)
	}
	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM agent_run_events
WHERE run_id=$1 AND reason_code IN ('EFFECT_COMPLETED','EFFECT_OUTCOME_UNKNOWN')`, run.ID).Scan(&eventCount); err != nil || eventCount != 3 {
		t.Fatalf("terminal event count = %d, %v", eventCount, err)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2`, run.ID, run.SnapshotID)
}

func TestPostgresAgentBrokerRevokedGrantBlocksCommitBeforeDispatch(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx := context.Background()
	userID := "99999999-9999-4999-8999-999999999999"
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Broker Revoke Test') ON CONFLICT(id) DO NOTHING`, userID)
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	run, lease := preparePostgresBrokerAttempt(t, ctx, orchestrator, userID, "agent-broker-grant-revoke-test")
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.GrantID = "grant_8888888888888888"
	grant.Subject.UserID, grant.Run.RunID = userID, run.ID
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, _ := canonicalGrantFingerprint(grant)
	authority := AttemptAuthority{UserID: userID, RunID: run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, SnapshotFingerprint: run.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint,
		KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, db)}
	executor := &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage}}
	service, err := NewService(NewPostgresRepository(db), map[string]EffectExecutor{"workspace_read": executor})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Prepare(ctx, PrepareInput{RequestID: "request_9999999999999999",
		Attempt: authority, Grant: grant, Registry: registry, ToolIdentity: "workspace_read",
		Action: "read", Resource: "project/a", Arguments: json.RawMessage(`{"path":"project/a"}`),
		TTL: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	revocation := GrantRevocationInput{GrantID: grant.GrantID, GrantFingerprint: grantFingerprint,
		ActorType: "operator", ActorID: "agent-broker-test", ReasonCode: "OPERATOR_REVOKED"}
	created, err := service.RevokeGrant(ctx, revocation)
	if err != nil || !created {
		t.Fatalf("RevokeGrant = %v, %v", created, err)
	}
	created, err = service.RevokeGrant(ctx, revocation)
	if err != nil || created {
		t.Fatalf("RevokeGrant replay = %v, %v", created, err)
	}
	if _, err := service.Commit(ctx, testCommitInput(prepared, authority)); !errors.Is(err, ErrGrantDenied) {
		t.Fatalf("revoked Grant Commit = %v", err)
	}
	if executor.commits != 0 || executor.statuses != 0 {
		t.Fatalf("revoked Grant dispatched %d/%d times", executor.commits, executor.statuses)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2`, run.ID, run.SnapshotID)
}

func TestPostgresAgentBrokerCancelAndCommitRaceHasOneWinner(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx := context.Background()
	userID := "77777777-7777-4777-8777-777777777777"
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Broker Cancel Race Test') ON CONFLICT(id) DO NOTHING`, userID)
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	run, lease := preparePostgresBrokerAttempt(t, ctx, orchestrator, userID, "agent-broker-cancel-race-test")
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.Subject.UserID, grant.Run.RunID = userID, run.ID
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "workspace_read", Capability: "workspace.read",
		Actions: []string{"read"}, Classification: ClassificationRead, Idempotent: true}},
		[]string{"workspace_read"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	grantFingerprint, _ := canonicalGrantFingerprint(grant)
	authority := AttemptAuthority{UserID: userID, RunID: run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, SnapshotFingerprint: run.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint,
		KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, db)}
	executor := &fakeExecutor{receipt: ExecutorReceipt{ReceiptFingerprint: testPackage}}
	service, err := NewService(NewPostgresRepository(db), map[string]EffectExecutor{"workspace_read": executor})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Prepare(ctx, PrepareInput{RequestID: "request_7777777777777777",
		Attempt: authority, Grant: grant, Registry: registry, ToolIdentity: "workspace_read",
		Action: "read", Resource: "project/a", Arguments: json.RawMessage(`{"path":"project/a"}`),
		TTL: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	cancel := CancelInput{CancellationID: "cancellation_7777777777777777", UserID: userID,
		IntentID: prepared.IntentID, IntentFingerprint: prepared.IntentFingerprint,
		ActorType: "user", ActorID: userID, ReasonCode: "USER_CANCELED"}
	start := make(chan struct{})
	cancelResult := make(chan error, 1)
	commitResult := make(chan error, 1)
	go func() {
		<-start
		_, raceErr := service.Cancel(ctx, cancel)
		cancelResult <- raceErr
	}()
	go func() {
		<-start
		_, raceErr := service.Commit(ctx, testCommitInput(prepared, authority))
		commitResult <- raceErr
	}()
	close(start)
	cancelErr, commitErr := <-cancelResult, <-commitResult
	if cancelErr == nil {
		if commitErr != nil {
			t.Fatalf("Cancel won but Commit replay failed: %v", commitErr)
		}
		if executor.commits != 0 {
			t.Fatalf("Cancel winner dispatched %d effects", executor.commits)
		}
	} else {
		if !errors.Is(cancelErr, ErrInvalidTransition) || commitErr != nil || executor.commits != 1 {
			t.Fatalf("Commit winner = cancel:%v commit:%v dispatches:%d", cancelErr, commitErr, executor.commits)
		}
	}
	var intentState, attemptState string
	if err := db.QueryRowContext(ctx, `SELECT intent.state,attempt.state
FROM agent_effect_intents intent JOIN agent_attempts attempt ON attempt.id=intent.attempt_id
WHERE intent.id=$1`, prepared.IntentID).Scan(&intentState, &attemptState); err != nil {
		t.Fatal(err)
	}
	if cancelErr == nil && (intentState != IntentCanceled || attemptState != IntentCanceled) {
		t.Fatalf("Cancel terminal states = intent:%s attempt:%s", intentState, attemptState)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2`, run.ID, run.SnapshotID)
}

func preparePostgresBrokerAttempt(t *testing.T, ctx context.Context, service *agentorchestrator.Service, userID, key string) (agentorchestrator.Run, agentorchestrator.Lease) {
	t.Helper()
	enqueued, err := service.EnqueueRun(ctx, agentorchestrator.EnqueueInput{UserID: userID,
		IdempotencyKey: key, Snapshot: json.RawMessage(`{"snapshot":"broker-test"}`),
		Steps: []agentorchestrator.StepPlan{{Kind: "execute"}}})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := service.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID,
		RunID: enqueued.Run.ID, StepID: enqueued.Run.Steps[0].ID, LeaseOwner: "neo-runner-broker-test",
		LeaseDuration: 20 * time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "broker-test"},
		ReasonCode: "BROKER_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	transition := agentorchestrator.TransitionInput{UserID: userID, RunID: lease.RunID,
		StepID: lease.StepID, AttemptID: lease.ID, Generation: lease.Generation,
		LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token, Actor: agentorchestrator.Actor{Type: "runner", ID: lease.LeaseOwner},
		ReasonCode: "BROKER_TEST"}
	transition.Expected, transition.To = "leased", "starting"
	if err := service.TransitionAttempt(ctx, transition); err != nil {
		t.Fatal(err)
	}
	transition.Expected, transition.To = "starting", "running"
	if err := service.TransitionAttempt(ctx, transition); err != nil {
		t.Fatal(err)
	}
	return enqueued.Run, lease
}

func currentPostgresKillSwitchEpoch(t *testing.T, ctx context.Context, db *sql.DB) int64 {
	t.Helper()
	var epoch int64
	if err := db.QueryRowContext(ctx, `SELECT epoch FROM agent_kill_switch_state WHERE singleton`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	return epoch
}
