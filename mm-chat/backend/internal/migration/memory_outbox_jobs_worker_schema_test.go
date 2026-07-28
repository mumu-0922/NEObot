package migration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestMemoryOutboxJobsWorkerMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "054_memory_outbox_jobs_worker.up.sql")
	down := readPhase15SQL(t, "054_memory_outbox_jobs_worker.down.sql")

	outboxDDL := phase15TableDDL(t, up, "memory_outbox")
	assertPhase15Fragments(t, outboxDDL,
		"054 outbox must be durable, versioned, bounded, and ID-only",
		"event_schema_major smallint not null",
		"event_type = 'turn.completed'",
		"event_schema_major in ( 1 , 2 )",
		"payload jsonb not null",
		"providerprofile",
		"status in ( 'pending' , 'processing' , 'completed' , 'dead_letter' )",
		"lease_owner uuid",
		"lease_token uuid",
		"lease_expires_at timestamptz")
	if strings.Contains(outboxDDL, "user_message_content") ||
		strings.Contains(outboxDDL, "assistant_message_content") ||
		strings.Contains(outboxDDL, "encrypted_secret") {
		t.Fatal("054 outbox schema must not persist message content or provider secrets")
	}

	jobsDDL := phase15TableDDL(t, up, "memory_jobs")
	assertPhase15Fragments(t, jobsDDL,
		"054 jobs must pin source/profile generations and support lease replay",
		"event_id uuid not null references memory_outbox",
		"source_message_id uuid not null",
		"source_hash text not null",
		"processing_profile text not null",
		"scope_generation bigint not null",
		"visibility_epoch bigint not null default 1",
		"attempt_count integer not null default 0",
		"max_attempts integer not null default 8")

	assertPhase15Fragments(t, up,
		"054 must expose narrow SECURITY DEFINER capabilities instead of table CRUD",
		"create function memory_append_turn_completed_event",
		"create function memory_worker_claim_job",
		"create function memory_worker_hydrate_capture",
		"create function memory_worker_apply_capture_candidate",
		"create function memory_worker_complete_job",
		"create function memory_worker_retry_job",
		"security definer",
		"to go_api_runtime",
		"to memory_worker_runtime",
		"memory_worker_runtime_forbidden_owner_membership",
		"revoke all on memory_outbox , memory_jobs from go_api_runtime , memory_worker_runtime")
	assertPhase15Fragments(t, down,
		"054 rollback must preserve queued work",
		"memory_worker_rollback_requires_empty_queue",
		"drop function memory_worker_claim_job",
		"drop table memory_jobs",
		"drop table memory_outbox")
}

func TestMemoryOutboxJobsWorkerLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	files := phase15MigrationFSThrough(t, 37)
	for _, path := range []string{
		"053_memory_project_scope_settings.up.sql",
		"053_memory_project_scope_settings.down.sql",
		"054_memory_outbox_jobs_worker.up.sql",
		"054_memory_outbox_jobs_worker.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		files[path] = &fstest.MapFile{Data: data}
	}
	runner := NewRunner(db, files)
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("apply migrations through 054: %v", err)
	}
	if len(applied) == 0 || applied[len(applied)-1].ID() != "054_memory_outbox_jobs_worker" {
		t.Fatalf("migration head = %#v", applied)
	}

	const (
		userA         = "10000000-0000-4000-8000-000000000001"
		userB         = "10000000-0000-4000-8000-000000000002"
		conversation  = "20000000-0000-4000-8000-000000000001"
		userMessage   = "30000000-0000-4000-8000-000000000001"
		assistant     = "30000000-0000-4000-8000-000000000002"
		provider      = "40000000-0000-4000-8000-000000000001"
		eventID       = "50000000-0000-4000-8000-000000000001"
		jobID         = "60000000-0000-4000-8000-000000000001"
		workerA       = "70000000-0000-4000-8000-000000000001"
		workerB       = "70000000-0000-4000-8000-000000000002"
		leaseA        = "80000000-0000-4000-8000-000000000001"
		leaseB        = "80000000-0000-4000-8000-000000000002"
		memoryID      = "90000000-0000-4000-8000-000000000001"
		rollbackMsg   = "30000000-0000-4000-8000-000000000003"
		rollbackEvt   = "50000000-0000-4000-8000-000000000003"
		rollbackJob   = "60000000-0000-4000-8000-000000000003"
		crossConv     = "20000000-0000-4000-8000-000000000004"
		crossSource   = "30000000-0000-4000-8000-000000000004"
		crossAnswer   = "30000000-0000-4000-8000-000000000005"
		crossEvent    = "50000000-0000-4000-8000-000000000004"
		crossJob      = "60000000-0000-4000-8000-000000000004"
		foreignConv   = "20000000-0000-4000-8000-000000000005"
		foreignSource = "30000000-0000-4000-8000-000000000006"
		foreignAnswer = "30000000-0000-4000-8000-000000000007"
	)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES
  ($1, 'Memory A'), ($2, 'Memory B');
INSERT INTO conversations(id, user_id, title) VALUES ($3, $1, 'capture');
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES
  ($4, $3, $1, 1, 'user', 'completed', 'Remember that I prefer concise answers', now()),
  ($5, $3, $1, 2, 'assistant', 'completed', 'Understood', now());
UPDATE messages SET parent_message_id = $4 WHERE id = $5;
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled
) VALUES ($1, true, true, true);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $6, $1, 'fixture', 'Fixture', '{"v":1}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
`, userA, userB, conversation, userMessage, assistant, provider)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no,
  role, status, content
) VALUES ($1, $2, $3, $4, 3, 'assistant', 'streaming', 'partial');
`, rollbackMsg, conversation, userA, userMessage)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE messages SET status = 'completed', completed_at = now() WHERE id = $1
`, rollbackMsg); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6, 'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, rollbackEvt, rollbackJob, userA, conversation, userMessage, rollbackMsg); err != nil {
		_ = tx.Rollback()
		t.Fatalf("transactional append: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var rollbackStatus string
	var rollbackEventCount int
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT status FROM messages WHERE id = $1),
  (SELECT count(*) FROM memory_outbox WHERE event_id = $2)
`, rollbackMsg, rollbackEvt).Scan(&rollbackStatus, &rollbackEventCount); err != nil {
		t.Fatal(err)
	}
	if rollbackStatus != "streaming" || rollbackEventCount != 0 {
		t.Fatalf("rolled back finalize = %q/%d", rollbackStatus, rollbackEventCount)
	}

	// Ineligible Learn policy returns no event authority. The API can therefore
	// skip a Redis wake instead of publishing an ID that does not exist.
	mustExecPhase151C(t, ctx, db, `
UPDATE conversations SET memory_learn_mode = 'off' WHERE id = $1;
`, conversation)
	var skippedEvent sql.NullString
	if err := db.QueryRowContext(ctx, `
SELECT memory_append_turn_completed_event(
  '50000000-0000-4000-8000-000000000099',
  '60000000-0000-4000-8000-000000000099',
  $1, $2, $3, $4, 'server-stored', 'fixture', 'fixture-model', 2::smallint
)::text
`, userA, conversation, userMessage, assistant).Scan(&skippedEvent); err != nil {
		t.Fatalf("skip ineligible capture: %v", err)
	}
	if skippedEvent.Valid {
		t.Fatalf("ineligible capture event = %q, want NULL", skippedEvent.String)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE conversations SET memory_learn_mode = 'inherit' WHERE id = $1;
`, conversation)

	appendTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appendTx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		_ = appendTx.Rollback()
		t.Fatalf("set API role: %v", err)
	}
	var captured string
	if err := appendTx.QueryRowContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6, 'server-stored', 'fixture', 'fixture-model', 2::smallint
)::text
`, eventID, jobID, userA, conversation, userMessage, assistant).Scan(&captured); err != nil {
		_ = appendTx.Rollback()
		t.Fatalf("append capture: %v", err)
	}
	if err := appendTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if captured != eventID {
		t.Fatalf("captured event = %q", captured)
	}
	var duplicate string
	if err := db.QueryRowContext(ctx, `
SELECT memory_append_turn_completed_event(
  '50000000-0000-4000-8000-000000000002',
  '60000000-0000-4000-8000-000000000002',
  $1, $2, $3, $4, 'server-stored', 'fixture', 'fixture-model', 2::smallint
)::text
`, userA, conversation, userMessage, assistant).Scan(&duplicate); err != nil {
		t.Fatalf("duplicate append: %v", err)
	}
	if duplicate != eventID {
		t.Fatalf("duplicate event = %q", duplicate)
	}
	var eventCount, jobCount int
	var payload string
	if err := db.QueryRowContext(ctx, `
SELECT
  (SELECT count(*) FROM memory_outbox),
  (SELECT count(*) FROM memory_jobs),
  (SELECT payload::text FROM memory_outbox WHERE event_id = $1)
`, eventID).Scan(&eventCount, &jobCount, &payload); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 || jobCount != 1 || strings.Contains(payload, "concise answers") ||
		strings.Contains(payload, "encrypted") || !strings.Contains(payload, "sourceHash") {
		t.Fatalf("event/job/payload = %d/%d/%s", eventCount, jobCount, payload)
	}

	assertMemoryWorkerRoleCannotReadTables(t, ctx, db)

	claimTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimTx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		_ = claimTx.Rollback()
		t.Fatalf("set worker role for claim: %v", err)
	}
	claimMemoryJob(t, ctx, claimTx, workerA, leaseA, jobID, 1)
	if err := claimTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_hydrate_capture($1, $2, $3)
`, jobID, workerA, leaseB); err == nil || !strings.Contains(err.Error(), "MEMORY_JOB_LEASE_LOST") {
		t.Fatalf("wrong-token hydration error = %v", err)
	}
	var hydratedUser, hydratedContent string
	if err := db.QueryRowContext(ctx, `
SELECT user_id::text, user_message_content
FROM memory_worker_hydrate_capture($1, $2, $3)
`, jobID, workerA, leaseA).Scan(&hydratedUser, &hydratedContent); err != nil {
		t.Fatalf("hydrate capture: %v", err)
	}
	if hydratedUser != userA || !strings.Contains(hydratedContent, "concise") {
		t.Fatalf("hydrated = %q/%q", hydratedUser, hydratedContent)
	}

	// Simulate a crash after claim. The PostgreSQL lease, not Redis or process
	// memory, remains the replay authority.
	mustExecPhase151C(t, ctx, db, `
UPDATE memory_jobs SET lease_expires_at = created_at WHERE job_id = $1;
UPDATE memory_outbox SET lease_expires_at = created_at WHERE event_id = $2;
`, jobID, eventID)
	claimMemoryJob(t, ctx, db, workerB, leaseB, jobID, 2)
	if _, err := db.ExecContext(ctx, `
SELECT memory_worker_complete_job($1, $2, $3)
`, jobID, workerA, leaseA); err == nil || !strings.Contains(err.Error(), "MEMORY_JOB_LEASE_LOST") {
		t.Fatalf("stale completion error = %v", err)
	}

	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_apply_capture_candidate(
  $1, $2, $3, $4, 'preference', 'Use concise answers',
  'use concise answers', 5::smallint, ARRAY['style']::text[]
)
`, jobID, workerB, leaseB, memoryID); err != nil {
		t.Fatalf("apply leased candidate: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_worker_complete_job($1, $2, $3)
`, jobID, workerB, leaseB); err != nil {
		t.Fatalf("complete reclaimed job: %v", err)
	}
	var status string
	var attempts int
	if err := db.QueryRowContext(ctx, `
SELECT status, attempt_count FROM memory_jobs WHERE job_id = $1
`, jobID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || attempts != 2 {
		t.Fatalf("job = %q/%d", status, attempts)
	}

	// Even a deliberately corrupted job cannot hydrate another user's source.
	mustExecPhase151C(t, ctx, db, `
INSERT INTO conversations(id, user_id, title) VALUES
  ($1, $2, 'cross owner'), ($3, $4, 'foreign owner');
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no,
  role, status, content, completed_at
) VALUES
  ($6, $1, $2, NULL, 0, 'user', 'completed', 'A source', now()),
  ($7, $3, $4, NULL, 1, 'user', 'completed', 'foreign source', now());
INSERT INTO messages(
  id, conversation_id, user_id, parent_message_id, sequence_no,
  role, status, content, completed_at
) VALUES
  ($5, $1, $2, $6, 1, 'assistant', 'completed', 'A answer', now()),
  ($8, $3, $4, $7, 2, 'assistant', 'completed', 'foreign answer', now());
`, crossConv, userA, foreignConv, userB, crossAnswer, crossSource, foreignSource, foreignAnswer)
	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $3, $4, $5, $6, $1, $2, 'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, crossSource, crossAnswer, crossEvent, crossJob, userA, crossConv); err != nil {
		t.Fatalf("append cross-user fixture: %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE memory_jobs
SET source_conversation_id = $1,
    source_message_id = $2,
    assistant_message_id = $3,
    source_hash = encode(sha256(convert_to('foreign source', 'UTF8')), 'hex')
WHERE job_id = $4;
`, foreignConv, foreignSource, foreignAnswer, crossJob)
	claimMemoryJob(t, ctx, db, workerA, leaseA, crossJob, 1)
	if _, err := db.ExecContext(ctx, `
SELECT * FROM memory_worker_hydrate_capture($1, $2, $3)
`, crossJob, workerA, leaseA); err == nil || !strings.Contains(err.Error(), "MEMORY_CAPTURE_SOURCE_DRIFT") {
		t.Fatalf("cross-user hydration error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
SELECT memory_worker_retry_job($1, $2, $3, 'SOURCE_DRIFT', now(), true)
`, crossJob, workerA, leaseA); err != nil {
		t.Fatalf("dead-letter cross-user fixture: %v", err)
	}

	// A crash on the final attempt is terminalized rather than left forever in
	// processing state.
	mustExecPhase151C(t, ctx, db, `
UPDATE messages SET status = 'completed', completed_at = now() WHERE id = $1;
`, rollbackMsg)
	if _, err := db.ExecContext(ctx, `
SELECT memory_append_turn_completed_event(
  $1, $2, $3, $4, $5, $6, 'server-stored', 'fixture', 'fixture-model', 2::smallint
)
`, rollbackEvt, rollbackJob, userA, conversation, userMessage, rollbackMsg); err != nil {
		t.Fatalf("append exhausted fixture: %v", err)
	}
	mustExecPhase151C(t, ctx, db, `
UPDATE memory_jobs SET max_attempts = 1 WHERE job_id = $1;
UPDATE memory_outbox SET max_attempts = 1 WHERE event_id = $2;
`, rollbackJob, rollbackEvt)
	claimMemoryJob(t, ctx, db, workerA, leaseA, rollbackJob, 1)
	mustExecPhase151C(t, ctx, db, `
UPDATE memory_jobs SET lease_expires_at = created_at WHERE job_id = $1;
UPDATE memory_outbox SET lease_expires_at = created_at WHERE event_id = $2;
`, rollbackJob, rollbackEvt)
	var exhaustedJob string
	err = db.QueryRowContext(ctx, `
SELECT job_id::text FROM memory_worker_claim_job($1, $2, 120)
`, workerB, leaseB).Scan(&exhaustedJob)
	if err != sql.ErrNoRows {
		t.Fatalf("exhausted claim error = %v, job = %q", err, exhaustedJob)
	}
	var exhaustedStatus, exhaustedCode string
	if err := db.QueryRowContext(ctx, `
SELECT status, error_code FROM memory_jobs WHERE job_id = $1
`, rollbackJob).Scan(&exhaustedStatus, &exhaustedCode); err != nil {
		t.Fatal(err)
	}
	if exhaustedStatus != "dead_letter" || exhaustedCode != "LEASE_EXPIRED" {
		t.Fatalf("exhausted job = %q/%q", exhaustedStatus, exhaustedCode)
	}

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_WORKER_ROLLBACK_REQUIRES_EMPTY_QUEUE") {
		t.Fatalf("guarded rollback error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `DELETE FROM memory_jobs; DELETE FROM memory_outbox;`)
	rolledBack, err := runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 54 {
		t.Fatalf("clean rollback = %#v, %v", rolledBack, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 54 {
		t.Fatalf("re-up = %#v, %v", reapplied, err)
	}
}

func assertMemoryWorkerRoleCannotReadTables(
	t *testing.T,
	ctx context.Context,
	db interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	},
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatalf("set worker role: %v", err)
	}
	for _, table := range []string{
		"memory_jobs",
		"memory_outbox",
		"user_memories",
		"messages",
		"provider_configs",
	} {
		if _, err := tx.ExecContext(ctx, `SELECT count(*) FROM `+table); err == nil {
			t.Fatalf("memory_worker_runtime unexpectedly read %s directly", table)
		}
	}
}

func claimMemoryJob(
	t *testing.T,
	ctx context.Context,
	db interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	workerID string,
	leaseToken string,
	wantJobID string,
	wantAttempt int,
) {
	t.Helper()
	var jobID string
	var attempt int
	if err := db.QueryRowContext(ctx, `
SELECT job_id::text, attempt_count
FROM memory_worker_claim_job($1, $2, 120)
`, workerID, leaseToken).Scan(&jobID, &attempt); err != nil {
		t.Fatalf("claim memory job: %v", err)
	}
	if jobID != wantJobID || attempt != wantAttempt {
		t.Fatalf("claimed = %q/%d, want %q/%d", jobID, attempt, wantJobID, wantAttempt)
	}
}
