package runtimeconfig

import "strings"

const (
	providerConfigKindVoice   = "voice"
	voiceProviderRecordPrefix = "VOICE:"
)

type voiceProviderID string

const (
	voiceProviderElevenLabs voiceProviderID = "elevenlabs"
	voiceProviderMimo       voiceProviderID = "mimo"
)

var reservedVoiceProviderIDs = [...]voiceProviderID{
	voiceProviderElevenLabs,
	voiceProviderMimo,
}

func normalizeVoiceProviderID(value string) (voiceProviderID, bool) {
	providerID := voiceProviderID(strings.ToLower(strings.TrimSpace(value)))
	for _, supported := range reservedVoiceProviderIDs {
		if providerID == supported {
			return providerID, true
		}
	}
	return "", false
}

func voiceProviderRecordID(providerID voiceProviderID) string {
	return voiceProviderRecordPrefix + strings.ToUpper(string(providerID))
}

func isReservedVoiceProviderRecordID(providerID string) bool {
	return strings.HasPrefix(
		strings.ToUpper(strings.TrimSpace(providerID)),
		voiceProviderRecordPrefix,
	)
}

func voiceProviderIngressContext(providerID voiceProviderID) string {
	return "provider:voice:" + string(providerID)
}

func voiceProviderSecretContext(userID string, recordID string) string {
	return "provider:voice:" + strings.TrimSpace(userID) + ":" + strings.TrimSpace(recordID)
}
