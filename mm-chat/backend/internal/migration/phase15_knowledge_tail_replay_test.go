package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

var published2010D73MigrationHashes = map[string]string{
	"001_initial_schema.down.sql":                 "a70acf09ce296812d9fc0bf8f8892c7ca2e819eb880698104784646363529651",
	"001_initial_schema.up.sql":                   "013b10dd74ead5b847d0aeac1e62ad38f9532a647dbc4cea3ccf99371aaa1a2d",
	"002_messages_run_id_index.down.sql":          "6f8aa63bd938e9bb6fde01846e5a8f8624b2b8fdb6cba09f89e6b5bce541754e",
	"002_messages_run_id_index.up.sql":            "4486788cd34b42c5a2465b5f3c1c7168ec52ce7f846dfe1c171b2bbddcb6ddb4",
	"003_import_batches.down.sql":                 "81c6e947e10a3400a0513d2dbf889db6ddd3b91621211dc7939ce20e109d1065",
	"003_import_batches.up.sql":                   "ec8acf15254363c5e8f8d5c450157e2f070107f2508833c676ee0baf2184f394",
	"004_phase15_identity_knowledge_acl.down.sql": "2b2e73a893cadefb7a98071a3732e9514b3fc430d69611f724033f0724c11af9",
	"004_phase15_identity_knowledge_acl.up.sql":   "4b6448ffb309544a5236998500f42c3abe79335c0e41a3976af6f51df190295e",
	"005_phase15_team_services.down.sql":          "35446b7b5b8fb30298b4a895b3bc789061838fc7e53160828d3c498cf1246614",
	"005_phase15_team_services.up.sql":            "1c66e0a03958df882e83c372e0d1d9f62118a3bc76d4892a68f43ce8e95ef1b1",
	"006_phase15_knowledge_services.down.sql":     "f6c6372ee86b2b25b05ec55fcebaed71cff71f388b66add5bbb487c3c06c598b",
	"006_phase15_knowledge_services.up.sql":       "39d7466e61708ad379d0d755a5dd28406b75b295e6a47ff7726e5bf3383b6124",
}

func TestPhase151DKnowledgeTailFreshRollbackAndReplay(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	assertPhase151DPostgres16(t, ctx, db)

	runner := NewRunner(db, migrationfiles.FS)
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("apply fresh 001-009 migrations: %v", err)
	}
	if len(applied) != 9 || applied[len(applied)-1].ID() != "009_phase15_consent_expiry_materialization" {
		t.Fatalf("fresh migration set = %#v, want 001-009", applied)
	}
	assertPhase151DKnowledgeTailApplied(t, ctx, db)

	for _, expected := range []string{
		"009_phase15_consent_expiry_materialization",
		"008_phase15_governance_immutability",
		"007_phase15_knowledge_deletion",
	} {
		rolledBack, rollbackErr := runner.Down(ctx, false)
		if rollbackErr != nil {
			t.Fatalf("rollback %s: %v", expected, rollbackErr)
		}
		if len(rolledBack) != 1 || rolledBack[0].ID() != expected {
			t.Fatalf("rollback = %#v, want only %s", rolledBack, expected)
		}
	}
	assertPhase151CColumnAbsent(t, ctx, db, "processing_consents", "expiry_materialized_at")
	assertPhase151DDatabaseObjectCount(t, ctx, db, `
SELECT count(*)
FROM pg_trigger trigger
JOIN pg_class relation ON relation.oid = trigger.tgrelid
JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
WHERE trigger.tgname = 'processor_governance_profiles_immutable'
  AND NOT trigger.tgisinternal
  AND namespace.nspname = current_schema()
`, 0, "governance immutability trigger after rollback")
	assertPhase151DDatabaseObjectCount(t, ctx, db, `
SELECT count(*)
FROM pg_indexes
WHERE schemaname = current_schema()
  AND indexname = 'idx_knowledge_processing_jobs_purge_fence'
`, 1, "migration 006 purge fence after tail rollback")
	assertPhase151CTableExists(t, ctx, db, "knowledge_processing_jobs")

	applied, err = runner.Up(ctx)
	if err != nil {
		t.Fatalf("reapply 007-009 migrations: %v", err)
	}
	wantIDs := []string{
		"007_phase15_knowledge_deletion",
		"008_phase15_governance_immutability",
		"009_phase15_consent_expiry_materialization",
	}
	if len(applied) != len(wantIDs) {
		t.Fatalf("reapplied migrations = %#v, want %v", applied, wantIDs)
	}
	for index, want := range wantIDs {
		if applied[index].ID() != want {
			t.Fatalf("reapplied migration %d = %s, want %s", index, applied[index].ID(), want)
		}
	}
	assertPhase151DKnowledgeTailApplied(t, ctx, db)

	applied, err = runner.Up(ctx)
	if err != nil {
		t.Fatalf("no-op replay 001-009 migrations: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("no-op replay changed migrations = %#v", applied)
	}

	var checksum string
	if err := db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = 9`).Scan(&checksum); err != nil {
		t.Fatalf("read migration 009 checksum: %v", err)
	}
	if len(checksum) != 64 {
		t.Fatalf("migration 009 checksum length = %d, want 64", len(checksum))
	}
	mustExecPhase151C(t, ctx, db, `UPDATE schema_migrations SET checksum = 'drift' WHERE version = 9`)
	if _, err := runner.Up(ctx); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("migration checksum drift error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `UPDATE schema_migrations SET checksum = $1, name = 'drifted_name' WHERE version = 9`, checksum)
	if _, err := runner.Up(ctx); err == nil || !strings.Contains(err.Error(), "name mismatch") {
		t.Fatalf("migration name drift error = %v", err)
	}
	mustExecPhase151C(t, ctx, db, `UPDATE schema_migrations SET name = 'phase15_consent_expiry_materialization', checksum = NULL WHERE version = 9`)
	if _, err := runner.Up(ctx); err == nil || !strings.Contains(err.Error(), "migrate baseline") {
		t.Fatalf("legacy checksum fail-closed error = %v", err)
	}
	baselinedMigrations, err := runner.BaselineLegacyChecksums(ctx)
	if err != nil || len(baselinedMigrations) != 1 || baselinedMigrations[0].Version != 9 {
		t.Fatalf("explicit legacy checksum baseline = %#v, err=%v", baselinedMigrations, err)
	}
	applied, err = runner.Up(ctx)
	if err != nil || len(applied) != 0 {
		t.Fatalf("post-baseline replay = %#v, err=%v", applied, err)
	}
	var baselined string
	if err := db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = 9`).Scan(&baselined); err != nil {
		t.Fatalf("read baselined migration 009 checksum: %v", err)
	}
	if baselined != checksum {
		t.Fatalf("baselined checksum = %q, want %q", baselined, checksum)
	}
}

func TestPhase151DPublished2010D73UpgradesToCurrent009(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	assertPhase151DPostgres16(t, ctx, db)

	migrations, err := Load(migrationfiles.FS)
	if err != nil {
		t.Fatalf("load current migrations: %v", err)
	}
	if len(migrations) < 9 {
		t.Fatalf("current migration count = %d, want at least 9", len(migrations))
	}

	mustExecPhase151C(t, ctx, db, `
CREATE TABLE schema_migrations (
  version BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`)
	for _, migration := range migrations[:6] {
		assertPublished2010D73MigrationFile(t, migration.UpPath)
		assertPublished2010D73MigrationFile(t, migration.DownPath)
		up, readErr := fs.ReadFile(migrationfiles.FS, migration.UpPath)
		if readErr != nil {
			t.Fatalf("read historical migration %s: %v", migration.ID(), readErr)
		}
		tx, beginErr := db.BeginTx(ctx, nil)
		if beginErr != nil {
			t.Fatalf("begin historical migration %s: %v", migration.ID(), beginErr)
		}
		if _, execErr := tx.ExecContext(ctx, string(up)); execErr != nil {
			_ = tx.Rollback()
			t.Fatalf("apply historical migration %s: %v", migration.ID(), execErr)
		}
		if _, execErr := tx.ExecContext(
			ctx,
			`INSERT INTO schema_migrations(version,name) VALUES ($1,$2)`,
			migration.Version,
			migration.Name,
		); execErr != nil {
			_ = tx.Rollback()
			t.Fatalf("record historical migration %s: %v", migration.ID(), execErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			t.Fatalf("commit historical migration %s: %v", migration.ID(), commitErr)
		}
	}

	runner := NewRunner(db, migrationfiles.FS)
	if _, err := runner.Up(ctx); err == nil || !strings.Contains(err.Error(), "migrate baseline") {
		t.Fatalf("historical migration without baseline error = %v", err)
	}
	baselined, err := runner.BaselineLegacyChecksums(ctx)
	if err != nil {
		t.Fatalf("baseline published 001-006: %v", err)
	}
	if len(baselined) != 6 || baselined[len(baselined)-1].ID() != "006_phase15_knowledge_services" {
		t.Fatalf("published baseline set = %#v, want 001-006", baselined)
	}
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("upgrade published 006 to current 009: %v", err)
	}
	if len(applied) != 3 || applied[0].Version != 7 || applied[2].Version != 9 {
		t.Fatalf("published upgrade set = %#v, want 007-009", applied)
	}
	assertPhase151DKnowledgeTailApplied(t, ctx, db)
}

func assertPublished2010D73MigrationFile(t *testing.T, filePath string) {
	t.Helper()
	want, ok := published2010D73MigrationHashes[filePath]
	if !ok {
		t.Fatalf("published 2010d73 manifest has no entry for %s", filePath)
	}
	contents, err := fs.ReadFile(migrationfiles.FS, filePath)
	if err != nil {
		t.Fatalf("read published migration fixture %s: %v", filePath, err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(contents))
	if got != want {
		t.Fatalf("published migration fixture %s hash = %s, want %s", filePath, got, want)
	}
}

func TestPhase151DConcurrentMigratorsSerialize(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	assertPhase151DPostgres16(t, ctx, db)

	type result struct {
		changed []Migration
		err     error
	}
	runConcurrent := func(operation func(*Runner) ([]Migration, error)) []result {
		t.Helper()
		start := make(chan struct{})
		results := make(chan result, 2)
		var wait sync.WaitGroup
		for range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				changed, err := operation(NewRunner(db, migrationfiles.FS))
				results <- result{changed: changed, err: err}
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		values := make([]result, 0, 2)
		for value := range results {
			values = append(values, value)
		}
		return values
	}

	upResults := runConcurrent(func(runner *Runner) ([]Migration, error) {
		return runner.Up(ctx)
	})
	var applied int
	for _, value := range upResults {
		if value.err != nil {
			t.Fatalf("concurrent Up error: %v", value.err)
		}
		applied += len(value.changed)
	}
	if applied != 9 {
		t.Fatalf("concurrent Up total changes = %d, want 9", applied)
	}

	mustExecPhase151C(t, ctx, db, `UPDATE schema_migrations SET checksum = NULL`)
	baselineResults := runConcurrent(func(runner *Runner) ([]Migration, error) {
		return runner.BaselineLegacyChecksums(ctx)
	})
	var baselined int
	for _, value := range baselineResults {
		if value.err != nil {
			t.Fatalf("concurrent baseline error: %v", value.err)
		}
		baselined += len(value.changed)
	}
	if baselined != 9 {
		t.Fatalf("concurrent baseline total changes = %d, want 9", baselined)
	}
}

func TestPhase151DMigratorWaitsForAdvisoryLock(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	assertPhase151DPostgres16(t, ctx, db)

	blocker, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	if _, err := blocker.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		t.Fatalf("acquire blocking advisory lock: %v", err)
	}
	locked := true
	defer func() {
		if locked {
			_, _ = blocker.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID)
		}
	}()

	type result struct {
		changed []Migration
		err     error
	}
	done := make(chan result, 1)
	go func() {
		changed, runErr := NewRunner(db, migrationfiles.FS).Up(ctx)
		done <- result{changed: changed, err: runErr}
	}()
	select {
	case value := <-done:
		t.Fatalf("migrator bypassed held advisory lock: changed=%d err=%v", len(value.changed), value.err)
	case <-time.After(100 * time.Millisecond):
	}

	var blockerPID int
	if err := blocker.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("read blocker backend pid: %v", err)
	}
	var waitingPID int
	if err := db.QueryRowContext(ctx, `
SELECT pid
FROM pg_stat_activity
WHERE pid <> $1
  AND wait_event_type = 'Lock'
  AND wait_event = 'advisory'
ORDER BY pid
LIMIT 1
`, blockerPID).Scan(&waitingPID); err != nil {
		t.Fatalf("find distinct advisory-lock waiter: %v", err)
	}
	if waitingPID == blockerPID {
		t.Fatalf("advisory lock waiter pid = blocker pid %d", blockerPID)
	}

	var unlocked bool
	if err := blocker.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID).Scan(&unlocked); err != nil || !unlocked {
		t.Fatalf("release blocking advisory lock = %v, err=%v", unlocked, err)
	}
	locked = false
	value := <-done
	if value.err != nil || len(value.changed) != 9 {
		t.Fatalf("migrator after advisory unlock: changed=%d err=%v", len(value.changed), value.err)
	}
}

func assertPhase151DPostgres16(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var rawVersion string
	if err := db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&rawVersion); err != nil {
		t.Fatalf("read PostgreSQL server_version_num: %v", err)
	}
	version, err := strconv.Atoi(rawVersion)
	if err != nil {
		t.Fatalf("parse PostgreSQL server_version_num %q: %v", rawVersion, err)
	}
	if major := version / 10000; major != 16 {
		t.Fatalf("PostgreSQL major version = %d, want 16 for promotion", major)
	}
}

func assertPhase151DKnowledgeTailApplied(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	assertPhase151CColumnExists(t, ctx, db, "processing_consents", "expiry_materialized_at")
	assertPhase151DDatabaseObjectCount(t, ctx, db, `
SELECT count(*)
FROM pg_trigger trigger
JOIN pg_class relation ON relation.oid = trigger.tgrelid
JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
WHERE trigger.tgname = 'processor_governance_profiles_immutable'
  AND NOT trigger.tgisinternal
  AND namespace.nspname = current_schema()
`, 1, "governance immutability trigger")
	assertPhase151DDatabaseObjectCount(t, ctx, db, `
SELECT count(*)
FROM pg_indexes
WHERE schemaname = current_schema()
  AND indexname = 'idx_knowledge_processing_jobs_purge_fence'
`, 1, "purge fence index")
	assertPhase151DDatabaseObjectCount(t, ctx, db, `
SELECT count(*)
FROM pg_indexes
WHERE schemaname = current_schema()
  AND indexname = 'idx_processing_consents_expiry_due'
`, 1, "consent expiry due index")
}

func assertPhase151DDatabaseObjectCount(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	query string,
	want int,
	label string,
) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(ctx, query).Scan(&got); err != nil {
		t.Fatalf("query %s: %v", label, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", label, got, want)
	}
}
