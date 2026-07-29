package memorycapture

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/usermemory"
)

const maximumEmbeddingBatch = 32

type PassageEmbedder interface {
	EmbedSiliconFlowPassages(
		context.Context,
		ragproviders.PassageEmbeddingRequest,
	) (ragproviders.PassageEmbeddingResponse, error)
}

// PopulateProjectionVectors builds the synthetic fixture's derived vector
// projection through the fixed production Provider request contract. Direct
// projection writes are restricted to the ephemeral seed stage; query-time
// capture still uses the normal API capability.
func PopulateProjectionVectors(
	ctx context.Context,
	db *sql.DB,
	runID string,
	embedder PassageEmbedder,
) (int, error) {
	if db == nil || embedder == nil || !runIDPattern.MatchString(runID) {
		return 0, ErrCaptureInvalid
	}
	if err := verifySeedDatabase(ctx, db, runID); err != nil {
		return 0, err
	}
	rows, err := db.QueryContext(ctx, `
SELECT projection.memory_id::text, memory.content
FROM user_memory_search_projections projection
JOIN user_memories memory
  ON memory.id = projection.memory_id AND memory.user_id = projection.user_id
WHERE projection.lexical_status = 'ready'
  AND projection.embedding_status = 'pending'
ORDER BY projection.memory_id
`)
	if err != nil {
		return 0, errors.New("list Memory regression projection inputs")
	}
	type input struct{ id, text string }
	inputs := make([]input, 0)
	for rows.Next() {
		var item input
		if err := rows.Scan(&item.id, &item.text); err != nil {
			_ = rows.Close()
			return 0, errors.New("scan Memory regression projection input")
		}
		item.text = usermemory.RedactMemoryProviderText(item.text, true)
		if strings.TrimSpace(item.text) == "" {
			_ = rows.Close()
			return 0, fmt.Errorf("%w: fixture Memory redacted before embedding", ErrCaptureInvalid)
		}
		inputs = append(inputs, item)
	}
	if err := rows.Close(); err != nil || len(inputs) == 0 {
		return 0, errors.New("finish Memory regression projection input scan")
	}

	completed := 0
	for start := 0; start < len(inputs); start += maximumEmbeddingBatch {
		end := min(start+maximumEmbeddingBatch, len(inputs))
		request := ragproviders.PassageEmbeddingRequest{
			Passages: make([]ragproviders.PassageEmbeddingInput, end-start),
		}
		for index, item := range inputs[start:end] {
			request.Passages[index] = ragproviders.PassageEmbeddingInput{
				PassageID: item.id, Text: item.text,
			}
		}
		response, err := embedder.EmbedSiliconFlowPassages(ctx, request)
		if err != nil {
			return completed, errors.New("Memory regression fixture embedding failed")
		}
		if response.Model != ragproviders.SiliconFlowEmbeddingModel ||
			response.Dimensions != ragproviders.SiliconFlowEmbeddingDimensions ||
			len(response.Vectors) != len(request.Passages) {
			return completed, fmt.Errorf("%w: fixture embedding response profile", ErrCaptureStateConflict)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return completed, errors.New("begin Memory regression vector publish")
		}
		for index, vector := range response.Vectors {
			expected := request.Passages[index].PassageID
			if vector.PassageID != expected || !validVector(vector.Embedding) {
				_ = tx.Rollback()
				return completed, fmt.Errorf("%w: fixture embedding vector", ErrCaptureStateConflict)
			}
			result, err := tx.ExecContext(ctx, `
UPDATE user_memory_search_projections
SET embedding_status = 'ready',
    embedding_vector = $2::real[]::vector(1024),
    embedding_error_code = NULL,
    embedding_updated_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE memory_id = $1::uuid AND embedding_status = 'pending'
`, vector.PassageID, realArrayLiteral(vector.Embedding))
			if err != nil {
				_ = tx.Rollback()
				return completed, errors.New("publish Memory regression fixture vector")
			}
			affected, affectedErr := result.RowsAffected()
			if affectedErr != nil || affected != 1 {
				_ = tx.Rollback()
				return completed, fmt.Errorf("%w: fixture projection was not pending", ErrCaptureStateConflict)
			}
			deletedJob, err := tx.ExecContext(ctx, `
DELETE FROM user_memory_embedding_jobs WHERE memory_id = $1::uuid
`, vector.PassageID)
			if err != nil {
				_ = tx.Rollback()
				return completed, errors.New("remove Memory regression fixture embedding job")
			}
			deleted, deletedErr := deletedJob.RowsAffected()
			if deletedErr != nil || deleted != 1 {
				_ = tx.Rollback()
				return completed, fmt.Errorf("%w: fixture embedding job cardinality", ErrCaptureStateConflict)
			}
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return completed, errors.New("commit Memory regression vector publish")
		}
		completed += len(response.Vectors)
	}
	if completed != len(inputs) {
		return completed, ErrCaptureStateConflict
	}
	return completed, nil
}

func verifySeedDatabase(ctx context.Context, db *sql.DB, runID string) error {
	var databaseName, currentUser, schemaVersion string
	if err := db.QueryRowContext(ctx, `
SELECT current_database(), current_user, schema_version
FROM memory_regression_runtime_guard WHERE run_id = $1
`, runID).Scan(&databaseName, &currentUser, &schemaVersion); err != nil {
		return fmt.Errorf("%w: seed database guard", ErrCaptureInvalid)
	}
	if !strings.HasPrefix(databaseName, ephemeralDatabasePrefix) || currentUser == "go_api_runtime" ||
		schemaVersion != ephemeralGuardSchema {
		return fmt.Errorf("%w: seed database authority", ErrCaptureInvalid)
	}
	return nil
}

func validVector(vector []float32) bool {
	if len(vector) != ragproviders.SiliconFlowEmbeddingDimensions {
		return false
	}
	norm := 0.0
	for _, component := range vector {
		value := float64(component)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		norm += value * value
	}
	return norm > 0 && !math.IsInf(norm, 0)
}

func realArrayLiteral(values []float32) string {
	var builder strings.Builder
	builder.Grow(len(values) * 12)
	builder.WriteByte('{')
	for index, value := range values {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	builder.WriteByte('}')
	return builder.String()
}
