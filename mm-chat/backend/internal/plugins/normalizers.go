package plugins

import "strings"

const (
	agnesImagePluginID = "agnes-image-generation"
	agnesVideoPluginID = "agnes-video-generation"
	unsplashPluginID   = "unsplash"
)

func normalizePluginResponse(plugin *Plugin, responseData any) any {
	if plugin == nil {
		return responseData
	}

	switch plugin.ID {
	case "jina-web-reader":
		return normalizeJinaWebReaderResponse(responseData)
	case agnesImagePluginID:
		return normalizeAgnesImageResponse(responseData)
	case agnesVideoPluginID:
		return normalizeAgnesVideoResponse(responseData)
	case unsplashPluginID:
		return normalizeUnsplashResponse(responseData)
	default:
		return responseData
	}
}

func normalizeJinaWebReaderResponse(responseData any) any {
	record, ok := asRecord(responseData)
	if !ok || !numberEquals(record["code"], 200) {
		return responseData
	}
	data, ok := asRecord(record["data"])
	if !ok {
		return responseData
	}
	content := stringValue(data["content"])
	if content == "" {
		return responseData
	}
	return content
}

func normalizeAgnesImageResponse(responseData any) any {
	record, ok := asRecord(responseData)
	if !ok {
		return responseData
	}
	items, ok := record["data"].([]any)
	if !ok {
		return responseData
	}
	var first map[string]any
	for _, item := range items {
		if candidate, ok := asRecord(item); ok {
			first = candidate
			break
		}
	}
	return map[string]any{
		"imageUrl":      nullableString(first, "url"),
		"imageBase64":   nullableString(first, "b64_json"),
		"revisedPrompt": nullableString(first, "revised_prompt"),
		"raw":           responseData,
	}
}

func normalizeAgnesVideoResponse(responseData any) any {
	record, ok := asRecord(responseData)
	if !ok {
		return responseData
	}
	return map[string]any{
		"taskId":           firstNullableString(record, "task_id", "id"),
		"videoId":          nullableString(record, "video_id"),
		"status":           nullableString(record, "status"),
		"generationStatus": agnesVideoGenerationStatus(record),
		"progress":         nullableNumber(record, "progress"),
		"seconds":          nullableStringOrNumber(record, "seconds"),
		"size":             nullableString(record, "size"),
		"videoUrl":         agnesVideoURL(record),
		"error":            nullableExistingValue(record, "error"),
		"raw":              responseData,
	}
}

func normalizeUnsplashResponse(responseData any) any {
	record, ok := asRecord(responseData)
	if !ok {
		return responseData
	}
	items, ok := record["results"].([]any)
	if !ok {
		return responseData
	}
	normalized := make([]any, 0, len(items))
	for _, item := range items {
		result, ok := asRecord(item)
		if !ok {
			continue
		}
		urls, _ := asRecord(result["urls"])
		normalized = append(normalized, map[string]any{
			"alt_description": result["alt_description"],
			"created_at":      result["created_at"],
			"likes":           result["likes"],
			"url":             nullableString(urls, "regular"),
		})
	}
	return normalized
}

func agnesVideoGenerationStatus(responseData map[string]any) string {
	status := strings.ToLower(stringValue(responseData["status"]))
	if hasAgnesVideoError(responseData) {
		return "failed"
	}
	switch status {
	case "failed", "error", "cancelled", "canceled":
		return "failed"
	case "completed":
		return "generated"
	}
	if agnesVideoURL(responseData) != nil {
		return "generated"
	}
	return "generating"
}

func hasAgnesVideoError(responseData map[string]any) bool {
	value, ok := responseData["error"]
	if !ok || value == nil {
		return false
	}
	if text, ok := value.(string); ok {
		return text != ""
	}
	return true
}

func agnesVideoURL(responseData map[string]any) any {
	for _, key := range []string{"remixed_from_video_id", "video_url"} {
		if value := stringValue(responseData[key]); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return nil
}

func firstNullableString(record map[string]any, keys ...string) any {
	for _, key := range keys {
		if value := nullableString(record, key); value != nil {
			return value
		}
	}
	return nil
}

func nullableString(record map[string]any, key string) any {
	if record == nil {
		return nil
	}
	if value, ok := record[key].(string); ok {
		return value
	}
	return nil
}

func nullableNumber(record map[string]any, key string) any {
	if record == nil {
		return nil
	}
	value := record[key]
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return value
	default:
		return nil
	}
}

func nullableStringOrNumber(record map[string]any, key string) any {
	if value := nullableString(record, key); value != nil {
		return value
	}
	return nullableNumber(record, key)
}

func nullableExistingValue(record map[string]any, key string) any {
	if record == nil {
		return nil
	}
	if value, ok := record[key]; ok {
		return value
	}
	return nil
}

func numberEquals(value any, expected float64) bool {
	switch typed := value.(type) {
	case int:
		return float64(typed) == expected
	case int8:
		return float64(typed) == expected
	case int16:
		return float64(typed) == expected
	case int32:
		return float64(typed) == expected
	case int64:
		return float64(typed) == expected
	case uint:
		return float64(typed) == expected
	case uint8:
		return float64(typed) == expected
	case uint16:
		return float64(typed) == expected
	case uint32:
		return float64(typed) == expected
	case uint64:
		return float64(typed) == expected
	case float32:
		return float64(typed) == expected
	case float64:
		return typed == expected
	default:
		return false
	}
}
