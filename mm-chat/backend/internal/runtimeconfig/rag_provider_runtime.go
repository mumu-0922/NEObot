package runtimeconfig

import (
	"context"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

// ResolveRAGProviderCredential is an in-process runtime seam. It never has an
// HTTP route and only resolves one fixed, enabled, attested RAG record.
func (s *Service) ResolveRAGProviderCredential(
	ctx context.Context,
	providerValue string,
) (string, error) {
	if s == nil || s.repo == nil {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	providerID, err := normalizeRAGProviderID(providerValue)
	if err != nil {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	userID := auth.UserOrDevelopment(ctx).ID
	recordID := ragProviderRecordID(providerID)
	stored, found, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	if !found {
		return "", ragproviders.ErrProviderGatewayNotFound
	}
	resolvedProvider, err := validateStoredRAGProvider(stored)
	if err != nil || resolvedProvider != providerID || stored.UserID != userID ||
		stored.ProviderID != recordID {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	if !stored.Config.Enabled || !RAGProviderConnectionTestValid(stored) {
		return "", ragproviders.ErrProviderGatewayActivationRequired
	}
	credential, err := s.decryptStoredRAGProviderSecret(stored)
	if err != nil || !validRAGAPIKey(credential) {
		credential = ""
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	return strings.TrimSpace(credential), nil
}

// ResolveHydratedStoredRAGProviderCredential validates and decrypts one
// capability-hydrated provider row. It lets the isolated Memory worker retain
// least-privilege table denial while applying the same enabled/attested/vault
// checks as the API runtime.
func (s *Service) ResolveHydratedStoredRAGProviderCredential(
	stored StoredProviderConfig,
) (string, error) {
	stored.ID = strings.TrimSpace(stored.ID)
	stored.UserID = strings.TrimSpace(stored.UserID)
	stored.ProviderID = strings.TrimSpace(stored.ProviderID)
	if s == nil || stored.ID == "" || stored.UserID == "" || stored.ProviderID == "" {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	providerID, err := validateStoredRAGProvider(stored)
	if err != nil || providerID != RAGProviderSiliconFlow ||
		stored.ProviderID != ragProviderRecordID(providerID) {
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	if !stored.Config.Enabled || !RAGProviderConnectionTestValid(stored) {
		return "", ragproviders.ErrProviderGatewayActivationRequired
	}
	credential, err := s.decryptStoredRAGProviderSecret(stored)
	if err != nil || !validRAGAPIKey(credential) {
		credential = ""
		return "", ragproviders.ErrProviderGatewayUnavailable
	}
	return strings.TrimSpace(credential), nil
}

var _ ragproviders.ProviderCredentialResolver = (*Service)(nil)
