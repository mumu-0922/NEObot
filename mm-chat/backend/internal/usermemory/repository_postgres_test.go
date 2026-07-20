package usermemory

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresMemoryPersistsAndIsolatesUsers(t *testing.T) {
	db := openMemoryPostgresIntegrationDB(t)
	repo := NewPostgresRepository(db)
	service := NewService(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userAID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	userBID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{userAID, userBID} {
		if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, display_name) VALUES ($1, 'memory fixture')
`, userID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{userAID, userBID})
	})

	ctxA := auth.WithUser(ctx, auth.User{ID: userAID, DisplayName: "A"})
	ctxB := auth.WithUser(ctx, auth.User{ID: userBID, DisplayName: "B"})
	enabled := true
	settings, err := service.UpdateSettings(ctxA, SettingsPatch{
		Enabled: &enabled, AutoRecordEnabled: &enabled,
	})
	if err != nil || !settings.Enabled || !settings.AutoRecordEnabled {
		t.Fatalf("UpdateSettings() = %#v/%v", settings, err)
	}
	created, err := service.CreateManual(ctxA, Candidate{
		Type: "preference", Content: "Keep answers concise",
		Importance: 5, Tags: []string{"style"},
	})
	if err != nil {
		t.Fatal(err)
	}

	listedA, err := service.List(ctxA)
	if err != nil || len(listedA) != 1 || listedA[0].ID != created.ID {
		t.Fatalf("user A list = %#v/%v", listedA, err)
	}
	listedB, err := service.List(ctxB)
	if err != nil || len(listedB) != 0 {
		t.Fatalf("user B list = %#v/%v, want empty", listedB, err)
	}
	if _, err := service.Update(ctxB, created.ID, Candidate{
		Type: "preference", Content: "stolen", Importance: 3,
	}); err != ErrMemoryNotFound {
		t.Fatalf("cross-user update error = %v, want ErrMemoryNotFound", err)
	}

	matches, err := service.SearchRelevant(ctxA, "Please keep the answer concise", 5)
	if err != nil || len(matches) != 1 || matches[0].ID != created.ID {
		t.Fatalf("SearchRelevant() = %#v/%v", matches, err)
	}
	other, err := service.CreateManual(ctxA, Candidate{
		Type: "fact", Content: "The preferred editor is Vim", Importance: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(ctxA, other.ID, Candidate{
		Type: "preference", Content: created.Content, Importance: 3,
	}); err != ErrMemoryConflict {
		t.Fatalf("duplicate-content update error = %v, want ErrMemoryConflict", err)
	}
	if err := service.Delete(ctxA, other.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(ctxA, created.ID); err != nil {
		t.Fatal(err)
	}
	listedA, err = service.List(ctxA)
	if err != nil || len(listedA) != 0 {
		t.Fatalf("list after delete = %#v/%v", listedA, err)
	}
	var deleted bool
	if err := db.QueryRowContext(ctx, `
SELECT deleted_at IS NOT NULL AND NOT enabled
FROM user_memories WHERE id = $1
`, created.ID).Scan(&deleted); err != nil || !deleted {
		t.Fatalf("soft-delete projection = %v/%v", deleted, err)
	}
}

func openMemoryPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
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
		t.Fatalf("ping integration database: %v", err)
	}
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}
