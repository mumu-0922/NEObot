package files

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

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
