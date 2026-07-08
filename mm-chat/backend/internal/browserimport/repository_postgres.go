package browserimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const idempotencyReplayWaitTimeout = 5 * time.Second

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
		newID:  newUUID,
	}
}

func (r *PostgresRepository) Commit(ctx context.Context, pkg Package) (CommitResponse, error) {
	if err := r.requireDB(); err != nil {
		return CommitResponse{}, err
	}

	manifest := pkg.Manifest
	if existing, ok, err := r.findExistingBatch(ctx, manifest.IdempotencyKey); err != nil {
		return CommitResponse{}, err
	} else if ok {
		if existing.PackageHash == pkg.PackageHash && existing.ManifestHash == pkg.ManifestHash && existing.Status == "completed" {
			return existing.Response, nil
		}
		return CommitResponse{}, ErrIdempotencyConflict
	}

	batchID, err := r.generateID()
	if err != nil {
		return CommitResponse{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CommitResponse{}, fmt.Errorf("begin browser import: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := r.ensureDevUser(ctx, tx); err != nil {
		return CommitResponse{}, err
	}
	if _, err := tx.ExecContext(
		ctx,
		importBatchInsertSQL,
		batchID,
		r.userID,
		strings.TrimSpace(manifest.IdempotencyKey),
		pkg.PackageHash,
		pkg.ManifestHash,
	); err != nil {
		if isImportBatchIdempotencyConflict(err) {
			if existing, ok, replayErr := r.waitForExistingBatch(ctx, manifest.IdempotencyKey); replayErr != nil {
				return CommitResponse{}, replayErr
			} else if ok && existing.PackageHash == pkg.PackageHash && existing.ManifestHash == pkg.ManifestHash && existing.Status == "completed" {
				return existing.Response, nil
			}
			return CommitResponse{}, ErrIdempotencyConflict
		}
		return CommitResponse{}, fmt.Errorf("insert import batch: %w", err)
	}

	conversationMappings := make(map[string]string, len(manifest.Conversations))
	messageMappings := make(map[string]string, len(manifest.Messages))
	fileMappings := map[string]string{}

	for _, conversation := range manifest.Conversations {
		serverID, err := r.generateID()
		if err != nil {
			return CommitResponse{}, err
		}
		createdAt := parseOptionalTime(conversation.CreatedAt, fallbackImportTime(conversation.UpdatedAt))
		updatedAt := parseOptionalTime(conversation.UpdatedAt, createdAt)
		metadata := mergeImportMetadata(conversation.Config, map[string]any{
			"batchId":        batchID,
			"clientId":       conversation.ClientID,
			"idempotencyKey": manifest.IdempotencyKey,
			"source":         "browser-import",
		})
		metadata = addOptionalMetadata(metadata, "workspaceClientId", conversation.WorkspaceClientID)
		if conversation.Pinned {
			metadata["pinned"] = true
		}
		encodedMetadata, err := marshalJSONObject(metadata)
		if err != nil {
			return CommitResponse{}, err
		}

		modelProvider, modelID := modelRefParts(conversation.ModelRef)
		if _, err := tx.ExecContext(
			ctx,
			conversationImportInsertSQL,
			serverID,
			r.userID,
			conversation.Title,
			normalizeConversationStatus(conversation.Status),
			nullIfEmpty(modelProvider),
			nullIfEmpty(modelID),
			nullIfEmpty(conversation.SystemInstruction),
			importIdempotencyKey(batchID, "conversation", conversation.ClientID),
			string(encodedMetadata),
			createdAt,
			updatedAt,
		); err != nil {
			return CommitResponse{}, fmt.Errorf("insert imported conversation: %w", err)
		}
		conversationMappings[conversation.ClientID] = serverID
	}

	messages := append([]ImportMessage(nil), manifest.Messages...)
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].ConversationClientID == messages[j].ConversationClientID {
			return messages[i].SequenceNo < messages[j].SequenceNo
		}
		return messages[i].ConversationClientID < messages[j].ConversationClientID
	})
	for _, message := range messages {
		conversationID := conversationMappings[message.ConversationClientID]
		if conversationID == "" {
			return CommitResponse{}, newValidationError("INVALID_IMPORT_PAYLOAD", "message references an unknown conversation")
		}
		serverID, err := r.generateID()
		if err != nil {
			return CommitResponse{}, err
		}
		parentID := ""
		if message.ParentClientID != "" {
			parentID = messageMappings[message.ParentClientID]
			if parentID == "" {
				return CommitResponse{}, newValidationError("INVALID_MESSAGE_TREE", "parentClientId must reference an earlier message")
			}
		}
		createdAt := parseOptionalTime(message.CreatedAt, time.Now().UTC())
		completedAt := parseCompletedTime(message, createdAt)
		metadata := mergeImportMetadata(message.Metadata, map[string]any{
			"batchId":        batchID,
			"clientId":       message.ClientID,
			"idempotencyKey": manifest.IdempotencyKey,
			"source":         "browser-import",
		})
		if len(message.Attachments) > 0 {
			metadata["deferredAttachments"] = message.Attachments
		}
		encodedMetadata, err := marshalJSONObject(metadata)
		if err != nil {
			return CommitResponse{}, err
		}
		outputBlocks, err := marshalJSONArray(message.OutputBlocks)
		if err != nil {
			return CommitResponse{}, err
		}
		modelProvider, modelID := modelRefParts(message.ModelRef)
		if _, err := tx.ExecContext(
			ctx,
			messageImportInsertSQL,
			serverID,
			conversationID,
			r.userID,
			nullIfEmpty(parentID),
			message.SequenceNo,
			strings.ToLower(strings.TrimSpace(message.Role)),
			normalizeMessageStatus(message.Status),
			message.Content,
			nullIfEmpty(modelProvider),
			nullIfEmpty(modelID),
			importIdempotencyKey(batchID, "message", message.ClientID),
			string(outputBlocks),
			string(encodedMetadata),
			createdAt,
			createdAt,
			nullTime(completedAt),
		); err != nil {
			return CommitResponse{}, fmt.Errorf("insert imported message: %w", err)
		}
		messageMappings[message.ClientID] = serverID
	}

	response := CommitResponse{
		BatchID: batchID,
		Status:  "completed",
		Created: CreatedCounts{
			Conversations: len(conversationMappings),
			Messages:      len(messageMappings),
			Files:         0,
			Attachments:   0,
		},
		Mappings: ImportMappings{
			Conversations: conversationMappings,
			Messages:      messageMappings,
			Files:         fileMappings,
		},
		Warnings: pkg.Warnings,
	}
	encodedResponse, err := json.Marshal(response)
	if err != nil {
		return CommitResponse{}, fmt.Errorf("encode import response: %w", err)
	}
	if _, err := tx.ExecContext(ctx, importBatchCompleteSQL, string(encodedResponse), batchID, r.userID); err != nil {
		return CommitResponse{}, fmt.Errorf("complete import batch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CommitResponse{}, fmt.Errorf("commit browser import: %w", err)
	}

	return response, nil
}

func (r *PostgresRepository) GetBatchStatus(ctx context.Context, batchID string) (BatchStatusResponse, error) {
	if err := r.requireDB(); err != nil {
		return BatchStatusResponse{}, err
	}
	var status BatchStatusResponse
	var createdAt time.Time
	err := r.db.QueryRowContext(ctx, `
SELECT id, status, created_at
FROM import_batches
WHERE id = $1
  AND user_id = $2
`, batchID, r.userID).Scan(&status.BatchID, &status.Status, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BatchStatusResponse{}, ErrBatchNotFound
	}
	if err != nil {
		return BatchStatusResponse{}, fmt.Errorf("query import batch: %w", err)
	}
	status.CreatedAt = formatTime(createdAt)
	return status, nil
}

func (r *PostgresRepository) Rollback(ctx context.Context, batchID string) error {
	if err := r.requireDB(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import rollback: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var status string
	var completedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
SELECT status, completed_at
FROM import_batches
WHERE id = $1
  AND user_id = $2
FOR UPDATE
`, batchID, r.userID).Scan(&status, &completedAt); errors.Is(err, sql.ErrNoRows) {
		return ErrBatchNotFound
	} else if err != nil {
		return fmt.Errorf("query import batch for rollback: %w", err)
	}
	if status == "rolled_back" {
		return tx.Commit()
	}
	if status != "completed" || !completedAt.Valid {
		return ErrBatchModified
	}

	var modifiedCount int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM (
  SELECT updated_at
  FROM conversations
  WHERE user_id = $1
    AND metadata #>> '{import,batchId}' = $2
    AND deleted_at IS NULL
  UNION ALL
  SELECT updated_at
  FROM messages
  WHERE user_id = $1
    AND metadata #>> '{import,batchId}' = $2
    AND deleted_at IS NULL
) imported_rows
WHERE updated_at > $3
`, r.userID, batchID, completedAt.Time).Scan(&modifiedCount); err != nil {
		return fmt.Errorf("query modified import rows: %w", err)
	}
	if modifiedCount > 0 {
		return ErrBatchModified
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE messages
SET deleted_at = now(), updated_at = now()
WHERE user_id = $1
  AND metadata #>> '{import,batchId}' = $2
  AND deleted_at IS NULL
`, r.userID, batchID); err != nil {
		return fmt.Errorf("soft delete imported messages: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE conversations
SET status = 'deleted', deleted_at = now(), updated_at = now()
WHERE user_id = $1
  AND metadata #>> '{import,batchId}' = $2
  AND deleted_at IS NULL
`, r.userID, batchID); err != nil {
		return fmt.Errorf("soft delete imported conversations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE import_batches
SET status = 'rolled_back', rolled_back_at = now(), updated_at = now()
WHERE id = $1
  AND user_id = $2
`, batchID, r.userID); err != nil {
		return fmt.Errorf("mark import batch rolled back: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import rollback: %w", err)
	}
	return nil
}

func (r *PostgresRepository) findExistingBatch(ctx context.Context, idempotencyKey string) (existingImportBatch, bool, error) {
	var existing existingImportBatch
	var response []byte
	err := r.db.QueryRowContext(ctx, `
SELECT package_hash, manifest_hash, status, response
FROM import_batches
WHERE user_id = $1
  AND idempotency_key = $2
`, r.userID, strings.TrimSpace(idempotencyKey)).Scan(&existing.PackageHash, &existing.ManifestHash, &existing.Status, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return existingImportBatch{}, false, nil
	}
	if err != nil {
		return existingImportBatch{}, false, fmt.Errorf("query import idempotency: %w", err)
	}
	if len(response) > 0 {
		if err := json.Unmarshal(response, &existing.Response); err != nil {
			return existingImportBatch{}, false, fmt.Errorf("decode import response: %w", err)
		}
	}
	return existing, true, nil
}

func (r *PostgresRepository) waitForExistingBatch(ctx context.Context, idempotencyKey string) (existingImportBatch, bool, error) {
	deadline := time.Now().Add(idempotencyReplayWaitTimeout)
	for {
		existing, ok, err := r.findExistingBatch(ctx, idempotencyKey)
		if err != nil {
			return existingImportBatch{}, false, err
		}
		if ok && existing.Status != "pending" {
			return existing, true, nil
		}
		if time.Now().After(deadline) {
			return existing, ok, nil
		}
		select {
		case <-ctx.Done():
			return existingImportBatch{}, false, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

type existingImportBatch struct {
	PackageHash  string
	ManifestHash string
	Status       string
	Response     CommitResponse
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func (r *PostgresRepository) generateID() (string, error) {
	if r.newID == nil {
		return newUUID()
	}
	return r.newID()
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

func modelRefParts(modelRef *ModelRef) (string, string) {
	if modelRef == nil {
		return "", ""
	}
	return strings.TrimSpace(modelRef.ProviderID), strings.TrimSpace(modelRef.ModelID)
}

func fallbackImportTime(value string) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}

func parseOptionalTime(value string, fallback time.Time) time.Time {
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
		return parsed.UTC()
	}
	return fallback.UTC()
}

func parseCompletedTime(message ImportMessage, fallback time.Time) time.Time {
	if message.CompletedAt != "" {
		return parseOptionalTime(message.CompletedAt, fallback)
	}
	if normalizeMessageStatus(message.Status) == "completed" {
		return fallback.UTC()
	}
	return time.Time{}
}

func mergeImportMetadata(original map[string]any, importInfo map[string]any) map[string]any {
	metadata := map[string]any{}
	for key, value := range original {
		metadata[key] = value
	}
	metadata["import"] = importInfo
	return metadata
}

func addOptionalMetadata(metadata map[string]any, key string, value string) map[string]any {
	value = strings.TrimSpace(value)
	if value != "" {
		metadata[key] = value
	}
	return metadata
}

func importIdempotencyKey(batchID string, resourceType string, clientID string) string {
	return "import:" + batchID + ":" + resourceType + ":" + clientID
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

func marshalJSONArray(value []any) ([]byte, error) {
	if value == nil {
		value = []any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode output blocks: %w", err)
	}
	return encoded, nil
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func isImportBatchIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_import_batches_user_idempotency"
}

const importBatchInsertSQL = `
INSERT INTO import_batches (
  id,
  user_id,
  idempotency_key,
  package_hash,
  manifest_hash,
  status,
  response
) VALUES ($1, $2, $3, $4, $5, 'pending', '{}'::jsonb)
`

const importBatchCompleteSQL = `
UPDATE import_batches
SET status = 'completed', response = $1::jsonb, completed_at = now(), updated_at = now()
WHERE id = $2
  AND user_id = $3
`

const conversationImportInsertSQL = `
INSERT INTO conversations (
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
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11)
`

const messageImportInsertSQL = `
INSERT INTO messages (
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
  idempotency_key,
  output_blocks,
  metadata,
  created_at,
  updated_at,
  completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb, $14, $15, $16)
`
