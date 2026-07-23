package runtimeconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func parseToolCapabilityOverride(value string) (ToolCapabilityOverride, bool) {
	switch ToolCapabilityOverride(strings.ToLower(strings.TrimSpace(value))) {
	case ToolCapabilityAuto:
		return ToolCapabilityAuto, true
	case ToolCapabilityEnabled:
		return ToolCapabilityEnabled, true
	case ToolCapabilityDisabled:
		return ToolCapabilityDisabled, true
	default:
		return "", false
	}
}

func normalizeToolCapabilityOverride(
	value ToolCapabilityOverride,
) ToolCapabilityOverride {
	if normalized, ok := parseToolCapabilityOverride(string(value)); ok {
		return normalized
	}
	return ToolCapabilityAuto
}

func normalizeToolCapabilityModelOverrides(
	values map[string]string,
	models []string,
) (map[string]ToolCapabilityOverride, error) {
	available := make(map[string]struct{}, len(models))
	for _, model := range models {
		available[model] = struct{}{}
	}
	result := make(map[string]ToolCapabilityOverride, len(values))
	for rawModel, rawValue := range values {
		model := strings.TrimSpace(rawModel)
		if model == "" || len(model) > 512 {
			return nil, ErrProviderConfigUnsupported
		}
		if _, ok := available[model]; !ok {
			return nil, ErrProviderConfigUnsupported
		}
		value, ok := parseToolCapabilityOverride(rawValue)
		if !ok {
			return nil, ErrProviderConfigUnsupported
		}
		if value == ToolCapabilityAuto {
			continue
		}
		result[model] = value
	}
	return result, nil
}

func filterToolCapabilityModelOverrides(
	values map[string]ToolCapabilityOverride,
	models []string,
) map[string]ToolCapabilityOverride {
	available := make(map[string]struct{}, len(models))
	for _, model := range models {
		available[model] = struct{}{}
	}
	result := make(map[string]ToolCapabilityOverride, len(values))
	for model, rawValue := range values {
		if _, ok := available[model]; !ok {
			continue
		}
		value := normalizeToolCapabilityOverride(rawValue)
		if value != ToolCapabilityAuto {
			result[model] = value
		}
	}
	return result
}

func cloneToolCapabilityOverrides(
	values map[string]ToolCapabilityOverride,
) map[string]ToolCapabilityOverride {
	result := make(map[string]ToolCapabilityOverride, len(values))
	for model, value := range values {
		value = normalizeToolCapabilityOverride(value)
		if value != ToolCapabilityAuto {
			result[model] = value
		}
	}
	return result
}

func toolCapabilityConfigHash(stored StoredProviderConfig) string {
	secretHash := sha256.Sum256([]byte(strings.TrimSpace(stored.EncryptedSecretRef)))
	payload := struct {
		UserID         string                            `json:"userId"`
		ProviderID     string                            `json:"providerId"`
		Type           ProviderType                      `json:"type"`
		BaseURL        string                            `json:"baseUrl"`
		Models         []string                          `json:"models"`
		SecretSHA256   string                            `json:"secretSha256"`
		ConnectionHash string                            `json:"connectionHash"`
		Default        ToolCapabilityOverride            `json:"default"`
		Overrides      map[string]ToolCapabilityOverride `json:"overrides"`
	}{
		UserID:         strings.TrimSpace(stored.UserID),
		ProviderID:     strings.TrimSpace(stored.ProviderID),
		Type:           stored.Config.Type,
		BaseURL:        strings.TrimRight(strings.TrimSpace(stored.Config.BaseURL), "/"),
		Models:         normalizeModelList(stored.Config.Models),
		SecretSHA256:   hex.EncodeToString(secretHash[:]),
		ConnectionHash: strings.TrimSpace(stored.Config.ConnectionTestSHA256),
		Default:        normalizeToolCapabilityOverride(stored.Config.ToolCapabilityDefault),
		Overrides:      cloneToolCapabilityOverrides(stored.Config.ToolCapabilityModelOverrides),
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (provider ResolvedProvider) ToolCapabilityForModel(
	modelID string,
) ToolCapabilityOverride {
	if value, ok := provider.ToolCapabilityModelOverrides[strings.TrimSpace(modelID)]; ok {
		return normalizeToolCapabilityOverride(value)
	}
	return normalizeToolCapabilityOverride(provider.ToolCapabilityDefault)
}
