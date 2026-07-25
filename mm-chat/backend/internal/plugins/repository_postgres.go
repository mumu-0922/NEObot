package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

type PostgresRegistry struct {
	db       *sql.DB
	builtIns Registry
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewPostgresRegistry(db *sql.DB, builtIns ...Plugin) *PostgresRegistry {
	return &PostgresRegistry{
		db:       db,
		builtIns: NewMemoryRegistry(builtIns...),
	}
}

func (r *PostgresRegistry) Save(ctx context.Context, plugin Plugin) error {
	if isRetiredPluginID(plugin.ID) {
		return ErrPluginReservedID
	}
	if r == nil || r.db == nil {
		return ErrPluginRegistryUnavailable
	}
	if _, ok, err := r.builtIns.Get(ctx, plugin.ID); err != nil {
		return err
	} else if ok {
		return ErrPluginReservedID
	}
	payload, err := json.Marshal(plugin)
	if err != nil {
		return ErrPluginExecutionPayload
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin plugin registry save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	user := auth.UserOrDevelopment(ctx)
	if err := ensureUser(ctx, tx, user); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO plugin_registry (plugin_id, plugin, installed_by_user_id, built_in)
VALUES ($1, $2::jsonb, $3, false)
ON CONFLICT (plugin_id) DO UPDATE SET
  plugin = EXCLUDED.plugin,
  installed_by_user_id = EXCLUDED.installed_by_user_id,
  built_in = false,
  updated_at = now()
`, strings.TrimSpace(plugin.ID), string(payload), user.ID); err != nil {
		return fmt.Errorf("upsert plugin registry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit plugin registry save: %w", err)
	}
	return nil
}

func (r *PostgresRegistry) Get(ctx context.Context, pluginID string) (Plugin, bool, error) {
	pluginID = strings.TrimSpace(pluginID)
	if isRetiredPluginID(pluginID) {
		return Plugin{}, false, nil
	}
	if r == nil || r.db == nil {
		return r.getBuiltIn(ctx, pluginID)
	}
	if plugin, ok, err := r.getBuiltIn(ctx, pluginID); err != nil || ok {
		return plugin, ok, err
	}

	var payload []byte
	err := r.db.QueryRowContext(ctx, `
SELECT plugin
FROM plugin_registry
WHERE plugin_id = $1
`, pluginID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Plugin{}, false, nil
	}
	if err != nil {
		return Plugin{}, false, fmt.Errorf("query plugin registry: %w", err)
	}
	plugin, err := unmarshalPlugin(payload)
	if err != nil {
		return Plugin{}, false, err
	}
	return plugin, true, nil
}

func (r *PostgresRegistry) List(ctx context.Context) ([]Plugin, error) {
	if r == nil {
		return nil, ErrPluginRegistryUnavailable
	}
	builtIns, err := r.builtIns.List(ctx)
	if err != nil {
		return nil, err
	}
	if r == nil || r.db == nil {
		return builtIns, nil
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT plugin
FROM plugin_registry
WHERE lower(trim(plugin_id)) <> lower($1)
ORDER BY lower(plugin_id)
`, retiredJinaPluginID)
	if err != nil {
		return nil, fmt.Errorf("list plugin registry: %w", err)
	}
	defer rows.Close()

	plugins := make([]Plugin, 0, len(builtIns))
	seen := map[string]struct{}{}
	for _, plugin := range builtIns {
		plugins = append(plugins, plugin)
		seen[plugin.ID] = struct{}{}
	}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan plugin registry: %w", err)
		}
		plugin, err := unmarshalPlugin(payload)
		if err != nil {
			return nil, err
		}
		if isRetiredPluginID(plugin.ID) {
			continue
		}
		if _, ok := seen[plugin.ID]; ok {
			continue
		}
		plugins = append(plugins, plugin)
		seen[plugin.ID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugin registry: %w", err)
	}
	return plugins, nil
}

func (r *PostgresRegistry) getBuiltIn(ctx context.Context, pluginID string) (Plugin, bool, error) {
	if r == nil || r.builtIns == nil {
		return Plugin{}, false, nil
	}
	return r.builtIns.Get(ctx, pluginID)
}

func ensureUser(ctx context.Context, execer sqlExecer, user auth.User) error {
	user = auth.UserOrDevelopment(auth.WithUser(context.Background(), user))
	_, err := execer.ExecContext(ctx, `
INSERT INTO users (id, display_name)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING
`, user.ID, user.DisplayName)
	if err != nil {
		return fmt.Errorf("ensure plugin registry user: %w", err)
	}
	return nil
}

func unmarshalPlugin(payload []byte) (Plugin, error) {
	var plugin Plugin
	if err := json.Unmarshal(payload, &plugin); err != nil {
		return Plugin{}, fmt.Errorf("decode plugin registry payload: %w", err)
	}
	return plugin, nil
}
