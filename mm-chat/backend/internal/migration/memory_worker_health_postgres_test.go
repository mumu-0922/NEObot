package migration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestMemoryWorkerHealthLivePostgres(t *testing.T) {
	db := openMemoryLexicalMigrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	base := NewRunner(db, phase15MigrationFSThrough(t, 69))
	if _, err := base.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Up(ctx); err != nil {
		t.Fatalf("apply through 069: %v", err)
	}
	runner := NewRunner(db, phase15MigrationFSThrough(t, 70))
	if _, err := runner.WithPhase15GovernanceMapping(Phase15GovernanceMapping{}); err != nil {
		t.Fatal(err)
	}
	applied, err := runner.Up(ctx)
	if err != nil || len(applied) != 1 || applied[0].Version != 70 {
		t.Fatalf("apply 070 = %#v/%v", applied, err)
	}
	assertMemoryWorkerHealthPrivileges(t, ctx, db)

	const (
		userA     = "17000000-0000-4000-8000-000000000001"
		userB     = "17000000-0000-4000-8000-000000000002"
		readyID   = "57000000-0000-4000-8000-000000000001"
		pendingID = "57000000-0000-4000-8000-000000000002"
		failedID  = "57000000-0000-4000-8000-000000000003"
		foreignID = "57000000-0000-4000-8000-000000000004"
		workerID  = "77000000-0000-4000-8000-000000000001"
	)
	assertMemoryWorkerHeartbeatRejectsInvalidInput(t, ctx, db, workerID)
	mustExecPhase151C(t, ctx, db, `
INSERT INTO users(id, display_name) VALUES ($1, 'Health A'), ($2, 'Health B');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled
) VALUES ($1, true, true, false), ($2, true, true, false);
`, userA, userB)
	for _, memory := range []struct {
		id, userID, content string
	}{
		{readyID, userA, "Health ready fixture"},
		{pendingID, userA, "Health pending fixture"},
		{failedID, userA, "Health failed fixture"},
		{foreignID, userB, "Foreign health fixture"},
	} {
		var returned string
		if err := db.QueryRowContext(ctx, `
SELECT id::text FROM memory_upsert_global_manual(
  $1::uuid, $2::uuid, 'fact', $3::text, lower($3::text),
  3::smallint, ARRAY[]::text[], NULL, NULL, true
)
`, memory.id, memory.userID, memory.content).Scan(&returned); err != nil || returned != memory.id {
			t.Fatalf("create health Memory %s = %q/%v", memory.id, returned, err)
		}
	}
	setMemoryHybridProjectionReady(t, ctx, db, readyID, memoryHybridVectorLiteral(0))
	setMemoryHybridProjectionReady(t, ctx, db, foreignID, memoryHybridVectorLiteral(1))
	mustExecPhase151C(t, ctx, db, `
UPDATE user_memory_search_projections
SET embedding_status = 'failed', embedding_vector = NULL,
    embedding_error_code = 'FIXTURE_FAILED',
    embedding_updated_at = clock_timestamp(), updated_at = clock_timestamp()
WHERE memory_id = $1::uuid;
`, failedID)

	recordMemoryWorkerHeartbeat(t, ctx, db, workerID, true)
	assertMemoryWorkerUserHealth(t, ctx, db, userA, true, true, 1, 1, 1)
	assertMemoryWorkerRuntimeReadiness(t, ctx, db, true)

	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "memory_health_rollback_requires_stopped_workers") {
		t.Fatalf("active 070 down error = %v", err)
	}
	retireMemoryWorkerHeartbeat(t, ctx, db, workerID)
	down, err := runner.Down(ctx, false)
	if err != nil || len(down) != 1 || down[0].Version != 70 {
		t.Fatalf("070 clean down = %#v/%v", down, err)
	}
	assertMemoryWorkerRuntimeReadiness(t, ctx, db, true)
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 1 || reapplied[0].Version != 70 {
		t.Fatalf("070 re-up = %#v/%v", reapplied, err)
	}
	assertMemoryWorkerHealthPrivileges(t, ctx, db)
	assertMemoryWorkerUserHealth(t, ctx, db, userA, false, false, 1, 1, 1)
}

func assertMemoryWorkerHeartbeatRejectsInvalidInput(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	workerID string,
) {
	t.Helper()
	for _, test := range []struct {
		query string
		args  []any
	}{
		{query: `SELECT memory_worker_heartbeat(NULL::uuid, 20, true)`},
		{query: `SELECT memory_worker_heartbeat($1::uuid, NULL::integer, true)`, args: []any{workerID}},
		{query: `SELECT memory_worker_heartbeat($1::uuid, 20, NULL::boolean)`, args: []any{workerID}},
		{query: `SELECT memory_worker_heartbeat($1::uuid, 4, true)`, args: []any{workerID}},
		{query: `SELECT memory_worker_heartbeat($1::uuid, 121, true)`, args: []any{workerID}},
	} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		var accepted bool
		err = tx.QueryRowContext(ctx, test.query, test.args...).Scan(&accepted)
		_ = tx.Rollback()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "memory_worker_heartbeat_invalid") {
			t.Fatalf("invalid heartbeat query %q = accepted:%t err:%v", test.query, accepted, err)
		}
	}
}

func recordMemoryWorkerHeartbeat(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	workerID string,
	embeddingEnabled bool,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatal(err)
	}
	var accepted bool
	if err := tx.QueryRowContext(ctx, `
SELECT memory_worker_heartbeat($1::uuid, 20, $2)
`, workerID, embeddingEnabled).Scan(&accepted); err != nil || !accepted {
		t.Fatalf("worker heartbeat = %t/%v", accepted, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func retireMemoryWorkerHeartbeat(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	workerID string,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatal(err)
	}
	var retired bool
	if err := tx.QueryRowContext(ctx, `
SELECT memory_worker_retire($1::uuid)
`, workerID).Scan(&retired); err != nil || !retired {
		t.Fatalf("worker retire = %t/%v", retired, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func assertMemoryWorkerUserHealth(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID string,
	wantWorker bool,
	wantEmbedding bool,
	wantReady int64,
	wantPending int64,
	wantFailed int64,
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
	var worker, embedding bool
	var capturePending, captureProcessing, captureDead int64
	var ready, pending, failed int64
	if err := tx.QueryRowContext(ctx, `
SELECT worker_available, embedding_worker_available,
       capture_pending_count, capture_processing_count,
       capture_dead_letter_count, projection_ready_count,
       projection_pending_count, projection_failed_count
FROM memory_user_health($1::uuid)
`, userID).Scan(
		&worker, &embedding, &capturePending, &captureProcessing, &captureDead,
		&ready, &pending, &failed,
	); err != nil {
		t.Fatal(err)
	}
	if worker != wantWorker || embedding != wantEmbedding ||
		capturePending != 0 || captureProcessing != 0 || captureDead != 0 ||
		ready != wantReady || pending != wantPending || failed != wantFailed {
		t.Fatalf("user health = worker:%t embedding:%t capture:%d/%d/%d projection:%d/%d/%d",
			worker, embedding, capturePending, captureProcessing, captureDead,
			ready, pending, failed)
	}
}

func assertMemoryWorkerRuntimeReadiness(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	want bool,
) {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE memory_worker_runtime`); err != nil {
		t.Fatal(err)
	}
	var ready bool
	if err := tx.QueryRowContext(ctx, `
SELECT consumer_ready FROM memory_worker_readiness()
`).Scan(&ready); err != nil || ready != want {
		t.Fatalf("worker readiness = %t/%v, want %t", ready, err, want)
	}
}

func assertMemoryWorkerHealthPrivileges(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) {
	t.Helper()
	var workerHeartbeat, workerHealth, apiHeartbeat, apiHealth bool
	var workerTableRead, apiTableRead bool
	if err := db.QueryRowContext(ctx, `
SELECT
  has_function_privilege('memory_worker_runtime',
    'memory_worker_heartbeat(uuid,integer,boolean)', 'EXECUTE'),
  has_function_privilege('memory_worker_runtime',
    'memory_user_health(uuid)', 'EXECUTE'),
  has_function_privilege('go_api_runtime',
    'memory_worker_heartbeat(uuid,integer,boolean)', 'EXECUTE'),
  has_function_privilege('go_api_runtime',
    'memory_user_health(uuid)', 'EXECUTE'),
  has_table_privilege('memory_worker_runtime', 'memory_worker_heartbeats', 'SELECT'),
  has_table_privilege('go_api_runtime', 'memory_worker_heartbeats', 'SELECT')
`).Scan(
		&workerHeartbeat, &workerHealth, &apiHeartbeat, &apiHealth,
		&workerTableRead, &apiTableRead,
	); err != nil {
		t.Fatal(err)
	}
	if !workerHeartbeat || workerHealth || apiHeartbeat || !apiHealth ||
		workerTableRead || apiTableRead {
		t.Fatalf("070 privileges = worker heartbeat:%t health:%t table:%t api heartbeat:%t health:%t table:%t",
			workerHeartbeat, workerHealth, workerTableRead,
			apiHeartbeat, apiHealth, apiTableRead)
	}
}
