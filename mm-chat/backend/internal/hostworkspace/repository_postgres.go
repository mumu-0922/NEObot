package hostworkspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/auth"
)

type PostgresRepository struct{ db *sql.DB }

type rowScanner interface{ Scan(...any) error }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

const workspaceColumns = `
  id, owner_user_id, name, system_prompt, files, color,
  enable_search, enable_reasoning, revision,
  runner_id, canonical_path, display_path, path_kind, directory_fingerprint,
  bound_at, legacy_imported_at, created_at, updated_at, deleted_at
`

func (repository *PostgresRepository) List(ctx context.Context) ([]Workspace, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrDisabled
	}
	userID := auth.UserOrDevelopment(ctx).ID
	rows, err := repository.db.QueryContext(ctx, `
SELECT `+workspaceColumns+`
FROM workspaces
WHERE owner_user_id = $1 AND deleted_at IS NULL
ORDER BY updated_at DESC, created_at DESC, id
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list Host Workspaces: %w", err)
	}
	defer rows.Close()
	items := []Workspace{}
	for rows.Next() {
		workspace, scanErr := scanWorkspace(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan Host Workspace: %w", scanErr)
		}
		items = append(items, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Host Workspaces: %w", err)
	}
	return items, nil
}

func (repository *PostgresRepository) Get(ctx context.Context, workspaceID string) (Workspace, error) {
	if repository == nil || repository.db == nil {
		return Workspace{}, ErrDisabled
	}
	workspace, err := scanWorkspace(repository.db.QueryRowContext(ctx, `
SELECT `+workspaceColumns+`
FROM workspaces
WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL
`, workspaceID, auth.UserOrDevelopment(ctx).ID))
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("get Host Workspace: %w", err)
	}
	return workspace, nil
}

func (repository *PostgresRepository) ImportLegacy(
	ctx context.Context,
	workspaceID string,
	settings Settings,
	importedAt time.Time,
) (Workspace, error) {
	if repository == nil || repository.db == nil {
		return Workspace{}, ErrDisabled
	}
	files, err := json.Marshal(settings.Files)
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	userID := auth.UserOrDevelopment(ctx).ID
	workspace, err := scanWorkspace(repository.db.QueryRowContext(ctx, `
INSERT INTO workspaces (
  id, owner_user_id, name, system_prompt, files, color,
  enable_search, enable_reasoning, legacy_imported_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5::jsonb, NULLIF($6, ''), $7, $8, $9, $9, $9)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  system_prompt = EXCLUDED.system_prompt,
  files = EXCLUDED.files,
  color = EXCLUDED.color,
  enable_search = EXCLUDED.enable_search,
  enable_reasoning = EXCLUDED.enable_reasoning,
  legacy_imported_at = COALESCE(workspaces.legacy_imported_at, EXCLUDED.legacy_imported_at),
  revision = CASE WHEN (
    workspaces.name, workspaces.system_prompt, workspaces.files, workspaces.color,
    workspaces.enable_search, workspaces.enable_reasoning
  ) IS DISTINCT FROM (
    EXCLUDED.name, EXCLUDED.system_prompt, EXCLUDED.files, EXCLUDED.color,
    EXCLUDED.enable_search, EXCLUDED.enable_reasoning
  ) THEN workspaces.revision + 1 ELSE workspaces.revision END,
  updated_at = CASE WHEN (
    workspaces.name, workspaces.system_prompt, workspaces.files, workspaces.color,
    workspaces.enable_search, workspaces.enable_reasoning
  ) IS DISTINCT FROM (
    EXCLUDED.name, EXCLUDED.system_prompt, EXCLUDED.files, EXCLUDED.color,
    EXCLUDED.enable_search, EXCLUDED.enable_reasoning
  ) THEN EXCLUDED.updated_at ELSE workspaces.updated_at END
WHERE workspaces.owner_user_id = EXCLUDED.owner_user_id
  AND workspaces.deleted_at IS NULL
RETURNING `+workspaceColumns+`
`, workspaceID, userID, settings.Name, settings.SystemPrompt, string(files), settings.Color,
		settings.EnableSearch, settings.EnableReasoning, importedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, ErrNotFound
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("import legacy Workspace: %w", normalizePostgresError(err))
	}
	return workspace, nil
}

func (repository *PostgresRepository) UpdateSettings(
	ctx context.Context,
	workspaceID string,
	expectedRevision int64,
	settings Settings,
) (Workspace, error) {
	if repository == nil || repository.db == nil {
		return Workspace{}, ErrDisabled
	}
	files, err := json.Marshal(settings.Files)
	if err != nil {
		return Workspace{}, ErrInvalid
	}
	workspace, err := scanWorkspace(repository.db.QueryRowContext(ctx, `
UPDATE workspaces SET
  name = $4, system_prompt = $5, files = $6::jsonb, color = NULLIF($7, ''),
  enable_search = $8, enable_reasoning = $9,
  revision = revision + 1, updated_at = now()
WHERE id = $1 AND owner_user_id = $2 AND revision = $3 AND deleted_at IS NULL
RETURNING `+workspaceColumns+`
`, workspaceID, auth.UserOrDevelopment(ctx).ID, expectedRevision, settings.Name,
		settings.SystemPrompt, string(files), settings.Color,
		settings.EnableSearch, settings.EnableReasoning))
	if errors.Is(err, sql.ErrNoRows) {
		return Workspace{}, repository.classifyWorkspaceMiss(ctx, workspaceID, expectedRevision)
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("update Host Workspace: %w", err)
	}
	return workspace, nil
}

func (repository *PostgresRepository) Bind(
	ctx context.Context,
	workspaceID string,
	expectedRevision int64,
	descriptor agenthost.WorkspaceDescriptor,
	runnerID string,
	boundAt time.Time,
) (Workspace, error) {
	if repository == nil || repository.db == nil {
		return Workspace{}, ErrDisabled
	}
	workspace, err := scanWorkspace(repository.db.QueryRowContext(ctx, `
UPDATE workspaces SET
  runner_id = $4, canonical_path = $5, display_path = $6, path_kind = $7,
  directory_fingerprint = $8, bound_at = $9,
  revision = revision + 1, updated_at = $9
WHERE id = $1 AND owner_user_id = $2 AND revision = $3 AND deleted_at IS NULL
  AND runner_id IS NULL
RETURNING `+workspaceColumns+`
`, workspaceID, auth.UserOrDevelopment(ctx).ID, expectedRevision,
		runnerID, descriptor.CanonicalPath, descriptor.DisplayPath, descriptor.PathKind,
		descriptor.DirectoryFingerprint, boundAt))
	if errors.Is(err, sql.ErrNoRows) {
		existing, getErr := repository.Get(ctx, workspaceID)
		switch {
		case getErr != nil:
			return Workspace{}, getErr
		case existing.Revision != expectedRevision:
			return Workspace{}, ErrRevisionConflict
		case existing.Bound():
			return Workspace{}, ErrAlreadyBound
		default:
			return Workspace{}, ErrNotFound
		}
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("bind Host Workspace: %w", normalizePostgresError(err))
	}
	return workspace, nil
}

func (repository *PostgresRepository) Delete(
	ctx context.Context,
	workspaceID string,
	expectedRevision int64,
	deletedAt time.Time,
) error {
	if repository == nil || repository.db == nil {
		return ErrDisabled
	}
	result, err := repository.db.ExecContext(ctx, `
UPDATE workspaces SET deleted_at = $4, updated_at = $4, revision = revision + 1
WHERE id = $1 AND owner_user_id = $2 AND revision = $3 AND deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM conversations
    WHERE agent_workspace_id = workspaces.id AND deleted_at IS NULL
  )
`, workspaceID, auth.UserOrDevelopment(ctx).ID, expectedRevision, deletedAt)
	if err != nil {
		return fmt.Errorf("delete Host Workspace: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete Host Workspace result: %w", err)
	}
	if rows == 1 {
		return nil
	}
	existing, getErr := repository.Get(ctx, workspaceID)
	if getErr != nil {
		return getErr
	}
	if existing.Revision != expectedRevision {
		return ErrRevisionConflict
	}
	return ErrWorkspaceInUse
}

func (repository *PostgresRepository) SetConversationWorkspace(
	ctx context.Context,
	conversationID string,
	workspaceID string,
) error {
	if repository == nil || repository.db == nil {
		return ErrDisabled
	}
	userID := auth.UserOrDevelopment(ctx).ID
	result, err := repository.db.ExecContext(ctx, `
UPDATE conversations SET workspace_id = $3, updated_at = now()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
  AND (agent_workspace_id IS NULL OR agent_workspace_id = $3)
  AND EXISTS (
    SELECT 1 FROM workspaces
    WHERE id = $3 AND owner_user_id = $2 AND deleted_at IS NULL
  )
`, conversationID, userID, workspaceID)
	if err != nil {
		return fmt.Errorf("set conversation Host Workspace: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 1 {
		return nil
	}
	var exists bool
	var locked bool
	err = repository.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM conversations WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
), EXISTS (
  SELECT 1 FROM conversations
  WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
    AND agent_workspace_id IS NOT NULL AND agent_workspace_id <> $3
)
`, conversationID, userID, workspaceID).Scan(&exists, &locked)
	if err != nil {
		return fmt.Errorf("classify conversation Host Workspace: %w", err)
	}
	if !exists {
		return ErrConversationNotFound
	}
	if locked {
		return ErrConversationLocked
	}
	return ErrNotFound
}

func (repository *PostgresRepository) ClearConversationWorkspace(
	ctx context.Context,
	conversationID string,
	workspaceID string,
) error {
	if repository == nil || repository.db == nil {
		return ErrDisabled
	}
	userID := auth.UserOrDevelopment(ctx).ID
	result, err := repository.db.ExecContext(ctx, `
UPDATE conversations SET workspace_id = NULL, updated_at = now()
WHERE id = $1 AND user_id = $2 AND workspace_id = $3
  AND deleted_at IS NULL AND agent_workspace_id IS NULL
`, conversationID, userID, workspaceID)
	if err != nil {
		return fmt.Errorf("clear conversation Host Workspace: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 1 {
		return nil
	}
	var exists, locked bool
	err = repository.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM conversations WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
), EXISTS (
  SELECT 1 FROM conversations
  WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
    AND agent_workspace_id IS NOT NULL
)
`, conversationID, userID).Scan(&exists, &locked)
	if err != nil {
		return fmt.Errorf("classify clear conversation Host Workspace: %w", err)
	}
	if !exists {
		return ErrConversationNotFound
	}
	if locked {
		return ErrConversationLocked
	}
	return ErrNotFound
}

func (repository *PostgresRepository) LockConversationExecutionWorkspace(
	ctx context.Context,
	conversationID string,
	workspaceID string,
	boundAt time.Time,
) (ExecutionBinding, error) {
	if repository == nil || repository.db == nil {
		return ExecutionBinding{}, ErrDisabled
	}
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return ExecutionBinding{}, fmt.Errorf("begin Agent Workspace lock: %w", err)
	}
	defer tx.Rollback()
	userID := auth.UserOrDevelopment(ctx).ID
	var groupedWorkspace sql.NullString
	var existingID, existingRunner, existingPath, existingFingerprint sql.NullString
	var permissionMode string
	var existingBoundAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
SELECT workspace_id, agent_workspace_id, agent_workspace_runner_id,
       agent_workspace_canonical_path, agent_workspace_fingerprint,
       agent_workspace_bound_at, agent_permission_mode
FROM conversations
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
FOR UPDATE
`, conversationID, userID).Scan(
		&groupedWorkspace, &existingID, &existingRunner, &existingPath,
		&existingFingerprint, &existingBoundAt, &permissionMode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionBinding{}, ErrConversationNotFound
	}
	if err != nil {
		return ExecutionBinding{}, fmt.Errorf("lock conversation Agent Workspace: %w", err)
	}
	if existingID.Valid {
		if existingID.String != workspaceID {
			return ExecutionBinding{}, ErrConversationLocked
		}
		return ExecutionBinding{
			ConversationID: conversationID, WorkspaceID: existingID.String,
			RunnerID: existingRunner.String, CanonicalPath: existingPath.String,
			DirectoryFingerprint: existingFingerprint.String, BoundAt: existingBoundAt.Time,
			PermissionMode: agenthost.PermissionMode(permissionMode),
		}, nil
	}
	if !groupedWorkspace.Valid || groupedWorkspace.String != workspaceID {
		return ExecutionBinding{}, ErrConversationLocked
	}
	var binding ExecutionBinding
	err = tx.QueryRowContext(ctx, `
SELECT id, runner_id, canonical_path, directory_fingerprint
FROM workspaces
WHERE id = $1 AND owner_user_id = $2 AND deleted_at IS NULL AND runner_id IS NOT NULL
FOR SHARE
`, workspaceID, userID).Scan(
		&binding.WorkspaceID, &binding.RunnerID, &binding.CanonicalPath,
		&binding.DirectoryFingerprint,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ExecutionBinding{}, ErrWorkspaceUnbound
	}
	if err != nil {
		return ExecutionBinding{}, fmt.Errorf("resolve conversation Agent Workspace: %w", err)
	}
	binding.ConversationID = conversationID
	binding.BoundAt = boundAt
	binding.PermissionMode = agenthost.PermissionMode(permissionMode)
	_, err = tx.ExecContext(ctx, `
UPDATE conversations SET
  agent_workspace_id = $3,
  agent_workspace_runner_id = $4,
  agent_workspace_canonical_path = $5,
  agent_workspace_fingerprint = $6,
  agent_workspace_bound_at = $7,
  updated_at = $7
WHERE id = $1 AND user_id = $2 AND agent_workspace_id IS NULL
`, conversationID, userID, binding.WorkspaceID, binding.RunnerID,
		binding.CanonicalPath, binding.DirectoryFingerprint, boundAt)
	if err != nil {
		return ExecutionBinding{}, fmt.Errorf("persist conversation Agent Workspace: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ExecutionBinding{}, fmt.Errorf("commit conversation Agent Workspace: %w", err)
	}
	return binding, nil
}

func (repository *PostgresRepository) SetConversationPermission(
	ctx context.Context,
	conversationID string,
	mode agenthost.PermissionMode,
) error {
	if repository == nil || repository.db == nil {
		return ErrDisabled
	}
	userID := auth.UserOrDevelopment(ctx).ID
	result, err := repository.db.ExecContext(ctx, `
UPDATE conversations AS conversation SET
  agent_permission_mode = $3,
  updated_at = now()
WHERE conversation.id = $1 AND conversation.user_id = $2
  AND conversation.deleted_at IS NULL AND conversation.workspace_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM workspaces
    WHERE id = conversation.workspace_id AND owner_user_id = $2
      AND deleted_at IS NULL AND runner_id IS NOT NULL
  )
  AND NOT EXISTS (
    SELECT 1 FROM messages
    WHERE conversation_id = conversation.id AND deleted_at IS NULL
      AND role = 'assistant' AND status IN ('pending', 'streaming')
  )
`, conversationID, userID, string(mode))
	if err != nil {
		return fmt.Errorf("set conversation Agent permission: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 1 {
		return nil
	}
	var exists, bound, active bool
	err = repository.db.QueryRowContext(ctx, `
SELECT
  EXISTS (
    SELECT 1 FROM conversations
    WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
  ),
  EXISTS (
    SELECT 1 FROM conversations AS conversation
    JOIN workspaces ON workspaces.id = conversation.workspace_id
      AND workspaces.owner_user_id = conversation.user_id
    WHERE conversation.id = $1 AND conversation.user_id = $2
      AND conversation.deleted_at IS NULL AND workspaces.deleted_at IS NULL
      AND workspaces.runner_id IS NOT NULL
  ),
  EXISTS (
    SELECT 1 FROM messages
    WHERE conversation_id = $1 AND user_id = $2 AND deleted_at IS NULL
      AND role = 'assistant' AND status IN ('pending', 'streaming')
  )
`, conversationID, userID).Scan(&exists, &bound, &active)
	if err != nil {
		return fmt.Errorf("classify conversation Agent permission: %w", err)
	}
	if !exists {
		return ErrConversationNotFound
	}
	if !bound {
		return ErrWorkspaceUnbound
	}
	if active {
		return ErrPermissionLocked
	}
	return ErrPermissionUnavailable
}

func (repository *PostgresRepository) classifyWorkspaceMiss(
	ctx context.Context,
	workspaceID string,
	expectedRevision int64,
) error {
	existing, err := repository.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if existing.Revision != expectedRevision {
		return ErrRevisionConflict
	}
	return ErrNotFound
}

func scanWorkspace(scanner rowScanner) (Workspace, error) {
	var workspace Workspace
	var files []byte
	var color, runnerID, canonicalPath, displayPath, pathKind, fingerprint sql.NullString
	var enableSearch, enableReasoning sql.NullBool
	var boundAt, importedAt, deletedAt sql.NullTime
	if err := scanner.Scan(
		&workspace.ID, &workspace.UserID, &workspace.Settings.Name,
		&workspace.Settings.SystemPrompt, &files, &color,
		&enableSearch, &enableReasoning, &workspace.Revision,
		&runnerID, &canonicalPath, &displayPath, &pathKind, &fingerprint,
		&boundAt, &importedAt, &workspace.CreatedAt, &workspace.UpdatedAt, &deletedAt,
	); err != nil {
		return Workspace{}, err
	}
	if err := json.Unmarshal(files, &workspace.Settings.Files); err != nil {
		return Workspace{}, err
	}
	workspace.Settings.Color = color.String
	if enableSearch.Valid {
		workspace.Settings.EnableSearch = &enableSearch.Bool
	}
	if enableReasoning.Valid {
		workspace.Settings.EnableReasoning = &enableReasoning.Bool
	}
	workspace.RunnerID = runnerID.String
	workspace.CanonicalPath = canonicalPath.String
	workspace.DisplayPath = displayPath.String
	workspace.PathKind = pathKind.String
	workspace.DirectoryFingerprint = fingerprint.String
	if boundAt.Valid {
		workspace.BoundAt = &boundAt.Time
	}
	if importedAt.Valid {
		workspace.LegacyImportedAt = &importedAt.Time
	}
	if deletedAt.Valid {
		workspace.DeletedAt = &deletedAt.Time
	}
	return workspace, nil
}

func normalizePostgresError(err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return err
	}
	if postgresError.ConstraintName == "idx_workspaces_owner_runner_directory_active" {
		return ErrDirectoryAlreadyRegistered
	}
	return err
}
