package files

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresRepositoryCreatesGetsAndDeletesFileMetadata(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fileID := mustUUID(t)
	record, err := repo.CreateFile(ctx, CreateFileInput{
		ID:               fileID,
		OriginalFilename: "hello.txt",
		MimeType:         "text/plain",
		ByteSize:         11,
		SHA256:           "b94d27b9934d3e08a52e52d7da7dabfadebca7838dfb27f4f9174e65a2f27f21",
		StorageBackend:   "local",
		ObjectKey:        objectKeyFor(fileID),
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile() error = %v", err)
	}
	if record.ID != fileID || record.UserID != DevUserID || record.UploadStatus != "available" {
		t.Fatalf("created record = %#v", record)
	}

	got, err := repo.GetFile(ctx, fileID)
	if err != nil {
		t.Fatalf("GetFile() error = %v", err)
	}
	if got.ObjectKey != objectKeyFor(fileID) || got.Metadata["purpose"] != "chat" {
		t.Fatalf("got record = %#v", got)
	}

	deleted, err := repo.MarkFileDeleted(ctx, fileID)
	if err != nil {
		t.Fatalf("MarkFileDeleted() error = %v", err)
	}
	if deleted.UploadStatus != "deleted" || deleted.DeletedAt == nil {
		t.Fatalf("deleted record = %#v", deleted)
	}

	if _, err := repo.GetFile(ctx, fileID); err != ErrFileNotFound {
		t.Fatalf("GetFile() after delete error = %v, want ErrFileNotFound", err)
	}
}

func TestPostgresRepositoryEnforcesTwoUserFileIsolation(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)

	userAID := mustUUID(t)
	userBID := mustUUID(t)
	baseA := auth.WithUser(context.Background(), auth.User{ID: userAID, DisplayName: "User A"})
	ctxA, cancelA := context.WithTimeout(baseA, 5*time.Second)
	defer cancelA()
	baseB := auth.WithUser(context.Background(), auth.User{ID: userBID, DisplayName: "User B"})
	ctxB, cancelB := context.WithTimeout(baseB, 5*time.Second)
	defer cancelB()

	fileID := mustUUID(t)
	record, err := repo.CreateFile(ctxA, CreateFileInput{
		ID:               fileID,
		OriginalFilename: "a-only.txt",
		MimeType:         "text/plain",
		ByteSize:         11,
		SHA256:           "b94d27b9934d3e08a52e52d7da7dabfadebca7838dfb27f4f9174e65a2f27f21",
		StorageBackend:   "local",
		ObjectKey:        objectKeyForUser(userAID, fileID),
		Metadata:         map[string]any{"purpose": "chat"},
	})
	if err != nil {
		t.Fatalf("CreateFile(user A) error = %v", err)
	}
	if record.UserID != userAID || record.ObjectKey != objectKeyForUser(userAID, fileID) {
		t.Fatalf("created record = %#v, want user A scoped object key", record)
	}

	if _, err := repo.GetFile(ctxB, fileID); err != ErrFileNotFound {
		t.Fatalf("GetFile(user B on user A file) error = %v, want ErrFileNotFound", err)
	}
	if _, err := repo.MarkFileDeleted(ctxB, fileID); err != ErrFileNotFound {
		t.Fatalf("MarkFileDeleted(user B on user A file) error = %v, want ErrFileNotFound", err)
	}
	gotA, err := repo.GetFile(ctxA, fileID)
	if err != nil {
		t.Fatalf("GetFile(user A after user B attempts) error = %v", err)
	}
	if gotA.UploadStatus != "available" || gotA.DeletedAt != nil {
		t.Fatalf("user A file after user B attempts = %#v", gotA)
	}

	if _, err := repo.MarkFileDeleted(ctxA, fileID); err != nil {
		t.Fatalf("MarkFileDeleted(user A) error = %v", err)
	}
	if _, err := repo.GetFile(ctxA, fileID); err != ErrFileNotFound {
		t.Fatalf("GetFile(user A after delete) error = %v, want ErrFileNotFound", err)
	}
}

func TestPostgresRepositorySerializesKnowledgeBindAndDelete(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	userID := mustUUID(t)
	ctx := auth.WithUser(context.Background(), auth.User{ID: userID, DisplayName: "Knowledge Owner"})
	fileID := mustUUID(t)
	file, err := repo.CreateFile(ctx, CreateFileInput{
		ID: fileID, OriginalFilename: "source.txt", MimeType: "text/plain", ByteSize: 6,
		SHA256:         "b94d27b9934d3e08a52e52d7da7dabfadebca7838dfb27f4f9174e65a2f27f21",
		StorageBackend: "local", ObjectKey: objectKeyForUser(userID, fileID),
		Metadata: map[string]any{"purpose": "knowledge"},
	})
	if err != nil {
		t.Fatal(err)
	}
	collectionID, documentID, versionID := mustUUID(t), mustUUID(t), mustUUID(t)
	if _, err := db.ExecContext(ctx, `
INSERT INTO knowledge_collections (id, name, scope, owner_user_id)
VALUES ($1, 'Binding Test', 'personal', $2);
INSERT INTO knowledge_documents (id, collection_id) VALUES ($3, $1)
`, collectionID, userID, documentID); err != nil {
		t.Fatal(err)
	}
	binder, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = binder.Rollback() }()
	var lockedID string
	if err := binder.QueryRowContext(ctx, `SELECT id FROM files WHERE id = $1 FOR UPDATE`, fileID).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}
	deleteResult := make(chan error, 1)
	go func() {
		_, deleteErr := repo.MarkFileDeleted(ctx, fileID)
		deleteResult <- deleteErr
	}()
	if _, err := binder.ExecContext(ctx, `
INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, status, content_hash
) VALUES ($1, $2, $3, 1, 'uploaded', $4)
`, versionID, documentID, fileID, file.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := binder.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-deleteResult; !errors.Is(err, ErrFileInUse) {
		t.Fatalf("concurrent delete error = %v, want ErrFileInUse", err)
	}
	if got, err := repo.GetFile(ctx, fileID); err != nil || got.UploadStatus != "available" {
		t.Fatalf("bound file after rejected delete = %#v, err=%v", got, err)
	}
	if _, err := db.ExecContext(ctx, `
UPDATE knowledge_document_versions SET status = 'tombstoned' WHERE id = $1;
UPDATE knowledge_documents
SET status = 'tombstoned', deleted_at = now(), updated_at = now()
WHERE id = $2
`, versionID, documentID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkFileDeleted(ctx, fileID); err != nil {
		t.Fatalf("delete after tombstone: %v", err)
	}
	var events int
	if err := db.QueryRowContext(ctx, `
SELECT count(*) FROM knowledge_outbox
WHERE aggregate_type = 'file' AND aggregate_key = $1
  AND event_type = 'file.object.delete.requested'
`, fileID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("file cleanup events = %d, want 1", events)
	}
}

func TestPostgresRepositoryRollsBackFileDeleteWhenOutboxFails(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	userID, fileID := mustUUID(t), mustUUID(t)
	ctx := auth.WithUser(context.Background(), auth.User{ID: userID, DisplayName: "Rollback Owner"})
	if _, err := repo.CreateFile(ctx, CreateFileInput{
		ID: fileID, OriginalFilename: "rollback.txt", MimeType: "text/plain", ByteSize: 1,
		SHA256:         "b94d27b9934d3e08a52e52d7da7dabfadebca7838dfb27f4f9174e65a2f27f21",
		StorageBackend: "local", ObjectKey: objectKeyForUser(userID, fileID),
		Metadata: map[string]any{"purpose": "knowledge"},
	}); err != nil {
		t.Fatal(err)
	}
	repo.newEventID = func() (string, error) { return "", errors.New("synthetic outbox failure") }
	if _, err := repo.MarkFileDeleted(ctx, fileID); err == nil {
		t.Fatal("MarkFileDeleted() error = nil when outbox generation fails")
	}
	got, err := repo.GetFile(ctx, fileID)
	if err != nil || got.UploadStatus != "available" || got.DeletedAt != nil {
		t.Fatalf("file after rolled-back delete = %#v, err=%v", got, err)
	}
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
	t.Cleanup(func() {
		_ = db.Close()
	})

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

func mustUUID(t *testing.T) string {
	t.Helper()
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID() error = %v", err)
	}
	return id
}
