package agentdelegation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

// TestPostgresAgentChildReapTransport proves the migration-093 production
// bridge through the exact function-only G21.4 LOGIN. The administrator
// connection is used only to seed the synthetic User and inject one deliberate
// Runner projection mismatch that a Runtime principal cannot create.
func TestPostgresAgentChildReapTransport(t *testing.T) {
	adminURL := os.Getenv("MM_CHAT_AGENT_CHILD_CANARY_DATABASE_URL")
	runtimeURL := os.Getenv("MM_CHAT_AGENT_CHILD_CANARY_RUNTIME_DATABASE_URL")
	if adminURL == "" || runtimeURL == "" {
		t.Skip("Agent Child canary PostgreSQL URLs are unset")
	}
	admin := openChildCanaryPostgres(t, adminURL)
	defer admin.Close()
	runtime := openChildCanaryPostgres(t, runtimeURL)
	defer runtime.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	const userID = "00000000-0000-4000-8000-000000000004"
	if _, err := admin.ExecContext(ctx, `
INSERT INTO users(id,display_name) VALUES($1,'G21.4 Child canary')
ON CONFLICT(id) DO NOTHING`, userID); err != nil {
		t.Fatal(err)
	}

	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(runtime))
	rootResult, err := orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{
		UserID: userID, IdempotencyKey: "g21.4-child-canary-parent-postgres",
		Snapshot: json.RawMessage(`{"schemaVersion":"neo.agent-snapshot/v1","mode":"synthetic-child-canary"}`),
		Steps:    []agentorchestrator.StepPlan{{Kind: "child_canary_parent"}},
	})
	if err != nil || len(rootResult.Run.Steps) != 1 {
		t.Fatalf("enqueue Parent=%#v err=%v", rootResult, err)
	}
	parentLease := acquireAndStartChildCanaryAttempt(t, ctx, orchestrator, rootResult.Run,
		"neo-runner-primary", 2*time.Minute)

	now := time.Now().UTC()
	rootGrant := grantAt(now, rootResult.Run.ID, 0, "")
	rootGrant.Subject.UserID = userID
	rootGrant.Capabilities = append([]agentbroker.Capability(nil), rootGrant.Capabilities[:1]...)
	rootGrant.Capabilities[0].MaxCalls = 1
	rootGrant.Capabilities[0].Resources = agentbroker.Selector{
		Kind: "exact", Values: []string{"g21.4/synthetic-child"},
	}
	rootGrant.Budget = agentbroker.Budget{
		MaxWallSeconds: 120, MaxModelTokens: 1000, MaxToolCalls: 2, MaxArtifactBytes: 8192,
	}
	rootRegistry, err := agentbroker.BuildRegistry(catalog(), []string{"delegate_task"}, rootGrant, now)
	if err != nil || len(rootRegistry.Tools) != 1 {
		t.Fatalf("Parent Registry=%#v err=%v", rootRegistry, err)
	}
	service := NewService(NewPostgresRepository(runtime), &fakeReaper{})
	rootAuthority, created, err := service.RegisterRoot(ctx, RegisterRootInput{
		UserID: userID, RunID: rootResult.Run.ID, Grant: rootGrant, Registry: rootRegistry,
		Model: ModelBinding{Provider: "fixture", ModelID: "fixture-model"},
	})
	if err != nil || !created {
		t.Fatalf("register Parent=%#v created=%v err=%v", rootAuthority, created, err)
	}

	childGrant := grantAt(now, "run_childtemplate0000", 1, rootResult.Run.ID)
	childGrant.Subject.UserID = userID
	childGrant.Capabilities = []agentbroker.Capability{}
	childGrant.ExpiresAt = now.Add(2 * time.Minute)
	childGrant.Budget = agentbroker.Budget{
		MaxWallSeconds: 30, MaxModelTokens: 250, MaxToolCalls: 1, MaxArtifactBytes: 2048,
	}
	child, err := service.EnqueueChild(ctx, ChildProposal{
		UserID: userID, IdempotencyKey: "g21.4-child-canary-child-postgres",
		ParentRunID: rootResult.Run.ID,
		ParentAttempt: ParentAttempt{StepID: parentLease.StepID, AttemptID: parentLease.ID,
			Generation: parentLease.Generation, LeaseOwner: parentLease.LeaseOwner,
			LeaseToken: parentLease.Token},
		Model: rootAuthority.Model, Grant: childGrant,
		RequestedTools: []string{"delegate_task"}, Catalog: catalog(),
		Steps: []agentorchestrator.StepPlan{{Kind: "child_canary_work"}},
	})
	if err != nil || !child.Created || child.Authority.Depth != 1 ||
		child.Authority.ParentRunID != rootResult.Run.ID || len(child.Authority.Registry.Tools) != 0 {
		t.Fatalf("enqueue Child=%#v err=%v", child, err)
	}
	children, err := service.ListChildren(ctx, userID, rootResult.Run.ID)
	if err != nil || len(children) != 1 || children[0].ChildRunID != child.Authority.RunID {
		t.Fatalf("Child lineage=%#v err=%v", children, err)
	}

	childRun, err := orchestrator.GetRun(ctx, userID, child.Authority.RunID)
	if err != nil || len(childRun.Steps) != 1 {
		t.Fatalf("get Child=%#v err=%v", childRun, err)
	}
	childLease := acquireAndStartChildCanaryAttempt(t, ctx, orchestrator, childRun,
		"neo-runner-primary", 30*time.Second)
	if err := service.AdmitLaunch(ctx, LaunchAdmissionInput{
		UserID: userID, RunID: childRun.ID, AttemptID: childLease.ID,
		Generation: childLease.Generation, LeaseOwner: childLease.LeaseOwner,
		LeaseToken: childLease.Token, SnapshotFingerprint: child.Authority.SnapshotFingerprint,
		GrantFingerprint:    child.Authority.GrantFingerprint,
		RegistryFingerprint: child.Authority.RegistryFingerprint, RegistryTools: []string{},
	}); err != nil {
		t.Fatal(err)
	}

	requestID := "rpc_0000000000000004"
	nonce := strings.Repeat("n", 32)
	requestFingerprint := "sha256:" + strings.Repeat("7", 64)
	tokenDigest := sha256.Sum256([]byte(childLease.Token))
	if _, err := runtime.ExecContext(ctx, `
SELECT count(*) FROM agent_runner_issue_authority(
  $1,$2,$3,'launch',$4,$5,$6::uuid,$7,$8,$9,$10,$5,$11,$12,1
)`, "spiffe://neo-chat/agent-runtime-child-canary", requestID, nonce,
		requestFingerprint, childLease.LeaseOwner, userID, childRun.ID,
		childLease.StepID, childLease.ID, childLease.Generation,
		hex.EncodeToString(tokenDigest[:]), child.Authority.SnapshotFingerprint); err != nil {
		t.Fatal(err)
	}
	const sandboxID = "sandbox_0000000000000004"
	var accepted bool
	if err := runtime.QueryRowContext(ctx, `SELECT agent_runner_expect_sandbox(
  $1,$2,$3,$4,$5::uuid,$6,$7,$8,$9,$10,$11,$12,$13
)`, sandboxID, "spiffe://neo-chat/agent-runtime-child-canary", requestID, nonce,
		userID, childRun.ID, childLease.StepID, childLease.ID, childLease.Generation,
		childLease.LeaseOwner, child.Authority.SnapshotFingerprint,
		"sha256:"+strings.Repeat("8", 64), "sha256:"+strings.Repeat("9", 64)).Scan(&accepted); err != nil || !accepted {
		t.Fatalf("expect Child Sandbox accepted=%v err=%v", accepted, err)
	}
	for _, transition := range [][2]string{{"expected", "starting"}, {"starting", "running"}} {
		if err := runtime.QueryRowContext(ctx,
			`SELECT agent_runner_update_sandbox($1,$2,$3,$4,$5,NULL)`,
			sandboxID, childLease.ID, childLease.Generation, transition[0], transition[1]).Scan(&accepted); err != nil || !accepted {
			t.Fatalf("Sandbox transition %v accepted=%v err=%v", transition, accepted, err)
		}
	}
	transition := agentorchestrator.TransitionInput{UserID: userID, RunID: childRun.ID,
		StepID: childLease.StepID, AttemptID: childLease.ID, Generation: childLease.Generation,
		LeaseOwner: childLease.LeaseOwner, LeaseToken: childLease.Token,
		Actor:    agentorchestrator.Actor{Type: "runner", ID: childLease.LeaseOwner},
		Expected: agentorchestrator.AttemptStarting, To: agentorchestrator.AttemptRunning,
		ReasonCode: "CHILD_CANARY_RUNNING"}
	if err := orchestrator.TransitionAttempt(ctx, transition); err != nil {
		t.Fatal(err)
	}

	targets, err := service.repository.Cascade(ctx, CascadeInput{
		UserID: userID, ParentRunID: rootResult.Run.ID, Mode: "cancel",
		ActorType: "orchestrator", ActorID: "g21.4-child-canary",
		ReasonCode: "CHILD_CANARY_COMPLETE",
	})
	if err != nil || len(targets) != 1 {
		t.Fatalf("cascade targets=%#v err=%v", targets, err)
	}
	target := targets[0]
	if target.UserID != userID || target.ChildRunID != childRun.ID ||
		target.StepID != childLease.StepID || target.AttemptID != childLease.ID ||
		target.Generation != childLease.Generation || target.LeaseOwner != childLease.LeaseOwner ||
		target.AttemptState != agentorchestrator.AttemptCanceled || target.SandboxID != sandboxID ||
		target.SandboxState != "running" || target.LaunchAuthorityExpiresAt == nil {
		t.Fatalf("reap inventory drifted: %#v", target)
	}

	if err := service.repository.CompleteReap(ctx, target.ReapID, false, "RUNTIME_UNAVAILABLE"); err != nil {
		t.Fatal(err)
	}
	var state, errorCode string
	var retries int
	if err := admin.QueryRowContext(ctx, `
SELECT state,retry_count,error_code FROM agent_delegation_reaps WHERE reap_id=$1`,
		target.ReapID).Scan(&state, &retries, &errorCode); err != nil ||
		state != "failed" || retries != 1 || errorCode != "RUNTIME_UNAVAILABLE" {
		t.Fatalf("retryable reap state=%s retries=%d error=%s err=%v", state, retries, errorCode, err)
	}
	if err := runtime.QueryRowContext(ctx,
		`SELECT agent_delegation_complete_reap($1,NULL,NULL)`, target.ReapID).Scan(&accepted); err == nil {
		t.Fatal("NULL reap result accepted")
	}
	if err := service.repository.CompleteReap(ctx, target.ReapID, true, ""); err == nil {
		t.Fatal("active launch authority accepted")
	}

	if _, err := admin.ExecContext(ctx,
		`UPDATE agent_runner_sandboxes SET runner_id='neo-runner-mismatch' WHERE sandbox_id=$1`, sandboxID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(*target.LaunchAuthorityExpiresAt) + 100*time.Millisecond)
	if err := service.repository.CompleteReap(ctx, target.ReapID, true, ""); err == nil {
		t.Fatal("mismatched Sandbox accepted")
	}
	if _, err := admin.ExecContext(ctx,
		`UPDATE agent_runner_sandboxes SET runner_id=$2 WHERE sandbox_id=$1`, sandboxID, childLease.LeaseOwner); err != nil {
		t.Fatal(err)
	}
	if err := service.repository.CompleteReap(ctx, target.ReapID, true, ""); err != nil {
		t.Fatal(err)
	}
	var sandboxState, terminal string
	var completedAt time.Time
	if err := admin.QueryRowContext(ctx, `
SELECT reap.state,reap.retry_count,reap.completed_at,sandbox.state,sandbox.observed_terminal
FROM agent_delegation_reaps reap JOIN agent_runner_sandboxes sandbox
  ON sandbox.attempt_id=reap.attempt_id WHERE reap.reap_id=$1`, target.ReapID).
		Scan(&state, &retries, &completedAt, &sandboxState, &terminal); err != nil ||
		state != "reaped" || retries != 2 || completedAt.IsZero() ||
		sandboxState != "terminal" || terminal != "canceled" {
		t.Fatalf("atomic completion reap=%s retries=%d completed=%s Sandbox=%s/%s err=%v",
			state, retries, completedAt, sandboxState, terminal, err)
	}
	if err := service.repository.CompleteReap(ctx, target.ReapID, true, ""); err != nil {
		t.Fatalf("exact success replay failed: %v", err)
	}
	if err := service.repository.CompleteReap(ctx, target.ReapID, false, "RUNTIME_UNAVAILABLE"); err == nil {
		t.Fatal("reaped failure replay accepted")
	}
	if pending, err := service.PendingReaps(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("pending reaps=%#v err=%v", pending, err)
	}
}

func openChildCanaryPostgres(t *testing.T, databaseURL string) *sql.DB {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*config)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func acquireAndStartChildCanaryAttempt(t *testing.T, ctx context.Context,
	orchestrator *agentorchestrator.Service, run agentorchestrator.Run, runnerID string,
	duration time.Duration,
) agentorchestrator.Lease {
	t.Helper()
	lease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{
		UserID: run.UserID, RunID: run.ID, StepID: run.Steps[0].ID,
		LeaseOwner: runnerID, LeaseDuration: duration,
		Actor:      agentorchestrator.Actor{Type: "orchestrator", ID: "g21.4-child-canary"},
		ReasonCode: "CHILD_CANARY_CLAIMED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.TransitionAttempt(ctx, agentorchestrator.TransitionInput{
		UserID: run.UserID, RunID: run.ID, StepID: lease.StepID, AttemptID: lease.ID,
		Generation: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token,
		Actor:    agentorchestrator.Actor{Type: "orchestrator", ID: "g21.4-child-canary"},
		Expected: agentorchestrator.AttemptLeased, To: agentorchestrator.AttemptStarting,
		ReasonCode: "CHILD_CANARY_STARTING",
	}); err != nil {
		t.Fatal(err)
	}
	return lease
}
