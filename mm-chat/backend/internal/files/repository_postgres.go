package files

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

type PostgresRepository struct {
	db *sql.DB
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{
		db: db,
	}
}

func (r *PostgresRepository) CreateFile(ctx context.Context, input CreateFileInput) (FileRecord, error) {
	if err := r.requireDB(); err != nil {
		return FileRecord{}, err
	}
	input.ID = strings.TrimSpace(input.ID)
	if !isUUID(input.ID) {
		return FileRecord{}, newValidationError("INVALID_FILE_ID", "file id must be a UUID")
	}
	input.OriginalFilename = safeDisplayFilename(input.OriginalFilename)
	input.MimeType = normalizeMimeType(input.MimeType)
	input.StorageBackend = strings.ToLower(strings.TrimSpace(input.StorageBackend))
	if input.StorageBackend == "" {
		input.StorageBackend = DefaultStorageBackend
	}
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)
	if input.ObjectKey == "" {
		return FileRecord{}, newValidationError("INVALID_OBJECT_KEY", "object key is required")
	}
	metadata, err := marshalJSONObject(input.Metadata)
	if err != nil {
		return FileRecord{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return FileRecord{}, fmt.Errorf("begin create file: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	user := auth.UserOrDevelopment(ctx)
	if err := r.ensureUser(ctx, tx, user); err != nil {
		return FileRecord{}, err
	}

	record, err := scanFile(tx.QueryRowContext(
		ctx,
		fileInsertSQL,
		input.ID,
		user.ID,
		input.OriginalFilename,
		input.MimeType,
		input.ByteSize,
		input.SHA256,
		input.StorageBackend,
		input.ObjectKey,
		string(metadata),
	))
	if err != nil {
		return FileRecord{}, fmt.Errorf("insert file metadata: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return FileRecord{}, fmt.Errorf("commit create file: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) GetFile(ctx context.Context, fileID string) (FileRecord, error) {
	if err := r.requireDB(); err != nil {
		return FileRecord{}, err
	}
	fileID = strings.TrimSpace(fileID)
	if !isUUID(fileID) {
		return FileRecord{}, newValidationError("INVALID_FILE_ID", "file id must be a UUID")
	}

	userID := auth.UserOrDevelopment(ctx).ID
	record, err := scanFile(r.db.QueryRowContext(ctx, fileByIDSQL, fileID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return FileRecord{}, ErrFileNotFound
	}
	if err != nil {
		return FileRecord{}, fmt.Errorf("query file metadata: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) MarkFileDeleted(ctx context.Context, fileID string) (FileRecord, error) {
	if err := r.requireDB(); err != nil {
		return FileRecord{}, err
	}
	fileID = strings.TrimSpace(fileID)
	if !isUUID(fileID) {
		return FileRecord{}, newValidationError("INVALID_FILE_ID", "file id must be a UUID")
	}

	userID := auth.UserOrDevelopment(ctx).ID
	record, err := scanFile(r.db.QueryRowContext(ctx, fileMarkDeletedSQL, fileID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return FileRecord{}, ErrFileNotFound
	}
	if err != nil {
		return FileRecord{}, fmt.Errorf("mark file deleted: %w", err)
	}

	return record, nil
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	return nil
}

func (r *PostgresRepository) ensureUser(ctx context.Context, execer sqlExecer, user auth.User) error {
	user = auth.UserOrDevelopment(auth.WithUser(context.Background(), user))
	_, err := execer.ExecContext(ctx, `
INSERT INTO users (id, display_name)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING
`, user.ID, user.DisplayName)
	if err != nil {
		return fmt.Errorf("ensure request user: %w", err)
	}

	return nil
}

const fileSelectSQL = `
SELECT
  id,
  user_id,
  original_filename,
  mime_type,
  byte_size,
  sha256,
  storage_backend,
  object_key,
  upload_status,
  metadata,
  created_at,
  updated_at,
  deleted_at
FROM files
`

const fileInsertSQL = `
INSERT INTO files (
  id,
  user_id,
  original_filename,
  mime_type,
  byte_size,
  sha256,
  storage_backend,
  object_key,
  upload_status,
  metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'available', $9::jsonb)
RETURNING
  id,
  user_id,
  original_filename,
  mime_type,
  byte_size,
  sha256,
  storage_backend,
  object_key,
  upload_status,
  metadata,
  created_at,
  updated_at,
  deleted_at
`

const fileByIDSQL = fileSelectSQL + `
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
  AND upload_status = 'available'
`

const fileMarkDeletedSQL = `
UPDATE files
SET
  upload_status = 'deleted',
  deleted_at = now(),
  updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL
RETURNING
  id,
  user_id,
  original_filename,
  mime_type,
  byte_size,
  sha256,
  storage_backend,
  object_key,
  upload_status,
  metadata,
  created_at,
  updated_at,
  deleted_at
`

func scanFile(scanner rowScanner) (FileRecord, error) {
	var record FileRecord
	var metadataBytes []byte
	var deletedAt sql.NullTime

	err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&record.OriginalFilename,
		&record.MimeType,
		&record.ByteSize,
		&record.SHA256,
		&record.StorageBackend,
		&record.ObjectKey,
		&record.UploadStatus,
		&metadataBytes,
		&record.CreatedAt,
		&record.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return FileRecord{}, err
	}

	metadata := map[string]any{}
	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			return FileRecord{}, fmt.Errorf("decode file metadata: %w", err)
		}
	}
	record.Metadata = metadata
	if deletedAt.Valid {
		value := deletedAt.Time
		record.DeletedAt = &value
	}

	return record, nil
}

func marshalJSONObject(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, fmt.Errorf("validate metadata: %w", err)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return nil, newValidationError("INVALID_METADATA", "metadata must be an object")
	}
	return payload, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
