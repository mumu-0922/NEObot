package agentorchestrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestPostgresAuthorityIdempotencyLeaseReclaimRecoveryAndKillSwitch(t *testing.T) {
	database := openAgentPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	userID := uuid.NewString()
	retainFixture := os.Getenv("MM_CHAT_AGENT_RETAIN_FIXTURE") == "1"
	mustAgentExec(t, ctx, database, `
INSERT INTO users(id,email,display_name) VALUES ($1,$2,'Agent orchestrator fixture')
`, userID, "agent-"+userID+"@example.test")
	t.Cleanup(func() {
		if retainFixture {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM agent_kill_switches WHERE actor_id='integration'`)
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM users WHERE id=$1`, userID)
	})

	repository := NewPostgresRepository(database)
	service := NewService(repository)
	enqueue := EnqueueInput{
		UserID: userID, IdempotencyKey: "agent-integration-enqueue",
		Snapshot:      json.RawMessage(`{"schemaVersion":"neo.agent-snapshot/v1","model":{"provider":"fixture","id":"fixture"},"budget":{"wallSeconds":30}}`),
		Steps:         []StepPlan{{Kind: "reason"}},
		ScopeBindings: []ScopeBinding{{Type: "skill", Value: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	}
	created, err := service.EnqueueRun(ctx, enqueue)
	if err != nil || !created.Created || len(created.Run.Steps) != 1 || created.Run.State != RunQueued {
		t.Fatalf("create Run = %#v, %v", created, err)
	}
	replayed, err := service.EnqueueRun(ctx, enqueue)
	if err != nil || replayed.Created || replayed.Run.ID != created.Run.ID {
		t.Fatalf("replay Run = %#v, %v", replayed, err)
	}
	drifted := enqueue
	drifted.Snapshot = json.RawMessage(`{"schemaVersion":"neo.agent-snapshot/v1","model":{"provider":"fixture","id":"changed"},"budget":{"wallSeconds":30}}`)
	if _, err := service.EnqueueRun(ctx, drifted); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency drift error = %v", err)
	}

	actor := Actor{Type: "orchestrator", ID: "integration"}
	lease, err := service.AcquireStep(ctx, AcquireInput{
		UserID: userID, RunID: created.Run.ID, StepID: created.Run.Steps[0].ID,
		LeaseOwner: "integration-worker", LeaseDuration: 5 * time.Second,
		Actor: actor, ReasonCode: "LEASE_ACQUIRED",
	})
	if err != nil || lease.Generation != 1 || lease.Token == "" {
		t.Fatalf("acquire lease = %#v, %v", lease, err)
	}
	if retainFixture {
		return
	}
	if err := service.TransitionAttempt(ctx, TransitionInput{
		UserID: userID, RunID: created.Run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, Expected: AttemptLeased, To: AttemptStarting,
		Actor: actor, ReasonCode: "ATTEMPT_STARTING",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.HeartbeatAttempt(ctx, HeartbeatInput{
		UserID: userID, RunID: created.Run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: "wrong-token", LeaseDuration: 30 * time.Second,
		Actor: actor, ReasonCode: "LEASE_HEARTBEAT",
	}); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("wrong heartbeat error = %v", err)
	}
	mustAgentExec(t, ctx, database, `
UPDATE agent_attempts SET lease_expires_at=created_at
WHERE id=$1
`, lease.ID)
	reclaimed, err := service.AcquireStep(ctx, AcquireInput{
		UserID: userID, RunID: created.Run.ID, StepID: lease.StepID,
		LeaseOwner: "integration-worker-2", LeaseDuration: 30 * time.Second,
		Actor: actor, ReasonCode: "LEASE_RECLAIMED",
	})
	if err != nil || reclaimed.Generation != 2 || reclaimed.ID == lease.ID {
		t.Fatalf("reclaimed lease = %#v, %v", reclaimed, err)
	}
	if err := service.TransitionAttempt(ctx, TransitionInput{
		UserID: userID, RunID: created.Run.ID, StepID: lease.StepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, Expected: AttemptStarting, To: AttemptRunning,
		Actor: actor, ReasonCode: "STALE_ATTEMPT",
	}); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("stale transition error = %v", err)
	}

	recovery, err := service.ListRecoveryRuns(ctx, 100)
	if err != nil || len(recovery) == 0 || recovery[0].RunID != created.Run.ID || recovery[0].Generation != 2 {
		t.Fatalf("recovery = %#v, %v", recovery, err)
	}

	racedInput := enqueue
	racedInput.IdempotencyKey = "agent-integration-race"
	raced, err := service.EnqueueRun(ctx, racedInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.TransitionStep(ctx, TransitionInput{
		UserID: userID, RunID: raced.Run.ID, StepID: raced.Run.Steps[0].ID,
		Expected: StepReady, To: StepFailed, Actor: actor, ReasonCode: "RACE_SETUP_FAILED",
	}); err != nil {
		t.Fatal(err)
	}
	transitionErrors := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			transitionErrors <- service.TransitionRun(ctx, TransitionInput{
				UserID: userID, RunID: raced.Run.ID, Expected: RunQueued, To: RunFailed,
				Actor: actor, ReasonCode: "RACE_TERMINAL",
			})
		}()
	}
	wait.Wait()
	close(transitionErrors)
	succeeded, rejected := 0, 0
	for transitionErr := range transitionErrors {
		if transitionErr == nil {
			succeeded++
		} else if errors.Is(transitionErr, ErrInvalidTransition) {
			rejected++
		} else {
			t.Fatalf("race transition error = %v", transitionErr)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("race transition winners=%d rejected=%d", succeeded, rejected)
	}
	if err := service.ObserveTerminalConflict(ctx, TransitionInput{
		UserID: userID, RunID: raced.Run.ID, Expected: RunFailed, To: RunOutcomeUnknown,
		Actor: actor, ReasonCode: "CONFLICTING_OBSERVATION",
		Detail: map[string]any{"observationCode": "LATE_OUTCOME"},
	}); err != nil {
		t.Fatal(err)
	}
	var eventCount, maxSequence, nextSequence int64
	var tracedState string
	if err := database.QueryRowContext(ctx, `
SELECT count(*),max(event.sequence),run.next_sequence,run.state
FROM agent_runs run JOIN agent_run_events event ON event.run_id=run.id
WHERE run.id=$1 GROUP BY run.next_sequence,run.state
`, raced.Run.ID).Scan(&eventCount, &maxSequence, &nextSequence, &tracedState); err != nil {
		t.Fatal(err)
	}
	if eventCount != 8 || maxSequence != 8 || nextSequence != 9 || tracedState != RunFailed {
		t.Fatalf("event race projection count=%d max=%d next=%d state=%s", eventCount, maxSequence, nextSequence, tracedState)
	}

	global, err := service.AppendKillSwitch(ctx, KillSwitchInput{
		ScopeType: "global", ScopeValue: "*", Mode: KillDenyNew, Active: true,
		Actor: Actor{Type: "operator", ID: "integration"}, ReasonCode: "INCIDENT",
	})
	if err != nil || global.Revision != 1 {
		t.Fatalf("global switch = %#v, %v", global, err)
	}
	runSwitch, err := service.AppendKillSwitch(ctx, KillSwitchInput{
		ScopeType: "run", ScopeValue: created.Run.ID, Mode: KillKill, Active: true,
		Actor: Actor{Type: "operator", ID: "integration"}, ReasonCode: "RUNAWAY",
	})
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := service.ResolveKillSwitch(ctx, userID, created.Run.ID)
	if err != nil || !resolution.Active || resolution.Mode != KillKill || len(resolution.SwitchIDs) != 2 {
		t.Fatalf("kill resolution = %#v, %v", resolution, err)
	}
	if _, err := service.AcquireStep(ctx, AcquireInput{
		UserID: userID, RunID: created.Run.ID, StepID: lease.StepID,
		LeaseOwner: "denied", LeaseDuration: 30 * time.Second,
		Actor: actor, ReasonCode: "DENIED_LEASE",
	}); !errors.Is(err, ErrKillSwitchActive) {
		t.Fatalf("Kill Switch acquire error = %v", err)
	}
	if _, err := service.AppendKillSwitch(ctx, KillSwitchInput{
		ID: runSwitch.ID, ScopeType: "run", ScopeValue: created.Run.ID,
		Mode: KillKill, Active: false, ExpectedRevision: 0,
		Actor: Actor{Type: "operator", ID: "integration"}, ReasonCode: "STALE_DISABLE",
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale switch revision error = %v", err)
	}

	before, err := service.GetRun(ctx, userID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	mustAgentExec(t, ctx, database, `
UPDATE agent_runs SET state='pending',next_sequence=1,terminal_at=NULL WHERE id=$1;
UPDATE agent_steps SET state='pending',current_generation=0,terminal_at=NULL WHERE run_id=$1;
UPDATE agent_attempts SET state='failed',terminal_at=clock_timestamp() WHERE run_id=$1;
`, created.Run.ID)
	if err := service.RebuildProjection(ctx, userID, created.Run.ID); err != nil {
		t.Fatal(err)
	}
	after, err := service.GetRun(ctx, userID, created.Run.ID)
	if err != nil || after.State != before.State || after.NextSequence != before.NextSequence ||
		after.Steps[0].CurrentGeneration != before.Steps[0].CurrentGeneration ||
		after.Attempts[len(after.Attempts)-1].State != before.Attempts[len(before.Attempts)-1].State {
		t.Fatalf("rebuilt=%#v before=%#v error=%v", after, before, err)
	}

	if err := service.TransitionAttempt(ctx, TransitionInput{
		UserID: userID, RunID: created.Run.ID, StepID: reclaimed.StepID,
		AttemptID: reclaimed.ID, Generation: reclaimed.Generation,
		LeaseOwner: reclaimed.LeaseOwner, LeaseToken: reclaimed.Token,
		Expected: AttemptLeased, To: AttemptFailed,
		Actor: actor, ReasonCode: "FIXTURE_FAILED",
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.TransitionStep(ctx, TransitionInput{
		UserID: userID, RunID: created.Run.ID, StepID: reclaimed.StepID,
		AttemptID: reclaimed.ID, Generation: reclaimed.Generation,
		LeaseOwner: reclaimed.LeaseOwner, LeaseToken: reclaimed.Token,
		Expected: StepRunning, To: StepFailed, Actor: actor, ReasonCode: "FIXTURE_FAILED",
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.TransitionRun(ctx, TransitionInput{
		UserID: userID, RunID: created.Run.ID, Expected: RunRunning, To: RunFailed,
		Actor: actor, ReasonCode: "FIXTURE_FAILED",
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	pruned, err := service.PruneTerminalRuns(ctx, time.Now().UTC(), 100)
	if err != nil || pruned != 2 {
		t.Fatalf("PruneTerminalRuns() = %d, %v", pruned, err)
	}
	if _, err := service.GetRun(ctx, userID, created.Run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pruned Run lookup error = %v", err)
	}
}

func openAgentPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Agent Orchestrator Postgres integration tests")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	database := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = database.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("ping Agent Orchestrator database: %v", err)
	}
	return database
}

func mustAgentExec(t *testing.T, ctx context.Context, database *sql.DB, query string, arguments ...any) {
	t.Helper()
	if _, err := database.ExecContext(ctx, query, arguments...); err != nil {
		t.Fatalf("execute Agent Orchestrator fixture: %v", err)
	}
}
