package agentrootcanary

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
	"neo-chat/mm-chat/backend/internal/agentrunner"
)

func TestPostgresTerminalRepositoryCommitsWholeCanceledChainOrRollsBack(t *testing.T) {
	admin := openRootCanaryPostgres(t, "MM_CHAT_TEST_DATABASE_URL")
	runtime := openRootCanaryPostgres(t, "MM_CHAT_AGENT_ROOT_CANARY_DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := uuid.NewString()
	if _, err := admin.ExecContext(ctx, `
INSERT INTO users(id,email,display_name) VALUES ($1,$2,'Root canary fixture')
`, userID, "root-canary-"+userID+"@example.test"); err != nil {
		t.Fatalf("create Root canary user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = admin.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, userID)
	})

	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(runtime))
	enqueued, err := orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{
		UserID: userID, IdempotencyKey: "g21.1-root-canary-postgres-" + userID,
		Snapshot: json.RawMessage(`{"schemaVersion":"neo.agent-snapshot/v1","mode":"synthetic","noEgress":true,"noSecrets":true}`),
		Steps:    []agentorchestrator.StepPlan{{Kind: "root_canary"}},
		ScopeBindings: []agentorchestrator.ScopeBinding{
			{Type: "runner", Value: "neo-runner-primary"},
			{Type: "skill", Value: fp('2')},
		},
	})
	if err != nil || !enqueued.Created || len(enqueued.Run.Steps) != 1 {
		t.Fatalf("enqueue Root canary = %#v, %v", enqueued, err)
	}
	actor := agentorchestrator.Actor{Type: "orchestrator", ID: "g21.1-root-canary"}
	lease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{
		UserID: userID, RunID: enqueued.Run.ID, StepID: enqueued.Run.Steps[0].ID,
		LeaseOwner: "neo-runner-primary", LeaseDuration: time.Minute,
		Actor: actor, ReasonCode: "ROOT_CANARY_CLAIMED",
	})
	if err != nil {
		t.Fatal(err)
	}
	transition := agentorchestrator.TransitionInput{
		UserID: userID, RunID: lease.RunID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, Actor: actor,
	}
	transition.Expected, transition.To, transition.ReasonCode =
		agentorchestrator.AttemptLeased, agentorchestrator.AttemptStarting, "ROOT_CANARY_STARTING"
	if err := orchestrator.TransitionAttempt(ctx, transition); err != nil {
		t.Fatal(err)
	}
	transition.Expected, transition.To, transition.ReasonCode =
		agentorchestrator.AttemptStarting, agentorchestrator.AttemptRunning, "ROOT_CANARY_RUNNING"
	if err := orchestrator.TransitionAttempt(ctx, transition); err != nil {
		t.Fatal(err)
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	runnerRepository := agentrunner.NewPostgresControlRepository(runtime)
	authority, err := agentrunner.NewControlService(runnerRepository, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	requestID := "rpc_0123456789abcdef"
	nonce := "rootcanarypostgresnonce0123456789"
	attempt := agentrunner.AttemptRef{RunID: lease.RunID, StepID: lease.StepID,
		AttemptID: lease.ID, LeaseGeneration: lease.Generation,
		LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token}
	if _, replay, err := authority.IssueAuthority(ctx, agentrunner.IssueAuthorityInput{
		CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary",
		RunnerID:       "neo-runner-primary", RequestID: requestID, Nonce: nonce,
		Method: agentrunner.MethodLaunch, RequestFingerprint: fp('a'), UserID: userID,
		Attempt: attempt, SnapshotFingerprint: enqueued.Run.SnapshotFingerprint,
		TTL: 15 * time.Second,
	}); err != nil || len(replay) != 0 {
		t.Fatalf("issue launch authority = %q, %v", replay, err)
	}
	sandboxID := "sandbox_0123456789abcdef"
	expected := agentrunner.ExpectedSandbox{
		SandboxID: sandboxID, CallerIdentity: "spiffe://neo-chat/agent-runtime-root-canary",
		RequestID: requestID, Nonce: nonce, UserID: userID, Attempt: attempt.Identity(),
		RunnerID: "neo-runner-primary", SnapshotFingerprint: enqueued.Run.SnapshotFingerprint,
		SpecFingerprint: fp('b'), ProbeFingerprint: fp('c'),
	}
	if err := runnerRepository.ExpectSandbox(ctx, expected); err != nil {
		t.Fatal(err)
	}
	if err := runnerRepository.UpdateSandbox(ctx, sandboxID, lease.ID, lease.Generation,
		"expected", "starting", ""); err != nil {
		t.Fatal(err)
	}
	if err := runnerRepository.UpdateSandbox(ctx, sandboxID, lease.ID, lease.Generation,
		"starting", "running", ""); err != nil {
		t.Fatal(err)
	}

	repository := NewPostgresTerminalRepository(runtime)
	input := TerminalInput{UserID: userID, RunID: lease.RunID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, SandboxID: "sandbox_ffffffffffffffff"}
	if err := repository.FinalizeCanceled(ctx, input); err == nil {
		t.Fatal("FinalizeCanceled accepted the wrong Sandbox identity")
	}
	assertRootCanaryStates(t, ctx, admin, lease.RunID, lease.StepID, lease.ID, sandboxID,
		"running", "running", "running", "running", 0, "")

	input.SandboxID = sandboxID
	if err := repository.FinalizeCanceled(ctx, input); err != nil {
		t.Fatal(err)
	}
	assertRootCanaryStates(t, ctx, admin, lease.RunID, lease.StepID, lease.ID, sandboxID,
		"canceled", "canceled", "canceled", "terminal", 3, "attempt,step,run")
}

func openRootCanaryPostgres(t *testing.T, name string) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv(name)
	if databaseURL == "" {
		t.Skipf("set %s to run Root canary PostgreSQL integration tests", name)
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	database := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("ping %s: %v", name, err)
	}
	return database
}

func assertRootCanaryStates(t *testing.T, ctx context.Context, database *sql.DB,
	runID, stepID, attemptID, sandboxID, wantAttempt, wantStep, wantRun, wantSandbox string,
	wantEvents int, wantEntities string,
) {
	t.Helper()
	var attemptState, stepState, runState, sandboxState, entities string
	var events int
	err := database.QueryRowContext(ctx, `
SELECT attempt.state,step.state,run.state,sandbox.state,
  count(event.id) FILTER (WHERE event.reason_code='ROOT_CANARY_CANCELED'),
  COALESCE(string_agg(event.entity,',' ORDER BY event.sequence)
    FILTER (WHERE event.reason_code='ROOT_CANARY_CANCELED'),'')
FROM agent_attempts attempt
JOIN agent_steps step ON step.id=attempt.step_id
JOIN agent_runs run ON run.id=attempt.run_id
JOIN agent_runner_sandboxes sandbox ON sandbox.attempt_id=attempt.id
LEFT JOIN agent_run_events event ON event.run_id=run.id
WHERE run.id=$1 AND step.id=$2 AND attempt.id=$3 AND sandbox.sandbox_id=$4
GROUP BY attempt.state,step.state,run.state,sandbox.state
`, runID, stepID, attemptID, sandboxID).Scan(
		&attemptState, &stepState, &runState, &sandboxState, &events, &entities)
	if err != nil {
		t.Fatal(err)
	}
	if attemptState != wantAttempt || stepState != wantStep || runState != wantRun ||
		sandboxState != wantSandbox || events != wantEvents || entities != wantEntities {
		t.Fatalf("states attempt=%s step=%s run=%s sandbox=%s events=%d entities=%q",
			attemptState, stepState, runState, sandboxState, events, entities)
	}
}
