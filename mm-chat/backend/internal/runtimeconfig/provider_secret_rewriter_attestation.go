package runtimeconfig

import (
	"encoding/json"
	"strings"
)

func (r *PostgresProviderSecretRewriter) rewrittenProviderConnectionAttestation(
	row providerSecretRewriteRow,
	rewrittenSecretRef string,
) (string, bool, error) {
	var payload StoredProviderConfigPayload
	if err := json.Unmarshal([]byte(row.configJSON), &payload); err != nil {
		return "", false, ErrProviderSecretRewriteInvalid
	}
	stored := StoredProviderConfig{
		ID: row.id, UserID: row.userID, ProviderID: row.providerID,
		EncryptedSecretRef: row.encryptedSecretRef, Config: payload,
	}
	switch strings.TrimSpace(payload.Kind) {
	case "", providerConfigKindModel:
		if !IsModelProviderConfig(stored) {
			return "", false, ErrProviderSecretRewriteInvalid
		}
		if !ProviderConnectionTestValid(stored) {
			return "", false, nil
		}
		return providerConnectionFingerprint(
			stored.ProviderID,
			payload.Type,
			payload.BaseURL,
			rewrittenSecretRef,
		), true, nil
	case providerConfigKindSearch:
		providerID, err := validateStoredSearchProvider(stored)
		if err != nil {
			return "", false, ErrProviderSecretRewriteInvalid
		}
		if !SearchProviderConnectionTestValid(stored) {
			return "", false, nil
		}
		return searchProviderConnectionFingerprint(
			stored.ProviderID,
			providerID,
			payload.BaseURL,
			rewrittenSecretRef,
		), true, nil
	case providerConfigKindRAG:
		if isRetiredJinaRAGProvider(stored) {
			return "", false, nil
		}
		providerID, err := validateStoredRAGProvider(stored)
		if err != nil {
			return "", false, ErrProviderSecretRewriteInvalid
		}
		if !RAGProviderConnectionTestValid(stored) {
			return "", false, nil
		}
		return ragProviderConnectionFingerprint(
			stored.ProviderID,
			providerID,
			rewrittenSecretRef,
		), true, nil
	case providerConfigKindVoice:
		providerID, ok := normalizeVoiceProviderID(payload.VoiceProvider)
		if !ok || stored.ProviderID != voiceProviderRecordID(providerID) {
			return "", false, ErrProviderSecretRewriteInvalid
		}
		// F5 reserves only the at-rest identity. It must not invent a Voice
		// connection proof before a future provider-specific real test exists.
		return "", false, nil
	default:
		return "", false, ErrProviderSecretRewriteInvalid
	}
}
