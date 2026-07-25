package plugins

import (
	"reflect"
	"testing"
)

func TestNormalizeAgnesImageResponse(t *testing.T) {
	response := map[string]any{
		"data": []any{map[string]any{
			"url":            "https://storage.example/image.png",
			"b64_json":       "base64",
			"revised_prompt": "revised",
		}},
	}

	result := normalizePluginResponse(&Plugin{ID: agnesImagePluginID}, response)

	want := map[string]any{
		"imageUrl":      "https://storage.example/image.png",
		"imageBase64":   "base64",
		"revisedPrompt": "revised",
		"raw":           response,
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %#v, want %#v", result, want)
	}
}

func TestNormalizeAgnesVideoResponse(t *testing.T) {
	response := map[string]any{
		"id":       "task_1",
		"video_id": "video_1",
		"status":   "failed",
		"progress": float64(75),
		"error":    "Generation failed upstream",
	}

	result := normalizePluginResponse(&Plugin{ID: agnesVideoPluginID}, response)

	want := map[string]any{
		"taskId":           "task_1",
		"videoId":          "video_1",
		"status":           "failed",
		"generationStatus": "failed",
		"progress":         float64(75),
		"seconds":          nil,
		"size":             nil,
		"videoUrl":         nil,
		"error":            "Generation failed upstream",
		"raw":              response,
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %#v, want %#v", result, want)
	}
}

func TestNormalizeUnsplashResponse(t *testing.T) {
	result := normalizePluginResponse(&Plugin{ID: unsplashPluginID}, map[string]any{
		"results": []any{map[string]any{
			"alt_description": "mountain",
			"created_at":      "2026-07-15T00:00:00Z",
			"likes":           float64(42),
			"urls":            map[string]any{"regular": "https://images.example/mountain.jpg"},
		}},
	})

	want := []any{map[string]any{
		"alt_description": "mountain",
		"created_at":      "2026-07-15T00:00:00Z",
		"likes":           float64(42),
		"url":             "https://images.example/mountain.jpg",
	}}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %#v, want %#v", result, want)
	}
}
