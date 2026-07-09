package browserimport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	"neo-chat/mm-chat/backend/internal/storage"
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

func TestPostgresRepositoryCommitsFileAttachmentsAndRollsBackObjects(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	store := newImportFakeObjectStore()
	repo := NewPostgresRepository(db, WithObjectStore(store), WithStorageBackend("local"))
	batchID := mustIntegrationUUID(t)
	fileID := mustIntegrationUUID(t)
	conversationID := mustIntegrationUUID(t)
	firstMessageID := mustIntegrationUUID(t)
	attachmentID := mustIntegrationUUID(t)
	secondMessageID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, fileID, conversationID, firstMessageID, attachmentID, secondMessageID)
	manifest := validFileManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := repo.Commit(ctx, pkg)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if response.Created.Files != 1 || response.Created.Attachments != 1 {
		t.Fatalf("created counts = %#v, want one file and one attachment", response.Created)
	}
	if response.Mappings.Files["file-client-1"] != fileID {
		t.Fatalf("file mapping = %#v", response.Mappings.Files)
	}
	objectKey := importedObjectKey(DevUserID, fileID)
	if got := string(store.objects[objectKey].payload); got != "hello" {
		t.Fatalf("stored object = %q, want hello; objects=%#v", got, store.objects)
	}
	assertImportedFileRows(t, ctx, db, batchID, fileID, firstMessageID, attachmentID)
	encodedResponse, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if bytes.Contains(encodedResponse, []byte(objectKey)) || bytes.Contains(encodedResponse, []byte("object_key")) {
		t.Fatalf("response leaks object key: %s", encodedResponse)
	}

	if err := repo.Rollback(ctx, response.BatchID); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, ok := store.objects[objectKey]; ok {
		t.Fatalf("object %s still exists after rollback", objectKey)
	}
	assertRolledBack(t, ctx, db, response.BatchID)
	assertImportedFilesRolledBack(t, ctx, db, response.BatchID)
}

func TestPostgresRepositoryEnforcesTwoUserImportIsolation(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	store := newImportFakeObjectStore()
	repoA := NewPostgresRepository(db, WithObjectStore(store), WithStorageBackend("local"))
	repoB := NewPostgresRepository(db, WithObjectStore(store), WithStorageBackend("local"))

	batchAID := mustIntegrationUUID(t)
	fileAID := mustIntegrationUUID(t)
	conversationAID := mustIntegrationUUID(t)
	firstMessageAID := mustIntegrationUUID(t)
	attachmentAID := mustIntegrationUUID(t)
	secondMessageAID := mustIntegrationUUID(t)
	repoA.newID = deterministicIDs(t, batchAID, fileAID, conversationAID, firstMessageAID, attachmentAID, secondMessageAID)

	batchBID := mustIntegrationUUID(t)
	fileBID := mustIntegrationUUID(t)
	conversationBID := mustIntegrationUUID(t)
	firstMessageBID := mustIntegrationUUID(t)
	attachmentBID := mustIntegrationUUID(t)
	secondMessageBID := mustIntegrationUUID(t)
	repoB.newID = deterministicIDs(t, batchBID, fileBID, conversationBID, firstMessageBID, attachmentBID, secondMessageBID)

	manifest := validFileManifest()
	manifest.IdempotencyKey = "shared-import-" + batchAID
	pkg := readPackageFromManifest(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")})

	userAID := mustIntegrationUUID(t)
	userBID := mustIntegrationUUID(t)
	baseA := auth.WithUser(context.Background(), auth.User{ID: userAID, DisplayName: "User A"})
	ctxA, cancelA := context.WithTimeout(baseA, 5*time.Second)
	defer cancelA()
	baseB := auth.WithUser(context.Background(), auth.User{ID: userBID, DisplayName: "User B"})
	ctxB, cancelB := context.WithTimeout(baseB, 5*time.Second)
	defer cancelB()

	responseA, err := repoA.Commit(ctxA, pkg)
	if err != nil {
		t.Fatalf("Commit(user A) error = %v", err)
	}
	if responseA.BatchID != batchAID || responseA.Mappings.Files["file-client-1"] != fileAID {
		t.Fatalf("response A = %#v", responseA)
	}
	objectKeyA := importedObjectKey(userAID, fileAID)
	if _, ok := store.objects[objectKeyA]; !ok {
		t.Fatalf("object %s missing after user A import; objects=%#v", objectKeyA, store.objects)
	}

	if _, err := repoB.GetBatchStatus(ctxB, responseA.BatchID); !errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("GetBatchStatus(user B on user A batch) error = %v, want ErrBatchNotFound", err)
	}
	if err := repoB.Rollback(ctxB, responseA.BatchID); !errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("Rollback(user B on user A batch) error = %v, want ErrBatchNotFound", err)
	}
	statusA, err := repoA.GetBatchStatus(ctxA, responseA.BatchID)
	if err != nil {
		t.Fatalf("GetBatchStatus(user A after cross-user rollback) error = %v", err)
	}
	if statusA.Status != "completed" {
		t.Fatalf("status A after cross-user rollback = %#v, want completed", statusA)
	}
	if _, ok := store.objects[objectKeyA]; !ok {
		t.Fatalf("user A object %s was deleted by user B rollback", objectKeyA)
	}
	assertImportedRows(t, ctxA, db, batchAID, firstMessageAID)
	assertImportedFileRows(t, ctxA, db, batchAID, fileAID, firstMessageAID, attachmentAID)

	responseB, err := repoB.Commit(ctxB, pkg)
	if err != nil {
		t.Fatalf("Commit(user B same idempotency key) error = %v", err)
	}
	if responseB.BatchID != batchBID || responseB.BatchID == responseA.BatchID {
		t.Fatalf("response B = %#v, want distinct user B batch", responseB)
	}
	objectKeyB := importedObjectKey(userBID, fileBID)
	if _, ok := store.objects[objectKeyB]; !ok {
		t.Fatalf("object %s missing after user B import; objects=%#v", objectKeyB, store.objects)
	}

	if err := repoA.Rollback(ctxA, responseA.BatchID); err != nil {
		t.Fatalf("Rollback(user A) error = %v", err)
	}
	if _, ok := store.objects[objectKeyA]; ok {
		t.Fatalf("user A object %s still exists after rollback", objectKeyA)
	}
	if _, ok := store.objects[objectKeyB]; !ok {
		t.Fatalf("user B object %s was deleted by user A rollback", objectKeyB)
	}
	statusB, err := repoB.GetBatchStatus(ctxB, responseB.BatchID)
	if err != nil {
		t.Fatalf("GetBatchStatus(user B after user A rollback) error = %v", err)
	}
	if statusB.Status != "completed" {
		t.Fatalf("status B = %#v, want completed", statusB)
	}
	assertImportedRows(t, ctxB, db, batchBID, firstMessageBID)
	assertImportedFileRows(t, ctxB, db, batchBID, fileBID, firstMessageBID, attachmentBID)
}

func TestPostgresRepositoryRequiresStorageForFileImport(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	batchID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID)
	manifest := validFileManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := repo.Commit(ctx, pkg); !errors.Is(err, ErrStorageRequired) {
		t.Fatalf("Commit() error = %v, want ErrStorageRequired", err)
	}
}

func TestPostgresRepositoryObjectPutFailureLeavesNoDatabaseRows(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	store := newImportFakeObjectStore()
	store.putErr = errors.New("object store down")
	repo := NewPostgresRepository(db, WithObjectStore(store), WithStorageBackend("local"))
	batchID := mustIntegrationUUID(t)
	fileID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, fileID)
	manifest := validFileManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := repo.Commit(ctx, pkg); err == nil {
		t.Fatal("Commit() error = nil, want object store failure")
	}
	var batchRows int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_batches WHERE id = $1`, batchID).Scan(&batchRows); err != nil {
		t.Fatalf("query import batch rows: %v", err)
	}
	if batchRows != 0 {
		t.Fatalf("batch rows = %d, want 0 after object put failure", batchRows)
	}
	if len(store.objects) != 0 {
		t.Fatalf("objects after put failure = %#v, want none", store.objects)
	}
}

func TestPostgresRepositoryCleansObjectsWhenFileMetadataInsertFails(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	store := newImportFakeObjectStore()
	repo := NewPostgresRepository(db, WithObjectStore(store), WithStorageBackend("bad-backend"))
	batchID := mustIntegrationUUID(t)
	fileID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, fileID)
	manifest := validFileManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := repo.Commit(ctx, pkg); err == nil {
		t.Fatal("Commit() error = nil, want metadata insert failure")
	}
	if len(store.objects) != 0 {
		t.Fatalf("objects after failed commit = %#v, want none", store.objects)
	}
}

func TestPostgresRepositoryRollbackRejectsModifiedImportedFiles(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	store := newImportFakeObjectStore()
	repo := NewPostgresRepository(db, WithObjectStore(store), WithStorageBackend("local"))
	batchID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t))
	manifest := validFileManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := repo.Commit(ctx, pkg)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE files
SET original_filename = original_filename || '.changed', updated_at = now() + interval '1 second'
WHERE id = $1
`, response.Mappings.Files["file-client-1"]); err != nil {
		t.Fatalf("mark imported file modified: %v", err)
	}
	if err := repo.Rollback(ctx, response.BatchID); !errors.Is(err, ErrBatchModified) {
		t.Fatalf("Rollback() error = %v, want ErrBatchModified", err)
	}
}

func TestPostgresRepositoryRollbackRejectsExternalAttachmentReferences(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	store := newImportFakeObjectStore()
	repo := NewPostgresRepository(db, WithObjectStore(store), WithStorageBackend("local"))
	batchID := mustIntegrationUUID(t)
	repo.newID = deterministicIDs(t, batchID, mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t), mustIntegrationUUID(t))
	manifest := validFileManifest()
	manifest.IdempotencyKey = "import-" + batchID
	pkg := readPackageFromManifest(t, manifest, zipEntry{name: manifest.Files[0].BlobPath, body: []byte("hello")})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := repo.Commit(ctx, pkg)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	externalConversationID := mustIntegrationUUID(t)
	externalMessageID := mustIntegrationUUID(t)
	externalAttachmentID := mustIntegrationUUID(t)
	if _, err := db.ExecContext(ctx, `
INSERT INTO conversations (id, user_id, title)
VALUES ($1, $2, 'external')
`, externalConversationID, DevUserID); err != nil {
		t.Fatalf("insert external conversation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO messages (id, conversation_id, user_id, sequence_no, role, status, content)
VALUES ($1, $2, $3, 0, 'user', 'completed', 'external')
`, externalMessageID, externalConversationID, DevUserID); err != nil {
		t.Fatalf("insert external message: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO message_attachments (id, message_id, file_id, user_id, display_order, purpose, updated_at)
VALUES ($1, $2, $3, $4, 0, 'input', now() + interval '1 second')
`, externalAttachmentID, externalMessageID, response.Mappings.Files["file-client-1"], DevUserID); err != nil {
		t.Fatalf("insert external attachment: %v", err)
	}

	if err := repo.Rollback(ctx, response.BatchID); !errors.Is(err, ErrBatchModified) {
		t.Fatalf("Rollback() error = %v, want ErrBatchModified", err)
	}
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

func readPackageFromManifest(t *testing.T, manifest Manifest, extra ...zipEntry) Package {
	t.Helper()
	pkg, issues, err := PackageReader{Now: fixedNow}.Read(bytes.NewReader(testImportZip(t, manifest, extra...)))
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

func assertImportedFileRows(t *testing.T, ctx context.Context, db *sql.DB, batchID string, fileID string, messageID string, attachmentID string) {
	t.Helper()
	var fileCount int
	var purpose string
	var metadataBatchID string
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*), max(metadata ->> 'purpose'), max(metadata #>> '{import,batchId}')
FROM files
WHERE id = $1
  AND metadata #>> '{import,batchId}' = $2
  AND upload_status = 'available'
`, fileID, batchID).Scan(&fileCount, &purpose, &metadataBatchID); err != nil {
		t.Fatalf("query imported file: %v", err)
	}
	if fileCount != 1 || purpose != "chat" || metadataBatchID != batchID {
		t.Fatalf("file count/purpose/batch = %d/%s/%s", fileCount, purpose, metadataBatchID)
	}

	var attachmentCount int
	var attachmentPurpose string
	var displayOrder int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*), max(purpose), max(display_order)
FROM message_attachments
WHERE id = $1
  AND message_id = $2
  AND file_id = $3
`, attachmentID, messageID, fileID).Scan(&attachmentCount, &attachmentPurpose, &displayOrder); err != nil {
		t.Fatalf("query imported attachment: %v", err)
	}
	if attachmentCount != 1 || attachmentPurpose != "input" || displayOrder != 0 {
		t.Fatalf("attachment count/purpose/displayOrder = %d/%s/%d", attachmentCount, attachmentPurpose, displayOrder)
	}
}

func assertImportedFilesRolledBack(t *testing.T, ctx context.Context, db *sql.DB, batchID string) {
	t.Helper()
	var activeFiles int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM files
WHERE metadata #>> '{import,batchId}' = $1
  AND deleted_at IS NULL
  AND upload_status = 'available'
`, batchID).Scan(&activeFiles); err != nil {
		t.Fatalf("query active imported files: %v", err)
	}
	if activeFiles != 0 {
		t.Fatalf("active imported files = %d, want 0", activeFiles)
	}
	var attachmentRows int
	if err := db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM message_attachments ma
JOIN files f ON f.id = ma.file_id
WHERE f.metadata #>> '{import,batchId}' = $1
`, batchID).Scan(&attachmentRows); err != nil {
		t.Fatalf("query imported attachment rows after rollback: %v", err)
	}
	if attachmentRows != 0 {
		t.Fatalf("imported attachment rows = %d, want 0", attachmentRows)
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

type importFakeObjectStore struct {
	objects map[string]importFakeObject
	putErr  error
}

type importFakeObject struct {
	payload     []byte
	contentType string
}

func newImportFakeObjectStore() *importFakeObjectStore {
	return &importFakeObjectStore{objects: map[string]importFakeObject{}}
}

func (s *importFakeObjectStore) Put(_ context.Context, key string, body io.Reader, size int64, contentType string) error {
	if s.putErr != nil {
		return s.putErr
	}
	payload, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(payload)) != size {
		return errors.New("size mismatch")
	}
	s.objects[key] = importFakeObject{payload: payload, contentType: contentType}
	return nil
}

func (s *importFakeObjectStore) Get(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	object, ok := s.objects[key]
	if !ok {
		return nil, storage.ObjectInfo{}, storage.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(object.payload)), storage.ObjectInfo{
		Key:         key,
		Size:        int64(len(object.payload)),
		ContentType: object.contentType,
		UpdatedAt:   time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (s *importFakeObjectStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
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
