package chat

type chatToolMode string

const (
	chatToolModeChat  chatToolMode = "chat"
	chatToolModeAgent chatToolMode = "agent"
)

func requestedChatToolMode(config map[string]any) chatToolMode {
	if value, ok := config["toolMode"].(string); ok && value == string(chatToolModeChat) {
		return chatToolModeChat
	}
	return chatToolModeAgent
}

func effectiveChatToolMode(config map[string]any, toolRoundCapable bool) chatToolMode {
	requested := requestedChatToolMode(config)
	if requested == chatToolModeAgent && !toolRoundCapable {
		return chatToolModeChat
	}
	return requested
}
