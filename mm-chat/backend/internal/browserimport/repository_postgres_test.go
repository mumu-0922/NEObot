package browserimport

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresRepositoryCommitsReplaysAndRollsBackChatImport(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	batchID := mustIntegrationUUID(t)
	conversationID := mustIntegrationUUID(t)
	firstMessageID := mustIntegrationUUID(t)
	secondMessageID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, conversationID, firstMessageID, secondMessageID)
	manifest := validManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := repo.Commit(ctx, pkg)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if response.BatchID != batchID || response.Status != "completed" {
		t.Fatalf("response identity = %#v", response)
	}
	if response.Created.Conversations != 1 || response.Created.Messages != 2 || response.Created.Files != 0 {
		t.Fatalf("created counts = %#v", response.Created)
	}
	if response.Mappings.Conversations["conversation-client-1"] != conversationID {
		t.Fatalf("conversation mapping = %#v", response.Mappings.Conversations)
	}
	assertImportedRows(t, ctx, db, response.BatchID, firstMessageID)

	replayed, err := repo.Commit(ctx, pkg)
	if err != nil {
		t.Fatalf("replay Commit() error = %v", err)
	}
	if replayed.BatchID != response.BatchID || replayed.Created.Messages != response.Created.Messages {
		t.Fatalf("replay response = %#v, want %#v", replayed, response)
	}

	status, err := repo.GetBatchStatus(ctx, response.BatchID)
	if err != nil {
		t.Fatalf("GetBatchStatus() error = %v", err)
	}
	if status.Status != "completed" || status.BatchID != response.BatchID || status.CreatedAt == "" {
		t.Fatalf("batch status = %#v", status)
	}

	if err := repo.Rollback(ctx, response.BatchID); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	assertRolledBack(t, ctx, db, response.BatchID)
}

func TestPostgresRepositoryRejectsIdempotencyKeyForDifferentPackage(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	batchID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t))
	manifest := validManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := repo.Commit(ctx, pkg); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	changedManifest := validManifest()
	changedManifest.IdempotencyKey = manifest.IdempotencyKey
	changedManifest.Conversations[0].Title = "Changed"
	changedPkg := readPackageFromManifest(t, changedManifest)
	if _, err := repo.Commit(ctx, changedPkg); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("Commit() changed package error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestPostgresRepositoryReplaysConcurrentSamePackage(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repoA := NewPostgresRepository(db)
	repoB := NewPostgresRepository(db)
	batchID := mustIntegrationUUID(t)
	repoA.newID = deterministicIDs(t, batchID, mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t))
	repoB.newID = deterministicIDs(t, mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t))
	manifest := validManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := make(chan struct{})
	responses := make([]CommitResponse, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for index, repo := range []*PostgresRepository{repoA, repoB} {
		wg.Add(1)
		go func(index int, repo *PostgresRepository) {
			defer wg.Done()
			<-start
			responses[index], errs[index] = repo.Commit(ctx, pkg)
		}(index, repo)
	}
	close(start)
	wg.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf("commit %d error = %v", index, err)
		}
	}
	if responses[0].BatchID != responses[1].BatchID {
		t.Fatalf("batch ids = %q/%q, want same", responses[0].BatchID, responses[1].BatchID)
	}
}

func TestPostgresRepositoryRollbackRejectsModifiedImportRows(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	batchID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t))
	manifest := validManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := repo.Commit(ctx, pkg)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE conversations
SET title = title || ' changed', updated_at = now() + interval '1 second'
WHERE id = $1
`, response.Mappings.Conversations["conversation-client-1"]); err != nil {
		t.Fatalf("mark imported conversation modified: %v", err)
	}
	if err := repo.Rollback(ctx, response.BatchID); !errors.Is(err, ErrBatchModified) {
		t.Fatalf("Rollback() error = %v, want ErrBatchModified", err)
	}
}

func TestPostgresRepositoryRollbackRejectsModifiedImportedMessages(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	batchID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t))
	manifest := validManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := repo.Commit(ctx, pkg)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE messages
SET content = content || ' changed', updated_at = now() + interval '1 second'
WHERE id = $1
`, response.Mappings.Messages["message-client-1"]); err != nil {
		t.Fatalf("mark imported message modified: %v", err)
	}
	if err := repo.Rollback(ctx, response.BatchID); !errors.Is(err, ErrBatchModified) {
		t.Fatalf("Rollback() error = %v, want ErrBatchModified", err)
	}
}

func TestPostgresMigrationUpDownIncludesImportBatches(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runner := migration.NewRunner(db, migrationfiles.FS)
	for {
		changed, err := runner.Down(ctx, false)
		if err != nil {
			t.Fatalf("Down(false) error = %v", err)
		}
		if len(changed) != 1 {
			t.Fatalf("down changed = %#v, want one migration", changed)
		}
		if changed[0].ID() == "003_import_batches" {
			break
		}
	}
	var tableName sql.NullString
	err := db.QueryRowContext(ctx, `SELECT to_regclass('public.import_batches')::text`).Scan(&tableName)
	if err != nil {
		t.Fatalf("query import_batches regclass: %v", err)
	}
	if tableName.Valid {
		t.Fatalf("import_batches regclass = %q, want empty after down", tableName.String)
	}
	changed, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("Up() after down error = %v", err)
	}
	foundImportBatch := false
	for _, migration := range changed {
		if migration.ID() == "003_import_batches" {
			foundImportBatch = true
		}
	}
	if !foundImportBatch {
		t.Fatalf("up changed = %#v, want 003_import_batches included", changed)
	}
}

func readPackageFromManifest(t *testing.T, manifest Manifest) Package {
	t.Helper()
	pkg, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(t, manifest)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(filterIssues(issues, "error")) != 0 {
		t.Fatalf("package issues = %#v", issues)
	}
	return pkg
}

func assertImportedRows(t *testing.T, ctx context.Context, db *sql.DB, batchID string, firstMessageID string) {
	t.Helper()
	var conversationCount int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM conversations
WHERE metadata #>> '{import,batchId}' = $1
  AND deleted_at IS NULL
`, batchID).Scan(&conversationCount); err != nil {
		t.Fatalf("query conversation count: %v", err)
	}
	if conversationCount != 1 {
		t.Fatalf("conversation count = %d, want 1", conversationCount)
	}

	var messageCount int
	var parentID string
	var assistantRole string
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*), max(parent_message_id::text), max(role) FILTER (WHERE sequence_no = 1)
FROM messages
WHERE metadata #>> '{import,batchId}' = $1
  AND deleted_at IS NULL
`, batchID).Scan(&messageCount, &parentID, &assistantRole); err != nil {
		t.Fatalf("query message import rows: %v", err)
	}
	if messageCount != 2 || parentID != firstMessageID || assistantRole != "assistant" {
		t.Fatalf("messages count/parent/role = %d/%s/%s", messageCount, parentID, assistantRole)
	}
}

func assertRolledBack(t *testing.T, ctx context.Context, db *sql.DB, batchID string) {
	t.Helper()
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM import_batches WHERE id = $1`, batchID).Scan(&status); err != nil {
		t.Fatalf("query batch status after rollback: %v", err)
	}
	if status != "rolled_back" {
		t.Fatalf("batch status = %q, want rolled_back", status)
	}
	var activeRows int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM (
  SELECT id FROM conversations WHERE metadata #>> '{import,batchId}' = $1 AND deleted_at IS NULL
  UNION ALL
  SELECT id FROM messages WHERE metadata #>> '{import,batchId}' = $1 AND deleted_at IS NULL
) rows
`, batchID).Scan(&activeRows); err != nil {
		t.Fatalf("query active imported rows after rollback: %v", err)
	}
	if activeRows != 0 {
		t.Fatalf("active imported rows = %d, want 0", activeRows)
	}
}

func deterministicIDs(t *testing.T, ids ...string) func() (string, error) {
	t.Helper()
	index := 0
	return func() (string, error) {
		if index >= len(ids) {
			t.Fatalf("deterministicIDs exhausted")
			return "", nil
		}
		id := ids[index]
		index++
		return id, nil
	}
}

func mustIntegrationUUID(t *testing.T) string {
	t.Helper()
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID() error = %v", err)
	}
	return id
}

func openPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}
	pgxConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	pgxConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*pgxConfig)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}
