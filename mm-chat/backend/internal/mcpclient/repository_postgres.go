package mcpclient

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PostgresRepository struct {
	db    *sql.DB
	newID func() string
}

var _ Repository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db, newID: func() string { return uuid.NewString() }}
}

func (r *PostgresRepository) CountPrivateServers(ctx context.Context, userID string) (int, error) {
	if err := r.requireDB(); err != nil {
		return 0, err
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)::int FROM mcp_servers WHERE user_id = $1 AND deleted_at IS NULL
`, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count private mcp servers: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) CreatePrivateServer(
	ctx context.Context,
	userID string,
	input CreateServerInput,
) (Server, error) {
	if err := r.requireDB(); err != nil {
		return Server{}, err
	}
	id := r.newID()
	authConfig, err := json.Marshal(map[string]any{
		"headerName": strings.TrimSpace(input.HeaderName),
		"clientId":   strings.TrimSpace(input.ClientID),
		"scopes":     normalizeStrings(input.Scopes, 32, 256),
	})
	if err != nil {
		return Server{}, fmt.Errorf("marshal mcp auth config: %w", err)
	}
	row := r.db.QueryRowContext(ctx, `
INSERT INTO mcp_servers (
  id, user_id, name, endpoint_url, transport, auth_type, auth_config, status
) VALUES ($1, $2, $3, $4, 'streamable_http', $5, $6::jsonb, 'draft')
RETURNING id, name, endpoint_url, transport, auth_type, auth_config, status,
  tool_snapshot, tool_snapshot_hash, validated_at, last_error_code,
  created_at, updated_at
`, id, userID, input.Name, input.EndpointURL, input.AuthType, string(authConfig))
	server, err := scanPrivateServer(row)
	if err != nil {
		return Server{}, fmt.Errorf("create private mcp server: %w", err)
	}
	return server, nil
}

func (r *PostgresRepository) ListPrivateServers(ctx context.Context, userID string) ([]Server, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT s.id, s.name, s.endpoint_url, s.transport, s.auth_type, s.auth_config,
  s.status, s.tool_snapshot, s.tool_snapshot_hash, s.validated_at,
  s.last_error_code, s.created_at, s.updated_at,
  EXISTS (
    SELECT 1 FROM mcp_credentials c
    WHERE c.server_source = 'private' AND c.server_ref = s.id::text
      AND c.user_id = s.user_id
  )
FROM mcp_servers s
WHERE s.user_id = $1 AND s.deleted_at IS NULL
ORDER BY s.updated_at DESC, s.id DESC
`, userID)
	if err != nil {
		return nil, fmt.Errorf("list private mcp servers: %w", err)
	}
	defer rows.Close()
	servers := []Server{}
	for rows.Next() {
		server, err := scanPrivateServerWithCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("scan private mcp server: %w", err)
		}
		servers = append(servers, server)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate private mcp servers: %w", err)
	}
	return servers, nil
}

func (r *PostgresRepository) GetPrivateServer(
	ctx context.Context,
	userID string,
	serverID string,
) (Server, error) {
	if err := r.requireDB(); err != nil {
		return Server{}, err
	}
	server, err := scanPrivateServerWithCredential(r.db.QueryRowContext(ctx, `
SELECT s.id, s.name, s.endpoint_url, s.transport, s.auth_type, s.auth_config,
  s.status, s.tool_snapshot, s.tool_snapshot_hash, s.validated_at,
  s.last_error_code, s.created_at, s.updated_at,
  EXISTS (
    SELECT 1 FROM mcp_credentials c
    WHERE c.server_source = 'private' AND c.server_ref = s.id::text
      AND c.user_id = s.user_id
  )
FROM mcp_servers s
WHERE s.id = $1 AND s.user_id = $2 AND s.deleted_at IS NULL
`, serverID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrServerNotFound
	}
	if err != nil {
		return Server{}, fmt.Errorf("get private mcp server: %w", err)
	}
	return server, nil
}

func (r *PostgresRepository) UpdateServerValidation(
	ctx context.Context,
	userID string,
	serverID string,
	status string,
	tools []Tool,
	hash string,
	errorCode string,
	validatedAt *time.Time,
) (Server, error) {
	if err := r.requireDB(); err != nil {
		return Server{}, err
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return Server{}, fmt.Errorf("marshal mcp tool snapshot: %w", err)
	}
	server, err := scanPrivateServerWithCredential(r.db.QueryRowContext(ctx, `
UPDATE mcp_servers s
SET status = $3,
    tool_snapshot = $4::jsonb,
    tool_snapshot_hash = NULLIF($5, ''),
    last_error_code = NULLIF($6, ''),
    validated_at = $7,
    updated_at = now()
WHERE s.id = $1 AND s.user_id = $2 AND s.deleted_at IS NULL
RETURNING s.id, s.name, s.endpoint_url, s.transport, s.auth_type, s.auth_config,
  s.status, s.tool_snapshot, s.tool_snapshot_hash, s.validated_at,
  s.last_error_code, s.created_at, s.updated_at,
  EXISTS (
    SELECT 1 FROM mcp_credentials c
    WHERE c.server_source = 'private' AND c.server_ref = s.id::text
      AND c.user_id = s.user_id
  )
`, serverID, userID, status, string(encoded), hash, errorCode, validatedAt))
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrServerNotFound
	}
	if err != nil {
		return Server{}, fmt.Errorf("update private mcp server validation: %w", err)
	}
	return server, nil
}

func (r *PostgresRepository) DeletePrivateServer(ctx context.Context, userID, serverID string) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete private mcp server: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE mcp_servers
SET status = 'disabled', deleted_at = COALESCE(deleted_at, now()), updated_at = now()
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
`, serverID, userID)
	if err != nil {
		return fmt.Errorf("delete private mcp server: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted mcp server count: %w", err)
	}
	if count == 0 {
		return ErrServerNotFound
	}
	for _, statement := range []string{
		`DELETE FROM mcp_credentials WHERE user_id = $2 AND server_source = 'private' AND server_ref = $1`,
		`DELETE FROM mcp_oauth_states WHERE user_id = $2 AND server_source = 'private' AND server_ref = $1`,
	} {
		if _, err := tx.ExecContext(ctx, statement, serverID, userID); err != nil {
			return fmt.Errorf("clean private mcp server references: %w", err)
		}
	}
	for _, statement := range []string{
		`DELETE FROM mcp_conversation_servers WHERE server_source = 'private' AND server_ref = $1`,
		`DELETE FROM mcp_workspace_servers WHERE server_source = 'private' AND server_ref = $1`,
		`DELETE FROM mcp_server_grants WHERE server_source = 'private' AND server_ref = $1`,
	} {
		if _, err := tx.ExecContext(ctx, statement, serverID); err != nil {
			return fmt.Errorf("clean private mcp server selections: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete private mcp server: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetCredential(
	ctx context.Context,
	userID string,
	serverRef ServerRef,
) (Credential, bool, error) {
	if err := r.requireDB(); err != nil {
		return Credential{}, false, err
	}
	credential, err := scanCredential(r.db.QueryRowContext(ctx, `
SELECT id, user_id, server_source, server_ref, kind, encrypted_secret_ref, metadata,
  expires_at, created_at, updated_at
FROM mcp_credentials
WHERE user_id = $1 AND server_source = $2 AND server_ref = $3
`, userID, serverRef.Source, serverRef.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return Credential{}, false, nil
	}
	if err != nil {
		return Credential{}, false, fmt.Errorf("get mcp credential: %w", err)
	}
	return credential, true, nil
}

func (r *PostgresRepository) UpsertCredential(ctx context.Context, input Credential) (Credential, error) {
	if err := r.requireDB(); err != nil {
		return Credential{}, err
	}
	if strings.TrimSpace(input.ID) == "" {
		input.ID = r.newID()
	}
	metadata, err := json.Marshal(objectOrEmpty(input.Metadata))
	if err != nil {
		return Credential{}, fmt.Errorf("marshal mcp credential metadata: %w", err)
	}
	credential, err := scanCredential(r.db.QueryRowContext(ctx, `
INSERT INTO mcp_credentials (
  id, user_id, server_source, server_ref, kind, encrypted_secret_ref, metadata, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
ON CONFLICT (user_id, server_source, server_ref) DO UPDATE
SET kind = EXCLUDED.kind,
    encrypted_secret_ref = EXCLUDED.encrypted_secret_ref,
    metadata = EXCLUDED.metadata,
    expires_at = EXCLUDED.expires_at,
    updated_at = now()
RETURNING id, user_id, server_source, server_ref, kind, encrypted_secret_ref, metadata,
  expires_at, created_at, updated_at
`, input.ID, input.UserID, input.ServerRef.Source, input.ServerRef.ID, input.Kind,
		input.EncryptedSecretRef, string(metadata), input.ExpiresAt))
	if err != nil {
		return Credential{}, fmt.Errorf("upsert mcp credential: %w", err)
	}
	return credential, nil
}

func (r *PostgresRepository) DeleteCredential(ctx context.Context, userID string, serverRef ServerRef) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `
DELETE FROM mcp_credentials
WHERE user_id = $1 AND server_source = $2 AND server_ref = $3
`, userID, serverRef.Source, serverRef.ID); err != nil {
		return fmt.Errorf("delete mcp credential: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ConversationScope(
	ctx context.Context,
	userID string,
	conversationID string,
) (ConversationScope, error) {
	if err := r.requireDB(); err != nil {
		return ConversationScope{}, err
	}
	var scope ConversationScope
	var workspaceID, teamID sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT c.id, c.user_id, c.workspace_id, w.team_id
FROM conversations c
LEFT JOIN workspaces w ON w.id = c.workspace_id AND w.deleted_at IS NULL
WHERE c.id = $1 AND c.user_id = $2 AND c.deleted_at IS NULL
`, conversationID, userID).Scan(&scope.ConversationID, &scope.UserID, &workspaceID, &teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return ConversationScope{}, ErrSelectionInvalid
	}
	if err != nil {
		return ConversationScope{}, fmt.Errorf("get mcp conversation scope: %w", err)
	}
	scope.WorkspaceID = workspaceID.String
	scope.TeamID = teamID.String
	return scope, nil
}

func (r *PostgresRepository) GetSelection(
	ctx context.Context,
	userID string,
	conversationID string,
) (Selection, bool, error) {
	if err := r.requireDB(); err != nil {
		return Selection{}, false, err
	}
	selection := Selection{ConversationID: conversationID}
	err := r.db.QueryRowContext(ctx, `
SELECT mode, revision
FROM mcp_conversation_selections
WHERE conversation_id = $1 AND user_id = $2
`, conversationID, userID).Scan(&selection.Mode, &selection.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return Selection{}, false, nil
	}
	if err != nil {
		return Selection{}, false, fmt.Errorf("get mcp selection: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT server_source, server_ref, disabled_tools
FROM mcp_conversation_servers
WHERE conversation_id = $1
ORDER BY server_source, server_ref
`, conversationID)
	if err != nil {
		return Selection{}, false, fmt.Errorf("list mcp selected servers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanSelectionServer(rows)
		if err != nil {
			return Selection{}, false, fmt.Errorf("scan mcp selected server: %w", err)
		}
		selection.Servers = append(selection.Servers, item)
	}
	if err := rows.Err(); err != nil {
		return Selection{}, false, fmt.Errorf("iterate mcp selected servers: %w", err)
	}
	return selection, true, nil
}

func (r *PostgresRepository) ReplaceSelection(
	ctx context.Context,
	userID string,
	selection Selection,
) (Selection, error) {
	if err := r.requireDB(); err != nil {
		return Selection{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Selection{}, fmt.Errorf("begin replace mcp selection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var existingRevision int64
	err = tx.QueryRowContext(ctx, `
SELECT revision
FROM mcp_conversation_selections
WHERE conversation_id = $1 AND user_id = $2
FOR UPDATE
`, selection.ConversationID, userID).Scan(&existingRevision)
	if errors.Is(err, sql.ErrNoRows) {
		var owner string
		if err := tx.QueryRowContext(ctx, `
SELECT user_id FROM conversations
WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
`, selection.ConversationID, userID).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return Selection{}, ErrSelectionInvalid
			}
			return Selection{}, fmt.Errorf("authorize mcp selection: %w", err)
		}
		existingRevision = 0
	} else if err != nil {
		return Selection{}, fmt.Errorf("lock mcp selection: %w", err)
	}
	if selection.Revision > 0 && selection.Revision != existingRevision {
		return Selection{}, ErrSelectionInvalid
	}
	newRevision := existingRevision + 1
	if _, err := tx.ExecContext(ctx, `
INSERT INTO mcp_conversation_selections (
  conversation_id, user_id, mode, revision
) VALUES ($1, $2, $3, $4)
ON CONFLICT (conversation_id) DO UPDATE
SET mode = EXCLUDED.mode, revision = EXCLUDED.revision, updated_at = now()
`, selection.ConversationID, userID, selection.Mode, newRevision); err != nil {
		return Selection{}, fmt.Errorf("upsert mcp selection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_conversation_servers WHERE conversation_id = $1`, selection.ConversationID); err != nil {
		return Selection{}, fmt.Errorf("clear mcp selected servers: %w", err)
	}
	for _, item := range selection.Servers {
		disabled, err := json.Marshal(item.DisabledTools)
		if err != nil {
			return Selection{}, fmt.Errorf("marshal mcp disabled tools: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO mcp_conversation_servers (
  conversation_id, server_source, server_ref, disabled_tools
) VALUES ($1, $2, $3, $4::jsonb)
`, selection.ConversationID, item.Ref.Source, item.Ref.ID, string(disabled)); err != nil {
			return Selection{}, fmt.Errorf("insert mcp selected server: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Selection{}, fmt.Errorf("commit mcp selection: %w", err)
	}
	selection.Revision = newRevision
	return selection, nil
}

func (r *PostgresRepository) GetWorkspaceSelection(
	ctx context.Context,
	userID string,
	workspaceID string,
) (WorkspaceSelection, bool, error) {
	if err := r.requireDB(); err != nil {
		return WorkspaceSelection{}, false, err
	}
	if err := r.authorizeWorkspace(ctx, r.db, userID, workspaceID); err != nil {
		return WorkspaceSelection{}, false, err
	}
	selection := WorkspaceSelection{WorkspaceID: workspaceID}
	err := r.db.QueryRowContext(ctx, `
SELECT revision FROM mcp_workspace_selections WHERE workspace_id = $1
`, workspaceID).Scan(&selection.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceSelection{}, false, nil
	}
	if err != nil {
		return WorkspaceSelection{}, false, fmt.Errorf("get mcp workspace selection: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT server_source, server_ref, disabled_tools
FROM mcp_workspace_servers
WHERE workspace_id = $1
ORDER BY server_source, server_ref
`, workspaceID)
	if err != nil {
		return WorkspaceSelection{}, false, fmt.Errorf("list mcp workspace servers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanSelectionServer(rows)
		if err != nil {
			return WorkspaceSelection{}, false, fmt.Errorf("scan mcp workspace server: %w", err)
		}
		selection.Servers = append(selection.Servers, item)
	}
	if err := rows.Err(); err != nil {
		return WorkspaceSelection{}, false, fmt.Errorf("iterate mcp workspace servers: %w", err)
	}
	return selection, true, nil
}

func (r *PostgresRepository) ReplaceWorkspaceSelection(
	ctx context.Context,
	userID string,
	selection WorkspaceSelection,
) (WorkspaceSelection, error) {
	if err := r.requireDB(); err != nil {
		return WorkspaceSelection{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return WorkspaceSelection{}, fmt.Errorf("begin replace mcp workspace selection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.authorizeWorkspace(ctx, tx, userID, selection.WorkspaceID); err != nil {
		return WorkspaceSelection{}, err
	}
	var existingRevision int64
	err = tx.QueryRowContext(ctx, `
SELECT revision FROM mcp_workspace_selections WHERE workspace_id = $1 FOR UPDATE
`, selection.WorkspaceID).Scan(&existingRevision)
	if errors.Is(err, sql.ErrNoRows) {
		existingRevision = 0
	} else if err != nil {
		return WorkspaceSelection{}, fmt.Errorf("lock mcp workspace selection: %w", err)
	}
	if selection.Revision > 0 && selection.Revision != existingRevision {
		return WorkspaceSelection{}, ErrSelectionInvalid
	}
	newRevision := existingRevision + 1
	if _, err := tx.ExecContext(ctx, `
INSERT INTO mcp_workspace_selections (workspace_id, revision)
VALUES ($1, $2)
ON CONFLICT (workspace_id) DO UPDATE
SET revision = EXCLUDED.revision, updated_at = now()
`, selection.WorkspaceID, newRevision); err != nil {
		return WorkspaceSelection{}, fmt.Errorf("upsert mcp workspace selection: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM mcp_workspace_servers WHERE workspace_id = $1`, selection.WorkspaceID); err != nil {
		return WorkspaceSelection{}, fmt.Errorf("clear mcp workspace servers: %w", err)
	}
	for _, item := range selection.Servers {
		disabled, err := json.Marshal(item.DisabledTools)
		if err != nil {
			return WorkspaceSelection{}, fmt.Errorf("marshal mcp workspace disabled tools: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO mcp_workspace_servers (
  workspace_id, server_source, server_ref, disabled_tools
) VALUES ($1, $2, $3, $4::jsonb)
`, selection.WorkspaceID, item.Ref.Source, item.Ref.ID, string(disabled)); err != nil {
			return WorkspaceSelection{}, fmt.Errorf("insert mcp workspace server: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return WorkspaceSelection{}, fmt.Errorf("commit mcp workspace selection: %w", err)
	}
	selection.Revision = newRevision
	return selection, nil
}

func (r *PostgresRepository) CreateRunSnapshot(ctx context.Context, snapshot RunSnapshot) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal mcp run snapshot: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO mcp_run_snapshots (
  run_id, user_id, conversation_id, message_id, selection_revision,
  snapshot_hash, snapshot, created_at, expires_at
) VALUES ($1::uuid, $2::uuid, $3::uuid, NULLIF($4, '')::uuid, $5, $6, $7::jsonb, $8, $9)
`, snapshot.RunID, snapshot.UserID, snapshot.ConversationID, snapshot.MessageID,
		snapshot.SelectionRevision, snapshot.Hash, string(encoded), snapshot.CreatedAt, snapshot.ExpiresAt); err != nil {
		return fmt.Errorf("create mcp run snapshot: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CountPendingOAuthStates(ctx context.Context, userID string, now time.Time) (int, error) {
	if err := r.requireDB(); err != nil {
		return 0, err
	}
	var count int
	if err := r.db.QueryRowContext(ctx, `
SELECT COUNT(*)::int FROM mcp_oauth_states
WHERE user_id = $1 AND consumed_at IS NULL AND expires_at > $2
`, userID, now).Scan(&count); err != nil {
		return 0, fmt.Errorf("count mcp oauth states: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) CreateOAuthState(ctx context.Context, state OAuthState) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if strings.TrimSpace(state.ID) == "" {
		state.ID = r.newID()
	}
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO mcp_oauth_states (
  id, state_hash, user_id, server_source, server_ref,
  encrypted_flow_ref, return_url, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, state.ID, state.StateHash, state.UserID, state.ServerRef.Source, state.ServerRef.ID,
		state.EncryptedFlowRef, state.ReturnURL, state.ExpiresAt); err != nil {
		return fmt.Errorf("create mcp oauth state: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ConsumeOAuthState(
	ctx context.Context,
	stateHash string,
	now time.Time,
) (OAuthState, error) {
	if err := r.requireDB(); err != nil {
		return OAuthState{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return OAuthState{}, fmt.Errorf("begin consume mcp oauth state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := scanOAuthState(tx.QueryRowContext(ctx, `
SELECT id, state_hash, user_id, server_source, server_ref,
  encrypted_flow_ref, return_url,
  expires_at, consumed_at, created_at
FROM mcp_oauth_states
WHERE state_hash = $1
FOR UPDATE
`, stateHash))
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthState{}, ErrOAuthStateInvalid
	}
	if err != nil {
		return OAuthState{}, fmt.Errorf("lock mcp oauth state: %w", err)
	}
	if state.ConsumedAt != nil {
		return OAuthState{}, ErrOAuthStateConsumed
	}
	if !now.Before(state.ExpiresAt) {
		return OAuthState{}, ErrOAuthStateExpired
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE mcp_oauth_states SET consumed_at = $2, updated_at = $2 WHERE id = $1
`, state.ID, now); err != nil {
		return OAuthState{}, fmt.Errorf("consume mcp oauth state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return OAuthState{}, fmt.Errorf("commit mcp oauth state: %w", err)
	}
	state.ConsumedAt = &now
	return state, nil
}

func (r *PostgresRepository) CreateCall(
	ctx context.Context,
	userID string,
	call CallRecord,
) (CallRecord, error) {
	if err := r.requireDB(); err != nil {
		return CallRecord{}, err
	}
	if strings.TrimSpace(call.ID) == "" {
		call.ID = r.newID()
	}
	arguments, err := json.Marshal(objectOrEmpty(call.ArgumentsSummary))
	if err != nil {
		return CallRecord{}, fmt.Errorf("marshal mcp call arguments: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO mcp_tool_calls (
  id, user_id, conversation_id, message_id, run_id,
  server_source, server_ref, tool_name, tool_alias, classification,
  status, round_no, call_no, arguments_summary, retained_until
) VALUES ($1, $2, $3, NULLIF($4, '')::uuid, $5::uuid,
  $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15)
`, call.ID, userID, call.ConversationID, call.MessageID, call.RunID,
		call.ServerRef.Source, call.ServerRef.ID, call.ToolName, call.ToolAlias,
		call.Classification, call.Status, call.Round, call.Call, string(arguments), call.RetainUntil); err != nil {
		return CallRecord{}, fmt.Errorf("create mcp tool call: %w", err)
	}
	return call, nil
}

func (r *PostgresRepository) FinishCall(
	ctx context.Context,
	userID string,
	call CallRecord,
	contents []Content,
	objectKeys []string,
	byteSize int64,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin finish mcp tool call: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
UPDATE mcp_tool_calls
SET status = $3,
    result_summary = NULLIF($4, ''),
    error_code = NULLIF($5, ''),
    started_at = $6,
    completed_at = $7,
    duration_ms = $8,
    updated_at = now()
WHERE id = $1 AND user_id = $2
`, call.ID, userID, call.Status, call.ResultSummary, call.ErrorCode,
		call.StartedAt, call.CompletedAt, call.DurationMillis)
	if err != nil {
		return fmt.Errorf("finish mcp tool call: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fmt.Errorf("finish mcp tool call ownership")
	}
	contentJSON, err := json.Marshal(contents)
	if err != nil {
		return fmt.Errorf("marshal mcp tool result content: %w", err)
	}
	keysJSON, err := json.Marshal(objectKeys)
	if err != nil {
		return fmt.Errorf("marshal mcp tool result keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO mcp_tool_results (call_id, content, object_keys, byte_size)
VALUES ($1, $2::jsonb, $3::jsonb, $4)
ON CONFLICT (call_id) DO UPDATE
SET content = EXCLUDED.content,
    object_keys = EXCLUDED.object_keys,
    byte_size = EXCLUDED.byte_size,
    updated_at = now()
`, call.ID, string(contentJSON), string(keysJSON), byteSize); err != nil {
		return fmt.Errorf("upsert mcp tool result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit mcp tool call: %w", err)
	}
	return nil
}

func (r *PostgresRepository) ListCalls(
	ctx context.Context,
	userID string,
	conversationID string,
	runID string,
) ([]CallRecord, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT calls.id, calls.conversation_id, COALESCE(calls.message_id::text, ''),
  calls.run_id, calls.server_source, calls.server_ref, calls.tool_name,
  calls.tool_alias, calls.classification, calls.status, calls.round_no,
  calls.call_no, calls.arguments_summary, COALESCE(calls.result_summary, ''),
  COALESCE(calls.error_code, ''), calls.started_at, calls.completed_at,
  COALESCE(calls.duration_ms, 0)
FROM mcp_tool_calls calls
JOIN conversations conversation
  ON conversation.id = calls.conversation_id
 AND conversation.user_id = $1
 AND conversation.deleted_at IS NULL
WHERE calls.user_id = $1
  AND calls.conversation_id = $2
  AND calls.retained_until > now()
  AND (NULLIF($3, '') IS NULL OR calls.run_id = NULLIF($3, '')::uuid)
ORDER BY calls.created_at, calls.call_no, calls.id
LIMIT 256
`, userID, conversationID, runID)
	if err != nil {
		return nil, fmt.Errorf("list mcp tool calls: %w", err)
	}
	defer rows.Close()
	calls := make([]CallRecord, 0)
	for rows.Next() {
		var call CallRecord
		var arguments []byte
		var started, completed sql.NullTime
		if err := rows.Scan(
			&call.ID, &call.ConversationID, &call.MessageID, &call.RunID,
			&call.ServerRef.Source, &call.ServerRef.ID, &call.ToolName,
			&call.ToolAlias, &call.Classification, &call.Status, &call.Round,
			&call.Call, &arguments, &call.ResultSummary, &call.ErrorCode,
			&started, &completed, &call.DurationMillis,
		); err != nil {
			return nil, fmt.Errorf("scan mcp tool call: %w", err)
		}
		if err := json.Unmarshal(arguments, &call.ArgumentsSummary); err != nil {
			return nil, fmt.Errorf("decode mcp tool call arguments: %w", err)
		}
		if started.Valid {
			value := started.Time.UTC()
			call.StartedAt = &value
		}
		if completed.Valid {
			value := completed.Time.UTC()
			call.CompletedAt = &value
		}
		calls = append(calls, call)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mcp tool calls: %w", err)
	}
	return calls, nil
}

func (r *PostgresRepository) ListConversationObjectKeys(
	ctx context.Context,
	userID string,
	conversationID string,
) ([]string, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT DISTINCT object_key.value
FROM conversations conversation
JOIN mcp_tool_calls call
  ON call.conversation_id = conversation.id
JOIN mcp_tool_results result
  ON result.call_id = call.id
CROSS JOIN LATERAL jsonb_array_elements_text(result.object_keys) AS object_key(value)
WHERE conversation.id = $1
  AND conversation.user_id = $2
  AND conversation.deleted_at IS NULL
ORDER BY object_key.value
`, conversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("list mcp conversation object keys: %w", err)
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan mcp conversation object key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mcp conversation object keys: %w", err)
	}
	return keys, nil
}

func (r *PostgresRepository) DeleteConversationData(
	ctx context.Context,
	userID string,
	conversationID string,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete mcp conversation data: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var authorized bool
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM conversations
  WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
)
`, conversationID, userID).Scan(&authorized); err != nil {
		return fmt.Errorf("authorize mcp conversation cleanup: %w", err)
	}
	if !authorized {
		return ErrSelectionInvalid
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`DELETE FROM mcp_conversation_selections WHERE conversation_id = $1`, []any{conversationID}},
		{`DELETE FROM mcp_run_snapshots WHERE conversation_id = $1`, []any{conversationID}},
		{`DELETE FROM mcp_tool_calls WHERE conversation_id = $1 AND user_id = $2`, []any{conversationID, userID}},
	} {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("delete mcp conversation data: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete mcp conversation data: %w", err)
	}
	return nil
}

var _ ConversationLifecycleRepository = (*PostgresRepository)(nil)

func (r *PostgresRepository) ListPendingArtifacts(ctx context.Context, limit int) ([]PendingArtifact, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT object_key, conversation_id, call_id
FROM mcp_artifact_cleanup_queue
ORDER BY created_at, object_key
LIMIT $1
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending mcp artifacts: %w", err)
	}
	defer rows.Close()
	artifacts := []PendingArtifact{}
	for rows.Next() {
		var artifact PendingArtifact
		if err := rows.Scan(&artifact.ObjectKey, &artifact.ConversationID, &artifact.CallID); err != nil {
			return nil, fmt.Errorf("scan pending mcp artifact: %w", err)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending mcp artifacts: %w", err)
	}
	return artifacts, nil
}

func (r *PostgresRepository) DeletePendingArtifacts(ctx context.Context, objectKeys []string) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	if len(objectKeys) == 0 {
		return nil
	}
	placeholders := make([]string, len(objectKeys))
	arguments := make([]any, len(objectKeys))
	for index, key := range objectKeys {
		placeholders[index] = fmt.Sprintf("$%d", index+1)
		arguments[index] = key
	}
	query := `DELETE FROM mcp_artifact_cleanup_queue WHERE object_key IN (` +
		strings.Join(placeholders, ",") + `)`
	if _, err := r.db.ExecContext(ctx, query, arguments...); err != nil {
		return fmt.Errorf("delete pending mcp artifacts: %w", err)
	}
	return nil
}

var _ ArtifactCleanupRepository = (*PostgresRepository)(nil)

func (r *PostgresRepository) ListExpiredCalls(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]ExpiredCall, error) {
	if err := r.requireDB(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
WITH expired AS (
  SELECT id, conversation_id
  FROM mcp_tool_calls
  WHERE retained_until <= $1
  ORDER BY retained_until, id
  LIMIT $2
)
SELECT expired.id, expired.conversation_id, object_key.value
FROM expired
LEFT JOIN mcp_tool_results result ON result.call_id = expired.id
LEFT JOIN LATERAL jsonb_array_elements_text(
  COALESCE(result.object_keys, '[]'::jsonb)
) AS object_key(value) ON true
ORDER BY expired.id, object_key.value
`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list expired mcp calls: %w", err)
	}
	defer rows.Close()
	calls := []ExpiredCall{}
	byID := map[string]int{}
	for rows.Next() {
		var callID, conversationID string
		var objectKey sql.NullString
		if err := rows.Scan(&callID, &conversationID, &objectKey); err != nil {
			return nil, fmt.Errorf("scan expired mcp call: %w", err)
		}
		index, ok := byID[callID]
		if !ok {
			index = len(calls)
			byID[callID] = index
			calls = append(calls, ExpiredCall{ID: callID, ConversationID: conversationID})
		}
		if objectKey.Valid {
			calls[index].ObjectKeys = append(calls[index].ObjectKeys, objectKey.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expired mcp calls: %w", err)
	}
	return calls, nil
}

func (r *PostgresRepository) DeleteExpiredData(
	ctx context.Context,
	now time.Time,
	callIDs []string,
) error {
	if err := r.requireDB(); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin prune expired mcp data: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if len(callIDs) > 0 {
		placeholders := make([]string, len(callIDs))
		arguments := make([]any, 0, len(callIDs)+1)
		arguments = append(arguments, now)
		for index, callID := range callIDs {
			placeholders[index] = fmt.Sprintf("$%d::uuid", index+2)
			arguments = append(arguments, callID)
		}
		query := `DELETE FROM mcp_tool_calls WHERE retained_until <= $1 AND id IN (` +
			strings.Join(placeholders, ",") + `)`
		if _, err := tx.ExecContext(ctx, query, arguments...); err != nil {
			return fmt.Errorf("delete expired mcp calls: %w", err)
		}
	}
	for _, statement := range []string{
		`DELETE FROM mcp_run_snapshots WHERE expires_at <= $1`,
		`DELETE FROM mcp_oauth_states WHERE expires_at <= $1`,
	} {
		if _, err := tx.ExecContext(ctx, statement, now); err != nil {
			return fmt.Errorf("delete expired mcp state: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit prune expired mcp data: %w", err)
	}
	return nil
}

var _ RetentionRepository = (*PostgresRepository)(nil)

type scanner interface {
	Scan(...any) error
}

type rowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *PostgresRepository) authorizeWorkspace(
	ctx context.Context,
	query rowQuerier,
	userID string,
	workspaceID string,
) error {
	var authorized bool
	err := query.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM workspaces w
  WHERE w.id = $1 AND w.deleted_at IS NULL
    AND (
      w.owner_user_id = $2
      OR EXISTS (
        SELECT 1 FROM workspace_memberships membership
        WHERE membership.workspace_id = w.id
          AND membership.user_id = $2
          AND membership.status = 'active'
      )
    )
)
`, workspaceID, userID).Scan(&authorized)
	if err != nil {
		return fmt.Errorf("authorize mcp workspace: %w", err)
	}
	if !authorized {
		return ErrSelectionInvalid
	}
	return nil
}

func scanSelectionServer(row scanner) (SelectionServer, error) {
	var item SelectionServer
	var disabled []byte
	if err := row.Scan(&item.Ref.Source, &item.Ref.ID, &disabled); err != nil {
		return SelectionServer{}, err
	}
	if err := json.Unmarshal(disabled, &item.DisabledTools); err != nil {
		return SelectionServer{}, err
	}
	if item.DisabledTools == nil {
		item.DisabledTools = []string{}
	}
	return item, nil
}

func scanPrivateServer(row scanner) (Server, error) {
	server, _, err := scanPrivateServerRow(row, false)
	return server, err
}

func scanPrivateServerWithCredential(row scanner) (Server, error) {
	server, credential, err := scanPrivateServerRow(row, true)
	server.HasCredential = credential
	return server, err
}

func scanPrivateServerRow(row scanner, withCredential bool) (Server, bool, error) {
	var server Server
	var authConfig, toolsJSON []byte
	var snapshotHash, errorCode sql.NullString
	var validatedAt sql.NullTime
	var hasCredential bool
	destinations := []any{
		&server.Ref.ID, &server.Name, &server.EndpointURL, &server.Transport,
		&server.AuthType, &authConfig, &server.Status, &toolsJSON, &snapshotHash,
		&validatedAt, &errorCode, &server.CreatedAt, &server.UpdatedAt,
	}
	if withCredential {
		destinations = append(destinations, &hasCredential)
	}
	if err := row.Scan(destinations...); err != nil {
		return Server{}, false, err
	}
	server.Ref.Source = SourcePrivate
	server.LastErrorCode = errorCode.String
	if validatedAt.Valid {
		value := validatedAt.Time.UTC()
		server.ValidatedAt = &value
	}
	server.CreatedAt = server.CreatedAt.UTC()
	server.UpdatedAt = server.UpdatedAt.UTC()
	var auth map[string]any
	if err := json.Unmarshal(authConfig, &auth); err != nil {
		return Server{}, false, err
	}
	if server.AuthType == AuthHeader {
		server.HeaderAuth = &HeaderAuth{Name: stringField(auth, "headerName")}
	}
	if server.AuthType == AuthOAuth {
		server.OAuthClient = &OAuthClient{
			ClientID: stringField(auth, "clientId"),
			Scopes:   stringSliceField(auth, "scopes"),
		}
	}
	if err := json.Unmarshal(toolsJSON, &server.Tools); err != nil {
		return Server{}, false, err
	}
	for _, tool := range server.Tools {
		if tool.Supported {
			server.ToolCount++
		} else {
			server.UnsupportedCount++
		}
	}
	_ = snapshotHash
	return server, hasCredential, nil
}

func scanCredential(row scanner) (Credential, error) {
	var credential Credential
	var metadata []byte
	var expiry sql.NullTime
	if err := row.Scan(
		&credential.ID, &credential.UserID, &credential.ServerRef.Source,
		&credential.ServerRef.ID, &credential.Kind,
		&credential.EncryptedSecretRef, &metadata, &expiry,
		&credential.CreatedAt, &credential.UpdatedAt,
	); err != nil {
		return Credential{}, err
	}
	if err := json.Unmarshal(metadata, &credential.Metadata); err != nil {
		return Credential{}, err
	}
	if expiry.Valid {
		value := expiry.Time.UTC()
		credential.ExpiresAt = &value
	}
	credential.CreatedAt = credential.CreatedAt.UTC()
	credential.UpdatedAt = credential.UpdatedAt.UTC()
	return credential, nil
}

func scanOAuthState(row scanner) (OAuthState, error) {
	var state OAuthState
	var consumed sql.NullTime
	if err := row.Scan(
		&state.ID, &state.StateHash, &state.UserID, &state.ServerRef.Source,
		&state.ServerRef.ID,
		&state.EncryptedFlowRef, &state.ReturnURL, &state.ExpiresAt,
		&consumed, &state.CreatedAt,
	); err != nil {
		return OAuthState{}, err
	}
	state.ExpiresAt = state.ExpiresAt.UTC()
	state.CreatedAt = state.CreatedAt.UTC()
	if consumed.Valid {
		value := consumed.Time.UTC()
		state.ConsumedAt = &value
	}
	return state, nil
}

func (r *PostgresRepository) requireDB() error {
	if r == nil || r.db == nil {
		return ErrRepositoryRequired
	}
	return nil
}

func objectOrEmpty(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func stringField(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return strings.TrimSpace(result)
}

func stringSliceField(value map[string]any, key string) []string {
	raw, _ := value[key].([]any)
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}
