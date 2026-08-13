package agents

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestAssistantPostgresRepositoryAuthorityAndCAS(t *testing.T) {
	db := openAssistantPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	userOne, userTwo := uuid.NewString(), uuid.NewString()
	for _, userID := range []string{userOne, userTwo} {
		mustExecAssistant(t, ctx, db, `
INSERT INTO users (id, email, display_name) VALUES ($1, $2, 'Assistant integration')
`, userID, "assistant-"+userID+"@example.test")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id IN ($1, $2)`, userOne, userTwo)
	})

	repo := NewPostgresRepository(db)
	customOne, err := normalizeCustomSnapshot(CreateCustomInput{Title: "Writer", SystemPrompt: "Write."})
	if err != nil {
		t.Fatal(err)
	}
	customTwo, _ := normalizeCustomSnapshot(CreateCustomInput{Title: "Reviewer", SystemPrompt: "Review."})
	first, err := repo.CreateCustom(ctx, userOne, customOne)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateCustom(ctx, userOne, customTwo); err != nil {
		t.Fatalf("second custom: %v", err)
	}
	listed, err := repo.ListLibrary(ctx, userOne)
	if err != nil || len(listed) != 2 {
		t.Fatalf("custom list=%#v error=%v", listed, err)
	}
	if _, err := repo.GetLibrary(ctx, userTwo, first.ID); err != ErrLibraryNotFound {
		t.Fatalf("cross-user get error=%v", err)
	}
	updatedInput, _ := normalizeCustomSnapshot(CreateCustomInput{Title: "Writer 2", SystemPrompt: "Write carefully."})
	updated, err := repo.UpdateCustom(ctx, userOne, first.ID, first.Revision, updatedInput)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("update=%#v error=%v", updated, err)
	}
	if _, err := repo.UpdateCustom(ctx, userOne, first.ID, first.Revision, customOne); err != ErrRevisionConflict {
		t.Fatalf("stale update error=%v", err)
	}

	store := customOne
	store.SourceIdentifier = "writer-store"
	store.RequiredTools = []string{"search", "files"}
	store.ContentFingerprint = fingerprintSnapshot(store)
	installed, err := repo.Install(ctx, userOne, store)
	if err != nil || len(installed.RequiredTools) != 2 {
		t.Fatalf("install=%#v error=%v", installed, err)
	}
	if _, err := repo.Install(ctx, userOne, store); err != ErrLibraryConflict {
		t.Fatalf("duplicate install error=%v", err)
	}
	if _, err := repo.Install(ctx, userTwo, store); err != nil {
		t.Fatalf("same Store item for another user: %v", err)
	}
	if err := repo.Uninstall(ctx, userOne, installed.ID, installed.Revision+1); err != ErrRevisionConflict {
		t.Fatalf("stale uninstall error=%v", err)
	}
}

func openAssistantPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Assistant Postgres integration tests")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping Assistant integration database: %v", err)
	}
	return db
}

func mustExecAssistant(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("execute Assistant fixture: %v", err)
	}
}
