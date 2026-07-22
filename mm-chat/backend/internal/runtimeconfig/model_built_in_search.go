package runtimeconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/websearch"
)

const (
	ModelBuiltInSearchProtocolOpenAIResponses = "openai_responses"
	ModelBuiltInSearchProtocolGeminiGoogle    = "gemini_google_search"
	ModelBuiltInSearchProtocolAnthropicWeb    = "anthropic_web_search"

	modelBuiltInSearchFingerprintVersion = "model-built-in-search/v1"
	modelBuiltInSearchTestTimeout        = 45 * time.Second
)

func officialModelBuiltInSearchProtocol(providerType ProviderType) string {
	switch providerType {
	case ProviderTypeOpenAI:
		return ModelBuiltInSearchProtocolOpenAIResponses
	case ProviderTypeGemini:
		return ModelBuiltInSearchProtocolGeminiGoogle
	case ProviderTypeAnthropic:
		return ModelBuiltInSearchProtocolAnthropicWeb
	default:
		return ""
	}
}

func modelBuiltInProviderID(protocol string) websearch.ModelBuiltInProviderID {
	switch strings.TrimSpace(protocol) {
	case ModelBuiltInSearchProtocolOpenAIResponses:
		return websearch.ModelBuiltInOpenAI
	case ModelBuiltInSearchProtocolGeminiGoogle:
		return websearch.ModelBuiltInGemini
	case ModelBuiltInSearchProtocolAnthropicWeb:
		return websearch.ModelBuiltInAnthropic
	default:
		return ""
	}
}

func modelSupportsOfficialBuiltInSearch(providerType ProviderType, modelID string) bool {
	model := strings.ToLower(strings.TrimSpace(modelID))
	if model == "" {
		return false
	}
	switch providerType {
	case ProviderTypeOpenAI:
		if strings.Contains(model, "image") || strings.Contains(model, "audio") ||
			strings.Contains(model, "realtime") || strings.Contains(model, "embedding") ||
			strings.Contains(model, "transcri") || strings.Contains(model, "tts") {
			return false
		}
		return strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "o1") ||
			strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4") ||
			strings.HasPrefix(model, "chatgpt-")
	case ProviderTypeGemini:
		if !strings.HasPrefix(model, "gemini-") || strings.Contains(model, "image") ||
			strings.Contains(model, "embedding") || strings.Contains(model, "tts") ||
			strings.Contains(model, "native-audio") {
			return false
		}
		return true
	case ProviderTypeAnthropic:
		return strings.HasPrefix(model, "claude-")
	default:
		return false
	}
}

func normalizeCustomModelBuiltInSearch(protocol string, model string) (string, string, error) {
	protocol = strings.TrimSpace(protocol)
	model = strings.TrimSpace(model)
	if protocol == "" {
		return "", "", nil
	}
	if protocol != ModelBuiltInSearchProtocolOpenAIResponses || model == "" || len(model) > 256 {
		return "", "", ErrModelBuiltInSearchUnsupported
	}
	return protocol, model, nil
}

func ModelBuiltInSearchConnectionTestValid(stored StoredProviderConfig) bool {
	if !IsModelProviderConfig(stored) || stored.Config.Type != ProviderTypeOpenAICompatible {
		return false
	}
	protocol, model, err := normalizeCustomModelBuiltInSearch(
		stored.Config.ModelBuiltInSearchProtocol,
		stored.Config.ModelBuiltInSearchModel,
	)
	if err != nil || protocol == "" || !modelListContains(stored.Config.Models, model) {
		return false
	}
	storedFingerprint := strings.TrimSpace(stored.Config.ModelBuiltInSearchTestSHA256)
	if storedFingerprint == "" || strings.TrimSpace(stored.Config.ModelBuiltInSearchTestedAt) == "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, stored.Config.ModelBuiltInSearchTestedAt); err != nil {
		return false
	}
	expected := modelBuiltInSearchFingerprint(stored, protocol, model)
	return subtleStringEqual(storedFingerprint, expected)
}

func modelBuiltInSearchFingerprint(stored StoredProviderConfig, protocol string, model string) string {
	parts := []string{
		modelBuiltInSearchFingerprintVersion,
		strings.TrimSpace(stored.ProviderID),
		string(normalizeProviderType(string(stored.Config.Type))),
		normalizeProviderBaseURL(stored.Config.BaseURL, stored.Config.Type),
		strings.TrimSpace(stored.EncryptedSecretRef),
		strings.TrimSpace(protocol),
		strings.TrimSpace(model),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func (s *Service) TestAdminModelBuiltInSearchConnection(
	ctx context.Context,
	providerID string,
	request TestAdminModelBuiltInSearchRequest,
) (AdminModelBuiltInSearchConnectionResponse, error) {
	if s == nil || s.repo == nil {
		return AdminModelBuiltInSearchConnectionResponse{}, ErrDatabaseRequired
	}
	if s.modelBuiltInSearchTester == nil {
		return AdminModelBuiltInSearchConnectionResponse{}, ErrModelBuiltInSearchUnsupported
	}
	providerID = strings.TrimSpace(providerID)
	stored, provider, err := s.loadProviderForConnectionTest(ctx, providerID)
	if err != nil {
		return AdminModelBuiltInSearchConnectionResponse{}, err
	}
	if stored.Config.Type != ProviderTypeOpenAICompatible || !ProviderConnectionTestValid(stored) {
		return AdminModelBuiltInSearchConnectionResponse{}, ErrModelBuiltInSearchUnsupported
	}
	protocol, model, err := normalizeCustomModelBuiltInSearch(request.Protocol, request.Model)
	if err != nil || protocol == "" || !modelListContains(stored.Config.Models, model) ||
		stored.Config.ModelBuiltInSearchProtocol != protocol ||
		stored.Config.ModelBuiltInSearchModel != model {
		return AdminModelBuiltInSearchConnectionResponse{}, ErrModelBuiltInSearchUnsupported
	}

	testCtx, cancel := context.WithTimeout(ctx, modelBuiltInSearchTestTimeout)
	sourceCount, testErr := s.modelBuiltInSearchTester.TestModelBuiltInSearch(
		testCtx,
		ModelBuiltInSearchTestInput{
			ProviderID: providerID,
			Type:       stored.Config.Type,
			BaseURL:    stored.Config.BaseURL,
			APIKey:     provider.APIKey,
			Protocol:   protocol,
			Model:      model,
		},
	)
	cancel()
	provider.APIKey = ""
	if testErr != nil || sourceCount <= 0 {
		return AdminModelBuiltInSearchConnectionResponse{}, ErrModelBuiltInSearchTestFailed
	}

	fingerprint := modelBuiltInSearchFingerprint(stored, protocol, model)
	committed, err := s.repo.CommitModelBuiltInSearchConnection(
		ctx,
		CommitModelBuiltInSearchConnectionInput{
			ID: stored.ID, UserID: stored.UserID, ProviderID: stored.ProviderID,
			ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
			ExpectedType:               stored.Config.Type,
			ExpectedBaseURL:            stored.Config.BaseURL,
			ExpectedProtocol:           protocol,
			ExpectedModel:              model,
			ConnectionTestSHA256:       fingerprint,
			ConnectionTestedAt:         time.Now().UTC(),
		},
	)
	if err != nil {
		if errors.Is(err, ErrProviderConfigChanged) {
			return AdminModelBuiltInSearchConnectionResponse{}, ErrModelBuiltInSearchConfigChanged
		}
		return AdminModelBuiltInSearchConnectionResponse{}, err
	}

	source := "server-stored"
	resolved := s.resolveStoredProvider(committed)
	if committed.ProviderID == serverDefaultProviderID {
		source = "server-default"
		resolved = s.resolveStoredServerDefault(committed)
	}
	return AdminModelBuiltInSearchConnectionResponse{
		Provider:    adminProviderResponse(resolved, committed.ProviderID, source),
		SourceCount: sourceCount,
	}, nil
}

// ResolveActive remains the legacy external-only resolver entry point used by
// the standalone /v1/search route.
func (s *Service) ResolveActive(ctx context.Context) (websearch.ActiveExecution, error) {
	return s.ResolveExternal(ctx)
}

func (s *Service) ResolveModelBuiltIn(
	ctx context.Context,
	request websearch.ModelBuiltInResolutionRequest,
) (websearch.ActiveExecution, error) {
	if s == nil || s.repo == nil {
		return websearch.ActiveExecution{}, websearch.ErrNotConfigured
	}
	providerID := strings.TrimSpace(request.ProviderID)
	modelID := strings.TrimSpace(request.ModelID)
	if providerID == "" || modelID == "" || request.Protocol == "" {
		return websearch.ActiveExecution{}, websearch.ErrNotConfigured
	}
	stored, ok, err := s.repo.GetProviderConfig(
		ctx,
		auth.UserOrDevelopment(ctx).ID,
		providerID,
	)
	if err != nil {
		return websearch.ActiveExecution{}, websearch.ErrResolutionFailed
	}
	if !ok || !IsModelProviderConfig(stored) || !stored.Config.Enabled ||
		!ProviderConnectionTestValid(stored) || !modelListContains(stored.Config.Models, modelID) {
		return websearch.ActiveExecution{}, websearch.ErrNotConfigured
	}
	apiKey, err := s.decryptStoredProviderSecret(stored, stored.Config.Type)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		return websearch.ActiveExecution{}, websearch.ErrResolutionFailed
	}
	apiKey = ""

	protocol := officialModelBuiltInSearchProtocol(stored.Config.Type)
	if protocol != "" {
		if !modelSupportsOfficialBuiltInSearch(stored.Config.Type, modelID) ||
			modelBuiltInProviderID(protocol) != request.Protocol {
			return websearch.ActiveExecution{}, websearch.ErrNotConfigured
		}
	} else {
		if stored.Config.Type != ProviderTypeOpenAICompatible ||
			!ModelBuiltInSearchConnectionTestValid(stored) ||
			stored.Config.ModelBuiltInSearchModel != modelID ||
			modelBuiltInProviderID(stored.Config.ModelBuiltInSearchProtocol) != request.Protocol {
			return websearch.ActiveExecution{}, websearch.ErrNotConfigured
		}
	}
	return websearch.ActiveExecution{
		Mode: websearch.ExecutionModelBuiltIn, ModelBuiltIn: request.Protocol,
	}, nil
}

func modelListContains(models []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, model := range models {
		if strings.TrimSpace(model) == target {
			return true
		}
	}
	return false
}
