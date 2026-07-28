package usermemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/auth"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) GetSettings(ctx context.Context) (Settings, bool, error) {
	if err := r.requireDB(); err != nil {
		return Settings{}, false, err
	}
	user := auth.UserOrDevelopment(ctx)
	var settings Settings
	err := r.db.QueryRowContext(ctx, `
SELECT enabled, search_enabled, auto_record_enabled
FROM user_memory_settings
WHERE user_id = $1
`, user.ID).Scan(
		&settings.Enabled,
		&settings.SearchEnabled,
		&settings.AutoRecordEnabled,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, fmt.Errorf("get user memory settings: %w", err)
	}
	return settings, true, nil
}

func (r *PostgresRepository) UpsertSettings(ctx context.Context, input Settings) (Settings, error) {
	if err := r.requireDB(); err != nil {
		return Settings{}, err
	}
	user := auth.UserOrDevelopment(ctx)
	var settings Settings
	err := r.db.QueryRowContext(ctx, `
INSERT INTO user_memory_settings (
  user_id, enabled, search_enabled, auto_record_enabled
)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE SET
  enabled = EXCLUDED.enabled,
  search_enabled = EXCLUDED.search_enabled,
  auto_record_enabled = EXCLUDED.auto_record_enabled,
  updated_at = now()
RETURNING enabled, search_enabled, auto_record_enabled
`,
		user.ID,
		input.Enabled,
		input.SearchEnabled,
		input.AutoRecordEnabled,
	).Scan(
		&settings.Enabled,
		&settings.SearchEnabled,
		&settings.AutoRecordEnabled,
	)
	if err != nil {
		return Settings{}, fmt.Errorf("upsert user memory settings: %w", err)
	}
	return settings, nil
}

func (r *PostgresRepository) List(ctx context.Context) ([]Memory, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	user := auth.UserOrDevelopment(ctx)
	rows, err := r.db.QueryContext(ctx, `
SELECT
  id, user_id, memory_type, content, normalized_content, importance,
  to_json(tags)::text AS tags,
  source, source_conversation_id, source_message_id, enabled, last_used_at,
  created_at, updated_at, deleted_at
FROM user_memories
WHERE user_id = $1 AND scope_type = 'global' AND deleted_at IS NULL
ORDER BY updated_at DESC, id
LIMIT $2
`, user.ID, MaxMemories)
	if err != nil {
		return nil, fmt.Errorf("list user memories: %w", err)
	}
	defer rows.Close()

	memories := make([]Memory, 0)
	for rows.Next() {
		memory, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user memory: %w", err)
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user memories: %w", err)
	}
	return memories, nil
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (Memory, error) {
	if err := r.requireDB(); err != nil {
		return Memory{}, err
	}
	user := auth.UserOrDevelopment(ctx)
	row := r.db.QueryRowContext(ctx, `
INSERT INTO user_memories (
  id, user_id, memory_type, content, normalized_content, importance, tags,
  source, source_conversation_id, source_message_id, enabled, scope_type
)
VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8,
  NULLIF($9, '')::uuid, NULLIF($10, '')::uuid, $11, 'global'
)
ON CONFLICT (user_id, normalized_content)
WHERE deleted_at IS NULL AND scope_type = 'global' DO UPDATE SET
  memory_type = EXCLUDED.memory_type,
  content = EXCLUDED.content,
  importance = GREATEST(user_memories.importance, EXCLUDED.importance),
  tags = EXCLUDED.tags,
  source = EXCLUDED.source,
  source_conversation_id = EXCLUDED.source_conversation_id,
  source_message_id = EXCLUDED.source_message_id,
  enabled = true,
  updated_at = now()
RETURNING
  id, user_id, memory_type, content, normalized_content, importance,
  to_json(tags)::text AS tags,
  source, source_conversation_id, source_message_id, enabled, last_used_at,
  created_at, updated_at, deleted_at
`,
		input.ID,
		user.ID,
		input.Type,
		input.Content,
		input.NormalizedContent,
		input.Importance,
		input.Tags,
		input.Source,
		input.SourceConversationID,
		input.SourceMessageID,
		input.Enabled,
	)
	memory, err := scanMemory(row)
	if err != nil {
		return Memory{}, fmt.Errorf("create user memory: %w", err)
	}
	return memory, nil
}

func (r *PostgresRepository) Update(ctx context.Context, memoryID string, input UpdateInput) (Memory, error) {
	if err := r.requireDB(); err != nil {
		return Memory{}, err
	}
	user := auth.UserOrDevelopment(ctx)
	row := r.db.QueryRowContext(ctx, `
UPDATE user_memories
SET
  memory_type = $3,
  content = $4,
  normalized_content = $5,
  importance = $6,
  tags = $7,
  enabled = $8,
  updated_at = now()
WHERE id = $1 AND user_id = $2 AND scope_type = 'global' AND deleted_at IS NULL
RETURNING
  id, user_id, memory_type, content, normalized_content, importance,
  to_json(tags)::text AS tags,
  source, source_conversation_id, source_message_id, enabled, last_used_at,
  created_at, updated_at, deleted_at
`,
		memoryID,
		user.ID,
		input.Type,
		input.Content,
		input.NormalizedContent,
		input.Importance,
		input.Tags,
		input.Enabled,
	)
	memory, err := scanMemory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Memory{}, ErrMemoryNotFound
	}
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return Memory{}, ErrMemoryConflict
		}
		return Memory{}, fmt.Errorf("update user memory: %w", err)
	}
	return memory, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, memoryID string) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	user := auth.UserOrDevelopment(ctx)
	result, err := r.db.ExecContext(ctx, `
UPDATE user_memories
SET enabled = false, deleted_at = now(), updated_at = now()
WHERE id = $1 AND user_id = $2 AND scope_type = 'global' AND deleted_at IS NULL
`, memoryID, user.ID)
	if err != nil {
		return fmt.Errorf("delete user memory: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted user memory count: %w", err)
	}
	if count == 0 {
		return ErrMemoryNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkUsed(ctx context.Context, ids []string, usedAt time.Time) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	user := auth.UserOrDevelopment(ctx)
	_, err := r.db.ExecContext(ctx, `
UPDATE user_memories
SET last_used_at = $3
WHERE user_id = $1
  AND id = ANY($2::uuid[])
  AND scope_type = 'global'
  AND deleted_at IS NULL
  AND enabled
`, user.ID, ids, usedAt.UTC())
	if err != nil {
		return fmt.Errorf("mark user memories used: %w", err)
	}
	return nil
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	return nil
}

type memoryScanner interface {
	Scan(...any) error
}

func scanMemory(scanner memoryScanner) (Memory, error) {
	var memory Memory
	var tagsJSON string
	var conversationID sql.NullString
	var messageID sql.NullString
	var lastUsedAt sql.NullTime
	var deletedAt sql.NullTime
	err := scanner.Scan(
		&memory.ID,
		&memory.UserID,
		&memory.Type,
		&memory.Content,
		&memory.NormalizedContent,
		&memory.Importance,
		&tagsJSON,
		&memory.Source,
		&conversationID,
		&messageID,
		&memory.Enabled,
		&lastUsedAt,
		&memory.CreatedAt,
		&memory.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return Memory{}, err
	}
	if err := json.Unmarshal([]byte(tagsJSON), &memory.Tags); err != nil {
		return Memory{}, fmt.Errorf("decode memory tags: %w", err)
	}
	memory.SourceConversationID = strings.TrimSpace(conversationID.String)
	memory.SourceMessageID = strings.TrimSpace(messageID.String)
	if lastUsedAt.Valid {
		value := lastUsedAt.Time.UTC()
		memory.LastUsedAt = &value
	}
	if deletedAt.Valid {
		value := deletedAt.Time.UTC()
		memory.DeletedAt = &value
	}
	if memory.Tags == nil {
		memory.Tags = []string{}
	}
	return memory, nil
}
