package plugins

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
	if !containsPlugin(listed, "jina-web-reader") || !containsPlugin(listed, plugin.ID) {
		t.Fatalf("listed plugins missing built-in or persisted plugin: %#v", listed)
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
