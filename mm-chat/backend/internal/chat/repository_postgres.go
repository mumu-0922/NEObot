package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type PostgresRepository struct {
	db     *sql.DB
	userID string
	newID  func() (string, error)
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{
		db:     db,
		userID: DevUserID,
		newID:  NewUUID,
	}
}

func (r *PostgresRepository) CreateConversation(
	ctx context.Context,
	input CreateConversationInput,
) (Conversation, error) {
	if err := r.requireDB(); err != nil {
		return Conversation{}, err
	}
	if err := r.ensureDevUser(ctx, r.db); err != nil {
		return Conversation{}, err
	}

	id, err := r.generateID()
	if err != nil {
		return Conversation{}, err
	}
	metadata, err := marshalJSONObject(input.Metadata)
	if err != nil {
		return Conversation{}, err
	}

	row := r.db.QueryRowContext(ctx, `
INSERT INTO conversations (
  id,
  user_id,
  title,
  model_provider,
  model_id,
  system_prompt,
  idempotency_key,
  metadata
) VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8::jsonb)
RETURNING
  id,
  user_id,
  title,
  status,
  model_provider,
  model_id,
  system_prompt,
  idempotency_key,
  metadata,
  created_at,
  updated_at,
  deleted_at,
  0::bigint AS message_count
`, id, r.userID, input.Title, input.ModelProvider, input.ModelID, input.SystemPrompt, input.IdempotencyKey, string(metadata))

	conversation, err := scanConversation(row)
	if err != nil {
		if isIdempotencyConflict(err, input.IdempotencyKey, "idx_conversations_user_idempotency") {
			return Conversation{}, ErrIdempotencyConflict
		}
		return Conversation{}, fmt.Errorf("insert conversation: %w", err)
	}

	return conversation, nil
}

func (r *PostgresRepository) ListConversations(ctx context.Context) ([]Conversation, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
  c.id,
  c.user_id,
  c.title,
  c.status,
  c.model_provider,
  c.model_id,
  c.system_prompt,
  c.idempotency_key,
  c.metadata,
  c.created_at,
  c.updated_at,
  c.deleted_at,
  COALESCE(message_counts.message_count, 0)::bigint AS message_count
FROM conversations c
LEFT JOIN (
  SELECT conversation_id, COUNT(*) AS message_count
  FROM messages
  WHERE deleted_at IS NULL
  GROUP BY conversation_id
) message_counts ON message_counts.conversation_id = c.id
WHERE c.user_id = $1
  AND c.deleted_at IS NULL
ORDER BY c.updated_at DESC, c.created_at DESC, c.id DESC
`, r.userID)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	conversations := []Conversation{}
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, conversation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}

	return conversations, nil
}

func (r *PostgresRepository) ListMessages(
	ctx context.Context,
	conversationID string,
) ([]Message, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	if !isUUID(conversationID) {
		return nil, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}
	if err := r.requireConversation(ctx, conversationID); err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, messageSelectSQL+`
WHERE conversation_id = $1
  AND deleted_at IS NULL
ORDER BY sequence_no ASC, created_at ASC, id ASC
`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	messages := []Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return messages, nil
}

func (r *PostgresRepository) CreateMessage(
	ctx context.Context,
	conversationID string,
	input CreateMessageInput,
) (Message, error) {
	if err := r.requireDB(); err != nil {
		return Message{}, err
	}
	if !isUUID(conversationID) {
		return Message{}, newValidationError("INVALID_CONVERSATION_ID", "conversation id must be a UUID")
	}
	role, err := normalizeClientMessageRole(input.Role)
	if err != nil {
		return Message{}, err
	}
	input.Role = role
	if strings.TrimSpace(input.Content) == "" {
		return Message{}, newValidationError("EMPTY_CONTENT", "message content is required")
	}
	input.ParentMessageID = strings.TrimSpace(input.ParentMessageID)
	if input.ParentMessageID != "" && !isUUID(input.ParentMessageID) {
		return Message{}, newValidationError("INVALID_PARENT_MESSAGE_ID", "parent message id must be a UUID")
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)

	id, err := r.generateID()
	if err != nil {
		return Message{}, err
	}
	metadata, err := marshalJSONObject(input.Metadata)
	if err != nil {
		return Message{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, fmt.Errorf("begin create message: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := r.ensureDevUser(ctx, tx); err != nil {
		return Message{}, err
	}
	if err := lockConversationForUser(ctx, tx, conversationID, r.userID); err != nil {
		return Message{}, err
	}

	var nextSequence int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(sequence_no) + 1, 0) FROM messages WHERE conversation_id = $1`,
		conversationID,
	).Scan(&nextSequence); err != nil {
		return Message{}, fmt.Errorf("calculate next message sequence: %w", err)
	}

	row := tx.QueryRowContext(ctx, messageInsertSQL, id, conversationID, r.userID, nullIfEmpty(input.ParentMessageID), nextSequence, input.Role, input.Content, nullIfEmpty(input.IdempotencyKey), string(metadata))
	message, err := scanMessage(row)
	if err != nil {
		if isIdempotencyConflict(err, input.IdempotencyKey, "idx_messages_conversation_idempotency") {
			return Message{}, ErrIdempotencyConflict
		}
		return Message{}, fmt.Errorf("insert message: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE conversations SET updated_at = now() WHERE id = $1 AND user_id = $2`,
		conversationID,
		r.userID,
	); err != nil {
		return Message{}, fmt.Errorf("touch conversation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Message{}, fmt.Errorf("commit create message: %w", err)
	}

	return message, nil
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}

	return nil
}

func (r *PostgresRepository) generateID() (string, error) {
	newID := r.newID
	if newID == nil {
		newID = NewUUID
	}

	return newID()
}

func (r *PostgresRepository) ensureDevUser(ctx context.Context, execer sqlExecer) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO users (id, display_name)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING
`, r.userID, "Development User")
	if err != nil {
		return fmt.Errorf("ensure dev user: %w", err)
	}

	return nil
}

func (r *PostgresRepository) requireConversation(ctx context.Context, conversationID string) error {
	var id string
	err := r.db.QueryRowContext(ctx, `
SELECT id
FROM conversations
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
`, conversationID, r.userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConversationNotFound
	}
	if err != nil {
		return fmt.Errorf("query conversation ownership: %w", err)
	}

	return nil
}

func lockConversationForUser(
	ctx context.Context,
	tx *sql.Tx,
	conversationID string,
	userID string,
) error {
	var id string
	err := tx.QueryRowContext(ctx, `
SELECT id
FROM conversations
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
FOR UPDATE
`, conversationID, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConversationNotFound
	}
	if err != nil {
		return fmt.Errorf("lock conversation: %w", err)
	}

	return nil
}

const messageSelectSQL = `
SELECT
  id,
  conversation_id,
  user_id,
  parent_message_id,
  sequence_no,
  role,
  status,
  content,
  model_provider,
  model_id,
  provider_message_id,
  idempotency_key,
  output_blocks,
  metadata,
  created_at,
  updated_at,
  completed_at,
  deleted_at
FROM messages
`

const messageInsertSQL = `
INSERT INTO messages (
  id,
  conversation_id,
  user_id,
  parent_message_id,
  sequence_no,
  role,
  status,
  content,
  idempotency_key,
  output_blocks,
  metadata,
  completed_at
) VALUES ($1, $2, $3, $4, $5, $6, 'completed', $7, $8, '[]'::jsonb, $9::jsonb, now())
RETURNING
  id,
  conversation_id,
  user_id,
  parent_message_id,
  sequence_no,
  role,
  status,
  content,
  model_provider,
  model_id,
  provider_message_id,
  idempotency_key,
  output_blocks,
  metadata,
  created_at,
  updated_at,
  completed_at,
  deleted_at
`

func scanConversation(scanner rowScanner) (Conversation, error) {
	var conversation Conversation
	var modelProvider sql.NullString
	var modelID sql.NullString
	var systemPrompt sql.NullString
	var idempotencyKey sql.NullString
	var metadata []byte
	var deletedAt sql.NullTime
	var messageCount int64

	if err := scanner.Scan(
		&conversation.ID,
		&conversation.UserID,
		&conversation.Title,
		&conversation.Status,
		&modelProvider,
		&modelID,
		&systemPrompt,
		&idempotencyKey,
		&metadata,
		&conversation.CreatedAt,
		&conversation.UpdatedAt,
		&deletedAt,
		&messageCount,
	); err != nil {
		return Conversation{}, err
	}

	conversation.ModelProvider = modelProvider.String
	conversation.ModelID = modelID.String
	conversation.SystemPrompt = systemPrompt.String
	conversation.IdempotencyKey = idempotencyKey.String
	conversation.MessageCount = int(messageCount)
	if deletedAt.Valid {
		conversation.DeletedAt = &deletedAt.Time
	}

	decoded, err := unmarshalJSONObject(metadata)
	if err != nil {
		return Conversation{}, err
	}
	conversation.Metadata = decoded

	return conversation, nil
}

func scanMessage(scanner rowScanner) (Message, error) {
	var message Message
	var userID sql.NullString
	var parentMessageID sql.NullString
	var modelProvider sql.NullString
	var modelID sql.NullString
	var providerMessageID sql.NullString
	var idempotencyKey sql.NullString
	var outputBlocks []byte
	var metadata []byte
	var completedAt sql.NullTime
	var deletedAt sql.NullTime

	if err := scanner.Scan(
		&message.ID,
		&message.ConversationID,
		&userID,
		&parentMessageID,
		&message.SequenceNo,
		&message.Role,
		&message.Status,
		&message.Content,
		&modelProvider,
		&modelID,
		&providerMessageID,
		&idempotencyKey,
		&outputBlocks,
		&metadata,
		&message.CreatedAt,
		&message.UpdatedAt,
		&completedAt,
		&deletedAt,
	); err != nil {
		return Message{}, err
	}

	message.UserID = userID.String
	message.ParentMessageID = parentMessageID.String
	message.ModelProvider = modelProvider.String
	message.ModelID = modelID.String
	message.ProviderMessageID = providerMessageID.String
	message.IdempotencyKey = idempotencyKey.String
	if completedAt.Valid {
		message.CompletedAt = &completedAt.Time
	}
	if deletedAt.Valid {
		message.DeletedAt = &deletedAt.Time
	}

	decodedOutputBlocks, err := unmarshalJSONArray(outputBlocks)
	if err != nil {
		return Message{}, err
	}
	message.OutputBlocks = decodedOutputBlocks

	decodedMetadata, err := unmarshalJSONObject(metadata)
	if err != nil {
		return Message{}, err
	}
	message.Metadata = decodedMetadata

	return message, nil
}

func marshalJSONObject(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}

	return encoded, nil
}

func unmarshalJSONObject(value []byte) (map[string]any, error) {
	if len(value) == 0 {
		return map[string]any{}, nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}

	return decoded, nil
}

func unmarshalJSONArray(value []byte) ([]any, error) {
	if len(value) == 0 {
		return []any{}, nil
	}

	var decoded []any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, fmt.Errorf("decode output blocks: %w", err)
	}
	if decoded == nil {
		return []any{}, nil
	}

	return decoded, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func isIdempotencyConflict(err error, idempotencyKey string, constraintNames ...string) bool {
	if strings.TrimSpace(idempotencyKey) == "" {
		return false
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	for _, name := range constraintNames {
		if pgErr.ConstraintName == name {
			return true
		}
	}

	return false
}
