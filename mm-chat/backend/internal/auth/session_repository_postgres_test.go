package auth

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/migration"
	"neo-chat/mm-chat/backend/internal/redisstate"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresSessionRepositoryLookupSessionByTokenHash(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID := "33333333-3333-4333-8333-333333333333"
	sessionID := "44444444-4444-4444-8444-444444444444"
	tokenHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name)
VALUES ($1, 'session-test@example.test', 'Session User')
ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name, deleted_at = NULL
`, userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at, revoked_at = NULL
`, sessionID, userID, tokenHash, expiresAt); err != nil {
		t.Fatalf("insert test session: %v", err)
	}

	session, err := repo.LookupSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("LookupSessionByTokenHash() error = %v", err)
	}
	if session.ID != sessionID || session.UserID != userID || session.DisplayName != "Session User" || session.Role != defaultUserRole {
		t.Fatalf("LookupSessionByTokenHash() session = %#v", session)
	}
	if !session.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("ExpiresAt = %s, want %s", session.ExpiresAt, expiresAt)
	}

	_, err = repo.LookupSessionByTokenHash(ctx, "missing-token-hash")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("LookupSessionByTokenHash() missing error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionResolverIntegrationFallsBackToPostgresAfterRedisFlush(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	redisURL := testRedisURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisClient, err := redisstate.Open(ctx, config.RedisConfig{
		URL:       redisURL,
		KeyPrefix: "mm-chat-auth-session-test",
	})
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	defer redisClient.Close()
	flushRedisDB(t, ctx, redisURL)

	userID := "55555555-5555-4555-8555-555555555555"
	sessionID := "66666666-6666-4666-8666-666666666666"
	tokenHash := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name)
VALUES ($1, 'session-flush@example.test', 'Flush User')
ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name, deleted_at = NULL
`, userID); err != nil {
		t.Fatalf("insert flush user: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE SET token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at, revoked_at = NULL
`, sessionID, userID, tokenHash, expiresAt); err != nil {
		t.Fatalf("insert flush session: %v", err)
	}

	resolver := NewSessionResolver(
		NewPostgresSessionRepository(db),
		WithSessionCache(redisClient.SessionCacheStore(5*time.Minute)),
	)

	first, err := resolver.ResolveByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("ResolveByTokenHash() first error = %v", err)
	}
	if first.DisplayName != "Flush User" {
		t.Fatalf("first DisplayName = %q", first.DisplayName)
	}

	if _, err := db.ExecContext(ctx, `UPDATE users SET display_name = 'Flush User Updated' WHERE id = $1`, userID); err != nil {
		t.Fatalf("update flush user: %v", err)
	}
	flushRedisDB(t, ctx, redisURL)

	second, err := resolver.ResolveByTokenHash(ctx, tokenHash)
	if err != nil {
		t.Fatalf("ResolveByTokenHash() after Redis flush error = %v", err)
	}
	if second.DisplayName != "Flush User Updated" {
		t.Fatalf("second DisplayName = %q, want Postgres value after Redis flush", second.DisplayName)
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

func testRedisURL(t *testing.T) string {
	t.Helper()

	redisURL := os.Getenv("MM_CHAT_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set MM_CHAT_TEST_REDIS_URL to run Redis integration tests")
	}

	return redisURL
}

func flushRedisDB(t *testing.T, ctx context.Context, redisURL string) {
	t.Helper()
	if os.Getenv("MM_CHAT_TEST_REDIS_ALLOW_FLUSH") != "true" {
		t.Skip("set MM_CHAT_TEST_REDIS_ALLOW_FLUSH=true only with a disposable Redis DB")
	}

	options, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_REDIS_URL: %v", err)
	}
	client := redis.NewClient(options)
	defer client.Close()
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush redis test database: %v", err)
	}
}
