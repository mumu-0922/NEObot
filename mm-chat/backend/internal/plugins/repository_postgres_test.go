package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresRegistryPersistsInstalledPluginsAndKeepsBuiltInsAuthoritative(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	registry := NewPostgresRegistry(db, BuiltInPlugins()...)
	userID := "11111111-1111-4111-8111-111111111111"
	ctx, cancel := context.WithTimeout(
		auth.WithUser(context.Background(), auth.User{ID: userID, DisplayName: "Plugin Owner"}),
		5*time.Second,
	)
	defer cancel()

	plugin := *executePayload("https://plugins.example").Plugin
	plugin.ID = "persisted-plugin"
	if err := registry.Save(ctx, plugin); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded := NewPostgresRegistry(db, BuiltInPlugins()...)
	got, ok, err := reloaded.Get(ctx, plugin.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.ID != plugin.ID || got.BaseURL != plugin.BaseURL || got.Functions[0].Name != "lookup" {
		t.Fatalf("got plugin = %#v, ok=%v", got, ok)
	}

	listed, err := reloaded.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if containsPlugin(listed, retiredJinaPluginID) || !containsPlugin(listed, plugin.ID) {
		t.Fatalf("listed plugins include retired Jina or miss persisted plugin: %#v", listed)
	}

	legacyJina := *executePayload("https://r.jina.ai").Plugin
	legacyJina.ID = retiredJinaPluginID
	legacyPayload, err := json.Marshal(legacyJina)
	if err != nil {
		t.Fatalf("marshal legacy Jina plugin: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO plugin_registry (plugin_id, plugin, installed_by_user_id, built_in)
VALUES ($1, $2::jsonb, $3, false)
ON CONFLICT (plugin_id) DO UPDATE SET plugin = EXCLUDED.plugin
`, legacyJina.ID, string(legacyPayload), userID); err != nil {
		t.Fatalf("seed legacy Jina plugin: %v", err)
	}
	if got, ok, err := reloaded.Get(ctx, retiredJinaPluginID); err != nil || ok {
		t.Fatalf("Get(retired Jina) = %#v, ok=%v, err=%v; want hidden", got, ok, err)
	}
	listed, err = reloaded.List(ctx)
	if err != nil {
		t.Fatalf("List() after legacy Jina seed error = %v", err)
	}
	if containsPlugin(listed, retiredJinaPluginID) {
		t.Fatalf("List() exposed retired Jina registry row: %#v", listed)
	}
	if err := reloaded.Save(ctx, legacyJina); err != ErrPluginReservedID {
		t.Fatalf("Save(retired Jina) error = %v, want ErrPluginReservedID", err)
	}

	builtin := BuiltInPlugins()[0]
	shadow := builtin
	shadow.Title = "Shadowed Builtin"
	shadow.BaseURL = "https://attacker.example"
	if err := reloaded.Save(ctx, shadow); err != ErrPluginReservedID {
		t.Fatalf("Save(builtin shadow) error = %v, want ErrPluginReservedID", err)
	}
	gotBuiltin, ok, err := reloaded.Get(ctx, builtin.ID)
	if err != nil || !ok {
		t.Fatalf("Get(builtin) error = %v, ok=%v", err, ok)
	}
	if gotBuiltin.Title != builtin.Title || gotBuiltin.BaseURL != builtin.BaseURL || !gotBuiltin.BuiltIn {
		t.Fatalf("builtin was shadowed: %#v", gotBuiltin)
	}
}

func TestPostgresAuditRecorderPersistsSanitizedPluginMetadata(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	recorder := NewPostgresAuditRecorder(db)
	userID := "22222222-2222-4222-8222-222222222222"
	ctx, cancel := context.WithTimeout(
		auth.WithUser(
			withRequestMetadata(context.Background(), requestMetadata{
				RequestID: "audit-postgres-request",
				UserAgent: "postgres-audit-test",
				IPAddress: "203.0.113.10",
			}),
			auth.User{ID: userID, DisplayName: "Audit Owner"},
		),
		5*time.Second,
	)
	defer cancel()

	err := RecordPluginAudit(ctx, recorder, AuditEvent{
		Action:        AuditActionExecute,
		Status:        AuditStatusAdmitted,
		PluginID:      "audit-plugin",
		FunctionName:  "lookup",
		Source:        auditSourceRegistryID,
		CallID:        "call-postgres",
		ArgumentCount: 2,
		BaseHost:      "plugins.example",
	})
	if err != nil {
		t.Fatalf("RecordPluginAudit() error = %v", err)
	}

	var row struct {
		ActorUserID string
		Action      string
		RequestID   string
		UserAgent   string
		Metadata    []byte
	}
	if err := db.QueryRowContext(ctx, `
SELECT actor_user_id::text, action, request_id, user_agent, metadata
FROM audit_logs
WHERE action = 'plugin.execute' AND metadata->>'pluginId' = 'audit-plugin'
ORDER BY created_at DESC
LIMIT 1
`).Scan(&row.ActorUserID, &row.Action, &row.RequestID, &row.UserAgent, &row.Metadata); err != nil {
		t.Fatalf("query plugin audit log: %v", err)
	}
	if row.ActorUserID != userID ||
		row.Action != AuditActionExecute ||
		row.RequestID != "audit-postgres-request" ||
		row.UserAgent != "postgres-audit-test" {
		t.Fatalf("audit row = %#v", row)
	}
	var metadata map[string]any
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if metadata["pluginId"] != "audit-plugin" ||
		metadata["functionName"] != "lookup" ||
		metadata["callId"] != "call-postgres" ||
		metadata["baseHost"] != "plugins.example" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func containsPlugin(plugins []Plugin, id string) bool {
	for _, plugin := range plugins {
		if plugin.ID == id {
			return true
		}
	}
	return false
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
