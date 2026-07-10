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

	userID := mustSessionTestUUID(t)
	sessionID := mustSessionTestUUID(t)
	userEmail := "session-test-" + userID + "@example.test"
	tokenHash := HashSessionToken("lookup-token-" + sessionID)
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name)
VALUES ($1, $2, 'Session User')
ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name, deleted_at = NULL
`, userID, userEmail); err != nil {
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

func TestPostgresSessionRepositoryCreatesTwoUserSessions(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	repo := NewPostgresSessionRepository(db)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userAID := mustSessionTestUUID(t)
	userBID := mustSessionTestUUID(t)
	sessionAID := mustSessionTestUUID(t)
	sessionBID := mustSessionTestUUID(t)
	tokenA := "raw-token-user-a-" + sessionAID
	tokenB := "raw-token-user-b-" + sessionBID
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name)
VALUES ($1, $2, 'User A'), ($3, $4, 'User B')
`,
		userAID,
		"session-a-"+userAID+"@example.test",
		userBID,
		"session-b-"+userBID+"@example.test",
	); err != nil {
		t.Fatalf("insert session users: %v", err)
	}

	sessionA, err := repo.CreateSession(ctx, CreateSessionInput{
		SessionID:   sessionAID,
		UserID:      userAID,
		DisplayName: "User A",
		TokenHash:   HashSessionToken(tokenA),
		UserAgent:   "test-a",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateSession(user A) error = %v", err)
	}
	sessionB, err := repo.CreateSession(ctx, CreateSessionInput{
		SessionID:   sessionBID,
		UserID:      userBID,
		DisplayName: "User B",
		TokenHash:   HashSessionToken(tokenB),
		UserAgent:   "test-b",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateSession(user B) error = %v", err)
	}
	if sessionA.UserID != userAID || sessionB.UserID != userBID {
		t.Fatalf("created sessions = %#v/%#v", sessionA, sessionB)
	}

	resolver := NewSessionResolver(repo, WithClock(func() time.Time { return time.Now().UTC() }))
	resolvedA, err := resolver.ResolveByTokenHash(ctx, HashSessionToken(tokenA))
	if err != nil {
		t.Fatalf("ResolveByTokenHash(user A) error = %v", err)
	}
	resolvedB, err := resolver.ResolveByTokenHash(ctx, HashSessionToken(tokenB))
	if err != nil {
		t.Fatalf("ResolveByTokenHash(user B) error = %v", err)
	}
	if resolvedA.UserID != userAID || resolvedA.DisplayName != "User A" {
		t.Fatalf("resolved A = %#v", resolvedA)
	}
	if resolvedB.UserID != userBID || resolvedB.DisplayName != "User B" {
		t.Fatalf("resolved B = %#v", resolvedB)
	}
	if resolvedA.ID == resolvedB.ID || resolvedA.UserID == resolvedB.UserID {
		t.Fatalf("sessions were not distinct: %#v/%#v", resolvedA, resolvedB)
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

	userID := mustSessionTestUUID(t)
	sessionID := mustSessionTestUUID(t)
	userEmail := "session-flush-" + userID + "@example.test"
	tokenHash := HashSessionToken("flush-token-" + sessionID)
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)

	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, email, display_name)
VALUES ($1, $2, 'Flush User')
ON CONFLICT (id) DO UPDATE SET display_name = EXCLUDED.display_name, deleted_at = NULL
`, userID, userEmail); err != nil {
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

func mustSessionTestUUID(t *testing.T) string {
	t.Helper()
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID() error = %v", err)
	}
	return id
}
