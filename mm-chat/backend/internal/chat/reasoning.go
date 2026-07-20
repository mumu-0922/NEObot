package chat

import "strings"

type ReasoningEffort string

const (
	ReasoningEffortAuto   ReasoningEffort = "auto"
	ReasoningEffortLow    ReasoningEffort = "low"
	ReasoningEffortMedium ReasoningEffort = "medium"
	ReasoningEffortHigh   ReasoningEffort = "high"
	ReasoningEffortXHigh  ReasoningEffort = "xhigh"
	ReasoningEffortMax    ReasoningEffort = "max"
)

func reasoningEffortFromConfig(config map[string]any, enabled bool) ReasoningEffort {
	if !enabled {
		return ""
	}
	value, _ := config["reasoningEffort"].(string)
	if effort, ok := normalizeReasoningEffort(value); ok {
		return effort
	}
	return ReasoningEffortHigh
}

func effectiveReasoningEffort(input ProviderRequest) ReasoningEffort {
	if !input.UseReasoning {
		return ""
	}
	if effort, ok := normalizeReasoningEffort(string(input.ReasoningEffort)); ok {
		return effort
	}
	return ReasoningEffortHigh
}

func normalizeReasoningEffort(value string) (ReasoningEffort, bool) {
	effort := ReasoningEffort(strings.ToLower(strings.TrimSpace(value)))
	switch effort {
	case ReasoningEffortAuto,
		ReasoningEffortLow,
		ReasoningEffortMedium,
		ReasoningEffortHigh,
		ReasoningEffortXHigh,
		ReasoningEffortMax:
		return effort, true
	default:
		return "", false
	}
}

func openAIReasoningEffort(
	modelID string,
	enabled bool,
	requested ReasoningEffort,
) string {
	if !enabled {
		return ""
	}
	effort, ok := normalizeReasoningEffort(string(requested))
	if !ok {
		effort = ReasoningEffortHigh
	}
	if effort == ReasoningEffortAuto {
		return ""
	}
	if effort == ReasoningEffortMax && !openAIModelSupportsMaxReasoning(modelID) {
		if openAIModelSupportsXHighReasoning(modelID) {
			return string(ReasoningEffortXHigh)
		}
		return string(ReasoningEffortHigh)
	}
	if effort == ReasoningEffortXHigh && !openAIModelSupportsXHighReasoning(modelID) {
		return string(ReasoningEffortHigh)
	}
	return string(effort)
}

func openAIModelSupportsXHighReasoning(modelID string) bool {
	model := strings.ToLower(strings.TrimSpace(modelID))
	return strings.Contains(model, "deepseek") ||
		openAIModelFamily(model, "gpt-5.2") ||
		openAIModelFamily(model, "gpt-5.3") ||
		openAIModelFamily(model, "gpt-5.4") ||
		openAIModelFamily(model, "gpt-5.5") ||
		openAIModelFamily(model, "gpt-5.6")
}

func openAIModelSupportsMaxReasoning(modelID string) bool {
	return openAIModelFamily(strings.ToLower(strings.TrimSpace(modelID)), "gpt-5.6")
}

func openAIModelFamily(modelID string, family string) bool {
	return modelID == family ||
		strings.HasPrefix(modelID, family+"-") ||
		strings.HasPrefix(modelID, family+".")
}

func anthropicThinkingBudget(effort ReasoningEffort) int {
	switch effort {
	case ReasoningEffortLow:
		return 1_024
	case ReasoningEffortMedium:
		return 2_048
	case ReasoningEffortXHigh:
		return 8_192
	case ReasoningEffortMax:
		return 16_384
	case ReasoningEffortAuto, ReasoningEffortHigh:
		return defaultAnthropicThinkingTokens
	default:
		return defaultAnthropicThinkingTokens
	}
}

func anthropicMaxTokens(budgetTokens int) int {
	const answerReserveTokens = 4_096
	if required := budgetTokens + answerReserveTokens; required > defaultAnthropicMaxTokens {
		return required
	}
	return defaultAnthropicMaxTokens
}
