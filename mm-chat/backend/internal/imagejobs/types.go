package imagejobs

type ModelRef struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

type GenerateRequest struct {
	ModelRef ModelRef `json:"modelRef"`
	Prompt   string   `json:"prompt"`
	Size     string   `json:"size,omitempty"`
	Count    int      `json:"count,omitempty"`
}

type GeneratedImage struct {
	FileID      string `json:"fileId,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

type GenerateResponse struct {
	Images  []GeneratedImage `json:"images"`
	Message string           `json:"message"`
}
