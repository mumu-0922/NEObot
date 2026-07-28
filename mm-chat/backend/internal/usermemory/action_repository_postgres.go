package usermemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (r *PostgresRepository) HydrateDirectAction(
	ctx context.Context,
	input DirectActionHydrationInput,
) (DirectActionContext, error) {
	if err := r.requireDB(); err != nil {
		return DirectActionContext{}, err
	}
	userID := auth.UserOrDevelopment(ctx).ID
	var result DirectActionContext
	var projectID sql.NullString
	var projectGeneration sql.NullInt64
	var memoriesJSON []byte
	err := r.db.QueryRowContext(ctx, `
SELECT
  project_id,
  conversation_scope_generation,
  project_scope_generation,
  sensitive_memory_enabled,
  current_memories
FROM memory_hydrate_direct_user_action(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid
)
`, userID, input.ConversationID, input.SourceMessageID, input.AssistantMessageID).Scan(
		&projectID,
		&result.ConversationScopeGeneration,
		&projectGeneration,
		&result.SensitiveMemoryEnabled,
		&memoriesJSON,
	)
	if err != nil {
		return DirectActionContext{}, fmt.Errorf("hydrate direct memory action: %w", err)
	}
	result.ProjectID = strings.TrimSpace(projectID.String)
	if projectGeneration.Valid {
		value := projectGeneration.Int64
		result.ProjectScopeGeneration = &value
	}
	if err := json.Unmarshal(memoriesJSON, &result.Memories); err != nil {
		return DirectActionContext{}, fmt.Errorf("decode direct memory action context: %w", err)
	}
	if result.Memories == nil {
		result.Memories = []DirectActionMemory{}
	}
	return result, nil
}

func (r *PostgresRepository) ApplyDirectAction(
	ctx context.Context,
	input DirectActionApplyInput,
) (DirectActionResult, error) {
	if err := r.requireDB(); err != nil {
		return DirectActionResult{}, err
	}
	targets, err := json.Marshal(input.Targets)
	if err != nil {
		return DirectActionResult{}, fmt.Errorf("encode direct memory action targets: %w", err)
	}
	userID := auth.UserOrDevelopment(ctx).ID
	var result DirectActionResult
	var memoryID sql.NullString
	var memoryRevision sql.NullInt64
	var activityID sql.NullString
	err = r.db.QueryRowContext(ctx, `
SELECT
  action_id,
  action_status,
  action_result_code,
  result_memory_id,
  result_memory_revision,
  resolved_scope_type,
  activity_id
FROM memory_apply_direct_user_action(
  $1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6::uuid, $7::uuid,
  $8::uuid, $9::uuid, $10::uuid, $11::uuid, $12::smallint,
  $13::text, NULLIF($14, '')::text, NULLIF($15, '')::text,
  NULLIF($16, '')::text, $17::text, $18::smallint, $19::text[],
  $20::text, $21::text, $22::double precision, $23::jsonb,
  NULLIF($24, '')::text, NULLIF($25, '')::text
)
`,
		input.ActionID,
		input.ActivityID,
		input.MemoryID,
		input.EventID,
		input.JobID,
		input.TombstoneID,
		input.ManifestID,
		userID,
		input.ConversationID,
		input.SourceMessageID,
		input.AssistantMessageID,
		input.SchemaMajor,
		input.RequestedAction,
		input.MemoryType,
		input.Content,
		input.NormalizedContent,
		input.CandidateHash,
		input.Importance,
		input.Tags,
		input.Sensitivity,
		input.ScopeType,
		input.Confidence,
		string(targets),
		input.PreflightStatus,
		input.PreflightResultCode,
	).Scan(
		&result.ActionID,
		&result.Status,
		&result.ResultCode,
		&memoryID,
		&memoryRevision,
		&result.ScopeType,
		&activityID,
	)
	if err != nil {
		return DirectActionResult{}, fmt.Errorf("apply direct memory action: %w", err)
	}
	result.MemoryID = strings.TrimSpace(memoryID.String)
	if memoryRevision.Valid {
		result.MemoryRevision = memoryRevision.Int64
	}
	result.ActivityID = strings.TrimSpace(activityID.String)
	return result, nil
}

func (r *PostgresRepository) ListActivities(
	ctx context.Context,
	cursor string,
	limit int,
) ([]MemoryActivity, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	userID := auth.UserOrDevelopment(ctx).ID
	rows, err := r.db.QueryContext(ctx, `
SELECT
  id, assistant_message_id, ordinal, subject_type, subject_id,
  subject_revision, action, status, reason_code, undo_kind, undo_status,
  memory_type, memory_content, memory_revision, memory_deleted,
  created_at, updated_at
FROM memory_list_activities($1::uuid, NULLIF($2, '')::uuid, $3::integer)
	`, userID, cursor, limit)
	if err != nil {
		return nil, mapActionPostgresError(err)
	}
	defer rows.Close()
	items := make([]MemoryActivity, 0)
	for rows.Next() {
		var item MemoryActivity
		var subjectRevision sql.NullInt64
		var memoryType sql.NullString
		var memoryContent sql.NullString
		var memoryRevision sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.AssistantMessageID,
			&item.Ordinal,
			&item.SubjectType,
			&item.SubjectID,
			&subjectRevision,
			&item.Action,
			&item.Status,
			&item.ReasonCode,
			&item.UndoKind,
			&item.UndoStatus,
			&memoryType,
			&memoryContent,
			&memoryRevision,
			&item.MemoryDeleted,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan memory activity: %w", err)
		}
		if subjectRevision.Valid {
			value := subjectRevision.Int64
			item.SubjectRevision = &value
		}
		item.MemoryType = memoryType.String
		item.MemoryContent = memoryContent.String
		if memoryRevision.Valid {
			value := memoryRevision.Int64
			item.MemoryRevision = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapActionPostgresError(err)
	}
	return items, nil
}

func (r *PostgresRepository) ListMessageUsages(
	ctx context.Context,
	assistantMessageID string,
) ([]MessageMemoryUsage, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	userID := auth.UserOrDevelopment(ctx).ID
	rows, err := r.db.QueryContext(ctx, `
SELECT
  assistant_message_id, ordinal, memory_id, memory_revision, scope_type,
  memory_type, memory_content, memory_deleted, created_at
FROM memory_list_message_usages($1::uuid, $2::uuid)
	`, userID, assistantMessageID)
	if err != nil {
		return nil, mapActionPostgresError(err)
	}
	defer rows.Close()
	items := make([]MessageMemoryUsage, 0)
	for rows.Next() {
		var item MessageMemoryUsage
		var memoryType sql.NullString
		var memoryContent sql.NullString
		if err := rows.Scan(
			&item.AssistantMessageID,
			&item.Ordinal,
			&item.MemoryID,
			&item.MemoryRevision,
			&item.ScopeType,
			&memoryType,
			&memoryContent,
			&item.MemoryDeleted,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message memory usage: %w", err)
		}
		item.MemoryType = memoryType.String
		item.MemoryContent = memoryContent.String
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, mapActionPostgresError(err)
	}
	return items, nil
}

func (r *PostgresRepository) UndoActivity(
	ctx context.Context,
	input UndoActivityInput,
) (UndoActivityResult, error) {
	if err := r.requireDB(); err != nil {
		return UndoActivityResult{}, err
	}
	userID := auth.UserOrDevelopment(ctx).ID
	var result UndoActivityResult
	var memoryRevision sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
SELECT undo_status, result_code, memory_id, memory_revision
FROM memory_undo_activity(
  $1::uuid, $2::uuid, $3::bigint,
  $4::uuid, $5::uuid, $6::uuid, $7::uuid
)
`,
		userID,
		input.ActivityID,
		input.ExpectedRevision,
		input.EventID,
		input.JobID,
		input.TombstoneID,
		input.ManifestID,
	).Scan(
		&result.Status,
		&result.ResultCode,
		&result.MemoryID,
		&memoryRevision,
	)
	if err != nil {
		mapped := mapActionPostgresError(err)
		return UndoActivityResult{}, mapped
	}
	if memoryRevision.Valid {
		result.MemoryRevision = memoryRevision.Int64
	}
	return result, nil
}

func mapActionPostgresError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return fmt.Errorf("memory action operation: %w", err)
	}
	switch postgresError.Message {
	case "MEMORY_ACTIVITY_CURSOR_INVALID":
		return validation(
			"INVALID_MEMORY_ACTIVITY_CURSOR",
			"activity cursor is invalid",
		)
	case "MEMORY_ACTIVITY_NOT_FOUND":
		return ErrActivityNotFound
	case "MEMORY_ACTIVITY_UNDO_UNAVAILABLE":
		return ErrActivityUndoUnavailable
	case "MEMORY_ACTIVITY_REVISION_INVALID":
		return validation(
			"INVALID_MEMORY_ACTIVITY_REVISION",
			"memory activity revision is invalid",
		)
	case "MEMORY_USAGE_MESSAGE_INVALID":
		return validation(
			"INVALID_ASSISTANT_MESSAGE_ID",
			"assistant message id is invalid",
		)
	default:
		return fmt.Errorf("memory action operation: %w", err)
	}
}

var _ ActionRepository = (*PostgresRepository)(nil)
