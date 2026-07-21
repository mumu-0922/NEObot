package runtimeconfig

import (
	"context"
	"errors"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

type ConfigureAdminRAGProviderRequest struct {
	APIKeySecret map[string]any `json:"apiKeySecret"`
}

type ConfigureRAGProviderConnectionInput struct {
	UserID                     string
	ProviderID                 string
	Label                      string
	EncryptedSecretRef         string
	RAGProvider                string
	ConnectionTestSHA256       string
	ConnectionTestedAt         time.Time
	ExpectedExists             bool
	ExpectedID                 string
	ExpectedEncryptedSecretRef string
	ExpectedRAGProvider        string
	ExpectedEnabled            bool
}

type ragProviderConfigurationRepository interface {
	ConfigureRAGProviderConnection(
		context.Context,
		ConfigureRAGProviderConnectionInput,
	) (StoredProviderConfig, error)
}

func (s *Service) ConfigureAdminRAGProvider(
	ctx context.Context,
	providerValue string,
	request ConfigureAdminRAGProviderRequest,
) (AdminRAGProviderConnectionResponse, error) {
	if s.repo == nil {
		return AdminRAGProviderConnectionResponse{}, ErrDatabaseRequired
	}
	configurationRepo, ok := s.repo.(ragProviderConfigurationRepository)
	if !ok {
		return AdminRAGProviderConnectionResponse{}, ErrDatabaseRequired
	}
	providerID, err := normalizeRAGProviderID(providerValue)
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	if len(request.APIKeySecret) == 0 {
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderSecretRequired
	}
	envelope, err := parseEncryptedSecretEnvelope(request.APIKeySecret)
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	apiKey, err := s.DecryptOptionalSecret(
		envelope,
		ragProviderIngressContext(providerID),
	)
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	if !validRAGAPIKey(apiKey) {
		apiKey = ""
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderConnectionFailed
	}
	if s.providerSecrets == nil {
		apiKey = ""
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderSecretVaultUnavailable
	}

	recordID := ragProviderRecordID(providerID)
	userID := auth.UserOrDevelopment(ctx).ID
	current, hasCurrent, err := s.repo.GetProviderConfig(ctx, userID, recordID)
	if err != nil {
		apiKey = ""
		return AdminRAGProviderConnectionResponse{}, err
	}
	if hasCurrent {
		if _, err := validateStoredRAGProvider(current); err != nil {
			apiKey = ""
			return AdminRAGProviderConnectionResponse{}, err
		}
	}

	testCtx, cancel := context.WithTimeout(ctx, ragConnectionTestTimeout)
	checks, err := s.testRAGProviderConnection(testCtx, providerID, apiKey)
	cancel()
	if err != nil {
		apiKey = ""
		return AdminRAGProviderConnectionResponse{}, ErrRAGProviderConnectionFailed
	}
	secretRef, err := s.encryptRAGProviderSecretAtRest(userID, recordID, apiKey)
	apiKey = ""
	if err != nil {
		return AdminRAGProviderConnectionResponse{}, err
	}
	testedAt := time.Now().UTC()
	configured, err := configurationRepo.ConfigureRAGProviderConnection(
		ctx,
		ConfigureRAGProviderConnectionInput{
			UserID: userID, ProviderID: recordID,
			Label:              ragProviderDefaultName(providerID),
			EncryptedSecretRef: secretRef,
			RAGProvider:        string(providerID),
			ConnectionTestSHA256: ragProviderConnectionFingerprint(
				recordID,
				providerID,
				secretRef,
			),
			ConnectionTestedAt:         testedAt,
			ExpectedExists:             hasCurrent,
			ExpectedID:                 current.ID,
			ExpectedEncryptedSecretRef: current.EncryptedSecretRef,
			ExpectedRAGProvider:        current.Config.RAGProvider,
			ExpectedEnabled:            current.Config.Enabled,
		},
	)
	if err != nil {
		if errors.Is(err, ErrProviderConfigChanged) {
			return AdminRAGProviderConnectionResponse{}, ErrRAGProviderConfigChanged
		}
		return AdminRAGProviderConnectionResponse{}, err
	}
	return AdminRAGProviderConnectionResponse{
		Provider: adminRAGProviderResponse(configured, providerID),
		Checks:   checks,
	}, nil
}
