package ragsource

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type Repository interface {
	FetchParseSourceMetadata(ctx context.Context, input SourceObjectInput) (SourceMetadata, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) FetchParseSourceMetadata(ctx context.Context, input SourceObjectInput) (SourceMetadata, error) {
	if r == nil || r.db == nil {
		return SourceMetadata{}, ErrServiceUnavailable
	}
	input, err := normalizeInput(input)
	if err != nil {
		return SourceMetadata{}, err
	}
	var metadata SourceMetadata
	err = r.db.QueryRowContext(ctx, `
SELECT file_id, storage_backend, object_key, sha256, byte_size, content_type
FROM knowledge_fetch_parse_source_metadata($1, $2, $3, $4, $5)
`, input.JobID, input.WorkerID, input.LeaseToken, input.FileID, input.MaterializationID).Scan(
		&metadata.FileID,
		&metadata.StorageBackend,
		&metadata.ObjectKey,
		&metadata.SHA256,
		&metadata.ByteSize,
		&metadata.ContentType,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SourceMetadata{}, ErrSourceUnavailable
	}
	if err != nil {
		return SourceMetadata{}, fmt.Errorf("fetch parse source metadata: %w", err)
	}
	metadata, err = normalizeMetadata(metadata)
	if err != nil {
		return SourceMetadata{}, err
	}
	return metadata, nil
}
