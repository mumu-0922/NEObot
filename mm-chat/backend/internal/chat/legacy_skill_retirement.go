package chat

const retiredLegacySkillSelectionKey = "activeSkills"

func stripRetiredLegacySkillSelection(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	if _, exists := metadata[retiredLegacySkillSelectionKey]; !exists {
		return metadata
	}
	sanitized := make(map[string]any, len(metadata)-1)
	for key, value := range metadata {
		if key != retiredLegacySkillSelectionKey {
			sanitized[key] = value
		}
	}
	return sanitized
}

func appendRetiredLegacySkillDeleteKey(keys []string) []string {
	for _, key := range keys {
		if key == retiredLegacySkillSelectionKey {
			return keys
		}
	}
	return append(keys, retiredLegacySkillSelectionKey)
}

func stripRetiredLegacySkillConversation(conversation Conversation) Conversation {
	conversation.Metadata = stripRetiredLegacySkillSelection(conversation.Metadata)
	return conversation
}
