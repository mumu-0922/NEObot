package agentrunner

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agentorchestrator"
)

func TestPostgresRunnerAuthorityReplaySandboxAndRetention(t *testing.T) {
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MM_CHAT_TEST_DATABASE_URL is unset")
	}
	config, parseErr := pgx.ParseConfig(databaseURL)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	ctx := context.Background()
	retainFixture := os.Getenv("MM_CHAT_AGENT_RUNNER_RETAIN_FIXTURE") != ""
	userID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	idempotencyKey := "runner-authority-test"
	if retainFixture {
		userID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		idempotencyKey = "runner-authority-retained"
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO users(id,display_name) VALUES($1,'Agent Runner Test') ON CONFLICT(id) DO NOTHING`, userID)
	orchestrator := agentorchestrator.NewService(agentorchestrator.NewPostgresRepository(db))
	enqueue, err := orchestrator.EnqueueRun(ctx, agentorchestrator.EnqueueInput{UserID: userID, IdempotencyKey: idempotencyKey, Snapshot: json.RawMessage(`{"snapshot":"runner-test"}`), Steps: []agentorchestrator.StepPlan{{Kind: "execute"}}})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := orchestrator.AcquireStep(ctx, agentorchestrator.AcquireInput{UserID: userID, RunID: enqueue.Run.ID, StepID: enqueue.Run.Steps[0].ID, LeaseOwner: testRunner, LeaseDuration: time.Minute, Actor: agentorchestrator.Actor{Type: "orchestrator", ID: "runner-test"}, ReasonCode: "RUNNER_TEST"})
	if err != nil {
		t.Fatal(err)
	}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	control, err := NewControlService(NewPostgresControlRepository(db), private)
	if err != nil {
		t.Fatal(err)
	}
	input := IssueAuthorityInput{CallerIdentity: testCaller, RunnerID: testRunner, RequestID: testRequestID, Nonce: "postgresrunnernonce0123456789abcd", Method: MethodLaunch, RequestFingerprint: testFingerprint('1'), UserID: userID, Attempt: AttemptRef{RunID: lease.RunID, StepID: lease.StepID, AttemptID: lease.ID, LeaseGeneration: lease.Generation, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.Token}, SnapshotFingerprint: enqueue.Run.SnapshotFingerprint, TTL: 10 * time.Second}
	wrongRunner := input
	wrongRunner.RunnerID = "other-runner"
	if _, _, err := control.IssueAuthority(ctx, wrongRunner); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("runner/lease-owner mismatch=%v", err)
	}
	ticket, response, err := control.IssueAuthority(ctx, input)
	if err != nil || response != nil {
		t.Fatalf("issue=%#v/%s/%v", ticket, response, err)
	}
	verifier, _ := NewSignedAuthorityVerifier(public)
	if err := verifier.Verify(testCaller, testRunner, MethodLaunch, testRequestID, input.Nonce, input.RequestFingerprint, enqueue.Run.SnapshotFingerprint, input.Attempt, ticket, time.Now()); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errorsCh := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _, claimErr := control.IssueAuthority(ctx, input); errorsCh <- claimErr }()
	}
	wg.Wait()
	close(errorsCh)
	for claimErr := range errorsCh {
		if !errors.Is(claimErr, ErrReplayDetected) {
			t.Fatalf("concurrent claim=%v", claimErr)
		}
	}
	responseBody := []byte(`{"schemaVersion":"neo.runner-rpc/v1","method":"launch.result","requestId":"rpc_0123456789abcdef","sentAt":"2026-08-13T00:00:00Z","nonce":"postgresrunnernonce0123456789abcd","body":{"accepted":true,"sandboxId":"sandbox_0123456789abcdef"}}`)
	if err := control.CompleteRequest(ctx, input.CallerIdentity, input.RequestID, input.Nonce, input.RequestFingerprint, responseBody); err != nil {
		t.Fatal(err)
	}
	_, replayed, err := control.IssueAuthority(ctx, input)
	if err != nil || !json.Valid(replayed) {
		t.Fatalf("replay=%s/%v", replayed, err)
	}
	mismatch := input
	mismatch.RequestFingerprint = testFingerprint('2')
	if _, _, err := control.IssueAuthority(ctx, mismatch); !errors.Is(err, ErrReplayDetected) {
		t.Fatalf("mismatch=%v", err)
	}

	specDigest := sha256.Sum256([]byte("sandbox"))
	expected := ExpectedSandbox{SandboxID: "sandbox_0123456789abcdef", CallerIdentity: input.CallerIdentity,
		RequestID: input.RequestID, Nonce: input.Nonce, UserID: userID,
		Attempt: input.Attempt.Identity(), RunnerID: testRunner,
		SnapshotFingerprint: enqueue.Run.SnapshotFingerprint,
		SpecFingerprint:     "sha256:" + hex.EncodeToString(specDigest[:]), ProbeFingerprint: testFingerprint('9')}
	repository := NewPostgresControlRepository(db)
	if err := repository.ExpectSandbox(ctx, expected); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateSandbox(ctx, expected.SandboxID, expected.Attempt.AttemptID, expected.Attempt.LeaseGeneration, "expected", "starting", ""); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateSandbox(ctx, expected.SandboxID, expected.Attempt.AttemptID, expected.Attempt.LeaseGeneration, "starting", "running", ""); err != nil {
		t.Fatal(err)
	}
	recovery, err := repository.RecoverySandboxes(ctx, 10)
	if err != nil || len(recovery) < 1 {
		t.Fatalf("recovery=%#v/%v", recovery, err)
	}
	if retainFixture {
		return
	}
	if err := repository.UpdateSandbox(ctx, expected.SandboxID, expected.Attempt.AttemptID, expected.Attempt.LeaseGeneration, "running", "terminal", "killed"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE agent_runner_requests SET issued_at=clock_timestamp()-interval '3 hours',expires_at=clock_timestamp()-interval '3 hours'+interval '10 seconds',completed_at=clock_timestamp()-interval '2 hours' WHERE request_id=$1`, input.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE agent_runner_sandboxes SET created_at=clock_timestamp()-interval '3 hours',updated_at=clock_timestamp()-interval '2 hours',terminal_at=clock_timestamp()-interval '2 hours' WHERE sandbox_id=$1`, expected.SandboxID); err != nil {
		t.Fatal(err)
	}
	requests, sandboxes, err := repository.Prune(ctx, time.Now().Add(-time.Hour), 10)
	if err != nil || requests != 1 || sandboxes != 1 {
		t.Fatalf("prune=%d/%d/%v", requests, sandboxes, err)
	}
	if !retainFixture {
		_, _ = db.ExecContext(ctx, `DELETE FROM agent_runs WHERE id=$1;DELETE FROM agent_run_snapshots WHERE id=$2`, enqueue.Run.ID, enqueue.Run.SnapshotID)
	}
}
