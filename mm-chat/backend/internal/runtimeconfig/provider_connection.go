package runtimeconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

const providerConnectionFingerprintVersion = "model-provider-connection/v1"

func ProviderConnectionTestValid(stored StoredProviderConfig) bool {
	return providerConnectionTestValidForValues(
		stored.ProviderID,
		stored.Config.Type,
		stored.Config.BaseURL,
		stored.EncryptedSecretRef,
		stored.Config.ConnectionTestSHA256,
		stored.Config.ConnectionTestedAt,
	)
}

func ProviderConnectionTestFingerprint(stored StoredProviderConfig) string {
	return providerConnectionFingerprint(
		stored.ProviderID,
		stored.Config.Type,
		stored.Config.BaseURL,
		stored.EncryptedSecretRef,
	)
}

func providerConnectionTestValidForValues(
	providerID string,
	providerType ProviderType,
	baseURL string,
	secretRef string,
	storedFingerprint string,
	testedAt string,
) bool {
	storedFingerprint = strings.TrimSpace(storedFingerprint)
	if storedFingerprint == "" || strings.TrimSpace(testedAt) == "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(testedAt)); err != nil {
		return false
	}
	expected := providerConnectionFingerprint(providerID, providerType, baseURL, secretRef)
	return subtleStringEqual(storedFingerprint, expected)
}

func providerConnectionFingerprint(
	providerID string,
	providerType ProviderType,
	baseURL string,
	secretRef string,
) string {
	normalizedType := normalizeProviderType(string(providerType))
	parts := []string{
		providerConnectionFingerprintVersion,
		strings.TrimSpace(providerID),
		string(normalizedType),
		normalizeProviderBaseURL(baseURL, normalizedType),
		strings.TrimSpace(secretRef),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func subtleStringEqual(left string, right string) bool {
	leftDigest := sha256.Sum256([]byte(left))
	rightDigest := sha256.Sum256([]byte(right))
	return leftDigest == rightDigest
}

func parseProviderConnectionTestState(
	provider resolvedServerDefaultProvider,
) (*time.Time, bool) {
	if !provider.ConnectionTestValid {
		return nil, false
	}
	testedAt, err := time.Parse(
		time.RFC3339Nano,
		strings.TrimSpace(provider.ConnectionTestedAt),
	)
	if err != nil {
		return nil, false
	}
	testedAt = testedAt.UTC()
	return &testedAt, true
}

func (s *Service) TestAdminProviderConnection(
	ctx context.Context,
	providerID string,
) (AdminProviderConnectionResponse, error) {
	return s.commitAdminProviderConnection(ctx, providerID, false)
}

func (s *Service) ActivateAdminProvider(
	ctx context.Context,
	providerID string,
) (AdminProviderConnectionResponse, error) {
	return s.commitAdminProviderConnection(ctx, providerID, true)
}

func (s *Service) commitAdminProviderConnection(
	ctx context.Context,
	providerID string,
	activate bool,
) (AdminProviderConnectionResponse, error) {
	stored, provider, err := s.loadProviderForConnectionTest(ctx, providerID)
	if err != nil {
		return AdminProviderConnectionResponse{}, err
	}
	models, err := s.fetchProviderModelsForConnectionTest(ctx, provider)
	if err != nil {
		if errors.Is(err, ErrProviderConfigUnsupported) ||
			errors.Is(err, ErrProviderSecretRequired) ||
			errors.Is(err, ErrProviderSecretInvalid) ||
			errors.Is(err, ErrProviderSecretVaultUnavailable) {
			return AdminProviderConnectionResponse{}, err
		}
		return AdminProviderConnectionResponse{}, ErrProviderConnectionTestFailed
	}

	fingerprint := providerConnectionFingerprint(
		stored.ProviderID,
		stored.Config.Type,
		stored.Config.BaseURL,
		stored.EncryptedSecretRef,
	)
	committed, err := s.repo.CommitProviderConnection(ctx, CommitProviderConnectionInput{
		ID:                         stored.ID,
		UserID:                     stored.UserID,
		ProviderID:                 stored.ProviderID,
		ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
		ExpectedType:               stored.Config.Type,
		ExpectedBaseURL:            stored.Config.BaseURL,
		ExpectedEnabled:            stored.Config.Enabled,
		ConnectionTestSHA256:       fingerprint,
		ConnectionTestedAt:         time.Now().UTC(),
		Enabled:                    activate || stored.Config.Enabled,
	})
	if err != nil {
		return AdminProviderConnectionResponse{}, err
	}

	source := "server-stored"
	resolved := s.resolveStoredProvider(committed)
	if committed.ProviderID == serverDefaultProviderID {
		source = "server-default"
		resolved = s.resolveStoredServerDefault(committed)
	}
	return AdminProviderConnectionResponse{
		Provider: adminProviderResponse(resolved, committed.ProviderID, source),
		Models:   append([]string(nil), models...),
	}, nil
}

func (s *Service) loadProviderForConnectionTest(
	ctx context.Context,
	providerID string,
) (StoredProviderConfig, resolvedServerDefaultProvider, error) {
	if s.repo == nil {
		return StoredProviderConfig{}, resolvedServerDefaultProvider{}, ErrDatabaseRequired
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" || len(providerID) > 128 {
		return StoredProviderConfig{}, resolvedServerDefaultProvider{}, ErrProviderConfigUnsupported
	}
	stored, ok, err := s.repo.GetProviderConfig(
		ctx,
		auth.UserOrDevelopment(ctx).ID,
		providerID,
	)
	if err != nil {
		return StoredProviderConfig{}, resolvedServerDefaultProvider{}, err
	}
	if !ok {
		return StoredProviderConfig{}, resolvedServerDefaultProvider{}, ErrProviderConfigNotFound
	}
	provider := s.resolveStoredProvider(stored)
	if providerID == serverDefaultProviderID {
		provider = s.resolveStoredServerDefault(stored)
	}
	if provider.SecretErr != nil {
		return StoredProviderConfig{}, resolvedServerDefaultProvider{}, provider.SecretErr
	}
	if strings.TrimSpace(provider.APIKey) == "" {
		return StoredProviderConfig{}, resolvedServerDefaultProvider{}, ErrProviderSecretRequired
	}
	return stored, provider, nil
}
