package migration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestMemoryWorkerHealthResolutionLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	base := NewRunner(db, phase15MigrationFSThrough(t, 70))
	if _, err := base.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Up(ctx); err != nil {
		t.Fatalf("apply through 070: %v", err)
	}

	const (
		userA        = "17100000-0000-4000-8000-000000000001"
		userB        = "17100000-0000-4000-8000-000000000002"
		conversation = "27100000-0000-4000-8000-000000000001"
		provider     = "37100000-0000-4000-8000-000000000001"
		historical   = "67100000-0000-4000-8000-000000000001"
		pending      = "67100000-0000-4000-8000-000000000002"
		reviewExpire = "67100000-0000-4000-8000-000000000003"
		sourceDrift  = "67100000-0000-4000-8000-000000000004"
	)
	seedMemoryHealthResolutionJobs(
		t, ctx, db, userA, userB, conversation, provider,
		historical, pending, reviewExpire, sourceDrift,
	)
	assertMemoryHealthCaptureCounts(t, ctx, db, userA, 2, 0, 2)

	runner := NewRunner(db, phase15MigrationFSThrough(t, 71))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 71 {
		t.Fatalf("apply 071 = %#v/%v", applied, err)
	}
	assertMemoryHealthResolutionPrivileges(t, ctx, db)
	assertMemoryHealthCaptureCounts(t, ctx, db, userA, 1, 0, 2)

	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 71 {
		t.Fatalf("clean down 071 = %#v/%v", down, err)
	}
	assertMemoryHealthCaptureCounts(t, ctx, db, userA, 2, 0, 2)
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 71 {
		t.Fatalf("re-up 071 = %#v/%v", reapplied, err)
	}

	callMemoryHealthResolutionExpectError(
		t, ctx, db, userB, historical, "EXTRACTION_INVALID",
		"historical_failure_accepted", "memory_job_health_resolution_not_eligible",
	)
	callMemoryHealthResolutionExpectError(
		t, ctx, db, userA, historical, "SOURCE_DRIFT",
		"historical_failure_accepted", "memory_job_health_resolution_drift",
	)
	callMemoryHealthResolutionExpectError(
		t, ctx, db, userA, pending, "PENDING",
		"historical_failure_accepted", "memory_job_health_resolution_not_eligible",
	)
	if _, err := db.ExecContext(ctx, `
UPDATE conversations SET status = 'active', deleted_at = NULL
WHERE id = $1::uuid
`, conversation); err != nil {
		t.Fatal(err)
	}
	callMemoryHealthResolutionExpectError(
		t, ctx, db, userA, sourceDrift, "SOURCE_DRIFT",
		"source_no_longer_current", "memory_job_health_resolution_source_current",
	)
	if _, err := db.ExecContext(ctx, `
UPDATE memory_jobs SET source_conversation_id = $1::uuid
WHERE job_id = $2::uuid
`, "27100000-0000-4000-8000-000000000099", sourceDrift); err != nil {
		t.Fatal(err)
	}
	callMemoryHealthResolutionExpectError(
		t, ctx, db, userA, sourceDrift, "SOURCE_DRIFT",
		"source_no_longer_current", "memory_job_health_resolution_source_current",
	)
	if _, err := db.ExecContext(ctx, `
UPDATE memory_jobs SET source_conversation_id = $1::uuid
WHERE job_id = $2::uuid;
UPDATE conversations SET status = 'deleted', deleted_at = now()
WHERE id = $1::uuid;
`, conversation, sourceDrift); err != nil {
		t.Fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		t.Fatal(err)
	}
	var historicalCreated, driftCreated, historicalReplay bool
	if err := tx.QueryRowContext(ctx, `
SELECT memory_acknowledge_job_health(
  $1::uuid, $2::uuid, 'EXTRACTION_INVALID', 'historical_failure_accepted'
)
`, userA, historical).Scan(&historicalCreated); err != nil {
		t.Fatalf("acknowledge historical failure: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_acknowledge_job_health(
  $1::uuid, $2::uuid, 'SOURCE_DRIFT', 'source_no_longer_current'
)
`, userA, sourceDrift).Scan(&driftCreated); err != nil {
		t.Fatalf("acknowledge source drift: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT memory_acknowledge_job_health(
  $1::uuid, $2::uuid, 'EXTRACTION_INVALID', 'historical_failure_accepted'
)
`, userA, historical).Scan(&historicalReplay); err != nil {
		t.Fatalf("replay historical acknowledgement: %v", err)
	}
	if !historicalCreated || !driftCreated || historicalReplay {
		t.Fatalf("acknowledgements = historical:%t drift:%t replay:%t",
			historicalCreated, driftCreated, historicalReplay)
	}
	assertMemoryHealthCaptureCountsWithQueryer(t, ctx, tx, userA, 1, 0, 0)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if _, err := db.ExecContext(ctx, `
UPDATE memory_job_health_resolutions SET resolution_code = resolution_code
WHERE job_id = $1::uuid
`, historical); err == nil || !strings.Contains(
		strings.ToLower(err.Error()), "memory_job_health_resolution_append_only",
	) {
		t.Fatalf("resolution update error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
DELETE FROM memory_job_health_resolutions WHERE job_id = $1::uuid
`, historical); err == nil || !strings.Contains(
		strings.ToLower(err.Error()), "memory_job_health_resolution_append_only",
	) {
		t.Fatalf("resolution delete error = %v", err)
	}
	if _, err := runner.Down(ctx, false); err == nil || !strings.Contains(
		strings.ToLower(err.Error()), "memory_job_health_resolution_rollback_requires_empty",
	) {
		t.Fatalf("resolved 071 down error = %v", err)
	}
}

func seedMemoryHealthResolutionJobs(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userA, userB, conversation, provider string,
	historical, pending, reviewExpire, sourceDrift string,
) {
	t.Helper()
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'Health owner'), ($2, 'Other');
INSERT INTO conversations(id, user_id, title) VALUES ($3, $1, 'health fixture');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled
) VALUES ($1, true, true, true);
INSERT INTO provider_configs(
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES (
  $4, $1, 'fixture', 'Fixture', '{"v":1}',
  '{"kind":"model","type":"OpenAI Compatible","enabled":true}'::jsonb
);
`, userA, userB, conversation, provider)

	jobs := []struct {
		jobID, eventID, sourceID, assistantID string
	}{
		{historical, "57100000-0000-4000-8000-000000000001", "47100000-0000-4000-8000-000000000001", "47100000-0000-4000-8000-000000000002"},
		{pending, "57100000-0000-4000-8000-000000000002", "47100000-0000-4000-8000-000000000003", "47100000-0000-4000-8000-000000000004"},
		{reviewExpire, "57100000-0000-4000-8000-000000000003", "47100000-0000-4000-8000-000000000005", "47100000-0000-4000-8000-000000000006"},
		{sourceDrift, "57100000-0000-4000-8000-000000000004", "47100000-0000-4000-8000-000000000007", "47100000-0000-4000-8000-000000000008"},
	}
	for index, job := range jobs {
		mustExecPhase151C(t, ctx, db, `
INSERT INTO messages(
  id, conversation_id, user_id, sequence_no, role, status, content, completed_at
) VALUES (
  $1, $2, $3, $4, 'user', 'completed', 'synthetic health source', now()
), (
  $5, $2, $3, $6, 'assistant', 'completed', 'synthetic health answer', now()
);
UPDATE messages SET parent_message_id = $1 WHERE id = $5;
SELECT memory_append_turn_completed_event(
  $7::uuid, $8::uuid, $3::uuid, $2::uuid, $1::uuid, $5::uuid,
  'server-stored', 'fixture', 'fixture-model', 2::smallint
);
`, job.sourceID, conversation, userA, index*2+1, job.assistantID, index*2+2,
			job.eventID, job.jobID)
	}

	mustExecPhase151C(t, ctx, db, `
UPDATE memory_jobs SET
  status = 'dead_letter', attempt_count = 1,
  completed_at = now() - interval '48 hours', error_code = 'EXTRACTION_INVALID',
  created_at = now() - interval '72 hours', updated_at = now() - interval '48 hours'
WHERE job_id = $1::uuid;
UPDATE memory_jobs SET
  stage = 'review_expire', source_conversation_id = NULL,
  source_message_id = NULL, assistant_message_id = NULL, source_hash = NULL,
  provider_source = NULL, provider_id = NULL, provider_record_id = NULL,
  provider_config_updated_at = NULL, model_id = NULL, processing_profile = NULL,
  project_scope_generation = NULL, max_attempts = 128,
  available_at = now() + interval '30 days'
WHERE job_id = $2::uuid;
UPDATE memory_jobs SET
  status = 'dead_letter', attempt_count = 1,
  completed_at = now(), error_code = 'SOURCE_DRIFT', updated_at = now()
WHERE job_id = $3::uuid;
UPDATE conversations SET status = 'deleted', deleted_at = now()
WHERE id = $4::uuid;
`, historical, reviewExpire, sourceDrift, conversation)
}

func assertMemoryHealthResolutionPrivileges(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	var apiExecute, workerExecute, apiRead, workerRead bool
	if err := db.QueryRowContext(ctx, `
SELECT
  has_function_privilege('go_api_runtime',
    'memory_acknowledge_job_health(uuid,uuid,text,text)', 'EXECUTE'),
  has_function_privilege('memory_worker_runtime',
    'memory_acknowledge_job_health(uuid,uuid,text,text)', 'EXECUTE'),
  has_table_privilege('go_api_runtime', 'memory_job_health_resolutions', 'SELECT'),
  has_table_privilege('memory_worker_runtime', 'memory_job_health_resolutions', 'SELECT')
`).Scan(&apiExecute, &workerExecute, &apiRead, &workerRead); err != nil {
		t.Fatal(err)
	}
	if !apiExecute || workerExecute || apiRead || workerRead {
		t.Fatalf("071 privileges = api execute:%t read:%t worker execute:%t read:%t",
			apiExecute, apiRead, workerExecute, workerRead)
	}
}

func callMemoryHealthResolutionExpectError(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID, jobID, errorCode, resolutionCode, want string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		t.Fatal(err)
	}
	var created bool
	err = tx.QueryRowContext(ctx, `
SELECT memory_acknowledge_job_health($1::uuid, $2::uuid, $3::text, $4::text)
`, userID, jobID, errorCode, resolutionCode).Scan(&created)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), want) {
		t.Fatalf("acknowledgement error = %v created=%t, want %s", err, created, want)
	}
}

func assertMemoryHealthCaptureCounts(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	pending, processing, dead int64,
) {
	t.Helper()
	assertMemoryHealthCaptureCountsWithQueryer(
		t, ctx, db, userID, pending, processing, dead,
	)
}

func assertMemoryHealthCaptureCountsWithQueryer(
	t *testing.T,
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	userID string,
	pending, processing, dead int64,
) {
	t.Helper()
	var gotPending, gotProcessing, gotDead int64
	if err := queryer.QueryRowContext(ctx, `
SELECT capture_pending_count, capture_processing_count, capture_dead_letter_count
FROM memory_user_health($1::uuid)
`, userID).Scan(&gotPending, &gotProcessing, &gotDead); err != nil {
		t.Fatal(err)
	}
	if gotPending != pending || gotProcessing != processing || gotDead != dead {
		t.Fatalf("capture health = %d/%d/%d, want %d/%d/%d",
			gotPending, gotProcessing, gotDead, pending, processing, dead)
	}
}
