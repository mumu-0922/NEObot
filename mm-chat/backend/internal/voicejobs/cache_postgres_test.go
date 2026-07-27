package voicejobs

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/migration"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresSynthesisCacheLifecycleIsolationLRUExpiryAndReplay(t *testing.T) {
	db := openVoiceCachePostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repo := NewPostgresSynthesisCacheRepository(db)

	userA := "71000000-0000-4000-8000-000000000001"
	userB := "71000000-0000-4000-8000-000000000002"
	conversationA := "72000000-0000-4000-8000-000000000001"
	conversationB := "72000000-0000-4000-8000-000000000002"
	messageA := "73000000-0000-4000-8000-000000000001"
	messageB := "73000000-0000-4000-8000-000000000002"
	messageC := "73000000-0000-4000-8000-000000000003"
	fileA := "74000000-0000-4000-8000-000000000001"
	fileB := "74000000-0000-4000-8000-000000000002"
	fileC := "74000000-0000-4000-8000-000000000003"
	fileD := "74000000-0000-4000-8000-000000000004"
	cleanupVoiceCacheFixtures(t, ctx, db, userA, userB)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		cleanupVoiceCacheFixtures(t, cleanupCtx, db, userA, userB)
	})

	mustExecVoiceCache(t, ctx, db, `
INSERT INTO users (id, display_name) VALUES ($1, 'Voice A'), ($2, 'Voice B')
`, userA, userB)
	mustExecVoiceCache(t, ctx, db, `
INSERT INTO conversations (id, user_id, title)
VALUES ($1, $2, 'Voice A'), ($3, $4, 'Voice B')
`, conversationA, userA, conversationB, userB)
	mustExecVoiceCache(t, ctx, db, `
INSERT INTO messages (id, conversation_id, user_id, sequence_no, role, content)
VALUES
  ($1, $2, $3, 0, 'assistant', 'first message'),
  ($4, $2, $3, 1, 'assistant', 'second message'),
  ($5, $2, $3, 2, 'assistant', 'third message')
`, messageA, conversationA, userA, messageB, messageC)
	insertVoiceCacheFile(t, ctx, db, fileA, userA, 60)
	insertVoiceCacheFile(t, ctx, db, fileB, userA, 60)
	insertVoiceCacheFile(t, ctx, db, fileC, userA, 50)
	insertVoiceCacheFile(t, ctx, db, fileD, userA, 60)

	userACtx := auth.WithUser(ctx, auth.User{ID: userA})
	sourceA, err := repo.ResolveSynthesisSource(userACtx, messageA)
	if err != nil {
		t.Fatal(err)
	}
	sourceB, err := repo.ResolveSynthesisSource(userACtx, messageB)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	keyA := integrationCacheKey(sourceA)
	keyB := integrationCacheKey(sourceB)
	if err := repo.CommitCachedSynthesis(userACtx, CommitCachedSynthesisInput{
		ID: "75000000-0000-4000-8000-000000000001", Key: keyA,
		FileID: fileA, ContentType: "audio/mpeg", Size: 60, AccessedAt: now, MaxBytes: 100,
	}); err != nil {
		t.Fatalf("commit first cache: %v", err)
	}
	if cached, ok, err := repo.GetCachedSynthesis(userACtx, keyA, now.Add(time.Minute)); err != nil || !ok || cached.FileID != fileA {
		t.Fatalf("first cache hit = %#v/%v/%v", cached, ok, err)
	}
	userBCtx := auth.WithUser(ctx, auth.User{ID: userB})
	if _, ok, err := repo.GetCachedSynthesis(userBCtx, keyA, now.Add(time.Minute)); err != nil || ok {
		t.Fatalf("cross-user cache lookup = %v/%v, want miss", ok, err)
	}

	if err := repo.CommitCachedSynthesis(userACtx, CommitCachedSynthesisInput{
		ID: "75000000-0000-4000-8000-000000000002", Key: keyB,
		FileID: fileB, ContentType: "audio/mpeg", Size: 60, AccessedAt: now.Add(2 * time.Minute), MaxBytes: 100,
	}); err != nil {
		t.Fatalf("commit second cache: %v", err)
	}
	assertVoiceCacheFile(t, ctx, db, messageA, "")
	assertVoiceCacheFile(t, ctx, db, messageB, fileB)
	assertVoiceCleanupReason(t, ctx, db, fileA, "lru")

	if err := repo.CommitCachedSynthesis(userACtx, CommitCachedSynthesisInput{
		ID: "75000000-0000-4000-8000-000000000003", Key: keyB,
		FileID: fileC, ContentType: "audio/mpeg", Size: 50, AccessedAt: now.Add(3 * time.Minute), MaxBytes: 100,
	}); err != nil {
		t.Fatalf("replace second cache: %v", err)
	}
	assertVoiceCacheFile(t, ctx, db, messageB, fileC)
	assertVoiceCleanupReason(t, ctx, db, fileB, "replaced")

	mustExecVoiceCache(t, ctx, db, `
	UPDATE tts_audio_cache
	SET created_at = $2, updated_at = $2, last_accessed_at = $2
	WHERE message_id = $1
`, messageB, now.Add(-voiceCacheIdleTTL-time.Minute))
	if _, ok, err := repo.GetCachedSynthesis(userACtx, keyB, now); err != nil || ok {
		t.Fatalf("expired cache lookup = %v/%v, want miss", ok, err)
	}
	if err := repo.PrepareArtifactCleanup(ctx, now.Add(-voiceCacheIdleTTL), 100, 16); err != nil {
		t.Fatalf("prepare expired cleanup: %v", err)
	}
	assertVoiceCacheFile(t, ctx, db, messageB, "")
	assertVoiceCleanupReason(t, ctx, db, fileC, "expired")

	mustExecVoiceCache(t, ctx, db, `
UPDATE messages SET content = 'changed message', updated_at = updated_at + interval '1 second'
WHERE id = $1
`, messageB)
	if err := repo.CommitCachedSynthesis(userACtx, CommitCachedSynthesisInput{
		ID: "75000000-0000-4000-8000-000000000004", Key: keyB,
		FileID: fileD, ContentType: "audio/mpeg", Size: 60, AccessedAt: now.Add(4 * time.Minute), MaxBytes: 100,
	}); !errors.Is(err, ErrVoiceSourceMessageChanged) {
		t.Fatalf("stale source commit error = %v", err)
	}

	claimOne := "76000000-0000-4000-8000-000000000001"
	claimed, err := repo.ClaimArtifactCleanup(ctx, claimOne, now.Add(-time.Hour), 16)
	if err != nil || len(claimed) != 3 {
		t.Fatalf("first cleanup claim = %#v, %v", claimed, err)
	}
	if err := repo.ReleaseArtifactCleanup(ctx, claimed[0].ID, claimOne); err != nil {
		t.Fatalf("release cleanup claim: %v", err)
	}
	for _, item := range claimed[1:] {
		if err := repo.CompleteArtifactCleanup(ctx, item.ID, claimOne); err != nil {
			t.Fatalf("complete cleanup claim: %v", err)
		}
	}
	claimTwo := "76000000-0000-4000-8000-000000000002"
	replayed, err := repo.ClaimArtifactCleanup(ctx, claimTwo, now.Add(-time.Hour), 16)
	if err != nil || len(replayed) != 1 || replayed[0].ID != claimed[0].ID {
		t.Fatalf("replayed cleanup claim = %#v, %v", replayed, err)
	}
	if err := repo.CompleteArtifactCleanup(ctx, replayed[0].ID, claimTwo); err != nil {
		t.Fatalf("complete replayed cleanup: %v", err)
	}

	sourceC, err := repo.ResolveSynthesisSource(userACtx, messageC)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CommitCachedSynthesis(userACtx, CommitCachedSynthesisInput{
		ID: "75000000-0000-4000-8000-000000000005", Key: keyA,
		FileID: fileA, ContentType: "audio/mpeg", Size: 60, AccessedAt: now.Add(10 * time.Minute), MaxBytes: 1000,
	}); err != nil {
		t.Fatalf("restore first cache for cleanup LRU: %v", err)
	}
	if err := repo.CommitCachedSynthesis(userACtx, CommitCachedSynthesisInput{
		ID: "75000000-0000-4000-8000-000000000006", Key: integrationCacheKey(sourceC),
		FileID: fileD, ContentType: "audio/mpeg", Size: 60, AccessedAt: now.Add(11 * time.Minute), MaxBytes: 1000,
	}); err != nil {
		t.Fatalf("commit third cache for cleanup LRU: %v", err)
	}
	if err := repo.PrepareArtifactCleanup(ctx, now.Add(-365*24*time.Hour), 100, 16); err != nil {
		t.Fatalf("prepare LRU cleanup: %v", err)
	}
	assertVoiceCacheFile(t, ctx, db, messageA, "")
	assertVoiceCacheFile(t, ctx, db, messageC, fileD)
	assertVoiceCleanupReason(t, ctx, db, fileA, "lru")
	claimThree := "76000000-0000-4000-8000-000000000003"
	lruClaimed, err := repo.ClaimArtifactCleanup(ctx, claimThree, now.Add(-time.Hour), 16)
	if err != nil || len(lruClaimed) != 1 || lruClaimed[0].FileID != fileA {
		t.Fatalf("LRU cleanup claim = %#v, %v", lruClaimed, err)
	}
	if err := repo.CompleteArtifactCleanup(ctx, lruClaimed[0].ID, claimThree); err != nil {
		t.Fatalf("complete LRU cleanup: %v", err)
	}
}

func openVoiceCachePostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("MM_CHAT_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}
	parsed, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	if !strings.HasPrefix(parsed.Database, "mm_chat_") || !strings.HasSuffix(parsed.Database, "_test") {
		t.Fatal("MM_CHAT_TEST_DATABASE_URL must name an isolated mm_chat_*_test database")
	}
	parsed.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*parsed)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close Voice cache integration database: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}

func integrationCacheKey(source SynthesisSource) SynthesisCacheKey {
	return SynthesisCacheKey{
		MessageID: source.MessageID, TextSHA256: synthesisTextDigest(strings.TrimSpace(source.Text)),
		SourceUpdatedAt: source.UpdatedAt, ProviderID: "siliconflow",
		ModelID: "FunAudioLLM/CosyVoice2-0.5B", VoiceID: "FunAudioLLM/CosyVoice2-0.5B:claire",
	}
}

func insertVoiceCacheFile(t *testing.T, ctx context.Context, db *sql.DB, fileID string, userID string, size int64) {
	t.Helper()
	mustExecVoiceCache(t, ctx, db, `
INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256,
  storage_backend, object_key, upload_status
) VALUES ($1, $2, 'voice.mp3', 'audio/mpeg', $3, $4, 'local', $5, 'available')
`, fileID, userID, size, strings.Repeat("a", 64), "voice-cache-test/"+fileID)
}

func assertVoiceCacheFile(t *testing.T, ctx context.Context, db *sql.DB, messageID string, wantFileID string) {
	t.Helper()
	var fileID string
	err := db.QueryRowContext(ctx, `
SELECT file_id::text FROM tts_audio_cache WHERE message_id = $1
`, messageID).Scan(&fileID)
	if wantFileID == "" && errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil || fileID != wantFileID {
		t.Fatalf("cache file for %s = %q, %v; want %q", messageID, fileID, err, wantFileID)
	}
}

func assertVoiceCleanupReason(t *testing.T, ctx context.Context, db *sql.DB, fileID string, wantReason string) {
	t.Helper()
	var reason string
	if err := db.QueryRowContext(ctx, `
SELECT reason FROM tts_audio_cleanup_queue WHERE file_id = $1
`, fileID).Scan(&reason); err != nil || reason != wantReason {
		t.Fatalf("cleanup reason for %s = %q, %v; want %q", fileID, reason, err, wantReason)
	}
}

func mustExecVoiceCache(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatalf("Voice cache fixture SQL failed: %v", err)
	}
}

func cleanupVoiceCacheFixtures(t *testing.T, ctx context.Context, db *sql.DB, userIDs ...string) {
	t.Helper()
	for _, query := range []string{
		`DELETE FROM tts_audio_cleanup_queue WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM tts_audio_cache WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM files WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM messages WHERE conversation_id IN (SELECT id FROM conversations WHERE user_id = ANY($1::uuid[]))`,
		`DELETE FROM conversations WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM users WHERE id = ANY($1::uuid[])`,
	} {
		if _, err := db.ExecContext(ctx, query, userIDs); err != nil {
			t.Fatalf("clean Voice cache fixtures: %v", err)
		}
	}
}
