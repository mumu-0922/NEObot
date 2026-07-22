package chat

import "strings"

type chatSearchMode string

const (
	chatSearchModeOff          chatSearchMode = "off"
	chatSearchModeModelBuiltIn chatSearchMode = "model_builtin"
	chatSearchModeExternal     chatSearchMode = "external"
)

func searchModeFromConfig(config map[string]any) chatSearchMode {
	if value, ok := config["searchMode"].(string); ok {
		switch chatSearchMode(strings.TrimSpace(value)) {
		case chatSearchModeOff:
			return chatSearchModeOff
		case chatSearchModeModelBuiltIn:
			return chatSearchModeModelBuiltIn
		case chatSearchModeExternal:
			return chatSearchModeExternal
		}
		return chatSearchModeOff
	}
	if configBool(config, "useSearch") {
		return chatSearchModeExternal
	}
	return chatSearchModeOff
}

func (mode chatSearchMode) enabled() bool {
	return mode != chatSearchModeOff
}
