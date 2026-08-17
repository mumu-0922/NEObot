package chat

import "testing"

func TestChatToolModeCompatibilityAndCapabilityDowngrade(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]any
		toolCapable bool
		wantRequest chatToolMode
		wantMode    chatToolMode
	}{
		{name: "legacy defaults Agent", config: nil, toolCapable: true, wantRequest: chatToolModeAgent, wantMode: chatToolModeAgent},
		{name: "explicit Agent", config: map[string]any{"toolMode": "agent"}, toolCapable: true, wantRequest: chatToolModeAgent, wantMode: chatToolModeAgent},
		{name: "unsupported Agent downgrades", config: map[string]any{"toolMode": "agent"}, toolCapable: false, wantRequest: chatToolModeAgent, wantMode: chatToolModeChat},
		{name: "explicit Chat", config: map[string]any{"toolMode": "chat"}, toolCapable: true, wantRequest: chatToolModeChat, wantMode: chatToolModeChat},
		{name: "invalid stays compatible", config: map[string]any{"toolMode": "invalid"}, toolCapable: true, wantRequest: chatToolModeAgent, wantMode: chatToolModeAgent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := requestedChatToolMode(test.config); got != test.wantRequest {
				t.Fatalf("requestedChatToolMode() = %q, want %q", got, test.wantRequest)
			}
			if got := effectiveChatToolMode(test.config, test.toolCapable); got != test.wantMode {
				t.Fatalf("effectiveChatToolMode() = %q, want %q", got, test.wantMode)
			}
		})
	}
}
