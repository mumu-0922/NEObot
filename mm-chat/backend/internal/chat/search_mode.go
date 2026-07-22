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

func normalizeConversationSearchMetadata(
	metadata map[string]any,
	fallback *chatSearchMode,
) error {
	if metadata == nil {
		return nil
	}
	value, hasMode := metadata["searchMode"]
	legacy, hasLegacy := metadata["useSearch"]
	if !hasMode && !hasLegacy {
		if fallback != nil {
			metadata["searchMode"] = string(*fallback)
			metadata["useSearch"] = fallback.enabled()
		}
		return nil
	}
	mode := chatSearchModeOff
	if hasMode {
		text, ok := value.(string)
		if !ok {
			return newValidationError("INVALID_SEARCH_MODE", "searchMode is invalid")
		}
		switch chatSearchMode(strings.TrimSpace(text)) {
		case chatSearchModeOff:
			mode = chatSearchModeOff
		case chatSearchModeModelBuiltIn:
			mode = chatSearchModeModelBuiltIn
		case chatSearchModeExternal:
			mode = chatSearchModeExternal
		default:
			return newValidationError("INVALID_SEARCH_MODE", "searchMode is invalid")
		}
	} else {
		enabled, ok := legacy.(bool)
		if !ok {
			return newValidationError("INVALID_SEARCH_MODE", "useSearch is invalid")
		}
		if enabled {
			mode = chatSearchModeExternal
		}
	}
	metadata["searchMode"] = string(mode)
	metadata["useSearch"] = mode.enabled()
	return nil
}

func inheritedConversationSearchMode(conversations []Conversation) chatSearchMode {
	for _, conversation := range conversations {
		metadata := conversation.Metadata
		if metadata == nil {
			continue
		}
		if value, ok := metadata["searchMode"].(string); ok {
			switch chatSearchMode(strings.TrimSpace(value)) {
			case chatSearchModeOff:
				return chatSearchModeOff
			case chatSearchModeModelBuiltIn:
				return chatSearchModeModelBuiltIn
			case chatSearchModeExternal:
				return chatSearchModeExternal
			}
		}
		if legacy, ok := metadata["useSearch"].(bool); ok {
			if legacy {
				return chatSearchModeExternal
			}
			return chatSearchModeOff
		}
	}
	return chatSearchModeOff
}
