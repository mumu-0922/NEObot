package runtimeconfig

import "time"

type ProviderType string

type ToolCapabilityOverride string

const (
	ProviderTypeOpenAICompatible ProviderType           = "OpenAI Compatible"
	ProviderTypeOpenAI           ProviderType           = "OpenAI"
	ProviderTypeGemini           ProviderType           = "Gemini"
	ProviderTypeAnthropic        ProviderType           = "Anthropic"
	ToolCapabilityAuto           ToolCapabilityOverride = "auto"
	ToolCapabilityEnabled        ToolCapabilityOverride = "enabled"
	ToolCapabilityDisabled       ToolCapabilityOverride = "disabled"
)

type PublicConfig struct {
	ModelProvider ModelProviderConfig `json:"modelProvider"`
	Search        SearchConfig        `json:"search"`
	Voice         VoiceConfig         `json:"voice"`
	Deployment    DeploymentConfig    `json:"deployment"`
}

type ModelProviderConfig struct {
	Available               bool              `json:"available"`
	ID                      string            `json:"id"`
	Name                    string            `json:"name"`
	Type                    ProviderType      `json:"type"`
	Models                  []string          `json:"models"`
	ModelMetadata           map[string]any    `json:"modelMetadata"`
	DefaultModels           map[string]string `json:"defaultModels"`
	DefaultModelsConfigured *bool             `json:"defaultModelsConfigured,omitempty"`
}

type SearchConfig struct {
	Available bool `json:"available"`
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
	PluginRegistryStore     string `json:"pluginRegistryStore"`
}

type ProviderModelsRequest struct {
	Provider ProviderRuntimeConfig `json:"provider"`
}

type AdminProviderConfigResponse struct {
	ID                  string                                `json:"id"`
	Name                string                                `json:"name"`
	Type                ProviderType                          `json:"type"`
	BaseURL             string                                `json:"baseUrl"`
	Models              []string                              `json:"models"`
	Enabled             bool                                  `json:"enabled"`
	HasAPIKey           bool                                  `json:"hasApiKey"`
	Source              string                                `json:"source"`
	ConnectionTestValid bool                                  `json:"connectionTestValid"`
	ConnectionTestedAt  *time.Time                            `json:"connectionTestedAt,omitempty"`
	ModelBuiltInSearch  AdminModelBuiltInSearchConfigResponse `json:"modelBuiltInSearch"`
	ToolCapability      AdminToolCapabilityConfigResponse     `json:"toolCapability"`
}

type AdminProviderConfigsResponse struct {
	Providers []AdminProviderConfigResponse `json:"providers"`
}

type UpdateAdminProviderConfigRequest struct {
	Name                         string            `json:"name"`
	Type                         string            `json:"type"`
	BaseURL                      string            `json:"baseUrl"`
	Models                       []string          `json:"models"`
	Enabled                      bool              `json:"enabled"`
	APIKeySecret                 map[string]any    `json:"apiKeySecret"`
	ClearAPIKey                  bool              `json:"clearApiKey"`
	ModelBuiltInSearchProtocol   *string           `json:"modelBuiltInSearchProtocol,omitempty"`
	ModelBuiltInSearchModel      *string           `json:"modelBuiltInSearchModel,omitempty"`
	ToolCapabilityDefault        *string           `json:"toolCapabilityDefault,omitempty"`
	ToolCapabilityModelOverrides map[string]string `json:"toolCapabilityModelOverrides,omitempty"`
}

type ProviderRuntimeConfig struct {
	ID           string         `json:"id"`
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

type AdminProviderConnectionResponse struct {
	Provider AdminProviderConfigResponse `json:"provider"`
	Models   []string                    `json:"models"`
}

type AdminModelBuiltInSearchConfigResponse struct {
	Protocol            string     `json:"protocol,omitempty"`
	Model               string     `json:"model,omitempty"`
	Source              string     `json:"source"`
	ConnectionTestValid bool       `json:"connectionTestValid"`
	ConnectionTestedAt  *time.Time `json:"connectionTestedAt,omitempty"`
}

type AdminToolCapabilityConfigResponse struct {
	Default        ToolCapabilityOverride            `json:"default"`
	ModelOverrides map[string]ToolCapabilityOverride `json:"modelOverrides"`
}

type TestAdminModelBuiltInSearchRequest struct {
	Protocol string `json:"protocol"`
	Model    string `json:"model"`
}

type AdminModelBuiltInSearchConnectionResponse struct {
	Provider    AdminProviderConfigResponse `json:"provider"`
	SourceCount int                         `json:"sourceCount"`
}

type BYOKPublicKeyResponse struct {
	KID          string         `json:"kid"`
	Alg          string         `json:"alg"`
	PublicKeyJWK map[string]any `json:"publicKeyJwk"`
}
