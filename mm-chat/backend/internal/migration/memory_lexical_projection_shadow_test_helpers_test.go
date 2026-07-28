package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func openMemoryLexicalMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}
	adminConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	adminConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	adminDB := stdlib.OpenDB(*adminConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatalf("ping lexical migration admin database: %v", err)
	}
	databaseName := fmt.Sprintf("mm_chat_memory_lexical_%d", time.Now().UnixNano())
	quotedDatabase := pgx.Identifier{databaseName}.Sanitize()
	if _, err := adminDB.ExecContext(ctx, `CREATE DATABASE `+quotedDatabase); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create disposable lexical migration database: %v", err)
	}

	testConfig := adminConfig.Copy()
	testConfig.Database = databaseName
	testDB := stdlib.OpenDB(*testConfig)
	if err := testDB.PingContext(ctx); err != nil {
		_ = testDB.Close()
		_, _ = adminDB.ExecContext(ctx, `DROP DATABASE `+quotedDatabase+` WITH (FORCE)`)
		_ = adminDB.Close()
		t.Fatalf("ping disposable lexical migration database: %v", err)
	}
	t.Cleanup(func() {
		_ = testDB.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dropCancel()
		if _, err := adminDB.ExecContext(
			dropCtx,
			`DROP DATABASE IF EXISTS `+quotedDatabase+` WITH (FORCE)`,
		); err != nil {
			t.Errorf("drop disposable lexical migration database: %v", err)
		}
		_ = adminDB.Close()
	})
	return testDB
}
