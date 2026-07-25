package runtimeconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/providersecrets"
)

func RAGProviderConnectionTestValid(stored StoredProviderConfig) bool {
	if !isRAGProviderConfig(stored) {
		return false
	}
	if isRetiredJinaRAGProvider(stored) {
		return false
	}
	providerID, err := validateStoredRAGProvider(stored)
	if err != nil {
		return false
	}
	return ragProviderConnectionTestValidForValues(
		stored.ProviderID,
		providerID,
		stored.EncryptedSecretRef,
		stored.Config.ConnectionTestSHA256,
		stored.Config.ConnectionTestedAt,
	)
}

func ragProviderConnectionTestValidForValues(
	recordID string,
	providerID RAGProviderID,
	secretRef string,
	fingerprint string,
	testedAt string,
) bool {
	if strings.TrimSpace(secretRef) == "" || strings.TrimSpace(fingerprint) == "" ||
		strings.TrimSpace(testedAt) == "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(testedAt)); err != nil {
		return false
	}
	return strings.TrimSpace(fingerprint) == ragProviderConnectionFingerprint(
		recordID,
		providerID,
		secretRef,
	)
}

func ragProviderConnectionFingerprint(
	recordID string,
	providerID RAGProviderID,
	secretRef string,
) string {
	parts := []string{
		ragConnectionFingerprintVersion,
		strings.TrimSpace(recordID),
		string(providerID),
	}
	if providerID == RAGProviderMinerU {
		parts = append(parts, minerUAllocateURL, minerUModelVersion)
	} else if providerID == RAGProviderSiliconFlow {
		parts = append(
			parts,
			siliconFlowEmbeddingsURL,
			siliconFlowEmbeddingModel,
			"1024",
			siliconFlowRerankURL,
			siliconFlowRerankModel,
		)
	}
	parts = append(parts, strings.TrimSpace(secretRef))
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func (s *Service) encryptRAGProviderSecretAtRest(
	userID string,
	recordID string,
	plaintext string,
) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return "", ErrRAGProviderSecretRequired
	}
	if s.providerSecrets == nil {
		return "", ErrRAGProviderSecretVaultUnavailable
	}
	secretBytes := []byte(plaintext)
	plaintext = ""
	envelope, err := s.providerSecrets.Encrypt(
		secretBytes,
		ragProviderSecretContext(userID, recordID),
	)
	clear(secretBytes)
	if err != nil {
		return "", ErrRAGProviderSecretInvalid
	}
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > maxStoredProviderSecretRefBytes {
		return "", ErrRAGProviderSecretInvalid
	}
	return string(encoded), nil
}

func (s *Service) decryptStoredRAGProviderSecret(
	stored StoredProviderConfig,
) (string, error) {
	encoded := strings.TrimSpace(stored.EncryptedSecretRef)
	if encoded == "" {
		return "", nil
	}
	if storedSecretAlgorithm(encoded) != providersecrets.Algorithm || s.providerSecrets == nil {
		if s.providerSecrets == nil {
			return "", ErrRAGProviderSecretVaultUnavailable
		}
		return "", ErrRAGProviderSecretInvalid
	}
	envelope, err := providersecrets.ParseEnvelope(encoded)
	if err != nil {
		return "", ErrRAGProviderSecretInvalid
	}
	plaintext, err := s.providerSecrets.Decrypt(
		envelope,
		ragProviderSecretContext(stored.UserID, stored.ProviderID),
	)
	if err != nil {
		return "", ErrRAGProviderSecretInvalid
	}
	decrypted := strings.TrimSpace(string(plaintext))
	clear(plaintext)
	if decrypted == "" {
		return "", ErrRAGProviderSecretInvalid
	}
	return decrypted, nil
}
