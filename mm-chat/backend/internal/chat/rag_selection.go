package chat

import "strings"

type ragSelection struct {
	Enabled       bool
	Strict        bool
	CollectionIDs []string
}

func extractRAGSelection(config map[string]any, metadata map[string]any) (ragSelection, error) {
	selection := ragSelection{Strict: ragStrictEnabled(config) || ragStrictEnabled(metadata)}
	for _, source := range []map[string]any{config, metadata} {
		ids, ok, err := collectionIDsFromObject(source)
		if err != nil {
			return ragSelection{}, err
		}
		if ok {
			selection.CollectionIDs = ids
			selection.Enabled = len(ids) > 0
			return selection, nil
		}
	}
	return selection, nil
}

func ragStrictEnabled(values map[string]any) bool {
	if values == nil {
		return false
	}
	for _, key := range []string{"knowledgeStrict", "strictKnowledge", "ragStrict", "strictRag"} {
		if value, ok := values[key].(bool); ok && value {
			return true
		}
	}
	return false
}

func collectionIDsFromObject(values map[string]any) ([]string, bool, error) {
	if values == nil {
		return nil, false, nil
	}
	for _, key := range []string{
		"knowledgeCollectionIds",
		"selectedKnowledgeCollectionIds",
		"selectedCollectionIds",
		"ragCollectionIds",
	} {
		if raw, ok := values[key]; ok {
			ids, err := normalizeRAGCollectionIDs(raw)
			return ids, true, err
		}
	}
	if raw, ok := values["knowledgeCollectionId"]; ok {
		ids, err := normalizeRAGCollectionIDs([]any{raw})
		return ids, true, err
	}
	return nil, false, nil
}

func normalizeRAGCollectionIDs(raw any) ([]string, error) {
	var values []any
	switch typed := raw.(type) {
	case []any:
		values = typed
	case []string:
		values = make([]any, 0, len(typed))
		for _, value := range typed {
			values = append(values, value)
		}
	case string:
		values = []any{typed}
	default:
		return nil, newValidationError("INVALID_RAG_SELECTION", "knowledge collection ids must be strings")
	}
	if len(values) > 32 {
		return nil, newValidationError("INVALID_RAG_SELECTION", "too many selected knowledge collections")
	}
	ids := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		id, ok := value.(string)
		if !ok {
			return nil, newValidationError("INVALID_RAG_SELECTION", "knowledge collection ids must be strings")
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !isUUID(id) {
			return nil, newValidationError("INVALID_RAG_SELECTION", "knowledge collection ids must be UUIDs")
		}
		key := strings.ToLower(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}
