package migration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestMemoryProjectScopeSettingsMigrationContract(t *testing.T) {
	up := readPhase15SQL(t, "053_memory_project_scope_settings.up.sql")
	down := readPhase15SQL(t, "053_memory_project_scope_settings.down.sql")

	projectDDL := phase15TableDDL(t, up, "projects")
	assertPhase15Fragments(t, projectDDL,
		"053 projects must be first-class user-owned scope authority",
		"user_id uuid not null references users ( id ) on delete cascade",
		"lifecycle_status text not null default 'active'",
		"revision bigint not null default 1",
		"scope_generation bigint not null default 1",
		"unique ( id , user_id )")

	conversationDDL := phase15AlterTableDDL(up, "conversations")
	assertPhase15Fragments(t, conversationDDL,
		"053 conversations must retain explicit Project ownership and independent Use/Learn modes",
		"add column project_id uuid",
		"memory_scope_generation bigint not null default 1",
		"memory_use_mode text not null default 'inherit'",
		"memory_learn_mode text not null default 'inherit'",
		"unique ( id , user_id )")
	assertPhase15CompositeForeignKey(t, conversationDDL, "projects", map[string]string{
		"project_id": "id",
		"user_id":    "user_id",
	}, "conversation Project membership must be constrained to the authenticated owner")

	settingsDDL := phase15AlterTableDDL(up, "user_memory_settings")
	assertPhase15Fragments(t, settingsDDL,
		"053 settings must add privacy-safe defaults without changing existing settings",
		"sensitive_memory_enabled boolean not null default false",
		"l2_mode text not null default 'inherit'",
		"l3_mode text not null default 'inherit'",
		"l2_mode in ( 'inherit' , 'on' , 'off' )",
		"l3_mode in ( 'inherit' , 'on' , 'off' )")

	memoryDDL := phase15AlterTableDDL(up, "user_memories")
	assertPhase15Fragments(t, up,
		"053 must explicitly backfill all existing memories as generation-one Global authority",
		"update user_memories set scope_type = 'global'",
		"project_id = null",
		"scope_conversation_id = null",
		"scope_generation = 1")
	assertPhase15Fragments(t, memoryDDL,
		"053 memory scopes must be database-enforced",
		"scope_type in ( 'global' , 'project' , 'conversation' )",
		"scope_generation >= 1",
		"scope_type = 'global' and project_id is null and scope_conversation_id is null",
		"scope_type = 'project' and project_id is not null and scope_conversation_id is null",
		"scope_type = 'conversation' and project_id is null and scope_conversation_id is not null")
	assertPhase15CompositeForeignKey(t, memoryDDL, "projects", map[string]string{
		"project_id": "id",
		"user_id":    "user_id",
	}, "Project-scoped Memory must not reference another user's Project")
	assertPhase15CompositeForeignKey(t, memoryDDL, "conversations", map[string]string{
		"scope_conversation_id": "id",
		"user_id":               "user_id",
	}, "Conversation-scoped Memory must not reference another user's Conversation")

	assertPhase15Fragments(t, up,
		"053 must replace global-only deduplication with exact same-scope deduplication",
		"drop index idx_user_memories_active_content",
		"create unique index idx_user_memories_active_global_content",
		"where deleted_at is null and scope_type = 'global'",
		"create unique index idx_user_memories_active_project_content",
		"on user_memories ( user_id , project_id , normalized_content )",
		"create unique index idx_user_memories_active_conversation_content",
		"on user_memories ( user_id , scope_conversation_id , normalized_content )")

	assertPhase15Fragments(t, down,
		"053 rollback must fail closed once v2 authority has been used",
		"memory_v2_rollback_requires_empty_projects",
		"memory_v2_rollback_requires_global_memory_only",
		"memory_v2_rollback_requires_inherited_conversation_policy",
		"memory_v2_rollback_requires_default_memory_settings",
		"create unique index idx_user_memories_active_content",
		"drop column if exists scope_type",
		"drop column if exists sensitive_memory_enabled",
		"drop column if exists project_id",
		"drop table projects")

	if strings.Contains(up, "to go_api_runtime") {
		t.Fatal("053 must not expose Project API authority to go_api_runtime")
	}
}

func TestMemoryProjectScopeSettingsMigrationLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	schema := fmt.Sprintf("memory_v2_pr2_%d", time.Now().UnixNano())
	if _, err := conn.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = conn.ExecContext(cleanupCtx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	}()
	if _, err := conn.ExecContext(ctx, `SET search_path TO `+schema+`, pg_catalog`); err != nil {
		t.Fatalf("select isolated schema: %v", err)
	}

	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
CREATE TABLE users (id UUID PRIMARY KEY);
CREATE TABLE conversations (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);
CREATE TABLE user_memory_settings (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  enabled BOOLEAN NOT NULL DEFAULT false,
  search_enabled BOOLEAN NOT NULL DEFAULT true,
  auto_record_enabled BOOLEAN NOT NULL DEFAULT false
);
CREATE TABLE user_memories (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  normalized_content TEXT NOT NULL,
  deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_user_memories_active_content
  ON user_memories(user_id, normalized_content)
  WHERE deleted_at IS NULL;
`)

	const (
		userA         = "11111111-1111-4111-8111-111111111111"
		userB         = "22222222-2222-4222-8222-222222222222"
		conversation  = "33333333-3333-4333-8333-333333333333"
		globalMemory  = "44444444-4444-4444-8444-444444444444"
		projectA      = "55555555-5555-4555-8555-555555555555"
		projectB      = "66666666-6666-4666-8666-666666666666"
		projectMemory = "77777777-7777-4777-8777-777777777777"
	)
	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
INSERT INTO users(id) VALUES
  ('`+userA+`'),
  ('`+userB+`');
INSERT INTO conversations(id, user_id) VALUES ('`+conversation+`', '`+userA+`');
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled
) VALUES ('`+userA+`', true, false, false);
INSERT INTO user_memories(id, user_id, normalized_content)
VALUES ('`+globalMemory+`', '`+userA+`', 'same fact');
`)

	upBytes, err := migrationfiles.FS.ReadFile("053_memory_project_scope_settings.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("053_memory_project_scope_settings.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExecMemoryV2MigrationSQL(t, ctx, conn, string(upBytes))

	var enabled, searchEnabled, autoRecordEnabled, sensitiveEnabled bool
	var l2Mode, l3Mode string
	if err := conn.QueryRowContext(ctx, `
SELECT enabled, search_enabled, auto_record_enabled,
       sensitive_memory_enabled, l2_mode, l3_mode
FROM user_memory_settings
WHERE user_id = $1`, userA).Scan(
		&enabled,
		&searchEnabled,
		&autoRecordEnabled,
		&sensitiveEnabled,
		&l2Mode,
		&l3Mode,
	); err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	if !enabled || searchEnabled || autoRecordEnabled || sensitiveEnabled ||
		l2Mode != "inherit" || l3Mode != "inherit" {
		t.Fatalf("migrated settings changed authority: enabled=%t search=%t auto=%t sensitive=%t l2=%q l3=%q",
			enabled, searchEnabled, autoRecordEnabled, sensitiveEnabled, l2Mode, l3Mode)
	}

	assertMemoryV2GlobalBackfill(t, ctx, conn, globalMemory)

	var scopeGeneration int64
	var useMode, learnMode string
	if err := conn.QueryRowContext(ctx, `
SELECT memory_scope_generation, memory_use_mode, memory_learn_mode
FROM conversations
WHERE id = $1`, conversation).Scan(&scopeGeneration, &useMode, &learnMode); err != nil {
		t.Fatalf("read migrated conversation policy: %v", err)
	}
	if scopeGeneration != 1 || useMode != "inherit" || learnMode != "inherit" {
		t.Fatalf("conversation policy = generation %d, use %q, learn %q", scopeGeneration, useMode, learnMode)
	}

	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
INSERT INTO projects(id, user_id, name) VALUES
  ('`+projectA+`', '`+userA+`', 'A'),
  ('`+projectB+`', '`+userB+`', 'B');
`)
	if _, err := conn.ExecContext(ctx, `
UPDATE conversations SET project_id = $1 WHERE id = $2`, projectB, conversation); err == nil {
		t.Fatal("cross-user Conversation Project membership unexpectedly succeeded")
	}
	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
UPDATE conversations
SET project_id = '`+projectA+`'
WHERE id = '`+conversation+`';
INSERT INTO user_memories(
  id, user_id, normalized_content, scope_type, project_id
) VALUES (
  '`+projectMemory+`', '`+userA+`', 'same fact', 'project', '`+projectA+`'
);
`)
	if _, err := conn.ExecContext(ctx, `
INSERT INTO user_memories(id, user_id, normalized_content, scope_type, project_id)
VALUES ('88888888-8888-4888-8888-888888888888', $1, 'same fact', 'project', $2)`, userA, projectA); err == nil {
		t.Fatal("same-Project active duplicate unexpectedly succeeded")
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO user_memories(id, user_id, normalized_content, scope_type, project_id)
VALUES ('99999999-9999-4999-8999-999999999999', $1, 'other fact', 'project', $2)`, userA, projectB); err == nil {
		t.Fatal("cross-user Project-scoped Memory unexpectedly succeeded")
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO user_memories(id, user_id, normalized_content, scope_type, project_id)
VALUES ('aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa', $1, 'invalid shape', 'global', $2)`, userA, projectA); err == nil {
		t.Fatal("invalid Global scope shape unexpectedly succeeded")
	}

	if _, err := conn.ExecContext(ctx, string(downBytes)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_V2_ROLLBACK_REQUIRES_EMPTY_PROJECTS") {
		t.Fatalf("rollback with v2 Project data error = %v", err)
	}

	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
DELETE FROM user_memories WHERE id = '`+projectMemory+`';
UPDATE conversations SET project_id = NULL WHERE id = '`+conversation+`';
DELETE FROM projects;
`)
	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
INSERT INTO user_memories(
  id, user_id, normalized_content, scope_type, scope_conversation_id
) VALUES (
  'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
  '`+userA+`',
  'same fact',
  'conversation',
  '`+conversation+`'
);
`)
	if _, err := conn.ExecContext(ctx, string(downBytes)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_V2_ROLLBACK_REQUIRES_GLOBAL_MEMORY_ONLY") {
		t.Fatalf("rollback with non-Global Memory error = %v", err)
	}
	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
DELETE FROM user_memories WHERE id = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
UPDATE conversations SET memory_use_mode = 'off' WHERE id = '`+conversation+`';
`)
	if _, err := conn.ExecContext(ctx, string(downBytes)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_V2_ROLLBACK_REQUIRES_INHERITED_CONVERSATION_POLICY") {
		t.Fatalf("rollback with changed Conversation policy error = %v", err)
	}
	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
UPDATE conversations SET memory_use_mode = 'inherit' WHERE id = '`+conversation+`';
`)
	mustExecMemoryV2MigrationSQL(t, ctx, conn, string(downBytes))

	var scopeColumnCount int
	if err := conn.QueryRowContext(ctx, `
SELECT count(*)
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = 'user_memories' AND column_name = 'scope_type'
`, schema).Scan(&scopeColumnCount); err != nil {
		t.Fatal(err)
	}
	if scopeColumnCount != 0 {
		t.Fatalf("scope_type column count after rollback = %d, want 0", scopeColumnCount)
	}
	var legacyIndexPresent bool
	if err := conn.QueryRowContext(ctx, `
SELECT to_regclass($1) IS NOT NULL`, schema+`.idx_user_memories_active_content`).Scan(&legacyIndexPresent); err != nil {
		t.Fatal(err)
	}
	if !legacyIndexPresent {
		t.Fatal("legacy active-content index was not restored")
	}

	mustExecMemoryV2MigrationSQL(t, ctx, conn, string(upBytes))
	assertMemoryV2GlobalBackfill(t, ctx, conn, globalMemory)
	mustExecMemoryV2MigrationSQL(t, ctx, conn, `
UPDATE user_memory_settings SET sensitive_memory_enabled = true WHERE user_id = '`+userA+`';
`)
	if _, err := conn.ExecContext(ctx, string(downBytes)); err == nil ||
		!strings.Contains(err.Error(), "MEMORY_V2_ROLLBACK_REQUIRES_DEFAULT_MEMORY_SETTINGS") {
		t.Fatalf("rollback with changed settings error = %v", err)
	}
}

func TestMemoryProjectScopeSettingsRuntimeRoleBoundaryLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	files := phase15MigrationFSThrough(t, 37)
	for _, path := range []string{
		"053_memory_project_scope_settings.up.sql",
		"053_memory_project_scope_settings.down.sql",
	} {
		data, err := migrationfiles.FS.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		files[path] = &fstest.MapFile{Data: data}
	}
	runner := NewRunner(db, files)
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("apply migrations through 053: %v", err)
	}
	if len(applied) == 0 || applied[len(applied)-1].ID() != "053_memory_project_scope_settings" {
		t.Fatalf("migration head = %#v, want 053_memory_project_scope_settings", applied)
	}

	const (
		userID   = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		memoryID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	)
	if _, err := db.ExecContext(ctx, `
INSERT INTO users(id, display_name) VALUES ($1, 'Memory v2 role fixture')
`, userID); err != nil {
		t.Fatalf("insert runtime role user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = db.ExecContext(cleanupCtx, `DELETE FROM users WHERE id = $1`, userID)
	})

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("set go_api_runtime: %v", err)
	}
	var sensitive bool
	var l2Mode, l3Mode string
	if err := tx.QueryRowContext(ctx, `
INSERT INTO user_memory_settings(
  user_id, enabled, search_enabled, auto_record_enabled
) VALUES ($1, true, true, false)
RETURNING sensitive_memory_enabled, l2_mode, l3_mode
`, userID).Scan(&sensitive, &l2Mode, &l3Mode); err != nil {
		_ = tx.Rollback()
		t.Fatalf("go_api_runtime insert settings: %v", err)
	}
	if sensitive || l2Mode != "inherit" || l3Mode != "inherit" {
		_ = tx.Rollback()
		t.Fatalf("runtime defaults = sensitive %t, l2 %q, l3 %q", sensitive, l2Mode, l3Mode)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO user_memories(
  id, user_id, memory_type, content, normalized_content, source, scope_type
) VALUES ($1, $2, 'fact', 'runtime role fact', 'runtime role fact', 'manual', 'global')
ON CONFLICT (user_id, normalized_content)
WHERE deleted_at IS NULL AND scope_type = 'global' DO UPDATE SET
  content = EXCLUDED.content,
  updated_at = now()
`, memoryID, userID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("go_api_runtime insert Global Memory: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit go_api_runtime proof: %v", err)
	}

	var projectDML bool
	if err := db.QueryRowContext(ctx, `
SELECT has_table_privilege(
  'go_api_runtime', format('%I.projects', current_schema()),
  'SELECT,INSERT,UPDATE,DELETE'
)`).Scan(&projectDML); err != nil {
		t.Fatalf("read projects runtime privilege: %v", err)
	}
	if projectDML {
		t.Fatal("go_api_runtime unexpectedly has Project table DML")
	}

	deniedTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deniedTx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		_ = deniedTx.Rollback()
		t.Fatalf("set go_api_runtime for denial proof: %v", err)
	}
	if _, err := deniedTx.ExecContext(ctx, `SELECT count(*) FROM projects`); err == nil {
		_ = deniedTx.Rollback()
		t.Fatal("go_api_runtime unexpectedly read projects")
	}
	if err := deniedTx.Rollback(); err != nil {
		t.Fatalf("rollback denied Project read: %v", err)
	}
}

func mustExecMemoryV2MigrationSQL(
	t *testing.T,
	ctx context.Context,
	conn *sql.Conn,
	query string,
) {
	t.Helper()
	if _, err := conn.ExecContext(ctx, query); err != nil {
		t.Fatalf("execute Memory v2 migration fixture SQL: %v", err)
	}
}

func assertMemoryV2GlobalBackfill(
	t *testing.T,
	ctx context.Context,
	conn *sql.Conn,
	memoryID string,
) {
	t.Helper()
	var scopeType string
	var projectID, conversationID sql.NullString
	var generation int64
	if err := conn.QueryRowContext(ctx, `
SELECT scope_type, project_id::text, scope_conversation_id::text, scope_generation
FROM user_memories
WHERE id = $1`, memoryID).Scan(&scopeType, &projectID, &conversationID, &generation); err != nil {
		t.Fatalf("read migrated Memory scope: %v", err)
	}
	if scopeType != "global" || projectID.Valid || conversationID.Valid || generation != 1 {
		t.Fatalf("migrated Memory scope = %q/%v/%v/%d", scopeType, projectID, conversationID, generation)
	}
}
