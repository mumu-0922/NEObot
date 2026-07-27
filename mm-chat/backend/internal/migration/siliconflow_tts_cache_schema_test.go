package migration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestSiliconFlowTTSCacheMigrationContract(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("051_siliconflow_tts_cache.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("051_siliconflow_tts_cache.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := string(upBytes)
	down := string(downBytes)
	for _, required := range []string{
		"provider_configs_voice_identity_check",
		"VOICE:SILICONFLOW",
		"FunAudioLLM/CosyVoice2-0.5B:claire",
		"CREATE TABLE tts_audio_cache",
		"UNIQUE (user_id, message_id)",
		"REFERENCES messages(id) ON DELETE RESTRICT",
		"CREATE TABLE tts_audio_cleanup_queue",
		"idx_tts_audio_cache_user_lru",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("051 up migration missing %q", required)
		}
	}
	for _, required := range []string{
		"DROP TABLE IF EXISTS tts_audio_cleanup_queue",
		"DROP TABLE IF EXISTS tts_audio_cache",
		"DROP CONSTRAINT IF EXISTS provider_configs_voice_identity_check",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("051 down migration missing %q", required)
		}
	}
}

func TestSiliconFlowTTSRuntimeRoleGrantMigrationContract(t *testing.T) {
	upBytes, err := migrationfiles.FS.ReadFile("052_tts_runtime_role_grants.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	downBytes, err := migrationfiles.FS.ReadFile("052_tts_runtime_role_grants.down.sql")
	if err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{
		"GRANT SELECT, INSERT, UPDATE, DELETE",
		"ON TABLE tts_audio_cache, tts_audio_cleanup_queue",
		"TO go_api_runtime",
	} {
		if !strings.Contains(string(upBytes), required) {
			t.Fatalf("052 up migration missing %q", required)
		}
	}
	for _, required := range []string{
		"REVOKE SELECT, INSERT, UPDATE, DELETE",
		"ON TABLE tts_audio_cache, tts_audio_cleanup_queue",
		"FROM go_api_runtime",
	} {
		if !strings.Contains(string(downBytes), required) {
			t.Fatalf("052 down migration missing %q", required)
		}
	}
}

func TestSiliconFlowTTSRuntimeRoleGrantMigrationLivePostgres(t *testing.T) {
	db := openPhase151CMigrationIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := NewRunner(db, migrationfiles.FS)
	applied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("apply migrations through 052: %v", err)
	}
	if len(applied) == 0 || applied[len(applied)-1].ID() != "052_tts_runtime_role_grants" {
		t.Fatalf("migration head = %#v, want 052_tts_runtime_role_grants", applied)
	}
	assertTTSRuntimeRoleGrants(t, ctx, db, true)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL ROLE go_api_runtime`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("set go_api_runtime: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `
LOCK TABLE tts_audio_cache, tts_audio_cleanup_queue
IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		_ = tx.Rollback()
		t.Fatalf("go_api_runtime lock TTS tables: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback role proof: %v", err)
	}

	rolledBack, err := runner.Down(ctx, false)
	if err != nil {
		t.Fatalf("roll back migration 052: %v", err)
	}
	if len(rolledBack) != 1 || rolledBack[0].ID() != "052_tts_runtime_role_grants" {
		t.Fatalf("rolled back = %#v, want only migration 052", rolledBack)
	}
	assertTTSRuntimeRoleGrants(t, ctx, db, false)

	reapplied, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("reapply migration 052: %v", err)
	}
	if len(reapplied) != 1 || reapplied[0].ID() != "052_tts_runtime_role_grants" {
		t.Fatalf("reapplied = %#v, want only migration 052", reapplied)
	}
	assertTTSRuntimeRoleGrants(t, ctx, db, true)
}

func assertTTSRuntimeRoleGrants(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	want bool,
) {
	t.Helper()
	for _, table := range []string{"tts_audio_cache", "tts_audio_cleanup_queue"} {
		var dml bool
		var truncate bool
		if err := db.QueryRowContext(ctx, `
SELECT
  has_table_privilege(
    'go_api_runtime', format('%I.%I', current_schema(), $1),
    'SELECT,INSERT,UPDATE,DELETE'
  ),
  has_table_privilege(
    'go_api_runtime', format('%I.%I', current_schema(), $1),
    'TRUNCATE'
  )`, table).Scan(&dml, &truncate); err != nil {
			t.Fatalf("read go_api_runtime privileges for %s: %v", table, err)
		}
		if dml != want {
			t.Errorf("go_api_runtime DML on %s = %t, want %t", table, dml, want)
		}
		if truncate {
			t.Errorf("go_api_runtime unexpectedly has TRUNCATE on %s", table)
		}
	}
}
