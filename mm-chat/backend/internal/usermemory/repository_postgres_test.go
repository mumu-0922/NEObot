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
		_, _ = db.ExecContext(cleanupCtx, `
DELETE FROM user_memory_deletion_manifests WHERE user_id = ANY($1::uuid[])
`, []string{userAID, userBID})
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
	var sensitiveEnabled bool
	var l2Mode, l3Mode string
	if err := db.QueryRowContext(ctx, `
SELECT sensitive_memory_enabled, l2_mode, l3_mode
FROM user_memory_settings
WHERE user_id = $1
`, userAID).Scan(&sensitiveEnabled, &l2Mode, &l3Mode); err != nil {
		t.Fatalf("read additive settings defaults: %v", err)
	}
	if sensitiveEnabled || l2Mode != "inherit" || l3Mode != "inherit" {
		t.Fatalf("additive settings = sensitive %t, l2 %q, l3 %q", sensitiveEnabled, l2Mode, l3Mode)
	}
	created, err := service.CreateManual(ctxA, Candidate{
		Type: "preference", Content: "Keep answers concise",
		Importance: 5, Tags: []string{"style"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var createdScope string
	if err := db.QueryRowContext(ctx, `
SELECT scope_type
FROM user_memories
WHERE id = $1
`, created.ID).Scan(&createdScope); err != nil {
		t.Fatalf("read v1 repository scope: %v", err)
	}
	if createdScope != "global" {
		t.Fatalf("v1 repository created scope %q, want global", createdScope)
	}
	duplicate, err := service.CreateManual(ctxA, Candidate{
		Type: "preference", Content: "Keep answers concise",
		Importance: 4, Tags: []string{"style"},
	})
	if err != nil || duplicate.ID != created.ID {
		t.Fatalf("same-Global duplicate = %#v/%v, want upsert of %s", duplicate, err, created.ID)
	}

	projectID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	projectMemoryID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO projects (id, user_id, name)
VALUES ($1, $2, 'repository isolation fixture');
INSERT INTO user_memories (
  id, user_id, memory_type, content, normalized_content, source,
  scope_type, project_id, content_hash, authority_kind
) VALUES (
  $3, $2, 'preference', $4, $5, 'manual', 'project', $1,
  encode(sha256(convert_to($4, 'UTF8')), 'hex'), 'manual'
)
`, projectID, userAID, projectMemoryID, created.Content, created.NormalizedContent); err != nil {
		t.Fatalf("insert Project-scoped fixture: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM user_memories WHERE id = $1`, projectMemoryID)
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM projects WHERE id = $1`, projectID)
	})

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
	if _, err := service.Update(ctxA, projectMemoryID, Candidate{
		Type: "preference", Content: "v1 must not mutate Project scope", Importance: 3,
	}); err != ErrMemoryNotFound {
		t.Fatalf("v1 Project-scope update error = %v, want ErrMemoryNotFound", err)
	}
	if err := service.Delete(ctxA, projectMemoryID); err != ErrMemoryNotFound {
		t.Fatalf("v1 Project-scope delete error = %v, want ErrMemoryNotFound", err)
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
	var tombstones, manifests, purgeJobs, deleteRevisions int
	if err := db.QueryRowContext(ctx, `

SELECT
  (SELECT deleted_at IS NOT NULL AND NOT enabled
    FROM user_memories WHERE id = $1),
  (SELECT count(*) FROM user_memory_tombstones WHERE memory_id = $1),
  (SELECT count(*) FROM user_memory_deletion_manifests WHERE memory_id = $1),
  (SELECT count(*) FROM memory_jobs WHERE target_memory_id = $1 AND stage = 'purge'),
  (SELECT count(*) FROM user_memory_revisions
    WHERE memory_id = $1 AND operation = 'delete')
`, created.ID).Scan(
		&deleted, &tombstones, &manifests, &purgeJobs, &deleteRevisions,
	); err != nil || !deleted || tombstones != 1 || manifests != 1 ||
		purgeJobs != 1 || deleteRevisions != 1 {
		t.Fatalf("delete authority = %v/%d/%d/%d/%d/%v", deleted,
			tombstones, manifests, purgeJobs, deleteRevisions, err)
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
