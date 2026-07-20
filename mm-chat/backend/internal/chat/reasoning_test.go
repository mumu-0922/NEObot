package chat

import "testing"

func TestReasoningEffortFromConfigPreservesLegacyHighAndRejectsUnknown(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		enabled bool
		want    ReasoningEffort
	}{
		{name: "disabled", config: map[string]any{"reasoningEffort": "max"}, want: ""},
		{name: "legacy enabled", config: map[string]any{}, enabled: true, want: ReasoningEffortHigh},
		{name: "normalized", config: map[string]any{"reasoningEffort": " XHIGH "}, enabled: true, want: ReasoningEffortXHigh},
		{name: "unknown", config: map[string]any{"reasoningEffort": "unbounded"}, enabled: true, want: ReasoningEffortHigh},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := reasoningEffortFromConfig(test.config, test.enabled); got != test.want {
				t.Fatalf("reasoningEffortFromConfig() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestOpenAIReasoningEffortNormalizesModelSpecificLevels(t *testing.T) {
	tests := []struct {
		name      string
		modelID   string
		enabled   bool
		requested ReasoningEffort
		want      string
	}{
		{name: "disabled", modelID: "gpt-5.6", requested: ReasoningEffortMax, want: ""},
		{name: "auto omitted", modelID: "gpt-5.6", enabled: true, requested: ReasoningEffortAuto, want: ""},
		{name: "gpt 5.6 max", modelID: "gpt-5.6-sol", enabled: true, requested: ReasoningEffortMax, want: "max"},
		{name: "gpt 5.4 max clamps xhigh", modelID: "gpt-5.4", enabled: true, requested: ReasoningEffortMax, want: "xhigh"},
		{name: "unknown xhigh clamps high", modelID: "reasoning-model", enabled: true, requested: ReasoningEffortXHigh, want: "high"},
		{name: "low portable", modelID: "reasoning-model", enabled: true, requested: ReasoningEffortLow, want: "low"},
		{name: "missing legacy high", modelID: "reasoning-model", enabled: true, want: "high"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := openAIReasoningEffort(test.modelID, test.enabled, test.requested); got != test.want {
				t.Fatalf("openAIReasoningEffort() = %q, want %q", got, test.want)
			}
		})
	}
}
