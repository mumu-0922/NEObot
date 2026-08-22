package migration

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestRAGFailureStateProjectionMigrationIsForwardOnlyAndLeastPrivilege(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("104_rag_failure_state_projection.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("104_rag_failure_state_projection.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down := string(upBytes), string(downBytes)

	for _, required := range []string{
		"CREATE OR REPLACE FUNCTION knowledge_claim_processing_job(",
		"CREATE OR REPLACE FUNCTION knowledge_finish_processing_job(",
		"UPDATE knowledge_document_versions AS version",
		"version.status IN ('uploaded', 'processing')",
		"job.stage IN ('parse', 'passage_embedding')",
		"terminal_error_code := 'MAX_ATTEMPTS_EXCEEDED'",
		"SET search_path FROM CURRENT",
		"OWNER TO rag_projection_owner",
		"FROM PUBLIC",
		"TO rag_worker_executor",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("104 up migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE schema_migrations",
		"DELETE FROM schema_migrations",
		"UPDATE knowledge_documents",
		"GRANT EXECUTE ON FUNCTION knowledge_finish_processing_job(\n  UUID, UUID, UUID, TEXT, TEXT, INTEGER\n) TO go_api_runtime",
	} {
		if strings.Contains(strings.ToUpper(up), strings.ToUpper(forbidden)) {
			t.Fatalf("104 up migration contains forbidden contract %q", forbidden)
		}
	}
	if !strings.Contains(down, "intentionally keeps the known-good behavior and backfill") ||
		strings.Contains(strings.ToUpper(down), "DROP FUNCTION") {
		t.Fatal("104 down migration must retain corrected function and Version state")
	}
}

func TestRAGFailureStateProjectionLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	migrations := ragFailureStateProjectionTestFS(t)
	runner := NewRunner(db, migrations)
	applied, err := NewRunner(db, phase15MigrationFSThrough(t, 17)).Up(ctx)
	if err != nil {
		t.Fatalf("apply migrations through 017: %v", err)
	}
	if len(applied) < 17 || applied[len(applied)-1].Version != 17 {
		t.Fatalf("applied migrations = %#v, want head 017", applied)
	}

	fixture := seedPhase15RAGParseProjectionFixture(t, ctx, db)
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_processing_jobs
SET status='failed',lease_owner=NULL,lease_token=NULL,lease_expires_at=NULL,
    completed_at=clock_timestamp(),error_code='FORMAT_UNSUPPORTED',
    updated_at=clock_timestamp()
WHERE id=$1`, fixture.jobID)

	applied, err = runner.Up(ctx)
	if err != nil {
		t.Fatalf("apply failure-state projection migration: %v", err)
	}
	if len(applied) != 1 || applied[0].Version != 104 {
		t.Fatalf("applied migration = %#v, want only 104", applied)
	}
	assertRAGFailureVersionState(
		t, ctx, db, fixture.documentVersionID, "failed", "FORMAT_UNSUPPORTED",
	)

	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_document_versions
SET status='uploaded',error_code=NULL,updated_at=clock_timestamp()
WHERE id=$1;
UPDATE knowledge_processing_jobs
SET status='processing',attempt_count=1,max_attempts=3,
    lease_owner=$2,lease_token=$3,lease_expires_at=clock_timestamp()+interval '10 minutes',
    completed_at=NULL,error_code=NULL,updated_at=clock_timestamp()
WHERE id=$4`, fixture.documentVersionID, fixture.workerID, fixture.leaseToken, fixture.jobID)
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_finish_processing_job($1,$2,$3,'retry','TRANSIENT_FAILURE',0)`,
		fixture.jobID, fixture.workerID, fixture.leaseToken)
	assertRAGFailureVersionState(t, ctx, db, fixture.documentVersionID, "uploaded", "")

	secondLease := "00000000-0000-0000-0000-0000000b1020"
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_processing_jobs
SET status='processing',attempt_count=3,max_attempts=3,
    lease_owner=$1,lease_token=$2,lease_expires_at=clock_timestamp()+interval '10 minutes',
    completed_at=NULL,error_code=NULL,updated_at=clock_timestamp()
WHERE id=$3`, fixture.workerID, secondLease, fixture.jobID)
	mustExecPhase151C(t, ctx, db, `
SELECT knowledge_finish_processing_job($1,$2,$3,'retry','TRANSIENT_FAILURE',0)`,
		fixture.jobID, fixture.workerID, secondLease)
	assertRAGFailureVersionState(
		t, ctx, db, fixture.documentVersionID, "failed", "MAX_ATTEMPTS_EXCEEDED",
	)

	thirdLease := "00000000-0000-0000-0000-0000000b1021"
	mustExecPhase151C(t, ctx, db, `
UPDATE knowledge_document_versions
SET status='uploaded',error_code=NULL,updated_at=clock_timestamp()
WHERE id=$1;
UPDATE knowledge_processing_jobs
SET status='processing',attempt_count=3,max_attempts=3,
    created_at=clock_timestamp()-interval '1 hour',
    lease_owner=$2,lease_token=$3,lease_expires_at=clock_timestamp()-interval '1 minute',
    completed_at=NULL,error_code=NULL,updated_at=clock_timestamp()
WHERE id=$4`, fixture.documentVersionID, fixture.workerID, thirdLease, fixture.jobID)
	claimLease := "00000000-0000-0000-0000-0000000b1022"
	mustExecPhase151C(t, ctx, db, `
SELECT count(*) FROM knowledge_claim_processing_job($1,$2,30,ARRAY['parse'])`,
		fixture.workerID, claimLease)
	assertRAGFailureVersionState(
		t, ctx, db, fixture.documentVersionID, "failed", "MAX_ATTEMPTS_EXCEEDED",
	)

	rolledBack, err := runner.Down(ctx, false)
	if err != nil {
		t.Fatalf("down 104: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0].Version != 104 {
		t.Fatalf("rolled back migration = %#v, want only 104", rolledBack)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("reapply 104: %v", err)
	}
	assertRAGFailureVersionState(
		t, ctx, db, fixture.documentVersionID, "failed", "MAX_ATTEMPTS_EXCEEDED",
	)
}

func ragFailureStateProjectionTestFS(t *testing.T) fstest.MapFS {
	t.Helper()
	result := phase15MigrationFSThrough(t, 17)
	for _, path := range []string{
		"104_rag_failure_state_projection.up.sql",
		"104_rag_failure_state_projection.down.sql",
	} {
		contents, err := fs.ReadFile(migrationfiles.FS, path)
		if err != nil {
			t.Fatal(err)
		}
		result[path] = &fstest.MapFile{Data: contents}
	}
	return result
}

func assertRAGFailureVersionState(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	versionID string,
	wantStatus string,
	wantErrorCode string,
) {
	t.Helper()
	var status string
	var errorCode *string
	if err := db.QueryRowContext(ctx, `
SELECT status,error_code FROM knowledge_document_versions WHERE id=$1`, versionID).
		Scan(&status, &errorCode); err != nil {
		t.Fatalf("read document Version failure state: %v", err)
	}
	gotErrorCode := ""
	if errorCode != nil {
		gotErrorCode = *errorCode
	}
	if status != wantStatus || gotErrorCode != wantErrorCode {
		t.Fatalf("Version state = %s/%s, want %s/%s", status, gotErrorCode, wantStatus, wantErrorCode)
	}
}
