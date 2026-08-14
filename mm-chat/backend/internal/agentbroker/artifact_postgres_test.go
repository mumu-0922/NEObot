package agentbroker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

func TestPostgresArtifactPublicationAuthorizeAttachReplayAndFences(t *testing.T) {
	adminURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	runtimeURL := os.Getenv("MM_CHAT_AGENT_ARTIFACT_DATABASE_URL")
	if adminURL == "" || runtimeURL == "" {
		t.Skip("artifact PostgreSQL test URLs are unset")
	}
	adminDB := openArtifactTestDB(t, adminURL)
	defer adminDB.Close()
	runtimeDB := openArtifactTestDB(t, runtimeURL)
	defer runtimeDB.Close()

	ctx := context.Background()
	retainFixture := os.Getenv("MM_CHAT_AGENT_ARTIFACT_RETAIN_FIXTURE") != ""
	userID := "21212121-2121-4212-8212-212121212121"
	if _, err := adminDB.ExecContext(ctx, `
INSERT INTO users(id,display_name) VALUES($1,'Agent Artifact Test')
ON CONFLICT(id) DO NOTHING`, userID); err != nil {
		t.Fatal(err)
	}

	runID := "run_2121212121212121"
	stepID := "step_2121212121212121"
	now := time.Now().UTC()
	grant := testGrantAt(now)
	grant.GrantID = "grant_2121212121212121"
	grant.Subject.UserID = userID
	grant.Run.RunID = runID
	grant.Capabilities = []Capability{{Capability: "artifact.publish", Actions: []string{"publish"},
		Resources: Selector{Kind: "exact", Values: []string{"result.txt"}},
		Approval:  ApprovalAutomatic, MaxCalls: 2}}
	grant.Budget.MaxToolCalls = 2
	grant.Budget.MaxArtifactBytes = 1024
	grantFingerprint, err := GrantFingerprint(grant)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := BuildRegistry([]ToolDefinition{{Identity: "artifact.publish",
		Capability: "artifact.publish", Actions: []string{"publish"},
		Classification: ClassificationMutable, Idempotent: false}}, []string{"artifact.publish"}, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := json.Marshal(map[string]any{
		"schemaVersion":       "neo.agent-broker-canary-snapshot/v1",
		"grantFingerprint":    grantFingerprint,
		"registryFingerprint": registry.Fingerprint,
		"artifactPolicy": map[string]any{
			"maxBytes":          1024,
			"allowedMediaTypes": []string{"text/plain"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(runtimeDB))
	enqueued, err := orchestrator.EnqueueRunWithID(ctx, runID, agentorchestrator.EnqueueInput{
		UserID: userID, IdempotencyKey: "g21.2/artifact/publication", Snapshot: snapshot,
		Steps: []agentorchestrator.StepPlan{{ID: stepID, Kind: "artifact_publish"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID,
		RunID: runID, StepID: stepID, LeaseOwner: "neo-runner-broker-canary",
		LeaseDuration: 20 * time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "g21.2-test"},
		ReasonCode: "BROKER_CANARY_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	transitionArtifactAttempt(t, ctx, orchestrator, userID, lease, "leased", "starting")
	transitionArtifactAttempt(t, ctx, orchestrator, userID, lease, "starting", "running")

	authority := AttemptAuthority{UserID: userID, RunID: runID, StepID: stepID,
		AttemptID: lease.ID, Generation: lease.Generation, LeaseOwner: lease.LeaseOwner,
		LeaseToken: lease.Token, SnapshotFingerprint: enqueued.Run.SnapshotFingerprint,
		GrantFingerprint: grantFingerprint, RegistryFingerprint: registry.Fingerprint,
		KillSwitchEpoch: currentPostgresKillSwitchEpoch(t, ctx, runtimeDB)}
	brokerRepository := NewPostgresRepository(runtimeDB)
	service, err := NewService(brokerRepository, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := service.Prepare(ctx, PrepareInput{RequestID: "request_2121212121212121",
		Attempt: authority, Grant: grant, Registry: registry, ToolIdentity: "artifact.publish",
		Action: "publish", Resource: "result.txt", Arguments: json.RawMessage(`{"name":"result.txt"}`),
		TTL: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := brokerRepository.ClaimCommit(ctx, testCommitInput(prepared, authority), tokenDigest(authority.LeaseToken))
	if err != nil || claim.Replay || claim.Intent.State != IntentCommitting {
		t.Fatalf("ClaimCommit = %#v, %v", claim, err)
	}

	candidate := ArtifactCandidate{ArtifactID: "artifact_2121212121212121", IntentID: prepared.IntentID,
		UserID: userID, RunID: runID, AttemptID: lease.ID, Generation: lease.Generation,
		SnapshotFingerprint: enqueued.Run.SnapshotFingerprint, GrantFingerprint: grantFingerprint,
		RegistryFingerprint: registry.Fingerprint, QuarantineRef: lease.ID + "/result.txt",
		Name: "result.txt", MediaType: "text/plain", Size: 8,
		Fingerprint: artifactFingerprint([]byte("artifact"))}
	artifacts := NewPostgresArtifactRepository(runtimeDB)
	if err := artifacts.AuthorizeArtifact(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	objectKey := artifactObjectKey(candidate)
	if err := artifacts.AttachArtifact(ctx, candidate, objectKey); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.AttachArtifact(ctx, candidate, objectKey); err != nil {
		t.Fatalf("exact attach replay = %v", err)
	}
	var count int
	if err := adminDB.QueryRowContext(ctx, `SELECT count(*) FROM agent_artifacts WHERE id=$1`,
		candidate.ArtifactID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("artifact row count = %d, %v", count, err)
	}

	collision := candidate
	collision.Fingerprint = artifactFingerprint([]byte("drifted"))
	collision.Size = 7
	if err := artifacts.AttachArtifact(ctx, collision, artifactObjectKey(collision)); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("artifact collision = %v", err)
	}
	staleGeneration := candidate
	staleGeneration.Generation++
	if err := artifacts.AuthorizeArtifact(ctx, staleGeneration); !errors.Is(err, ErrArtifactDenied) &&
		!errors.Is(err, ErrLeaseStale) {
		t.Fatalf("stale generation = %v", err)
	}
	if _, err := adminDB.ExecContext(ctx, `UPDATE agent_attempts
SET created_at=clock_timestamp()-interval '1 hour',updated_at=clock_timestamp()-interval '1 hour',
    lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, lease.ID); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.AuthorizeArtifact(ctx, candidate); !errors.Is(err, ErrLeaseStale) {
		t.Fatalf("expired lease = %v", err)
	}
	if _, err := adminDB.ExecContext(ctx, `UPDATE agent_attempts SET lease_expires_at=clock_timestamp()+interval '20 minutes' WHERE id=$1`, lease.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeGrant(ctx, GrantRevocationInput{GrantID: grant.GrantID,
		GrantFingerprint: grantFingerprint, ActorType: "operator", ActorID: "g21.2-test",
		ReasonCode: "BROKER_CANARY_TEST"}); err != nil {
		t.Fatal(err)
	}
	if err := artifacts.AuthorizeArtifact(ctx, candidate); !errors.Is(err, ErrGrantDenied) {
		t.Fatalf("revoked Grant = %v", err)
	}

	if retainFixture {
		return
	}
	if _, err := adminDB.ExecContext(ctx, `DELETE FROM agent_effect_grant_revocations WHERE grant_id=$1;
DELETE FROM agent_runs WHERE id=$2;DELETE FROM agent_run_snapshots WHERE id=$3`,
		grant.GrantID, runID, enqueued.Run.SnapshotID); err != nil {
		t.Fatal(err)
	}
}

func openArtifactTestDB(t *testing.T, databaseURL string) *sql.DB {
	t.Helper()
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func transitionArtifactAttempt(t *testing.T, ctx context.Context, service *agentorchestrator.Service,
	userID string, lease agentorchestrator.Lease, expected, target string,
) {
	t.Helper()
	if err := service.TransitionAttempt(ctx, agentorchestrator.TransitionInput{UserID: userID,
		RunID: lease.RunID, StepID: lease.StepID, AttemptID: lease.ID, Generation: lease.Generation,
		LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token, Expected: expected, To: target,
		Actor:      agentorchestrator.Actor{Type: "runner", ID: lease.LeaseOwner},
		ReasonCode: "BROKER_CANARY_TEST"}); err != nil {
		t.Fatal(err)
	}
}
