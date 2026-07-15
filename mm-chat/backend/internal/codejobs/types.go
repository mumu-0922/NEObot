package codejobs

type ModelRef struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelId"`
}

type ExecuteRequest struct {
	ModelRef ModelRef `json:"modelRef"`
	Language string   `json:"language"`
	Code     string   `json:"code"`
}

type ExecuteResponse struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}
