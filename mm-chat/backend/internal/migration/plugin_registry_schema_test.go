package migration

import "testing"

func TestPluginRegistrySchemaContract(t *testing.T) {
	up := readPhase15SQL(t, "011_plugin_registry.up.sql")
	down := readPhase15SQL(t, "011_plugin_registry.down.sql")

	body := mustPhase15TableBody(t, up, "plugin_registry")
	assertPhase15Columns(t, body, "plugin_registry",
		"plugin_id", "plugin", "installed_by_user_id", "built_in",
		"created_at", "updated_at")
	assertPhase15Fragments(t, body,
		"plugin registry must store canonical plugin JSON objects",
		"plugin jsonb not null", "jsonb_typeof", "plugin", "'object'",
		"plugin->>'id' = plugin_id")
	assertPhase15ReferenceOnDeleteRestrict(t, body,
		"installed_by_user_id", "users",
		"installed plugins must retain an auditable installing user")
	assertPhase15Fragments(t, up,
		"plugin registry must index installing user and built-in state",
		"idx_plugin_registry_installed_by_user_id", "idx_plugin_registry_built_in")
	if !phase15DropsTable(down, "plugin_registry") {
		t.Fatal("plugin registry down migration must drop plugin_registry")
	}
}
