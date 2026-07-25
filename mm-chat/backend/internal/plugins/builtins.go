package plugins

func BuiltInPlugins() []Plugin {
	return []Plugin{
		{
			ID:          "weather-gpt",
			Title:       "Real-time Weather",
			Description: "Get real-time weather information for any city.",
			LogoURL:     "https://cdn.weatherapi.com/v4/images/weatherapi_logo.png",
			ManifestURL: "https://openai-collections.chat-plugin.lobehub.com/weather-gpt/openapi.json",
			BaseURL:     "https://weathergpt.vercel.app",
			Category:    "Utilities",
			BuiltIn:     true,
			Functions: []PluginFunction{
				{
					Name:        "getCurrentWeather",
					Description: "Get the current weather for a specific city.",
					Method:      "GET",
					Path:        "/api/weather",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The city name to get weather for.",
							},
						},
						"required": []any{"location"},
					},
				},
			},
			Auth: &PluginAuth{Type: "none"},
		},
		{
			ID:          "unsplash",
			Title:       "Unsplash",
			Description: "Search for high-quality photos on Unsplash.",
			LogoURL:     "https://unsplash.com/apple-touch-icon.png",
			BaseURL:     "https://api.unsplash.com",
			Category:    "Image Search",
			BuiltIn:     true,
			Functions: []PluginFunction{
				{
					Name:        "search_photos",
					Description: "Search photos on Unsplash.",
					Method:      "GET",
					Path:        "/search/photos",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{
								"type":        "string",
								"description": "The search terms.",
							},
							"page": map[string]any{
								"type":        "integer",
								"description": "Page number to retrieve.",
							},
							"per_page": map[string]any{
								"type":        "integer",
								"description": "Number of items per page.",
							},
						},
						"required": []any{"query"},
					},
				},
			},
			Auth: &PluginAuth{Type: "apiKey", Name: "client_id", In: "query"},
		},
		{
			ID:              "agnes-image-generation",
			Title:           "Agnes Image Generation",
			Description:     "Generate images with Agnes Image 2.1 Flash.",
			LogoURL:         "https://agnes-ai.com/images/logo.png",
			ExternalDocsURL: "https://agnes-ai.com/en/docs/agnes-image-21-flash",
			BaseURL:         "https://apihub.agnes-ai.com",
			Category:        "Image Generation",
			BuiltIn:         true,
			Functions: []PluginFunction{
				{
					Name:        "generate_image",
					Description: "Generate or edit an image with Agnes Image 2.1 Flash.",
					Method:      "POST",
					Path:        "/v1/images/generations",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt": map[string]any{"type": "string"},
							"size":   map[string]any{"type": "string"},
						},
						"required": []any{"prompt", "size"},
					},
				},
			},
			Auth: &PluginAuth{Type: "bearer", Required: boolPtr(true)},
		},
		{
			ID:              "agnes-video-generation",
			Title:           "Agnes Video Generation",
			Description:     "Create and retrieve video generation tasks with Agnes Video V2.0.",
			LogoURL:         "https://agnes-ai.com/images/logo.png",
			ExternalDocsURL: "https://agnes-ai.com/en/docs/agnes-video-v20",
			BaseURL:         "https://apihub.agnes-ai.com",
			Category:        "Video Generation",
			BuiltIn:         true,
			Functions: []PluginFunction{
				{
					Name:        "create_video",
					Description: "Create an asynchronous Agnes Video V2.0 generation task.",
					Method:      "POST",
					Path:        "/v1/videos",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{"prompt": map[string]any{"type": "string"}},
						"required":   []any{"prompt"},
					},
				},
				{
					Name:        "get_video_result",
					Description: "Retrieve an Agnes Video V2.0 generation status or result.",
					Method:      "GET",
					Path:        "/agnesapi",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"video_id": map[string]any{"type": "string"},
							"task_id":  map[string]any{"type": "string"},
						},
					},
				},
			},
			Auth: &PluginAuth{Type: "bearer", Required: boolPtr(true)},
		},
	}
}

func boolPtr(value bool) *bool {
	return &value
}
