package chat

import "strings"

const treeParentMessageIDMetadataKey = "treeParentMessageId"

// buildProviderConversationMessages reconstructs the same effective branch
// that the frontend renders. It preserves explicit branches while keeping old
// rows without parent_message_id usable as one linear active path.
func buildProviderConversationMessages(
	messages []Message,
	anchorMessageID string,
	currentPrompt string,
	currentAttachments []ProviderAttachment,
) []ProviderMessage {
	parents := effectiveConversationParents(messages)
	byID := make(map[string]Message, len(messages))
	for _, message := range messages {
		byID[message.ID] = message
	}

	path := make([]Message, 0, len(messages))
	visited := make(map[string]struct{}, len(messages))
	for messageID := strings.TrimSpace(anchorMessageID); messageID != ""; messageID = parents[messageID] {
		if _, seen := visited[messageID]; seen {
			break
		}
		message, ok := byID[messageID]
		if !ok {
			break
		}
		visited[messageID] = struct{}{}
		path = append(path, message)
	}

	providerMessages := make([]ProviderMessage, 0, len(path))
	for index := len(path) - 1; index >= 0; index-- {
		message := path[index]
		if message.Role != "user" && message.Role != "assistant" {
			continue
		}
		content := message.Content
		var attachments []ProviderAttachment
		if message.ID == anchorMessageID {
			content = currentPrompt
			attachments = currentAttachments
		}
		if strings.TrimSpace(content) == "" && len(attachments) == 0 {
			continue
		}
		providerMessages = append(providerMessages, ProviderMessage{
			MessageID:   message.ID,
			Role:        message.Role,
			Content:     content,
			Attachments: attachments,
		})
	}

	return providerMessages
}

func effectiveConversationParents(messages []Message) map[string]string {
	parents := make(map[string]string, len(messages))
	known := make(map[string]struct{}, len(messages))
	activeChildren := make(map[string]string, len(messages))
	activeRoot := ""

	for _, message := range messages {
		messageID := strings.TrimSpace(message.ID)
		if messageID == "" {
			continue
		}

		parentID, explicit := explicitTreeParent(message.Metadata)
		if !explicit {
			parentID = strings.TrimSpace(message.ParentMessageID)
			explicit = parentID != ""
		}
		if explicit {
			if _, ok := known[parentID]; parentID == "" || !ok {
				parentID = ""
			}
		} else {
			parentID = activeConversationLeaf(activeRoot, activeChildren)
		}

		parents[messageID] = parentID
		known[messageID] = struct{}{}
		if parentID == "" {
			activeRoot = messageID
		} else {
			activeChildren[parentID] = messageID
		}
	}

	return parents
}

func explicitTreeParent(metadata map[string]any) (string, bool) {
	value, exists := metadata[treeParentMessageIDMetadataKey]
	if !exists {
		return "", false
	}
	if value == nil {
		return "", true
	}
	parentID, ok := value.(string)
	if !ok || strings.TrimSpace(parentID) == "" {
		return "", false
	}
	return strings.TrimSpace(parentID), true
}

func activeConversationLeaf(root string, activeChildren map[string]string) string {
	visited := map[string]struct{}{}
	current := root
	for current != "" {
		if _, seen := visited[current]; seen {
			return ""
		}
		visited[current] = struct{}{}
		next := activeChildren[current]
		if next == "" {
			return current
		}
		current = next
	}
	return ""
}
