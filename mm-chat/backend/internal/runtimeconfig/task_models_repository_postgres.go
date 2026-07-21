package runtimeconfig

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type PostgresTaskModelSettingsRepository struct {
	db *sql.DB
}

func NewPostgresTaskModelSettingsRepository(
	db *sql.DB,
) *PostgresTaskModelSettingsRepository {
	return &PostgresTaskModelSettingsRepository{db: db}
}

func (r *PostgresTaskModelSettingsRepository) GetTaskModelSettings(
	ctx context.Context,
	userID string,
) (StoredTaskModelSettings, bool, error) {
	if r == nil || r.db == nil {
		return StoredTaskModelSettings{}, false, ErrDatabaseRequired
	}
	var stored StoredTaskModelSettings
	err := r.db.QueryRowContext(ctx, `
SELECT
  title_generation,
  related_questions,
  context_compression,
  prompt_optimization,
  rag_query,
  memory,
  updated_at
FROM task_model_settings
WHERE user_id = $1
`, userID).Scan(
		&stored.Models.TitleGeneration,
		&stored.Models.RelatedQuestions,
		&stored.Models.ContextCompression,
		&stored.Models.PromptOptimization,
		&stored.Models.RAGQuery,
		&stored.Models.Memory,
		&stored.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredTaskModelSettings{}, false, nil
	}
	if err != nil {
		return StoredTaskModelSettings{}, false, fmt.Errorf("get task model settings: %w", err)
	}
	return stored, true, nil
}

func (r *PostgresTaskModelSettingsRepository) UpsertTaskModelSettings(
	ctx context.Context,
	userID string,
	models TaskModels,
) (StoredTaskModelSettings, error) {
	if r == nil || r.db == nil {
		return StoredTaskModelSettings{}, ErrDatabaseRequired
	}
	var stored StoredTaskModelSettings
	err := r.db.QueryRowContext(ctx, `
INSERT INTO task_model_settings (
  user_id,
  title_generation,
  related_questions,
  context_compression,
  prompt_optimization,
  rag_query,
  memory
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id) DO UPDATE SET
  title_generation = EXCLUDED.title_generation,
  related_questions = EXCLUDED.related_questions,
  context_compression = EXCLUDED.context_compression,
  prompt_optimization = EXCLUDED.prompt_optimization,
  rag_query = EXCLUDED.rag_query,
  memory = EXCLUDED.memory,
  updated_at = now()
RETURNING
  title_generation,
  related_questions,
  context_compression,
  prompt_optimization,
  rag_query,
  memory,
  updated_at
`,
		userID,
		models.TitleGeneration,
		models.RelatedQuestions,
		models.ContextCompression,
		models.PromptOptimization,
		models.RAGQuery,
		models.Memory,
	).Scan(
		&stored.Models.TitleGeneration,
		&stored.Models.RelatedQuestions,
		&stored.Models.ContextCompression,
		&stored.Models.PromptOptimization,
		&stored.Models.RAGQuery,
		&stored.Models.Memory,
		&stored.UpdatedAt,
	)
	if err != nil {
		return StoredTaskModelSettings{}, fmt.Errorf("upsert task model settings: %w", err)
	}
	return stored, nil
}
