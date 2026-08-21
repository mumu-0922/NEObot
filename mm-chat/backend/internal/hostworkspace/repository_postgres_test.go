package hostworkspace

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresRepositoryPreservesWorkspaceAndLocksExecutionBinding(t *testing.T) {
	adminDB := openHostWorkspacePostgres(t)
	runtimeDB := openHostWorkspaceRuntimePostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	userID := uuid.NewString()
	if _, err := adminDB.ExecContext(ctx, `
INSERT INTO users (id, display_name) VALUES ($1, 'Host Workspace test')
`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminDB.Exec(`DELETE FROM users WHERE id = $1`, userID) })
	userCtx := auth.WithUser(ctx, auth.User{ID: userID, DisplayName: "Host Workspace test"})
	repo := NewPostgresRepository(runtimeDB)
	workspaceID := uuid.NewString()
	settings := validSettings()
	created, err := repo.ImportLegacy(userCtx, workspaceID, settings, time.Now().UTC())
	if err != nil || created.ID != workspaceID || created.Revision != 1 || created.Bound() {
		t.Fatalf("ImportLegacy() = %+v, %v", created, err)
	}
	replayed, err := repo.ImportLegacy(userCtx, workspaceID, settings, time.Now().UTC())
	if err != nil || replayed.Revision != 1 {
		t.Fatalf("idempotent ImportLegacy() = %+v, %v", replayed, err)
	}

	descriptor := agenthost.WorkspaceDescriptor{
		CanonicalPath:        "/home/user/project",
		DisplayPath:          `D:\project`,
		PathKind:             "windows-mounted",
		DirectoryFingerprint: "sha256:" + strings.Repeat("a", 64),
	}
	bound, err := repo.Bind(
		userCtx, workspaceID, replayed.Revision, descriptor,
		"wsl-test-runner", time.Now().UTC(),
	)
	if err != nil || !bound.Bound() || bound.Revision != 2 {
		t.Fatalf("Bind() = %+v, %v", bound, err)
	}

	secondID := uuid.NewString()
	second, err := repo.ImportLegacy(
		userCtx, secondID,
		Settings{Name: "Alias", Files: []WorkspaceFile{}}, time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Bind(
		userCtx, secondID, second.Revision, descriptor,
		"wsl-test-runner", time.Now().UTC(),
	); !errors.Is(err, ErrDirectoryAlreadyRegistered) {
		t.Fatalf("alias Bind() error = %v, want %v", err, ErrDirectoryAlreadyRegistered)
	}

	conversationID := uuid.NewString()
	if _, err := adminDB.ExecContext(ctx, `
INSERT INTO conversations (id, user_id, title) VALUES ($1, $2, 'Host-bound conversation')
`, conversationID, userID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetConversationWorkspace(userCtx, conversationID, workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ClearConversationWorkspace(userCtx, conversationID, workspaceID); err != nil {
		t.Fatalf("ClearConversationWorkspace() error = %v", err)
	}
	if err := repo.SetConversationWorkspace(userCtx, conversationID, workspaceID); err != nil {
		t.Fatal(err)
	}
	binding, err := repo.LockConversationExecutionWorkspace(
		userCtx, conversationID, workspaceID, time.Now().UTC(),
	)
	if err != nil || binding.RunnerID != "wsl-test-runner" ||
		binding.CanonicalPath != descriptor.CanonicalPath ||
		binding.PermissionMode != agenthost.PermissionWorkspaceWrite {
		t.Fatalf("LockConversationExecutionWorkspace() = %+v, %v", binding, err)
	}
	if err := repo.SetConversationPermission(
		userCtx, conversationID, agenthost.PermissionReadOnly,
	); err != nil {
		t.Fatalf("SetConversationPermission(read-only) error = %v", err)
	}
	binding, err = repo.LockConversationExecutionWorkspace(
		userCtx, conversationID, workspaceID, time.Now().UTC(),
	)
	if err != nil || binding.PermissionMode != agenthost.PermissionReadOnly {
		t.Fatalf("durable permission binding = %+v, %v", binding, err)
	}
	messageID := uuid.NewString()
	if _, err := adminDB.ExecContext(ctx, `
INSERT INTO messages (id, conversation_id, user_id, sequence_no, role, status, content)
VALUES ($1, $2, $3, 1, 'assistant', 'streaming', '')
`, messageID, conversationID, userID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetConversationPermission(
		userCtx, conversationID, agenthost.PermissionFullAccess,
	); !errors.Is(err, ErrPermissionLocked) {
		t.Fatalf("active Turn permission change error = %v", err)
	}
	if _, err := adminDB.ExecContext(ctx, `
UPDATE messages SET status = 'completed', completed_at = now() WHERE id = $1
`, messageID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetConversationPermission(
		userCtx, conversationID, agenthost.PermissionFullAccess,
	); err != nil {
		t.Fatalf("SetConversationPermission(Full access) error = %v", err)
	}
	if err := repo.SetConversationWorkspace(userCtx, conversationID, secondID); !errors.Is(err, ErrConversationLocked) {
		t.Fatalf("move locked conversation error = %v, want %v", err, ErrConversationLocked)
	}
	if err := repo.ClearConversationWorkspace(userCtx, conversationID, workspaceID); !errors.Is(err, ErrConversationLocked) {
		t.Fatalf("clear locked conversation error = %v, want %v", err, ErrConversationLocked)
	}
	if err := repo.Delete(userCtx, workspaceID, bound.Revision, time.Now().UTC()); !errors.Is(err, ErrWorkspaceInUse) {
		t.Fatalf("delete used Workspace error = %v, want %v", err, ErrWorkspaceInUse)
	}
	if err := repo.SetConversationPermission(
		userCtx, conversationID, agenthost.PermissionWorkspaceWrite,
	); err != nil {
		t.Fatal(err)
	}
	runner := migration.NewRunner(adminDB, migrationfiles.FS)
	if peeled, err := runner.Down(ctx, false); err != nil || len(peeled) != 1 || peeled[0].Version != 103 {
		t.Fatalf("peel permission migration = %+v, %v", peeled, err)
	}
	if _, err := runner.Down(ctx, false); err == nil ||
		!strings.Contains(err.Error(), "HOST_WORKSPACE_ROLLBACK_BLOCKED") {
		t.Fatalf("guarded migration down error = %v", err)
	}
	if reapplied, err := runner.Up(ctx); err != nil || len(reapplied) != 1 || reapplied[0].Version != 103 {
		t.Fatalf("restore permission migration = %+v, %v", reapplied, err)
	}
}

func TestPostgresRepositoryEnforcesUserOwnership(t *testing.T) {
	adminDB := openHostWorkspacePostgres(t)
	runtimeDB := openHostWorkspaceRuntimePostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ownerID, otherID := uuid.NewString(), uuid.NewString()
	if _, err := adminDB.ExecContext(ctx, `
INSERT INTO users (id, display_name) VALUES ($1, 'owner'), ($2, 'other')
`, ownerID, otherID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = adminDB.Exec(`DELETE FROM users WHERE id IN ($1, $2)`, ownerID, otherID) })
	repo := NewPostgresRepository(runtimeDB)
	workspaceID := uuid.NewString()
	ownerCtx := auth.WithUser(ctx, auth.User{ID: ownerID})
	if _, err := repo.ImportLegacy(ownerCtx, workspaceID, validSettings(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	otherCtx := auth.WithUser(ctx, auth.User{ID: otherID})
	if _, err := repo.Get(otherCtx, workspaceID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user Get() error = %v, want %v", err, ErrNotFound)
	}
	_, err := repo.ImportLegacy(
		otherCtx, workspaceID, validSettings(), time.Now().UTC(),
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user import error = %v, want %v", err, ErrNotFound)
	}
}

func openHostWorkspacePostgres(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runner := migration.NewRunner(db, migrationfiles.FS)
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	rolledBack, err := runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 103 {
		t.Fatalf("rollback Agent permission migration: %+v, %v", rolledBack, err)
	}
	rolledBack, err = runner.Down(ctx, false)
	if err != nil || len(rolledBack) != 1 || rolledBack[0].Version != 102 {
		t.Fatalf("rollback empty Host Workspace migration: %+v, %v", rolledBack, err)
	}
	reapplied, err := runner.Up(ctx)
	if err != nil || len(reapplied) != 2 || reapplied[0].Version != 102 || reapplied[1].Version != 103 {
		t.Fatalf("reapply Host Workspace migration: %+v, %v", reapplied, err)
	}
	return db
}

func openHostWorkspaceRuntimePostgres(t *testing.T) *sql.DB {
	t.Helper()
	config, err := pgx.ParseConfig(os.Getenv("MM_CHAT_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["role"] = "go_api_runtime"
	db := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect as go_api_runtime: %v", err)
	}
	return db
}
