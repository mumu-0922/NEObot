package runtimeconfig

type ProviderType string

const (
	ProviderTypeOpenAICompatible ProviderType = "OpenAI Compatible"
	ProviderTypeOpenAI           ProviderType = "OpenAI"
	ProviderTypeGemini           ProviderType = "Gemini"
)

type PublicConfig struct {
	ModelProvider ModelProviderConfig `json:"modelProvider"`
	Search        SearchConfig        `json:"search"`
	RAG           RAGConfig           `json:"rag"`
	Voice         VoiceConfig         `json:"voice"`
	Deployment    DeploymentConfig    `json:"deployment"`
}

type ModelProviderConfig struct {
	Available     bool              `json:"available"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          ProviderType      `json:"type"`
	Models        []string          `json:"models"`
	ModelMetadata map[string]any    `json:"modelMetadata"`
	DefaultModels map[string]string `json:"defaultModels"`
}

type SearchConfig struct {
	Available bool `json:"available"`
}

type RAGConfig struct {
	VectorStoreAvailable        bool `json:"vectorStoreAvailable"`
	DocumentProcessingAvailable bool `json:"documentProcessingAvailable"`
}

type VoiceConfig struct {
	ElevenLabsAvailable bool `json:"elevenLabsAvailable"`
	MimoAvailable       bool `json:"mimoAvailable"`
	DefaultSTTAvailable bool `json:"defaultSttAvailable"`
	DefaultTTSAvailable bool `json:"defaultTtsAvailable"`
}

type DeploymentConfig struct {
	Mode                    string `json:"mode"`
	AccessPasswordEnabled   bool   `json:"accessPasswordEnabled"`
	TrustedProxyHeaders     bool   `json:"trustedProxyHeaders"`
	BYOKStableKeyConfigured bool   `json:"byokStableKeyConfigured"`
	BYOKEphemeralAllowed    bool   `json:"byokEphemeralAllowed"`
	RateLimitStore          string `json:"rateLimitStore"`
	DocumentParseJobStore   string `json:"documentParseJobStore"`
	PluginRegistryStore     string `json:"pluginRegistryStore"`
}

type ProviderModelsRequest struct {
	Provider ProviderRuntimeConfig `json:"provider"`
}

type ProviderRuntimeConfig struct {
	Type         string         `json:"type"`
	Name         string         `json:"name"`
	Source       string         `json:"source"`
	BaseURL      string         `json:"baseUrl"`
	APIKey       string         `json:"apiKey"`
	APIKeySecret map[string]any `json:"apiKeySecret"`
}

type ProviderModelsResponse struct {
	Models []string `json:"models"`
}

type BYOKPublicKeyResponse struct {
	KID          string         `json:"kid"`
	Alg          string         `json:"alg"`
	PublicKeyJWK map[string]any `json:"publicKeyJwk"`
}
