package plugins

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"

	"neo-chat/mm-chat/backend/internal/auth"
)

type PostgresAuditRecorder struct {
	db *sql.DB
}

func NewPostgresAuditRecorder(db *sql.DB) *PostgresAuditRecorder {
	return &PostgresAuditRecorder{db: db}
}

func (r *PostgresAuditRecorder) RecordPluginEvent(ctx context.Context, event AuditEvent) error {
	if r == nil || r.db == nil {
		return ErrPluginAuditUnavailable
	}
	event = NormalizeAuditEvent(ctx, event)
	id, err := newAuditUUID()
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(pluginAuditMetadata(event))
	if err != nil {
		return fmt.Errorf("marshal plugin audit metadata: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin plugin audit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	user := authUserFromAuditEvent(ctx, event)
	if err := ensureUser(ctx, tx, user); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO audit_logs (
  id,
  actor_user_id,
  actor_type,
  action,
  resource_type,
  request_id,
  outcome,
  ip_address,
  user_agent,
  metadata
)
VALUES ($1, $2, 'user', $3, 'plugin', $4, $5, NULLIF($6, '')::inet, NULLIF($7, ''), $8::jsonb)
`, id, user.ID, event.Action, nullIfBlank(event.RequestID), auditOutcome(event), event.IPAddress, event.UserAgent, string(metadata)); err != nil {
		return fmt.Errorf("insert plugin audit log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit plugin audit: %w", err)
	}
	return nil
}

func pluginAuditMetadata(event AuditEvent) map[string]any {
	metadata := map[string]any{
		"status":        event.Status,
		"pluginId":      event.PluginID,
		"source":        event.Source,
		"builtIn":       event.BuiltIn,
		"hasAuth":       event.HasAuth,
		"functionCount": event.FunctionCount,
		"argumentCount": event.ArgumentCount,
	}
	if event.FunctionName != "" {
		metadata["functionName"] = event.FunctionName
	}
	if event.CallID != "" {
		metadata["callId"] = event.CallID
	}
	if event.BaseHost != "" {
		metadata["baseHost"] = event.BaseHost
	}
	if event.ManifestHost != "" {
		metadata["manifestHost"] = event.ManifestHost
	}
	return metadata
}

func auditOutcome(event AuditEvent) string {
	switch event.Status {
	case AuditStatusAdmitted:
		return "success"
	default:
		return "success"
	}
}

func authUserFromAuditEvent(ctx context.Context, event AuditEvent) auth.User {
	user := auth.UserOrDevelopment(ctx)
	if event.UserID != "" {
		user.ID = event.UserID
	}
	return auth.UserOrDevelopment(auth.WithUser(context.Background(), user))
}

func nullIfBlank(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func newAuditUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate audit uuid: %w", err)
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}
