package chat

import "testing"

func TestSearchModeFromConfigPrefersThreeStateModeAndMapsLegacyToggle(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
		want   chatSearchMode
	}{
		{name: "off wins legacy true", config: map[string]any{"searchMode": "off", "useSearch": true}, want: chatSearchModeOff},
		{name: "built in", config: map[string]any{"searchMode": "model_builtin"}, want: chatSearchModeModelBuiltIn},
		{name: "external", config: map[string]any{"searchMode": "external"}, want: chatSearchModeExternal},
		{name: "legacy true", config: map[string]any{"useSearch": true}, want: chatSearchModeExternal},
		{name: "legacy false", config: map[string]any{"useSearch": false}, want: chatSearchModeOff},
		{name: "invalid", config: map[string]any{"searchMode": "invalid"}, want: chatSearchModeOff},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := searchModeFromConfig(test.config); got != test.want {
				t.Fatalf("searchModeFromConfig() = %q, want %q", got, test.want)
			}
		})
	}
}
