package runtimeconfig

import (
	"context"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func (s *Service) RAGProviderStatus(
	ctx context.Context,
) (ragproviders.StatusResponse, error) {
	response := ragproviders.Status(config.RAGConfig{
		JinaEmbeddingDimensions: config.DefaultRAGJinaDimensions,
	})
	if s == nil || s.repo == nil {
		return response, nil
	}
	stored, err := s.repo.ListProviderConfigs(ctx, auth.UserOrDevelopment(ctx).ID)
	if err != nil {
		return ragproviders.StatusResponse{}, err
	}
	seen := make(map[RAGProviderID]struct{})
	for _, item := range stored {
		if !isRAGProviderConfig(item) {
			continue
		}
		providerID, err := validateStoredRAGProvider(item)
		if err != nil {
			return ragproviders.StatusResponse{}, err
		}
		if _, duplicate := seen[providerID]; duplicate {
			return ragproviders.StatusResponse{}, ErrRAGProviderConfigUnsupported
		}
		seen[providerID] = struct{}{}
		state := ragproviders.ProviderState{
			Configured: strings.TrimSpace(item.EncryptedSecretRef) != "",
			Status:     ragproviders.ProviderStatusMissingSecret,
		}
		if providerID == RAGProviderJina {
			state.EmbeddingDimensions = config.DefaultRAGJinaDimensions
		}
		if state.Configured {
			state.Status = ragproviders.ProviderStatusActivationRequired
		}
		if item.Config.Enabled && RAGProviderConnectionTestValid(item) {
			secret, err := s.decryptStoredRAGProviderSecret(item)
			if err != nil || !validRAGAPIKey(secret) {
				state.Status = ragproviders.ProviderStatusUnavailable
			} else {
				state.Status = ragproviders.ProviderStatusReady
			}
			secret = ""
		}
		if providerID == RAGProviderMinerU {
			response.Providers.MinerU = state
		} else {
			response.Providers.Jina = state
		}
	}
	response.Ready = response.Providers.MinerU.Status == ragproviders.ProviderStatusReady &&
		response.Providers.Jina.Status == ragproviders.ProviderStatusReady
	return response, nil
}
