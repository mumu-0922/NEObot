-- Durable plugin registry for server-mode plugin execution.

CREATE TABLE plugin_registry (
  plugin_id TEXT PRIMARY KEY,
  plugin JSONB NOT NULL,
  installed_by_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
  built_in BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT plugin_registry_id_not_blank CHECK (length(trim(plugin_id)) > 0),
  CONSTRAINT plugin_registry_json_object CHECK (jsonb_typeof(plugin) = 'object'),
  CONSTRAINT plugin_registry_json_id_matches CHECK (plugin->>'id' = plugin_id),
  CONSTRAINT plugin_registry_timestamps_order CHECK (updated_at >= created_at)
);

CREATE INDEX idx_plugin_registry_installed_by_user_id
  ON plugin_registry(installed_by_user_id)
  WHERE installed_by_user_id IS NOT NULL;

CREATE INDEX idx_plugin_registry_built_in
  ON plugin_registry(built_in);
