package voicejobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

type PostgresSynthesisCacheRepository struct {
	db *sql.DB
}

func NewPostgresSynthesisCacheRepository(db *sql.DB) *PostgresSynthesisCacheRepository {
	return &PostgresSynthesisCacheRepository{db: db}
}

func (r *PostgresSynthesisCacheRepository) ResolveSynthesisSource(
	ctx context.Context,
	messageID string,
) (SynthesisSource, error) {
	if r == nil || r.db == nil {
		return SynthesisSource{}, ErrVoiceCacheUnavailable
	}
	messageID = strings.TrimSpace(messageID)
	if !isVoiceUUID(messageID) {
		return SynthesisSource{}, ErrVoiceSourceMessageNotFound
	}
	userID := auth.UserOrDevelopment(ctx).ID
	var source SynthesisSource
	err := r.db.QueryRowContext(ctx, `
SELECT message.id::text, message.content, message.updated_at
FROM messages AS message
JOIN conversations AS conversation ON conversation.id = message.conversation_id
JOIN users AS owner ON owner.id = conversation.user_id
WHERE message.id = $1
  AND conversation.user_id = $2
  AND message.deleted_at IS NULL
  AND conversation.deleted_at IS NULL
  AND owner.deleted_at IS NULL
`, messageID, userID).Scan(&source.MessageID, &source.Text, &source.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SynthesisSource{}, ErrVoiceSourceMessageNotFound
	}
	if err != nil {
		return SynthesisSource{}, fmt.Errorf("resolve voice synthesis source: %w", err)
	}
	return source, nil
}

func (r *PostgresSynthesisCacheRepository) GetCachedSynthesis(
	ctx context.Context,
	key SynthesisCacheKey,
	accessedAt time.Time,
) (CachedSynthesis, bool, error) {
	if r == nil || r.db == nil {
		return CachedSynthesis{}, false, ErrVoiceCacheUnavailable
	}
	userID := auth.UserOrDevelopment(ctx).ID
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CachedSynthesis{}, false, fmt.Errorf("begin voice cache lookup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var cacheID string
	var cached CachedSynthesis
	err = tx.QueryRowContext(ctx, `
SELECT cache.id::text, cache.file_id::text, cache.content_type, cache.byte_size
FROM tts_audio_cache AS cache
JOIN messages AS message ON message.id = cache.message_id
JOIN conversations AS conversation ON conversation.id = message.conversation_id
JOIN users AS owner ON owner.id = cache.user_id
JOIN files AS file ON file.id = cache.file_id
WHERE cache.user_id = $1
  AND cache.message_id = $2
  AND cache.text_sha256 = $3
  AND cache.source_updated_at = $4
  AND cache.provider_id = $5
  AND cache.model_id = $6
  AND cache.voice_id = $7
  AND cache.last_accessed_at >= $8
  AND message.updated_at = cache.source_updated_at
  AND message.deleted_at IS NULL
  AND conversation.user_id = cache.user_id
  AND conversation.deleted_at IS NULL
  AND owner.deleted_at IS NULL
  AND file.user_id = cache.user_id
  AND file.deleted_at IS NULL
  AND file.upload_status = 'available'
FOR UPDATE OF cache
`,
		userID,
		key.MessageID,
		key.TextSHA256,
		key.SourceUpdatedAt.UTC(),
		strings.TrimSpace(key.ProviderID),
		strings.TrimSpace(key.ModelID),
		strings.TrimSpace(key.VoiceID),
		accessedAt.UTC().Add(-voiceCacheIdleTTL),
	).Scan(&cacheID, &cached.FileID, &cached.ContentType, &cached.Size)
	if errors.Is(err, sql.ErrNoRows) {
		return CachedSynthesis{}, false, nil
	}
	if err != nil {
		return CachedSynthesis{}, false, fmt.Errorf("lookup voice cache: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE tts_audio_cache
SET last_accessed_at = $2, updated_at = $2
WHERE id = $1
`, cacheID, accessedAt.UTC()); err != nil {
		return CachedSynthesis{}, false, fmt.Errorf("touch voice cache: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CachedSynthesis{}, false, fmt.Errorf("commit voice cache lookup: %w", err)
	}
	return cached, true, nil
}

func (r *PostgresSynthesisCacheRepository) CommitCachedSynthesis(
	ctx context.Context,
	input CommitCachedSynthesisInput,
) error {
	if r == nil || r.db == nil {
		return ErrVoiceCacheUnavailable
	}
	userID := auth.UserOrDevelopment(ctx).ID
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin voice cache commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE tts_audio_cache IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock voice cache: %w", err)
	}

	var sourceText string
	var sourceUpdatedAt time.Time
	err = tx.QueryRowContext(ctx, `
SELECT message.content, message.updated_at
FROM messages AS message
JOIN conversations AS conversation ON conversation.id = message.conversation_id
JOIN users AS owner ON owner.id = conversation.user_id
WHERE message.id = $1
  AND conversation.user_id = $2
  AND message.deleted_at IS NULL
  AND conversation.deleted_at IS NULL
  AND owner.deleted_at IS NULL
FOR UPDATE OF message
`, input.Key.MessageID, userID).Scan(&sourceText, &sourceUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrVoiceSourceMessageNotFound
	}
	if err != nil {
		return fmt.Errorf("lock voice synthesis source: %w", err)
	}
	if !sourceUpdatedAt.Equal(input.Key.SourceUpdatedAt) ||
		synthesisTextDigest(strings.TrimSpace(sourceText)) != input.Key.TextSHA256 {
		return ErrVoiceSourceMessageChanged
	}

	var fileUserID string
	var fileContentType string
	var fileSize int64
	err = tx.QueryRowContext(ctx, `
SELECT user_id::text, mime_type, byte_size
FROM files
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
  AND upload_status = 'available'
FOR UPDATE
`, input.FileID, userID).Scan(&fileUserID, &fileContentType, &fileSize)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrVoiceCacheUnavailable
	}
	if err != nil {
		return fmt.Errorf("lock voice artifact: %w", err)
	}
	if fileUserID != userID || fileSize != input.Size ||
		fileContentType != input.ContentType || !strings.HasPrefix(fileContentType, "audio/") {
		return ErrVoiceCacheUnavailable
	}

	var oldFileID string
	err = tx.QueryRowContext(ctx, `
SELECT file_id::text
FROM tts_audio_cache
WHERE user_id = $1 AND message_id = $2
FOR UPDATE
`, userID, input.Key.MessageID).Scan(&oldFileID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lock existing voice cache: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO tts_audio_cache (
  id, user_id, message_id, file_id, text_sha256, source_updated_at,
  provider_id, model_id, voice_id, content_type, byte_size,
  created_at, updated_at, last_accessed_at
) VALUES (
  $1, $2, $3, $4, $5, $6,
  $7, $8, $9, $10, $11,
  $12, $12, $12
)
ON CONFLICT (user_id, message_id) DO UPDATE
SET file_id = EXCLUDED.file_id,
    text_sha256 = EXCLUDED.text_sha256,
    source_updated_at = EXCLUDED.source_updated_at,
    provider_id = EXCLUDED.provider_id,
    model_id = EXCLUDED.model_id,
    voice_id = EXCLUDED.voice_id,
    content_type = EXCLUDED.content_type,
    byte_size = EXCLUDED.byte_size,
    updated_at = EXCLUDED.updated_at,
    last_accessed_at = EXCLUDED.last_accessed_at
`,
		input.ID,
		userID,
		input.Key.MessageID,
		input.FileID,
		input.Key.TextSHA256,
		input.Key.SourceUpdatedAt.UTC(),
		strings.TrimSpace(input.Key.ProviderID),
		strings.TrimSpace(input.Key.ModelID),
		strings.TrimSpace(input.Key.VoiceID),
		input.ContentType,
		input.Size,
		input.AccessedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert voice cache: %w", err)
	}
	if oldFileID != "" && oldFileID != input.FileID {
		if err := queueArtifactCleanupTx(ctx, tx, userID, oldFileID, "replaced"); err != nil {
			return err
		}
	}
	if err := enforceUserVoiceCacheLimitTx(
		ctx,
		tx,
		userID,
		input.FileID,
		input.MaxBytes,
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit voice cache: %w", err)
	}
	return nil
}

func (r *PostgresSynthesisCacheRepository) QueueArtifactCleanup(
	ctx context.Context,
	fileID string,
	reason string,
) error {
	if r == nil || r.db == nil {
		return ErrVoiceCacheUnavailable
	}
	var userID string
	err := r.db.QueryRowContext(ctx, `SELECT user_id::text FROM files WHERE id = $1`, fileID).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve voice cleanup owner: %w", err)
	}
	cleanupID, err := newVoiceUUID()
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO tts_audio_cleanup_queue (id, user_id, file_id, reason)
VALUES ($1, $2, $3, $4)
ON CONFLICT (file_id) DO NOTHING
`, cleanupID, userID, fileID, reason)
	if err != nil {
		return fmt.Errorf("queue voice artifact cleanup: %w", err)
	}
	return nil
}

func (r *PostgresSynthesisCacheRepository) PrepareArtifactCleanup(
	ctx context.Context,
	expiresBefore time.Time,
	maxUserBytes int64,
	limit int,
) error {
	if r == nil || r.db == nil {
		return ErrVoiceCacheUnavailable
	}
	if limit <= 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin voice cleanup preparation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE tts_audio_cache IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return fmt.Errorf("lock voice cache cleanup: %w", err)
	}
	type candidate struct {
		id     string
		userID string
		fileID string
		reason string
	}
	candidates := make([]candidate, 0, limit)
	rows, err := tx.QueryContext(ctx, `
SELECT cache.id::text, cache.user_id::text, cache.file_id::text,
       CASE
         WHEN message.id IS NULL OR message.deleted_at IS NOT NULL
           OR conversation.id IS NULL OR conversation.deleted_at IS NOT NULL
           OR owner.id IS NULL OR owner.deleted_at IS NOT NULL THEN 'source_deleted'
         WHEN file.id IS NULL OR file.deleted_at IS NOT NULL
           OR file.upload_status <> 'available' THEN 'orphaned'
         ELSE 'expired'
       END AS reason
FROM tts_audio_cache AS cache
LEFT JOIN messages AS message ON message.id = cache.message_id
LEFT JOIN conversations AS conversation ON conversation.id = message.conversation_id
LEFT JOIN users AS owner ON owner.id = cache.user_id
LEFT JOIN files AS file ON file.id = cache.file_id
WHERE cache.last_accessed_at < $1
   OR message.id IS NULL OR message.deleted_at IS NOT NULL
   OR conversation.id IS NULL OR conversation.deleted_at IS NOT NULL
   OR owner.id IS NULL OR owner.deleted_at IS NOT NULL
   OR file.id IS NULL OR file.deleted_at IS NOT NULL
   OR file.upload_status <> 'available'
ORDER BY cache.last_accessed_at ASC, cache.created_at ASC
LIMIT $2
FOR UPDATE OF cache
`, expiresBefore.UTC(), limit)
	if err != nil {
		return fmt.Errorf("select expired voice cache: %w", err)
	}
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.userID, &item.fileID, &item.reason); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan expired voice cache: %w", err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close expired voice cache rows: %w", err)
	}
	for _, item := range candidates {
		if err := retireCachedArtifactTx(ctx, tx, item.id, item.userID, item.fileID, item.reason); err != nil {
			return err
		}
	}
	remaining := limit - len(candidates)
	if remaining > 0 && maxUserBytes > 0 {
		lruRows, err := tx.QueryContext(ctx, `
SELECT id::text, user_id::text, file_id::text
FROM (
  SELECT id, user_id, file_id, last_accessed_at, created_at,
         sum(byte_size) OVER (
           PARTITION BY user_id
           ORDER BY last_accessed_at DESC, created_at DESC, id DESC
           ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
         ) AS running_bytes
  FROM tts_audio_cache
) AS ranked
WHERE running_bytes > $1
ORDER BY last_accessed_at ASC, created_at ASC, id ASC
LIMIT $2
`, maxUserBytes, remaining)
		if err != nil {
			return fmt.Errorf("select voice cache LRU: %w", err)
		}
		lruCandidates := make([]candidate, 0, remaining)
		for lruRows.Next() {
			var item candidate
			item.reason = "lru"
			if err := lruRows.Scan(&item.id, &item.userID, &item.fileID); err != nil {
				_ = lruRows.Close()
				return fmt.Errorf("scan voice cache LRU: %w", err)
			}
			lruCandidates = append(lruCandidates, item)
		}
		if err := lruRows.Close(); err != nil {
			return fmt.Errorf("close voice cache LRU rows: %w", err)
		}
		for _, item := range lruCandidates {
			if err := retireCachedArtifactTx(ctx, tx, item.id, item.userID, item.fileID, item.reason); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit voice cleanup preparation: %w", err)
	}
	return nil
}

func (r *PostgresSynthesisCacheRepository) ClaimArtifactCleanup(
	ctx context.Context,
	claimID string,
	staleBefore time.Time,
	limit int,
) ([]ClaimedArtifactCleanup, error) {
	if r == nil || r.db == nil {
		return nil, ErrVoiceCacheUnavailable
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin voice cleanup claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
UPDATE tts_audio_cleanup_queue
SET claimed_at = NULL, claim_id = NULL, updated_at = now()
WHERE claimed_at IS NOT NULL AND claimed_at < $1
`, staleBefore.UTC()); err != nil {
		return nil, fmt.Errorf("release stale voice cleanup claims: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
WITH candidates AS (
  SELECT id
  FROM tts_audio_cleanup_queue
  WHERE claimed_at IS NULL
  ORDER BY created_at ASC, id ASC
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
UPDATE tts_audio_cleanup_queue AS queue
SET claimed_at = now(), claim_id = $1, attempts = attempts + 1, updated_at = now()
FROM candidates
WHERE queue.id = candidates.id
RETURNING queue.id::text, queue.user_id::text, queue.file_id::text
`, claimID, limit)
	if err != nil {
		return nil, fmt.Errorf("claim voice artifact cleanup: %w", err)
	}
	claimed := make([]ClaimedArtifactCleanup, 0, limit)
	for rows.Next() {
		var item ClaimedArtifactCleanup
		if err := rows.Scan(&item.ID, &item.UserID, &item.FileID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan voice cleanup claim: %w", err)
		}
		claimed = append(claimed, item)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close voice cleanup claims: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit voice cleanup claim: %w", err)
	}
	return claimed, nil
}

func (r *PostgresSynthesisCacheRepository) CompleteArtifactCleanup(
	ctx context.Context,
	cleanupID string,
	claimID string,
) error {
	if r == nil || r.db == nil {
		return ErrVoiceCacheUnavailable
	}
	result, err := r.db.ExecContext(ctx, `
DELETE FROM tts_audio_cleanup_queue WHERE id = $1 AND claim_id = $2
`, cleanupID, claimID)
	if err != nil {
		return fmt.Errorf("complete voice artifact cleanup: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return ErrVoiceCacheUnavailable
	}
	return nil
}

func (r *PostgresSynthesisCacheRepository) ReleaseArtifactCleanup(
	ctx context.Context,
	cleanupID string,
	claimID string,
) error {
	if r == nil || r.db == nil {
		return ErrVoiceCacheUnavailable
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE tts_audio_cleanup_queue
SET claimed_at = NULL, claim_id = NULL, updated_at = now()
WHERE id = $1 AND claim_id = $2
`, cleanupID, claimID)
	if err != nil {
		return fmt.Errorf("release voice artifact cleanup: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return ErrVoiceCacheUnavailable
	}
	return nil
}

func enforceUserVoiceCacheLimitTx(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	currentFileID string,
	maxBytes int64,
) error {
	if maxBytes <= 0 {
		return ErrVoiceCacheUnavailable
	}
	var total int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(sum(byte_size), 0) FROM tts_audio_cache WHERE user_id = $1
`, userID).Scan(&total); err != nil {
		return fmt.Errorf("sum voice cache bytes: %w", err)
	}
	if total <= maxBytes {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id::text, file_id::text, byte_size
FROM tts_audio_cache
WHERE user_id = $1 AND file_id <> $2
ORDER BY last_accessed_at ASC, created_at ASC, id ASC
FOR UPDATE
`, userID, currentFileID)
	if err != nil {
		return fmt.Errorf("select voice cache eviction candidates: %w", err)
	}
	type victim struct {
		id     string
		fileID string
		size   int64
	}
	victims := make([]victim, 0)
	for rows.Next() && total > maxBytes {
		var item victim
		if err := rows.Scan(&item.id, &item.fileID, &item.size); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan voice cache eviction candidate: %w", err)
		}
		victims = append(victims, item)
		total -= item.size
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close voice cache eviction candidates: %w", err)
	}
	if total > maxBytes {
		return ErrVoiceCacheUnavailable
	}
	for _, item := range victims {
		if err := retireCachedArtifactTx(ctx, tx, item.id, userID, item.fileID, "lru"); err != nil {
			return err
		}
	}
	return nil
}

func retireCachedArtifactTx(
	ctx context.Context,
	tx *sql.Tx,
	cacheID string,
	userID string,
	fileID string,
	reason string,
) error {
	if err := queueArtifactCleanupTx(ctx, tx, userID, fileID, reason); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tts_audio_cache WHERE id = $1`, cacheID); err != nil {
		return fmt.Errorf("retire voice cache row: %w", err)
	}
	return nil
}

func queueArtifactCleanupTx(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	fileID string,
	reason string,
) error {
	cleanupID, err := newVoiceUUID()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO tts_audio_cleanup_queue (id, user_id, file_id, reason)
VALUES ($1, $2, $3, $4)
ON CONFLICT (file_id) DO NOTHING
`, cleanupID, userID, fileID, reason); err != nil {
		return fmt.Errorf("queue retired voice artifact: %w", err)
	}
	return nil
}

func isVoiceUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, current := range value {
		switch index {
		case 8, 13, 18, 23:
			if current != '-' {
				return false
			}
		default:
			if !((current >= '0' && current <= '9') ||
				(current >= 'a' && current <= 'f') ||
				(current >= 'A' && current <= 'F')) {
				return false
			}
		}
	}
	return true
}
